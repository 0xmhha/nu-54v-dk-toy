"""OpenSSL Ed25519 cross-check of SLIP-0010-derived Solana transactions."""
import ctypes as C
import hashlib
import hmac
import pathlib
import struct
import subprocess
import tempfile

from chain_openssl_test import Key

ROOT = pathlib.Path(__file__).resolve().parents[1]
B = C.c_ubyte
P = C.POINTER(B)
HASH = C.CFUNCTYPE(C.c_int, P, C.c_size_t, P)
HMAC = C.CFUNCTYPE(C.c_int, P, C.c_size_t, P, C.c_size_t, P)
OP = C.CFUNCTYPE(C.c_int, P, C.c_void_p)
SIGN = C.CFUNCTYPE(C.c_int, C.c_void_p, P, P, C.c_size_t, P)


class Policy(C.Structure):
    _fields_ = [(name, B * 32) for name in ["payer", "blockhash", "message_hash"]] + [
        (name, C.c_uint64) for name in ["height", "last", "fee", "max_fee"]] + [
        ("allow_spl", C.c_bool), ("mint", B * 32), ("decimals", B), ("hash", HASH)]


class Intent(C.Structure):
    _fields_ = [(name, B * 32) for name in ["payer", "source", "recipient", "mint",
                                           "blockhash", "commitment"]] + [
        ("decimals", B), ("amount", C.c_uint64), ("fee", C.c_uint64),
        ("last", C.c_uint64), ("spl", C.c_bool), ("static_v0", C.c_bool)]


def buf(data):
    return (B * len(data)).from_buffer_copy(data)


@HASH
def hash_cb(data, size, target):
    C.memmove(target, hashlib.sha256(C.string_at(data, size)).digest(), 32)
    return 0


@HMAC
def hmac_cb(key, key_len, message, message_len, target):
    result = hmac.digest(C.string_at(key, key_len), C.string_at(message, message_len), "sha512")
    C.memmove(target, result, 64)
    return 0


def run(lib):
    lib.nu54_sol_child.argtypes = [P, C.c_uint32, HMAC, OP, C.c_void_p]
    lib.nu54_sol_review.argtypes = [P, C.c_size_t, C.POINTER(Policy), C.POINTER(Intent)]
    lib.nu54_sol_finalize.argtypes = [P, C.c_size_t, C.POINTER(Policy), P, SIGN, C.c_void_p,
                                     P, C.c_size_t, C.POINTER(C.c_size_t)]
    seed = bytes(range(32))
    keys = []

    @OP
    def derive(child, _):
        keys.append(Key(C.string_at(child, 32), "ed25519"))
        return 0

    for i in [0, 1, 0]:
        assert lib.nu54_sol_child(buf(seed), i, hmac_cb, derive, None) == 0
        node = hmac.digest(b"ed25519 seed", seed, "sha512")
        for n in [44, 501, i, 0]:
            node = hmac.digest(node[32:], b"\x00" + node[:32] + struct.pack(">I", n | 0x80000000), "sha512")
        expected = Key(node[:32], "ed25519").public_bytes()
        assert keys[-1].public_bytes() == expected
    payer, recipient, restored = [key.public_bytes() for key in keys]
    assert payer == restored and payer != recipient
    blockhash = bytes([42] * 32)
    mint = bytes([33] * 32)
    token = bytes.fromhex("06ddf6e1d765a193d9cbe146ceeb79ac1cb485ed5f5b37913a8cf5857eff00a9")
    lamports = struct.pack("<Q", 1_000_000)
    transfer = b"\x01\x00\x01\x03" + payer + recipient + bytes(32) + blockhash
    transfer += b"\x01\x02\x02\x00\x01\x0c\x02\x00\x00\x00" + lamports
    token_transfer = b"\x80\x01\x00\x02\x05" + payer + bytes([19] * 32) + recipient + mint + token
    token_transfer += blockhash + b"\x01\x04\x04\x01\x03\x02\x00\x0a\x0c" + lamports + b"\x06\x00"
    calls = []

    @SIGN
    def sign(_, account, message, length, result):
        if C.string_at(account, 32) != payer:
            return -1
        calls.append(1)
        C.memmove(result, keys[0].sign(C.string_at(message, length)), 64)
        return 0

    for wire, spl in [(transfer, False), (token_transfer, True)]:
        policy = Policy(buf(payer), buf(blockhash), buf(hashlib.sha256(wire).digest()),
                        90, 100, 5000, 6000, spl, buf(mint), 6, hash_cb)
        intent = Intent()
        assert lib.nu54_sol_review(buf(wire), len(wire), C.byref(policy), C.byref(intent)) == 0
        assert (intent.amount, bytes(intent.recipient), intent.spl, intent.static_v0) == (1000000, recipient, spl, spl)
        result = (B * 1232)()
        size = C.c_size_t()
        approved = intent.commitment

        def finalize(data):
            return lib.nu54_sol_finalize(buf(data), len(data), C.byref(policy),
                                         approved, sign, None, result, len(result), C.byref(size))

        assert finalize(wire) == 0
        signed = bytes(result[:size.value])
        assert signed[0] == 1 and signed[65:] == wire
        keys[0].verify(signed[1:65], wire)
        before = len(calls)
        policy.height = 101
        assert finalize(wire) != 0 and not size.value
        policy.height = 100
        for bad in [wire + b"\x00", b"\x81" + wire[1:], *[wire[:i] for i in range(len(wire))]]:
            assert finalize(bad) != 0 and not size.value
        policy.decimals = 8
        if spl:
            assert finalize(wire) != 0
        assert len(calls) == before

    # The 32-byte Stake Program public identifier.
    stake_program = bytes.fromhex("06a1d8179137542a983437bdfe2a7ab2557f535c8a78722b68a49dc000000000")
    delegation = (bytes.fromhex("01000507") + payer + recipient + bytes([61]*32) + bytes([62]*32) +
                  bytes([63]*32) + bytes([64]*32) + stake_program + blockhash + bytes.fromhex("0106060102030405000402000000"))
    policy = Policy(buf(payer), buf(blockhash), buf(hashlib.sha256(delegation).digest()),
                    90, 100, 5000, 6000, False, buf(mint), 9, hash_cb)
    before = len(calls)
    assert lib.nu54_sol_review(buf(delegation), len(delegation), C.byref(policy), C.byref(Intent())) != 0
    assert finalize(delegation) != 0 and not size.value and len(calls) == before


if __name__ == "__main__":
    with tempfile.TemporaryDirectory() as temp:
        shared = pathlib.Path(temp) / "libnu54sol.so"
        subprocess.run(["cc", "-shared", "-fPIC", "-Wall", "-Wextra", "-Werror",
                        "-I", str(ROOT / "core/include"), str(ROOT / "core/src/nu54_solana.c"),
                        "-o", str(shared)], check=True)
        run(C.CDLL(str(shared)))
        print("Solana SOL and SPL message review and independent Ed25519 verification passed")
