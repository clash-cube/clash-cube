# Notes for coding agents

MihomoBar is a macOS menu bar app for mihomo: Go + Wails v3 (beta) + React/TS.
`docs/design.md` has the architecture, the UI spec and the plan; read it
before larger changes. The user writes in Chinese; reply in Chinese.

## One binary, three roles

`main.go` dispatches on `os.Args[1]`:

| Role | Entry | Runs as | What |
| --- | --- | --- | --- |
| (none) | `internal/gui.Run` | user | Wails app: tray, panel, main window |
| `core` | `internal/core.Main` | user, or root via helper | mihomo in-process (`hub.Parse`) |
| `helper serve` | `internal/helper` | root (LaunchDaemon) | starts `core` as root for TUN |

The GUI never touches mihomo's package state. It talks to the core only
over the REST API (`internal/mihomoapi`), at a random `127.0.0.1` port with a
random secret made at every start (`coremgr.NewController`). mihomo is all
package-level singletons, and `/restart` would re-exec the host process, so
it must not run inside the GUI.

`internal/backend` is the app without windows: settings, profiles, core,
system proxy. `internal/gui` adapts it to Wails services (`services.go`) and
events (`events.go`). Put behaviour in `backend`, not in `gui`.

## mihomo is a submodule of a fork

- `third_party/mihomo` is `Demogorgon314/mihomo` on branch
  `openconnect-support`, compiled in from source
  (`replace github.com/metacubex/mihomo => ./third_party/mihomo`).
- `go.mod` must carry the fork's own `replace` lines, because dependents
  don't inherit them. After bumping the submodule, run
  `scripts/sync-replaces.sh` (it rewrites the block between `BEGIN`/`END
  mihomo replaces`).
- Fixes that belong in mihomo go in the fork, not in workarounds here. One
  example is the darwin process lookup in `component/process`. Commit in the
  submodule, then bump the pointer here.
- Push fork changes using an account authorized for that repository. If a
  separate clone is needed, ask for its location before using it.
- Push the fork before pushing a submodule bump here. Otherwise a fresh
  clone's `git submodule update` fails.

## Build, run, test

`task` isn't installed; the Taskfile runs through `wails3 task`.

```sh
wails3 task build:dev     # bindings + frontend (dev) + go build → bin/mihomobar
wails3 task app           # release bin/MihomoBar.app (ad-hoc signed)
wails3 task install       # app, then quit/replace/reopen ~/Applications/MihomoBar.app
go test ./internal/...    # backend/helper tests start real cores
cd frontend && npx tsc    # type check
```

- After changing an exported method of a Wails service, or a type it returns,
  regenerate the bindings with `wails3 task bindings`. They live in
  `frontend/bindings` and are committed.
- Run development instances on a separate home and port. Never use the
  user's real data directory (the default, under Application Support):
  `MIHOMOBAR_HOME=/tmp/mbhome ./bin/mihomobar`. Set that home's `mixedPort`
  to something other than 7890.
- Debug switches: `MIHOMOBAR_SHOW=main|panel|menu` opens a window or the tray
  menu at start. `MIHOMOBAR_VIEW=proxies` (or `settings#tun`) picks the page.
- Tests that need a core re-exec the test binary as `core`; see `TestMain` in
  `internal/backend` and `internal/helper`.

## Don't disturb the user's machine

An installed app at `~/Applications/MihomoBar.app` and root helper may be running.
When stopping development instances, match them exactly,
e.g. `pkill -f "^./bin/mihomobar"`. A loose pattern also matches the user's
app and the helper's core.

These all change the real machine. Ask before doing any of them:
- turning on the system proxy (`networksetup`) or TUN;
- installing or uninstalling the helper (it prompts for an admin password);
- writing to `/Library`, the LaunchAgents folder, or the user's data
  directory.

## Service mode / helper (security-sensitive)

`internal/helper` is a root daemon. Keep its surface small:
- one JSON-lines request per connection over `/var/run/mihomobar-helper.sock`;
- the peer uid is checked with `LOCAL_PEERCRED` and must be the installing
  user or root;
- `core` runs only on `<data>/core/runtime.yaml`, refusing symlinks and
  foreign owners, with a controller on `127.0.0.1`.

Don't add operations that run arbitrary commands or paths. `helper_test.go`
covers the refusals; extend it for anything new. The helper is installed by
copying this same executable to `/Library/PrivilegedHelperTools` via
osascript (`helper.Install`). The Settings page offers a reinstall when the
hashes differ.

## Configuration flow

The profile file is never changed in place. `runtimecfg.Build` overlays the
app's settings onto it and writes `core/runtime.yaml` (0600):
- ports, mode, allow-lan, ipv6, log level, tun, find-process-mode;
- the controller and secret, with every other controller removed.

Every start or reload runs `core -t` on that file first. A profile that fails
is refused, and the previous one stays (`TestCoreLifecycle`). Profile
switches hot-reload with `PUT /configs`; they don't restart.

## Frontend conventions

- Take colours, radii and shadows from the tokens in
  `src/styles/tokens.css`, and easing from `--ease`, `--ease-out` and
  `--spring`; don't add new ones.
- Reuse the animated parts: `Segmented` (sliding thumb), `Popover` (grows
  from its anchor), `Fold` (staggered unroll, children get `.stagger` and
  `--i`), `Switch`, `toast`.
- Interaction follows Surge (`docs/design.md` §10). The tray menu
  (`internal/gui/traymenu.go`) is rebuilt from state, not mutated.
- Every user-facing string goes through `t()`. Add its Chinese to
  `src/i18n.ts`; the key is the English text.
- State comes from Go events (`state`, `traffic`, `memory`, `log`,
  `profiles`) in `src/store.ts`. Poll only what has no event
  (connections, rules) with `usePoll`.

## Verifying UI changes

Take screenshots of the window, not the whole screen. Other windows of the
same name may be open. Get the window ID from the dev instance's PID with
`CGWindowListCopyWindowInfo`, then run `screencapture -l <id>`. To fill the
Connections and Overview pages, send traffic through the dev instance's
mixed port with `curl -x http://127.0.0.1:<port> --limit-rate …`.

## Commits

Commit only when asked. Commit messages are in English: a short imperative
subject, then a body that explains why.
