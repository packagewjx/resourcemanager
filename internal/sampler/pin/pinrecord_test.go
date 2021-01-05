package pin

import (
	"context"
	"github.com/packagewjx/resourcemanager/internal/algorithm"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/utils"
	"github.com/stretchr/testify/assert"
	"syscall"
	"testing"
	"time"
)

func TestRecord(t *testing.T) {
	recorder := NewMemRecorder(&Config{
		BufferSize:     core.RootConfig.MemTrace.BufferSize,
		WriteThreshold: core.RootConfig.MemTrace.WriteThreshold,
		PinToolPath:    core.RootConfig.MemTrace.PinToolPath,
		TraceCount:     100000,
		ConcurrentMax:  4,
	})
	pid := utils.ForkRunExample(1)
	resCh := recorder.RecordProcess(context.Background(), &MemRecordAttachRequest{
		MemRecordBaseRequest: MemRecordBaseRequest{
			Factory: func(tid int) algorithm.RTHCalculator {
				return algorithm.ReservoirCalculator(core.RootConfig.MemTrace.ReservoirSize)
			},
			Name: "test",
		},
		Pid: pid,
	})
	select {
	case <-time.After(10 * time.Second):
		t.Log("内存追踪超时")
		t.FailNow()
	case res := <-resCh:
		assert.NotNil(t, res)
		assert.NoError(t, res.Err)
		assert.NotZero(t, len(res.ThreadTrace))
		for _, calculator := range res.ThreadTrace {
			rth := calculator.GetRTH(10000)
			sum := 0
			for i := 0; i < len(rth); i++ {
				sum += rth[i]
			}
			assert.NotZero(t, sum)
		}
	}

	_, _ = syscall.Wait4(pid, nil, 0, nil)
}
