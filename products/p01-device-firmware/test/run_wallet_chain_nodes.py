#!/usr/bin/env python3
"""Verify portable firmware signing with isolated upstream reference nodes."""
import argparse
import base64
import ctypes as C
import hashlib
import importlib.util
import json
import pathlib
import socket
import struct
import sys
import subprocess
import tempfile
import time
from contextlib import contextmanager
from types import SimpleNamespace

ROOT = pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))
spec = importlib.util.spec_from_file_location("wallet_chain_lab", ROOT / "scripts/wallet_chain_lab.py")
lab = importlib.util.module_from_spec(spec)
spec.loader.exec_module(lab)
B = C.c_ubyte
P = C.POINTER(B)
SEED = bytes(range(1, 33))
XRPL_IMAGE = "xrpllabsofficial/xrpld@sha256:353d5e016bb93519e9fcac715cdc8c2205b96c4cfe2d1f0f1d22a22f6efaff70"


def command(args):
    run = subprocess.run([str(arg) for arg in args], check=False, capture_output=True, text=True, timeout=60)
    if run.returncode:
        raise RuntimeError(f"{args[0]} failed: {run.stderr[-1500:]}")
    return run.stdout


def buffer(data):
    return (B * len(data)).from_buffer_copy(data)


class FirmwareCore:
    def __init__(self, path):
        self.lib = C.CDLL(str(path))
        self.lib.chain_host_public.argtypes = [P, B, C.c_uint32, C.c_uint32, P, P, P]
        self.lib.chain_host_public_network.argtypes = [P, B, B, C.c_uint32, C.c_uint32, P, P, P]
        self.lib.chain_host_sign.argtypes = [P, P, C.c_size_t, P, C.POINTER(C.c_size_t)]

    def account(self, chain, account, branch=0, network=1):
        public, pkh, fingerprint = (B * 33)(), (B * 20)(), (B * 4)()
        assert self.lib.chain_host_public_network(buffer(SEED), chain, network, account, branch, public, pkh, fingerprint) == 0
        return bytes(public[:32 if chain == 2 else 33]), bytes(pkh), bytes(fingerprint)

    def bitcoin_public_at(self, account, branch, index):
        derive = self.lib.chain_host_bitcoin_public_at
        derive.argtypes = [P, C.c_uint32, C.c_uint32, C.c_uint32, P, P, P]
        public, pkh, fingerprint = (B * 33)(), (B * 20)(), (B * 4)()
        assert derive(buffer(SEED), account, branch, index, public, pkh, fingerprint) == 0
        return bytes(public), bytes(pkh), bytes(fingerprint)

    def sign(self, grant, reject=False):
        out = (B * 1232)()
        size = C.c_size_t()
        rc = self.lib.chain_host_sign(buffer(SEED), buffer(grant), len(grant), out, C.byref(size))
        if reject:
            assert rc and not size.value
            return None
        assert rc == 0 and size.value
        return bytes(out[:size.value])


def available_port():
    with socket.socket() as local:
        local.bind(("127.0.0.1", 0))
        return local.getsockname()[1]


def eventual(callback, timeout=30):
    deadline = time.monotonic() + timeout
    while True:
        try:
            value = callback()
            if value:
                return value
        except (ValueError, KeyError, RuntimeError, OSError):
            if time.monotonic() >= deadline:
                raise
        if time.monotonic() >= deadline:
            raise AssertionError("local validator timed out")
        time.sleep(0.2)


@contextmanager
def daemon(args, log):
    with log.open("wb") as output:
        process = subprocess.Popen([str(arg) for arg in args], stdout=output, stderr=subprocess.STDOUT)
        try:
            yield
        except Exception:
            print(log.read_text(errors="replace")[-2500:])
            raise
        finally:
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()


def b58(raw, alphabet=lab.ALPHABET):
    count = len(raw) - len(raw.lstrip(b"\x00"))
    n = int.from_bytes(raw, "big")
    tail = ""
    while n:
        n, index = divmod(n, 58)
        tail = alphabet[index] + tail
    return alphabet[0] * count + tail


def ripple(pkh):
    raw = b"\x00" + pkh
    return b58(raw + lab.d256(raw)[:4], lab.RIPPLE)


def base_grant(chain, raw, fee, context):
    assert len(context) == 90
    # The node oracle exercises the portable firmware's review and serializer.
    # P01 ARM separately enforces the enrollment and independent-display signatures.
    return struct.pack(">BBBIQQ", 11, chain, 1, 0, int(time.time()) + 55, fee) + context + bytes(104) + raw


