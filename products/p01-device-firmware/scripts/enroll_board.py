#!/usr/bin/env python3
"""Inspect a local P01 enrollment offer, issue a signed grant offline, or install it over BLE."""

import argparse
import asyncio
import base64
from binascii import crc_hqx
from datetime import datetime, timedelta, timezone
import hashlib
import json
import os
import re
import struct
import sys
import tempfile
import time
from pathlib import Path

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec, utils
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID

GATT = "41ba05a0-e0dd-4fd7-901c-4c25d501000"
MAX_BUNDLE = 1600
MAX_TICKET = 768
INSTALL_ALPN = b"nu54-install/1"
INSTALL_EXPORTER = b"EXPORTER-NU54-INSTALL-1"
INSTALL_PIN_GROUP = 0x5451
RESET_DOMAIN = b"NU54 operator reset v1".ljust(24, b"\0")
RESET_REQUEST = re.compile(r"NU54-RESET1:([0-9a-f]{64}):([1-9][0-9]{0,19}):([0-9a-f]{64})")


def write_private(path, data):
    with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb") as output:
        output.write(data)


def spki(key):
    return key.public_bytes(serialization.Encoding.DER, serialization.PublicFormat.SubjectPublicKeyInfo)


def point(key):
    if not isinstance(key, ec.EllipticCurvePublicKey) or not isinstance(key.curve, ec.SECP256R1):
        raise ValueError("A P-256 key is required")
    return key.public_bytes(serialization.Encoding.X962, serialization.PublicFormat.UncompressedPoint)


def public_file(path):
    return serialization.load_pem_public_key(path.read_bytes())


def private_key(key_path):
    if key_path.resolve().is_relative_to(Path(__file__).resolve().parents[3]):
        raise ValueError("Keep issuing and accessory private keys outside the checkout")
    key = serialization.load_pem_private_key(key_path.read_bytes(), None)
    point(key.public_key())
    return key


def signed(key_path, data):
    key = private_key(key_path)
    r, s = utils.decode_dss_signature(key.sign(data, ec.ECDSA(hashes.SHA256())))
    return r.to_bytes(32, "big") + s.to_bytes(32, "big")


def parse_offer(raw):
    if len(raw) != 217 or raw[:4] != b"N54E":
        raise ValueError("Invalid enrollment offer")
    key = ec.EllipticCurvePublicKey.from_encoded_point(ec.SECP256R1(), raw[36:101])
    r, s = (int.from_bytes(raw[i:i + 32], "big") for i in (153, 185))
    key.verify(utils.encode_dss_signature(r, s), raw[:153], ec.ECDSA(hashes.SHA256()))
    return key


def checked_offer(raw, board_pin, device):
    key = parse_offer(raw)
    if hashlib.sha256(spki(key)).digest() != exact_hex(board_pin, 32) or raw[121:153] != exact_hex(device, 32):
        raise ValueError("Offer identity differs from independently verified board identity")
    return key


def bound_offer(raw, exporter, board_pin, device):
    if len(raw) != 281 or len(exporter) != 32:
        raise ValueError("Invalid TLS-bound offer")
    offer, proof = raw[:217], raw[217:]
    key = checked_offer(offer, board_pin, device)
    r, s = (int.from_bytes(proof[i:i + 32], "big") for i in (0, 32))
    binding = b"NU54IB01" + exporter + hashlib.sha256(offer).digest()
    key.verify(utils.encode_dss_signature(r, s), binding, ec.ECDSA(hashes.SHA256()))
    return offer


def public_challenge(raw):
    if len(raw) != 33 or raw[0] != 1 or not any(raw[1:]):
        raise ValueError("Invalid installation challenge")
    return raw


