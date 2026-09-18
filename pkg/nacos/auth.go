package nacos

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// defaultTokenTTL Nacos 默认 token 有效期（秒），对应
	// nacos.core.auth.plugin.nacos.token.expire.seconds 的默认值 18000。
	defaultTokenTTL = 18000
	// tokenRenewAhead 提前续期：token 剩余有效期不足该值时不再复用，直接重新登录
	tokenRenewAhead = 5 * time.Minute
)

var (
	// ErrNoCredential 未提供用户名
	ErrNoCredential = errors.New(
		"未提供 Nacos 用户名：请通过 --username / NACOS_USERNAME 指定，或改用 --access-token 直接传 token")

	// ErrIncompleteCredential 用户名和密码只给了一半
	ErrIncompleteCredential = errors.New(
		"用户名和密码必须同时提供：请检查 --username/--password 或 NACOS_USERNAME/NACOS_PASSWORD")
)

// loginPathCandidates 返回按优先级排序的登录接口相对路径（相对 NACOS_ADDR）。
//
// 不同大版本的登录路径不一样：
//   - Nacos 1.x / 2.x：POST {addr}/v1/auth/login
//   - Nacos 3.x：      POST {addr}/v3/auth/user/login  （v1 路径通常仍保留做兼容）
//
// 这里按「与当前 ApiVersion 一致 → v1 → v3」的顺序逐个尝试，谁先通就用谁。
func loginPathCandidates(apiVersion string) []string {
	raw := []string{}
	if apiVersion != "" {
		raw = append(raw, apiVersion+"/auth/login")
	}
	raw = append(raw, "v1/auth/login", "v1/auth/user/login", "v3/auth/user/login")

	seen := make(map[string]struct{}, len(raw))
	candidates := make([]string, 0, len(raw))
	for _, p := range raw {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		candidates = append(candidates, p)
	}
	return candidates
}

