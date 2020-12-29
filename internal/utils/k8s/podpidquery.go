package k8s

import (
	"context"
	dockerclient "github.com/docker/docker/client"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
)

type PodPidQuery interface {
	Query(pod *v1.Pod) ([]int, error)
}

func NewPodPidQuery(node *v1.Node) (PodPidQuery, error) {
	runtime, _ := GetRuntime(node)
	switch runtime {
	case RuntimeDocker:
		// 目前只支持本机即可
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

type dockerPodPidQuery struct {
	dockerClient *dockerclient.Client
}

func (d dockerPodPidQuery) Query(pod *v1.Pod) ([]int, error) {
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
