#!/bin/sh
set -eu
here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo=$(CDPATH='' cd -- "$here/../../../.." && pwd)
cd "$repo"
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
port=$(python3 - <<'PY'
import socket
with socket.socket() as server:
    server.bind(("127.0.0.1", 0))
    print(server.getsockname()[1])
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
python3 scripts/contract_forge.py p06 build --offline --quiet
go run ./packages/contracts-abi/tools/v2fixture/main.go "$rpc"
