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
	"sync"
	"syscall"
	"time"
)

const (
	pAllLoads     = "mem_inst_retired.all_loads"
	pAllStores    = "mem_inst_retired.all_stores"
	pL3Miss       = "longest_lat_cache.miss"
	pL3LoadMisses = "LLC-load-misses"
	pCycles       = "cpu_clk_unhalted.thread"
	pInstructions = "inst_retired.any"
	pL3MissCycles = "cycle_activity.cycles_l3_miss"
	pMemAnyCycles = "cycle_activity.cycles_mem_any"
)

var perfStatEvents = fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s", pAllLoads, pAllStores, pL3Miss, pL3LoadMisses,
	pCycles, pInstructions, pL3MissCycles, pMemAnyCycles)

type PerfStatResult struct {
	Pid           int
	Error         error
	AllLoads      uint64 // mem_inst_retired.all_loads
	AllStores     uint64 // mem_inst_retired.all_stores
	Instructions  uint64 // inst_retired.any
	Cycles        uint64 // cpu_clk_unhalted.thread
	LLCReferences uint64 // longest_lat_cache.reference
	LLCMisses     uint64 // longest_lat_cache.miss
	MemAnyCycles  uint64 // cycle_activity.cycles_mem_any
	LLCMissCycles uint64 // cycle_activity.cycles_l3_miss
	LLCLoadMisses uint64 // LLC-load-misses
}

func (p *PerfStatResult) Clone() core.Cloneable {
	return &PerfStatResult{
		Pid:           p.Pid,
		Error:         p.Error,
		AllLoads:      p.AllLoads,
		AllStores:     p.AllStores,
		Instructions:  p.Instructions,
		Cycles:        p.Cycles,
		LLCReferences: p.LLCReferences,
		LLCMisses:     p.LLCMisses,
		MemAnyCycles:  p.MemAnyCycles,
		LLCMissCycles: p.LLCMissCycles,
		LLCLoadMisses: p.LLCLoadMisses,
	}
}

func (p *PerfStatResult) LLCMissRate() float64 {
	return float64(p.LLCMisses) / float64(p.LLCReferences)
}

func (p *PerfStatResult) AccessPerInstruction() float64 {
	return float64(p.AllLoads+p.AllStores) / float64(p.Instructions)
}

func (p *PerfStatResult) AverageCacheHitLatency() float64 {
	cycles := float64(p.MemAnyCycles - p.LLCMissCycles)
	hitCount := float64(p.AllLoads - p.LLCMisses)
	return cycles / hitCount
}

func (p *PerfStatResult) AverageCacheMissLatency() float64 {
	return float64(p.LLCMissCycles) / float64(p.LLCLoadMisses)
}

func (p *PerfStatResult) InstructionPerCycle() float64 {
	return float64(p.Instructions) / float64(p.Cycles)
}

func (p *PerfStatResult) MissPerKiloInstructions() float64 {
	return float64(p.LLCMisses) / float64(p.Instructions) * 1000
}

func (p *PerfStatResult) HitPerKiloInstructions() float64 {
	return float64(p.AllStores+p.AllLoads-p.LLCMisses) / float64(p.Instructions) * 1000
}

type PerfStatRunner interface {
	Start(ctx context.Context) <-chan map[int]*PerfStatResult
}

func NewPerfStatRunner(group *core.ProcessGroup) PerfStatRunner {
	return &perfStatRunner{
		group:  group,
		logger: log.New(os.Stdout, fmt.Sprintf("perfstat-%s: ", group.Id), log.Lshortfile|log.Lmsgprefix|log.LstdFlags),
		wg:     sync.WaitGroup{},
	}
}

type perfStatRunner struct {
	group  *core.ProcessGroup
	logger *log.Logger
	wg     sync.WaitGroup // 用于等待perf结束
}

func (p *perfStatRunner) perfRunner(pid int, cmd *exec.Cmd, position []*PerfStatResult) {
	defer p.wg.Done()
	buffer := bytes.NewBuffer(make([]byte, 0, 1024))
	cmd.Stderr = buffer
	p.logger.Printf("启动对进程%d的监控", pid)
	err := cmd.Run()
	if err != nil {
		p.logger.Printf("Perf Stat进程 %d 退出错误：%v", cmd.Process.Pid, err)
	} else {
		p.logger.Printf("Perf Stat进程 %d 正常退出", cmd.Process.Pid)
	}
	position[0] = p.parseResult(buffer)
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
		case pAllLoads:
			res.AllLoads = cnt
		case pAllStores:
			res.AllStores = cnt
		case pL3Miss:
			res.LLCMisses = cnt
		case pL3LoadMisses:
			res.LLCLoadMisses = cnt
		case pCycles:
			res.Cycles = cnt
		case pInstructions:
			res.Instructions = cnt
		case pL3MissCycles:
			res.LLCMissCycles = cnt
		case pMemAnyCycles:
			res.MemAnyCycles = cnt
		}
	}

	return res
}

func (p *perfStatRunner) Start(ctx context.Context) <-chan map[int]*PerfStatResult {
	resultCh := make(chan map[int]*PerfStatResult, 1)
	commands := make([]*exec.Cmd, len(p.group.Pid))
	results := make([]*PerfStatResult, len(p.group.Pid))
	for i := 0; i < len(p.group.Pid); i++ {
		commands[i] = exec.Command("perf", "stat", "-e", perfStatEvents, "-ip", fmt.Sprintf("%d", p.group.Pid[i]), "-x", ",")
		p.wg.Add(1)
		go p.perfRunner(p.group.Pid[i], commands[i], results[i:i+1])
	}

	p.logger.Println("启动perf stat监控")
	// 负责接收结果的主线成
	go func() {
		select {
		case <-time.After(core.RootConfig.PerfStat.SampleTime):
		case <-ctx.Done():
		}
		for _, command := range commands {
			_ = syscall.Kill(command.Process.Pid, syscall.SIGINT)
		}
		p.wg.Wait()
		resultMap := make(map[int]*PerfStatResult)
		for i, result := range results {
			resultMap[p.group.Pid[i]] = result
		}

		p.logger.Println("进程组perf stat监控结束")
		resultCh <- resultMap
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
