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
	Name     string // 用于日志显示
	Kill     bool
	RootDir  string // 预留，用于容器使用
	Consumer CacheLineAddressConsumer
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

type ShenModelConsumer interface {
	CacheLineAddressConsumer
	GetReuseTimeHistogram() map[int][]float64
}

type shenModelConsumer struct {
	m       map[int]*algorithm.ShenModel
	maxTime int
}

func (s *shenModelConsumer) Consume(tid int, addr []uint64) {
	c, ok := s.m[tid]
	if !ok {
		c = algorithm.NewShenModel(s.maxTime)
		s.m[tid] = c
	}
	c.AddAddresses(addr)
}

func (s *shenModelConsumer) GetReuseTimeHistogram() map[int][]float64 {
	res := make(map[int][]float64)
	for tid, model := range s.m {
		res[tid] = model.ReuseDistanceHistogram()
	}
	return res
}

func NewShenModelConsumer(maxTime int) ShenModelConsumer {
	return &shenModelConsumer{
		m:       make(map[int]*algorithm.ShenModel),
		maxTime: maxTime,
	}
}
