# Changelog

## v2.2.0 (2026-07-24)

### Added
- **`--resume` 标志** — 通过 HTTP Range 请求断点续传中断的 layer（默认关闭）
  - `LayerTask.OpenLayer` 签名新增 `offset` 参数，registry 层构造 `Range: bytes=N-` 请求
  - `NewLayerFetcher` 改为手动构造 blob 请求，支持 `206 Partial Content`
  - server 忽略 Range（返回 `200`）时自动丢弃已下载前缀，安全追加
  - 重试/失败后保留部分文件，供下次续传

### Changed
- **缓存判断流程图**（README CN/EN）更新为准确的缓存 + 续传逻辑

## v2.1.4 (2026-07-24)

### Fixed
- **文档** — 补充 v2.1.1~v2.1.3 变更记录，修正 DEVELOPMENT.md backoff 上限（120→30 秒）和版本示例

## v2.1.3 (2026-07-24)

### Fixed
- **ANSI 终端残留 "GBGB/MBMB"** — `renderFrame` 每行前加 `\033[2K` 清行，消除 layer done→downloading 切换时的字符残留
- **缓存未命中（export 中断后重跑）** — `processTask` 下载成功后立即写 `.verified` 标记，不再依赖 export 阶段创建
- **CRC 错误时 `.verified` 孤立** — `verifyGzip` 检测到 CRC 错误时同步删除 `.verified` 标记
- **export 进度条字符残留** — `\r` 改为 `\r\033[K`，清除旧行内容
- **tmp 文件并发冲突** — `outputPath + ".tmp"` 改为 `outputPath + ".{pid}.tmp"`

## v2.1.2 (2026-07-24)

### Fixed
- **ANSI 终端残留 "GBGB/MBMB"** — `renderFrame` 每行前加 `\033[2K` 清行
- **缓存未命中（export 中断后重跑）** — 下载成功后立即写 `.verified` 标记

## v2.1.1 (2026-07-24)

### Added
- **缓存总大小显示** — `imgp save` 完成后显示 `Cache: X.XX GB`
- **DEVELOPMENT.md** — 8 节开发文档：架构、设计决策、测试指南、发布流程

### Changed
- **README 精简 70%** — 从 265 行压缩至 81 行，英文版同步
- **README 架构图** — 新增 4 个 Mermaid 流程图（save 主流程、缓存逻辑、cache/config 子命令）

## v2.1.0 (2026-07-08)

### Added
- **`--gzip/-z` 标志** — 支持 gzip 压缩导出 tar 文件
- **版本号嵌入** — `build.ps1 -Version "x.y.z"` 将版本写入二进制，`imgp -v` 正确显示
- **User-Agent** — HTTP 请求头附带 `imgp/$Version`，方便 registry 审计
- **镜像表补全** — `registry.k8s.io` 和 `quay.io` 镜像地址在 README 中可见
- **golangci-lint** — CI 新增 lint 步骤，统一代码质量
- **CI 缓存** — 缓存 Go 构建/模块目录，加速 CI 运行
- **单元测试** — 新增 `config`、`puller`、`client`、`saver`、`cmd` 包的全面测试
- **Mock registry 测试** — 无需外部依赖即可测试镜像拉取流程
- **config 包** — 分离配置管理逻辑，支持缓存目录自动计算

### Changed
- **仅支持 Windows/amd64** — 移除 Linux/macOS 构建目标和相关文档，`build.ps1` 只编译 `windows/amd64`
- **`build.ps1`** — 移除 `-All` 参数，新增 `-Version` 参数（默认 `dev`）
- **重构** — 定义 `Parallelism` 常量、重命名 `startPull`、拆分 `runSaveOne`、合并 `BuildLayer` 函数

### Fixed
- **`configPath` 自动设置** — `ConfigPath()` 优先从二进制所在目录查找 `imgp.json`，而非工作目录

## v2.0.1 (2026-07-02)

