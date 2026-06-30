[English](README_EN.md) | 中文

# imgp

**imgp** 是一个跨平台的 Docker 镜像拉取和导出工具。它可以让你从 Docker 镜像仓库（如 Docker Hub、quay.io、gcr.io 等）拉取镜像，并导出为标准 `.tar` 文件。这个 `.tar` 文件可以用 `docker load` 导入到 Docker 中。

> 你不需要在电脑上安装 Docker 就能使用 imgp 拉取镜像。imgp 是纯 Go 编译的单文件，下载即用。

## 常见场景

| 场景 | 用 imgp 怎么做 |
|---|---|
| 我在没有 Docker 的机器上想下载一个镜像 | `imgp save nginx:latest -o nginx.tar`，然后把 tar 传到目标机器用 `docker load` 导入 |
| 国内下载 Docker Hub 镜像太慢 | imgp 内置国内镜像加速，自动走高速通道 |
| 我的树莓派是 arm64 架构，想拉取 arm64 镜像 | `imgp save nginx:latest --platform linux/arm64 -o nginx-arm64.tar` |
| 公司内网的私有仓库需要登录 | `imgp save private/app:latest --username user --password-env` |
| 下载到一半断了，不想重新下载 | imgp 自动缓存已下载的 layer，中断后继续下载会跳过已完成的 |

## 安装

### 方法一：下载二进制（推荐）

