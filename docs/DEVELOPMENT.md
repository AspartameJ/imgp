# imgp 开发文档

## 1. 项目概述

imgp 是一个 Docker 镜像拉取导出工具。从 registry 拉取镜像 → 缓存 layer → 导出为标准 Docker tar 文件。

- **语言**: Go 1.22+
- **核心依赖**: [`go-containerregistry`](github.com/google/go-containerregistry) — registry 协议通信与 manifest/layer 操作
- **CLI 框架**: [`cobra`](github.com/spf13/cobra)
- **目标平台**: Windows/amd64（源码可跨平台编译）

---

## 2. 项目结构

```
cmd/                  # cobra 命令注册、参数解析、业务流程编排
├── root.go           # 根命令、全局标志、Execute() 入口
├── save.go           # save 命令：runSave / runSaveOne
├── save_params.go    # 参数解析与合法性校验
├── pipeline.go       # 核心流水线：fetchManifest → pullLayers → exportImage
├── cache.go          # cache info / clear 命令
├── config_cmd.go     # config list / set 命令
└── terminal_windows.go # Windows 控制台 ANSI 支持

internal/             # 内部实现（不对外暴露）
├── config/           # 配置加载/保存（imgp.json）
├── registry/         # registry 通信（auth, mirror fallback, retry, transport）
├── puller/           # layer 下载引擎（并发、缓存、重试）
├── saver/            # 导出 tar 文件（layer 组装、gzip、CRC 验证）
├── ui/               # 终端进度显示
├── util/             # 工具函数（重试判定、backoff、架构验证）
└── version/          # 版本号（编译时通过 ldflags 注入）
```

### Package 职责

| Package | 输入 | 输出 | 关键类型 |
|---------|------|------|----------|
| `cmd` | CLI 参数 | 文件系统（tar） | — |
| `config` | `imgp.json` | `*Config` | Config, AuthConfig |
| `registry` | 镜像名 + platform | `v1.Image`, `name.Reference` | Client, LayerFetcher |
| `puller` | LayerTask 列表 | 缓存的 .gz 文件 | Puller, LayerTask |
| `saver` | 缓存 .gz 文件 | 标准 tar 文件 | — |
| `ui` | 事件流 | 终端输出 | ProgressDisplay |

---

## 3. 核心流程

### 3.1 save 命令完整链路

```
imgp save nginx:latest -o nginx.tar

cmd/root.go
  └─ Execute() → Cobra 路由 → saveCmd.RunE = runSave()

cmd/save.go
  ├─ runSave()                              # 多镜像批处理
  │   └─ runSaveOne()                       # 处理单个镜像
  │       ├─ resolveSaveParams()            # CLI 参数 + 配置文件合并
  │       ├─ registry.NewClient(cfg)        # 创建 registry 客户端
  │       ├─ fetchManifest()                # 获取镜像 manifest
  │       │   └─ client.FetchImage()        # registry 通信 + 镜像加速
  │       ├─ pullLayers()                   # 下载所有 layer
  │       │   ├─ client.NewLayerFetcher()   # 每个 layer 一个 fetcher
  │       │   ├─ puller.NewPuller()         # 创建下载引擎
  │       │   └─ pl.Pull()                  # 并发下载
  │       └─ exportImage()                  # 导出 tar
  │           └─ saver.Export()             # 组装标准 Docker tar
```

### 3.2 镜像加速（mirror_map）匹配逻辑

```go
// internal/registry/client.go
// resolveRefs() 为镜像构造尝试列表：镜像地址 → 原始地址

func (c *Client) resolveRefs(ref name.Reference) []name.Reference {
    // 1. 查 mirror_map[registry]
    // 2. 如果能找到镜像列表，构造镜像引用
    // 3. 添加原始引用作为最后回退
    // 返回顺序：镜像1 → 镜像2 → ... → 原始地址
}
```

FetchImage 循环遍历 `refsToTry`，镜像失败后自动尝试下一个。若所有地址均失败，触发重试（backoff + 重试）。

---

## 4. 关键设计

### 4.1 认证系统

**优先级链**: `--password` CLI > `--password-env` CLI > config 文件 auths

```
resolveSaveParams()                     # cmd/save_params.go
  └─ p.password:
      1. --password CLI 值（非空则用）
      2. --password-env → os.Getenv()
      3. 空字符串

registry.Client.authenticator(reg)      # internal/registry/client.go
  └─ auth chain:
      1. WithAuth() 设置的 username/password（来自 CLI）
      2. config.auths[regName] → resolveAuth()
         a. password_env → 环境变量（存在则覆盖）
         b. password 字段（config 文件，但不会被持久化保存）
      3. authn.Anonymous（匿名访问）
```

**注意**: `resolveAuth()` 中如果 `PasswordEnv` 设置了但环境变量不存在，会回退到 `Password` 字段并输出 WARN 到 stderr。

### 4.2 缓存机制

