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
	"os/exec"
	"sort"
	"strings"
)

// accessfinderCmd represents the accessfinder command
var accessfinderCmd = &cobra.Command{
	Use:   "accessfinder args",
	Short: "寻找能够访问内存的指令",
	Run: func(cmd *cobra.Command, args []string) {
		perfArgs := []string{"record", "-e", "intel_pt//"}
		perfArgs = append(perfArgs, args...)
		perfCmd := exec.Command("perf", perfArgs...)
		perfCmd.Stdout = os.Stdout
		perfCmd.Stderr = os.Stderr
		fmt.Println("正在执行命令", strings.Join(args, " "))
		_ = perfCmd.Run()
		p := &perf.MemoryAccessInstructionsFinder{TargetInstructions: map[string]struct{}{}}
		processor, err := perf.NewIntelPTProcessor("perf.data", p)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		ctx, cancel := context.WithCancel(context.Background())
		processor.Start(ctx)
		processor.Wait()
		cancel()
		_ = os.Remove("perf.data")

		// 输出
		arr := make([]string, 0, len(p.TargetInstructions))
		for i := range p.TargetInstructions {
			arr = append(arr, i)
		}
		sort.Strings(arr)
		for i := 0; i < len(arr); i++ {
			fmt.Println(arr[i])
		}
	},
}

func init() {
	rootCmd.AddCommand(accessfinderCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// accessfinderCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// accessfinderCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
