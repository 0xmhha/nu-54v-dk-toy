#!/usr/bin/env bash
# Stage owned inputs, optionally running the signed W12 installation test.
set -euo pipefail
umask 077
[[ $# == 1 && ( $1 == --prepare || $1 == --preflight || $1 == --install || $1 == --install-slot ) ]] || {
  echo 'usage: run-owned.sh --prepare|--preflight|--install|--install-slot' >&2
  exit 2
}
mode=$1
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
root=$(cd -- "$here/../../.." && pwd)
cd "$root"

fail() {
  echo "W12 preflight: $*" >&2
  exit 2
}
foundry_tool() {
  local name=$1 candidate=$2 tool version
  tool=$(command -v -- "${candidate:-$name}") && [[ -f $tool && -x $tool ]] ||
    fail "missing $name executable; check W12_FORGE or W12_ANVIL"
  version=$("$tool" --version) || fail "cannot read $name version"
  case ${version%%$'\n'*} in
    "$name Version: 1.5.1"|"$name Version: 1.5.1-v1.5.1") ;;
    *) fail "$name must be version 1.5.1" ;;
  esac
  printf '%s\n' "$tool"
}
if [[ $mode != --prepare ]]; then
  forge=$(foundry_tool forge "${W12_FORGE-}")
  anvil=$(foundry_tool anvil "${W12_ANVIL-}")
  pg_bin=${W12_PG_BIN:-/opt/homebrew/opt/postgresql@15/bin}
  solc=${W12_SOLC:-$root/cache/solc/solc-0.8.39}
  for tool in postgres psql pg_ctl initdb createdb; do
    [[ -x $pg_bin/$tool ]] || fail "missing $tool in W12_PG_BIN"
  done
  for tool in xcrun xcodebuild codesign node python3 go cmake; do
    command -v "$tool" >/dev/null || fail "missing $tool"
  done
  [[ -x $solc ]] || fail "missing pinned W12_SOLC"
  solc_version=$("$solc" --version) || fail "cannot read Solidity version"
  [[ $solc_version == *"Version: 0.8.39+commit.ce47f2c8."* ]] || fail "expected Solidity 0.8.39"
  [[ -x $here/run-w12-installed.sh &&
     -r $here/SceneSmoke.xcodeproj/project.pbxproj &&
     -r $root/products/p02-user-app/ios/NU54Renter.xcworkspace/contents.xcworkspacedata &&
     -r $root/products/p02-user-app/ios/Pods/Manifest.lock ]] || fail "missing existing iOS workspace, Pods or SceneSmoke inputs"
  if [[ $mode == --preflight ]]; then
    echo 'W12 prerequisites verified without opening a simulator'
    exit 0
  fi
fi
source "$here/private-build.sh"
# Keep cleanup diagnostics visible even if a signal interrupts a logged build phase.
exec 9>&2
scene_prepare
sim=
remove_owned_simulator() {
  local attempt devices
  xcrun simctl shutdown "$sim" >/dev/null 2>&1 || :
  for attempt in 1 2 3; do
    xcrun simctl delete "$sim" >/dev/null 2>&1 || :
    # Read the complete listing first. A failed listing isn't proof of absence.
    if devices=$(xcrun simctl list devices) && [[ $devices != *"$sim"* ]]; then
      return 0
    fi
    if [[ $attempt != 3 ]]; then sleep 0.2; fi
  done
  return 1
}

cleanup() {
  local status=$?
  # A repeated signal must not reenter the EXIT trap while shutting down.
  trap '' HUP INT TERM
  if [[ -n $sim ]] && ! remove_owned_simulator; then
    echo "could not confirm removal of owned W12 simulator $sim" >&9
    if [[ $status == 0 ]]; then status=1; fi
  fi
  scene_cleanup "$status"
}
trap cleanup EXIT

stage="$scene_work/source"
product=products/p02-user-app
source_app="$root/$product"
app="$stage/$product"
mkdir -p -- "$app"
for file in app.json babel.config.js index.js metro.config.js package.json; do
  cp -p -- "$source_app/$file" "$app/$file"
done
for directory in build-tools ios native src; do
  cp -cR "$source_app/$directory" "$app/$directory"
done
for source in node_modules packages package.json; do
  ln -s "$root/$source" "$stage/$source"
done
ln -s "$source_app/node_modules" "$app/node_modules"
# The P02 native market signer includes the P01 C sources by a relative path.
ln -s "$root/products/p01-device-firmware" "$stage/products/p01-device-firmware"
cat >>"$app/metro.config.js" <<'JS'

// Read-only installed package inputs resolve through a symlink in the test copy.
module.exports.watchFolders.push(process.env.NU54_W12_READONLY_ROOT);
module.exports.resolver.useWatchman = false;
JS
export NU54_W12_READONLY_ROOT="$root"

