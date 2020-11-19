package core

import (
	"flag"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"log"
	"os"
	"testing"
)

var noInit bool
var host string
var tokenFile string

func init() {
	flag.BoolVar(&noInit, "no-init", true, "无需初始化")
	flag.StringVar(&host, "host", "https://localhost:8443", "Kubernetes API Host")
	flag.StringVar(&tokenFile, "tokenFile", "token", "Token file")
}

func TestMain(m *testing.M) {
	flag.Parse()
	if !noInit {
		err := LibInit()
		if err != nil {
			os.Exit(1)
		}
		defer func() {
			_ = LibFinalize()
		}()
	}
	rmImpl = &impl{
		client: kubernetes.NewForConfigOrDie(&rest.Config{
			Host:            host,
			BearerTokenFile: tokenFile,
			TLSClientConfig: rest.TLSClientConfig{
				Insecure: true,
			},
		}),
		monitor: &fakeMonitorImpl{},
		logger:  log.New(os.Stdout, "tester", log.Lshortfile),
	}

	os.Exit(m.Run())
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
