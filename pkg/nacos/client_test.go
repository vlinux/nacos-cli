package nacos

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockNacos 模拟一个「开启了鉴权」的 Nacos，行为对齐实测的 2.4.3：
//   - 不带 token       -> 403 user not found!
//   - 签名被篡改       -> 403 Invalid signature
//   - token 不在白名单 -> 403 token expired!
//   - 配置不存在       -> 200 + 空 body（不是 404）
type mockNacos struct {
	mu         sync.Mutex
	server     *httptest.Server
	username   string
	password   string
	tokens     map[string]bool
	configs    map[string]string
	loginCalls int
	// rejectToken 命中该 token 时返回「签名无效」，用于验证自动重登
	rejectToken string
	requireAuth bool
	// lastQuery 记录最后一次配置请求的查询参数，便于断言传参
	lastQuery map[string]string
}

func newMockNacos(t *testing.T) *mockNacos {
	t.Helper()
	m := &mockNacos{
		username:    "nacos",
		password:    "Nacos@123456",
		tokens:      map[string]bool{},
		configs:     map[string]string{},
		requireAuth: true,
	}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(m.server.Close)
	return m
}

func (m *mockNacos) handle(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()

	switch {
	case r.URL.Path == "/nacos/v1/auth/login" && r.Method == http.MethodPost:
		m.handleLogin(w, r)
	case r.URL.Path == "/nacos/v1/cs/configs":
		m.handleConfig(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}
}

func (m *mockNacos) handleLogin(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.loginCalls++
	call := m.loginCalls
	m.mu.Unlock()

	if r.PostForm.Get("username") != m.username || r.PostForm.Get("password") != m.password {
		writeForbidden(w, "user not found!")
		return
	}

	token := fmt.Sprintf("token-%d", call)
	m.mu.Lock()
	m.tokens[token] = true
	m.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(LoginResult{
		AccessToken: token,
		TokenTTL:    18000,
		GlobalAdmin: true,
		Username:    r.PostForm.Get("username"),
	})
}

func (m *mockNacos) handleConfig(w http.ResponseWriter, r *http.Request) {
	// 真实 Nacos 会把 query 与 form 参数合并，这里保持一致
	token := param(r, "accessToken")

	m.mu.Lock()

	if m.requireAuth {
		switch {
		case token == "":
			m.mu.Unlock()
			writeForbidden(w, "user not found!")
			return
		case m.rejectToken != "" && token == m.rejectToken:
			m.mu.Unlock()
			writeForbidden(w, "Invalid signature")
			return
		case !m.tokens[token]:
			m.mu.Unlock()
			writeForbidden(w, "token expired!")
			return
		}
	}

	tenant := param(r, "tenant")
	group := param(r, "group")
	dataId := param(r, "dataId")

	m.lastQuery = map[string]string{"tenant": tenant, "group": group, "dataId": dataId}

	switch r.Method {
	case http.MethodGet:
		if dataId == "" {
			items := make([]NacosPageItem, 0, len(m.configs))
			for id := range m.configs {
				items = append(items, NacosPageItem{DataId: id, Group: group, Tenant: tenant})
			}
			result := NacosPageResult{TotalCount: len(items), PageNumber: 1, PagesAvailable: 1, PageItems: items}
			m.mu.Unlock()
			_ = json.NewEncoder(w).Encode(result)
			return
		}

		content, ok := m.configs[dataId]
		if !ok {
			// 对齐真实 Nacos：不存在时 200 + 空 body
			m.mu.Unlock()
			return
		}
		detail := NacosConfigDetail{
			DataID:  dataId,
			Group:   group,
			Tenant:  tenant,
			Content: content,
			Type:    "yaml",
			Md5:     "md5",
		}
		m.mu.Unlock()
		_ = json.NewEncoder(w).Encode(detail)

	case http.MethodPost:
		m.configs[dataId] = param(r, "content")
		m.mu.Unlock()
		_, _ = w.Write([]byte("true"))

	case http.MethodDelete:
		delete(m.configs, dataId)
		m.mu.Unlock()
		_, _ = w.Write([]byte("true"))

	default:
		m.mu.Unlock()
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func writeForbidden(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = fmt.Fprintf(w, `{"status":403,"error":"Forbidden","message":"%s"}`, message)
}

// param 模拟 Nacos 服务端合并 query 与 form 参数的取参方式
func param(r *http.Request, name string) string {
	if v := r.URL.Query().Get(name); v != "" {
		return v
	}
	return r.PostForm.Get(name)
}

func (m *mockNacos) addConfig(dataId, content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configs[dataId] = content
}

func (m *mockNacos) loginCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loginCalls
}

// newTestClient 构造一个指向 mock 服务的客户端，并把 token 缓存隔离到临时文件
func newTestClient(t *testing.T, m *mockNacos, mutate func(config *NacosConfig)) *Client {
	t.Helper()

	// 隔离 token 缓存，避免污染用户主目录
	t.Setenv(EnvTokenCache, filepath.Join(t.TempDir(), "token-cache.json"))

	config := &NacosConfig{
		Addr:       m.server.URL + "/nacos",
		ApiVersion: "v1",
		Namespace:  PublicNamespace,
		Group:      "DEFAULT_GROUP",
	}
	if mutate != nil {
		mutate(config)
	}
	return NewClient(config)
}

func TestGetWithAuth(t *testing.T) {
	m := newMockNacos(t)
	m.addConfig("common.yaml", "hello: world")

	client := newTestClient(t, m, func(c *NacosConfig) {
		c.Username = "nacos"
		c.Password = "Nacos@123456"
	})

	detail, err := client.Get(ConfigGetOperation{
		NacosOperation: &DefaultNacosOperation,
		DataId:         "common.yaml",
	})
	require.NoError(t, err)
	assert.Equal(t, "hello: world", detail.Content)
	assert.Equal(t, 1, m.loginCallCount(), "带凭据时必须先登录一次")
}

func TestAllConfigWithAuth(t *testing.T) {
	m := newMockNacos(t)
	m.addConfig("a.yaml", "a")
	m.addConfig("b.yaml", "b")

	client := newTestClient(t, m, func(c *NacosConfig) {
		c.Username = "nacos"
		c.Password = "Nacos@123456"
	})

	items, err := client.AllConfig(ConfigGetOperation{
		NacosOperation: &DefaultNacosOperation,
	})
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestEditAndDeleteWithAuth(t *testing.T) {
	m := newMockNacos(t)

	client := newTestClient(t, m, func(c *NacosConfig) {
		c.Username = "nacos"
		c.Password = "Nacos@123456"
	})

	require.NoError(t, client.Edit(ConfigEditOperation{
		NacosOperation: &DefaultNacosOperation,
		DataId:         "new.yaml",
		Content:        "x: 1",
		Type:           "yaml",
	}))

	detail, err := client.Get(ConfigGetOperation{
		NacosOperation: &DefaultNacosOperation,
		DataId:         "new.yaml",
	})
	require.NoError(t, err)
	assert.Equal(t, "x: 1", detail.Content)

	require.NoError(t, client.DeleteConfig(ConfigDeleteOperation{
		NacosOperation: &DefaultNacosOperation,
		DataId:         "new.yaml",
	}))

	_, err = client.Get(ConfigGetOperation{
		NacosOperation: &DefaultNacosOperation,
		DataId:         "new.yaml",
	})
	assert.ErrorContains(t, err, "配置不存在")
}

func TestWrongPassword(t *testing.T) {
	m := newMockNacos(t)

	client := newTestClient(t, m, func(c *NacosConfig) {
		c.Username = "nacos"
		c.Password = "wrong"
	})

	_, err := client.Get(ConfigGetOperation{
		NacosOperation: &DefaultNacosOperation,
		DataId:         "common.yaml",
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "用户名或密码错误")
}

func TestIncompleteCredential(t *testing.T) {
	m := newMockNacos(t)

	client := newTestClient(t, m, func(c *NacosConfig) {
		c.Username = "nacos"
	})

	_, err := client.Get(ConfigGetOperation{
		NacosOperation: &DefaultNacosOperation,
		DataId:         "common.yaml",
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "用户名和密码必须同时提供")

	// 只给密码没给用户名，同样属于配置错误
	client2 := newTestClient(t, m, func(c *NacosConfig) {
		c.Password = "Nacos@123456"
	})
	_, err = client2.Get(ConfigGetOperation{
		NacosOperation: &DefaultNacosOperation,
		DataId:         "common.yaml",
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "未提供 Nacos 用户名")
}

func TestPublicNamespaceUsesEmptyTenant(t *testing.T) {
	m := newMockNacos(t)
	m.requireAuth = false
	m.addConfig("a.yaml", "a")

	client := newTestClient(t, m, nil)

	_, err := client.Get(ConfigGetOperation{
		NacosOperation: &NacosOperation{Namespace: PublicNamespace, Group: "DEFAULT_GROUP"},
		DataId:         "a.yaml",
	})
	require.NoError(t, err)

	// public 的真实命名空间 ID 是空串，不能把 "public" 当 tenant 传过去
	assert.Equal(t, "", m.lastQuery["tenant"])
}

func TestCustomNamespacePassedAsTenant(t *testing.T) {
	m := newMockNacos(t)
	m.requireAuth = false
	m.addConfig("a.yaml", "a")

	client := newTestClient(t, m, nil)

	_, err := client.Get(ConfigGetOperation{
		NacosOperation: &NacosOperation{Namespace: "dev-ns", Group: "DEFAULT_GROUP"},
		DataId:         "a.yaml",
	})
	require.NoError(t, err)
	assert.Equal(t, "dev-ns", m.lastQuery["tenant"])
}

// TestNoAuthServerStillWorks 未开启鉴权的 Nacos 必须继续可用
func TestNoAuthServerStillWorks(t *testing.T) {
	m := newMockNacos(t)
	m.requireAuth = false
	m.addConfig("common.yaml", "hello")

	client := newTestClient(t, m, nil)

	detail, err := client.Get(ConfigGetOperation{
		NacosOperation: &DefaultNacosOperation,
		DataId:         "common.yaml",
	})
	require.NoError(t, err)
	assert.Equal(t, "hello", detail.Content)
	assert.Equal(t, 0, m.loginCallCount(), "没有凭据时不应尝试登录")
}

// TestAutoReloginOnInvalidToken 缓存里的 token 失效时应自动重新登录并重试
func TestAutoReloginOnInvalidToken(t *testing.T) {
	m := newMockNacos(t)
	m.addConfig("common.yaml", "hello")

	t.Setenv(EnvTokenCache, filepath.Join(t.TempDir(), "token-cache.json"))
	addr := m.server.URL + "/nacos"

	// 预置一个服务端不认的 token
	saveCachedToken(addr, "nacos", "stale-token", time.Now().Add(time.Hour))
	m.mu.Lock()
	m.rejectToken = "stale-token"
	m.mu.Unlock()

	client := NewClient(&NacosConfig{
		Addr:       addr,
		ApiVersion: "v1",
		Namespace:  PublicNamespace,
		Group:      "DEFAULT_GROUP",
		Username:   "nacos",
		Password:   "Nacos@123456",
	})

	detail, err := client.Get(ConfigGetOperation{
		NacosOperation: &DefaultNacosOperation,
		DataId:         "common.yaml",
	})
	require.NoError(t, err)
	assert.Equal(t, "hello", detail.Content)
	assert.Equal(t, 1, m.loginCallCount(), "失效后应当自动重新登录一次")
}

// TestNoReloginOnPermissionDenied 真的是没权限时不要白费力气反复登录
func TestNoReloginOnPermissionDenied(t *testing.T) {
	m := newMockNacos(t)

	t.Setenv(EnvTokenCache, filepath.Join(t.TempDir(), "token-cache.json"))
	addr := m.server.URL + "/nacos"

	m.mu.Lock()
	m.tokens["valid-token"] = true
	m.mu.Unlock()
	saveCachedToken(addr, "nacos", "valid-token", time.Now().Add(time.Hour))

	// 让服务端对这个合法 token 返回「无权限」而不是「token 失效」
	m.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/nacos/v1/cs/configs" {
			writeForbidden(w, "authorization failed!")
			return
		}
		m.handle(w, r)
	})

	client := NewClient(&NacosConfig{
		Addr:       addr,
		ApiVersion: "v1",
		Namespace:  PublicNamespace,
		Group:      "DEFAULT_GROUP",
		Username:   "nacos",
		Password:   "Nacos@123456",
	})

	_, err := client.Get(ConfigGetOperation{
		NacosOperation: &DefaultNacosOperation,
		DataId:         "common.yaml",
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "鉴权失败")
	assert.Equal(t, 0, m.loginCallCount(), "权限不足时不应触发重新登录")
}

// TestStaticTokenSkipsLogin 直接给 accessToken 时不应再登录
func TestStaticTokenSkipsLogin(t *testing.T) {
	m := newMockNacos(t)
	m.addConfig("common.yaml", "hello")
	t.Setenv(EnvTokenCache, filepath.Join(t.TempDir(), "token-cache.json"))

	client := NewClient(&NacosConfig{
		Addr:        m.server.URL + "/nacos",
		ApiVersion:  "v1",
		Namespace:   PublicNamespace,
		Group:       "DEFAULT_GROUP",
		Username:    "nacos",
		Password:    "Nacos@123456",
		AccessToken: "static-token",
	})
	m.mu.Lock()
	m.tokens["static-token"] = true
	m.mu.Unlock()

	_, err := client.Get(ConfigGetOperation{
		NacosOperation: &DefaultNacosOperation,
		DataId:         "common.yaml",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, m.loginCallCount())
}

func TestShouldRelogin(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"token 过期", 403, `{"message":"token expired!"}`, true},
		{"签名不匹配", 403, `{"message":"Invalid signature"}`, true},
		{"token 非法", 403, `{"message":"token invalid!"}`, true},
		{"完全没带 token", 403, `{"message":"user not found!"}`, true},
		{"真的没权限", 403, `{"message":"authorization failed!"}`, false},
		{"其他 403", 403, `{"message":"forbidden"}`, false},
		{"非 403", 500, `{"message":"token expired!"}`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, shouldRelogin(tc.status, []byte(tc.body)))
		})
	}
}

func TestTokenCacheRoundTrip(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "token-cache.json")
	t.Setenv(EnvTokenCache, cacheFile)

	addr := "http://127.0.0.1:8848/nacos"
	saveCachedToken(addr, "nacos", "tok-1", time.Now().Add(time.Hour))

	entry, ok := loadCachedToken(addr, "nacos")
	require.True(t, ok)
	assert.Equal(t, "tok-1", entry.AccessToken)

	// 写入的临时文件必须被清理干净
	entries, err := os.ReadDir(filepath.Dir(cacheFile))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "缓存目录里不应残留临时文件: %v", entries)

	// 缓存里是凭据，权限必须是 0600
	info, err := os.Stat(cacheFile)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	require.NoError(t, removeCachedToken(addr, "nacos"))
	_, ok = loadCachedToken(addr, "nacos")
	assert.False(t, ok)
}

