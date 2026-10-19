# Android port

## Phone UI, round 3

The phone shell has four tabs: Home, Proxies, Profiles and Tools. Its floating
dock stays visible on sub-pages and module editors, and hides for sheets and
the keyboard. Connections opens under Home or Tools and returns to that
tab on Back. Each tab retains its own navigation and scroll position.

Home uses status, directional speed, mode, proxy-group, usage, connectivity
and AI cards. The status address comes from AppService.NodeEgress: nested
selectors resolve to their current leaf and reuse the AI checks' three-minute
node trace cache and ten-minute IP attribute cache. It adds no lookup provider.
Unknown attributes stay absent; ISP metadata is not evidence of a residential
IP and retains the ISP label. Shared components accept optional headings/icons;
desktop callers retain their existing presentation.

The Android UI process embeds Wails v3 beta.27 and the existing backend in
`libwails.so`. There is exactly one Go window, named `main`. React uses the
ordinary generated services/events; `AppService.Platform()` reports `android`.
The desktop layout is intentionally unchanged.

`build/android/` is maintained source copied from Wails' Android build assets.
Do not regenerate the entire build-assets tree. The Java namespace stays
`com.wails.app` because Wails' JNI exports encode it. The application ID is
configured only in `build/android/app/build.gradle`; the install script reads
it from the resulting APK. Launcher icons are resized from `build/appicon.png`.

## Build and install

Requirements on this macOS development host: Go, Wails CLI, pnpm, JDK 21 on
PATH, and the SDK at `/opt/homebrew/share/android-commandlinetools` (override
with `ANDROID_HOME`). Install platform 35, build-tools 35.0.0 and NDK
29.0.14206865 beforehand. Gradle uses its normal `~/.gradle` home.

```sh
wails3 task android:apk
wails3 task android:install
# Select another attached device:
ANDROID_SERIAL=<serial> wails3 task android:install
bash scripts/check-android.sh
```

The debug APK is `build/android/app/build/outputs/apk/debug/app-debug.apk`.
It contains arm64-v8a and x86_64, targets/compiles API 35 and requires API 29.
Both libraries are built for both ABIs. `libwails.so` uses `with_gvisor`;
`libclashcore.so` uses `with_gvisor,cmfa,clashcube`. The build script clears the desktop
Taskfile's macOS CGO flags before selecting NDK clang. The debug APK uses the
standard Android debug signing key; no release distribution is configured.

`android:install` installs/replaces only this APK and launches its Activity
on `emulator-5554` by default. It disables adb mDNS/libusb discovery because
platform-tools 37 crashed when starting its server on this macOS 27 host;
the emulator's direct transport works with those backends disabled.

## Storage and platform boundaries

Before Wails initialization, `ClashCubePaths` passes
`Context.getFilesDir().getAbsolutePath()` to a small JNI export in
`paths_android.go`. It sets Go's `CLASHCUBE_HOME`; setting only libc's
environment after loading Go would miss Go's cached environment.
An uninitialized Android data root fails closed instead of using a desktop
or relative path. No Android data path is hardcoded.

`coremgr.AndroidRunner` controls the native services through JNI and Binder.
No mihomo core is loaded in the UI process. Backend Android adapters exclude desktop helper,
system proxy, autostart, hotkey, Windows utility and desktop updater imports.
System proxy requests fail without saving a proxy-only mode.

Desktop application startup, runner, helper operations and update installation
remain in desktop-selected files. Network binding/SSID, native desktop
dialogs/actions, hotkeys, notifications and updating are unavailable on
Android. The phone settings use a platform capability set to omit desktop
controls; notification delivery and its preference control are unavailable.

## Wails template adaptations

- The main window is the first and only window: Android routes all requests
  to the first Go window and the Activity loads `/` itself.
- Gradle 8.9 replaces the template's Gradle 9.2.1 wrapper to match AGP 8.7.3.
  Build-tools 35.0.0 is explicit so AGP does not try installing its default 34.
- The Android asset bridge returns bytes without preserving HTTP status.
  Wails intentionally returns 404 for the optional server-only
  `/wails/custom.js`; the template turned it into a 500. The Activity supplies
  a no-op JavaScript response, uses JavaScript MIME for runtime scripts and
  handles the absent browser favicon locally. No Wails module-cache edits
  or Go runtime overlays are required.
