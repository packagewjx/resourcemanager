package pin

import (
	"bufio"
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
	Start() (<-chan map[int]algorithm.RTHCalculator, error)
}

type RTHCalculatorFactory func(tid int) algorithm.RTHCalculator

const (
	DefaultWriteThreshold = 16000
	DefaultPinBufferSize  = 8000
)

func NewMemAttachRecorder(factory RTHCalculatorFactory, writeThreshold, pinBufferSize, pid int, pinToolPath string, groupName string) MemRecorder {
	fifoPath := mkTempFifo()
	pinToolPath, _ = filepath.Abs(pinToolPath)
	cmd := exec.Command("pin", "-p", fmt.Sprintf("%d", pid), "-t",
		pinToolPath, "-binary", "-fifo", fifoPath, "-buffersize", fmt.Sprintf("%d", pinBufferSize))

	return &pinRecorder{
		writeThreshold: writeThreshold,
		pinCmd:         cmd,
		factory:        factory,
		fifoPath:       fifoPath,
		logger:         log.New(os.Stdout, fmt.Sprintf("pin-record-%s: ", groupName), log.LstdFlags|log.Lmsgprefix),
	}
}

func NewMemRunRecorder(factory RTHCalculatorFactory, writeThreshold, pinBufferSize int, pinToolPath, groupName, cmd string, args ...string) MemRecorder {
	fifoPath := mkTempFifo()
	pinToolPath, _ = filepath.Abs(pinToolPath)
	pinArgs := []string{"-t", pinToolPath, "-binary", "-fifo", fifoPath, "-buffersize", fmt.Sprintf("%d", pinBufferSize), "--", cmd}
	pinArgs = append(pinArgs, args...)
	pinCmd := exec.Command("pin", pinArgs...)

	return &pinRecorder{
		writeThreshold: writeThreshold,
		pinCmd:         pinCmd,
		factory:        factory,
		fifoPath:       fifoPath,
		logger:         log.New(os.Stdout, fmt.Sprintf("pin-record-%s: ", groupName), log.LstdFlags|log.Lmsgprefix),
	}
}

type pinRecorder struct {
	writeThreshold int
	pinCmd         *exec.Cmd
	factory        RTHCalculatorFactory
	fifoPath       string
	readCnt        uint // 性能优化使用，监测读取速度
	running        bool
	logger         *log.Logger
}

func (m *pinRecorder) pinTraceReader(resChan chan map[int]algorithm.RTHCalculator) {
	defer func() {
		m.running = false
		_ = m.pinCmd.Process.Kill()
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
	for cnt, err = reader.Read(buf); err == nil || (err == io.EOF && cnt != 0); cnt, err = reader.Read(buf) {
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
	if err != nil && err != io.EOF {
		fmt.Println("读取异常结束", err)
	}

	m.logger.Printf("采集结束，总共采集 %d 条内存访问地址", m.readCnt)
	_ = fin.Close()
	resChan <- cMap
}

func (m *pinRecorder) pinRunner(errCh chan<- error) {
	m.pinCmd.Stdout = &logWriter{
		prefix: "Pin进程输出： ",
		logger: m.logger,
	}

	err := m.pinCmd.Run()
	if err != nil {
		errCh <- errors.Wrap(err, "运行Pin异常")
	}
	m.logger.Printf("Pin进程 %d 已退出，状态码 %d", m.pinCmd.Process.Pid, m.pinCmd.ProcessState.ExitCode())
}

func (m *pinRecorder) reporter() {
	cnt := uint(0)
	for m.running {
		<-time.After(time.Second)
		m.logger.Printf("采集速度： %10d/s 已采集： %d\n", m.readCnt-cnt, m.readCnt)
		cnt = m.readCnt
	}
}

func (m *pinRecorder) Start() (<-chan map[int]algorithm.RTHCalculator, error) {
	m.running = true

	errCh := make(chan error)
	go m.pinRunner(errCh)

	// 等待一段时间确保Pin成功运行，否则会引起OpenFile堵塞
	select {
	case <-time.After(500 * time.Millisecond):
	case err := <-errCh:
		return nil, err
	}
	go func() {
		<-errCh
	}()

	resChan := make(chan map[int]algorithm.RTHCalculator, 1)
	m.logger.Println("采集开始")
	go m.pinTraceReader(resChan)
	go m.reporter()
	return resChan, nil
}

var _ MemRecorder = &pinRecorder{}
