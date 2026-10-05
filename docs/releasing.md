# 发布 ClashCube

当前使用 ad-hoc 签名发布 DMG，不做 Apple 公证。工作流对 `v0.1.0` 这样的三段数字 tag 运行，在 Apple Silicon 和 Intel runner 上分别测试、编译、验证签名，然后生成 **prerelease 草稿**。版本号不带 `-beta` 等后缀，避免把它写进要求数字版本的 Info.plist；测试版身份由 GitHub Release 的 prerelease 标记表示。

私有仓库可以使用同一流程，但 Release 仅对有仓库权限的人可见，Actions 使用私有仓库的配额。以后公开仓库无需修改工作流。草稿不会因为仓库公开就自动发布。

## 首次配置

本机打包需要 Python 3（macOS runner 已提供）。`wails3 task dmg` 会在忽略提交的 `bin/dmg-tools` 中安装固定版本的 dmgbuild，生成带应用图标、箭头和 `/Applications` 快捷入口的 Finder 安装窗口，无需 Finder 自动化权限。

1. 在仓库启用 GitHub Actions，并确保组织策略允许工作流使用 `contents: write` 创建 Release。
2. 将现有 helper 私钥文件的内容存入 repository Actions secret `CLASHCUBE_UPDATE_KEY`。必须与 `internal/updatesig.PublicKey` 对应，不要重新生成或输出私钥。可以从本机直接通过标准输入设置：

   ```sh
   gh secret set CLASHCUBE_UPDATE_KEY --repo clash-cube/clash-cube < "${CLASHCUBE_UPDATE_KEY:-$HOME/.config/clashcube/update.key}"
   ```

   工作流只在 tag 构建时把它写入 runner 临时文件，用完删除；缺少密钥或验签失败都会阻止 Release。该密钥授予 helper 更新能力，仓库写权限和 tag 推送权限应只授予可信维护者。
3. 确认 `third_party/mihomo` 的提交已推送到公开 fork，且 `scripts/sync-replaces.sh` 的结果已同步。Actions 会递归检出子模块。

## 每次发布

1. 更新 `docs/release-notes.md` 的功能说明和已知问题，提交准备发布的改动。
2. 本地执行 `wails3 task test` 和 `wails3 task release VERSION=v0.1.0`。后者需要 helper 签名密钥，生成本机架构的 `bin/release/ClashCube-0.1.0-macos-<arch>.dmg`。
3. 在干净工作区给审核过的提交打 tag，再推送提交和 tag：

   ```sh
   git tag -a v0.1.0 -m 'Release v0.1.0'
   git push origin main
   git push origin v0.1.0
   ```

4. 等待 Release workflow 的两个架构都成功。草稿包含两个 DMG 和 `SHA256SUMS`。下载后检查校验和，分别在两种架构上验证启动、配置导入、代理和 helper 功能，尤其要验证声明支持的最低 macOS 版本。
5. 检查 Release 说明，再手动发布草稿。首版保留 prerelease 标记；确认稳定后可取消。

工作流不会覆盖已有 Release。构建失败时可以重跑；若已有草稿，应先检查其内容，再决定是否删除草稿重跑。不要移动已发布的 tag，也不要替换已经发布的安装包。重新构建会产生新的 helper 构建时间，helper 只接受比自身新的构建；所有架构在同一次 workflow 使用同一时间。

`wails3 task dmg` 仍可用于没有密钥的个人构建；`release` 则强制验证 helper 签名。helper 签名不等于 Apple Developer ID 签名，也不代表应用自动更新。

## 后续 Apple 公证

需要 Developer ID Application 证书、Hardened Runtime 和公证凭据。完成签名、公证与 staple 后再生成发布 DMG，并重新验证 helper 签名及首次启动体验。现有 Apple Development 证书不能替代发行证书。

- [Apple 公证文档](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution)
- [GitHub macOS runners](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)
