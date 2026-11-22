#!/usr/bin/env bash
set -euo pipefail
umask 077

here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/../../.." && pwd)"
simulator="${1:?Pass a booted disposable iOS simulator UUID}"
out="${2:?Pass a new absolute result directory}"
scenario="${3:-policy}"
[[ "$scenario" = policy || "$scenario" = settings || "$scenario" = recipients || "$scenario" = caps ]]
[[ "$simulator" =~ ^[A-Fa-f0-9-]{36}$ && "$out" = /* ]]
mkdir "$out"
out="$(cd "$out" && pwd)"
bundle=com.nu54renter.w04qa
if xcrun simctl get_app_container "$simulator" "$bundle" app >/dev/null 2>&1; then
  echo "Use a fresh simulator without the registration fixture." >&2
  exit 1
fi

host_pid=""
button_pid=""
installed=0
cleanup() {
  rm -rf -- "$out/xcode-environment"
  if [[ -n "$host_pid" ]]; then kill "$host_pid" 2>/dev/null || true; fi
  if [[ -n "$button_pid" ]]; then kill "$button_pid" 2>/dev/null || true; fi
  if [[ $installed == 1 ]]; then xcrun simctl uninstall "$simulator" "$bundle" || true; fi
  rm -rf "$out/RegistrationFixture.app" "$out/NU54Renter.app" "$out/host-build" "$out/test-build" "$out/build"
  rm -f "$out/host/"*.key "$out/host/identity.pem" "$out/host/policy-personal.pem" "$out/host/policy-display-proof" "$out/host/policy-challenge" "$out/host/policy-private-input"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
source "$here/../test-environment.sh"
ios_test_prepare_environment "$out"

openssl_root="${NU54_OPENSSL_ROOT:-$(brew --prefix openssl@3)}"
openssl="$openssl_root/bin/openssl"
p01="$root/products/p01-device-firmware"
p02="$root/products/p02-user-app/ios"
build="${NU54_W27_DERIVED_DATA:-$out/build}"
cmake -S "$p01/test" -B "$out/host-build" -DOPENSSL_ROOT_DIR="$openssl_root" >"$out/host-build.log" 2>&1
cmake --build "$out/host-build" --target owner_registration_host owner_limits_host_bridge >>"$out/host-build.log" 2>&1
host="$out/host-build/owner_registration_host"
bridge="$out/host-build/owner_limits_host_bridge"

if [[ "${NU54_W27_REUSE_BUILD:-0}" = 1 ]]; then
  [[ -n "${NU54_W27_DERIVED_DATA:-}" ]]
else
  "${ios_test_env[@]}" xcodebuild -workspace "$p02/NU54Renter.xcworkspace" -scheme NU54Renter -configuration Release \
    -destination "platform=iOS Simulator,id=$simulator" -derivedDataPath "$build" \
    "NU54_IOS_BUNDLE=$bundle" CODE_SIGNING_ALLOWED=YES CODE_SIGN_IDENTITY=- \
    NU54_OWNER_REGISTRATION_QA_FLAGS=-DNU54_OWNER_REGISTRATION_QA=1 build >"$out/app-build.log" 2>&1
fi
app="$out/NU54Renter.app"
ditto "$build/Build/Products/Release-iphonesimulator/NU54Renter.app" "$app"
xcent="$build/Build/Intermediates.noindex/NU54Renter.build/Release-iphonesimulator/NU54Renter.build/NU54Renter.app-Simulated.xcent"
codesign -fs - --preserve-metadata=entitlements "$app"
codesign --verify --strict "$app"
LC_ALL=C grep -aq 'nu54.owner.registration.w04qa.p256' "$app/NU54Renter"

fixture="$out/RegistrationFixture.app"
mkdir "$fixture"
cp "$here/fixture/Info.plist" "$fixture/Info.plist"
plutil -replace CFBundleIdentifier -string "$bundle" "$fixture/Info.plist"
plutil -replace CFBundleName -string RegistrationFixture "$fixture/Info.plist"
plutil -replace UIApplicationSceneManifest.UISceneConfigurations.UIWindowSceneSessionRoleApplication.0.UISceneDelegateClassName \
  -string RegistrationFixtureScene "$fixture/Info.plist"
arch="$(uname -m)"
xcrun --sdk iphonesimulator clang++ -target "$arch-apple-ios17.0-simulator" -fobjc-arc \
  -framework UIKit -framework Security -DNU54_OWNER_REGISTRATION_QA=1 -I "$p02/NU54Renter" \
  -Wl,-sectcreate,__TEXT,__entitlements,"$xcent" "$here/fixture/Registration.mm" -o "$fixture/NU54Fixture"
codesign -fs - "$fixture"
xcrun simctl install "$simulator" "$fixture"
installed=1
documents="$(xcrun simctl get_app_container "$simulator" "$bundle" data)/Documents"
mkdir -p "$documents"
old_scope="$(uuidgen)"
printf '%s' "$old_scope" >"$documents/w04-key-scope"
wait_file() {
  for _ in {1..80}; do
    if [[ -f "$1" ]]; then return; fi
    sleep .25
  done
  echo "Missing fixture output: $1" >&2
  return 1
}

xcrun simctl launch "$simulator" "$bundle"
wait_file "$documents/w04-owner-point"
xcrun simctl terminate "$simulator" "$bundle"

mkdir "$out/host"
cd "$out/host"
before="$("$host" "$PWD" inspect)"
address="$(awk '{print $3}' <<<"$before")"
[[ "$before" = "VAULT 0 $address 0 $(printf '%064d' 0)" && "$address" =~ ^[a-f0-9]{40}$ ]]
device="$(printf %s "$address" | xxd -r -p | shasum -a 256 | awk '{print $1}')"

"$openssl" req -new -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -noenc -days 1 \
  -subj /CN=NU54-W27-Test-CA -keyout ca.key -out ca.pem 2>"$out/certificates.log"
"$openssl" req -new -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -noenc \
  -subj /CN=NU54-W27-Test-Board -keyout board.key -out board.csr 2>>"$out/certificates.log"
printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=serverAuth\nsubjectAltName=URI:urn:nu54:role:board\n' >board.ext
printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=clientAuth\nsubjectAltName=URI:urn:nu54:role:owner\n' >owner.ext
"$openssl" x509 -req -in board.csr -CA ca.pem -CAkey ca.key -CAcreateserial -extfile board.ext -days 1 \
  -out board.pem 2>>"$out/certificates.log"

# The simulator creates the phone key. The ephemeral CA receives only its public point.
{ printf %s 3059301306072a8648ce3d020106082a8648ce3d030107034200 | xxd -r -p; cat "$documents/w04-owner-point"; } >owner.der
"$openssl" pkey -pubin -inform DER -in owner.der -out owner.pub
"$openssl" x509 -new -force_pubkey owner.pub -CA ca.pem -CAkey ca.key -extfile owner.ext -days 1 \
  -subj /CN=NU54-W27-Test-Owner -out owner.pem 2>>"$out/certificates.log"
peer="$(shasum -a 256 owner.der | awk '{print $1}')"
pin="$("$openssl" x509 -in board.pem -pubkey -noout | "$openssl" pkey -pubin -outform DER | shasum -a 256 | awk '{print $1}')"


printf '%s\n' "$device" >policy-board
printf '%s\n' 500ef69da230e42bff34487b578ea0cbefbda2ba >policy-asset
"$openssl" genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out policy-display.key
"$openssl" pkey -in policy-display.key -pubout -out policy-display.pem
export NU54_W27_PANEL="$p01/test/w27_panel_fixture.py"
export PYTHONDONTWRITEBYTECODE=1
export NU54_W27_BRIDGE="$bridge"
# Model a separate board-input event after the display returns its proof.
(
  while :; do
    if [[ -s policy-display-proof && ! -e policy-sw1-event ]]; then
      printf 'SW1\n' >policy-sw1.tmp
      mv policy-sw1.tmp policy-sw1-event
    fi
    sleep .05
  done
) &
button_pid=$!
now="$(date +%s)"
if [[ "$scenario" = recipients || "$scenario" = caps ]]; then
  now="$((now - 2*86400 - 200))"
fi
init="$(python3 - <<'PYWIRE'
print((bytes([26]) + (8283).to_bytes(8, "big") + bytes.fromhex("500ef69da230e42bff34487b578ea0cbefbda2ba") + bytes([5])*20 +
       (10_000_000).to_bytes(8,"big") + (30_000_000).to_bytes(8,"big")).hex())
PYWIRE
)"
printf "26/%s/%s/%s" "$(printf 'bb%.0s' {1..16})" "$(printf 'aa%.0s' {1..16})" "$((now+110))" >policy-private-input
[[ "$(NU54_W27_PANEL_CLOCK="$now" "$bridge" policy-state "$init" "$((now+110))" "$(printf 'aa%.0s' {1..16})" "$now" "$device" "$(printf 'bb%.0s' {1..16})" "$(printf 'cc%.0s' {1..32})" 2>"$out/initial-panel.log")" = 1a01 ]]
if [[ "$scenario" = recipients || "$scenario" = caps ]]; then
  python3 "$here/setup-policy-set.py" seed "$bridge" "$device" "$scenario" >"$out/seed-policy.log" 2>&1
  if [[ "$scenario" = recipients ]]; then
    export NU54_W27_PANEL_REJECT_ONCE=recipients NU54_W27_DROP_REPLY=proposal
  else
    export NU54_W27_DROP_REPLY=activation
  fi
else
  export NU54_W27_LEGACY_ONLY=1
fi
"$host" "$PWD" serve 0 40 660 >"$out/policy-transcript.log" 2>"$out/panel-and-host.log" &
host_pid=$!
for _ in {1..60}; do
  port="$(awk '/^PORT / {print $2}' "$out/policy-transcript.log")"
  if [[ "$port" =~ ^[0-9]+$ ]]; then break; fi
  kill -0 "$host_pid"
  sleep .25
done
[[ "$port" =~ ^[0-9]+$ ]]
printf %s "$port" >"$documents/w04-host-port"
personal="$(awk '/^POLICY_WALLET / {print $2}' "$out/policy-transcript.log")"
[[ "$personal" =~ ^[a-f0-9]{40}$ && "$personal" != "$address" ]]
ca="$("$openssl" x509 -in ca.pem -outform DER | "$openssl" base64 -A)"
owner="$("$openssl" x509 -in owner.pem -outform DER | "$openssl" base64 -A)"
python3 - "$device" "$personal" "$pin" "$ca" "$owner" "$scenario" "$here" >"$documents/w04-enrollment.json" <<'PYPROFILE'
import importlib.util, json, sys
board, address, pin, ca, chain, scenario, here = sys.argv[1:]
token = "0x500ef69da230e42bff34487b578ea0cbefbda2ba"
profile = dict(version=2, exercised=False, mode="personal",
    verified=dict(boardFingerprint=board,address="0x"+address,network="8283",generation=1),
    enrollment=dict(version=2,device=board,boardSPKI=pin,ca=ca,certificateChain=[chain]),
    walletRead=dict(selection=dict(id="stable",account=0,asset=token,hidden=False),
        endpoints={},contacts=[],observations=[], assets=[dict(board=board,generation=1,id="stable",
        account=0,contract=token,decimals=6,block="0x"+"09"*32,height="1",favorite=False,hidden=False)]))
if scenario == "recipients":
    spec = importlib.util.spec_from_file_location("policy", here + "/setup-policy-set.py")
    policy = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(policy)
    profile.update(policy.profile(board, "0x" + address))
print(json.dumps(profile))
PYPROFILE
xcrun simctl launch "$simulator" "$bundle"
wait_file "$documents/w04-profile-ready"
xcrun simctl terminate "$simulator" "$bundle"
xcrun simctl install "$simulator" "$app"
test_id=SceneSmoke/OwnerPolicySmoke/testPolicyUsesIndependentApprovalAndDeviceTime
if [[ "$scenario" = settings ]]; then
  test_id=SceneSmoke/OwnerSettingsRoutesSmoke/testSettingsOpensRecoverySignInNetworksAndTravelConsent
elif [[ "$scenario" = recipients ]]; then
  test_id=SceneSmoke/OwnerPolicySmoke/testRecipientSetPersistsAcrossLostReply
elif [[ "$scenario" = caps ]]; then
  test_id=SceneSmoke/OwnerPolicySmoke/testCapEditKeepsEightRecipientsAndReservation
fi
TEST_RUNNER_NU54_W27_DISPLAY_TEST_INPUT="$out/host/policy-private-input" "${ios_test_env[@]}" xcodebuild test -project "$here/SceneSmoke.xcodeproj" -scheme SceneSmoke \
  -destination "platform=iOS Simulator,id=$simulator" -parallel-testing-enabled NO "ARCHS=$(uname -m)" \
  -derivedDataPath "$out/test-build" -resultBundlePath "$out/policy.xcresult" -collect-test-diagnostics never \
  "-only-testing:$test_id" >"$out/policy-xctest.log" 2>&1
if [[ "$scenario" = policy ]]; then
  grep -q '^POLICY_LEGACY_UNSUPPORTED ' "$out/policy-transcript.log"
  [[ "$(grep -c '^POLICY_REFUSED 24 8$' "$out/policy-transcript.log")" = 1 ]]
  [[ "$(grep -c '^POLICY_REPLY 24 1801' "$out/policy-transcript.log")" = 1 ]]
  [[ "$(grep -c '^POLICY_REPLY 25 1900$' "$out/policy-transcript.log")" = 2 ]]
  [[ "$(grep -c '^POLICY_REPLY 25 1901$' "$out/policy-transcript.log")" = 1 ]]
elif [[ "$scenario" = recipients || "$scenario" = caps ]]; then
  [[ "$(grep -c '^POLICY_REPLY 35 230200$' "$out/policy-transcript.log")" = 2 ]]
  [[ "$(grep -c '^POLICY_REPLY 35 230201$' "$out/policy-transcript.log")" = 1 ]]
  if [[ "$scenario" = recipients ]]; then
    [[ "$(grep -c '^POLICY_REFUSED 33 8$' "$out/policy-transcript.log")" = 1 ]]
    [[ "$(grep -c '^POLICY_DROPPED_REPLY 33$' "$out/policy-transcript.log")" = 1 ]]
    [[ "$(grep -c '^POLICY_REPLY 33 210201' "$out/policy-transcript.log")" = 1 ]]
  else
    [[ "$(grep -c '^POLICY_DROPPED_REPLY 35$' "$out/policy-transcript.log")" = 1 ]]
    [[ "$(grep -c '^POLICY_REPLY 34 220201' "$out/policy-transcript.log")" = 1 ]]
  fi
  python3 "$here/setup-policy-set.py" verify "$bridge" "$device" "$scenario" >"$out/final-policy.log"
fi
[[ "$("$host" "$PWD" inspect)" = "VAULT 1 $address 1 $peer" ]]
echo "W27 simulator policy passed. Evidence in $out"
