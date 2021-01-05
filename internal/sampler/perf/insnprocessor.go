package perf

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type IntelPTProcessor interface {
	Start(ctx context.Context)
	GetInstructionProcessCount() uint64
	Wait() // 处理是异步的，使用本函数将会等待处理结束
}

type InstructionProcessor interface {
	// 处理1条指令
	// tid为线程号，ins为具体指令，cnt为第几个指令
	Process(ins string) string

	// 结束处理，进行收尾工作
	Finish()
}

type InstructionProcessorBuilder func(tid int) InstructionProcessor

func NewIntelPTProcessor(perfDataFile string, builder InstructionProcessorBuilder) (IntelPTProcessor, error) {
	_, err := os.Stat(perfDataFile)
	if os.IsNotExist(err) {
		return nil, err
	}
	cmd := exec.Command("perf", "script", "--insn-trace", "-F", "tid,insn", "--xed", "-i", perfDataFile)
	return &ptImpl{
		cnt:        0,
		cmd:        cmd,
		builder:    builder,
		processors: map[int]InstructionProcessor{},
		wg:         sync.WaitGroup{},
	}, nil
}

type ptImpl struct {
	cnt        uint64
	cmd        *exec.Cmd
	builder    InstructionProcessorBuilder
	processors map[int]InstructionProcessor
	wg         sync.WaitGroup
}

func (p *ptImpl) GetInstructionProcessCount() uint64 {
	return p.cnt
}

func (p *ptImpl) Wait() {
	p.wg.Wait()
}

func (p *ptImpl) Start(ctx context.Context) {
	p.wg.Add(1)
	p.cnt = 0
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
		err = p.cmd.Start()
		if err != nil {
			fmt.Println("启动perf出错", err)
			return
		}
		tidMap := make(map[string]int)
	outerLoop:
		for line, err := reader.ReadString('\n'); err == nil || (err == io.EOF && line != ""); line, err = reader.ReadString('\n') {
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
				ip, ok := p.processors[tid]
				if !ok {
					ip = p.builder(tid)
					p.processors[tid] = ip
				}
				ip.Process(arr[2])
				p.cnt++
			}
		}
		_ = pipe.Close()
		_ = p.cmd.Wait()
		for _, processor := range p.processors {
			processor.Finish()
		}
	}()
}

type InstructionProcessorChain struct {
	Processor InstructionProcessor
	Next      *InstructionProcessorChain
}

func (i InstructionProcessorChain) Process(ins string) string {
	ins = i.Processor.Process(ins)
	if i.Next != nil {
		ins = i.Next.Processor.Process(ins)
	}
	return ins
}

func (i InstructionProcessorChain) Finish() {
	i.Processor.Finish()
	if i.Next != nil {
		i.Next.Finish()
	}
}

func NewInstructionProcessorChain(ipList ...InstructionProcessor) InstructionProcessor {
	if len(ipList) == 0 {
		return nil
	}

	last := &InstructionProcessorChain{
		Processor: ipList[0],
		Next:      nil,
	}
	first := last
	for i := 1; i < len(ipList); i++ {
		curr := &InstructionProcessorChain{
			Processor: ipList[i],
			Next:      nil,
		}
		last.Next = curr
		last = curr
	}
	return first
}

type InstructionPreprocessor struct {
}

var operandStartPattern = regexp.MustCompile(" +[$%(\\-0-9]")

func (i InstructionPreprocessor) Process(ins string) string {
	operation := extractOperationFromInstruction(ins)
	switch operation {

	case "nop", "nopl", "nopw", // Nop指令
		"jmp", "jz", "jne", "jmpq", "jbe", "bnd jmp", "jnbe", "jnz", "jnb", "jb", "jle", "jnle",
		"js", "jns", "jl", "jnl", "jo", "call", "callq": // 跳转指令
		ins = ""
	case "movqq", "movlpdq", "movhpdq", "movhpsq", "movdqax", "movdqux", "movupsx", "movapsx", "vmovdqay", "vmovdquy",
		"pcmpeqbx", "stosqq", "rep stosqq", "paddqx", "pcmpistrix", "palignrx", "vpcmpeqby", "vmovdqux":
		// 去掉错误的后缀
		ins = ins[:len(operation)-1] + ins[len(operation):]
	}
	ins = strings.ReplaceAll(ins, ":,", ":0,")
	return ins
}

func extractOperationFromInstruction(ins string) string {
	stringIndex := operandStartPattern.FindStringIndex(ins)
	if len(stringIndex) == 0 {
		stringIndex = []int{len(ins)}
	}
	return ins[:stringIndex[0]]
}

func (i InstructionPreprocessor) Finish() {}

type GccAssembler struct {
	stdin io.WriteCloser
	cmd   *exec.Cmd
}

const gccStart = `	.file	"asm.s"
	.text
	.globl	main
	.type	main, @function
main:
	.cfi_startproc
`

const gccEnd = `.cfi_endproc
`

func NewGccAssembler(tid int) InstructionProcessor {
	cmd := exec.Command("gcc", "-x", "assembler", "-march=native", "-g", "-o", fmt.Sprintf("%d.out", tid), "-")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	pipe, err := cmd.StdinPipe()
	if err != nil {
		panic(err)
	}
	_ = cmd.Start()
	_, _ = pipe.Write([]byte(gccStart))
	return &GccAssembler{
		stdin: pipe,
		cmd:   cmd,
	}
}

func (g *GccAssembler) Process(ins string) string {
	_, _ = g.stdin.Write(append([]byte(ins), '\n'))
	return ins
}

func (g *GccAssembler) Finish() {
	_, _ = g.stdin.Write([]byte(gccEnd))
	_ = g.stdin.Close()
	_ = g.cmd.Wait()
}

type InstructionWriter struct {
	closer    io.Closer
	bufWriter *bufio.Writer
}

func NewInstructionWriter(tid int) InstructionProcessor {
	f, _ := os.Create(fmt.Sprintf("%d.run.s", tid))
	return &InstructionWriter{closer: f, bufWriter: bufio.NewWriter(f)}
}

func (i *InstructionWriter) Process(ins string) string {
	_, _ = i.bufWriter.WriteString(ins + "\n")
	return ins
}

func (i *InstructionWriter) Finish() {
	_ = i.bufWriter.Flush()
	_ = i.closer.Close()
}

type InstructionFinder struct {
	Map map[string]struct{}
}

func (i *InstructionFinder) Process(ins string) string {
	operation := extractOperationFromInstruction(ins)
	i.Map[operation] = struct{}{}
	return ins
}

func (i *InstructionFinder) Finish() {

}
