# ClashFerry

A macOS menu bar app for [mihomo](https://github.com/MetaCubeX/mihomo), built with Go, Wails v3, React and TypeScript.
mihomo is compiled in from source (`third_party/mihomo`, a submodule of
[Demogorgon314/mihomo@openconnect-support](https://github.com/Demogorgon314/mihomo/tree/openconnect-support)),
so the app is one binary: the GUI, and the same executable re-run as `clashferry core` for the proxy core.

See [docs/design.md](docs/design.md) for the architecture and plan.

## Build

Requires Go 1.25+, Node + pnpm, and the Wails v3 CLI (`go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.27`).

```sh
git submodule update --init
wails3 task deps        # pnpm install
wails3 task run         # dev build, opens the main window
wails3 task app         # bin/ClashFerry.app (ad-hoc signed)
wails3 task install     # copy it to ~/Applications and reopen (needed for notifications)
wails3 task test
```

`CLASHFERRY_HOME=/tmp/x` runs on a separate data directory (default `~/Library/Application Support/ClashFerry`).

## Layout

| Path | What |
|---|---|
| `main.go` | dispatches the `gui` / `core` / `helper` roles |
| `internal/core` | runs mihomo in-process (the `core` role) |
| `internal/coremgr` | starts, watches and stops the core process |
| `internal/backend` | settings + profiles + core + system proxy, without windows |
| `internal/runtimecfg` | the profile with the app's settings laid over it |
| `internal/gui` | Wails app: tray, panel, main window, services, events |
| `frontend/` | React UI |

## License

mihomo is GPL-3.0 and is linked into the binary, so ClashFerry is distributed under GPL-3.0 too.
