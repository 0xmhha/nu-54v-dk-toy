"""Independent display side of the opt-in travel settlement host test."""

import contextlib
from io import StringIO
import json
import os
from pathlib import Path
import subprocess
import sys

from coincurve.ecdsa import der_to_cdata, serialize_compact

ROOT = Path(__file__).resolve().parents[4]
sys.path.insert(0, str(ROOT / "products/p01-device-firmware/recovery-accessory"))
sys.path.insert(0, str(ROOT / "packages/protocol/python"))
from payment_presentation import payment_body, verify_presentation


def main():
    os.umask(0o077)
    directory = Path(sys.argv[1])
    expected = json.loads((directory / "expected.json").read_text())
    projection = json.loads((directory / "projection.json").read_text())
    quote = bytes.fromhex(projection["signedQuote"])
    verify_presentation(quote, projection, expected["presentationKey"], expected["branch"],
                        expected["customer"], sale=expected["sale"], beneficiary=expected["recipient"][2:])
    challenge = (directory / "challenge").read_bytes()
    with contextlib.redirect_stdout(StringIO()) as text:
        body = payment_body(challenge, quote, bytes.fromhex(expected["issuer"][2:]))
    visible = set(text.getvalue().splitlines())
    if not {
        f"StableNet {expected['chain']} escrow payment",
        f"Asset {expected['asset']}",
        f"Beneficiary {expected['recipient']}",
        f"Amount in base units {expected['amount']}",
        "Network fee payer: merchant relay",
    } <= visible:
        raise ValueError("The independently verified quote differs from the reviewed travel payment")

    fixture_key = Path(sys.argv[2]) if len(sys.argv) > 2 else None
    key = fixture_key or directory / "display-private.pem"
    try:
        if fixture_key is None:
            subprocess.run(["openssl", "genpkey", "-algorithm", "EC", "-pkeyopt", "ec_paramgen_curve:P-256",
                            "-out", key], check=True, capture_output=True)
            subprocess.run(["openssl", "pkey", "-in", key, "-pubout", "-out", directory / "display-public.pem"],
                           check=True, capture_output=True)
        der = subprocess.check_output(["openssl", "dgst", "-sha256", "-sign", key], input=body)
        proof = serialize_compact(der_to_cdata(der))
    finally:
        if fixture_key is None:
            key.unlink(missing_ok=True)
    (directory / "proof").write_bytes(proof)
    # Mode 1 is a merchant relay quote. The relay computes and pays gas on submission.
    print("Authenticated exact travel quote, full beneficiary, chain and merchant relay fee payer")


if __name__ == "__main__":
    main()
