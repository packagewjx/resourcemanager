package k8s

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"net"
	"strings"
)

type ContainerRuntime string

var (
	RuntimeDocker ContainerRuntime = "docker"
)

var (
	ErrRuntimeUnknown = fmt.Errorf("无法确定容器运行时类型")
)

func GetRuntime(node *v1.Node) (ContainerRuntime, error) {
	runtimeVersion := node.Status.NodeInfo.ContainerRuntimeVersion
	if strings.Contains(runtimeVersion, string(RuntimeDocker)) {
		return RuntimeDocker, nil
	} else {
		return "", ErrRuntimeUnknown
	}
}

func GetCurrentNode(client *kubernetes.Clientset) (*v1.Node, error) {
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

	list, err := client.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
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

func GetPodsOnNode(client *kubernetes.Clientset, nodeName string) (*v1.PodList, error) {
	return client.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
	})
}
