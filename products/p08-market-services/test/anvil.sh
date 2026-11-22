#!/usr/bin/env bash
set -euo pipefail
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$here"
: "${P08_TEST_DATABASE_URL:?a disposable migrated P08 database with a restricted keeper role is required}"
work=$(mktemp -d)
pid=
cleanup() {
  if [[ -n "$pid" ]]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$work"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
export FOUNDRY_OUT="$work/contracts" FOUNDRY_CACHE_PATH="$work/forge-cache"
port=$(python3 - <<'PY'
import socket
with socket.socket() as listener:
    listener.bind(("127.0.0.1", 0))
    print(listener.getsockname()[1])
PY
)
rpc="http://127.0.0.1:$port"
# Leave room for funding/reorg fixture jumps before reconnecting to wall time.
start=$(( $(date +%s) - 3600 ))
anvil --host 127.0.0.1 --port "$port" --chain-id 8283 --timestamp "$start" --silent > "$work/anvil.log" 2>&1 &
pid=$!
ready=
for _ in {1..10}; do
  if [[ $(cast chain-id --rpc-url "$rpc" 2>/dev/null) = 8283 ]]; then
    ready=1
    break
  fi
  sleep 1
done
if [[ -z "$ready" ]]; then
  cat "$work/anvil.log" >&2
  exit 1
fi
python3 -B ../../scripts/contract_forge.py p08 build --quiet
if [[ -z "${P08_TEST_BOARD_SIGNER:-}" ]]; then
  cmake -S ../p01-device-firmware/test -B "$work/board" -DCMAKE_BUILD_TYPE=Debug >/dev/null
  cmake --build "$work/board" --target test_device_evm >/dev/null
  export P08_TEST_BOARD_SIGNER="$work/board/test_device_evm"
fi
exec_args=("$@")
if [[ $# -eq 0 ]]; then exec_args=(-run '^TestIsolatedMarketExecutionAndKeeperRestart$' -v); fi
P08_TEST_RPC="$rpc" go test ./internal/market -count=1 "${exec_args[@]}"
