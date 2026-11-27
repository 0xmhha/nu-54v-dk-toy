#!/usr/bin/env bash
# Internal worker for mobile-fixture-workspace.py.
set -euo pipefail
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
marketd="${P08_MOBILE_FIXTURE_MARKETD:?fixture workspace required}"
output_argument="${1:?Pass a directory for the isolated fixture evidence}"
shift
output=$(mkdir -p -- "$output_argument" && cd -- "$output_argument" && pwd)
caller=$PWD
market=$(cd -- "$here/.." && pwd)
if [[ -e "$output/profile.json" ]]; then
  echo 'choose a new directory for the fixture' >&2
  exit 2
fi
if curl -fsS --max-time 1 http://127.0.0.1:8284/v1/profile >/dev/null 2>&1 ||
  cast chain-id --rpc-url http://127.0.0.1:8285 >/dev/null 2>&1; then
  echo 'ports 8284 and 8285 must be vacant for this fixture' >&2
  exit 2
fi
node_pid=
market_pid=
cleanup() {
  for pid in "$market_pid" "$node_pid"; do
    if [[ -n "$pid" ]]; then
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
}
trap cleanup EXIT
cd "$market"
python3 ../../scripts/contract_forge.py p08 build --quiet
go build -o "$marketd" ./cmd/marketd
fixture_phrase='test test test test test test test test test test test junk'
export P08_RPC=http://127.0.0.1:8285
anvil --host 127.0.0.1 --port 8285 --chain-id 8283 --mnemonic "$fixture_phrase" --slots-in-an-epoch 1   --block-time 1 --silent >"$output/anvil.log" 2>&1 &
node_pid=$!
for _ in {1..15}; do
  if [[ $(cast chain-id --rpc-url "$P08_RPC" 2>/dev/null) == 8283 ]]; then break; fi
  sleep 1
done
[[ $(cast chain-id --rpc-url "$P08_RPC") == 8283 ]]
# Cast derives deterministic, public fixture identities from the Anvil mnemonic.
fixture_key() { cast wallet private-key "$fixture_phrase" "$1"; }
fixture_address() { cast wallet address --private-key "$(fixture_key "$1")"; }
admin=$(fixture_key 0)
quote=$(fixture_key 1)
export P08_ADMIN_KEY=${admin#0x}
export P08_QUOTE_KEY=${quote#0x}
export P08_QUOTE_SIGNER=$(fixture_address 1)
export P08_PUBLISHER_A=$(fixture_address 2)
export P08_PUBLISHER_B=$(fixture_address 3)
export P08_KEEPER=$(fixture_address 4)
export P08_PROFILE="$output/profile.json"
"$marketd" deploy >"$P08_PROFILE"
read -r venue quote_asset < <(python3 - "$P08_PROFILE" <<'PYFIXTURE'
import json, sys
with open(sys.argv[1]) as source:
    p = json.load(source)
print(p["venue"]["address"], p["quote"]["address"])
PYFIXTURE
)
cast send --rpc-url "$P08_RPC" --private-key "$admin" "$quote_asset" 'mint(address,uint256)' "$(fixture_address 0)" 100000000 --quiet
cast send --rpc-url "$P08_RPC" --private-key "$admin" "$quote_asset" 'approve(address,uint256)' "$venue" 100000000 --quiet
cast send --rpc-url "$P08_RPC" --private-key "$admin" "$venue" 'seedRisk(uint256)' 100000000 --quiet
unset P08_ADMIN_KEY admin
"$marketd" serve >"$output/marketd.log" 2>&1 &
market_pid=$!
for _ in {1..15}; do
  if curl -fsS --max-time 2 http://127.0.0.1:8284/v1/profile >/dev/null 2>&1; then break; fi
  sleep 1
done
curl -fsS --max-time 2 http://127.0.0.1:8284/v1/profile >/dev/null
unset P08_QUOTE_KEY quote fixture_phrase
printf 'Isolated P08 ready. Deployment profile %s\n' "$P08_PROFILE"
cd "$caller"
if [[ $# -gt 0 ]]; then
  "$@"
else
  wait "$market_pid"
fi
