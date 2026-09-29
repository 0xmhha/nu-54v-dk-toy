#!/usr/bin/env python3
"""Generate the shared CBOR message vectors from the protocol schema.

Every implementation (firmware C, kiosk and phone app TypeScript, operator tool Go)
must encode these messages to exactly these bytes, following payment-protocol.md 4.2:
  - maps: text keys, RFC 8949 core deterministic order (sorted by encoded key bytes)
  - hex20 / hex32 / signature / sessionId: fixed-length byte strings (20 / 32 / 65 / 8)
  - uint: unsigned integer below 2^64, otherwise tag 2 bignum without leading zeros
  - shortest-form integers and lengths; no floats, indefinite lengths or other tags

Values are taken from eip712-vectors.json so the signatures in the vectors are real.

Usage:
  python3 cbor-gen/generate.py           write docs/content/specifications/protocol/cbor-vectors.json
  python3 cbor-gen/generate.py --check   fail if the committed file differs; also decodes every
                                         vector back and compares it with its JSON form
"""

from __future__ import annotations

import hashlib
import json
import struct
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parents[3]
PROTO = REPO / "docs/content/specifications/protocol"
SCHEMA = json.loads((PROTO / "payment-protocol.schema.json").read_text())
EIP712 = json.loads((PROTO / "eip712-vectors.json").read_text())
OUT = PROTO / "cbor-vectors.json"

BYTE_LENGTHS = {"hex20": 20, "hex32": 32, "signature": 65, "sessionId": 8}


# ---------------------------------------------------------------- encoding


def _head(major: int, n: int) -> bytes:
    """Shortest-form CBOR head for major type and argument n."""
    if n < 24:
        return bytes([major << 5 | n])
    for info, fmt, limit in ((24, ">B", 0xFF), (25, ">H", 0xFFFF), (26, ">I", 0xFFFFFFFF), (27, ">Q", 2**64 - 1)):
        if n <= limit:
            return bytes([major << 5 | info]) + struct.pack(fmt, n)
    raise ValueError("argument too large for a CBOR head")


