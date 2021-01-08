package algorithm

import (
	"github.com/packagewjx/resourcemanager/internal/librm"
	"github.com/packagewjx/resourcemanager/internal/utils"
)

const MaxIteration = 200
const InitialStep float32 = 10000.0
const MinStep float32 = 100
const StepReductionRatio float32 = 0.95

type PredictMetric struct {
	Pid       int
	MissRate  float32
	IPC       float32
	Occupancy int // L3缓存占用，单位为缓存行
}

type ProgramMetric struct {
	Pid          int
	MRC          []float32
	Instructions int
	L1Hit        int
	L2Hit        int
	L3Hit        int
	L3Miss       int
}

func (p ProgramMetric) Api() float32 {
	return float32(p.L1Hit+p.L2Hit+p.L3Hit+p.L3Miss) / float32(p.Instructions)
}

type predictContext struct {
	ProgramMetric
	PredictMetric
	scheme    *librm.CLOSScheme
	apc       float32
	miss      int
	pEviction float32
}

var cpiBase = utils.GetCPIBase()
var l1lat, l2lat, l3lat, memLat = utils.GetMemAccessLatency()
var numWays, numSets, _ = utils.GetL3Cap()

func estimateIPC(pred *predictContext) float32 {
	apiL1 := float32(pred.L1Hit) / float32(pred.Instructions)
	apiL2 := float32(pred.L2Hit) / float32(pred.Instructions)
	apiL3 := float32(pred.L3Hit+pred.L3Miss) / float32(pred.Instructions)
	cpi := cpiBase + float32(l1lat)*apiL1 + float32(l2lat)*apiL2 + float32(l3lat)*apiL3 + float32(memLat)*apiL3*pred.MissRate
	return 1 / cpi
}

func Predict(programs []*ProgramMetric, schemes []*librm.CLOSScheme) []*PredictMetric {
	programContext := make([]*predictContext, len(programs))
	idxMap := make(map[int]int)
	for i, metric := range programs {
		programContext[i] = &predictContext{
			ProgramMetric: *metric,
			PredictMetric: PredictMetric{},
			scheme:        nil,
			apc:           0,
			miss:          0,
			pEviction:     0,
		}
		idxMap[metric.Pid] = i
	}
	for _, scheme := range schemes {
		for _, pid := range scheme.Processes {
			programContext[idxMap[pid]].scheme = scheme
		}
	}

	doPredict(programContext)

	result := make([]*PredictMetric, len(programs))
	for i, context := range programContext {
		result[i] = &PredictMetric{}
		*result[i] = context.PredictMetric
	}

	return result
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
		numWays := utils.NumBits(programs[i].scheme.WayBit)
		for j := 0; j < numWays; j++ {
			if mask&programs[j].scheme.WayBit == mask {
				occupancy[i][j] = equalShare / numWays
			}
		}
	}

	step := InitialStep
	for iter := 0; iter < MaxIteration; iter++ {
		var PBase float32 = 0
		// Occupancy to Miss Rate
		for i, program := range programs {
			program.Occupancy = 0
			for j := 0; j < len(occupancy[i]); j++ {
				program.Occupancy += occupancy[i][j]
			}
			program.MissRate = program.MRC[program.Occupancy]
			program.IPC = estimateIPC(program)
			program.apc = program.IPC * program.Api()
			program.miss = int(program.MissRate * program.apc * step)
			PBase += float32(program.Occupancy) / program.apc
		}

		// Eviction Probability
		for _, program := range programs {
			program.pEviction = 1 / (PBase * program.apc / float32(program.Occupancy))
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
					int(float32(totalIntervalMiss)*programs[i].pEviction)
			}
		}
		if step > MinStep {
			step *= StepReductionRatio
		}
	}
}

func Allocate() {

}
