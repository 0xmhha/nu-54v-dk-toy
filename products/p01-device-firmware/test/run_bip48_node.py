#!/usr/bin/env python3
"""Sign an actual isolated Bitcoin Core 32.1 PSBTv2 with one P01 BIP48 key."""
import argparse
import base64
import ctypes
from decimal import Decimal
import hashlib
import hmac
import json
import pathlib
import re
import struct
import subprocess
import time
from types import SimpleNamespace
import tempfile

from chain_openssl_test import ORDER, secp
from run_wallet_chain_nodes import available_port, b58, daemon, eventual, lab
from test_bip48 import VECTOR, maps, sign


def counterpart_descriptor(descriptor):
    # Known synthetic second participant from the independent test vector.
    seed = hashlib.sha256(b"nu54/synthetic/bip48/cosigner/B").digest()
    node = hmac.digest(b"Bitcoin seed", seed, "sha512")
    scalar, code = int.from_bytes(node[:32], "big"), node[32:]
    for index in (48, 1, 0, 2):
        fingerprint = lab.hash160(secp(scalar).public_bytes())[:4]
        child = struct.pack(">I", 0x80000000 | index)
        node = hmac.digest(code, b"\x00" + scalar.to_bytes(32, "big") + child, "sha512")
        scalar, code = (scalar + int.from_bytes(node[:32], "big")) % ORDER, node[32:]
    serialized = bytes.fromhex("0435839404") + fingerprint + child + code + b"\x00" + scalar.to_bytes(32, "big")
    key = b58(serialized + lab.d256(serialized)[:4])
    unchecked = descriptor.split("#")[0]
    tagged = r"(\[" + VECTOR["fingerprints"][1] + r"/48h/1h/0h/2h\])tpub[1-9A-HJ-NP-Za-km-z]+"
    updated, count = re.subn(tagged, lambda match: match[1] + key, unchecked)
    assert count == 1
    return updated


