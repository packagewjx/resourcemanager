package core

import (
	"math"
	"runtime"
	"time"
)

type ProgramMetric struct {
	Id           string
	MRC          []float32
	Instructions int
	L1Hit        int
	L2Hit        int
	L3Hit        int
	L3Miss       int
}

func (p ProgramMetric) Api() float32 {
	return float32(p.L1Hit+p.L2Hit+p.L3Hit+p.L3Miss) / float32(p.Instructions)
}

type CLOSScheme struct {
	CLOSNum       int
	WayBit        int
	MemThrottle   int
	ProcessGroups []*ProcessGroup
}

type CLOSCapabilityInfo struct {
	NumCatClos uint
	NumMbaClos uint
	MaxLLCWays uint
	MinLLCWays uint
}

type ProcessGroup struct {
	Id  string
	Pid []int
}

// 顶层公共Config
type Config struct {
	MemTrace   MemTraceConfig
	PerfStat   PerfStatConfig
	Algorithm  AlgorithmConfig
	Kubernetes KubernetesConfig
}

type MemTraceConfig struct {
	TraceCount    int
	MaxRthTime    int
	ConcurrentMax int
	PinConfig     `mapstructure:",squash"`
}

type PinConfig struct {
	PinToolPath    string
	BufferSize     int
	WriteThreshold int
	ReservoirSize  int
}

type PerfStatConfig struct {
	SampleTime time.Duration
}

type AlgorithmConfig struct {
	MPKIVeryHigh         float64
	HPKIVeryHigh         float64
	HPKIVeryLow          float64
	IPCVeryLow           float64
	IPCLow               float64
	NonCriticalCacheSize int
	MediumCacheSize      int
}

type KubernetesConfig struct {
	TokenFile string
	CAFile    string
	Insecure  bool
	Host      string
}

var RootConfig = &Config{
	MemTrace: MemTraceConfig{
		TraceCount:    1000000000,
		MaxRthTime:    100000,
		ConcurrentMax: int(math.Min(math.Max(1, float64(runtime.NumCPU())/4), 4)),
		PinConfig: PinConfig{
			PinToolPath:    "/home/wjx/Workspace/pin-3.17/source/tools/MemTrace2/obj-intel64/MemTrace2.so",
			BufferSize:     10000,
			WriteThreshold: 20000,
			ReservoirSize:  100000,
		},
	},
	PerfStat: PerfStatConfig{
		SampleTime: 10 * time.Second,
	},
	Algorithm: AlgorithmConfig{
		MPKIVeryHigh:         10,
		HPKIVeryHigh:         10,
		HPKIVeryLow:          0.5,
		IPCVeryLow:           0.6,
		IPCLow:               1.3,
		NonCriticalCacheSize: 512,   // L1的大小
		MediumCacheSize:      16384, // L3两个Set的大小
	},
}