def check_restore(firmware, chain):
    first = firmware.account(chain, 0)
    restored = firmware.account(chain, 0)
    following = firmware.account(chain, 1)
    assert first == restored and first[0] != following[0]
    return first


def bitcoin(args, scratch, firmware):
    port = available_port()
    instance = ["-regtest", "-datadir=" + str(scratch), "-rpcport=" + str(port)]
    node = [args.bitcoind, *instance, "-networkactive=0", "-listen=0", "-dnsseed=0", "-txindex=1",
            "-rpcbind=127.0.0.1", "-rpcallowip=127.0.0.1", "-fallbackfee=0.00001"]
    with daemon(node, scratch / "node.log"):
        cookie = scratch / "regtest/.cookie"
        eventual(lambda: cookie.exists())
        local = "http://127.0.0.1:" + str(port)
        credentials = cookie.read_text().strip()
        connection = lab.rpc_client(local, credentials)
        eventual(lambda: connection("getblockchaininfo", []))
        connection("createwallet", {"wallet_name": "miner"})
        connection("createwallet", {"wallet_name": "watch", "disable_private_keys": True, "blank": True})
        miner = lab.rpc_client(local + "/wallet/miner", credentials, timeout=60)
        watch = lab.rpc_client(local + "/wallet/watch", credentials)
        key, _, fingerprint = check_restore(firmware, 1)
        change, _, _ = firmware.account(1, 0, 1)
        descriptor = connection("getdescriptorinfo", [f"wpkh({key.hex()})"])["descriptor"]
        owned = connection("deriveaddresses", [descriptor])[0]
        assert watch("importdescriptors", [[{"desc": descriptor, "timestamp": "now"}]])[0]["success"]
        blocks_at = miner("getnewaddress", [])
        miner("generatetoaddress", [101, blocks_at])
        funded = miner("sendtoaddress", {"address": owned, "amount": 0.006, "fee_rate": 2}, relay=True)
        assert not funded.get("error"), funded.get("error")
        funding = funded["result"]
        second_funding = miner("sendtoaddress", {"address": owned, "amount": 0.006, "fee_rate": 2})
        miner("generatetoaddress", [6, blocks_at])
        target = miner("getnewaddress", [])
        found = watch("getaddressinfo", [owned])
        assert found["address"] == owned and (found.get("ismine") or found.get("iswatchonly"))
        received = watch("listunspent", [6, 9999999, [owned]])
        assert sum(int(coin["amount"] * 100000000) for coin in received) == 1200000
        anchor = watch("getblockhash", [0])
        request = {"publicKey": key.hex(), "change": change.hex(), "fingerprint": fingerprint.hex(),
                   "anchor": anchor, "destination": target, "amount": "0.01", "replaceable": True}
        try:
            lab.bitcoin(watch, {**request, "anchor": "0" * 64}, 0)
            raise AssertionError("wrong Bitcoin genesis accepted")
        except ValueError:
            pass
        raw, fee, context, review = lab.bitcoin(watch, request, 0)
        assert {item.split(":")[0] for item in review["inputs"]} == {funding, second_funding} and len(review["allOutputs"]) == 2
        grant = base_grant(1, raw, fee, context)
        rejected = grant[:15] + struct.pack(">Q", fee - 1) + grant[23:]
        assert firmware.sign(rejected, reject=True) is None
        signed = firmware.sign(grant)
        row = watch("testmempoolaccept", [[signed.hex()]])[0]
        assert row["allowed"], row
        free_draft, free_fee, free_context, _ = lab.bitcoin(watch, {**request, "fee": 2000}, 0)
        free_candidate = firmware.sign(base_grant(1, free_draft, free_fee, free_context))
        assert watch("testmempoolaccept", [[free_candidate.hex()]])[0]["allowed"]
        assert lab.bitcoin_predecessor(watch, row["txid"])["state"] == "absent"
        transaction = watch("sendrawtransaction", [signed.hex()])
        assert transaction == row["txid"]
        # The wallet gateway independently rediscovers both confirmed parents from Core.
        raw2, fee2, context2, review2 = lab.bitcoin(watch, {**request, "fee": 2000, "replaces": transaction}, 0)
        assert fee2 > fee and review2["inputs"] == review["inputs"] and context2[1:33].hex() == transaction
        assert review2["allOutputs"][0] == review["allOutputs"][0]
        try:
            lab.bitcoin(watch, {**request, "amount": "0.009", "fee": 2000, "replaces": transaction}, 0)
            raise AssertionError("altered payout accepted for replacement")
        except ValueError:
            pass
        no_increase = context2[:33] + struct.pack(">Q", fee2) + context2[41:]
        assert firmware.sign(base_grant(1, raw2, fee2, no_increase), reject=True) is None
        replacement = firmware.sign(base_grant(1, raw2, fee2, context2))
        second = watch("testmempoolaccept", [[replacement.hex()]])[0]
        assert second["allowed"] and second["txid"] != transaction, second
        # Exercise the exact gateway used by W08. The pending scope and timestamp
        # file are disposable substitutes for the private local issuer directory.
        issuer = SimpleNamespace(lease_revision=1, review_dir=scratch,
                                 wall_start=int(time.time()), mono_start=time.monotonic())
        grant2 = bytearray(base_grant(1, raw2, fee2, context2))
        grant2[113:145] = lab.h256(anchor.encode("ascii"))
        grant2[145:153] = struct.pack(">Q", 1)
        digest = lab.h256(grant2).hex()
        (scratch / (digest + ".json")).write_text(json.dumps({"grant": grant2.hex()}))
        approved = {"original": transaction, "id": second["txid"], "signed": replacement.hex(),
                    "grant": digest, "anchor": anchor}

        def triage(disposition):
            checks = 0

            def simulate(method, params, relay=False):
                nonlocal checks
                if method == "getrawtransaction" and params == [transaction, True]:
                    if disposition == "confirmed":
                        return {"result": {"txid": transaction, "confirmations": 1}}
                    if disposition == "conflict":
                        return {"result": {"txid": transaction, "confirmations": -1}}
                if method == "getmempoolentry" and params == [transaction]:
                    checks += 1
                    if disposition == "absent" or (disposition == "race" and checks == 2):
                        return {"error": {"code": -5}, "result": None}
                if method == "sendrawtransaction":
                    raise AssertionError("an ineligible predecessor reached Core submission")
                return watch(method, params, relay=relay)

            try:
                lab.submit_bitcoin_replacement(issuer, simulate, approved)
                raise AssertionError(disposition + " predecessor was accepted")
            except ValueError:
                return

        for state in ("absent", "confirmed", "conflict", "race"):
            triage(state)
        assert lab.bitcoin_predecessor(watch, transaction)["state"] == "live"
        final = lab.submit_bitcoin_replacement(issuer, watch, approved)
        assert lab.bitcoin_predecessor(watch, transaction)["state"] == "absent"

        block = miner("generatetoaddress", [1, blocks_at])[0]
        observed = connection("getrawtransaction", [final, True])
        assert observed["blockhash"] == block and observed["confirmations"] == 1
        assert {row["txid"] for row in observed["vin"]} == {funding, second_funding} and observed["vout"][0]["scriptPubKey"]["address"] == target
        return {"initial": transaction, "funding": [funding, second_funding], "replacement": final, "block": block,
                "recipient": target, "feeSatoshis": fee2, "confirmed": True,
                "predecessorCases": ["absent", "confirmed", "conflict", "race", "live"]}