def run(args):
    oracle = ctypes.CDLL(str(args.oracle))
    with tempfile.TemporaryDirectory(prefix="nu54-bip48-") as temp:
        scratch = pathlib.Path(temp)
        port = available_port()
        node = [args.bitcoind, "-regtest", "-datadir=" + temp, "-rpcport=" + str(port), "-networkactive=0",
                "-listen=0", "-dnsseed=0", "-rpcbind=127.0.0.1", "-rpcallowip=127.0.0.1",
                "-fallbackfee=0.00001", "-txindex=1"]
        with daemon(node, scratch / "node.log"):
            cookie = scratch / "regtest/.cookie"
            eventual(cookie.exists)
            endpoint = "http://127.0.0.1:" + str(port)
            auth = cookie.read_text().strip()

            def rpc(wallet=""):
                return lab.rpc_client(endpoint + ("/wallet/" + wallet if wallet else ""), auth, timeout=60)

            core = rpc()
            eventual(lambda: core("getblockchaininfo", [])["chain"] == "regtest")
            core("createwallet", ["miner"])
            miner = rpc("miner")
            target = miner("getnewaddress", [])
            miner("generatetoaddress", [101, target])
            for wallet, watch in (("watch", True), ("counterpart", False)):
                core("createwallet", [wallet, watch, True])
                records = []
                for internal, public in enumerate(VECTOR["descriptors"]):
                    descriptor = public if watch else counterpart_descriptor(public)
                    if not watch:
                        descriptor += "#" + core("getdescriptorinfo", [descriptor])["checksum"]
                    records.append({"desc": descriptor, "timestamp": "now", "active": True,
                                    "internal": bool(internal), "range": [0, 10]})
                assert all(row["success"] for row in rpc(wallet)("importdescriptors", [records]))
            watch = rpc("watch")
            receive = watch("getnewaddress", [])
            assert receive == VECTOR["receiveAndChangeAddresses"][0][0]
            funding = [miner("sendtoaddress", [receive, 0.6 if args.two_inputs else 1]) for _ in range(2 if args.two_inputs else 1)]
            miner("generatetoaddress", [1, target])
            coins = watch("listunspent", [1, 999999, [receive]])
            assert {coin["txid"] for coin in coins} == set(funding)
            authority = scratch / "authority.pem"
            subprocess.run(["openssl", "genpkey", "-algorithm", "EC", "-pkeyopt", "ec_paramgen_curve:P-256",
                            "-out", str(authority)], check=True, capture_output=True)
            scope = scratch / "policy.json"
            scope.write_text(json.dumps({"account": 0, "maxFee": 100000, "owner": 0,
                                         "descriptors": VECTOR["descriptors"]}))
            issuer = SimpleNamespace(bip48_policy=scope, review_dir=scratch, authority=authority,
                                     board="ab" * 32, generation=3, lease_revision=5,
                                     wall_start=int(time.time()), mono_start=time.monotonic())
            intent = {"board": issuer.board, "generation": "3", "leaseRevision": "5",
                      "anchor": core("getblockhash", [0])}
            network = 2 if args.signet_profile else 1
            approval_node, destination = watch, target
            if args.signet_profile:
                from wallet_chain_review import segwit
                issuer.commissioned_token_file = True
                intent.update(network=2, account=0, anchor=lab.COMMISSIONED[1])
                destination = segwit(bytes.fromhex(watch("validateaddress", [target])["scriptPubKey"]), "tb")

                def approval_node(method, params):
                    if method == "getblockchaininfo":
                        return {**watch(method, params), "chain": "signet"}
                    if method == "getblockhash" and params in ([0], [321295]):
                        return lab.COMMISSIONED[1].split(":")[int(params[0] == 321295)]
                    if method == "validateaddress" and params == [destination]:
                        return watch(method, [target])
                    if method == "walletcreatefundedpsbt":
                        return watch(method, [params[0], [{target:params[1][0][destination]}], *params[2:]])
                    return watch(method, params)

            policy = lab.bip48_request(issuer, approval_node, intent, False)
            grant = bytes.fromhex(lab.bip48_request(issuer, approval_node, {**intent, "recipient": destination, "amount": "0.99"}, True)["grant"])
            assert grant[2] == network and bytes.fromhex(policy["policy"])[2] == network
            assert grant[23:55] == lab.h256(bytes.fromhex(policy["policy"]))
            original = base64.b64encode(grant[217:]).decode()
            first = sign(oracle, base64.b64decode(original), network=network)
            if args.native_bridge:
                from wallet_chain_review import bip48_original_id
                unsigned_id = bip48_original_id(base64.b64decode(original))
                (args.native_bridge / "original-id").write_text(unsigned_id)
                (args.native_bridge / "original.psbt").write_text(original)
                partial = args.native_bridge / "native-partial.psbt"
                eventual(partial.exists, 180)
                assert base64.b64decode(partial.read_text(), validate=True) == first
            assert first and sign(oracle, first, network=network) is None
            own = base64.b64encode(first).decode()
            other = rpc("counterpart")("walletprocesspsbt", [original])["psbt"]
            assert not core("finalizepsbt", [own])["complete"] and not core("finalizepsbt", [other])["complete"]
            if args.two_inputs:
                def encode(entries):
                    return b"psbt\xff" + b"".join(b"".join(lab.kv(key,value) for key,value in fields.items()) + b"\0"
                                                   for fields in entries)
                mixed = maps(base64.b64decode(other))
                mixed[2] = {key:value for key,value in mixed[2].items() if key[0] != 2}
                assert sign(oracle, encode(mixed), network=network) is None
                duplicate = maps(base64.b64decode(original))
                duplicate[2] = duplicate[1]
                assert sign(oracle, encode(duplicate), network=network) is None
                wrong = maps(base64.b64decode(original))
                fp = next(key for key in wrong[2] if key[0] == 6)
                wrong[2][fp] = bytes([wrong[2][fp][0] ^ 1]) + wrong[2][fp][1:]
                assert sign(oracle, encode(wrong), network=network) is None
            # Pure Core combines the two independently produced partials.
            combined = core("combinepsbt", [[own, other]])
            final = core("finalizepsbt", [combined])
            assert final["complete"] and sign(oracle, base64.b64decode(combined), network=network) is None
            # The P01 normal import route can also accept the counterpart first.
            augmented = sign(oracle, base64.b64decode(other), network=network)
            assert augmented and core("finalizepsbt", [base64.b64encode(augmented).decode()]) == final
            assert core("testmempoolaccept", [[final["hex"]]])[0]["allowed"]
            txid = core("sendrawtransaction", [final["hex"]])
            block = miner("generatetoaddress", [6 if args.native_bridge else 1, target])[0]
            result = core("getrawtransaction", [txid, True])
            assert result["blockhash"] == block and len(result["vin"]) == len(funding) and all(len(vin["txinwitness"]) == 4 for vin in result["vin"])
            assert result["vout"][0]["scriptPubKey"]["address"] == target
            assert result["vout"][1]["scriptPubKey"]["address"] == VECTOR["receiveAndChangeAddresses"][1][0]
            from wallet_chain_review import review_bip48
            review = review_bip48(grant)
            assert result["vsize"] <= review["maximumSignedVbytes"]
            assert Decimal(review["minimumFeeRateSatPerVbyte"]) <= Decimal(review["feeSatoshis"]) / result["vsize"]
            if args.native_bridge:
                assert txid == unsigned_id
                (args.native_bridge / "confirmed.json").write_text(json.dumps({"txid":txid,"confirmations":result["confirmations"]}))
            if args.proof_json:
                args.proof_json.write_text(json.dumps({"unsignedPSBT":original,"partialPSBTs":[own,other],
                    "descriptors":VECTOR["descriptors"],"id":txid},indent=2)+"\n")
            return {"node": "disconnected regtest", "approvalProfile": "Signet fixture" if args.signet_profile else "regtest", "threshold": "2 of 3", "id": txid, "block": block,
                    "inputs":len(result["vin"]), "singleComplete": False, "bothComplete": True, "counterpartImportMatches": True,
                    "feeSatoshis":review["feeSatoshis"], "signedVsize":result["vsize"], "maximumSignedVsize":review["maximumSignedVbytes"]}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--oracle", type=pathlib.Path, required=True)
    parser.add_argument("--proof-json",type=pathlib.Path)
    parser.add_argument("--native-bridge", type=pathlib.Path)
    parser.add_argument("--two-inputs", action="store_true")
    parser.add_argument("--signet-profile", action="store_true")
    parser.add_argument("--bitcoind", type=pathlib.Path, required=True)
    print(json.dumps(run(parser.parse_args()), sort_keys=True))
