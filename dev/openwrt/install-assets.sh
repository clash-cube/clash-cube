#!/bin/sh
# Keep current and previous deployment assets so open tabs can load lazy chunks.
set -eu
source=$1
destination=$2
state=$3
test -f "$source/index.html"
test -d "$source/assets"
mkdir -p "$destination/assets" "$state"
source=$(cd "$source" && pwd)
destination=$(cd "$destination" && pwd)
state=$(cd "$state" && pwd)
(cd "$source" && find assets -type f | sort) > "$state/assets.next"
if [ -f "$state/assets.current" ]; then
    cp "$state/assets.current" "$state/assets.previous"
else
    # Adopt an existing installation without breaking its open tabs. Older
    # untracked assets expire on the following successful deployment.
    (cd "$destination" && find assets -type f | sort) > "$state/assets.previous"
fi
cp -R "$source/assets/." "$destination/assets/"
cp "$source/index.html" "$state/index.html.new"
mv "$state/index.html.new" "$state/index.html"
mv "$state/assets.next" "$state/assets.current"
cd "$destination"
find assets -type f | while IFS= read -r asset; do
    if ! grep -Fqx "$asset" "$state/assets.current" && ! grep -Fqx "$asset" "$state/assets.previous"; then
        rm -f "$asset"
    fi
done
