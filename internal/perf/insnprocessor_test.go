package perf

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestProcess(t *testing.T) {
	cmd := exec.Command("perf", "record", "-e", "intel_pt//", "-p", strconv.FormatInt(int64(os.Getpid()), 10))
	_ = cmd.Start()
	<-time.After(time.Second)
	_ = cmd.Process.Signal(syscall.SIGINT)
	_ = cmd.Wait()
	p, _ := NewIntelPTProcessor("perf.data", func(tid int) InstructionProcessor {
		return &dummyProcessor{}
	})
	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)
	<-time.After(time.Second)
	cancel()
	assert.NotEqual(t, 0, p.GetInstructionProcessCount())
	_ = os.Remove("perf.data")
}

type dummyProcessor struct {
	cnt int
}

func (d *dummyProcessor) Process(ins string) string {
	d.cnt++
	return ins
}

func (d *dummyProcessor) Finish() {

}

func TestGccAssemble(t *testing.T) {
	ass := `	.file	"noploop.c"
	.text
	.globl	main
	.type	main, @function
main:
.LFB0:
	.cfi_startproc
	movl	$1000000000, %eax
.L2:
	subq	$1, %rax
	jne	.L2
	movl	$0, %eax
	ret
	.cfi_endproc
.LFE0:
	.size	main, .-main
	.ident	"GCC: (Ubuntu 7.5.0-3ubuntu1~18.04) 7.5.0"
	.section	.note.GNU-stack,"",@progbits
`
	ass = "stosqq (%rdi)\n"
	reader := bufio.NewReader(bytes.NewReader([]byte(ass)))
	p := NewInstructionProcessorChain(InstructionPreprocessor{}, NewGccAssembler(1))
	for line, err := reader.ReadString('\n'); err == nil || (err == io.EOF && line != ""); line, err = reader.ReadString('\n') {
		p.Process(line)
	}
	p.Finish()

	_, err := os.Stat("1.out")
	assert.False(t, os.IsNotExist(err))
	_ = os.Remove("1.out")
}

func TestAssemblePerfData(t *testing.T) {
	processor, _ := NewIntelPTProcessor("../../perf.data", func(tid int) InstructionProcessor {
		return NewInstructionProcessorChain(InstructionPreprocessor{}, NewGccAssembler(tid))
	})
	ctx, cancel := context.WithCancel(context.Background())
	processor.Start(ctx)
	tick := time.Tick(time.Second)
	for i := 0; i < 3; i++ {
		<-tick
		fmt.Println(processor.GetInstructionProcessCount())
	}
	cancel()
	processor.Wait()
}

// 用于处理gcc输出，获取编译错误的指令的
func TestOut(t *testing.T) {
	open, _ := os.Open("../../log")
	reader := bufio.NewReader(open)
	errorMap := map[string]string{}
	for line, err := reader.ReadString('\n'); err == nil; line, err = reader.ReadString('\n') {
		idx := strings.Index(line, "Error")
		if idx == -1 {
			continue
		}
		original := line
		line = line[idx:]
		idx = strings.Index(line, "`")
		stopIdx := strings.Index(line[idx:], " ")
		if stopIdx == -1 {
			stopIdx = strings.Index(line[idx:], "'")
		}
		ins := line[idx+1 : stopIdx+idx]
		if _, ok := errorMap[ins]; ok {
			continue
		}
		errorMap[ins] = original
	}
	for _, line := range errorMap {
		fmt.Println(line)
	}
}
