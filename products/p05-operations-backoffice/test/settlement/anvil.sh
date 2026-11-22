#!/bin/sh
set -eu
here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo=$(CDPATH='' cd -- "$here/../../../.." && pwd)
cd "$repo"
: "${P05_SETTLEMENT_TEST_DATABASE_URL:?use a disposable nu54_settlement_* database with P05 and P07 migrations}"
scratch=$(mktemp -d "${TMPDIR:-/tmp}/nu54-settlement.XXXXXX")
anvil=
cleanup() {
    if [ -n "$anvil" ]; then
        kill "$anvil" 2>/dev/null || true
        wait "$anvil" 2>/dev/null || true
    fi
    rm -rf "$scratch"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
export FOUNDRY_OUT="$scratch/contracts" FOUNDRY_CACHE_PATH="$scratch/forge-cache"
export NU54_RENTAL_CONTRACT_OUT="$FOUNDRY_OUT"
rental_browser=
if [ "${1:-}" = "--rental-browser" ]; then
    [ "$#" -eq 1 ] || { echo "use --rental-browser by itself" >&2; exit 2; }
    rental_browser=yes
    mkdir -p "$scratch/go" "$scratch/tmp" "$scratch/npm" "$scratch/xdg"
    export TMPDIR="$scratch/tmp" GOTMPDIR="$scratch/tmp" GOCACHE="$scratch/go"
    export XDG_CACHE_HOME="$scratch/xdg" npm_config_cache="$scratch/npm"
    export NODE_DISABLE_COMPILE_CACHE=1 PYTHONDONTWRITEBYTECODE=1
    export NU54_RENTAL_OPENING_BROWSER_DIST="$scratch/dist"
    pnpm --dir products/p05-operations-backoffice/web exec tsc --noEmit
    pnpm --dir products/p05-operations-backoffice/web exec vite build --outDir "$NU54_RENTAL_OPENING_BROWSER_DIST"
    go test -c -o "$scratch/settlement.test" ./products/p05-operations-backoffice/internal/settlement
fi
port=$(python3 - <<'PY'
import socket
with socket.socket() as listener:
    listener.bind(("127.0.0.1", 0))
    print(listener.getsockname()[1])
PY
)
rpc="http://127.0.0.1:$port"
# A day-close proof needs a canonical starting block before today's full
# branch reporting window. Ordinary payment fixtures use the recent head.
age=65
if [ -n "${NU54_NOTIFICATION_BROWSER:-}" ]; then age=90000; fi
timestamp=$(($(date +%s) - age))
"${ANVIL:-anvil}" --host 127.0.0.1 --port "$port" --chain-id 8283 --timestamp "$timestamp"     --slots-in-an-epoch 1 --silent > "$scratch/anvil.log" 2>&1 &
anvil=$!
ready=
for _ in 1 2 3 4 5 6 7 8 9 10; do
    if chain=$("${CAST:-cast}" chain-id --rpc-url "$rpc" 2>/dev/null) && [ "$chain" = 8283 ]; then
        ready=yes; break
    fi
    sleep 1
done
[ "$ready" = yes ] || { echo "isolated Anvil failed to become ready" >&2; exit 1; }
python3 -B scripts/contract_forge.py p06 build --offline --quiet
go build -o "$scratch/test-signer" ./products/p05-operations-backoffice/test/settlement/signfixture
go build -o "$scratch/opsd" ./products/p05-operations-backoffice/cmd/opsd
if [ -z "$rental_browser" ]; then
    go build -o "$scratch/indexer" ./products/p07-indexer/cmd/indexer
    go build -o "$scratch/visit-issuer" ./products/p05-operations-backoffice/cmd/visit-issuer
    go build -o "$scratch/visit-benefit" ./products/p05-operations-backoffice/cmd/visit-benefit
fi
if [ -z "${NU54_P01_INSTALL_TEST:-}" ] && { [ -z "$rental_browser" ] || [ -n "${P05_PILOT_OUTPUT:-}" ]; }; then
    cmake -S products/p01-device-firmware/test -B "$scratch/p01" -DCMAKE_BUILD_TYPE=Debug >&2
    cmake --build "$scratch/p01" --target test_install >&2
    export NU54_P01_INSTALL_TEST="$scratch/p01/test_install"
fi
if [ -n "$rental_browser" ]; then
    cd "$repo/products/p05-operations-backoffice/internal/settlement"
    P05_SETTLEMENT_TEST_RPC="$rpc" P05_SETTLEMENT_TEST_OPSD="$scratch/opsd" \
        P05_SETTLEMENT_TEST_SIGNER="$scratch/test-signer" \
        "$scratch/settlement.test" -test.run '^TestLoopbackManagedRentalFinalWithdrawal$' -test.v
    exit
fi
if [ "$#" -eq 0 ]; then set -- -run TestLoopbackSettlementEvidenceAndRefund -v; fi
P05_SETTLEMENT_TEST_VISIT_BENEFIT="$scratch/visit-benefit" P05_SETTLEMENT_TEST_VISIT_ISSUER="$scratch/visit-issuer" P05_SETTLEMENT_TEST_RPC="$rpc" P05_SETTLEMENT_TEST_INDEXER="$scratch/indexer" P05_SETTLEMENT_TEST_OPSD="$scratch/opsd" P05_SETTLEMENT_TEST_SIGNER="$scratch/test-signer"     go test ./products/p05-operations-backoffice/internal/settlement -count=1 "$@"
