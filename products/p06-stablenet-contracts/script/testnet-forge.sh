#!/usr/bin/env bash
# Run a forge script on the StableNet testnet with role keystores (N32).
#
# Each role's keystore password is read from the macOS Keychain into a short-lived file that
# only the current user can read, passed to forge with --password-file, and deleted on exit.
# (forge refuses /dev/fd paths and FIFOs, so process substitution does not work.)
#
# Usage:
#   script/testnet-forge.sh [--simulate] <script.s.sol> <role> [<role>...]
#     --simulate   run without --broadcast (no transactions are sent)
#   The first role is the default sender.
#
# Examples:
#   script/testnet-forge.sh --simulate script/Deploy.s.sol deployer
#   script/testnet-forge.sh script/Deploy.s.sol deployer
#   NU54_SETTLEMENT=0x... script/testnet-forge.sh script/SoftwareSettle.s.sol kiosk registry-admin token-owner operator device
set -euo pipefail

NU54_HOME="${NU54_HOME:-$HOME/.nu54}"
broadcast=(--broadcast --slow)
if [[ "${1:-}" == "--simulate" ]]; then
  broadcast=()
  shift
fi
[[ $# -ge 2 ]] || { sed -n '2,17p' "$0"; exit 2; }
script="$1"
shift

# Public role addresses (NU54_ADDR_*) for the scripts.
set -a
# shellcheck disable=SC1091
source "$NU54_HOME/testnet-accounts.env"
set +a
# Deploy.s.sol takes the role addresses under these names.
export NU54_OPERATOR="${NU54_OPERATOR:-$NU54_ADDR_OPERATOR}"
export NU54_REGISTRY_ADMIN="${NU54_REGISTRY_ADMIN:-$NU54_ADDR_REGISTRY_ADMIN}"
export NU54_TOKEN_OWNER="${NU54_TOKEN_OWNER:-$NU54_ADDR_TOKEN_OWNER}"

run_dir="$(mktemp -d "$NU54_HOME/run.XXXXXX")"
chmod 700 "$run_dir"
trap 'rm -rf "$run_dir"' EXIT

wallet_args=()
for role in "$@"; do
  name="nu54-$role"
  keystore="$NU54_HOME/keystores/$name"
  [[ -f "$keystore" ]] || { echo "missing keystore $keystore" >&2; exit 1; }
  pw="$run_dir/$name"
  (umask 077 && security find-generic-password -a nu54 -s "$name" -w > "$pw")
  wallet_args+=(--keystores "$keystore" --password-file "$pw")
done

# StableNet refuses a priority fee below its minimum (27,600 gwei on 2026-10-01) and a fee cap
# below base fee + that tip; forge would offer a 1 wei tip. Use the tip the node suggests.
rpc_url="$(cd "$(dirname "$0")/.." && forge config --json | python3 -c 'import json,sys; print(json.load(sys.stdin)["rpc_endpoints"]["stablenet_testnet"])')"
tip="$(cast to-dec "$(cast rpc eth_maxPriorityFeePerGas --rpc-url "$rpc_url" | tr -d '"')")"
# Fee cap = 2 x base fee + tip, so a base fee rise between blocks does not strand the tx.
fee_cap=$(( 2 * $(cast base-fee --rpc-url "$rpc_url") + tip ))

sender_var="NU54_ADDR_$(echo "$1" | tr 'a-z-' 'A-Z_')"
forge script "$script" --rpc-url stablenet_testnet "${broadcast[@]}" \
  --priority-gas-price "$tip" --with-gas-price "$fee_cap" --sender "${!sender_var}" "${wallet_args[@]}"
