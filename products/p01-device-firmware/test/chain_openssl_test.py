"""Host-only independent OpenSSL CLI operations for deterministic synthetic keys."""
import pathlib
import subprocess
import tempfile

ORDER = 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141


def command(args):
    return subprocess.run(["openssl", *args], capture_output=True, check=True).stdout


class Key:
    def __init__(self, seed: bytes, kind: str):
        self.state = tempfile.TemporaryDirectory()
        self.directory = pathlib.Path(self.state.name)
        self.kind = kind
        self.private = self.directory / "private"
        self.public = self.directory / "public"
        if kind == "ed25519":
            self.private.write_bytes(bytes.fromhex("302e020100300506032b657004220420") + seed)
        elif kind == "secp256k1":
            self.private.write_bytes(bytes.fromhex("302e0201010420") + seed +
                                     bytes.fromhex("a00706052b8104000a"))
        else:
            raise ValueError(kind)
        args = ["pkey", "-inform", "DER", "-in", str(self.private), "-pubout", "-outform", "DER"]
        if kind == "secp256k1":
            args += ["-ec_conv_form", "compressed"]
        self.public.write_bytes(command(args))

    def public_bytes(self):
        return self.public.read_bytes()[-(32 if self.kind == "ed25519" else 33):]

    def sign(self, message):
        path = self.directory / "msg"
        path.write_bytes(message)
        args = ["pkeyutl", "-sign", "-keyform", "DER", "-inkey", str(self.private), "-in", str(path)]
        if self.kind == "ed25519":
            args += ["-rawin"]
        return command(args)

    def verify(self, signature, message):
        path = self.directory / "msg"
        proof = self.directory / "sig"
        path.write_bytes(message)
        proof.write_bytes(signature)
        args = ["pkeyutl", "-verify", "-pubin", "-keyform", "DER", "-inkey", str(self.public),
                "-in", str(path), "-sigfile", str(proof)]
        if self.kind == "ed25519":
            args += ["-rawin"]
        command(args)


def secp(value):
    return Key(value.to_bytes(32, "big"), "secp256k1")


def compact(signature):
    assert signature[0] == 0x30 and signature[1] == len(signature) - 2 and signature[2] == 2
    size = signature[3]
    r = int.from_bytes(signature[4:4 + size], "big")
    assert signature[4 + size] == 2 and signature[5 + size] == len(signature) - 6 - size
    s = int.from_bytes(signature[6 + size:], "big")
    return r.to_bytes(32, "big") + min(s, ORDER - s).to_bytes(32, "big")


def encode_der(rs):
    def integer(data):
        data = data.lstrip(b"\x00") or b"\x00"
        if data[0] & 0x80:
            data = b"\x00" + data
        return bytes([2, len(data)]) + data

    numbers = integer(rs[:32]) + integer(rs[32:])
    return bytes([0x30, len(numbers)]) + numbers