func TestLoadCachedTokenForAddr(t *testing.T) {
	t.Setenv(EnvTokenCache, filepath.Join(t.TempDir(), "token-cache.json"))

	addr := "http://127.0.0.1:8848/nacos"
	other := "http://nacos.other:8848/nacos"
	saveCachedToken(addr, "alice", "tok-alice", time.Now().Add(time.Hour))
	saveCachedToken(other, "bob", "tok-bob", time.Now().Add(2*time.Hour))

	entry, ok := loadCachedTokenForAddr(addr)
	require.True(t, ok)
	assert.Equal(t, "tok-alice", entry.AccessToken, "不应串到别的地址")

	require.NoError(t, removeCachedTokensForAddr(addr))
	_, ok = loadCachedToken(addr, "alice")
	assert.False(t, ok)
	_, ok = loadCachedToken(other, "bob")
	assert.True(t, ok, "不应误删其他地址的缓存")
}

func TestExpiredTokenNotReused(t *testing.T) {
	t.Setenv(EnvTokenCache, filepath.Join(t.TempDir(), "token-cache.json"))

	addr := "http://127.0.0.1:8848/nacos"

	saveCachedToken(addr, "nacos", "tok-old", time.Now().Add(-time.Minute))
	_, ok := loadCachedToken(addr, "nacos")
	assert.False(t, ok)

	// 临近过期的 token 应提前重新登录
	saveCachedToken(addr, "nacos", "tok-soon", time.Now().Add(tokenRenewAhead/2))
	_, ok = loadCachedToken(addr, "nacos")
	assert.False(t, ok)
}

