#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

export ANDROID_HOME="${ANDROID_HOME:-/opt/homebrew/share/android-commandlinetools}"
export ANDROID_SDK_ROOT="$ANDROID_HOME"
ndk_bin="$ANDROID_HOME/ndk/29.0.14206865/toolchains/llvm/prebuilt/darwin-x86_64/bin"
# Taskfile.yml exports macOS flags. Never pass them to NDK clang.
unset CGO_CFLAGS CGO_CPPFLAGS CGO_CXXFLAGS CGO_LDFLAGS MACOSX_DEPLOYMENT_TARGET
export GOOS=android CGO_ENABLED=1
core_version=$(sed -n 's/.*Version    = "\([^"]*\)"/\1/p' third_party/mihomo/constant/version.go)
test -n "$core_version"
for abi in arm64-v8a x86_64; do
  case "$abi" in
    arm64-v8a) export GOARCH=arm64 CC="$ndk_bin/aarch64-linux-android29-clang" ;;
    x86_64) export GOARCH=amd64 CC="$ndk_bin/x86_64-linux-android29-clang" ;;
  esac
  output="build/android/app/src/main/jniLibs/$abi"
  mkdir -p "$output"
  go build -tags with_gvisor -ldflags "-X github.com/localhost-copilot/clashcube/internal/gui.bundledCoreVersion=$core_version" -buildmode=c-shared -buildvcs=false -o "$output/libwails.so" .
  go build -tags with_gvisor,cmfa,clashcube -buildmode=c-shared -buildvcs=false -o "$output/libclashcore.so" ./cmd/clashcore
done
cd build/android
./gradlew --console=plain :app:assembleDebug
