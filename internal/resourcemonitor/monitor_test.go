package resourcemonitor

import (
	"context"
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/utils"
	"github.com/stretchr/testify/assert"
	"sync"
	"syscall"
	"testing"
)

var testConfig *Config

func init() {
	testConfig = DefaultConfig()
	testConfig.TotalMemTrace = 100000
}

func checkResult(t *testing.T, results []*MemoryTraceProcessResult) {
	t.Helper()
	for _, result := range results {
		assert.NotEqual(t, 0, result.Pid)
		total := 0
		for i := 0; i < len(result.Rth); i++ {
			for j := 0; j < len(result.Rth[i]); j++ {
				total += result.Rth[i][j]
			}
		}
		assert.NotEqual(t, 0, total)
	}
}

func TestMemTrace(t *testing.T) {
	pid := utils.ForkRunExample(1)

	monitor := New(context.Background(), testConfig)

	ch := monitor.MemoryTrace(&MemoryTraceRequest{
		Group: &core.ProcessGroup{
			Id:  "test",
			Pid: []int{pid},
		},
	})
	result := <-ch
	checkResult(t, result.Result)

	_, _ = syscall.Wait4(pid, nil, 0, nil)
}

func TestNonExistProcess(t *testing.T) {
	monitor := New(context.Background(), testConfig)
	wg := sync.WaitGroup{}
	wg.Add(1)
	ch := monitor.MemoryTrace(&MemoryTraceRequest{
		Group: &core.ProcessGroup{
			Id:  "test",
			Pid: []int{10000000},
		},
	})
	result := <-ch
	assert.Error(t, result.Error)

	monitor.WaitForShutdown()
}

func TestAddProcessExceedRMID(t *testing.T) {
	monitor := New(context.Background(), testConfig)

	pidList := make([]int, 8)
	channels := make([]<-chan *MemoryTraceResult, 0, len(pidList))
	for i := 0; i < len(pidList); i++ {
		pidList[i] = utils.ForkRunExample(1)
		ch := monitor.MemoryTrace(&MemoryTraceRequest{
			Group: &core.ProcessGroup{
				Id:  fmt.Sprintf("group-%d", i),
				Pid: []int{pidList[i]},
			},
		})
		channels = append(channels, ch)
	}

	for _, channel := range channels {
		result := <-channel
		assert.NoError(t, result.Error)
		checkResult(t, result.Result)
	}

	for _, pid := range pidList {
		_, _ = syscall.Wait4(pid, nil, 0, nil)
	}
}
