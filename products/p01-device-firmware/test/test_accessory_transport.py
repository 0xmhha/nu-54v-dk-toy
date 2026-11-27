"""Exercise the accessory executable over serial and UI PTYs with disposable mTLS peers."""
import hashlib
import os
from pathlib import Path
import pty
import select
import shutil
import ssl
import struct
import subprocess
import sys
import tempfile
import time
import tty


PROFILE = bytes.fromhex("0201")
NETWORK = struct.pack(">BIQ", 1, 0, 8283)
ADDRESS = bytes.fromhex("919538116b4f25f1ce01429fd9ed7964556bf565")
PUBLIC_KEY = bytes.fromhex(
    "0492df2f2c31a5c4b0d4946e922cc3bd25ad7196ffeb049905b0952b9ac48ef25"
    "fcc3d07fe5a557d01f607680d8862447e55c9bc06c292537115d6f7b1d2af88a8"
)
SOURCE, TARGET = b"s" * 32, b"t" * 32
SEED = bytes(range(32))


class Peer:
    def __init__(self, serial, context):
        self.serial = serial
        self.reader, self.writer = ssl.MemoryBIO(), ssl.MemoryBIO()
        self.tls = context.wrap_bio(self.reader, self.writer, server_side=True)

    def flush(self):
        while self.writer.pending:
            data = self.writer.read()
            while data:
                if not select.select([], [self.serial], [], 5)[1]:
                    raise TimeoutError("serial transport stopped accepting TLS records")
                used = os.write(self.serial, data[:128])
                if used <= 0:
                    raise RuntimeError("serial write accepted zero bytes")
                data = data[used:]

    def call(self, operation, *args):
        while True:
            try:
                value = operation(*args)
                self.flush()
                return value
            except ssl.SSLWantReadError:
                self.flush()
                if not select.select([self.serial], [], [], 7)[0]:
                    raise TimeoutError("offline TLS peer timed out")
                self.reader.write(os.read(self.serial, 16384))

    def read(self, size):
        output = b""
        while len(output) < size:
            output += self.call(self.tls.read, size - len(output))
        return output

    def packet(self, data=None):
        if data is None:
            return self.read(struct.unpack(">H", self.read(2))[0])
        frame = struct.pack(">H", len(data)) + data
        while frame:
            frame = frame[self.call(self.tls.write, frame):]
        return None


def openssl(*args, data=None):
    return subprocess.run(
        [os.environ.get("OPENSSL", "openssl"), *map(str, args)], input=data,
        capture_output=True, check=True,
    ).stdout


def proof(path, data):
    return openssl("dgst", "-sha256", "-sign", path, data=data)


def identities(folder, enrolled=None):
    for role in ("boards", "independent"):
        root, key = folder / f"{role}-root.pem", folder / f"{role}-root.key"
        if enrolled is not None and role == "boards":
            shutil.copyfile(enrolled / "ca.pem", root)
            shutil.copyfile(enrolled / "ca.key", key)
            continue
        openssl("req", "-new", "-x509", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256",
                "-nodes", "-days", 1, "-subj", f"/CN=Disposable {role} CA", "-keyout", key, "-out", root)
    for serial, name in enumerate(("source", "target", "accessory", "swapped"), 1):
        if enrolled is not None and name == "source":
            shutil.copyfile(enrolled / "board.pem", folder / "source.pem")
            shutil.copyfile(enrolled / "board.key", folder / "source.key")
            continue
        role = "recovery-accessory" if name == "accessory" else "board"
        req, ext, cert, secret = (folder / f"{name}.{suffix}" for suffix in ("csr", "ext", "pem", "key"))
        openssl("req", "-new", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256",
                "-nodes", "-subj", f"/CN={name}", "-keyout", secret, "-out", req)
        ext.write_text(f"subjectAltName=URI:urn:nu54:role:{role}\n"
                       "basicConstraints=critical,CA:FALSE\nkeyUsage=digitalSignature\n"
                       "extendedKeyUsage=clientAuth,serverAuth\n")
        ca = "independent" if name == "accessory" else "boards"
        openssl("x509", "-req", "-in", req, "-CA", folder / f"{ca}-root.pem", "-CAkey", folder / f"{ca}-root.key",
                "-set_serial", serial, "-days", 1, "-extfile", ext, "-out", cert)


