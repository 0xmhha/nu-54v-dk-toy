"""Provide loopback authority, board host review and real Anvil receipts for the installed W28 app."""
import argparse
import asyncio
import base64
import json
from pathlib import Path
import signal
import ssl
import tempfile

from w28DirectOracle import CHAINS, ORACLE, sign_direct
from wallet_connect_reference import anvil_node, chain_ready, finalized_payment
from wallet_connect_spend import fixture, SPENDER


async def main(options):
    task = asyncio.current_task()
    asyncio.get_running_loop().add_signal_handler(signal.SIGTERM, task.cancel)
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(options.cert, options.key)
    cert = base64.b64encode(ssl.PEM_cert_to_DER_cert(options.cert.read_text())).decode()
    with tempfile.TemporaryDirectory(prefix="nu54-w28-installed-") as path, anvil_node(options.anvil, options.chain) as rpc:
        genesis = await chain_ready(rpc)
        if options.chain == 84532:
            rpc("anvil_setCode", [ORACLE, "0x6101f460005260206000f3"])
        balance = int(rpc("eth_getBalance", [SPENDER,"latest"]),16)
        control = {"chain": options.chain, "boardCalls": 0, "recipient": SPENDER}

        def direct(*args):
            result = sign_direct(*args)
            control["boardCalls"] += 1
            return result

        def submitted(tx_hash, board_calls):
            proof = finalized_payment(rpc, tx_hash, balance, board_calls)
            control["receipt"] = proof.pop("txHash")
            print(json.dumps({"chain":options.chain,"hostSignatures":proof["hostBoardSignatures"],
                              "finalizedWei":proof["finalizedWei"]}),flush=True)

        config = {"cert":cert, "relay":"wss://127.0.0.1:18449", "case":control, "evm_chain":options.chain,
                  "direct":direct, "submitted":submitted}
        server, thread, _, _ = fixture(Path(path),rpc,genesis,None,None,context,config)
        try:
            print("W28 ordinary transfer fixture ready",flush=True)
            await asyncio.get_running_loop().create_future()
        finally:
            server.shutdown()
            server.server_close()
            thread.join()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("chain", type=int, choices=CHAINS)
    parser.add_argument("cert", type=Path)
    parser.add_argument("key", type=Path)
    parser.add_argument("--anvil", default="anvil")
    try:
        asyncio.run(main(parser.parse_args()))
    except (KeyboardInterrupt, asyncio.CancelledError):
        pass
