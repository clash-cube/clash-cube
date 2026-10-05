#!/bin/sh
# Assemble bin/ClashCube.app around bin/clashcube and sign it ad hoc.
set -eu
cd "$(dirname "$0")/.."
VERSION="${1:-dev}"
STAMP="${2:-}"
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
# Intel linkers may leave the executable unsigned. The helper hash needs
# a code-signature region before it can ignore later bundle re-signing.
codesign --force --sign - "$APP/Contents/MacOS/clashcube"
# Sign the executable for the helper's passwordless self-update when the
# update key is here (internal/updatesig); without it the helper is updated
# by reinstalling, with a password.
KEY="${CLASHCUBE_UPDATE_KEY:-$HOME/.config/clashcube/update.key}"
if [ -n "$STAMP" ] && [ -f "$KEY" ]; then
  go run ./scripts/signhelper -key "$KEY" -stamp "$STAMP" "$APP/Contents/MacOS/clashcube" > "$APP/Contents/Resources/helper.sig"
fi
codesign --force --deep --sign - "$APP"
if [ -f "$APP/Contents/Resources/helper.sig" ]; then
  go run ./scripts/signhelper -check "$APP/Contents/MacOS/clashcube" "$APP/Contents/Resources/helper.sig" >/dev/null
fi
echo "built $APP"
