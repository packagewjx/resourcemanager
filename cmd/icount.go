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
	"github.com/packagewjx/resourcemanager/internal/perf"
	"github.com/spf13/cobra"
	"os"
)

// icountCmd represents the icount command
var icountCmd = &cobra.Command{
	Use:   "icount <perf.data> <out csv>",
	Short: "统计指令数量",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 2 {
			fmt.Println("参数错误")
			os.Exit(1)
		}
		instructions, err := perf.CountInstructions(args[0])
		if err != nil {
			fmt.Println(err.Error())
		}
		f, err := os.Create(args[1])
		if err != nil {
			fmt.Println("创建文件失败")
			os.Exit(1)
		}
		perf.WriteInstructionCount(f, instructions)
		_ = f.Close()
	},
}

func init() {
	rootCmd.AddCommand(icountCmd)
}
