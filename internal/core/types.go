package core

import "time"

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

// FIXME 引入顶层公共Config
type Config struct {
	MemTraceConfig
	PerfStatConfig
	AlgorithmConfig
}

type MemTraceConfig struct {
	TraceCount int
	MaxRthTime int
	PinConfig
}

type PinConfig struct {
	PinToolPath    string
	BufferSize     int
	WriteThreshold int
}

type PerfStatConfig struct {
	ReservoirSize int
	SampleTime    time.Duration
}

type AlgorithmConfig struct {
	MPKIVeryHigh         float32
	HPKIVeryHigh         float32
	IPCLow               float32
	NonCriticalCacheSize int
}
