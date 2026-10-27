# OpenWrt development prototype

The first router target is the pinned Lean QWRT ARM64 Docker lab in
[`dev/openwrt`](../dev/openwrt/README.md). It runs the existing backend and
mihomo core without Wails. No firmware build is needed. This is a development
deployment with IPv4 TCP and DNS interception. Three independently installable
ARM64 IPKs target this QWRT/fw3 environment; fw4 and other architectures still
need implementations and validation.

## Process and transport

```text
browser → uhttpd / LuCI admin session → Lua controller
        → /var/run/clashcube/web.sock → clashcube-web backend
        → random loopback REST port + secret → mihomo

procd → clashcube-core supervisor → mihomo child + LAN firewall guard
web backend → /var/run/clashcube-core/control.sock → supervisor lifecycle RPC
```

procd runs two independent services. `clashcube` owns the replaceable web
backend. `clashcube-core` owns a separate supervisor binary, its mihomo child
and the firewall guard. The child's lifetime pipe belongs to the supervisor,
so exiting or upgrading management leaves proxy connections and listeners
running. Stopping the core service ends both supervisor and child and clears
interception and restores dnsmasq. On routers, configuration lives in
`/etc/clashcube`, because OpenWrt's `/var` is normally volatile. The lab links
that directory to its existing `/var/lib/clashcube` Docker data volume.

The supervisor exposes an additional private socket in its own mode-0700
directory, with a mode-0600 socket. Its allowlist covers status, start, stop,
configuration testing and guard operations. It accepts no executable paths,
config paths or home directories. Controller credentials must match the fixed
runtime file and use authenticated IPv4 loopback. They are returned only over
the private socket, never through LuCI. Desired core state and credentials are
saved atomically in mode-0600 `core-service.json` for supervisor restarts.

Management startup attaches to an existing core without regenerating runtime
configuration, changing selections or restarting the process. On first setup
it honors AutoStart; once initialized it preserves an explicitly stopped core
across management restarts. Router boot honors the separate AutoStart preference
through a once-consumed init marker; service upgrades and subsequent procd
respawns preserve the current running/stopped intent. Shutdown detaches observers and flushes statistics
without stopping the core. Reconnection restores API/event streams after a
core service restart; the displayed core version follows the installed service.
Management-only statistics/events are not collected while management is offline.

The daemon exposes HTTP only over a Unix socket, mode 0600, in a private
directory. The LuCI controller under `admin/services/clashcube` inherits root
authentication. It injects LuCI's per-session CSRF token into the app page
and requires it for every JSON POST, including reads and event polls. The
legacy uhttpd CGI interface drops custom headers, so the bridge uses LuCI's
token query parameter. The index is served from `/usr/share/clashcube`, not
the public static directory. Bundled JS/CSS/images are public static assets.

`internal/webui` registers an explicit RPC allowlist. JSON decoding rejects
unknown fields, incorrect argument counts and trailing values. Browser file
imports carry YAML text, never server file paths. Desktop helper operations,
system proxy control, native file dialogs and TUN toggles are absent. The
core's private controller credentials are redacted from configuration previews.
This is an admin interface: imported configurations and subscriptions remain
trusted inputs capable of configuring mihomo.

The menu entry renders the normal LuCI header and footer, with the React app
in a same-origin iframe inside its content area. Internal navigation stays in
that frame; entering ClashCube does not open another window. A separate
authenticated `/app` route serves the frame and supplies the absolute API path.
When the session expires, the outer page reloads to LuCI's login form.

The frontend builds with `--mode web`. Vite resolves `@platform` to either
the desktop Wails bindings or the browser RPC transport. Event subscriptions
use a bounded history with independent cursors and long polling. A stream ID
detects daemon restarts; stale cursors recover from state/profile snapshots.
No Wails runtime is needed in the browser bundle.

## Shared behavior and platform limits

Group/provider presentation and partial settings validation live in
`internal/backend`, used by both surfaces. Existing runtime generation,
`core -t`, reload and rejection rollback remain authoritative. The starter
profile is embedded by `internal/defaults` for both entry points.

Linux adds TCP listener inspection through `/proc/self/fd`, process-group
setup, route discovery through `ip -j`, interface binding and ping probes.
The desktop helper, desktop updater and system proxy have Linux stubs. The
router entry point also clears those desktop settings and TUN/network-rule
automation at startup. Desktop app updates must eventually be replaced by
router package upgrades; they are disabled here.

