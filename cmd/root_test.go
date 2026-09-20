package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadPasswordFromStdin(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{name: "LF 结尾", in: "s3cret\n", want: "s3cret"},
		{name: "CRLF 结尾", in: "s3cret\r\n", want: "s3cret"},
		{name: "没有换行结尾", in: "s3cret", want: "s3cret"},
		{name: "保留密码里的空格", in: "  p a s s  \n", want: "  p a s s  "},
		{name: "空行", in: "\n", wantErr: "密码为空"},
		{name: "EOF", in: "", wantErr: "读取密码失败"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readPasswordFromStdin(strings.NewReader(tc.in))
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("期望报错包含 %q，但成功了（got=%q）", tc.wantErr, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("错误信息 %q 不含 %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("不该报错: %v", err)
			}
			if got != tc.want {
				t.Fatalf("密码 = %q，期望 %q", got, tc.want)
			}
		})
	}
}

// TestInitClientReadsStdinPasswordOnlyOnce 锁住一个真实踩过的坑：
// initClient 被重复调用时（PersistentPreRunE 与子命令 RunE 各调一次），
// 第二次读 stdin 会拿到 EOF，导致 --password-stdin 直接失败。
func TestInitClientReadsStdinPasswordOnlyOnce(t *testing.T) {
	t.Setenv("NACOS_ADDR", "http://127.0.0.1:8848/nacos")
	t.Setenv("NACOS_USERNAME", "nacos")
	t.Setenv("NACOS_TOKEN_CACHE", "off")

	prevFlag, prevRead, prevPwd := flagPasswordStdin, stdinPasswordRead, stdinPassword
	flagPasswordStdin, stdinPasswordRead, stdinPassword = true, false, ""
	t.Cleanup(func() {
		flagPasswordStdin, stdinPasswordRead, stdinPassword = prevFlag, prevRead, prevPwd
	})

	rootCmd.SetIn(strings.NewReader("s3cret\n"))

	if err := initClient(rootCmd); err != nil {
		t.Fatalf("第一次 initClient 失败: %v", err)
	}
	if got := nacosClient.Config.Password; got != "s3cret" {
		t.Fatalf("第一次解析出的密码 = %q，期望 s3cret", got)
	}

	if err := initClient(rootCmd); err != nil {
		t.Fatalf("重复调用 initClient 不应失败（stdin 已被读空）: %v", err)
	}
	if got := nacosClient.Config.Password; got != "s3cret" {
		t.Fatalf("重复调用后密码丢失: %q，期望 s3cret", got)
	}
}

// TestLoginWithPasswordStdin 端到端回归：起一个开启鉴权的模拟 Nacos，
// 用 `login --password-stdin` 走完整命令链路，确认只消费一次 stdin。
func TestLoginWithPasswordStdin(t *testing.T) {
	const password = "s3cret"

	var loginCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/login"):
			loginCalls++
			_ = r.ParseForm()
			if r.PostFormValue("username") != "nacos" || r.PostFormValue("password") != password {
				w.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(w, `{"message":"user not found!"}`)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w,
				`{"accessToken":"tok-from-mock","tokenTtl":18000,"globalAdmin":true,"username":"nacos"}`)
		case strings.HasSuffix(r.URL.Path, "/cs/configs"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w,
				`{"totalCount":0,"pageNumber":1,"pagesAvailable":0,"pageItems":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Setenv("NACOS_ADDR", srv.URL)
	t.Setenv("NACOS_USERNAME", "nacos")
	t.Setenv("NACOS_TOKEN_CACHE", "off")

	prevFlag, prevRead, prevPwd := flagPasswordStdin, stdinPasswordRead, stdinPassword
	prevNS, prevGroup := namespace, group
	t.Cleanup(func() {
		flagPasswordStdin, stdinPasswordRead, stdinPassword = prevFlag, prevRead, prevPwd
		namespace, group = prevNS, prevGroup
		rootCmd.SetArgs(nil)
		rootCmd.SetIn(nil)
	})
	stdinPasswordRead, stdinPassword = false, ""

	rootCmd.SetArgs([]string{"login", "--password-stdin"})
	rootCmd.SetIn(strings.NewReader(password + "\n"))

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("login --password-stdin 失败: %v", err)
	}
	if loginCalls != 1 {
		t.Fatalf("登录接口被调用 %d 次，期望 1 次", loginCalls)
	}
}
