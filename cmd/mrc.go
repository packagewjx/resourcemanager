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
	"encoding/csv"
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/algorithm"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"strconv"
)

var (
	precision int
)

// mrcCmd represents the mrc command
var mrcCmd = &cobra.Command{
	Use:   "mrc <rth.csv> <cache size> <out file>",
	Short: "使用RTH Csv文件，使用AET模型，计算MRC，输出到指定CSV",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			return fmt.Errorf("参数数量不对")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := os.Open(args[0])
		if err != nil {
			return errors.Wrap(err, "打开文件出错")
		}
		defer func() {
			_ = f.Close()
		}()
		model, err := algorithm.NewAETModelFromFile(f)
		if err != nil {
			return errors.Wrap(err, "解析RTH文件出错")
		}

		cacheSize, err := strconv.ParseInt(args[1], 10, 32)
		if err != nil {
			return errors.Wrap(err, "解析CacheSize出错")
		}

		mrc := model.MRC(int(cacheSize))

		fout, err := os.Create(args[2])
		if err != nil {
			return errors.Wrap(err, "创建输出文件出错")
		}
		defer func() {
			_ = fout.Close()
		}()
		writer := csv.NewWriter(fout)
		for cacheSize, missRate := range mrc {
			err := writer.Write([]string{strconv.FormatInt(int64(cacheSize), 10),
				strconv.FormatFloat(float64(missRate), 'f', precision, 32)})
			if err != nil {
				return errors.Wrap(err, "打印输出内容出错")
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(mrcCmd)

	mrcCmd.Flags().IntVarP(&precision, "precision", "p", 2, "Miss Rate精度")
}
