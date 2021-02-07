package perf

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type StatRunner interface {
	Start(ctx context.Context) <-chan map[int]*StatResult
}

func NewPerfStatRunner(group *core.ProcessGroup) StatRunner {
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

func (p *perfStatRunner) perfRunner(pid int, cmd *exec.Cmd, position []*StatResult) {
	defer p.wg.Done()
	buffer := bytes.NewBuffer(make([]byte, 0, 1024))
	cmd.Stderr = buffer
	p.logger.Printf("启动对进程 %d 的监控，命令：%s", pid, strings.Join(cmd.Args, " "))
	err := cmd.Run()
	if err != nil {
		p.logger.Printf("监控进程 %d 的Perf Stat进程 %d 退出错误：%v", pid, cmd.Process.Pid, err)
	} else {
		p.logger.Printf("监控进程 %d 的Perf Stat进程 %d 正常退出", pid, cmd.Process.Pid)
	}
	position[0] = p.parseResult(buffer)
	position[0].Pid = pid
}

func (p *perfStatRunner) parseResult(out io.Reader) *StatResult {
	statRecords, err := csv.NewReader(out).ReadAll()
	if err != nil {
		all, _ := ioutil.ReadAll(out)
		fmt.Println(string(all))
		return &StatResult{
			Pid:   0,
			Error: errors.Wrap(err, "解析Perf输出出错"),
		}
	}

	res := &StatResult{}
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
		cnt = uint64(float64(cnt) / (percent / 100.0))
		err = res.SetCount(record[2], cnt)
		if err != nil {
			p.logger.Println(err)
		}
	}

	return res
}

func (p *perfStatRunner) Start(ctx context.Context) <-chan map[int]*StatResult {
	resultCh := make(chan map[int]*StatResult, 1)
	commands := make([]*exec.Cmd, len(p.group.Pid))
	results := make([]*StatResult, len(p.group.Pid))
	for i := 0; i < len(p.group.Pid); i++ {
		commands[i] = exec.Command("perf", "stat", "-e", getEventList(), "-ip", fmt.Sprintf("%d", p.group.Pid[i]), "-x", ",")
		p.wg.Add(1)
		go p.perfRunner(p.group.Pid[i], commands[i], results[i:i+1])
	}

	p.logger.Println("启动perf stat监控")
	// 负责接收结果的主线程
	go func() {
		select {
		case <-time.After(core.RootConfig.PerfStat.SampleTime):
			p.logger.Printf("对进程组 %s 的监控结束", p.group.Id)
		case <-ctx.Done():
			p.logger.Printf("对进程组 %s 的监控中途被结束", p.group.Id)
		}
		for _, command := range commands {
			for command.Process == nil {
				// ll to prevent Process nil
				_ = syscall.Kill(command.Process.Pid, syscall.SIGINT)
			}
		}
		p.wg.Wait()
		resultMap := make(map[int]*StatResult)
		for i, result := range results {
			resultMap[p.group.Pid[i]] = result
		}

		p.logger.Println("进程组perf stat监控结束")
		resultCh <- resultMap
		close(resultCh)
	}()
	return resultCh
}

// Deprecated. 保留本函数测试用
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
