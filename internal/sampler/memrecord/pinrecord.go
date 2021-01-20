package memrecord

import (
	"bufio"
	"context"
	"encoding/binary"
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
	"time"
	"unsafe"
)

type Config struct {
	BufferSize     int
	WriteThreshold int
	PinToolPath    string
	TraceCount     int
	ConcurrentMax  int
}

type pinContext struct {
	name       string
	pinCmd     *exec.Cmd
	kill       bool
	factory    RTHCalculatorFactory
	resCh      chan *MemRecordResult
	readCnt    uint // 性能优化使用，监测读取速度
	fifoPath   string
	iCountPath string
}

func checkKernelConfig() error {
	file, err := ioutil.ReadFile("/proc/sys/kernel/yama/ptrace_scope")
	if err != nil {
		return errors.Wrap(err, "读取系统配置出错")
	}
	if string(file) != "0\n" {
		err = ioutil.WriteFile("/proc/sys/kernel/yama/ptrace_scope", []byte{'0', '\n'}, 0644)
		if err != nil {
			return errors.Wrap(err, "写入系统配置出错")
		}
	}
	return nil
}

func NewPinMemRecorder(config *Config) (MemRecorder, error) {
	err := checkKernelConfig()
	if err != nil {
		return nil, err
	}

	return &pinRecorder{
		traceCount:     config.TraceCount,
		toolPath:       config.PinToolPath,
		bufferSize:     config.BufferSize,
		writeThreshold: config.WriteThreshold,
		logger:         log.New(os.Stdout, "PinRecorder: ", log.LstdFlags|log.Lmsgprefix|log.Lshortfile),
		controlChan:    make(chan struct{}, config.ConcurrentMax),
	}, nil
}

type pinRecorder struct {
	traceCount     int
	toolPath       string
	bufferSize     int
	writeThreshold int
	logger         *log.Logger
	controlChan    chan struct{}
}

