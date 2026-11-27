#!/usr/bin/env bash
# Pair with the isolated settlement harness using NU54_TRAVEL_NATIVE_LAB=FIXTURE.
set -euo pipefail
if [[ $# -ne 5 ]]; then
  echo 'usage: run-travel-booking-refunds.sh SIMULATOR_UUID P02_DERIVED_DATA FIXTURE RESULT CATALOG_PORT' >&2
  exit 2
fi
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
export NU54_BOOKING_CATALOG_PORT=$5
exec bash "$here/run-subscription-denied.sh" "$1" "$2" "$3" "$4"
