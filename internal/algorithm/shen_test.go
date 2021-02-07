package algorithm

import (
	"github.com/packagewjx/resourcemanager/internal/utils"
	"github.com/packagewjx/resourcemanager/test"
	"github.com/stretchr/testify/assert"
	"math"
	"math/big"
	"math/rand"
	"os"
	"testing"
)

func TestCombination(t *testing.T) {
	c := newCombination(10)
	expect := []float64{1, 10, 45, 120, 210, 252, 210, 120, 45, 10, 1}
	for i := 0; i <= 10; i++ {
		assert.Condition(t, func() (success bool) {
			r := big.NewFloat(expect[i])
			return r.Cmp(c.k(i)) == 0
		})
	}

	c = newCombination(4000)
	f, _ := c.k(10).Float64()
	assert.InDelta(t, f, 2.8572431154929766e+29, 10000)
}

func TestShenModelWithRandomAddress(t *testing.T) {
	addr := make([]uint64, 5000)
	const addrMask = 0xFFFFC
	r := rand.New(rand.NewSource(1))
	for i := 0; i < len(addr); i++ {
		addr[i] = r.Uint64() & addrMask
	}
	calculator := ReservoirCalculator(10000)
	calculator.Update(addr)
	rth := calculator.GetRTH(10000)
	model := NewShenModel(rth)
	assert.NotNil(t, model)
	sum := float64(0)
	rdh := model.ReuseDistanceHistogram()
	for i, f := range rdh {
		sum += f
		assert.False(t, math.IsNaN(f), "%d should not be NaN", i)
	}
	assert.NotZero(t, sum)
}

func TestShenModelWithLsData(t *testing.T) {
	file := test.GetTestDataDir() + "/ls.dat"
	reader, err := utils.NewPinBinaryReader(file)
	assert.NoError(t, err)
	all := reader.ReadAll()
	for _, addr := range all {
		for i := 0; i < len(addr); i++ {
			addr[i] &= 0xFFFFFFFFFFFF
		}
		c := ReservoirCalculator(100000)
		c.Update(addr)
		model := NewShenModel(c.GetRTH(100000))
		rdh := model.ReuseDistanceHistogram()
		sum := float64(0)
		for _, f := range rdh {
			sum += f
		}
		assert.Less(t, sum, float64(1))
		assert.NotZero(t, sum)
	}
}

func TestShenModelWithRth(t *testing.T) {
	file, _ := os.Open("/home/wjx/Documents/基于Kubernetes的在离线混合部署/实验数据/pin采样rth-200亿/deepsjeng.rth.csv")
	rth := LoadRthFromCsv(file)
	rdh := NewShenModel(rth).ReuseDistanceHistogram()
	assert.NotZero(t, len(rdh))
}

func BenchmarkCombination(b *testing.B) {
	for i := 0; i < b.N; i++ {
		newCombination(10000)
	}
}

func BenchmarkReuseDistanceHistogram(b *testing.B) {
	addr := make([]uint64, 5000)
	const addrMask = 0xFFFFC
	r := rand.New(rand.NewSource(1))
	for i := 0; i < len(addr); i++ {
		addr[i] = r.Uint64() & addrMask
	}
	c := FullTraceCalculator()
	c.Update(addr)
	model := NewShenModel(c.GetRTH(100000))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		model.ReuseDistanceHistogram()
	}
}
