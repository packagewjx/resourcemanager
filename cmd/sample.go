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
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/pin"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
)

// sampleCmd represents the sample command
var sampleCmd = &cobra.Command{
	Use:   "sample cmd args",
	Short: "运行pin并获取RTH。有两种采集模式，看子命令。",
}

func init() {
	rootCmd.AddCommand(sampleCmd)

	sampleCmd.PersistentFlags().IntP("max-time", "t", core.RootConfig.MemTrace.MaxRthTime,
		"最大RTH时间")
	_ = viper.BindPFlag("memtrace.maxrthtime", sampleCmd.PersistentFlags().Lookup("max-time"))
	sampleCmd.PersistentFlags().IntP("buffer-size", "b", core.RootConfig.MemTrace.BufferSize,
		"pin缓冲大小")
	_ = viper.BindPFlag("memtrace.buffersize", sampleCmd.PersistentFlags().Lookup("buffer-size"))
	sampleCmd.PersistentFlags().IntP("write-threshold", "w", core.RootConfig.MemTrace.WriteThreshold,
		"消费数据阈值")
	_ = viper.BindPFlag("memtrace.writethreshold", sampleCmd.PersistentFlags().Lookup("write-threshold"))
	sampleCmd.PersistentFlags().IntP("stop-at", "s", core.RootConfig.MemTrace.TraceCount,
		"采集内存数据总数")
	_ = viper.BindPFlag("memtrace.tracecount", sampleCmd.PersistentFlags().Lookup("stop-at"))
}

func sampleCommandExecute(recorder pin.MemRecorder) {
	fmt.Println("开始采样")
	ch, err := recorder.Start(context.Background())
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	m := <-ch
	fmt.Println("采样结束，正在输出结果")
	for tid, calculator := range m {
		outFile, err := os.Create(fmt.Sprintf("sample_%d.csv", tid))
		if err != nil {
			fmt.Println("无法创建输出文件", err)
			os.Exit(1)
		}
		calculator.WriteAsCsv(core.RootConfig.MemTrace.MaxRthTime, outFile)
		_ = outFile.Close()
	}
}
