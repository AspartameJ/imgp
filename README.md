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
imgp version
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

### `imgp save [镜像名]` — 拉取并导出镜像

```bash
imgp save [镜像名] [参数]
```

完整参数：

| 参数 | 默认值 | 说明 |
|---|---|---|
| `-o, --output` | `镜像名_平台.tar` | 导出的 tar 文件路径 |
| `-p, --platform` | `linux/amd64` | 目标平台，常用值：`linux/amd64`、`linux/arm64`、`windows/amd64` |
| `--username` | 空 | Registry 登录用户名 |
| `--password` | 空 | Registry 登录密码（建议用 `--password-env`） |
| `--password-env` | `IMG_REGISTRY_PASSWORD` | 存放密码的环境变量名 |
| `--insecure` | false | 允许非 TLS 连接（用于 HTTP 的私有仓库） |
| `-P, --parallel` | 配置文件中设置，默认 4 | 同时下载的 layer 数量，网络好可以调大 |
| `--no-cache` | false | 忽略本地缓存，强制重新下载所有 layer |
| `--cache-dir` | 见缓存管理 | 指定缓存目录 |
| `-q, --quiet` | false | 静默模式，只输出 tar 文件路径 |
| `-h, --help` | - | 显示帮助 |

### `imgp cache` — 缓存管理

```bash
# 查看缓存占用了多少空间
imgp cache info

# 清空所有缓存
imgp cache clear
```

默认缓存位置：

| 操作系统 | 缓存路径 |
|---|---|
| Windows | `%LOCALAPPDATA%\imgp\cache`（通常是 `C:\Users\你的用户名\AppData\Local\imgp\cache`） |
| Linux | `~/.cache/imgp` 或 `$XDG_CACHE_HOME/imgp` |
| macOS | `~/Library/Caches/imgp` |

你也可以用 `--cache-dir` 参数临时指定其他目录：

```bash
imgp save hello-world:latest -o hello-world.tar --cache-dir D:\my-temp-cache
```

### `imgp config` — 配置管理

```bash
# 查看当前配置
imgp config list

# 修改镜像加速映射
imgp config set mirror-map "docker.io=docker.m.daocloud.io,gcr.io=gcr.mirrors.daocloud.io"

# 修改并行下载数
imgp config set parallelism 8

# 添加不安全 registry（允许 HTTP 连接）
imgp config set insecure-registries "192.168.1.100:5000"
```

配置文件 `imgp.json` 保存在 **imgp 二进制所在的目录**。默认内容：

```json
{
  "mirror_map": {
    "docker.io": ["docker.m.daocloud.io"],
    "gcr.io": ["gcr.mirrors.daocloud.io"]
  },
  "parallelism": 4
}
```

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

### Q: 下载中断了怎么办？

下次对同一个镜像执行 `imgp save`，已下载的 layer 会自动跳过（断点续传）。如果想强制重下，加 `--no-cache`。

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
