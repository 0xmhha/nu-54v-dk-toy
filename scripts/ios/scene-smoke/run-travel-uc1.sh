#!/usr/bin/env bash
# Opt-in installed UC1 receipt test against fresh local PostgreSQL and Anvil.
set -euo pipefail
umask 077
travel_case=uc1
case ${1-} in
  --t06) travel_case=t06; shift ;;
  --t07-deposit) travel_case=t07; shift ;;
  --t07-full) travel_case=t07-full; shift ;;
esac
if [[ $# != 3 ]]; then
  echo 'usage: run-travel-uc1.sh [--t06|--t07-deposit|--t07-full] SIMULATOR_UUID RESULT DISPLAY_PYTHON' >&2
  exit 2
fi
sim=$1
out=$2
export NU54_TRAVEL_DISPLAY_PYTHON=$3
[[ "$sim" =~ ^[A-Fa-f0-9-]{36}$ && "$out" = /* && "$NU54_TRAVEL_DISPLAY_PYTHON" = /* ]]
"$NU54_TRAVEL_DISPLAY_PYTHON" -c 'import coincurve, Crypto.Hash.keccak'
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$here/../../.."
mkdir -- "$out"
scratch=$(mktemp -d /private/tmp/nu54-travel-uc1.XXXXXX)
fixture="$scratch/fixture"
ui=
ledger=
set -m
cleanup() {
  local outcome=$?
  trap - EXIT
  trap '' HUP INT TERM
  for pid in "$ui" "$ledger"; do
    if [[ -n "$pid" ]]; then kill -TERM -- "-$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; fi
  done
  if [[ -f "$scratch/db/postmaster.pid" ]] && ! pg_ctl -D "$scratch/db" -m fast -w stop >/dev/null; then
    local status=0
    pg_ctl -D "$scratch/db" status >/dev/null 2>&1 || status=$?
    if [[ $status != 3 ]]; then
      echo "could not confirm disposable travel database stopped at $scratch" >&2
      if ((outcome == 0)); then outcome=1; fi
      exit "$outcome"
    fi
  fi
  if ! rm -rf -- "$scratch" && ((outcome == 0)); then outcome=1; fi
  exit "$outcome"
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
mkdir "$fixture" "$scratch/socket" "$scratch/tmp"
export TMPDIR="$scratch/tmp" GOTMPDIR="$scratch/tmp"

if [[ -n "${POSTGRES_BIN:-}" ]]; then export PATH="$POSTGRES_BIN:$PATH"; fi
export PGHOST="$scratch/socket" PGPORT=5432 PGSSLMODE=disable
export PGUSER=travel_uc1_qa
export GOCACHE="$scratch/go-cache" PYTHONDONTWRITEBYTECODE=1
mkdir "$GOCACHE"
initdb -D "$scratch/db" -A trust --no-locale --encoding UTF8 -U "$PGUSER" >"$out/database.log" 2>&1
pg_ctl -D "$scratch/db" -w -l "$scratch/postgres.log" \
  -o "-c listen_addresses='' -c unix_socket_directories=$PGHOST -c unix_socket_permissions=0700" start >>"$out/database.log" 2>&1
createdb nu54_settlement_travel_uc1
dsn="host=$PGHOST port=$PGPORT user=$PGUSER dbname=nu54_settlement_travel_uc1 sslmode=disable"
P05_MIGRATION_DATABASE_URL="$dsn" python3 products/p10-platform/database/migrate.py p05 >>"$out/database.log" 2>&1
P07_MIGRATION_DATABASE_URL="$dsn" python3 products/p10-platform/database/migrate.py p07 >>"$out/database.log" 2>&1
cmake -S products/p01-device-firmware/test -B "$scratch/p01" -DCMAKE_BUILD_TYPE=Debug >"$out/board.log" 2>&1
cmake --build "$scratch/p01" --target test_install >>"$out/board.log" 2>&1
export NU54_P01_INSTALL_TEST="$scratch/p01/test_install"
export FOUNDRY_DYNAMIC_TEST_LINKING=false
export NU54_TRAVEL_NATIVE_BOARD=1 NU54_TRAVEL_NATIVE_UC1=1
unset NU54_TRAVEL_NATIVE_T06 NU54_TRAVEL_NATIVE_T07 NU54_TRAVEL_NATIVE_T07_FULL
test_child=TravelBookingPaymentsAndPickup
if [[ $travel_case == t06 ]]; then
  unset NU54_TRAVEL_NATIVE_UC1
  export NU54_TRAVEL_NATIVE_T06=1
  test_child=TravelPickupOriginalPendingToReady
fi
if [[ $travel_case == t07 || $travel_case == t07-full ]]; then export NU54_TRAVEL_NATIVE_T07=1; fi
if [[ $travel_case == t07-full ]]; then export NU54_TRAVEL_NATIVE_T07_FULL=1; fi
port=$(python3 - <<'PY'
import socket
with socket.socket() as listener:
    listener.bind(("127.0.0.1", 0))
    print(listener.getsockname()[1])
PY
)
bash "$here/run-travel-booking-refunds.sh" "$sim" "$scratch/derived" "$fixture" "$out/ui" "$port" >"$out/ui.log" 2>&1 &
ui=$!
for ((attempt=0; attempt<600; attempt++)); do
  [[ ! -s "$fixture/public-keys.json" ]] || break
  if ! kill -0 "$ui" 2>/dev/null; then
    wait "$ui" || { code=$?; echo 'travel UI failed before enrollment readiness' >&2; exit "$code"; }
    echo 'travel UI exited before enrollment readiness' >&2
    exit 1
  fi
  sleep 1
done
test -s "$fixture/public-keys.json"
P05_SETTLEMENT_TEST_DATABASE_URL="$dsn" NU54_TRAVEL_NATIVE_LAB="$fixture" \
  sh products/p05-operations-backoffice/test/settlement/anvil.sh \
     -run "^TestLoopbackSettlementEvidenceAndRefund$/^${test_child}\$" -v >"$out/settlement.log" 2>&1 &
ledger=$!
# Either child can fail while the other is waiting at a stage barrier.
while [[ -n $ui || -n $ledger ]]; do
  if [[ -n $ui ]] && ! kill -0 "$ui" 2>/dev/null; then
    wait "$ui" || { code=$?; echo 'travel UI failed' >&2; exit "$code"; }
    ui=
  fi
  if [[ -n $ledger ]] && ! kill -0 "$ledger" 2>/dev/null; then
    wait "$ledger" || { code=$?; echo 'travel settlement failed' >&2; exit "$code"; }
    ledger=
  fi
  if [[ -n $ui || -n $ledger ]]; then sleep .2; fi
done
if [[ $travel_case == t06 ]]; then
  test "$(rg -cx '[a-f0-9]{64}' "$out/ui/t06-signed.sha256")" = 2
  rg -F -- "--- PASS: TestLoopbackSettlementEvidenceAndRefund/TravelPickupOriginalPendingToReady" "$out/settlement.log"
  rg -F 'T06 original: quote/approval/relay/payment/number/merchant ACK = 1/1/1/1/1/1; seats = 0' "$out/settlement.log"
  exit 0
fi
test -f "$out/ui/privacy-proof.json"
test -f "$out/ui/original-receipt.json"
if [[ $travel_case == t07-full ]]; then
  rg -F 'T07 full paid booking: one 10 TestUSDC and one 20 TestUSDC refund verified' "$out/settlement.log"
else
  rg -F 'Original installed booking, one 10 TestUSDC deposit and one receipt verified' "$out/settlement.log"
fi
if [[ $travel_case == t07 ]]; then
  rg -F 'T07 installed customer cancelled original booking; one separate 10 TestUSDC refund verified' "$out/settlement.log"
fi
