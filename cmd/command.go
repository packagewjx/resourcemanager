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
	"github.com/packagewjx/resourcemanager/internal/sampler/memrecord"
	"github.com/spf13/cobra"
	"os"
)

// commandCmd represents the command command
var commandCmd = &cobra.Command{
	Use:   "command <cmd> <args>...",
	Short: "运行命令并采样",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("参数错误")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		rq := &memrecord.RunRequest{
			BaseRequest: memrecord.BaseRequest{
				Name: "test",
			},
			Cmd:  args[0],
			Args: args[1:],
		}
		err := executeSampleCommand(rq)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
}

func init() {
	sampleCmd.AddCommand(commandCmd)
}
