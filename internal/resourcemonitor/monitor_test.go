package resourcemonitor

import (
	"context"
	"fmt"
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

func checkResult(t *testing.T, results []*MonitorResult) {
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

func TestMonitorProcess(t *testing.T) {
	pid := utils.ForkRunExample(1)

	monitor := New(context.Background(), testConfig)
	wg := sync.WaitGroup{}
	wg.Add(1)

	monitor.MonitorGroup(&Request{
		GroupId: "test",
		PidList: []int{pid},
		OnFinish: func(request *Request, result []*MonitorResult) {
			checkResult(t, result)
			wg.Done()
		},
		OnError: func(request *Request, err error) {
			wg.Done()
			assert.FailNow(t, err.Error())
		},
	})

	wg.Wait()

	_, _ = syscall.Wait4(pid, nil, 0, nil)
}

func TestNonExistProcess(t *testing.T) {
	monitor := New(context.Background(), testConfig)
	errored := false
	wg := sync.WaitGroup{}
	wg.Add(1)
	monitor.MonitorGroup(&Request{
		GroupId:  "test",
		PidList:  []int{10000000},
		OnFinish: nil,
		OnError: func(request *Request, err error) {
			errored = true
			wg.Done()
		},
	})
	wg.Wait()
	assert.True(t, errored)

	monitor.WaitForShutdown()
}

func TestAddProcessExceedRMID(t *testing.T) {
	monitor := New(context.Background(), testConfig)

	wg := sync.WaitGroup{}
	errored := false

	pidList := make([]int, 8)
	for i := 0; i < len(pidList); i++ {
		pidList[i] = utils.ForkRunExample(1)
		wg.Add(1)
		monitor.MonitorGroup(&Request{
			GroupId: fmt.Sprintf("group-%d", i),
			PidList: []int{pidList[i]},
			OnFinish: func(request *Request, result []*MonitorResult) {
				checkResult(t, result)
				wg.Done()
			},
			OnError: func(request *Request, err error) {
				t.Log(err)
				errored = true
				wg.Done()
			},
		})
	}

	wg.Wait()

	for _, pid := range pidList {
		_, _ = syscall.Wait4(pid, nil, 0, nil)
	}

	assert.False(t, errored)
}
