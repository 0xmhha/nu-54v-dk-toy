"""Verify P01 BIP48 keys and canonical partials against an independent Core vector."""
import base64
import ctypes as C
import hashlib
import json
import pathlib
import re
import sys

HERE = pathlib.Path(__file__).resolve().parent
ALPHABET = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
VECTOR = json.loads((HERE / "bip48_vector.json").read_text())
B = C.c_ubyte
P = C.POINTER(B)


def roots():
    result = bytearray()
    for fp, key in re.findall(r"\[([a-f0-9]{8})/48h/1h/0h/2h\](tpub[1-9A-HJ-NP-Za-km-z]+)/0/\*", VECTOR["descriptors"][0]):
        number = 0
        for char in key:
            number = number * 58 + ALPHABET.index(char)
        decoded = number.to_bytes(82, "big")
        assert hashlib.sha256(hashlib.sha256(decoded[:78]).digest()).digest()[:4] == decoded[78:]
        result.extend(bytes.fromhex(fp) + decoded[13:78])
    assert len(result) == 207
    return bytes(result)


def maps(raw):
    offset = 5

    def var():
        nonlocal offset
        length = raw[offset]
        offset += 1
        if length == 253:
            length = int.from_bytes(raw[offset:offset + 2], "little")
            offset += 2
        result = raw[offset:offset + length]
        offset += length
        return result

    result = []
    while offset < len(raw):
        entry = {}
        while key := var():
            entry[key] = var()
        result.append(entry)
    return result


def sign(lib, psbt, registry=None, network=1):
    lib.chain_host_bip48_network.argtypes = [P, P, B, B, P, C.c_size_t, P, C.POINTER(C.c_size_t)]
    out, used = (B * 1998)(), C.c_size_t()

    def array(payload):
        return (B * len(payload)).from_buffer_copy(payload)

    result = lib.chain_host_bip48_network(array(bytes(range(1, 33))), array(registry or roots()),
                                 0, network, array(psbt), len(psbt), out, C.byref(used))
    if result:
        assert used.value == 0
        return None
    return bytes(out[:used.value])


def verify(lib):
    unsigned = base64.b64decode(VECTOR["unsignedPSBT"])
    standalone = sign(lib, unsigned)
    assert standalone
    one = maps(standalone)[1]
    partials = [{k: v for k, v in row.items() if k[0] == 2}
                for row in [maps(base64.b64decode(item))[1] for item in VECTOR["partialPSBTs"]]]
    product = {k: v for k, v in one.items() if k[0] == 2}
    assert len(product) == 1
    expected = next(item for item in partials if item.keys() == product.keys())
    # Independent implementations may choose different ECDSA nonces.
    assert all(sig[-1] == 1 for sig in expected.values())
    counterpart = base64.b64decode(VECTOR["partialPSBTs"][partials.index(next(item for item in partials
                                            if item.keys() != product.keys()))])
    final = sign(lib, counterpart)
    assert final
    full = {k: v for k, v in maps(final)[1].items() if k[0] == 2}
    assert len(full) == 2
    assert sign(lib, standalone) is None
    assert sign(lib, final) is None
    assert sign(lib, unsigned, roots()[:98] + bytes([roots()[98] ^ 1]) + roots()[99:]) is None
    assert all(sign(lib, unsigned[:i]) is None for i in range(len(unsigned)))
    return full


def verify_two(lib):
    fixture = json.loads((HERE / "bip48_two_input_vector.json").read_text())
    unsigned = base64.b64decode(fixture["unsignedPSBT"])
    expected, counterpart = map(base64.b64decode, fixture["partialPSBTs"])
    standalone = sign(lib, unsigned, network=2)
    assert standalone == expected
    full = sign(lib, counterpart, network=2)
    assert full and sign(lib, full, network=2) is None
    for fields in maps(full)[1:3]:
        assert len([key for key in fields if key[0] == 2]) == 2
    broken = maps(counterpart)
    broken[2] = {k:v for k,v in broken[2].items() if k[0] != 2}

    def var(blob):
        header = bytes((len(blob),)) if len(blob) < 253 else b"\xfd" + len(blob).to_bytes(2, "little")
        return header + blob

    incomplete = b"psbt\xff" + b"".join(b"".join(var(k)+var(v) for k,v in row.items())+b"\0" for row in broken)
    assert sign(lib, incomplete, network=2) is None


if __name__ == "__main__":
    lib = C.CDLL(sys.argv[1])
    verify(lib)
    verify_two(lib)
    print("Portable P01 BIP48 registered roots and 2-of-3 partial PSBT review passed")