The prototype starts with mixed port 7897 and Allow LAN enabled. Explicit
proxy access works without enabling interception. Top-level mixed, HTTP and
SOCKS ports listen on loopback plus explicit LAN listeners, preserving global
authentication, source-IP restrictions and the default inbound names. They do
not bind the WAN address. Custom profile listeners remain administrator-managed.
An existing runtime retained across a management-only upgrade needs one explicit
reload to adopt these bindings. The image uses
fw3/iptables-legacy; a fw4/nftables implementation needs separate testing.
Container kernel limitations also mean final TUN/TPROXY behavior must be
tested in a VM or on router hardware.

## LAN interception

Settings → Network → LAN proxy enables the router manager. Its desired state
is stored in `/etc/clashcube/router.json`, independently of desktop
settings. Enabling requires a running core. The final `router` runtime layer
adds a redir listener at the LAN IPv4 address on port 7898 and DNS on 1053.
Those ports are reserved while enabled. Existing policies and upstream DNS
are retained; an empty upstream list defaults to DoH. Numeric bootstrap
resolvers are supplied when missing. Upstreams that recurse into the router's
own DNS, including `system` and `dhcp://lan`, are rejected before activation.
DNS uses `redir-host`
and suppresses AAAA answers because non-DNS UDP and IPv6 are not intercepted.
The runtime preview includes this layer and still redacts controller secrets.

After `core -t` and reload, activation checks that the core owns both TCP
listeners. Only then does it attach `CLASHCUBE_LAN` to NAT PREROUTING, scoped
to the discovered LAN device and IPv4 subnet. UDP/TCP port 53 goes to
dnsmasq on port 53, followed by public IPv4 TCP redirection. Local, private, multicast
and special destinations bypass TCP interception. Router-originated traffic
is outside the hook, avoiding recursion through the core's own outbound
connections. Existing forwarding, WAN NAT and other services' chains remain.

dnsmasq keeps answering local hosts, static names and DHCP hostnames. Its
public queries go to the core's LAN address on port 1053. The guard saves the
original first dnsmasq instance's `server` and `noresolv` options in a private,
atomic `dns-restore.json` before changing them through ubus/UCI and reloading
dnsmasq. Runtime changes are not committed to flash. Stop, crash cleanup,
rollback and next-start reconciliation restore the original upstreams; the
recovery file is kept until reload succeeds. Restoration preserves upstream
options edited by an administrator while interception was active. Multiple
dnsmasq instances are not managed in this version.
DNS restoration is attempted even when firewall cleanup fails; the guard
continues retrying the failed cleanup independently.

Disabling removes the hook before removing listeners. A rejected activation
restores the previous desired state and configuration. Core stop/restart
removes rules first; a crash notification and a one-second reconciliation
loop clear stale rules after an unexpected exit. Ordinary forwarding resumes
when interception is removed. Restarting the core or procd restores enabled
intent. The core service's procd stop callback also clears the owned chain
after termination. Management stop has no firewall cleanup callback.
The fw3 script include invokes the same reconciliation after firewall reload
or restart directly through the core supervisor, including while management
is offline; the periodic check repairs missing or changed owned rules.

Configuration transactions pause interception through the supervisor, remove
hooks before listener changes and resume after validation or rollback. A pause
expires after one minute if management dies during a transaction. Reconnecting
management also releases an abandoned pause. The guard is the only process
that writes firewall rules, avoiding competing owners during redeployment.

Settings → Network → Device access selects all devices, only listed devices,
or bypass listed devices. Entries accept IPv4 hosts/subnets and unicast MAC
addresses; either an IP match or a MAC match includes a device in the list.
An empty include list intercepts no devices. Lists are validated and saved with
the interception preference in `router.json`. Editing them reconciles firewall
rules without reloading the runtime or restarting the core, and survives
management downtime and firewall reloads.

The policy controls transparent TCP interception and forced DNS redirection.
It does not restrict explicit proxy access. Clients choosing the router as
their DNS server still share its dnsmasq upstream, including bypassed clients.
Use proxy authentication and profile source-IP restrictions for explicit access.

## Independent deployment

`sh dev/openwrt/deploy.sh` updates management by default. It builds and
atomically replaces only `clashcube-web`, copies LuCI/browser assets and
restarts only `clashcube`. It preserves the installed core binary, running
core process and runtime file. Hashed assets from the current and previous
deployment remain usable by open tabs; older assets are removed. The first
deployment after adopting an untracked installation preserves its old assets
until the next deployment. uhttpd does not need restarting for this update.

`sh dev/openwrt/deploy.sh core` explicitly replaces/restarts the core service
while retaining management. Existing proxy connections end during a core
upgrade. `sh dev/openwrt/deploy.sh all` updates both. The first deployment
automatically installs both binaries; migrating the old combined deployment
requires one core restart because that old process owned the lifetime pipe.
These commands install local builds in the isolated lab. For opkg packages,
build on the host without a firmware SDK:

