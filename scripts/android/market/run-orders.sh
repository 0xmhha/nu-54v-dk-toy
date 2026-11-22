#!/usr/bin/env bash
# Test a signed custom APK without replacing any emulator installation.
set -euo pipefail
umask 077
if [[ $# -lt 2 || $# -gt 3 || ! "$1" =~ ^emulator-[0-9]+$ || ( $# == 3 && "$3" != --home-only ) ]]; then
  echo 'usage: run-orders.sh EMULATOR_SERIAL NEW_EVIDENCE_DIRECTORY [--home-only]' >&2
  exit 2
fi
serial=$1
if [[ ${3:-} == --home-only ]]; then export NU54_ANDROID_MARKET_HOME_ONLY=1; fi
output=$(mkdir -p -- "$2" && cd -- "$2" && pwd)
[[ ! -e "$output/result.log" && ! -e "$output/chain/profile.json" ]]
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
adb="${ANDROID_HOME:?Android SDK is required}/platform-tools/adb"
[[ $("$adb" -s "$serial" shell getprop ro.kernel.qemu | tr -d '\r') == 1 ]]
[[ $("$adb" -s "$serial" reverse --list) != *tcp:9443* ]] || {
  echo 'the dedicated emulator already has a 9443 tunnel' >&2
  exit 2
}
: "${JAVA_HOME:?Java is required}"
exec python3 -B "$here/run_orders.py" "$serial" "$output"
