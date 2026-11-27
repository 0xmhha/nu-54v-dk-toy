"""Build local laboratory credential pins for two existing disposable pilot fixtures."""
import hashlib
import os
from pathlib import Path
import subprocess
import sys


def openssl(*args, data=None):
    return subprocess.run(
        [os.environ.get("OPENSSL", "openssl"), *map(str, args)], input=data,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True,
    ).stdout


def board_public(directory):
    cert = directory / "board.pem"
    public = openssl("x509", "-in", cert, "-pubkey", "-noout")
    return openssl("pkey", "-pubin", "-outform", "DER", data=public)


def main():
    source, target, destination = map(Path, sys.argv[1:])
    destination = destination.resolve()
    repo = Path(__file__).resolve().parents[3]
    if destination.is_relative_to(repo):
        raise SystemExit("Use a new private directory outside the checkout")
    source_pub, target_pub = board_public(source), board_public(target)
    if len(source_pub) != 91 or len(target_pub) != 91 or source_pub == target_pub:
        raise SystemExit("Two distinct P-256 board certificate public keys are required")
    os.umask(0o077)
    destination.mkdir(mode=0o700)
    ca = destination / "accessory-ca.pem"
    ca_key = destination / "accessory-ca.key"
    openssl("req", "-new", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes",
            "-days", 1, "-subj", "/CN=NU54 Disposable Recovery CA", "-keyout", ca_key, "-out", ca)
    key = destination / "accessory.key"
    cert = destination / "accessory.pem"
    request = destination / "accessory.csr"
    ext = destination / "accessory.ext"
    openssl("req", "-new", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes",
            "-subj", "/CN=NU54 Disposable Recovery Panel", "-keyout", key, "-out", request)
    ext.write_text("subjectAltName=URI:urn:nu54:role:recovery-accessory\n"
                   "basicConstraints=critical,CA:FALSE\nkeyUsage=digitalSignature\n"
                   "extendedKeyUsage=clientAuth\n")
    openssl("x509", "-req", "-in", request, "-CA", ca, "-CAkey", ca_key, "-set_serial", 1,
            "-days", 1, "-extfile", ext, "-out", cert)
    pub = openssl("pkey", "-in", key, "-pubout", "-outform", "DER")
    fields = {
        "ca_cert": ca.read_bytes() + b"\0",
        "source_key": source_pub,
        "target_key": target_pub,
        "proof_key": pub,
        "peer_pin": hashlib.sha256(pub).digest(),
    }
    header = destination / "recovery_fixture.h"
    header.write_text("\n".join(
        f"static const uint8_t accessory_{name}[] = {{" + ",".join(map(str, data)) + "};"
        for name, data in fields.items()
    ) + "\n")
    (destination / "boards-ca.pem").write_bytes((source / "ca.pem").read_bytes() + (target / "ca.pem").read_bytes())
    print(header)


if __name__ == "__main__":
    if len(sys.argv) != 4:
        raise SystemExit("usage: make_recovery_fixture.py SOURCE_PILOT_DIR TARGET_PILOT_DIR OUTPUT")
    main()
