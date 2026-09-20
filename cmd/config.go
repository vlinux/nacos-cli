/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"bytes"
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
	Short: "读取配置：给 dataId 读单条，不给则列出全部",
	Long: `读取 Nacos 配置。

  不传 dataId —— 列出当前命名空间下的配置列表（等价于 -A/--all）
  传 dataId   —— 把该配置的内容打印到标准输出

命名空间/分组默认取自命令行参数、环境变量或 ~/.nacos-cli/config.yaml。`,
	Example: `  # 列出当前命名空间的全部配置
  nacos-cli get config

  # 只列某个分组
  nacos-cli get config -g MY_GROUP

  # 打印单条配置的内容
  nacos-cli get config app.yaml

  # 指定分组与命名空间
  nacos-cli get config app.yaml -g MY_GROUP -n dev`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {

		// 没给 dataId 时按「列出配置」处理，和 kubectl get 的习惯保持一致；
		// 上游这里直接报 "data id required"，很容易被误解成命名空间没生效。
		listMode := getAllConfig || len(args) == 0

		if getAllConfig && len(args) > 0 {
			return fmt.Errorf("-A/--all 是「列出全部配置」，不能和 dataId %q 一起用；想读单条请去掉 -A", args[0])
		}

		if listMode {
			// 默认跨分组列出全部；只有用户显式传了 -g 才按分组过滤
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
	Short: "用 $EDITOR 打开配置，保存后自动回写",
	Long: `把配置内容拉到本地临时文件，用 $EDITOR 打开，保存退出后自动回写 Nacos。
内容没变则不会发起发布。

编辑器取 $EDITOR / $VISUAL，没设置时回退到 vi。`,
	Example: `  nacos-cli edit config app.yaml
  nacos-cli edit config app.yaml -g MY_GROUP -n dev`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {

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
	Short: "删除配置",
	Long:  `删除指定配置。删除前请自行确认数据Id 与分组是否正确。`,
	Example: `  nacos-cli delete config app.yaml
  nacos-cli delete config app.yaml -g MY_GROUP -n dev`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {

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

	editConfig.Flags().StringVarP(&fileType, "type", "t", "", "配置类型，默认沿用服务端已有的类型")

	getConfig.Flags().BoolVarP(&getAllConfig, "all", "A", false,
		"显式声明「列出全部配置」。不传 dataId 时默认就是这个行为，该参数主要用于脚本里写明意图")

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
