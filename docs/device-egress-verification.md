# Device fixed egress: Docker verification

Verified on 2026-10-28 using the local `0.3.0-dev.1` IPKs. These artifacts have
not been published and the official update feed has not changed.

## Environment and evidence

Lean QWRT ARM64 runs under procd in the existing OrbStack Docker lab, using fw3
plus an independent nftables device guard. The router has separate LAN/WAN
networks. A LAN peer connects to a deterministic WAN origin through either of
two HTTP proxies. The origin distinguishes node A, node B and direct routing by
source address; DNS fixtures also count proxied and direct queries. Successful
HTTP/DNS alone is not treated as proof of the selected exit.

`python3 dev/openwrt/device-egress-smoke.py --packages` passed **79 checks**.
The generated scenario list is `bin/openwrt/device-egress-report.json`.

| Boundary | Observed result |
| --- | --- |
| Fixed A/B nodes and rule/global/direct modes | Protected TCP uses the selected node. Unprotected control clients retain ordinary routing. |
| Ordinary LAN interception and bypass list | Device TCP and DNS retain their dedicated path, including when the device is excluded from ordinary interception. |
| Router/public UDP and TCP DNS | Queries use the selected proxy tunnel; resolver counters show no direct fallback. |
| UDP, IPv6 and shared/private proxy bypass attempts | Unsupported Internet traffic and bypass attempts are blocked. Router administration remains reachable. |
| Existing direct/proxied streams | Protection activation and pause terminate old streams. |
| Stop, restart, core/supervisor SIGKILL, management restart | Repeated routed probes observe the selected exit or failure; no direct result. Supervisor respawns exactly one child. |
| fw3 reload and full QWRT container restart | Guard remains in force, including boot with management disabled. |
| Changed MAC/IP, profile or same-name node endpoint | No automatic reassignment or direct fallback. |
| Modified firewall, incomplete cache, interrupted settings write | Drift becomes unverified; emergency or recovered Pending state blocks until reviewed. |
| Disabled boot guard, enabled flow offload, stale writer | Activation/write is refused. |
| Real guard/web/core IPK replacement | Persistent protection survives each package replacement. |
| Core/web removal and reinstall | Traffic stays blocked while absent and returns through the selected node after reinstall. Guard removal is refused while policies exist. |
| Explicit release | Ordinary TCP and UDP routing return. |

`node dev/openwrt/device-egress-ui.mjs --fresh` passed against the real LuCI
iframe and backend. It verifies the first policy without an existing policy
file, manual entry, DHCP reservation, node selection/change, pause/resume,
release confirmation/cancel, stale status without a green badge, and recovery.
Desktop, dark and 390px layouts have no horizontal overflow, JavaScript errors
or extra windows. Screenshot artifacts are `bin/openwrt/device-egress-*.png`.
The mobile add button and centered dialogs were also reviewed visually.

`python3 dev/openwrt/package-smoke.py` passed complete four-package removal and
reinstall with no protected devices: processes/assets are removed, shared DNS
and firewall state recover, data and the selected profile survive.

`wails3 task test` passed the full internal Go suite, production frontend build
and all 45 frontend tests. Race checks passed for devicepolicy, router,
coreservice and webui; `go vet -tags with_gvisor ./...` also passed. Both ARM64
binaries passed UPX integrity tests. Existing Vite chunk-size and macOS linker
warnings remain non-failing.

## Storage and delivery

Payload sizes below exclude dependencies, filesystem overhead, profiles and GEO
databases. UPX LZMA remains enabled by default.

| Package | Download bytes | Installed payload bytes |
| --- | ---: | ---: |
| clashcube-guard | 2,249 | 2,039 |
| clashcube-core | 14,414,789 | 14,406,895 |
| clashcube-web, including panel assets and enhanced YAML source editor | 3,958,004 | 5,136,635 |
| luci-app-clashcube | 3,253 | 5,732 |

The source editor was subsequently enhanced with syntax highlighting, live
YAML checks, line numbers and inline diff. Its isolated package-size increment
is recorded in [editor-size.md](editor-size.md).

After tests, the lab was restored to the default profile and rule mode, with the
core running, ordinary LAN interception disabled and no protected policies or
test DHCP reservations. Temporary test profiles were removed. The fixtures and
test scripts remain available for reproducing verification.

## Scope still requiring hardware validation

This version pins concrete remote nodes for IPv4 TCP and public DNS, with at
most 32 device identities. Unsupported WAN UDP/ICMP and IPv6 are blocked.
Groups, UDP/IPv6 proxy forwarding and public exit-IP verification are outside
this version. Creating/changing a binding requires a running core.

Docker tests exercise the OrbStack kernel, not physical router drivers or flash
boot. Target firmware still needs validation of nft modules, startup ordering,
wireless/bridge MAC visibility, fw4 reload behavior and hardware offloading.
Identity protection covers traffic matching the saved MAC or IPv4 on this
router; a different MAC *and* IP, another gateway or a LAN relay requires network
isolation beyond this feature. Router control-plane resolution of a proxy
hostname is separate from protected device DNS.
