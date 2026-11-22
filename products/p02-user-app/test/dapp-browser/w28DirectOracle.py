"""Check ordinary EVM board review and production raw transactions on disposable ledgers."""
import argparse
import asyncio
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import time

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import ec

from wallet_connect_reference import SANDBOX, anvil_node, chain_ready
from wallet_connect_spend import P01
from test_revoke_anvil import BOARD, KEY, OWNER, SPENDER, TOKEN, WalletAuthority
from evm_transfer import ORACLE, keccak256, parse

CHAINS = (8283, 11155111, 84532)
NATIVE = "0x" + "00" * 20


def sign_direct(gateway, wire, public, now):
    sender = bytes.fromhex(OWNER[2:])
    board = bytes.fromhex(BOARD)
    terms = parse(wire, public, board, sender, 1, now)
    statement = b"N54V" + board + (1).to_bytes(8, "big") + sender + wire[:101] + keccak256(wire[165:])
    call = [*SANDBOX, str(gateway), wire.hex(), statement.hex(), wire[101:165].hex(), BOARD, OWNER[2:],
            KEY[2:], str(now), "1"]
    result = subprocess.run(call, text=True, timeout=10, capture_output=True)
    if result.returncode:
        raise RuntimeError(f"Direct host signer refused the authorized grant (code={result.returncode})")
    raw = result.stdout.strip()
    return terms, "0x" + raw


async def verify(anvil, gateway, chain):
    with anvil_node(anvil, chain) as rpc:
        genesis = await chain_ready(rpc)
        if chain == 84532:
            # Stub only Base's protocol-owned predeploy in this disposable generic ledger.
            # VM execution, block anchoring and transaction receipts are from Anvil.
            rpc("anvil_setCode", [ORACLE, "0x6101f460005260206000f3"])
        now = int(time.time())
        issuer = ec.generate_private_key(ec.SECP256R1())
        profile = {"device": BOARD, "sender": OWNER, "generation": 1, "leaseRevision": "1", "expires": now + 110,
                   "networks": [{"chain": str(chain), "genesis": genesis, "rpc": "loopback",
                                 "feeLimit": str(10**17), "assets": [{"address": NATIVE}]}]}
        packet = {"jsonrpc": "2.0", "id": 1, "method": "nu54_walletQuote",
                  "params": [{"token": TOKEN, "chain": str(chain), "asset": NATIVE, "recipient": SPENDER, "amount": "9"}]}
        before = int(rpc("eth_getBalance", [SPENDER, "latest"]), 16)
        with tempfile.TemporaryDirectory(prefix="nu54-direct-review-") as path:
            pem = Path(path) / "authority.pem"
            pem.touch(mode=0o600)
            pem.write_bytes(issuer.private_bytes(serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8,
                                                  serialization.NoEncryption()))
            authority = WalletAuthority(profile, pem, hashlib.sha256(TOKEN.encode()).hexdigest(), lambda _: rpc)
            wire = bytes.fromhex(authority.handle(packet))
        terms, signed = sign_direct(gateway, wire, issuer.public_key(), now)
        assert terms[0] == chain and terms[3] == 9 and terms[4].hex() == SPENDER[2:]
        assert int.from_bytes(wire[25:33], "big") == (1000 if chain == 84532 else 0)
        reference = rpc("eth_sendRawTransaction", [signed])
        assert reference == "0x" + keccak256(bytes.fromhex(signed[2:])).hex()
        rpc("anvil_mine", ["0x81"])
        receipt = rpc("eth_getTransactionReceipt", [reference])
        assert receipt["transactionHash"] == reference and receipt["status"] == "0x1"
        canonical = rpc("eth_getBlockByNumber", [receipt["blockNumber"], False])
        finalized = rpc("eth_getBlockByNumber", ["finalized", False])
        assert receipt["blockHash"] == canonical["hash"]
        assert int(receipt["blockNumber"], 16) <= int(finalized["number"], 16)
        assert int(rpc("eth_getBalance", [SPENDER, "finalized"]), 16) - before == 9
        return {"chain": chain, "hostSignatures": 1, "finalizedWei": 9, "sameReceiptHash": True}


async def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--anvil", default="anvil")
    parser.add_argument("--gateway", type=Path, default=P01 / "out/host/test_device_evm")
    options = parser.parse_args()
    for chain in CHAINS:
        print(json.dumps(await verify(options.anvil, options.gateway.resolve(), chain)), flush=True)


if __name__ == "__main__":
    asyncio.run(main())
