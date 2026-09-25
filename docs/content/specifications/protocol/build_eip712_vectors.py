#!/usr/bin/env python3
"""Build eip712-vectors.json from payment-protocol.schema.json.

Each vector carries the EIP-712 digest computed here (Keccak-256 via Foundry
`cast keccak`) and a signature produced by `cast wallet sign --data`, which
hashes the same typed data independently. The script fails if cast's recovered
signer differs, so the digest and the typed-data encoding are cross-checked.

Signer: Foundry's public test mnemonic, account index 0 (test-only EOA; never
fund it on any network). No key material is written to the repository.

Usage:
    python3 docs/content/specifications/protocol/build_eip712_vectors.py
    python3 docs/content/specifications/protocol/build_eip712_vectors.py --check   # regenerate and diff
Requires: Foundry `cast` on PATH.
"""

from __future__ import annotations

import json
import subprocess
import sys
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
SCHEMA = HERE / "payment-protocol.schema.json"
OUT = HERE / "eip712-vectors.json"
MNEMONIC = "test test test test test test test test test test test junk"


def addr(tag: str) -> str:
    """Deterministic placeholder address made of one repeated byte, e.g. addr('c0')."""
    return "0x" + tag * 20


def b32(tag: str) -> str:
    return "0x" + tag * 32


def cast(*args: str) -> str:
    proc = subprocess.run(["cast", *args], capture_output=True, text=True)
    if proc.returncode != 0:
        raise SystemExit(f"cast {args[0]} {args[1] if len(args) > 1 else ''} failed: {proc.stderr.strip()}")
    return proc.stdout.strip()


def keccak(hex_data: str) -> str:
    return cast("keccak", hex_data)


def encode_type(name: str, types: dict) -> str:
    """EIP-712 encodeType for the flat structs used here (no nested struct members)."""
    return f"{name}(" + ",".join(f"{f['type']} {f['name']}" for f in types[name]) + ")"


def encode_value(ftype: str, value) -> str:
    if ftype == "string":
        return keccak("0x" + str(value).encode("utf-8").hex())[2:]
    if ftype == "address":
        return value[2:].lower().rjust(64, "0")
    if ftype == "bytes32":
        return value[2:].lower()
    if ftype.startswith("uint"):
        return format(int(value), "064x")
    raise ValueError(ftype)


def hash_struct(name: str, types: dict, message: dict) -> str:
    type_hash = keccak("0x" + encode_type(name, types).encode().hex())[2:]
    body = "".join(encode_value(f["type"], message[f["name"]]) for f in types[name])
    return keccak("0x" + type_hash + body)


def digest(domain_fields: list, domain: dict, name: str, types: dict, message: dict) -> str:
    all_types = {"EIP712Domain": [{"name": f["name"], "type": f["type"]} for f in domain_fields], **types}
    domain_hash = hash_struct("EIP712Domain", all_types, domain)[2:]
    msg_hash = hash_struct(name, all_types, message)[2:]
    return keccak("0x1901" + domain_hash + msg_hash)


def sign_typed(domain_fields: list, domain: dict, name: str, types: dict, message: dict) -> str:
    typed = {
        "types": {"EIP712Domain": [{"name": f["name"], "type": f["type"]} for f in domain_fields], name: types[name]},
        "primaryType": name,
        "domain": domain,
        "message": {k: (str(v) if isinstance(v, int) else v) for k, v in message.items()},
    }
    with tempfile.NamedTemporaryFile("w", suffix=".json", delete=False) as fh:
        json.dump(typed, fh)
        path = fh.name
    return cast("wallet", "sign", "--mnemonic", MNEMONIC, "--mnemonic-index", "0", "--data", "--from-file", path)


def build() -> dict:
    schema = json.loads(SCHEMA.read_text())
    domain_fields = schema["eip712Domain"]
    types = {**schema["eip712Types"], **schema["operatorSignedTypes"]}
    signer = cast("wallet", "address", "--mnemonic", MNEMONIC, "--mnemonic-index", "0")
    contract = addr("c0")
    domain = {"name": "NU54 Payment Settlement", "version": "1", "chainId": 8283, "verifyingContract": contract}
    cases = [
        ("PA-01", "PaymentAuthorization", "device", {
            "chainId": 8283, "contract": contract, "merchant": addr("a1"), "payout": addr("b1"), "token": addr("d1"),
            "amount": 4500000, "orderId": b32("01"), "nonce": 1, "expiry": 1790000120}),
        ("PA-02", "PaymentAuthorization", "device", {
            "chainId": 8283, "contract": contract, "merchant": addr("a1"), "payout": addr("b1"), "token": addr("d1"),
            "amount": 50000000, "orderId": b32("02"), "nonce": 2**255 + 7, "expiry": 1790000300}),
        ("LC-01", "LimitChange", "device", {
            "chainId": 8283, "contract": contract, "perPaymentLimit": 20000000, "dailyLimit": 100000000,
            "nonce": 3, "expiry": 1790000120}),
        ("MA-01", "MerchantAttestation", "operator", {
            "merchant": addr("a1"), "payout": addr("b1"), "name": "Cafe Test 01",
            "validFrom": 1790000000, "validUntil": 1790086400}),
        ("MO-01", "MerchantOrder", "merchant", {
            "orderId": b32("01"), "token": addr("d1"), "amount": 4500000, "payout": addr("b1"), "expiry": 1790000120}),
        ("TA-01", "TimeAnchor", "operator", {"device": signer, "timestamp": 1790000000}),
    ]
    vectors = []
    for vid, name, role, message in cases:
        d = digest(domain_fields, domain, name, types, message)
        sig = sign_typed(domain_fields, domain, name, types, message)
        recovered = cast("wallet", "verify", "--address", signer, "--no-hash", d, sig)
        if "succeeded" not in recovered.lower() and "valid" not in recovered.lower():
            raise SystemExit(f"{vid}: cast signature does not verify against the computed digest: {recovered}")
        vectors.append({
            "id": vid,
            "primaryType": name,
            "signerRole": role,
            "encodeType": encode_type(name, types),
            "message": {k: (str(v) if isinstance(v, int) else v) for k, v in message.items()},
            "digest": d,
            "signer": signer,
            "signature": sig,
        })
    return {
        "description": "Deterministic EIP-712 vectors for DF-20260925-02 [N04][N21]. Test-only EOA from Foundry's public test mnemonic, index 0. Addresses are repeated-byte placeholders. Never fund these accounts.",
        "schema": "payment-protocol.schema.json",
        "domain": domain,
        "vectors": vectors,
    }


def main() -> int:
    text = json.dumps(build(), indent=2) + "\n"
    if "--check" in sys.argv:
        same = OUT.exists() and OUT.read_text() == text
        print("OK vectors reproducible" if same else "FAIL vectors differ from a fresh build")
        return 0 if same else 1
    OUT.write_text(text)
    print(OUT.name)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