def station_identity(key_path, certificate_path):
    if key_path.resolve().is_relative_to(Path(__file__).resolve().parents[3]):
        raise ValueError("Keep the ephemeral station private key outside the checkout")
    key = ec.generate_private_key(ec.SECP256R1())
    name = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "NU54 commissioning station")])
    now = datetime.now(timezone.utc)
    cert = (x509.CertificateBuilder().subject_name(name).issuer_name(name).public_key(key.public_key())
            .serial_number(x509.random_serial_number()).not_valid_before(now - timedelta(seconds=5))
            .not_valid_after(now + timedelta(minutes=5))
            .add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True)
            .add_extension(x509.SubjectAlternativeName([x509.UniformResourceIdentifier("urn:nu54:role:installer")]), False)
            .add_extension(x509.ExtendedKeyUsage([ExtendedKeyUsageOID.SERVER_AUTH]), True)
            .sign(key, hashes.SHA256()))
    write_private(key_path, key.private_bytes(serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8,
                                              serialization.NoEncryption()))
    write_private(certificate_path, cert.public_bytes(serialization.Encoding.PEM))


def authorize_station(challenge, certificate, authority):
    public_challenge(challenge)
    if point(certificate.public_key()) == point(private_key(authority).public_key()):
        raise ValueError("The station key must differ from the enrollment authority")
    certificate.verify_directly_issued_by(certificate)
    roles = certificate.extensions.get_extension_for_class(x509.SubjectAlternativeName).value
    if list(roles) != [x509.UniformResourceIdentifier("urn:nu54:role:installer")]:
        raise ValueError("Station certificate must have only the installer role")
    now = datetime.now(timezone.utc)
    if (not certificate.not_valid_before_utc <= now < certificate.not_valid_after_utc or
            certificate.not_valid_after_utc - certificate.not_valid_before_utc > timedelta(minutes=6)):
        raise ValueError("A fresh short-lived station certificate is required")
    der = certificate.public_bytes(serialization.Encoding.DER)
    body = b"NU54IT01" + challenge + struct.pack(">H", len(der)) + der
    if len(body) + 64 > MAX_TICKET:
        raise ValueError("Station certificate exceeds the installation ticket limit")
    return body + signed(authority, body)


def ticket_certificate(ticket, challenge):
    body = ticket[:-64]
    prefix = b"NU54IT01" + public_challenge(challenge)
    if (not 43 + 64 < len(ticket) <= MAX_TICKET or not body.startswith(prefix) or
            int.from_bytes(body[41:43], "big") != len(body) - 43):
        raise ValueError("Installation ticket does not match the current physical window")
    cert = x509.load_der_x509_certificate(body[43:])
    now = datetime.now(timezone.utc)
    if not cert.not_valid_before_utc <= now < cert.not_valid_after_utc:
        raise ValueError("Station certificate expired or is not yet valid")
    return cert


def read_serial_pins(serial_port):
    import cbor2

    sequence = os.urandom(1)[0]
    request = struct.pack(">BBHHBB", 8, 0, 1, INSTALL_PIN_GROUP, sequence, 0) + b"\xa0"
    packet = struct.pack(">H", len(request) + 2) + request + struct.pack(">H", crc_hqx(request, 0))
    serial_port.write(b"\x06\x09" + base64.b64encode(packet) + b"\n")
    length = None
    response = bytearray()
    for _ in range(3):
        line = serial_port.read_until(b"\n", 128)
        if not line.endswith(b"\n") or line[:2] != (b"\x06\x09" if length is None else b"\x04\x14"):
            raise ValueError("No valid response from the wired installation port")
        fragment = base64.b64decode(line[2:-1], validate=True)
        if length is None:
            length = int.from_bytes(fragment[:2], "big")
            if not 10 < length <= 192:
                raise ValueError("Invalid wired installation response length")
            fragment = fragment[2:]
        response.extend(fragment)
        if len(response) >= length:
            break
    if len(response) != length or crc_hqx(response, 0):
        raise ValueError("Invalid wired installation response checksum")
    op, flags, size, group, reply_sequence, command = struct.unpack_from(">BBHHBB", response)
    if (op, flags, size, group, reply_sequence, command) != (9, 0, len(response) - 10, INSTALL_PIN_GROUP, sequence, 0):
        raise ValueError("Wired installation response does not match the read request")
    result = cbor2.loads(response[8:-2])
    pins = result.get("pins") if isinstance(result, dict) and set(result) == {"pins"} else None
    if not isinstance(pins, bytes) or len(pins) != 64 or not any(pins[:32]) or not any(pins[32:]):
        raise ValueError("The board did not return two installation fingerprints")
    return pins


