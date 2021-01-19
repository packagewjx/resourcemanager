package resourcemanager

import (
	"github.com/packagewjx/resourcemanager/internal/algorithm"
	"github.com/packagewjx/resourcemanager/internal/sampler/pin"
)

// 给所有线程计算的加权平均MRC
func WeightedAverageMRC(m *pin.MemRecordResult, maxRTH, cacheSize int) []float32 {
	model := algorithm.NewAETModel(WeightedAverageRTH(m, maxRTH))
	return model.MRC(cacheSize)
}

func WeightedAverageRTH(m *pin.MemRecordResult, maxRTH int) []int {
	averageRth := make([]int, maxRTH+2)
	for tid, calculator := range m.ThreadTrace {
		rth := calculator.GetRTH(maxRTH)
		weight := float32(m.ThreadInstructionCount[tid]) / float32(m.TotalInstructions)
		for i := 0; i < len(averageRth); i++ {
			averageRth[i] += int(float32(rth[i]) * weight)
		}
	}
	return averageRth
}

func diffIntArray(a, b []int) (add []int, remove []int) {
	am := map[int]struct{}{}
	bm := map[int]struct{}{}
	for _, i := range a {
		am[i] = struct{}{}
	}
	for _, i := range b {
		if _, ok := am[i]; !ok {
			add = append(add, i)
		}
		bm[i] = struct{}{}
	}
	for _, i := range a {
		if _, ok := bm[i]; !ok {
			remove = append(remove, i)
		}
	}
	return
}
