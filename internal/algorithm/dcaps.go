package algorithm

import (
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/pqos"
	"github.com/packagewjx/resourcemanager/internal/sampler/perf"
	"github.com/packagewjx/resourcemanager/internal/utils"
	"math"
	"math/rand"
	"sync"
)

// DCAPS算法输入。以进程为单位进行分配的计算。
type ProgramMetric struct {
	Pid      int
	MRC      []float32
	PerfStat *perf.PerfStatResult
}

type predictSystemMetric struct {
	averageMpki    float64
	throughput     float64 // 定义为IPC的总和
	averageSpeedUp float64
	maximumSpeedUp float64
}

//var numWays, numSets, _ = utils.GetL3Cap()
//var numClos = 8

func absInt(a int) int {
	if a < 0 {
		return -a
	} else {
		return a
	}
}

func cloneSchemes(schemes []*pqos.CLOSScheme) []*pqos.CLOSScheme {
	clone := make([]*pqos.CLOSScheme, len(schemes))
	for i, scheme := range schemes {
		processClone := make([]int, len(scheme.Processes))
		copy(processClone, scheme.Processes)
		clone[i] = &pqos.CLOSScheme{
			CLOSNum:     scheme.CLOSNum,
			WayBit:      scheme.WayBit,
			MemThrottle: scheme.MemThrottle,
			Processes:   processClone,
		}
	}
	return clone
}

type predictData struct {
	program      *ProgramMetric
	wayOccupancy []int
	occupancy    int
	intervalMiss []int
	apc          float64
	miss         int
	pEviction    float64
	ipc          float64
	missRate     float64
	schemeNum    int
}

