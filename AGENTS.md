# Notes for coding agents

ClashCube is a macOS menu bar app for mihomo: Go + Wails v3 (beta) + React/TS.
`docs/design.md` explains the architecture, the security model and how the
runtime config is built; read it before larger changes.

## Architecture in brief

`main.go` dispatches on `os.Args[1]`: no argument runs the GUI
(`internal/gui`), `core` runs mihomo in-process (`internal/core`), and
`helper serve` is the root LaunchDaemon that starts `core` as root for TUN
(`internal/helper`).

- The GUI never touches mihomo's package state. It talks to the core only over
  the REST API (`internal/mihomoapi`) on `127.0.0.1`, with a port and secret
  that are random on every start.
- `internal/backend` is the app without windows. `internal/gui` adapts it to
  Wails services (`services.go`) and events (`events.go`). Put behaviour in
  `backend`, not in `gui`.
- Profile files are never changed in place. `runtimecfg.Build` writes
  `core/runtime.yaml`, and every start or reload runs `core -t` on it first.
  If the core rejects it, the previous config stays.

## Module kinds

Modules made from a form (a route, a port) implement `modules.Kind`
(`internal/modules/kind.go`); the frontend has a matching `ModuleKind` in
`frontend/src/components/modules/`. To add a kind: a Go type implementing
`Kind`, a pointer field on `Module` plus a case in `Module.Kind`/`withKind`/
`kinds`, then a `ModuleKind` file listed in `registry.ts`. Don't add
`if m.Route != nil` style branches elsewhere. See `docs/design.md` §3.2.

## mihomo is a submodule of a fork

- `third_party/mihomo` is `Demogorgon314/mihomo` on branch
  `openconnect-support`. It is compiled from source through a `replace`.
- `go.mod` must also carry the fork's own `replace` lines, because dependents
  don't inherit them. After bumping the submodule, run
  `scripts/sync-replaces.sh`.
- Fixes that belong in mihomo go in the fork, not in workarounds here. Commit
  in the submodule, then bump the pointer here.
- Push fork changes using an account authorized for that repository. If a
  separate clone is needed, ask for its location before using it.
- Push the fork before you push a submodule bump here. Otherwise
  `git submodule update` fails on a fresh clone.

## Build, run, test

`task` isn't installed, so run the Taskfile through `wails3 task`.

```sh
wails3 task build:dev     # bindings + frontend (dev) + go build → bin/clashcube
wails3 task app           # release bin/ClashCube.app (ad-hoc signed)
wails3 task install       # app, then quit/replace/reopen ~/Applications/ClashCube.app
go test ./internal/...    # backend/helper tests start real cores
cd frontend && npx tsc    # type check
```

- After changing an exported method of a Wails service, or a type it returns,
  regenerate the bindings with `wails3 task bindings`. They are committed.
- Run development instances with their own home and port, never on the user's
  real data directory: `CLASHCUBE_HOME=/tmp/mbhome ./bin/clashcube`. Set
  `mixedPort` in that home to something other than 7890.
- Debug switches: `CLASHCUBE_SHOW=main|panel|menu` opens a window or the tray
  menu at start. `CLASHCUBE_VIEW=proxies` (or `settings#tun`, or `globe` for global
  connections) picks the page.
  `CLASHCUBE_UPDATE_FEED=<url of an update.json>` checks for app updates there
  instead of GitHub; sign a test manifest with `scripts/signhelper -manifest`.
- Tests that need a core re-exec the test binary as `core` (see `TestMain` in
  `internal/backend` and `internal/helper`).

## Don't disturb the user's machine

An installed `~/Applications/ClashCube.app` and root helper may be running.
To stop development instances, match them exactly, e.g.
`pkill -f "^./bin/clashcube"`. A looser pattern also kills the user's app and
the helper's core.

Ask first before any of these:
- turning on the system proxy (`networksetup`) or TUN;
- installing or uninstalling the helper (it prompts for an admin password);
- writing to `/Library`, the LaunchAgents folder, or the user's data
  directory.

## The helper is security-sensitive

`internal/helper` runs as root, so keep its surface small. It has four
operations (`start`, `stop`, `version`, `update`), checks the peer uid, and
runs `core` only on the user's own `<data>/core/runtime.yaml`. `update`
replaces the helper only with a build signed by the update key
(`internal/updatesig`) and newer than itself. Don't add operations that
run arbitrary commands or paths. Extend `helper_test.go`, which covers the
refusals, for anything new.

The private update key is `~/.config/clashcube/update.key` (or
`$CLASHCUBE_UPDATE_KEY`). `wails3 task app` signs with it when it exists;
builds without it fall back to reinstalling the helper with a password.
Never commit the key or print it.

## Frontend conventions

- Take colours, radii, shadows and easing from `src/styles/tokens.css`. Don't
  add new ones.
- Reuse the animated components: `Segmented`, `Popover`, `Fold` (children
  get `.stagger` and `--i`), `Switch` and `toast`.
- Interaction follows Surge for Mac. The tray menu
  (`internal/gui/traymenu.go`) is rebuilt from state, never mutated.
- Every user-facing string goes through `t()`, with its Chinese in
  `src/i18n.ts`. The key is the English text.
- State comes from Go events in `src/store.ts`. Poll with `usePoll` only for
  data that has no event (connections, rules).

## Verifying UI changes

Screenshot the window, not the whole screen, because other windows with the
same name may be open. Find the window ID from the dev instance's PID with
`CGWindowListCopyWindowInfo`, then run `screencapture -l <id>`. To fill the
Connections and Overview pages, send traffic through the dev instance's mixed
port with `curl -x http://127.0.0.1:<port> --limit-rate …`.

## Commits

Commit only when asked. Write commit messages in English: a short imperative
subject, then a body that explains why.
