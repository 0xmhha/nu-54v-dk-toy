#!/usr/bin/env bash
set -euo pipefail
umask 077
root=$(cd -- "$(dirname -- "$0")/../../.." && pwd)
cd "$root"
sim=${1:?supply a disposable simulator UDID}
derived=${2:?supply a reusable P02 build directory}
out=${3:?supply a new output directory}
mkdir "$out"
out=$(cd "$out" && pwd)
scratch=$(mktemp -d /tmp/nu54-visit-scene.XXXXXX)
trap 'rm -rf -- "$scratch"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
fixture_pid=
test_pid=
namespace="com.nu54renter.travel.visits.$(uuidgen | tr '[:upper:]' '[:lower:]')"

cleanup() {
  if [[ -n "$test_pid" ]]; then
    kill -TERM "$test_pid" 2>/dev/null || true
    wait "$test_pid" 2>/dev/null || true
  fi
  if [[ -n "$fixture_pid" ]]; then
    kill -TERM "$fixture_pid" 2>/dev/null || true
    wait "$fixture_pid" 2>/dev/null || true
  fi
  xcrun simctl uninstall "$sim" "$namespace" >/dev/null 2>&1 || true
  rm -rf -- "$scratch"
}
source "$root/scripts/ios/test-environment.sh"
ios_test_prepare_environment "$scratch"
trap cleanup EXIT

xcrun simctl bootstatus "$sim" -b
"${ios_test_env[@]}" xcodebuild -workspace products/p02-user-app/ios/NU54Renter.xcworkspace -scheme NU54Renter \
  -configuration Release -destination 'generic/platform=iOS Simulator' \
  -derivedDataPath "$derived" -jobs 3 ARCHS=arm64 CODE_SIGNING_ALLOWED=YES CODE_SIGN_IDENTITY=- \
  NU54_IOS_BUNDLE="$namespace" build >"$out/app-build.log" 2>&1
xcrun simctl install "$sim" "$derived/Build/Products/Release-iphonesimulator/NU54Renter.app"

python3 -u scripts/ios/scene-smoke/fixtures/p09_visit_journey.py --ready-file "$scratch/ready" >"$out/fixture.log" 2>&1 &
fixture_pid=$!
"${ios_test_env[@]}" xcodebuild build-for-testing -project scripts/ios/scene-smoke/SceneSmoke.xcodeproj -scheme SceneSmoke \
  -destination "id=$sim" -derivedDataPath "$scratch/Build" >"$out/test-build.log" 2>&1
for _ in $(seq 1 60); do
  if [[ -s "$scratch/ready" ]]; then break; fi
  if ! kill -0 "$fixture_pid" 2>/dev/null; then
    echo "Travel visit fixture failed. See $out/fixture.log." >&2
    exit 1
  fi
  sleep 1
done
test -s "$scratch/ready"
manifest=$(find "$scratch/Build" -name 'SceneSmoke_*.xctestrun' -print -quit)
python3 - "$manifest" "$namespace" "$scratch/ready" <<'PYTHON'
from pathlib import Path
import plistlib
import sys

path, bundle, ready = Path(sys.argv[1]), sys.argv[2], Path(sys.argv[3])
manifest = plistlib.loads(path.read_bytes())
if "TestConfigurations" in manifest:
    targets = [target for config in manifest["TestConfigurations"] for target in config["TestTargets"]]
else:
    targets = [target for name, target in manifest.items() if not name.startswith("__")]
for target in targets:
    target.setdefault("EnvironmentVariables", {}).update(
        NU54_VISIT_QA_BUNDLE=bundle, NU54_VISIT_QA_FIXTURE=ready.read_text().strip()
    )
path.write_bytes(plistlib.dumps(manifest))
PYTHON
code=0
"${ios_test_env[@]}" xcodebuild test-without-building -xctestrun "$manifest" -destination "id=$sim" \
  -resultBundlePath "$out/TravelVisits.xcresult" -parallel-testing-enabled NO \
  -only-testing:SceneSmoke/TravelVisitSmoke >"$out/native.log" 2>&1 &
test_pid=$!
wait "$test_pid" || code=$?
test_pid=
tail -22 "$out/native.log"
exit "$code"