def solana(args, scratch, firmware):
    port = available_port()
    faucet = available_port()
    gossip = available_port()
    payer, _, _ = check_restore(firmware, 2)
    recipient = b58(firmware.account(2, 1)[0])
    sender = b58(payer)
    mint, source, token_to = [hashlib.sha256(name).digest() for name in (b"mint", b"from", b"to")]

    def token_account(owner, amount):
        return (mint + owner + struct.pack("<Q", amount) + bytes(36) + b"\x01" + bytes(56))

    mint_data = struct.pack("<I", 1) + payer + struct.pack("<QBB", 100000000, 6, 1) + bytes(36)
    validators_args = []
    for key, state in [
        (mint, mint_data), (source, token_account(payer, 100000000)),
        (token_to, token_account(firmware.account(2, 1)[0], 0)),
    ]:
        assert len(state) in (82, 165)
        file = scratch / (b58(key) + ".json")
        file.write_text(json.dumps({"pubkey": b58(key), "account": {"lamports": 10_000_000,
            "data": [base64.b64encode(state).decode(), "base64"], "owner": b58(lab.TOKENS),
            "executable": False, "rentEpoch": 0}}))
        validators_args += ["--account", b58(key), file]
    validator = [args.validator, "--ledger", scratch / "ledger", "--reset", "--bind-address", "127.0.0.1",
                 "--rpc-port", str(port), "--faucet-port", str(faucet), "--mint", sender,
                 "--gossip-port", str(gossip), "--log", *validators_args]
    with daemon(validator, scratch / "node.log"):
        rpc = lab.rpc_client("http://127.0.0.1:" + str(port), None)
        anchor = eventual(lambda: rpc("getGenesisHash", []), 90)
        eventual(lambda: rpc("getBlockHeight", [{"commitment": "confirmed"}]) > 0, 90)

        def finalized(id):
            value = rpc("getSignatureStatuses", [[id], {"searchTransactionHistory": True}])["value"][0]
            return value and value["confirmationStatus"] == "finalized" and value["err"] is None
        policy = SimpleNamespace(spl_mint=None, spl_decimals=6)
        raw, fee, context, _ = lab.solana(rpc, {"anchor": anchor, "publicKey": payer.hex(),
            "destination": recipient, "amount": "0.01"}, policy)
        grant = base_grant(2, raw, fee, context)
        aged = bytearray(grant)
        aged[55:63] = (int.from_bytes(context[40:48], "big") + 1).to_bytes(8, "big")
        assert firmware.sign(bytes(aged), reject=True) is None
        signed = firmware.sign(grant)
        def send(message):
            reply = rpc("sendTransaction", [base64.b64encode(message).decode(),
                {"encoding": "base64", "skipPreflight": False, "preflightCommitment": "confirmed"}], relay=True)
            assert not reply.get("error"), reply.get("error")
            assert reply["result"] == b58(message[1:65])
            return reply["result"]

        txid = send(signed)
        eventual(lambda: finalized(txid), 90)
        balance = rpc("getBalance", [recipient, {"commitment": "finalized"}])["value"]
        assert balance == 10_000_000
        result = rpc("getTransaction", [txid, {"encoding": "json", "commitment": "finalized"}])
        assert result["meta"]["fee"] == fee and result["meta"]["err"] is None
        policy.spl_mint = b58(mint)
        raw2, fee2, context2, _ = lab.solana(rpc, {"anchor": anchor, "publicKey": payer.hex(),
            "destination": b58(token_to), "source": b58(source), "asset": b58(mint), "amount": "1.234567"}, policy)
        v0 = b"\x80" + raw2 + b"\x00"
        replacement_fee = rpc("getFeeForMessage", [base64.b64encode(v0).decode(), {"commitment": "confirmed"}])["value"]
        assert replacement_fee == fee2
        variant = firmware.sign(base_grant(2, v0, fee2, context2))
        token_id = send(variant)
        eventual(lambda: finalized(token_id), 90)
        token_balance = rpc("getTokenAccountBalance", [b58(token_to), {"commitment": "finalized"}])["value"]
        assert token_balance["amount"] == "1234567" and token_balance["decimals"] == 6
        return {"id": txid, "slot": result["slot"], "recipient": recipient, "lamports": balance,
                "feeLamports": fee, "finalized": True,
                "spl": {"id": token_id, "mint": b58(mint), "baseUnits": token_balance["amount"], "version": 0}}


