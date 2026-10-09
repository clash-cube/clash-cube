# Windows

Windows 10/11，支持 amd64 和 arm64。GUI 使用 WebView2 Runtime（需要预先安装），以普通用户运行；关闭主窗口后留在托盘，从托盘菜单退出。

## 构建

需要 Go（版本须满足 go.mod 及其依赖）、Node.js 24、pnpm 和 Git。

```powershell
git submodule update --init --recursive
./scripts/build-windows.ps1
# 或 wails3 task windows
./scripts/build-windows.ps1 -Arch arm64
```

产物为 `bin/clashcube-windows-amd64.exe` / `bin/clashcube-windows-arm64.exe`，包含前端、mihomo、Wintun 和 Windows 图标/清单。脚本使用固定版本的 go-winres，生成普通用户权限、Per Monitor V2 DPI、长路径资源。`-Dev` 保留开发工具，`-SkipFrontend` 复用已构建的前端。

Windows CI 构建两个架构，在 amd64 上运行测试。macOS 构建命令保持原样。

## 平台行为

- 数据放在 `%APPDATA%\ClashCube`，仍可用 `CLASHCUBE_HOME` 隔离开发数据。相同数据目录只允许一个 GUI 实例。
- 系统代理使用当前用户的 WinINet LAN 设置，同时设置 HTTP、HTTPS、SOCKS 和绕过列表，并通知应用刷新；不修改 WinHTTP 机器代理。IPv4 CIDR 绕过项转换为等价通配符，IPv6 CIDR 会明确拒绝。关闭时仅清理由本应用接管的代理。
- 登录启动使用当前用户的 Run 注册表项，识别任务管理器禁用状态。开发实例指定 `CLASHCUBE_HOME` 时不自动同步登录启动项。
- 主窗口使用 Windows 原生标题栏，隐藏内容区域中重复的 Logo 和名称。托盘支持左右键、面板、菜单和深浅色图标，内核运行时显示 R/G/D 模式角标；点击动画尚未实现。速度显示在托盘提示中。程序选择器列出进程路径，Windows 暂用默认应用图标。
- 推荐全局快捷键为 Ctrl+Alt+Shift 加字母；复制终端代理命令使用 PowerShell 语法。配置文件由记事本打开，位置由文件资源管理器显示。
- 网络变化、出口接口、端口占用和延迟使用 Windows API。Wi-Fi 名称读取受 Windows 定位权限控制；保存网络列表目前仅包含已连接的网络。计费网络状态由 Network List Manager 提供。
- 应用更新检查仍验证签名清单，Windows 通过发布页手动下载替换；不会下载或安装清单中的 macOS bundle。自定义 URL 协议的系统注册尚未提供，订阅可在应用内粘贴导入。

## TUN 的权限边界

Windows 不安装 LaunchDaemon，也不要求整个 GUI 以管理员身份运行。用户开启 TUN 时通过 UAC 启动同一可执行文件的 `helper serve` 角色，只为当前 GUI 会话服务；每次重新打开应用，需要重新授权，TUN 和服务模式不跨会话恢复。

通信使用每会话随机命名的本机命名管道，ACL 只允许启动 GUI 的用户、SYSTEM 和管理员。辅助进程只运行自身的 `core` 角色，只接受已授权数据目录下的 `core/runtime.yaml`，拒绝重解析点、其他路径和非回环控制接口。管道断开会停止内核；GUI 退出后辅助进程也退出。“结束提权会话”会停止辅助进程，无常驻安装或免密自更新。

与 macOS 一样，内核读取用户可修改的配置；能以该用户身份运行的程序仍可影响其配置。这不是针对同一用户恶意代码的隔离边界。

## 验证

```powershell
go test -short ./internal/...
go test -timeout 8m ./internal/... # 部分测试下载 GEO 数据
pnpm --dir frontend test
pnpm --dir frontend run build

$env:CLASHCUBE_HOME = "$PWD\bin\dev-home"
# 在该目录的 settings.json 中选一个非 7890 的 mixedPort，关闭 systemProxy 和 tun。
$env:CLASHCUBE_SHOW = 'main'
./bin/clashcube-windows-amd64.exe
```

代理开关、UAC/TUN、登录启动均属于真实系统集成，自动测试不会更改这些机器设置。

## mihomo fork

Windows 构建要求 fork 中 `component/process/process_darwin_libproc.go` 显式声明 `//go:build darwin`；文件名中的 `darwin` 不在最终后缀位置，不会自动限定平台。该修复属于 mihomo fork。可以先在本地提交子模块修复及上层指针；推送时必须先推送子模块提交，再推送引用它的主项目提交，不能仅发布上层修改。
