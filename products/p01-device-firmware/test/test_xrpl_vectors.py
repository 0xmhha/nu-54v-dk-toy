"""Synthetic canonical XRPL Payment tests with OpenSSL-backed independent verification."""
import ctypes as C
import hashlib
import pathlib
import struct
import subprocess
import tempfile

from chain_openssl_test import compact, encode_der, secp

ROOT = pathlib.Path(__file__).resolve().parents[1]
B = C.c_ubyte
P = C.POINTER(B)
HASH = C.CFUNCTYPE(C.c_int, P, C.c_size_t, P)
SIGN = C.CFUNCTYPE(C.c_int, C.c_void_p, P, P, P, C.POINTER(C.c_size_t))


class Policy(C.Structure):
    _fields_ = [("account", B * 20), ("public_key", B * 33), ("issuer", B * 20), ("currency", B * 20),
                ("ledger", C.c_uint32), ("max_delta", C.c_uint32), ("sequence", C.c_uint32),
                ("ticket", C.c_uint32), ("max_fee", C.c_uint64), ("allow_issued", C.c_bool),
                ("sha", HASH)]


class Intent(C.Structure):
    _fields_ = [(name, B * 20) for name in ["account", "destination", "issuer", "currency"]] + [
        ("digest", B * 32)] + [(name, C.c_uint32) for name in ["tag", "sequence", "ticket", "last"]] + [
        ("coefficient", C.c_uint64), ("fee", C.c_uint64), ("exponent", C.c_int16),
        ("has_tag", C.c_bool), ("issued", C.c_bool)]


def buf(data):
    return (B * len(data)).from_buffer_copy(data)


def half_sha512(data):
    return hashlib.sha512(data).digest()[:32]


@HASH
def hash_cb(data, size, target):
    C.memmove(target, half_sha512(C.string_at(data, size)), 32)
    return 0


def xrp(value):
    return struct.pack(">Q", (1 << 62) | value)


def u32(value):
    return struct.pack(">I", value)


def run(lib):
    lib.nu54_xrpl_review.argtypes = [P, C.c_size_t, C.POINTER(Policy), C.POINTER(Intent)]
    lib.nu54_xrpl_finalize.argtypes = [P, C.c_size_t, C.POINTER(Policy), P, SIGN, C.c_void_p,
                                      P, C.c_size_t, C.POINTER(C.c_size_t)]
    private = secp(144)
    public = private.public_bytes()
    account = hashlib.new("ripemd160", hashlib.sha256(public).digest()).digest()
    dest = bytes([9] * 20)
    issuer = account
    currency = bytes(12) + b"USD" + bytes(5)
    calls = []

    @SIGN
    def sign(_, expected, digest, output, size):
        if C.string_at(expected, 33) != public:
            return -1
        calls.append(1)
        der = encode_der(compact(private.sign(C.string_at(digest, 32))))
        C.memmove(output, der, len(der))
        size[0] = len(der)
        return 0

    for issued in [False, True]:
        policy = Policy(buf(account), buf(public), buf(issuer), buf(currency), 100, 20, 7, 8, 20, issued, hash_cb)
        coefficient = 1234567890123456 if issued else 2_000_000
        exponent = -15 if issued else 0
        value = (struct.pack(">Q", (3 << 62) | ((exponent + 97) << 54) | coefficient)
                 + currency + issuer) if issued else xrp(coefficient)
        header = b"\x12\x00\x00\x22" + u32(0x80000000) + b"\x24" + u32(0 if issued else 7)
        header += b"\x2e" + u32(0) + b"\x20\x1b" + u32(110)
        if issued:
            header += b"\x20\x29" + u32(8)
        before_sig = header + b"\x61" + value + b"\x68" + xrp(12) + b"\x73\x21" + public
        after_sig = b"\x81\x14" + account + b"\x83\x14" + dest
        raw = before_sig + after_sig
        intent = Intent()
        assert lib.nu54_xrpl_review(buf(raw), len(raw), C.byref(policy), C.byref(intent)) == 0
        assert (intent.coefficient, intent.exponent, intent.issued, intent.fee,
                intent.tag, intent.has_tag, intent.last) == (coefficient, exponent, issued, 12, 0, True, 110)
        assert (intent.sequence, intent.ticket) == ((0, 8) if issued else (7, 0))
        result = (B * 298)()
        size = C.c_size_t()

        def finalize(wire):
            return lib.nu54_xrpl_finalize(buf(wire), len(wire), C.byref(policy), intent.digest,
                                          sign, None, result, len(result), C.byref(size))

        assert finalize(raw) == 0
        signed = bytes(result[:size.value])
        assert signed.startswith(before_sig + b"\x74") and signed.endswith(after_sig)
        assert signed[len(before_sig) + 1] == len(signed) - len(before_sig) - len(after_sig) - 2
        der = signed[len(before_sig) + 2:-len(after_sig)]
        private.verify(der, half_sha512(b"STX\x00" + raw))
        before = len(calls)
        policy.ledger = 110
        assert finalize(raw) != 0
        policy.ledger = 100
        partial = raw[:4] + u32(0x80020000) + raw[8:]
        for bad in [partial, raw + b"\x01\x12", *[raw[:i] for i in range(len(raw))]]:
            assert finalize(bad) != 0 and not size.value
        assert len(calls) == before


if __name__ == "__main__":
    with tempfile.TemporaryDirectory() as temp:
        shared = pathlib.Path(temp) / "libnu54xrp.so"
        subprocess.run(["cc", "-shared", "-fPIC", "-Wall", "-Wextra", "-Werror",
                        "-I", str(ROOT / "core/include"), str(ROOT / "core/src/nu54_xrpl.c"),
                        "-o", str(shared)], check=True)
        run(C.CDLL(str(shared)))
        print("XRPL XRP/issued review and independently verified canonical Payment passed")
