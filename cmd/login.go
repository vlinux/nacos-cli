package cmd

import (
	"fmt"

	"github/szpinc/nacosctl/pkg/nacos"

	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "登录 Nacos 并缓存 accessToken（Nacos 开启鉴权时使用）",
	Long: `登录 Nacos 服务器，验证用户名密码并缓存 accessToken。

登录成功后 token 会缓存到本地（默认 ~/.nacos-cli/token-cache.json，权限 0600），
后续命令直接复用，不需要每次重新登录；token 过期后会自动重新登录。

示例：
  # 命令行传密码
  nacos-cli login --addr http://nacos.prod:8848/nacos -u nacos -p 'your-password'

  # 密码从标准输入读，避免留在 shell 历史里
  echo 'your-password' | nacos-cli login -u nacos --password-stdin

  # 或者干脆走环境变量
  export NACOS_ADDR=http://nacos.prod:8848/nacos
  export NACOS_USERNAME=nacos
  export NACOS_PASSWORD='your-password'
  nacos-cli login`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(cmd); err != nil {
			return err
		}

		fmt.Println("使用以下配置登录 Nacos：")
		fmt.Println(nacosClient.Config.Describe())

		result, err := nacosClient.Authenticate()
		if err != nil {
			return err
		}

		fmt.Println("\n登录成功")
		fmt.Printf("  accessToken : %s\n", maskToken(result.AccessToken))
		fmt.Printf("  有效期      : %d 秒（约 %.1f 小时）\n", result.TokenTTL, float64(result.TokenTTL)/3600)
		fmt.Printf("  全局管理员  : %v\n", result.GlobalAdmin)
		if result.Username != "" {
			fmt.Printf("  用户名      : %s\n", result.Username)
		}

		if cachePath := nacos.TokenCachePath(); cachePath != "" {
			fmt.Printf("\ntoken 已缓存到 %s，后续命令无需重复登录\n", cachePath)
		} else {
			fmt.Println("\ntoken 缓存已通过 NACOS_TOKEN_CACHE 关闭，后续每条命令都会重新登录")
		}

		// 顺手探一下这个账号能不能真的读到配置，只提示不报错
		items, checkErr := nacosClient.AllConfig(nacos.ConfigGetOperation{
			NacosOperation: &nacos.NacosOperation{Namespace: namespace},
		})
		if checkErr != nil {
			fmt.Printf("\n[提示] 试读配置列表失败，请确认该账号权限：%v\n", checkErr)
		} else {
			fmt.Printf("\n[提示] 试读配置列表成功，当前命名空间共 %d 条配置\n", len(items))
		}
		return nil
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "清除本地缓存的 accessToken",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initClient(cmd); err != nil {
			return err
		}
		if err := nacosClient.Logout(); err != nil {
			return err
		}
		fmt.Println("已清除本地缓存的 token")
		return nil
	},
}

// maskToken 只展示 token 前几位，避免把凭据打到日志里
func maskToken(token string) string {
	if len(token) <= 16 {
		return "****"
	}
	return token[:16] + "......(已省略)"
}

func init() {
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
}
