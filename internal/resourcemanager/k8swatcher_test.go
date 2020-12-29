package resourcemanager

import (
	"context"
	"fmt"
	dockerclient "github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDocker(t *testing.T) {
	dockerClient, err := dockerclient.NewEnvClient()
	assert.NoError(t, err)
	version, err := dockerClient.ServerVersion(context.TODO())
	fmt.Printf("%v %v\n", version, err)
}
