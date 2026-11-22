#!/usr/bin/env bash
set -euo pipefail

root=$(cd -- "$(dirname -- "$0")/../../.." && pwd)
scratch=$(mktemp -d "${TMPDIR:-/tmp}/nu54-fota-candidate.XXXXXXXX")
trap 'rm -rf -- "$scratch"' EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
mkdir "$scratch/go-cache" "$scratch/go-tmp"
export GOCACHE="$scratch/go-cache" GOTMPDIR="$scratch/go-tmp" TMPDIR="$scratch"
export PYTHONDONTWRITEBYTECODE=1
cd "$root"
go build -o "$scratch/verify-firmware" ./products/p05-operations-backoffice/cmd/verify-firmware
release="$root/products/p10-platform/release"
cd "$scratch"

for role in station publisher boot; do
  openssl genpkey -algorithm ED25519 -out "$role.key"
  openssl pkey -in "$role.key" -pubout -out "$role.pem"
done
printf 'qa candidate image' > input.bin
"${PYTHON:-python3}" "${IMGTOOL:?Set IMGTOOL to the pinned MCUboot imgtool.py}" sign -k boot.key \
  --sha 512 --pad-header --align 4 -H 512 -S 65536 -v '2.3.258+65538' -s 513 input.bin signed.bin
python3 -B - <<'PY'
import hashlib
import json
import subprocess
from pathlib import Path

def save(name, contents):
    Path(name).write_text(json.dumps(contents))

image = Path("signed.bin").read_bytes()
save("build.json", {"channel": "custom", "debugProtection": True,
     "artifacts": {"app/zephyr/zephyr.signed.bin": hashlib.sha256(image).hexdigest()},
     "serialRecovery": {"hardwareSecurityCounter": 513}})
keys = {}
for role in ("station", "publisher", "boot"):
    der = subprocess.check_output(["openssl", "pkey", "-pubin", "-in", role + ".pem", "-outform", "DER"])
    keys[role + "KeySha256"] = hashlib.sha256(der).hexdigest()
save("trust.json", {"board": "a1" * 32, "boardRevision": "nu54_rev1", "channel": "custom", **keys})
save("observation.json", {"board": "a1" * 32, "generation": "7", "measurement": "b2" * 32, "counter": 512})
PY
python3 -B "$release/fota_descriptor.py" --channel custom --board-revision nu54_rev1 \
  --build-manifest build.json --image signed.bin --publisher-key publisher.key \
  --output release.json --signature release.sig
issue() {
  local statement="${4:-release.json}" signature="${5:-release.sig}"
  python3 -B "$release/fota_candidate.py" --deployment-trust trust.json \
    --running-observation observation.json --descriptor "$statement" --descriptor-signature "$signature" \
    --image "$1" --publisher-public publisher.pem --boot-public "$2" \
    --station-key station.key --verify-firmware "$scratch/verify-firmware" --output "$3"
}
issue signed.bin boot.pem receipt.json
if issue signed.bin publisher.pem denied.json; then exit 1; fi
cp signed.bin changed.bin
printf trailing >> changed.bin
if issue changed.bin boot.pem denied.json; then exit 1; fi
"${PYTHON:-python3}" "$IMGTOOL" sign -k publisher.key --sha 512 --pad-header --align 4 \
  -H 512 -S 65536 -v '2.3.258+65538' -s 513 input.bin wrong-signer.bin
python3 -B - <<'PYIMAGE'
from pathlib import Path
import hashlib
import json

build = json.loads(Path("build.json").read_bytes())
build["artifacts"]["app/zephyr/zephyr.signed.bin"] = hashlib.sha256(Path("wrong-signer.bin").read_bytes()).hexdigest()
Path("wrong-build.json").write_text(json.dumps(build))
PYIMAGE
python3 -B "$release/fota_descriptor.py" --channel custom --board-revision nu54_rev1 \
  --build-manifest wrong-build.json --image wrong-signer.bin --publisher-key publisher.key \
  --output wrong-release.json --signature wrong-release.sig
if issue wrong-signer.bin boot.pem denied.json wrong-release.json wrong-release.sig; then exit 1; fi
test ! -e denied.json
python3 -B - <<'PY'
import json
from pathlib import Path

receipt = json.loads(Path("receipt.json").read_bytes())
Path("receipt-payload.json").write_text(receipt["payload"])
Path("receipt.sig").write_bytes(bytes.fromhex(receipt["signature"]))
claim = json.loads(receipt["payload"])
assert claim["descriptor"] == Path("release.json").read_text()
assert claim["descriptorSignature"] == Path("release.sig").read_bytes().hex()
assert claim["generation"] == "7" and claim["counter"] == 512
assert claim["expiresAt"] - claim["issuedAt"] == 120
PY
openssl pkeyutl -verify -rawin -pubin -inkey station.pem -in receipt-payload.json -sigfile receipt.sig
