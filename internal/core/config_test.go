package core

import (
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
	"time"
)

func TestViper(t *testing.T) {
	configIN := strings.NewReader(`
memtrace:
    traceCount: 1
    maxrthtime: 2
    pintoolpath: /wujunxian
    buffersize: 3
    writethreshold: 4
    reservoirsize: 5
perfstat:
    sampletime: 10s
algorithm:
    mpkiveryhigh: 6
    hpkiveryhigh: 7
    ipclow: 8
    noncriticalcachesize: 9
`)
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(configIN)
	assert.NoError(t, err)
	c := &Config{}
	err = viper.Unmarshal(c)
	assert.NoError(t, err)
	assert.Equal(t, 1, c.MemTrace.TraceCount)
	assert.Equal(t, 2, c.MemTrace.MaxRthTime)
	assert.Equal(t, "/wujunxian", c.MemTrace.PinToolPath)
	assert.Equal(t, 3, c.MemTrace.BufferSize)
	assert.Equal(t, 4, c.MemTrace.WriteThreshold)
	assert.Equal(t, 5, c.MemTrace.ReservoirSize)
	assert.Equal(t, 10*time.Second, c.PerfStat.SampleTime)
	assert.Equal(t, float32(6), c.Algorithm.MPKIVeryHigh)
	assert.Equal(t, float32(7), c.Algorithm.HPKIVeryHigh)
	assert.Equal(t, float32(8), c.Algorithm.IPCLow)
	assert.Equal(t, 9, c.Algorithm.NonCriticalCacheSize)

}
