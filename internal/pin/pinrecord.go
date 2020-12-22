package pin

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/algorithm"
	"github.com/pkg/errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

type MemRecorder interface {
	Start() (<-chan map[int]algorithm.RTHCalculator, error)
}

type RTHCalculatorFactory func(tid int) algorithm.RTHCalculator

func NewMemAttachRecorder(factory RTHCalculatorFactory, pinToolPath string, pid int) MemRecorder {
	fifoPath := fmt.Sprintf("%d.fifo", pid)
	pinToolPath, _ = filepath.Abs(pinToolPath)
	cmd := exec.Command("pin", "-p", fmt.Sprintf("%d", pid), "-t",
		pinToolPath, "-binary", "-fifo", fifoPath)

	return &pinRecorder{
		pinCmd:   cmd,
		factory:  factory,
		fifoPath: fifoPath,
	}
}

func NewMemRunRecorder(factory RTHCalculatorFactory, pinToolPath string, cmd string, args ...string) MemRecorder {
	fifoPath := strings.Join(append(append([]string{strings.ReplaceAll(cmd, "/", "_")}, args...)), "_") + ".fifo"
	pinToolPath, _ = filepath.Abs(pinToolPath)
	pinArgs := []string{"-t", pinToolPath, "-binary", "-fifo", fifoPath, "-buffersize", "10000", "--", cmd}
	pinArgs = append(pinArgs, args...)
	pinCmd := exec.Command("pin", pinArgs...)

	return &pinRecorder{
		pinCmd:   pinCmd,
		factory:  factory,
		fifoPath: fifoPath,
	}
}

type pinRecorder struct {
	pinCmd   *exec.Cmd
	factory  RTHCalculatorFactory
	fifoPath string
	readCnt  uint // 性能优化使用，监测读取速度
}

func (m *pinRecorder) pinTraceReader(resChan chan map[int]algorithm.RTHCalculator) {
	fin, err := os.OpenFile(m.fifoPath, os.O_RDONLY, os.ModeNamedPipe)
	if err != nil {
		_ = os.Remove(m.fifoPath)
		return
	}
	defer func() {
		_ = fin.Close()
	}()

	reader := bufio.NewReader(fin)
	buf := make([]byte, unsafe.Sizeof(uint64(1)))
	var cnt int
	currTid := 0
	// 使用双重缓冲来降低开销
	buffer := [][]uint64{make([]uint64, 0, 20000), make([]uint64, 0, 20000)}
	using := 0
	addrList := buffer[using]
	cMap := make(map[int]algorithm.RTHCalculator)
	wg := sync.WaitGroup{}
	for cnt, err = reader.Read(buf); err == nil || (err == io.EOF && cnt != 0); cnt, err = reader.Read(buf) {
		m.readCnt++
		data := binary.LittleEndian.Uint64(buf)
		if data == 0 {
			// 上一次结束
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

			addrList = buffer[1^using]
			currTid = 0
			continue
		}
		if currTid == 0 {
			currTid = int(data)
		} else {
			addrList = append(addrList, data&0xFFFFFFFFFFFFFFC0)
		}
	}

	fmt.Println("结束了")
	_ = m.pinCmd.Process.Kill()
	_ = m.pinCmd.Wait()
	_ = os.Remove(m.fifoPath)
	resChan <- cMap
	close(resChan)
}

func (m *pinRecorder) Start() (<-chan map[int]algorithm.RTHCalculator, error) {
	err := syscall.Mkfifo(m.fifoPath, 0600)
	if err != nil {
		return nil, errors.Wrap(err, "创建具名管道失败")
	}

	m.pinCmd.Stderr = os.Stderr
	m.pinCmd.Stdout = os.Stdout
	err = m.pinCmd.Start()
	if err != nil {
		_ = os.Remove(m.fifoPath)
		return nil, errors.Wrap(err, "启动pin失败")
	}
	resChan := make(chan map[int]algorithm.RTHCalculator, 1)

	go m.pinTraceReader(resChan)
	go func(p *pinRecorder) {
		cnt := uint(0)
		for {
			<-time.After(time.Second)
			fmt.Printf("Curr: %-10d Total: %d\n", p.readCnt-cnt, p.readCnt)
			cnt = p.readCnt
		}
	}(m)
	return resChan, nil
}

var _ MemRecorder = &pinRecorder{}
