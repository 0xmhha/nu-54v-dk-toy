#!/usr/bin/env bash
set -euo pipefail
umask 077
: "${NU54_K10_SIM:?}" "${NU54_K10_XCTESTRUN:?}" "${NU54_K10_APP:?}" "${NU54_K10_TOKEN:?}"
if [[ $# != 1 ]]; then echo 'usage: run-k10-installed.sh LOOPBACK_FIXTURE_URL' >&2; exit 2; fi
if [[ ${NU54_W10_INSTALLED_BOARD:-} == 1 && ${NU54_K10_SIGNED_REVIEW:-} != 1 ]]; then
  echo 'The sponsored board fixture requires NU54_K10_SIGNED_REVIEW=1' >&2
  exit 2
fi
if [[ ${NU54_W10_INSTALLED_SETTLEMENT:-} == 1 && ${NU54_W10_INSTALLED_BOARD:-} != 1 ]]; then
  echo 'The installed settlement fixture requires NU54_W10_INSTALLED_BOARD=1' >&2
  exit 2
fi
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
source "$here/private-build.sh"
scene_prepare
scene_allocate_spec K10 "$NU54_K10_XCTESTRUN"
spec=$scene_owned_spec
python3 - "$NU54_K10_XCTESTRUN" "$spec" "$1" "$NU54_K10_APP" <<'PY'
import os, pathlib, plistlib, sys
source, target = map(pathlib.Path, sys.argv[1:3])
spec = plistlib.loads(source.read_bytes())
if "TestConfigurations" in spec:
    tests = [test for entry in spec["TestConfigurations"] for test in entry["TestTargets"]]
else:
    tests = [entry for name, entry in spec.items() if not name.startswith("__")]
assert tests
for test in tests:
    test.setdefault("EnvironmentVariables", {}).update(zip(
        ("NU54_K10_CONTROL", "NU54_K10_TOKEN", "NU54_K10_APP"),
        (sys.argv[3], os.environ["NU54_K10_TOKEN"], sys.argv[4]), strict=True
    ))
with target.open("wb") as output:
    plistlib.dump(spec, output)
PY
method=testOriginalDenialSurvivesRelaunch
result_label='K10 installed virtual-nearby'
if [[ ${NU54_K10_SIGNED_REVIEW:-} == 1 ]]; then
  method=testLostSignedQuoteReopensOriginalReview
  result_label='W10 installed signed virtual-nearby'
fi
if [[ ${NU54_W10_INSTALLED_BOARD:-} == 1 ]]; then
  method=testSponsoredReviewAndBoardRefusal
  result_label='W10 installed sponsored C board refusal'
fi
if [[ ${NU54_W10_INSTALLED_SETTLEMENT:-} == 1 ]]; then
  method=testSponsoredBoardApprovalAndSettlement
  result_label='W10 installed sponsored C board settlement'
fi
if [[ ${NU54_W11_INSTALLED_PICKUP:-} == 1 ]]; then
  [[ ${NU54_W10_INSTALLED_SETTLEMENT:-} == 1 ]] || exit 2
  method=testSponsoredPickupReceipt
  result_label='W11 installed original nearby pickup receipt'
fi
if [[ ${NU54_W11_INSTALLED_PENDING:-} == 1 ]]; then
  [[ ${NU54_W10_INSTALLED_SETTLEMENT:-} == 1 && ${NU54_W11_INSTALLED_PICKUP:-} != 1 ]] || exit 2
  method=testSponsoredPendingReceipt
  result_label='W11 installed original nearby pending settlement'
fi
if [[ ${NU54_W11_INSTALLED_UNPAID:-} == 1 ]]; then
  [[ ${NU54_W10_INSTALLED_SETTLEMENT:-} == 1 && ${NU54_W11_INSTALLED_PICKUP:-} != 1 && ${NU54_W11_INSTALLED_PENDING:-} != 1 ]] || exit 2
  method=testSponsoredUnpaidReceipt
  result_label='W11 installed original nearby unpaid settlement'
fi
if [[ ${NU54_W12_INSTALLED_BUSY:-} == 1 ]]; then
  [[ ${NU54_W10_INSTALLED_BOARD:-} == 1 && ${NU54_W10_INSTALLED_SETTLEMENT:-} != 1 ]] || exit 2
  method=testSponsoredBoardApprovalMaintenanceHold
  result_label='W12 installed authenticated approval holds maintenance'
fi
scene_run xcodebuild test-without-building -xctestrun "$spec" \
  -destination "platform=iOS Simulator,id=$NU54_K10_SIM" -parallel-testing-enabled NO \
  -collect-test-diagnostics never -only-testing:"SceneSmoke/K10NearbySmoke/$method" \
  -resultBundlePath "$scene_work/K10.xcresult"
xcrun xcresulttool get test-results summary --path "$scene_work/K10.xcresult" | python3 -c '
import json, sys
summary = json.load(sys.stdin)
assert summary["passedTests"] == summary["totalTestCount"] == 1, summary
print(sys.argv[1] + ": one passing test")' "$result_label"
