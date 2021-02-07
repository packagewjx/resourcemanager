package rthsample

import (
	"bufio"
	"context"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const doneFileName = "rth_sample_done"

func NewPinSampler(config *PinSamplerConfig) (RthSampler, error) {
	return &pinSampler{
		pinPath:     config.PinPath,
		pinToolPath: config.PinToolPath,
		logger:      log.New(os.Stdout, "PinSampler: ", log.LstdFlags|log.Lmsgprefix|log.Lshortfile),
	}, nil
}

type PinSamplerConfig struct {
	PinToolPath string
	PinPath     string
}

type pinSampler struct {
	pinPath     string
	pinToolPath string
	logger      *log.Logger
}

type pinContext struct {
	pinCmd  *exec.Cmd
	resCh   chan *Result
	maxTime int
	tmpDir  string
}

func (p *pinSampler) SampleCommand(ctx context.Context, cmd string, args []string, maxTime int) chan *Result {
	tmpDir, _ := ioutil.TempDir(os.TempDir(), "tmp.rth.*")
	args = append([]string{"-t", p.pinToolPath, "-output_dir", tmpDir, "-reservoir_size", fmt.Sprintf("%d", 300000),
		"--", cmd}, args...)
	pinCmd := exec.Command(p.pinPath, args...)
	resCh := make(chan *Result, 1)
	pinCtx := &pinContext{
		pinCmd:  pinCmd,
		resCh:   resCh,
		maxTime: maxTime,
		tmpDir:  tmpDir,
	}

	go p.mainRoutine(ctx, pinCtx)

	return resCh
}

func (p *pinSampler) SampleProcess(ctx context.Context, pid int, maxTime int) chan *Result {
	tmpDir, _ := ioutil.TempDir(os.TempDir(), "tmp.rth.*")
	pinCmd := exec.Command(p.pinPath, "-pid", fmt.Sprintf("%d", pid), "-t", p.pinToolPath, "-output_path", tmpDir,
		"-reservoir_size", fmt.Sprintf("%d", 300000))
	resCh := make(chan *Result, 1)
	pinCtx := &pinContext{
		pinCmd:  pinCmd,
		resCh:   resCh,
		maxTime: maxTime,
		tmpDir:  tmpDir,
	}

	go p.mainRoutine(ctx, pinCtx)

	return resCh
}

func (p *pinSampler) mainRoutine(ctx context.Context, pinCtx *pinContext) {
	err := pinCtx.pinCmd.Start()
	if err != nil {
		pinCtx.resCh <- &Result{
			Error: errors.Wrap(err, "启动Pin错误"),
		}
		return
	}

	fileWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		pinCtx.resCh <- &Result{
			Error: errors.Wrap(err, "启动监视器错误"),
		}
		return
	}

	err = fileWatcher.Add(pinCtx.tmpDir)
	if err != nil {
		pinCtx.resCh <- &Result{
			Error: errors.Wrap(err, "监听输出错误"),
		}
		return
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- pinCtx.pinCmd.Wait()
	}()

	defer func() {
		close(errCh)
		close(pinCtx.resCh)
		_ = fileWatcher.Close()
		p.logger.Println("RTH取样退出")
		_ = os.RemoveAll(pinCtx.tmpDir)
	}()

	doneFileAppear := false
	pinEnded := false
	fileList := make([]string, 0, 10)
	for !(doneFileAppear && pinEnded) {
		select {
		case <-ctx.Done():
			p.logger.Println("接收到退出信号，正在清理并退出")
			if pinCtx.pinCmd.ProcessState == nil {
				_ = pinCtx.pinCmd.Process.Kill()
			}
			pinCtx.resCh <- &Result{
				Error: fmt.Errorf("取样提前结束"),
			}
			return
		case ev := <-fileWatcher.Events:
			name := filepath.Base(ev.Name)
			if name == doneFileName {
				doneFileAppear = true
			} else {
				fileList = append(fileList, ev.Name)
			}
		case err = <-errCh:
			if err != nil {
				p.logger.Println("正常退出")
			}
			pinEnded = true
		}
	}

	pinCtx.resCh <- &Result{
		Rth: p.readFiles(fileList, pinCtx.maxTime),
	}
}

func (p *pinSampler) readFiles(fileList []string, maxTime int) map[int][]int {
	res := make([][]int, len(fileList))
	tid := make([]int, len(fileList))
	for i := 0; i < len(fileList); i++ {
		go func(resPos [][]int, tidPos []int, fileName string) {
			resPos[0] = ReadRthSampleCsv(fileName, maxTime)
			tidString := fileName[7 : len(fileName)-4]
			tid, _ := strconv.ParseInt(tidString, 0, 32)
			tidPos[0] = int(tid)
		}(res[i:i+1], tid[i:i+1], fileList[i])
	}

	m := make(map[int][]int)
	for i, re := range res {
		m[tid[i]] = re
	}
	return m
}

func ReadRthSampleCsv(file string, maxTime int) []int {
	f, _ := os.Open(file)
	reader := bufio.NewReader(f)
	var line string
	var err error
	rth := make([]int, maxTime+2)
	for line, err = reader.ReadString('\n'); err == nil; line, err = reader.ReadString('\n') {
		startIdx := strings.Index(line, ",")
		rt, _ := strconv.ParseInt(line[startIdx+1:len(line)-1], 0, 32)
		if rt > int64(maxTime) {
			rth[maxTime+1]++
		} else {
			rth[rt]++
		}
	}
	if err != nil && err != io.EOF {
		panic("居然读取出错了：" + err.Error())
	}
	return rth
}