def wired_pins(args):
    import serial

    with serial.Serial(args.port, 115200, timeout=2, write_timeout=2, exclusive=True) as port:
        port.reset_input_buffer()
        pins = read_serial_pins(port)
    record = {"boardSPKI": pins[:32].hex(), "device": pins[32:].hex()}
    write_private(args.out, (json.dumps(record, sort_keys=True) + "\n").encode())
    if args.qr:
        from qrcode import make
        from qrcode.image.svg import SvgPathImage

        uri = "nu54://commissioning/v1/" + record["device"] + "/" + record["boardSPKI"]
        write_private(args.qr, make(uri, image_factory=SvgPathImage).to_string())


def exact_hex(value, length):
    raw = bytes.fromhex(value.removeprefix("0x"))
    if len(raw) != length or not any(raw):
        raise ValueError(f"Expected a nonzero {length}-byte value")
    return raw


def publish_w10_commissioning(grant_path, grant, publication_path, publication):
    # Stage complete private files alongside their destinations. Linking uses
    # no-replace semantics and keeps the publication invisible until the board
    # grant is available.
    with tempfile.NamedTemporaryFile(dir=grant_path.parent, prefix=".w10-grant-") as grant_stage, \
            tempfile.NamedTemporaryFile(dir=publication_path.parent, prefix=".w10-publication-") as publication_stage:
        grant_stage.write(grant)
        grant_stage.flush()
        publication_stage.write(publication)
        publication_stage.flush()
        os.link(grant_stage.name, grant_path)
        try:
            os.link(publication_stage.name, publication_path)
        except OSError:
            try:
                installed = grant_path.stat(follow_symlinks=False)
            except FileNotFoundError:
                pass
            else:
                if os.path.samestat(installed, os.fstat(grant_stage.fileno())):
                    grant_path.unlink()
            raise


def issue(args):
    raw = args.offer.read_bytes()
    identity = checked_offer(raw, args.board_pin, args.device)

    now = int(time.time())
    policy = json.loads(args.policy.read_text())
    settings = ("revision", "payer_generation", "not_after", "merchant_key_version", "beneficiary_version", "max_amount")
    if any(type(policy[name]) is not int or policy[name] <= 0 for name in settings):
        raise ValueError("Policy counters and limits must be positive integers")
    cap = 30_000_000 if args.commercial_testnet else 1_000_000
    if getattr(args, "spending", False) and (not args.commercial_testnet or policy["payer_role"] != 1):
        raise ValueError("The spending profile needs --commercial-testnet and payer_role 1")
    if not 0 < policy["not_after"] - now <= 86400 or policy["max_amount"] > cap or policy["payer_role"] not in (1, 2):
        raise ValueError("Invalid policy expiry, payer role, or profile cap")
    attributes = struct.pack(">6QB", *(policy[name] for name in settings), policy["payer_role"])
    attributes += b"".join(exact_hex(policy[name], 20) for name in ("contract", "token", "beneficiary", "merchant_signer"))
    attributes += b"".join(exact_hex(policy[name], 32) for name in ("merchant", "owner", "terminal"))
    attributes += point(public_file(args.display_public)) + point(public_file(args.recovery_public))

    ca = x509.load_pem_x509_certificate(args.ca.read_bytes())
    board = x509.load_pem_x509_certificate(args.board_certificate.read_bytes())
    if spki(board.public_key()) != spki(identity):
        raise ValueError("Certificate must contain the board-generated public key")
    board.verify_directly_issued_by(ca)
    san = board.extensions.get_extension_for_class(x509.SubjectAlternativeName).value
    if list(san) != [x509.UniformResourceIdentifier("urn:nu54:role:board")]:
        raise ValueError("Board certificate role mismatch")
    for cert in (ca, board):
        if cert.not_valid_before_utc.timestamp() > now or cert.not_valid_after_utc.timestamp() < policy["not_after"]:
            raise ValueError("Certificate expires outside the lease interval")
    ca_der, board_der = (entry.public_bytes(serialization.Encoding.DER) for entry in (ca, board))
    body = (b"N54Q" if args.commercial_testnet else b"N54P") + raw[4:121] + struct.pack(">Q", now) + attributes
    body += struct.pack(">HH", len(ca_der), len(board_der)) + ca_der + board_der
    if len(body) + 64 > MAX_BUNDLE:
        raise ValueError("Enrollment certificate bundle exceeds board limit")
    grant = body + signed(args.authority, body)
    w10_manifest = getattr(args, "w10_manifest", None)
    w10_out = getattr(args, "w10_out", None)
    if bool(w10_manifest) != bool(w10_out) or (w10_out and (not args.commercial_testnet or policy["payer_role"] != 2)):
        raise ValueError("W10 commissioning requires a commercial renter grant, v2 manifest digest and separate output")
    if w10_out and w10_out.resolve() == args.out.resolve():
        raise ValueError("W10 commissioning output must differ from the board grant")
    if w10_out:
        deployment = b"N54V" + hashlib.sha256(grant).digest() + exact_hex(w10_manifest, 32)
        proof = deployment + signed(args.authority, deployment)
        publication = [{"grant": base64.b64encode(grant).decode("ascii"),
                        "deploymentProof": base64.b64encode(proof).decode("ascii")}]
        publish_w10_commissioning(args.out, grant, w10_out,
                                  (json.dumps(publication, separators=(",", ":")) + chr(10)).encode())
    else:
        args.out.write_bytes(grant)


