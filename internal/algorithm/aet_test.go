package algorithm

import (
	"github.com/packagewjx/resourcemanager/internal/utils"
	"github.com/stretchr/testify/assert"
	"os"
	"strings"
	"testing"
)

const case1 = `1,50
2,49
3,48
4,47
5,46
6,45
7,44
8,43
9,42
10,41
11,40
12,39
13,38
14,37
15,36
16,35
17,34
18,33
19,32
20,31
21,30
22,29
23,28
24,27
25,26
26,25
27,24
28,23
29,22
30,21
31,20
32,19
33,18
34,17
35,16
36,15
37,14
38,13
39,12
40,11
41,10
0,5
`

func TestAetImpl_ProbabilityReuseTimeGreaterThan(t *testing.T) {
	read := strings.NewReader(case1)
	model, err := NewAETModelFromFile(read)
	impl := model.(*aetImpl)
	assert.NoError(t, err)
	assert.Equal(t, 5, impl.numColdMiss)
	assert.Equal(t, 10, impl.numBeyondMax)
	assert.Equal(t, 1220, impl.rthPrefixSum[len(impl.rthPrefixSum)-1])
	assert.InDelta(t, 1, model.ProbabilityReuseTimeGreaterThan(0), 0.000001)
	assert.InDelta(t, 0.959514, model.ProbabilityReuseTimeGreaterThan(1), 0.000001)
	assert.InDelta(t, 0.631578, model.ProbabilityReuseTimeGreaterThan(10), 0.000001)
	assert.InDelta(t, 0.012145, model.ProbabilityReuseTimeGreaterThan(50), 0.000001)
	assert.InDelta(t, 0.012145, model.ProbabilityReuseTimeGreaterThan(100), 0.000001)
}

func TestAetImpl_AET(t *testing.T) {
	reader := strings.NewReader(case1)
	model, err := NewAETModelFromFile(reader)
	assert.NoError(t, err)

	P := make([]float32, 41)
	for i := 1; i <= 40; i++ {
		P[i] = model.ProbabilityReuseTimeGreaterThan(i)
	}

	cur := float32(1)
	for i := 1; i <= 40; i++ {
		next := cur + P[i]
		if int(next) > int(cur) {
			if assert.Equal(t, i, model.AET(int(next))) {
				t.Logf("AET(%d) = %d", int(next), i)
			}
		}
		cur = next
	}
	assert.Equal(t, 40, model.AET(17))
	assert.Equal(t, 40, model.AET(20))
	assert.Equal(t, 40, model.AET(100))
}

func TestAetImpl_MR(t *testing.T) {
	reader := strings.NewReader(case1)
	model, err := NewAETModelFromFile(reader)
	assert.NoError(t, err)
	P := make([]float32, 41)
	for i := 1; i <= 40; i++ {
		P[i] = model.ProbabilityReuseTimeGreaterThan(i)
	}
	for i := 2; i <= 16; i++ {
		assert.Equal(t, model.MR(i), model.ProbabilityReuseTimeGreaterThan(model.AET(i)))
	}
}

func TestAetImpl_MRC(t *testing.T) {
	reader := strings.NewReader(case1)
	model, err := NewAETModelFromFile(reader)
	assert.NoError(t, err)
	mrc := model.MRC(20)
	P := make([]float32, 41)
	for i := 1; i <= 40; i++ {
		P[i] = model.ProbabilityReuseTimeGreaterThan(i)
	}
	for i := 2; i <= 16; i++ {
		assert.Equal(t, model.ProbabilityReuseTimeGreaterThan(model.AET(i)), mrc[i])
		assert.Equal(t, model.MR(i), mrc[i])
	}
	for i := 17; i <= 20; i++ {
		assert.Equal(t, mrc[16], mrc[i])
	}
}

func TestBA(t *testing.T) {
	trace, err := utils.ParseCTFTrace("/home/wjx/Workspace/valgrind-tracegen/inst/out")
	assert.NoError(t, err)
	rth := FullTraceCalculator()
	for _, addrList := range trace {
		rth.Update(addrList)
	}
	out, _ := os.Create("mcf.rth.csv")
	WriteAsCsv(rth.GetRTH(100000), out)
	_ = out.Close()
}
