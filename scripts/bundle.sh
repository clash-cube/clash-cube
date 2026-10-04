#!/bin/sh
# Assemble bin/ClashCube.app around bin/clashcube and sign it ad hoc.
set -eu
cd "$(dirname "$0")/.."
VERSION="${1:-dev}"
APP=bin/ClashCube.app
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp bin/clashcube "$APP/Contents/MacOS/clashcube"
sed "s/@VERSION@/${VERSION#v}/g" build/darwin/Info.plist > "$APP/Contents/Info.plist"
if [ ! -f build/darwin/icons.icns ]; then
  set=$(mktemp -d)/AppIcon.iconset
  mkdir -p "$set"
  for s in 16 32 128 256 512; do
    sips -z $s $s build/appicon.png --out "$set/icon_${s}x${s}.png" >/dev/null
    sips -z $((s*2)) $((s*2)) build/appicon.png --out "$set/icon_${s}x${s}@2x.png" >/dev/null
  done
  iconutil -c icns "$set" -o build/darwin/icons.icns
fi
cp build/darwin/icons.icns "$APP/Contents/Resources/icons.icns"
codesign --force --deep --sign - "$APP"
echo "built $APP"
