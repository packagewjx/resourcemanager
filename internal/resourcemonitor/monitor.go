package resourcemonitor

import "C"
import (
	"context"
	"github.com/packagewjx/resourcemanager/internal/algorithm"
	"github.com/packagewjx/resourcemanager/internal/perf"
	"github.com/packagewjx/resourcemanager/internal/pin"
	"log"
	"os"
	"reflect"
	"sync"
)

var _ Monitor = &monitorImpl{}

type monitorImpl struct {
	config                           *Config
	memoryTraceRoutineControlChannel chan struct{}
	logger                           *log.Logger
	wg                               sync.WaitGroup
	parentCtx                        context.Context
}

// 创建一个新的ResourceMonitor
// ctx用于控制监控时使用的goroutine的运行。config用于配置监控参数。
func New(ctx context.Context, config *Config) Monitor {
	return &monitorImpl{
		logger:                           log.New(os.Stdout, "Monitor: ", log.LstdFlags|log.Lshortfile|log.Lmsgprefix),
		wg:                               sync.WaitGroup{},
		config:                           config,
		memoryTraceRoutineControlChannel: make(chan struct{}, config.ConcurrentMax),
		parentCtx:                        ctx,
	}
}

func (m *monitorImpl) PerfStat(rq *PerfStatRequest) <-chan *PerfStatResult {
	perfRunner := perf.NewPerfStatRunner(rq.Group, rq.SampleTime)
	ch := perfRunner.Start(m.parentCtx)
	resCh := make(chan *PerfStatResult, 1)

	go func(perfCh <-chan *perf.PerfStatResult, resultChan chan *PerfStatResult) {
		result := <-perfCh
		resultChan <- &PerfStatResult{
			PerfStatResult: *result,
		}
	}(ch, resCh)
	return resCh
}

func (m *monitorImpl) MemoryTrace(rq *MemoryTraceRequest) <-chan *MemoryTraceResult {
	m.wg.Add(1)
	ch := make(chan *MemoryTraceResult, 1)
	go func(rq *MemoryTraceRequest, resultCh chan *MemoryTraceResult) {
		m.memoryTraceRoutineControlChannel <- struct{}{}
		ctx, cancel := context.WithCancel(m.parentCtx)
		defer func() {
			cancel()
			m.wg.Done()
			<-m.memoryTraceRoutineControlChannel
		}()

		m.logger.Printf("正在添加监控组%s并启动Pin", rq.Group.Id)

		resultChan, err := m.startPin(ctx, rq)
		if err != nil {
			resultCh <- &MemoryTraceResult{
				Group:  rq.Group,
				Error:  err,
				result: nil,
			}
			return
		}

		m.logger.Printf("监控组%s等待Pin追踪结果", rq.Group.Id)
		result, err := m.receiveMemTraceResult(ctx, rq, resultChan)
		resultCh <- &MemoryTraceResult{
			Group:  rq.Group,
			Error:  err,
			result: result,
		}
		close(resultCh)
	}(rq, ch)
	return ch
}

func (m *monitorImpl) startPin(ctx context.Context, rq *MemoryTraceRequest) (map[int]<-chan map[int]algorithm.RTHCalculator, error) {
	resultChan := make(map[int]<-chan map[int]algorithm.RTHCalculator)
	for _, pid := range rq.Group.Pid {
		recorder := pin.NewMemAttachRecorder(&pin.MemRecorderAttachConfig{
			MemRecorderBaseConfig: pin.MemRecorderBaseConfig{
				Factory: func(tid int) algorithm.RTHCalculator {
					return algorithm.ReservoirCalculator(m.config.ReservoirSize)
				},
				WriteThreshold: m.config.WriteThreshold,
				PinBufferSize:  m.config.PinBufferSize,
				PinStopAt:      m.config.TotalMemTrace,
				PinToolPath:    m.config.PinToolPath,
				GroupName:      rq.Group.Id,
			},
			Pid: pid,
		})
		ch, err := recorder.Start(ctx)
		if err != nil {
			return nil, err
		}
		resultChan[pid] = ch
		m.logger.Printf("监控组 %s： 成功启动对进程Pid %d 的监控", rq.Group.Id, pid)
	}
	return resultChan, nil
}

func (m *monitorImpl) WaitForShutdown() {
	m.wg.Wait()
}

func (m *monitorImpl) receiveMemTraceResult(ctx context.Context, request *MemoryTraceRequest,
	pinChMap map[int]<-chan map[int]algorithm.RTHCalculator) ([]*MemoryTraceProcessResult, error) {
	result := make([]*MemoryTraceProcessResult, 0, len(pinChMap))
	cases := make([]reflect.SelectCase, 0, len(pinChMap)+1)
	pidList := make([]int, 0, len(pinChMap))
	for pid, ch := range pinChMap {
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
			return result, ErrStopped
		}
		m.logger.Printf("监控组 %s： 进程Pid %d 结束监控", request.Group.Id, pidList[chosen])
		rthMap := recv.Interface().(map[int]algorithm.RTHCalculator)
		rthList := make([][]int, 0, len(rthMap))
		for _, calculator := range rthMap {
			rthList = append(rthList, calculator.GetRTH(m.config.MaxRthTime))
		}
		result = append(result, &MemoryTraceProcessResult{
			Pid: pidList[chosen],
			Rth: rthList,
		})
	}

	m.logger.Printf("监控组 %s： 监控结束", request.Group.Id)
	return result, nil
}
