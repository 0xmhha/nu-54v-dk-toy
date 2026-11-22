#!/usr/bin/env bash
set -euo pipefail
umask 077

here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/../../.." && pwd)"
simulator="${1:?Pass an explicitly selected, booted disposable simulator UUID}"
out="${2:?Pass a new absolute output directory}"
bundle=com.nu54renter.w04qa
scenario="${3:-registration}"
unset NU54_TEST_RUNNING_FIRMWARE
[[ "$scenario" = registration || "$scenario" = backup || "$scenario" = backup-blocked || "$scenario" = running-firmware || "$scenario" = firmware-candidate || "$scenario" = replacement || "$scenario" = board || "$scenario" = tools || "$scenario" = recovery-chooser ]]
if [[ "$scenario" = firmware-candidate && ( -z "${NU54_PICKUP_READONLY_ROOT:-}" || "$root" = "$NU54_PICKUP_READONLY_ROOT" ) ]]; then
  echo "Use the owned firmware-candidate entry point" >&2
  exit 2
fi
[[ "$simulator" =~ ^[A-Fa-f0-9-]{36}$ && "$out" = /* ]]
mkdir "$out"
out="$(cd "$out" && pwd)"
if xcrun simctl get_app_container "$simulator" "$bundle" app >/dev/null 2>&1; then
  echo "The dedicated registration bundle is already installed. Use a fresh simulator." >&2
  exit 1
fi

source "$here/registration-test-build.sh"
build="${NU54_W04_DERIVED_DATA:-$out/build}"

host_pid=""
other_host_pid=""
accessory_pid=""
installed=0
cleanup() {
  local status=$?
  trap '' HUP INT TERM
  if registration_test_cleanup; then
    if [[ -z "${NU54_W04_DERIVED_DATA:-}" ]]; then
      registration_release_clean_cache "$build" || { if [[ $status == 0 ]]; then status=1; fi; }
    fi
  else
    echo "Retained XCTest workspace: $registration_test_work" >&2
    if [[ $status == 0 ]]; then status=1; fi
  fi
  rm -rf -- "$out/xcode-environment"
  if [[ -n "$host_pid" ]]; then kill "$host_pid" 2>/dev/null || true; wait "$host_pid" 2>/dev/null || true; fi
  if [[ -n "$other_host_pid" ]]; then kill "$other_host_pid" 2>/dev/null || true; wait "$other_host_pid" 2>/dev/null || true; fi
  if [[ -n "$accessory_pid" ]]; then kill "$accessory_pid" 2>/dev/null || true; fi
  if [[ $installed == 1 ]]; then xcrun simctl uninstall "$simulator" "$bundle" || true; fi
  rm -f "$out/host/"*.key "$out/host/identity.pem" "$out/host/hd-seed" "$out/other-host/"*.key "$out/other-host/identity.pem"
  if [[ "$scenario" = tools ]]; then
    rm -rf -- "$out/host-build" "$out/RegistrationFixture.app"
  fi
  rm -f "$out/recovery-kit/"*.key "$out/recovery-kit/target-vault/hd-seed" "$out/new-board/"*.key \
    "$out/new-board/identity.pem" "$out/new-board/hd-seed"
  exit "$status"
}
trap cleanup EXIT
trap 'registration_test_interrupt 129' HUP
trap 'registration_test_interrupt 130' INT
trap 'registration_test_interrupt 143' TERM
registration_test_prepare
source "$here/../test-environment.sh"
ios_test_prepare_environment "$out"

openssl_root="${NU54_OPENSSL_ROOT:-$(brew --prefix openssl@3)}"
openssl="$openssl_root/bin/openssl"
p01="$root/products/p01-device-firmware"
p02="$root/products/p02-user-app/ios"
cmake -S "$p01/test" -B "$out/host-build" -DOPENSSL_ROOT_DIR="$openssl_root" >"$out/host-build.log" 2>&1
cmake --build "$out/host-build" --target owner_registration_host >>"$out/host-build.log" 2>&1
host="$out/host-build/owner_registration_host"
if [[ "$scenario" != registration && "$scenario" != running-firmware && "$scenario" != firmware-candidate && "$scenario" != recovery-chooser ]]; then
  cmake --build "$out/host-build" --target hd_accessory_board recovery-accessory >>"$out/host-build.log" 2>&1
fi

if [[ "$scenario" = running-firmware || "$scenario" = firmware-candidate ]]; then
  imgtool=()
  if [[ -n ${NU54_IMGTOOL_PYTHON:-} || -n ${NU54_IMGTOOL_SCRIPT:-} ]]; then
    [[ -x ${NU54_IMGTOOL_PYTHON:-} && -r ${NU54_IMGTOOL_SCRIPT:-} ]] || {
      echo "Set both NU54_IMGTOOL_PYTHON and NU54_IMGTOOL_SCRIPT" >&2; exit 2;
    }
    imgtool=("$NU54_IMGTOOL_PYTHON" "$NU54_IMGTOOL_SCRIPT")
  else
    imgtool=("$(command -v imgtool)") || { echo "MCUboot imgtool is required" >&2; exit 2; }
  fi
  running="$registration_test_work/running"
  mkdir "$running"
  export NU54_TEST_RUNNING_FIRMWARE="$running/signed.bin"
  printf 'disposable counted running-image fixture\n' >"$running/input.bin"
  {
    "${imgtool[@]}" keygen -k "$running/qa.key" -t ed25519
    "${imgtool[@]}" sign -k "$running/qa.key" --sha 512 --pad-header --align 4       -H 512 -S 65536 -v '2.3.258+65538' -s 512 "$running/input.bin" "$NU54_TEST_RUNNING_FIRMWARE"
    "${imgtool[@]}" verify -k "$running/qa.key" "$NU54_TEST_RUNNING_FIRMWARE"
  } >"$out/imgtool.log" 2>&1
  rm -f -- "$running/input.bin"
  if [[ "$scenario" = running-firmware ]]; then rm -f -- "$running/qa.key"; fi
  running_measurement=$(python3 -B - "$NU54_TEST_RUNNING_FIRMWARE" <<'PYIMAGE'
import hashlib, struct, sys
from pathlib import Path
image = Path(sys.argv[1]).read_bytes()
header, protected, payload = struct.unpack_from("<HHI", image, 8)
assert header + protected + payload < len(image)
print(hashlib.sha256(image[:header + protected + payload]).hexdigest())
PYIMAGE
)
fi

mkdir "$out/host"
before="$("$host" "$out/host" inspect)"
address="$(awk '{print $3}' <<<"$before")"
[[ "$before" = "VAULT 0 $address 0 $(printf '%064d' 0)" && "$address" =~ ^[a-f0-9]{40}$ ]]
device="$(printf %s "$address" | xxd -r -p | shasum -a 256 | awk '{print $1}')"

qa_flags=-DNU54_OWNER_REGISTRATION_QA=1
if [[ "$scenario" = firmware-candidate ]]; then
  qa_flags+=" -DNU54_FIRMWARE_CANDIDATE_QA=1"
  /usr/libexec/PlistBuddy -c 'Add :UIFileSharingEnabled bool true' "$p02/NU54Renter/Info.plist"
  /usr/libexec/PlistBuddy -c 'Add :LSSupportsOpeningDocumentsInPlace bool true' "$p02/NU54Renter/Info.plist"
  source "$here/fixture/candidate.sh"
  candidate_prepare >"$out/candidate-build.log" 2>&1
fi

registration_test_run xcodebuild -workspace "$p02/NU54Renter.xcworkspace" -scheme NU54Renter -configuration Release \
  -destination "platform=iOS Simulator,id=$simulator" -derivedDataPath "$build" \
  "NU54_IOS_BUNDLE=$bundle" CODE_SIGNING_ALLOWED=YES CODE_SIGN_IDENTITY=- \
  "NU54_OWNER_REGISTRATION_QA_FLAGS=$qa_flags" build >"$out/app-build.log" 2>&1
app="$build/Build/Products/Release-iphonesimulator/NU54Renter.app"
xcent="$build/Build/Intermediates.noindex/NU54Renter.build/Release-iphonesimulator/NU54Renter.build/NU54Renter.app-Simulated.xcent"
LC_ALL=C grep -aq 'nu54.owner.registration.w04qa.p256' "$app/NU54Renter"

run_recovery_chooser() {
  local phase=$1
  registration_test_run xcodebuild test -project "$here/SceneSmoke.xcodeproj" -scheme SceneSmoke \
    -destination "platform=iOS Simulator,id=$simulator" -parallel-testing-enabled NO \
    -derivedDataPath "$registration_test_work/test-build" -resultBundlePath "$out/$phase.xcresult" \
    -collect-test-diagnostics never "-only-testing:SceneSmoke/OwnerRecoveryChooserSmoke/$phase" \
    >"$out/xctest.log" 2>&1
  xcrun xcresulttool get test-results summary --path "$out/$phase.xcresult" >"$out/$phase.json"
  python3 -B - "$out/$phase.json" <<'PYRESULT'
import json, sys
with open(sys.argv[1]) as stream:
    result = json.load(stream)
assert (result["passedTests"], result["failedTests"], result["skippedTests"]) == (1, 0, 0)
PYRESULT
  grep -E "^Test Case '.*OwnerRecoveryChooserSmoke $phase.*' passed" "$out/xctest.log"
}
if [[ "$scenario" = recovery-chooser ]]; then
  xcrun simctl install "$simulator" "$app"
  installed=1
  run_recovery_chooser testFreshProfileShowsAllRecoveryChoices
  xcrun simctl terminate "$simulator" "$bundle" 2>/dev/null || :
  # Add only a public configuration to a disposable copy of the same build.
  # These fixture realms are never opened during the navigation test.
  supported="$out/CloudRecoveryFixture.app"
  ditto "$app" "$supported"
  cloud_profile=$(printf '%s' '{"api":"https://wallet.example","identity":"https://identity.example","approval":"https://approve.example","recovery":"https://recover.example"}' | base64 | tr -d '\n')
  plutil -replace NU54P03Profile -string "$cloud_profile" "$supported/Info.plist"
  codesign --force --sign - --preserve-metadata=entitlements "$supported"
fi

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
if [[ "$scenario" = recovery-chooser ]]; then : >"$documents/w06-check-empty"; fi
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

cd "$out/host"

"$openssl" req -new -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -noenc -days 1 \
  -subj /CN=NU54-W04-Test-CA -keyout ca.key -out ca.pem 2>"$out/certificates.log"
"$openssl" req -new -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -noenc \
  -subj /CN=NU54-W04-Test-Board -keyout board.key -out board.csr 2>>"$out/certificates.log"
printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=serverAuth\nsubjectAltName=URI:urn:nu54:role:board\n' >board.ext
printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=clientAuth\nsubjectAltName=URI:urn:nu54:role:owner\n' >owner.ext
"$openssl" x509 -req -in board.csr -CA ca.pem -CAkey ca.key -CAcreateserial -extfile board.ext -days 1 \
  -out board.pem 2>>"$out/certificates.log"

# The simulator creates the phone key. The ephemeral CA receives only its public point.
{ printf %s 3059301306072a8648ce3d020106082a8648ce3d030107034200 | xxd -r -p; cat "$documents/w04-owner-point"; } >owner.der
"$openssl" pkey -pubin -inform DER -in owner.der -out owner.pub
"$openssl" x509 -new -force_pubkey owner.pub -CA ca.pem -CAkey ca.key -extfile owner.ext -days 1 \
  -subj /CN=NU54-W04-Test-Owner -out owner.pem 2>>"$out/certificates.log"
peer="$(shasum -a 256 owner.der | awk '{print $1}')"
pin="$("$openssl" x509 -in board.pem -pubkey -noout | "$openssl" pkey -pubin -outform DER | shasum -a 256 | awk '{print $1}')"

# Same advertised name and issuing CA, distinct board TLS key and durable identity.
other="$out/other-host"
mkdir "$other"
cp ca.pem owner.pem "$other"
"$openssl" req -new -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -noenc \
  -subj /CN=NU54-W04-Test-Board -keyout "$other/board.key" -out "$other/board.csr" 2>>"$out/certificates.log"
"$openssl" x509 -req -in "$other/board.csr" -CA ca.pem -CAkey ca.key -extfile board.ext -days 1 \
  -out "$other/board.pem" 2>>"$out/certificates.log"
other_before="$("$host" "$other" inspect)"
[[ "$other_before" != "$before" ]]

wait_port() {
  local logfile="$1" pid="$2" port
  for _ in {1..40}; do
    port="$(awk '/^PORT / {print $2}' "$logfile")"
    if [[ "$port" =~ ^[0-9]+$ ]]; then printf '%s' "$port"; return; fi
    kill -0 "$pid"
    sleep .25
  done
  echo "Missing host port in $logfile" >&2
  return 1
}
host_args=("$PWD" serve 0 4 300)
if [[ "$scenario" = backup-blocked ]]; then
  host_args=("$PWD" serve 0 5 300 backup-blocked)
elif [[ "$scenario" = firmware-candidate ]]; then
  host_args=("$PWD" serve 0 5 300 backup)
elif [[ "$scenario" = running-firmware ]]; then
  host_args=("$PWD" serve 0 3 300 backup)
elif [[ "$scenario" != registration && "$scenario" != recovery-chooser ]]; then
  host_args+=(backup)
fi
"$host" "${host_args[@]}" >"$out/host-transcript.log" 2>"$out/host-errors.log" &
host_pid=$!
"$host" "$other" serve 0 1 300 >"$out/other-host-transcript.log" 2>"$out/other-host-errors.log" &
other_host_pid=$!
port="$(wait_port "$out/host-transcript.log" "$host_pid")"
other_port="$(wait_port "$out/other-host-transcript.log" "$other_host_pid")"
printf %s "$port" >"$documents/w04-host-port"
printf %s "$other_port" >"$documents/w04-other-host-port"
ca="$("$openssl" x509 -in ca.pem -outform DER | "$openssl" base64 -A)"
owner="$("$openssl" x509 -in owner.pem -outform DER | "$openssl" base64 -A)"
printf '{"version":2,"exercised":false,"mode":"personal","enrollment":{"version":2,"device":"%s","boardSPKI":"%s","ca":"%s","certificateChain":["%s"]}}' \
  "$device" "$pin" "$ca" "$owner" >"$documents/w04-enrollment.json"
if [[ "$scenario" = replacement ]]; then cp "$documents/w04-enrollment.json" "$out/old-enrollment.json"; fi
xcrun simctl launch "$simulator" "$bundle"
wait_file "$documents/w04-profile-ready"
xcrun simctl terminate "$simulator" "$bundle"
if [[ "$scenario" = recovery-chooser ]]; then
  recovery_baseline="$registration_test_work/w06-original-profile.json"
  cp "$documents/w04-enrollment.json" "$recovery_baseline"
  xcrun simctl install "$simulator" "$supported"
  run_recovery_chooser testSettingsReturnsWithOriginalWalletAndCloudEntry
  xcrun simctl terminate "$simulator" "$bundle" 2>/dev/null || :
  xcrun simctl install "$simulator" "$fixture"
  documents="$(xcrun simctl get_app_container "$simulator" "$bundle" data)/Documents"
  mkdir -p "$documents"
  : >"$documents/w06-check-profile"
  xcrun simctl launch "$simulator" "$bundle"
  wait_file "$documents/w06-wallet-observed"
  cmp "$recovery_baseline" "$documents/w06-wallet-observed"
  [[ "$(wc -l <"$out/host-transcript.log" | tr -d ' ')" = 1 ]]
  [[ "$(wc -l <"$out/other-host-transcript.log" | tr -d ' ')" = 1 ]]
  [[ "$("$host" "$PWD" inspect)" = "$before" ]]
  [[ "$("$host" "$other" inspect)" = "$other_before" ]]
  printf '{"passedTests":2,"failedTests":0,"skippedTests":0,"totalTestCount":2}\n' >"$out/results.json"
  printf 'W06_RECOVERY_CHOOSER fresh=pass settings=pass board_commands=0 wallet_profile_unchanged=true\n'
  exit 0
fi

xcrun simctl install "$simulator" "$app"

test_project="$here/SceneSmoke.xcodeproj"
test_options=()
test_filter=SceneSmoke/OwnerRegistrationSmoke/testRegistrationCommitWithoutReplyResumesAfterRestart
if [[ "$scenario" = running-firmware ]]; then
  test_filter=SceneSmoke/OwnerFirmwareSmoke/testRunningVersionIsReadFromSignedImage
elif [[ "$scenario" = firmware-candidate ]]; then
  documents="$(xcrun simctl get_app_container "$simulator" "$bundle" data)/Documents"
  mkdir -p "$documents"
  candidate_verify_and_stage
  test_options+=(SWIFT_ACTIVE_COMPILATION_CONDITIONS=NU54_CANDIDATE_FIXTURE)
  test_filter=SceneSmoke/OwnerFirmwareSmoke/testCandidateIsVerifiedForTheCurrentBoard
elif [[ "$scenario" != registration ]]; then
  test_filter=SceneSmoke/OwnerBackupSmoke/testIndependentBackupUnlocksOnlyTheRestoredAddress
  if [[ "$scenario" = backup-blocked ]]; then
    test_filter=SceneSmoke/OwnerBackupSmoke/testAuthenticatedBackupRefusalCanRetrySameOriginal
  fi
  controller_args=("$out/host-build/hd_accessory_board" "$out/host-build/recovery-accessory" \
    "$PWD" "$out/host-transcript.log" "$out/backup-result.json")
  if [[ "$scenario" = board ]]; then controller_args+=("$out/recovery-kit"); fi
  OPENSSL="$openssl" python3 "$p01/test/test_accessory_board.py" "${controller_args[@]}" >"$out/accessory.log" 2>&1 &
  accessory_pid=$!
fi
registration_test_run xcodebuild test -project "$test_project" -scheme SceneSmoke \
  ${test_options[@]+"${test_options[@]}"} \
  -destination "platform=iOS Simulator,id=$simulator" -parallel-testing-enabled NO \
  -derivedDataPath "$registration_test_work/test-build" -resultBundlePath "$out/result.xcresult" -collect-test-diagnostics never \
  "-only-testing:$test_filter" \
  >"$out/xctest.log" 2>&1
xcrun xcresulttool get test-results summary --path "$out/result.xcresult" >"$out/results.json"
python3 - "$out/results.json" <<'PY'
import json, sys
with open(sys.argv[1]) as stream:
    result = json.load(stream)
assert (result["passedTests"], result["failedTests"], result["skippedTests"]) == (1, 0, 0)
PY
wait "$host_pid"
host_pid=""
if [[ "$scenario" = running-firmware || "$scenario" = firmware-candidate ]]; then
  [[ "$("$host" "$PWD" inspect)" = "VAULT 1 $address 0 $peer" ]]
  reads=1
  if [[ "$scenario" = firmware-candidate ]]; then reads=3; fi
  [[ "$(grep -c '^RUNNING_FIRMWARE ' "$out/host-transcript.log")" = "$reads" ]]
  grep -qx "RUNNING_FIRMWARE 1 $address 0 $peer" "$out/host-transcript.log"
  grep -qx "RUNNING_IMAGE_MEASUREMENT $running_measurement" "$out/host-transcript.log"
  echo "Signed fixture measurement matched the authenticated board response"
  echo "W12 simulator read the confirmed running firmware"
  if [[ "$scenario" = firmware-candidate ]]; then
    echo "W12 simulator verified a separately signed custom candidate and held application"
  fi
  exit 0
fi
if [[ "$scenario" != registration ]]; then
  wait "$accessory_pid"
  accessory_pid=""
  [[ "$("$host" "$PWD" inspect)" = "VAULT 1 $address 0 $peer" ]]
  [[ "$(grep -c '^BACKUP_OPEN ' "$out/host-transcript.log")" = 2 ]]
  [[ "$(grep -c '^PERSONAL_ADDRESS ' "$out/host-transcript.log")" = 1 ]]
  xcrun xcresulttool export attachments --path "$out/result.xcresult" --output-path "$out/attachments" >/dev/null
  python3 - "$out" <<'PYRESULT'
import json
from pathlib import Path
import sys
out = Path(sys.argv[1])
address = json.loads((out / "backup-result.json").read_text())["address"].encode()
assert any(path.suffix != ".png" and address in path.read_bytes() for path in (out / "attachments").iterdir())
PYRESULT
  echo "W05 simulator backup passed. Evidence in $out"
  if [[ "$scenario" = backup-blocked ]]; then
    awk -v original="$address" -v owner="$peer" '
      /^BACKUP_(REFUSED|OPEN) / { if ($2 != 1 || $3 != original || $4 != 0 || $5 != owner) exit 1 }
      /^BACKUP_REFUSED / { if (opened || ++refused != 1) exit 1 }
      /^BACKUP_OPEN / { if (refused != 1) exit 1; opened++ }
      END { if (refused != 1 || opened != 2) exit 1 }
    ' "$out/host-transcript.log"
  fi
  if [[ "$scenario" = backup || "$scenario" = backup-blocked ]]; then exit 0; fi

  if [[ "$scenario" = tools ]]; then
    "$host" "$PWD" serve 0 4 300 backup >"$out/tools-transcript.log" 2>"$out/tools-host-errors.log" &
    host_pid=$!
    port="$(wait_port "$out/tools-transcript.log" "$host_pid")"
    documents="$(xcrun simctl get_app_container "$simulator" "$bundle" data)/Documents"
    mkdir -p "$documents"
    printf %s "$port" >"$documents/w04-host-port"
    registration_test_run xcodebuild test -project "$here/SceneSmoke.xcodeproj" -scheme SceneSmoke \
      -destination "platform=iOS Simulator,id=$simulator" -parallel-testing-enabled NO \
      -derivedDataPath "$registration_test_work/test-build" -resultBundlePath "$out/tools.xcresult" -collect-test-diagnostics never \
      -only-testing:SceneSmoke/OwnerToolsSmoke/testToolsUseAuthenticatedBoardAndExplicitCancel >"$out/tools-xctest.log" 2>&1
    wait "$host_pid"
    host_pid=""
    for event in TOOL_READ TOOL_FIND TOOL_CANCEL; do
      [[ "$(grep -c "^$event 1 " "$out/tools-transcript.log")" = 1 ]]
    done
    [[ "$("$host" "$PWD" inspect)" = "VAULT 1 $address 0 $peer" ]]
    echo "W12 simulator owner tools passed. Evidence in $out"
    exit 0
  fi

  start_phone_fixture() {
    local scope="$1"
    xcrun simctl terminate "$simulator" "$bundle" 2>/dev/null || true
    xcrun simctl install "$simulator" "$fixture"
    documents="$(xcrun simctl get_app_container "$simulator" "$bundle" data)/Documents"
    mkdir -p "$documents"
    rm -f "$documents/w04-enrollment.json" "$documents/w04-owner-point" "$documents/w04-profile-ready"
    printf '%s' "$scope" >"$documents/w04-key-scope"
    printf '%s' "$other_port" >"$documents/w04-other-host-port"
    xcrun simctl launch "$simulator" "$bundle"
    wait_file "$documents/w04-owner-point"
    xcrun simctl terminate "$simulator" "$bundle"
  }
  provision_phone_fixture() {
    xcrun simctl launch "$simulator" "$bundle"
    wait_file "$documents/w04-profile-ready"
    xcrun simctl terminate "$simulator" "$bundle"
    xcrun simctl install "$simulator" "$app"
  }
  if [[ "$scenario" = board ]]; then
    kit="$out/recovery-kit"
    target="$out/new-board"
    mkdir "$target"
    cp ca.pem owner.pem "$target"
    "$openssl" req -new -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -noenc \
      -subj /CN=NU54-W06-New-Board -keyout "$target/board.key" -out "$target/board.csr" 2>>"$out/certificates.log"
    "$openssl" x509 -req -in "$target/board.csr" -CA ca.pem -CAkey ca.key -extfile board.ext -days 1 \
      -out "$target/board.pem" 2>>"$out/certificates.log"
    backup_address="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["address"])' "$out/backup-result.json")"
    target_identity="$("$host" "$target" inspect | awk '{print $3}')"
    target_device="$(printf %s "$target_identity" | xxd -r -p | shasum -a 256 | awk '{print $1}')"
    target_pin="$("$openssl" x509 -in "$target/board.pem" -pubkey -noout | "$openssl" pkey -pubin -outform DER | shasum -a 256 | awk '{print $1}')"
    [[ "$target_device" != "$device" && "$target_pin" != "$pin" ]]
    cp "$target/board.pem" "$kit/target.pem"
    mv "$kit/proof.bin" "$kit/trial-proof.bin"
    start_phone_fixture "$old_scope"
    printf '{"version":2,"exercised":false,"mode":"personal","enrollment":{"version":2,"device":"%s","boardSPKI":"%s","ca":"%s","certificateChain":["%s"]}}' \
      "$target_device" "$target_pin" "$ca" "$owner" >"$documents/w04-enrollment.json"
    "$host" "$target" serve 0 3 300 restore-target >"$out/board-transcript.log" 2>"$out/board-errors.log" &
    host_pid=$!
    port="$(wait_port "$out/board-transcript.log" "$host_pid")"
    printf %s "$port" >"$documents/w04-host-port"
    kill "$other_host_pid" 2>/dev/null || true
    wait "$other_host_pid" 2>/dev/null || true
    "$host" "$other" serve 0 1 300 >"$out/board-other-transcript.log" 2>"$out/board-other-errors.log" &
    other_host_pid=$!
    other_port="$(wait_port "$out/board-other-transcript.log" "$other_host_pid")"
    printf %s "$other_port" >"$documents/w04-other-host-port"
    touch "$documents/w04-reverse-scan"
    OPENSSL="$openssl" python3 "$p01/test/test_accessory_board.py" "$out/host-build/hd_accessory_board" \
      "$out/host-build/recovery-accessory" restore "$kit" "$target" "$out/board-transcript.log" "$device" "$backup_address" \
      >"$out/board-accessory.log" 2>&1 &
    accessory_pid=$!
    provision_phone_fixture

    project="$out/board-test-project"
    mkdir "$project"
    cp -R "$here/SceneSmoke.xcodeproj" "$project/"
    cp "$here/SceneSmoke.swift" "$project/SceneSmoke.swift"
    python3 - "$openssl" "$kit" "$device" "$target_device" "$backup_address" >>"$project/SceneSmoke.swift" <<'PYTICKET'
import hashlib
import json
from pathlib import Path
import subprocess
import sys
tool, kit, source, target, address = sys.argv[1:]
keys = {}
for role, field in (("source", "original"), ("target", "target"), ("accessory", "accessory")):
    cert = Path(kit) / f"{role}.pem"
    pem = subprocess.check_output([tool, "x509", "-in", cert, "-noout", "-pubkey"])
    keys[field] = subprocess.check_output([tool, "pkey", "-pubin", "-outform", "DER"], input=pem)
document = dict(source=source, targetBoard=target, sourceGeneration=1,
                **{field: key.hex() for field, key in keys.items()})
print("private let w06TrustDocument = " + json.dumps(json.dumps(document, separators=(",", ":"))))
print("private let w06TrustPins = " + json.dumps([hashlib.sha256(key).hexdigest() for key in keys.values()]))
print("private let w06ExpectedAddress = " + json.dumps(address))
PYTICKET
    registration_test_run xcodebuild test -project "$project/SceneSmoke.xcodeproj" -scheme SceneSmoke \
      -destination "platform=iOS Simulator,id=$simulator" -parallel-testing-enabled NO \
      -derivedDataPath "$registration_test_work/board-test-build" -resultBundlePath "$out/board-restore.xcresult" \
      -collect-test-diagnostics never SWIFT_ACTIVE_COMPILATION_CONDITIONS=NU54_BOARD_RESTORE_TICKET \
      -only-testing:SceneSmoke/OwnerBoardRestoreSmoke/testNewBoardRequiresBoundReceiptAndPreservesAReadyRoot \
      >"$out/board-restore.log" 2>&1
    wait "$accessory_pid"
    accessory_pid=""
    wait "$host_pid"
    host_pid=""
    xcrun xcresulttool get test-results summary --path "$out/board-restore.xcresult" >"$out/board-restore.json"
    python3 - "$out/board-restore.json" <<'PYRESTORE'
import json, sys
with open(sys.argv[1]) as stream:
    result = json.load(stream)
assert (result["passedTests"], result["failedTests"], result["skippedTests"]) == (1, 0, 0)
PYRESTORE
    [[ "$("$host" "$PWD" inspect)" = "VAULT 1 $address 0 $peer" ]]
    [[ "$("$host" "$target" inspect)" = "VAULT 1 $target_identity 0 $peer" ]]
    [[ "$("$host" "$other" inspect)" = "$other_before" ]]
    [[ "$(cat "$out/board-other-transcript.log")" = "PORT $other_port" ]]
    grep -q '^TLS handshake failed$' "$out/board-other-errors.log"
    [[ "$(grep -c '^W06_WRONG_CANDIDATE_BLOCKED$' "$out/board-restore.log")" = 1 ]]
    [[ "$(grep -c '^RESTORE_OPEN 1 ' "$out/board-transcript.log")" = 1 ]]
    [[ "$(grep -c '^RESTORE_REFUSED 1 ' "$out/board-transcript.log")" = 1 ]]
    [[ "$(grep -c '^RESTORE_RECEIPT 1 ' "$out/board-transcript.log")" = 2 ]]
    [[ "$(grep -c '^PERSONAL_ADDRESS 1 ' "$out/board-transcript.log")" = 1 ]]
    for slot in "$PWD" "$target"; do
      [[ "0x$("$out/host-build/hd_accessory_board" "$slot" inspect | xxd -p)" = "$backup_address" ]]
    done
    echo "W06 simulator board restore passed. Evidence in $out"
    exit 0
  fi

  start_phone_fixture "$(uuidgen)"
  { printf %s 3059301306072a8648ce3d020106082a8648ce3d030107034200 | xxd -r -p; cat "$documents/w04-owner-point"; } >replacement.der
  "$openssl" pkey -pubin -inform DER -in replacement.der -out replacement.pub
  "$openssl" x509 -new -force_pubkey replacement.pub -CA ca.pem -CAkey ca.key -extfile owner.ext -days 1 \
    -subj /CN=NU54-W06-Test-Replacement -out replacement.pem 2>>"$out/certificates.log"
  replacement_spki="$(shasum -a 256 replacement.der | awk '{print $1}')"
  [[ "$replacement_spki" != "$peer" ]]
  for role in replacement-verifier wrong-verifier; do
    "$openssl" genpkey -algorithm EC -pkeyopt ec_paramgen_curve:prime256v1 -out "$role.key"
  done
  "$openssl" pkey -in replacement-verifier.key -pubout -out replacement-verifier.pem
  replacement_cert="$("$openssl" x509 -in replacement.pem -outform DER | "$openssl" base64 -A)"
  printf '{"version":2,"exercised":false,"mode":"personal","enrollment":{"version":2,"device":"%s","boardSPKI":"%s","ca":"%s","certificateChain":["%s"]}}' \
    "$device" "$pin" "$ca" "$replacement_cert" >"$documents/w04-enrollment.json"

  "$host" "$PWD" serve 0 5 350 replacement >"$out/replacement-transcript.log" 2>"$out/replacement-errors.log" &
  host_pid=$!
  port="$(wait_port "$out/replacement-transcript.log" "$host_pid")"
  printf %s "$port" >"$documents/w04-host-port"
  python3 "$p01/test/test_replacement_panel.py" "$PWD" "$device" "$replacement_spki" >"$out/replacement-panel.log" 2>&1 &
  accessory_pid=$!
  provision_phone_fixture
  run_replacement_test() {
    local function="$1" result="$2"
    registration_test_run xcodebuild test -project "$here/SceneSmoke.xcodeproj" -scheme SceneSmoke \
      -destination "platform=iOS Simulator,id=$simulator" -parallel-testing-enabled NO \
      -derivedDataPath "$registration_test_work/test-build" -resultBundlePath "$out/$result.xcresult" -collect-test-diagnostics never \
      "-only-testing:SceneSmoke/OwnerReplacementSmoke/$function" >"$out/$result.log" 2>&1
    xcrun xcresulttool get test-results summary --path "$out/$result.xcresult" >"$out/$result.json"
    python3 - "$out/$result.json" <<'PYCHECK'
import json, sys
with open(sys.argv[1]) as stream:
    result = json.load(stream)
assert (result["passedTests"], result["failedTests"], result["skippedTests"]) == (1, 0, 0)
PYCHECK
  }
  run_replacement_test testPhoneReplacementResumesAtNewGenerationWithSameAddress replacement
  wait "$accessory_pid"
  accessory_pid=""
  start_phone_fixture "$old_scope"
  cp "$out/old-enrollment.json" "$documents/w04-enrollment.json"
  printf '%s' "$port" >"$documents/w04-host-port"
  provision_phone_fixture
  run_replacement_test testRetiredPhoneCannotReadActiveOwnerStatus retired-phone
  wait "$host_pid"
  host_pid=""
  [[ "$("$host" "$PWD" inspect)" = "VAULT 2 $address 0 $replacement_spki" ]]
  [[ "$(grep -c '^REPLACEMENT_REFUSED 1 ' "$out/replacement-transcript.log")" = 1 ]]
  [[ "$(grep -c '^REPLACEMENT_COMMITTED_REPLY_DROPPED 2 ' "$out/replacement-transcript.log")" = 1 ]]
  [[ "$(grep -c '^OLD_OWNER_REFUSED 2 ' "$out/replacement-transcript.log")" = 1 ]]
  mkdir "$out/replacement-attachments"
  xcrun xcresulttool export attachments --path "$out/replacement.xcresult" --output-path "$out/replacement-attachments" >/dev/null
  python3 - "$out" <<'PYADDRESS'
import json
from pathlib import Path
import sys
out = Path(sys.argv[1])
address = json.loads((out / "backup-result.json").read_text())["address"].encode()
assert any(address in path.read_bytes() for path in (out / "replacement-attachments").iterdir())
PYADDRESS
  echo "W06 simulator phone replacement passed. Evidence in $out"
  exit 0
fi
wait "$other_host_pid"
other_host_pid=""
[[ "$("$host" "$other" inspect)" = "$other_before" ]]
[[ "$(cat "$out/other-host-transcript.log")" = "PORT $other_port" ]]
grep -q '^TLS handshake failed$' "$out/other-host-errors.log"
[[ "$("$host" "$PWD" inspect)" = "VAULT 1 $address 0 $peer" ]]
[[ "$(find . -name identity.pem | wc -l | tr -d ' ')" = 1 ]]
[[ ! -e record-1 ]]
diff -u <(printf 'PORT %s\nSTATUS 0 %s 0 %064d\nBEGIN 0 %s 0 %064d\nDROPPED_BEFORE_CONFIRMATION 0 %s 0 %064d\nSTATUS 0 %s 0 %064d\nBEGIN 0 %s 0 %064d\nDROPPED_COMMITTED_REPLY 1 %s 0 %s\nSTATUS 1 %s 0 %s\nSTATUS 1 %s 0 %s\n' \
  "$port" "$address" 0 "$address" 0 "$address" 0 "$address" 0 "$address" 0 "$address" "$peer" "$address" "$peer" "$address" "$peer") "$out/host-transcript.log"
echo "W04 simulator registration passed. Evidence in $out"
