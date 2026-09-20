// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Yongheng Liu

package main

import "github.com/vlinux/nacos-cli/cmd"

// version 由构建时注入：
//
//	go build -ldflags "-X main.version=v1.0.0"
//
// 未注入时显示 dev。
var version = "dev"

func main() {
	cmd.SetVersion(version)
	cmd.Execute()
}
