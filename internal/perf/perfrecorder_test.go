package perf

import (
	"context"
	"encoding/csv"
	"github.com/packagewjx/resourcemanager/internal/algorithm"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/utils"
	"github.com/stretchr/testify/assert"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestReadPerf(t *testing.T) {
	go utils.SequenceMemoryReader()
	cmd := exec.Command("perf", "record", "-e", "cpu/mem-loads/P,cpu/mem-stores/P", "-d", "-c", "10",
		"-ip", strconv.FormatInt(int64(os.Getpid()), 10), "-o", "perf.data", "sleep", "2")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		t.Fatal(err)
	}

	res, err := readPerfMemFile("perf.data")
	assert.NoError(t, err)
	assert.NotEqual(t, 0, len(res))
}

func TestPerfMemRecorder(t *testing.T) {
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		utils.SequenceMemoryReader()
		wg.Done()
	}()
	//recorder, err := NewPerfMemRecorderWithReservoir(&core.ProcessGroup{
	//	Id:  "test",
	//	Pid: []int{os.Getpid()},
	//}, 100000)
	recorder, err := NewPerfMemRecorderWithFullTrace(&core.ProcessGroup{
		Id:  "test",
		Pid: []int{os.Getpid()},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	recorder.Start(ctx)
	wg.Wait()
	cancel()
	result, _ := recorder.FinishSampling(100000)
	f, err := os.Create("test.result.csv")
	if err != nil {
		t.Fatal(err)
	}
	writer := csv.NewWriter(f)
	nonZeroCount := 0
	for i := 0; i < len(result.Rth); i++ {
		if result.Rth[i] != 0 {
			nonZeroCount++
		}
		_ = writer.Write([]string{strconv.FormatInt(int64(i), 10), strconv.FormatInt(int64(result.Rth[i]), 10)})
	}
	writer.Flush()
	_ = f.Close()
	assert.NotEqual(t, 0, nonZeroCount)

	model := algorithm.NewAETModel(result.Rth)
	mrc := model.MRC(10000)
	f, err = os.Create("test.mrc.csv")
	if err != nil {
		t.Fatal(err)
	}
	writer = csv.NewWriter(f)
	for i := 0; i < len(mrc); i++ {
		_ = writer.Write([]string{strconv.FormatInt(int64(i), 10), strconv.FormatFloat(float64(mrc[i]), 'f', 4, 32)})
	}
	writer.Flush()
	_ = f.Close()
}