- A WebChromeClient records JavaScript console errors in logcat.
- `onRenderProcessGone` logs renderer death, removes and destroys the affected
  WebView, then builds a fresh one and reloads the phone UI. It keeps the Go
  bridge and the independent core service alive, including during background
  recovery; transient page navigation returns to Home.

## Verification

On MuMuPlayer `emulator-5554` (Android 12/API 32, arm64), the existing main
overview renders, the default profile is loaded from the Go backend, and
starting the core produces the expected stub error without hanging.
Back/background and reopening the launcher task retain a working UI.
Screenshot: `/tmp/ccandroid/spikeA.png`. PID-filtered logcat and WebView
DevTools are used to distinguish app failures from emulator graphics warnings.

Desktop validation: `wails3 task build:dev`, `go vet ./...`,
`go test ./internal/...`, `cd frontend && npx tsc`, and frontend tests.
Android validation: both c-shared libraries for both ABIs and
`scripts/check-android.sh`, which vets every selected application production
package and rejects desktop/core dependencies. Desktop tests are not
cross-compiled as Android tests because several re-exec a native core.

## VPN runtime (step B)

```text
UI process: React → Wails → backend/coremgr → own JNI → UIBridge
                              │                         │ private Binder
                              │ authenticated REST      ├─ :check / CheckService
                              ▼                         │   libclashcore → core.Validate → exit
                         :core / CoreService ◀──────────┘
                         VpnService → libclashcore → internal/core → mihomo
                         TUN fd       protect / socket owner callbacks
```

`internal/core.Initialize`, `Validate`, `Start` and `Shutdown` are shared by
the desktop CLI and Android library. Validation runs in a disposable bound
`:check` process; it cannot mutate a live core's package globals. The core
service is started independently of binding, has a `specialUse/vpn` foreground
notification, and exits its process after cleanup. UI bindings deliberately
omit `BIND_AUTO_CREATE` so service death cannot silently create an idle core.
VPN consent is requested on the Activity thread; refusal returns a start error.

The JNI namespace is `com.clashcube.android`. `NativeBridge.nativeInit`
registers a process-owned adapter; Go calls its exception-contained `call`
method. UI operations are start/check/stop/attach/status. Binder death invokes
`nativeStopped`; a liveness check is also retained as a fallback. `CoreNative.run`
accepts start/check/stop/DNS operations; core callbacks protect sockets and
resolve connection owners. Callback objects outlive the TUN and hooks are
installed before startup, never cleared during a dial. Protection and package
callbacks never acquire the native command or TUN-startup locks.

The service owns `Builder.establish().detachFd()`. IPv4 is `172.19.0.1/30`,
DNS `172.19.0.2`, MTU 9000, nonblocking/unmetered. Positive routes use CMFA's
`arrays.xml` exclusions plus the virtual DNS host route. IPv6 adds
`fdfe:dcba:9876::1/126`, virtual DNS and routes excluding ULA/link-local/multicast
only when enabled in settings. Socket protection prevents core egress from
re-entering the VPN; refused protection does not turn into repeated dial errors.
Non-VPN network callbacks feed system DNS and flush resolver state, preferring
validated Wi-Fi/Ethernet over cellular. Physical network handover remains a
device-testing boundary.

Android runtime generation forces TUN on, app-owned routing off, DNS hijacking
on, and process lookup to `always` because the resolver hook is installed.
Mixed/HTTP/SOCKS/redir/tproxy ports and profile listeners are disabled: Android
loopback is shared with other apps. Readiness uses the authenticated controller,
not a mixed port. The controller still has a random loopback port and secret;
external UI and secondary controllers are removed. `/clashcube/listening`
inspects this process's descriptors, since Android restricts `/proc/net`.

The fork's ClashCube-only TUN policy preserves the FD and initial stack/interface
parameters across both PUT and PATCH config requests. Changing stack or IPv6
therefore requires a full restart. NTP system-time writes are disabled even on
REST reload. Config/rule/GEO mutations remain available; executable restart and
core upgrade remain disabled. Ordinary CMFA embed-mode behavior is unchanged.
The shared CMFA system-DNS slice is synchronized against concurrent queries.

Two other `cmfa` effects are retained: `constant/path.go` bypasses mihomo's home
path allowlist, and `component/loopback` disables its loop detector. Android's
UID sandbox is the remaining filesystem boundary; configs are trusted input
within that sandbox. The service only accepts its private `core/runtime.yaml`.
The protect hook and disabled proxy listeners are important with no loop detector.

