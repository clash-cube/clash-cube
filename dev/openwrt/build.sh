#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
mode=${1:-all}
case "$mode" in management|core|all) ;; *) echo 'Usage: build.sh [management|core|all]' >&2; exit 2;; esac
packing=${CLASHCUBE_UPX:-lzma}
case "$packing" in none|nrv|lzma) ;; *) echo 'CLASHCUBE_UPX must be none, nrv or lzma' >&2; exit 2;; esac
if [ "$packing" != none ] && ! command -v upx >/dev/null 2>&1; then
    echo 'UPX is required for compression; install UPX or set CLASHCUBE_UPX=none' >&2
    exit 1
fi
pack_binary() {
    case "$packing" in
        nrv) upx --no-lzma -9 "$1"; upx -t "$1" ;;
        lzma) upx --lzma -9 "$1"; upx -t "$1" ;;
    esac
    echo "$(basename "$1"): $(wc -c < "$1" | tr -d ' ') bytes ($packing)"
}
if [ "$mode" != management ]; then
    core_version="alpha-$(git -C third_party/mihomo rev-parse --short HEAD)"
    # UPX preserves Go build IDs; remove packed output before asking Go to build.
    rm -f bin/openwrt/clashcube-core
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags with_gvisor -trimpath \
        -ldflags="-s -w -X github.com/metacubex/mihomo/constant.Version=$core_version" \
        -o bin/openwrt/clashcube-core ./cmd/clashcube-core
    pack_binary bin/openwrt/clashcube-core
fi
if [ "$mode" != core ]; then
    version=${CLASHCUBE_VERSION:-$(git describe --always --dirty)}
    rm -f bin/openwrt/clashcube-web
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath \
        -ldflags="-s -w -X main.version=$version" \
        -o bin/openwrt/clashcube-web ./cmd/clashcube-web
    pack_binary bin/openwrt/clashcube-web
    (cd frontend && npm run build:web)
fi
