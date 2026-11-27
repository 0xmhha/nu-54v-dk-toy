#!/usr/bin/env bash
# Starts only after the caller owns and boots a disposable simulator and builds
# the signed app and SceneSmoke test bundle into caller-owned scratch.
set -euo pipefail
umask 077
: "${NU54_W12_SIM:?}"
: "${NU54_W12_XCTESTRUN:?}"
: "${NU54_W12_APP:?}"
: "${NU54_W12_TOKEN:?}"
if [[ $# != 1 || ! -f $NU54_W12_XCTESTRUN ]]; then
  echo 'usage: run-w12-installed.sh LOOPBACK_FIXTURE_URL' >&2
  exit 2
fi
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
source "$here/private-build.sh"
scene_prepare
scene_allocate_spec W12 "$NU54_W12_XCTESTRUN" resolved
spec=$scene_owned_spec
python3 - "$NU54_W12_XCTESTRUN" "$spec" "$1" "$NU54_W12_APP" <<'PY'
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
        ("NU54_W12_CONTROL", "NU54_W12_TOKEN", "NU54_W12_APP"),
        (sys.argv[3], os.environ["NU54_W12_TOKEN"], sys.argv[4]), strict=True
    ))
    for flag in ("NU54_W12_LINKED_PAYMENTS", "NU54_W12_CREDENTIAL", "NU54_W12_SLOT_OPEN"):
        if os.environ.get(flag) == "1":
            test["EnvironmentVariables"][flag] = "1"
with target.open("wb") as output:
    plistlib.dump(spec, output)
PY
scene_run xcodebuild test-without-building -xctestrun "$spec" \
  -destination "platform=iOS Simulator,id=$NU54_W12_SIM" -parallel-testing-enabled NO \
  -only-testing:SceneSmoke/W12RenterStatusSmoke/testEnrolledRenterRestoresOriginalHandoverAndHidesRevokedStatus \
  -resultBundlePath "$scene_work/W12.xcresult" >"$scene_work/w12.log" 2>&1 || {
  status=$?
  sed -nE 's/^.*SceneSmoke\.swift:([0-9]+): error:.*W12RenterStatusSmoke.*$/W12 XCTest failure at line \1/p' "$scene_work/w12.log" >&2
  exit "$status"
}
xcrun xcresulttool get test-results summary --path "$scene_work/W12.xcresult" | python3 -c '
import json, sys
summary = json.load(sys.stdin)
assert summary["passedTests"] == summary["totalTestCount"] == 1, summary
print("W12 installed handover: one passing test")'
