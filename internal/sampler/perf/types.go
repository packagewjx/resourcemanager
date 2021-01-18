package perf

import (
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/core"
	"log"
)

const (
	pAllLoads          = "mem_inst_retired.all_loads"
	pAllStores         = "mem_inst_retired.all_stores"
	pCycles            = "cpu_clk_unhalted.thread"
	pInstructions      = "inst_retired.any"
	pL3MissCycles      = "cycle_activity.cycles_l3_miss"
	pMemAnyCycles      = "cycle_activity.cycles_mem_any"
	pL3HitCommon       = "mem_load_retired.l3_hit"
	pL3MissCommon      = "mem_load_retired.l3_miss"
	pL3HitSkyLake      = "cpu/event=0xb7,umask=0x01,offcore_rsp=0x801C0003,name=L3Hit/"
	pL3MissSkyLake     = "cpu/event=0xb7,umask=0x01,offcore_rsp=0x84000003,name=L3Miss/"
	pL3HitCascadeLake  = "offcore_response.all_data_rd.l3_hit.any_snoop"
	pL3MissCascadeLake = "offcore_response.all_data_rd.l3_miss.any_snoop"
)

var (
	cascadeLakePerfEvents = fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s", pAllLoads, pAllStores,
		pL3HitSkyLake, pL3MissSkyLake, pCycles, pInstructions, pL3MissCycles, pMemAnyCycles)
	skyLakePerfEvents = fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s", pAllLoads, pAllStores,
		pL3HitCascadeLake, pL3MissCascadeLake, pCycles, pInstructions, pL3MissCycles, pMemAnyCycles)
	commonPerfEvents = fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s", pAllLoads, pAllStores,
		pL3HitCommon, pL3MissCommon, pCycles, pInstructions, pL3MissCycles, pMemAnyCycles)
)

func getEventList() string {
	switch core.RootConfig.PerfStat.MicroArchitecture {
	case core.MicroArchitectureNameSkyLake:
		return cascadeLakePerfEvents
	case core.MicroArchitectureNameCascadeLake:
		return skyLakePerfEvents
	default:
		log.Printf("未知微处理器架构名 %s", core.RootConfig.PerfStat.MicroArchitecture)
		return commonPerfEvents
	}
}

type eventSetter func(r *StatResult, count uint64)

var (
	llcHitSetter eventSetter = func(r *StatResult, count uint64) {
		r.LLCHit = count
	}
	llcMissSetter eventSetter = func(r *StatResult, count uint64) {
		r.LLCMiss = count
	}
)

var eventSetterMap = map[string]eventSetter{
	pAllLoads: func(r *StatResult, count uint64) {
		r.AllLoads = count
	},
	pAllStores: func(r *StatResult, count uint64) {
		r.AllStores = count
	},
	pCycles: func(r *StatResult, count uint64) {
		r.Cycles = count
	},
	pInstructions: func(r *StatResult, count uint64) {
		r.Instructions = count
	},
	pL3MissCycles: func(r *StatResult, count uint64) {
		r.LLCMissCycles = count
	},
	pMemAnyCycles: func(r *StatResult, count uint64) {
		r.MemAnyCycles = count
	},
	"L3Hit":            llcHitSetter,
	"L3Miss":           llcMissSetter,
	pL3HitCascadeLake:  llcHitSetter,
	pL3MissCascadeLake: llcMissSetter,
}

type StatResult struct {
	Pid           int
	Error         error
	AllLoads      uint64 // mem_inst_retired.all_loads
	AllStores     uint64 // mem_inst_retired.all_stores
	Instructions  uint64 // inst_retired.any
	Cycles        uint64 // cpu_clk_unhalted.thread
	MemAnyCycles  uint64 // cycle_activity.cycles_mem_any 可能有prefetch的
	LLCMissCycles uint64 // cycle_activity.cycles_l3_miss 看上去是只有demand read 和 rfo
	LLCHit        uint64 // L3Hit
	LLCMiss       uint64 // L3Miss
}

func (p *StatResult) SetCount(eventName string, val uint64) error {
	setter := eventSetterMap[eventName]
	if setter == nil {
		return fmt.Errorf("没有记录事件名 %s", eventName)
	}
	setter(p, val)
	return nil
}

func (p *StatResult) Clone() core.Cloneable {
	if p == nil {
		return nil
	} else {
		return &StatResult{
			Pid:           p.Pid,
			Error:         p.Error,
			AllLoads:      p.AllLoads,
			AllStores:     p.AllStores,
			Instructions:  p.Instructions,
			Cycles:        p.Cycles,
			MemAnyCycles:  p.MemAnyCycles,
			LLCMissCycles: p.LLCMissCycles,
			LLCHit:        p.LLCHit,
			LLCMiss:       p.LLCMiss,
		}
	}
}

func (p *StatResult) LLCMissRate() float64 {
	return float64(p.LLCMiss) / float64(p.LLCMiss+p.LLCHit)
}

func (p *StatResult) AccessPerInstruction() float64 {
	return float64(p.AllLoads+p.AllStores) / float64(p.Instructions)
}

func (p *StatResult) AverageCacheHitLatency() float64 {
	cycles := float64(p.MemAnyCycles - p.LLCMissCycles)
	hitCount := float64(p.AllStores + p.AllLoads - p.LLCMiss)
	return cycles / hitCount
}

func (p *StatResult) AverageCacheMissLatency() float64 {
	return float64(p.LLCMissCycles) / float64(p.LLCMiss)
}

func (p *StatResult) InstructionPerCycle() float64 {
	return float64(p.Instructions) / float64(p.Cycles)
}

func (p *StatResult) LLCMissPerKiloInstructions() float64 {
	return float64(p.LLCMiss) / float64(p.Instructions) * 1000
}

func (p *StatResult) HitPerKiloInstructions() float64 {
	return float64(p.AllStores+p.AllLoads-p.LLCMiss) / float64(p.Instructions) * 1000
}

func (p *StatResult) LLCHitPerKiloInstructions() float64 {
	return float64(p.LLCHit) / float64(p.Instructions) * 1000
}

// 不访问内存的指令的CPI
func (p *StatResult) CyclesPerNoAccessInstructions() float64 {
	return float64(p.Cycles-p.MemAnyCycles) / float64(p.Instructions-p.AllStores-p.AllLoads)
}

func (p *StatResult) AccessLLCPerInstructions() float64 {
	return float64(p.LLCMiss+p.LLCHit) / float64(p.Instructions)
}
