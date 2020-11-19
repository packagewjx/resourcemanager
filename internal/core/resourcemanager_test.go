package core

import (
	"context"
	"github.com/packagewjx/resourcemanager/internal/monitor"
	"github.com/stretchr/testify/assert"
	"testing"
)

var rmImpl *impl

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

func (f fakeMonitorImpl) AddProcess(_ *monitor.Request) {
}

func (f fakeMonitorImpl) Start(_ context.Context) {
}

func (f fakeMonitorImpl) ShutDownNow() {
}
