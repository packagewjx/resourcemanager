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
	"github.com/packagewjx/resourcemanager/internal/sampler/pin"
	"github.com/pkg/errors"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

var attachPid int

// attachCmd represents the process command
var attachCmd = &cobra.Command{
	Use:   "attach <pid>",
	Short: "对运行中进程进行采样",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		pid, err := strconv.ParseInt(args[0], 10, 32)
		if err != nil {
			return errors.Wrap(err, "解析Pid参数出错")
		}
		attachPid = int(pid)
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		memRecorder, err := pin.NewMemRecorder(&pin.Config{
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
		ctx, cancel := context.WithCancel(context.Background())
		resCh := memRecorder.RecordProcess(ctx, &pin.MemRecordAttachRequest{
			MemRecordBaseRequest: pin.MemRecordBaseRequest{
				Factory: pin.GetCalculatorFromRootConfig(),
				Name:    "sample",
			},
			Pid: attachPid,
		})
		receiveResult(resCh, cancel)
	},
}

func init() {
	sampleCmd.AddCommand(attachCmd)
}
