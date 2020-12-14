package perf

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var counting = true

func CountInstructions(dataFile string) (map[string]int, error) {
	cmd := exec.Command("perf", "script", "--insn-trace", "-F", "insn", "--xed", "-i", dataFile)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Wrap(err, "设置输出出错")
	}
	err = cmd.Start()
	if err != nil {
		return nil, errors.Wrap(err, "启动Perf失败")
	}
	reader := bufio.NewReader(stdout)
	result := make(map[string]int)
	lineCount := 0
	go func() {
		ch := make(chan os.Signal)
		signal.Notify(ch, syscall.SIGINT)
		tick := time.Tick(time.Second)
		for {
			select {
			case <-ch:
				fmt.Println("接收到中断信号")
				counting = false
				_ = cmd.Process.Kill()
				return
			case <-tick:
				fmt.Printf("\r已处理%d条指令", lineCount)
			}
		}
	}()
	var line string
	for line, err = reader.ReadString('\n'); err == nil && counting; line, err = reader.ReadString('\n') {
		lineCount++
		line = strings.TrimSpace(line)
		idxSpace := strings.Index(line, " ")
		if idxSpace != -1 {
			line = line[:idxSpace]
		}
		result[line]++
	}
	_ = stdout.Close()

	if !counting {
		err = fmt.Errorf("统计过程被中断")
	} else {
		err = nil
	}
	_ = cmd.Wait()

	return result, err
}

func WriteInstructionCount(out io.Writer, countMap map[string]int) {
	writer := csv.NewWriter(out)
	keys := make([]string, 0, len(countMap))
	for i := range countMap {
		keys = append(keys, i)
	}
	sort.Strings(keys)
	for _, insn := range keys {
		_ = writer.Write([]string{insn, strconv.FormatInt(int64(countMap[insn]), 10)})
	}
	writer.Flush()
}

func MergeInstructions(countMap map[string]int) {

}
