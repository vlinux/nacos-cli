// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Yongheng Liu

package cmd

import (
	"github.com/spf13/cobra"
)

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "从 Nacos 读取资源",
	Long: `从 Nacos 读取资源。

  nacos-cli get config              列出配置
  nacos-cli get config <dataId>     读取单条配置内容`,
	Example: "nacos-cli get config app.yaml",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
	// ValidArgs: []string{"config"},
}

func init() {
	rootCmd.AddCommand(getCmd)
}
