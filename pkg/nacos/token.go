package nacos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// defaultCacheDir 本地缓存目录名（位于用户主目录下）
	defaultCacheDir = ".nacos-cli"
	// tokenCacheFileName token 缓存文件名
	tokenCacheFileName = "token-cache.json"
)

// cachedToken 一条本地缓存的 token
type cachedToken struct {
	AccessToken string    `json:"accessToken"`
	Username    string    `json:"username"`
	ExpireAt    time.Time `json:"expireAt"`
}

type tokenCacheFile struct {
	Tokens map[string]cachedToken `json:"tokens"`
}

// cacheKey 以 addr+username 为键，同一台机器连多个环境/多个账号互不干扰
func cacheKey(addr, username string) string {
	return strings.TrimRight(addr, "/") + "|" + username
}

// TokenCachePath 返回当前生效的 token 缓存文件路径，空串表示缓存已关闭
func TokenCachePath() string {
	return tokenCachePath()
}

// tokenCachePath 返回 token 缓存文件路径。
//
//	NACOS_TOKEN_CACHE 未设置      -> ~/.nacos-cli/token-cache.json
//	NACOS_TOKEN_CACHE=off 等      -> 关闭缓存（返回空串）
//	NACOS_TOKEN_CACHE=/some/path  -> 使用指定路径
func tokenCachePath() string {
	switch v := strings.TrimSpace(os.Getenv(EnvTokenCache)); strings.ToLower(v) {
	case "off", "none", "false", "0", "disable", "disabled":
		return ""
	case "":
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, defaultCacheDir, tokenCacheFileName)
	default:
		return v
	}
}

// loadCachedToken 读取指定 地址+用户名 的缓存 token；不存在/已过期/文件损坏都按「没有」处理
func loadCachedToken(addr, username string) (cachedToken, bool) {
	if addr == "" || username == "" {
		return cachedToken{}, false
	}
	entry, ok := readTokenCache()[cacheKey(addr, username)]
	if !ok {
		return cachedToken{}, false
	}
	if entry.AccessToken == "" || !isTokenUsable(entry) {
		return cachedToken{}, false
	}
	return entry, true
}

// loadCachedTokenForAddr 在没指定用户名时，取该地址下最近到期的一条缓存 token。
// 这样 `nacos-cli login` 之后直接 `nacos-cli get config -A` 也能跑通。
func loadCachedTokenForAddr(addr string) (cachedToken, bool) {
	if addr == "" {
		return cachedToken{}, false
	}
	var (
		best  cachedToken
		found bool
	)
	for key, entry := range readTokenCache() {
		if !strings.HasPrefix(key, cacheKey(addr, "")) {
			continue
		}
		if entry.AccessToken == "" || !isTokenUsable(entry) {
			continue
		}
		if !found || entry.ExpireAt.After(best.ExpireAt) {
			best, found = entry, true
		}
	}
	return best, found
}

// isTokenUsable 剩余有效期还够，才值得复用
func isTokenUsable(entry cachedToken) bool {
	return time.Now().Before(entry.ExpireAt.Add(-tokenRenewAhead))
}

// readTokenCache 读取整个缓存文件，任何异常都退化成空缓存
func readTokenCache() map[string]cachedToken {
	path := tokenCachePath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	cache := tokenCacheFile{}
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil
	}
	return cache.Tokens
}

// saveCachedToken 写入 token 缓存，尽力而为：失败不影响命令正常执行。
// 采用「写临时文件 + rename」保证不会读到半个文件。
func saveCachedToken(addr, username, token string, expireAt time.Time) {
	updateTokenCache(func(cache *tokenCacheFile) {
		cache.Tokens[cacheKey(addr, username)] = cachedToken{
			AccessToken: token,
			Username:    username,
			ExpireAt:    expireAt,
		}
	})
}

// removeCachedToken 删除某个 地址+用户名 的缓存
func removeCachedToken(addr, username string) error {
	if addr == "" {
		return nil
	}
	updateTokenCache(func(cache *tokenCacheFile) {
		delete(cache.Tokens, cacheKey(addr, username))
	})
	return nil
}

// removeCachedTokensForAddr 删除该地址下所有用户的缓存（logout 用）
func removeCachedTokensForAddr(addr string) error {
	if addr == "" {
		return nil
	}
	prefix := cacheKey(addr, "")
	updateTokenCache(func(cache *tokenCacheFile) {
		for key := range cache.Tokens {
			if strings.HasPrefix(key, prefix) {
				delete(cache.Tokens, key)
			}
		}
	})
	return nil
}

// updateTokenCache 读-改-写缓存文件
func updateTokenCache(mutate func(cache *tokenCacheFile)) {
	path := tokenCachePath()
	if path == "" {
		return
	}

	cache := tokenCacheFile{Tokens: map[string]cachedToken{}}
	if data, err := os.ReadFile(path); err == nil {
		loaded := tokenCacheFile{}
		if err := json.Unmarshal(data, &loaded); err == nil && loaded.Tokens != nil {
			cache = loaded
		}
	}

	mutate(&cache)

	// 顺手清掉已经过期的条目，避免文件无限膨胀
	now := time.Now()
	for k, v := range cache.Tokens {
		if !now.Before(v.ExpireAt) {
			delete(cache.Tokens, k)
		}
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), tokenCacheFileName+".tmp*")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	// 缓存里是凭据，权限收窄到 0600
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmpName, path)
}
