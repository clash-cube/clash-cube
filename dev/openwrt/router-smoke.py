#!/usr/bin/env python3
"""Verify real LAN interception, cleanup and fw3 reload in the Docker lab only."""
import http.cookiejar
import json
import re
import subprocess
import time
import urllib.parse
import urllib.request

from smoke import ROOT, BASE, response

COMPOSE = ["docker", "compose", "-f", str(ROOT / "dev/openwrt/compose.yaml")]


def command(service, *args, check=True):
    return subprocess.run(COMPOSE + ["exec", "-T", service, *args], capture_output=True, text=True, check=check)


def main():
    client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    login = urllib.parse.urlencode({"luci_username": "root", "luci_password": (ROOT / "dev/openwrt/root-password").read_text().strip()}).encode()
    response(client, BASE, login)
    _, page, _ = response(client, BASE + "/app")
    token = re.search(rb'name="clashcube-token" content="([a-fA-F0-9]+)"', page).group(1).decode()

    def rpc(method, *args, expected=200):
        status, body, _ = response(client, BASE + "/api/rpc/" + method + "?token=" + token, json.dumps(args).encode(), {"Content-Type": "application/json"})
        assert status == expected, f"{method}: HTTP {status}: {body.decode()[:300]}"
        return json.loads(body)

    def wait_active(active):
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            code, body, _ = response(client, BASE + "/api/rpc/Router/Get?token=" + token, b"[]", {"Content-Type": "application/json"})
            assert code in (200, 503), f"Router/Get: HTTP {code}"
            if code == 200:
                status = json.loads(body)
                if active is None or status["active"] == active:
                    return status
            time.sleep(0.1)
        raise AssertionError(f"LAN proxy did not become active={active}")

    def direct_https():
        return command("client", "curl", "-sS", "--noproxy", "*", "--connect-timeout", "5", "--max-time", "20",
            "-o", "/dev/null", "-w", "%{http_code}", "https://example.com", check=False)

    initial = rpc("App/State")
    before = rpc("Router/Get")
    assert initial["core"] == "running", "start the core first"
    profile = None
    holder = None
    service_needs_recovery = False
    try:
        rpc("Router/SetEnabled", False)
        profile = rpc("Profiles/ImportContent", "LAN smoke", "proxy-groups:\n  - name: LAN test\n    type: select\n    proxies: [DIRECT, REJECT]\nrules:\n  - MATCH,LAN test\n")
        rpc("Profiles/Use", profile["id"])
        rpc("App/SetMode", "rule")
        rpc("Router/SetEnabled", True)
        assert wait_active(True)["enabled"]
        command("client", "nslookup", "example.com", "8.8.8.8")
        counters = command("router", "iptables-save", "-c", "-t", "nat").stdout
        dns_packets = re.search(r"\[(\d+):\d+\] -A CLASHCUBE_LAN -p udp .*--dport 53 .*--to-ports 53(?:\s|$)", counters)
        assert dns_packets and int(dns_packets[1]) > 0, "external DNS did not cross the interception rule"
        assert direct_https().stdout == "200", "transparent HTTPS failed"
        rules = command("router", "iptables-save", "-t", "nat").stdout
        assert rules.count("-j CLASHCUBE_LAN\n") == 1, "duplicate LAN hooks"
        assert not any("CLASHCUBE_LAN" in line and "OUTPUT" in line for line in rules.splitlines()), "router's own traffic was captured"
        assert "MASQUERADE" in rules, "ordinary WAN NAT lost"

        rpc("Proxy/Select", "LAN test", "REJECT")
        assert direct_https().returncode != 0, "unconfigured client bypassed REJECT"
        rpc("Proxy/Select", "LAN test", "DIRECT")
        command("router", "/etc/init.d/firewall", "reload")
        wait_active(True)
        assert direct_https().stdout == "200", "firewall reload lost interception"
        command("router", "/etc/init.d/firewall", "restart")
        wait_active(True)
        assert direct_https().stdout == "200", "firewall restart lost interception"

        rpc("App/Stop")
        assert not wait_active(False)["active"]
        assert "CLASHCUBE_LAN" not in command("router", "iptables-save", "-t", "nat").stdout, "stopped core left hooks"
        rpc("App/Start")
        wait_active(True)
        rpc("App/Restart")
        wait_active(True)

        core_pid = command("router", "pgrep", "-f", r"^/usr/bin/clashcube-core core -d ").stdout.strip()
        assert core_pid.isdigit(), "could not identify the lab core"
        command("router", "kill", "-9", core_pid)
        wait_active(False)
        assert "CLASHCUBE_LAN" not in command("router", "iptables-save", "-t", "nat").stdout, "crashed core left hooks"
        assert direct_https().stdout == "200", "crashed core blocked ordinary forwarding"
        rpc("App/Start")
        wait_active(True)

        service_needs_recovery = True
        command("router", "/etc/init.d/clashcube", "stop")
        core_pid = command("router", "pgrep", "-f", r"^/usr/bin/clashcube-core core -d ").stdout.strip()
        assert "CLASHCUBE_LAN" in command("router", "iptables-save", "-t", "nat").stdout, "management stop removed interception"
        assert direct_https().stdout == "200", "management stop broke forwarding"
        command("router", "/etc/init.d/firewall", "reload")
        assert direct_https().stdout == "200", "firewall reload needs management"
        command("router", "/etc/init.d/clashcube", "start")
        wait_active(True)
        service_needs_recovery = False
        assert command("router", "pgrep", "-f", r"^/usr/bin/clashcube-core core -d ").stdout.strip() == core_pid, "management restart replaced the core"
        assert direct_https().stdout == "200", "procd restart lost persistent LAN proxy state"

        rpc("Router/SetEnabled", False)
        rpc("Proxy/Select", "LAN test", "REJECT")
        assert direct_https().stdout == "200", "disable did not restore direct forwarding"
        rpc("Proxy/Select", "LAN test", "DIRECT")
        # A foreign listener must not be mistaken for a working redir listener.
        command("router", "/usr/sbin/uhttpd", "-p", "172.30.77.2:7898", "-h", "/tmp")
        holder = command("router", "pgrep", "-f", r"^/usr/sbin/uhttpd -p 172\.30\.77\.2:7898 -h /tmp$").stdout.strip()
        assert holder.isdigit()
        probe = command("client", "curl", "-sS", "--noproxy", "*", "--max-time", "3", "-o", "/dev/null", "-w", "%{http_code}", "http://172.30.77.2:7898").stdout
        assert probe.isdigit() and probe != "000", "port collision fixture is not listening"
        rpc("Router/SetEnabled", True, expected=422)
        assert not rpc("Router/Get")["enabled"], "failed activation was persisted"
        assert "CLASHCUBE_LAN" not in command("router", "iptables-save", "-t", "nat").stdout, "failed activation left hooks"
        print("PASS: transparent DNS + HTTPS, REJECT, LAN-only hook, preserved NAT, firewall reload/restart, core crash cleanup, management restart preserves core, offline-management firewall reload, disable, occupied-port rollback")
    finally:
        if service_needs_recovery:
            command("router", "/etc/init.d/clashcube", "start")
            wait_active(None)
        if holder:
            command("router", "kill", holder, check=False)
        rpc("Router/SetEnabled", False)
        if rpc("App/State")["core"] != "running":
            rpc("App/Start")
        rpc("Profiles/Use", initial["profile"])
        rpc("App/SetMode", initial["mode"])
        if profile:
            rpc("Profiles/Remove", profile["id"])
        if before["enabled"]:
            rpc("Router/SetEnabled", True)


if __name__ == "__main__":
    main()