def reset_request(text):
    """Board id, generation and one-time value of a phone's reset request. Nothing else may be in it."""
    found = RESET_REQUEST.fullmatch(text)
    if not found:
        raise ValueError("Expected NU54-RESET1:<board id>:<generation>:<nonce> in lowercase hexadecimal")
    generation = int(found[2])
    if generation >= 1 << 64:
        raise ValueError("The generation does not fit the board's counter")
    return exact_hex(found[1], 32), generation, exact_hex(found[3], 32)


def reset_statement(board, generation, nonce):
    """The bytes nu54_spend_operator_statement (core/src/nu54_spending.c) rebuilds on the board."""
    return RESET_DOMAIN + board + struct.pack(">Q", generation) + nonce


def operator_reset(args):
    board, generation, nonce = reset_request(args.request)
    if args.device and exact_hex(args.device, 32) != board:
        raise ValueError("The request is for another board than --device")
    if args.yes and not args.device:
        raise ValueError("--yes needs --device, since nobody compares the board id")
    if not args.yes:
        print(f"Board id:   {board.hex()}\nGeneration: {generation}\n"
              "The reset erases this board's wallet key, PIN, owner and records. Its remaining balance is lost.\n"
              "Compare the board id with the commissioning record, then type RESET to sign: ",
              end="", file=sys.stderr, flush=True)
        if sys.stdin.readline().rstrip("\r\n") != "RESET":
            raise ValueError("The operator did not confirm the reset")
    print(f"NU54-RESET1-OK:{nonce.hex()}:{signed(args.authority, reset_statement(board, generation, nonce)).hex()}")


def image_measurement(image):
    if len(image) < 32 or image[:4] != bytes.fromhex("3db8f396"):
        raise ValueError("Expected a signed MCUboot image")
    header_size, protected_tlv, image_size = struct.unpack_from("<HHI", image, 8)
    end = header_size + image_size + protected_tlv
    if header_size < 32 or not image_size or end > len(image):
        raise ValueError("Signed image length is invalid")
    return hashlib.sha256(image[:end]).digest()


