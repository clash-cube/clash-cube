#!/usr/bin/env python3
"""Build installable ARM64/fw3 IPKs from the existing cross-compiled binaries.

No firmware build or SDK is required. The four packages have independent
lifecycle scripts: replacing management never stops the core service.
"""
import argparse
import gzip
import hashlib
import io
import json
import pathlib
import re
import subprocess
import tarfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
PAYLOAD = ROOT / "openwrt/root"


def archive(files):
    buffer = io.BytesIO()
    with tarfile.open(fileobj=buffer, mode="w", format=tarfile.GNU_FORMAT) as tar:
        # opkg's extractor requires explicit parent entries on a fresh install;
        # a development deployment may have left these directories behind.
        directories = {str(parent) for name in files for parent in pathlib.PurePosixPath(name).parents if str(parent) != "."}
        for name in sorted(directories):
            item = tarfile.TarInfo("./" + name.lstrip("/") + "/")
            item.type, item.mode = tarfile.DIRTYPE, 0o755
            item.uid = item.gid = 0
            item.uname = item.gname = "root"
            tar.addfile(item)
        for name, (data, mode) in sorted(files.items()):
            item = tarfile.TarInfo("./" + name.lstrip("/"))
            item.size, item.mode = len(data), mode
            item.uid = item.gid = 0
            item.uname = item.gname = "root"
            tar.addfile(item, io.BytesIO(data))
    return gzip.compress(buffer.getvalue(), compresslevel=9, mtime=0)


def build_packages(version, destination):
    destination.mkdir(parents=True, exist_ok=True)
    specs = {
        "clashcube-guard": {
            "arch": "all", "depends": "nftables-json, kmod-nft-nat, jsonfilter, flock",
            "description": "Persistent fail-closed device firewall independent of mihomo and management",
            "files": ["etc/init.d/clashcube-guard", "usr/share/clashcube/device-guard.sh", "lib/upgrade/keep.d/clashcube-guard"],
            "hooks": "guard",
        },
        "clashcube-core": {
            "arch": "aarch64_generic", "depends": "libc, ca-bundle, dnsmasq, iptables, kmod-ipt-nat, ip-full, procd, ubus, rpcd, clashcube-guard",
            "description": "ClashCube mihomo core service, fw3 and storage integration",
            "files": ["etc/init.d/clashcube-core", "etc/config/clashcube", "lib/upgrade/keep.d/clashcube",
                      "usr/share/clashcube/storage.sh", "usr/share/clashcube/firewall.sh"],
            "hooks": "core", "binary": "core", "conffiles": "/etc/config/clashcube\n",
        },
        "clashcube-web": {
            "arch": "aarch64_generic", "depends": "libc, clashcube-core",
            "description": "ClashCube independent management backend and browser assets",
            "files": ["etc/init.d/clashcube", "etc/init.d/clashcube-update", "usr/share/clashcube/package-assets.sh"],
            "hooks": "web", "binary": "web",
        },
        "luci-app-clashcube": {
            "arch": "all", "depends": "clashcube-web, luci-base, luci-compat, uhttpd",
            "description": "ClashCube embedded LuCI management entry",
            "files": ["usr/lib/lua/luci/controller/clashcube.lua", "usr/lib/lua/luci/view/clashcube.htm"],
            "hooks": "luci",
        },
    }
    manifest = {"version": version, "packages": []}
    for name, spec in specs.items():
        files = {}
        for path in spec["files"]:
            mode = 0o755 if "/init.d/" in path or path.endswith(".sh") else 0o644
            files[path] = ((PAYLOAD / path).read_bytes(), mode)
        if "binary" in spec:
            role = spec["binary"]
            files[f"usr/bin/clashcube-{role}"] = ((ROOT / f"bin/openwrt/clashcube-{role}").read_bytes(), 0o755)
        if name == "clashcube-web":
            www = ROOT / "bin/openwrt/www"
            files["usr/share/clashcube/index.html"] = ((www / "index.html").read_bytes(), 0o644)
            assets = sorted(p for p in (www / "assets").rglob("*") if p.is_file())
            for path in assets:
                files["www/luci-static/resources/clashcube/" + path.relative_to(www).as_posix()] = (path.read_bytes(), 0o644)
            files["usr/share/clashcube/assets.packaged"] = (("\n".join(p.relative_to(www).as_posix() for p in assets) + "\n").encode(), 0o644)
            files["usr/share/clashcube/install-assets.sh"] = ((ROOT / "dev/openwrt/install-assets.sh").read_bytes(), 0o755)
        installed = sum(len(data) for data, _ in files.values())
        data_archive = archive(files)
        # OpenWrt Installed-Size is bytes of the uncompressed tar, unlike the
        # KiB field used by Debian. Match scripts/ipkg-build's disk-space check.
        installed_tar = len(gzip.decompress(data_archive))
        control = {
            "control": ((f"Package: {name}\nVersion: {version}\nArchitecture: {spec['arch']}\n"
                         f"Maintainer: ClashCube contributors\nSection: net\nPriority: optional\n"
                         f"Depends: {spec['depends']}\nInstalled-Size: {installed_tar}\n"
                         f"Description: {spec['description']}\n").encode(), 0o644),
            "md5sums": (("".join(f"{hashlib.md5(data).hexdigest()}  {path}\n" for path, (data, _) in sorted(files.items()))).encode(), 0o644),
        }
        if spec.get("conffiles"):
            control["conffiles"] = (spec["conffiles"].encode(), 0o644)
        for hook in ("preinst", "postinst", "prerm", "postrm"):
            source = ROOT / "openwrt/package" / f"{spec['hooks']}.{hook}"
            if source.exists():
                control[hook] = (source.read_bytes(), 0o755)
        # Use the gzip/tar envelope emitted by OpenWrt's own ipkg-build.
        package = archive({"debian-binary": (b"2.0\n", 0o644), "control.tar.gz": (archive(control), 0o644), "data.tar.gz": (data_archive, 0o644)})
        target = destination / f"{name}_{version}_{spec['arch']}.ipk"
        target.write_bytes(package)
        manifest["packages"].append({"name": name, "file": target.name, "bytes": len(package), "installedBytes": installed,
                                      "sha256": hashlib.sha256(package).hexdigest()})
        print(f"{target.name}: {len(package)} bytes download, {installed} bytes installed")
    (destination / "packages.json").write_text(json.dumps(manifest, indent=2) + "\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--skip-build", action="store_true", help="Package existing bin/openwrt artifacts")
    parser.add_argument("--version", default="0.1.0+git." + subprocess.check_output(["git", "rev-parse", "--short", "HEAD"], cwd=ROOT, text=True).strip())
    parser.add_argument("--output", type=pathlib.Path, default=ROOT / "bin/openwrt/packages")
    args = parser.parse_args()
    if not re.fullmatch(r"[0-9][A-Za-z0-9.+~_-]*", args.version):
        parser.error("version must start with a digit and contain only package-version characters")
    if not args.skip_build:
        subprocess.run(["sh", "dev/openwrt/build.sh", "all"], cwd=ROOT, check=True)
    build_packages(args.version, args.output)


if __name__ == "__main__":
    main()
