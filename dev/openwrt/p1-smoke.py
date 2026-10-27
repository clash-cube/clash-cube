#!/usr/bin/env python3
"""Exercise device access and diagnostic exports against real QWRT services."""
import json

from lab import Lab, command


def main():
    lab = Lab()
    initial = lab.rpc("App/State")
    router = lab.rpc("Router/Get")
    profile = None
    try:
        lab.rpc("Router/SetEnabled", False)
        lab.rpc("App/Reload")
        command("client", "curl", "-fsS", "--max-time", "12", "-x", "http://172.30.77.2:7897", "https://example.com")
        command("router", "curl", "-fsS", "--max-time", "12", "-x", "http://127.0.0.1:7897", "https://example.com")
        assert command("router", "curl", "-fsS", "--connect-timeout", "2", "-x", "http://172.30.76.2:7897", "https://example.com", check=False).returncode != 0, "explicit proxy is exposed on WAN"
        profile = lab.rpc("Profiles/ImportContent", "P1 access smoke", 'rules: ["MATCH,REJECT"]\n')
        lab.rpc("Profiles/Use", profile["id"])
        lab.rpc("App/SetMode", "rule")
        lab.rpc("Router/SetEnabled", True)
        pid = command("router", "pgrep", "-f", r"^/usr/bin/clashcube-core core -d ").stdout
        runtime = command("router", "sha256sum", "/etc/clashcube/core/runtime.yaml").stdout
        mac = command("client", "cat", "/sys/class/net/eth0/address").stdout.strip()

        def test(policy, direct):
            lab.rpc("Router/SetAccess", policy)
            result = command("client", "curl", "--noproxy", "*", "-fsS", "--connect-timeout", "4", "--max-time", "12", "https://example.com", check=False)
            assert (result.returncode == 0) == direct, f"unexpected device routing for {policy['mode']}"
            assert command("router", "pgrep", "-f", r"^/usr/bin/clashcube-core core -d ").stdout == pid, "device-list edit restarted core"
            assert command("router", "sha256sum", "/etc/clashcube/core/runtime.yaml").stdout == runtime, "device-list edit rebuilt runtime"

        test({"mode": "all", "addresses": [], "macs": []}, False)
        test({"mode": "exclude", "addresses": ["172.30.77.3"], "macs": []}, True)
        test({"mode": "exclude", "addresses": [], "macs": [mac]}, True)
        test({"mode": "include", "addresses": ["172.30.77.250"], "macs": []}, True)
        test({"mode": "include", "addresses": ["172.30.77.250"], "macs": [mac]}, False)
        test({"mode": "include", "addresses": [], "macs": []}, True)
        test({"mode": "include", "addresses": ["172.30.77.0/24"], "macs": []}, False)
        policy = lab.rpc("Router/Get")["access"]
        try:
            lab.rpc("Router/SetAccess", {"mode": "exclude", "addresses": ["::1"], "macs": []})
            raise AssertionError("accepted IPv6 device access in an IPv4-only router")
        except AssertionError as error:
            assert "HTTP 422" in str(error)
        assert lab.rpc("Router/Get")["access"] == policy
        command("router", "/etc/init.d/clashcube", "stop")
        command("router", "/etc/init.d/firewall", "reload")
        result = command("client", "curl", "--noproxy", "*", "-fsS", "--connect-timeout", "4", "--max-time", "12", "https://example.com", check=False)
        assert result.returncode != 0, "firewall reload with management offline lost device policy"
        command("router", "/etc/init.d/clashcube", "start")
        lab.ready()
        assert lab.rpc("Router/Get")["access"] == policy
        lab.rpc("Router/SetEnabled", False)
        assert lab.rpc("Router/Get")["access"] == policy, "disabling erased device policy"
        lab.rpc("Router/SetEnabled", True)
        report = lab.rpc("Router/Diagnostics")
        assert "OpenWrt" in report["system"] and report["memoryTotal"] > 0
        assert len(report["storage"]) == 3 and report["storage"][0]["total"] > 0
        private = json.loads(command("router", "cat", "/etc/clashcube/core-service.json").stdout)["controller"]["Secret"]
        assert private and private not in json.dumps(report), "diagnostic export leaked controller credentials"
        print("PASS: IP/subnet/MAC include and exclude, empty include, IP-or-MAC matching, rejected edits, same PID/runtime, offline firewall reload, diagnostic export")
    finally:
        command("router", "/etc/init.d/clashcube", "start")
        lab.ready()
        lab.rpc("Router/SetEnabled", False)
        lab.rpc("Router/SetAccess", router.get("access") or {"mode": "all", "addresses": [], "macs": []})
        lab.rpc("Profiles/Use", initial["profile"])
        lab.rpc("App/SetMode", initial["mode"])
        if router["enabled"]:
            lab.rpc("Router/SetEnabled", True)
        if profile:
            lab.rpc("Profiles/Remove", profile["id"])


if __name__ == "__main__":
    main()
