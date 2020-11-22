package resourcemanager

import (
	"context"
	"flag"
	"github.com/packagewjx/resourcemanager/internal/resourcemonitor"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log"
	"os"
	"testing"
)

var rmImpl *impl

var host string
var tokenFile string

func init() {
	flag.StringVar(&host, "host", "https://localhost:8443", "Kubernetes API Host")
	flag.StringVar(&tokenFile, "tokenFile", "token", "Token file")
}

func TestMain(m *testing.M) {
	flag.Parse()
	rmImpl = &impl{
		client: kubernetes.NewForConfigOrDie(&rest.Config{
			Host:            host,
			BearerTokenFile: tokenFile,
			TLSClientConfig: rest.TLSClientConfig{
				Insecure: true,
			},
		}),
		monitor: &fakeMonitorImpl{},
		logger:  log.New(os.Stdout, "tester", log.Lshortfile),
	}

	os.Exit(m.Run())
}

func TestGetCurrentNodeAndPods(t *testing.T) {
	node, err := rmImpl.getCurrentNode()
	assert.NoError(t, err)
	assert.NotNil(t, node)

	podsOnNode, err := rmImpl.getPodsOnNode(node.Name)
	assert.NoError(t, err)
	assert.NotNil(t, podsOnNode)
	assert.NotEqual(t, 0, len(podsOnNode.Items))
}

type fakeMonitorImpl struct {
}

var _ resourcemonitor.Monitor = &fakeMonitorImpl{}

func (f fakeMonitorImpl) AddProcess(_ *resourcemonitor.Request) {
}

func (f fakeMonitorImpl) RemoveProcess(_ string) {
}

func (f fakeMonitorImpl) Start(_ context.Context) {
}

func (f fakeMonitorImpl) ShutDownNow() {
}
