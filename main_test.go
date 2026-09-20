// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Yongheng Liu

package main

import "testing"

func TestResolveVersion(t *testing.T) {
	original := version
	t.Cleanup(func() { version = original })

	// 构建时注入的版本号优先级最高
	version = "v9.9.9"
	if got := resolveVersion(); got != "v9.9.9" {
		t.Fatalf("注入版本未生效: got %q, 期望 v9.9.9", got)
	}

	// 未注入时回退读取构建信息；go test 环境下取不到模块版本，应仍是 dev
	version = "dev"
	if got := resolveVersion(); got == "" {
		t.Fatal("版本号不该为空")
	}
}
