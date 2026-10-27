#!/usr/bin/env python3
"""Remove/reinstall the three IPKs and verify DNS cleanup and data retention."""
import json
import time

from lab import Lab, command, uci
from smoke import ROOT


def main():
    destination = ROOT / "bin/openwrt/packages"
    manifest = json.loads((destination / "packages.json").read_text())
    paths = ["/workspace/clash-cube/" + (destination / p["file"]).relative_to(ROOT).as_posix() for p in manifest["packages"]]
    lab = Lab()
    initial = lab.rpc("App/State")
    enabled = lab.rpc("Router/Get")["enabled"]
    lab.rpc("Router/SetEnabled", False)
    dns_before = uci("get", section="@dnsmasq[0]")["values"]
    marker = "/etc/clashcube/p0-package-marker"
    command("router", "touch", marker)
    try:
        lab.rpc("Router/SetEnabled", True)
        for name in ("luci-app-clashcube", "clashcube-web", "clashcube-core"):
            command("router", "opkg", "remove", name)
        assert command("router", "pgrep", "-f", r"^/usr/bin/clashcube-(core|web)( |$)", check=False).returncode != 0
        assert command("router", "iptables", "-t", "nat", "-S", "CLASHCUBE_LAN", check=False).returncode != 0
        assert uci("get", section="@dnsmasq[0]")["values"] == dns_before, "removal left DNS pointing at the removed core"
        command("router", "test", "-f", marker)
        command("router", "test", "-f", "/etc/clashcube/settings.json")
        assert command("router", "test", "-e", "/www/luci-static/resources/clashcube", check=False).returncode != 0
        # procd reload is asynchronous. Verify actual DNS recovery rather than
        # assuming the dnsmasq process is already listening when opkg returns.
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            if command("client", "nslookup", "example.com", "172.30.77.2", check=False).returncode == 0:
                break
            time.sleep(0.1)
        else:
            raise AssertionError("DNS did not recover after core removal")
    finally:
        command("router", "opkg", "install", *paths)
        lab = Lab()
        lab.ready()
        command("router", "test", "-f", marker)
        assert lab.rpc("App/State")["profile"] == initial["profile"], "reinstall lost the selected profile"
        lab.rpc("Router/SetEnabled", enabled)
        command("router", "rm", "-f", marker)
    print("PASS: opkg removal stops services, restores DNS/firewall, removes public assets and retains data; reinstall restores selected profile")


if __name__ == "__main__":
    main()
