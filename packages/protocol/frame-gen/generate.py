#!/usr/bin/env python3
"""Generate the shared BLE fragment vectors from the CBOR message vectors.

payment-protocol.md 4 defines two layers above GATT:
  envelope = length(u16 BE, whole envelope incl. the 10-byte header) | digest(8) | CBOR body
  fragment = sequence(u8) | index(u8) | up to ATT_MTU - 5 envelope bytes

The CBOR vectors already carry each message's envelope. This file adds the fragment layer:
  - valid: every CBOR vector split for ATT_MTU 23, 185 and 247, with the sender's sequence
  - invalid: fragment streams a receiver must refuse with error{BAD_FRAME}

Receiver rules the vectors encode (the firmware reassembler follows them):
  - a message starts with index 0; within a message the sequence stays the same and the
    index grows by one; the sequence is not compared between messages
  - every fragment carries at least one data byte
  - the length field is 10..2058; bytes beyond it are refused
  - the digest (first 8 bytes of SHA-256 of the body) is checked once the envelope is complete

Usage:
  python3 frame-gen/generate.py           write docs/content/specifications/protocol/frame-vectors.json
  python3 frame-gen/generate.py --check   fail if the committed file differs; also reassembles
                                          every vector with the reference receiver below
"""

from __future__ import annotations

import hashlib
import json
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parents[3]
PROTO = REPO / "docs/content/specifications/protocol"
CBOR = json.loads((PROTO / "cbor-vectors.json").read_text())
OUT = PROTO / "frame-vectors.json"

MTUS = [23, 185, 247]
HEADER = 10
MAX_BODY = 2048


def envelope(body: bytes) -> bytes:
    if len(body) > MAX_BODY:
        raise ValueError("body too large")
    return (HEADER + len(body)).to_bytes(2, "big") + hashlib.sha256(body).digest()[:8] + body


def fragments(env: bytes, sequence: int, mtu: int) -> list[bytes]:
    size = mtu - 5
    chunks = [env[i : i + size] for i in range(0, len(env), size)]
    if len(chunks) > 256:
        raise ValueError("more than 256 fragments")
    return [bytes([sequence, i]) + c for i, c in enumerate(chunks)]


def reassemble(frags: list[bytes]) -> tuple[str, bytes | None]:
    """Reference receiver for one message: ('ok', body) or ('BAD_FRAME', None)."""
    buf, seq, want = b"", None, 0
    for i, f in enumerate(frags):
        if len(f) <= 2:
            return "BAD_FRAME", None
        if i == 0:
            if f[1] != 0:
                return "BAD_FRAME", None
            seq = f[0]
        elif f[0] != seq or f[1] != i:
            return "BAD_FRAME", None
        buf += f[2:]
        if not want and len(buf) >= 2:
            want = int.from_bytes(buf[:2], "big")
            if want < HEADER or want > HEADER + MAX_BODY:
                return "BAD_FRAME", None
        if want and len(buf) > want:
            return "BAD_FRAME", None
        if want and len(buf) == want:
            if i != len(frags) - 1:
                return "BAD_FRAME", None  # a fragment after a complete message starts no new one here
            body = buf[HEADER:]
            if hashlib.sha256(body).digest()[:8] != buf[2:HEADER]:
                return "BAD_FRAME", None
            return "ok", body
    return "incomplete", None