### Fixed
- **Registry retry 逻辑** — 多 mirror 重试时 `lastErr` 被最后一个 mirror 覆盖，改用 `anyRetryable` 追踪任一 mirror 的可重试性，确保重试机会不被丢失
- **退避判断** — 退避阶段同样从 `lastErr` 改为 `anyRetryable`，避免最后一个 mirror 的非重试错误中断重试
- **Context 取消不响应** — 新增 `cancelWriter` 包装器，每次 `Write` 前检查 `ctx.Err()`，使 `tarball.Write` 可被 Ctrl+C/超时中断
- **`f.Close()` 与 `f.Write()` data race** — `cancelWriter` 消除了跨 goroutine 并发关闭/写入文件的问题
- **拉取取消后继续导出** — `<-pullDone` 后加 `ctx.Err()` 检查，取消后用不完整 layer 导出损坏 tar 的问题
- **`cache clear` 文件删除竞争** — `os.Remove` 后 `e.Info()` 导致统计少报 → 先 `e.Info()` 再 `os.Remove`
- **`cache clear` 外部删除** — 文件在 `ReadDir` 和 `Remove` 之间被外部删除时整个命令报错退出 → 加 `os.IsNotExist` 守卫
- **Go 1.22 timer goroutine 泄露** — 已触发的 `time.Timer` 调用 `Stop()` 后未 drain channel，两处修复（puller + registry）

### Added
- **缓存目录持久化** — `imgp config set cache-dir` 和 `imgp config list` 支持 `cache-dir` 配置项的读写，save/cache 命令均生效
- **README 完整参数参考** — 所有命令、标志、配置项、`imgp.json` 字段的完整文档表格

### Changed
- **tar 导出取消机制** — 从 goroutine + `f.Close()` 改为 `cancelWriter` 包装器，消除 data race，支持 context 优雅取消

## v2.0.0 (2026-06-23)

### Removed
- **Web GUI** — 完全移除 `imgp gui` 子命令及相关 697 行代码，专注 CLI 工具
- **GUI 前端** — 删除 `cmd/web/` 目录（`index.html`、`app.js`、`style.css`）
- **双击启动** — 删除 `main_windows.go` 和 `main_unix.go`，`main.go` 直接调用 CLI
- **GUI 构建** — `build.ps1` 不再支持 `-GUI` 参数和 `-H=windowsgui` 链接标志

### Fixed
- **`--retry` 覆盖 manifest 拉取** — `FetchImage` 新增重试循环，`--retry` 参数同时作用于 manifest 获取和层下载
- **`--platform` 校验** — 拒绝 `linux/aarch64` 等无效架构名，提示 `did you mean "arm64"?`
- **`--retry` 上限** — `--retry` 最大值限制为 30，防止 `1 << n` 溢出
- **镜像回退认证** — 镜像地址回退时传递原始 registry 认证信息，支持带认证的镜像加速
- **goroutine panic 未恢复** — 静默模式错误协程和 tar 导出 reader 协程添加 `recover()`，避免 panic 导致进程崩溃
- **默认镜像地址** — `registry.k8s.io` 镜像从 `registry-k8s-io.m.daocloud.io` 修正为 `m.daocloud.io/registry.k8s.io`
- **网络错误提示** — 连接失败时增加镜像建议提示（如 `imgp config set mirror-map registry.k8s.io=m.daocloud.io/registry.k8s.io`）
- **`--retry` 指数退避溢出** — `1 << n` 最大移位限制为 30，防止超大镜像下载超时时间溢出为负数
- **平台解析** — `SplitN("linux/arm64/v8", "/", 2)` → `Split("/")`，正确支持三段式平台格式

### Added
- **批量下载** — `imgp save` 支持多个镜像参数依次下载，`-o` 在多镜像时报错，每个镜像自动命名 tar，某个失败继续下一个

### Changed
- **精简** — 项目代码从 ~1600 行减少至 ~900 行，删除 40+ 处 GUI 相关 bug

## v1.4.0 (2026-06-09)

### Added
- **CLI 彩色输出** — Pulling/Exporting/Done 消息使用青色/绿色，进度条使用彩色符号
- **GUI 深色模式** — 页面右上角 🌙/☀️ 切换，本地持久化
- **GUI 视觉升级** — 新配色、圆角卡片、阴影悬停效果、输入框焦点光晕
- **进度条渐变动效** — 渐变色填充 + 扫描光效动画
- **打开文件位置** — 下载完成后点击"📂 打开位置"自动打开资源管理器

### Changed
- CSS 全面重构为 CSS 变量体系，支持主题切换
- 升级至 v1.4.0

