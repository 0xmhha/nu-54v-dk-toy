#!/usr/bin/env bash
# Start the disposable P08 mobile fixture before running this gate.
set -euo pipefail
cd "$(dirname "$0")"
adb_bin="${ANDROID_HOME:?Set ANDROID_HOME}/platform-tools/adb"
serial="${1:?Pass the serial of an isolated Android emulator}"
[[ "$serial" =~ ^emulator-[0-9]+$ ]]
log=$(mktemp)
cleanup() {
  "$adb_bin" -s "$serial" reverse --remove tcp:8284 >/dev/null 2>&1 || true
  "$adb_bin" -s "$serial" reverse --remove tcp:8285 >/dev/null 2>&1 || true
  rm -f "$log"
}
trap cleanup EXIT
curl -fsS --max-time 3 http://127.0.0.1:8284/v1/profile >/dev/null
./gradlew --no-daemon "${@:2}" -PreactNativeArchitectures=arm64-v8a \
  -Pnu54MarketProfile=loopback -Pnu54LabRunner=com.nu54renter.ble.MarketLabNative :app:assembleDebug :app:assembleDebugAndroidTest
"$adb_bin" -s "$serial" install -r app/build/outputs/apk/debug/app-debug.apk
"$adb_bin" -s "$serial" install -r app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk
"$adb_bin" -s "$serial" reverse tcp:8284 tcp:8284
"$adb_bin" -s "$serial" reverse tcp:8285 tcp:8285
"$adb_bin" -s "$serial" shell am instrument -w \
  com.nu54renter.test/com.nu54renter.ble.MarketLabNative | tee "$log"
grep -q '^INSTRUMENTATION_RESULT: market_lab=pass' "$log"
grep -q '^INSTRUMENTATION_RESULT: native_guard=wrong-scope-tampering-target-route-and-future-head-rejected' "$log"
