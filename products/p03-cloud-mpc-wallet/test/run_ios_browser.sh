#!/usr/bin/env bash
# Use a dedicated simulator, a private scratch DB and synthesized issuer accounts.
set -euo pipefail
umask 077
if [[ $# != 2 ]]; then
  echo 'usage: run_ios_browser.sh SIMULATOR_UUID APP_DERIVED_DATA_DIRECTORY' >&2
  exit 2
fi
sim=$1
derived=$2
bundle=org.nu54.p03browser
if [[ ( "${NU54_P03_BROWSER_REFUSAL:-}" == 1 || "${NU54_P03_BROWSER_PARTIAL:-}" == 1 || "${NU54_P03_BROWSER_RESUME:-}" == 1 ) && "${NU54_P03_BROWSER_RECOVERY:-}" != 1 ]]; then
  echo "Refusal and partial ACK scenarios require recovery." >&2
  exit 2
fi
if [[ "${NU54_P03_BROWSER_REFUSAL:-}" == 1 && "${NU54_P03_BROWSER_PARTIAL:-}" == 1 ]]; then
  echo "Select one recovery scenario per run." >&2
  exit 2
fi
if [[ "${NU54_P03_BROWSER_RESUME:-}" == 1 && ( "${NU54_P03_BROWSER_REFUSAL:-}" == 1 || "${NU54_P03_BROWSER_PARTIAL:-}" == 1 ) ]]; then
  echo "Select receipt recovery without other scenarios." >&2
  exit 2
fi
allowance=(360 480)
scenario=testTwoWalletOwnersUseExternalS256BrowserConsent
if [[ "${NU54_P03_BROWSER_SIGN_REFUSAL:-}" == 1 &&
      ( "${NU54_P03_BROWSER_MARKET:-}" == 1 || "${NU54_P03_BROWSER_RECOVERY:-}" == 1 ) ]]; then
  echo 'Select only the signing-refusal scenario.' >&2
  exit 2
fi
if [[ "${NU54_P03_BROWSER_OIDC:-}" == 1 &&
      ( "${NU54_P03_BROWSER_MARKET:-}" == 1 || "${NU54_P03_BROWSER_RECOVERY:-}" == 1 || "${NU54_P03_BROWSER_SIGN_REFUSAL:-}" == 1 ) ]]; then
  echo 'Select the registered OIDC scenario independently.' >&2
  exit 2
fi
if [[ "${NU54_P03_BROWSER_OIDC:-}" == 1 ]]; then
  bundle=org.nu54.p03browseroidc.fixture$(openssl rand -hex 6)
  scenario=testRegisteredOIDCOnboardingAndReopen
elif [[ "${NU54_P03_BROWSER_MARKET:-}" == 1 ]]; then
  export P03_TEST_MARKET_PROFILE="${P08_PROFILE:?start the isolated P08 fixture first}"
  bundle=org.nu54.p03browsertestmarket.fixture$(openssl rand -hex 6)
  scenario=testSelectedMarketWalletUsesIndependentConsentAndKeepsOriginalHash
elif [[ "${NU54_P03_BROWSER_SIGN_REFUSAL:-}" == 1 ]]; then
  bundle=org.nu54.p03browsersignrefusal.fixture$(openssl rand -hex 6)
  allowance=(500 600)
elif [[ "${NU54_P03_BROWSER_RECOVERY:-}" == 1 ]]; then
  bundle=org.nu54.p03browserrecovery.fixture$(openssl rand -hex 6)
  allowance=(840 900)
fi
product=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
root=$(cd -- "$product/../.." && pwd)
out=$(mktemp -d /tmp/nu54-p03-browser.XXXXXXXX)
backend=
runner=
run_logged() {
  local log=$1 status=0
  shift
  "$@" >"$log" 2>&1 &
  runner=$!
  wait "$runner" || status=$?
  runner=
  return "$status"
}
cleanup() {
  local status=$?
  trap - EXIT
  trap '' INT TERM
  if [[ -n "$runner" ]]; then
    kill -TERM "$runner" 2>/dev/null || true
    wait "$runner" || true
  fi
  if [[ -n "$backend" ]]; then
    touch "$out/failed"
    wait "$backend" || true
  fi
  # XCTest hierarchy and activity can contain even concealed fixture credentials.
  if [[ -d "$out/Browser.xcresult" && ! -s "$out/apple-summary.json" ]]; then
    xcrun xcresulttool get test-results summary --path "$out/Browser.xcresult" >"$out/apple-summary.json" 2>/dev/null || true
  fi
  python3 "$product/test/ios-browser/summarize.py" "$out" "$status" || true
  if [[ "$bundle" == org.nu54.p03browser*.fixture* ]]; then
    xcrun simctl uninstall "$sim" "$bundle" >/dev/null 2>&1 || true
  fi
  rm -rf -- "$out/xcode-environment"
  rm -rf "$out/Browser.xcresult" "$out/UI" "$out/project" "$out/app-derived" "$out/go-cache"
  rm -f "$out/browser.json" "$out/browser.pending" "$out/ca.der" "$out/done" "$out/failed" \
    "$out/apple-summary.json" "$out/ui.log" "$out/ui-build.log" "$out/app-build.log" "$out/backend.log"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
echo "Browser verification evidence: $out"
source "$root/scripts/ios/test-environment.sh"
ios_test_prepare_environment "$out"
if [[ "${NU54_P03_BROWSER_SIGN_REFUSAL:-}" == 1 ]]; then
  # Keep the caller's caches intact while isolating every write for this run.
  if [[ -d "$derived" ]]; then cp -cR "$derived" "$out/app-derived"; fi
  if [[ -n "${GOCACHE:-}" && -d "$GOCACHE" ]]; then cp -cR "$GOCACHE" "$out/go-cache"; fi
  derived="$out/app-derived"
  export GOCACHE="$out/go-cache"
fi
xcrun simctl bootstatus "$sim" -b
mkdir "$out/project"
cp -R "$root/scripts/ios/scene-smoke/SceneSmoke.xcodeproj" "$out/project/"
cp "$product/test/ios-browser/SceneSmoke.swift" "$out/project/"
run_logged "$out/ui-build.log" "${ios_test_env[@]}" xcodebuild build-for-testing -project "$out/project/SceneSmoke.xcodeproj" -scheme SceneSmoke \
  -destination "platform=iOS Simulator,id=$sim" -derivedDataPath "$out/UI"

NU54_P03_MULTI=1 NU54_P03_BROWSER_LAB="$out" NU54_P03_BROWSER_RECOVERY="${NU54_P03_BROWSER_RECOVERY:-}" make -C "$product" db-test > "$out/backend.log" 2>&1 &
backend=$!
for ((i=0; i<150; i++)); do
  [[ -s "$out/browser.json" ]] && break
  kill -0 "$backend" || exit 1
  sleep 1
done
profile=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["profile"])' "$out/browser.json")
xcrun simctl keychain "$sim" add-root-cert "$out/ca.der"
run_logged "$out/app-build.log" "${ios_test_env[@]}" xcodebuild build -workspace "$root/products/p02-user-app/ios/NU54Renter.xcworkspace" -scheme NU54Renter \
  -configuration Release -destination 'generic/platform=iOS Simulator' -derivedDataPath "$derived" \
  ARCHS=arm64 CODE_SIGNING_ALLOWED=YES CODE_SIGN_IDENTITY=- CODE_SIGN_STYLE=Automatic DEVELOPMENT_TEAM= \
  PROVISIONING_PROFILE_SPECIFIER= PRODUCT_BUNDLE_IDENTIFIER="$bundle" \
  NU54_P03_PROFILE="$profile" NU54_MARKET_PROFILE="$([[ "${NU54_P03_BROWSER_MARKET:-}" == 1 ]] && echo loopback || true)"
xcrun simctl install "$sim" "$derived/Build/Products/Release-iphonesimulator/NU54Renter.app"
spec=$(find "$out/UI" -name 'SceneSmoke_*.xctestrun' -print -quit)
browser=$(dirname -- "$spec")/Browser.xctestrun
python3 - "$spec" "$browser" "$out/browser.json" "$bundle" <<'PY'
import base64, pathlib, plistlib, sys
source, target, fixture = map(pathlib.Path, sys.argv[1:4])
spec = plistlib.loads(source.read_bytes())
if "TestConfigurations" in spec:
    targets = [entry for config in spec["TestConfigurations"] for entry in config["TestTargets"]]
else:
    targets = [entry for name, entry in spec.items() if not name.startswith("__")]
assert targets
for entry in targets:
    variables = entry.setdefault("EnvironmentVariables", {})
    variables["NU54_P03_TEST_BROWSER"] = base64.b64encode(fixture.read_bytes()).decode()
    variables["NU54_P03_TEST_BUNDLE"] = sys.argv[4]
target.write_bytes(plistlib.dumps(spec))
PY
run_logged "$out/ui.log" "${ios_test_env[@]}" xcodebuild test-without-building -xctestrun "$browser" -destination "platform=iOS Simulator,id=$sim" \
  -parallel-testing-enabled NO -only-testing:"SceneSmoke/SceneSmoke/$scenario" \
  -test-timeouts-enabled YES -default-test-execution-time-allowance "${allowance[0]}" -maximum-test-execution-time-allowance "${allowance[1]}" \
  -resultBundlePath "$out/Browser.xcresult"
xcrun xcresulttool get test-results summary --path "$out/Browser.xcresult" > "$out/apple-summary.json"
python3 -c '
import json, sys
result = json.load(sys.stdin)
if result["passedTests"] != 1 or result["totalTestCount"] != 1:
    raise SystemExit("expected one completed browser test")' < "$out/apple-summary.json"
touch "$out/done"
wait "$backend"
backend=
