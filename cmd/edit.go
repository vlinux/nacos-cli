// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Yongheng Liu

package cmd

import (
	"github.com/spf13/cobra"
)

// editCmd represents the edit command
var editCmd = &cobra.Command{
	Use:     "edit",
	Short:   "编辑 Nacos 上的资源",
	Long:    `编辑 Nacos 上的资源。目前支持 config。`,
	Example: "nacos-cli edit config app.yaml",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
	ValidArgs: []string{"config"},
}

func init() {
	rootCmd.AddCommand(editCmd)
}
