#!/usr/bin/env python3
"""Test Apple Network.framework using separately enrolled disposable keys and in-process links."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


def local(args, capture=False):
    return subprocess.run(args, check=True, stdout=subprocess.PIPE if capture else None).stdout


def test(headers, payment):
    with tempfile.TemporaryDirectory(prefix="nu54-apple-nearby-") as temporary:
        folder = Path(temporary)
        openssl = os.environ.get("OPENSSL", "openssl")
        ca, ca_key = folder / "ca.pem", folder / "ca.key"
        local([openssl, "req", "-new", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256",
               "-x509", "-days", "1", "-nodes", "-subj", "/CN=Temporary Nearby Root",
               "-keyout", ca_key, "-out", ca])
        local([openssl, "x509", "-in", ca, "-outform", "DER", "-out", folder / "ca.der"])
        names = ("customer", "customer2", "terminal", "payment", "board") if payment else ("customer", "customer2", "terminal")
        for number, name in enumerate(names, 1):
            role = "terminal" if name == "payment" else "customer" if name.startswith("customer") else name
            key, csr, pem, ext = [folder / f"{name}.{extension}" for extension in ("key", "csr", "pem", "ext")]
            local([openssl, "req", "-new", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes",
                   "-subj", f"/CN=Temporary {name}", "-keyout", key, "-out", csr])
            ext.write_text(f"subjectAltName=URI:urn:nu54:role:{role}\n"
                           "basicConstraints=critical,CA:FALSE\nkeyUsage=digitalSignature\n"
                           "extendedKeyUsage=clientAuth,serverAuth\n")
            local([openssl, "x509", "-req", "-in", csr, "-CA", ca, "-CAkey", ca_key, "-set_serial", str(number),
                   "-days", "1", "-extfile", ext, "-out", pem])
            local([openssl, "x509", "-in", pem, "-outform", "DER", "-out", folder / f"{name}.der"])
            text = local([openssl, "ec", "-in", key, "-noout", "-text"], capture=True).decode()
            private, public = re.search(r"priv:\s*(.*?)\s*pub:\s*(.*?)\s*ASN1 OID:", text, re.S).groups()
            private, public = (bytes.fromhex(re.sub(r"[:\s]", "", part)) for part in (private, public))
            assert len(private) == 32 and len(public) == 65 and public[0] == 4
            (folder / f"{name}.raw").write_bytes(public + private)
        program = folder / "nearby-apple"
        local(["xcrun", "clang++", "-fobjc-arc", "-std=c++17", "-I", headers,
               "-framework", "Foundation", "-framework", "Security", "-framework", "Network",
               "-framework", "CoreBluetooth", Path(__file__).with_name("nearby_apple.mm"), "-o", program])
        local([program, folder])
        if payment:
            compare_payment(folder, program)


def compare_payment(folder, program):
    peer = folder / "peer"
    protocol = Path(__file__).resolve().parents[2]
    source = Path(__file__).with_name("payment_case.mts")
    subprocess.run(["sh", protocol / "v2/testdata/test-transport.sh"], check=True,
                   env={**os.environ, "NU54_MOBILE_TEST_PEER_OUTPUT": str(peer)})
    local(["node", "--experimental-strip-types", source, folder])
    expected = json.loads((folder / "expected.json").read_text())
    with subprocess.Popen([peer, folder / "ca.pem", folder / "board.pem", folder / "board.key",
                           folder / "payment.pem", "2", folder / "case.bin"], stdout=subprocess.PIPE, text=True) as board:
        try:
            port = board.stdout.readline().strip()
            assert port.isdecimal()
            first, resumed = folder / "first.bin", folder / "resumed.bin"
            for path in (first, resumed):
                local([program, folder, port, str(expected["expiry"]), path])
            one, two = first.read_bytes(), resumed.read_bytes()
            assert one[0] == 1 and two[0] == 2 and one[1:] == two[1:]
            local(["node", "--experimental-strip-types", source, folder, first])
            print("Apple native C2: live P01 C wire/signer counterpart, same independently recovered approval after reconnect")
        finally:
            board.terminate()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--headers", type=Path, default=Path(__file__).resolve().parents[1] / "ios")
    parser.add_argument("--payment", action="store_true")
    args = parser.parse_args()
    test(args.headers.resolve(), args.payment)
