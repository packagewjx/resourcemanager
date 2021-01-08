package pqos

import (
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestSetCLOSScheme(t *testing.T) {
	schemes := make([]*CLOSScheme, 1)
	pid := os.Getpid()
	schemes[0] = &CLOSScheme{
		CLOSNum:     1,
		Processes:   []int{pid},
		WayBit:      0x7FF,
		MemThrottle: 100,
	}

	err := SetCLOSScheme(schemes)
	assert.NoError(t, err)

	clos, err := GetProcessCLOS(pid)
	assert.Equal(t, 1, clos)
}
