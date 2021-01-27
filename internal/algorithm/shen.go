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

type shenModel struct {
	numAddr int       // 论文中的N
	pt      []float64 // 复用时间占比
	p3      []float64
	c       *combination
	maxTime int
}

func newShenModel(addrList []uint64, maxTime int) *shenModel {
	lastAccess := make(map[uint64]int)
	rth := make([]int, maxTime+2)
	for t, a := range addrList {
		addr := a & cacheLineMask
		tl, ok := lastAccess[addr]
		if ok {
			reuseTime := t - tl
			if t-tl > maxTime {
				rth[maxTime+1]++
			} else {
				rth[reuseTime]++
			}
		}
		lastAccess[addr] = t
	}

	rthSum := 0
	for _, i := range rth {
		rthSum += i
	}
	pt := make([]float64, len(rth))
	for i, v := range rth {
		pt[i] = float64(v) / float64(rthSum)
	}

	ptPostFixSum := make([]float64, len(pt))
	ptPostFixSum[len(ptPostFixSum)-1] = pt[len(pt)-1]
	for i := len(ptPostFixSum) - 2; i > 0; i-- {
		ptPostFixSum[i] = pt[i] + ptPostFixSum[i+1]
	}

	N := len(lastAccess)
	p3 := make([]float64, maxTime+1)
	p3[1] = 1 / float64(N-1) * ptPostFixSum[2]
	for t := 2; t <= maxTime; t++ {
		p3[t] = p3[t-1] + 1/float64(N-1)*ptPostFixSum[t+1]
	}

	return &shenModel{
		numAddr: N,
		pt:      pt,
		p3:      p3,
		c:       newCombination(N),
		maxTime: maxTime,
	}
}

func (m *shenModel) reuseDistanceHistogram() []float64 {
	wg := sync.WaitGroup{}
	result := make([]float64, m.numAddr+1)
	for d := 1; d <= m.numAddr; d++ {
		wg.Add(1)
		go func(d int) {
			result[d] = m.prk(d)
			wg.Done()
		}(d)
	}
	wg.Wait()
	return result
}

func (m *shenModel) prk(k int) float64 {
	res := float64(0)
	for delta := 1; delta <= m.maxTime; delta++ {
		res += m.pkdelta(k, delta) * m.pt[delta]
	}
	return res
}

func (m *shenModel) pkdelta(k int, delta int) float64 {
	p1 := big.NewFloat(math.Pow(m.p3[delta], float64(k)))
	mck := m.c.k(k)
	p2 := big.NewFloat(math.Pow(1-m.p3[delta], float64(m.numAddr-k)))
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
