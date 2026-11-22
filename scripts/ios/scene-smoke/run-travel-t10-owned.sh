#!/usr/bin/env bash
# Reproduce registered T10 with an owned simulator and disposable build.
set -euo pipefail
umask 077
if [[ $# != 1 || ! $1 =~ ^[1-9][0-9]{3,4}$ ]] || (( $1 > 65533 )); then
  echo 'usage: run-travel-t10-owned.sh FIRST_LOOPBACK_PORT' >&2
  exit 2
fi
port=$1
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
source "$here/private-build.sh"
exec 8>&1 9>&2
export PYTHONDONTWRITEBYTECODE=1
sim=
phase=setup
scene_child=

cleanup_t10() {
  local status=$? removed=false attempt
  trap '' HUP INT TERM
  if [[ -n "$sim" ]]; then
    xcrun simctl shutdown "$sim" >/dev/null 2>&1 || :
    for attempt in 1 2 3; do
      xcrun simctl delete "$sim" >/dev/null 2>&1 || :
      if simulator_inventory absent "$sim"; then
        removed=true
        break
      fi
    done
    if [[ $removed != true ]]; then
      echo "could not confirm removal of owned T10 simulator $sim" >&9
      if ((status == 0)); then status=1; fi
    fi
  fi
  # An unconfirmed detached PG stop intentionally leaves that fixture's own
  # workspace for recovery. Preserve its recovery pointer, never its raw log.
  if [[ -s "$driver" ]]; then
    grep -E '^Retained integration workspace at (/private)?/tmp/nu54-private-ios-[A-Za-z0-9_-]+$' "$driver" >&9 || :
  fi
  if ! python3 -B - "$status" "$phase" "$removed" "$result" >&8 2>&9 <<'PY'
import json, re, sys
from pathlib import Path
status, phase, simulator = sys.argv[1:4]
result = Path(sys.argv[4])
report = {"testExit": int(status), "phase": phase, "ownedSimulatorRemoved": simulator == "true"}
summary = result / "test-summary.json"
if summary.is_file():
    data = json.loads(summary.read_text())
    report["xctest"] = {key: data[key] for key in
                        ("passedTests", "failedTests", "skippedTests", "totalTestCount", "seconds")
                        if key in data and type(data[key]) in (int, float)}
signed = result / "signed.sha256"
if signed.is_file():
    report["signedAppSHA256"] = re.findall(r"^[0-9a-f]{64}$", signed.read_text(), re.M)[:2]
counts = result / "db-check.log"
if counts.is_file() and re.fullmatch(r"[0-9]+(?:\|[0-9]+){5}", counts.read_text().strip()):
    report["privateWrites"] = [int(value) for value in counts.read_text().strip().split("|")]
assertions = result / "assertion-lines.txt"
if assertions.is_file():
    report["assertionLines"] = [int(line) for line in assertions.read_text().splitlines() if line.isdecimal()][:12]
if not int(status):
    assert report.get("xctest", {}).get("passedTests") == report["xctest"].get("totalTestCount") == 1
    assert len(report.get("signedAppSHA256", [])) == 2 and report.get("privateWrites") == [0] * 6
print(json.dumps(report, sort_keys=True))
PY
  then
    echo 'could not summarize the owned T10 run' >&9
    if ((status == 0)); then status=1; fi
  fi
  scene_cleanup "$status" 2>&9
}

interrupt_t10() {
  local status=$1
  trap '' HUP INT TERM
  if [[ $phase == acceptance && -n $scene_child ]]; then
    # The lower driver owns separate fixture groups and confirms PostgreSQL
    # shutdown itself. Wait for it rather than force-killing that supervisor.
    kill -TERM -- "-$scene_child" 2>/dev/null || :
    wait "$scene_child" 2>/dev/null || :
    scene_child=
  fi
  scene_interrupt "$status"
}

defer_t10() {
  pending=
  trap 'pending=${pending:-129}' HUP
  trap 'pending=${pending:-130}' INT
  trap 'pending=${pending:-143}' TERM
}

resume_t10() {
  trap 'interrupt_t10 "${pending:-129}"' HUP
  trap 'interrupt_t10 "${pending:-130}"' INT
  trap 'interrupt_t10 "${pending:-143}"' TERM
  if [[ -n $pending ]]; then interrupt_t10 "$pending"; fi
}

simulator_inventory() {
  xcrun simctl list devices --json | python3 -B -c '
import json, re, sys
action, target = sys.argv[1:]
try:
    groups = json.load(sys.stdin)["devices"]
    if not isinstance(groups, dict) or not groups:
        raise ValueError("missing device inventory")
    ids, matches = set(), []
    for devices in groups.values():
        if not isinstance(devices, list):
            raise ValueError("invalid simulator group")
        for device in devices:
            udid = device["udid"]
            if not re.fullmatch(r"[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}", udid, re.I):
                raise ValueError("invalid simulator ID")
            ids.add(udid.lower())
            if device.get("name") == target:
                matches.append(udid)
except (KeyError, TypeError, ValueError):
    raise SystemExit(1)
if action == "find" and len(matches) == 1:
    print(matches[0])
elif action != "absent" or target.lower() in ids:
    raise SystemExit(1)
' "$@"
}

run_t10() {
  local status=0
  defer_t10
  set -m
  "${ios_test_env[@]}" "$@" &
  scene_child=$!
  set +m
  resume_t10
  wait "$scene_child" || status=$?
  scene_child=
  return "$status"
}

# Keep both resource creators alive until their identifiers reach this shell.
defer_t10
temp=$(cd -- "${TMPDIR:-/tmp}" && pwd -P)
scene_work=$(trap '' HUP INT TERM; exec mktemp -d "$temp/nu54-scene.XXXXXXXX")
result="$scene_work/result"
driver="$scene_work/driver.log"
trap cleanup_t10 EXIT
resume_t10
ios_test_prepare_environment "$scene_work"

phase=simulator
sim_name="NU54 T10 QA $(uuidgen)"
defer_t10
created=0
candidate=$(
  trap '' HUP INT TERM
  exec xcrun simctl create "$sim_name" \
    "${T10_DEVICE_TYPE:-com.apple.CoreSimulator.SimDeviceType.iPhone-17-Pro}" \
    "${T10_RUNTIME:-com.apple.CoreSimulator.SimRuntime.iOS-27-0}"
) || created=$?
if [[ $candidate =~ ^[0-9A-Fa-f]{8}(-[0-9A-Fa-f]{4}){3}-[0-9A-Fa-f]{12}$ ]]; then
  sim=$candidate
else
  sim=$(trap '' HUP INT TERM; simulator_inventory find "$sim_name") || :
fi
resume_t10
if [[ -z $sim ]]; then
  echo "could not identify the T10 simulator named $sim_name" >&2
  if ((created == 0)); then created=1; fi
  exit "$created"
fi
if ((created != 0)); then exit "$created"; fi

phase=boot
run_t10 xcrun simctl boot "$sim" >"$scene_work/boot.log" 2>&1
run_t10 xcrun simctl bootstatus "$sim" -b >>"$scene_work/boot.log" 2>&1
phase=acceptance
run_t10 bash "$here/run-itinerary-conflict.sh" "$sim" "$scene_work/app-derived" "$result" "$port" --my-trip >"$driver" 2>&1
phase=complete