从 [Releases 页面](https://gitcode.com/DonaldTom/imgp/releases) 下载对应你操作系统的文件：

| 操作系统 | 下载文件 |
|---|---|
| Windows (64位) | `imgp-windows-amd64.exe` |
| Windows (ARM) | `imgp-windows-arm64.exe` |
| Linux (64位) | `imgp-linux-amd64` |
| Linux (ARM64) | `imgp-linux-arm64` |
| macOS (Intel) | `imgp-darwin-amd64` |
| macOS (Apple Silicon) | `imgp-darwin-arm64` |

下载后：

**Windows**：把 `.exe` 文件放到任意目录，建议放到 `C:\Users\你的用户名\go\bin\` 或其他已加入 `PATH` 的目录。或者直接在文件所在目录打开 PowerShell 运行 `.\imgp-windows-amd64.exe save ...`。

**Linux / macOS**：

```bash
chmod +x imgp-linux-amd64
sudo mv imgp-linux-amd64 /usr/local/bin/imgp
```

### 方法二：用 Go 安装（需要安装 Go 语言）

```bash
go install gitcode.com/DonaldTom/imgp@latest
```

> 国内用户如果遇到网络问题，先设置 Go proxy：
> ```bash
> go env -w GOPROXY=https://goproxy.cn,direct
> ```

### 验证安装

打开命令行，运行：

```bash
imgp -v
```

如果看到输出版本号，说明安装成功。

## 快速开始

### 最简单的用法

拉取 `hello-world` 镜像（一个很小的测试镜像）并导出为 tar 文件：

```bash
imgp save hello-world:latest -o hello-world.tar
```

执行过程中你会看到：
1. 镜像加速自动生效
2. 显示下载进度条
3. 导出为 tar 文件

完成后你可以用 Docker 导入验证：

```bash
docker load -i hello-world.tar
```

### 拉取指定平台的镜像

拉取适用于 `linux/arm64`（如树莓派、Mac M 系列）的 nginx：

```bash
imgp save nginx:latest --platform linux/arm64 -o nginx-arm64.tar
```

### 从私有仓库拉取

```bash
# 从环境变量读取密码（推荐）
export IMG_REGISTRY_PASSWORD=your_password
imgp save private.registry.com/myapp:latest --username user

# 或者直接传密码（不推荐，因为密码会在进程列表中可见）
imgp save private.registry.com/myapp:latest --username user --password your_password
```

## 全部命令

### `imgp` — 全局选项

| 参数 | 说明 |
|---|---|
| `-v, --version` | 输出版本号 |
| `-h, --help` | 显示帮助信息 |

### `imgp save [镜像名...]` — 拉取并导出镜像

拉取一个或多个 Docker 镜像，导出为标准 `.tar` 文件（可用 `docker load` 导入）。

```bash
# 单个镜像
imgp save hello-world:latest -o hello-world.tar

# 多个镜像（自动生成文件名，不能使用 -o）
imgp save nginx:latest redis:latest alpine:latest
```

完整参数：

| 参数 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `-o, --output` | string | `镜像名_平台.tar` | 输出 tar 文件路径。多镜像时不可用（自动命名） |
| `-p, --platform` | string | `linux/amd64` | 目标平台。格式 `os/arch` 或 `os/arch/variant`，如 `linux/arm64`、`linux/arm64/v8`、`windows/amd64` |
| `--username` | string | 空 | Registry 登录用户名 |
| `--password` | string | 空 | Registry 登录密码。优先级高于 `--password-env`。注意：密码会在进程列表中可见，建议用 `--password-env` |
| `--password-env` | string | `IMG_REGISTRY_PASSWORD` | 存放密码的环境变量名。当 `--password` 未设置时从此变量读取 |
| `--insecure` | bool | `false` | 允许 HTTP 连接（跳过 TLS 验证），用于内网私有仓库 |
| `-P, --parallel` | int | 配置文件中的值，默认 `4` | 同时下载的 layer 数量。网络好可调大（如 8），反之调小（如 2） |
| `--no-cache` | bool | `false` | 忽略本地缓存，强制重新下载所有 layer |
| `--cache-dir` | string | OS 默认路径 | 临时指定缓存目录。优先级：CLI > 配置文件 > OS 默认（见下文缓存管理） |
| `--timeout` | int | `0`（无限制） | 整体操作超时（分钟）。从拉取到导出完毕的总时间上限 |
| `--layer-timeout` | int | `30` | 每层下载超时（分钟）。大镜像或慢网络可调大，`0` = 无限制 |
| `--retry` | int | `2` | 网络错误重试次数。上限 `30`，`0` = 不重试。4xx 错误（401/403/404）不会重试 |
| `-q, --quiet` | bool | `false` | 静默模式，只输出 tar 文件路径。适合脚本调用 |
| `-h, --help` | — | — | 显示 save 命令帮助 |

### `imgp cache` — 缓存管理

管理已下载的 layer 缓存，避免重复下载。

#### `imgp cache info`

显示缓存使用情况：

```bash
imgp cache info
```

输出示例：
```
Cache directory: C:\Users\你\AppData\Local\imgp\cache
Cached layers:   12
Total size:      156.3 MB
```

#### `imgp cache clear`

清空所有缓存文件：

```bash
imgp cache clear
```

输出示例：
```
Cleared 12 cached layers (156.3 MB)
```

#### `--cache-dir` 标志

`info` 和 `clear` 都支持 `--cache-dir` 临时指定目录：

```bash
imgp cache info --cache-dir D:\my-cache
imgp cache clear --cache-dir D:\my-cache
```

#### 默认缓存路径

| 操作系统 | 缓存路径 |
|---|---|
| Windows | `%LOCALAPPDATA%\imgp\cache`（通常 `C:\Users\你\AppData\Local\imgp\cache`） |
| Linux | `$XDG_CACHE_HOME/imgp` 或 `~/.cache/imgp` |
| macOS | `~/Library/Caches/imgp` |

> 优先级：`--cache-dir` CLI 标志 > 配置文件 `cache_dir` > OS 默认路径

### `imgp config` — 配置管理

查看和修改持久化配置（保存于 `imgp.json`）。

#### `imgp config list`

显示当前所有配置：

```bash
imgp config list
```

输出示例：
```
Mirror Map: map[docker.io:[docker.m.daocloud.io] ...
Insecure Registries: [192.168.1.100:5000]
Parallelism: 4
Layer Timeout: 30 min
Timeout: 0 min
Retry: 2
Cache Dir: D:\image-cache
```

#### `imgp config set <key> <value>`

支持的配置键：

| Key | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `mirror-map` | string | 内置加速列表 | Registry 镜像映射。格式：`reg1=mirror1\|mirror2,reg2=mirror`（多组用 `,` 分隔，多个镜像用 `\|` 分隔） |
| `insecure-registries` | string | 空 | 允许 HTTP 连接的 registry（多个用 `,` 分隔） |
| `parallelism` | int | `4` | 并行下载数（最小 1） |
| `layer-timeout` | int | `30` | 每层下载超时分钟数（`0` = 无限制） |
| `timeout` | int | `0` | 整体超时分钟数（`0` = 无限制） |
| `retry` | int | `2` | 重试次数（上限 30，`0` = 不重试） |
| `cache-dir` | string | 空 | 缓存目录路径。设为 `""` 回退 OS 默认 |

示例：

```bash
imgp config set mirror-map "docker.io=docker.m.daocloud.io,gcr.io=gcr.mirrors.daocloud.io"
imgp config set parallelism 8
imgp config set insecure-registries "192.168.1.100:5000"
imgp config set layer-timeout 60
imgp config set timeout 120
imgp config set retry 3
imgp config set cache-dir "D:\image-cache"
imgp config set cache-dir ""      # 重置为 OS 默认
```

### 配置文件 `imgp.json`

保存在 **imgp 二进制所在目录**。完整结构示例：

```json
{
  "mirror_map": {
    "docker.io": ["docker.m.daocloud.io"],
    "gcr.io": ["gcr.mirrors.daocloud.io"],
    "registry.k8s.io": ["m.daocloud.io/registry.k8s.io"]
  },
  "auths": {
    "registry.example.com": {
      "username": "your-username",
      "password_env": "IMG_REGISTRY_PASSWORD"
    }
  },
  "insecure_registries": ["192.168.1.100:5000"],
  "parallelism": 4,
  "layer_timeout": 30,
  "timeout": 0,
  "retry": 2,
  "cache_dir": ""
}
```

字段说明：

| 字段 | 类型 | 说明 |
|---|---|---|
| `mirror_map` | object | Registry → 镜像地址列表的映射 |
| `auths` | object | Registry 认证配置。key 为 registry 域名，value 含 `username`、`password`、`password_env` |
| `insecure_registries` | array | 允许 HTTP 连接的 registry |
| `parallelism` | int | 并行下载数 |
| `layer_timeout` | int | 每层下载超时（分钟），`0` = 无限制 |
| `timeout` | int | 整体超时（分钟），`0` = 无限制 |
| `retry` | int | 重试次数 |
| `cache_dir` | string | 缓存目录，空 = 使用 OS 默认路径 |

> `auths` 中的明文 `password` 字段不会持久化保存。推荐使用 `password_env` 引用环境变量，运行时读取。

## 镜像加速（mirror_map）说明

`mirror_map` 是 imgp 的核心功能，它决定了从哪些地址拉取镜像。

工作原理：当你要拉取 `docker.io/library/nginx:latest` 时，imgp 会先查 `mirror_map` 中有没有 `docker.io` 的映射。如果有，就先用**镜像地址** `docker.m.daocloud.io/library/nginx:latest` 拉取。如果镜像不可用，自动回退到原始地址 `index.docker.io/library/nginx:latest`。

目前支持加速的 registry：

| 原始 registry | 加速地址 | 说明 |
|---|---|---|
| `docker.io` | `docker.m.daocloud.io` | DaoCloud 提供的 Docker Hub 镜像 |
| `gcr.io` | `gcr.mirrors.daocloud.io` | DaoCloud 提供的 Google 镜像 |

你可以随时添加或修改镜像映射。如果有自己的镜像站，可以这样配置：

```bash
imgp config set mirror-map "docker.io=my-mirror.com"
```

一个 registry 也可以配多个镜像（用 `|` 分隔），按顺序尝试：

```bash
imgp config set mirror-map "docker.io=mirror1.example.com|mirror2.example.com"
```

## 效果展示

```
$ imgp save hello-world:latest -o hello-world.tar

Pulling hello-world:latest (linux/amd64)
Image manifest fetched, downloading layers...
  layers: [1/1] 100% | 2.4 KB / 2.4 KB
    ✓ sha256:4f55086f  100%
  exporting: 100% | 6.5 KB / 6.5 KB
Done: hello-world:latest saved to hello-world.tar
```

大文件下载效果（多 layer 并行）：

```
  layers: [2/3] 87.4% | 10.3 MB / 11.8 MB
    ✓ sha256:9f1abecd  100%
    ✓ sha256:c2caafd5  100%
    ◌ sha256:b7e1cbd2  86% 9.2 MB / 10.7 MB
```

- `✓` = 下载完成
- `◌` = 正在下载
- `·` = 等待中

## 源码构建

### Windows

```powershell
# 只编译当前平台（默认，快速）
.\build.ps1

# 编译全部 6 个平台（发布用）
.\build.ps1 -All
```

编译产物在 `bin\` 目录下。

### Linux / macOS

```bash
# 当前平台
go build -o imgp .

# 指定平台
GOOS=linux GOARCH=amd64 go build -o imgp-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -o imgp-linux-arm64 .
GOOS=darwin GOARCH=arm64 go build -o imgp-darwin-arm64 .
```

## 工作原理

```
你输入: imgp save quay.io/prometheus/node-exporter:v1.11.1 -o out.tar

步骤 1: 解析镜像名
        → registry = quay.io
        → 仓库 = prometheus/node-exporter
        → 标签 = v1.11.1

步骤 2: 应用镜像加速
        → 查找 mirror_map["quay.io"]
        → 先尝试 quay.mirrors.daocloud.io
        → 如果失败，回退到原始 quay.io

步骤 3: 获取 manifest（镜像清单）
        → 拉取多架构 manifest list
        → 匹配你指定的平台（默认 linux/amd64）

步骤 4: 并行下载 layer
        → 每个 layer 独立 HTTP 连接
        → 默认 4 个同时下载
        → 显示实时进度

步骤 5: 导出 tar
        → 组装为标准 Docker tar 格式
        → 写入 out.tar

整个过程不需要 Docker 守护进程
```

## 常见问题

### Q: 和 `docker pull` + `docker save` 有什么区别？

`docker pull` + `docker save` 需要先安装 Docker 守护进程，且 Docker 在 Windows 上需要通过 WSL2 或 Hyper-V 运行，比较重。imgp 是单文件二进制，无需任何依赖，下载即用。

### Q: 导出的 tar 怎么用？

```bash
# 把 tar 文件传到有 Docker 的机器上
scp hello-world.tar user@remote-server:~/
ssh user@remote-server
docker load -i hello-world.tar

# 查看导入的镜像
docker images
```

### Q: 下载中断、超时或出错怎么办？

imgp 提供三重保护：

**断点续传** — 已下载的 layer 自动缓存，重跑直接跳过。加 `--no-cache` 强制重下。

**自动重试** — 网络抖动（如 `unexpected EOF`、`connection reset`）自动重试 2 次：

```bash
# 重试 5 次
imgp save hello-world:latest -o hello-world.tar --retry 5

# 不重试
imgp save hello-world:latest -o hello-world.tar --retry 0

# 持久化到配置
imgp config set retry 3
```

404/403/401 等错误不会重试。

**超时控制** — 大镜像或慢网络增大超时时间：

```bash
# 每层 60 分钟，整体 2 小时
imgp save large-image:latest -o large.tar --layer-timeout 60 --timeout 120

# 持久化到配置
imgp config set layer-timeout 60
imgp config set timeout 120
```

默认每层 30 分钟，整体无限制（0）。

### Q: `--platform` 支持哪些值？

格式为 `os/arch` 或 `os/arch/variant`：

| 平台 | 值 |
|---|---|
| Linux x86-64 | `linux/amd64`（默认） |
| Linux ARM64 | `linux/arm64` |
| Linux ARMv8 | `linux/arm64/v8` |
| Windows x86-64 | `windows/amd64` |
| Windows ARM64 | `windows/arm64` |
| macOS Intel | `darwin/amd64` |
| macOS Apple Silicon | `darwin/arm64` |

### Q: 拉取 `windows/amd64` 的镜像失败？

大多数 Docker 官方的镜像（如 `nginx`、`hello-world`）只有 Linux 版本。你需要拉取专门标注了 Windows 支持的镜像。

### Q: `imgp.json` 在哪里？

和 `imgp.exe`（或 `imgp` 二进制）在同一个目录。运行 `where.exe imgp`（Windows）或 `which imgp`（Linux/macOS）查看二进制位置。

### Q: Docker Hub 访问不了怎么配镜像？

`mirror_map` 中默认已经配置了国内加速镜像。如果你有自己的镜像站：

```bash
imgp config set mirror-map "docker.io=你的镜像地址"
```

## 注意事项

- **镜像加速地址**：不需要加 `https://` 前缀，直接写域名如 `docker.m.daocloud.io`
- **多镜像配置**：用 `|` 分隔，如 `"docker.io=mirror1|mirror2"`
- **digest 引用**：`image@sha256:...` 格式暂不支持镜像加速
- **配置修改**：`imgp config set` 后立即生效，无需重启

## License

GNU General Public License v3.0。详见 [LICENSE](LICENSE)。
