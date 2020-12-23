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
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/algorithm"
	"github.com/packagewjx/resourcemanager/internal/pin"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

var sampleMaxRTHTime int

// sampleCmd represents the sample command
var sampleCmd = &cobra.Command{
	Use:   "sample cmd args",
	Short: "运行pin并获取RTH",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			fmt.Println("参数不对")
			os.Exit(1)
		}

		recorder := pin.NewMemRunRecorder(func(tid int) algorithm.RTHCalculator {
			return algorithm.ReservoirCalculator(100000)
		}, pin.DefaultWriteThreshold, pin.DefaultPinBufferSize,
			"/home/wjx/Workspace/pin-3.17/source/tools/MemTrace2/obj-intel64/MemTrace2.so", "sample",
			args[0], args[1:]...)
		fmt.Println("开始采样")
		ch, err := recorder.Start()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		m := <-ch
		fmt.Println("采样结束，正在输出结果")
		args[0] = strings.ReplaceAll(args[0], "/", "_")
		filePrefix := strings.Join(args, "_")
		for tid, calculator := range m {
			calculator.GetRTH(sampleMaxRTHTime)
			outFile, err := os.Create(fmt.Sprintf("%s_%d.csv", filePrefix, tid))
			if err != nil {
				fmt.Println("无法创建输出文件", err)
				os.Exit(1)
			}
			calculator.WriteAsCsv(sampleMaxRTHTime, outFile)
			_ = outFile.Close()
		}
	},
}

func init() {
	rootCmd.AddCommand(sampleCmd)

	sampleCmd.Flags().IntVarP(&sampleMaxRTHTime, "max-time", "t", 100000, "最大RTH时间")
}
