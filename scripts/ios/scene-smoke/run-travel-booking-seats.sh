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
scratch=$(mktemp -d /tmp/nu54-bookingseats-qa.XXXXXX)
trap 'rm -rf -- "$scratch"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
bundle="com.nu54renter.bookingseats.$(uuidgen | tr '[:upper:]' '[:lower:]' | tr -d '-')"
fixture_pid=
commerce_pid=
test_pid=
cleanup() {
  for worker in "$test_pid" "$fixture_pid" "$commerce_pid"; do
    if [[ -n "$worker" ]]; then
      kill -TERM "$worker" 2>/dev/null || true
      wait "$worker" 2>/dev/null || true
    fi
  done
  xcrun simctl uninstall "$sim" "$bundle" >/dev/null 2>&1 || true
  rm -rf -- "$scratch"
}
source "$root/scripts/ios/test-environment.sh"
ios_test_prepare_environment "$scratch"
trap cleanup EXIT

export POSTGRES_BIN=${POSTGRES_BIN:-/opt/homebrew/opt/postgresql@15/bin}
export PATH="$POSTGRES_BIN:$PATH"
port=$(python3 - <<'PYTHON'
import socket
with socket.socket() as listener:
    listener.bind(("127.0.0.1", 0))
    print(listener.getsockname()[1])
PYTHON
)
xcrun simctl bootstatus "$sim" -b
NU54_P09_PUBLIC_ORIGIN="https://127.0.0.1:$port" \
  "${ios_test_env[@]}" xcodebuild -workspace products/p02-user-app/ios/NU54Renter.xcworkspace -scheme NU54Renter \
    -configuration Release -destination 'generic/platform=iOS Simulator' \
    -derivedDataPath "$derived" -jobs 3 ARCHS=arm64 CODE_SIGNING_ALLOWED=YES CODE_SIGN_IDENTITY=- \
    NU54_IOS_BUNDLE="$bundle" build >"$out/app-build.log" 2>&1
xcrun simctl install "$sim" "$derived/Build/Products/Release-iphonesimulator/NU54Renter.app"

python3 -u scripts/ios/scene-smoke/fixtures/p09_booking_catalog_tls.py "$scratch" "$port" "$sim" --itinerary \
  >"$out/fixture.log" 2>&1 &
fixture_pid=$!
python3 -u scripts/ios/scene-smoke/fixtures/p05_pickup_native.py --booking-seats --ready-file "$scratch/commerce.json" \
  >"$out/commerce-fixture.log" 2>&1 &
commerce_pid=$!
"${ios_test_env[@]}" xcodebuild build-for-testing -project scripts/ios/scene-smoke/SceneSmoke.xcodeproj -scheme SceneSmoke \
  -destination "id=$sim" -derivedDataPath "$scratch/UI" >"$out/test-build.log" 2>&1
for _ in $(seq 1 60); do
  [[ ! -f "$scratch/catalog-ready" || ! -s "$scratch/commerce.json" ]] || break
  if ! kill -0 "$fixture_pid" 2>/dev/null || ! kill -0 "$commerce_pid" 2>/dev/null; then
    echo "Signed public catalog failed to start. See $out/fixture.log." >&2
    exit 1
  fi
  sleep 1
done
test -f "$scratch/catalog-ready"
test -s "$scratch/commerce.json"
driver=$(find "$scratch/UI" -name 'SceneSmoke_*.xctestrun' -print -quit)
python3 - "$driver" "$bundle" "$scratch/commerce.json" <<'PYTHON'
from pathlib import Path
import json
import plistlib
import sys

path, bundle = Path(sys.argv[1]), sys.argv[2]
commerce = json.loads(Path(sys.argv[3]).read_text())
manifest = plistlib.loads(path.read_bytes())
if "TestConfigurations" in manifest:
    targets = [target for config in manifest["TestConfigurations"] for target in config["TestTargets"]]
else:
    targets = [target for name, target in manifest.items() if not name.startswith("__")]
for target in targets:
    target.setdefault("EnvironmentVariables", {}).update(NU54_BOOKING_P02=bundle,
        NU54_BOOKING_CONTROL=commerce["origin"], NU54_BOOKING_TOKEN=commerce["access"])
path.write_bytes(plistlib.dumps(manifest))
PYTHON
code=0
"${ios_test_env[@]}" xcodebuild test-without-building -xctestrun "$driver" -destination "id=$sim" \
  -resultBundlePath "$out/TravelBookingSeats.xcresult" -parallel-testing-enabled NO -collect-test-diagnostics never \
  -only-testing:SceneSmoke/TravelBookingSeatsSmoke >"$out/native.log" 2>&1 &
test_pid=$!
wait "$test_pid" || code=$?
test_pid=
tail -25 "$out/native.log"
exit "$code"
