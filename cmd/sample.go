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
	"github.com/packagewjx/resourcemanager/internal/algorithm"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/resourcemanager"
	"github.com/packagewjx/resourcemanager/internal/sampler/memrecord"
	"github.com/packagewjx/resourcemanager/internal/utils"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"os/signal"
	"syscall"
)

var useShenModel bool

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
	sampleCmd.PersistentFlags().IntP("buffer-size", "b", core.RootConfig.MemTrace.PinConfig.BufferSize,
		"pin缓冲大小")
	_ = viper.BindPFlag("memtrace.buffersize", sampleCmd.PersistentFlags().Lookup("buffer-size"))
	sampleCmd.PersistentFlags().IntP("write-threshold", "w", core.RootConfig.MemTrace.PinConfig.WriteThreshold,
		"消费数据阈值")
	_ = viper.BindPFlag("memtrace.writethreshold", sampleCmd.PersistentFlags().Lookup("write-threshold"))
	sampleCmd.PersistentFlags().IntP("stop-at", "s", core.RootConfig.MemTrace.TraceCount,
		"采集内存数据总数")
	_ = viper.BindPFlag("memtrace.tracecount", sampleCmd.PersistentFlags().Lookup("stop-at"))

	sampleCmd.PersistentFlags().BoolVarP(&useShenModel, "useShenModel", "d", false, "")
}

func executeSampleCommand(rq interface{}) error {
	var recorder memrecord.MemRecorder
	var err error
	memTraceConfig := core.RootConfig.MemTrace
	if memTraceConfig.Sampler == core.MemTraceSamplerPerf {
		recorder, err = memrecord.NewPerfRecorder(memTraceConfig.PerfRecordConfig.OverflowCount,
			memTraceConfig.PerfRecordConfig.SwitchOutput,
			memTraceConfig.PerfRecordConfig.PerfExecPath)
	} else {
		recorder, err = memrecord.NewPinMemRecorder(&memrecord.Config{
			BufferSize:     memTraceConfig.PinConfig.BufferSize,
			WriteThreshold: memTraceConfig.PinConfig.WriteThreshold,
			PinToolPath:    memTraceConfig.PinConfig.PinToolPath,
			ConcurrentMax:  memTraceConfig.ConcurrentMax,
			TraceCount:     memTraceConfig.TraceCount,
		})
	}
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	var consumer memrecord.CacheLineAddressConsumer
	consumer = memrecord.NewRTHCalculatorConsumer(memrecord.GetCalculatorFromRootConfig())

	ctx, cancel := context.WithCancel(context.Background())
	// 注册信号处理
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	var ch <-chan *memrecord.Result
	switch v := rq.(type) {
	case *memrecord.AttachRequest:
		v.Consumer = consumer
		ch, _ = recorder.RecordProcess(ctx, v)
	case *memrecord.RunRequest:
		v.Consumer = consumer
		ch, _ = recorder.RecordCommand(ctx, v)
	default:
		panic("错误类型")
	}

	m := <-ch
	if m.Err != nil {
		return m.Err
	}

	return rthOutput(consumer.(memrecord.RTHCalculatorConsumer), m)
}

func rthOutput(consumer memrecord.RTHCalculatorConsumer, m *memrecord.Result) error {
	threadTrace := consumer.GetCalculatorMap()
	for tid, calculator := range threadTrace {
		outFile, err := os.Create(fmt.Sprintf("sample_%d.mcf.rth.csv", tid))
		if err != nil {
			return errors.Wrap(err, "无法创建输出文件")
		}
		algorithm.WriteAsCsv(calculator.GetRTH(core.RootConfig.MemTrace.MaxRthTime), outFile)
		_ = outFile.Close()
	}
	// 输出加权平均MRC
	numWays, numSets, _ := utils.GetL3Cap()
	mrc := resourcemanager.WeightedAverageMRC(threadTrace, m.ThreadInstructionCount, m.TotalInstructions,
		core.RootConfig.MemTrace.MaxRthTime, numWays*numSets*2)
	outFile, err := os.Create("sample_weighted_mrc.csv")
	if err != nil {
		return errors.Wrap(err, "无法创建输出文件")
	}
	for c, miss := range mrc {
		_, _ = fmt.Fprintf(outFile, "%d,%.4f\n", c, miss)
	}
	_ = outFile.Close()
	return nil
}
