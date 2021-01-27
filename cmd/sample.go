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
	"bufio"
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
	sampleCmd.PersistentFlags().IntP("buffer-size", "b", core.RootConfig.MemTrace.BufferSize,
		"pin缓冲大小")
	_ = viper.BindPFlag("memtrace.buffersize", sampleCmd.PersistentFlags().Lookup("buffer-size"))
	sampleCmd.PersistentFlags().IntP("write-threshold", "w", core.RootConfig.MemTrace.WriteThreshold,
		"消费数据阈值")
	_ = viper.BindPFlag("memtrace.writethreshold", sampleCmd.PersistentFlags().Lookup("write-threshold"))
	sampleCmd.PersistentFlags().IntP("stop-at", "s", core.RootConfig.MemTrace.TraceCount,
		"采集内存数据总数")
	_ = viper.BindPFlag("memtrace.tracecount", sampleCmd.PersistentFlags().Lookup("stop-at"))

	sampleCmd.PersistentFlags().BoolVarP(&useShenModel, "useShenModel", "d", false, "")
}

func executeSampleCommand(rq interface{}) error {
	recorder, err := memrecord.NewPinMemRecorder(&memrecord.Config{
		BufferSize:     core.RootConfig.MemTrace.BufferSize,
		WriteThreshold: core.RootConfig.MemTrace.WriteThreshold,
		PinToolPath:    core.RootConfig.MemTrace.PinToolPath,
		ConcurrentMax:  core.RootConfig.MemTrace.ConcurrentMax,
		TraceCount:     core.RootConfig.MemTrace.TraceCount,
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	var consumer memrecord.CacheLineAddressConsumer
	if useShenModel {
		consumer = memrecord.NewShenModelConsumer(core.RootConfig.MemTrace.MaxRthTime)
	} else {
		consumer = memrecord.NewRTHCalculatorConsumer(memrecord.GetCalculatorFromRootConfig())
	}

	ctx, cancel := context.WithCancel(context.Background())
	// 注册信号处理
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	var ch <-chan *memrecord.MemRecordResult
	switch v := rq.(type) {
	case *memrecord.MemRecordAttachRequest:
		v.Consumer = consumer
		ch = recorder.RecordProcess(ctx, v)
	case *memrecord.MemRecordRunRequest:
		v.Consumer = consumer
		ch = recorder.RecordCommand(ctx, v)
	default:
		panic("错误类型")
	}

	m := <-ch
	if m.Err != nil {
		return m.Err
	}

	if useShenModel {
		return shenOutput(consumer.(memrecord.ShenModelConsumer))
	} else {
		return rthOutput(consumer.(memrecord.RTHCalculatorConsumer), m)
	}
}

func rthOutput(consumer memrecord.RTHCalculatorConsumer, m *memrecord.MemRecordResult) error {
	threadTrace := consumer.GetCalculatorMap()
	for tid, calculator := range threadTrace {
		outFile, err := os.Create(fmt.Sprintf("sample_%d.rth.csv", tid))
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

func shenOutput(consumer memrecord.ShenModelConsumer) error {
	histogram := consumer.GetReuseTimeHistogram()
	for tid, rdh := range histogram {
		outFile, err := os.Create(fmt.Sprintf("sample_%d.rdh.csv", tid))
		if err != nil {
			return errors.Wrap(err, "无法创建输出文件")
		}
		writer := bufio.NewWriter(outFile)
		for d, p := range rdh {
			_, _ = writer.WriteString(fmt.Sprintf("%d, %.20f\n", d, p))
		}
		_ = writer.Flush()
		_ = outFile.Close()
	}
	return nil
}
