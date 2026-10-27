#!/bin/sh
# opkg removes old owned hashes before postinst. Recover the previous assets
# from RAM and use the same bounded retention as development deployments.
set -eu
root=/usr/share/clashcube
public=/www/luci-static/resources/clashcube
backup=/tmp/clashcube-assets-backup
stage=$(mktemp -d /tmp/clashcube-assets-new.XXXXXX)
trap 'rm -rf "$stage" "$backup"' EXIT
mkdir -p "$stage/assets" "$public/assets"
cp "$root/index.html" "$stage/index.html"
while IFS= read -r asset; do
    mkdir -p "$stage/$(dirname "$asset")"
    cp "$public/$asset" "$stage/$asset"
done < "$root/assets.packaged"
if [ -d "$backup/assets" ]; then cp -Rn "$backup/assets/." "$public/assets/"; fi
sh "$root/install-assets.sh" "$stage" "$public" "$root"
