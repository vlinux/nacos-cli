# nacos-cli

[![CI](https://github.com/vlinux/nacos-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/vlinux/nacos-cli/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/vlinux/nacos-cli)](https://github.com/vlinux/nacos-cli/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/vlinux/nacos-cli)](go.mod)
[![License](https://img.shields.io/github/license/vlinux/nacos-cli)](LICENSE)

用终端管理 Nacos 配置，替代打开图形界面点点点。**完整支持开启了鉴权（`nacos.core.auth.enabled=true`）的 Nacos 服务端。**

> A command-line client for Nacos with full authentication support.
> 本项目基于 [szpinc/nacos-cli](https://github.com/szpinc/nacos-cli) 二次开发，详见 [与上游的区别](#与上游的区别)。

## 特性

- **鉴权开箱可用** —— 提供用户名密码即可，自动登录获取 `accessToken`、注入到每个请求、过期自动续期
- **token 本地缓存** —— 登录一次即可，不必每条命令都重新登录
- **凭据三级优先级** —— 命令行参数 > 环境变量 > 配置文件，方便在本地、跳板机、CI 里用不同方式注入
- **兼容未开鉴权的 Nacos** —— 没配置凭据时行为与普通客户端一致
- **配置增删改查** —— 支持多命名空间、多分组，列表自动翻页
- **单个静态二进制** —— 无运行时依赖，拷到服务器即可运行

## 安装

### 下载预编译二进制

``` bash
# Linux x86_64
curl -L -o /usr/local/bin/nacos-cli \
  https://github.com/vlinux/nacos-cli/releases/latest/download/nacos-cli_linux_amd64

# Linux arm64
curl -L -o /usr/local/bin/nacos-cli \
  https://github.com/vlinux/nacos-cli/releases/latest/download/nacos-cli_linux_arm64

# macOS (Apple Silicon)
curl -L -o /usr/local/bin/nacos-cli \
  https://github.com/vlinux/nacos-cli/releases/latest/download/nacos-cli_darwin_arm64

chmod +x /usr/local/bin/nacos-cli
nacos-cli --version
```

### 使用 go install

``` bash
go install github.com/vlinux/nacos-cli@latest
```

### 从源码构建

``` bash
git clone https://github.com/vlinux/nacos-cli.git
cd nacos-cli
make build            # 当前平台
make dist             # 交叉编译三个平台，产物在 dist/
make install          # 装到 /usr/local/bin
```

也可以直接用 `go build`：

``` bash
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o nacos-cli .
```

## 快速开始

``` bash
# 1. 指向你的 Nacos，并给它凭据
export NACOS_ADDR="http://nacos.example.com:8848/nacos"
export NACOS_USERNAME="nacos"
export NACOS_PASSWORD="your-password"

# 2. 列出现有配置
nacos-cli get config

# 3. 读一条配置
nacos-cli get config app.yaml

# 4. 发布/更新配置
nacos-cli apply -f app.yaml

# 5. 删除配置
nacos-cli delete config app.yaml
```

> `NACOS_ADDR` 需要带上 context path，即 `http://host:8848/nacos`，工具会在后面拼 `/v1/cs/configs`。

## 鉴权

Nacos 开启鉴权后，所有接口都要求携带 `accessToken`。本工具会自动完成登录、注入和续期。

### 方式一：环境变量

推荐方式，凭据不会出现在 shell 历史或 `ps` 输出里。

``` bash
export NACOS_ADDR="http://nacos.example.com:8848/nacos"
export NACOS_USERNAME="nacos"
export NACOS_PASSWORD="your-password"
```

### 方式二：配置文件

``` bash
mkdir -p ~/.nacos-cli
cp config.example.yaml ~/.nacos-cli/config.yaml
chmod 600 ~/.nacos-cli/config.yaml
```

``` yaml
addr: http://nacos.example.com:8848/nacos
username: nacos
password: your-password
namespace: public          # 命名空间 ID
group: DEFAULT_GROUP
apiVersion: v1
insecure: false            # HTTPS 自签证书时设为 true
```

### 方式三：命令行参数

``` bash
nacos-cli get config app.yaml --addr http://nacos.example.com:8848/nacos -u nacos -p 'your-password'
```

优先级：**命令行参数 > 环境变量 > 配置文件 > 内置默认值**。

### 登录一次，后续免登录

``` bash
nacos-cli login
```

```
登录成功
  accessToken : eyJhbGciOiJIUzM4......(已省略)
  有效期      : 18000 秒（约 5.0 小时）
  全局管理员  : true
  用户名      : nacos
```

登录成功后 token 会缓存到 `~/.nacos-cli/token-cache.json`（权限 `0600`），之后直接敲命令即可。
token 过期或服务端密钥轮换时，只要凭据仍配置着，工具会**自动重新登录并重试**，无需人工干预。

``` bash
nacos-cli logout                 # 清除缓存的 token
export NACOS_TOKEN_CACHE=off     # 或者完全关闭缓存
```

### 密码从标准输入读取

避免密码出现在 `ps` 和 shell 历史里，适合 CI：

``` bash
echo "$NACOS_PASSWORD" | nacos-cli login -u nacos --password-stdin
```

### 使用现成的 token

如果 token 由别处签发（OIDC / LDAP 模式，或由 CI 注入）：

``` bash
nacos-cli get config app.yaml --access-token "$TOKEN"
# 或
export NACOS_ACCESS_TOKEN="$TOKEN"
```

指定 `--access-token` 时不会再尝试登录。

## 命令

| 命令 | 说明 |
| --- | --- |
| `nacos-cli get config` | 列出当前命名空间下的配置 |
| `nacos-cli get config <dataId>` | 打印单条配置的内容 |
| `nacos-cli apply -f <file>` | 从本地文件发布或更新配置 |
| `nacos-cli edit config <dataId>` | 用 `$EDITOR` 打开配置，保存后自动回写 |
| `nacos-cli delete config <dataId>` | 删除配置 |
| `nacos-cli login` | 登录并缓存 `accessToken` |
| `nacos-cli logout` | 清除本地缓存的 `accessToken` |
| `nacos-cli completion <shell>` | 生成 shell 补全脚本 |

``` bash
# 列出全部配置（省略 dataId 即可，-A/--all 是等价写法）
nacos-cli get config

# 只列某个分组
nacos-cli get config -g MY_GROUP

# 指定命名空间与分组
nacos-cli get config app.yaml -n dev -g MY_GROUP

# 发布：dataId 默认取文件名，类型按后缀推断
nacos-cli apply -f /path/to/app.yaml --id app.yaml -g MY_GROUP

# 与其他命令配合
nacos-cli get config app.yaml | grep -A2 'redis:'
```

开启 shell 补全后，`get config <TAB>` 会补全服务端已有的 dataId：

``` bash
source <(nacos-cli completion bash)    # 写入 ~/.bashrc 可永久生效
source <(nacos-cli completion zsh)     # zsh
```

## 配置项

### 命令行参数

| 参数 | 说明 | 默认值 |
| --- | --- | --- |
| `--addr` | Nacos 地址，**需带 context path** | `http://127.0.0.1:8848/nacos` |
| `-n, --namespace` | 命名空间 **ID** | `public` |
| `-g, --group` | 分组 | `DEFAULT_GROUP` |
| `-u, --username` | 用户名 | 空 |
| `-p, --password` | 密码 | 空 |
| `--password-stdin` | 从标准输入读取密码 | `false` |
| `--access-token` | 直接指定 `accessToken`，跳过登录 | 空 |
| `--api-version` | 接口版本 | `v1` |
| `-k, --insecure` | 跳过 HTTPS 证书校验 | `false` |
| `--config` | 配置文件路径 | `~/.nacos-cli/config.yaml` |

### 环境变量

| 变量 | 对应参数 |
| --- | --- |
| `NACOS_ADDR` | `--addr` |
| `NACOS_NAMESPACE` | `--namespace` |
| `NACOS_GROUP` | `--group` |
| `NACOS_USERNAME` | `--username` |
| `NACOS_PASSWORD` | `--password` |
| `NACOS_ACCESS_TOKEN` | `--access-token` |
| `NACOS_API_VERSION` | `--api-version` |
| `NACOS_INSECURE` | `--insecure` |
| `NACOS_CONFIG_FILE` | `--config` |
| `NACOS_TOKEN_CACHE` | token 缓存路径；设为 `off` 关闭缓存 |

## 常见问题

**`鉴权失败 (HTTP 403)：... user not found!`**

请求没有携带有效 token。确认 `--addr` 指向正确的环境、凭据已配置。
若密码确认无误，可能是 Nacos 2.4.0 之后**不再内置默认密码**，需要先初始化管理员密码：

``` bash
curl -X POST 'http://<nacos>:8848/nacos/v1/auth/users/admin' -d 'password=<your-password>'
```

**`鉴权失败 (HTTP 403)：... authorization failed!`**

凭据有效，但该账号没有目标命名空间/分组的权限。需要在 Nacos 控制台为账号授权；
工具不会因为这个错误反复重登。

**`配置不存在: namespace=... group=... dataId=...`**

`dataId` 与 `group` 需要完全匹配（`DEFAULT_GROUP` 而非 `default_group`）。
这条错误本身说明鉴权已经通过。

**配置文件里的 `namespace` 生效了吗？**

`nacos-cli get config` 输出表格的最后一列 `NAMESPACE` 就是实际使用的命名空间。
命令行 `-n` 会覆盖配置文件中的 `namespace`。

**`-n` 该写命名空间名称还是 ID？**

ID，即控制台命名空间列表里的那一列。默认空间比较特殊：显示名是 `public`，
真实 ID 是空字符串，工具会自动转换，因此 `-n public` 与不传 `-n` 等价。

**HTTPS 自签证书报 x509 错误**

加 `-k`，或在配置文件中设置 `insecure: true`。

## 与上游的区别

本项目 fork 自 [szpinc/nacos-cli](https://github.com/szpinc/nacos-cli)。
上游版本所有请求均为裸 `http.Get`，`NacosConfig` 中的 `Username`/`Password` 字段从未被使用，
因此无法连接开启鉴权的 Nacos。本版本补齐了这部分能力。

**新增**

- 自动调用登录接口获取 `accessToken` 并注入所有请求。登录路径按 `ApiVersion` 依次尝试
  `/v1/auth/login`（Nacos 1.x/2.x）与 `/v3/auth/user/login`（Nacos 3.x），跨大版本可用
- token 失效自动清缓存、重新登录、重试一次。失效判定依据服务端返回文案区分：
  `token expired!` / `Invalid signature` / `token invalid!` / `user not found!` 会触发重登，
  而 `authorization failed!`（确实无权限）不会
- 凭据三级优先级解析：命令行参数 / 环境变量 / `~/.nacos-cli/config.yaml`
- `login`、`logout` 子命令；token 落盘缓存（`0600`、原子写）
- `--access-token`、`--password-stdin`、`--addr`、`--api-version`、`--config`、`-k/--insecure` 参数
- 鉴权失败时输出可读提示，区分「密码错误」「权限不足」「token 过期」

**修复**

- `public` 命名空间：上游将 `"public"` 作为 tenant 传给服务端，而 public 的真实命名空间 ID
  是空字符串，导致默认空间查询不到数据
- `get config` 省略 `dataId` 时改为列出配置列表，不再报 `data id required`
  （上游必须额外加 `-A` 才能列出，该报错容易被误解为命名空间未生效）
- `-A` 与 `dataId` 同时传入时明确报错，不再静默忽略 `dataId`
- 所有请求补充 `resp.Body.Close()`，修复连接泄漏；HTTP 客户端增加 30s 超时
- 「配置不存在」改为按响应体判断——Nacos 2.4.3 返回 `200 + 空 body`，而非 404
- `get config -A` 自动翻页，修复上游固定 `pageSize=999` 导致的静默截断
- `edit config` 不带参数不再 panic；移除补全回调中的多余输出；运行时报错不再打印整页 usage
- 清理 cobra 生成器遗留的占位版权声明，补齐各子命令的 `--help` 与示例

## 开发

``` bash
make build     # 编译到当前目录
make dist      # 交叉编译三个平台到 dist/
make test      # go test -race ./...
make vet       # go vet ./...
make install   # 安装到 /usr/local/bin
```

`make` 会在 `go` 不在 `PATH` 时自动回退到 `/usr/local/go/bin/go`，也可显式指定：

``` bash
make build GO=/path/to/go
```

测试位于 `pkg/nacos`，使用 `httptest` 启动一个「开启鉴权」的模拟 Nacos，覆盖登录并注入 token、
token 失效自动重登、无权限时不重登、静态 token 不登录、`public` 命名空间租户转换、
凭据三级优先级、token 缓存读写与 `0600` 权限、损坏缓存容错等场景。

## 许可证

[MIT](LICENSE)

## 致谢

- [szpinc/nacos-cli](https://github.com/szpinc/nacos-cli) —— 本项目的上游
- [Alibaba Nacos](https://github.com/alibaba/nacos) —— 服务端
- [spf13/cobra](https://github.com/spf13/cobra) —— 命令行框架