func TestCorruptedTokenCacheIsIgnored(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "token-cache.json")
	t.Setenv(EnvTokenCache, cacheFile)
	require.NoError(t, os.WriteFile(cacheFile, []byte("not-json{{{"), 0o600))

	_, ok := loadCachedToken("http://127.0.0.1:8848/nacos", "nacos")
	assert.False(t, ok)
}

func TestTokenCacheDisabled(t *testing.T) {
	t.Setenv(EnvTokenCache, "off")
	assert.Equal(t, "", TokenCachePath())

	addr := "http://127.0.0.1:8848/nacos"
	saveCachedToken(addr, "nacos", "tok", time.Now().Add(time.Hour))
	_, ok := loadCachedToken(addr, "nacos")
	assert.False(t, ok)
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		EnvAddr, EnvApiVersion, EnvUsername, EnvPassword,
		EnvAccessToken, EnvNamespace, EnvGroup, EnvConfigFile, EnvInsecure,
	} {
		t.Setenv(key, "")
	}
}

func TestResolveConfigPrecedence(t *testing.T) {
	t.Setenv(EnvTokenCache, filepath.Join(t.TempDir(), "token-cache.json"))
	clearEnv(t)

	configFile := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(strings.Join([]string{
		"addr: http://from-file:8848/nacos",
		"username: file-user",
		"password: file-pass",
		"namespace: file-ns",
		"group: FILE_GROUP",
	}, "\n")), 0o600))

	t.Setenv(EnvAddr, "http://from-env:8848/nacos")
	t.Setenv(EnvUsername, "env-user")

	flagAddr := "http://from-flag:8848/nacos"

	config, err := ResolveConfig(ConfigOptions{
		Addr:       &flagAddr,
		ConfigFile: configFile,
	})
	require.NoError(t, err)

	// 命令行 > 环境变量 > 配置文件
	assert.Equal(t, "http://from-flag:8848/nacos", config.Addr)
	assert.Equal(t, "env-user", config.Username)
	assert.Equal(t, "file-pass", config.Password)
	assert.Equal(t, "file-ns", config.Namespace)
	assert.Equal(t, "FILE_GROUP", config.Group)
}

