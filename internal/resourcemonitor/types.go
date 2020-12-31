package resourcemonitor

import (
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/perf"
	"time"
)

var (
	ErrStopped = fmt.Errorf("进程组在监控完成之前被结束")
)

var (
	DefaultReservoirSize = 100000
	DefaultMaxRthTime    = 100000
)

type Monitor interface {
	MemoryTrace(rq *MemoryTraceRequest) <-chan *MemoryTraceResult // 添加进程到监控队列。若监控队列已满，将会把进程放入等待队列。完成监控时将会调用onFinish函数。出错则调用onError函数
	PerfStat(rq *PerfStatRequest) <-chan *PerfStatResult          // 监控IPC、L1Hit等指标
	WaitForShutdown()                                             // 等待所有监控结束。必须在parentContext结束后调用，否则可能陷入无限等待
}

type MemoryTraceRequest struct {
	Group *core.ProcessGroup
}

type MemoryTraceResult struct {
	Group  *core.ProcessGroup
	Error  error
	result []*MemoryTraceProcessResult
}

type PerfStatRequest struct {
	Group      *core.ProcessGroup
	SampleTime time.Duration
}

type PerfStatResult struct {
	perf.PerfStatResult
}

type MemoryTraceProcessResult struct {
	Pid int     // 进程号
	Rth [][]int // 每一条线程的具体RTH
}

type Config struct {
	ReservoirSize  int
	MaxRthTime     int
	ConcurrentMax  int
	WriteThreshold int
	PinBufferSize  int
	TotalMemTrace  int
	PinToolPath    string
}

func DefaultConfig() *Config {
	return &Config{
		ReservoirSize:  100000,
		MaxRthTime:     100000,
		ConcurrentMax:  4,
		WriteThreshold: 20000,
		PinBufferSize:  10000,
		TotalMemTrace:  5000000000,
		PinToolPath:    "/home/wjx/Workspace/pin-3.17/source/tools/MemTrace2/obj-intel64/MemTrace2.so",
	}
}
