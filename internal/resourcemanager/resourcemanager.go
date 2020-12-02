package resourcemanager

import (
	"context"
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/resourcemonitor"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log"
	"net"
	"os"
	"strings"
)

const (
	DefaultHost     = "https://localhost:8443"
	DefaultInterval = 1000
)

type ResourceManager interface {
	Run() error
}

type Config struct {
	Host          string
	CaFile        string
	TokenFile     string
	Insecure      bool // 指定是否忽略TSL证书错误
	Interval      int
	ReservoirSize int // 内存使用追踪时，监控的集合大小
	MaxRthTime    int // 将内存使用转换为RTH时，最大的Reuse Time
}

type monitorStatus string

var (
	statusMonitoring monitorStatus = "monitoring"
	statusRunning    monitorStatus = "running"
	statusNoControl  monitorStatus = "noControl"
)

type podManageContext struct {
	monitorFile string
	status      monitorStatus
}

func New(config *Config) (ResourceManager, error) {
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

	m, err := resourcemonitor.New(config.Interval, config.ReservoirSize, config.MaxRthTime)
	if err != nil {
		return nil, errors.Wrap(err, "创建资源监控错误")
	}

	return &impl{
		client:  client,
		monitor: m,
		logger:  log.New(os.Stdout, "ResourceManager", log.LstdFlags|log.Lshortfile|log.Lmsgprefix),
	}, nil
}

type impl struct {
	client  *kubernetes.Clientset
	monitor resourcemonitor.Monitor
	watcher podPidWatcher
	logger  *log.Logger
}

func (i *impl) Run() error {
	panic("implement me")
}

var _ ResourceManager = &impl{}

func (i *impl) getPodsOnNode(nodeName string) (*v1.PodList, error) {
	return i.client.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
	})
}

func (i *impl) getCurrentNode() (*v1.Node, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, errors.Wrap(err, "获取本地IP地址出错")
	}
	addrMap := make(map[string]struct{})
	for _, addr := range addrs {
		addrString := addr.String()
		addrString = addrString[:strings.Index(addrString, "/")] // 去掉子网标识
		addrMap[addrString] = struct{}{}
	}

	list, err := i.client.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "获取本机节点名称失败")
	}
	for _, item := range list.Items {
		for _, address := range item.Status.Addresses {
			if _, ok := addrMap[address.Address]; ok {
				return &item, nil
			}
		}
	}
	return nil, fmt.Errorf("本机可能不是Kubernetes有效节点")
}
