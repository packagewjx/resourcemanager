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
	"github.com/fsnotify/fsnotify"
	"github.com/packagewjx/resourcemanager/internal/core"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"log"
	"os"
)

var configPath string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "resourcemanager",
	Short: "单机资源管控程序",
	Long: `本资源管控程序将对本机上运行的所有容器进行内存带宽和缓存等资源的使用管控。
管控过程完全自动化，通过学习各个应用程序的运行特征，合理分配各项资源，最大程度减少混合部署的容器之间的干扰。
运行本程序有以下要求
1. 需要运行在根名称空间，也就是不能运行在容器中。
2. 需要使用root权限运行。
3. 需要系统内核版本4.18及以上，linux启动参数加入'rdt=mba,cmt,l3cat,mbmlocal,mbmtotal'参数，启动后需挂载resctrl程序。
`}

func init() {
	cobra.OnInitialize(readConfig)

	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "配置文件路径")
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func readConfig() {
	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("/etc/resourcemanager")
	}

	err := viper.ReadInConfig()
	if err != nil {
		log.Println("读取配置出错", err)
	}
	err = viper.UnmarshalExact(core.RootConfig)
	if err != nil {
		log.Println("读取配置出错", err)
	}
	log.Println("读取到配置", core.RootConfig)

	viper.WatchConfig()
	viper.OnConfigChange(func(in fsnotify.Event) {
		if in.Op == fsnotify.Write {
			log.Printf("配置文件已更改，正在重新读取")
			_ = viper.ReadInConfig()
			_ = viper.UnmarshalExact(core.RootConfig)
			log.Println("读取到配置", core.RootConfig)
		}
	})
}
