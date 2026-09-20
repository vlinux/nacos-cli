package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github/szpinc/nacosctl/pkg/nacos"

	"github.com/spf13/cobra"
)

var (
	namespace string
	group     string
)

var (
	flagAddr          string
	flagApiVersion    string
	flagUsername      string
	flagPassword      string
	flagAccessToken   string
	flagConfigFile    string
	flagInsecure      bool
	flagPasswordStdin bool
)

var nacosClient *nacos.Client

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "nacos-cli",
	Short: "nacos cli tools",
	Long: `nacos-cli 用命令行操作 Nacos 配置。

常用命令：
  nacos-cli get config              列出当前命名空间下的全部配置
  nacos-cli get config app.yaml     打印单条配置的内容
  nacos-cli apply -f app.yaml       从本地文件发布/更新配置
  nacos-cli edit config app.yaml    用 $EDITOR 改配置
  nacos-cli delete config app.yaml  删除配置
  nacos-cli login / logout          登录并缓存 accessToken / 清除缓存

凭据优先级（从高到低）：
  命令行参数 > 环境变量 > 配置文件(~/.nacos-cli/config.yaml) > 内置默认值

Nacos 开启鉴权（nacos.core.auth.enabled=true）后，提供用户名密码即可：
本工具会自动登录拿 accessToken、注入到每个请求里，并在 token 过期时自动重新登录。`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return initClient(cmd)
	},
	// 运行时错误（网络/鉴权失败等）不需要打印整页 flag 用法
	SilenceUsage: true,
	ValidArgs:    []string{"get", "delete", "edit", "apply", "login", "logout"},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	pf := rootCmd.PersistentFlags()

	pf.StringVarP(&namespace, "namespace", "n", "public", "nacos namespace（命名空间 ID，public 为默认空间）")
	pf.StringVarP(&group, "group", "g", "DEFAULT_GROUP", "nacos group")

	pf.StringVar(&flagAddr, "addr", "", "nacos 地址，如 http://127.0.0.1:8848/nacos（环境变量 NACOS_ADDR）")
	pf.StringVar(&flagApiVersion, "api-version", "", "nacos API 版本，默认 v1（环境变量 NACOS_API_VERSION）")
	pf.StringVarP(&flagUsername, "username", "u", "", "nacos 用户名（环境变量 NACOS_USERNAME）")
	pf.StringVarP(&flagPassword, "password", "p", "", "nacos 密码（环境变量 NACOS_PASSWORD）")
	pf.BoolVar(&flagPasswordStdin, "password-stdin", false, "从标准输入读取密码，避免密码出现在 ps 与 shell 历史里")
	pf.StringVar(&flagAccessToken, "access-token", "", "直接指定 accessToken，跳过登录（环境变量 NACOS_ACCESS_TOKEN）")
	pf.StringVar(&flagConfigFile, "config", "", "配置文件路径，默认 ~/.nacos-cli/config.yaml（环境变量 NACOS_CONFIG_FILE）")
	pf.BoolVarP(&flagInsecure, "insecure", "k", false, "跳过 HTTPS 证书校验（内网自签证书场景）")

	// 先构造一个只依赖环境变量/配置文件的客户端，
	// 这样 shell 补全等不经过 PersistentPreRunE 的路径也不会拿到 nil。
	nacosClient = nacos.NewDefaultClient()
}

// initClient 解析命令行参数并与环境变量/配置文件合并，构建最终客户端
func initClient(cmd *cobra.Command) error {
	opts := nacos.ConfigOptions{
		Addr:        changedString(cmd, "addr", flagAddr),
		ApiVersion:  changedString(cmd, "api-version", flagApiVersion),
		Username:    changedString(cmd, "username", flagUsername),
		Password:    changedString(cmd, "password", flagPassword),
		AccessToken: changedString(cmd, "access-token", flagAccessToken),
		Namespace:   changedString(cmd, "namespace", namespace),
		Group:       changedString(cmd, "group", group),
		Insecure:    changedBool(cmd, "insecure", flagInsecure),
	}
	if cmd.Flags().Changed("config") {
		opts.ConfigFile = flagConfigFile
	}

	config, err := nacos.ResolveConfig(opts)
	if err != nil {
		return err
	}

	if flagPasswordStdin {
		password, err := readPasswordFromStdin()
		if err != nil {
			return err
		}
		config.Password = password
	}

	nacosClient = nacos.NewClient(config)

	// 子命令统一使用最终生效的命名空间/分组，避免各处重复解析
	namespace, group = config.Namespace, config.Group
	return nil
}

// changedString 仅当用户真的传了该参数时才返回指针，nil 表示「不覆盖下一优先级」
func changedString(cmd *cobra.Command, name, value string) *string {
	if !cmd.Flags().Changed(name) {
		return nil
	}
	return &value
}

func changedBool(cmd *cobra.Command, name string, value bool) *bool {
	if !cmd.Flags().Changed(name) {
		return nil
	}
	return &value
}

// readPasswordFromStdin 从标准输入读取一行作为密码
func readPasswordFromStdin() (string, error) {
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("从标准输入读取密码失败: %w", err)
	}
	password := strings.TrimRight(line, "\r\n")
	if password == "" {
		return "", errors.New("从标准输入读取到的密码为空")
	}
	return password, nil
}
