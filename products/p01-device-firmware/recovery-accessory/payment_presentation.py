"""Verify a private P05 projection on the independently provisioned payment display."""

from datetime import date
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile
import time
import uuid

from nu54_protocol.quote_v2 import quote_digest

DOMAIN = b"NU54-private-presentation-v1\0"
ED25519_SPKI = bytes.fromhex("302a300506032b6570032100")


def payment_body(challenge, quote, merchant_signer=None):
    if len(quote) != 737 or len(challenge) != 32:
        raise ValueError("Malformed quote or challenge")
    word = lambda index: quote[index * 32:(index + 1) * 32]
    if int.from_bytes(word(0), "big") != 8283 or int.from_bytes(word(19), "big") != 1:
        raise ValueError("Unsupported network or payment mode")
    if merchant_signer is not None:
        from nu54_protocol.quote_v2 import QuotePolicy, verify_quote

        if verify_quote(quote, QuotePolicy(quote[:672], merchant_signer, int(time.time()), False)):
            raise ValueError("Quote issuer differs from the registered signer or quote is expired")
        print("Issuer", "0x" + merchant_signer.hex())
    print("StableNet 8283 escrow payment")
    print("Order", word(5).hex())
    print("Merchant", word(6).hex())
    print("Settlement", "0x" + word(1)[12:].hex())
    print("Asset", "0x" + word(10)[12:].hex())
    print("Beneficiary", "0x" + word(11)[12:].hex())
    print("Payer", "0x" + word(9)[12:].hex())
    print("Amount in base units", int.from_bytes(word(12), "big"))
    print("Network fee payer: merchant relay")
    return b"N54D" + challenge + quote


def reference(value):
    parsed = uuid.UUID(value)
    if parsed.version != 4 or str(parsed) != value:
        raise ValueError("Presentation reference is not canonical")
    return parsed.bytes


def exact(value, width):
    if not isinstance(value, str) or not re.fullmatch(rf"[0-9a-f]{{{width * 2}}}", value):
        raise ValueError("Malformed private presentation field")
    return bytes.fromhex(value)


def verify_presentation(quote, projection, trusted_key, branch, customer, sale=None, origin=None, now=None, invoice=None, beneficiary=None):
    if sale is None and origin is None:
        raise ValueError("Supply an independently known sale or origin reference")
    body = quote[:672]
    proof = projection["presentation"]
    deadline = int.from_bytes(body[576:608], "big")
    generation = int.from_bytes(body[512:544], "big")
    timestamp = int(time.time()) if now is None else now
    if len(quote) != 737 or not generation or not timestamp < deadline or projection["expiresAt"] != deadline:
        raise ValueError("Private presentation expired or invalid")
    if projection["branchRef"] != branch or proof["customer"] != customer or (sale and projection["saleRef"] != sale) or (origin and proof["origin"] != origin):
        raise ValueError("Private presentation belongs to a different checkout")
    digest = quote_digest(body)
    if projection["quoteDigest"] != digest.hex() or projection["orderKey"] != body[160:192].hex():
        raise ValueError("Private presentation does not bind the staged quote")
    payload = DOMAIN + b"".join(map(reference, (branch, projection["saleRef"], customer, proof["origin"])))
    payload += exact(proof["claim"], 32) + body[160:192] + digest
    payload += generation.to_bytes(8, "big") + deadline.to_bytes(8, "big")
    verify_signature(payload, proof["signature"], trusted_key)
    signed_invoice = proof.get("invoice")
    projected_invoice = None if signed_invoice is None else {
        key: signed_invoice.get(key) for key in ("cycle", "termsVersion")
    }
    if projected_invoice != invoice or (invoice is not None and origin is None):
        raise ValueError("Private presentation belongs to a different invoice")
    if beneficiary is not None and body[364:384] != exact(beneficiary, 20):
        raise ValueError("Quote belongs to a different beneficiary")
    if projected_invoice is not None:
        cycle = projected_invoice["cycle"]
        terms = projected_invoice["termsVersion"].encode("utf-8")
        if date.fromisoformat(cycle).isoformat() != cycle or len(terms) not in range(1, 161):
            raise ValueError("Malformed private invoice presentation")
        payload += b"\x01" + cycle.encode("ascii") + len(terms).to_bytes(2, "big") + terms + hashlib.sha256(quote).digest()
        verify_signature(payload, signed_invoice["signature"], trusted_key)
    return projected_invoice


def verify_signature(payload, signature, trusted_key):
    with tempfile.TemporaryDirectory(prefix="nu54-presentation-") as scratch:
        directory = Path(scratch)
        for name, data in (("key", ED25519_SPKI + exact(trusted_key, 32)), ("proof", exact(signature, 64)), ("payload", payload)):
            (directory / name).write_bytes(data)
        checked = subprocess.run([
            "openssl", "pkeyutl", "-verify", "-pubin", "-keyform", "DER", "-rawin",
            "-inkey", directory / "key", "-sigfile", directory / "proof", "-in", directory / "payload",
        ], capture_output=True, check=False)
    if checked.returncode:
        raise ValueError("Unauthenticated private presentation")


def read_private_projection(path):
    with path.open("rb") as stream:
        raw = stream.read(8193)
    if len(raw) > 8192:
        raise ValueError("Private presentation exceeds limit")
    return json.loads(raw)
