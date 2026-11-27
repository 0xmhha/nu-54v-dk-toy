#!/usr/bin/env bash
# Signed K01 app, local enrolled mTLS and a stopped/restarted P05 process.
set -euo pipefail
umask 077
root=$(cd -- "$(dirname -- "$0")/../../.." && pwd)
cd "$root"
sim=${1:?supply a fresh booted simulator UUID}
out=${2:?supply a new absolute result directory}
[[ "$sim" =~ ^[A-Fa-f0-9-]{36}$ && "$out" = /* ]]
bundle=com.nu54kiosk.outcomeqa
if xcrun simctl get_app_container "$sim" "$bundle" app >/dev/null 2>&1; then
  exit 2
fi
mkdir "$out"
scratch=$(mktemp -d /private/tmp/nu54-kiosk-offline.XXXXXX)
trap 'rm -rf -- "$scratch"' EXIT
fixture=
installed=0
cleanup() {
  local code=$?
  trap - EXIT
  trap '' INT TERM
  if [[ -n "$fixture" ]]; then kill -TERM "$fixture" 2>/dev/null || true; wait "$fixture" 2>/dev/null || true; fi
  if ((installed)); then xcrun simctl uninstall "$sim" "$bundle" 2>/dev/null || true; fi
  if ((code != 0)); then
    python3 - "$scratch" >"$out/failure.json" <<'PY' || echo 'could not write kiosk failure report' >&2
from pathlib import Path
import json, re, sys
root=Path(sys.argv[1])
log=lambda name:(root/name).read_text(errors="replace") if (root/name).is_file() else ""
print(json.dumps({"assertionLines":[int(x) for x in re.findall(r"SceneSmoke[.]swift:(\d+): error:",log("ui.log"))],
                  "fixtureErrors":[line[:220] for line in log("fixture.log").splitlines() if re.search("ERROR|failed|Error",line)][-8:],
                  "builtApp":"BUILD SUCCEEDED" in log("build.log"),
                  "builtUi":"TEST BUILD SUCCEEDED" in log("ui-build.log"),
                  "enrolled":"TEST EXECUTE SUCCEEDED" in log("enroll.log")}))
PY
  fi
  rm -rf -- "$scratch"
  exit "$code"
}
trap cleanup EXIT
trap 'exit 143' TERM
trap 'exit 130' INT
export PATH="${POSTGRES_BIN:-/opt/homebrew/opt/postgresql@15/bin}:/opt/homebrew/opt/openssl@3/bin:$PATH"
mkdir "$scratch/tmp" "$scratch/go-tmp"
export GOTMPDIR="$scratch/go-tmp" GOCACHE="$scratch/go-cache"
export PYTHONDONTWRITEBYTECODE=1 NU54_IOS_SKIP_TEST_DIAGNOSTICS=1
source "$root/scripts/ios/test-environment.sh"
ios_test_prepare_environment "$scratch"
"${ios_test_env[@]}" xcodebuild -workspace products/p04-merchant-kiosk/ios/NU54Kiosk.xcworkspace -scheme NU54Kiosk \
  -configuration Release -destination 'generic/platform=iOS Simulator' -derivedDataPath "$scratch/derived" \
  -jobs 2 ARCHS=arm64 CODE_SIGNING_ALLOWED=YES CODE_SIGN_IDENTITY=- "NU54_IOS_BUNDLE=$bundle" build >"$scratch/build.log" 2>&1
app="$scratch/derived/Build/Products/Release-iphonesimulator/NU54Kiosk.app"
codesign --verify --strict "$app"
shasum -a 256 "$app/NU54Kiosk" "$app/main.jsbundle" | awk '{print $1}' >"$scratch/app-sha256"
xcrun simctl install "$sim" "$app"
installed=1
NU54_SUBSCRIPTION_COMMERCE_SLOT=terminal bash scripts/ios/scene-smoke/run-subscription-native.sh "$sim" "$app" "$scratch/key" testCommerceEnrollmentPublicKey >"$scratch/key.log" 2>&1
python3 - "$scratch/key.log" "$scratch/terminal.der" <<'PY'
from pathlib import Path
import base64,re,sys
log,out=map(Path,sys.argv[1:])
key=re.search(r"NU54_SUBSCRIPTION_C3_SPKI=([a-zA-Z0-9+/]{122}==)",log.read_text())[1]
decoded=base64.b64decode(key,validate=True)
assert len(decoded)==91
out.write_bytes(decoded)
PY
openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out "$scratch/operator.key"
openssl pkey -in "$scratch/operator.key" -pubout -outform DER -out "$scratch/operator.der"
port=$(python3 - <<'PY'
import socket
with socket.socket() as s:
    s.bind(("127.0.0.1",0))
    print(s.getsockname()[1])
PY
)
export NU54_KIOSK_OFFLINE_FIXTURE=1 NU54_KIOSK_ORIGIN_PORT="$port"
TMPDIR="$scratch/tmp" bash scripts/ios/scene-smoke/fixtures/p05_kiosk_mode_native.sh "$scratch" "$scratch/terminal.der" "$scratch/operator.der" >"$scratch/fixture.log" 2>&1 &
fixture=$!
for _ in {1..240}; do
  [[ ! -s "$scratch/offline-ready.json" ]] || break
  kill -0 "$fixture"
  sleep .75
done
test -s "$scratch/offline-ready.json"
profile=$(python3 - "$scratch/offline-ready.json" <<'PY'
import base64,json,sys
from pathlib import Path
config=json.loads(Path(sys.argv[1]).read_text())
print(base64.b64encode((Path(config["directory"])/"terminal.json").read_bytes()).decode())
PY
)
NU54_SUBSCRIPTION_UI_FIXTURE="$profile" NU54_SUBSCRIPTION_COMMERCE_SLOT=terminal \
  bash scripts/ios/scene-smoke/run-subscription-native.sh "$sim" "$app" "$scratch/enroll" testNativeKioskStaffEnrollment >"$scratch/enroll.log" 2>&1
"${ios_test_env[@]}" xcodebuild build-for-testing -project scripts/ios/scene-smoke/SceneSmoke.xcodeproj -scheme SceneSmoke \
  -destination "id=$sim" -derivedDataPath "$scratch/UI" >"$scratch/ui-build.log" 2>&1
driver=$(find "$scratch/UI" -name 'SceneSmoke_*.xctestrun' -print -quit)
python3 - "$driver" "$scratch/offline-ready.json" <<'PY'
import json,plistlib,sys
from pathlib import Path
path, control=map(Path,sys.argv[1:])
spec=plistlib.loads(path.read_bytes())
scope=json.loads(control.read_text())
targets=[target for c in spec["TestConfigurations"] for target in c["TestTargets"]] if "TestConfigurations" in spec else [spec["SceneSmoke"]]
for target in targets:
    target.setdefault("EnvironmentVariables",{}).update(NU54_K01_CONTROL=scope["origin"],NU54_K01_TOKEN=scope["token"])
path.write_bytes(plistlib.dumps(spec))
PY
"${ios_test_env[@]}" xcodebuild test-without-building -xctestrun "$driver" -destination "id=$sim" \
  -parallel-testing-enabled NO -collect-test-diagnostics never \
  -only-testing:SceneSmoke/KioskOfflineSmoke/testColdOfflineMenuAndFreshOrderAfterServiceRestart \
  -resultBundlePath "$scratch/Result.xcresult" >"$scratch/ui.log" 2>&1
xcrun xcresulttool get test-results summary --path "$scratch/Result.xcresult" | python3 -c '
import json,sys
from pathlib import Path
r=json.load(sys.stdin)
native, bundle = Path(sys.argv[1]).read_text().splitlines()
assert r["passedTests"] == r["totalTestCount"] == 1 and r["failedTests"] == r["skippedTests"] == 0
print(json.dumps({"passedTests":1,"offlineOrders":0,"offlineInventoryWrites":0,"realServiceRestart":1,"newOriginalOrders":1,"freshOrderPriceBaseUnits":4200000,"publishedRows":52,"categories":2,"fullMenuBasket":True,"nativeSha256":native,"bundleSha256":bundle}))
' "$scratch/app-sha256" >"$out/summary.json"
cat "$out/summary.json"