func TestResolveConfigDefaults(t *testing.T) {
	t.Setenv(EnvTokenCache, filepath.Join(t.TempDir(), "token-cache.json"))
	clearEnv(t)

	// 显式指定的配置文件不存在应当报错，避免静默用错配置
	_, err := ResolveConfig(ConfigOptions{ConfigFile: filepath.Join(t.TempDir(), "missing.yaml")})
	assert.Error(t, err)

	config, err := ResolveConfig(ConfigOptions{})
	require.NoError(t, err)
	assert.Equal(t, defaultAddr, config.Addr)
	assert.Equal(t, defaultApiVersion, config.ApiVersion)
	assert.Equal(t, PublicNamespace, config.Namespace)
	assert.Equal(t, "DEFAULT_GROUP", config.Group)
}

func TestConfigDescribeMasksSecrets(t *testing.T) {
	config := &NacosConfig{
		Addr:      "http://127.0.0.1:8848/nacos",
		Username:  "nacos",
		Password:  "super-secret",
		Namespace: PublicNamespace,
		Group:     "DEFAULT_GROUP",
	}
	described := config.Describe()
	assert.NotContains(t, described, "super-secret")
}

func TestTenantOf(t *testing.T) {
	assert.Equal(t, "", tenantOf(""))
	assert.Equal(t, "", tenantOf("public"))
	assert.Equal(t, "", tenantOf("PUBLIC"))
	assert.Equal(t, "dev-ns", tenantOf("dev-ns"))
}
