#!/bin/sh
# Build public, signed ARM64/fw3 artifacts. Publishing is a separate step so
# the channel advances only after its immutable release assets are available.
set -eu
cd "$(dirname "$0")/.."
version=${1:?Usage: sh scripts/release-openwrt.sh VERSION}
printf '%s\n' "$version" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-[1-9][0-9]*$' || {
    echo 'Expected an opkg release version such as 0.2.0-4' >&2
    exit 1
}
tag="openwrt-v$version"
destination="bin/openwrt/release/$tag"
test ! -e "$destination" || {
    echo 'Release output already exists; use a new version or inspect it first.' >&2
    exit 1
}
CLASHCUBE_VERSION="$version" CLASHCUBE_UPX=lzma sh dev/openwrt/build.sh all
python3 openwrt/package.py --skip-build --version "$version" --output "$destination"
go run ./scripts/signrouter \
    -base "https://github.com/clash-cube/clash-cube/releases/download/$tag" \
    -out "$destination/router-update.json" "$destination/packages.json"
python3 - "$destination" <<'PY'
import hashlib
import pathlib
import sys

destination = pathlib.Path(sys.argv[1])
names = sorted(p.name for p in destination.iterdir() if p.suffix in ('.ipk', '.json'))
(destination / 'SHA256SUMS').write_text(''.join(
    f'{hashlib.sha256((destination / name).read_bytes()).hexdigest()}  {name}\n'
    for name in names
))
PY
echo "Release artifacts: $destination"
