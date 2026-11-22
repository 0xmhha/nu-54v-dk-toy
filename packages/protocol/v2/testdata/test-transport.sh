#!/bin/sh
set -eu
here=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
protocol=$(CDPATH='' cd -- "$here/../.." && pwd)
repo=$(CDPATH='' cd -- "$protocol/../.." && pwd)
firmware="$repo/products/p01-device-firmware"
secp="$firmware/third_party/secp256k1"
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT

cc=${CC:-cc}
if [ -z "${OPENSSL_PREFIX:-}" ] && [ "$(uname -s)" = Darwin ] && command -v brew >/dev/null 2>&1; then
    OPENSSL_PREFIX=$(brew --prefix openssl@3)
fi
if [ -n "${OPENSSL_PREFIX:-}" ]; then
    set -- "-I$OPENSSL_PREFIX/include" "-L$OPENSSL_PREFIX/lib"
    export OPENSSL="$OPENSSL_PREFIX/bin/openssl"
else
    set --
fi
"$cc" -std=c11 -O2 -Wall -Wextra -Werror -I "$protocol/c/include" "$@" \
    "$protocol/c/src/nu54_transport_v2.c" "$protocol/c/test/test_transport_v2.c" -lcrypto -o "$out/unit"
"$out/unit" "$here/payment-review.vectors"

"$cc" -std=c11 -O2 -DENABLE_MODULE_RECOVERY=1 -DECMULT_WINDOW_SIZE=4 \
    -I "$protocol/c/include" -I "$firmware/core/include" -I "$secp/include" "$@" \
    "$protocol/c/src/nu54_transport_v2.c" "$protocol/c/src/nu54_transport_v2_openssl.c" \
    "$protocol/c/src/nu54_quote_v2.c" "$protocol/c/test/transport_tls_peer.c" \
    "$firmware/core/src/nu54_pilot.c" "$firmware/core/src/nu54_local_approval.c" \
    "$firmware/core/src/nu54_keccak.c" "$firmware/core/src/nu54_sig.c" \
    "$firmware/third_party/keccak/Keccak-readable-and-compact.c" \
    "$secp/src/secp256k1.c" "$secp/src/precomputed_ecmult.c" "$secp/src/precomputed_ecmult_gen.c" \
    -lssl -lcrypto -o "$out/peer"
if [ -n "${NU54_MOBILE_TEST_PEER_OUTPUT:-}" ]; then
    test ! -e "$NU54_MOBILE_TEST_PEER_OUTPUT"
    cp "$out/peer" "$NU54_MOBILE_TEST_PEER_OUTPUT"
else
    NU54_V2_TLS_TEST_BIN="$out/peer" node --experimental-strip-types --test "$protocol/ts/interop/transport-tls.test.ts"
fi
