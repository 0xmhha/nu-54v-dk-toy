#!/usr/bin/env bash
set -euo pipefail
umask 077
root=$(cd -- "$(dirname -- "$0")/../../.." && pwd)
cd "$root"
sim=${1:?supply a disposable simulator UDID}
phone_derived=${2:?supply a reusable P02 build directory}
kiosk_derived=${3:?supply a reusable P04 build directory}
out=${4:?supply a new output directory}
test_method=testPaidAcceptanceKitchenHandoverAndOriginalRefund
fixture_flags=()
if [[ ${5-} == --changed ]]; then
  test_method=testChangedPickupRequiresNewConsent
  fixture_flags=(--pickup-changed)
elif [[ $# != 4 ]]; then
  exit 2
fi
mkdir "$out"
out=$(cd "$out" && pwd)
scratch=$(mktemp -d /tmp/nu54-pickup-scene.XXXXXX)
trap 'rm -rf -- "$scratch"' EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
suffix=$(uuidgen | tr '[:upper:]' '[:lower:]' | tr -d '-')
phone="com.nu54renter.pickup.$suffix"
kiosk="com.nu54kiosk.pickup.$suffix"
fixture_pid=
catalog_pid=
test_pid=
phone_pid=
kiosk_pid=
cleanup() {
  for build in "$phone_pid" "$kiosk_pid"; do
    if [[ -n "$build" ]]; then
      kill -TERM -- "-$build" 2>/dev/null || true
      wait "$build" 2>/dev/null || true
    fi
  done
  for worker in "$test_pid" "$catalog_pid" "$fixture_pid"; do
    if [[ -n "$worker" ]]; then
      kill -TERM "$worker" 2>/dev/null || true
      wait "$worker" 2>/dev/null || true
    fi
  done
  xcrun simctl uninstall "$sim" "$phone" >/dev/null 2>&1 || true
  xcrun simctl uninstall "$sim" "$kiosk" >/dev/null 2>&1 || true
  rm -rf -- "$scratch"
}
source "$root/scripts/ios/test-environment.sh"
ios_test_prepare_environment "$scratch"
trap cleanup EXIT
source "$root/scripts/ios/stage-project.sh"
stage="$scratch/source"
scene_stage_ios_source "$root" "$stage"
scene_stage_ios_project "$root" "$stage" "$scratch" "$out" p02-user-app NU54RenterSpec
if [[ $test_method != testChangedPickupRequiresNewConsent ]]; then
  scene_stage_ios_project "$root" "$stage" "$scratch" "$out" p04-merchant-kiosk NU54KioskSpec
fi
port=$(python3 - <<'PYTHON'
import socket
with socket.socket() as listener:
    listener.bind(("127.0.0.1", 0))
    print(listener.getsockname()[1])
PYTHON
)
export POSTGRES_BIN=${POSTGRES_BIN:-/opt/homebrew/opt/postgresql@15/bin}
export PATH="$POSTGRES_BIN:$PATH"
xcrun simctl bootstatus "$sim" -b
set -m
NU54_P09_PUBLIC_ORIGIN="https://127.0.0.1:$port"   "${ios_test_env[@]}" xcodebuild -workspace "$stage/products/p02-user-app/ios/NU54Renter.xcworkspace" "BUNDLE_CONFIG=$stage/products/p02-user-app/metro.config.js" -scheme NU54Renter   -configuration Release -destination 'generic/platform=iOS Simulator' -derivedDataPath "$phone_derived"   -jobs 3 ARCHS=arm64 CODE_SIGNING_ALLOWED=YES CODE_SIGN_IDENTITY=- NU54_IOS_BUNDLE="$phone" build >"$out/p02.log" 2>&1 &
phone_pid=$!
if [[ $test_method != testChangedPickupRequiresNewConsent ]]; then
  "${ios_test_env[@]}" xcodebuild -workspace "$stage/products/p04-merchant-kiosk/ios/NU54Kiosk.xcworkspace" "BUNDLE_CONFIG=$stage/products/p04-merchant-kiosk/metro.config.js" -scheme NU54Kiosk   -configuration Release -destination 'generic/platform=iOS Simulator' -derivedDataPath "$kiosk_derived"   -jobs 3 ARCHS=arm64 CODE_SIGNING_ALLOWED=YES CODE_SIGN_IDENTITY=- NU54_IOS_BUNDLE="$kiosk" build >"$out/p04.log" 2>&1 &
  kiosk_pid=$!
fi
build_code=0
wait "$phone_pid" || build_code=$?
phone_pid=
if [[ -n $kiosk_pid ]]; then
  wait "$kiosk_pid" || build_code=$?
  kiosk_pid=
fi
set +m
if ((build_code)); then exit "$build_code"; fi
phone_app="$phone_derived/Build/Products/Release-iphonesimulator/NU54Renter.app"
codesign --verify --strict "$phone_app"
shasum -a 256 "$phone_app/NU54Renter" "$phone_app/main.jsbundle" | awk '{print $1}' > "$out/signed.sha256"
xcrun simctl install "$sim" "$phone_app"
if [[ $test_method != testChangedPickupRequiresNewConsent ]]; then
  kiosk_app="$kiosk_derived/Build/Products/Release-iphonesimulator/NU54Kiosk.app"
  codesign --verify --strict "$kiosk_app"
  shasum -a 256 "$kiosk_app/NU54Kiosk" "$kiosk_app/main.jsbundle" | awk '{print $1}' >> "$out/signed.sha256"
  xcrun simctl install "$sim" "$kiosk_app"
fi

python3 -u scripts/ios/scene-smoke/fixtures/p05_pickup_native.py   "${fixture_flags[@]}" --ready-file "$scratch/commerce.json" --catalog-dsn-file "$scratch/catalog-dsn" >"$out/fixture.log" 2>&1 &
fixture_pid=$!
python3 -u scripts/ios/scene-smoke/fixtures/p09_booking_catalog_tls.py   "$scratch" "$port" "$sim" --pickup --pickup-dsn-file "$scratch/catalog-dsn" >"$out/catalog.log" 2>&1 &
catalog_pid=$!
"${ios_test_env[@]}" xcodebuild build-for-testing -project scripts/ios/scene-smoke/SceneSmoke.xcodeproj -scheme SceneSmoke   -destination "id=$sim" -derivedDataPath "$scratch/UI" >"$out/ui-build.log" 2>&1
for _ in $(seq 1 60); do
  if [[ -f "$scratch/catalog-ready" && -s "$scratch/commerce.json" ]]; then break; fi
  kill -0 "$fixture_pid" 2>/dev/null && kill -0 "$catalog_pid" 2>/dev/null || exit 1
  sleep 1
done
test -f "$scratch/catalog-ready"
test -s "$scratch/commerce.json"
driver=$(find "$scratch/UI" -name 'SceneSmoke_*.xctestrun' -print -quit)
python3 - "$driver" "$scratch/commerce.json" "$phone" "$kiosk" <<'PYTHON'
import json
from pathlib import Path
import plistlib
import sys

path, fixture = Path(sys.argv[1]), Path(sys.argv[2])
manifest = plistlib.loads(path.read_bytes())
control = json.loads(fixture.read_text())
if "TestConfigurations" in manifest:
    targets = [target for config in manifest["TestConfigurations"] for target in config["TestTargets"]]
else:
    targets = [target for name, target in manifest.items() if not name.startswith("__")]
for target in targets:
    target.setdefault("EnvironmentVariables", {}).update(
        NU54_PICKUP_CONTROL=control["origin"], NU54_PICKUP_TOKEN=control["access"],
        NU54_PICKUP_P02=sys.argv[3], NU54_PICKUP_P04=sys.argv[4],
    )
path.write_bytes(plistlib.dumps(manifest))
PYTHON
code=0
"${ios_test_env[@]}" xcodebuild test-without-building -xctestrun "$driver" -destination "id=$sim"   -resultBundlePath "$out/TravelPickup.xcresult" -parallel-testing-enabled NO -collect-test-diagnostics never   -only-testing:"SceneSmoke/TravelPickupSmoke/$test_method" >"$out/native.log" 2>&1 &
test_pid=$!
wait "$test_pid" || code=$?
test_pid=
tail -25 "$out/native.log"
exit "$code"