## v1.3.0 (2026-06-09)

### Added
- **Web GUI** — `imgp gui` 启动浏览器图形界面，支持双击启动（`build.ps1 -GUI`）
- **取消下载** — GUI 端可随时取消进行中的下载
- **自动停止** — 关闭浏览器标签页自动停止服务器（`beforeunload` + `sendBeacon`）
- **自动重试** — 网络错误（`unexpected EOF`、`connection reset` 等）自动重试，`--retry` 可配置次数
- **超时控制** — `--timeout` 整体超时 + `--layer-timeout` 每层超时，分钟级控制
- **导出进度** — CLI 和 GUI 均显示 tar 打包实时进度
- **OS 标准缓存路径** — Windows `%LOCALAPPDATA%`、Linux `~/.cache`、macOS `~/Library/Caches`
- **缓存管理** — `imgp cache info` 查看用量，`imgp cache clear` 清空缓存
- **缓存选项** — `--no-cache` 忽略缓存强制重下，`--cache-dir` 指定缓存目录
- **错误详情** — 进度条显示具体失败原因（如 `connection reset by peer`），取代 `download failed`
- **多镜像配置** — `mirror-map` 支持 `|` 分隔多个镜像地址
- **跨平台构建** — `build.ps1 -All` 编译 6 个平台，`build.ps1 -GUI` 编译 GUI 版
- **中英双语** — 完整的中文和英文 README + FAQ + 使用示例

### Changed
- **默认平台** — 从 `runtime.GOOS/GOARCH` 改为 `linux/amd64`（绝大多数镜像只有 Linux 版）
- **默认镜像** — Docker Hub 加速从 `docker.daocloud.io` → `docker.m.daocloud.io`（DaoCloud 正确域名）
- **默认缓存** — 从二进制同目录改为操作系统标准缓存目录
- **错误信息** — 显示所有 registry 尝试记录而非仅最后一次
- **端口号** — 默认从 `8080` → `19191`，避免与常见服务冲突
- **版本号** — 更新为 `1.3.0`

### Fixed
- **Context 取消 panic** — Ctrl+C 时下载协程安全退出，不再向已关闭 channel 发送数据
- **`--insecure` 无效** — CLI 的 `--insecure` 参数正确传递给 HTTP 传输层
- **GUI 请求上下文被取消** — goroutine 改用 `context.Background()` 避免 `r.Context()` 提前失效
- **跨平台编译失败** — Windows API（`GetStdHandle`、`GetConsoleMode`）拆入 `main_windows.go`，加 build tag
- **GUI 取消按钮不可点击** — `cancelDownload()` 嵌套在 `updateProgress()` 内部，移出到全局作用域
- **Digest/Size 错误被忽略** — CLI 和 GUI 中 `l.Digest()`、`l.Size()` 错误现在被正确处理
- **导出失败残留不完整 tar** — 先写入 `.tmp` 文件，成功后才 rename
- **build.ps1 死变量** — 移除未定义的 `$ldflags`
- **导出进度 0% 显示** — 小镜像快速完成时正确显示导出进度而非 0%
- **平台格式校验** — 拒绝 `arm64/linux` 等无效格式
- **平台 variant 支持** — `linux/arm64/v8` 三段式格式完整支持

### Security
- **密码提示** — `--password` 参数说明中提醒使用 `--password-env`
- **取消隐私清零** — 取消下载/关闭标签页后错误信息不再暴露原始注册表地址

### Removed
- **gui 分支** — 合并入 main，一个分支维护
- **`registry_mirrors` 配置字段** — 合并到 `mirror_map`，一个机制统一管理
- **中间调试标签** — v1.0.0 ~ v1.1.0 全部删除
- **冗余停止按钮** — 关闭标签页自动停止，不再需要页面内停止按钮
- **死字段/变量** — `progressDisplay.current`、`guiServer`、`cfgFile` 等

---

## v1.0.0 (2026-06-09)

### Added
- 初始发布
- Docker 镜像拉取并导出为 tar (`imgp save`)
- 多架构支持 (`--platform`)
- 国内镜像加速（`mirror_map`）
- 并行下载、断点续传、详细进度条
- 私有仓库认证
- 配置文件 `imgp.json`
- 零依赖，纯 Go 单二进制
