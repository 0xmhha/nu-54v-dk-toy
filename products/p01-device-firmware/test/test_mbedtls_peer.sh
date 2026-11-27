#!/bin/sh
set -eu

firmware=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
protocol="$firmware/../../packages/protocol"
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
mkdir "$out/tmp"
export TMPDIR="$out/tmp" TMP="$out/tmp" TEMP="$out/tmp"
export PYTHONPATH="$firmware/scripts"
python3 -B - "$out" <<'PY'
from pathlib import Path
import sys
from mbedtls_input import prepare

root = prepare(Path(sys.argv[1]))
config = root / "include/mbedtls/mbedtls_config.h"
# Match the product's certificate-only profile while retaining a host client.
disabled = {
    "MBEDTLS_SSL_PROTO_TLS1_2", "MBEDTLS_SSL_PROTO_DTLS",
    "MBEDTLS_SSL_DTLS_ANTI_REPLAY", "MBEDTLS_SSL_DTLS_CLIENT_PORT_REUSE",
    "MBEDTLS_SSL_DTLS_CONNECTION_ID", "MBEDTLS_SSL_DTLS_HELLO_VERIFY",
    "MBEDTLS_SSL_RENEGOTIATION", "MBEDTLS_SSL_EARLY_DATA", "MBEDTLS_SSL_SESSION_TICKETS",
    "MBEDTLS_SSL_CACHE_C", "MBEDTLS_SSL_TLS1_3_KEY_EXCHANGE_MODE_PSK_ENABLED",
    "MBEDTLS_SSL_TLS1_3_KEY_EXCHANGE_MODE_PSK_EPHEMERAL_ENABLED",
    "MBEDTLS_KEY_EXCHANGE_ECJPAKE_ENABLED",
}
config.write_text("".join(line for line in config.read_text().splitlines(keepends=True)
                          if not (line.startswith("#define ") and line.split()[1] in disabled)))
Path(sys.argv[1], "source").write_text(str(root))
PY
source=$(cat "$out/source")
# The pinned library is checked by its digest, not by its warnings: a newer compiler warns inside it (ssl_tls.c).
cmake -S "$source" -B "$out/build" -DCMAKE_BUILD_TYPE=Release -DENABLE_TESTING=OFF -DENABLE_PROGRAMS=OFF -DMBEDTLS_FATAL_WARNINGS=OFF
cmake --build "$out/build" -j 4
"${CC:-cc}" -std=c11 -Wall -Wextra -Werror \
    -I "$source/include" -I "$source/tf-psa-crypto/include" \
    -I "$source/tf-psa-crypto/drivers/builtin/include" \
    -I "$protocol/c/include" -I "$firmware/app/src" -I "$firmware/core/include" \
    "$firmware/test/test_mbedtls_peer.c" "$firmware/app/src/device_tls_v2.c" "$firmware/core/src/nu54_install.c" \
    "$protocol/c/src/nu54_transport_v2.c" "$out/build/library/libmbedtls.a" \
    "$out/build/library/libmbedx509.a" "$out/build/tf-psa-crypto/core/libtfpsacrypto.a" -o "$out/peer"
cd "$out"
openssl req -new -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -keyout ca.key -out ca.crt \
    -noenc -subj /CN=nu54-fixture-ca -days 2
for role in board owner terminal; do
    if [ "$role" = board ]; then usage=serverAuth; else usage=clientAuth; fi
    printf 'basicConstraints=CA:FALSE\nkeyUsage=digitalSignature\nextendedKeyUsage=%s\nsubjectAltName=URI:urn:nu54:role:%s\n' \
        "$usage" "$role" >"$role.ext"
    openssl req -new -newkey ec -pkeyopt ec_paramgen_curve:P-256 -noenc \
        -keyout "$role.key" -out "$role.csr" -subj "/CN=$role"
    openssl x509 -req -in "$role.csr" -CA ca.crt -CAkey ca.key -CAcreateserial \
        -out "$role.crt" -days 1 -extfile "$role.ext"
done
./peer
