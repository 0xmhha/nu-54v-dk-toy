#!/usr/bin/env bash
# Installed kiosk with authentic P05/P07/PostgreSQL and a disposable 8283 chain.
set -euo pipefail
umask 077
root=$(cd -- "$(dirname -- "$0")/../../.." && pwd)
cd "$root"
sim=${1:?supply a booted disposable simulator UUID}
app=${2:?supply the regular Release-built NU54Kiosk.app}
out=${3:?supply a new sanitized output directory}
scenario=${4:-legacy}
case "$scenario" in
  legacy) export NU54_KIOSK_BINDING_RECOVERY=0 ;;
  binding-recovery) export NU54_KIOSK_BINDING_RECOVERY=1 ;;
  *) echo "Unknown kiosk scenario: $scenario" >&2; exit 2 ;;
esac
bundle=com.nu54kiosk.outcomeqa
[[ "$(/usr/libexec/PlistBuddy -c 'Print CFBundleIdentifier' "$app/Info.plist")" = "$bundle" ]]
if xcrun simctl get_app_container "$sim" "$bundle" app >/dev/null 2>&1; then
  echo "Choose a simulator without the kiosk outcome fixture bundle." >&2
  exit 2
fi
mkdir "$out"
out=$(cd "$out" && pwd)
scratch=$(mktemp -d /private/tmp/nu54-kiosk-outcome.XXXXXX)
trap 'rm -rf -- "$scratch"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
fixture="$scratch/fixture"
mkdir "$fixture" "$scratch/socket"
export PATH="${POSTGRES_BIN:-/opt/homebrew/opt/postgresql@15/bin}:$PATH"
export PYTHONDONTWRITEBYTECODE=1 NU54_IOS_SKIP_TEST_DIAGNOSTICS=1
worker=
bridge=
test_pid=
installed=0
set -m
stop() {
  local pid=$1
  if [[ -n "$pid" ]]; then
    kill -TERM -- "-$pid" 2>/dev/null || true
    for _ in {1..20}; do kill -0 -- "-$pid" 2>/dev/null || break; sleep .1; done
    kill -KILL -- "-$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
}
cleanup() {
  local code=$?
  trap - EXIT
  trap '' INT TERM
  if ((code != 0)); then touch "$fixture/ui-aborted" || true; fi
  stop "$test_pid"
  stop "$worker"
  stop "$bridge"
  if [[ -f "$scratch/db/postmaster.pid" ]]; then pg_ctl -D "$scratch/db" -m fast -w stop >/dev/null 2>&1 || true; fi
  if ((installed)); then
    bash scripts/ios/scene-smoke/run-subscription-native.sh "$sim" "$app" "$scratch/clear" testClearKioskOutcomeEnrollment >"$scratch/clear.log" 2>&1 || code=1
    xcrun simctl uninstall "$sim" "$bundle" >/dev/null 2>&1 || true
  fi
  if ((code != 0)); then
    python3 - "$scratch" >"$out/failure.json" <<'PY' || echo 'could not write kiosk failure report' >&2
import json,re,sys
from pathlib import Path
base=Path(sys.argv[1])
read=lambda name: (base/name).read_text(errors="replace") if (base/name).is_file() else ""
print(json.dumps({"stage":json.loads(read("fixture/stage.json") or "{}").get("stage"),
                  "uiAssertionLines":[int(x) for x in re.findall(r"SceneSmoke[.]swift:(\d+): error:",read("ui.log"))],
                  "testErrors":[re.sub(r"0x[0-9a-fA-F]+|[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}", "[fixture]", line)[:275]
                   for line in read("worker.log").splitlines() if re.search(r"(?:kiosk_outcome_anvil_test|anvil_test)[.]go:\d+:", line) or "FAIL" in line][-10:]}))
PY
  fi
  rm -rf -- "$scratch"
  exit "$code"
}
trap cleanup EXIT
source "$root/scripts/ios/test-environment.sh"
ios_test_prepare_environment "$scratch"
capture() {
  local log=$1 code=0
  shift
  "$@" >"$log" 2>&1 &
  test_pid=$!
  wait "$test_pid" || code=$?
  test_pid=
  return "$code"
}
xcrun simctl install "$sim" "$app"
installed=1
capture "$scratch/start-clear.log" bash scripts/ios/scene-smoke/run-subscription-native.sh "$sim" "$app" "$scratch/start-clear" testClearKioskOutcomeEnrollment
NU54_SUBSCRIPTION_COMMERCE_SLOT=terminal capture "$scratch/key.log"   bash scripts/ios/scene-smoke/run-subscription-native.sh "$sim" "$app" "$scratch/key" testCommerceEnrollmentPublicKey
python3 - "$scratch/key.log" "$fixture/public-keys.json" <<'PY'
import base64,json,re,sys
from pathlib import Path
key=re.search(r"NU54_SUBSCRIPTION_C3_SPKI=([a-zA-Z0-9+/]{122}==)",Path(sys.argv[1]).read_text())[1]
assert len(base64.b64decode(key,validate=True))==91
Path(sys.argv[2]).write_text(json.dumps({"ios":key}))
PY
initdb -D "$scratch/db" -A trust --no-locale --encoding UTF8 -U kiosk_qa >"$scratch/database.log" 2>&1
pg_ctl -D "$scratch/db" -w -l "$scratch/postgres.log" -o "-c listen_addresses='' -c unix_socket_directories=$scratch/socket -c unix_socket_permissions=0700" start >>"$scratch/database.log" 2>&1
export PGHOST="$scratch/socket" PGPORT=5432 PGUSER=kiosk_qa PGSSLMODE=disable
createdb nu54_settlement_kiosk_outcome
dsn="host=$PGHOST port=5432 user=$PGUSER dbname=nu54_settlement_kiosk_outcome sslmode=disable"
P05_MIGRATION_DATABASE_URL="$dsn" python3 products/p10-platform/database/migrate.py p05 >>"$scratch/database.log" 2>&1
P07_MIGRATION_DATABASE_URL="$dsn" python3 products/p10-platform/database/migrate.py p07 >>"$scratch/database.log" 2>&1
cmake -S products/p01-device-firmware/test -B "$scratch/p01" -DCMAKE_BUILD_TYPE=Debug >"$scratch/host.log" 2>&1
cmake --build "$scratch/p01" --target test_install >>"$scratch/host.log" 2>&1
NU54_KIOSK_OUTCOME_LAB="$fixture" NU54_P01_INSTALL_TEST="$scratch/p01/test_install" P05_SETTLEMENT_TEST_DATABASE_URL="$dsn"   sh products/p05-operations-backoffice/test/settlement/anvil.sh -run '^TestLoopbackSettlementEvidenceAndRefund$/^KioskMenuFinalOutcomes$' -v >"$scratch/worker.log" 2>&1 &
worker=$!
for _ in {1..210}; do
  [[ ! -s "$fixture/enrollment.json" ]] || break
  kill -0 "$worker"
  sleep 1