func initEqualShare(schemes []*pqos.CLOSScheme, data []*predictData, numWays, numSets int) {
	schemeProgramCount := make([]int, len(schemes))
	for _, d := range data {
		schemeProgramCount[d.schemeNum]++
	}
	wg := sync.WaitGroup{}
	for i := 0; i < numWays; i++ {
		wg.Add(1)
		go func(wi int) {
			// 每一个way的所有program均分这个way的所有set
			mask := 1 << wi
			wayProgramCount := 0
			waySchemes := map[int]struct{}{}
			for j := 0; j < len(schemes); j++ {
				if mask&schemes[j].WayBit == mask {
					wayProgramCount += schemeProgramCount[j]
					waySchemes[j] = struct{}{}
				}
			}
			equalShare := numSets / wayProgramCount

			for j := 0; j < len(data); j++ {
				if _, ok := waySchemes[data[j].schemeNum]; ok {
					data[j].wayOccupancy[wi] = equalShare
				}
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
}

func estimateIPC(p *predictData) float64 {
	cpiBase := p.program.PerfStat.CyclesPerNoAccessInstructions()
	latCache := p.program.PerfStat.AverageCacheHitLatency()
	predictMissPenalty := p.program.PerfStat.AccessLLCPerInstructions() * p.missRate * p.program.PerfStat.AverageCacheMissLatency()
	cpi := cpiBase + latCache + predictMissPenalty
	return 1 / cpi
}

func doPredict(programs []*ProgramMetric, schemes []*pqos.CLOSScheme, schemeMap []int, numWays, numSets int) (ipc, missRate []float64) {
	data := make([]*predictData, len(programs))
	for i := 0; i < len(programs); i++ {
		data[i] = &predictData{
			program:      programs[i],
			wayOccupancy: make([]int, numWays),
			occupancy:    0,
			intervalMiss: make([]int, numWays),
			apc:          0,
			miss:         0,
			pEviction:    0,
			ipc:          0,
			missRate:     0,
			schemeNum:    schemeMap[i],
		}
	}
	initEqualShare(schemes, data, numWays, numSets)

	// DCAPS Algorithm 1 Main Iteration
	step := core.RootConfig.Algorithm.DCAPS.InitialStep
	aggregateChangeOfOccupancy := math.MaxInt32
	for iter := 0; iter < core.RootConfig.Algorithm.DCAPS.MaxIteration &&
		aggregateChangeOfOccupancy > core.RootConfig.Algorithm.DCAPS.AggregateChangeOfOccupancyThreshold; iter++ {
		var PBase float64 = 0
		aggregateChangeOfOccupancy = 0
		// Occupancy to Miss Rate
		wg := sync.WaitGroup{}
		lock := sync.Mutex{}
		for _, d := range data {
			wg.Add(1)
			go func(data *predictData) {
				data.occupancy = 0
				for _, o := range data.wayOccupancy {
					data.occupancy += o
				}
				data.missRate = float64(data.program.MRC[data.occupancy])
				data.ipc = estimateIPC(data)
				data.apc = data.ipc * data.program.PerfStat.AccessPerInstruction()
				data.miss = int(data.missRate * data.apc * step)
				lock.Lock()
				PBase += float64(data.occupancy) / data.apc
				lock.Unlock()
				wg.Done()
			}(d)
		}
		wg.Wait()

		// Eviction Probability
		for _, d := range data {
			d.pEviction = 1 / (PBase * d.apc / float64(d.occupancy))
		}

		// Miss Rate to Occupancy
		for i := 0; i < numWays; i++ {
			wg.Add(1)
			go func(wi int) {
				delta := 0
				totalIntervalMiss := 0
				for _, d := range data {
					if d.wayOccupancy[wi] == 0 {
						continue
					}
					d.intervalMiss[wi] = d.miss / utils.NumBits(schemes[d.schemeNum].WayBit)
					totalIntervalMiss += d.intervalMiss[wi]
				}
				for _, d := range data {
					newWayOccupancy := d.wayOccupancy[wi] + d.intervalMiss[wi] - int(float64(totalIntervalMiss)*d.pEviction)
					delta += absInt(newWayOccupancy - d.wayOccupancy[wi])
					d.wayOccupancy[wi] = newWayOccupancy
				}
				lock.Lock()
				aggregateChangeOfOccupancy += delta
				lock.Unlock()
				wg.Done()
			}(i)
		}
		wg.Wait()
		if step > core.RootConfig.Algorithm.DCAPS.MinStep {
			step *= core.RootConfig.Algorithm.DCAPS.StepReductionRatio
		}
	}

	// 将结果提取出来返回
	ipc = make([]float64, len(programs))
	missRate = make([]float64, len(programs))
	for i, d := range data {
		ipc[i] = d.ipc
		missRate[i] = d.missRate
	}
	return
}

func calculateSystemMetric(programs []*ProgramMetric, ipc, missRate []float64) *predictSystemMetric {
	res := &predictSystemMetric{}
	// 初始化
	totalMpki := float64(0)
	totalSpeedUp := float64(0)
	res.maximumSpeedUp = math.MaxInt32
	// 遍历所有数据
	for pi, p := range programs {
		totalMpki += p.PerfStat.AccessPerInstruction() * missRate[pi] * 1000
		res.throughput += ipc[pi]
		oldIpc := p.PerfStat.InstructionPerCycle()
		speedUp := oldIpc / ipc[pi]
		totalSpeedUp += speedUp
		if speedUp < res.maximumSpeedUp {
			res.maximumSpeedUp = speedUp
		}
	}
	// 计算平均
	res.averageMpki = totalMpki / float64(len(programs))
	res.averageSpeedUp = totalSpeedUp / float64(len(programs))
	return res
}

// 返回正数代表a好，返回负数代表b好，0代表相等
func compareMetric(a, b *predictSystemMetric) int {
	aScore := 0
	bScore := 0
	preferLarger := func(x, y float64, add int) {
		if x > y {
			aScore += add
		} else if x < y {
			bScore += add
		}
	}
	preferSmaller := func(x, y float64, add int) {
		if x < y {
			aScore += add
		} else if x > y {
			bScore += add
		}
	}
	preferSmaller(a.averageSpeedUp, b.averageSpeedUp, 2)
	preferSmaller(a.maximumSpeedUp, b.maximumSpeedUp, 1)
	preferLarger(a.throughput, b.throughput, 1)
	preferSmaller(a.averageMpki, b.averageMpki, 2)
	return aScore - bScore
}

func randomNeighbor(schemes []*pqos.CLOSScheme, schemeMap []int, numWays, numClos int) (newSchemes []*pqos.CLOSScheme, newMap []int) {
	randClos := func() int {
		return 2 + rand.Intn(numClos-2)
	}
	// 前置条件：
	// 1. CLOS 0预留给系统和未分配的程序
	// 2. CLOS 1将只有两个way可用，分配给Bully、Squanderer和NonCritical去竞争，其他程序用其他的way

	// 改变的内容可以是
	// 1. Way分配改变：way + 1, way - 1, way更改位置。在只剩下1个way的时候不会继续减。不会动CLOS 0与CLOS 1的设置
	// 2. Process更改：进程从一个CLOS移动到另一个CLOS
	// 两个的概率是不一样的，这个概率应该需要研究
	sample := rand.Float64()
	if sample < core.RootConfig.Algorithm.DCAPS.ProbabilityChangeScheme {
		newSchemes = cloneSchemes(schemes)
		newMap = schemeMap
		// 随机修改Way
		clos := randClos()                  // 随机选一个更改
		pos := rand.Intn(numWays)           // 随机挑选一个位置
		newSchemes[clos].WayBit ^= 1 << pos // 异或一个位置，可能加可能减
		if newSchemes[clos].WayBit == 0 {
			pos = rand.Intn(numWays)
			newSchemes[clos].WayBit ^= 1 << pos
		}
	} else {
		newSchemes = schemes
		newMap = make([]int, len(schemeMap))
		copy(newMap, schemeMap)
		// 随机修改Process的CLOS分配
		pos := rand.Intn(len(newMap))
		oldClos := newMap[pos]
		for newMap[pos] == oldClos {
			newMap[pos] = randClos()
		}
	}
	return
}

func readFromOldSchemes(programs []*ProgramMetric, oldSchemes []*pqos.CLOSScheme, numWays, numClos int) (schemes []*pqos.CLOSScheme, schemeMap []int) {
	// 首先检查oldScheme对号入座
	schemes = make([]*pqos.CLOSScheme, numClos)
	schemeMap = make([]int, len(programs))
	for _, scheme := range oldSchemes {
		schemes[scheme.CLOSNum] = scheme
	}
	// 填充空的CLOS
	for i := 0; i < len(schemes); i++ {
		if schemes[i] == nil {
			schemes[i] = &pqos.CLOSScheme{
				CLOSNum:     i,
				WayBit:      utils.GetLowestBits(numWays),
				MemThrottle: 100,
				Processes:   nil,
			}
		}
	}
	// 将schemeMap赋值。programs中有的，但是oldScheme中没有的，赋值为0即可。oldSchemes中有的而programs没有的则不需要处理
	pidIdxMap := make(map[int]int)
	for pi, program := range programs {
		pidIdxMap[program.Pid] = pi
	}
	for _, scheme := range oldSchemes {
		for _, process := range scheme.Processes {
			if idx, ok := pidIdxMap[process]; ok {
				schemeMap[idx] = scheme.CLOSNum
			}
		}
	}
	return
}

// DCAPS算法修改版。
// oldScheme可以为nil，此时将使用初始化方案。当不为nil时，将用于平滑两次分配方案之间的改变。
// numClos至少为3，前两个CLOS不会使用，预留给其他类型的进程l
// 为保证性能，将不会检查输入。
func DCAPS(programs []*ProgramMetric, oldSchemes []*pqos.CLOSScheme, numWays, numSets, numClos int) []*pqos.CLOSScheme {
	var schemes []*pqos.CLOSScheme
	var schemeMap []int // 将每个程序的closNum保存下来用于加速查找过程
	if oldSchemes != nil {
		schemes, schemeMap = readFromOldSchemes(programs, oldSchemes, numWays, numClos)
	} else {
		schemes = make([]*pqos.CLOSScheme, numClos)
		for i := 0; i < len(schemes); i++ {
			schemes[i] = &pqos.CLOSScheme{
				CLOSNum:     i,
				WayBit:      utils.GetLowestBits(numWays),
				MemThrottle: 100,
				Processes:   nil,
			}
		}
		schemeMap = make([]int, len(programs))
	}

	ipc, missRate := doPredict(programs, schemes, schemeMap, numWays, numSets)
	metric := calculateSystemMetric(programs, ipc, missRate)
	var bestScheme = schemes
	var bestMetric = metric
	var bestSchemeMap = schemeMap

	// 模拟退火算法计算分配方案
	t := core.RootConfig.Algorithm.DCAPS.InitialTemperature
	k := core.RootConfig.Algorithm.DCAPS.K

	for t > core.RootConfig.Algorithm.DCAPS.TemperatureMin {
		newSchemes, newSchemeMap := randomNeighbor(schemes, schemeMap, numWays, numClos)
		newIpc, newMissRate := doPredict(programs, newSchemes, newSchemeMap, numWays, numSets)
		newMetric := calculateSystemMetric(programs, newIpc, newMissRate)
		if compareMetric(bestMetric, newMetric) < 0 {
			bestMetric = newMetric
			bestScheme = newSchemes
			bestSchemeMap = newSchemeMap
		}
		// 决定是否更换新的Metric
		diff := compareMetric(metric, newMetric)
		if diff < 0 || math.Exp(float64(-diff)/(k*t)) <= rand.Float64() {
			// math.Exp(float64(-diff)/(k*t)) 随着t减小，假设diff基本不变，结果会越来越大，最后概率就越来越低了
			metric = newMetric
			schemeMap = newSchemeMap
			schemes = newSchemes
		}
		t *= core.RootConfig.Algorithm.DCAPS.TemperatureReductionRatio
	}

	// 组装结果
	for pi, s := range bestSchemeMap {
		bestScheme[s].Processes = append(bestScheme[s].Processes, programs[pi].Pid)
	}

	return bestScheme
}
