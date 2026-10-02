#!/bin/sh
# replace directives are not inherited by dependents: copy third_party/mihomo's
# into our go.mod, between the BEGIN/END markers.
set -eu
cd "$(dirname "$0")/.."
tmp=$(mktemp)
{
  sed -n '1,/^\/\/ BEGIN mihomo replaces$/p' go.mod
  grep '^replace ' third_party/mihomo/go.mod | sed 's/$/\n/' | sed '$d'
  sed -n '/^\/\/ END mihomo replaces$/,$p' go.mod
} > "$tmp"
mv "$tmp" go.mod
go mod tidy
echo "synced: $(grep -c '^replace ' third_party/mihomo/go.mod) replaces"
