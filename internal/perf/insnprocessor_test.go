package perf

import (
	"bufio"
	"bytes"
	"context"
	"github.com/stretchr/testify/assert"
	"os"
	"os/exec"
	"strconv"
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
	processor := &dummyProcessor{}
	p, _ := NewIntelPTProcessor("perf.data", processor)
	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)
	<-time.After(time.Second)
	cancel()
	assert.NotEqual(t, 0, processor.cnt)
	_ = os.Remove("perf.data")
}

type dummyProcessor struct {
	cnt int
}

func (d *dummyProcessor) Process(_ int, _ string) {
	d.cnt++
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
	reader := bufio.NewReader(bytes.NewReader([]byte(ass)))
	p := &GccAssembler{
		Contexts: map[int]*GccAssembleContext{},
	}
	for line, err := reader.ReadString('\n'); err == nil; line, err = reader.ReadString('\n') {
		p.Process(1, line)
	}
	p.Finish()

	_, err := os.Stat("1.out")
	assert.False(t, os.IsNotExist(err))
	_ = os.Remove("1.out")
}
