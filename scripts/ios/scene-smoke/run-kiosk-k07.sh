#!/usr/bin/env bash
# Exercise the published menu in either fresh Release simulator app over real mTLS.
set -euo pipefail
umask 077
root=$(cd -- "$(dirname -- "$0")/../../.." && pwd)
cd "$root"
sim=${1:?supply a disposable simulator UUID}
app=${2:?supply a fresh signed Release kiosk or customer application}
output=${3:?supply a new evidence directory}
bundle=$(/usr/libexec/PlistBuddy -c 'Print CFBundleIdentifier' "$app/Info.plist")
case "${4:-kiosk}" in
  kiosk)
    [[ "$bundle" == com.nu54kiosk.k07.* ]]
    target=KioskK07Smoke
    auxiliary=customer
    result=KioskK07.xcresult
    ;;
  customer-enrollment)
    [[ "$bundle" == com.nu54renter.enrollqa.* ]]
    target=CustomerEnrollmentSmoke
    auxiliary=terminal
    result=CustomerEnrollment.xcresult
    ;;
  *) echo "choose kiosk or customer-enrollment" >&2; exit 2 ;;
esac
if xcrun simctl get_app_container "$sim" "$bundle" app >/dev/null 2>&1; then
  echo "the installed menu test requires a fresh simulator app bundle" >&2
  exit 2
fi
mkdir "$output"
out=$(cd "$output" && pwd)
scratch=$(mktemp -d /private/tmp/nu54-k07-ui.XXXXXX)
trap 'rm -rf -- "$scratch"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
fixture_pid=
test_pid=
installed=0
cleanup() {
  for pid in "$test_pid" "$fixture_pid"; do
    if [[ -n "$pid" ]]; then
      kill -TERM "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
  if ((installed)); then xcrun simctl uninstall "$sim" "$bundle" >/dev/null 2>&1 || true; fi
  rm -rf -- "$scratch"
}
source "$root/scripts/ios/test-environment.sh"
ios_test_prepare_environment "$scratch"
trap cleanup EXIT

export POSTGRES_BIN=${POSTGRES_BIN:-/opt/homebrew/opt/postgresql@15/bin}
export PYTHONPATH="$root/scripts${PYTHONPATH:+:$PYTHONPATH}"
export PYTHONDONTWRITEBYTECODE=1
xcrun simctl install "$sim" "$app"
installed=1
python3 -u scripts/ios/scene-smoke/fixtures/p05_pickup_native.py \
  --k07-menu --ready-file "$scratch/fixture.json" > "$out/fixture.log" 2>&1 &
fixture_pid=$!
"${ios_test_env[@]}" xcodebuild build-for-testing -project scripts/ios/scene-smoke/SceneSmoke.xcodeproj -scheme SceneSmoke \
  -destination "id=$sim" -derivedDataPath "$scratch/UI" -parallel-testing-enabled NO > "$out/ui-build.log" 2>&1 &
test_pid=$!
for _ in {1..90}; do
  [[ ! -s "$scratch/fixture.json" ]] || break
  kill -0 "$fixture_pid"
  sleep 1
done
test -s "$scratch/fixture.json"
wait "$test_pid"
test_pid=
driver=$(find "$scratch/UI" -name 'SceneSmoke_*.xctestrun' -print -quit)
python3 - "$driver" "$scratch/fixture.json" "$bundle" "$scratch" "$auxiliary" <<'PY'
import base64
import json
from pathlib import Path
import plistlib
import subprocess
import sys
from urllib.request import Request, urlopen

driver, source, bundle, scratch, auxiliary = sys.argv[1:]
fixture = json.loads(Path(source).read_text())
key = Path(scratch) / "auxiliary.pem"
subprocess.run(["openssl", "genpkey", "-algorithm", "EC", "-pkeyopt", "ec_paramgen_curve:P-256", "-out", key], check=True)
public = subprocess.check_output(["openssl", "pkey", "-in", key, "-pubout", "-outform", "DER"])
request = Request(fixture["origin"] + "/enroll/" + auxiliary, data=base64.b64encode(public),
                  headers={"Authorization": "Bearer " + fixture["access"]})
with urlopen(request, timeout=10):
    pass

path = Path(driver)
manifest = plistlib.loads(path.read_bytes())
if "TestConfigurations" in manifest:
    targets = [target for config in manifest["TestConfigurations"] for target in config["TestTargets"]]
else:
    targets = [target for name, target in manifest.items() if not name.startswith("__")]
for target in targets:
    target.setdefault("EnvironmentVariables", {}).update(
        NU54_K07_CONTROL=fixture["origin"], NU54_K07_TOKEN=fixture["access"], NU54_K07_APP=bundle,
    )
path.write_bytes(plistlib.dumps(manifest))
PY
"${ios_test_env[@]}" xcodebuild test-without-building -xctestrun "$driver" -destination "id=$sim" \
  -parallel-testing-enabled NO -collect-test-diagnostics never -only-testing:"SceneSmoke/$target" \
  -resultBundlePath "$out/$result" > "$out/native.log" 2>&1 &
test_pid=$!
code=0
wait "$test_pid" || code=$?
test_pid=
tail -25 "$out/native.log"
((code == 0)) || exit "$code"
xcrun xcresulttool get test-results summary --path "$out/$result" > "$out/summary.json"
python3 - "$out/summary.json" <<'PY'
import json
import sys
from pathlib import Path
summary = json.loads(Path(sys.argv[1]).read_text())
assert summary["passedTests"] == summary["totalTestCount"] == 1, summary
PY

if [[ "$target" == CustomerEnrollmentSmoke && "${NU54_K07_VERIFY_PRIOR_ALERTS:-}" == 1 ]]; then
  export NU54_SUBSCRIPTION_PRIOR_ORDER
  NU54_SUBSCRIPTION_PRIOR_ORDER=$(python3 - "$scratch/fixture.json" <<'PYORDER'
import json
from pathlib import Path
import sys
from urllib.request import Request, urlopen

fixture = json.loads(Path(sys.argv[1]).read_text())
request = Request(fixture["origin"] + "/k07-proof", method="POST",
                  headers={"Authorization": "Bearer " + fixture["access"]})
with urlopen(request, timeout=10) as response:
    row = json.load(response)
assert row["orders"] == 1
print(row["orderRef"])
PYORDER
)
  "$root/scripts/ios/scene-smoke/run-subscription-native.sh" "$sim" "$app" "$out/prior-alerts" \
    testRealPriorCustomerAlertsClearWithoutChangingProfile > "$out/prior-alerts.log" 2>&1
  xcrun xcresulttool get test-results summary --path "$out/prior-alerts/Result.xcresult" > "$out/prior-alerts-summary.json"
fi
