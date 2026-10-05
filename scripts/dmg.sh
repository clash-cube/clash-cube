#!/bin/sh
# Write Finder metadata directly so CI does not need Finder automation.
set -eu
cd "$(dirname "$0")/.."
tools=bin/dmg-tools
if [ ! -x "$tools/bin/python" ]; then
  python3 -m venv "$tools"
fi
"$tools/bin/python" -m pip install --disable-pip-version-check -r build/darwin/dmg-requirements.txt
# Keep the previous image intact if packaging fails.
staging=$(mktemp -d "$PWD/bin/dmg.XXXXXX")
trap 'rm -rf "$staging"' EXIT
swift build/darwin/dmg-background.swift "$staging/background.tiff"
"$tools/bin/python" -m dmgbuild -s build/darwin/dmg-settings.py \
  -D background="$staging/background.tiff" ClashCube "$staging/ClashCube.dmg"
hdiutil verify "$staging/ClashCube.dmg"
mv "$staging/ClashCube.dmg" bin/ClashCube.dmg
