# Changelog

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
