package perf

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/algorithm"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	perfL1Hit        = "mem_load_retired.l1_hit"
	perfL2Hit        = "mem_load_retired.l2_hit"
	perfL3Hit        = "mem_load_retired.l3_hit"
	perfL3Miss       = "mem_load_retired.l3_miss"
	perfInstructions = "instructions"
	perfStatEvents   = perfL1Hit + "," + perfL2Hit + "," + perfL3Hit + "," + perfL3Miss + "," + perfInstructions
)

var l1Size int

func init() {
	s, err := ioutil.ReadFile("/sys/devices/system/cpu/cpu0/cache/index0/size")
	if err == nil {
		if s[len(s)-1] == 'K' {
			i, _ := strconv.ParseInt(string(s[:len(s)-1]), 10, 32)
			l1Size = 1024 * int(i) / 64
		} else {
			i, _ := strconv.ParseInt(string(s), 10, 32)
			l1Size = int(i) / 64
		}
	}
}

type PerfRecord struct {
	Rth          []int
	L1Hit        uint64
	L2Hit        uint64
	L3Hit        uint64
	L3Miss       uint64
	Instructions uint64
}

type PerfRecorder interface {
	Start(ctx context.Context)
	FinishSampling(maxTime int) (*PerfRecord, error) // 结束采样并计算RTH
}

func NewPerfMemRecorderWithReservoir(group *core.ProcessGroup, reservoirSize int) (PerfRecorder, error) {
	return newPerfRecorder(group, algorithm.ReservoirCalculator(reservoirSize))
}

// Just for verifying
func NewPerfMemRecorderWithFullTrace(group *core.ProcessGroup) (PerfRecorder, error) {
	return newPerfRecorder(group, algorithm.FullTraceCalculator())
}

func newPerfRecorder(group *core.ProcessGroup, calculator algorithm.RTHCalculator) (PerfRecorder, error) {
	dir, err := ioutil.TempDir("", "tmp.*")
	if err != nil {
		return nil, errors.Wrap(err, "创建临时文件夹出错")
	}
	return &perfMemRecorder{
		group:         group,
		rthCalculator: calculator,
		tmpDir:        dir,
		logger:        log.New(os.Stdout, "PerfRecord-"+group.Id+": ", log.Lshortfile|log.Lmsgprefix|log.LstdFlags),
		wg:            sync.WaitGroup{},
	}, nil
}

type perfMemRecorder struct {
	cancelFunc    context.CancelFunc
	group         *core.ProcessGroup
	rthCalculator algorithm.RTHCalculator
	tmpDir        string
	logger        *log.Logger
	wg            sync.WaitGroup
	statOut       []byte
}

func (p *perfMemRecorder) Start(ctx context.Context) {
	outFile := filepath.Join(p.tmpDir, "perf.data")
	pidString := make([]string, len(p.group.Pid))
	for i, pid := range p.group.Pid {
		pidString[i] = strconv.FormatInt(int64(pid), 10)
	}
	pidListString := strings.Join(pidString, ",")
	recordCmd := exec.Command("perf", "record", "-e", "cpu/mem-loads,ldlat=30/P,cpu/mem-stores/P", "-d", "-c", "1", "-o",
		outFile,
		"--switch-output=1s", "-ip", pidListString)
	statCmd := exec.Command("perf", "stat", "-e", perfStatEvents, "-ip", pidListString, "-x", ",")
	statStderr, err := statCmd.StderrPipe()
	if err != nil {
		p.logger.Printf("打开stat命令的stdout出错：%s", err.Error())
	}
	err = recordCmd.Start()
	if err != nil {
		p.logger.Printf("perf record命令执行出错：%s", err.Error())
		return
	}
	err = statCmd.Start()
	if err != nil {
		p.logger.Printf("perf stat命令执行出错：%s", err.Error())
		_ = recordCmd.Process.Signal(syscall.SIGINT)
		_ = recordCmd.Wait()
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	p.cancelFunc = cancel

	go func() {
		p.wg.Add(1)
		defer p.wg.Done()
		defer func() {
			_ = os.RemoveAll(p.tmpDir)
			p.logger.Printf("等待perf record进程%d结束", recordCmd.Process.Pid)
			_ = recordCmd.Process.Signal(syscall.SIGINT)
			_ = recordCmd.Wait()
			p.logger.Printf("等待perf stat进程%d结束", statCmd.Process.Pid)
			_ = statStderr.Close()
			_ = statCmd.Wait()
			p.logger.Println("perf进程退出完毕，监控退出")
		}()

		tick := time.Tick(time.Second)
		for {
			select {
			case <-tick:
				if err := syscall.Kill(recordCmd.Process.Pid, 0); err != nil {
					p.logger.Println(err)
					p.logger.Printf("perf record进程已退出")
					return
				}
				addr := make([]uint64, 0)
				err := filepath.Walk(p.tmpDir, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					if info.IsDir() {
						return nil
					}
					if info.Name() == "perf.data" {
						return nil
					}

					p.logger.Printf("正在读取文件%s", path)
					res, err := ReadPerfMemTrace(path)
					if err != nil {
						return errors.Wrap(err, fmt.Sprintf("读取文件%s出错", path))
					}
					_ = os.Remove(path)
					addr = append(addr, res...)
					return nil
				})
				if err != nil {
					p.logger.Printf("读取出错：%s", err.Error())
					continue
				}
				p.logger.Printf("读取到%d条地址记录", len(addr))
				p.rthCalculator.Update(addr)
			case <-ctx.Done():
				p.logger.Printf("结束对进程组%s的内存访问监控", p.group.Id)
				// 读取StatCommand的输出
				_ = statCmd.Process.Signal(syscall.SIGINT)
				statOut, err := ioutil.ReadAll(statStderr)
				if err != nil {
					p.logger.Printf("读取perf stat输出出错：%s", err.Error())
				}
				p.statOut = statOut
				return
			}
		}
	}()
}

func (p *perfMemRecorder) FinishSampling(maxTime int) (*PerfRecord, error) {
	if p.cancelFunc == nil {
		return nil, fmt.Errorf("尚未开始取样")
	}
	if maxTime < l1Size {
		p.logger.Printf("maxTime为 %d 小于L1Size %d，将设置为%d", maxTime, l1Size, l1Size)
		maxTime = l1Size
	}
	p.cancelFunc()
	p.wg.Wait()
	res := &PerfRecord{}
	res.Rth = p.rthCalculator.GetRTH(maxTime)
	statRecords, err := csv.NewReader(bytes.NewBuffer(p.statOut)).ReadAll()
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("进程组%s 读取perf stat失败", p.group.Id))
	}
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
		case perfL1Hit:
			res.L1Hit = cnt
		case perfL2Hit:
			res.L2Hit = cnt
		case perfL3Hit:
			res.L3Hit = cnt
		case perfL3Miss:
			res.L3Miss = cnt
		case perfInstructions:
			res.Instructions = cnt
		}
	}
	// 修正数据

	return res, nil
}

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
	for record, err = reader.Read(); err == nil; record, err = reader.Read() {
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
