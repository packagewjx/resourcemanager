package perf

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/pkg/errors"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	perfInstructions = "instructions:u"
	perfCycles       = "cycles:u"
	perfAllLoads     = "L1-dcache-loads:u"
	perfAllStores    = "L1-dcache-stores:u"
	perfLoadMisses   = "LLC-load-misses:u"
	perfStoreMisses  = "LLC-store-misses:u"
	perfStatEvents   = perfAllLoads + "," + perfAllStores + "," + perfLoadMisses + "," + perfStoreMisses + "," + perfInstructions + "," + perfCycles
)

type StatFinishFunc func(group *core.ProcessGroup, record *PerfStatResult)

type PerfStatResult struct {
	Group          *core.ProcessGroup
	Error          error
	AllLoads       uint64
	AllStores      uint64
	LLCLoadMisses  uint64
	LLCStoreMisses uint64
	Instructions   uint64
	Cycles         uint64
}

type PerfStatRunner interface {
	Start(ctx context.Context) <-chan *PerfStatResult
}

func NewPerfStatRunner(group *core.ProcessGroup, sampleTime time.Duration) PerfStatRunner {
	return &perfStatRunner{
		group:      group,
		logger:     log.New(os.Stdout, "PerfStat-"+group.Id+": ", log.Lshortfile|log.Lmsgprefix|log.LstdFlags),
		statOut:    make([]byte, 0, 1024),
		wg:         sync.WaitGroup{},
		sampleTime: sampleTime,
	}
}

type perfStatRunner struct {
	group      *core.ProcessGroup
	logger     *log.Logger
	statOut    []byte
	wg         sync.WaitGroup // 用于等待perf结束
	sampleTime time.Duration
}

func (p *perfStatRunner) perfRunner(cmd *exec.Cmd) {
	defer p.wg.Done()
	err := cmd.Run()
	if err != nil {
		p.logger.Printf("Perf Stat进程 %d 退出错误：%v", cmd.Process.Pid, err)
	} else {
		p.logger.Printf("Perf Stat进程 %d 正常退出", cmd.Process.Pid)
	}
}

func (p *perfStatRunner) parseResult(out io.Reader) *PerfStatResult {
	res := &PerfStatResult{}
	statRecords, _ := csv.NewReader(out).ReadAll()
	for _, record := range statRecords {
		cnt, err := strconv.ParseUint(record[0], 10, 64)
		if err != nil {
			p.logger.Printf("解析perf stat计数值异常，异常行：%v", record)
			continue
		}
		percent, err := strconv.ParseFloat(record[4], 32)
		if err != nil {
			p.logger.Printf("解析perf stat百分比异常，异常行：%v", record)
		}
		cnt = uint64((float64(cnt) / (percent / 100.0)))

		switch record[2] {
		default:
			p.logger.Printf("出现了未知事件，异常行：%v", record)
		case perfInstructions:
			res.Instructions = cnt
		case perfCycles:
			res.Cycles = cnt
		case "L1-dcache-loads":
			res.AllLoads = cnt
		case "L1-dcache-stores":
			res.AllStores = cnt
		case "LLC-load-misses":
			res.LLCLoadMisses = cnt
		case "LLC-store-misses":
			res.LLCStoreMisses = cnt
		}
	}
	// 修正数据

	return res
}

func (p *perfStatRunner) Start(ctx context.Context) <-chan *PerfStatResult {
	pidString := make([]string, len(p.group.Pid))
	for i, pid := range p.group.Pid {
		pidString[i] = strconv.FormatInt(int64(pid), 10)
	}
	pidListString := strings.Join(pidString, ",")
	statCmd := exec.Command("perf", "stat", "-e", perfStatEvents, "-ip", pidListString, "-x", ",")
	buffer := bytes.NewBuffer(make([]byte, 0, 1024))
	statCmd.Stderr = buffer
	resultCh := make(chan *PerfStatResult, 1)

	p.wg.Add(1)
	go p.perfRunner(statCmd)
	go func() {
		select {
		case <-time.After(p.sampleTime):
		case <-ctx.Done():
		}
		_ = syscall.Kill(statCmd.Process.Pid, syscall.SIGINT)
		p.wg.Wait()
		result := p.parseResult(buffer)
		result.Group = p.group
		result.Error = nil
		resultCh <- result
		close(resultCh)
	}()
	return resultCh
}

// 保留本函数测试用
func ReadPerfMemTrace(path string) ([]uint64, error) {
	reportCmd := exec.Command("perf", "mem", "report", "-D", "-x", ",", "-i", path)
	stdout, err := reportCmd.StdoutPipe()
	if err != nil {
		return nil, errors.Wrap(err, "获取stdout失败")
	}
	err = reportCmd.Start()
	if err != nil {
		_ = stdout.Close()
		return nil, errors.Wrap(err, "启动perf report失败")
	}
	defer func() {
		_ = stdout.Close()
		_ = reportCmd.Wait()
	}()

	res := make([]uint64, 0)
	reader := csv.NewReader(stdout)
	// 去掉第一行
	_, _ = reader.Read()
	var record []string
	for record, err = reader.Read(); err == nil || (err == io.EOF && record != nil); record, err = reader.Read() {
		addrString := record[3]
		addr, err2 := strconv.ParseUint(addrString, 0, 64)
		if err2 != nil {
			fmt.Printf("从perf report读取数据%s出错\n", addrString)
			continue
		}
		addr &= 0xFFFFFFFFFFFFFFC0
		res = append(res, addr)
	}
	if err != nil && err != io.EOF {
		return nil, errors.Wrap(err, "读取stdout失败")
	}

	return res, nil
}
