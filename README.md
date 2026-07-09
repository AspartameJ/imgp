[English](README_EN.md) | 中文

# imgp

**imgp** 是一个 Windows Docker 镜像拉取导出工具。纯 Go 单文件，无需 Docker 守护进程，下载即用。

```
imgp save hello-world:latest -o hello-world.tar
docker load -i hello-world.tar    # 在有 Docker 的机器上导入
```

> 源码跨平台（Linux/macOS 可编译），但仅提供 Windows/amd64 二进制和 CI 测试。

---

## 安装

### 下载二进制

从 [Releases](https://gitcode.com/DonaldTom/imgp/releases) 下载 `imgp-windows-amd64.exe`，放入 `PATH` 目录即可。

### Go 安装

```bash
go install gitcode.com/DonaldTom/imgp@latest
```

国内用户先设 Go proxy：

```bash
go env -w GOPROXY=https://goproxy.cn,direct
```

### 验证

```bash
imgp -v
```

---

## 快速开始

```bash
# 拉取并导出
imgp save hello-world:latest

# 指定平台
imgp save nginx:latest --platform linux/arm64 -o nginx-arm64.tar

# 私有仓库
export IMG_REGISTRY_PASSWORD=your_password
imgp save private.registry.com/myapp:latest --username user

# 多个镜像（自动命名，不可用 -o）
imgp save nginx:latest redis:latest alpine:latest

# 启用 gzip 压缩
imgp save nginx:latest -z -o nginx.tar.gz
```

---

## 命令参考

### `imgp save [镜像名...]` — 拉取并导出

| 参数 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `-o, --output` | string | `镜像名_平台.tar` | 输出路径。多镜像时不可用 |
| `-p, --platform` | string | `linux/amd64` | 目标平台，如 `linux/arm64`、`windows/amd64` |
| `--username` | string | — | Registry 登录用户名 |
| `--password` | string | — | 密码（进程列表可见，建议用 `--password-env`） |
| `--password-env` | string | — | 存放密码的环境变量名 |
| `--insecure` | bool | `false` | 跳过 TLS 验证（内网 HTTP registry） |
| `-P, --parallel` | int | `4` | 并行下载数 |
| `--no-cache` | bool | `false` | 忽略缓存，强制重下 |
| `-z, --gzip` | bool | `false` | gzip 压缩输出 |
| `--cache-dir` | string | OS 默认 | 临时缓存目录 |
| `--timeout` | int | `0`(不限) | 整体超时（分钟） |
| `--layer-timeout` | int | `30` | 每层超时（分钟） |
| `--retry` | int | `2` | 重试次数（`0` = 不重试，上限 `30`） |
| `-q, --quiet` | bool | `false` | 静默模式，只输出路径 |

### `imgp cache` — 缓存管理

```bash
imgp cache info                # 查看缓存
imgp cache clear               # 清空缓存
imgp cache info --cache-dir D:\my-cache    # 指定目录
```

| 操作系统 | 默认缓存路径 |
|---|---|
| Windows | `%LOCALAPPDATA%\imgp\cache` |
| Linux | `$XDG_CACHE_HOME/imgp` 或 `~/.cache/imgp` |
| macOS | `~/Library/Caches/imgp` |

> 优先级：`--cache-dir` CLI > 配置文件 `cache_dir` > OS 默认路径

### `imgp config` — 配置管理

```bash
imgp config list                                          # 查看配置
imgp config set mirror-map "docker.io=my-mirror.com"      # 设置镜像映射
imgp config set parallelism 8                             # 并行数
imgp config set retry 3                                   # 重试次数
imgp config set cache-dir "D:\image-cache"                # 缓存目录
imgp config set insecure-registries "192.168.1.100:5000"  # HTTP registry
imgp config set layer-timeout 60                          # 每层超时
imgp config set timeout 120                               # 整体超时
```

配置文件 `imgp.json` 保存在二进制同目录，完整结构：

```json
{
  "mirror_map": {
    "docker.io": ["docker.m.daocloud.io"],
    "gcr.io": ["gcr.mirrors.daocloud.io"]
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

> `password` 字段不会持久化保存。使用 `password_env` 引用环境变量。

---

## 镜像加速（mirror_map）

imgp 内置国内加速镜像，拉取时自动使用：

| 原始 Registry | 加速地址 | 提供方 |
|---|---|---|
| `docker.io` | `docker.m.daocloud.io` | DaoCloud |
| `gcr.io` | `gcr.mirrors.daocloud.io` | DaoCloud |
| `registry.k8s.io` | `m.daocloud.io/registry.k8s.io` | DaoCloud |
| `quay.io` | `quay.nju.edu.cn` | 南京大学 |

工作原理：拉取 `docker.io/library/nginx:latest` → 先尝试镜像地址（失败则回退原始地址）。

```bash
# 自定义镜像
imgp config set mirror-map "docker.io=my-mirror.com"

# 多个镜像（用 | 分隔，按顺序尝试）
imgp config set mirror-map "docker.io=mirror1.example.com|mirror2.example.com"
```

---

## 效果展示

```text
$ imgp save hello-world:latest -o hello-world.tar

Pulling hello-world:latest (linux/amd64)
Image manifest fetched, downloading layers...
  layers: [1/1] 100% | 2.4 KB / 2.4 KB
    ✓ sha256:4f55086f  100%
  exporting: 100% | 6.5 KB / 6.5 KB
Done: hello-world:latest saved to hello-world.tar
```

多 layer 并行：

```text
  layers: [2/3] 87.4% | 10.3 MB / 11.8 MB
    ✓ sha256:9f1abecd  100%
    ✓ sha256:c2caafd5  100%
    ◌ sha256:b7e1cbd2  86% 9.2 MB / 10.7 MB
```

- `✓` = 下载完成 | `◌` = 正在下载 | `·` = 等待中

---

## 工作原理

```text
输入: imgp save quay.io/prometheus/node-exporter:v1.11.1 -o out.tar

1. 解析镜像名 → registry / 仓库 / 标签
2. 应用镜像加速 → 查 mirror_map，先走镜像，失败回退
3. 获取 manifest → 匹配目标平台
4. 并行下载 layer → 默认 4 个并发，实时进度
5. 导出 tar → 标准 Docker tar 格式

整个过程不需要 Docker 守护进程
```

---

## 常见问题

### 和 `docker pull` + `docker save` 有什么区别？

`docker pull` 需要 Docker 守护进程（Windows 上需 WSL2/Hyper-V）。imgp 是单文件二进制，零依赖。

### 中断了怎么办？

已下载的 layer 自动缓存，重跑即续传。`--no-cache` 强制重下。

```bash
# 自动重试（默认 2 次）
imgp save hello-world:latest --retry 5

# 超时控制
imgp save large-image:latest --layer-timeout 60 --timeout 120
```

404/401/403 等错误不会重试。

### 支持哪些 `--platform` 值？

格式 `os/arch` 或 `os/arch/variant`：

| 平台 | 值 |
|---|---|
| Linux x86-64 | `linux/amd64`（默认） |
| Linux ARM64 | `linux/arm64` |
| Linux ARMv8 | `linux/arm64/v8` |
| Windows x86-64 | `windows/amd64` |
| Windows ARM64 | `windows/arm64` |
| macOS Intel | `darwin/amd64` |
| macOS Apple Silicon | `darwin/arm64` |

### 为什么拉 `windows/amd64` 失败？

大多数官方镜像只有 Linux 版。需拉取专门标注了 Windows 支持的镜像。

### Docker Hub 访问不了？

默认已配国内加速。自定义镜像站：

```bash
imgp config set mirror-map "docker.io=你的镜像地址"
```

---

## 注意事项

- **镜像地址**：不加 `https://`，直接写域名
- **多镜像**：用 `|` 分隔，`"docker.io=mirror1|mirror2"`
- **digest 引用**：`image@sha256:...` 暂不支持镜像加速
- **配置修改**：`imgp config set` 即时生效，无需重启

---

## License

GNU General Public License v3.0。详见 [LICENSE](LICENSE)。
