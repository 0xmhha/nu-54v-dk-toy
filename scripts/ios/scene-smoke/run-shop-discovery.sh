#!/usr/bin/env bash
set -euo pipefail
if [[ $# -ne 5 || ! "$3" =~ ^(online|offline)$ ]]; then
  echo 'usage: run-shop-discovery.sh SIMULATOR_UUID SIGNED_ENROLLED_APP online|offline FIXTURE_JSON OUTPUT_DIRECTORY' >&2
  exit 2
fi
sim=$1
app=$2
phase=$3
fixture=$4
output=$5
here=$(cd -- "$(dirname -- "$0")" && pwd)
mkdir -p -- "$output"
result="$output/ShopDiscovery-$phase.xcresult"
[[ ! -e "$result" ]] || { echo 'choose a fresh result directory' >&2; exit 2; }
bundle=$(/usr/libexec/PlistBuddy -c 'Print CFBundleIdentifier' "$app/Info.plist")
[[ "$bundle" =~ ^com\.nu54renter\.[A-Za-z0-9_.]+$ ]]
source "$here/private-build.sh"
scene_prepare
scene_run codesign --verify --strict --deep "$app"
scene_run xcrun simctl install "$sim" "$app"
scene_run xcodebuild build-for-testing -project "$here/SceneSmoke.xcodeproj" -scheme SceneSmoke \
  -destination "id=$sim" -derivedDataPath "$scene_work/Derived"
spec=$(find "$scene_work/Derived" -name 'SceneSmoke_*.xctestrun' -print -quit)
python3 - "$spec" "$fixture" "$bundle" <<'PY'
import json, pathlib, plistlib, sys
spec, fixture = map(pathlib.Path, sys.argv[1:3])
data = json.loads(fixture.read_bytes())
assert data["bundle"] == sys.argv[3]
data = json.dumps(data, ensure_ascii=False, separators=(",", ":"))
runner = plistlib.loads(spec.read_bytes())
if "TestConfigurations" in runner:
    targets = [entry for config in runner["TestConfigurations"] for entry in config["TestTargets"]]
else:
    targets = [entry for name, entry in runner.items() if not name.startswith("__")]
assert targets
for entry in targets:
    entry.setdefault("EnvironmentVariables", {})["NU54_SHOP_SMOKE_FIXTURE"] = data
spec.write_bytes(plistlib.dumps(runner))
PY
case "$phase" in
  online) method=testVerifiedEntranceAndPrivateCheckout ;;
  offline) method=testOfflineReopensOnlyOriginal ;;
esac
scene_run xcodebuild test-without-building -xctestrun "$spec" -destination "id=$sim" \
  -parallel-testing-enabled NO -collect-test-diagnostics never \
  -only-testing:"SceneSmoke/ShopDiscoverySmoke/$method" -resultBundlePath "$result"
xcrun xcresulttool get test-results summary --path "$result" | python3 -c '
import json, sys
summary = json.load(sys.stdin)
assert summary["passedTests"] == summary["totalTestCount"] == 1, summary'
