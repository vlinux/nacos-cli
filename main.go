// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Yongheng Liu

package main

import (
	"runtime/debug"

	"github.com/vlinux/nacos-cli/cmd"
)

// version 由构建时注入：
//
//	go build -ldflags "-X main.version=v1.0.0"
//
// 未注入时（例如 go install github.com/vlinux/nacos-cli@v1.0.0）
// 回退到 Go 构建信息里的模块版本，两者都取不到才显示 dev。
var version = "dev"

// resolveVersion 返回最终展示用的版本号。
//
// `go install module@vX.Y.Z` 无法注入 ldflags，此时只能从构建信息里读模块版本，
// 否则用户看到的是 dev，无法确认自己装的到底是哪个版本。
func resolveVersion() string {
	if version != "dev" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return version
	}
	return info.Main.Version
}

func main() {
	cmd.SetVersion(resolveVersion())
	cmd.Execute()
}
