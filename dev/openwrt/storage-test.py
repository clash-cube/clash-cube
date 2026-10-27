#!/usr/bin/env python3
"""Verify external storage against a real Docker volume, without lab mounts."""
import subprocess
import uuid

from smoke import ROOT


def main():
    image = subprocess.check_output(["docker", "compose", "-f", str(ROOT / "dev/openwrt/compose.yaml"), "images", "-q", "router"], text=True).strip()
    volume = "clashcube-storage-test-" + uuid.uuid4().hex
    subprocess.run(["docker", "volume", "create", volume], check=True, capture_output=True)
    script = """set -e
cat > /etc/config/clashcube <<'EOF'
config clashcube 'main'
 option data_dir '/etc/clashcube-test'
 option storage_mode 'external'
 option external_mount '/mnt/p0-storage'
EOF
. /workspace/clash-cube/openwrt/root/usr/share/clashcube/storage.sh
clashcube_storage
test "$CLASHCUBE_CORE_HOME" = /mnt/p0-storage/clashcube/core
test "$CLASHCUBE_HOME" = /etc/clashcube-test
test "$SAFE_PATHS" = /etc/clashcube-test/core
test -L "$CLASHCUBE_CORE_HOME/tailscale"
test "$(readlink "$CLASHCUBE_CORE_HOME/tailscale")" = /etc/clashcube-test/core/tailscale
grep -q /etc/clashcube-test/profiles/ /lib/upgrade/keep.d/clashcube-data
if grep -qE 'Geo|cache.db' /lib/upgrade/keep.d/clashcube-data; then exit 1; fi
if [ "$1" = write ]; then
 echo persistent > "$CLASHCUBE_CORE_HOME/marker"
else
 test "$(cat "$CLASHCUBE_CORE_HOME/marker")" = persistent
fi
"""
    try:
        for operation in ("write", "read"):
            result = subprocess.run(["docker", "run", "--rm", "-i", "--network", "none", "--platform", "linux/amd64",
                            "--mount", f"type=bind,source={ROOT},target=/workspace/clash-cube,readonly",
                            "--mount", f"type=volume,source={volume},target=/mnt/p0-storage",
                            "--entrypoint", "/bin/sh", image, "-s", "--", operation],
                           input=script, text=True, capture_output=True, timeout=45)
            assert result.returncode == 0, result.stderr
        print("PASS: mounted external volume, configuration/identity separation, custom sysupgrade backup, data survives container replacement")
    finally:
        subprocess.run(["docker", "volume", "rm", volume], check=True, capture_output=True)


if __name__ == "__main__":
    main()
