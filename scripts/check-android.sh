#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
sdk="${ANDROID_HOME:-/opt/homebrew/share/android-commandlinetools}"
unset CGO_CFLAGS CGO_CPPFLAGS CGO_CXXFLAGS CGO_LDFLAGS MACOSX_DEPLOYMENT_TARGET
export GOOS=android GOARCH=arm64 CGO_ENABLED=1
export CC="$sdk/ndk/29.0.14206865/toolchains/llvm/prebuilt/darwin-x86_64/bin/aarch64-linux-android29-clang"

packages=$(go list -deps -tags with_gvisor -f '{{if .Module}}{{if eq .Module.Path "github.com/localhost-copilot/clashcube"}}{{.ImportPath}}{{end}}{{end}}' .)
while IFS= read -r package; do
  case "${package##*/}" in
    core|helper|sysproxy|autostart|hotkeys|winutil|appupdate|updatesig)
      echo "Desktop/core package leaked into Android UI: $package" >&2
      exit 1 ;;
  esac
  directory=$(go list -tags with_gvisor -f '{{.Dir}}' "$package")
  files=$(go list -tags with_gvisor -f '{{join .GoFiles " "}} {{join .CgoFiles " "}}' "$package")
  # Vet the selected production files: desktop tests intentionally re-exec
  # their binary as a core, which is not an Android execution model.
  (cd "$directory" && go vet -tags with_gvisor $files)
  echo "vetted $package"
done <<< "$packages"

# The separate library deliberately owns the core dependency excluded above.
go vet -tags with_gvisor,cmfa,clashcube ./cmd/clashcore
