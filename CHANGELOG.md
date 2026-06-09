# Changelog

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
