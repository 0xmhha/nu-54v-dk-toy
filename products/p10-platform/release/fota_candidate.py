#!/usr/bin/env python3
"""Issue a read-only W12 candidate receipt after checking a custom deployment."""

import argparse
import hashlib
import json
import re
import subprocess
import tempfile
import time
from pathlib import Path

from evidence import authenticate, digest, key_sha256, openssl, parse, read, require


def issue(args):
    trust = read(args.deployment_trust)
    live = read(args.running_observation)
    statement = args.descriptor.read_bytes()
    require(len(statement) <= 2048, "descriptor too long")
    release = parse(statement)
    require(set(trust) == {"board", "boardRevision", "channel", "stationKeySha256", "publisherKeySha256", "bootKeySha256"}
            and trust["channel"] == "custom", "qualified custom deployment trust required")
    require(set(live) == {"board", "generation", "measurement", "counter"}
            and live["board"] == trust["board"], "running observation outside deployed board")
    require(all(isinstance(v, str) and re.fullmatch(r"[a-f0-9]{64}", v) and int(v, 16) > 0 for v in
                (live["board"], live["measurement"], trust["stationKeySha256"], trust["publisherKeySha256"], trust["bootKeySha256"])),
            "invalid deployment fingerprint")
    require(isinstance(live["generation"], str) and re.fullmatch(r"[1-9][0-9]{0,19}", live["generation"])
            and int(live["generation"]) <= 2**64 - 1, "invalid owner generation")
    require(set(release) == {"version", "boardRevision", "counter", "channel", "measurement", "imageSha256"}
            and release["boardRevision"] == trust["boardRevision"] and release["channel"] == trust["channel"]
            and type(live["counter"]) is int and type(release["counter"]) is int
            and 0 < live["counter"] < release["counter"] <= 65535, "release outside authorized revision, channel or counter")
    require(release["measurement"] != live["measurement"], "candidate measurement equals running measurement")
    signature = args.descriptor_signature.read_bytes()
    require(len(signature) == 64, "invalid publisher signature")
    with tempfile.TemporaryDirectory(prefix="nu54-candidate-") as directory:
        scratch = Path(directory)
        signature_file = scratch / "publisher.sig"
        signature_file.write_bytes(signature)
        boot_public = scratch / "boot.pem"
        boot_public.write_bytes(args.boot_public.read_bytes())
        publisher = authenticate(statement, signature_file, args.publisher_public.read_bytes())
        boot = key_sha256(boot_public)
        station = hashlib.sha256(openssl(["pkey", "-in", args.station_key, "-pubout", "-outform", "DER"])).hexdigest()
        require((station, publisher, boot) == (trust["stationKeySha256"], trust["publisherKeySha256"], trust["bootKeySha256"])
                and len({station, publisher, boot}) == 3, "deployment signing keys differ")
        image = scratch / "candidate.bin"
        with args.image.open("rb") as source:
            image_bytes = source.read(1024 * 1024 + 1)
        require(len(image_bytes) <= 1024 * 1024, "candidate image too large")
        image.write_bytes(image_bytes)
        require(digest(image) == release["imageSha256"], "candidate image differs")
        check = subprocess.run([str(args.verify_firmware.resolve()), "-key", str(boot_public),
                                "-image", str(image), "-version", release["version"], "-board", release["boardRevision"],
                                "-counter", str(release["counter"])], capture_output=True)
        require(check.returncode == 0 and check.stdout.decode().strip() == release["measurement"],
                "counted MCUboot signature or measurement differs")
        issued = int(time.time())
        payload = json.dumps({"format": "nu54-firmware-candidate-v1", "model": "nu54v_dk/nrf54l15/cpuapp",
                              **live, "bootKey": boot, "descriptor": statement.decode(), "descriptorSignature": signature.hex(),
                              "issuedAt": issued, "expiresAt": issued + 120}, separators=(",", ":"), sort_keys=True)
        payload_file = scratch / "receipt.json"
        payload_file.write_text(payload)
        signed = openssl(["pkeyutl", "-sign", "-rawin", "-inkey", args.station_key, "-in", payload_file])
        require(len(signed) == 64, "station must use Ed25519")
        with args.output.open("x") as output:
            json.dump({"payload": payload, "signature": signed.hex()}, output, separators=(",", ":"))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("deployment-trust", "running-observation", "descriptor", "descriptor-signature", "image",
                 "publisher-public", "boot-public", "station-key", "verify-firmware", "output"):
        parser.add_argument(f"--{name}", type=Path, required=True)
    args = parser.parse_args()
    try:
        issue(args)
    except (ValueError, KeyError, OSError, TypeError) as exc:
        parser.exit(1, f"{exc}\n")


if __name__ == "__main__":
    main()
