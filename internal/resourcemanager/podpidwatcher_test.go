package resourcemanager

import (
	"context"
	"fmt"
	dockerclient "github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestWatcher(t *testing.T) {
	node, err := rmImpl.getCurrentNode()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	added := 0
	watcher, err := newPodPidWatcher(ctx, rmImpl.client, node, func(id string, oldPid, newPid []int) {
		added++
		assert.NotEqual(t, 0, len(newPid))
		for _, pid := range newPid {
			assert.NotEqual(t, 0, pid)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	go watcher.Run()
	<-time.After(time.Second)
	cancel()
	assert.NotEqual(t, 0, added)
}

func TestDocker(t *testing.T) {
	dockerClient, err := dockerclient.NewEnvClient()
	assert.NoError(t, err)
	version, err := dockerClient.ServerVersion(context.TODO())
	fmt.Printf("%v %v\n", version, err)
}
