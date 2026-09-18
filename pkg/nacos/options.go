package nacos

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultAddr       = "http://127.0.0.1:8848/nacos"
	defaultApiVersion = "v1"

	// 环境变量名
	EnvAddr        = "NACOS_ADDR"
	EnvApiVersion  = "NACOS_API_VERSION"
	EnvUsername    = "NACOS_USERNAME"
	EnvPassword    = "NACOS_PASSWORD"
	EnvAccessToken = "NACOS_ACCESS_TOKEN"
	EnvNamespace   = "NACOS_NAMESPACE"
	EnvGroup       = "NACOS_GROUP"
	EnvInsecure    = "NACOS_INSECURE"
	EnvTokenCache  = "NACOS_TOKEN_CACHE"
	EnvConfigFile  = "NACOS_CONFIG_FILE"
)

// ConfigOptions 配置覆盖项。字段为 nil 表示「命令行没指定」，保留下一优先级的值。
type ConfigOptions struct {
	Addr        *string
	ApiVersion  *string
	Username    *string
	Password    *string
	AccessToken *string
	Namespace   *string
	Group       *string
	Insecure    *bool
	// ConfigFile 非空表示用户显式指定了配置文件，文件不存在时报错
	ConfigFile string
}

// DefaultConfigFilePath 返回默认配置文件路径 ~/.nacos-cli/config.yaml
func DefaultConfigFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, defaultCacheDir, "config.yaml")
}

// ResolveConfig 合并出最终配置，优先级从低到高：
//
//	内置默认值 < 配置文件 < 环境变量 < 命令行参数
func ResolveConfig(opts ConfigOptions) (*NacosConfig, error) {
	config := &NacosConfig{}

	// 1) 配置文件
	path := opts.ConfigFile
	explicit := path != ""
	if path == "" {
		path = os.Getenv(EnvConfigFile)
		explicit = path != ""
	}
	if path == "" {
		path = DefaultConfigFilePath()
	}
	if err := mergeConfigFile(config, path, explicit); err != nil {
		return nil, err
	}

	// 2) 环境变量
	mergeEnv(config)

	// 3) 命令行参数
	mergeOptions(config, opts)

	applyDefaults(config)
	return config, nil
}

// mergeConfigFile 读取 yaml 配置文件，只覆盖文件中实际写了的字段
func mergeConfigFile(config *NacosConfig, path string, explicit bool) error {
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !explicit {
			// 默认路径没有配置文件是正常情况
			return nil
		}
		return fmt.Errorf("读取配置文件失败 %s: %w", path, err)
	}

	fileConfig := NacosConfig{}
	if err := yaml.Unmarshal(data, &fileConfig); err != nil {
		return fmt.Errorf("解析配置文件失败 %s: %w", path, err)
	}

	if fileConfig.Addr != "" {
		config.Addr = fileConfig.Addr
	}
	if fileConfig.ApiVersion != "" {
		config.ApiVersion = fileConfig.ApiVersion
	}
	if fileConfig.Username != "" {
		config.Username = fileConfig.Username
	}
	if fileConfig.Password != "" {
		config.Password = fileConfig.Password
	}
	if fileConfig.AccessToken != "" {
		config.AccessToken = fileConfig.AccessToken
	}
	if fileConfig.Namespace != "" {
		config.Namespace = fileConfig.Namespace
	}
	if fileConfig.Group != "" {
		config.Group = fileConfig.Group
	}
	if fileConfig.Insecure {
		config.Insecure = true
	}
	return nil
}

func mergeEnv(config *NacosConfig) {
	if v := os.Getenv(EnvAddr); v != "" {
		config.Addr = v
	}
	if v := os.Getenv(EnvApiVersion); v != "" {
		config.ApiVersion = v
	}
	if v := os.Getenv(EnvUsername); v != "" {
		config.Username = v
	}
	if v := os.Getenv(EnvPassword); v != "" {
		config.Password = v
	}
	if v := os.Getenv(EnvAccessToken); v != "" {
		config.AccessToken = v
	}
	if v := os.Getenv(EnvNamespace); v != "" {
		config.Namespace = v
	}
	if v := os.Getenv(EnvGroup); v != "" {
		config.Group = v
	}
	if v := os.Getenv(EnvInsecure); v != "" {
		if parsed, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			config.Insecure = parsed
		}
	}
}

func mergeOptions(config *NacosConfig, opts ConfigOptions) {
	if opts.Addr != nil {
		config.Addr = *opts.Addr
	}
	if opts.ApiVersion != nil {
		config.ApiVersion = *opts.ApiVersion
	}
	if opts.Username != nil {
		config.Username = *opts.Username
	}
	if opts.Password != nil {
		config.Password = *opts.Password
	}
	if opts.AccessToken != nil {
		config.AccessToken = *opts.AccessToken
	}
	if opts.Namespace != nil {
		config.Namespace = *opts.Namespace
	}
	if opts.Group != nil {
		config.Group = *opts.Group
	}
	if opts.Insecure != nil {
		config.Insecure = *opts.Insecure
	}
}

// Describe 返回脱敏后的配置摘要，便于 login 等命令回显
func (config *NacosConfig) Describe() string {
	mask := func(s string) string {
		if s == "" {
			return "(未设置)"
		}
		return "(已设置)"
	}
	return fmt.Sprintf(
		"  地址        : %s\n  命名空间    : %s\n  分组        : %s\n  用户名      : %s\n  密码        : %s\n  静态 token  : %s",
		config.Addr, config.Namespace, config.Group,
		orDefault(config.Username, "(未设置)"), mask(config.Password), mask(config.AccessToken))
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
