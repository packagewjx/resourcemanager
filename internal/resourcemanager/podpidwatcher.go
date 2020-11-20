package resourcemanager

import (
	"context"
	"fmt"
	dockerclient "github.com/docker/docker/client"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"strings"
)

type changeFunc func(id string, oldPid, newPid []int)

type containerRuntime string

var (
	runtimeDocker containerRuntime = "docker"
)

var (
	ErrRuntimeUnknown = fmt.Errorf("无法确定容器运行时类型")
)

type podPidWatcher struct {
	ctx            context.Context
	watchInterface watch.Interface
	onChange       changeFunc
	podPidMap      map[string][]int
	podPidQuery    podPidService
}

func newPodPidWatcher(ctx context.Context, clientSet *kubernetes.Clientset, node *v1.Node, onChange changeFunc) (*podPidWatcher, error) {
	w, err := clientSet.CoreV1().Pods("").Watch(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", node.Name),
	})
	if err != nil {
		return nil, err
	}
	runtime, err := getRuntime(node)
	if err != nil {
		return nil, err
	}
	service, err := newPodPidService(runtime)
	if err != nil {
		return nil, err
	}

	return &podPidWatcher{
		ctx:            ctx,
		watchInterface: w,
		onChange:       onChange,
		podPidMap:      make(map[string][]int),
		podPidQuery:    service,
	}, nil
}

func (p *podPidWatcher) Run() {
	for {
		select {
		case event := <-p.watchInterface.ResultChan():
			pod := event.Object.(*v1.Pod)
			newPid, _ := p.podPidQuery.query(pod)
			oldPid := p.podPidMap[pod.Name]
			p.podPidMap[pod.Name] = newPid
			p.onChange(pod.Name, oldPid, newPid)
		case <-p.ctx.Done():
			p.watchInterface.Stop()
			return
		}
	}
}

func getRuntime(node *v1.Node) (containerRuntime, error) {
	runtimeVersion := node.Status.NodeInfo.ContainerRuntimeVersion
	if strings.Contains(runtimeVersion, string(runtimeDocker)) {
		return runtimeDocker, nil
	} else {
		return "", ErrRuntimeUnknown
	}
}

func newPodPidService(runtime containerRuntime) (podPidService, error) {
	switch runtime {
	case runtimeDocker:
		dockerClient, err := dockerclient.NewEnvClient()
		if err != nil {
			return nil, errors.Wrap(err, "创建Docker客户端出错")
		}
		return &dockerPodPidQuery{
			dockerClient: dockerClient,
		}, nil
	default:
		return nil, ErrRuntimeUnknown
	}
}

type podPidService interface {
	query(pod *v1.Pod) ([]int, error)
}

type dockerPodPidQuery struct {
	dockerClient *dockerclient.Client
}

func (d dockerPodPidQuery) query(pod *v1.Pod) ([]int, error) {
	result := make([]int, len(pod.Status.ContainerStatuses))
	for i := 0; i < len(result); i++ {
		cid := pod.Status.ContainerStatuses[i].ContainerID[9:]
		cinfo, err := d.dockerClient.ContainerInspect(context.TODO(), cid)
		if err != nil {
			return []int{}, errors.Wrap(err, "查询Pod Container Pid出错")
		}
		result[i] = cinfo.State.Pid
	}
	return result, nil
}
