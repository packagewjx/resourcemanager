package resourcemonitor

import "C"
import (
	"context"
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/algorithm"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/perf"
	"github.com/packagewjx/resourcemanager/internal/pin"
	"log"
	"os"
	"reflect"
	"sync"
)

var (
	ErrStopped = fmt.Errorf("进程组在监控完成之前被结束")
)

var (
	DefaultReservoirSize = 100000
	DefaultMaxRthTime    = 100000
)

type Monitor interface {
	MemoryTrace(rq *MemoryTraceRequest) // 添加进程到监控队列。若监控队列已满，将会把进程放入等待队列。完成监控时将会调用onFinish函数。出错则调用onError函数
	PerfStat(rq *PerfStatRequest)       // 监控IPC、L1Hit等指标
	WaitForShutdown()                   // 等待所有监控结束。必须在parentContext结束后调用，否则可能陷入无限等待
}

type MonitorResult struct {
	Pid int
	Rth [][]int
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

type MemoryTraceFinishFunc func(request *MemoryTraceRequest, result []*MonitorResult)
type ErrorFunc func(request *MemoryTraceRequest, err error)

var _ Monitor = &monitorImpl{}

type MemoryTraceRequest struct {
	core.ProcessGroup
	OnFinish MemoryTraceFinishFunc
	OnError  ErrorFunc
}

type PerfStatRequest struct {
	core.ProcessGroup
	OnFinish perf.StatFinishFunc
}

type monitorImpl struct {
	config                *Config
	routineControlChannel chan struct{}
	logger                *log.Logger
	wg                    sync.WaitGroup
	parentCtx             context.Context
}

func (m *monitorImpl) PerfStat(rq *PerfStatRequest) {
	perfRunner := perf.NewPerfStatRunner(&rq.ProcessGroup, rq.OnFinish)
	_ = perfRunner.Start(m.parentCtx)
}

func New(ctx context.Context, config *Config) Monitor {
	return &monitorImpl{
		logger:                log.New(os.Stdout, "Monitor: ", log.LstdFlags|log.Lshortfile|log.Lmsgprefix),
		wg:                    sync.WaitGroup{},
		config:                config,
		routineControlChannel: make(chan struct{}, config.ConcurrentMax),
		parentCtx:             ctx,
	}
}

func (m *monitorImpl) MemoryTrace(rq *MemoryTraceRequest) {
	if rq.OnError == nil {
		rq.OnError = func(_ *MemoryTraceRequest, _ error) {

		}
	}
	if rq.OnFinish == nil {
		rq.OnFinish = func(_ *MemoryTraceRequest, _ []*MonitorResult) {

		}
	}

	m.wg.Add(1)
	go func(rq *MemoryTraceRequest) {
		m.routineControlChannel <- struct{}{}
		ctx, cancel := context.WithCancel(m.parentCtx)
		defer func() {
			cancel()
			m.wg.Done()
			<-m.routineControlChannel
		}()

		m.logger.Printf("正在添加监控组%s并启动Pin", rq.Id)

		resultChan, err := m.startPin(ctx, rq)
		if err != nil {
			rq.OnError(rq, err)
			return
		}

		m.logger.Printf("监控组%s等待Pin追踪结果", rq.Id)
		m.receiveResult(ctx, rq, resultChan)
	}(rq)
}

func (m *monitorImpl) startPin(ctx context.Context, rq *MemoryTraceRequest) (map[int]<-chan map[int]algorithm.RTHCalculator, error) {
	resultChan := make(map[int]<-chan map[int]algorithm.RTHCalculator)
	for _, pid := range rq.Pid {
		recorder := pin.NewMemAttachRecorder(&pin.MemRecorderAttachConfig{
			MemRecorderBaseConfig: pin.MemRecorderBaseConfig{
				Factory: func(tid int) algorithm.RTHCalculator {
					return algorithm.ReservoirCalculator(m.config.ReservoirSize)
				},
				WriteThreshold: m.config.WriteThreshold,
				PinBufferSize:  m.config.PinBufferSize,
				PinStopAt:      m.config.TotalMemTrace,
				PinToolPath:    m.config.PinToolPath,
				GroupName:      rq.Id,
			},
			Pid: pid,
		})
		ch, err := recorder.Start(ctx)
		if err != nil {
			return nil, err
		}
		resultChan[pid] = ch
		m.logger.Printf("监控组 %s： 成功启动对进程Pid %d 的监控", rq.Id, pid)
	}
	return resultChan, nil
}

func (m *monitorImpl) WaitForShutdown() {
	m.wg.Wait()
}

func (m *monitorImpl) receiveResult(ctx context.Context, request *MemoryTraceRequest, resultChan map[int]<-chan map[int]algorithm.RTHCalculator) {
	result := make([]*MonitorResult, 0, len(resultChan))
	cases := make([]reflect.SelectCase, 0, len(resultChan)+1)
	pidList := make([]int, 0, len(resultChan))
	for pid, ch := range resultChan {
		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(ch),
		})
		pidList = append(pidList, pid)
	}
	cases = append(cases, reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(ctx.Done()),
	})

	for len(result) < len(pidList) {
		chosen, recv, _ := reflect.Select(cases)
		if chosen == len(pidList) {
			m.logger.Printf("监控组 %s 提前结束监控", request.Id)
			request.OnError(request, ErrStopped)
			return
		}
		m.logger.Printf("监控组 %s： 进程Pid %d 结束监控", request.Id, pidList[chosen])
		rthMap := recv.Interface().(map[int]algorithm.RTHCalculator)
		rthList := make([][]int, 0, len(rthMap))
		for _, calculator := range rthMap {
			rthList = append(rthList, calculator.GetRTH(m.config.MaxRthTime))
		}
		result = append(result, &MonitorResult{
			Pid: pidList[chosen],
			Rth: rthList,
		})
	}

	m.logger.Printf("监控组 %s： 监控结束", request.Id)
	request.OnFinish(request, result)
}
