#!/usr/bin/env bash
# Pair with NU54_SUBSCRIPTION_NATIVE_LAB=FIXTURE_DIRECTORY make commercial-integration-test.
set -euo pipefail
if [[ $# -ne 4 ]]; then
  echo 'usage: run-subscription-enrolled.sh SIMULATOR_UUID SIGNED_RENTER_APP FIXTURE_DIRECTORY RESULT_DIRECTORY' >&2
  exit 2
fi
sim=$1
app=$2
fixture=$3
out=$4
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
mkdir -p -- "$out"
out=$(cd -- "$out" && pwd)
if [[ -e "$out/Enrolled.xcresult" || -e "$fixture/ui-completed" ]]; then
  echo 'use a fresh UI result directory and active native fixture' >&2
  exit 2
fi
source "$here/private-build.sh"
scene_prepare
scene_run xcodebuild build-for-testing -project "$here/SceneSmoke.xcodeproj" -scheme SceneSmoke   -destination "platform=iOS Simulator,id=$sim" -derivedDataPath "$scene_work/Derived"
for ((i=0; i<180; i++)); do
  [[ -s "$fixture/preview.json" ]] && break
  sleep 1
done
export NU54_SUBSCRIPTION_UI_FIXTURE
NU54_SUBSCRIPTION_UI_FIXTURE=$(python3 - "$fixture/preview.json" <<'PY'
import base64, pathlib, sys
print(base64.b64encode(pathlib.Path(sys.argv[1]).read_bytes()).decode())
PY
)
scene_run env TMPDIR="${scene_work%/*}" "$here/run-subscription-native.sh" "$sim" "$app" "$out/Native" testEnrollNativeCustomerAndQueueActualInvoice
spec=$(find "$scene_work/Derived" -name 'SceneSmoke_*.xctestrun' -print -quit)
enrolled=$(dirname -- "$spec")/SubscriptionEnrolled.xctestrun
python3 - "$spec" "$enrolled" "$fixture/preview.json" <<'PY'
import json, pathlib, plistlib, sys
source, target, fixture = map(pathlib.Path, sys.argv[1:])
spec = plistlib.loads(source.read_bytes())
if "TestConfigurations" in spec:
    targets = [entry for config in spec["TestConfigurations"] for entry in config["TestTargets"]]
else:
    targets = [entry for name, entry in spec.items() if not name.startswith("__")]
assert targets
for entry in targets:
    entry.setdefault("EnvironmentVariables", {})["NU54_SUBSCRIPTION_SALE"] = json.loads(fixture.read_bytes())["invoice"]["saleRef"]
target.write_bytes(plistlib.dumps(spec))
PY
scene_run xcodebuild test-without-building -xctestrun "$enrolled"   -destination "platform=iOS Simulator,id=$sim" -parallel-testing-enabled NO   -only-testing:SceneSmoke/SubscriptionEnrolledSmoke/testLocalNotificationOpensTheExactAuthenticatedUnpaidInvoice   -resultBundlePath "$out/Enrolled.xcresult"
xcrun xcresulttool get test-results summary --path "$out/Enrolled.xcresult" | python3 -c '
import json, sys
result = json.load(sys.stdin)
assert result["passedTests"] == result["totalTestCount"] == 1, result'
touch "$fixture/ui-completed"
