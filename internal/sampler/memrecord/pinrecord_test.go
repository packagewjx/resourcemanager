package memrecord

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
	recorder, err := NewPinMemRecorder(&Config{
		BufferSize:     core.RootConfig.MemTrace.PinConfig.BufferSize,
		WriteThreshold: core.RootConfig.MemTrace.PinConfig.WriteThreshold,
		PinToolPath:    core.RootConfig.MemTrace.PinConfig.PinToolPath,
		TraceCount:     100000,
		ConcurrentMax:  4,
	})
	assert.NoError(t, err)
	pid := utils.ForkRunExample(1)
	consumer := NewRTHCalculatorConsumer(func(tid int) algorithm.RTHCalculator {
		return algorithm.ReservoirCalculator(core.RootConfig.MemTrace.ReservoirSize)
	})
	resCh, _ := recorder.RecordProcess(context.Background(), &AttachRequest{
		BaseRequest: BaseRequest{
			Name:     "test",
			Consumer: consumer,
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
		threadTrace := consumer.GetCalculatorMap()
		assert.NotZero(t, len(threadTrace))
		for _, calculator := range threadTrace {
			rth := calculator.GetRTH(10000)
			sum := 0
			for i := 0; i < len(rth); i++ {
				sum += rth[i]
			}
			assert.NotZero(t, sum)
		}
		assert.NotZero(t, res.TotalInstructions)
		for _, u := range res.ThreadInstructionCount {
			assert.NotZero(t, u)
		}
	}

	_, _ = syscall.Wait4(pid, nil, 0, nil)
}