```sh
python3 openwrt/package.py --version 0.2.0-3
# Outputs: bin/openwrt/packages/*.ipk and packages.json (sizes + SHA-256).
```

Install the generated packages together on the target router, or core → web →
LuCI in that order. `clashcube-core` contains mihomo, its supervisor, fw3 and
storage integration; `clashcube-web` contains management and browser assets;
`luci-app-clashcube` supplies the embedded menu entry. Management package
upgrades stop only management and retain the previous frontend assets for open
tabs. Core package upgrades restart only the core. Core removal restores DNS
and firewall state; removing packages preserves user configuration.

Dependencies are resolved by the router's existing opkg feeds. Packages target
`aarch64_generic` and legacy Lua LuCI/fw3, as verified in the pinned QWRT lab;
they are not universal OpenWrt binaries. Use an increasing `--version` for each
release. The default git-derived version is convenient for local builds, but
uncommitted rebuilds have the same version. The browser supports signed core
updates as described below; management and LuCI packages still use opkg.

## Core updates, GEO maintenance and diagnostics

Settings → Network → Router maintenance configures a signed core feed and an
optional GEO interval. New installations select the official signed feed;
scheduled GEO updates default to disabled. Preferences live in the persistent
`router-maintenance.json`, included in sysupgrade backups. Device access and
scheduled GEO updates require the P1 core; management detects an older core
and asks for its package upgrade before enabling these features.

To produce a feed from the host-built IPKs:

```sh
python3 openwrt/package.py --version 0.2.0-4
go run ./scripts/signrouter -base https://your-server.example/releases/0.2.0-4 \
  -out bin/openwrt/packages/router-update.json bin/openwrt/packages/packages.json
```

The signer reads the existing release key from `CLASHCUBE_UPDATE_KEY` or
`~/.config/clashcube/update.key` and verifies that it matches the built-in
public key. Publish only the generated public feed and IPKs, then enter the
feed URL in the browser. The official channel is
`https://raw.githubusercontent.com/clash-cube/clash-cube/main/openwrt/updates/aarch64_generic.json`.
It is a signed pointer to immutable, versioned GitHub release assets. An
explicitly empty or custom feed is preserved across upgrades. Signatures use
Ed25519 with a router-specific domain, distinct from
desktop/helper updates. This updater accepts only the matching
`aarch64_generic` core IPK and a version newer than the installed package.

Checking verifies the signed manifest. Applying starts a separate, fixed
`clashcube-update` procd worker: downloading, hashing and size verification
finish before opkg can stop the old core. Invalid manifests or corrupt packages
leave the running binary untouched. Management may stop or restart while the
worker downloads and installs. Successful core replacement closes existing
proxy connections; an explicitly stopped core stays stopped. There is no
automatic opkg rollback after an installation failure. Installation details
stay in private mode-0600 `/tmp/clashcube-cache/updates/opkg.log`; the browser
receives a controlled error. Job state and staging files use RAM, and abandoned
jobs are recoverable after reboot. Binary upgrades require an explicit click.

Scheduled GEO updates run in the core supervisor, independently of management.
The supervisor checks the schedule every 30 seconds and invokes mihomo's
existing GEO updater for databases required by the active profile. Its URLs
and selected storage directory remain authoritative. The last attempt and
result persist in `geo-schedule.json`; failed attempts wait a full configured
interval before retrying, including after supervisor restarts. The existing
manual GEO update action remains available. Updating data does not restart
the core process.

Router diagnostics shows installed package versions, system/kernel, available
memory, config/core/cache storage capacity and service checks. Its downloadable
JSON is a curated projection, excluding raw configuration, controller secrets,
subscription links and logs. In Docker, kernel and memory describe the host
Linux environment rather than a physical router.

### Publishing a router release

`sh scripts/release-openwrt.sh 0.2.0-4` builds the three IPKs with UPX LZMA,
embeds the package version in management, signs `router-update.json` and emits
`SHA256SUMS` under `bin/openwrt/release/openwrt-v0.2.0-4`. Run it from the
verified source commit with the existing release key available. Router tags
use `openwrt-v…` separately from the desktop's `v…` release workflow.

Push the source and tag, upload those artifacts to that tag's GitHub release,
and publish it with `--latest=false` so the desktop download remains current.
After the versioned assets are publicly accessible and verified, copy the
signed `router-update.json` to `openwrt/updates/aarch64_generic.json`, commit it,
and push main to advance the channel. Do not advance the channel before the
referenced assets are available or replace published IPKs in place. Subsequent
releases repeat these steps with a strictly higher opkg version. The feed is
public verification material; the private key never belongs in the repository.

