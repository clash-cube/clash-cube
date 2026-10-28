#!/usr/bin/env python3
"""Declare already-tested host-kernel/BusyBox facilities to this Docker opkg.

Never use on a real router. QWRT's firmware package database describes its 5.4
kernel, while Docker runs OrbStack's kernel. No firmware module is installed.
"""
import json
import sys

from lab import command
from smoke import ROOT

sys.path.insert(0, str(ROOT / "openwrt"))
from package import archive


def main():
    # Verify actual facilities before registering the lab-only virtual provider.
    command("router", "flock", "/tmp/clashcube-capability-check.lock", "true")
    json.loads(command("router", "nft", "-j", "list", "tables").stdout)
    command("router", "nft", "-c", "-f", "-", input="""table inet clashcube_capability_check {
 chain prerouting { type nat hook prerouting priority -111; policy accept;
  meta l4proto tcp redirect to :18101
 }
}
""")
    name = "clashcube-docker-capabilities"
    control = archive({"control": ((f"Package: {name}\nVersion: 1\nArchitecture: all\n"
                                    "Depends: busybox, kmod-nft-core\nProvides: flock, kmod-nft-nat\n"
                                    "Installed-Size: 0\nDescription: Docker-only host-kernel NAT and BusyBox flock capability marker; not firmware modules\n").encode(), 0o644)})
    body = archive({"debian-binary": (b"2.0\n", 0o644), "control.tar.gz": (control, 0o644), "data.tar.gz": (archive({}), 0o644)})
    directory = ROOT / "bin/openwrt/lab"
    directory.mkdir(parents=True, exist_ok=True)
    path = directory / (name + "_1_all.ipk")
    path.write_bytes(body)
    command("router", "opkg", "install", "/workspace/clash-cube/" + path.relative_to(ROOT).as_posix())
    print("PASS: registered tested Docker-only NAT/flock capabilities; no firmware kernel module installed")


if __name__ == "__main__":
    main()
