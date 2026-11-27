#!/usr/bin/env bash
# Use a simulator build with a dedicated bundle ID and no prior market enrollment.
set -euo pipefail
if [[ $# -lt 3 || $# -gt 4 ]]; then
  echo 'usage: run-market.sh SIMULATOR_UUID RENTER_APP OUTPUT_DIRECTORY [entry|lp|fx|orders]' >&2
  exit 2
fi
sim=$1
renter=$2
bundle=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$renter/Info.plist")
case "${4:-lp}" in
  entry) scenario=testMarketEntryRequiresInstalledDeployment ;;
  lp) scenario=testNativeWalletLiquidityAndOriginalHashAfterRestart ;;
  fx) scenario=testIsolatedFXQuoteAndUnchangedTokenBalances ;;
  orders) scenario=testSwapRemoveAndPosition ;;
  *) echo 'unknown market scenario' >&2; exit 2 ;;
esac
output=$(mkdir -p -- "$3" && cd -- "$3" && pwd)
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
market=$(cd -- "$here/../../../products/p08-market-services" && pwd)
[[ ! -e "$output/MarketSmoke.xcresult" ]] || { echo 'choose a new result directory' >&2; exit 2; }
xcrun simctl bootstatus "$sim" -b
xcrun simctl install "$sim" "$renter"
destination="platform=iOS Simulator,id=$sim"
source "$here/private-build.sh"
scene_prepare
scene_run xcodebuild build-for-testing -project "$here/SceneSmoke.xcodeproj" -scheme SceneSmoke \
  -destination "$destination" -derivedDataPath "$scene_work/Derived" ONLY_ACTIVE_ARCH=YES
source_spec=$(find "$scene_work/Derived" -name 'SceneSmoke_*.xctestrun' -print -quit)
spec=$(dirname "$source_spec")/MarketSmoke.xctestrun
python3 - "$source_spec" "$spec" "$bundle" <<'PY'
from pathlib import Path
import plistlib
import sys
source, target, bundle = sys.argv[1:]
model = plistlib.loads(Path(source).read_bytes())
entries = ([test for config in model["TestConfigurations"] for test in config["TestTargets"]]
           if "TestConfigurations" in model else [test for name, test in model.items() if not name.startswith("__")])
for entry in entries:
    entry.setdefault("EnvironmentVariables", {})["NU54_MARKET_TEST_BUNDLE"] = bundle
Path(target).write_bytes(plistlib.dumps(model))
PY
run=(xcodebuild test-without-building -xctestrun "$spec" -destination "$destination"
    -parallel-testing-enabled NO -only-testing:"SceneSmoke/MarketSmoke/$scenario"
    -resultBundlePath "$output/MarketSmoke.xcresult")
if [[ "${4:-lp}" == entry || -n "${NU54_MOBILE_QA_PROFILE:-}" ]]; then
  scene_run "${run[@]}"
else
  scene_run bash "$market/test/mobile-fixture.sh" "$output" "${run[@]}"
fi
xcrun xcresulttool get test-results summary --path "$output/MarketSmoke.xcresult" | python3 -c '
import json, sys
result = json.load(sys.stdin)
assert result["passedTests"] == result["totalTestCount"] == 1, result'
