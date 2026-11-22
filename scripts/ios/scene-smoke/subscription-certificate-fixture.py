#!/usr/bin/env python3
"""Issue two disposable client profiles to one simulator's native public key."""
import argparse
import base64
import hashlib
import json
from datetime import datetime, timedelta, timezone
from pathlib import Path
from uuid import uuid4

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID


def der(cert):
    return base64.b64encode(cert.public_bytes(serialization.Encoding.DER)).decode()


def issue(label, key, start, end, issuer=None, parent=None, is_ca=False):
    name = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, label)])
    builder = (x509.CertificateBuilder().subject_name(name)
               .issuer_name(parent.subject if parent else name).public_key(key)
               .serial_number(x509.random_serial_number())
               .not_valid_before(start).not_valid_after(end)
               .add_extension(x509.BasicConstraints(ca=is_ca, path_length=None), critical=True))
    if not is_ca:
        builder = builder.add_extension(x509.ExtendedKeyUsage([ExtendedKeyUsageOID.CLIENT_AUTH]), critical=False)
    return builder.sign(issuer, hashes.SHA256())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("slot", choices=("customer", "terminal"))
    parser.add_argument("public_key", type=Path)
    args = parser.parse_args()
    device = serialization.load_der_public_key(base64.b64decode(args.public_key.read_text().strip(), validate=True))
    if not isinstance(device, ec.EllipticCurvePublicKey) or not isinstance(device.curve, ec.SECP256R1):
        parser.error("expected the native enrollment P-256 public key")
    start = datetime.now(timezone.utc) - timedelta(minutes=1)
    expiry = start + timedelta(minutes=16)
    root_key = ec.generate_private_key(ec.SECP256R1())
    intermediate_key = ec.generate_private_key(ec.SECP256R1())
    root = issue("Synthetic root", root_key.public_key(), start, expiry, root_key, is_ca=True)
    intermediate = issue("Synthetic intermediate", intermediate_key.public_key(), start, expiry, root_key, root, True)
    direct = issue("Direct native client", device, start, expiry, root_key, root)
    chained = issue("Chained native client", device, start, expiry, intermediate_key, intermediate)
    pin = hashlib.sha256(root_key.public_key().public_bytes(
        serialization.Encoding.DER, serialization.PublicFormat.SubjectPublicKeyInfo)).hexdigest()
    common = {"endpoint": "https://127.0.0.1:1/v2/commerce", "pin": pin, "ca": der(root),
              "scope": {"role": args.slot, "expiresAt": int(expiry.timestamp()),
                        "tenantRef": str(uuid4()), "branchRef": str(uuid4()), "subjectRef": str(uuid4())}}
    print(json.dumps({"slot": args.slot, "profiles": [
        {**common, "certificates": [der(direct)]},
        {**common, "certificates": [der(chained), der(intermediate)]},
    ]}, separators=(",", ":")))


if __name__ == "__main__":
    main()
