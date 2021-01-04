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
	"github.com/packagewjx/resourcemanager/internal/librm"
	"github.com/packagewjx/resourcemanager/internal/resourcemanager/watcher"
	"github.com/packagewjx/resourcemanager/internal/resourcemonitor"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "启动管控系统",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		return librm.LibInit()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
	PostRun: func(cmd *cobra.Command, args []string) {
		_ = librm.LibFinalize()
	},
}

func init() {
	rootCmd.AddCommand(startCmd)

	startCmd.Flags().StringP("token-file", "t", "",
		"用于访问集群的Service Account Token")
	_ = viper.BindPFlag("kubernetes.tokenfile", startCmd.Flags().Lookup("token-file"))
	startCmd.Flags().StringP("ca-file", "c", "",
		"集群CA文件")
	_ = viper.BindPFlag("kubernetes.cafile", startCmd.Flags().Lookup("ca-file"))
	startCmd.Flags().BoolP("insecure", "n", false,
		"支持TSL不安全连接")
	_ = viper.BindPFlag("kubernetes.insecure", startCmd.Flags().Lookup("insecure"))
	startCmd.Flags().StringP("host", "h", watcher.DefaultHost,
		"Kubernetes API地址")
	_ = viper.BindPFlag("kubernetes.host", startCmd.Flags().Lookup("host"))
	startCmd.Flags().IntP("reservoir-size", "r", resourcemonitor.DefaultReservoirSize,
		"内存使用追踪时Reservoir Sampling方法的Reservoir大小")
	_ = viper.BindPFlag("memtrace.reservoirsize", startCmd.Flags().Lookup("reservoir-size"))
	startCmd.Flags().IntP("max-rth-time", "m", resourcemonitor.DefaultMaxRthTime,
		"将内存使用记录转换为RTH时最大的Reuse Time大小")
	_ = viper.BindPFlag("memtrace.maxrthtime", startCmd.Flags().Lookup("max-rth-time"))
}
