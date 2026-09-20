/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "删除 Nacos 上的资源",
	Long: `删除 Nacos 上的资源。目前支持 config。

  nacos-cli delete config app.yaml`,
	Example: "nacos-cli delete config app.yaml",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
	ValidArgs: []string{"config"},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}