## Persistent configuration and storage modes

`/etc/config/clashcube` selects storage for both procd services. Configuration,
profiles, the validated `core/runtime.yaml`, restart intent and managed
Tailscale identities remain in persistent `data_dir` (default `/etc/clashcube`).
Logs, traffic history and the optional city database use `/tmp/clashcube-cache`
to avoid recurring flash writes. Runtime history is lost on reboot.

| `storage_mode` | Core working directory (GEO, providers and caches) |
| --- | --- |
| `persistent` (default) | `<data_dir>/core` |
| `ram` | `/tmp/clashcube-data/core` |
| `external` | `<external_mount>/clashcube/core` |

GEO files are downloaded into the selected core working directory. RAM mode
redownloads required data after reboot and needs working upstream networking;
automatic restoration retries failed startup every five seconds. An explicit
Stop cancels pending restoration. The runtime file stays on flash, with only
that directory added to mihomo's safe reload paths. Managed Tailscale state is
linked back to persistent storage in RAM/external modes.

Stop both services before switching directories:

```sh
/etc/init.d/clashcube stop
/etc/init.d/clashcube-core stop
uci set clashcube.main.storage_mode=ram
# For external storage instead:
# uci set clashcube.main.storage_mode=external
# uci set clashcube.main.external_mount=/mnt/usb
uci commit clashcube
/etc/init.d/clashcube-core start
/etc/init.d/clashcube start
```

External storage must already be mounted at the exact configured path under
`/mnt/` or `/media/`. An absent disk fails startup without creating a fallback
directory on internal flash. Mount ordering remains the router's fstab/procd
responsibility. Existing disposable data is not migrated when switching modes;
returning to a previous mode reuses its files. Configure these modes through
UCI; the browser does not yet expose a storage selector.

Sysupgrade's keep list includes the UCI file and persistent configuration and
identity paths, including a custom `data_dir`. Downloadable GEO/provider caches
are excluded. The standard sysupgrade backup/restore policy still applies.

## Storage and optional UPX

Both ARM64 binaries already use `CGO_ENABLED=0`, `-trimpath` and `-s -w`.
Measurements on 2026-10-27 with Go 1.27.2 and UPX 5.2.1, retaining the
current protocols and `with_gvisor` in the core:

| Installed component | Stripped | UPX NRV (`--no-lzma -9`) | UPX LZMA (`--lzma -9`) |
| --- | ---: | ---: | ---: |
| Core service + mihomo, one file | 57.69 MiB | 20.21 MiB | 13.71 MiB |
| Management backend | 11.00 MiB | 4.14 MiB | 3.01 MiB |
| Both binaries | 68.69 MiB | 24.35 MiB | 16.72 MiB |

The frontend adds 1.33 MiB per build (shared hashes are stored once). Before
bounded retention, accumulated old bundles occupied 3.34 MiB in the lab;
redeployment reduced this to 1.34 MiB allocated. Runtime data is separate:
the lab's GeoIP database adds about 8.04 MiB and profiles/caches grow with use.
The supervisor and mihomo child execute the same core file, so storage does
not contain two copies. During replacement, `.new` temporarily requires room
for the incoming binary alongside the installed one.

These are installed file sizes, not download sizes or measured firmware flash
usage. Docker's writable filesystem does not model router flash compression.
The generated `packages.json` reports download and installed payload sizes;
the filesystem's allocated space and upgrade scratch space remain separate.

The P1 `0.2.0-3` packages with default LZMA total 18.10 MiB of installed
payload: 13.73 MiB core, 4.36 MiB management including frontend, and about
5 KiB LuCI integration. GEO data is additional. Core and management IPK
downloads are 13.74 MiB and 3.57 MiB respectively.

Deployment defaults to UPX LZMA to minimize storage. Compression runs on the
build host before any service is stopped:

```sh
sh dev/openwrt/deploy.sh all                  # default: UPX LZMA
CLASHCUBE_UPX=nrv sh dev/openwrt/deploy.sh all # faster decompression
CLASHCUBE_UPX=none sh dev/openwrt/deploy.sh all # rebuilds unpacked
```

The same setting works with `management` or `core`, changing only that
component. Compressed builds require UPX and check each output with `upx -t`;
no unpacked backup is installed on the router. Missing UPX stops deployment
with instructions to install it or explicitly select `CLASHCUBE_UPX=none`.

