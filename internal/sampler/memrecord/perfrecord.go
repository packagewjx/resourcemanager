package memrecord

import (
	"bytes"
	"context"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func NewPerfRecorder(overflowCount int, switchOutput, perfExecPath string) (MemRecorder, error) {
	r := regexp.MustCompile("^(signal)|(\\d+[smhdBKMG])$")
	match := r.FindString(switchOutput)
	if match != switchOutput {
		return nil, fmt.Errorf("switchOutput `%s` 格式错误", switchOutput)
	}
	perfExecPath, err := filepath.Abs(perfExecPath)
	if err != nil {
		return nil, errors.Wrap(err, "解析perf路径出错")
	}
	if _, err = os.Stat(filepath.Join(perfExecPath, "perf")); os.IsNotExist(err) {
		return nil, fmt.Errorf("perf文件不存在于 %s", perfExecPath)
	}
	if overflowCount < 1 {
		overflowCount = 1
	}
	_, file, _, _ := runtime.Caller(0)

	return &perfRecorder{
		overflowCount: overflowCount,
		switchOutput:  "--switch-output=" + switchOutput,
		perfExecPath:  perfExecPath,
		currentPath:   filepath.Dir(file),
		logger:        log.New(os.Stdout, "PerfRecorder: ", log.LstdFlags|log.Lshortfile|log.Lmsgprefix),
	}, nil
}

type perfRecorder struct {
	overflowCount int
	switchOutput  string
	perfExecPath  string
	currentPath   string
	logger        *log.Logger
}

type perfRecordContext struct {
	ctx         context.Context
	cancelFunc  context.CancelFunc
	perfCmd     *exec.Cmd
	tmpDir      string
	resCh       chan *Result
	fileWatcher *fsnotify.Watcher
	consumer    CacheLineAddressConsumer
	readCnt     uint64
}

func (p *perfRecorder) newContext(ctx context.Context, consumer CacheLineAddressConsumer) (*perfRecordContext, error) {
	tmpDir, err := ioutil.TempDir(os.TempDir(), "tmp.perfaddr.*")
	if err != nil {
		return nil, errors.Wrap(err, "无法创建临时文件夹")
	}
	outFile := filepath.Join(tmpDir, "perf.data")
	args := []string{"record", "-e", "\"{mem_inst_retired.all_loads:P,mem_inst_retired.all_stores:P,pebs_addr:pebs_addr}\"",
		"-c", strconv.FormatInt(int64(p.overflowCount), 10), p.switchOutput, "-o", outFile}
	cmd := exec.Command(filepath.Join(p.perfExecPath, "perf"), args...)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, errors.Wrap(err, "创建File Watcher出错")
	}
	err = watcher.Add(tmpDir)
	if err != nil {
		return nil, errors.Wrap(err, "添加监控路径出错")
	}
	childCtx, cancel := context.WithCancel(ctx)
	return &perfRecordContext{
		ctx:         childCtx,
		cancelFunc:  cancel,
		perfCmd:     cmd,
		tmpDir:      tmpDir,
		resCh:       make(chan *Result),
		fileWatcher: watcher,
		consumer:    consumer,
		readCnt:     0,
	}, nil
}

func (p *perfRecorder) perfCmdRunner(perfCtx *perfRecordContext) {
	fmt.Println(strings.Join(perfCtx.perfCmd.Args, " "))
	_ = perfCtx.perfCmd.Start()
	errCh := make(chan error)
	go func() {
		errCh <- perfCtx.perfCmd.Wait()
		close(errCh)
		_ = perfCtx.fileWatcher.Close()
	}()

	for {
		select {
		case err := <-errCh:
			if err != nil {
				p.logger.Println("Perf进程退出错误", err)
			} else {
				p.logger.Println("Perf进程正常退出")
			}
			return
		case <-perfCtx.ctx.Done():
			_ = syscall.Kill(perfCtx.perfCmd.Process.Pid, syscall.SIGINT)
		}
	}
}

