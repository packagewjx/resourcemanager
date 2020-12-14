package algorithm

import (
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
)

func TestRTHCalculator(t *testing.T) {
	doTest := func(builder func() RTHCalculator) {
		t.Helper()
		rc := builder()
		testCase := []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
		rc.Update(testCase)
		rth := rc.GetRTH(10)
		assert.Equal(t, 20, rth[0])
		assert.Equal(t, 12, len(rth))

		rc = builder()
		testCase = []uint64{1, 1, 2, 20, 2, 3, 7, 8, 3, 4, 11, 12, 13, 4, 5, 16, 17, 18, 19, 5}
		rc.Update(testCase)
		rth = rc.GetRTH(10)
		assert.Equal(t, 10, rth[0])
		assert.Equal(t, 1, rth[1])
		assert.Equal(t, 1, rth[2])
		assert.Equal(t, 1, rth[3])
		assert.Equal(t, 1, rth[4])
		assert.Equal(t, 1, rth[5])

		rc = builder()
		testCase = make([]uint64, 1000)
		for i := 0; i < len(testCase); i++ {
			testCase[i] = uint64(rand.Intn(50))
		}
		rc.Update(testCase)
		rth = rc.GetRTH(50)
		nonZero := 0
		for i := 0; i < len(rth); i++ {
			if rth[i] != 0 {
				nonZero++
			}
		}
		assert.NotEqual(t, 0, nonZero)
	}

	doTest(FullTraceCalculator)
	doTest(func() RTHCalculator {
		return ReservoirCalculator(100)
	})

}
