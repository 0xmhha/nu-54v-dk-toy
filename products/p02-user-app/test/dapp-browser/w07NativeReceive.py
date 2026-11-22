"""Serve synthetic receive accounts to the installed wallet's normal C1 parser."""
import argparse
import asyncio
import base64
import json
from pathlib import Path
import signal
import ssl
import tempfile

from wallet_connect_reference import anvil_node, chain_ready
from wallet_connect_spend import fixture, P01
from run_wallet_chain_nodes import FirmwareCore


def chain_public(core, wire, network=1):
    # Test profile, account zero, external branch.
    if len(wire) != 11 or wire[0] != 10 or wire[1] not in (1, 2, 3) or wire[2] != network or any(wire[3:10]) or (
        wire[10] not in ((0, 1) if wire[1] == 1 else (0,))
    ):
        raise ValueError("Unexpected synthetic receive account")
    key, pkh, fingerprint = core.account(wire[1], 0, wire[10], network=network)
    return wire + (key + bytes(25) if wire[1] == 2 else key + pkh + (fingerprint if wire[1] == 1 else bytes(4)))


async def main(options):
    signal.signal(signal.SIGINT, signal.default_int_handler)
    task = asyncio.current_task()
    asyncio.get_running_loop().add_signal_handler(signal.SIGTERM, task.cancel)
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(options.cert, options.key)
    cert = base64.b64encode(ssl.PEM_cert_to_DER_cert(options.cert.read_text())).decode()
    core = FirmwareCore(options.oracle)
    count = {1:0, 2:0, 3:0}

    def capture(wire):
        response = chain_public(core, wire)
        count[wire[1]] += 1
        print(json.dumps({"hostPublicReply":wire[1],"count":count[wire[1]]}),flush=True)
        return response

    with tempfile.TemporaryDirectory(prefix="nu54-w07-native-") as directory, anvil_node(options.anvil) as rpc:
        genesis = await chain_ready(rpc)
        config = {"cert":cert, "relay":"wss://127.0.0.1:18449", "case":{}, "chain_public":capture}
        server, thread, _, _ = fixture(Path(directory), rpc, genesis, None, None, context, config)
        try:
            print("W07 loopback receive fixture ready", flush=True)
            await asyncio.get_running_loop().create_future()
        finally:
            server.shutdown()
            server.server_close()
            thread.join()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("cert", type=Path)
    parser.add_argument("key", type=Path)
    parser.add_argument("--oracle", type=Path, default=P01/"out/host/libwallet_chain_node_oracle.so")
    parser.add_argument("--anvil", default="anvil")
    try:
        asyncio.run(main(parser.parse_args()))
    except (KeyboardInterrupt, asyncio.CancelledError):
        pass
