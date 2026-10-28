#!/usr/bin/env python3
"""Destructive only to the isolated Docker lab; restore policy/profile on exit.

Requires deploy.sh all, then docker compose -f dev/openwrt/egress/compose.yaml up -d --build.
Real routed probes distinguish the two proxy source addresses from direct WAN.
"""
import argparse
import json
import subprocess
import sys
import time
from pathlib import Path

from lab import Lab, command

ROOT = Path(__file__).resolve().parents[2]
PEER = "clashcube-egress-tests-peer-1"
ORIGIN = "clashcube-egress-tests-origin-1"
COMPOSE = ["docker", "compose", "-f", str(ROOT / "dev/openwrt/compose.yaml")]
results = []
parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument("--packages",action="store_true",help="Also reinstall/remove real IPKs; install-lab-capabilities.py is required in Docker")
options=parser.parse_args()


def peer(*args, container=PEER):
    return subprocess.check_output(["docker", "exec", container, *args], text=True, timeout=15).strip()


def probe(kind="tcp", host="172.30.76.10", port=None):
    args = ["python3", "/lab/peer.py", "probe", kind, host]
    if port:
        args.append(str(port))
    return peer(*args)


def check(name, actual, expected):
    assert actual == expected, f"{name}: expected {expected!r}, got {actual!r}"
    results.append(name)
    print("PASS:", name, flush=True)


def wait_probe(expected, timeout=12):
    end = time.monotonic() + timeout
    while time.monotonic() < end:
        if probe() == expected:
            return
        time.sleep(0.15)
    raise AssertionError(f"traffic did not become {expected}")


def poll_during(name, action, allowed=("NODE-A", "BLOCKED"), seconds=5):
    process = subprocess.Popen(["docker", "exec", PEER, "python3", "/lab/peer.py", "poll", str(seconds)], stdout=subprocess.PIPE, text=True)
    try:
        action()
        counts = json.loads(process.communicate(timeout=seconds + 5)[0])
        assert counts and set(counts).issubset(allowed), f"{name}: {counts}"
        results.append(name)
        print(f"PASS: {name}: {counts}", flush=True)
    finally:
        if process.poll() is None:
            process.kill()


lab = Lab()
lab.ready()
initial = lab.rpc("Settings/Get")
initial_router=lab.rpc("Router/Get")
original_profile = initial["profile"]
profile = None
mac = peer("cat", "/sys/class/net/eth0/address")


def view():
    return lab.rpc("Router/Devices")


def apply(node="Lab node A", **changes):
    v = view()
    existing = v["policies"][0] if v["policies"] else None
    req = {"revision": v["revision"], "id": existing["id"] if existing else "", "name": "Egress smoke client", "mac": mac,
           "ip": "172.30.77.4", "target": next(n for n in v["nodes"] if n["name"] == node), "reserve": True}
    req.update(changes)
    return lab.rpc("Router/ApplyDevice", req)


def pause(on):
    v = view()
    return lab.rpc("Router/PauseDevice", v["policies"][0]["id"], v["revision"], on)


def release_all():
    for p in view()["policies"]:
        assert p["name"] in ("Egress smoke client", "Lab protected client"), "refusing to release an unrelated policy"
        lab.rpc("Router/ReleaseDevice", p["id"], view()["revision"])


