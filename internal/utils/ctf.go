package utils

import (
	"bytes"
	"fmt"
	"github.com/pkg/errors"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func ParseCTFTrace(dir string) (map[int][]uint64, error) {
	cmd := exec.Command("babeltrace", dir)
	buf := bytes.NewBuffer(make([]byte, 0, 10240))
	cmd.Stdout = buf
	errCh := make(chan error)
	go func() {
		errCh <- cmd.Run()
	}()
	readCnt := uint64(0)
	ended := false
	go func() {
		tick := time.Tick(time.Second)
		lastCnt := uint64(0)
		for !ended {
			select {
			case <-tick:
				fmt.Printf("速度： %10d/s 已读：%d\n", readCnt-lastCnt, readCnt)
				lastCnt = readCnt
			}
		}
	}()
	result := make(map[int][]uint64)

outerLoop:
	for {
		select {
		case err := <-errCh:
			if err != nil {
				return nil, errors.Wrap(err, "运行babeltrace出错")
			} else {
				fmt.Println("babeltrace退出")
			}
			ended = true
		case <-time.After(500 * time.Millisecond):
			var line string
			var err error
			for line, err = buf.ReadString('\n'); err == nil; line, err = buf.ReadString('\n') {
				if line[:3] != "mem" {
					continue
				}
				readCnt++
				split := strings.Split(line, " ")
				if len(split) < 24 {
					panic("行格式错误：" + line)
				}
				tidString := split[6]
				addrString := split[20][:len(split[20])-1]
				sizeString := split[23]
				addr, err := strconv.ParseUint(addrString, 10, 64)
				if err != nil {
					return nil, errors.Wrap(err, "解析地址出错")
				}
				size, err := strconv.ParseUint(sizeString, 10, 32)
				if err != nil {
					return nil, errors.Wrap(err, "解析Size出错")
				}
				addrEnd := addr + size - 1
				lineCount := int(addr&0xFFFFFFFFFFFFFFC0 - addrEnd&0xFFFFFFFFFFFFFFC0 + 1)
				addrList := make([]uint64, 0)
				for i := 0; i < lineCount; i++ {
					addrList = append(addrList, addr&0xFFFFFFFFFFFFFFC0+uint64(i<<6))
				}
				tid, err := strconv.ParseUint(tidString, 10, 32)
				if err != nil {
					return nil, errors.Wrap(err, "解析Tid出错")
				}
				result[int(tid)] = append(result[int(tid)], addrList...)
			}
			// 必须从这里break，保证最后的数据读取完毕
			if ended {
				break outerLoop
			}
		}
	}
	buf.Reset()
	close(errCh)
	return result, nil
}