def _uint(n: int) -> bytes:
    if n < 0:
        raise ValueError("negative uint")
    if n < 2**64:
        return _head(0, n)
    raw = n.to_bytes((n.bit_length() + 7) // 8, "big")  # no leading zero bytes
    return _head(6, 2) + _head(2, len(raw)) + raw


def _text(s: str) -> bytes:
    b = s.encode("utf-8")
    return _head(3, len(b)) + b


def _resolve(node: dict) -> tuple[str | None, dict]:
    ref = node.get("$ref")
    if ref:
        name = ref.rsplit("/", 1)[-1]
        return name, SCHEMA["$defs"][name]
    return None, node


def encode(node: dict, value) -> bytes:
    """Encode a JSON-form value according to its schema node."""
    name, node = _resolve(node)
    if name in BYTE_LENGTHS:
        raw = bytes.fromhex(value[2:] if value.startswith("0x") else value)
        if len(raw) != BYTE_LENGTHS[name]:
            raise ValueError(f"{name} must be {BYTE_LENGTHS[name]} bytes, got {len(raw)}")
        return _head(2, len(raw)) + raw
    if name == "uint":
        return _uint(int(value))
    if node.get("type") == "object":
        props = node["properties"]
        missing = set(node.get("required", [])) - set(value)
        unknown = set(value) - set(props)
        if missing or unknown:
            raise ValueError(f"object keys: missing {sorted(missing)}, unknown {sorted(unknown)}")
        items = sorted((_text(k), encode(props[k], v)) for k, v in value.items())
        return _head(5, len(items)) + b"".join(k + v for k, v in items)
    if node.get("type") == "boolean" or isinstance(value, bool):
        return b"\xf5" if value else b"\xf4"
    if isinstance(value, int):
        return _uint(value)
    if isinstance(value, str):
        return _text(value)
    raise ValueError(f"cannot encode {value!r}")


def encode_message(message: dict) -> bytes:
    return encode(SCHEMA["messages"][message["type"]], message)


def envelope(body: bytes) -> bytes:
    """length u16 BE (whole envelope, header included) | sha256(body)[:8] | body."""
    return struct.pack(">H", 10 + len(body)) + hashlib.sha256(body).digest()[:8] + body


# ---------------------------------------------------------------- decoding (for the self-check)


def _decode(buf: bytes, i: int):
    ib = buf[i]
    major, info = ib >> 5, ib & 0x1F
    i += 1
    if info < 24:
        n = info
    else:
        size = {24: 1, 25: 2, 26: 4, 27: 8}[info]
        n = int.from_bytes(buf[i : i + size], "big")
        i += size
    if major == 0:
        return n, i
    if major == 2:
        return buf[i : i + n], i + n
    if major == 3:
        return buf[i : i + n].decode("utf-8"), i + n
    if major == 5:
        out = {}
        for _ in range(n):
            k, i = _decode(buf, i)
            v, i = _decode(buf, i)
            out[k] = v
        return out, i
    if major == 6 and n == 2:
        raw, i = _decode(buf, i)
        return int.from_bytes(raw, "big"), i
    if major == 7 and info in (20, 21):
        return info == 21, i
    raise ValueError(f"unsupported CBOR item 0x{ib:02x}")


def to_json_form(node: dict, value):
    """Map a decoded CBOR value back to the schema's JSON form."""
    name, node = _resolve(node)
    if name in BYTE_LENGTHS:
        return value.hex() if name == "sessionId" else "0x" + value.hex()
    if name == "uint":
        return str(value)
    if node.get("type") == "object":
        return {k: to_json_form(node["properties"][k], v) for k, v in value.items()}
    return value


def _normal(message: dict):
    """Lower-case hex so the round trip compares values, not letter case."""
    def norm(v):
        if isinstance(v, dict):
            return {k: norm(x) for k, x in v.items()}
        return v.lower() if isinstance(v, str) and v.startswith("0x") else v
    return norm(message)


# ---------------------------------------------------------------- vectors


def _eip712(vid: str) -> dict:
    return next(v for v in EIP712["vectors"] if v["id"] == vid)


def vectors() -> list[dict]:
    pa, ma, mo = _eip712("PA-01"), _eip712("MA-01"), _eip712("MO-01")
    session = "0102030405060708"
    auth = {k: v for k, v in pa["message"].items() if k != "nonce"}
    big_nonce = str(2**255 + 12345)  # device nonces are 256-bit, so this exercises the bignum path
    cases = [
        ("CB-01", "kiosk opens a payment session", {
            "v": 1, "type": "session.open", "sessionId": session, "mode": "payment",
            "kioskNonce": "0x" + "a5" * 32}),
        ("CB-02", "device answers with its state", {
            "v": 1, "type": "session.open.ok", "sessionId": session, "device": pa["signer"],
            "deviceNonce": "0x" + "5a" * 32, "anchorValid": True, "firmware": "0.1.0", "state": "READY",
            "lastAnchor": "1790000000"}),
        ("CB-03", "kiosk sends the operator-signed merchant attestation", {
            "v": 1, "type": "payment.identify", "sessionId": session,
            "attestation": {**ma["message"], "operatorSignature": ma["signature"]}}),
        ("CB-04", "kiosk sends the order signed by the merchant key", {
            "v": 1, "type": "payment.prepare", "sessionId": session, "authorization": auth,
            "merchantSignature": mo["signature"]}),
        ("CB-05", "device shows the fields on the phone app", {
            "v": 1, "type": "confirm.show", "sessionId": session, "merchantName": ma["message"]["name"],
            "orderId": auth["orderId"], "token": auth["token"], "payout": auth["payout"], "amount": auth["amount"]}),
        ("CB-06", "device approves with a small nonce", {
            "v": 1, "type": "payment.result", "sessionId": session, "outcome": "approved",
            "signature": pa["signature"], "nonce": pa["message"]["nonce"]}),
        ("CB-07", "device approves with a 256-bit nonce (tag 2 bignum)", {
            "v": 1, "type": "payment.result", "sessionId": session, "outcome": "approved",
            "signature": pa["signature"], "nonce": big_nonce}),
        ("CB-08", "receiver rejects a malformed frame", {
            "v": 1, "type": "error", "sessionId": session, "reason": "BAD_FRAME"}),
    ]
    out = []
    for vid, note, msg in cases:
        body = encode_message(msg)
        out.append({"id": vid, "description": note, "message": msg, "cborHex": body.hex(),
                    "envelopeHex": envelope(body).hex()})
    return out


def render() -> str:
    doc = {
        "description": "Deterministic CBOR vectors for payment-protocol.md 4.2. Signatures and addresses come "
                       "from eip712-vectors.json (public test keys); they are not funded anywhere.",
        "generator": "packages/protocol/cbor-gen/generate.py",
        "vectors": vectors(),
    }
    return json.dumps(doc, indent=2) + "\n"


def self_check(doc: dict) -> list[str]:
    problems = []
    for v in doc["vectors"]:
        body = bytes.fromhex(v["cborHex"])
        decoded, end = _decode(body, 0)
        if end != len(body):
            problems.append(f"{v['id']}: trailing bytes")
        back = to_json_form(SCHEMA["messages"][v["message"]["type"]], decoded)
        if _normal(back) != _normal(v["message"]):
            problems.append(f"{v['id']}: decode does not round-trip")
        if encode_message(back) != body:
            problems.append(f"{v['id']}: re-encoding is not byte-identical")
    return problems


def main(argv: list[str]) -> int:
    text = render()
    if "--check" in argv:
        if not OUT.exists() or OUT.read_text() != text:
            print(f"{OUT.relative_to(REPO)} is stale; run python3 cbor-gen/generate.py")
            return 1
        problems = self_check(json.loads(text))
        if problems:
            print("\n".join(problems))
            return 1
        print("cbor vectors: up to date,", len(json.loads(text)["vectors"]), "vectors round-trip")
        return 0
    OUT.write_text(text)
    print("wrote", OUT.relative_to(REPO))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
