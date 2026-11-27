"""Synthetic C1 watch-only source for the installed native Signet QA journey."""
import argparse
import base64
import ctypes
import os
from pathlib import Path
import signal
import ssl
import tempfile
import time

from wallet_connect_spend import P01, fixture
from w07NativeReceive import chain_public
from run_wallet_chain_nodes import B, P, SEED, FirmwareCore, buffer, lab


def main(args):
    core = FirmwareCore(P01 / "out/host/libwallet_chain_node_oracle.so")
    core.lib.chain_host_watch.argtypes = [P, ctypes.c_uint32, P]
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(args.cert, args.key)
    cert = base64.b64encode(ssl.PEM_cert_to_DER_cert(args.cert.read_text())).decode()
    case = {"exports": 0, "publicReplies": 0}

    def export(wire):
        if wire != bytes([13, 1, 2, 0, 0, 0, 0]):
            raise ValueError("Unexpected watch scope")
        output = (B * 80)()
        if core.lib.chain_host_watch(buffer(SEED), 0, output):
            raise ValueError("Host watch export refused")
        if args.proof and not args.proof.exists():
            args.proof.write_bytes(bytes(output))
        case["exports"] += 1
        return bytes(output)

    def public(wire):
        data = chain_public(core, wire, 2)
        case["publicReplies"] += 1
        return data

    def refuse(*_):
        raise ValueError("Watch-only fixture has no spending RPC")

    with tempfile.TemporaryDirectory(prefix="w16-native.") as directory:
        options = {"cert":cert,"relay":"wss://127.0.0.1:18449","case":case,"chain_public":public,
                   "chain_watch":export,"commissioned":{"chain":1,"profile":"bitcoin-signet",
                   "anchor":lab.COMMISSIONED[1],"rpc":"https://wallet.localhost:18449/bitcoin"}}
        server, worker, _, _ = fixture(Path(directory), refuse, "0x"+"12"*32, None, None, context, options)
        print("W16 fixture ready", flush=True)
        try:
            while True:
                time.sleep(1)
        finally:
            server.shutdown()
            server.server_close()
            worker.join()
            print(case, flush=True)


def interrupt(_signal, _frame):
    raise KeyboardInterrupt


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("cert", type=Path)
    parser.add_argument("key", type=Path)
    parser.add_argument("--proof", type=Path)
    os.umask(0o077)
    signal.signal(signal.SIGTERM, interrupt)
    try:
        main(parser.parse_args())
    except KeyboardInterrupt:
        pass
