#!/usr/bin/env python3
"""Prove a transparent TCP session survives management stop, crash and deployment."""
import hashlib
import argparse
import http.cookiejar
import json
import re
import subprocess
import time
import urllib.parse
import urllib.request
import uuid

from smoke import ROOT, BASE, response

COMPOSE = ["docker", "compose", "-f", str(ROOT / "dev/openwrt/compose.yaml")]
CORE = r"^/usr/bin/clashcube-core core -d "


def command(service, *args, check=True):
    return subprocess.run(COMPOSE + ["exec", "-T", service, *args], capture_output=True, text=True, check=check)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--packages", action="store_true", help="Upgrade with real opkg packages instead of development deploy")
    options = parser.parse_args()
    package_paths = {}
    if options.packages:
        destination = ROOT / "bin/openwrt/packages-test"
        installed = command("router", "opkg", "list-installed", "clashcube-*").stdout
        latest = "0.1.0"
        for line in installed.splitlines():
            fields = line.split(" - ", 1)
            if len(fields) == 2 and command("router", "opkg", "compare-versions", fields[1], ">", latest, check=False).returncode == 0:
                latest = fields[1]
        version = latest + "+test." + str(time.time_ns())
        subprocess.run(["python3", "openwrt/package.py", "--skip-build", "--version", version, "--output", str(destination)], cwd=ROOT, check=True, capture_output=True)
        manifest = json.loads((destination / "packages.json").read_text())
        package_paths = {p["name"]: "/workspace/clash-cube/" + (destination / p["file"]).relative_to(ROOT).as_posix() for p in manifest["packages"]}
        command("router", "opkg", "install", package_paths["clashcube-guard"])

    def deploy_component(component):
        if options.packages:
            name = "clashcube-web" if component == "management" else "clashcube-core"
            return command("router", "opkg", "install", package_paths[name], check=False)
        return subprocess.run(["sh", "dev/openwrt/deploy.sh", component], cwd=ROOT, capture_output=True, text=True, timeout=60)

    client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    response(client, BASE, urllib.parse.urlencode({"luci_username": "root", "luci_password": (ROOT / "dev/openwrt/root-password").read_text().strip()}).encode())
    _, page, _ = response(client, BASE + "/app")
    token = re.search(rb'name="clashcube-token" content="([a-fA-F0-9]+)"', page).group(1).decode()

    def rpc(method, *args):
        code, body, _ = response(client, BASE + "/api/rpc/" + method + "?token=" + token, json.dumps(args).encode(), {"Content-Type": "application/json"})
        assert code == 200, f"{method}: HTTP {code}"
        return json.loads(body)

    def wait_management(core="running"):
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            code, body, _ = response(client, BASE + "/api/rpc/App/State?token=" + token, b"[]", {"Content-Type": "application/json"})
            if code == 200 and (core is None or json.loads(body)["core"] == core):
                # State can still describe the old core while its service is
                # restarting (especially with UPX decompression). Probe a live
                # supervisor RPC before treating management as reconnected.
                guard_code, _, _ = response(client, BASE + "/api/rpc/Router/Get?token=" + token, b"[]", {"Content-Type": "application/json"})
                if guard_code == 200:
                    return
                assert guard_code in (422, 503), f"core service: HTTP {guard_code}"
            assert code in (200, 503), f"management: HTTP {code}"
            time.sleep(0.1)
        raise AssertionError("management did not reconnect to the existing core")

    def connections():
        return [c for c in rpc("Proxy/Connections")["connections"] if c["metadata"]["destinationIP"] == "198.19.0.1"]

    def digest(path):
        return command("router", "sha256sum", path).stdout.split()[0]

    initial = rpc("App/State")
    before = rpc("Router/Get")
    auto_start = rpc("Settings/Get")["autoStart"]
    assert initial["core"] == "running", "start the core first"
    fixture = "/tmp/clashcube-session-" + uuid.uuid4().hex
    output = fixture + ".bin"
    profile = holder = stream = None
    address_added = False
    try:
        rpc("Router/SetEnabled", False)
        profile = rpc("Profiles/ImportContent", "Upgrade smoke", 'rules: ["MATCH,DIRECT"]\n')
        rpc("Profiles/Use", profile["id"])
        rpc("App/SetMode", "rule")
        rpc("Router/SetEnabled", True)
        command("router", "mkdir", fixture)
        command("router", "dd", "if=/dev/zero", "of=" + fixture + "/stream.bin", "bs=1M", "count=16")
        # A benchmark address stays entirely inside this network namespace, but
        # crosses the public-TCP interception rule rather than the private bypass.
        command("router", "ip", "addr", "add", "198.19.0.1/32", "dev", "lo")
        address_added = True
        command("router", "/usr/sbin/uhttpd", "-p", "198.19.0.1:18081", "-h", fixture)
        holder = command("router", "pgrep", "-f", "^/usr/sbin/uhttpd -p 198.19.0.1:18081 -h " + fixture + "$").stdout.strip()
        assert holder.isdigit()
        stream = subprocess.Popen(COMPOSE + ["exec", "-T", "client", "curl", "-fsS", "--noproxy", "*", "--limit-rate", "256k", "--max-time", "100", "-o", output, "http://198.19.0.1:18081/stream.bin"], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            conns = connections()
            if conns:
                break
            time.sleep(0.1)
        assert len(conns) == 1, "download did not cross mihomo"
        connection = conns[0]["id"]
        core_pid = command("router", "pgrep", "-f", CORE).stdout.strip()
        core_hash = digest("/usr/bin/clashcube-core")
        runtime_hash = digest("/var/lib/clashcube/core/runtime.yaml")

        def assert_session():
            assert stream.poll() is None, "download ended during management update"
            assert command("router", "pgrep", "-f", CORE).stdout.strip() == core_pid, "core PID changed"
            assert digest("/usr/bin/clashcube-core") == core_hash, "management replaced the core binary"
            assert digest("/var/lib/clashcube/core/runtime.yaml") == runtime_hash, "management reloaded runtime configuration"
            assert any(c["id"] == connection for c in connections()), "original proxy connection was lost"
            assert rpc("Router/Get")["active"], "interception was not preserved"

        command("router", "/etc/init.d/clashcube", "stop")
        assert command("router", "pgrep", "-f", CORE).stdout.strip() == core_pid
        command("router", "/etc/init.d/firewall", "reload")
        command("client", "nslookup", "example.com", "8.8.8.8")
        command("router", "/etc/init.d/clashcube", "start")
        wait_management()
        assert_session()

        web_pid = command("router", "pgrep", "-f", r"^/usr/bin/clashcube-web -socket ").stdout.strip()
        assert web_pid.isdigit()
        command("router", "kill", "-9", web_pid)
        wait_management()
        assert_session()

        deploy = deploy_component("management")
        assert deploy.returncode == 0, "management deployment failed: " + deploy.stderr[-1000:]
        wait_management()
        assert_session()
        _, error = stream.communicate(timeout=100)
        assert stream.returncode == 0, "continuous download failed: " + error[:200]
        received = command("client", "sha256sum", output).stdout.split()[0]
        expected = hashlib.sha256(bytes(16 * 1024 * 1024)).hexdigest()
        assert received == expected, "download content was truncated or changed"

        web_pid = command("router", "pgrep", "-f", r"^/usr/bin/clashcube-web -socket ").stdout.strip()
        deploy = deploy_component("core")
        assert deploy.returncode == 0, "core deployment failed: " + deploy.stderr[-1000:]
        wait_management()
        deadline = time.monotonic() + 10
        while not rpc("Router/Get")["active"] and time.monotonic() < deadline:
            time.sleep(0.1)
        assert rpc("Router/Get")["active"], "core upgrade did not restore interception"
        assert command("router", "pgrep", "-f", CORE).stdout.strip() != core_pid, "explicit core upgrade did not replace core"
        assert command("router", "pgrep", "-f", r"^/usr/bin/clashcube-web -socket ").stdout.strip() == web_pid, "core upgrade restarted management"
        assert digest("/var/lib/clashcube/core/runtime.yaml") == runtime_hash, "core upgrade rewrote runtime"
        assert command("client", "curl", "-fsS", "--noproxy", "*", "--max-time", "20", "-o", "/dev/null", "-w", "%{http_code}", "https://example.com").stdout == "200"

        rpc("App/Stop")
        command("router", "/etc/init.d/clashcube", "restart")
        wait_management("stopped")
        assert command("router", "pgrep", "-f", CORE, check=False).returncode != 0, "management update started an explicitly stopped core"
        assert not rpc("Router/Get")["active"], "stopped core retained interception"
        rpc("App/Start")

        rpc("Settings/Patch", {"autoStart": False})
        command("router", "/etc/init.d/clashcube-core", "stop")
        command("router", "/etc/init.d/clashcube-core", "boot")
        wait_management("stopped")
        assert command("router", "pgrep", "-f", CORE, check=False).returncode != 0, "router boot ignored AutoStart off"
        rpc("Settings/Patch", {"autoStart": True})
        command("router", "/etc/init.d/clashcube-core", "stop")
        command("router", "/etc/init.d/clashcube-core", "boot")
        wait_management()
        print("PASS: same core PID, binary, runtime and transparent TCP connection across management stop/crash/deploy; complete 16 MiB checksum; offline DNS/firewall reload; independent core deployment/reconnection; stopped state and router AutoStart preserved")
    finally:
        if stream and stream.poll() is None:
            stream.terminate()
            stream.communicate(timeout=5)
        command("router", "/etc/init.d/clashcube-core", "start", check=False)
        command("router", "/etc/init.d/clashcube", "start", check=False)
        wait_management(None)
        if rpc("App/State")["core"] != "running":
            rpc("App/Start")
        rpc("Router/SetEnabled", False)
        rpc("Settings/Patch", {"autoStart": auto_start})
        rpc("Profiles/Use", initial["profile"])
        rpc("App/SetMode", initial["mode"])
        if profile:
            rpc("Profiles/Remove", profile["id"])
        if before["enabled"]:
            rpc("Router/SetEnabled", True)
        if holder:
            command("router", "kill", holder, check=False)
        if address_added:
            command("router", "ip", "addr", "del", "198.19.0.1/32", "dev", "lo", check=False)
        command("router", "rm", "-rf", fixture, check=False)
        command("client", "rm", "-f", output, check=False)


if __name__ == "__main__":
    main()
