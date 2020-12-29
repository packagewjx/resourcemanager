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
	"sync"
	"time"
	"unsafe"
)

type MemRecorder interface {
	Start(ctx context.Context) (<-chan map[int]algorithm.RTHCalculator, error)
}

type RTHCalculatorFactory func(tid int) algorithm.RTHCalculator

type MemRecorderBaseConfig struct {
	Kill           bool
	Factory        RTHCalculatorFactory
	WriteThreshold int
	PinBufferSize  int
	PinStopAt      int
	PinToolPath    string
	RootDir        string
	GroupName      string
}

type MemRecorderRunConfig struct {
	MemRecorderBaseConfig
	Cmd  string
	Args []string
}

type MemRecorderAttachConfig struct {
	MemRecorderBaseConfig
	Pid int
}

const (
	DefaultWriteThreshold = 16000
	DefaultPinBufferSize  = 8000
	DefaultStopAt         = 5000000000
)

func NewMemAttachRecorder(config *MemRecorderAttachConfig) MemRecorder {
	fifoPath := mkTempFifo()
	pinToolPath, _ := filepath.Abs(config.PinToolPath)
	cmd := exec.Command("pin", "-pid", fmt.Sprintf("%d", config.Pid), "-t",
		pinToolPath, "-binary", "-fifo", fifoPath, "-buffersize", fmt.Sprintf("%d", config.PinBufferSize),
		"-stopat", fmt.Sprintf("%d", config.PinStopAt))

	return newMemRecorder(&config.MemRecorderBaseConfig, fifoPath, cmd)
}

func NewMemRunRecorder(config *MemRecorderRunConfig) MemRecorder {
	fifoPath := mkTempFifo()
	pinToolPath, _ := filepath.Abs(config.PinToolPath)
	pinArgs := []string{"-t", pinToolPath, "-binary", "-fifo", fifoPath, "-buffersize",
		fmt.Sprintf("%d", config.PinBufferSize), "-stopat", fmt.Sprintf("%d", config.PinStopAt), "--", config.Cmd}
	pinArgs = append(pinArgs, config.Args...)
	pinCmd := exec.Command("pin", pinArgs...)
	return newMemRecorder(&config.MemRecorderBaseConfig, fifoPath, pinCmd)
}

func newMemRecorder(config *MemRecorderBaseConfig, fifoPath string, pinCmd *exec.Cmd) MemRecorder {
	return &pinRecorder{
		kill:           config.Kill,
		writeThreshold: config.WriteThreshold,
		pinCmd:         pinCmd,
		factory:        config.Factory,
		fifoPath:       fifoPath,
		readCnt:        0,
		logger:         log.New(os.Stdout, fmt.Sprintf("pin-record-%s: ", config.GroupName), log.LstdFlags|log.Lmsgprefix|log.Lshortfile),
	}
}

type pinRecorder struct {
	kill           bool
	writeThreshold int
	pinCmd         *exec.Cmd
	factory        RTHCalculatorFactory
	fifoPath       string
	readCnt        uint // 性能优化使用，监测读取速度
	logger         *log.Logger
}

func (m *pinRecorder) pinTraceReader(ctx context.Context, resChan chan map[int]algorithm.RTHCalculator, cancelFunc context.CancelFunc) {
	defer func() {
		cancelFunc()
		if m.kill {
			// 由于结束太快，会导致Process为nil，需要等待一下再发信号
			for m.pinCmd.Process == nil {
				<-time.After(100 * time.Millisecond)
			}
			_ = m.pinCmd.Process.Kill()
		}
		_ = os.Remove(m.fifoPath)
		close(resChan)
	}()

	fin, err := os.OpenFile(m.fifoPath, os.O_RDONLY, os.ModeNamedPipe)
	if err != nil {
		m.logger.Println("打开管道文件失败", err)
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
			_ = exec.Command("cat", m.fifoPath, ">", "/dev/null").Run()
			break outerLoop
		default:
		}

		m.readCnt++
		data := binary.LittleEndian.Uint64(buf)
		if data == 0 {
			// 上一次结束
			if len(addrList) > m.writeThreshold {
				// 因为读取过快而消费过慢，会等待一段时间，因此读取到list足够长的时候，然后实际消费
				c, ok := cMap[currTid]
				if !ok {
					c = m.factory(currTid)
					cMap[currTid] = c
				}
				wg.Wait()
				wg.Add(1)
				go func(list []uint64) {
					c.Update(list)
					wg.Done()
				}(addrList)
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

	m.logger.Printf("采集结束，总共采集 %d 条内存访问地址", m.readCnt)
	_ = fin.Close()
	resChan <- cMap
}

func (m *pinRecorder) pinRunner(errCh chan<- error) {
	l := &logWriter{
		prefix: "Pin进程输出： ",
		logger: m.logger,
	}
	m.pinCmd.Stdout = l
	m.pinCmd.Stderr = l

	err := m.pinCmd.Run()
	if err != nil {
		errCh <- errors.Wrap(err, "运行Pin异常")
	} else {
		errCh <- nil
	}
	m.logger.Printf("Pin进程 %d 已退出，状态码 %d", m.pinCmd.Process.Pid, m.pinCmd.ProcessState.ExitCode())
}

func (m *pinRecorder) reporter(ctx context.Context) {
	cnt := uint(0)
	for {
		select {
		case <-time.After(time.Second):
			m.logger.Printf("采集速度： %10d/s 已采集： %d\n", m.readCnt-cnt, m.readCnt)
			cnt = m.readCnt
		case <-ctx.Done():
			return
		}
	}
}

func (m *pinRecorder) Start(childCtx context.Context) (<-chan map[int]algorithm.RTHCalculator, error) {
	errCh := make(chan error)
	go m.pinRunner(errCh)

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
			_ = os.Remove(m.fifoPath)
			return nil, err
		}
	}

	childCtx, cancel := context.WithCancel(childCtx)
	resChan := make(chan map[int]algorithm.RTHCalculator, 1)
	m.logger.Println("采集开始")
	go m.pinTraceReader(childCtx, resChan, cancel)
	go m.reporter(childCtx)
	return resChan, nil
}

var _ MemRecorder = &pinRecorder{}
