package memrecord

import (
	"context"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/test"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func init() {
	path := test.GetTestConfigFile()
	f, _ := os.Open(path)
	_ = viper.ReadConfig(f)
	_ = f.Close()
	_ = viper.UnmarshalExact(core.RootConfig)
}

func TestReader(t *testing.T) {
	m, err := NewPerfRecorder(core.RootConfig.MemTrace.PerfRecordConfig.OverflowCount,
		core.RootConfig.MemTrace.PerfRecordConfig.SwitchOutput,
		core.RootConfig.MemTrace.PerfRecordConfig.PerfExecPath)
	assert.NoError(t, err)
	p := m.(*perfRecorder)
	var cnt uint64
	result := p.parseResult(filepath.Join(test.GetTestDataDir(), "perf.perfaddr.data"), &cnt)
	assert.NotZero(t, len(result))
	for tid, uint64s := range result {
		assert.NotZero(t, tid)
		assert.NotZero(t, len(uint64s))
	}
}

type testConsumer struct {
	t *testing.T
}

func (t *testConsumer) Consume(tid int, addr []uint64) {
	assert.NotZero(t.t, tid)
	for _, u := range addr {
		assert.NotZero(t.t, u)
	}
}

func TestRecordLs(t *testing.T) {
	m, err := NewPerfRecorder(core.RootConfig.MemTrace.PerfRecordConfig.OverflowCount,
		core.RootConfig.MemTrace.PerfRecordConfig.SwitchOutput,
		core.RootConfig.MemTrace.PerfRecordConfig.PerfExecPath)
	assert.NoError(t, err)
	_, file, _, _ := runtime.Caller(0)
	dirs := strings.Split(filepath.Dir(file), string(filepath.Separator))
	dir := strings.Join(dirs[:len(dirs)-3], string(filepath.Separator))
	command, err := m.RecordCommand(context.Background(), &RunRequest{
		BaseRequest: BaseRequest{
			Name:     "test",
			Consumer: &testConsumer{t: t},
		},
		Cmd:  "ls",
		Args: []string{"-lR", dir},
	})
	assert.NoError(t, err)
	select {
	case <-time.After(3 * time.Second):
		t.Fatal("没有在时间内执行完毕")
	case res := <-command:
		assert.NotNil(t, res)
		assert.NotZero(t, res.TotalInstructions)
		assert.NotNil(t, res.ThreadInstructionCount)
		assert.NotZero(t, len(res.ThreadInstructionCount))
	}
}