func (m *pinRecorder) readFromPipe(ctx context.Context, file io.Reader, pinCtx *pinContext) (map[int]algorithm.RTHCalculator, error) {
	reader := bufio.NewReader(file)
	buf := make([]byte, unsafe.Sizeof(uint64(1)))
	var cnt int
	var err error
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
			addr := data & 0xFFFFFFFFFFFF
			addrLine := addr & 0xFFFFFFFFFFC0
			length := data >> 48
			if length == 0 {
				m.logger.Printf("长度出现了为0的条目")
				length = 1
			}
			addrEnd := addr + length - 1
			addrEndLine := addrEnd & 0xFFFFFFFFFFC0
			lineCount := int((addrEndLine-addrLine)>>6 + 1)
			for i := 0; i < lineCount; i++ {
				addrList = append(addrList, addrLine)
				addrLine += 0x40
			}
		}
	}
	wg.Wait() // 读取完毕后可能还没有计算完毕，需要等待
	if err != nil && err != io.EOF {
		fmt.Println("读取异常结束", err)
	}

	// 判断是否没有读取到任何数据，返回错误
	if len(cMap) == 0 {
		pinCtx.resCh <- &MemRecordResult{
			Err: fmt.Errorf("对进程组的内存追踪没有结果"),
		}
		return nil, nil
	}
	return cMap, nil
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
		_ = os.Remove(pinCtx.iCountPath)
		close(pinCtx.resCh)
	}()

	// 为了避免OpenFile阻塞或者读取不到任何内容，进入之前先进行判断是否已经结束
	select {
	case <-ctx.Done():
		m.logger.Printf("%s 采集未开始即结束", pinCtx.name)
		pinCtx.resCh <- &MemRecordResult{
			Err: fmt.Errorf("采集未开始就结束"),
		}
		return
	default:
	}

	// 从管道读取数据
	fin, err := os.OpenFile(pinCtx.fifoPath, os.O_RDONLY, os.ModeNamedPipe)
	if err != nil {
		pinCtx.resCh <- &MemRecordResult{
			ThreadTrace: nil,
			Err:         errors.Wrap(err, "打开管道失败"),
		}
		return
	}
	cMap, err := m.readFromPipe(ctx, fin, pinCtx)
	if err != nil {
		pinCtx.resCh <- &MemRecordResult{
			Err: err,
		}
	}
	_ = fin.Close()

	// 读取指标数量文件用于加权平均
	counts, err := readInstructionCounts(pinCtx.iCountPath)
	totalCount := uint64(0)
	if err != nil {
		m.logger.Printf("读取指令数量文件 %s 出错： %v", pinCtx.iCountPath, err)
	} else {
		totalCount = counts[0]
	}

	m.logger.Printf("采集结束，总共采集 %d 条内存访问地址", pinCtx.readCnt)
	_ = fin.Close()
	pinCtx.resCh <- &MemRecordResult{
		ThreadTrace:            cMap,
		ThreadInstructionCount: counts,
		TotalInstructions:      totalCount,
		Err:                    nil,
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

func (m *pinRecorder) reporter(ctx context.Context, pinCtx *pinContext, cancelFunc context.CancelFunc) {
	cnt := uint(0)
	secondElapsed := 0
	for {
		select {
		case <-time.After(time.Second):
			m.logger.Printf("%s 采集速度： %10d/s 已采集： %d\n", pinCtx.name, pinCtx.readCnt-cnt, pinCtx.readCnt)
			cnt = pinCtx.readCnt
			if secondElapsed > 10 && cnt == 0 {
				// 如果超过10秒还是没有，应该错误
				m.logger.Printf("超过10秒没有采集到数据，中途结束")
				cancelFunc()
				return
			}
			secondElapsed++
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
			<-m.controlChan
			return
		}
	}

	childCtx, cancel := context.WithCancel(ctx)
	m.logger.Printf("%s: 采集开始", pinCtx.name)
	go m.pinTraceReader(childCtx, pinCtx, cancel)
	go m.reporter(childCtx, pinCtx, cancel)
	go func() {
		<-childCtx.Done()
		<-m.controlChan
	}()
}

func (m *pinRecorder) recordPreparation(requestName string) (fifoPath, pinToolPath, iCountPath string, err error) {
	fifoPath, err = mkTempFifo()
	if err != nil {
		return "", "", "", errors.Wrap(err, "创建管道失败")
	}
	fifoPath, _ = filepath.Abs(fifoPath)
	pinToolPath, _ = filepath.Abs(m.toolPath)
	iCountPath, _ = filepath.Abs(fmt.Sprintf("%s.icount.csv", requestName))
	return
}

func (m *pinRecorder) RecordCommand(ctx context.Context, request *MemRecordRunRequest) <-chan *MemRecordResult {
	resCh := make(chan *MemRecordResult, 1)
	fifoPath, pinToolPath, iCountPath, err := m.recordPreparation(request.Name)
	if err != nil {
		resCh <- &MemRecordResult{
			Err: err,
		}
		close(resCh)
		return resCh
	}
	pinArgs := []string{"-t", pinToolPath, "-binary", "-fifo", fifoPath, "-buffersize",
		fmt.Sprintf("%d", m.bufferSize), "-stopat",
		fmt.Sprintf("%d", m.traceCount), "-icountcsv", iCountPath, "--", request.Cmd}
	pinArgs = append(pinArgs, request.Args...)
	pinCmd := exec.Command(core.RootConfig.MemTrace.PinPath, pinArgs...)
	pinCtx := &pinContext{
		name:       request.Name,
		pinCmd:     pinCmd,
		kill:       request.Kill,
		factory:    request.Factory,
		resCh:      resCh,
		fifoPath:   fifoPath,
		iCountPath: iCountPath,
	}

	m.startMemTrace(ctx, pinCtx)
	return resCh
}

func (m *pinRecorder) RecordProcess(ctx context.Context, request *MemRecordAttachRequest) <-chan *MemRecordResult {
	resCh := make(chan *MemRecordResult, 1)
	fifoPath, pinToolPath, iCountPath, err := m.recordPreparation(request.Name)
	if err != nil {
		resCh <- &MemRecordResult{
			Err: err,
		}
		close(resCh)
		return resCh
	}
	pinCmd := exec.Command(core.RootConfig.MemTrace.PinPath, "-pid", fmt.Sprintf("%d", request.Pid), "-t",
		pinToolPath, "-binary", "-fifo", fifoPath, "-buffersize", fmt.Sprintf("%d", m.bufferSize),
		"-stopat", fmt.Sprintf("%d", m.traceCount), "-icountcsv", iCountPath)

	pinCtx := &pinContext{
		name:       request.Name,
		pinCmd:     pinCmd,
		kill:       request.Kill,
		factory:    request.Factory,
		resCh:      resCh,
		fifoPath:   fifoPath,
		iCountPath: iCountPath,
	}

	m.startMemTrace(ctx, pinCtx)
	return resCh
}

var _ MemRecorder = &pinRecorder{}

func readInstructionCounts(file string) (map[int]uint64, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	all, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	res := make(map[int]uint64)
	for _, record := range all {
		tid, err := strconv.ParseInt(record[0], 10, 32)
		if err != nil {
			return nil, errors.Wrap(err, "解析线程ID出错")
		}
		count, err := strconv.ParseUint(record[1], 10, 64)
		if err != nil {
			return nil, errors.Wrap(err, "解析指令数量出错")
		}
		res[int(tid)] = count
	}
	return res, nil
}
