package algorithm

import (
	"math"
	"math/big"
	"sync"
)

/*
Locality Approximation Using Time
Xipeng Shen, Jonathan Shaw, et. al.
*/

const cacheLineMask = 0xFFFFFFFFFFFFFFC0

type ShenModel struct {
	lastAccess map[uint64]int
	maxTime    int
	time       int
	rth        []int
}

func NewShenModel(maxTime int) *ShenModel {
	return &ShenModel{
		lastAccess: make(map[uint64]int),
		maxTime:    maxTime,
		time:       0,
		rth:        make([]int, maxTime+2),
	}
}

func (m *ShenModel) AddAddresses(list []uint64) {
	for t, a := range list {
		addr := a & cacheLineMask
		tl, ok := m.lastAccess[addr]
		if ok {
			reuseTime := t - tl
			if t-tl > m.maxTime {
				m.rth[m.maxTime+1]++
			} else {
				m.rth[reuseTime]++
			}
		}
		m.lastAccess[addr] = t + m.time
	}
	m.time += len(list)
}

//ReuseDistanceHistogram 根据当前的所有地址，计算出现在的Reuse Time Histogram
func (m *ShenModel) ReuseDistanceHistogram() []float64 {
	rthSum := 0
	for _, i := range m.rth {
		rthSum += i
	}
	pt := make([]float64, len(m.rth))
	for i, v := range m.rth {
		pt[i] = float64(v) / float64(rthSum)
	}

	ptPostFixSum := make([]float64, len(pt))
	ptPostFixSum[len(ptPostFixSum)-1] = pt[len(pt)-1]
	for i := len(ptPostFixSum) - 2; i > 0; i-- {
		ptPostFixSum[i] = pt[i] + ptPostFixSum[i+1]
	}

	N := len(m.lastAccess)
	p3 := make([]float64, m.maxTime+1)
	p3[1] = 1 / float64(N-1) * ptPostFixSum[2]
	for t := 2; t <= m.maxTime; t++ {
		p3[t] = p3[t-1] + 1/float64(N-1)*ptPostFixSum[t+1]
	}
	c := newCombination(N)

	wg := sync.WaitGroup{}
	result := make([]float64, N+1)
	for d := 1; d <= N; d++ {
		wg.Add(1)
		go func(d int) {
			result[d] = m.prk(d, N, pt, p3, c)
			wg.Done()
		}(d)
	}
	wg.Wait()
	return result
}

func (m *ShenModel) prk(k, N int, pt, p3 []float64, c *combination) float64 {
	res := float64(0)
	for delta := 1; delta <= m.maxTime; delta++ {
		res += m.pkdelta(k, delta, N, p3, c) * pt[delta]
	}
	return res
}

func (m *ShenModel) pkdelta(k, delta, N int, p3 []float64, c *combination) float64 {
	p1 := big.NewFloat(math.Pow(p3[delta], float64(k)))
	mck := c.k(k)
	p2 := big.NewFloat(math.Pow(1-p3[delta], float64(N-k)))
	res := big.NewFloat(0)
	res.Mul(mck, p1)
	res.Mul(res, p2)
	f, _ := res.Float64()
	return f
}

type combination []*big.Float

func newCombination(n int) *combination {
	bigFloat1 := big.NewFloat(1)
	bigFloat2 := big.NewFloat(2)
	if n == 1 {
		res := []*big.Float{bigFloat1}
		return (*combination)(&res)
	}
	last := []*big.Float{bigFloat1}
	var c []*big.Float
	// 使用组合数性质加法，减少浮点数阶乘乘法
	calFunc := func(start, end int) {
		for i := start; i < end; i++ {
			c[i] = big.NewFloat(0)
			c[i].Add(last[i], last[i-1])
		}
	}
	for curr := 2; curr <= n; curr++ {
		c = make([]*big.Float, (curr+2)/2)
		c[0] = bigFloat1

		const threshold = 256
		if curr < threshold {
			calFunc(1, len(last))
		} else {
			// 并行计算
			wg := sync.WaitGroup{}
			var i int
			for i = 1; i+threshold <= len(last); i += threshold {
				wg.Add(1)
				go func(start, end int) {
					calFunc(start, end)
					wg.Done()
				}(i, i+threshold)
			}
			wg.Add(1)
			go func() {
				calFunc(i, len(last))
				wg.Done()
			}()
			wg.Wait()
		}

		if curr&1 == 0 {
			c[len(c)-1] = big.NewFloat(0)
			c[len(c)-1].Mul(last[len(last)-1], bigFloat2)
		}
		last = c
	}
	return (*combination)(&last)
}

func (c *combination) k(k int) *big.Float {
	if len(*c) == 1 {
		return big.NewFloat(1)
	}
	n, _ := (*c)[1].Int64()
	if k > int(n) || k < 0 {
		return big.NewFloat(0)
	} else if k >= len(*c) {
		// (*c)[1]就是原本的n
		return (*c)[len(*c)*2-k+int(n&1)-2]
	} else {
		return (*c)[k]
	}
}
