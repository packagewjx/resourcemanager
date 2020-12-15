package perf

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

type IntelPTProcessor interface {
	Start(ctx context.Context)
	Wait() // 处理是异步的，使用本函数将会等待处理结束
}

func NewIntelPTProcessor(perfDataFile string, ip InstructionProcessor) (IntelPTProcessor, error) {
	cmd := exec.Command("perf", "script", "--insn-trace", "-F", "tid,insn", "--xed", "-i", perfDataFile)
	return &ptImpl{
		cmd: cmd,
		ip:  ip,
		wg:  sync.WaitGroup{},
	}, nil
}

type ptImpl struct {
	cmd *exec.Cmd
	ip  InstructionProcessor
	wg  sync.WaitGroup
}

func (p *ptImpl) Wait() {
	p.wg.Wait()
}

func (p *ptImpl) Start(ctx context.Context) {
	p.wg.Add(1)
	go func() {
		defer func() {
			p.wg.Done()
		}()
		pipe, err := p.cmd.StdoutPipe()
		if err != nil {
			fmt.Println("打开标准输出出错", err)
			return
		}
		reader := bufio.NewReader(pipe)
		_ = p.cmd.Start()
		tidMap := make(map[string]int)
	outerLoop:
		for line, err := reader.ReadString('\n'); err == nil; line, err = reader.ReadString('\n') {
			select {
			case <-ctx.Done():
				break outerLoop
			default:
				line = strings.TrimSpace(line)
				arr := strings.Split(line, "\t")
				tid, ok := tidMap[arr[0]]
				if !ok {
					i, _ := strconv.ParseInt(strings.TrimSpace(arr[0]), 10, 64)
					tid = int(i)
					tidMap[arr[0]] = tid
				}
				p.ip.Process(tid, arr[2])
			}
		}
		_ = pipe.Close()
		_ = p.cmd.Wait()
	}()
}

type InstructionProcessor interface {
	// 处理1条指令
	// tid为线程号，ins为具体指令，cnt为第几个指令
	Process(tid int, ins string)
}

type InstructionProcessorFinisher interface {
	InstructionProcessor
	Finish()
}

type MemoryAccessInstructionsFinder struct {
	TargetInstructions map[string]struct{}
}

func (m *MemoryAccessInstructionsFinder) Process(_ int, ins string) {
	if strings.Index(ins, "(") != -1 {
		split := strings.Split(ins, " ")
		m.TargetInstructions[split[0]] = struct{}{}
	}
}

type GccAssembler struct {
	Contexts map[int]*GccAssembleContext
}

type GccAssembleContext struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
}

func (g *GccAssembler) Finish() {
	for _, ctx := range g.Contexts {
		_ = ctx.stdin.Close()
		_ = ctx.cmd.Wait()
	}
}

func (g *GccAssembler) Process(tid int, ins string) {
	ctx, ok := g.Contexts[tid]
	if !ok {
		cmd := exec.Command("gcc", "-x", "assembler", "-", "-o", fmt.Sprintf("%d.out", tid))
		pipe, err := cmd.StdinPipe()
		if err != nil {
			panic(err)
		}
		_ = cmd.Start()

		ctx = &GccAssembleContext{
			cmd:   cmd,
			stdin: pipe,
		}
		g.Contexts[tid] = ctx
	}
	_, _ = ctx.stdin.Write(append([]byte(ins), '\n'))
}
