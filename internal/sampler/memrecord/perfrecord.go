package memrecord

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

type PerfRecorder struct {
}

func parseResult(fileName string, consumer CacheLineAddressConsumer) *MemRecordResult {
	perfMemCmd := exec.Command("perf", "mem", "report", "-x", ",", "-i", fileName, "-D")
	buf := bytes.NewBuffer(make([]byte, 0, 10240))
	perfMemCmd.Stdout = buf
	perfMemCmd.Stderr = buf
	err := perfMemCmd.Start()
	if err != nil {
		panic(err)
	}
	for buf.Len() == 0 {
	}
	reader := bufio.NewReader(buf)
	line, err := reader.ReadString('\n')
	res := &MemRecordResult{
		ThreadInstructionCount: map[int]uint64{},
		TotalInstructions:      0,
		Err:                    nil,
	}
	for line, err = reader.ReadString('\n'); err == nil || (err == io.EOF && line != ""); line, err = reader.ReadString('\n') {
		if line[0] == '#' {
			log.Println("注释行", line)
			continue
		}
		record := strings.Split(line, ",")
		if len(record) != 7 {
			log.Println("异常行", line)
			continue
		}
		res.TotalInstructions++
		tid, _ := strconv.ParseInt(record[1], 10, 32)
		addr, _ := strconv.ParseUint(record[3], 0, 64)
		addr &= 0xFFFFFFFFFFFFFFC0
		res.ThreadInstructionCount[int(tid)]++
		consumer.Consume(int(tid), []uint64{addr})
	}
	if err != nil && err != io.EOF {
		res.Err = err
	}
	_ = perfMemCmd.Wait()
	return res
}

func (p *PerfRecorder) record(ctx context.Context, perfCommand *exec.Cmd, consumer CacheLineAddressConsumer) <-chan *MemRecordResult {
	resCh := make(chan *MemRecordResult)
	go func(resCh chan *MemRecordResult) {
		perfCommand.Stdout = os.Stdout
		perfCommand.Stderr = os.Stderr
		doneCh := make(chan struct{})
		go func() {
			err := perfCommand.Run()
			if err != nil {
				log.Println("perf退出错误", err)
			} else {
				log.Println("perf正常退出")
			}
			doneCh <- struct{}{}
		}()
		select {
		case <-doneCh:
		case <-ctx.Done():
			log.Println("提前结束")
			_ = syscall.Kill(perfCommand.Process.Pid, syscall.SIGINT)
			<-doneCh
		}

		log.Println("正在解析结果")
		resCh <- parseResult("perf.data", consumer)
	}(resCh)
	return resCh
}

func (p *PerfRecorder) RecordCommand(ctx context.Context, request *MemRecordRunRequest) <-chan *MemRecordResult {
	args := []string{"record", "-e", "mem_inst_retired.all_loads:P,mem_inst_retired.all_stores:P", "-d", "-c", "20", "--"}
	args = append(args, request.Cmd)
	args = append(args, request.Args...)
	cmd := exec.Command("perf", args...)
	return p.record(ctx, cmd, request.Consumer)
}

func (p *PerfRecorder) RecordProcess(ctx context.Context, request *MemRecordAttachRequest) <-chan *MemRecordResult {
	cmd := exec.Command("perf", "record", "-e", "mem_inst_retired.all_loads:P,mem_inst_retired.all_stores:P",
		"-d", "-c", "3", "-p", strconv.FormatInt(int64(request.Pid), 10))
	return p.record(ctx, cmd, request.Consumer)
}
