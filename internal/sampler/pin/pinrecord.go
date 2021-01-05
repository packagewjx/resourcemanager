package pin

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/algorithm"
	"github.com/pkg/errors"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"
)

type MemRecorder interface {
	// 对一个命令进行取样，返回该命令的所有子线程的取样结果，结果以RTHCalculator呈现，可用于计算MRC
	RecordCommand(ctx context.Context, request *MemRecordRunRequest) <-chan *MemRecordResult
	RecordProcess(ctx context.Context, request *MemRecordAttachRequest) <-chan *MemRecordResult
}

type MemRecordBaseRequest struct {
	Factory RTHCalculatorFactory
	Name    string // 用于日志显示
	Kill    bool
	RootDir string // 预留，用于容器使用
}

type MemRecordRunRequest struct {
	MemRecordBaseRequest
	Cmd  string
	Args []string
}

type MemRecordAttachRequest struct {
	MemRecordBaseRequest
	Pid int
}

type MemRecordResult struct {
	ThreadTrace map[int]algorithm.RTHCalculator
	Err         error
}

type RTHCalculatorFactory func(tid int) algorithm.RTHCalculator

type Config struct {
	BufferSize     int
	WriteThreshold int
	PinToolPath    string
	TraceCount     int
	ConcurrentMax  int
}

type MemRecorderRunConfig struct {
	Config
	Cmd  string
	Args []string
}

type MemRecorderAttachConfig struct {
	Config
	Pid int
}

type pinContext struct {
	name     string
	pinCmd   *exec.Cmd
	kill     bool
	factory  RTHCalculatorFactory
	resCh    chan *MemRecordResult
	readCnt  uint // 性能优化使用，监测读取速度
	fifoPath string
}

func NewMemRecorder(config *Config) MemRecorder {
	return &pinRecorder{
		traceCount:     config.TraceCount,
		toolPath:       config.PinToolPath,
		bufferSize:     config.BufferSize,
		writeThreshold: config.WriteThreshold,
		logger:         log.New(os.Stdout, "PinRecorder: ", log.LstdFlags|log.Lmsgprefix|log.Lshortfile),
		controlChan:    make(chan struct{}, config.ConcurrentMax),
	}
}

type pinRecorder struct {
	traceCount     int
	toolPath       string
	bufferSize     int
	writeThreshold int
	logger         *log.Logger
	controlChan    chan struct{}
}

func (m *pinRecorder) pinTraceReader(ctx context.Context, pinCtx *pinContext, cancelFunc context.CancelFunc) {
	defer func() {
		cancelFunc()
		if pinCtx.kill {
			// 由于结束太快，会导致Process为nil，需要等待一下再发信号
			for pinCtx.pinCmd.Process == nil {
				<-time.After(100 * time.Millisecond)
			}
			_ = pinCtx.pinCmd.Process.Kill()
		}
		_ = os.Remove(pinCtx.fifoPath)
		close(pinCtx.resCh)
	}()

	fin, err := os.OpenFile(pinCtx.fifoPath, os.O_RDONLY, os.ModeNamedPipe)
	if err != nil {
		pinCtx.resCh <- &MemRecordResult{
			ThreadTrace: nil,
			Err:         errors.Wrap(err, "打开管道失败"),
		}
		return
	}

	reader := bufio.NewReader(fin)
	buf := make([]byte, unsafe.Sizeof(uint64(1)))
	var cnt int
	currTid := 0
	addrListMap := make(map[int][]uint64) // 保存所有正在读取中的list
	var addrList []uint64                 // 当前使用的addrList
	cMap := make(map[int]algorithm.RTHCalculator)
	wg := sync.WaitGroup{}
	m.logger.Println("开始从管道读取监控数据")
outerLoop:
	for cnt, err = reader.Read(buf); err == nil || (err == io.EOF && cnt != 0); cnt, err = reader.Read(buf) {
		select {
		case <-ctx.Done():
			m.logger.Println("采集中途结束")
			// 需要将管道内容重导向到null
			_ = exec.Command("cat", pinCtx.fifoPath, ">", "/dev/null").Run()
			break outerLoop
		default:
		}

		pinCtx.readCnt++
		data := binary.LittleEndian.Uint64(buf)
		if data == 0 {
			// 上一次结束
			if len(addrList) > m.writeThreshold {
				// 因为读取过快而消费过慢，会等待一段时间，因此读取到list足够长的时候，然后实际消费
				c, ok := cMap[currTid]
				if !ok {
					c = pinCtx.factory(currTid)
					cMap[currTid] = c
				}
				wg.Wait()
				wg.Add(1)
				go func(calculator algorithm.RTHCalculator, list []uint64) {
					c.Update(list)
					wg.Done()
				}(c, addrList)
				addrListMap[currTid] = nil
			} else {
				addrListMap[currTid] = addrList
			}

			currTid = 0
			addrList = nil
			continue
		}
		if currTid == 0 {
			currTid = int(data)
			addrList = addrListMap[currTid]
			if addrList == nil {
				addrList = make([]uint64, 0, 32768)
				addrListMap[currTid] = addrList
			}
		} else {
			addrList = append(addrList, data&0xFFFFFFFFFFFFFFC0)
		}
	}
	wg.Wait() // 读取完毕后可能还没有计算完毕，需要等待
	if err != nil && err != io.EOF {
		fmt.Println("读取异常结束", err)
	}

	m.logger.Printf("采集结束，总共采集 %d 条内存访问地址", pinCtx.readCnt)
	_ = fin.Close()
	pinCtx.resCh <- &MemRecordResult{
		ThreadTrace: cMap,
		Err:         nil,
	}
}