UI shutdown does not stop the VPN. On relaunch, private Binder returns the live
controller address/secret; coremgr authenticates `/version`, adopts them and
resumes existing REST streams. It does not rewrite runtime.yaml on attach.
The credentials also remain in the private 0600 runtime config, not preferences
or logs. A private stop marker distinguishes notification/revoke stops from
process crashes. Full restart waits for service death before starting again.

Package names use connection-owner UID plus a per-service UID cache. No
`QUERY_ALL_PACKAGES` is requested; HTTPS intent visibility covers browsers.
Names for packages Android hides may be empty; shared UIDs use the first visible
package. Package installation changes require restarting to clear the cache.

On MuMu Android 12 arm64, consent and refusal, start, live REST traffic/proxies, browser
package attribution (`com.android.browser`, UID 10027), UI-process kill and
reattach, core-process crash reporting, full restart, UI stop and notification
Stop were exercised. PUT/PATCH `/configs` returned 204 and preserved the TUN FD;
unauthenticated `/version` returned 401; executable restart/upgrade returned 404. The running
core exposed only its controller TCP port, with mixed/HTTP/SOCKS ports all zero.
Evidence is under `/tmp/ccandroid/`: `connections-evidence.json` and
`spikeB-running.png`, `spikeB-reattached.png`, `spikeB-core-killed.png`,
`spikeB-stopped.png`, `spikeB-connections.png`, `spikeB-notification.png`,
`spikeB-notification-stopped.png`, `spikeB-consent-denied.png` and
`spikeB-example-final.png`. The latter shows example.com loaded through the VPN;
the connections capture also records successful browser downloads from Baidu.
`am kill` did not kill the bound UI on this emulator;
the test used `run-as` to kill only its PID and retained the core PID.

Usage sampling and subscription updates still belong to the UI backend; they
are not durable background jobs. Android 13 notification permission is requested
but this API-32 emulator cannot verify that flow or API-34 foreground enforcement.
APK updates, boot/always-on (explicitly opted out), and document/deep-link integration
remain future work. The VPN runtime and attach path have no step-B stubs.

## Phone UI, round 1

`frontend/src/mobile` owns the five-tab shell, Home, history navigation,
bottom-sheet presentation, touch context menus and native appearance bridge.

Round 2 adds `PageActions` for page-owned app-bar controls and search history,
`MobileConnections` for touch rows and inline details, and `EditorPage` for
module forms with an app-bar Save action. The other pages reuse their existing
data flows with Android-scoped layouts. Search, sheets and editors each consume
Back before the underlying route; Android may dismiss the IME first. Long-press
copy and menus share the touch adapter, including taps whose target is an SVG.
The Activity exposes clipboard reads only for the explicit Paste action.
Notifications and listener-module creation are unavailable in the phone UI.
Android always uses this shell. Each tab retains its navigation stack and page
scroll positions; returning to a non-Home tab restores that stack. Selecting
the current tab resets it. Back consumes a sheet before its page, then returns
through the tab root to Home; the next Back leaves the Activity without stopping
the service. Existing desktop views route through the store's `setView` adapter.

Settings reuse the shared sections behind `mobile/capabilities.ts`; the four
phone settings pages omit desktop operations. The update button opens releases
through the Activity browser bridge: Wails' desktop BrowserManager would try
`xdg-open` on Android.
Proxies, Connections, Profiles, Rules, Logs, Events, Usage and Globe retain their
existing content inside 16px page padding pending round 2.

All phone CSS is scoped to `[data-platform="android"]`. WebView 95 needs `vh`
and percentage-height fallbacks and explicit wrapping classes: it has neither
dynamic viewport units nor `:has()`. It also does not update its theme media
query reliably. The Activity's small `clashcubeAndroid` bridge supplies system
theme and monotonic service uptime, and applies the resolved CSS background to
both system bars with matching icon contrast. It fixes text zoom at 100%, uses
adjustResize and avoids drawing behind system bars. The UI receives uptime
through the private service start/attach reply, not a second core connection.

Round-1 emulator evidence: `/tmp/ccandroid/ui1-*.png`, covering stopped/running
Home, all tabs and settings, the hosted activity pages, and bottom sheets in
light/dark. Navigation regressions are covered in `mobileNavigation.test.mjs`.
Use real device taps (or CDP input events) when testing native Back: Chromium
may skip history entries created by scripts without a user gesture.