func (p *perfRecorder) resultReader(perfCtx *perfRecordContext) {
	ended := false
	evCh := perfCtx.fileWatcher.Events
	res := &Result{
		ThreadInstructionCount: map[int]uint64{},
		TotalInstructions:      0,
		Err:                    nil,
	}
	defer func() {
		perfCtx.cancelFunc()
		p.logger.Println("地址追踪读取结束")
		_ = os.RemoveAll(perfCtx.tmpDir)
		perfCtx.resCh <- res
	}()

	for !ended {
		select {
		case <-perfCtx.ctx.Done():
			ended = true
		case ev, ok := <-evCh:
			file := filepath.Base(ev.Name)
			if file != "perf.data" && ev.Op == fsnotify.Create {
				p.logger.Printf("正在读取文件 %s", ev.Name)
				if file != "perf.data" {
					result := p.parseResult(ev.Name, &perfCtx.readCnt)
					for tid, addrList := range result {
						res.TotalInstructions += uint64(len(addrList))
						res.ThreadInstructionCount[tid] += uint64(len(addrList))
						perfCtx.consumer.Consume(tid, addrList)
					}
					_ = os.Remove(ev.Name)
				}
			}
			if !ok {
				return
			}
		}
	}
}

func (p *perfRecorder) parseResult(file string, readCnt *uint64) map[int][]uint64 {
	res := map[int][]uint64{}
	readWriter := bytes.NewBuffer(make([]byte, 0, 4096))
	perfScriptCmd := exec.Command(filepath.Join(p.perfExecPath, "perf"), "script", "-s", filepath.Join(p.currentPath, "perfaddr.py"),
		"-i", file)
	perfScriptCmd.Env = append(perfScriptCmd.Env, "PERF_EXEC_PATH="+p.perfExecPath)
	perfScriptCmd.Stdout = readWriter
	perfScriptCmd.Stderr = os.Stdout
	_ = perfScriptCmd.Start()

	ended := false
	go func() {
		_ = perfScriptCmd.Wait()
		ended = true
	}()
	const lineLen = 30
	buf := make([]byte, lineLen)
	zeroCount := 0
	originalCount := *readCnt
	for !ended || readWriter.Len() > 0 {
		if readWriter.Len() < lineLen {
			<-time.After(100 * time.Millisecond)
		}
		_, _ = readWriter.Read(buf)
		tid, _ := strconv.ParseInt(string(buf[:10]), 0, 32)
		addr, _ := strconv.ParseUint(string(buf[11:len(buf)-1]), 0, 64)
		if addr == 0 {
			zeroCount++
			continue
		}
		addr &= 0x0000FFFFFFFFFFC0
		res[int(tid)] = append(res[int(tid)], addr)
		*readCnt++
	}
	p.logger.Printf("在文件 %s 读取到地址共 %d 条，其中为 0 的记录 %d 条", file, *readCnt-originalCount, zeroCount)
	return res
}

func (p *perfRecorder) RecordCommand(ctx context.Context, request *RunRequest) (<-chan *Result, error) {
	newContext, err := p.newContext(ctx, request.Consumer)
	if err != nil {
		return nil, err
	}
	newContext.perfCmd.Args = append(newContext.perfCmd.Args, "--", request.Cmd)
	newContext.perfCmd.Args = append(newContext.perfCmd.Args, request.Args...)

	go p.perfCmdRunner(newContext)
	go p.resultReader(newContext)
	return newContext.resCh, nil
}

func (p *perfRecorder) RecordProcess(ctx context.Context, request *AttachRequest) (<-chan *Result, error) {
	newContext, err := p.newContext(ctx, request.Consumer)
	if err != nil {
		return nil, err
	}
	newContext.perfCmd.Args = append(newContext.perfCmd.Args, "-p", strconv.FormatInt(int64(request.Pid), 10))

	go p.perfCmdRunner(newContext)
	go p.resultReader(newContext)
	return newContext.resCh, nil
}
