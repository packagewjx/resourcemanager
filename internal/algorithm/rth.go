package algorithm

import (
	"bufio"
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"strings"
)

type RTHCalculator interface {
	Update(traces []uint64)
	GetRTH(maxTime int) []int
}

func ReservoirCalculator(reservoirSize int) RTHCalculator {
	return &reservoirCalculator{
		time:      0,
		size:      reservoirSize,
		reservoir: map[uint64]*reservoirEntry{},
		addrSet:   map[uint64]struct{}{},
	}
}

type reservoirSampleState string

var (
	reservoirStateTagged   reservoirSampleState = "tagged"
	reservoirStateUnTagged reservoirSampleState = "untagged"
)

type reservoirEntry struct {
	state     reservoirSampleState
	firstTime uint64
	lastTime  uint64
}

type reservoirCalculator struct {
	time      uint64
	size      int
	reservoir map[uint64]*reservoirEntry
	addrSet   map[uint64]struct{}
}

func (r *reservoirCalculator) Update(traces []uint64) {
	for i := 0; i < len(traces); i++ {
		entry := r.reservoir[traces[i]]
		if entry == nil {
			r.addrSet[traces[i]] = struct{}{}
			if r.size == len(r.reservoir) {
				if rand.Float32() > (float32(r.size) / float32(len(r.addrSet))) {
					continue
				}
				// 随机丢弃记录
				for k := range r.reservoir {
					delete(r.reservoir, k)
					break
				}
			}
			// 加入新的
			entry = &reservoirEntry{
				state:     reservoirStateUnTagged,
				firstTime: r.time,
				lastTime:  r.time,
			}
			r.reservoir[traces[i]] = entry
		} else if entry.state == reservoirStateUnTagged {
			entry.state = reservoirStateTagged
			entry.lastTime = r.time
		} else {
			// entry not nil and tagged: nop
		}
		r.time++
	}
}

func (r *reservoirCalculator) GetRTH(maxTime int) []int {
	res := make([]int, maxTime+2)
	for _, entry := range r.reservoir {
		reuseTime := int(entry.lastTime - entry.firstTime)
		if reuseTime > maxTime {
			res[maxTime+1]++
		} else {
			res[reuseTime]++
		}
	}
	return res
}

func FullTraceCalculator() RTHCalculator {
	return &fullTraceCalculator{
		sample: map[uint64][]uint64{},
		time:   0,
	}
}

type fullTraceCalculator struct {
	sample map[uint64][]uint64
	time   uint64
}

func (f *fullTraceCalculator) Update(traces []uint64) {
	for i := 0; i < len(traces); i++ {
		if arr, ok := f.sample[traces[i]]; ok {
			if arr[1] == arr[0] {
				arr[1] = f.time
			}
		} else {
			f.sample[traces[i]] = []uint64{f.time, f.time}
		}
		f.time++
	}
}

func (f *fullTraceCalculator) GetRTH(maxTime int) []int {
	res := make([]int, maxTime+2)
	for _, arr := range f.sample {
		reuseTime := int(arr[1] - arr[0])
		if reuseTime > maxTime {
			res[maxTime+1]++
		} else {
			res[reuseTime]++
		}
	}
	return res
}

func WriteAsCsv(rth []int, writer io.Writer) {
	bufWriter := bufio.NewWriter(writer)
	for t, c := range rth {
		_, _ = bufWriter.WriteString(fmt.Sprintf("%d,%d\n", t, c))
	}
	_ = bufWriter.Flush()
}

func LoadRthFromCsv(f io.Reader) []int {
	reader := bufio.NewReader(f)
	var line string
	var err error
	m := map[int]int{}
	maxRt := int64(0)
	for line, err = reader.ReadString('\n'); err == nil; line, err = reader.ReadString('\n') {
		split := strings.Split(line[:len(line)-1], ",")
		rt, _ := strconv.ParseInt(split[0], 0, 32)
		cnt, _ := strconv.ParseInt(split[1], 0, 32)
		m[int(rt)] = int(cnt)
		if rt > maxRt {
			maxRt = rt
		}
	}
	rth := make([]int, maxRt+1)
	for r, c := range m {
		rth[r] = c
	}
	return rth
}
