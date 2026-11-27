"""Offline gateway boundary for the three commissioned wallet profiles."""
import ctypes
import hashlib
import hmac
import os
import json
import sys
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))
import wallet_chain_lab as gateway

VECTORS = ROOT.parent / "p02-user-app/test/fixtures/w28-local-chains.json"


class CommissionedChains(unittest.TestCase):
    def test_real_issuer_rejects_reserved_qa_dns(self):
        profile = {"chain":1,"profile":"bitcoin-signet","anchor":gateway.COMMISSIONED[1],
                   "rpc":"https://gateway.example/bitcoin"}
        gateway.validate_commissioned_profiles([profile])
        for endpoint in ("https://wallet.localhost:18449/bitcoin","https://127.0.0.1/bitcoin"):
            with self.assertRaises(ValueError):
                gateway.validate_commissioned_profiles([{**profile,"rpc":endpoint}])

    def test_foreign_checkpoint_genesis_and_unvalidated_ledger(self):
        for chain, responses in [
            (1, [{"chain": "signet"}, gateway.COMMISSIONED[1].split(":")[0], "wrong"]),
            (2, ["wrong"]),
            (3, [{"info":{"network_id":1}}, {"validated":False}]),
        ]:
            with self.subTest(chain=chain):
                values = iter(responses)
                with self.assertRaises(ValueError):
                    gateway.verify_commissioned_node(chain, lambda *_: next(values))

    def test_xrp_xaddress_tag_zero_matches_classic_payment(self):
        from wallet_chain_review import b58
        row = json.loads(VECTORS.read_text())[2]
        classic = row["destination"]
        payload = bytes((4, 147)) + gateway.xrpl_id(classic) + bytes((1,)) + bytes(8)
        address = b58(payload + gateway.d256(payload)[:4], gateway.RIPPLE)
        results = {"server_info":{"info":{"network_id":1}},
                   "ledger":{"validated":True,"ledger_hash":gateway.COMMISSIONED[3].split(":")[1],"ledger_index":22470370},
                   "account_info":{"account_data":{"Sequence":3,"Balance":"20000000","OwnerCount":0}},
                   "ledger_current":{"ledger_current_index":22470380},"fee":{"drops":{"open_ledger_fee":"10"}},
                   "server_state":{"state":{"validated_ledger":{"reserve_base":1000000,"reserve_inc":200000}}}}
        request = {"network":2,"anchor":gateway.COMMISSIONED[3],"publicKey":row["publicKey"],"sender":row["address"],
                   "destination":classic,"amount":"5","tag":0}
        rpc = lambda method, _: results[method]
        xaddress = gateway.xrpl(rpc, {**request,"destination":address,"tag":None}, None)
        self.assertEqual(xaddress, gateway.xrpl(rpc, request, None))
        self.assertEqual(xaddress[3]["destinationTag"], 0)
        with self.assertRaises(ValueError):
            gateway.xrpl(rpc, {**request,"destination":address,"tag":1}, None)
        with self.assertRaises(ValueError):
            gateway.xrpl_destination(address, None, False)

    def test_xrp_commissioned_board_rejects_legacy_sender(self):
        from chain_openssl_test import ORDER, secp
        from run_wallet_chain_nodes import B, P, SEED, FirmwareCore, buffer
        oracle = Path(os.environ.get("NU54_WALLET_CHAIN_HOST", ROOT / "out/host/libwallet_chain_node_oracle.so"))
        firmware = FirmwareCore(oracle)
        function = firmware.lib.chain_host_public_network
        function.argtypes = [P, B, B, ctypes.c_uint32, ctypes.c_uint32, P, P, P]
        commissioned, account, fingerprint = (B * 33)(), (B * 20)(), (B * 4)()
        def openssl_public(secret):
            key = secp(secret)
            try:
                return key.public_bytes()
            finally:
                key.state.cleanup()

        for profile_account in (0, 99):
            self.assertEqual(function(buffer(SEED), 3, 2, profile_account, 0, commissioned, account, fingerprint), 0)
            master = hmac.digest(b"Bitcoin seed", SEED, "sha512")
            secret, chain_code = int.from_bytes(master[:32], "big"), master[32:]
            for index in (2**31 + 44, 2**31 + 144, 2**31 + 100 + profile_account, 0, 0):
                key = (b"\0" + secret.to_bytes(32, "big")) if index >= 2**31 else openssl_public(secret)
                child = hmac.digest(chain_code, key + index.to_bytes(4, "big"), "sha512")
                secret = (secret + int.from_bytes(child[:32], "big")) % ORDER
                chain_code = child[32:]
            self.assertEqual(bytes(commissioned), openssl_public(secret))
            self.assertNotEqual(bytes(commissioned), firmware.account(3, profile_account)[0])

        row = json.loads(VECTORS.read_text())[2]
        grant = bytearray.fromhex(row["reviewed"])
        self.assertEqual(firmware.sign(grant), bytes.fromhex(row["signed"]))
        grant[2] = 2
        self.assertIsNone(firmware.sign(grant, reject=True))
        self.assertEqual(function(buffer(SEED), 3, 2, 0, 0, commissioned, account, fingerprint), 0)
        grant[-77:-44], grant[-42:-22] = bytes(commissioned), bytes(account)
        signed = firmware.sign(grant)
        self.assertNotEqual(signed, bytes.fromhex(row["signed"]))
        self.assertTrue(signed.endswith(bytes(grant[-44:])))

    def test_signed_bytes_match_original_before_offline_node_submission(self):
        for row, method in zip(json.loads(VECTORS.read_text()), ("sendrawtransaction", "sendTransaction", "submit")):
            with self.subTest(method=method), tempfile.TemporaryDirectory() as directory:
                chain, txid = row["chain"], row["id"]
                grant = bytearray.fromhex(row["reviewed"])
                grant[2] = 2
                grant[7:15] = (150).to_bytes(8, "big")
                grant[113:145] = gateway.h256(gateway.COMMISSIONED[chain].encode())
                grant[145:153] = (1).to_bytes(8, "big")
                digest = hashlib.sha256(grant).hexdigest()
                path = Path(directory)
                (path / (digest + ".json")).write_text(json.dumps({"grant":grant.hex()}))
                args = SimpleNamespace(review_dir=path, lease_revision=1, commissioned_expires=200)
                request = {"grant":digest, "id":txid, "signed":row["signed"]}
                seen = []

                def node(operation, _params):
                    seen.append(operation)
                    if operation == "testmempoolaccept":
                        return [{"allowed":True, "txid":txid}]
                    self.assertEqual(operation, method)
                    return {"tx_json":{"hash":txid}} if chain == 3 else txid

                changed = request["signed"][:-2] + "ff"
                with patch.object(gateway, "trusted_wall", return_value=101):
                    with self.assertRaises(ValueError):
                        gateway.submit_commissioned(args, chain, node, {**request,"signed":changed})
                    self.assertEqual(seen, [])
                    self.assertEqual(gateway.submit_commissioned(args, chain, node, request), txid)
                    self.assertEqual(seen.count(method), 1)
                with patch.object(gateway, "trusted_wall", return_value=150):
                    with self.assertRaises(ValueError):
                        gateway.submit_commissioned(args, chain, node, request)
                    self.assertEqual(seen.count(method), 1)


if __name__ == "__main__":
    unittest.main()
