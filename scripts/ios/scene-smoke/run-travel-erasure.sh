#!/usr/bin/env bash
set -euo pipefail
umask 077
root=$(cd -- "$(dirname -- "$0")/../../.." && pwd)
cd "$root"
sim=${1:?supply a disposable test simulator UDID}
derived=${2:?supply a reusable P02 build directory}
out=${3:?supply a new test output directory}
mkdir "$out"
scratch=$(mktemp -d /tmp/nu54-t12-qa.XXXXXX)
trap 'rm -rf -- "$scratch"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
fixture_pid=
test_pid=
namespace="com.nu54renter.travel.receipt-$(uuidgen | tr '[:upper:]' '[:lower:]' | cut -c1-12)"
cleanup() {
    if [[ -n "$test_pid" ]]; then
        kill -TERM "$test_pid" 2>/dev/null || true
        wait "$test_pid" 2>/dev/null || true
    fi
    if [[ -n "$fixture_pid" ]]; then
        kill -TERM "$fixture_pid" 2>/dev/null || true
        wait "$fixture_pid" 2>/dev/null || true
    fi
    for identity in qa peer observer; do
        xcrun simctl uninstall "$sim" "$namespace-$identity" >/dev/null 2>&1 || true
    done
    rm -rf -- "$scratch"
}
source "$root/scripts/ios/test-environment.sh"
ios_test_prepare_environment "$scratch"
trap cleanup EXIT
capture() {
    local log=$1 code=0
    shift
    "$@" >"$log" 2>&1 &
    test_pid=$!
    wait "$test_pid" || code=$?
    test_pid=
    return "$code"
}
xcrun simctl bootstatus "$sim" -b
for identity in qa peer observer; do
    bundle="$namespace-$identity"
    capture "$scratch/build-$identity.log" "${ios_test_env[@]}" xcodebuild -workspace products/p02-user-app/ios/NU54Renter.xcworkspace -scheme NU54Renter \
        -configuration Release -destination 'generic/platform=iOS Simulator' \
        -derivedDataPath "$derived" -jobs 3 ARCHS=arm64 CODE_SIGNING_ALLOWED=YES CODE_SIGN_IDENTITY=- \
        NU54_IOS_BUNDLE="$bundle" build
    xcrun simctl install "$sim" "$derived/Build/Products/Release-iphonesimulator/NU54Renter.app"
done
python3 -u scripts/ios/scene-smoke/fixtures/p09_erasure_tls.py --work-file "$scratch/work.path" >"$scratch/fixture.log" 2>&1 &
fixture_pid=$!
for _ in $(seq 1 60); do
    if grep -q 'PRIVATE ENROLLMENT READY' "$scratch/fixture.log"; then break; fi
    if ! kill -0 "$fixture_pid" 2>/dev/null; then echo "T12 fixture unavailable" >&2; exit 1; fi
    sleep 1
done
grep -q 'PRIVATE ENROLLMENT READY' "$scratch/fixture.log"
code=0
capture "$scratch/test-build.log" "${ios_test_env[@]}" xcodebuild build-for-testing -project scripts/ios/scene-smoke/SceneSmoke.xcodeproj -scheme SceneSmoke     -destination "id=$sim" -derivedDataPath "$scratch/Build"
config=$(find "$scratch/Build" -name 'SceneSmoke_*.xctestrun' -print -quit)
python3 - "$config" "$namespace" <<'PYTHON'
import pathlib, plistlib, sys
path = pathlib.Path(sys.argv[1])
manifest = plistlib.loads(path.read_bytes())
if "TestConfigurations" in manifest:
    targets = [target for config in manifest["TestConfigurations"] for target in config["TestTargets"]]
else:
    targets = [target for name, target in manifest.items() if not name.startswith("__")]
for target in targets:
    target.setdefault("EnvironmentVariables", {})["NU54_T12_QA_NAMESPACE"] = sys.argv[2]
path.write_bytes(plistlib.dumps(manifest))
PYTHON
capture "$scratch/test.log" "${ios_test_env[@]}" xcodebuild test-without-building -xctestrun "$config"     -destination "id=$sim" -resultBundlePath "$scratch/T12.xcresult"     -parallel-testing-enabled NO -only-testing:SceneSmoke/TravelErasureSceneSmoke || code=$?
if [[ -d "$scratch/T12.xcresult" ]]; then
    xcrun xcresulttool get test-results summary --path "$scratch/T12.xcresult" | python3 -c '
import json, sys
result = json.load(sys.stdin)
keys = ("totalTestCount", "passedTests", "failedTests", "skippedTests")
print(json.dumps({key: int(result.get(key, 0)) for key in keys}))
' >"$out/test-summary.json"
fi
test "$code" -eq 0
pgroot=$(cat "$scratch/work.path")
pgport=$(sed -n '4p' "$pgroot/pg/postmaster.pid")
psql_bin=${POSTGRES_BIN:-/opt/homebrew/opt/postgresql@15/bin}/psql
PGHOST="$pgroot" PGPORT="$pgport" "$psql_bin" -XtA -d nu54_p09_private_ios -c \
    "SELECT count(*) FROM travel_private.my_records('a4aa517d-b258-4f88-bea1-00630c790cd5'); SELECT count(*),count(journaled_at),count(this_device_erased_at),count(installation_proof_digest) FROM travel_private.erasure_request;" >"$out/db-check.log"
test "$(cat "$out/db-check.log")" = "$(printf '1\n1|1|1|1')"
python3 -c 'import json,sys; s=json.load(open(sys.argv[1])); assert s=={"totalTestCount":1,"passedTests":1,"failedTests":0,"skippedTests":0}' "$out/test-summary.json"
