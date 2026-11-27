#!/bin/sh
set -eu
here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo=$(CDPATH='' cd -- "$here/../../.." && pwd)
cd "$repo"
: "${P07_CHAINVIEW_TEST_URL:?use a disposable PostgreSQL 15 database with P07 migrations applied}"
scratch=$(mktemp -d)
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
port=$(python3 - <<'PY'
import socket
with socket.socket() as listener:
    listener.bind(("127.0.0.1", 0))
    print(listener.getsockname()[1])
PY
)
rpc="http://127.0.0.1:$port"
"${ANVIL:-anvil}" --host 127.0.0.1 --port "$port" --chain-id 8283 --timestamp 1800000000 --silent > "$scratch/anvil.log" 2>&1 &
anvil=$!
ready=
for _ in 1 2 3 4 5 6 7 8 9 10; do
    if chain=$("${CAST:-cast}" chain-id --rpc-url "$rpc" 2>/dev/null) && [ "$chain" = 8283 ]; then
        ready=yes
        break
    fi
    sleep 1
done
[ "$ready" = yes ] || { echo "isolated Anvil failed to become ready" >&2; exit 1; }
python3 -B scripts/contract_forge.py p06 build --offline --quiet
go run ./packages/contracts-abi/tools/v2fixture/main.go "$rpc" > "$scratch/settlement-v2.json"
go build -o "$scratch/indexer" ./products/p07-indexer/cmd/indexer
if [ "$#" -eq 0 ]; then set -- -run TestAnvilActualReceiptsAndDivergentFork -v; fi
P07_CHAINVIEW_TEST_RPC="$rpc" P07_CHAINVIEW_ANVIL_RECEIPTS="$scratch/settlement-v2.json" \
    P07_CHAINVIEW_TEST_INDEXER="$scratch/indexer" \
    go test ./products/p07-indexer/internal/chainview -count=1 "$@"
