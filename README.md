# nacos-cli

用命令行操作 Nacos 配置，代替开图形界面。支持 Nacos **开启鉴权**的部署。

> 这是 [szpinc/nacos-cli](https://github.com/szpinc/nacos-cli) 的二开版本。
> 上游版本所有请求都是裸 `http.Get`，配置里的 `Username`/`Password` 从未被使用，
> 所以 Nacos 一旦开启鉴权（`nacos.core.auth.enabled=true`）就完全不可用。
> 本版本补上了完整鉴权支持，改动清单见文末 [与上游的差异](#与上游的差异)。

---

## 一、编译（在 Mac 上编出 x86 服务器能跑的二进制）

只用标准库 + 几个纯 Go 依赖，**没有 cgo**，交叉编译出来是静态链接的 ELF，
直接 `scp` 到 x86_64 Linux 就能跑，目标机器不需要装 Go。

``` bash
cd nacos-cli

# x86_64 服务器（最常用）
GOOS=linux GOARCH=amd64 go build -trimpath -o nacos-cli_linux_amd64 .

# 如果服务器是 arm64
GOOS=linux GOARCH=arm64 go build -trimpath -o nacos-cli_linux_arm64 .

# 本机 Mac 自己用（Apple Silicon）
GOOS=darwin GOARCH=arm64 go build -trimpath -o nacos-cli_darwin_arm64 .

# 看架构对不对
file nacos-cli_linux_amd64
# -> ELF 64-bit LSB executable, x86-64, statically linked
```

> `go` 不在 PATH 里的话写全路径 `/usr/local/go/bin/go`。
> 首次编译要从网上下依赖，拉不动就先 `export all_proxy=http://127.0.0.1:7890`。

已经编好的三个平台产物在 `dist/` 目录里，可以直接拿走：

``` bash
scp dist/nacos-cli_linux_amd64 user@your-server:/usr/local/bin/nacos-cli
ssh user@your-server 'chmod +x /usr/local/bin/nacos-cli && nacos-cli --help'
```

## 二、给凭据（三选一）

### 方式一：环境变量（推荐）

登录信息不进 shell 历史，也不会出现在 `ps` 里。

``` bash
export NACOS_ADDR="http://nacos.prod:8848/nacos"
export NACOS_USERNAME="nacos"
export NACOS_PASSWORD="your-password"
```

想固化到 `~/.bashrc` 里就写进去（注意 `chmod 600`，别留在 `~/.bash_history`）。

### 方式二：配置文件 `~/.nacos-cli/config.yaml`

``` bash
mkdir -p ~/.nacos-cli
cp config.example.yaml ~/.nacos-cli/config.yaml
chmod 600 ~/.nacos-cli/config.yaml
vi ~/.nacos-cli/config.yaml
```

``` yaml
addr: http://nacos.prod:8848/nacos
username: nacos
password: your-password
namespace: public          # 命名空间 ID，public 是默认空间
group: DEFAULT_GROUP
apiVersion: v1
insecure: false            # HTTPS 自签证书时设为 true
```

### 方式三：单条命令直接给

``` bash
nacos-cli get config common.yaml --addr http://nacos.prod:8848/nacos -u nacos -p 'your-password'
```

优先级：**命令行参数 > 环境变量 > 配置文件 > 内置默认值**。

## 三、用

**先登录一次，之后不用再带凭据**

``` bash
nacos-cli login -u nacos -p 'your-password'
```

```
使用以下配置登录 Nacos：
  地址        : http://nacos.prod:8848/nacos
  命名空间    : public
  分组        : DEFAULT_GROUP
  用户名      : nacos
  密码        : (已设置)
  静态 token  : (未设置)

登录成功
  accessToken : eyJhbGciOiJIUzM4......(已省略)
  有效期      : 18000 秒（约 5.0 小时）
  全局管理员  : true
  用户名      : nacos

token 已缓存到 /root/.nacos-cli/token-cache.json，后续命令无需重复登录

[提示] 试读配置列表成功，当前命名空间共 42 条配置
```

token 缓存到 `~/.nacos-cli/token-cache.json`（权限 `0600`）。
**过期或服务端密钥轮换时会自动重新登录并重试**，不需要人工干预。
不想留缓存就 `nacos-cli logout`，或 `export NACOS_TOKEN_CACHE=off`。

**常用命令**

``` bash
# 列出全部配置（跨分组）
nacos-cli get config -A

# 只列某个分组
nacos-cli get config -A -g MY_GROUP

# 读一条配置，直接打到 stdout，可以管道给别的命令
nacos-cli get config common.yaml -n public -g DEFAULT_GROUP

# 从本地文件发布/更新配置（dataId 默认取文件名）
nacos-cli apply -f common.yaml -g DEFAULT_GROUP

# 发布并指定 dataId / group
nacos-cli apply -f /path/to/app.yaml --id app.yaml -g MY_GROUP

# 用 $EDITOR 打开配置改完自动回写
nacos-cli edit config common.yaml -g DEFAULT_GROUP

# 删除
nacos-cli delete config common.yaml -g DEFAULT_GROUP
```

**密码不想进 shell 历史**（CI 里常用）

``` bash
echo "$NACOS_PASSWORD" | nacos-cli login -u nacos --password-stdin
```

**已经有现成 token**（比如 OIDC/LDAP 模式，或由上游 CI 注入）

``` bash
nacos-cli get config common.yaml --access-token "$TOKEN"
# 或
export NACOS_ACCESS_TOKEN="$TOKEN"
```

给了 `--access-token` 就不会再尝试登录。

## 四、参数与环境变量

| 命令行参数 | 环境变量 | 说明 | 默认值 |
| --- | --- | --- | --- |
| `--addr` | `NACOS_ADDR` | Nacos 地址，**要带 context path** | `http://127.0.0.1:8848/nacos` |
| `-u, --username` | `NACOS_USERNAME` | 用户名 | 空 |
| `-p, --password` | `NACOS_PASSWORD` | 密码 | 空 |
| `--password-stdin` | — | 从标准输入读密码 | `false` |
| `--access-token` | `NACOS_ACCESS_TOKEN` | 直接用现成 token，跳过登录 | 空 |
| `-n, --namespace` | `NACOS_NAMESPACE` | 命名空间 **ID** | `public` |
| `-g, --group` | `NACOS_GROUP` | 分组 | `DEFAULT_GROUP` |
| `--api-version` | `NACOS_API_VERSION` | 接口版本 | `v1` |
| `-k, --insecure` | `NACOS_INSECURE` | 跳过 HTTPS 证书校验 | `false` |
| `--config` | `NACOS_CONFIG_FILE` | 配置文件路径 | `~/.nacos-cli/config.yaml` |
| — | `NACOS_TOKEN_CACHE` | token 缓存路径；设 `off` 关闭缓存 | `~/.nacos-cli/token-cache.json` |

子命令：`get` / `edit` / `apply` / `delete` / `login` / `logout`。

## 五、排查

**`鉴权失败 (HTTP 403)：... user not found!`**
请求没带上有效 token。先确认 `--addr` 指向对的环境，再确认用户名密码配上了。
如果密码是对的，很可能是 **Nacos 2.4.0 之后没有默认密码**，需要先初始化管理员密码：

``` bash
curl -X POST 'http://<nacos>:8848/nacos/v1/auth/users/admin' -d 'password=<your-password>'
```

**`鉴权失败 (HTTP 403)：... authorization failed!`**
凭据是有效的，但这个账号没有目标命名空间/分组的权限。
去 Nacos 控制台给账号授权，改密码没用（工具也不会因此反复重登）。

**`--addr` 到底带不带 `/nacos`？**
带。写 `http://host:8848/nacos`。工具会在后面拼 `/v1/cs/configs`。

**`-n` 要写命名空间 ID 还是名字？**
ID。控制台「命名空间」页面里那一列。默认空间比较特殊：显示名是 `public`，
真实 ID 是空字符串，工具会自动转换，所以 `-n public` 和不带 `-n` 是一个意思。

**HTTPS 自签证书报 x509 错误**
加 `-k`，或配置文件里写 `insecure: true`。

**`配置不存在: namespace=... group=... dataId=...`**
注意 `dataId` 和 `group` 要完全对得上（`DEFAULT_GROUP` 不是 `default_group`）。
这条报错本身说明鉴权已经通过了。

## 六、开发

``` bash
go test -race ./...     # 21 个用例，全绿
go vet ./...
```

`pkg/nacos` 下用 `httptest` 起了一个「开启鉴权」的模拟 Nacos，覆盖：登录并注入 token、
token 失效自动重登、无权限时不重登、静态 token 不登录、public 命名空间租户转换、
凭据三层优先级、token 缓存读写与 `0600` 权限、损坏缓存容错。

## 与上游的差异

**新增：鉴权支持**

- 自动调用登录接口取 `accessToken` 并注入所有请求。登录路径按 `ApiVersion` 依次尝试
  `/v1/auth/login`（Nacos 1.x/2.x）与 `/v3/auth/user/login`（Nacos 3.x），跨大版本可用
- token 失效自动清缓存、重新登录、重试一次。失效判定按实测的服务端文案来区分：
  `token expired!` / `Invalid signature` / `token invalid!` / `user not found!` 才重登，
  `authorization failed!`（真的没权限）不重登
- 凭据来源三层优先级：命令行参数 / 环境变量 / `~/.nacos-cli/config.yaml`
- 新增 `login`、`logout` 子命令；token 落盘缓存（`0600`、原子写），避免每条命令都登录一次
- 新增 `--access-token`、`--password-stdin`、`--addr`、`--api-version`、`--config`、`-k/--insecure`
- 鉴权失败给出可读提示，明确区分「密码错」「无权限」「token 过期」

**顺手修的问题**

- `public` 命名空间：上游把 `"public"` 当 tenant 传给服务端，导致默认空间永远查不到数据
- 所有请求补上 `resp.Body.Close()`，修连接泄漏；HTTP 客户端加 30s 超时
- 「配置不存在」改为按响应体判断——Nacos 2.4.3 返回的是 `200 + 空 body`，不是 404
- `get config -A` 自动翻页，上游固定 `pageSize=999`，超过会被静默截断
- `edit config` 不带参数不再 panic；去掉补全回调里的多余输出；运行时错误不再刷整页 `--help`

> 提醒：`go.mod` 里的 module path 沿用了上游的 `github/szpinc/nacosctl`（少个 `.com`）。
> 本地编译、交叉编译都没问题，但 `go install` 这类按路径拉取的用法会失败。
> 如果要发布自己的版本，改成自己的仓库路径更稳妥。

## LICENSE

跟随上游，见 [LICENSE](LICENSE)。
