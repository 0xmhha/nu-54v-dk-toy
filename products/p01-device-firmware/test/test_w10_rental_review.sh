#!/usr/bin/env bash
set -euo pipefail
root=$(cd -- "$(dirname -- "$0")/../../.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/nu54-w10-review.XXXXXXXX")
trap 'rm -rf -- "$work"' EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
mkdir "$work/tmp"
export TMPDIR="$work/tmp"
cmake -S "$root/products/p01-device-firmware/test" -B "$work/build" ${OPENSSL_ROOT_DIR:+-DOPENSSL_ROOT_DIR="$OPENSSL_ROOT_DIR"} >"$work/config.log" 2>&1 ||
  { tail -30 "$work/config.log"; exit 1; }
cmake --build "$work/build" --target test_install --parallel 4 >"$work/build.log" 2>&1 ||
  { tail -40 "$work/build.log"; exit 1; }
ctest --test-dir "$work/build" --output-on-failure -R '^enrollment_policy$'
NU54_W10_REVIEW_C="$work/build/test_install" NU54_W10_REVIEW_INPUT="$work/review.input" \
  node --experimental-strip-types --test "$root/packages/protocol/ts/test/rental-payment-review.test.ts"
