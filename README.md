[English](README_EN.md) | 中文

# imgp

跨平台 Docker 镜像拉取和导出工具。支持多架构镜像、镜像加速、并行下载、断点续传。无需 Docker 守护进程。

## 功能特性

| 特性 | 说明 |
|---|---|
| **多架构** | 拉取任意平台的镜像（`linux/amd64`、`linux/arm64`、`windows/amd64` 等） |
| **镜像加速** | 内置国内镜像加速，自动匹配 docker.io、quay.io、gcr.io |
| **并行下载** | 多 layer 并发拉取，速度翻倍 |
| **断点续传** | 已下载的 layer 缓存到本地，中断后自动跳过 |
| **详细进度** | 每层独立进度条，实时显示下载状态 |
| **私有仓库** | 支持用户名密码和 token 认证 |
| **零依赖** | 纯 Go 编译的单二进制，无需安装 Docker |
| **跨平台** | Windows、Linux、macOS 均可运行 |

## 快速开始

```bash
# 拉取并导出 nginx 最新版
imgp save nginx:latest -o nginx.tar

# 指定拉取 arm64 架构
imgp save nginx:latest --platform linux/arm64 -o nginx-arm64.tar

# 导入 Docker
docker load -i nginx.tar

# 一行命令完成
imgp save -o nginx.tar nginx:latest
```

## 安装

### 下载二进制

从 [Releases 页面](https://gitcode.com/DonaldTom/imgp/releases) 下载对应平台的二进制，放入 `PATH` 即可。

### 源码编译

```bash
go install gitcode.com/DonaldTom/imgp@latest
```

## 使用说明

```bash
imgp save [镜像名] [参数]
```

### 参数

| 参数 | 说明 |
|---|---|
| `-o, --output` | 导出 tar 文件路径 |
| `-p, --platform` | 目标平台，如 `linux/arm64` |
| `--username` | Registry 用户名 |
| `--password` | Registry 密码（建议用 `--password-env`） |
| `--password-env` | 密码环境变量名（默认 `IMG_REGISTRY_PASSWORD`） |
| `--insecure` | 允许非 TLS 连接 |
| `-P, --parallel` | 并行下载层数（默认 4） |
| `-q, --quiet` | 静默模式，仅输出 tar 路径 |
| `-h, --help` | 帮助信息 |

### 配置

配置文件 `imgp.json` 与二进制文件在同一目录。默认值：

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

命令行修改：

```bash
# 查看配置
imgp config list

# 修改镜像加速映射
imgp config set mirror-map "docker.io=docker.daocloud.io,quay.io=quay.mirrors.daocloud.io"

# 一个 registry 配多个镜像，用 | 分隔
imgp config set mirror-map "docker.io=mirror1|mirror2"

# 修改并行数
imgp config set parallelism 8
```

### 认证

```bash
# 从环境变量读取（推荐）
export IMG_REGISTRY_PASSWORD=your_password
imgp save private/image:latest --username user

# 直接传密码（不推荐，可能被其他进程看到）
imgp save private/image:latest --username user --password your_password

# 在配置文件中设置
# 详见 imgp.json.example
```

## 效果展示

```
$ imgp save nginx:latest -o nginx.tar --platform linux/arm64

Pulling nginx:latest (linux/arm64)
Image manifest fetched, downloading layers...
  layers: [2/3] 87.4% | 10.3 MB / 11.8 MB
    ✓ sha256:9f1abecd  100%
    ✓ sha256:c2caafd5  100%
    ◌ sha256:b7e1cbd2  86% 9.2 MB / 10.7 MB
  exporting: 100% | 11.8 MB / 11.8 MB
Done: nginx:latest (linux/arm64) saved to nginx_linux-arm64.tar
```

## 源码构建

```powershell
# Windows（构建所有平台）
.\build.ps1

# 只构建 Windows
.\build.ps1 -Target windows
```

```bash
# Linux / macOS
GOOS=linux GOARCH=amd64 go build -o imgp-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -o imgp-linux-arm64 .
GOOS=darwin GOARCH=arm64 go build -o imgp-darwin-arm64 .
```

## 工作原理

1. **解析** — 解析镜像引用（如 `quay.io/prometheus/node-exporter:v1.11.1`）
2. **镜像加速** — 根据 `mirror_map` 自动替换 registry 地址
3. **解析架构** — 获取 manifest list（多架构清单），匹配目标平台
4. **并行下载** — 每层一个 HTTP 请求，并发下载到缓存目录
5. **导出 tar** — 从缓存读取各层，组装为标准 Docker tar 包

整个过程无需 Docker 守护进程参与。

## 注意事项

- **缓存目录**：`.imgp-cache/` 位于二进制同目录，可手动删除释放空间
- **镜像加速格式**：加速地址前面不需要 `https://` 前缀，如 `docker.daocloud.io`
- **平台格式**：`os/arch` 或 `os/arch/variant`，如 `linux/amd64`、`linux/arm64/v8`
- **不支持 digest 引用**：`image@sha256:...` 格式暂不自动加速

## License

GNU General Public License v3.0。详见 [LICENSE](LICENSE)。
