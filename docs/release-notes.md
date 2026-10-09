ClashCube for macOS 12 and later and Windows 10/11. This is a prerelease; please report issues with your OS version and CPU architecture.

### What's new in v0.1.10

- Updated mihomo to v1.19.29-openconnect.10, including upstream fixes that preserve fake-IP allocations across configuration reloads and prevent abandoned DoH dials from piling up, plus TUN, WireGuard, and OpenConnect updates.
- MIPS is now the default TUN stack for new settings. Existing saved stack choices are preserved.

### 本次更新

- 更新 mihomo 至 v1.19.29-openconnect.10，包含配置重载时保留 fake-IP 分配状态、避免已放弃的 DoH 拨号堆积等上游修复，以及 TUN、WireGuard 和 OpenConnect 更新。
- 新设置默认使用 MIPS TUN 协议栈，已保存的协议栈选择保持不变。

### Downloads

- **Apple Silicon (M1 and later):** choose the `macos-arm64.dmg` file.
- **Intel:** choose the `macos-amd64.dmg` file.
- **Windows x64:** choose the `windows-amd64.zip` file.
- **Windows ARM64:** choose the `windows-arm64.zip` file.
- macOS ZIP packages are used by the in-app updater. `SHA256SUMS` contains checksums for all six DMG and ZIP packages; run `shasum -a 256 -c SHA256SUMS` in a folder containing all six.

### Installation

On macOS, open the DMG and copy ClashCube to Applications. These builds are ad-hoc signed and **not notarized by Apple**. After the first blocked launch, open System Settings → Privacy & Security → Open Anyway if you trust this download.

Import a profile, then enable System Proxy or Enhanced Mode. Enhanced Mode installs a privileged helper and requests your administrator password the first time. Official builds include the signature used to verify subsequent helper updates.

On Windows, extract the ZIP and run the executable. WebView2 Runtime must be installed. Enhanced Mode requests UAC authorization each time you reopen the app; updates are downloaded manually from the release page.

### 已知限制

当前为测试版本，支持 macOS 12 及以上和 Windows 10/11。macOS Apple Silicon 选择 `macos-arm64.dmg`，Intel 选择 `macos-amd64.dmg`。应用尚未经过 Apple 公证；首次启动被拦截后，请在确认下载可信的前提下，通过“系统设置 → 隐私与安全性 → 仍要打开”放行。首次启用增强模式需要管理员授权安装助手。

Windows x64 选择 `windows-amd64.zip`，ARM64 选择 `windows-arm64.zip`，解压后运行，需要预先安装 WebView2 Runtime。增强模式在每次重新打开应用后需要 UAC 授权；更新需从发布页手动下载。Windows 自定义 URL 协议尚未注册，请在应用内粘贴导入订阅。
