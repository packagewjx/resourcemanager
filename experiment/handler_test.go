package experiment

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestHandleCsv(t *testing.T) {
	f, _ := os.Create("out.csv")
	glob, _ := filepath.Glob("/home/wjx/Documents/基于Kubernetes的在离线混合部署/实验数据/clos-60秒-3套/clos/*")
	for _, fileName := range glob {
		name := filepath.Base(fileName)
		fin, _ := os.Open(fileName)
		reader := bufio.NewReader(fin)
		var line string
		_, _ = reader.ReadString('\n')
		_, _ = reader.ReadString('\n')
		line, _ = reader.ReadString('\n')
		line = line[:len(line)-1]
		split := strings.Split(line, ",")
		instructions, _ := strconv.ParseUint(split[0], 0, 64)
		time, _ := strconv.ParseUint(split[3], 0, 64)
		line, _ = reader.ReadString('\n')
		split = strings.Split(line, ",")
		cycles, _ := strconv.ParseUint(split[0], 0, 64)
		line, _ = reader.ReadString('\n')
		split = strings.Split(line, ",")
		cacheReferences, _ := strconv.ParseUint(split[0], 0, 64)
		line, _ = reader.ReadString('\n')
		split = strings.Split(line, ",")
		cacheMisses, _ := strconv.ParseUint(split[0], 0, 64)
		ipc := float64(instructions) / float64(cycles)
		missRate := float64(cacheMisses) / float64(cacheReferences)
		throughput := float64(instructions) / float64(time)
		_, _ = fmt.Fprintf(f, "%s,%.4f,%.4f,%.4f\n", name, ipc, missRate, throughput)
		_ = fin.Close()
	}
	_ = f.Close()
}
