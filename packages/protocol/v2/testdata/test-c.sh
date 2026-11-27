#!/bin/sh
set -eu
here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
protocol=$(CDPATH='' cd -- "$here/../.." && pwd)
repo=$(CDPATH='' cd -- "$protocol/../.." && pwd)
source_sdk=${1:-"$protocol/c"}
fixtures=${2:-"$here/quote.vectors"}
firmware="$repo/products/p01-device-firmware"
secp="$firmware/third_party/secp256k1"
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT

"${CC:-cc}" -std=c11 -O2 -Wall -Wextra -Werror     -I "$source_sdk/include" -I "$firmware/core/include"     -c "$source_sdk/src/nu54_quote_v2.c" -o "$out/quote.o"
"${CC:-cc}" -std=c11 -O2 -DENABLE_MODULE_RECOVERY=1 -DECMULT_WINDOW_SIZE=4     -I "$source_sdk/include" -I "$firmware/core/include" -I "$secp/include"     "$out/quote.o" "$protocol/c/test/quote_v2_test.c"     "$firmware/core/src/nu54_keccak.c" "$firmware/core/src/nu54_sig.c"     "$firmware/third_party/keccak/Keccak-readable-and-compact.c"     "$secp/src/secp256k1.c" "$secp/src/precomputed_ecmult.c" "$secp/src/precomputed_ecmult_gen.c"     -o "$out/quote-v2-test"
"$out/quote-v2-test" "$fixtures"