```
缓存目录结构:
  <cache-dir>/
    ├── <sha256-hex>.gz            # 压缩后的 layer 数据
    └── <sha256-hex>.gz.verified   # CRC 验证通过标记

checkCache()                  # internal/puller/puller.go
  ├─ 文件存在且大小匹配？
  ├─ gzip magic bytes (0x1f 0x8b) 正确？
  ├─ .verified 标记存在？
  └─ 全部满足 → 跳过下载，标记 "cached"

verifyGzip()                  # internal/saver/tar.go
  ├─ .verified 标记存在且新于 .gz → 跳过（快速路径）
  ├─ 解压并 CRC 校验
  └─ 通过 → 写 .verified 标记文件
```

**设计要点**:
- puller 只检查 gzip magic bytes（快速路径）
- saver 做完整 CRC 校验（写入 `.verified` 标记）
- 双层检查平衡了速度和安全性
- `.verified` 文件的 mtime 用于判断缓存是否过期（比 .gz 旧则重新验证）

### 4.3 重试策略

```go
// internal/registry/client.go
for attempt := 0; attempt <= c.retry; attempt++ {
    if attempt > 0 {
        util.Backoff(ctx, attempt)  // 指数退避: 1s, 2s, 4s, 8s, ...
    }
    // 遍历 refsToTry（镜像地址列表）
}

// internal/puller/puller.go
for attempt = 0; attempt <= p.maxRetries; attempt++ {
    if attempt > 0 {
        if !util.IsRetryable(lastErr) { break }   // 非可重试错误直接放弃
        p.backoffOrSendError(...)                  // 退避等待
        os.Remove(cacheFile)                        // 清理失败的缓存
    }
    downloadAttempt(...)
}
```

**可重试错误判定** (`internal/util/arch.go`):

| 类型 | 示例 | 是否重试 |
|------|------|----------|
| 网络错误 | connection reset, dial tcp, TLS handshake | ✅ |
| HTTP 5xx | 500, 502, 503 | ✅ |
| HTTP 4xx | 401, 403, 404 | ❌ |
| 上下文取消 | context canceled | ❌ |

**重试次数**:
- 默认 2 次（共 3 次尝试）
- `--retry 0` 禁用重试
- `--retry N` → 最多 N+1 次尝试
- config 文件 `retry` 字段优先级低于 CLI

### 4.4 并发控制

```
layer 并行下载
  │
  └─ Puller.Pull()
      ├─ tasks 通道 ← puller 逐个分发
      ├─ 工作协程池（semaphore 控制并行数）
      │   ├─ goroutine 1: layer A 下载
      │   ├─ goroutine 2: layer B 下载
      │   └─ goroutine 3: layer C 下载
      ├─ events 通道 ← 工作协程上报事件
      └─ UI 读取 events 通道并渲染
```

**关键参数**:
- `--parallel` / `-P`: 默认 4，控制最大并发下载数
- `--layer-timeout`: 每层超时（默认 30 分钟），超时视为可重试错误
- **semaphore 模式**: 内部用 channel 做令牌桶，非 WaitGroup

**UI 线程模型**:

```go
// ProgressDisplay.RunPullUI()
// 非安静模式:
//   1. ticker (100ms) → renderFrame() → 输出到 stderr
//   2. useANSI → \033[%dA 光标上移覆盖上一帧
// 安静模式:
//   1. 只收集 events，不渲染
//   2. 结束后输出最终 tar 路径
```

### 4.5 断点续传（`--resume`）

默认关闭。开启后，未完成的 layer 通过 HTTP Range 请求从断点继续下载。

```
LayerTask.OpenLayer(ctx, offset)     # internal/puller/puller.go
  └─ offset > 0 → 请求 bytes=offset-  # internal/registry/client.go

downloadAttempt()                    # internal/puller/puller.go
  ├─ p.resume 且 0 < 文件大小 < t.Size → offset = 文件大小
  ├─ offset > 0 → os.OpenFile(O_APPEND) 追加写
  └─ 否则 → os.Create 从头写
```

**关键设计**:
- `LayerTask.OpenLayer` 签名带 `offset`，由 registry 层构造 `Range: bytes=N-` 请求
- `NewLayerFetcher` 不再用 `remote.Layer().Compressed()`，改为手动构造 `/v2/<repo>/blobs/<digest>` 请求，复用 `c.transport(reg)` 的认证
- server 返回 `206` → 正常续传；返回 `200`（忽略 Range）→ `io.CopyN` 丢弃已下载前缀，安全追加
- `preservePartial()` 决定是否保留部分文件：仅当 resume 开启且 `0 < 文件大小 < 目标大小`
- 完整性由 `offset + written == t.Size` 保证，续传不会绕过校验
- 重试时保留部分进度（不删除文件），失败后也保留，供下次续传

---

## 5. 构建与发布

### 5.1 构建

```powershell
# PowerShell
.\build.ps1                     # 编译到 bin\imgp.exe
.\build.ps1 -Version "2.1.3"    # 嵌入版本号

# Makefile（需要 sh 环境）
make build                      # go build -o imgp .
make VERSION=2.1.3 release      # 构建 + 打 tag
```

