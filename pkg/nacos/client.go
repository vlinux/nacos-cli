package nacos

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrNotFound 服务端返回 404
var ErrNotFound = errors.New("资源不存在")

const (
	baseUrl = "/cs/configs"
	// defaultHTTPTimeout 单次请求超时
	defaultHTTPTimeout = 30 * time.Second
	// maxErrorBody 报错时回显的服务端响应体最大长度
	maxErrorBody = 500
)

// Client Nacos客户端
//
// 与上游实现相比，这里所有请求都经过 do() 统一发出，因此可以：
//   - 自动按需登录并注入 accessToken（Nacos 开启鉴权后必需）
//   - 统一处理超时/连接复用/响应体关闭
//   - 统一把 Nacos 的鉴权失败翻译成人能看懂的错误
type Client struct {
	Config *NacosConfig

	httpClient *http.Client

	// 以下字段由 ensureToken 维护，进程内共享
	mu          sync.Mutex
	token       string
	tokenExpire time.Time
	// cacheUsername 当前 token 属于哪个用户，用于失效时精确清理缓存
	cacheUsername string
}

// NewClient 使用给定配置创建客户端
func NewClient(config *NacosConfig) *Client {
	if config == nil {
		config = &NacosConfig{}
	}
	applyDefaults(config)

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if config.Insecure {
		// 内网自签证书场景，按用户要求显式跳过校验
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}

	return &Client{
		Config:     config,
		httpClient: &http.Client{Transport: transport, Timeout: defaultHTTPTimeout},
	}
}

// NewDefaultClient 按 默认值 < 配置文件 < 环境变量 的优先级创建客户端。
// 命令行参数请使用 ResolveConfig + NewClient 自行覆盖。
func NewDefaultClient() *Client {
	config, err := ResolveConfig(ConfigOptions{})
	if err != nil {
		// 配置解析失败不让 CLI 直接崩溃，退化为默认值，由后续请求给出可读错误
		config = &NacosConfig{}
		applyDefaults(config)
	}
	return NewClient(config)
}

func applyDefaults(config *NacosConfig) {
	if config.Addr == "" {
		config.Addr = defaultAddr
	}
	if config.ApiVersion == "" {
		config.ApiVersion = defaultApiVersion
	}
	if config.Namespace == "" {
		config.Namespace = PublicNamespace
	}
	if config.Group == "" {
		config.Group = DefaultNacosOperation.Group
	}
}

// tenantOf 把用户输入的命名空间转换成 Nacos API 需要的 tenant 参数。
// public 命名空间的真实 ID 是空字符串，直接传 "public" 会查不到数据。
func tenantOf(namespace string) string {
	if namespace == "" || strings.EqualFold(namespace, PublicNamespace) {
		return ""
	}
	return namespace
}

// configURL 拼出配置接口地址，例如 http://host:8848/nacos/v1/cs/configs
func (c *Client) configURL() (string, error) {
	return url.JoinPath(c.Config.Addr, c.Config.ApiVersion, baseUrl)
}

// do 发送一次请求，自动注入鉴权信息，并在 token 失效时自动重新登录后重试一次。
//
//	method   HTTP 方法
//	rawURL   完整请求地址
//	query    URL 查询参数（会合并进地址）
//	form     application/x-www-form-urlencoded 表单，GET/DELETE 传 nil
func (c *Client) do(method, rawURL string, query, form url.Values) ([]byte, error) {
	var (
		lastStatus int
		lastBody   []byte
	)

	// 最多两次：第一次用现有 token，若判定为 token 失效则清缓存重登后再来一次
	for attempt := 0; ; attempt++ {
		var bodyReader io.Reader
		if form != nil {
			bodyReader = strings.NewReader(form.Encode())
		}

		req, err := http.NewRequest(method, rawURL, bodyReader)
		if err != nil {
			return nil, err
		}
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}

		values := req.URL.Query()
		for k, vs := range query {
			values.Del(k)
			for _, v := range vs {
				values.Add(k, v)
			}
		}

		token, err := c.ensureToken()
		if err != nil {
			return nil, err
		}
		if token != "" {
			values.Set("accessToken", token)
		}
		req.URL.RawQuery = values.Encode()

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}

		lastStatus, lastBody = resp.StatusCode, data

		if attempt == 0 && shouldRelogin(resp.StatusCode, data) && !c.hasStaticToken() {
			c.invalidateToken()
			continue
		}
		break
	}

	switch lastStatus {
	case http.StatusOK:
		return lastBody, nil
	case http.StatusNotFound:
		return nil, fmt.Errorf("%w：%s", ErrNotFound, compactBody(lastBody))
	case http.StatusForbidden, http.StatusUnauthorized:
		return nil, fmt.Errorf(
			"鉴权失败 (HTTP %d)：%s\n"+
				"请检查 --username/--password（或 NACOS_USERNAME/NACOS_PASSWORD）是否正确，"+
				"以及该账号对目标命名空间/分组是否有权限",
			lastStatus, compactBody(lastBody))
	default:
		return nil, fmt.Errorf("请求失败 (HTTP %d)：%s", lastStatus, compactBody(lastBody))
	}
}

// shouldRelogin 判断 403 是否属于「token 过期/失效」而非「真的没权限」。
// 只有前者才值得清掉缓存重新登录，后者重登多少次都没用。
//
// 实测 Nacos 2.4.3 的几种响应：
//   - token 被篡改 / 签名密钥轮换 -> 403 "Invalid signature"
//   - token 不是合法 JWT         -> 403 "token invalid!"
//   - token 过期                 -> 403 "token expired!"
//   - 完全不带 token             -> 403 "user not found!"
//   - 账号无权限                 -> 403 "authorization failed!"
func shouldRelogin(status int, body []byte) bool {
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		return false
	}
	msg := strings.ToLower(string(body))
	for _, kw := range []string{
		"expired",
		"signature",
		"token invalid",
		"invalid token",
		"user not found",
	} {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

// compactBody 把服务端响应体压成一行短文本，便于塞进错误信息
func compactBody(body []byte) string {
	text := strings.Join(strings.Fields(string(body)), " ")
	if text == "" {
		return "(空响应)"
	}
	if len(text) > maxErrorBody {
		text = text[:maxErrorBody] + "..."
	}
	return text
}
