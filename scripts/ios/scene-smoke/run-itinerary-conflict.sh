#!/usr/bin/env bash
set -euo pipefail
umask 077
if [[ ( $# -ne 4 && $# -ne 5 ) || ! "$4" =~ ^[0-9]{4,5}$ ]]; then
  echo 'usage: run-itinerary-conflict.sh SIMULATOR_UUID DERIVED_DATA RESULT FIRST_LOOPBACK_PORT [--recommendation|--my-trip]' >&2
  exit 2
fi
fixture_mode=--conflict
scenario=testReviewedCandidateAfterConcurrentVersionChange
expected_db='1|3'
if [[ ${5:-} == --recommendation ]]; then
  fixture_mode=--recommendation
  scenario=testPreConsentDraftCancelRetryAndSave
  expected_db='1|1'
elif [[ ${5:-} == --my-trip ]]; then
  fixture_mode=--recommendation
  scenario=testRegisteredMyTripDestinationsRemainReadOnly
elif [[ $# -eq 5 ]]; then
  exit 2
fi
export POSTGRES_BIN=${POSTGRES_BIN:-/opt/homebrew/opt/postgresql@15/bin}
sim=$1
derived=$2
result=$3
catalog_port=$4
record_port=$((catalog_port + 1))
enroll_port=$((catalog_port + 2))
[[ "$enroll_port" -le 65535 ]]
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../../.." && pwd)
cd "$root"
mkdir -- "$result"
result=$(cd -- "$result" && pwd)
scratch=$(mktemp -d "${TMPDIR:-/private/tmp}/nu54-t13-route.XXXXXX")
trap 'rm -rf -- "$scratch"' EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
bundle="com.nu54renter.travel.conflict-$(uuidgen | tr '[:upper:]' '[:lower:]' | tr -d '-' | cut -c1-12)"
grant=$(python3 -c 'import secrets; print(secrets.token_urlsafe(32))')
set -m
catalog=
private=
command=
stop() {
  local pid=$1
  [[ -n "$pid" ]] || return 0
  kill -TERM -- "-$pid" 2>/dev/null || true
  for ((i=0; i<30; i++)); do
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.1
  done
  kill -KILL -- "-$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
}
cleanup() {
  local code=$?
  trap - EXIT
  trap '' HUP INT TERM
  stop "$command"
  if [[ -n "$private" ]]; then
    # The fixture owns detached PostgreSQL and bounded child teardown. Do not
    # kill its supervisor while it is checking whether the database stopped.
    kill -TERM -- "-$private" 2>/dev/null || true
    if wait "$private"; then
      :
    else
      local fixture_status=$?
      grep -E 'Retained integration workspace at (/private)?/tmp/nu54-private-ios-[A-Za-z0-9_-]+$' "$scratch/private.log" >&2 || true
      if ((code == 0)); then code=$fixture_status; fi
    fi
  fi
  stop "$catalog"
  rm -rf -- "$scratch"
  xcrun simctl uninstall "$sim" "$bundle" >/dev/null 2>&1 || true
  if ((code != 0)); then echo "T13 itinerary acceptance ended (status $code)" >&2; fi
  exit "$code"
}
source "$root/scripts/ios/test-environment.sh"
ios_test_prepare_environment "$scratch"
trap cleanup EXIT
defer_child_start() {
  pending_interrupt=
  trap 'pending_interrupt=${pending_interrupt:-129}' HUP
  trap 'pending_interrupt=${pending_interrupt:-130}' INT
  trap 'pending_interrupt=${pending_interrupt:-143}' TERM
}
finish_child_start() {
  trap 'exit "${pending_interrupt:-129}"' HUP
  trap 'exit "${pending_interrupt:-130}"' INT
  trap 'exit "${pending_interrupt:-143}"' TERM
  if [[ -n "$pending_interrupt" ]]; then exit "$pending_interrupt"; fi
}
capture() {
  local log=$1 code=0
  shift
  defer_child_start
  "$@" >"$log" 2>&1 &
  command=$!
  finish_child_start
  wait "$command" || code=$?
  command=
  return "$code"
}
export NU54_P09_PUBLIC_ORIGIN="https://127.0.0.1:$catalog_port"
runner=scripts/ios/scene-smoke
capture "$scratch/app.log" "${ios_test_env[@]}" xcodebuild -workspace products/p02-user-app/ios/NU54Renter.xcworkspace   -scheme NU54Renter -configuration Release -destination 'generic/platform=iOS Simulator' -derivedDataPath "$derived"   -jobs 2 ARCHS=arm64 CODE_SIGNING_ALLOWED=YES CODE_SIGN_IDENTITY=- NU54_IOS_BUNDLE="$bundle" build
app="$derived/Build/Products/Release-iphonesimulator/NU54Renter.app"
if [[ $scenario == testRegisteredMyTripDestinationsRemainReadOnly ]]; then
  codesign --verify --strict "$app"
  shasum -a 256 "$app/NU54Renter" "$app/main.jsbundle" | awk '{print $1}' > "$result/signed.sha256"
fi
capture "$scratch/install.log" xcrun simctl install "$sim" "$app"
defer_child_start
python3 "$runner/fixtures/p09_booking_catalog_tls.py" "$scratch" "$catalog_port" "$sim" --itinerary >"$scratch/catalog.log" 2>&1 &
catalog=$!
finish_child_start
defer_child_start
NU54_T13_TEST_GRANT="$grant" python3 "$runner/fixtures/p09_private_tls.py" "$fixture_mode"   --records-port "$record_port" --enrollment-port "$enroll_port" --work-file "$scratch/private.path" >"$scratch/private.log" 2>&1 &
private=$!
finish_child_start
capture "$scratch/scheme.log" "${ios_test_env[@]}" xcodebuild build-for-testing -project "$runner/SubscriptionDeniedSmoke.xcodeproj"   -scheme SceneSmoke -destination "platform=iOS Simulator,id=$sim" -derivedDataPath "$scratch/UI"
for ((i=0; i<90; i++)); do
  [[ -f "$scratch/catalog-ready" && -s "$scratch/private.path" ]] && break
  kill -0 "$catalog" && kill -0 "$private" || exit 1
  sleep 1
done
[[ -f "$scratch/catalog-ready" && -s "$scratch/private.path" ]]
spec=$(find "$scratch/UI" -name 'SceneSmoke_*.xctestrun' -print -quit)
NU54_T13_TEST_GRANT="$grant" python3 - "$spec" "$bundle" "$enroll_port" <<'PYTHON'
import os, pathlib, plistlib, sys
spec = pathlib.Path(sys.argv[1])
manifest = plistlib.loads(spec.read_bytes())
if "TestConfigurations" in manifest:
    targets = [target for config in manifest["TestConfigurations"] for target in config["TestTargets"]]
else:
    targets = [target for name, target in manifest.items() if not name.startswith("__")]
for target in targets:
    target.setdefault("EnvironmentVariables", {}).update(
        NU54_T13_TEST_BUNDLE=sys.argv[2], NU54_T13_TEST_PORT=sys.argv[3],
        NU54_T13_TEST_GRANT=os.environ["NU54_T13_TEST_GRANT"],
    )
spec.write_bytes(plistlib.dumps(manifest))
PYTHON
code=0
capture "$scratch/test.log" "${ios_test_env[@]}" xcodebuild test-without-building -xctestrun "$spec"   -destination "platform=iOS Simulator,id=$sim" -parallel-testing-enabled NO -collect-test-diagnostics never   -only-testing:"SceneSmoke/TravelItineraryConflictSmoke/$scenario"   -resultBundlePath "$scratch/Conflict.xcresult" || code=$?
if [[ -d "$scratch/Conflict.xcresult" ]]; then
  xcrun xcresulttool get test-results summary --path "$scratch/Conflict.xcresult" | python3 -c '
import json,sys
data=json.load(sys.stdin)
keys=("passedTests","failedTests","skippedTests","totalTestCount")
summary={key:int(data.get(key,0)) for key in keys}
if data.get("startTime") and data.get("finishTime"):
    summary["seconds"]=round(data["finishTime"]-data["startTime"],3)
print(json.dumps(summary))
' >"$result/test-summary.json"
fi
if ((code != 0)) && [[ $scenario == testRegisteredMyTripDestinationsRemainReadOnly ]]; then
  sed -nE 's|^.*TravelItineraryConflictSmoke\.swift:([0-9]+): error:.*|\1|p' "$scratch/test.log" >"$result/assertion-lines.txt"
fi
test "$code" -eq 0
pgroot=$(cat "$scratch/private.path")
pgport=$(sed -n '4p' "$pgroot/pg/postmaster.pid")
pg() {
  PGHOST="$pgroot" PGPORT="$pgport" "${POSTGRES_BIN:-/opt/homebrew/opt/postgresql@15/bin}/psql" \
    -XtA -d nu54_p09_private_ios "$@"
}
if [[ $scenario == testRegisteredMyTripDestinationsRemainReadOnly ]]; then
  pg -c 'SELECT (SELECT count(*) FROM travel_private.consent_decision),
                (SELECT count(*) FROM travel_private.record),
                (SELECT count(*) FROM travel_private.review),
                (SELECT count(*) FROM travel_private.visit_claim),
                (SELECT count(*) FROM travel_private.itinerary),
                (SELECT count(*) FROM travel_private.erasure_request)' >"$result/db-check.log"
  test "$(cat "$result/db-check.log")" = '0|0|0|0|0|0'
else
  pg -c 'SELECT count(*),max(revision) FROM travel_private.itinerary' >"$result/db-check.log"
  test "$(cat "$result/db-check.log")" = "$expected_db"
fi
python3 -c 'import json,sys; s=json.load(open(sys.argv[1])); assert s["totalTestCount"]==s["passedTests"]==1' "$result/test-summary.json"