def build() -> dict:
    valid = []
    for n, v in enumerate(CBOR["vectors"]):
        env = bytes.fromhex(v["envelopeHex"])
        for mtu in MTUS:
            seq = n  # the sender's message number in this direction
            frags = fragments(env, seq, mtu)
            valid.append({
                "id": f"FR-{v['id']}-{mtu}",
                "message": v["id"],
                "attMtu": mtu,
                "sequence": seq,
                "fragmentsHex": [f.hex() for f in frags],
            })

    # Invalid streams are built from CB-04 (the largest message) at ATT_MTU 23.
    base_env = bytes.fromhex(next(v for v in CBOR["vectors"] if v["id"] == "CB-04")["envelopeHex"])
    good = fragments(base_env, 7, 23)

    def hexes(fs: list[bytes]) -> list[str]:
        return [f.hex() for f in fs]

    flipped_digest = bytearray(base_env)
    flipped_digest[2] ^= 0x01
    short_len = (9).to_bytes(2, "big") + base_env[2:]
    huge_len = (HEADER + MAX_BODY + 1).to_bytes(2, "big") + base_env[2:]
    trailing = good[:-1] + [good[-1] + b"\x00"]
    invalid = [
        ("BF-01", "first fragment index is 1", [bytes([7, 1]) + good[0][2:]] + good[1:]),
        ("BF-02", "index skips from 0 to 2", [good[0]] + good[2:]),
        ("BF-03", "sequence changes in the middle of a message", [good[0], bytes([8]) + good[1][1:]] + good[2:]),
        ("BF-04", "fragment with a header and no data", [good[0], bytes([7, 1])]),
        ("BF-05", "length field below the 10-byte header", fragments(short_len, 7, 23)),
        ("BF-06", "length field above 2058", fragments(huge_len, 7, 23)[:1]),
        ("BF-07", "bytes beyond the length field", trailing),
        ("BF-08", "digest does not match the body", fragments(bytes(flipped_digest), 7, 23)),
        ("BF-09", "a new message (index 0) starts before the current one is complete", [good[0], bytes([8, 0]) + good[1][2:]] + good[2:]),
    ]
    cases = []
    for cid, desc, fs in invalid:
        result, _ = reassemble(fs)
        if result != "BAD_FRAME":
            raise AssertionError(f"{cid} is not refused by the reference receiver: {result}")
        cases.append({"id": cid, "description": desc, "fragmentsHex": hexes(fs), "expect": "BAD_FRAME"})

    return {
        "description": "Shared BLE fragment vectors (payment-protocol.md 4). Valid entries split each CBOR vector's envelope for ATT_MTU 23, 185 and 247; a receiver must rebuild exactly that message's CBOR body. Invalid entries are fragment streams of one message that a receiver must refuse with error{BAD_FRAME}. The sequence is the sender's per-direction message number; receivers do not compare it between messages.",
        "generator": "packages/protocol/frame-gen/generate.py",
        "payloadPerFragment": "ATT_MTU - 5",
        "valid": valid,
        "invalid": cases,
    }


def check_roundtrip(doc: dict) -> list[str]:
    errors = []
    bodies = {v["id"]: bytes.fromhex(v["cborHex"]) for v in CBOR["vectors"]}
    for v in doc["valid"]:
        result, body = reassemble([bytes.fromhex(h) for h in v["fragmentsHex"]])
        if result != "ok" or body != bodies[v["message"]]:
            errors.append(f"{v['id']}: {result}")
        if any(len(bytes.fromhex(h)) > v["attMtu"] - 3 for h in v["fragmentsHex"]):
            errors.append(f"{v['id']}: a fragment exceeds ATT_MTU - 3")
    for v in doc["invalid"]:
        result, _ = reassemble([bytes.fromhex(h) for h in v["fragmentsHex"]])
        if result != "BAD_FRAME":
            errors.append(f"{v['id']}: {result}")
    return errors


def main(argv: list[str]) -> int:
    doc = build()
    text = json.dumps(doc, indent=2) + "\n"
    errors = check_roundtrip(doc)
    if errors:
        print("FAIL", *errors, sep="\n  ")
        return 1
    if "--check" in argv:
        if not OUT.exists() or OUT.read_text() != text:
            print(f"FAIL {OUT.relative_to(REPO)} is stale; run frame-gen/generate.py")
            return 1
        print(f"ok: {len(doc['valid'])} valid and {len(doc['invalid'])} invalid fragment vectors")
        return 0
    OUT.write_text(text)
    print(f"wrote {OUT.relative_to(REPO)}: {len(doc['valid'])} valid, {len(doc['invalid'])} invalid")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
