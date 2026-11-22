#!/bin/bash
set -euo pipefail
umask 077
[[ $# == 1 && ( $1 == backup || $1 == backup-blocked || $1 == running-firmware || $1 == firmware-candidate || $1 == recovery-chooser ) ]] || {
  echo 'Usage: run-backup-owned.sh backup|backup-blocked|running-firmware|firmware-candidate|recovery-chooser' >&2
  exit 2
}
scenario=$1
if [[ $scenario == running-firmware || $scenario == firmware-candidate ]]; then
  if [[ -n ${NU54_IMGTOOL_PYTHON:-} || -n ${NU54_IMGTOOL_SCRIPT:-} ]]; then
    [[ -x ${NU54_IMGTOOL_PYTHON:-} && -r ${NU54_IMGTOOL_SCRIPT:-} ]] || {
      echo "Set both NU54_IMGTOOL_PYTHON and NU54_IMGTOOL_SCRIPT" >&2; exit 2;
    }
  elif ! command -v imgtool >/dev/null; then
    echo "MCUboot imgtool is required" >&2; exit 2
  fi
fi
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
root=$(cd -- "$here/../../.." && pwd)
cd "$root"
temp=$(cd -- "${TMPDIR:-/tmp}" && pwd -P)
work=
sim=
phase=allocate
exec 9>&2
source "$root/scripts/ios/w0406-smoke/registration-test-build.sh"
simulator_inventory() {
  xcrun simctl list devices --json | python3 -B -c '
import json, re, sys
action, target = sys.argv[1:]
try:
    groups = json.load(sys.stdin)["devices"]
    if not isinstance(groups, dict) or not groups:
        raise ValueError("missing simulator inventory")
    devices = []
    for group in groups.values():
        if not isinstance(group, list):
            raise ValueError("invalid simulator group")
        for device in group:
            udid, name = device["udid"], device["name"]
            if not isinstance(name, str) or not re.fullmatch(r"[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}", udid, re.I):
                raise ValueError("invalid simulator entry")
            devices.append((udid, name))
    if len({udid.lower() for udid, _ in devices}) != len(devices):
        raise ValueError("duplicate simulator IDs")
except (KeyError, TypeError, ValueError):
    raise SystemExit(2)
matches = [udid for udid, name in devices if name == target]
if action == "find":
    if len(matches) == 1:
        print(matches[0])
    else:
        raise SystemExit(1 if matches else 3)
elif action == "free-name":
    raise SystemExit(1 if matches else 0)
elif action != "absent" or target.lower() in {udid.lower() for udid, _ in devices}:
    raise SystemExit(1)
' "$@"
}
clean() {
  local rc=$? absent=not-created attempt
  trap '' HUP TERM INT
  if ! registration_test_stop; then
    echo "Could not stop the owned W05 tools. Workspace retained at $work" >&9
    if [[ $rc == 0 ]]; then rc=1; fi
    exit "$rc"
  fi
  if [[ -n $sim ]]; then
    xcrun simctl shutdown "$sim" >/dev/null 2>&1 || :
    absent=false
    for attempt in 1 2 3; do
      xcrun simctl delete "$sim" >/dev/null 2>&1 || :
      if simulator_inventory absent "$sim"; then absent=true; break; fi
    done
    if [[ $absent != true ]]; then
      echo "Could not confirm removal of owned W05 simulator $sim" >&9
      if [[ $rc == 0 ]]; then rc=1; fi
    fi
  elif [[ ${sim_unknown:-0} == 1 ]]; then
    absent=unconfirmed
    echo "Could not identify the owned W05 simulator named $sim_name" >&9
  fi
  local results=native-not-completed
  if [[ -n $work && -f $work/evidence/results.json ]]; then
    results=$(python3 -B -c 'import json,sys;x=json.load(open(sys.argv[1]));print({k:x.get(k) for k in ("passedTests","failedTests","skippedTests","totalTestCount")})' "$work/evidence/results.json") || results=invalid-summary
  elif [[ -n $work && -f $work/evidence/xctest.log ]]; then
    results=$(python3 -B - "$work/evidence/xctest.log" <<'PY'
import re,sys
data = open(sys.argv[1], errors="replace").read()
print("safe_assertion_sites=" + ",".join(sorted(set(re.findall(r"SceneSmoke\.swift:[0-9]+", data)))))
PY
) || results=invalid-summary
  fi
  if [[ -n $work && -f $work/pins ]]; then
    if (cd "$root" && shasum -a 256 -c "$work/pins") >/dev/null; then echo 'input and generated pins matched' >&9; else echo 'input pins changed' >&9; [[ $rc != 0 ]] || rc=1; fi
  fi
  if [[ -n $work ]] && ! rm -rf -- "$work"; then
    if [[ $rc == 0 ]]; then rc=1; fi
  fi
  echo "W05 phase=$phase status=$rc sim_absent=$absent run_path_absent=$([[ -n $work && ! -e $work ]] && echo true || echo false) $results" >&9
  exit "$rc"
}
trap clean EXIT
trap 'registration_test_interrupt 129' HUP
trap 'registration_test_interrupt 130' INT
trap 'registration_test_interrupt 143' TERM
registration_test_transition=1
work=$(mktemp -d "$temp/w05.XXXXXXXX")
registration_test_checkpoint
echo "Owned W05 work: $work"
mkdir "$work/temp" "$work/prepare"
export TMPDIR="$work/temp" PYTHONDONTWRITEBYTECODE=1
unset NU54_W04_DERIVED_DATA NU54_TEST_RUNNING_FIRMWARE
phase=stage
shasum -a 256 products/p02-user-app/src/wallet/{WalletHome.tsx,walletEnglish.ts} \
  products/p01-device-firmware/{test/{owner_registration_host.c,test_accessory_board.py},core/src/nu54_hd_slot.c} \
  scripts/ios/{stage-project.sh,w0406-smoke/SceneSmoke.swift,w0406-smoke/run-registration.sh} >"$work/pins"
if [[ $scenario == recovery-chooser ]]; then
  shasum -a 256 products/p02-user-app/src/wallet/cloud/CloudWalletScreen.tsx     scripts/ios/w0406-smoke/{run-backup-owned.sh,fixture/Registration.mm} >>"$work/pins"
fi
find products/p02-user-app/ios/build/generated/ios/ReactCodegen -type f -name 'NU54RenterSpec*' -exec shasum -a 256 {} + >>"$work/pins"
if [[ $scenario == firmware-candidate ]]; then
  shasum -a 256 products/p02-user-app/src/wallet/{WalletMaintenance.tsx,firmwareCandidate.ts,firmwareCandidateDeployment.ts}     products/p10-platform/release/{evidence.py,fota_candidate.py,fota_descriptor.py}     products/p05-operations-backoffice/{cmd/verify-firmware/main.go,internal/commerce/firmware.go}     scripts/ios/w0406-smoke/fixture/candidate.sh products/p02-user-app/ios/NU54Renter/OwnerWallet.mm >>"$work/pins"
fi
source scripts/ios/test-environment.sh
source scripts/ios/stage-project.sh
ios_test_prepare_environment "$work/prepare"
scene_stage_ios_source "$root" "$work/source"
scene_stage_ios_project "$root" "$work/source" "$work" "$work/prepare" p02-user-app NU54RenterSpec
mkdir -p "$work/source/scripts/ios"
cp -R scripts/ios/w0406-smoke "$work/source/scripts/ios/w0406-smoke"
cp scripts/ios/test-environment.sh "$work/source/scripts/ios/"
phase=simulator
sim_name="NU54 W05 QA $(uuidgen)"
if ! simulator_inventory free-name "$sim_name"; then
  echo "Could not verify the W05 simulator inventory before creation" >&9
  exit 1
fi
registration_test_transition=1
created=0
candidate=$(
  trap '' HUP INT TERM
  exec xcrun simctl create "$sim_name" "${W05_DEVICE_TYPE:-com.apple.CoreSimulator.SimDeviceType.iPhone-17}" \
    "${W05_RUNTIME:-com.apple.CoreSimulator.SimRuntime.iOS-27-0}"
) || created=$?
if [[ $candidate =~ ^[0-9A-Fa-f]{8}(-[0-9A-Fa-f]{4}){3}-[0-9A-Fa-f]{12}$ ]]; then
  sim=$candidate
else
  found=0
  sim=$(trap '' HUP INT TERM; simulator_inventory find "$sim_name") || found=$?
  if [[ $found != 0 && $found != 3 ]] || [[ $found == 3 && $created == 0 ]]; then sim_unknown=1; fi
fi
registration_test_checkpoint
if [[ $created != 0 ]]; then exit "$created"; fi
if [[ -z $sim ]]; then exit 1; fi
echo "Owned W05 simulator: $sim"
phase=boot
registration_test_run xcrun simctl boot "$sim"
registration_test_run xcrun simctl bootstatus "$sim" -b >"$work/boot.log" 2>&1
phase=backup
registration_test_run "$work/source/scripts/ios/w0406-smoke/run-registration.sh" "$sim" "$work/evidence" "$scenario"
phase=verified
if [[ $scenario == backup-blocked ]]; then
  grep -c '^BACKUP_REFUSED ' "$work/evidence/host-transcript.log" | sed 's/^/authenticated_refusals=/'
  grep -c '^BACKUP_OPEN ' "$work/evidence/host-transcript.log" | sed 's/^/accepted_fresh_requests=/'
fi
if [[ $scenario == running-firmware || $scenario == firmware-candidate ]]; then
  grep -c '^RUNNING_FIRMWARE ' "$work/evidence/host-transcript.log" | sed 's/^/authenticated_running_image_reads=/'
fi
