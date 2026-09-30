#!/usr/bin/env bash
# Create the testnet role accounts as encrypted keystores (N32).
#
# For each role this script:
#   1. generates a random keystore password and stores it only in the macOS login Keychain
#      (service "nu54-<role>", account "nu54");
#   2. creates an encrypted keystore with `cast wallet new`, reading the password from the
#      Keychain through CAST_PASSWORD, so the password never touches a file or this repository;
#   3. records the public address in $NU54_HOME/testnet-accounts.env.
#
# Nothing secret is written inside the repository. Existing keystores are never overwritten.
#
# Usage:
#   script/testnet-accounts.sh            create missing accounts and print the address list
#   script/testnet-accounts.sh --list     print the address list only
#   script/testnet-accounts.sh --dry-run  show what would be created
#
# forge scripts open these keystores through script/testnet-forge.sh, which passes each
# password to forge in a short-lived file and removes it afterwards.
set -euo pipefail

NU54_HOME="${NU54_HOME:-$HOME/.nu54}"
KEYSTORES="$NU54_HOME/keystores"
ADDRESSES="$NU54_HOME/testnet-accounts.env"
# deployer: deploys contracts. operator: deposits, withdrawals, closing, attestations.
# registry-admin: registers merchants. token-owner: mints the test token.
# kiosk: pays gas for payment submissions. device: software signer for the week-6 gate only.
ROLES=(deployer operator registry-admin token-owner kiosk device)

mode="${1:-create}"

list() {
  if [[ -f "$ADDRESSES" ]]; then
    cat "$ADDRESSES"
  else
    echo "no accounts yet ($ADDRESSES missing)"
  fi
}

if [[ "$mode" == "--list" ]]; then
  list
  exit 0
fi

command -v cast >/dev/null || { echo "cast (Foundry) is required" >&2; exit 1; }
command -v security >/dev/null || { echo "macOS 'security' is required for the Keychain" >&2; exit 1; }

if [[ "$mode" != "--dry-run" ]]; then
  mkdir -p "$KEYSTORES"
  chmod 700 "$NU54_HOME" "$KEYSTORES"
  touch "$ADDRESSES"
  chmod 600 "$ADDRESSES"
fi

for role in "${ROLES[@]}"; do
  name="nu54-$role"
  var="NU54_ADDR_$(echo "$role" | tr 'a-z-' 'A-Z_')"
  if [[ -f "$KEYSTORES/$name" ]]; then
    echo "skip   $name (keystore exists)"
    continue
  fi
  if [[ "$mode" == "--dry-run" ]]; then
    echo "create $name -> $KEYSTORES/$name, password in Keychain service $name, address as $var"
    continue
  fi
  if ! security find-generic-password -a nu54 -s "$name" >/dev/null 2>&1; then
    security add-generic-password -a nu54 -s "$name" -l "$name keystore password" \
      -w "$(openssl rand -base64 32)"
  fi
  address="$(CAST_PASSWORD="$(security find-generic-password -a nu54 -s "$name" -w)" \
    cast wallet new "$KEYSTORES" "$name" | awk '/^Address:/ {print $2}')"
  [[ "$address" =~ ^0x[0-9a-fA-F]{40}$ ]] || { echo "could not read the address for $name" >&2; exit 1; }
  chmod 600 "$KEYSTORES/$name"
  echo "$var=$address" >> "$ADDRESSES"
  echo "create $name $address"
done

if [[ "$mode" != "--dry-run" ]]; then
  echo
  echo "addresses ($ADDRESSES):"
  list
  echo
  echo "Next: fund deployer, operator, registry-admin, token-owner and kiosk at https://faucet.stablenet.network"
  echo "(2,000 WKRC per address per 24 h). The device account needs no gas."
fi
