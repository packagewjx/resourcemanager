package watcher

import (
	"context"
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/utils/k8s"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log"
	"os"
)

type K8sWatcherConfig struct {
	Host      string
	CaFile    string
	TokenFile string
	Insecure  bool // 指定是否忽略TSL证书错误
	NodeName  string
}

const DefaultHost = "https://localhost:8443"

type k8sWatcher struct {
	client      *kubernetes.Clientset
	podPidQuery k8s.PodPidQuery
	base        *baseChannelWatcher
}

func (p *k8sWatcher) StopWatch(ch <-chan *ProcessGroupStatus) {
	p.base.StopWatch(ch)
}

func (p *k8sWatcher) Watch() <-chan *ProcessGroupStatus {
	return p.base.Watch()
}

func NewK8sWatcher(ctx context.Context, config *K8sWatcherConfig) (ProcessGroupWatcher, error) {
	restConfig := &rest.Config{
		Host:            config.Host,
		BearerTokenFile: config.TokenFile,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: config.Insecure,
			CAFile:   config.CaFile,
		},
	}
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, errors.Wrap(err, "创建Kubernetes客户端发生错误")
	}

	podWatchInterface, err := client.CoreV1().Pods("").Watch(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", config.NodeName),
	})
	if err != nil {
		return nil, errors.Wrap(err, "创建PodWatchInterface出错")
	}
	node, err := client.CoreV1().Nodes().Get(context.TODO(), config.NodeName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "查询节点出错")
	}
	service, err := k8s.NewPodPidQuery(node)
	if err != nil {
		return nil, errors.Wrap(err, "创建PodPidQuery出错")
	}

	w := &k8sWatcher{
		client:      client,
		podPidQuery: service,
		base:        &baseChannelWatcher{channels: []chan *ProcessGroupStatus{}},
	}
	go w.run(ctx, podWatchInterface)
	return w, nil
}

func (p *k8sWatcher) run(ctx context.Context, watchInterface watch.Interface) {
	logger := log.New(os.Stdout, "K8S Watcher: ", log.LstdFlags|log.Lshortfile|log.Lmsgprefix)
	logger.Printf("Kubernetes监视器启动")

	for {
		select {
		case event := <-watchInterface.ResultChan():
			pod := event.Object.(*v1.Pod)
			pidList, err := p.podPidQuery.Query(pod)
			if err != nil {
				// FIXME 处理错误
				panic(err)
			}

			var condition ProcessGroupCondition
			switch event.Type {
			case watch.Added:
				condition = ProcessGroupStatusAdd
			case watch.Deleted, watch.Error:
				condition = ProcessGroupStatusRemove
			case watch.Modified:
				condition = ProcessGroupStatusUpdate
			}
			s := &ProcessGroupStatus{
				Group: core.ProcessGroup{
					Id:  pod.Name,
					Pid: pidList,
				},
				Status: condition,
			}
			p.base.notifyAll(s)

		case <-ctx.Done():
			logger.Println("Kubernetes监视器关闭")
			watchInterface.Stop()
			return
		}
	}
}
