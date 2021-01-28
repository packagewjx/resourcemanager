package algorithm

import (
	"context"
	"go.uber.org/atomic"
	"log"
	"math"
	"math/big"
	"sync"
	"time"
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
	for _, a := range list {
		addr := a & cacheLineMask
		tl, ok := m.lastAccess[addr]
		if ok {
			reuseTime := m.time - tl
			if reuseTime > m.maxTime {
				m.rth[m.maxTime+1]++
			} else {
				m.rth[reuseTime]++
			}
		}
		m.lastAccess[addr] = m.time
		m.time++
	}
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

	// 进度汇报
	cnt := atomic.NewInt32(0)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		tick := time.Tick(time.Second)
		for {
			select {
			case <-tick:
				log.Printf("prk计算进度：%10d/%10d\n", cnt.Load(), N)
			case <-ctx.Done():
				return
			}
		}
	}()

	wg := sync.WaitGroup{}
	result := make([]float64, N+1)
	for d := 1; d <= N; d++ {
		wg.Add(1)
		go func(d int) {
			result[d] = m.prk(d, N, pt, p3, c)
			cnt.Inc()
			wg.Done()
		}(d)
	}
	wg.Wait()
	cancel()
	return result
}

func (m *ShenModel) prk(k, N int, pt, p3 []float64, c *combination) float64 {
	res := float64(0)
	for delta := 1; delta <= m.maxTime; delta++ {
		if pt[delta] == 0 {
			continue
		}
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

	// 报告进度使用
	cnt := atomic.NewInt32(0)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		tick := time.Tick(500 * time.Millisecond)
		for {
			select {
			case <-tick:
				log.Printf("组合数计算已完成：%10d\n", cnt.Load())
			case <-ctx.Done():
				return
			}
		}
	}()

	// 使用组合数性质加法，减少浮点数阶乘乘法
	calFunc := func(start, end int) {
		for i := start; i < end; i++ {
			c[i] = big.NewFloat(0)
			c[i].Add(last[i], last[i-1])
			cnt.Inc()
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
	cancel()
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
