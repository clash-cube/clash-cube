#!/bin/sh
# Verify the bundle before giving a release artifact its versioned name.
set -eu
cd "$(dirname "$0")/.."
version="${1:?release version required}"
printf '%s\n' "$version" | grep -Eq '^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' || {
  echo 'Expected a version such as v0.1.0' >&2
  exit 1
}
app=bin/ClashCube.app
exe="$app/Contents/MacOS/clashcube"
codesign --verify --deep --strict "$app"
go run ./scripts/signhelper -check "$exe" "$app/Contents/Resources/helper.sig"
arch=$(lipo -archs "$exe")
case "$arch" in
  arm64) ;;
  x86_64) arch=amd64 ;;
  *) echo "Unsupported release architecture: $arch" >&2; exit 1 ;;
esac
plutil -lint "$app/Contents/Info.plist"
test "$(/usr/libexec/PlistBuddy -c 'Print CFBundleShortVersionString' "$app/Contents/Info.plist")" = "${version#v}"
# Exercise the production binary without starting a proxy or using real profiles.
check_home=$(mktemp -d)
trap 'rm -rf "$check_home"' EXIT
printf 'mode: direct\nrules: []\n' > "$check_home/config.yaml"
"$exe" core -t -d "$check_home" -f "$check_home/config.yaml"
hdiutil verify bin/ClashCube.dmg
mkdir -p bin/release
cp bin/ClashCube.dmg "bin/release/ClashCube-${version#v}-macos-$arch.dmg"
# The app updates itself from this zip (internal/appupdate); ditto keeps
# the bundle's code seal.
rm -f "bin/release/ClashCube-${version#v}-macos-$arch.zip"
ditto -c -k --keepParent "$app" "bin/release/ClashCube-${version#v}-macos-$arch.zip"
