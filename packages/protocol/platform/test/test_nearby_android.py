#!/usr/bin/env python3
"""Exercise actual Android Keystore and Conscrypt on an explicitly selected disposable AVD."""
import argparse
import base64
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


SCRIPT = Path(__file__).with_name("payment_case.mts")
PROTOCOL = Path(__file__).resolve().parents[2]
RUNNER = "com.nu54renter.test/com.nu54renter.ble.NearbyTlsLab"


def run(arguments, captures=False, env=None):
    return subprocess.run(arguments, check=True, text=True, env=env,
                          stdout=subprocess.PIPE if captures else None).stdout


def test(serial, apk, tests):
    assert re.fullmatch(r"emulator-[0-9]+", serial), "select an AVD, never a physical device"
    adb = Path(os.environ["ANDROID_HOME"]) / "platform-tools/adb"
    connected = [adb, "-s", serial]
    run([*connected, "install", "-r", apk])
    run([*connected, "install", "-r", tests])

    def instrument(**values):
        flags = [entry for pair in values.items() for entry in ("-e", *pair)]
        result = run([*connected, "shell", "am", "instrument", "-w", *flags, RUNNER], True)
        if "nearby_tls_lab=pass" not in result:
            raise RuntimeError(result)
        return dict(re.findall(r"^INSTRUMENTATION_RESULT: ([a-z0-9_]+)=(.*)$", result, re.M))

    def b64(path):
        return base64.b64encode(path.read_bytes()).decode()

    with tempfile.TemporaryDirectory(prefix="nu54-android-nearby-") as tmp:
        folder = Path(tmp)
        keys = instrument(phase="identity")
        openssl = os.environ.get("OPENSSL", "openssl")

        def crypto(*args):
            return run([openssl, *args])

        root, root_key = folder / "ca.pem", folder / "ca.key"
        crypto("req", "-new", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-x509", "-days", "1",
               "-nodes", "-subj", "/CN=Disposable Test Root", "-keyout", root_key, "-out", root)
        crypto("x509", "-in", root, "-outform", "DER", "-out", folder / "ca.der")
        args = {"ca": b64(folder / "ca.der")}
        for i, name in enumerate(("customer", "customer2", "terminal", "payment", "board"), 1):
            role = "customer" if name.startswith("customer") else "terminal" if name == "payment" else name
            key, csr, pem, der, extension = [folder / f"{name}.{suffix}" for suffix in ("key", "csr", "pem", "der", "ext")]
            crypto("req", "-new", "-newkey", "ec", "-pkeyopt", "ec_paramgen_curve:P-256", "-nodes",
                   "-subj", f"/CN={name}", "-keyout", key, "-out", csr)
            extension.write_text(f"subjectAltName=URI:urn:nu54:role:{role}\n"
                                 "basicConstraints=critical,CA:FALSE\nkeyUsage=digitalSignature\n"
                                 "extendedKeyUsage=clientAuth,serverAuth\n")
            extra = []
            if name in keys:
                public_der, public_pem = folder / "public.der", folder / "public.pem"
                public_der.write_bytes(base64.b64decode(keys[name]))
                crypto("pkey", "-pubin", "-inform", "DER", "-in", public_der, "-out", public_pem)
                extra = ["-force_pubkey", public_pem]
                key.unlink()
            crypto("x509", "-req", "-in", csr, "-CA", root, "-CAkey", root_key, "-set_serial", str(i), "-days", "1",
                   "-extfile", extension, *extra, "-out", pem)
            crypto("x509", "-in", pem, "-outform", "DER", "-out", der)
            args[name] = b64(der)
        instrument(**args)
        print("Android native: independent visitor keys/epochs, 600 framed messages, forged/pin/expiry/role/ALPN cases", flush=True)
        peer = folder / "native-peer"
        env = {**os.environ, "NU54_MOBILE_TEST_PEER_OUTPUT": str(peer)}
        run(["sh", PROTOCOL / "v2/testdata/test-transport.sh"], env=env)
        run(["node", "--experimental-strip-types", SCRIPT, folder])
        config = json.loads((folder / "expected.json").read_text())
        with subprocess.Popen([peer, root, folder / "board.pem", folder / "board.key", folder / "payment.pem", "2",
                               folder / "case.bin"], stdout=subprocess.PIPE, text=True) as board:
            port = board.stdout.readline().strip()
            assert port.isdecimal()
            port_ref = f"tcp:{port}"
            run([*connected, "reverse", port_ref, port_ref])
            try:
                approval_args = {**args, "phase": "payment", "port": port, "quote": b64(folder / "quote.bin"),
                                 "expires": str(config["expiry"])}
                first = base64.b64decode(instrument(**approval_args)["approval"])
                resumed = base64.b64decode(instrument(**approval_args)["approval"])
                assert first[0] == 1 and resumed[0] == 2 and first[1:] == resumed[1:]
                response = folder / "approval.bin"
                response.write_bytes(first)
                run(["node", "--experimental-strip-types", SCRIPT, folder, response])
                print("Android native C2: live P01 C wire/signer counterpart, independently recovered approval and same signature after reconnect", flush=True)
            finally:
                run([*connected, "reverse", "--remove", port_ref])
                board.terminate()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("serial")
    parser.add_argument("apk", type=Path)
    parser.add_argument("tests", type=Path)
    args = parser.parse_args()
    test(args.serial, args.apk.resolve(), args.tests.resolve())
