# nacos-cli

`nacos-cli`是一个命令行工具，用来代替 nacos 的图形界面操作。

![carbon](https://github.com/szpinc/nacos-cli/assets/19821378/2899922a-e7c7-402d-80d4-a6bb27912efc)

> 这是 [szpinc/nacos-cli](https://github.com/szpinc/nacos-cli) 的二开版本，
> **主要增加了对「开启鉴权的 Nacos」的支持**（见下方 [与上游的差异](#与上游的差异)）。

## 安装

### 从源码编译

``` bash
git clone <this-repo> && cd nacos-cli
go build -o nacos-cli .
sudo mv nacos-cli /usr/local/bin/
```

### 交叉编译

``` bash
GOOS=linux GOARCH=amd64 go build -o nacos-cli_linux_amd64 .
GOOS=linux GOARCH=arm64 go build -o nacos-cli_linux_arm64 .
```

## 鉴权（重点）

Nacos 开启鉴权（`nacos.core.auth.enabled=true`）后，所有接口都需要携带 `accessToken`。
本工具会自动完成登录、注入 token 和过期续期，你只需要提供用户名密码。

### 1. 给凭据的三种方式

**方式一：环境变量**（推荐，登录信息不进 shell 历史）

``` bash
export NACOS_ADDR="http://nacos.prod:8848/nacos"
export NACOS_USERNAME="nacos"
export NACOS_PASSWORD="your-password"
```

**方式二：配置文件** `~/.nacos-cli/config.yaml`

``` yaml
addr: http://nacos.prod:8848/nacos
username: nacos
password: your-password
namespace: public          # 命名空间 ID，public 是默认空间
group: DEFAULT_GROUP
apiVersion: v1
insecure: false            # HTTPS 自签证书时设为 true
```

**方式三：命令行参数**

``` bash
nacos-cli get config common.yaml -u nacos -p 'your-password'
```

优先级：**命令行参数 > 环境变量 > 配置文件 > 内置默认值**。

### 2. 登录一次，后续免登录

``` bash
nacos-cli login -u nacos -p 'your-password'
# 登录成功
#   accessToken : eyJhbGciOiJIUzM4......(已省略)
#   有效期      : 18000 秒（约 5.0 小时）
#   全局管理员  : true
```

登录成功后 token 会缓存到 `~/.nacos-cli/token-cache.json`（权限 `0600`），
之后直接敲命令即可，无需再带凭据：

``` bash
nacos-cli get config -A
```

token 过期或服务端密钥轮换时，只要凭据还配置着，工具会**自动重新登录并重试**，无需人工干预。

清除缓存：`nacos-cli logout`

### 3. 直接使用现成 token

如果你从别处拿到了 token（例如 OIDC/LDAP 模式、或 CI 里由上游注入）：

``` bash
nacos-cli get config common.yaml --access-token "$TOKEN"
# 或
export NACOS_ACCESS_TOKEN="$TOKEN"
```

指定 `--access-token` 时工具不会再尝试登录。

### 4. 密码从标准输入读

避免密码出现在 `ps` 和 shell 历史里：

``` bash
echo "$NACOS_PASSWORD" | nacos-cli login -u nacos --password-stdin
```

## 使用

**获取所有配置列表**

``` bash
nacos-cli get config -A
```

**获取指定配置**

``` bash
nacos-cli get config common.yaml -n public -g DEFAULT_GROUP
```

**编辑配置**（调用 `$EDITOR` 打开临时文件，保存后自动回写）

``` bash
nacos-cli edit config common.yaml -n public -g DEFAULT_GROUP
```

**从文件更新配置**

``` bash
nacos-cli apply -f common.yaml -n public -g DEFAULT_GROUP --id common.yaml
```

**删除配置**

``` bash
nacos-cli delete config common.yaml -n public -g DEFAULT_GROUP
```

## 环境变量

| 变量 | 说明 | 默认值 |
| --- | --- | --- |
| `NACOS_ADDR` | Nacos 地址，需带上 context path | `http://127.0.0.1:8848/nacos` |
| `NACOS_USERNAME` | 用户名 | 空 |
| `NACOS_PASSWORD` | 密码 | 空 |
| `NACOS_ACCESS_TOKEN` | 直接指定 token，跳过登录 | 空 |
| `NACOS_NAMESPACE` | 命名空间 ID | `public` |
| `NACOS_GROUP` | 分组 | `DEFAULT_GROUP` |
| `NACOS_API_VERSION` | 接口版本 | `v1` |
| `NACOS_INSECURE` | 跳过 HTTPS 证书校验 | `false` |
| `NACOS_CONFIG_FILE` | 配置文件路径 | `~/.nacos-cli/config.yaml` |
| `NACOS_TOKEN_CACHE` | token 缓存路径；设为 `off` 可关闭缓存 | `~/.nacos-cli/token-cache.json` |

## 常见问题

**报 `鉴权失败 (HTTP 403)：... user not found!`**
说明请求没带上有效 token。检查用户名密码是否配置、是否带对了 `--addr`；
Nacos 2.4.0 之后不再有默认密码，需要用下面的接口先初始化管理员密码：

``` bash
curl -X POST 'http://<nacos>:8848/nacos/v1/auth/users/admin' -d 'password=<your-password>'
```

**报 `鉴权失败 (HTTP 403)：... authorization failed!`**
凭据是有效的，但该账号没有目标命名空间/分组的权限。去 Nacos 控制台给账号授权，而不是改密码。

**`-n public` 和 `-n ""` 有什么区别？**
没有区别。`public` 是控制台显示名，它的真实命名空间 ID 是空字符串，工具会自动转换。
自定义命名空间请传**命名空间 ID**（控制台里能看到），不是显示名。

**HTTPS 自签证书报 x509 错误**
加 `-k/--insecure`，或在配置文件里写 `insecure: true`。

## 与上游的差异

**新增：鉴权支持**

- 自动调用登录接口获取 `accessToken`（`/v1/auth/login`，并兼容 Nacos 3.x 的 `/v3/auth/user/login`），
  注入到所有请求，token 失效自动重新登录重试一次
- 凭据来源支持 命令行参数 / 环境变量 / 配置文件`~/.nacos-cli/config.yaml` 三层优先级
- 新增 `login` / `logout` 子命令，token 落盘缓存（`0600`），避免每条命令都登录一次
- 新增 `--access-token`（直接用现成 token）、`--password-stdin`（密码走管道）、`-k/--insecure`（跳过证书校验）
- 鉴权失败时给出可读提示，区分「密码错」「无权限」「token 过期」

**顺手修的**

- `public` 命名空间：上游把 `public` 当 tenant 传给服务端，导致默认空间永远查不到数据
- 所有请求补上 `resp.Body.Close()`，修掉连接泄漏；HTTP 客户端加 30s 超时
- 「配置不存在」判断改为看响应体：Nacos 2.4.3 返回的是 `200 + 空 body`，不是 404
- `get config -A` 自动翻页（上游固定 `pageSize=999`，超过会被静默截断）
- `edit config` 不带参数时不再 panic；`-A` 时 shell 补全的多余输出已去掉
- 错误不再打印整页 flag 用法

**测试**

`pkg/nacos` 下有 21 个用例，用 `httptest` 起了一个「开启鉴权」的模拟 Nacos，覆盖登录注入、
token 过期自动重登、无权限不重登、命名空间租户转换、配置优先级、缓存读写与权限等。

``` bash
go test -race ./...
```

> 提醒：`go.mod` 里的 module path 沿用了上游的 `github/szpinc/nacosctl`（少了个 `.com`），
> 本地编译没问题，但 `go install` 这类需要按路径拉取的场景会失败。
> 如果要发布自己的版本，建议改成自己的仓库路径。
