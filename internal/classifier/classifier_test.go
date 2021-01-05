package classifier

import (
	"context"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/utils"
	"github.com/stretchr/testify/assert"
	"syscall"
	"testing"
)

func TestClassifier(t *testing.T) {
	pid := utils.ForkRunExample(1)
	classifier := NewProcessGroupClassifier(&Config{
		Group: &core.ProcessGroup{
			Id:  "test",
			Pid: []int{pid},
		},
	})
	ch := classifier.Classify(context.Background())
	result := <-ch
	assert.NotNil(t, result.Group)
	assert.Nil(t, result.Error)
	assert.Equal(t, 1, len(result.Processes))
	assert.Equal(t, MemoryCharacteristicNonCritical, result.Processes[0].Characteristic)
	assert.NotNil(t, result.Processes[0].StatResult)
	assert.Equal(t, pid, result.Processes[0].Pid)

	_, _ = syscall.Wait4(pid, nil, 0, nil)

	pid = utils.ForkRunExample(2)
	classifier = NewProcessGroupClassifier(&Config{
		Group: &core.ProcessGroup{
			Id:  "test",
			Pid: []int{pid},
		},
	})
	ch = classifier.Classify(context.Background())
	result = <-ch
	assert.Equal(t, MemoryCharacteristicBully, result.Processes[0].Characteristic)

	_, _ = syscall.Wait4(pid, nil, 0, nil)
}
