#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
sdk="${ANDROID_HOME:-/opt/homebrew/share/android-commandlinetools}"
adb="$sdk/platform-tools/adb"
apk=build/android/app/build/outputs/apk/debug/app-debug.apk
# Read the identity from the APK so applicationId stays in app/build.gradle.
package=$("$sdk/build-tools/35.0.0/aapt" dump badging "$apk" | sed -n "s/^package: name='\([^']*\)'.*/\1/p")
test -n "$package"
device="${ANDROID_SERIAL:-emulator-5554}"
# The emulator uses a direct adb transport; disable host discovery backends
# that crash platform-tools 37 on the macOS 27 development host.
export ADB_MDNS=0 ADB_LIBUSB=0
"$adb" -s "$device" install -r "$apk"
"$adb" -s "$device" shell am start -W -n "$package/com.wails.app.MainActivity"
