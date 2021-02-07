package rthsample

import (
	"bufio"
	"fmt"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestReadRthSampleCsv(t *testing.T) {
	//file := test.GetTestDataDir() + "/sample-perlbench.csv"
	file := "/home/wjx/Documents/基于Kubernetes的在离线混合部署/实验数据/pin采样rth-200亿/sample-xz.csv"
	res := ReadRthSampleCsv(file, 100000)
	assert.NotZero(t, len(res))
	assert.NotZero(t, res[1])

	f, _ := os.Create("rth.csv")
	writer := bufio.NewWriter(f)
	for i, re := range res {
		_, _ = writer.WriteString(fmt.Sprintf("%d,%d\n", i, re))
	}
	_ = writer.Flush()
	_ = f.Close()
}
