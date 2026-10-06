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
# Finder aliases can resolve a background on another mounted image with the
# same volume name. Give each release and architecture its own volume name.
version=$(/usr/libexec/PlistBuddy -c 'Print CFBundleShortVersionString' bin/ClashCube.app/Contents/Info.plist)
arch=$(lipo -archs bin/ClashCube.app/Contents/MacOS/clashcube)
volume="ClashCube $version $arch"
"$tools/bin/python" -m dmgbuild -s build/darwin/dmg-settings.py \
  -D background="$staging/background.tiff" "$volume" "$staging/ClashCube.dmg"
hdiutil verify "$staging/ClashCube.dmg"
mv "$staging/ClashCube.dmg" bin/ClashCube.dmg
