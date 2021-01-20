package memrecord

import (
	"context"
	"github.com/packagewjx/resourcemanager/internal/algorithm"
	"github.com/packagewjx/resourcemanager/internal/core"
	"log"
)

type MemRecorder interface {
	// 对一个命令进行取样，返回该命令的所有子线程的取样结果，结果以RTHCalculator呈现，可用于计算MRC
	RecordCommand(ctx context.Context, request *MemRecordRunRequest) <-chan *MemRecordResult
	RecordProcess(ctx context.Context, request *MemRecordAttachRequest) <-chan *MemRecordResult
}

type MemRecordBaseRequest struct {
	Factory RTHCalculatorFactory
	Name    string // 用于日志显示
	Kill    bool
	RootDir string // 预留，用于容器使用
}

type MemRecordRunRequest struct {
	MemRecordBaseRequest
	Cmd  string
	Args []string
}

type MemRecordAttachRequest struct {
	MemRecordBaseRequest
	Pid int
}

type MemRecordResult struct {
	ThreadTrace            map[int]algorithm.RTHCalculator
	ThreadInstructionCount map[int]uint64
	TotalInstructions      uint64
	Err                    error
}

type RTHCalculatorFactory func(tid int) algorithm.RTHCalculator

var (
	factoryFullTrace RTHCalculatorFactory = func(tid int) algorithm.RTHCalculator {
		return algorithm.FullTraceCalculator()
	}
	factoryReservoir RTHCalculatorFactory = func(tid int) algorithm.RTHCalculator {
		return algorithm.ReservoirCalculator(core.RootConfig.MemTrace.ReservoirSize)
	}
	factoryNoUpdate RTHCalculatorFactory = func(tid int) algorithm.RTHCalculator {
		return algorithm.NoUpdateCalculator{}
	}
)

func GetCalculatorFromRootConfig() RTHCalculatorFactory {
	var factory RTHCalculatorFactory
	switch core.RootConfig.MemTrace.RthCalculatorType {
	case core.RthCalculatorTypeFull:
		factory = factoryFullTrace
	case core.RthCalculatorTypeReservoir:
		factory = factoryReservoir
	case core.RthCalculatorTypeNoUpdate:
		factory = factoryNoUpdate
	default:
		log.Printf("RTHCalculator值错误：%s，将使用FullTrace", core.RootConfig.MemTrace.RthCalculatorType)
		factory = factoryFullTrace
	}
	return factory
}