def issue_hd(args):
    offer = args.offer.read_bytes()
    identity = parse_offer(offer)
    if hashlib.sha256(spki(identity)).hexdigest() != args.board_pin:
        raise ValueError("Offer and independently recorded TLS board pin differ")
    source = spki(x509.load_pem_x509_certificate(args.source.read_bytes()).public_key())
    target = spki(x509.load_pem_x509_certificate(args.target.read_bytes()).public_key())
    accessory_cert = x509.load_pem_x509_certificate(args.accessory.read_bytes())
    ca = x509.load_pem_x509_certificate(args.ca.read_bytes())
    accessory_cert.verify_directly_issued_by(ca)
    now = time.time()
    if any(not cert.not_valid_before_utc.timestamp() <= now < cert.not_valid_after_utc.timestamp()
           for cert in (ca, accessory_cert)):
        raise ValueError("Recovery CA and accessory certificate must be current")
    roles = accessory_cert.extensions.get_extension_for_class(x509.SubjectAlternativeName).value
    if list(roles) != [x509.UniformResourceIdentifier("urn:nu54:role:recovery-accessory")]:
        raise ValueError("Accessory certificate needs the dedicated recovery URI")
    accessory = spki(accessory_cert.public_key())
    if len({source,target,accessory}) != 3 or any(len(entry) != 91 for entry in (source,target,accessory)) or spki(identity) not in (source,target):
        raise ValueError("Expected distinct P-256 source, target, and accessory keys")
    image = args.image.read_bytes()
    manifest = json.loads(args.manifest.read_text())
    if (not manifest.get("debugProtection") or not manifest.get("serialRecovery", {}).get("hardwareSecurityCounter") or
        hashlib.sha256(image).hexdigest() != manifest.get("artifacts", {}).get("app/zephyr/zephyr.signed.bin")):
        raise ValueError("Recovery requires a verified protected counted release image")
    der = ca.public_bytes(serialization.Encoding.DER)
    body = (b"N54H" + offer[4:101] + struct.pack(">Q", args.revision) + image_measurement(image) +
            source + target + accessory + struct.pack(">H", len(der)) + der)
    if len(body) + 64 > MAX_BUNDLE:
        raise ValueError("Recovery trust grant exceeds board limit")
    args.out.write_bytes(body + signed(args.authority, body))


def issue_reclaim(args):
    evidence = json.loads(args.evidence.read_text())
    dispositions = {"finalized": 1, "finalized_without_effect": 2}
    if evidence.get("disposition") not in dispositions:
        raise ValueError("Issuer must archive a finalized outcome or verified no-effect result")
    quote = args.quote.read_bytes()
    if len(quote) not in (672, 737):
        raise ValueError("Expected exact archived v2 quote bytes")
    quote = quote[:672]
    issuer_bound_quote = exact_hex(evidence["unsignedQuoteSha256"], 32)
    if hashlib.sha256(quote).digest() != issuer_bound_quote:
        raise ValueError("Archived disposition refers to another quote")
    generation = quote[536:544]
    expiry = quote[600:608]
    if int(time.time()) < int.from_bytes(expiry, "big") + 300:
        raise ValueError("Quote replay hold is still active")
    device = exact_hex(evidence["device"], 32)
    original = exact_hex(evidence["requestId"], 16)
    receipt_digest = exact_hex(evidence["approvalPreimageSha256"], 32)
    terminal = bytes([dispositions[evidence["disposition"]]])
    body = b"N54C" + device + original + receipt_digest + generation + expiry + terminal
    args.out.write_text((bytes([8]) + original + quote + terminal + signed(args.authority, body)).hex() + "\n")


