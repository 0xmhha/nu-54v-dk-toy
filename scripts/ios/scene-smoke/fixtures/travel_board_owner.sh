#!/usr/bin/env bash
# Loaded by the private travel acceptance runner. The QA bundle is simulator-only.
prepare_travel_board() {
  local app=$1 pending=$2
  local repo owner_dir documents board_address device pin scope
  repo=$(cd "$here/../../.." && pwd)
  owner_dir="$fixture/board"
  mkdir "$owner_dir"
  capture "$scratch/owner-clear.log" bash "$here/run-subscription-native.sh" "$sim" "$app" "$scratch/OwnerClear" testClearTravelBoardWallet
  documents="$(xcrun simctl get_app_container "$sim" com.nu54renter.w04qa data)/Documents"
  mkdir -p "$documents"
  scope=$(uuidgen)
  printf '%s' "$scope" >"$owner_dir/owner-scope"
  printf '%s' "$scope" >"$documents/w04-key-scope"
  NU54_TRAVEL_OWNER_SCOPE="$scope" capture "$scratch/owner-key.log" bash "$here/run-subscription-native.sh" "$sim" "$app" "$scratch/OwnerKey" testPaymentFixtureOwnerIdentity
  python3 - "$scratch/owner-key.log" "$owner_dir/owner.der" <<'PYTHON'
import base64, re, sys
from pathlib import Path
log = Path(sys.argv[1]).read_text()
points = re.findall(r"NU54_PAYMENT_OWNER_SPKI=([A-Za-z0-9+/]{122}==)(?:\s|$)", log)
assert len(set(points)) == 1
der = base64.b64decode(points[0], validate=True)
assert len(der) == 91
Path(sys.argv[2]).write_bytes(der)
PYTHON

  capture "$scratch/board-build.log" cmake -S "$repo/products/p01-device-firmware/test" -B "$scratch/board" -DCMAKE_BUILD_TYPE=Debug
  capture "$scratch/board-link.log" cmake --build "$scratch/board" --target owner_registration_host test_install
  local host="$scratch/board/owner_registration_host"
  board_address=$("$host" "$owner_dir" inspect | awk '{print $3}')
  device=$(printf %s "$board_address" | xxd -r -p | shasum -a 256 | awk '{print $1}')
  (
    cd "$owner_dir"
    openssl req -new -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 1       -subj /CN=NU54-Travel-Board-QA -keyout ca.key -out ca.pem
    openssl req -new -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes       -subj /CN=NU54-Travel-Board -keyout board.key -out board.csr
    printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=serverAuth\nsubjectAltName=URI:urn:nu54:role:board\n' >board.ext
    printf 'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature\nextendedKeyUsage=clientAuth\nsubjectAltName=URI:urn:nu54:role:owner\n' >owner.ext
    openssl x509 -req -in board.csr -CA ca.pem -CAkey ca.key -CAcreateserial -extfile board.ext -days 1 -out board.pem
    openssl pkey -pubin -inform DER -in owner.der -out owner.pub
    openssl x509 -new -force_pubkey owner.pub -CA ca.pem -CAkey ca.key -extfile owner.ext -days 1       -subj /CN=NU54-Travel-Owner -out owner.pem
  ) >"$scratch/board-tls.log" 2>&1
  pin=$(openssl x509 -in "$owner_dir/board.pem" -pubkey -noout | openssl pkey -pubin -outform DER | shasum -a 256 | awk '{print $1}')
  (cd "$repo" && node --experimental-strip-types --input-type=module - "$owner_dir" <<'NODE'
import {writeFileSync} from "node:fs";
import {addressOfPrivateKey, hexToBytes} from "./packages/protocol/ts/src/index.ts";
const key = value => Uint8Array.from({length: 32}, (_, i) => i === 31 ? value : 0);
writeFileSync(process.argv[2] + "/payment-key.fixture",
  Buffer.concat([hexToBytes(addressOfPrivateKey(key(101))), key(102)]));
writeFileSync(process.argv[2] + "/payer", addressOfPrivateKey(key(102)));
NODE
  )
  python3 - "$owner_dir" "$device" "$pin" >"$pending" <<'PY'
import base64, json, ssl, sys
from pathlib import Path
directory, device, pin = Path(sys.argv[1]), sys.argv[2], sys.argv[3]
def cert(name):
    return base64.b64encode(ssl.PEM_cert_to_DER_cert((directory / (name + ".pem")).read_text())).decode()
print(json.dumps({
    "version": 2, "exercised": False, "mode": "personal",
    "enrollment": {"version": 2, "device": device, "boardSPKI": pin, "ca": cert("ca"),
                   "certificateChain": [cert("owner")]},
    "verified": {"boardFingerprint": device, "address": (directory / "payer").read_text(),
                 "network": "8283", "generation": 1},
}))
PY
  NU54_SUBSCRIPTION_UI_FIXTURE="$(openssl base64 -A -in "$pending")"     capture "$scratch/owner-seed.log" bash "$here/run-subscription-native.sh" "$sim" "$app" "$scratch/OwnerSeed" testSeedTravelBoardWallet
  mkdir "$fixture/display"
  openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out "$fixture/display/key.pem" >/dev/null 2>&1
  openssl pkey -in "$fixture/display/key.pem" -pubout -out "$owner_dir/display-key" >/dev/null 2>&1
  NU54_BOARD_PAYMENT_INSTALL_TEST="$scratch/board/test_install"     "$host" "$owner_dir" serve 0 8 720 >"$scratch/board.log" 2>&1 &
  board_pid=$!
  local port=""
  for _ in {1..40}; do
    port=$(awk '/^PORT / {print $2; exit}' "$scratch/board.log")
    [[ -z "$port" ]] || break
    kill -0 "$board_pid"
    sleep .25
  done
  [[ "$port" =~ ^[0-9]+$ ]]
  documents="$(xcrun simctl get_app_container "$sim" com.nu54renter.w04qa data)/Documents"
  mkdir -p "$documents"
  printf '%s' "$scope" >"$documents/w04-key-scope"
  printf '%s' "$port" >"$documents/w04-host-port"
}
