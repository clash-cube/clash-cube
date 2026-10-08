ClashCube for macOS 12 and later. This is a prerelease; please report issues with your macOS version and CPU architecture.

### What's new in v0.1.7

- SOCKS5 port modules serve a selected node or group on a dedicated port, independently of routing rules and outbound mode. Use them on this Mac or the LAN, with an optional password, and copy them as a SOCKS5 URL, Clash node, or Surge node.
- Port modules report listening status and offer a free port when another app occupies the configured one.
- Popovers fit short windows, and switches inside forms no longer submit the form.

- AI service checks no longer connect to AI domains. The core analyzes routing rules, while services sharing a node reuse one Cloudflare exit-IP lookup for three minutes. IP-based rules may still require DNS lookups.
- IP attributes now come from IPLocate, with shared caching by exit IP. Missing attributes do not hide routing results, and ISP addresses are not assumed to be residential.

- ClashCube updates itself. It checks GitHub every 6 hours (not on metered networks), downloads a new version in the background, and installs it when you choose Restart to Update or quit. Each download is checked against a manifest signed with the release key before it replaces the app. Turn this off in Settings → General → About.
- A globe of where connections go, with front-node chains and city-level places.
- The merged runtime configuration, shown as a diff against the profile.
- Where each rule leads and what it matches; rule providers unfold under their row.
- Modules can belong to one profile; switch a connection's proxy from Connections.

### 本次更新

- 新增 SOCKS5 端口模块：为指定节点或组提供独立端口，不受路由规则和出站模式影响。支持本机或局域网访问、可选密码，并可复制为 SOCKS5 URL、Clash 节点或 Surge 节点。
- 端口模块显示监听状态；端口被其他应用占用时，可选择空闲端口。
- 修复弹出菜单超出较矮窗口，以及表单内开关触发表单提交的问题。
- AI 服务检测不再连接 AI 域名：由内核分析路由规则，同一节点的各服务共用一次 Cloudflare 出口 IP 查询，缓存三分钟。IP 类规则仍可能需要 DNS 解析。
- IP 属性改用 IPLocate，按出口 IP 共用缓存。属性查询失败不影响路由展示，运营商 IP 不再被推断为住宅 IP。
- 支持自动更新：每 6 小时检查一次 GitHub（按流量计费的网络上不检查），在后台下载新版本，点“重启以更新”或退出时安装。替换前会用发布密钥签名的清单校验下载内容。可在“设置 → 通用 → 关于”中关闭。
- 新增连接地球视图，显示前置节点链路和城市级位置。
- 可查看合并后的运行配置及其与原配置的差异。
- 显示每条规则的去向和匹配内容；规则集可在所在行内展开。
- 模块可只属于某个配置；可在连接页直接切换连接的代理。

### Downloads

- **Apple Silicon (M1 and later):** choose the `macos-arm64.dmg` file.
- **Intel:** choose the `macos-amd64.dmg` file.
- ZIP packages are used by the in-app updater. `SHA256SUMS` contains checksums for all four DMG and ZIP packages; run `shasum -a 256 -c SHA256SUMS` in a folder containing all four.

### Installation

Open the DMG and copy ClashCube to Applications. These builds are ad-hoc signed and **not notarized by Apple**. After the first blocked launch, open System Settings → Privacy & Security → Open Anyway if you trust this download.

Import a profile, then enable System Proxy or Enhanced Mode. Enhanced Mode installs a privileged helper and requests your administrator password the first time. Official builds include the signature used to verify subsequent helper updates.

### 已知限制

当前为测试版本，支持 macOS 12 及以上。Apple Silicon 选择 `arm64`，Intel 选择 `amd64`。应用尚未经过 Apple 公证；首次启动被拦截后，请在确认下载可信的前提下，通过“系统设置 → 隐私与安全性 → 仍要打开”放行。首次启用增强模式需要管理员授权安装助手。
