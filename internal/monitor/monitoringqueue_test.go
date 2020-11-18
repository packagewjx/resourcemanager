package monitor

import (
	"container/heap"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestMonitoringQueue(t *testing.T) {
	queue := monitoringQueue(make([]*monitorContext, 0))
	heap.Init(&queue)
	heap.Push(&queue, &monitorContext{
		pid:            1,
		monitorEndTime: time.Now().Add(time.Minute),
		onFinish:       nil,
	})
	heap.Push(&queue, &monitorContext{
		pid:            2,
		monitorEndTime: time.Now().Add(time.Hour),
		onFinish:       nil,
	})
	heap.Push(&queue, &monitorContext{
		pid:            3,
		monitorEndTime: time.Now().Add(time.Second),
		onFinish:       nil,
	})

	assert.Equal(t, 3, len(queue))
	assert.Equal(t, uint(3), heap.Pop(&queue).(*monitorContext).pid)
	assert.Equal(t, uint(1), heap.Pop(&queue).(*monitorContext).pid)
	assert.Equal(t, uint(2), heap.Pop(&queue).(*monitorContext).pid)
}
