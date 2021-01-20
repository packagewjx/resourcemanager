package algorithm

import (
	"encoding/csv"
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/pqos"
	"github.com/packagewjx/resourcemanager/internal/sampler/perf"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

// from: https://stackoverflow.com/questions/31873396/is-it-possible-to-get-the-current-root-of-package-structure-as-a-string-in-golan
var (
	_, b, _, _ = runtime.Caller(0)
	testDir    = filepath.Dir(b)
)

func TestCloneSchemes(t *testing.T) {
	oldSchemes := []*pqos.CLOSScheme{
		{
			CLOSNum:     1,
			WayBit:      1,
			MemThrottle: 1,
			Processes:   []int{1},
		},
		{
			CLOSNum:     2,
			WayBit:      2,
			MemThrottle: 2,
			Processes:   []int{2},
		},
	}

	clone := cloneSchemes(oldSchemes)
	assert.Condition(t, func() (success bool) {
		return &clone != &oldSchemes && &clone[0] != &oldSchemes[0] &&
			&clone[0].Processes != &oldSchemes[0].Processes &&
			&clone[1] != &oldSchemes[1] && &clone[1].Processes != &oldSchemes[1].Processes
	})
	assert.Equal(t, oldSchemes[0].CLOSNum, clone[0].CLOSNum)
	assert.Equal(t, oldSchemes[0].WayBit, clone[0].WayBit)
	assert.Equal(t, oldSchemes[0].MemThrottle, clone[0].MemThrottle)
	assert.Equal(t, oldSchemes[1].CLOSNum, clone[1].CLOSNum)
	assert.Equal(t, oldSchemes[1].WayBit, clone[1].WayBit)
	assert.Equal(t, oldSchemes[1].MemThrottle, clone[1].MemThrottle)
}

func TestInitEqualShare(t *testing.T) {
	numWays := 11
	numSets := 20480
	schemes := []*pqos.CLOSScheme{
		{
			CLOSNum:     0,
			WayBit:      0x7FF,
			MemThrottle: 0,
			Processes:   nil,
		},
		{
			CLOSNum:     1,
			WayBit:      0x6,
			MemThrottle: 0,
			Processes:   nil,
		},
		{
			CLOSNum:     2,
			WayBit:      0xE,
			MemThrottle: 0,
			Processes:   nil,
		},
		{
			CLOSNum:     3,
			WayBit:      0xF0,
			MemThrottle: 0,
			Processes:   nil,
		},
	}
	data := []*predictData{
		{
			schemeNum:    0,
			wayOccupancy: make([]int, numWays),
		},
		{
			schemeNum:    1,
			wayOccupancy: make([]int, numWays),
		},
		{
			schemeNum:    1,
			wayOccupancy: make([]int, numWays),
		},
		{
			schemeNum:    2,
			wayOccupancy: make([]int, numWays),
		},
		{
			schemeNum:    2,
			wayOccupancy: make([]int, numWays),
		},
		{
			schemeNum:    2,
			wayOccupancy: make([]int, numWays),
		},
		{
			schemeNum:    3,
			wayOccupancy: make([]int, numWays),
		},
		{
			schemeNum:    3,
			wayOccupancy: make([]int, numWays),
		},
		{
			schemeNum:    3,
			wayOccupancy: make([]int, numWays),
		},
		{
			schemeNum:    3,
			wayOccupancy: make([]int, numWays),
		},
	}

	initEqualShare(schemes, data, numWays, numSets)

	// CLOS 0
	assert.Equal(t, numSets, data[0].wayOccupancy[0])
	assert.Equal(t, numSets/6, data[0].wayOccupancy[1])
	assert.Equal(t, numSets/6, data[0].wayOccupancy[2])
	assert.Equal(t, numSets/4, data[0].wayOccupancy[3])
	assert.Equal(t, numSets/5, data[0].wayOccupancy[4])
	assert.Equal(t, numSets/5, data[0].wayOccupancy[5])
	assert.Equal(t, numSets/5, data[0].wayOccupancy[6])
	assert.Equal(t, numSets/5, data[0].wayOccupancy[7])
	assert.Equal(t, numSets, data[0].wayOccupancy[8])
	assert.Equal(t, numSets, data[0].wayOccupancy[9])
	assert.Equal(t, numSets, data[0].wayOccupancy[10])

	// CLOS1
	assert.Equal(t, 0, data[1].wayOccupancy[0])
	assert.Equal(t, numSets/6, data[1].wayOccupancy[1])
	assert.Equal(t, numSets/6, data[1].wayOccupancy[2])
	assert.Equal(t, 0, data[1].wayOccupancy[3])
	assert.Equal(t, 0, data[1].wayOccupancy[4])
	assert.Equal(t, 0, data[1].wayOccupancy[5])
	assert.Equal(t, 0, data[1].wayOccupancy[6])
	assert.Equal(t, 0, data[1].wayOccupancy[7])
	assert.Equal(t, 0, data[1].wayOccupancy[8])
	assert.Equal(t, 0, data[1].wayOccupancy[9])
	assert.Equal(t, 0, data[1].wayOccupancy[10])

	// CLOS 2
	assert.Equal(t, 0, data[3].wayOccupancy[0])
	assert.Equal(t, numSets/6, data[3].wayOccupancy[1])
	assert.Equal(t, numSets/6, data[3].wayOccupancy[2])
	assert.Equal(t, numSets/4, data[3].wayOccupancy[3])
	assert.Equal(t, 0, data[3].wayOccupancy[4])
	assert.Equal(t, 0, data[3].wayOccupancy[5])
	assert.Equal(t, 0, data[3].wayOccupancy[6])
	assert.Equal(t, 0, data[3].wayOccupancy[7])
	assert.Equal(t, 0, data[3].wayOccupancy[8])
	assert.Equal(t, 0, data[3].wayOccupancy[9])
	assert.Equal(t, 0, data[3].wayOccupancy[10])

	// CLOS 3
	assert.Equal(t, 0, data[9].wayOccupancy[0])
	assert.Equal(t, 0, data[9].wayOccupancy[1])
	assert.Equal(t, 0, data[9].wayOccupancy[2])
	assert.Equal(t, 0, data[9].wayOccupancy[3])
	assert.Equal(t, numSets/5, data[9].wayOccupancy[4])
	assert.Equal(t, numSets/5, data[9].wayOccupancy[5])
	assert.Equal(t, numSets/5, data[9].wayOccupancy[6])
	assert.Equal(t, numSets/5, data[9].wayOccupancy[7])
	assert.Equal(t, 0, data[9].wayOccupancy[8])
	assert.Equal(t, 0, data[9].wayOccupancy[9])
	assert.Equal(t, 0, data[9].wayOccupancy[10])
}

func TestEstimateIPC(t *testing.T) {
	p := &predictData{
		program: &ProgramMetric{
			Pid: 1,
			MRC: nil,
			PerfStat: &perf.StatResult{
				Pid:           1,
				AllLoads:      10000,
				AllStores:     10000,
				Instructions:  50000,
				Cycles:        2462000, // 2432000 + 30000 * 1
				MemAnyCycles:  2432000, // 12000 * 200 + 8000 * 4
				LLCMissCycles: 2400000, // 12000 * 200
				LLCHit:        4000,
				LLCMiss:       12000,
			},
		},
		ipc:      0.5,
		missRate: 0.6,
	}
	assert.NotZero(t, estimateIPC(p))
}

func TestCalculateSystemMetric(t *testing.T) {
	programs := []*ProgramMetric{
		{
			// 高性能程序
			Pid: 1,
			MRC: nil,
			PerfStat: &perf.StatResult{
				AllLoads:     700,
				AllStores:    300,
				Cycles:       9000*0.5 + 1000*4,
				Instructions: 10000,
			},
		},
		{
			// 高IO程序
			Pid: 2,
			MRC: nil,
			PerfStat: &perf.StatResult{
				AllLoads:     4500,
				AllStores:    3500,
				Cycles:       uint64(float32(2000*0.5 + 8000*30)),
				Instructions: 10000,
			},
		},
		{
			// 平均
			Pid: 3,
			MRC: nil,
			PerfStat: &perf.StatResult{
				AllLoads:     1500,
				AllStores:    1500,
				Cycles:       7000*0.5 + 3000*10,
				Instructions: 10000,
			},
		},
	}
	ipc := []float64{1.5, 0.6, 0.9}
	missRate := []float64{0.01, 0.3, 0.1}
	metric := calculateSystemMetric(programs, ipc, missRate)
	assert.NotNil(t, metric)
	assert.NotZero(t, metric.averageMpki)
	assert.NotZero(t, metric.maximumSpeedUp)
	assert.NotZero(t, metric.averageSpeedUp)
	assert.NotZero(t, metric.throughput)
}

func TestCompareMetric(t *testing.T) {
	a := &predictSystemMetric{
		averageMpki:    1,
		throughput:     1,
		averageSpeedUp: 1,
		maximumSpeedUp: 1,
	}
	b := &predictSystemMetric{
		averageMpki:    1,
		throughput:     1,
		averageSpeedUp: 1,
		maximumSpeedUp: 1,
	}

	b.averageMpki = 2
	assert.Greater(t, compareMetric(a, b), 0)

	b.averageMpki = 1
	b.throughput = 0.8
	assert.Greater(t, compareMetric(a, b), 0)

	b.throughput = 1
	b.averageSpeedUp = 1.2
	assert.Greater(t, compareMetric(a, b), 0)

	b.averageSpeedUp = 1
	b.maximumSpeedUp = 1.2
	assert.Greater(t, compareMetric(a, b), 0)
}

func TestRandomNeighbor(t *testing.T) {
	schemes := []*pqos.CLOSScheme{
		{
			CLOSNum:     0,
			WayBit:      0x7FF,
			MemThrottle: 100,
			Processes:   nil,
		},
		{
			CLOSNum:     1,
			WayBit:      0x3,
			MemThrottle: 100,
			Processes:   nil,
		},
		{
			CLOSNum:     2,
			WayBit:      0x3FF,
			MemThrottle: 100,
			Processes:   nil,
		},
		{
			CLOSNum:     3,
			WayBit:      0xFF,
			MemThrottle: 100,
			Processes:   nil,
		},
		{
			CLOSNum:     4,
			WayBit:      0xFF,
			MemThrottle: 100,
			Processes:   nil,
		},
		{
			CLOSNum:     5,
			WayBit:      0xFF,
			MemThrottle: 100,
			Processes:   nil,
		},
		{
			CLOSNum:     6,
			WayBit:      0xFF,
			MemThrottle: 100,
			Processes:   nil,
		},
		{
			CLOSNum:     7,
			WayBit:      0xFF,
			MemThrottle: 100,
			Processes:   nil,
		},
	}
	schemeMap := []int{2, 3, 3, 2, 3, 3}
	m := make(map[string]struct{})
	sm := (*schemeVisited)(&m)
	sm.add(schemes, schemeMap)
	// 首先测试是否真的会产生新的方案
	for try := 0; try < 5000; try++ {
		newSchemes, newMap := randomNeighbor(schemes, schemeMap, 11, 8, sm)
		assert.Equal(t, len(schemes), len(newSchemes))
		assert.Equal(t, len(schemeMap), len(newMap))
		diff := 0
		for i := 0; i < len(schemes); i++ {
			assert.Equal(t, schemes[i].CLOSNum, newSchemes[i].CLOSNum)
			assert.Equal(t, schemes[i].MemThrottle, newSchemes[i].MemThrottle)
			assert.NotZero(t, schemes[i].WayBit)
			if schemes[i].WayBit != newSchemes[i].WayBit {
				diff++
			}
		}
		for i := 0; i < len(schemeMap); i++ {
			assert.NotEqual(t, 0, schemeMap[i])
			assert.NotEqual(t, 1, schemeMap[i])
			if schemeMap[i] != newMap[i] {
				diff++
			}
		}
		assert.Equal(t, 1, diff)
	}

	// 测试新的方案是否与原本的方案本质上不同，即确实会分配到cacheway不一样的
	schemes = []*pqos.CLOSScheme{
		{
			CLOSNum: 0,
			WayBit:  0x7FF,
		},
		{
			CLOSNum: 1,
			WayBit:  0x7FF,
		},
		{
			CLOSNum: 2,
			WayBit:  0x7FF,
		},
		{
			CLOSNum: 3,
			WayBit:  0x7FF,
		},
	}
	schemeMap = []int{0, 0, 0, 0}
	sm.add(schemes, schemeMap)
	for i := 0; i < 1000; i++ {
		newSchemes, newMap := randomNeighbor(schemes, schemeMap, 11, 4, sm)
		assert.Equal(t, newMap, schemeMap)
		diff := 0
		for _, scheme := range newSchemes {
			if scheme.WayBit != 0x7FF {
				diff++
			}
		}
		assert.Equal(t, 1, diff)
	}
}

func TestReadFromOldScheme(t *testing.T) {
	programs := []*ProgramMetric{
		{
			Pid: 1,
		},
		{
			Pid: 2,
		},
		{
			Pid: 3,
		},
		{
			Pid: 4,
		},
		{
			Pid: 5,
		},
	}

	schemes := []*pqos.CLOSScheme{
		{
			CLOSNum:     0,
			WayBit:      0x7FF,
			MemThrottle: 100,
			Processes:   []int{},
		},
		{
			CLOSNum:     2,
			WayBit:      0xFF,
			MemThrottle: 100,
			Processes:   []int{1, 2, 3},
		},
		{
			CLOSNum:     3,
			WayBit:      0x700,
			MemThrottle: 100,
			Processes:   []int{4, 5},
		},
	}

	readSchemes, schemeMap := readFromOldSchemes(programs, schemes, 11, 4)
	assert.Equal(t, 2, schemeMap[0])
	assert.Equal(t, 2, schemeMap[1])
	assert.Equal(t, 2, schemeMap[2])
	assert.Equal(t, 3, schemeMap[3])
	assert.Equal(t, 3, schemeMap[4])

	assert.Equal(t, 4, len(readSchemes))
	for i := 0; i < len(readSchemes); i++ {
		assert.Equal(t, i, readSchemes[i].CLOSNum)
	}
}

func TestSchemeVisitedKey(t *testing.T) {
	schemes := []*pqos.CLOSScheme{
		{
			CLOSNum: 0,
			WayBit:  0x00000001,
		},
		{
			CLOSNum: 1,
			WayBit:  0x00000010,
		},
		{
			CLOSNum: 2,
			WayBit:  0x00000100,
		},
		{
			CLOSNum: 3,
			WayBit:  0x00001000,
		},
		{
			CLOSNum: 4,
			WayBit:  0x00010000,
		},
		{
			CLOSNum: 5,
			WayBit:  0x00100000,
		},
		{
			CLOSNum: 6,
			WayBit:  0x01000000,
		},
		{
			CLOSNum: 7,
			WayBit:  0x10000000,
		},
	}
	schemeMap := []int{0, 1, 2, 3, 4, 5, 6, 7}
	m := map[string]struct{}{}
	sm := (*schemeVisited)(&m)
	key := []byte(sm.key(schemes, schemeMap))
	assert.Equal(t, byte(0x01), key[0])
	assert.Equal(t, byte(0x10), key[4])
	assert.Equal(t, byte(0x01), key[9])
	assert.Equal(t, byte(0x10), key[13])
	assert.Equal(t, byte(0x01), key[18])
	assert.Equal(t, byte(0x10), key[22])
	assert.Equal(t, byte(0x01), key[27])
	assert.Equal(t, byte(0x10), key[31])
	assert.Equal(t, byte(0), key[32])
	assert.Equal(t, byte(1), key[33])
	assert.Equal(t, byte(2), key[34])
	assert.Equal(t, byte(3), key[35])
	assert.Equal(t, byte(4), key[36])
	assert.Equal(t, byte(5), key[37])
	assert.Equal(t, byte(6), key[38])
	assert.Equal(t, byte(7), key[39])
}

func loadTestData() []*ProgramMetric {
	_, testFilePath, _, _ := runtime.Caller(0)
	dir := filepath.Dir(testFilePath) + "/data/"
	fmt.Println(dir)
	perfStatFile, _ := os.Open(dir + "perfstat.csv")
	perfStatRecords, _ := csv.NewReader(perfStatFile).ReadAll()
	p := make([]*ProgramMetric, len(perfStatRecords))
	for i, record := range perfStatRecords {
		pid, _ := strconv.ParseInt(record[1], 10, 32)
		p[i] = &ProgramMetric{
			Pid: int(pid),
			PerfStat: &perf.StatResult{
				Pid: int(pid),
			},
		}
		p[i].PerfStat.Instructions, _ = strconv.ParseUint(record[2], 10, 64)
		p[i].PerfStat.Cycles, _ = strconv.ParseUint(record[3], 10, 64)
		p[i].PerfStat.AllStores, _ = strconv.ParseUint(record[4], 10, 64)
		p[i].PerfStat.AllLoads, _ = strconv.ParseUint(record[5], 10, 64)
		p[i].PerfStat.LLCMiss, _ = strconv.ParseUint(record[6], 10, 64)
		p[i].PerfStat.LLCHit, _ = strconv.ParseUint(record[7], 10, 64)
		p[i].PerfStat.MemAnyCycles, _ = strconv.ParseUint(record[8], 10, 64)
		p[i].PerfStat.LLCMissCycles, _ = strconv.ParseUint(record[9], 10, 64)
		mrcCsvFile, _ := os.Open(dir + record[0] + ".csv")
		all, _ := csv.NewReader(mrcCsvFile).ReadAll()
		p[i].MRC = make([]float32, len(all))
		for _, mrcRecord := range all {
			c, _ := strconv.ParseInt(mrcRecord[0], 10, 32)
			f, _ := strconv.ParseFloat(mrcRecord[1], 32)
			p[i].MRC[c] = float32(f)
		}
		_ = mrcCsvFile.Close()
	}
	_ = perfStatFile.Close()
	return p
}

func TestDoPredict(t *testing.T) {
	data := loadTestData()
	schemes := []*pqos.CLOSScheme{
		{
			CLOSNum: 0,
			WayBit:  0x7FF,
		},
		{
			CLOSNum: 1,
			WayBit:  0x7FF,
		},
		{
			CLOSNum: 2,
			WayBit:  0x7FF,
		},
		{
			CLOSNum: 3,
			WayBit:  0x7FF,
		},
		{
			CLOSNum: 4,
			WayBit:  0x7FF,
		},
		{
			CLOSNum: 5,
			WayBit:  0x7FF,
		},
	}
	schemeMap := []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	ipc, missRate := doPredict(data, schemes, schemeMap, 11, 20480)
	for _, f := range ipc {
		assert.NotZero(t, f)
	}
	for _, f := range missRate {
		assert.NotZero(t, f)
	}

	schemeMap = []int{2, 2, 4, 2, 5, 5, 5, 5, 3, 3}
	schemes = []*pqos.CLOSScheme{
		{
			CLOSNum: 0,
			WayBit:  0x7FF,
		},
		{
			CLOSNum: 1,
			WayBit:  0x7FF,
		},
		{
			CLOSNum: 2,
			WayBit:  0xF0,
		},
		{
			CLOSNum: 3,
			WayBit:  0xF,
		},
		{
			CLOSNum: 4,
			WayBit:  0xF,
		},
		{
			CLOSNum: 5,
			WayBit:  0x700,
		},
	}
	newIpc, newMissRate := doPredict(data, schemes, schemeMap, 11, 20480)
	for i := 0; i < len(newIpc); i++ {
		assert.NotEqual(t, ipc[i], newIpc[i])
		assert.NotEqual(t, missRate[i], newMissRate[i])
	}
}
