package classifier

import (
	"context"
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/algorithm"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/sampler/perf"
	"github.com/packagewjx/resourcemanager/internal/sampler/pin"
	"github.com/packagewjx/resourcemanager/internal/utils"
	"log"
	"os"
	"sync"
)

type MemoryCharacteristic string

var (
	MemoryCharacteristicToDetermine MemoryCharacteristic = ""
	MemoryCharacteristicNonCritical MemoryCharacteristic = "non-critical"
	MemoryCharacteristicSquanderer  MemoryCharacteristic = "squanderer"
	MemoryCharacteristicBully       MemoryCharacteristic = "bully"
	MemoryCharacteristicMedium      MemoryCharacteristic = "medium"
	MemoryCharacteristicSensitive   MemoryCharacteristic = "sensitive"
	MemoryCharacteristicUnknown     MemoryCharacteristic = "unknown"
)

var L3Size int

func init() {
	ways, sets, _ := utils.GetL3Cap()
	L3Size = ways * sets
}

const (
	thresholdMPKIVeryHigh         = 10
	thresholdHPKIVeryHigh         = 10
	thresholdIPCLow               = 0.6
	thresholdNonCriticalCacheSize = 512 // 若在这个范围内，MR降到了5%以下，可以认为是内存非敏感型应用。取值为典型L1大小
)

type FinishFunc func(group *core.ProcessGroup, characteristic MemoryCharacteristic, perfStat *perf.PerfStatResult, rth []int)
type ErrorFunc func(group *core.ProcessGroup, err error)

type Config struct {
	MemTraceConfig *pin.Config
	ReservoirSize  int
}

type Result struct {
	Group     *core.ProcessGroup
	Error     error
	Processes []*ProcessResult
}

type ProcessResult struct {
	Pid            int
	Error          error
	Characteristic MemoryCharacteristic
	StatResult     *perf.PerfStatResult
	MemTraceResult map[int]algorithm.RTHCalculator
}

type Classifier interface {
	// 对一个进程组进行分类。对于
	Classify(ctx context.Context, group *core.ProcessGroup) <-chan *Result
}

func New(config *Config) Classifier {
	return &impl{
		logger:        log.New(os.Stdout, fmt.Sprintf("Classifier: "), log.Lmsgprefix|log.LstdFlags|log.Lshortfile),
		memRecorder:   pin.NewMemRecorder(config.MemTraceConfig),
		reservoirSize: config.ReservoirSize,
	}
}

type impl struct {
	memRecorder   pin.MemRecorder
	reservoirSize int
	logger        *log.Logger
}

var _ Classifier = &impl{}

func (c *impl) Classify(ctx context.Context, group *core.ProcessGroup) <-chan *Result {
	resultCh := make(chan *Result, 1)
	go func(group *core.ProcessGroup) {
		defer close(resultCh)
		processResults := make([]*ProcessResult, len(group.Pid))
		c.logger.Printf("开始对进程组 %s 执行分类。正在执行Perf Stat追踪", group.Id)
		perfCh := perf.NewPerfStatRunner(group).Start(ctx)
		perfResult := <-perfCh
		for i, pid := range group.Pid {
			processResults[i] = &ProcessResult{
				Pid:            pid,
				Error:          nil,
				Characteristic: MemoryCharacteristicToDetermine,
				StatResult:     perfResult[pid],
				MemTraceResult: nil,
			}
			if isBully(perfResult[pid]) {
				processResults[i].Characteristic = MemoryCharacteristicBully
			}
		}

		c.logger.Printf("开始对进程组 %s 执行内存追踪", group.Id)
		wg := sync.WaitGroup{}
		for i := 0; i < len(processResults); i++ {
			if processResults[i].Characteristic == MemoryCharacteristicToDetermine {
				wg.Add(1)
				go c.classifyProcess(ctx, group.Pid[i], processResults[i:i+1], &wg)
			}
		}
		wg.Wait()
		resultCh <- &Result{
			Group:     group,
			Error:     nil,
			Processes: processResults,
		}

		c.logger.Printf("进程组 %s 分类结束", group.Id)
	}(group)
	return resultCh
}

func (i *impl) classifyProcess(ctx context.Context, pid int, position []*ProcessResult, wg *sync.WaitGroup) {
	defer wg.Done()
	ch := i.memRecorder.RecordProcess(ctx, &pin.MemRecordAttachRequest{
		MemRecordBaseRequest: pin.MemRecordBaseRequest{
			Factory: func(tid int) algorithm.RTHCalculator {
				return algorithm.ReservoirCalculator(i.reservoirSize)
			},
			Name: fmt.Sprintf("%d", pid),
		},
		Pid: pid,
	})
	result := <-ch
	if result.Err != nil {
		position[0].Error = result.Err
		return
	}
	rth := make([][]int, 0, len(result.ThreadTrace))
	for _, calculator := range result.ThreadTrace {
		rth = append(rth, calculator.GetRTH(core.RootConfig.MemTrace.MaxRthTime))
	}
	position[0].Characteristic = determineCharacteristic(rth)
	position[0].MemTraceResult = result.ThreadTrace
}

func isBully(stat *perf.PerfStatResult) bool {
	mpki := float64(stat.LLCStoreMisses+stat.LLCLoadMisses) / float64(stat.Instructions) * 1000.0
	hpki := float64(stat.AllStores+stat.AllLoads-stat.LLCLoadMisses-stat.LLCStoreMisses) / float64(stat.Instructions) * 1000.0
	ipc := float64(stat.Instructions) / float64(stat.Cycles)
	return mpki > thresholdMPKIVeryHigh && hpki > thresholdHPKIVeryHigh && ipc < thresholdIPCLow
}

func isNonCritical(mrc []float32) bool {
	return mrc[thresholdNonCriticalCacheSize] < 0.05
}

func isSquanderer(rth []int, mrc []float32) bool {
	// FIXME 实现
	return false
}

func isMedium(rth []int, mrc []float32) bool {
	// FIXME 实现
	return false
}

func isHighlySensitive(rth []int, mrc []float32) bool {
	// FIXME 实现
	return true
}

func determineCharacteristic(rth [][]int) MemoryCharacteristic {
	averageRth := make([]int, len(rth[0]))
	for i := 0; i < len(averageRth); i++ {
		for j := 0; j < len(rth); j++ {
			averageRth[i] += rth[j][i]
		}
	}
	for i := 0; i < len(averageRth); i++ {
		averageRth[i] /= len(rth)
	}
	// 使用这个RTH计算MRC
	model := algorithm.NewAETModel(averageRth)
	mrc := model.MRC(L3Size)
	if isNonCritical(mrc) {
		return MemoryCharacteristicNonCritical
	} else if isSquanderer(averageRth, mrc) {
		return MemoryCharacteristicSquanderer
	} else if isMedium(averageRth, mrc) {
		return MemoryCharacteristicMedium
	} else if isHighlySensitive(averageRth, mrc) {
		return MemoryCharacteristicSensitive
	} else {
		return MemoryCharacteristicUnknown
	}
}

var _ Classifier = &impl{}
