package core

import (
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	err := LibInit()
	if err != nil {
		panic(err)
	}
	res := m.Run()
	_ = LibFinalize()
	os.Exit(res)
}

func TestGetInfo(t *testing.T) {
	info, err := GetCapabilityInfo()
	assert.NoError(t, err)
	assert.NotEqual(t, 0, info.MaxLLCWays)
	assert.NotEqual(t, 0, info.MinLLCWays)
	assert.NotEqual(t, 0, info.NumCatClos)
	assert.NotEqual(t, 0, info.NumMbaClos)
}

func TestSetControlScheme(t *testing.T) {
	schemes := make([]*ControlScheme, 1)
	pid := os.Getpid()
	schemes[0] = &ControlScheme{
		clos:        1,
		pidList:     []int{pid},
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