build_phase() {
  local label=$1
  shift
  if ! scene_run "$@" >"$scene_work/$label.log" 2>&1; then
    echo "W12 $label failed" >&2
    if [[ $label == app || $label == smoke ]]; then
      grep -m 5 -E '(^|: )(error|fatal error):' "$scene_work/$label.log" >&2 || :
    elif [[ $label == integration ]]; then
      grep -E '^(=== RUN   TestGenerationReader|[[:space:]]*--- FAIL: TestGenerationReader|W12 XCTest failure at line [0-9]+|P07 .*scratch removed)'         "$scene_work/$label.log" | tail -12 >&2 || :
    fi
    return 1
  fi
}
build_phase codegen node "$root/node_modules/react-native/scripts/generate-codegen-artifacts.js"   -p "$app" -o "$scene_work/codegen" -t ios
bindings=(NU54RenterSpec/NU54RenterSpec.h NU54RenterSpec/NU54RenterSpec-generated.mm NU54RenterSpecJSI.h)
for entry in "${bindings[@]}"; do
  cp -p "$scene_work/codegen/build/generated/ios/ReactCodegen/$entry" "$app/ios/build/generated/ios/ReactCodegen/$entry"
done

shasum -a 256 "$app/src/wallet/RenterReturnStatus.tsx" "$here/SceneSmoke.swift" \
  "$here/run-w12-installed.sh" "$root/products/p07-indexer/test/generation-v1.py"
if [[ $mode == --prepare ]]; then
  echo 'W12 staged source and codegen verified'
  exit 0
fi

sim=$(xcrun simctl create "W12 QA $(uuidgen)" \
  "${W12_DEVICE_TYPE:-com.apple.CoreSimulator.SimDeviceType.iPhone-17-Pro}" \
  "${W12_RUNTIME:-com.apple.CoreSimulator.SimRuntime.iOS-27-0}")
bundle="com.nu54.renter.w12qa.r$(uuidgen | tr '[:upper:]' '[:lower:]' | tr -d '-')"
build_phase boot xcrun simctl boot "$sim"
build_phase smoke xcodebuild build-for-testing -project "$here/SceneSmoke.xcodeproj" -scheme SceneSmoke \
  -destination "platform=iOS Simulator,id=$sim" -derivedDataPath "$scene_work/Smoke" -jobs 2 \
  "MODULE_CACHE_DIR=$scene_work/ModuleCache"
export NU54_W12_SIM=$sim NU54_W12_APP=$bundle
NU54_W12_XCTESTRUN=$(find "$scene_work/Smoke" -name 'SceneSmoke_*.xctestrun' -print -quit)
[[ -f $NU54_W12_XCTESTRUN ]] || fail "SceneSmoke did not produce an xctestrun"
plutil -lint "$NU54_W12_XCTESTRUN" >/dev/null || fail "invalid SceneSmoke xctestrun"
export NU54_W12_XCTESTRUN
build_phase app xcodebuild -workspace "$app/ios/NU54Renter.xcworkspace" -scheme NU54Renter \
  -configuration Release -sdk iphonesimulator -destination "id=$sim" -derivedDataPath "$scene_work/App" \
  -jobs 2 CODE_SIGN_IDENTITY=- "PRODUCT_BUNDLE_IDENTIFIER=$bundle" "BUNDLE_CONFIG=$app/metro.config.js" \
  "MODULE_CACHE_DIR=$scene_work/ModuleCache" build
binary="$scene_work/App/Build/Products/Release-iphonesimulator/NU54Renter.app"
codesign --verify --strict "$binary"
test "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$binary/Info.plist")" = "$bundle"
shasum -a 256 "$binary/NU54Renter" "$binary/main.jsbundle"
build_phase bootstatus xcrun simctl bootstatus "$sim" -b
build_phase install xcrun simctl install "$sim" "$binary"
export P05_O09_W12_CREDENTIAL=1 P05_O09_RENTER_V1_ORIGIN=1 P05_O09_RENTER_ASSESSMENT=mixed
unset P05_O09_W12_SLOT_OPEN
if [[ $mode == --install-slot ]]; then export P05_O09_W12_SLOT_OPEN=1; fi
export P05_O09_W12_INSTALLED_RUNNER="$here/run-w12-installed.sh"
build_phase integration python3 -B products/p07-indexer/test/generation-v1.py --combined-p05 \
  --pg-bin "$pg_bin" \
  --solc "$solc" \
  --forge "$forge" \
  --anvil "$anvil"
grep -E 'W12 installed handover:|InstalledRenter(Credential|SlotOpen)AndOriginalPayment|compiler artifacts and Go scratch removed' \
  "$scene_work/integration.log"
