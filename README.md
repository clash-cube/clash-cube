<div align="center">

<img src="docs/images/icon.svg" width="128" height="128" alt="ClashCube icon">

# ClashCube

**A native-feeling macOS menu bar app for [mihomo](https://github.com/MetaCubeX/mihomo).**
One binary, the core compiled in, every config checked before it goes live.

[![License: GPL-3.0](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](LICENSE)
![macOS 12+](https://img.shields.io/badge/macOS-12%2B-black?logo=apple)
![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)
![Wails v3](https://img.shields.io/badge/Wails-v3-DF0000)

English · [简体中文](README.zh-CN.md)

<img src="docs/images/hero.png" alt="ClashCube main window and menu bar panel" width="100%">

</div>

## Why ClashCube

- **One app, no sidecar.** mihomo is compiled into the app from source. The
  GUI, the proxy core and the privileged helper are the same executable in
  different roles. You don't download a core, pick its version or replace it
  by hand; it upgrades with the app.
- **A bad config can't take you offline.** Your profiles are never edited in
  place. ClashCube builds a runtime config from the profile, your modules,
  rules and settings, runs `core -t` on it, and only then starts or hot-reloads
  the core. If the core rejects it, the running config stays and you see the
  core's error.
- **Your settings survive subscription updates.** Modules (Clash Verge–style
  merge syntax) and your own rules are layered on top of every profile, so
  changes don't disappear the next time the provider pushes an update.
- **Secure by default.** The control API listens on `127.0.0.1` with a random
  port and a random secret on every start. The root helper only accepts four
  operations, checks the caller's uid, and only runs the core on your own
  `runtime.yaml`. Helper updates are verified with an ed25519 signature, so
  upgrading the app doesn't ask for your password again.
- **Feels like a Mac app.** Interaction follows Surge for Mac: a fast tray
  panel, a compact custom-drawn tray menu, global hotkeys, light and dark
  themes, reduced-motion support, and an Overview that tells you whether
  traffic is actually being proxied.
- **Knows what network you're on.** Rules per Wi-Fi SSID, wired or any other
  network switch the profile, mode, system proxy, TUN and group selection.
  On networks macOS marks as metered (hotspot, Low Data Mode), background
  updates and checks pause.

## Screenshots

<table>
  <tr>
    <td width="50%"><img src="docs/images/proxies.png" alt="Proxies"><br><sub><b>Proxies</b>: groups and nodes with live latency; test one node, a group, or everything.</sub></td>
    <td width="50%"><img src="docs/images/connections.png" alt="Connections"><br><sub><b>Connections</b>: grouped by app, with the matched rule and full proxy chain; filter, pause, close.</sub></td>
  </tr>
  <tr>
    <td><img src="docs/images/usage.png" alt="Traffic statistics"><br><sub><b>Traffic statistics</b>: per app, host, policy and network, by hour, for 90 days.</sub></td>
    <td><img src="docs/images/modules.png" alt="Modules"><br><sub><b>Modules</b>: one-click presets for DNS, ad blocking, sniffing, and per-service routing (OpenAI, Claude, Google, YouTube, Telegram…).</sub></td>
  </tr>
  <tr>
    <td><img src="docs/images/netrules.png" alt="Network rules"><br><sub><b>Network rules</b>: different behaviour per Wi-Fi, wired and other networks.</sub></td>
    <td><img src="docs/images/tun.png" alt="Enhanced mode"><br><sub><b>Enhanced Mode (TUN)</b>: a privileged helper plus leak protection for IPv6, DNS, STUN and lookups.</sub></td>
  </tr>
  <tr>
    <td><img src="docs/images/rules.png" alt="Rules"><br><sub><b>Rules</b>: active rules with hit counts, providers, and a lookup that explains where a host would go.</sub></td>
    <td><img src="docs/images/proxies-dark.png" alt="Dark mode"><br><sub><b>Dark mode</b>: follows the system, or pick one.</sub></td>
  </tr>
</table>

<p align="center">
  <img src="docs/images/tray.png" width="560" alt="Menu bar panel and tray menu"><br>
  <sub>Left click opens the panel, right click opens the menu. Switch mode, nodes, system proxy and TUN without opening a window.</sub>
</p>

## Features

**Everyday use**
- Menu bar panel and a right-click menu with every group, latency, connectivity and top apps
- Outbound mode (Rule / Global / Direct), system proxy and TUN as separate switches
- Tray icon shows whether traffic is really being taken over; optional live speed beside it
- Global hotkeys (Carbon, no Accessibility permission), with conflict detection
- "Copy shell export command" for the terminal
- Notifications when the core stops, a profile update is refused or fails, a network rule applies, or another app changes the system proxy

**Profiles and rules**
- Import from a subscription URL, a local file, or `clash://` / `clashmeta://install-config` links
- Shows remaining traffic and expiry from `subscription-userinfo`; scheduled updates
- Hot-reloads when the profile in use changes on disk
- Modules: deep merge, `+key` / `key+` / `key!`, `prepend-/append-rules`, `-proxies`, `-proxy-groups`
- Service routing: send a service to a region, a list of nodes or keywords, with `REJECT` instead of a silent direct fallback
- Your own rules, added by hand or from a connection, placed ahead of the profile's

**Visibility**
- Overview: live throughput, today's traffic, proxied share, top clients, and four connectivity probes (router, DNS, internet, proxy)
- Connections: active and closed, grouped by app, column picker, search and filters
- Traffic statistics by app, host, policy and network; app helpers are counted under their parent app
- Rule hit counts and a host lookup that shows which rule matches
- Core logs and an app event log

**Network**
- Network rules by SSID / wired / other, deterministic and reversible
- Handles network switches and wake from sleep: flushes DNS and fake-ip, closes stale connections, re-applies the system proxy
- Detects when another app overwrites the system proxy and offers to take it back
- TUN stacks: System, gVisor, Mixed, MIPS; ICMP forwarding
- Leak protection: route IPv6 into TUN, take over DNS, block STUN over UDP, resolve along the rules

**Core**
- mihomo built from source, from a fork that adds an OpenConnect (AnyConnect-compatible) outbound and the hooks ClashCube needs
- The core runs as its own process (as root only in TUN mode), so a crash can't take the UI down with it

## Install

> [!NOTE]
> ClashCube isn't notarized yet; release builds are ad-hoc signed.

1. Download the `macos-arm64.dmg` (Apple Silicon) or `macos-amd64.dmg` (Intel) asset from [Releases](https://github.com/clash-cube/clash-cube/releases) and drag the app to `Applications`.
2. After the first blocked launch, open **System Settings → Privacy & Security → Open Anyway** if you trust the download.
3. Import a subscription from **Profiles → Import**, then turn on **System Proxy** or **Enhanced Mode**.

Enhanced Mode installs a LaunchDaemon helper the first time you turn it on and asks for your administrator password once.

## Build from source

Maintainers: see [the release workflow](docs/releasing.md) for signing setup, version tags and Release drafts. While the repository is private, downloads require repository access.

Requirements: macOS 12+, Xcode command line tools, Go 1.25+, Node with pnpm, and the Wails v3 CLI:

```sh
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.27
```

```sh
git clone --recursive https://github.com/clash-cube/clash-cube.git
cd clash-cube
wails3 task deps        # submodule + pnpm install
wails3 task run         # dev build, opens the main window
wails3 task app         # bin/ClashCube.app (ad-hoc signed)
wails3 task dmg         # bin/ClashCube.dmg
wails3 task install     # replace ~/Applications/ClashCube.app and reopen it
wails3 task test        # Go tests (start real cores), type check, frontend tests
```

To try a build without touching your real data, give it its own home and port:

```sh
CLASHCUBE_HOME=/tmp/cc ./bin/clashcube    # then set Mixed port to something other than 7890
```

## How it works

```
ClashCube.app/Contents/MacOS/clashcube
├── (no argument)   GUI: Wails v3 tray panel + main window; talks to the core over REST/WebSocket only
├── core            mihomo in-process, controller on 127.0.0.1:<random> with a random secret
└── helper serve    root LaunchDaemon that starts `core` for TUN; 4 operations, peer-uid checked
```

| Path | What |
|---|---|
| `main.go` | dispatches the GUI / `core` / `helper` roles |
| `internal/backend` | the app without windows: settings, profiles, core, system proxy, network |
| `internal/runtimecfg` | builds `runtime.yaml` from the profile, modules, rules and settings |
| `internal/coremgr` | starts, watches and stops the core process |
| `internal/helper` | the privileged helper and its signed self-update |
| `internal/gui` | Wails services, events, tray menu and windows |
| `frontend/` | React + TypeScript UI |
| `third_party/mihomo` | the mihomo fork, as a submodule |

[docs/design.md](docs/design.md) (Chinese) covers the architecture, the security model and the exact config build rules.

## Contributing

Issues and pull requests are welcome. Before a larger change, read
[docs/design.md](docs/design.md) and [AGENTS.md](AGENTS.md); the latter lists
the build, test and UI conventions. Fixes that belong in mihomo go to the
[fork](https://github.com/Demogorgon314/mihomo/tree/openconnect-support)
rather than a workaround here.

## Acknowledgements

- [mihomo](https://github.com/MetaCubeX/mihomo), the proxy core
- [Wails](https://wails.io), Go + web UI for desktop apps
- [Surge for Mac](https://nssurge.com), whose interaction model ClashCube follows
- [Clash Verge Rev](https://github.com/clash-verge-rev/clash-verge-rev) and [zashboard](https://github.com/Zephyruso/zashboard), for the module syntax and latency testing behaviour
- [three.js](https://threejs.org), which draws the globe
- [Solar System Scope](https://www.solarsystemscope.com/textures/), whose Earth day and night textures (CC BY 4.0, resized) the globe uses

## License

[GPL-3.0](LICENSE). mihomo is GPL-3.0 and is linked into the binary, so ClashCube is distributed under the same license.
