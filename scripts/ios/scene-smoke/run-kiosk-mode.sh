#!/usr/bin/env bash
# Run the K06 installed-app scenario against an already running disposable loopback fixture.
set -euo pipefail
if [[ $# != 3 ]]; then
  echo 'usage: run-kiosk-mode.sh OPERATOR_SIMULATOR TERMINAL_SIMULATOR OUTPUT_DIRECTORY' >&2
  exit 2
fi
operator=$1
terminal=$2
if [[ "$operator" = "$terminal" ]]; then
  echo 'the enrolled operator and terminal must use separate test simulators' >&2
  exit 2
fi
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
mkdir -p "$3"
out=$(cd -- "$3" && pwd)
[[ "$(curl -fsS http://127.0.0.1:18966/state)" = 'menu|1|0|0|0|0|21' ]]
source "$here/private-build.sh"
scene_prepare
scene_run xcodebuild build-for-testing -project "$here/SceneSmoke.xcodeproj" -scheme SceneSmoke \
  -destination "platform=iOS Simulator,id=$operator" -derivedDataPath "$scene_work/Derived" -parallel-testing-enabled NO
run=$(find "$scene_work/Derived/Build/Products" -maxdepth 1 -name '*.xctestrun' -print -quit)
run_test() {
  local simulator=$1 method=$2
  scene_run xcodebuild test-without-building -xctestrun "$run" -destination "platform=iOS Simulator,id=$simulator" \
    -parallel-testing-enabled NO -collect-test-diagnostics never \
    -only-testing:"SceneSmoke/KioskModeReviewSmoke/$method" -resultBundlePath "$out/$method.xcresult"
  xcrun xcresulttool get test-results summary --path "$out/$method.xcresult" | python3 -c '
import json, sys
result = json.load(sys.stdin)
assert result["passedTests"] == result["totalTestCount"] == 1, result'
}
run_test "$operator" testOperatorReviewResumeAndHandoff
run_test "$terminal" testTerminalAppliesExactVersion
run_test "$operator" testOperatorRecoversActiveOriginalRequest
[[ "$(curl -fsS http://127.0.0.1:18966/state)" = 'pos|2|1|1|1|21|21' ]]