Do not choose UPX from file size alone. A separate minimal idle core in the
ARM64 lab used 38.2 MiB PSS unpacked versus 59.6 MiB with either compression
method, sampled two seconds after controller readiness. Warm `core -t`
medians over five runs, including Docker exec overhead, were 0.081 s unpacked,
0.226 s NRV and 0.634 s LZMA. These timings are from Apple Silicon, not a
router CPU. Linux packed executables also lose ordinary file-backed sharing
between processes, relevant to our supervisor/child design; see the
[UPX Linux notes](https://github.com/upx/upx/blob/v5.2.1/doc/upx.pod#notes-for-linux).
LZMA is the default storage tradeoff. NRV offers faster startup, and `none`
remains available when RAM or startup time takes priority.

This currently manages one IPv4 `network.interface.lan`. A LAN address change
clears interception until the core is reloaded/restarted with the new address.
Additional LAN zones, non-DNS UDP, IPv6, fw4, router-originated interception
and TUN are outside the current implementation. Do not enable another proxy
service's transparent interception alongside it.

## Verification

`python3 dev/openwrt/smoke.py` tests the real LuCI login and CSRF boundary,
uploads, subscription downloads/updates, settings, groups/modes, module
rejection rollback, redacted previews, DIRECT and REJECT proxy behavior,
and core start/stop/restart. It uses the isolated client, keeps credentials
out of output and restores its test changes.

`python3 dev/openwrt/router-smoke.py` exercises the client without explicit
proxy settings: DNS, HTTPS, DIRECT versus REJECT, one LAN-only hook, preserved
WAN NAT, firewall reload/restart, core stop/crash cleanup, management restart
without core replacement, offline-management firewall reload, disable and
rollback when another process occupies the redir port. It
restores the original profile, mode and interception preference.

`python3 dev/openwrt/upgrade-smoke.py` streams 16 MiB through transparent
interception from a local fixture. Management stop, SIGKILL/procd respawn and
an actual management deployment must preserve the original core PID, binary
hash, runtime hash and connection ID. It verifies the complete download hash,
tests DNS/firewall reload while management is off, then restores configuration
and removes the fixture. It also exercises both services across independent
core deployment and verifies management reattachment without a management restart,
preservation of an explicit stop and router boot with AutoStart on/off.
Use `--packages` to exercise actual opkg management/core upgrades with the same
connection-preservation checks instead of development file deployment.

`python3 dev/openwrt/p0-smoke.py` verifies local hosts, a real DHCP handshake
and hostname, forced UDP/TCP DNS, recursive-upstream rejection and restoration
of original dnsmasq options after core stop. It also tests RAM-cache loss,
safe reloads and missing external disks, restoring the lab's original settings.
It also verifies that explicitly stopping a failed boot restoration prevents
its retry loop from starting the core after the configuration is repaired.
`python3 dev/openwrt/storage-test.py` uses a temporary mounted Docker volume to
verify external storage, identity separation, sysupgrade backup selection and
data surviving container replacement. It removes its test volume afterwards.
`python3 dev/openwrt/package-smoke.py` removes all three installed packages with
interception active, checks DNS/firewall cleanup and persistent data, then
reinstalls the packages listed in `bin/openwrt/packages/packages.json` and
verifies the selected profile is retained.

Go tests cover RPC decoding and allowlisting, private socket ownership,
independent event consumers, restart/overflow recovery and Linux TCP listener
ownership. Existing Go and frontend suites still cover the shared behavior.
Browser verification should exercise the main pages and settings, check for
JS/RPC errors, and confirm native desktop controls are absent.
`python3 dev/openwrt/assets-test.py` checks bounded asset retention, lazy-chunk
availability across updates and migration from an untracked installation.

`python3 dev/openwrt/p1-smoke.py` exercises real IP/subnet/MAC policies, empty
include lists, rejection rollback, offline firewall reconciliation, unchanged
core PID/runtime, LAN-only explicit proxy bindings and diagnostic export.
`python3 dev/openwrt/core-update-smoke.py` builds and signs local IPK fixtures
using the existing release key. It tests rejected signatures and corrupt
downloads, a successful real opkg update with management offline, version
comparison and preservation of an explicit stop.
`python3 dev/openwrt/geo-schedule-smoke.py` serves a valid local GEO database
and verifies its scheduled download with management offline and an unchanged
core PID. These tests restore their preferences and remove network fixtures.
`node dev/openwrt/ui-smoke.mjs` verifies embedded browser interactions and the
diagnostic download; `CLASHCUBE_PLAYWRIGHT_MODULE` and
`CLASHCUBE_BROWSER_PATH` can select an existing Playwright installation/browser.

Next work includes hardware validation, additional architectures and fw4
support. Do not implement router interception by enabling the desktop
privileged helper.