### 5.2 版本号机制

```
ldflags 注入:
  -X 'gitcode.com/DonaldTom/imgp/internal/version.Version=$(VERSION)'

internal/version/version.go:
  var Version = "dev"         // 默认值，未用 ldflags 时显示 "dev"
```

`dev` 是默认值。正式发布必须用 `build.ps1 -Version X.Y.Z` 或 `make VERSION=X.Y.Z`。

---

## 6. 测试

### 6.1 运行测试

```powershell
# 全部测试
go test ./...

# 含竞态检测
go test -race ./...

# 覆盖率
go test -coverprofile=cover.out ./...
go tool cover -html=cover.out    # HTML 报告
go tool cover -func=cover.out    # 函数级覆盖率
```

### 6.2 Mock Registry 测试模式

`internal/registry/client_test.go` 中的 `mockRegistry()` 创建了一个完整的 HTTP 测试服务器，模拟真实的 registry 行为：

```go
server, img, ref := mockRegistry(t, imgSize, numLayers)
// server 提供 /v2/、/manifests/、/blobs/ 端点
// img 是随机生成的测试镜像
// ref 是包含 server 地址的 image reference
```

测试原则：
- 网络操作尽量用 mock server，不依赖外部网络
- layer 下载用 `gzipBytes()` 生成测试数据
- 上下文取消测试通过 `context.WithCancel` + goroutine 模拟

### 6.3 覆盖率目标

| Package | 当前覆盖率 | 目标 |
|---------|-----------|------|
| `cmd/` | ~86% | ≥80% |
| `internal/config/` | ~88% | ≥85% |
| `internal/puller/` | ~81% | ≥80% |
| `internal/registry/` | ~95% | ≥90% |
| `internal/saver/` | ~86% | ≥85% |
| `internal/ui/` | ~86% | ≥85% |
| `internal/util/` | ~94% | ≥90% |

---

## 7. 常见开发任务

### 7.1 添加新 registry 支持

修改 `internal/config/config.go` 中的 `DefaultConfig()`，在 `MirrorMap` 中增加条目：

```go
MirrorMap: map[string][]string{
    "docker.io": {"docker.m.daocloud.io"},
    "ghcr.io":   {"ghcr.mirror.example.com"},  // 新增
},
```

### 7.2 修改输出格式

- 进度条格式: `internal/ui/display.go` — `renderFrame()`
- 颜色/ANSI: `internal/ui/display.go` — `Cyan()`, `Green()`
- 消息文案: `cmd/pipeline.go` — `fetchManifest()`, `exportImage()`

### 7.3 调试 UI 输出

```powershell
# 捕获原始输出（不启用 ANSI）
imgp save hello-world:latest -o test.tar 2>&1 | Out-File -Encoding utf8 output.log

# 安静模式，只输出路径
imgp save hello-world:latest -o test.tar -q
```

### 7.4 测试缓存行为

```powershell
# 首次下载（写入缓存）
imgp save hello-world:latest -o test1.tar

# 第二次（应命中缓存，显示 "cached"）
imgp save hello-world:latest -o test2.tar

# 强制重下
imgp save hello-world:latest -o test3.tar --no-cache
```

---

## 8. CI/CD 与发布流程

### 8.1 CI（.github/workflows/ci.yml）

- 平台: `windows-latest`
- 步骤: checkout → setup Go → go build → go test → go vet → gofmt 检查

### 8.2 发布流程

```powershell
# 1. 更新版本号
.\build.ps1 -Version "2.2.0"

# 2. 运行全部测试
go test -race ./...

# 3. 提交并打 tag
git add -A
git commit -m "feat: ..."
git tag v2.2.0
git push && git push origin v2.2.0

# 4. 在 GitHub/GitCode Releases 页面创建 Release
#    上传 bin\imgp.exe 作为附件
```

### 8.3 兼容性注意事项

- `go-containerregistry` 版本更新可能影响 registry 协议兼容性
- Windows ANSI 支持依赖 `terminal_windows.go` 中的控制台模式设置
- 缓存格式（`.gz` + `.verified`）更改需考虑向后兼容

---

## 附录

### 关键常量与默认值

| 常量 | 值 | 位置 |
|------|-----|------|
| 默认并行数 | 4 | `internal/config/config.go:10` |
| 默认重试次数 | 2 | `internal/config/config.go:43` |
| 默认 layer 超时 | 30 分钟 | `cmd/save_params.go:91` |
| 默认超时 | 0（无限制） | `cmd/save_params.go:99` |
| 最大重试次数 | 30 | `cmd/save_params.go:89` |
| 缓存目录 | `%LOCALAPPDATA%\imgp\cache` | `internal/config/config.go:128` |
| 配置文件 | `imgp.json`（与二进制同目录） | `internal/config/config.go:48` |
| backoff 基数 | 1 秒 | `internal/util/backoff.go:15` |
| backoff 上限 | 30 秒 | `internal/util/backoff.go:18` |
