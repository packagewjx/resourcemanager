package aet

import (
	"encoding/csv"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"strconv"
)

type AETModel interface {
	// 论文中的P计算
	// 若t大与样本的最大再使用时间，结果将会不再准确。最大再使用时间由输入决定。
	ProbabilityReuseTimeGreaterThan(t int) float32

	// 计算出缓存行数量为cacheSize时候的Average Eviction Time。
	// 返回值单位为访问次数。
	// 如果cacheSize很大，将会达到最大样本的reuseTime值，再继续增大结果将不变
	AET(cacheSize int) int

	// 计算出缓存行数量为cacheSize时候的Miss Rate
	MR(cacheSize int) float32
}

type aetImpl struct {
	rthPrefixSum []int
	numBeyondMax int
	numColdMiss  int
}

var _ AETModel = &aetImpl{}

func NewAETModel(file io.Reader) (AETModel, error) {
	rth, err := readRTHCsv(file)
	if err != nil {
		return nil, errors.Wrap(err, "读取RTH数据出错")
	}

	numColdMiss := rth[0]
	numBeyondMax := rth[len(rth)-1]
	rth[0] = 0
	rth = rth[:len(rth)-1]
	for i := 1; i < len(rth); i++ {
		rth[i] += rth[i-1]
	}

	return &aetImpl{
		rthPrefixSum: rth,
		numColdMiss:  numColdMiss,
		numBeyondMax: numBeyondMax,
	}, nil
}

func readRTHCsv(file io.Reader) ([]int, error) {
	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("读取文件%s出错", file))
	}

	result := make([]int, len(records))
	for _, record := range records {
		rt, _ := strconv.ParseInt(record[0], 10, 32)
		oc, _ := strconv.ParseInt(record[1], 10, 32)
		result[rt] = int(oc)
	}

	return result, nil
}

func (a *aetImpl) ProbabilityReuseTimeGreaterThan(t int) float32 {
	if t >= len(a.rthPrefixSum) {
		return (float32(a.numBeyondMax+a.numColdMiss) /
			float32(a.numColdMiss+a.numBeyondMax+a.rthPrefixSum[len(a.rthPrefixSum)-1]))
	} else {
		return (float32(a.numBeyondMax + a.numColdMiss + a.rthPrefixSum[len(a.rthPrefixSum)-1] - a.rthPrefixSum[t])) /
			float32(a.numColdMiss+a.numBeyondMax+a.rthPrefixSum[len(a.rthPrefixSum)-1])
	}
}

func (a *aetImpl) AET(cacheSize int) int {
	curr := float32(0)
	var res int
	for res = 0; res < len(a.rthPrefixSum) && curr < float32(cacheSize); res++ {
		curr += a.ProbabilityReuseTimeGreaterThan(res)
	}
	return res - 1
}

func (a *aetImpl) MR(cacheSize int) float32 {
	return a.ProbabilityReuseTimeGreaterThan(a.AET(cacheSize))
}
