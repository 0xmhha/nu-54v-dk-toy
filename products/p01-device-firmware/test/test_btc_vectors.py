"""Independent OpenSSL-backed secp256k1 witness verification with synthetic PSBTs."""
import ctypes as C
import hashlib
import pathlib
import struct
import subprocess
import tempfile

from chain_openssl_test import compact, secp

ROOT = pathlib.Path(__file__).resolve().parents[1]
B = C.c_ubyte
PTR = C.POINTER(B)
D256 = C.CFUNCTYPE(C.c_int, PTR, C.c_size_t, PTR)
DERIVE = C.CFUNCTYPE(C.c_int, C.c_void_p, C.c_uint32, C.c_uint32, PTR, PTR)


class Policy(C.Structure):
    _fields_ = [("hash256", D256), ("derive", DERIVE), ("wallet", C.c_void_p),
                ("fingerprint", C.c_uint32), ("coin", C.c_uint32),
                ("account", C.c_uint32), ("max_fee", C.c_uint64)]


class Input(C.Structure):
    _fields_ = [("branch", C.c_uint32), ("index", C.c_uint32),
                ("amount", C.c_uint64), ("pubkey", B * 33), ("digest", B * 32)]


class Output(C.Structure):
    _fields_ = [("amount", C.c_uint64), ("script", B * 34),
                ("length", B), ("change", C.c_bool)]


class Intent(C.Structure):
    _fields_ = [("inputs", Input * 4), ("outputs", Output * 4), ("fee", C.c_uint64),
                ("locktime", C.c_uint32), ("input_count", B), ("output_count", B),
                ("commitment", B * 32), ("replaceable", C.c_bool)]


SIGN = C.CFUNCTYPE(C.c_int, C.c_void_p, C.POINTER(Input), PTR)


def d256(data):
    return hashlib.sha256(hashlib.sha256(data).digest()).digest()


def hash160(data):
    return hashlib.new("ripemd160", hashlib.sha256(data).digest()).digest()


def u32(value):
    return struct.pack("<I", value)


def amount(value):
    return struct.pack("<Q", value)


def var(data):
    n = len(data)
    prefix = bytes([n]) if n < 253 else b"\xfd" + struct.pack("<H", n)
    return prefix + data


def entry(key, value):
    return var(key) + var(value)


def txout(value, script):
    return amount(value) + var(script)


def witness_script(public):
    return b"\x00\x14" + hash160(public)


def buf(data):
    return (B * len(data)).from_buffer_copy(data)


def run(lib):
    from functools import lru_cache

    @lru_cache
    def private(branch, index):
        return secp(101 + branch * 10 + index)

    def public(branch, index):
        return private(branch, index).public_bytes()

    @D256
    def hash_cb(source, length, target):
        C.memmove(target, d256(C.string_at(source, length)), 32)
        return 0

    @DERIVE
    def derive(_, branch, index, target, digest):
        if index > 500:
            return -1
        key = public(branch, index)
        C.memmove(target, key, 33)
        C.memmove(digest, hash160(key), 20)
        return 0

    calls = []

    @SIGN
    def sign(_, key, signature):
        child = key.contents
        calls.append((child.branch, child.index))
        raw = private(child.branch, child.index).sign(bytes(child.digest))
        C.memmove(signature, compact(raw), 64)
        return 0

    lib.nu54_btc_review.argtypes = [PTR, C.c_size_t, C.POINTER(Policy), C.POINTER(Intent)]
    lib.nu54_btc_finalize.argtypes = [PTR, C.c_size_t, C.POINTER(Policy), PTR, SIGN,
                                     PTR, C.c_size_t, C.POINTER(C.c_size_t)]
    policy = Policy(hash_cb, derive, None, 0x1234, 1, 0, 1000)
    own = public(0, 3)
    change = public(1, 2)
    prevout = txout(12000, witness_script(own))
    previous = (u32(1) + b"\x01" + bytes(32) + u32(0xFFFFFFFF) + b"\x02\x01\x51" +
                u32(0xFFFFFFFF) + b"\x01" + prevout + u32(0))
    outpoint = d256(previous) + u32(0)
    sequence = u32(0xFFFFFFFD)
    external = txout(10000, witness_script(public(0, 7)))
    change_out = txout(1500, witness_script(change))
    raw = u32(2) + b"\x01" + outpoint + b"\x00" + sequence + b"\x02" + external + change_out + u32(0)

    def path(branch, index):
        return b"".join(map(u32, [0x1234, 0x80000054, 0x80000001, 0x80000000, branch, index]))

    header = b"psbt\xff" + entry(b"\x00", raw) + b"\x00"

    def psbt(sighash=1, witness=prevout, change_path=path(1, 2), prev=previous):
        return (header + entry(b"\x00", prev) + entry(b"\x01", witness) +
                entry(b"\x03", u32(sighash)) + entry(b"\x06" + own, path(0, 3)) +
                b"\x00\x00" + entry(b"\x02" + change, change_path) + b"\x00")

    valid = psbt()
    review = Intent()
    assert lib.nu54_btc_review(buf(valid), len(valid), C.byref(policy), C.byref(review)) == 0
    assert (review.fee, review.replaceable, review.input_count, review.output_count) == (500, True, 1, 2)
    assert [review.outputs[i].change for i in range(2)] == [False, True]

    signed = (B * 800)()
    size = C.c_size_t()

    def finalize(data, approval):
        return lib.nu54_btc_finalize(buf(data), len(data), C.byref(policy), approval,
                                     sign, signed, len(signed), C.byref(size))

    for bad in [psbt(sighash=0x81), psbt(witness=txout(12001, witness_script(own))),
                psbt(change_path=path(0, 2)), psbt(prev=previous[:-1]), valid + b"\x00",
                *[valid[:n] for n in range(len(valid))]]:
        assert finalize(bad, review.commitment) != 0
        assert not size.value and not calls
    assert finalize(valid, buf(bytes(32))) != 0
    assert not calls
    assert finalize(valid, review.commitment) == 0 and calls == [(0, 3)]
    wire = bytes(signed[:size.value])
    assert wire[:6] == u32(2) + b"\x00\x01"
    assert wire[6:len(raw) - 2] == raw[4:-4]
    witness = wire[len(raw) - 2:-4]
    assert witness[0] == 2 and witness[1 + witness[1]] == 1
    assert witness[2 + witness[1]:] == b"\x21" + own
    assert wire[-4:] == raw[-4:]
    script_code = b"\x76\xa9\x14" + hash160(own) + b"\x88\xac"
    preimage = (u32(2) + d256(outpoint) + d256(sequence) + outpoint + var(script_code) +
                amount(12000) + sequence + d256(external + change_out) + u32(0) + u32(1))
    private(0, 3).verify(witness[2:1 + witness[1]], d256(preimage))


if __name__ == "__main__":
    with tempfile.TemporaryDirectory() as temp:
        shared = pathlib.Path(temp) / "libnu54btc.so"
        subprocess.run(["cc", "-shared", "-fPIC", "-Wall", "-Wextra", "-Werror",
                        "-I", str(ROOT / "core/include"), str(ROOT / "core/src/nu54_btc.c"),
                        "-o", str(shared)], check=True)
        run(C.CDLL(str(shared)))
        print("Bitcoin PSBT review and independently verified native SegWit witness passed")
