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
	"os"
	"os/exec"
	"sort"

	"github.com/spf13/cobra"
)

// ifinderCmd represents the ifinder command
var ifinderCmd = &cobra.Command{
	Use:   "ifinder command args",
	Short: "执行一条命令，获取其perf record，然后获取里面涉及的指令列表",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			fmt.Println("命令为空，返回")
			os.Exit(1)
		}

		const outPerfData = "ifinder.perf.data"
		perfCmd := exec.Command("perf", append([]string{"record", "-o", outPerfData, "-e", "intel_pt//"}, args...)...)
		perfCmd.Stdout = os.Stdout
		perfCmd.Stderr = os.Stderr
		fmt.Println("开始录制")
		err := perfCmd.Run()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		_, err = os.Stat(outPerfData)
		if os.IsNotExist(err) {
			fmt.Println("录制失败")
			os.Exit(1)
		}

		fmt.Println("录制完成，开始分析")
		finder := &perf.InstructionFinder{Map: map[string]struct{}{}}
		processData(func(_ int) perf.InstructionProcessor {
			return finder
		})(cmd, []string{outPerfData})

		insList := make([]string, 0, len(finder.Map))
		for ins := range finder.Map {
			insList = append(insList, ins)
		}
		sort.Strings(insList)
		fmt.Println()
		for _, ins := range insList {
			fmt.Println(ins)
		}

		_ = os.Remove(outPerfData)
	},
}

func init() {
	rootCmd.AddCommand(ifinderCmd)
}