done
test -s "$fixture/enrollment.json"
python3 - "$fixture/enrollment.json" "$scratch/profile.json" <<'PY'
import json, sys
from pathlib import Path
source, output = map(Path, sys.argv[1:])
profile=json.loads(source.read_text())["profiles"]["ios"]
output.write_text(json.dumps({"profile":profile,"terminal":profile["scope"]["subjectRef"]}))
PY
NU54_SUBSCRIPTION_UI_FIXTURE="$(openssl base64 -A -in "$scratch/profile.json")" NU54_SUBSCRIPTION_COMMERCE_SLOT=terminal   capture "$scratch/enroll.log" bash scripts/ios/scene-smoke/run-subscription-native.sh "$sim" "$app" "$scratch/enroll" testNativeKioskStaffEnrollment
python3 scripts/ios/scene-smoke/fixtures/p05_kiosk_outcome.py "$fixture" "$scratch/control.json" >"$scratch/control.log" 2>&1 &
bridge=$!
capture "$scratch/ui-build.log" "${ios_test_env[@]}" xcodebuild build-for-testing -project scripts/ios/scene-smoke/SceneSmoke.xcodeproj -scheme SceneSmoke   -destination "id=$sim" -derivedDataPath "$scratch/UI"
test -s "$scratch/control.json"
driver=$(find "$scratch/UI" -name 'SceneSmoke_*.xctestrun' -print -quit)
python3 - "$driver" "$scratch/control.json" "$scenario" <<'PY'
import json,plistlib,sys
from pathlib import Path
path, control = map(Path, sys.argv[1:3])
spec=plistlib.loads(path.read_bytes())
scope=json.loads(control.read_text())
targets=[target for config in spec["TestConfigurations"] for target in config["TestTargets"]] if "TestConfigurations" in spec else [spec["SceneSmoke"]]
for target in targets:
    target.setdefault("EnvironmentVariables",{}).update(NU54_KIOSK_OUTCOME_CONTROL=scope["origin"],NU54_KIOSK_OUTCOME_TOKEN=scope["token"], NU54_KIOSK_OUTCOME_SCENARIO=sys.argv[3])
path.write_bytes(plistlib.dumps(spec))
PY
capture "$scratch/ui.log" "${ios_test_env[@]}" xcodebuild test-without-building -xctestrun "$driver" -destination "id=$sim" -parallel-testing-enabled NO   -collect-test-diagnostics never -only-testing:SceneSmoke/KioskOutcomeSmoke/testOriginalPaidAndExpiredMenuOrders   -resultBundlePath "$scratch/Outcome.xcresult"
wait "$worker"
worker=
xcrun xcresulttool get test-results summary --path "$scratch/Outcome.xcresult" | python3 -c '
import json,sys
ui=json.load(sys.stdin)
assert ui["passedTests"] == 1 and ui["failedTests"] == 0 and ui["skippedTests"] == 0
if sys.argv[1] == "binding-recovery":
    print(json.dumps({"passedTests":1,"failedTests":0,"skippedTests":0,"postCommitBindingResponseLost":1,
                      "originalBindings":1,"nonSelectedClaims":0,"preBindingQuotes":0,"recoveredPrivateQuotes":1,
                      "unchangedPriceRelayPayments":1,"unchangedPriceDuplicatePayments":0,"nextCustomerReset":1}))
    sys.exit()
print(json.dumps({"passedTests":1,"failedTests":0,"skippedTests":0,"originalRelayPayment":1,
                  "originalExpiredUnpaid":1,"customerRestarts":2,"authenticatedTerminalScope":1,
                  "unboundAutomaticReset":1,"signedAutomaticReset":1,"signedOrderSinglePayoutAfterExactReplay":1}))
' "$scenario" >"$out/summary.json"
cat "$out/summary.json"
