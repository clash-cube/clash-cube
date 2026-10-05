ClashCube for macOS 12 and later. This is a prerelease; please report issues with your macOS version and CPU architecture.

### Downloads

- **Apple Silicon (M1 and later):** choose the `macos-arm64.dmg` file.
- **Intel:** choose the `macos-amd64.dmg` file.
- `SHA256SUMS` contains checksums for both downloads. Run `shasum -a 256 -c SHA256SUMS` in a folder containing both DMGs.

### Installation

Open the DMG and copy ClashCube to Applications. These builds are ad-hoc signed and **not notarized by Apple**. After the first blocked launch, open System Settings → Privacy & Security → Open Anyway if you trust this download.

Import a profile, then enable System Proxy or Enhanced Mode. Enhanced Mode installs a privileged helper and requests your administrator password the first time. Official builds include the signature used to verify subsequent helper updates.

### 已知限制

当前为测试版本，支持 macOS 12 及以上。Apple Silicon 选择 `arm64`，Intel 选择 `amd64`。应用尚未经过 Apple 公证；首次启动被拦截后，请在确认下载可信的前提下，通过“系统设置 → 隐私与安全性 → 仍要打开”放行。首次启用增强模式需要管理员授权安装助手。
