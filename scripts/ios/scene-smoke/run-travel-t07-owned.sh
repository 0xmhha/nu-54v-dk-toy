#!/usr/bin/env bash
# Own the device and disposable results for one travel payment acceptance.
set -euo pipefail
umask 077
mode=--t07-deposit
journey=T07
case ${1-} in
  --full) mode=--t07-full; shift ;;
  --pickup) mode=--t06; journey=T06; shift ;;
esac
[[ $# == 1 && $1 == /* && -x $1 ]] || {
  echo 'usage: run-travel-t07-owned.sh [--full|--pickup] ABS_DISPLAY_PYTHON' >&2
  exit 2
}
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
source "$here/private-build.sh"
exec 8>&1 9>&2
scene_prepare
export TMPDIR="$scene_work/xcode-environment/tmp"
sim=
phase=simulator
output="$scene_work/result"
cleanup() {
  local code=$? attempt listing removed=0
  trap '' HUP INT TERM
  if [[ -n $sim ]]; then
    xcrun simctl shutdown "$sim" >/dev/null 2>&1 || :
    for attempt in 1 2 3; do
      xcrun simctl delete "$sim" >/dev/null 2>&1 || :
      if listing=$(xcrun simctl list devices) && [[ $listing != *"$sim"* ]]; then removed=1; break; fi
    done
    if ((removed != 1)); then
      echo "could not confirm removal of owned $journey simulator $sim" >&9
      if ((code == 0)); then code=1; fi
    fi
  fi
  if ! python3 -B - "$code" "$phase" "$removed" "$output" "$scene_work/driver.log" "$journey" >&8 2>&9 <<'PY'
import hashlib, json, pathlib, re, sys
from contextlib import suppress
status, phase, removed = sys.argv[1:4]
result, driver = map(pathlib.Path, sys.argv[4:6])
journey = sys.argv[6]
if phase == "acceptance":
    for name in ("settlement", "ui", "board", "database"):
        if (result / (name + ".log")).is_file():
            phase += "/" + name
            break
report = {"exit": int(status), "phase": phase, "ownedSimulatorRemoved": removed == "1"}
with suppress(OSError, ValueError, KeyError, TypeError):
    data = (result / "ui/test-summary.json").read_text()
    counts = json.loads(data)
    report["xctestPassed"] = int(counts["passedTests"])
with suppress(OSError, ValueError, KeyError, TypeError):
    data = json.loads((result / "ui/failed-stages.json").read_text())
    report["assertionLines"] = [v for v in data["assertionLines"] if type(v) is int][:12]
    if journey == "T06":
        report["reached"] = [stage for stage in ("reserved", "quote", "board-approve", "submitted",
                             "pending", "paid", "preparing", "ready", "restarted")
                             if data["reached"].get("t06-" + stage, 0) > 0]
    report["cutoffMismatch"] = [v for v in data.get("cutoffMismatch", [])
                                if isinstance(v, str) and re.fullmatch(r"utc_[0-9TZ:+-]{1,32}_wall_[0-9_]{0,48}_zone_-?[0-9]{1,6}", v)][:2]
with suppress(OSError):
    report["originalReceiptSHA256"] = hashlib.sha256((result / "ui/original-receipt.json").read_bytes()).hexdigest()
with suppress(OSError):
    report["signedAppSHA256"] = re.findall(r"^[0-9a-f]{64}$", (result / ("ui/" + journey.lower() + "-signed.sha256")).read_text(), re.M)
with suppress(OSError):
    log = driver.read_text()
    if journey == "T06":
        report["singleOriginalPaidPickupVerified"] = (
            "T06 original: quote/approval/relay/payment/number/merchant ACK = 1/1/1/1/1/1; seats = 0" in log
        )
    else:
        report["singleRefundVerified"] = "T07 installed customer cancelled original booking; one separate 10 TestUSDC refund verified" in log
        report["twoRefundsVerified"] = "T07 full paid booking: one 10 TestUSDC and one 20 TestUSDC refund verified" in log
print(json.dumps(report, sort_keys=True))
PY
  then
    echo "could not summarize the owned $journey run" >&9
    if ((code == 0)); then code=1; fi
  fi
  scene_cleanup "$code" 2>&9
}
# Give the inner driver time to stop its own PostgreSQL before removing results.
interrupt_t07() {
  local code=$1 attempt
  trap '' HUP INT TERM
  if [[ $phase == acceptance && -n $scene_child ]]; then
    kill -TERM -- "-$scene_child" 2>/dev/null || :
    for ((attempt=0; attempt<650; attempt++)); do
      kill -0 -- "-$scene_child" 2>/dev/null || break
      sleep .1
    done
  fi
  scene_interrupt "$code"
}
trap cleanup EXIT
trap 'interrupt_t07 129' HUP
trap 'interrupt_t07 130' INT
trap 'interrupt_t07 143' TERM
sim=$(xcrun simctl create "$journey QA $(uuidgen)" \
  "${T07_DEVICE_TYPE:-com.apple.CoreSimulator.SimDeviceType.iPhone-17-Pro}" \
  "${T07_RUNTIME:-com.apple.CoreSimulator.SimRuntime.iOS-27-0}")
phase=boot
scene_run xcrun simctl boot "$sim"
scene_run xcrun simctl bootstatus "$sim" -b
phase=acceptance
scene_run bash "$here/run-travel-uc1.sh" "$mode" "$sim" "$output" "$1" >"$scene_work/driver.log" 2>&1
phase=complete