class StationTLS:
    def __init__(self, key_path, certificate):
        from OpenSSL import SSL, crypto

        self.ssl = SSL
        context = SSL.Context(SSL.TLS_SERVER_METHOD)
        context.set_min_proto_version(SSL.TLS1_3_VERSION)
        context.set_max_proto_version(SSL.TLS1_3_VERSION)
        context.set_options(SSL.OP_NO_TICKET)
        context.set_session_cache_mode(SSL.SESS_CACHE_OFF)
        context.set_alpn_select_callback(
            lambda _connection, protocols: INSTALL_ALPN if INSTALL_ALPN in protocols else SSL.NO_OVERLAPPING_PROTOCOLS
        )
        context.use_certificate(crypto.X509.from_cryptography(certificate))
        context.use_privatekey(crypto.PKey.from_cryptography_key(private_key(key_path)))
        context.check_privatekey()
        self.connection = SSL.Connection(context, None)
        self.connection.set_accept_state()
        self.incoming = asyncio.Queue(maxsize=64)
        self.overflow = False

    def notified(self, _characteristic, data):
        try:
            self.incoming.put_nowait(bytes(data))
        except asyncio.QueueFull:
            self.overflow = True

    async def flush(self, send):
        while True:
            try:
                data = self.connection.bio_read(4096)
            except self.ssl.WantReadError:
                return
            await send(data)

    async def exchange(self, action, send):
        while True:
            try:
                result = action()
            except self.ssl.WantReadError:
                await self.flush(send)
                if self.overflow:
                    raise ValueError("Installation TLS receive buffer exceeded")
                self.connection.bio_write(await self.incoming.get())
            except self.ssl.WantWriteError:
                await self.flush(send)
            else:
                await self.flush(send)
                return result

    async def read(self, length, send):
        data = bytearray()
        while len(data) < length:
            chunk = await self.exchange(lambda: self.connection.recv(length - len(data)), send)
            if not chunk:
                raise ValueError("Installation TLS stream ended before the response")
            data.extend(chunk)
        return bytes(data)


async def commission(client, args, challenge):
    ticket = args.ticket.read_bytes()
    certificate = ticket_certificate(ticket, challenge)
    station = StationTLS(args.station_key, certificate)
    await client.start_notify(GATT + "2", station.notified)
    try:
        for offset in range(0, len(ticket), 16):
            await client.write_gatt_char(GATT + "4",
                struct.pack(">HH", len(ticket), offset) + ticket[offset:offset + 16], response=True)

        async def send(data):
            for offset in range(0, len(data), 20):
                await client.write_gatt_char(GATT + "1", data[offset:offset + 20], response=True)

        tls = station.connection
        await station.exchange(tls.do_handshake, send)
        if tls.get_protocol_version_name() != "TLSv1.3" or tls.get_alpn_proto_negotiated() != INSTALL_ALPN:
            raise ValueError("Installation TLS profile mismatch")
        exporter = tls.export_keying_material(INSTALL_EXPORTER, 32, challenge[1:])
        offer = bound_offer(await station.read(281, send), exporter, args.board_pin, args.device)
        if args.command == "offer":
            write_private(args.out, offer)
            print("Verified physical board and station TLS channel")
        else:
            bundle = args.bundle.read_bytes()
            kind = bundle[:4]
            if kind not in (b"N54P", b"N54Q", b"N54H") or not 121 <= len(bundle) <= MAX_BUNDLE:
                raise ValueError("Invalid enrollment bundle")
            end = 101 if kind == b"N54H" else 121
            if bundle[4:end] != offer[4:end]:
                raise ValueError("Grant differs from the current authenticated offer")
            payload = struct.pack(">H", len(bundle)) + bundle
            while payload:
                sent = await station.exchange(lambda: tls.send(payload), send)
                if not sent:
                    raise ValueError("Installation TLS stream closed")
                payload = payload[sent:]
            if await station.read(4, send) != b"N54K":
                raise ValueError("Board did not confirm the installation commit")
            print("Board confirmed the installation")
    finally:
        if client.is_connected:
            await client.stop_notify(GATT + "2")


async def nearby(args):
    from bleak import BleakClient

    async with BleakClient(args.address) as client:
        challenge = public_challenge(bytes(await client.read_gatt_char(GATT + "3")))
        if args.command == "challenge":
            write_private(args.out, challenge)
        else:
            await asyncio.wait_for(commission(client, args, challenge), timeout=30)