def xrpl(args, scratch, firmware):
    name = "nu54-xrpl-" + scratch.name.lower()
    config = scratch / "xrpld.cfg"
    config.write_text("""[server]
port_rpc_admin_local
[port_rpc_admin_local]
port=5005
ip=127.0.0.1
admin=127.0.0.1
protocol=http
[node_db]
type=NuDB
path=/tmp/xrpl-data/nudb
[database_path]
/tmp/xrpl-data/sqlite
[debug_logfile]
/tmp/xrpl-data/debug.log
[network_id]
1
""")
    auto = ["docker", "run", "--detach", "--rm", "--platform", "linux/amd64", "--pull", "never",
            "--network", "none", "--name", name, "-v", str(config) + ":/tmp/local.cfg:ro",
            "--entrypoint", "/usr/bin/xrpld", XRPL_IMAGE, "--conf", "/tmp/local.cfg", "--standalone", "--start"]
    command(auto)
    try:
        def rpc(method, params):
            payload, = params
            packet = json.loads(command(["docker", "exec", name, "/usr/bin/xrpld", "--silent",
                "--conf", "/tmp/local.cfg", "json", method, json.dumps(payload)]))
            return packet["result"]

        eventual(lambda: rpc("server_info", [{}]).get("info"), 90)
        key, hash_value, _ = check_restore(firmware, 3)
        sender = ripple(hash_value)
        recipient = ripple(firmware.account(3, 1)[1])
        master = "rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"
        for address in (sender, recipient):
            funding = rpc("submit", [{"secret": "masterpassphrase", "tx_json": {
                "TransactionType": "Payment", "Account": master, "Destination": address, "Amount": "100000000"}}])
            assert funding["engine_result"] == "tesSUCCESS", funding.get("engine_result")
            rpc("ledger_accept", [{}])
        policy = SimpleNamespace(xrpl_network=1, xrpl_currency=None, xrpl_issuer=None)
        request = {"anchor": "1", "publicKey": key.hex(), "sender": sender, "destination": recipient,
                   "amount": "5", "tag": 0}
        raw, fee, context, _ = lab.xrpl(rpc, request, policy)
        grant = base_grant(3, raw, fee, context)
        partial_flag = raw[:4] + b"\x80\x02\x00\x00" + raw[8:]
        assert firmware.sign(base_grant(3, partial_flag, fee, context), reject=True) is None
        signed = firmware.sign(grant)
        txid = hashlib.sha512(b"TXN\x00" + signed).digest()[:32].hex().upper()
        submit = rpc("submit", [{"tx_blob": signed.hex().upper(), "fail_hard": True}])
        assert submit["engine_result"] == "tesSUCCESS" and submit["tx_json"]["hash"] == txid, submit
        rpc("ledger_accept", [{}])
        validated = rpc("tx", [{"transaction": txid, "binary": False, "api_version": 1}])
        assert validated["validated"] and validated["hash"] == txid and validated["Destination"] == recipient
        assert validated["DestinationTag"] == 0 and validated["meta"]["delivered_amount"] == "5000000"
        receive = rpc("wallet_propose", [{"passphrase": "nu54 wallet reference receiver"}])
        destination = receive["account_id"]
        secret = receive["master_seed"]
        funding = rpc("submit", [{"secret": "masterpassphrase", "tx_json": {
            "TransactionType": "Payment", "Account": master, "Destination": destination, "Amount": "100000000"}}])
        assert funding["engine_result"] == "tesSUCCESS"
        rpc("ledger_accept", [{}])
        trust = rpc("submit", [{"secret": secret, "tx_json": {"TransactionType": "TrustSet",
            "Account": destination, "LimitAmount": {"currency": "USD", "issuer": sender, "value": "10"}}}])
        assert trust["engine_result"] == "tesSUCCESS"
        rpc("ledger_accept", [{}])
        policy.xrpl_currency, policy.xrpl_issuer = "USD", sender
        intent = {**request, "destination": destination, "asset": "USD", "amount": "1.234567890123456", "tag": 1234}
        raw2, fee2, context2, details = lab.xrpl(rpc, intent, policy)
        second = firmware.sign(base_grant(3, raw2, fee2, context2))
        issued_hash = hashlib.sha512(b"TXN\x00" + second).digest()[:32].hex().upper()
        issued = rpc("submit", [{"tx_blob": second.hex().upper(), "fail_hard": True}])
        assert issued["engine_result"] == "tesSUCCESS" and issued["tx_json"]["hash"] == issued_hash, issued
        rpc("ledger_accept", [{}])
        second_result = rpc("tx", [{"transaction": issued_hash, "binary": False, "api_version": 1}])
        delivered = second_result["meta"]["delivered_amount"]
        assert second_result["validated"] and second_result["DestinationTag"] == 1234
        assert delivered == {"currency": "USD", "issuer": sender, "value": details["amount"]}
        return {"id": txid, "ledger": validated["ledger_index"], "recipient": recipient, "destinationTag": 0,
                "deliveredDrops": validated["meta"]["delivered_amount"], "validated": True,
                "issued": {"id": issued_hash, "ledger": second_result["ledger_index"], "amount": delivered}}
    except Exception:
        print(subprocess.run(["docker", "logs", name], capture_output=True, text=True).stderr[-2500:])
        raise
    finally:
        subprocess.run(["docker", "stop", "-t", "2", name], capture_output=True, check=False)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("chain", choices=("bitcoin", "solana", "xrpl"))
    parser.add_argument("--oracle", required=True, type=pathlib.Path)
    parser.add_argument("--bitcoind", type=pathlib.Path)
    parser.add_argument("--validator", type=pathlib.Path)
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="nu54-node-") as temp:
        print(json.dumps(globals()[args.chain](args, pathlib.Path(temp), FirmwareCore(args.oracle)), sort_keys=True))
