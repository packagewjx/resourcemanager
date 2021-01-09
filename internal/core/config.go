package core

import (
	"math"
	"runtime"
	"time"
)

// 顶层公共Config
type Config struct {
	MemTrace   MemTraceConfig
	PerfStat   PerfStatConfig
	Algorithm  AlgorithmConfig
	Kubernetes KubernetesConfig
	Manager    ManagerConfig
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

type ClassifyConfig struct {
	MPKIVeryHigh         float64
	HPKIVeryHigh         float64
	HPKIVeryLow          float64
	IPCVeryLow           float64
	IPCLow               float64
	LLCMissRateHigh      float64
	LLCAPIHigh           float64
	MRCLowest            float64
	NonCriticalCacheSize int
	MediumCacheSize      int
}

type DCAPSConfig struct {
	MaxIteration       int
	InitialStep        float64
	MinStep            float64
	StepReductionRatio float64
}

type AlgorithmConfig struct {
	Classify ClassifyConfig
	DCAPS    DCAPSConfig
}

type KubernetesConfig struct {
	TokenFile string
	CAFile    string
	Insecure  bool
	Host      string
}

type ManagerConfig struct {
	AllocCoolDown               time.Duration // 再分配的冷却时间，避免频繁分配
	AllocSquash                 time.Duration // 在这个时间段内，多个分配请求合并到一次完成
	ChangeProcessCountThreshold int           // 多个进程组更新时，更新的进程的数量达到这个数字时才进行再分配
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
		Classify: ClassifyConfig{
			MPKIVeryHigh:         10,
			HPKIVeryHigh:         10,
			HPKIVeryLow:          0.5,
			IPCVeryLow:           0.6,
			IPCLow:               1.3,
			LLCMissRateHigh:      0.4,
			LLCAPIHigh:           0.005,
			MRCLowest:            0.3,
			NonCriticalCacheSize: 512,   // L1的大小
			MediumCacheSize:      16384, // L3两个Set的大小
		},
		DCAPS: DCAPSConfig{},
	},
	Manager: ManagerConfig{
		AllocCoolDown:               60 * time.Second,
		AllocSquash:                 50 * time.Millisecond,
		ChangeProcessCountThreshold: 100, // 暂定
	},
}
