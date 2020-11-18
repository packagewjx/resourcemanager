package core

import (
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	err := LibInit()
	if err != nil {
		os.Exit(1)
	}

	res := m.Run()
	_ = LibFinalize()
	os.Exit(res)
}

func TestGetInfo(t *testing.T) {
	info, err := GetCapabilityInfo()
	assert.NoError(t, err)
	assert.NotEqual(t, 0, info.maxLLCWays)
	assert.NotEqual(t, 0, info.minLLCWays)
	assert.NotEqual(t, 0, info.numCatClos)
	assert.NotEqual(t, 0, info.numMbaClos)
}

func TestSetControlScheme(t *testing.T) {
	schemes := make([]*ControlScheme, 1)
	pid := os.Getpid()
	schemes[0] = &ControlScheme{
		clos:        1,
		pidList:     []uint{uint(pid)},
		llc:         0,
		mbaThrottle: 0,
	}

	err := SetControlScheme(schemes)
	assert.NoError(t, err)

	clos, err := GetProcessCLOS(uint(pid))
	assert.NoError(t, err)
	assert.Equal(t, uint(1), clos)
}

func TestGetCLOS(t *testing.T) {
	clos, err := GetProcessCLOS(1)
	assert.NoError(t, err)
	assert.Equal(t, uint(0), clos)
}
