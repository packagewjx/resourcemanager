package classifier

import (
	"context"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/utils"
	"github.com/stretchr/testify/assert"
	"syscall"
	"testing"
	"time"
)

func TestClassifier(t *testing.T) {
	core.RootConfig.PerfStat.SampleTime = 2 * time.Second
	pid := utils.ForkRunExample(1)
	classifier, err := New(&Config{})
	assert.NoError(t, err)
	t.Log("正在执行第一个测试用例")
	ch := classifier.Classify(context.Background(), &core.ProcessGroup{
		Id:  "test",
		Pid: []int{pid},
	})
	result := <-ch
	assert.NotNil(t, result.Group)
	assert.Nil(t, result.Error)
	assert.Equal(t, 1, len(result.Processes))
	assert.Equal(t, MemoryCharacteristicNonCritical, result.Processes[0].Characteristic)
	assert.NotNil(t, result.Processes[0].StatResultAllWays)
	assert.Equal(t, pid, result.Processes[0].Pid)

	t.Log("测试结束，等待进程结束")
	_, _ = syscall.Wait4(pid, nil, 0, nil)

	t.Log("正在运行第二个测试用例")
	pid = utils.ForkRunExample(2)
	ch = classifier.Classify(context.Background(), &core.ProcessGroup{
		Id:  "test",
		Pid: []int{pid},
	})
	result = <-ch
	assert.Equal(t, MemoryCharacteristicBully, result.Processes[0].Characteristic)

	t.Log("测试结束，等待进程结束")
	_, _ = syscall.Wait4(pid, nil, 0, nil)
}
