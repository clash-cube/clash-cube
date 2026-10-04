#!/bin/sh
# Copy bin/ClashCube.app to ~/Applications and open it there. macOS lets an
# app ask for notifications only from an Applications folder; from the repo
# (or /tmp) the request is refused without a prompt.
set -eu
cd "$(dirname "$0")/.."
SRC=bin/ClashCube.app
DEST="$HOME/Applications/ClashCube.app"
EXE="$DEST/Contents/MacOS/clashcube"
# quit the installed copy, or the one in bin/, and wait for it to clear the
# system proxy and stop its core
for app in "$EXE" "$PWD/$SRC/Contents/MacOS/clashcube"; do
  osascript -e 'tell application id "com.localhost-copilot.clashcube" to quit' >/dev/null 2>&1 || true
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    pgrep -fx "$app" >/dev/null || break
    sleep 1
  done
  if pgrep -fx "$app" >/dev/null; then
    echo "Could not quit $app; installation cancelled." >&2
    exit 1
  fi
done
mkdir -p "$HOME/Applications"
rm -rf "$DEST"
ditto "$SRC" "$DEST"
open "$DEST"
echo "installed $DEST"
