package resourcemonitor

import (
	"container/heap"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestMonitoringQueue(t *testing.T) {
	queue := queue(make([]*monitorContext, 0))
	heap.Init(&queue)
	heap.Push(&queue, &monitorContext{
		pidList:        []int{1},
		monitorEndTime: time.Now().Add(time.Minute),
		onFinish:       nil,
	})
	heap.Push(&queue, &monitorContext{
		pidList:        []int{2},
		monitorEndTime: time.Now().Add(time.Hour),
		onFinish:       nil,
	})
	heap.Push(&queue, &monitorContext{
		pidList:        []int{3},
		monitorEndTime: time.Now().Add(time.Second),
		onFinish:       nil,
	})

	assert.Equal(t, 3, len(queue))
	assert.Equal(t, 3, heap.Pop(&queue).(*monitorContext).pidList[0])
	assert.Equal(t, 1, heap.Pop(&queue).(*monitorContext).pidList[0])
	assert.Equal(t, 2, heap.Pop(&queue).(*monitorContext).pidList[0])
}
