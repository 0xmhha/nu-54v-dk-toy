# Sourced only after the caller has cloned the application into its owned workspace.
candidate_dir="$registration_test_work/candidate"
candidate_release="$NU54_PICKUP_READONLY_ROOT/products/p10-platform/release"
mkdir "$candidate_dir" "$candidate_dir/cache" "$candidate_dir/go-tmp"

candidate_prepare() {
  local config="$root/products/p02-user-app/src/wallet/firmwareCandidateDeployment.ts"
  [[ -f "$config" && ! -L "$config" && -n "$NU54_PICKUP_READONLY_ROOT" && "$root" != "$NU54_PICKUP_READONLY_ROOT" ]]
  (
    cd "$NU54_PICKUP_READONLY_ROOT"
    GOCACHE="$candidate_dir/cache" GOTMPDIR="$candidate_dir/go-tmp" \
      go build -o "$candidate_dir/verify-firmware" ./products/p05-operations-backoffice/cmd/verify-firmware
  )
  (
    cd "$candidate_dir"
    cp "$running/qa.key" boot.key
    rm -f -- "$running/qa.key"
    for role in station publisher; do
      "$openssl" genpkey -algorithm ED25519 -out "$role.key"
      "$openssl" pkey -in "$role.key" -pubout -out "$role.pem"
    done
    "$openssl" pkey -in boot.key -pubout -out boot.pem
    printf 'disposable counted update candidate fixture\n' >input.bin
    "${imgtool[@]}" sign -k boot.key --sha 512 --pad-header --align 4 -H 512 -S 65536 \
      -v '2.4.0' -s 513 input.bin signed.bin
    python3 -B - "$device" "$running_measurement" "$config" "$openssl" <<'PY'
import hashlib
import json
import sys
import subprocess
from pathlib import Path

board, measurement, config, openssl = sys.argv[1:]
shared = dict(board=board, boardRevision="nu54_rev1", channel="custom")
pins, public = {}, {}
for role in ("station", "publisher", "boot"):
    der = subprocess.check_output([openssl, "pkey", "-pubin", "-in", role + ".pem", "-outform", "DER"])
    assert der[:12].hex() == "302a300506032b6570032100" and len(der) == 44
    pins[role + "KeySha256"] = hashlib.sha256(der).hexdigest()
    public[role + "Key"] = der[12:].hex()
Path("trust.json").write_text(json.dumps(shared | pins))
Path("observation.json").write_text(json.dumps(dict(board=board, generation="1", measurement=measurement, counter=512)))
image = Path("signed.bin").read_bytes()
Path("build.json").write_text(json.dumps(dict(channel="custom", debugProtection=True,
    artifacts={"app/zephyr/zephyr.signed.bin": hashlib.sha256(image).hexdigest()},
    serialRecovery={"hardwareSecurityCounter": 513})))
Path(config).write_text("import type {FirmwareCandidateTrust} from './firmwareCandidate.ts';\n"
    "export const firmwareCandidateDeployment:FirmwareCandidateTrust = " + json.dumps(shared | public) + ";\n")
PY
    PATH="$(dirname "$openssl"):$PATH" python3 -B "$candidate_release/fota_descriptor.py" \
      --channel custom --board-revision nu54_rev1 --build-manifest build.json --image signed.bin \
      --publisher-key publisher.key --output descriptor.json --signature descriptor.sig
  )
}

candidate_verify_and_stage() (
  cd "$candidate_dir"
  PATH="$(dirname "$openssl"):$PATH" python3 -B "$candidate_release/fota_candidate.py" \
    --deployment-trust trust.json --running-observation observation.json --descriptor descriptor.json \
    --descriptor-signature descriptor.sig --image signed.bin --publisher-public publisher.pem \
    --boot-public boot.pem --station-key station.key --verify-firmware "$candidate_dir/verify-firmware" --output receipt.json \
    >"$out/candidate-receipt.log" 2>&1 || {
      echo 'W12 candidate fixture station verification failed' >&2
      exit 1
    }
  echo 'W12 candidate fixture station verification passed'
  cp receipt.json "$documents/w12-valid-candidate.json" || {
    echo 'W12 candidate fixture valid document staging failed' >&2
    exit 1
  }
  python3 -B - "$documents/w12-invalid-candidate.json" <<'PY'
from pathlib import Path
import json
import sys

receipt = json.loads(Path("receipt.json").read_bytes())
receipt["signature"] = "ab" * 64
Path(sys.argv[1]).write_text(json.dumps(receipt))
PY
)
