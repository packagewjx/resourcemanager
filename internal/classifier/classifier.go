package classifier

import (
	"context"
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/algorithm"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/perf"
	"github.com/packagewjx/resourcemanager/internal/pin"
	"github.com/packagewjx/resourcemanager/internal/utils"
	"log"
	"os"
	"sync"
	"time"
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

type ClassifyResult struct {
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

type ProcessGroupClassifier interface {
	// 对一个进程组进行分类。对于
	Classify(ctx context.Context) <-chan *ClassifyResult
}

type Config struct {
	Group         *core.ProcessGroup
	ReservoirSize int
	SampleTime    time.Duration
	MaxRthTime    int
}

func NewProcessGroupClassifier(config *Config) ProcessGroupClassifier {
	return &impl{
		config: config,
		logger: log.New(os.Stdout, fmt.Sprintf("Classifier-%s: ", config.Group.Id), log.Lmsgprefix|log.LstdFlags|log.Lshortfile),
		wg:     sync.WaitGroup{},
	}
}

type impl struct {
	config *Config
	logger *log.Logger
	wg     sync.WaitGroup
}

var _ ProcessGroupClassifier = &impl{}

func (c *impl) Classify(ctx context.Context) <-chan *ClassifyResult {
	resultCh := make(chan *ClassifyResult, 1)
	go func() {
		defer close(resultCh)
		processResults := make([]*ProcessResult, len(c.config.Group.Pid))
		c.logger.Println("开始对进程组执行分类。正在执行Perf Stat追踪")
		perfCh := perf.NewPerfStatRunner(c.config.Group, c.config.SampleTime).Start(ctx)
		perfResult := <-perfCh
		for i, pid := range c.config.Group.Pid {
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

		c.logger.Println("开始对进程组执行内存追踪")
		for i := 0; i < len(processResults); i++ {
			if processResults[i].Characteristic == MemoryCharacteristicToDetermine {
				c.wg.Add(1)
				go c.classifyProcess(ctx, c.config.Group.Pid[i], processResults[i:i+1])
			}
		}
		c.wg.Wait()
		resultCh <- &ClassifyResult{
			Group:     c.config.Group,
			Error:     nil,
			Processes: processResults,
		}

		c.logger.Println("进程组分类结束")
	}()
	return resultCh
}

func (i *impl) classifyProcess(ctx context.Context, pid int, position []*ProcessResult) {
	defer i.wg.Done()
	ch, err := pin.NewMemAttachRecorder(&pin.MemRecorderAttachConfig{
		MemRecorderBaseConfig: pin.MemRecorderBaseConfig{
			Factory: func(tid int) algorithm.RTHCalculator {
				return algorithm.ReservoirCalculator(i.config.ReservoirSize)
			},
			WriteThreshold: pin.DefaultWriteThreshold,
			PinBufferSize:  pin.DefaultPinBufferSize,
			PinStopAt:      pin.DefaultStopAt,
			PinToolPath:    "/home/wjx/Workspace/pin-3.17/source/tools/MemTrace2/obj-intel64/MemTrace2.so",
			GroupName:      i.config.Group.Id,
		},
		Pid: pid,
	}).Start(ctx)
	if err != nil {
		position[0].Error = err
		return
	}
	result := <-ch
	rth := make([][]int, 0, len(result))
	for _, calculator := range result {
		rth = append(rth, calculator.GetRTH(i.config.MaxRthTime))
	}
	position[0].Characteristic = determineCharacteristic(rth)
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
	panic("implement me")
}

func isMedium(rth []int, mrc []float32) bool {
	panic("implement me")
}

func isHighlySensitive(rth []int, mrc []float32) bool {
	panic("implement me")
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

var _ ProcessGroupClassifier = &impl{}
