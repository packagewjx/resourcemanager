/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"context"
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/perf"
	"github.com/spf13/cobra"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// assembleptCmd represents the assemblept command
var assembleptCmd = &cobra.Command{
	Use:   "assemblept <perf.data>",
	Short: "将Perf Intel PT编译为可执行文件",
	Long:  `perf.data必须使用'perf record -e intel_pt'命令录制。命令将会一直编译，编译时间可能会很长，使用Ctrl + C指令可以中断`,
	Run: processData(func(tid int) perf.InstructionProcessor {
		return perf.NewInstructionProcessorChain(perf.InstructionPreprocessor{}, perf.NewGccAssembler(tid))
	}),
}

var processptCmd = &cobra.Command{
	Use:   "processpt <perf.data>",
	Short: "处理Intel PT，输出为可被gcc编译的文件",
	Run: processData(func(tid int) perf.InstructionProcessor {
		return perf.NewInstructionProcessorChain(perf.InstructionPreprocessor{}, perf.NewInstructionWriter(tid))
	}),
}

func init() {
	rootCmd.AddCommand(assembleptCmd)
	rootCmd.AddCommand(processptCmd)
}

func processData(builder perf.InstructionProcessorBuilder) func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		if len(args) != 1 {
			fmt.Println("参数错误")
			os.Exit(1)
		}
		processor, err := perf.NewIntelPTProcessor(args[0], builder)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		ctx, cancel := context.WithCancel(context.Background())
		processor.Start(ctx)
		sigCh := make(chan os.Signal)
		signal.Notify(sigCh, syscall.SIGINT)
		tick := time.Tick(time.Second)
		finishCh := make(chan struct{})
		go func() {
			processor.Wait()
			finishCh <- struct{}{}
		}()
		lastCnt := uint64(0)
	outerLoop:
		for {
			select {
			case <-sigCh:
				fmt.Println("接收到中断，正在退出")
				break outerLoop
			case <-finishCh:
				break outerLoop
			case <-tick:
				currCnt := processor.GetInstructionProcessCount()
				fmt.Printf("\r处理速度：%8d/s  已处理行数：%d", currCnt-lastCnt, currCnt)
				lastCnt = currCnt
			}
		}
		cancel()
		processor.Wait()
	}
}
