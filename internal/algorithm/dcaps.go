package algorithm

import (
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/pqos"
	"github.com/packagewjx/resourcemanager/internal/sampler/perf"
	"github.com/packagewjx/resourcemanager/internal/utils"
)

type predictResult struct {
	Pid       int
	MissRate  float64
	IPC       float64
	Occupancy int // L3缓存占用，单位为缓存行
}

// DCAPS算法输入。以进程为单位进行分配的计算。
type ProgramMetric struct {
	Pid      int
	MRC      []float32
	PerfStat *perf.PerfStatResult
}

type predictMetric struct {
	averageMpki     float64
	throughput      float64
	averageSlowDown float64
	fairSlowDown    float64
	maximumSlowDown float64
}

type predictContext struct {
	ProgramMetric
	predictResult
	scheme    *pqos.CLOSScheme
	apc       float64
	miss      int
	pEviction float64
}

var numWays, numSets, _ = utils.GetL3Cap()

func estimateIPC(pred *predictContext) float64 {
	cpiBase := pred.PerfStat.CyclesPerNoAccessInstructions()
	latCache := pred.PerfStat.AverageCacheHitLatency()
	predictMissPenalty := pred.PerfStat.AccessLLCPerInstructions() * pred.MissRate * pred.PerfStat.AverageCacheMissLatency()
	cpi := cpiBase + latCache + predictMissPenalty
	return 1 / cpi
}

func getMaxWayBit(ways int) int {
	return (1 << ways) - 1
}

func doPredict(programs []*predictContext) {
	// 初始化为均等分
	occupancy := make([][]int, len(programs))
	intervalMiss := make([][]int, len(programs))
	equalShare := numWays * numSets
	for i := 0; i < len(occupancy); i++ {
		occupancy[i] = make([]int, numWays)
		intervalMiss[i] = make([]int, numWays)
		mask := 0x1
		schemeWays := utils.NumBits(programs[i].scheme.WayBit)
		for j := 0; j < schemeWays; j++ {
			if mask&programs[j].scheme.WayBit == mask {
				occupancy[i][j] = equalShare / schemeWays
			}
		}
	}

	step := core.RootConfig.Algorithm.DCAPS.InitialStep
	for iter := 0; iter < core.RootConfig.Algorithm.DCAPS.MaxIteration; iter++ {
		var PBase float64 = 0
		// Occupancy to Miss Rate
		for i, program := range programs {
			program.Occupancy = 0
			for j := 0; j < len(occupancy[i]); j++ {
				program.Occupancy += occupancy[i][j]
			}
			program.MissRate = float64(program.MRC[program.Occupancy])
			program.IPC = estimateIPC(program)
			program.apc = program.IPC * program.PerfStat.AccessPerInstruction()
			program.miss = int(program.MissRate * program.apc * step)
			PBase += float64(program.Occupancy) / program.apc
		}

		// Eviction Probability
		for _, program := range programs {
			program.pEviction = 1 / (PBase * program.apc / float64(program.Occupancy))
		}

		// Miss Rate to Occupancy
		for j := 0; j < numWays; j++ {
			totalIntervalMiss := 0
			for i := 0; i < len(occupancy); i++ {
				if occupancy[i][j] == 0 {
					continue
				}
				intervalMiss[i][j] = programs[i].miss / utils.NumBits(programs[i].scheme.WayBit)
				totalIntervalMiss += intervalMiss[i][j]
			}
			for i := 0; i < len(occupancy); i++ {
				occupancy[i][j] = occupancy[i][j] + intervalMiss[i][j] -
					int(float64(totalIntervalMiss)*programs[i].pEviction)
			}
		}
		if step > core.RootConfig.Algorithm.DCAPS.MinStep {
			step *= core.RootConfig.Algorithm.DCAPS.StepReductionRatio
		}
	}
}

func calculateMetric(list []*predictContext) *predictMetric {
	panic("implement me")
}

// 返回负数代表a好，返回正数代表b好，0代表相等
func compareMetric(a, b *predictMetric) int {
	panic("implement me")
}

func randomNeighbor(list []*predictContext) []*predictContext {
	panic("implement me")
}

// DCAPS算法修改版
func DCAPS(processes []*ProgramMetric) []*pqos.CLOSScheme {
	list := make([]*predictContext, len(processes))
	idxMap := make(map[int]int)
	for i, metric := range processes {
		list[i] = &predictContext{
			ProgramMetric: *metric,
			predictResult: predictResult{},
			scheme: &pqos.CLOSScheme{
				CLOSNum:     0,
				WayBit:      0x7FF,
				MemThrottle: 100,
				Processes:   nil,
			},
			apc:       0,
			miss:      0,
			pEviction: 0,
		}
		idxMap[metric.Pid] = i
	}

	// 计算分配方案

	// 预测IPC等
	doPredict(list)

	panic("implement me")
}