// Login 用用户名/密码调用 Nacos 登录接口换取 accessToken。
//
// 注意这里不会写入缓存，纯登录动作；需要带缓存请用 Authenticate。
func (c *Client) Login(username, password string) (*LoginResult, error) {
	if username == "" {
		return nil, ErrNoCredential
	}
	if password == "" {
		return nil, ErrIncompleteCredential
	}

	var lastErr error
	for _, p := range loginPathCandidates(c.Config.ApiVersion) {
		loginURL, err := url.JoinPath(c.Config.Addr, p)
		if err != nil {
			return nil, err
		}

		form := url.Values{
			"username": {username},
			"password": {password},
		}

		req, err := http.NewRequest(http.MethodPost, loginURL, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			// 连不上就是连不上，换端点也没意义
			return nil, fmt.Errorf("请求登录接口失败 %s: %w", loginURL, err)
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		switch {
		case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed:
			// 该版本没有这个登录接口，换下一个
			lastErr = fmt.Errorf("登录接口不存在: %s (HTTP %d)", loginURL, resp.StatusCode)
			continue
		case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized:
			return nil, fmt.Errorf("登录被拒绝 (HTTP %d)：用户名或密码错误。服务端返回: %s",
				resp.StatusCode, compactBody(body))
		case resp.StatusCode != http.StatusOK:
			lastErr = fmt.Errorf("登录失败: %s (HTTP %d) %s",
				loginURL, resp.StatusCode, compactBody(body))
			continue
		}

		result := LoginResult{}
		if err := json.Unmarshal(body, &result); err != nil {
			lastErr = fmt.Errorf("解析登录响应失败: %w。服务端返回: %s", err, compactBody(body))
			continue
		}
		if result.AccessToken == "" {
			lastErr = fmt.Errorf("登录响应里没有 accessToken。服务端返回: %s", compactBody(body))
			continue
		}
		if result.TokenTTL <= 0 {
			result.TokenTTL = defaultTokenTTL
		}
		return &result, nil
	}

	if lastErr == nil {
		lastErr = errors.New("找不到可用的登录接口，请确认 NACOS_ADDR 与 NACOS_API_VERSION 是否正确")
	}
	return nil, lastErr
}

// Authenticate 强制登录一次，并把 token 写入内存与本地缓存。
// 供 `nacos-cli login` 使用。
func (c *Client) Authenticate() (*LoginResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loginAndCache()
}

// Logout 清除内存与本地缓存的 token
func (c *Client) Logout() error {
	c.mu.Lock()
	c.token = ""
	c.tokenExpire = time.Time{}
	c.cacheUsername = ""
	c.mu.Unlock()

	// 登出就是「登出这台服务器」，清掉该地址下的全部账号缓存
	return removeCachedTokensForAddr(c.Config.Addr)
}

// TokenInfo 返回当前内存中的 token 及过期时间，供命令行展示
func (c *Client) TokenInfo() (string, time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.token, c.tokenExpire
}

// hasStaticToken 用户是否显式传了 token（这种情况不做自动重登）
func (c *Client) hasStaticToken() bool {
	return c.Config.AccessToken != ""
}

// invalidateToken 丢掉当前 token，下次请求会重新走登录
func (c *Client) invalidateToken() {
	c.mu.Lock()
	username := c.cacheUsername
	if username == "" {
		username = c.Config.Username
	}
	c.token = ""
	c.tokenExpire = time.Time{}
	c.cacheUsername = ""
	c.mu.Unlock()

	_ = removeCachedToken(c.Config.Addr, username)
}

// useCachedToken 采用一条本地缓存 token。调用前需持有 c.mu。
func (c *Client) useCachedToken(entry cachedToken) {
	c.token = entry.AccessToken
	c.tokenExpire = entry.ExpireAt
	c.cacheUsername = entry.Username
}

// ensureToken 返回本次请求应当携带的 accessToken。
//
// 取值顺序：
//  1. 显式指定的 --access-token
//  2. 进程内已缓存的 token
//  3. 本地 token 缓存文件（避免每次敲命令都登录一次；login 过就能直接用）
//  4. 用用户名/密码登录
//
// 什么都没配时返回空串，此时请求不带 accessToken，兼容未开启鉴权的 Nacos。
func (c *Client) ensureToken() (string, error) {
	if c.hasStaticToken() {
		return c.Config.AccessToken, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.tokenExpire.Add(-tokenRenewAhead)) {
		return c.token, nil
	}

	username, password := c.Config.Username, c.Config.Password

	if username != "" {
		// 先看缓存：可能之前 nacos-cli login 过，此时不必再要密码
		if cached, ok := loadCachedToken(c.Config.Addr, username); ok {
			c.useCachedToken(cached)
			return c.token, nil
		}
		if password == "" {
			return "", ErrIncompleteCredential
		}

		result, err := c.loginAndCache()
		if err != nil {
			return "", err
		}
		return result.AccessToken, nil
	}

	// 只给了密码没给用户名，属于配置错误，直接说清楚
	if username == "" && password != "" {
		return "", ErrNoCredential
	}

	// 一个凭据都没配：若本地存过这个地址的 token（例如刚 login 过），直接复用
	if cached, ok := loadCachedTokenForAddr(c.Config.Addr); ok {
		c.useCachedToken(cached)
		return c.token, nil
	}

	return "", nil
}

// loginAndCache 调用登录接口并写入内存/磁盘缓存。调用前需持有 c.mu。
func (c *Client) loginAndCache() (*LoginResult, error) {
	result, err := c.Login(c.Config.Username, c.Config.Password)
	if err != nil {
		return nil, err
	}

	ttl := time.Duration(result.TokenTTL) * time.Second
	expireAt := time.Now().Add(ttl)
	c.token = result.AccessToken
	c.tokenExpire = expireAt
	c.cacheUsername = c.Config.Username

	saveCachedToken(c.Config.Addr, c.Config.Username, result.AccessToken, expireAt)
	return result, nil
}