try:
    release_all()
    check("unrestricted IPv4 control", probe(), "DIRECT")
    check("unrestricted UDP control", probe("udp"), "DIRECT-UDP")
    check("unrestricted DNS control", probe("dns"), "DNS-OK")
    for service,interface,address in [("router","eth0","fd30:76::2/64"),("router","eth1","fd30:77::2/64")]:
        command(service,"ip","-6","addr","replace",address,"dev",interface)
    peer("ip","-6","addr","replace","fd30:77::4/64","dev","eth0")
    peer("ip","-6","route","replace","default","via","fd30:77::2")
    peer("ip","-6","addr","replace","fd30:76::10/64","dev","eth0",container=ORIGIN)
    peer("ip","-6","route","replace","fd30:77::/64","via","fd30:76::2",container=ORIGIN)
    # Wait for duplicate-address detection before proving IPv6 reachability.
    deadline=time.monotonic()+5
    while probe("tcp","fd30:76::10")!="DIRECT" and time.monotonic()<deadline:time.sleep(.1)
    check("unrestricted IPv6 control",probe("tcp","fd30:76::10"),"DIRECT")
    content = {"proxies": [{"name": n, "type": "http", "server": "172.30.76.10", "port": port} for n, port in [("Lab node A", 18081), ("Lab node B", 18082)]],
               "proxy-groups": [{"name": "Lab select", "type": "select", "proxies": ["DIRECT", "Lab node A", "Lab node B"]}], "rules": ["MATCH,DIRECT"]}
    profile = lab.rpc("Profiles/ImportContent", "Egress smoke fixture", json.dumps(content))
    lab.rpc("Profiles/Use", profile["id"])
    command("router","/etc/init.d/clashcube-guard","disable")
    try:
        try:apply();raise AssertionError("disabled boot guard was accepted")
        except AssertionError as e:assert "Enable the clashcube-guard" in str(e)
    finally:command("router","/etc/init.d/clashcube-guard","enable")
    results.append("disabled boot guard prevents policy activation")
    command("router","uci","set","firewall.@defaults[0].flow_offloading=1")
    try:
        try:apply();raise AssertionError("flow offload was accepted")
        except AssertionError as e:assert "flow offloading" in str(e)
    finally:command("router","uci","revert","firewall.@defaults[0].flow_offloading")
    results.append("flow offload prevents policy activation")
    # An established direct socket must also lose connectivity when protected.
    stream = subprocess.Popen(["docker", "exec", PEER, "python3", "/lab/peer.py", "stream"], stdout=subprocess.PIPE, text=True)
    assert stream.stdout.readline().strip() == "CONNECTED"
    protected = apply()
    stream.communicate(timeout=4)
    check("existing direct stream terminated", stream.returncode, 0)
    check("kernel protection verified", protected["policies"][0]["protection"], "protected")
    check("fixed node A", probe(), "NODE-A")
    check("unsupported UDP blocked", probe("udp"), "BLOCKED")
    check("unsupported IPv6 blocked", probe("tcp","fd30:76::10"),"BLOCKED")
    check("IPv6 router DNS cannot bypass protection",probe("dns","fd30:77::2"),"BLOCKED")
    check("shared proxy cannot bypass fixed device egress",probe("proxy","172.30.77.2",7897),"BLOCKED")
    check("private inbound requires NAT identity",probe("proxy","172.30.77.2",18101),"BLOCKED")
    check("private DNS requires NAT identity",probe("dns","172.30.77.2",18201),"BLOCKED")
    check("unprotected client can use ordinary shared proxy",command("client","curl","-fsS","--max-time","2","-x","http://172.30.77.2:7897","http://172.30.76.10:8080").stdout.strip(),"DIRECT")
    assert peer("curl","-sS","--max-time","2","-o","/dev/null","-w","%{http_code}","http://172.30.77.2/cgi-bin/luci/") in ("200","302","403")
    results.append("protected device retains router management access")
    lab.rpc("Router/SetEnabled",True)
    check("fixed node wins over ordinary LAN interception",probe(),"NODE-A")
    check("protected DNS wins over shared dnsmasq",probe("dns","172.30.77.2"),"DNS-OK")
    lab.rpc("Router/SetAccess",{"mode":"exclude","addresses":["172.30.77.4"],"macs":[]})
    check("ordinary bypass list cannot release device protection",probe(),"NODE-A")
    lab.rpc("Router/SetAccess",initial_router["access"])
    lab.rpc("Router/SetEnabled",initial_router["enabled"])
    stats_before=json.loads(peer("curl","-fsS","http://127.0.0.1:8080/stats",container=ORIGIN))
    check("router DNS uses fixed node", probe("dns", "172.30.77.2"), "DNS-OK")
    check("external UDP DNS captured", probe("dns", "8.8.8.8"), "DNS-OK")
    check("external TCP DNS captured", probe("dns-tcp", "8.8.8.8"), "DNS-OK")
    stats_after=json.loads(peer("curl","-fsS","http://127.0.0.1:8080/stats",container=ORIGIN))
    assert stats_after.get("proxy-A",0)-stats_before.get("proxy-A",0)>=3, "DNS did not traverse selected node"
    check("public DNS reaches proxy TCP resolver",stats_after.get("tcp-dns-proxy",0)-stats_before.get("tcp-dns-proxy",0),3)
    check("protected DNS has no direct resolver traffic",stats_after.get("direct-dns",0)+stats_after.get("tcp-dns-direct",0),stats_before.get("direct-dns",0)+stats_before.get("tcp-dns-direct",0))
    check("unprotected client still routes", command("client", "curl", "-fsS", "--max-time", "2", "http://172.30.76.10:8080").stdout.strip(), "DIRECT")
    reservation = command("router", "uci", "show", "dhcp").stdout
    assert "172.30.77.4" in reservation and mac in reservation
    results.append("DHCP address reservation persisted")
    for mode in ("direct", "global", "rule"):
        lab.rpc("App/SetMode", mode)
        check("fixed node overrides " + mode, probe(), "NODE-A")
    poll_during("switch node without direct fallback", lambda: apply("Lab node B"), ("NODE-A", "NODE-B", "BLOCKED"))
    check("fixed node B", probe(), "NODE-B")
    apply()
    stream = subprocess.Popen(["docker", "exec", PEER, "python3", "/lab/peer.py", "stream"], stdout=subprocess.PIPE, text=True)
    assert stream.stdout.readline().strip() == "CONNECTED"
    pause(True)
    stream.communicate(timeout=4)
    check("existing proxy stream terminated on pause", stream.returncode, 0)
    check("pause retains TCP block", probe(), "BLOCKED")
    check("pause has no public DNS fallback", probe("dns", "172.30.77.2"), "SERVFAIL")
    pause(False)
    check("resume fixed egress", probe(), "NODE-A")
    poll_during("UI stop never routes direct", lambda: lab.rpc("App/Stop"))
    check("stopped core blocks TCP", probe(), "BLOCKED")
    check("stopped core returns DNS failure", probe("dns", "172.30.77.2"), "SERVFAIL")
    check("stopped state retains protection", view()["policies"][0]["protection"], "protected")
    lab.rpc("App/Start")
    wait_probe("NODE-A")
    poll_during("UI restart never routes direct", lambda: lab.rpc("App/Restart"))
    poll_during("fw3 reload preserves independent guard", lambda: command("router", "/etc/init.d/firewall", "reload"))
    check("guard remains verified after fw3 reload", view()["policies"][0]["protection"], "protected")
    poll_during("management restart preserves routing", lambda: command("router", "/etc/init.d/clashcube", "restart"))
    lab.ready()
    poll_during("core service stop keeps protection", lambda: command("router", "/etc/init.d/clashcube-core", "stop"))
    check("offline supervisor cannot leak DNS", probe("dns", "172.30.77.2"), "BLOCKED")
    command("router", "/etc/init.d/clashcube-core", "start")
    lab.ready()
    wait_probe("NODE-A")
    poll_during("SIGKILL core never routes direct", lambda: command("router", "sh", "-c", "kill -KILL $(pgrep -f '^/usr/bin/clashcube-core core -d ')") )
    lab.rpc("App/Start")
    wait_probe("NODE-A")
    poll_during("SIGKILL supervisor never routes direct",lambda:command("router","sh","-c","kill -KILL $(pgrep -f '^/usr/bin/clashcube-core$')"),seconds=9)
    lab.ready()
    wait_probe("NODE-A",timeout=20)
    check("supervisor respawn has exactly one child core",len(command("router","pgrep","-f","^/usr/bin/clashcube-core core -d ").stdout.splitlines()),1)
    # Rule drift is detected structurally, rather than by table existence.
    command("router", "nft", "add", "rule", "inet", "clashcube_guard", "forward", "counter")
    check("modified guard cannot display verified", view()["policies"][0]["protection"], "unknown")
    command("router", "/etc/init.d/clashcube-guard", "reload")
    check("independent guard restores verified rules", view()["policies"][0]["protection"], "protected")
    stale = view()
    pause(True)
    try:
        lab.rpc("Router/PauseDevice", stale["policies"][0]["id"], stale["revision"], False)
        raise AssertionError("stale writer succeeded")
    except AssertionError as e:
        assert "HTTP 422" in str(e)
    results.append("stale concurrent writes refused")
    pause(False)
    # A different MAC borrowing the reserved IP cannot use the private inbound.
    peer("ip", "link", "set", "eth0", "address", "02:cc:00:00:00:04")
    try:
        check("wrong MAC cannot inherit fixed IP policy", probe(), "BLOCKED")
        check("wrong MAC cannot borrow DNS relay", probe("dns", "172.30.77.2"), "BLOCKED")
    finally:
        peer("ip", "link", "set", "eth0", "address", mac)
    peer("ip", "addr", "del", "172.30.77.4/24", "dev", "eth0")
    peer("ip", "addr", "add", "172.30.77.5/24", "dev", "eth0")
    peer("ip", "route", "replace", "default", "via", "172.30.77.2")
    try:
        check("same MAC with changed IP remains blocked", probe(), "BLOCKED")
    finally:
        peer("ip", "addr", "del", "172.30.77.5/24", "dev", "eth0")
        peer("ip", "addr", "add", "172.30.77.4/24", "dev", "eth0")
        peer("ip", "route", "replace", "default", "via", "172.30.77.2")
    command("router", "ip", "neigh", "flush", "to", "172.30.77.4", "dev", "eth1")
    wait_probe("NODE-A")
    # A same-name endpoint replacement is treated as a missing target.
    content["proxies"][0]["port"] = 18082
    command("router", "tee", "/etc/clashcube/profiles/" + profile["id"] + ".yaml", input=json.dumps(content))
    lab.rpc("App/Reload")
    check("same-name endpoint replacement blocks", probe(), "BLOCKED")
    check("missing target visible", view()["policies"][0]["connectivity"], "target-missing")
    content["proxies"][0]["port"] = 18081
    command("router", "tee", "/etc/clashcube/profiles/" + profile["id"] + ".yaml", input=json.dumps(content))
    lab.rpc("App/Reload")
    wait_probe("NODE-A")
    lab.rpc("Profiles/Use", "default")
    check("profile without node blocks", probe(), "BLOCKED")
    lab.rpc("Profiles/Use", profile["id"])
    wait_probe("NODE-A")
    # Boot the full router with management disabled; guard and core are separate.
    command("router", "/etc/init.d/clashcube", "disable")
    poll_during("full QWRT reboot never routes direct", lambda: subprocess.run(COMPOSE + ["restart", "router"], check=True, capture_output=True, timeout=35), seconds=12)
    wait_probe("NODE-A", timeout=30)
    check("management disabled during reboot", command("router", "pgrep", "-f", "^/usr/bin/clashcube-web", check=False).returncode, 1)
    command("router", "/etc/init.d/clashcube", "enable")
    command("router", "/etc/init.d/clashcube", "start")
    lab = Lab()
    lab.ready()
    check("boot restores verified protection", view()["policies"][0]["protection"], "protected")
    # Cache mismatch must produce an emergency gate, including router DNS.
    command("router", "sh", "-c", "sed -i '1s/$/broken/' /etc/clashcube-guard/guard.nft")
    command("router", "/etc/init.d/clashcube-guard", "reload")
    check("interrupted cache blocks external routing", probe(), "BLOCKED")
    check("emergency blocks shared DNS", probe("dns", "172.30.77.2"), "BLOCKED")
    check("emergency cannot display verified", view()["policies"][0]["protection"], "unknown")
    # Re-saving a reviewed policy repairs the emergency cache transaction.
    apply()
    check("reviewed policy repairs emergency", probe(), "NODE-A")
    config=json.loads(command("router","cat","/etc/clashcube/devices.json").stdout)
    config["revision"]="0"*24
    command("router","tee","/etc/clashcube/devices.json",input=json.dumps(config))
    poll_during("interrupted settings save recovers closed",lambda:command("router","/etc/init.d/clashcube-core","restart"))
    lab.ready()
    check("authoritative boot intent recovers as pending",view()["policies"][0]["connectivity"],"pending")
    check("recovered pending identity remains blocked",probe(),"BLOCKED")
    apply()
    check("reviewed recovery restores chosen node",probe(),"NODE-A")
    if options.packages:
        destination=ROOT/"bin/openwrt/packages"
        manifest=json.loads((destination/"packages.json").read_text())
        paths={p["name"]:"/workspace/clash-cube/"+(destination/p["file"]).relative_to(ROOT).as_posix() for p in manifest["packages"]}
        # Only Docker's separate capability marker supplies the host-kernel
        # facilities. The actual application packages use normal opkg installs.
        for package in ("clashcube-guard","clashcube-web","clashcube-core"):
            poll_during("protected "+package+" package replacement",lambda p=package:command("router","opkg","--force-reinstall","install",paths[p]))
            lab.ready();wait_probe("NODE-A")
        poll_during("stopping guard service preserves kernel rules",lambda:command("router","/etc/init.d/clashcube-guard","stop"))
        def remove_services():
            command("router","opkg","--force-depends","remove","clashcube-web")
            command("router","opkg","--force-depends","remove","clashcube-core")
        poll_during("core and management package removal remains closed",remove_services)
        check("removed core blocks public DNS",probe("dns","172.30.77.2"),"BLOCKED")
        removal=command("router","opkg","--force-depends","remove","clashcube-guard",check=False)
        assert removal.returncode!=0 and "Release protected devices" in removal.stderr+removal.stdout, "guard removal was not refused"
        results.append("guard package refuses removal while devices are protected")
        command("router","opkg","install",paths["clashcube-core"],paths["clashcube-web"])
        lab.ready();wait_probe("NODE-A")
        check("package reinstall restores original fixed egress",probe(),"NODE-A")
    release_all()
    check("explicit release restores ordinary routing", probe(), "DIRECT")
    check("explicit release restores ordinary UDP", probe("udp"), "DIRECT-UDP")
    report = {"passed": len(results), "scenarios": results, "environment": "QWRT ARM64 / procd / fw3 + independent nftables guard / Docker OrbStack"}
    target = ROOT / "bin/openwrt/device-egress-report.json"
    target.write_text(json.dumps(report, indent=2) + "\n")
    print(f"PASS: {len(results)} device egress checks; report: {target}")
finally:
    # Preserve a failed policy for diagnosis if services are unreachable rather
    # than deleting protection behind the user's back.
    try:
        command("router", "/etc/init.d/clashcube", "enable")
        command("router", "/etc/init.d/clashcube", "start")
        command("router", "/etc/init.d/clashcube-core", "start")
        lab = Lab()
        lab.ready()
        release_all()
        lab.rpc("Profiles/Use", original_profile)
        if profile:
            lab.rpc("Profiles/Remove", profile["id"])
        lab.rpc("App/SetMode", initial["mode"])
        lab.rpc("Router/SetAccess",initial_router["access"])
        lab.rpc("Router/SetEnabled",initial_router["enabled"])
    except Exception as e:
        print("Cleanup requires review:", str(e)[:200], file=sys.stderr)
