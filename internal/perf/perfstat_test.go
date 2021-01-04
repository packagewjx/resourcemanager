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
	runner := NewPerfStatRunner(&core.ProcessGroup{
		Id:  "test",
		Pid: []int{os.Getpid()},
	}, time.Second)

	ch := runner.Start(context.Background())
	resultMap := <-ch
	for _, result := range resultMap {
		assert.NoError(t, result.Error)
		assert.NotZero(t, result.AllLoads)
		assert.NotZero(t, result.AllStores)
		assert.NotZero(t, result.LLCLoadMisses)
		assert.NotZero(t, result.LLCStoreMisses)
		assert.NotZero(t, result.Instructions)
		assert.NotZero(t, result.Cycles)
	}
}
