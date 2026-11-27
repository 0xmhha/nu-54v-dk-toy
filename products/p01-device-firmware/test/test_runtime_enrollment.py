"""Generate disposable local offers and run the real issuer and P01 policy reader."""

import datetime
import hashlib
import json
from pathlib import Path
import secrets
import subprocess
import sys
import tempfile
import time

from cryptography import x509
from cryptography.exceptions import InvalidSignature
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec

SCRIPTS = Path(__file__).resolve().parents[1] / "scripts"
sys.path.insert(0, str(SCRIPTS))
from enroll_board import authorize_station, bound_offer, point, signed, spki, station_identity, ticket_certificate


def key():
    return ec.generate_private_key(ec.SECP256R1())


def cert(subject, issuer, public_key, authority, root=False, lifetime=datetime.timedelta(hours=2)):
    now = datetime.datetime.now(datetime.timezone.utc)
    builder = (x509.CertificateBuilder().subject_name(subject).issuer_name(issuer).public_key(public_key)
               .serial_number(x509.random_serial_number()).not_valid_before(now - datetime.timedelta(minutes=1))
               .not_valid_after(now + lifetime)
               .add_extension(x509.BasicConstraints(ca=root, path_length=None), critical=True))
    if not root:
        builder = builder.add_extension(x509.SubjectAlternativeName([
            x509.UniformResourceIdentifier("urn:nu54:role:board")]), critical=False)
    return builder.sign(authority, hashes.SHA256()).public_bytes(serialization.Encoding.PEM)


def rejects(action, error=ValueError):
    try:
        action()
    except error:
        return
    raise AssertionError("Invalid commissioning input was accepted")


def station_ticket_tests(paths, authority):
    station_key = paths / "station.key"
    station_pem = paths / "station.crt"
    station_identity(station_key, station_pem)
    assert station_key.stat().st_mode & 0o777 == 0o600
    station = x509.load_pem_x509_certificate(station_pem.read_bytes())
    current = b"\x01" + secrets.token_bytes(32)
    ticket = authorize_station(current, station, authority)
    assert ticket_certificate(ticket, current) == station
    rejects(lambda: ticket_certificate(ticket, b"\x01" + secrets.token_bytes(32)))
    now = datetime.datetime.now(datetime.timezone.utc)
    signer = key()
    expired = (x509.CertificateBuilder().subject_name(station.subject).issuer_name(station.subject)
               .public_key(signer.public_key()).serial_number(x509.random_serial_number())
               .not_valid_before(now - datetime.timedelta(minutes=3)).not_valid_after(now - datetime.timedelta(minutes=1))
               .add_extension(x509.SubjectAlternativeName([x509.UniformResourceIdentifier("urn:nu54:role:installer")]), False)
               .sign(signer, hashes.SHA256()))
    rejects(lambda: authorize_station(current, expired, authority))


def main(runner):
    authority = key()
    ca = key()
    issuer = x509.Name([x509.NameAttribute(x509.NameOID.COMMON_NAME, "P01 local test CA")])
    with tempfile.TemporaryDirectory(prefix="nu54-commission-") as directory:
        paths = Path(directory)
        auth_public = paths / "authority.pub"
        auth_private = paths / "authority.pem"
        ca_cert = paths / "ca.pem"
        auth_public.write_bytes(authority.public_key().public_bytes(serialization.Encoding.PEM, serialization.PublicFormat.SubjectPublicKeyInfo))
        auth_private.touch(mode=0o600)
        auth_private.write_bytes(authority.private_bytes(serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8, serialization.NoEncryption()))
        station_ticket_tests(paths, auth_private)
        ca_cert.write_bytes(cert(issuer, issuer, ca.public_key(), ca, root=True))
        keys = {}
        for role in ("display", "recovery"):
            keys[role] = paths / f"{role}.pub"
            keys[role].write_bytes(key().public_key().public_bytes(serialization.Encoding.PEM, serialization.PublicFormat.SubjectPublicKeyInfo))
        policy = paths / "policy.json"
        policy.write_text(json.dumps({
            "revision": 1, "payer_generation": 1, "not_after": int(time.time()) + 1800, "max_amount": 100,
            "payer_role": 1, "merchant_key_version": 1, "beneficiary_version": 1,
            "contract": "11" * 20, "token": "22" * 20, "beneficiary": "33" * 20, "merchant_signer": "44" * 20,
            "merchant": "55" * 32, "owner": "66" * 32, "terminal": "77" * 32,
        }))
        boards = []
        for index in range(2):
            board = key()
            board_private = paths / f"board{index}.key"
            board_private.touch(mode=0o600)
            board_private.write_bytes(board.private_bytes(serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8, serialization.NoEncryption()))
            leaf = paths / f"board{index}.pem"
            leaf.write_bytes(cert(x509.Name([x509.NameAttribute(x509.NameOID.COMMON_NAME, "P01 test board")]),
                                  issuer, board.public_key(), ca))
            boards.append((board_private, point(board.public_key()), hashlib.sha256(spki(board.public_key())).hexdigest(),
                           secrets.token_bytes(20), secrets.token_bytes(32), leaf))
        args = []
        for turn, board in enumerate((boards[0], boards[1], boards[0])):
            private, public, pin, payer, device, leaf = board
            offer = paths / f"offer{turn}.bin"
            grant = paths / f"grant{turn}.bin"
            body = b"N54E" + secrets.token_bytes(32) + public + payer + device
            offer_data = body + signed(private, body)
            exporter = secrets.token_bytes(32)
            response = offer_data + signed(private, b"NU54IB01" + exporter + hashlib.sha256(offer_data).digest())
            assert bound_offer(response, exporter, pin, device.hex()) == offer_data
            rejects(lambda: bound_offer(response, secrets.token_bytes(32), pin, device.hex()), InvalidSignature)
            rejects(lambda: bound_offer(response, exporter, "01" * 32, device.hex()))
            offer.write_bytes(offer_data)
            subprocess.run([sys.executable, str(SCRIPTS / "enroll_board.py"), "issue", "--offer", str(offer),
                            "--policy", str(policy), "--ca", str(ca_cert), "--board-certificate", str(leaf),
                            "--display-public", str(keys["display"]), "--recovery-public", str(keys["recovery"]),
                            "--board-pin", pin, "--device", device.hex(), "--authority", str(auth_private),
                            "--out", str(grant)], check=True)
            args += [str(offer), str(grant)]
        subprocess.run([runner, "--grants", str(auth_public), *args], check=True)


if __name__ == "__main__":
    main(sys.argv[1])
