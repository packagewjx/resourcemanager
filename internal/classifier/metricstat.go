package classifier

import "math"

type metricStat struct {
	data []float64 // 存float32节省空间，精度应该足够使用了
	sum  float64
	avg  float64
	std  float64
}

type dataLevel int

var (
	dataLevelVeryLow  dataLevel = -2
	dataLevelLow      dataLevel = -1
	dataLevelNormal   dataLevel = 0
	dataLevelHigh     dataLevel = 1
	dataLevelVeryHigh dataLevel = 2
)

func (m *metricStat) addData(data float64) {
	m.data = append(m.data, data)
	m.sum += data
	m.avg = m.sum / float64(len(m.data))
	diffSum := float64(0)
	for _, datum := range m.data {
		diff := datum - m.avg
		diffSum += diff * diff
	}
	m.std = math.Sqrt(diffSum / float64(len(m.data)))
}

func (m *metricStat) dataLevel(data float64) dataLevel {
	diff := data - m.avg
	level := diff / m.std
	if level < -3 {
		return dataLevelVeryLow
	} else if level < -1.5 {
		return dataLevelLow
	} else if level < 1.5 {
		return dataLevelNormal
	} else if level < 3 {
		return dataLevelHigh
	} else {
		return dataLevelVeryHigh
	}
}