def exchange(program, folder, role, function, *, source_id=SOURCE, target_id=TARGET, seal_key=b"k" * 32,
             board_key=None):
    serial, client_serial = pty.openpty()
    panel, client_panel = pty.openpty()
    tty.setraw(client_serial)
    os.set_blocking(serial, False)
    read_key, write_key = os.pipe()
    os.write(write_key, seal_key)
    os.close(write_key)
    subprocess_args = [
        program, os.ttyname(client_serial), folder / "boards-root.pem", folder / "accessory.pem",
        folder / "accessory.key", folder / "source.pem", source_id.hex(), folder / "target.pem", target_id.hex(),
        folder / "capsule.bin", folder / "proof.bin", str(read_key),
    ]
    child = subprocess.Popen(subprocess_args, stdin=client_panel, stdout=client_panel, stderr=client_panel,
                             pass_fds=(read_key,))
    os.close(read_key)
    os.close(client_panel)
    try:
        context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        context.minimum_version = context.maximum_version = ssl.TLSVersion.TLSv1_3
        context.verify_mode = ssl.CERT_REQUIRED
        context.num_tickets = 0
        context.load_verify_locations(folder / "independent-root.pem")
        context.load_cert_chain(folder / f"{role}.pem", board_key or folder / f"{role}.key")
        context.set_alpn_protocols(["nu54-recovery/1"])
        peer = Peer(serial, context)
        peer.call(peer.tls.do_handshake)
        assert peer.tls.selected_alpn_protocol() == "nu54-recovery/1"
        expected_accessory = ssl.PEM_cert_to_DER_cert((folder / "accessory.pem").read_text())
        assert peer.tls.getpeercert(binary_form=True) == expected_accessory

        def confirm(approved=True):
            displayed = b""
            while b"Type AUTHORIZE" not in displayed:
                if not select.select([panel], [], [], 7)[0]:
                    raise TimeoutError("independent panel did not ask for authorization")
                displayed += os.read(panel, 4096)
            assert b"Request " in displayed or role == "source"
            os.write(panel, b"AUTHORIZE\n" if approved else b"CANCEL\n")

        if function:
            function(peer, confirm)
        return child.wait(timeout=9)
    except Exception:
        if select.select([panel], [], [], 0)[0]:
            try:
                print(os.read(panel, 4096).decode(errors="replace"), file=sys.stderr)
            except OSError:
                pass
        raise
    finally:
        if child.poll() is None:
            child.kill()
            child.wait()
        for fd in (serial, client_serial, panel):
            os.close(fd)


def run(program, folder):
    identities(folder)
    claim = b"NU54-backup-claim-v1\x00" + PROFILE + SOURCE + struct.pack(">Q", 1) + NETWORK + PUBLIC_KEY + ADDRESS
    signed = proof(folder / "source.key", claim)
    capsule = bytes([3]) + claim + bytes([len(signed)]) + signed + SEED

    def backup(peer, confirm):
        peer.packet(bytes([6]) + b"c" * 32 + b"r" * 16 + SOURCE + b"o" * 32 + b"e" * 16 +
                    struct.pack(">QQ", 1, int(time.time()) + 90))
        confirm()
        assert peer.packet() == bytes([10])
        peer.packet(capsule)
        want = bytes([8]) + expected[0] if expected else bytes([7])
        assert peer.packet() == want

    expected = []
    assert exchange(program, folder, "source", backup) == 0
    assert SEED not in (folder / "capsule.bin").read_bytes()
    assert exchange(program, folder, "swapped", None) != 0

    def restore(peer, confirm, acknowledge=True):
        now = int(time.time())
        challenge, request, owner, epoch = b"c" * 32, b"r" * 16, b"o" * 32, b"e" * 16
        expiry = now + 90
        peer.packet(bytes([5]) + challenge + request + TARGET + owner + epoch + struct.pack(">QQ", 2, expiry))
        confirm()
        assert peer.packet() == capsule
        body = b"NU54-recovery-receipt-v1\x00" + challenge + request + PROFILE + SOURCE + TARGET
        body += NETWORK + PUBLIC_KEY + ADDRESS + owner + epoch + struct.pack(">QQBQQ", 1, 2, 1, now, expiry)
        target_signature = proof(folder / "target.key", body)
        partial = body + bytes([len(target_signature)]) + target_signature
        peer.packet(partial)
        full = peer.packet()
        assert full.startswith(partial)
        used = full[len(partial)]
        accessory_proof = full[len(partial) + 1:len(partial) + 1 + used]
        assert full[len(partial) + 1 + used:] == bytes([len(signed)]) + signed
        (folder / "signature.der").write_bytes(accessory_proof)
        (folder / "public.pem").write_bytes(openssl("pkey", "-in", folder / "accessory.key", "-pubout"))
        openssl("dgst", "-sha256", "-verify", folder / "public.pem", "-signature", folder / "signature.der", data=body)
        assert not (folder / "proof.bin").exists()
        digest = hashlib.sha256(full).digest() if acknowledge else bytes(32)
        peer.packet(bytes([9]) + digest)
        if acknowledge:
            expected.append(full)

    assert exchange(program, folder, "target", lambda peer, confirm: restore(peer, confirm, False)) != 0
    assert not (folder / "proof.bin").exists()
    assert exchange(program, folder, "target", restore) == 0
    assert (folder / "proof.bin").exists()
    assert exchange(program, folder, "source", backup) == 0


if __name__ == "__main__":
    with tempfile.TemporaryDirectory(prefix="nu54-accessory-") as directory:
        run(Path(sys.argv[1]).resolve(), Path(directory))
