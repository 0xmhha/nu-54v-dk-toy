#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"
adb_bin="${ANDROID_HOME:?Set ANDROID_HOME}/platform-tools/adb"
serial="${1:?Pass the serial of an isolated Android emulator}"
[[ "$serial" =~ ^emulator-[0-9]+$ ]]
scratch="$(mktemp -d)"
server=""
port=""
cleanup() {
    if [[ -n "$server" ]]; then kill "$server" 2>/dev/null || true; fi
    if [[ -n "$port" ]]; then "$adb_bin" -s "$serial" reverse --remove "tcp:$port" >/dev/null 2>&1 || true; fi
    rm -rf "$scratch"
}
trap cleanup EXIT

openssl_root="${NU54_OPENSSL_ROOT:-}"
if [[ -z "$openssl_root" ]] && command -v brew >/dev/null; then
    openssl_root="$(brew --prefix openssl@3)"
fi
cmake_args=(-S ../../p01-device-firmware/test -B "$scratch/build")
if [[ -n "$openssl_root" ]]; then
    PATH="$openssl_root/bin:$PATH"
    cmake_args+=("-DOPENSSL_ROOT_DIR=$openssl_root")
fi
cmake "${cmake_args[@]}" >"$scratch/build.log" 2>&1 ||
    { cat "$scratch/build.log"; exit 1; }
cmake --build "$scratch/build" --target owner_registration_host >>"$scratch/build.log" 2>&1 ||
    { cat "$scratch/build.log"; exit 1; }
./gradlew --no-daemon -PreactNativeArchitectures=arm64-v8a :app:assembleDebug :app:assembleDebugAndroidTest
"$adb_bin" -s "$serial" install -r app/build/outputs/apk/debug/app-debug.apk
"$adb_bin" -s "$serial" install -r app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk
openssl req -new -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -noenc -days 1 -subj /CN=NU54-Lab-Root -keyout "$scratch/ca.key" -out "$scratch/ca.pem" 2>/dev/null
for role in board owner; do
    openssl req -new -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -noenc -subj "/CN=NU54-$role" -keyout "$scratch/$role.key" -out "$scratch/$role.csr" 2>/dev/null
    usage=serverAuth
    if [[ "$role" == owner ]]; then usage=clientAuth; fi
    printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=%s\nsubjectAltName=URI:urn:nu54:role:%s\n' "$usage" "$role" >"$scratch/$role.ext"
    openssl x509 -req -in "$scratch/$role.csr" -CA "$scratch/ca.pem" -CAkey "$scratch/ca.key" -CAcreateserial \
        -extfile "$scratch/$role.ext" -days 1 -out "$scratch/$role.pem" 2>/dev/null
done
pin="$(openssl x509 -in "$scratch/board.pem" -pubkey -noout | openssl pkey -pubin -outform DER | shasum -a 256 | cut -d' ' -f1)"
"$scratch/build/owner_registration_host" "$scratch" serve 0 3 90 >"$scratch/server.log" 2>"$scratch/server-errors.log" &
server=$!
for _ in {1..40}; do
    if [[ -s "$scratch/server.log" ]]; then
        port="$(awk '/^PORT / {print $2}' "$scratch/server.log")"
        if [[ "$port" =~ ^[0-9]+$ ]]; then break; fi
    fi
    kill -0 "$server"
    sleep 0.1
done
[[ "$port" =~ ^[0-9]+$ ]]
"$adb_bin" -s "$serial" reverse "tcp:$port" "tcp:$port"
"$adb_bin" -s "$serial" shell am instrument -w \
    -e port "$port" -e pin "$pin" \
    -e ca "$(openssl x509 -in "$scratch/ca.pem" -outform DER | openssl base64 -A)" \
    -e cert "$(openssl x509 -in "$scratch/owner.pem" -outform DER | openssl base64 -A)" \
    -e priv "$(openssl pkcs8 -topk8 -in "$scratch/owner.key" -nocrypt -outform DER | openssl base64 -A)" \
    com.nu54renter.test/com.nu54renter.ble.OwnerTlsLab | tee "$scratch/client.log"
if ! grep -q '^INSTRUMENTATION_RESULT: owner_tls_lab=pass' "$scratch/client.log"; then
    cat "$scratch/server.log" "$scratch/server-errors.log" >&2
    exit 1
fi
wait "$server"
server=""
[[ "$(grep -c '^STATUS 0 ' "$scratch/server.log")" = 1 ]]
[[ "$(grep -c '^Wire2 management request rejected$' "$scratch/server-errors.log")" = 1 ]]
grep -q '^INSTRUMENTATION_RESULT: epoch=alpn-matched-and-mismatch-rejected' "$scratch/client.log"
grep -q '^INSTRUMENTATION_RESULT: ng=wrong-pin-rejected' "$scratch/client.log"
