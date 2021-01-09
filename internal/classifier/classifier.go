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
)

var L3Size int

func init() {
	ways, sets, _ := utils.GetL3Cap()
	L3Size = ways * sets
}

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
	Pid                int
	Error              error
	Characteristic     MemoryCharacteristic
	StatResult         *perf.PerfStatResult
	MemTraceResult     *pin.MemRecordResult
	WeightedAverageMRC []float32 // 加权平均MRC，权值为指令数量占比
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
	position[0].MemTraceResult = result
	position[0].Characteristic = determineCharacteristic(position[0])
}

func isBully(stat *perf.PerfStatResult) bool {
	mpki := stat.MissPerKiloInstructions()
	hpki := stat.HitPerKiloInstructions()
	ipc := stat.InstructionPerCycle()
	return mpki >= core.RootConfig.Algorithm.Classify.MPKIVeryHigh && hpki >= core.RootConfig.Algorithm.Classify.HPKIVeryHigh &&
		ipc <= core.RootConfig.Algorithm.Classify.IPCVeryLow
}

func isNonCritical(mrc []float32) bool {
	return mrc[core.RootConfig.Algorithm.Classify.NonCriticalCacheSize] < 0.05
}

func isSquanderer(mrc []float32, stat *perf.PerfStatResult) bool {
	ipc := float64(stat.Instructions) / float64(stat.Cycles)
	if ipc <= core.RootConfig.Algorithm.Classify.IPCLow || stat.LLCMissRate() > core.RootConfig.Algorithm.Classify.LLCMissRateHigh ||
		stat.AccessLLCPerInstructions() >= core.RootConfig.Algorithm.Classify.LLCAPIHigh {
		// MRC必然是单调递减的。因此分成多个区间，每个区间查看其斜率，找到斜率低于阈值的位置。阈值通常是取值为加大缓存空间收益小的位置。
		const intervalCount = 1000
		const slopeThreshold = 1.0 / intervalCount
		targetPosition := -1
		stepSize := len(mrc) / intervalCount
		idx := 0
		// i到intervalCount-1是没有必要再检查最后一个区间了，不仅要加入判断逻辑，还没有多大意义
		for i := 0; i < intervalCount-1; i++ {
			slope := float64(mrc[idx]-mrc[idx+stepSize]) / float64(stepSize)
			if slope < slopeThreshold {
				targetPosition = idx
				break
			}
			idx += stepSize
		}
		if targetPosition == -1 {
			// 这个情况应该保证很少发生
			return false
		}
		// 若MissRate基本不变化时依旧很高，就认为是Squanderer
		return float64(mrc[targetPosition]) > core.RootConfig.Algorithm.Classify.MRCLowest
	}
	return false
}

func isMedium(mrc []float32) bool {
	return mrc[core.RootConfig.Algorithm.Classify.MediumCacheSize] < 0.05
}

func determineCharacteristic(p *ProcessResult) MemoryCharacteristic {
	// 使用平均RTH判断
	mrc := WeightedAverageMRC(p.MemTraceResult, core.RootConfig.MemTrace.MaxRthTime, L3Size*2)
	p.WeightedAverageMRC = mrc
	if isNonCritical(mrc) {
		return MemoryCharacteristicNonCritical
	} else if isSquanderer(mrc, p.StatResult) {
		return MemoryCharacteristicSquanderer
	} else if isMedium(mrc) {
		return MemoryCharacteristicMedium
	} else {
		return MemoryCharacteristicSensitive
	}
}

var _ Classifier = &impl{}

// 给所有线程计算的加权平均MRC
func WeightedAverageMRC(m *pin.MemRecordResult, maxRTH, cacheSize int) []float32 {
	averageRth := make([]int, maxRTH+2)
	for tid, calculator := range m.ThreadTrace {
		rth := calculator.GetRTH(maxRTH)
		weight := float32(m.ThreadInstructionCount[tid]) / float32(m.TotalInstructions)
		for i := 0; i < len(averageRth); i++ {
			averageRth[i] += int(float32(rth[i]) * weight)
		}
	}
	model := algorithm.NewAETModel(averageRth)
	return model.MRC(cacheSize)
}
