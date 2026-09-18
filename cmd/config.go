/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github/szpinc/nacosctl/pkg/editor"
	"github/szpinc/nacosctl/pkg/nacos"
	"github/szpinc/nacosctl/pkg/util"

	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
)

var (
	getAllConfig bool   // 获取所有配置
	fileType     string // 配置类型
)

var getConfig = &cobra.Command{
	Use:   "config [dataId]",
	Short: "nacos config",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {

		if getAllConfig {
			// -A 默认跨分组列出全部；只有用户显式传了 -g 才按分组过滤
			listGroup := ""
			if cmd.Flags().Changed("group") {
				listGroup = group
			}

			items, err := nacosClient.AllConfig(nacos.ConfigGetOperation{
				NacosOperation: &nacos.NacosOperation{
					Namespace: namespace,
					Group:     listGroup,
				},
			})
			if err != nil {
				return err
			}

			printTable(items)
			return nil
		}

		if len(args) == 0 {
			return errors.New("data id required")
		}

		dataId := args[0]

		configData, err := nacosClient.Get(nacos.ConfigGetOperation{
			NacosOperation: &nacos.NacosOperation{
				Namespace: namespace,
				Group:     group,
			},
			DataId: dataId,
		})
		if err != nil {
			return err
		}

		fmt.Println(configData.Content)
		return nil
	},
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if nacosClient == nil {
			if err := initClient(cmd); err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
		}

		items, err := nacosClient.AllConfig(nacos.ConfigGetOperation{
			NacosOperation: &nacos.NacosOperation{
				Namespace: namespace,
			},
		})
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		names := make([]string, 0, len(items))
		for _, item := range items {
			names = append(names, item.DataId)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	},
}

var editConfig = &cobra.Command{
	Use:   "config <dataId>",
	Short: "nacos config",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {

		if len(args) == 0 {
			return errors.New("data id required")
		}

		var dataId = args[0]

		configData, err := nacosClient.Get(nacos.ConfigGetOperation{
			NacosOperation: &nacos.NacosOperation{
				Namespace: namespace,
				Group:     group,
			},
			DataId: dataId,
		})
		if err != nil {
			return err
		}

		e := editor.NewDefaultEditor([]string{})

		buf := &bytes.Buffer{}
		buf.Write([]byte(configData.Content))

		edited, file, err := e.LaunchTempFile(fmt.Sprintf("%s-edit-", filepath.Base(os.Args[0])), configData.Type, buf)
		if err != nil {
			return err
		}

		editedMd5 := util.Md5BytesToString(edited)

		if configData.Md5 == editedMd5 {
			fmt.Println("Not Changed")
			return nil
		}

		defer func(f string) {
			if e := os.Remove(f); e != nil {
				fmt.Println("delete temp file error:", e)
			}
		}(file)

		if fileType == "" {
			fileType = configData.Type
		}

		if err = nacosClient.Edit(nacos.ConfigEditOperation{
			NacosOperation: &nacos.NacosOperation{
				Namespace: namespace,
				Group:     group,
			},
			DataId:  dataId,
			Content: string(edited),
			Type:    fileType,
		}); err != nil {
			return err
		}

		fmt.Println("Edited")
		return nil
	},
}

var deleteConfig = &cobra.Command{
	Use:   "config <dataId>",
	Short: "nacos config",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {

		if len(args) == 0 {
			return errors.New("data id required")
		}

		return nacosClient.DeleteConfig(nacos.ConfigDeleteOperation{
			NacosOperation: &nacos.NacosOperation{
				Namespace: namespace,
				Group:     group,
			},
			DataId: args[0],
		})
	},
}

func init() {

	editConfig.Flags().StringVarP(&fileType, "type", "t", "", "file type")

	getConfig.Flags().BoolVarP(&getAllConfig, "all", "A", false,
		"If present, list the requested object(s) across all config name（配合 -g 可按分组过滤）")

	editCmd.AddCommand(editConfig)
	getCmd.AddCommand(getConfig)
	deleteCmd.AddCommand(deleteConfig)
}

func printTable(items []nacos.NacosPageItem) {
	table := uitable.New()
	table.MaxColWidth = 50

	table.AddRow("DATA-ID", "GROUP", "NAMESPACE")

	for _, item := range items {
		if item.Tenant == "" {
			item.Tenant = "public"
		}
		table.AddRow(item.DataId, item.Group, item.Tenant)
	}

	fmt.Println(table)
}
