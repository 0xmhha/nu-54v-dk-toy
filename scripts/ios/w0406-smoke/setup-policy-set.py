"""Seed and verify signed W27 state for the isolated iOS simulator."""
import base64
import json
import os
from pathlib import Path
import subprocess
import sys
import time

from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec, utils

ASSET = "500ef69da230e42bff34487b578ea0cbefbda2ba"
GENESIS = "0x" + "aa" * 32
ADDRESS = {n: bytes([n] * 20).hex() for n in range(1, 10)}
NOW = int(time.time())
HOLD = 86_400
EPOCH = "bb" * 16
NONCE = "cc" * 32
RESERVATION_ID = "a9" * 16
RESERVATION = 5_000_000
RECIPIENTS = [1, 2, 3, 4, 5, 7, 8, 9]
CAPS = list(range(1, 9))


def uint(value):
    return value.to_bytes(8, "big")


def bridge(program, board, wire, clock, expiry, request, display=False):
    if display:
        Path("policy-private-input").write_text(f"{wire[0]}/{EPOCH}/{request}/{expiry}")
    return bytes.fromhex(subprocess.check_output(
        [program, "policy-state", wire.hex(), str(expiry), request, str(clock), board, EPOCH, NONCE],
        env={**os.environ, "NU54_W27_PANEL_CLOCK": str(clock)}, text=True,
    ).strip())


def stage_record(board, revision, active, candidate, per=10_000_000, daily=30_000_000):
    record = dict(board=board, asset=ASSET, generation=1, revision=revision, chain=8283,
                  previous_per_transfer=10_000_000, previous_daily=30_000_000, per_transfer=per, daily=daily,
                  previous_recipients=[ADDRESS[n] for n in active], recipients=[ADDRESS[n] for n in candidate])
    Path("policy-set-expect.json").write_text(json.dumps(record))


def seed(program, board, scenario):
    active = [5, 6] if scenario == "recipients" else CAPS
    stage_record(board, 1, [5], active)
    initial_clock = NOW - 2 * HOLD
    expiry = initial_clock + 90
    due = expiry + HOLD
    request = "ae"*16
    stage = bytes([33, 2]) + uint(1) + bytes([len(active)]) + bytes.fromhex("".join(ADDRESS[n] for n in active)) + bytes(20 * (8-len(active)))
    if bridge(program, board, stage, initial_clock, expiry, request, True) != bytes([33, 2, 1])+uint(due):
        raise ValueError("The baseline recipient proposal was not approved")
    activate = bytes([35, 2]) + uint(1) + bytes.fromhex(request)
    if bridge(program, board, activate, due, due + 90, "af" * 16) != bytes([35, 2, 1]):
        raise ValueError("The baseline policy did not activate")
    if scenario == "caps":
        reservation_expiry = NOW + 3 * HOLD
        if bridge(program, board, bytes([100])+uint(RESERVATION), NOW, reservation_expiry, RESERVATION_ID) != bytes([100, 1]):
            raise ValueError("Could not reserve funds in the baseline")
        Path("policy-reservation-expiry").write_text(str(reservation_expiry))
        stage_record(board, 2, active, active, 8_000_000, 30_000_000)
    else:
        stage_record(board, 2, active, RECIPIENTS)


def verify(program, board, scenario):
    clock = int(Path("policy-not-before").read_text())
    report = bridge(program, board, bytes([32, 2]), clock, clock + 90, "ad" * 16)
    target = RECIPIENTS if scenario == "recipients" else CAPS
    expected = dict(per=10_000_000, spent=0) if scenario == "recipients" else dict(per=8_000_000, spent=RESERVATION)
    numeric = lambda start: int.from_bytes(report[start:start+8], "big")
    assert len(report) == 466 and report[:3] == bytes([32, 2, 1]) and report[264] == 0
    assert numeric(43) == 3 and numeric(79) == expected["per"] and numeric(87) == 30_000_000
    assert numeric(95) == expected["spent"] and report[103] == 8
    assert report[104:264].hex() == "".join(ADDRESS[n] for n in target)
    if scenario == "caps":
        expires = int(Path("policy-reservation-expiry").read_text())
        replay = bridge(program, board, bytes([100])+uint(RESERVATION), clock, expires, RESERVATION_ID)
        assert replay == bytes([100, 2]), "Original reservation did not survive exactly"
    print(f"Verified {scenario}: revision 3, exact 8 recipients, caps, spend and applicable original pending reservation")


def profile(board, sender):
    """Generate an issuer-signed registry for locally named address-book entries."""
    key = ec.generate_private_key(ec.SECP256R1())
    payload = json.dumps(dict(device=board, sender=sender, generation=1, leaseRevision="1", expires=NOW+3600,
                              networks=[dict(chain="8283", genesis=GENESIS, rpc="https://rpc.invalid",
                                             assets=[dict(address="0x"+ASSET, symbol="TestUSDC", decimals=6)],
                                             feeLimit="600000")]), separators=(",", ":")).encode()
    r, s = utils.decode_dss_signature(key.sign(b"N54G"+payload, ec.ECDSA(hashes.SHA256())))
    registry = dict(endpoint="https://issuer.invalid", token="11"*32, payload=base64.b64encode(payload).decode(),
                    proof=(r.to_bytes(32,"big")+s.to_bytes(32,"big")).hex(),
                    anchor=key.public_key().public_bytes(serialization.Encoding.X962, serialization.PublicFormat.UncompressedPoint).hex())
    contacts = [dict(name=f"QA-{n:02}", address="0x"+ADDRESS[n], genesis=GENESIS) for n in ADDRESS]
    return dict(evmRegistry=registry, evmRecipients=contacts)


if __name__ == "__main__":
    command, program, board, scenario = sys.argv[1:]
    {"seed": seed, "verify": verify}[command](program, board, scenario)
