# imgp

**imgp** is a cross-platform Docker image pull and save tool. It pulls multi-architecture images from registries with mirror acceleration support and saves them as standard `docker load`-compatible tar archives — no Docker daemon required.

**imgp** 是一个跨平台的 Docker 镜像拉取和导出工具。它支持从 registry 拉取多架构镜像，自动使用镜像加速，并保存为标准 `docker load` 可导入的 tar 包——无需安装 Docker 守护进程。

## Features / 功能特点

| English | 中文 |
|---|---|
| **Multi-architecture** — pull for any platform (`linux/amd64`, `linux/arm64`, etc.) | **多架构** — 支持拉取任意平台的镜像 |
| **Mirror acceleration** — auto mirror resolution for docker.io, quay.io, gcr.io | **镜像加速** — 内置国内镜像加速，自动切换 |
| **Parallel downloads** — concurrent layer pulling | **并行下载** — 多 layer 并发拉取 |
| **Resume support** — cached layers skip on retry | **断点续传** — 已下载的 layer 自动跳过 |
| **Detailed progress** — per-layer progress bars | **详细进度** — 每层独立进度条显示 |
| **Private registries** — username/password and token auth | **私有仓库** — 支持用户名密码和 token 认证 |
| **Zero deps** — pure Go binary, no Docker needed | **无依赖** — 纯 Go 编译的单二进制，无需 Docker |
| **Cross-platform** — Windows, Linux, macOS | **跨平台** — 原生支持 Windows / Linux / macOS |

## Quick Start / 快速开始

```bash
# Save nginx for your current platform / 拉取并导出
imgp save nginx:latest -o nginx.tar

# Specify a target architecture / 指定目标架构
imgp save nginx:latest --platform linux/arm64 -o nginx-arm64.tar

# Load into Docker / 导入 Docker
docker load -i nginx.tar
```

## Install / 安装

### Download binary / 下载二进制

Download from the [Releases page](https://gitcode.com/DonaldTom/imgp/releases) and place it in your `PATH`.

从 [Releases 页面](https://gitcode.com/DonaldTom/imgp/releases) 下载，放入 `PATH` 即可使用。

### From source / 源码编译

```bash
go install gitcode.com/DonaldTom/imgp@latest
```

## Usage / 使用说明

```bash
imgp save [image] [flags]
```

### Flags / 参数

| Flag | English | 中文 |
|---|---|---|
| `-o, --output` | Output tar file path | 导出 tar 文件路径 |
| `-p, --platform` | Target platform (e.g. `linux/arm64`) | 目标平台 |
| `--username` | Registry username | Registry 用户名 |
| `--password` | Registry password | Registry 密码 |
| `--password-env` | Env var for password (default `IMG_REGISTRY_PASSWORD`) | 密码环境变量名 |
| `--insecure` | Allow insecure registry connections | 允许非 TLS 连接 |
| `-P, --parallel` | Parallel downloads (from config, or 4) | 并行下载数 |
| `-q, --quiet` | Quiet mode, output only the tar path | 静默模式 |

### Configuration / 配置

Configuration is stored in `imgp.json` next to the binary.

配置文件 `imgp.json` 与二进制文件在同一目录。

```json
{
  "mirror_map": {
    "docker.io": ["docker.daocloud.io"],
    "quay.io": ["quay.mirrors.daocloud.io"],
    "gcr.io": ["gcr.mirrors.daocloud.io"]
  },
  "parallelism": 4
}
```

Edit via CLI / 命令行修改：

```bash
imgp config list
imgp config set mirror-map "docker.io=docker.daocloud.io,quay.io=quay.mirrors.daocloud.io"
imgp config set parallelism 8
```

### Authentication / 认证

```bash
# Password as argument / 直接传密码
imgp save myregistry.com/private/app:latest --username user --password pass123

# Password from env var / 从环境变量读取
export IMG_REGISTRY_PASSWORD=pass123
imgp save myregistry.com/private/app:latest --username user

# Or configure in imgp.json / 或在配置文件中设置
# See imgp.json.example for details
```

## Build from source / 源码构建

```powershell
# Windows
.\build.ps1
```

```bash
# Linux / macOS
GOOS=linux GOARCH=amd64 go build -o imgp-linux-amd64 .
GOOS=darwin GOARCH=arm64 go build -o imgp-darwin-arm64 .
```

## How it works / 工作原理

1. **Parse** — parse image reference (e.g. `quay.io/prometheus/node-exporter:v1.11.1`) / 解析镜像引用
2. **Mirror** — apply mirror map (e.g. `quay.io` → `quay.mirrors.daocloud.io`) / 应用镜像加速映射
3. **Resolve** — fetch manifest list and resolve target platform / 获取 manifest 并解析目标架构
4. **Download** — download layers in parallel with progress / 并行下载各 layer 并显示进度
5. **Export** — assemble and write a standard Docker tar / 组装并写入标准 Docker tar 包

No Docker daemon is required at any step.

整个过程无需 Docker 守护进程参与。

## License / 许可证

GNU General Public License v3.0. See [LICENSE](LICENSE).
