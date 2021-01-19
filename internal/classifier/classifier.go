package classifier

import (
	"context"
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/pqos"
	"github.com/packagewjx/resourcemanager/internal/sampler/perf"
	"github.com/pkg/errors"
	"log"
	"math"
	"os"
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

type Config struct {
}

type Result struct {
	Group     *core.ProcessGroup
	Error     error
	Processes []*ProcessResult
}

type ProcessResult struct {
	Pid               int
	Error             error
	Characteristic    MemoryCharacteristic
	StatResultAllWays *perf.StatResult
	StatResultTwoWays *perf.StatResult
}

type Classifier interface {
	// 对一个进程组进行分类。对于
	Classify(ctx context.Context, group *core.ProcessGroup) <-chan *Result
}

func New(_ *Config) (Classifier, error) {
	return &impl{
		logger: log.New(os.Stdout, fmt.Sprintf("Classifier: "), log.Lmsgprefix|log.LstdFlags|log.Lshortfile),
		mpkiStat: &metricStat{
			data: []float64{},
			sum:  0,
			avg:  0,
			std:  0,
		},
	}, nil
}

type impl struct {
	reservoirSize int
	mpkiStat      *metricStat
	logger        *log.Logger
}

var _ Classifier = &impl{}

func (c *impl) Classify(ctx context.Context, group *core.ProcessGroup) <-chan *Result {
	resultCh := make(chan *Result, 1)
	go func(group *core.ProcessGroup) {
		defer close(resultCh)
		c.logger.Printf("开始对进程组 %s 执行分类", group.Id)
		processResults := c.perfProcesses(ctx, group)
		errCount := 0
		for _, result := range processResults {
			if result.Error == nil {
				result.Characteristic = c.determineCharacteristic(result)
			} else {
				c.logger.Printf("进程组 %s 进程 %d 分类出错：%v", group.Id, result.Pid, result.Error)
				errCount++
			}
		}
		res := &Result{
			Group:     group,
			Processes: processResults,
		}
		if errCount == len(processResults) {
			res.Error = fmt.Errorf("采样全部出现错误")
		}
		c.logger.Printf("进程组 %s 分类结束", group.Id)
		resultCh <- res
	}(group)
	return resultCh
}

func (c *impl) perfProcesses(ctx context.Context, group *core.ProcessGroup) []*ProcessResult {
	// 首先在全缓存way时测试一次
	processResults := make([]*ProcessResult, len(group.Pid))
	for i := 0; i < len(processResults); i++ {
		processResults[i] = &ProcessResult{
			Pid:            group.Pid[i],
			Characteristic: MemoryCharacteristicToDetermine,
		}
	}

	c.logger.Printf("正在对进程组 %s 进行缓存way为2的perf stat", group.Id)
	err := pqos.SetCLOSScheme([]*pqos.CLOSScheme{
		{
			CLOSNum:     1,
			WayBit:      0x3,
			MemThrottle: 0,
			Processes:   group.Pid,
		},
	})
	if err != nil {
		for _, result := range processResults {
			result.Error = errors.Wrap(err, "无法设置缓存")
		}
		return processResults
	}
	perfCh := perf.NewPerfStatRunner(group).Start(ctx)
	perfResult := <-perfCh
	for i, pid := range group.Pid {
		perfProcessResult := perfResult[pid]
		if perfProcessResult.Error != nil {
			processResults[i].Error = perfProcessResult.Error
		} else {
			processResults[i].StatResultTwoWays = perfProcessResult
		}
	}

	c.logger.Printf("正在对进程组 %s 进行全缓存way perf stat", group.Id)
	_ = pqos.SetCLOSScheme([]*pqos.CLOSScheme{
		{
			CLOSNum:   0,
			Processes: group.Pid,
		},
	})
	perfCh = perf.NewPerfStatRunner(group).Start(ctx)
	perfResult = <-perfCh
	for i, pid := range group.Pid {
		perfProcessResult := perfResult[pid]
		if perfProcessResult.Error != nil {
			processResults[i].Error = perfProcessResult.Error
		} else {
			processResults[i].StatResultAllWays = perfProcessResult
		}
	}
	return processResults
}

func (c *impl) determineCharacteristic(p *ProcessResult) MemoryCharacteristic {
	all := p.StatResultAllWays
	two := p.StatResultTwoWays
	config := core.RootConfig.Algorithm.Classify
	bully := func() bool {
		// 与论文一致
		ipcVeryLow := all.InstructionPerCycle() < config.IPCVeryLow || two.InstructionPerCycle() < config.IPCVeryLow
		allHigh := all.LLCMissPerKiloInstructions() >= config.MPKIVeryHigh && all.LLCHitPerKiloInstructions() >= config.HPKIVeryHigh
		twoHigh := two.LLCMissPerKiloInstructions() >= config.MPKIVeryHigh && two.LLCHitPerKiloInstructions() >= config.HPKIVeryHigh
		return ipcVeryLow && allHigh || twoHigh
	}
	squanderer := func() bool {
		// 与论文一致
		return (all.LLCHitPerKiloInstructions() >= config.HPKIVeryHigh && all.LLCMissPerKiloInstructions() >= config.MPKIHigh) ||
			(two.LLCHitPerKiloInstructions() >= config.HPKIVeryHigh && two.LLCMissPerKiloInstructions() >= config.MPKIHigh)
	}
	medium := func() bool {
		ipcMedium := all.InstructionPerCycle() >= config.IPCLow
		missRateDownSignificant := (two.LLCMissRate()-all.LLCMissRate())/two.LLCMissRate() > config.SignificantChangeThreshold
		ipcUp := (all.InstructionPerCycle()-two.InstructionPerCycle())/two.InstructionPerCycle() >= config.NoChangeThreshold
		return ipcMedium && missRateDownSignificant || ipcUp
	}
	sensitive := func() bool {
		ipcLow := all.InstructionPerCycle() < config.IPCLow
		missRateDownSignificant := (two.LLCMissRate()-all.LLCMissRate())/two.LLCMissRate() > config.SignificantChangeThreshold
		ipcUp := (all.InstructionPerCycle()-two.InstructionPerCycle())/two.InstructionPerCycle() >= config.NoChangeThreshold
		return ipcLow && missRateDownSignificant || ipcUp
	}
	nonCritical := func() bool {
		// APKI小于1，IPC基本不变
		apkiLow := all.AccessLLCPerInstructions()*1000.0 < config.APKILow || two.AccessLLCPerInstructions()*1000.0 < config.APKILow
		ipcNonChange := math.Abs(all.InstructionPerCycle()-two.InstructionPerCycle())/all.InstructionPerCycle() < config.NoChangeThreshold
		return apkiLow && ipcNonChange
	}

	if bully() {
		return MemoryCharacteristicBully
	} else if squanderer() {
		return MemoryCharacteristicSquanderer
	} else if nonCritical() {
		return MemoryCharacteristicNonCritical
	} else if medium() {
		return MemoryCharacteristicMedium
	} else if sensitive() {
		return MemoryCharacteristicSensitive
	} else {
		c.logger.Printf("进程 %d 没有分类，暂定为non critical", all.Pid)
		return MemoryCharacteristicNonCritical
	}
}

var _ Classifier = &impl{}
