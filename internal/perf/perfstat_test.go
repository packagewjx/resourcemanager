package perf

import (
	"context"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
	"time"
)

func TestPerfStat(t *testing.T) {
	successCalled := false
	runner := NewPerfStatRunner(&core.ProcessGroup{
		Id:  "test",
		Pid: []int{os.Getpid()},
	}, func(group *core.ProcessGroup, record *PerfStat) {
		successCalled = true
		assert.NotZero(t, record.L3Miss)
		assert.NotZero(t, record.L1Hit)
		assert.NotZero(t, record.L2Hit)
		assert.NotZero(t, record.L3Hit)
		assert.NotZero(t, record.Instructions)
	})
	ctx, cancel := context.WithCancel(context.Background())
	err := runner.Start(ctx)
	assert.NoError(t, err)
	<-time.After(time.Second)
	cancel()
	<-time.After(100 * time.Millisecond)
	assert.True(t, successCalled)
}
