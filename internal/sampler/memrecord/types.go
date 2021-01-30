package memrecord

import (
	"context"
	"github.com/packagewjx/resourcemanager/internal/algorithm"
	"github.com/packagewjx/resourcemanager/internal/core"
	"log"
)

type MemRecorder interface {
	// 对一个命令进行取样，返回该命令的所有子线程的取样结果，结果以RTHCalculator呈现，可用于计算MRC
	RecordCommand(ctx context.Context, request *RunRequest) (<-chan *Result, error)
	RecordProcess(ctx context.Context, request *AttachRequest) (<-chan *Result, error)
}

type BaseRequest struct {
	Name     string // 用于日志显示
	Kill     bool
	RootDir  string // 预留，用于容器使用
	Consumer CacheLineAddressConsumer
}

type RunRequest struct {
	BaseRequest
	Cmd  string
	Args []string
}

type AttachRequest struct {
	BaseRequest
	Pid int
}

type Result struct {
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
)

func GetCalculatorFromRootConfig() RTHCalculatorFactory {
	var factory RTHCalculatorFactory
	switch core.RootConfig.MemTrace.RthCalculatorType {
	case core.RthCalculatorTypeFull:
		factory = factoryFullTrace
	case core.RthCalculatorTypeReservoir:
		factory = factoryReservoir
	default:
		log.Printf("RTHCalculator值错误：%s，将使用FullTrace", core.RootConfig.MemTrace.RthCalculatorType)
		factory = factoryFullTrace
	}
	return factory
}

type CacheLineAddressConsumer interface {
	Consume(tid int, addr []uint64)
}

type RTHCalculatorConsumer interface {
	CacheLineAddressConsumer
	GetCalculatorMap() map[int]algorithm.RTHCalculator
}

type rthCalculatorConsumer struct {
	factory RTHCalculatorFactory
	cMap    map[int]algorithm.RTHCalculator
}

func (r *rthCalculatorConsumer) GetCalculatorMap() map[int]algorithm.RTHCalculator {
	return r.cMap
}

func NewRTHCalculatorConsumer(factory RTHCalculatorFactory) RTHCalculatorConsumer {
	return &rthCalculatorConsumer{
		factory: factory,
		cMap:    make(map[int]algorithm.RTHCalculator),
	}
}

func (r *rthCalculatorConsumer) Consume(tid int, addr []uint64) {
	c, ok := r.cMap[tid]
	if !ok {
		c = r.factory(tid)
		r.cMap[tid] = c
	}
	c.Update(addr)
}

type DummyConsumer struct {
}

func (d DummyConsumer) Consume(_ int, _ []uint64) {
}
