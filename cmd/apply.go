/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github/szpinc/nacosctl/pkg/nacos"

	"github.com/spf13/cobra"
)

var (
	file   string
	dataId string
)

// applyCmd represents the apply command
var applyCmd = &cobra.Command{
	Use:   "apply -f <file>",
	Short: "从本地文件发布或更新配置",
	Long: `读取本地文件并发布到 Nacos（已存在则覆盖）。

dataId 默认取文件名，也可以用 --id 指定；
配置类型默认按文件后缀推断，也可以用 --type 指定。`,
	Example: `  # dataId 取文件名 app.yaml
  nacos-cli apply -f app.yaml

  # 指定 dataId 与分组
  nacos-cli apply -f /path/to/app.yaml --id app.yaml -g MY_GROUP

  # 指定命名空间
  nacos-cli apply -f app.yaml -n dev`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return nacosClient.ApplyConfig(nacos.ConfigApplyOperation{
			NacosOperation: &nacos.NacosOperation{
				Namespace: namespace,
				Group:     group,
			},
			DataId: dataId,
			File:   file,
			Type:   fileType,
		})
	},
}

func init() {

	applyCmd.Flags().StringVarP(&file, "file", "f", "", "要发布的本地文件（必填）")
	applyCmd.Flags().StringVarP(&dataId, "id", "d", "", "dataId，默认取文件名")
	applyCmd.Flags().StringVarP(&fileType, "type", "t", "", "配置类型，默认按文件后缀推断，如 yaml")

	_ = applyCmd.MarkFlagRequired("file")

	rootCmd.AddCommand(applyCmd)
}