def main():
    cli = argparse.ArgumentParser(description=__doc__)
    sub = cli.add_subparsers(dest="command", required=True)
    pin = sub.add_parser("authority-pin")
    pin.add_argument("public_key", type=Path)
    station = sub.add_parser("station-key")
    station.add_argument("private_key", type=Path)
    station.add_argument("certificate", type=Path)
    challenge = sub.add_parser("challenge")
    challenge.add_argument("address")
    challenge.add_argument("out", type=Path)
    authorize = sub.add_parser("authorize-station")
    for name in ("challenge", "certificate", "authority", "out"):
        authorize.add_argument("--" + name, type=Path, required=True)
    wired = sub.add_parser("wired-pins")
    wired.add_argument("port")
    wired.add_argument("out", type=Path)
    wired.add_argument("--qr", type=Path)
    offer = sub.add_parser("offer")
    offer.add_argument("address")
    offer.add_argument("out", type=Path)
    key = sub.add_parser("offer-public-key")
    key.add_argument("offer", type=Path)
    key.add_argument("out", type=Path)
    put = sub.add_parser("install")
    put.add_argument("address")
    put.add_argument("bundle", type=Path)
    for command in (offer, put):
        command.add_argument("--station-key", type=Path, required=True)
        command.add_argument("--ticket", type=Path, required=True)
        command.add_argument("--board-pin", required=True)
        command.add_argument("--device", required=True)
    grant = sub.add_parser("issue")
    grant.add_argument("--board-pin", required=True)
    grant.add_argument("--device", required=True)
    grant.add_argument("--commercial-testnet", action="store_true",
                       help="enroll the exact StableNet 8283 TestUSDC2 contract/token and an explicit cap of at most 30 TestUSDC")
    grant.add_argument("--spending", action="store_true",
                       help="refuse a policy that the spending profile cannot use")
    grant.add_argument("--w10-manifest", help="explicit reviewed v2 manifest SHA-256 for a commercial renter board")
    grant.add_argument("--w10-out", type=Path, help="write the corresponding P05 commissioning publication separately")
    for argument in ("offer", "policy", "ca", "board-certificate", "display-public", "recovery-public", "authority", "out"):
        grant.add_argument("--" + argument, type=Path, required=True)
    hd = sub.add_parser("issue-hd")
    hd.add_argument("--board-pin", required=True)
    hd.add_argument("--revision", required=True, type=int)
    for name in ("offer", "source", "target", "accessory", "ca", "image", "manifest", "authority", "out"):
        hd.add_argument("--" + name, type=Path, required=True)
    reclaim = sub.add_parser("issue-reclaim")
    for name in ("evidence", "quote", "authority", "out"):
        reclaim.add_argument("--" + name, type=Path, required=True)
    reset = sub.add_parser("operator-reset", help="sign the reset of a spending wallet whose PIN is forgotten")
    reset.add_argument("request", help="the text the phone shows: NU54-RESET1:<board id>:<generation>:<nonce>")
    reset.add_argument("--authority", type=Path, required=True)
    reset.add_argument("--device", help="board id from the commissioning record; the request must match it")
    reset.add_argument("--yes", action="store_true", help="sign without asking; needs --device")
    args = cli.parse_args()
    if args.command == "authority-pin":
        print(point(public_file(args.public_key)).hex())
    elif args.command == "station-key":
        station_identity(args.private_key, args.certificate)
    elif args.command == "authorize-station":
        cert = x509.load_pem_x509_certificate(args.certificate.read_bytes())
        write_private(args.out, authorize_station(args.challenge.read_bytes(), cert, args.authority))
    elif args.command == "wired-pins":
        wired_pins(args)
    elif args.command == "offer-public-key":
        args.out.write_bytes(parse_offer(args.offer.read_bytes()).public_bytes(
            serialization.Encoding.PEM, serialization.PublicFormat.SubjectPublicKeyInfo))
    elif args.command == "issue":
        issue(args)
    elif args.command == "issue-hd":
        issue_hd(args)
    elif args.command == "issue-reclaim":
        issue_reclaim(args)
    elif args.command == "operator-reset":
        operator_reset(args)
    else:
        asyncio.run(nearby(args))


if __name__ == "__main__":
    main()
