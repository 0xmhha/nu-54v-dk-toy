#!/usr/bin/env bash
# Start the settlement harness when FIXTURE/public-keys.json becomes available.
set -euo pipefail
umask 077
if [[ $# -ne 4 ]]; then
  echo 'usage: run-subscription-denied.sh SIMULATOR_UUID P02_DERIVED_DATA FIXTURE RESULT' >&2
  exit 2
fi
sim=$1
derived=$2
fixture=$3
out=$4
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
mkdir -p -- "$fixture"
mkdir -- "$out"
fixture=$(cd -- "$fixture" && pwd)
out=$(cd -- "$out" && pwd)
[[ "$(stat -f %Lp "$fixture")" == 700 ]] || { echo 'Use a private mode-0700 fixture directory' >&2; exit 2; }
[[ ! -e "$fixture/preview.json" && ! -e "$fixture/ui-completed" && ! -e "$fixture/ui-aborted" && ! -e "$fixture/public-keys.json" ]]
scratch=$(mktemp -d /private/tmp/nu54-subscription-denied.XXXXXX)
trap 'rm -rf -- "$scratch"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
namespace="com.nu54renter.subscription-denied.$(uuidgen | tr '[:upper:]' '[:lower:]' | tr -d '-' | cut -c1-12)"
test_pid=
bridge_pid=
catalog_pid=
board_pid=
catalog_args=()
test_method=testDeniedPermissionKeepsSameAuthenticatedInvoiceAcrossRestart
flow=subscription
bridge_args=("$fixture" "$scratch/bridge.json")
roles=(ios ios-second)
board_mode=0
qa_bundle=com.nu54renter.w04qa
qa_application=
if [[ "${NU54_SUBSCRIPTION_PAIR_ANDROID:-0}" == 1 ]]; then
  roles=(ios)
  bridge_args+=(--single)
fi
if [[ -n "${NU54_BOOKING_CATALOG_PORT:-}" ]]; then
  [[ "${#roles[@]}" == 2 ]]
  [[ "$NU54_BOOKING_CATALOG_PORT" =~ ^[0-9]{4,5}$ ]]
  flow=booking
  bridge_args+=(--travel)
  test_method=testBookingReopensDepositAndPartialRefund
  if [[ "${NU54_TRAVEL_NATIVE_T06:-0}" == 1 ]]; then
    test_method=testTravelPickupWaitsForOriginalPaymentAndAcceptance
    roles=(ios)
    bridge_args+=(--single)
    catalog_args+=(--pickup --wait-pickup-branch)
  fi
  export NU54_P09_PUBLIC_ORIGIN="https://127.0.0.1:$NU54_BOOKING_CATALOG_PORT"
  board_mode="${NU54_TRAVEL_NATIVE_BOARD:-0}"
  [[ "$board_mode" = 0 || "$board_mode" = 1 ]]
  if [[ "$board_mode" = 1 ]] && xcrun simctl get_app_container "$sim" "$qa_bundle" app >/dev/null 2>&1; then
    echo "Use a simulator without an installed owner QA bundle." >&2
    exit 1
  fi
fi
# Bash job control assigns a separate group to each subprocess and its children.
set -m
stop() {
  local pid=$1
  [[ -n "$pid" ]] || return 0
  kill -TERM -- "-$pid" 2>/dev/null || true
  for ((attempt=0; attempt<10; attempt++)); do
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.1
  done
  kill -KILL -- "-$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
}
cleanup() {
  local code=$?
  trap - EXIT
  trap '' INT TERM
  if ((code != 0)); then touch "$fixture/ui-aborted"; fi
  stop "$test_pid"
  stop "$bridge_pid"
  stop "$catalog_pid"
  stop "$board_pid"
  rm -f -- "$fixture/preview.json" "$fixture/public-keys.json" "$fixture/public-keys.tmp" "$fixture/preview.tmp"
  if [[ "$flow" == booking ]]; then
    rm -f -- "$fixture/travel-booking" "$fixture/original-receipt.json" "$fixture/privacy-check" "$fixture/privacy-proof.json" "$fixture/privacy-proof.tmp"
    rm -f -- "$fixture/public-branch" "$fixture/travel-stage" "$fixture/catalog-ready" \
      "$fixture/travel-deposit-quote" "$fixture/travel-deposit-refused" \
      "$fixture/travel-deposit-board-refuse" "$fixture/travel-deposit-board-approve" "$fixture/travel-deposit-approved" \
      "$fixture/travel-deposit" "$fixture/travel-deposit-checking" \
      "$fixture/travel-balance-quote" "$fixture/travel-balance-board-refuse" "$fixture/travel-balance-refused" \
      "$fixture/travel-balance-board-approve" "$fixture/travel-balance-approved" "$fixture/travel-both-paid" \
      "$fixture/travel-balance-checking" "$fixture/travel-partial" "$fixture/travel-refunded"
  fi
  if [[ -n "$qa_application" ]]; then
    NU54_TRAVEL_OWNER_SCOPE="$(cat "$fixture/board/owner-scope" 2>/dev/null || true)"       bash "$here/run-subscription-native.sh" "$sim" "$qa_application" "$scratch/OwnerCleanup" testClearTravelBoardWallet >/dev/null 2>&1 || {
      if ((code == 0)); then code=1; fi
    }
    xcrun simctl uninstall "$sim" "$qa_bundle" >/dev/null 2>&1 || true
    rm -rf -- "$fixture/board" "$fixture/display"
  fi
  rm -rf -- "$scratch"
  for role in "${roles[@]}"; do
    xcrun simctl uninstall "$sim" "$namespace-$role" >/dev/null 2>&1 || true
  done
  if ((code != 0)); then echo "Subscription denial acceptance ended (status $code)" >&2; fi
  exit "$code"
}
source "$here/../test-environment.sh"
ios_test_prepare_environment "$scratch"
trap cleanup EXIT
capture() {
  local log=$1 code=0
  shift
  "$@" >"$log" 2>&1 &
  test_pid=$!
  wait "$test_pid" || code=$?
  test_pid=
  return "$code"
}
for role in "${roles[@]}"; do
  bundle="$namespace-$role"
  flags=
  if [[ "$board_mode" = 1 ]]; then
    flags="-DNU54_OWNER_REGISTRATION_QA=1"
    if [[ "$role" = ios ]]; then bundle="$qa_bundle"; fi
  fi
  capture "$scratch/$role-build.log" "${ios_test_env[@]}" xcodebuild -workspace "$here/../../../products/p02-user-app/ios/NU54Renter.xcworkspace" -scheme NU54Renter \
    -configuration Release -destination 'generic/platform=iOS Simulator' -derivedDataPath "$derived" \
    -jobs 2 ARCHS=arm64 CODE_SIGNING_ALLOWED=YES CODE_SIGN_IDENTITY=- \
    NU54_IOS_BUNDLE="$bundle" NU54_OWNER_REGISTRATION_QA_FLAGS="$flags" build
  application="$derived/Build/Products/Release-iphonesimulator/NU54Renter.app"
  if [[ "${NU54_TRAVEL_NATIVE_T06:-0}" == 1 ]]; then
    codesign --verify --strict "$application"
    shasum -a 256 "$application/NU54Renter" "$application/main.jsbundle" | awk '{print $1}' >>"$out/t06-signed.sha256"
  fi
  if [[ "${NU54_TRAVEL_NATIVE_T07:-0}" == 1 ]]; then
    codesign --verify --strict "$application"
    shasum -a 256 "$application/NU54Renter" "$application/main.jsbundle" | awk '{print $1}' >>"$out/t07-signed.sha256"
  fi
  capture "$scratch/$role-install.log" xcrun simctl install "$sim" "$application"
  if [[ "$bundle" = "$qa_bundle" ]]; then
    qa_application="$scratch/NU54Renter.app"
    ditto "$application" "$qa_application"
    source "$here/fixtures/travel_board_owner.sh"
    prepare_travel_board "$qa_application" "$scratch/wallet.json"
    rm -f -- "$scratch/wallet.json"
  fi
done
python3 "$here/fixtures/p05_subscription_enrollment.py" "${bridge_args[@]}" >"$scratch/bridge.log" 2>&1 &
bridge_pid=$!
if [[ "$flow" == booking ]]; then
  (cd "$here/../../.." && exec python3 "$here/fixtures/p09_booking_catalog_tls.py" "$fixture" "$NU54_BOOKING_CATALOG_PORT" "$sim" "${catalog_args[@]}") >"$scratch/catalog.log" 2>&1 &
  catalog_pid=$!
fi
for ((i=0; i<10; i++)); do
  [[ -s "$scratch/bridge.json" ]] && break
  sleep 1
done
[[ -s "$scratch/bridge.json" ]] || { echo 'Local enrollment fixture unavailable' >&2; exit 1; }
capture "$scratch/UI-build.log" "${ios_test_env[@]}" xcodebuild build-for-testing -project "$here/SubscriptionDeniedSmoke.xcodeproj" -scheme SceneSmoke   -destination "platform=iOS Simulator,id=$sim" -derivedDataPath "$scratch/UI"
configured=$(find "$scratch/UI" -name 'SceneSmoke_*.xctestrun' -print -quit)
python3 - "$configured" "$scratch/bridge.json" "$namespace" "$board_mode" <<'PYTHON'
import json, os, pathlib, plistlib, sys
spec, bridge = map(pathlib.Path, sys.argv[1:3])
manifest = plistlib.loads(spec.read_bytes())
grant = json.loads(bridge.read_bytes())
if "TestConfigurations" in manifest:
    targets = [entry for config in manifest["TestConfigurations"] for entry in config["TestTargets"]]
else:
    targets = [entry for name, entry in manifest.items() if not name.startswith("__")]
assert targets
for entry in targets:
    entry.setdefault("EnvironmentVariables", {}).update(
        NU54_SUBSCRIPTION_NAMESPACE=sys.argv[3],
        NU54_SUBSCRIPTION_ENROLLMENT=grant["origin"],
        NU54_SUBSCRIPTION_TEST_AUTH=grant["access"],
        NU54_SUBSCRIPTION_PAIR_ANDROID=os.environ.get("NU54_SUBSCRIPTION_PAIR_ANDROID", "0"),
        NU54_TRAVEL_NATIVE_BOARD=sys.argv[4],
        NU54_TRAVEL_NATIVE_UC1=os.environ.get("NU54_TRAVEL_NATIVE_UC1", "0"),
        NU54_TRAVEL_NATIVE_T06=os.environ.get("NU54_TRAVEL_NATIVE_T06", "0"),
        NU54_TRAVEL_NATIVE_T07=os.environ.get("NU54_TRAVEL_NATIVE_T07", "0"),
        NU54_TRAVEL_NATIVE_T07_FULL=os.environ.get("NU54_TRAVEL_NATIVE_T07_FULL", "0"),
        NU54_TRAVEL_NATIVE_T05=os.environ.get("NU54_TRAVEL_NATIVE_T05", "0"),
        NU54_TRAVEL_LOSE_DEPOSIT_ACK=os.environ.get("NU54_TRAVEL_LOSE_DEPOSIT_ACK", "0"),
    )
spec.write_bytes(plistlib.dumps(manifest))
PYTHON
result="$scratch/Denied.xcresult"
code=0
capture "$scratch/denied.log" "${ios_test_env[@]}" xcodebuild test-without-building -xctestrun "$configured" \
  -destination "platform=iOS Simulator,id=$sim" -parallel-testing-enabled NO \
  -collect-test-diagnostics never \
  "-only-testing:SceneSmoke/SubscriptionDeniedSmoke/$test_method" \
  -resultBundlePath "$result" || code=$?
if [[ -d "$result" ]]; then
  xcrun xcresulttool get test-results summary --path "$result" | python3 -c '
import json, pathlib, sys
result = json.load(sys.stdin)
keys = ("passedTests", "failedTests", "skippedTests", "totalTestCount")
summary = {key: int(result.get(key, 0)) for key in keys}
if summary == {"passedTests": 1, "failedTests": 0, "skippedTests": 0, "totalTestCount": 1}:
    import os
    if os.environ.get("NU54_TRAVEL_NATIVE_UC1") == "1":
        summary.update(normalEnrollments=1, depositOnly=1, originalReceipt=1)
    elif os.environ.get("NU54_TRAVEL_NATIVE_T06") == "1":
        summary.update(signedPickupCustomer=1)
    elif sys.argv[1] == "booking":
        summary.update(normalEnrollments=1, restarted=4, depositOnly=1, originalBalancePending=1, partialRefund=1, bothRefunds=1)
        fixture = pathlib.Path(sys.argv[3])
        log = pathlib.Path(sys.argv[4]).read_text(errors="replace")
        summary.update(depositAddressReturns=log.count("NU54_TRAVEL_ADDRESS_RETURN=deposit"),
                       balanceAddressReturns=log.count("NU54_TRAVEL_ADDRESS_RETURN=balance"))
        if "NU54_TRAVEL_COMMITTED_REPLY_UNAVAILABLE" in log:
            summary["committedDepositReplyUnavailable"] = 1
        if (fixture / "travel-deposit-quote").exists() and (fixture / "travel-deposit-refused").exists():
            summary.update(restarted=6, originalSignedDepositQuote=1, sameUnpaidAfterHostRefusal=1)
            if (fixture / "travel-deposit-approved").exists():
                summary.update(restarted=5, authenticatedBoardRefusal=1, authenticatedBoardApproval=1, explicitDepositSubmission=1)
            if (fixture / "travel-both-paid").exists():
                summary.update(restarted=8, depositPendingRestart=1, independentBalanceApproval=1,
                               explicitBalanceSubmission=1, originalReceiptsBeforeRefund=2)
    else:
        count = int(sys.argv[2])
        summary.update(osRefusals=count, normalEnrollments=count, authenticatedInvoices=count, restarted=count)
print(json.dumps(summary))
' "$flow" "${#roles[@]}" "$fixture" "$scratch/denied.log" >"$out/test-summary.json"
fi
if ((code != 0)); then
  python3 - "$scratch/denied.log" >"$out/failed-stages.json" <<'PYTHON'
import json, pathlib, re, sys
log = pathlib.Path(sys.argv[1]).read_text(errors="replace")
phases = ("t06-reserved", "t06-quote", "t06-board-approve", "t06-submitted", "t06-pending",
          "t06-paid", "t06-preparing", "t06-ready", "t06-restarted", "key", "received", "pasted", "submit", "denied", "registration-rejected", "registration-locked", "registration-pending", "registration-idle", "deposit-quote", "deposit-refused", "deposit-board-refuse", "deposit-board-approve", "deposit-approved", "deposit", "balance-checking", "partial", "refunded", "deposit-checking", "balance-quote", "balance-board-refuse", "balance-refused", "balance-board-approve", "balance-approved", "both-paid", "cancel-preview", "cancel-requested", "deposit-refunded")
print(json.dumps({
    "reached": {phase: log.count("NU54_SUBSCRIPTION_CHECKPOINT=" + phase) for phase in phases},
    "missingSteps": [label for label in ("구독 관리", "회차 알림 허용", "같은 회차 결제 검토", "여행지 찾기", "서울", "지역 맛집", "내 예약 다시 열기", "예약 지급 및 영수증 조회")
                     if "Missing button " + label in log],
    "cutoffMismatch": re.findall(r"NU54_T07_CUTOFF_DIAGNOSTIC=(utc_[0-9TZ:+-]{1,32}_wall_[0-9_]{0,48}_zone_-?[0-9]{1,6})", log)[:2],
    "assertionLines": [int(line) for line in re.findall(r"SubscriptionDeniedSmoke[.]swift:(\d+): error:", log)],
}))
PYTHON
fi
test "$code" -eq 0
python3 -c 'import json,sys; assert json.load(open(sys.argv[1]))["passedTests"] == 1' "$out/test-summary.json"
if [[ "${NU54_TRAVEL_NATIVE_UC1:-0}" == 1 ]]; then
  touch "$fixture/privacy-check"
  for _ in {1..80}; do
    [[ -s "$fixture/privacy-proof.json" ]] && break
    kill -0 "$catalog_pid"
    sleep .25
  done
  python3 - "$fixture/privacy-proof.json" "$fixture/original-receipt.json" "$out" <<'PYVERIFY'
import json, pathlib, sys
privacy, receipt, out = map(pathlib.Path, sys.argv[1:])
proof = json.loads(privacy.read_text())
assert proof["optionalRelations"] == 17 and proof["optionalWrites"] == 0
assert proof["before"] == proof["after"] and proof["before"]["consent"] == proof["before"]["consent_decision"] == 0
assert not proof["publicPrivatePrivileges"] and proof["publicRequests"] >= 2 and proof["authenticatedPublicRequests"] == 0
(out / "privacy-proof.json").write_bytes(privacy.read_bytes())
(out / "original-receipt.json").write_bytes(receipt.read_bytes())
PYVERIFY
fi
touch "$fixture/ui-completed"
