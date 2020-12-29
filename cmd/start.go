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
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/packagewjx/resourcemanager/internal/resourcemanager"
	"github.com/packagewjx/resourcemanager/internal/resourcemonitor"
	"github.com/spf13/cobra"
)

var (
	interval      int
	tokenFile     string
	caFile        string
	insecure      bool
	host          string
	reservoirSize int
	maxRthTime    int
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "启动管控系统",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if tokenFile == "" {
			return fmt.Errorf("请提供Token")
		}
		if !insecure && caFile == "" {
			return fmt.Errorf("请提供CA，或者设置不安全的连接")
		}
		if interval < 1000 {
			return fmt.Errorf("取样间隔不能低于1000毫秒")
		}

		return core.LibInit()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
	PostRun: func(cmd *cobra.Command, args []string) {
		_ = core.LibFinalize()
	},
}

func init() {
	rootCmd.AddCommand(startCmd)

	startCmd.Flags().StringVarP(&tokenFile, "token-file", "t", "",
		"用于访问集群的Service Account Token")
	startCmd.Flags().StringVarP(&caFile, "ca-file", "c", "",
		"集群CA文件")
	startCmd.Flags().BoolVarP(&insecure, "insecure", "n", false,
		"支持TSL不安全连接")
	startCmd.Flags().StringVarP(&host, "host", "h", resourcemanager.DefaultHost,
		"Kubernetes API地址")
	startCmd.Flags().IntVarP(&reservoirSize, "reservoir-size", "r", resourcemonitor.DefaultReservoirSize,
		"内存使用追踪时Reservoir Sampling方法的Reservoir大小")
	startCmd.Flags().IntVarP(&maxRthTime, "max-rth-time", "m", resourcemonitor.DefaultMaxRthTime,
		"将内存使用记录转换为RTH时最大的Reuse Time大小")
}
