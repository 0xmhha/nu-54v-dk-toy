#!/usr/bin/env bash
# Verify fresh Release simulator bundles, including RN startup and the order URL scene callbacks.
set -euo pipefail
if [[ $# -ne 4 ]]; then
  echo 'usage: run.sh SIMULATOR_UUID RENTER_APP KIOSK_APP OUTPUT_DIRECTORY' >&2
  exit 2
fi
sim=$1
renter=$2
kiosk=$3
output=$4
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
if [[ -e "$output/SceneSmoke.xcresult" ]]; then
  echo 'choose a new output directory for the result' >&2
  exit 2
fi
mkdir -p -- "$output"
# Use the explicitly chosen test simulator and preserve any installed application data.
xcrun simctl bootstatus "$sim" -b
xcrun simctl install "$sim" "$renter"
xcrun simctl install "$sim" "$kiosk"
source "$here/private-build.sh"
scene_prepare
scene_run xcodebuild test -project "$here/SceneSmoke.xcodeproj" -scheme SceneSmoke \
  -destination "platform=iOS Simulator,id=$sim" -parallel-testing-enabled NO \
  -only-testing:SceneSmoke/SceneSmoke \
  -derivedDataPath "$scene_work/Derived" -resultBundlePath "$output/SceneSmoke.xcresult"
# A passing invocation with skipped/missing tests cannot satisfy this gate.
xcrun xcresulttool get test-results summary --path "$output/SceneSmoke.xcresult" | python3 -c '
import json, sys
result = json.load(sys.stdin)
assert result["passedTests"] == result["totalTestCount"] == 4, result'
