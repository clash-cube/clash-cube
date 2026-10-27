# ClashCube OpenWrt 0.2.0-4

First public ARM64 OpenWrt development release, verified in the pinned Lean
QWRT Docker lab with fw3/iptables-legacy and Lua LuCI. Other architectures,
fw4/nftables and physical-router hardware are not yet validated.

## Features / 功能

- Embedded LuCI panel with profiles, subscriptions, proxy policies, modules,
  connections, logs and traffic. 页面内嵌 LuCI，无需打开新窗口。
- Independent core and management packages. Management upgrades preserve the
  running core and existing proxy connections. 管理界面升级不停止内核。
- IPv4 TCP interception and DNS integration preserving local/DHCP names;
  IP/subnet/MAC include and bypass lists. 支持设备访问名单。
- Signed core updates and scheduled GEO updates that continue with management
  offline. 内核支持签名独立升级，GEO 可定时更新。
- Persistent configuration, optional RAM/external caches, UPX LZMA by default,
  and downloadable diagnostics. 默认 UPX 压缩，支持诊断报告。

## Install / 安装

Download all three `.ipk` files and install them together with `opkg install`.
Dependencies come from the router's configured feeds. The core and web packages
require `aarch64_generic`; the LuCI entry requires legacy Lua LuCI compatibility.
Open **Services → ClashCube** after installation. The starter profile is DIRECT
and transparent interception defaults to off. GEO databases download separately
when needed; they are not included in the packages.

下载三个 IPK，通过 opkg 一起安装，随后进入「服务 → ClashCube」。初始配置直连，
透明接管默认关闭。仅支持已验证的 ARM64、fw3 环境；GEO 数据按需另行下载。

The official signed core update feed is:

```text
https://raw.githubusercontent.com/clash-cube/clash-cube/main/openwrt/updates/aarch64_generic.json
```

New installations select this feed automatically. Existing installations with
an explicitly empty/custom feed retain it; configure the URL under
**Settings → Network → Router maintenance**. Checking and applying a core update
are explicit actions. Core upgrades close existing proxy connections, while an
explicitly stopped core remains stopped. Management and LuCI still update via
opkg. `SHA256SUMS` covers the public release artifacts, and `router-update.json`
contains the signed version, package size and hash.

## Limits / 限制

Transparent interception covers IPv4 TCP and DNS on one LAN only. Other UDP,
IPv6, multiple LAN zones, router-originated interception and TUN are outside
this release. Device lists control transparent interception and forced DNS;
explicit proxy access and shared router DNS have separate semantics.

See [OpenWrt documentation](https://github.com/clash-cube/clash-cube/blob/main/docs/openwrt.md)
for storage, backup and recovery behavior.