func (m *pinRecorder) pinCmdRunner(pinCmd *exec.Cmd, errCh chan<- error) {
	l := &logWriter{
		logger: m.logger,
	}
	pinCmd.Stdout = l
	pinCmd.Stderr = l

	err := pinCmd.Start()
	if err != nil {
		errCh <- errors.Wrap(err, "启动Pin异常")
		return
	}
	m.logger.Printf("Pin进程 %d 启动，命令：%s", pinCmd.Process.Pid, strings.Join(pinCmd.Args, " "))
	l.prefix = fmt.Sprintf("Pin进程 %d 输出： ", pinCmd.Process.Pid)

	err = pinCmd.Wait()
	m.logger.Printf("Pin进程 %d 已退出，状态码 %d", pinCmd.Process.Pid, pinCmd.ProcessState.ExitCode())
	if err != nil {
		errCh <- errors.Wrap(err, "Pin退出异常")
	} else {
		errCh <- nil
	}
}

func (m *pinRecorder) reporter(ctx context.Context, pinCtx *pinContext) {
	cnt := uint(0)
	for {
		select {
		case <-time.After(time.Second):
			m.logger.Printf("%s 采集速度： %10d/s 已采集： %d\n", pinCtx.name, pinCtx.readCnt-cnt, pinCtx.readCnt)
			cnt = pinCtx.readCnt
		case <-ctx.Done():
			return
		}
	}
}

func (m *pinRecorder) startMemTrace(ctx context.Context, pinCtx *pinContext) {
outerLoop:
	for {
		select {
		case <-time.After(time.Second):
			m.logger.Printf("%s: 正在等待可用pin实例", pinCtx.name)
		case m.controlChan <- struct{}{}:
			break outerLoop
		}
	}

	errCh := make(chan error)
	go m.pinCmdRunner(pinCtx.pinCmd, errCh)

	// 确保Pin成功运行，否则会引起OpenFile堵塞
	select {
	case <-time.After(500 * time.Millisecond):
		go func() {
			<-errCh
			close(errCh)
		}()
	case err := <-errCh:
		close(errCh)
		if err != nil {
			_ = os.Remove(pinCtx.fifoPath)
			pinCtx.resCh <- &MemRecordResult{
				ThreadTrace: nil,
				Err:         err,
			}
			close(pinCtx.resCh)
			return
		}
	}

	childCtx, cancel := context.WithCancel(ctx)
	m.logger.Printf("%s: 采集开始", pinCtx.name)
	go m.pinTraceReader(childCtx, pinCtx, cancel)
	go m.reporter(childCtx, pinCtx)
	go func() {
		<-childCtx.Done()
		<-m.controlChan
	}()
}

func (m *pinRecorder) RecordCommand(ctx context.Context, request *MemRecordRunRequest) <-chan *MemRecordResult {
	resCh := make(chan *MemRecordResult, 1)
	fifoPath, err := mkTempFifo()
	if err != nil {
		resCh <- &MemRecordResult{
			ThreadTrace: nil,
			Err:         errors.Wrap(err, "创建管道失败"),
		}
		close(resCh)
		return resCh
	}
	pinToolPath, _ := filepath.Abs(m.toolPath)
	pinArgs := []string{"-t", pinToolPath, "-binary", "-fifo", fifoPath, "-buffersize",
		fmt.Sprintf("%d", m.bufferSize), "-stopat",
		fmt.Sprintf("%d", m.traceCount), "--", request.Cmd}
	pinArgs = append(pinArgs, request.Args...)
	pinCmd := exec.Command("pin", pinArgs...)
	pinCtx := &pinContext{
		name:     request.Name,
		pinCmd:   pinCmd,
		kill:     request.Kill,
		factory:  request.Factory,
		resCh:    resCh,
		fifoPath: fifoPath,
	}

	m.startMemTrace(ctx, pinCtx)
	return resCh
}

func (m *pinRecorder) RecordProcess(ctx context.Context, request *MemRecordAttachRequest) <-chan *MemRecordResult {
	resCh := make(chan *MemRecordResult, 1)
	fifoPath, err := mkTempFifo()
	if err != nil {
		resCh <- &MemRecordResult{
			ThreadTrace: nil,
			Err:         errors.Wrap(err, "创建管道失败"),
		}
		return resCh
	}
	pinToolPath, _ := filepath.Abs(m.toolPath)
	pinCmd := exec.Command("pin", "-pid", fmt.Sprintf("%d", request.Pid), "-t",
		pinToolPath, "-binary", "-fifo", fifoPath, "-buffersize", fmt.Sprintf("%d", m.bufferSize),
		"-stopat", fmt.Sprintf("%d", m.traceCount))

	pinCtx := &pinContext{
		name:     request.Name,
		pinCmd:   pinCmd,
		kill:     request.Kill,
		factory:  request.Factory,
		resCh:    resCh,
		fifoPath: fifoPath,
	}

	m.startMemTrace(ctx, pinCtx)
	return resCh
}

var _ MemRecorder = &pinRecorder{}
