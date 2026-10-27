# QWRT Docker development lab

This lab runs the existing [`coolsnowwolf/qwrt:arm64-latest`](https://hub.docker.com/r/coolsnowwolf/qwrt)
image published by Lean, pinned to digest
`sha256:b6197c0bb344959641101cd68e64d263d3c807d5c1ddad60b4eab53d1143e0e7`.
No firmware compilation or Docker build is required. This is the publisher's
QWRT image, not a build of a specific current `coolsnowwolf/lede` commit.

The image reports OpenWrt 24.10.3, revision `r6954-1270397e47`, target
`armvirt/64`, and uses fw3/iptables. Its binaries are AArch64, although its
Docker metadata incorrectly says amd64. The explicit platform in Compose
matches that metadata; the binaries run on the ARM64 Docker host. This lab
was verified with OrbStack on Apple Silicon and Docker Compose v5.5.0.
Compose must support `interface_name` and `gw_priority` (v2.36.0 or newer).

## Start and access

From the repository root:

```sh
sh dev/openwrt/up.sh
```

- LuCI: <http://127.0.0.1:18080/cgi-bin/luci/>, user `root`.
- Password: generated in `dev/openwrt/root-password`, which Git ignores.
  Read it locally with `cat dev/openwrt/root-password`.
- SSH: `ssh -p 10022 root@127.0.0.1`.
- Router shell: `docker compose -f dev/openwrt/compose.yaml exec router sh`.
- Client shell: `docker compose -f dev/openwrt/compose.yaml exec client sh`.

Only loopback ports are published. The lab does not change the Mac's system
proxy, routes, TUN or installed ClashCube helper. Containers have `NET_ADMIN`
within their own network namespaces; neither privileged mode nor host
networking is used.

## Test network

```text
client (172.30.77.3)
  → QWRT LAN eth1 (172.30.77.2), DNS + IPv4 forwarding + NAT
  → QWRT WAN eth0 (172.30.76.2)
  → Docker gateway (172.30.76.1)
  → OrbStack / existing host network
```

The client's default route and resolver point to QWRT, and its only Docker
network is the internal LAN. Its startup command replaces Docker's injected
resolver file so queries actually cross the router. With LAN proxy disabled,
QWRT forwards DNS to Docker's embedded resolver at
127.0.0.11. Upstream DNS and egress therefore reflect the existing host
network, including any proxy or fake-IP resolver already running there.
These initial checks establish the lab's connectivity, not proxy correctness.
If either subnet overlaps another local network, change the addresses in
both `compose.yaml` and `configure.sh`, plus the client/check commands.

The entrypoint starts the real `/sbin/init` → procd → ubus/netifd service
stack. On first boot it keeps the boot, system, log, rpcd, DNS, SSH, firewall, network,
web and ucitrack services. Hardware, time, storage and bundled proxy startup
links are disabled. Vendor first-boot customisations that replace networking
or feeds are excluded; base migrations and LuCI setup are retained.
The installed OpenClash pages remain available for comparison, with its
proxy service initially disabled.
Services installed and enabled afterwards are preserved across restarts.

The repository is mounted read-only at `/workspace/clash-cube`. Plugin data
is stored at `/etc/clashcube`, linked in the lab to `/var/lib/clashcube` and
backed by the named `data` volume. On actual routers `/etc/clashcube` is
persistent; `/var` is normally a RAM filesystem.

## Verify and manage

```sh
sh dev/openwrt/check.sh
docker compose -f dev/openwrt/compose.yaml logs
docker compose -f dev/openwrt/compose.yaml restart
docker compose -f dev/openwrt/compose.yaml stop
sh dev/openwrt/up.sh
```

The check verifies ubus, the LuCI login form, the client's route, DNS and
HTTPS through the router, and prints the WAN NAT counters. LuCI in this
image returns HTTP 403 with its unauthenticated login form; the healthcheck
checks that form rather than requiring HTTP 200.

Container changes survive stop/start and restart. `docker compose down`
removes the containers, so subsequent startup resets their system changes.
The password file and plugin data volume survive. `docker compose down -v`
also deletes plugin data. Init-script changes require a recreated router
to rerun first-boot configuration.

## Development scope

Use this environment for the browser UI, authenticated API, configuration
generation, subscriptions, package installation and procd service integration.
The isolated client also supports IPv4 forwarding and firewall experiments.
This image's initial network integration target is fw3/iptables; fw4/nftables
requires another image or separate compatibility testing.
The initial firewall covers IPv4 only and disables flow offloading. fw3 may
still warn that the FLOWOFFLOAD extension is unavailable, and log failures
to write TCP sysctls because Docker mounts `/proc/sys` read-only. The lab
uses ordinary NAT and the host kernel's TCP settings; the connectivity
checks pass without those features or writes.

Containers share OrbStack's Linux kernel, not the kernel in the QWRT image.
Do not install firmware kernel modules expecting them to match the running
kernel. Firmware boot, hardware, IPv6 and final transparent proxy/TUN
behaviour still need a VM or real router.

## ClashCube plugin development

Build the ARM64 daemon and browser frontend, then install them into the running
lab container (Go, Node and the existing frontend dependencies are required):

```sh
sh dev/openwrt/deploy.sh
python3 dev/openwrt/smoke.py
python3 dev/openwrt/router-smoke.py
python3 dev/openwrt/upgrade-smoke.py
python3 dev/openwrt/p0-smoke.py
python3 dev/openwrt/storage-test.py
python3 dev/openwrt/p1-smoke.py
python3 dev/openwrt/geo-schedule-smoke.py
```

Open <http://127.0.0.1:18080/cgi-bin/luci/admin/services/clashcube> and use the
same LuCI root login. ClashCube also appears under LuCI's Services menu.
Its interface is embedded in the LuCI content area, preserving LuCI navigation
without opening a new window.
The deployment script changes only the lab container. It enables independent
`clashcube` (management) and `clashcube-core` (core/firewall) procd services and
raises uhttpd's request limit for concurrent event polls. Redeploy after a
`docker compose down` / recreation.

The default deployment updates management without restarting the core or
dropping existing proxy connections. To update the core explicitly:

```sh
sh dev/openwrt/deploy.sh core   # restarts core, keeps management running
sh dev/openwrt/deploy.sh all    # updates both
```

Initial installation builds both binaries. Migration from the earlier combined
binary needs one core restart. Subsequent management updates preserve core PID,
binary and runtime configuration. Updating the core still interrupts existing
proxy connections. Installable IPKs are also available:

```sh
python3 openwrt/package.py --version 0.2.0-3
python3 dev/openwrt/upgrade-smoke.py --packages
python3 dev/openwrt/package-smoke.py
```

The build emits `clashcube-core`, `clashcube-web` and `luci-app-clashcube` under
`bin/openwrt/packages/`, plus a size/hash manifest. Install all three together
with opkg. Their lifecycle scripts preserve independent core/management
upgrades. Settings also supports explicit core updates from a signed feed,
verified before installation by an independent procd worker. Management/LuCI
updates still use opkg. Fresh installations select the official signed feed;
explicitly empty or custom feeds are preserved. See the release documentation
for the stable channel URL and versioned artifacts.
See [feed signing and maintenance](../../docs/openwrt.md#core-updates-geo-maintenance-and-diagnostics).
`python3 dev/openwrt/core-update-smoke.py` uses the existing release-signing key
to test real updates against a temporary server on the isolated LAN.

Deployment defaults to UPX LZMA compression and requires UPX on the build
host. Use `CLASHCUBE_UPX=nrv` for faster decompression or `CLASHCUBE_UPX=none`
to rebuild unpacked binaries. Measured core/backend sizes are
57.7/11.0 MiB unpacked, 20.2/4.1 MiB with NRV, or 13.7/3.0 MiB with LZMA.
Compression increases RAM use and startup time, so evaluate it on the target
router. See [storage measurements](../../docs/openwrt.md#storage-and-optional-upx).
Frontend resources retain only the current and previous deployment; older
hashes expire so repeated development updates do not grow storage indefinitely.

The current prototype supports core start/stop/restart, profile uploads and
subscriptions, policy selection, modes, modules, rules, connections, logs and
traffic. It reuses the desktop backend and React pages, with native desktop
controls hidden. Profile files can be downloaded; an in-browser profile text
editor is not included. The mixed proxy listens on the router's LAN at port
7897 by default, with loopback retained for local backend requests. Top-level
proxy ports do not bind the WAN interface. It is not published as a host port.
Test it from the client:

```sh
docker compose -f dev/openwrt/compose.yaml exec client \
  curl -x http://172.30.77.2:7897 https://example.com
```

Settings → Network → LAN proxy enables IPv4 TCP and DNS interception with
fw3/iptables-legacy. Once enabled, the client needs no explicit proxy:

```sh
docker compose -f dev/openwrt/compose.yaml exec client \
  curl --noproxy '*' https://example.com
```

The switch defaults to off and persists in the data volume. Rules target only
the LAN interface/subnet; router-originated traffic and ordinary WAN NAT are
preserved. Core stop/crash, disabling and stopping `clashcube-core` clean up
the owned rules. Core service start and firewall reload/restart restore enabled
interception. Stopping/updating `clashcube` leaves the core, interception and
DNS running; the management process reconnects without reloading configuration.
Activation checks core-owned listeners and rolls back failures, including
port conflicts. DNS queries pass through dnsmasq for local hosts and DHCP names;
public queries go to mihomo. Core stop restores the original dnsmasq upstreams.
Other UDP traffic and IPv6 are not intercepted. fw4 support remains subsequent
work.

Device access supports all devices, only listed devices or bypass listed
devices, matching IPv4 hosts/subnets or MAC addresses. Changes update firewall
rules without restarting the core. This controls transparent TCP and forced
DNS; explicit proxy authentication and the shared router DNS remain separate.
Scheduled GEO updates run under the core supervisor even with management
offline. Diagnostics exports curated system, package and storage information
without configuration secrets. Docker kernel/memory figures describe the host.

Storage is selected with `clashcube.main.storage_mode` in UCI: `persistent`
(default), `ram`, or `external` with an already-mounted `external_mount`.
Configuration and identities remain persistent in every mode, while GEO and
provider caches follow the selected core directory. Logs/history use RAM.
Stop both services before switching modes. See
[storage configuration](../../docs/openwrt.md#persistent-configuration-and-storage-modes)
for commands, backup behavior and mount requirements.

The starter profile routes everything DIRECT. Both smoke tests also select
REJECT and confirm requests fail, so successful HTTPS alone is not mistaken
for proof of policy application. The tests restore their configuration changes.

`openwrt/root/` contains the LuCI controller and procd init script;
`cmd/clashcube-web` is the daemon entry point. See
[`docs/openwrt.md`](../../docs/openwrt.md) for the transport and lifecycle.
