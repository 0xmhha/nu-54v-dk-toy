"""Synthetic RPC checks for the normal wallet registry and fee/authorization service."""

import hashlib
from pathlib import Path
import sys
import tempfile
import unittest

from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec, utils

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "scripts"))
from evm_service import WalletAuthority, ZERO
from evm_transfer import ORACLE, keccak256, parse, parse_siwe_capsule
from walletconnect_peer import issue as issue_peer


class WalletService(unittest.TestCase):
    def test_base_quote_binds_registry_and_canonical_fee_inputs(self):
        now = 1800
        genesis, tip = "0x" + "aa" * 32, "0x" + "bb" * 32
        token = "c4" * 32
        board = "1b" * 32
        sender, recipient = "0x" + "11" * 20, "0x" + "22" * 20
        calls = []
        profile = {
            "device": board, "sender": sender, "generation": 3, "leaseRevision": "2", "expires": now+1800,
            "networks": [{"chain": "84532", "genesis": genesis, "rpc": "https://rpc.example.test", "feeLimit": "800000",
                          "assets": [{"address": ZERO, "symbol": "ETH", "decimals": 18}]}],
        }

        def rpc(method, args):
            calls.append((method, args))
            if method == "eth_getBlockByNumber":
                return {"hash": genesis if args[0] == "0x0" else tip, "baseFeePerGas": "0x5"}
            if method == "eth_call":
                self.assertEqual(args[0]["to"], ORACLE)
                self.assertEqual(args[1], {"blockHash": tip, "requireCanonical": True})
                return "0x" + f"{123:064x}"
            return {"eth_chainId": "0x14a34", "eth_getTransactionCount": "0x3",
                    "eth_maxPriorityFeePerGas": "0x2", "eth_getCode": "0x", "eth_estimateGas": "0x5208"}[method]

        def envelope(method, **fields):
            return {"jsonrpc": "2.0", "id": 1, "method": method, "params": [{"token": token, **fields}]}

        with tempfile.TemporaryDirectory(prefix="nu54-evm-service-") as name:
            path = Path(name) / "authority.pem"
            path.touch(mode=0o600)
            issuer = ec.generate_private_key(ec.SECP256R1())
            path.write_bytes(issuer.private_bytes(serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8, serialization.NoEncryption()))
            panel = "ab" * 32
            service = WalletAuthority(profile, path, hashlib.sha256(token.encode()).hexdigest(), lambda _: rpc, lambda: now,
                                      hashlib.sha256(panel.encode()).hexdigest())
            registry = service.handle(envelope("nu54_walletRegistry"))
            self.assertEqual(len(registry["proof"]), 128)
            with self.assertRaises(ValueError):
                service.handle(envelope("nu54_walletRegistry", token="ac"*32))
            quoted = bytes.fromhex(service.handle(envelope("nu54_walletQuote", chain="84532", asset=ZERO, amount="9", recipient=recipient)))
            with self.assertRaises(ValueError):
                service.handle(envelope("nu54_walletDisplay"))
            self.assertEqual(service.handle(envelope("nu54_walletDisplay", token=panel)), quoted.hex())
            terms = parse(quoted, issuer.public_key(), bytes.fromhex(board), bytes.fromhex(sender[2:]), 3, now)
            self.assertEqual((terms[0], terms[1], terms[3], terms[4].hex(), terms[5], terms[6], terms[7].hex()),
                             (84532, 3, 9, recipient[2:], 252246, 246, genesis[2:]))
            self.assertEqual(sum(method == "eth_call" for method, _ in calls), 2)
            with self.assertRaises(Exception):
                parse(quoted[:53] + bytes.fromhex("cc"*32) + quoted[85:], issuer.public_key(),
                      bytes.fromhex(board), bytes.fromhex(sender[2:]), 3, now)
            origin, pairing, session, peer = "https://commerce.example", "44"*32, "55"*32, "66"*32
            profile["dapp"] = {"origin":origin,"chain":"84532"}
            profile["walletConnect"] = {"relay":"wss://relay.example"}
            profile["walletConnect"]["peer"] = issue_peer(profile,path,pairing,peer,now)
            external = envelope("nu54_walletExternalSpend", chain="84532", asset=ZERO, recipient=recipient, amount="9",
                                origin=origin,pairingTopic=pairing,sessionTopic=session,peerKey=peer)
            wire = bytes.fromhex(service.handle(external))
            self.assertEqual(wire[0],20)
            self.assertEqual(wire[165:261].hex(),pairing+session+peer)
            self.assertEqual(parse(wire,issuer.public_key(),bytes.fromhex(board),bytes.fromhex(sender[2:]),3,now),terms)
            for offset in (165,197,229,270):
                modified = bytearray(wire)
                modified[offset] ^= 1
                with self.assertRaises(Exception):
                    parse(modified,issuer.public_key(),bytes.fromhex(board),bytes.fromhex(sender[2:]),3,now)
            for method, parameters in (
                ("nu54_walletQuote", {"chain":"84532", "asset":ZERO, "recipient":recipient, "amount":"9"}),
                ("nu54_walletExternalSpend", external["params"][0]),
            ):
                for extra in (
                    {"data":"0x095ea7b3"+"00"*12+recipient[2:]+"00"*31+"05"},
                    {"data":"0x095ea7b3"+"00"*12+recipient[2:]+"ff"*32},
                    {"approval":True},
                ):
                    with self.assertRaisesRegex(ValueError, "Unexpected transaction fields"):
                        service.handle(envelope(method, **parameters, **extra))
            with self.assertRaisesRegex(ValueError, "Unsupported wallet authority operation"):
                service.handle(envelope("nu54_walletExternalAllowance", **external["params"][0]))
            for field in ("peerKey","origin","pairingTopic"):
                switched = {**external,"params":[{**external["params"][0],field:"77"*32}]}
                with self.assertRaises(ValueError):
                    service.handle(switched)
            asset = "0x" + "80" * 20
            profile["networks"][0]["assets"].append({"address":asset,"symbol":"DEMO","decimals":6})
            profile["dapp"]["allowance"] = {"token":asset,"spender":recipient,"maximum":"5000000"}
            approval = dict(chain="84532", asset=asset, recipient=recipient, amount="5000000",
                            origin=origin, sessionTopic=session, sessionExpires=now + 30, transport="browser")
            def quote(parameters):
                return bytes.fromhex(service.handle(envelope("nu54_walletDappAllowance", **parameters)))
            wire = quote(approval)
            self.assertEqual((wire[0], int.from_bytes(wire[1:9], "big")), (36, now + 30))
            self.assertEqual(parse(wire, issuer.public_key(), bytes.fromhex(board), bytes.fromhex(sender[2:]),3,now)[2:5],
                             (bytes.fromhex(asset[2:]), 5000000, bytes.fromhex(recipient[2:])))
            self.assertEqual(quote({**approval, "transport":"walletconnect", "pairingTopic":pairing, "peerKey":peer})[165],1)
            for fields in ({"amount":"0"}, {"amount":"5000001"}, {"asset":ZERO}, {"recipient":sender},
                           {"origin":"https://bad.example"}, {"sessionExpires":now}, {"sessionTopic":"0"*64}):
                with self.assertRaises(ValueError):
                    quote({**approval, **fields})
            profile["networks"][0]["genesis"] = "0x" + "cc"*32
            with self.assertRaises(ValueError):
                service.handle(envelope("nu54_walletQuote", chain="84532", asset=ZERO, amount="9", recipient=recipient))

    def test_siwe_issuer_rejects_cross_origin_changed_expired_and_consumed_challenge(self):
        origin, token, board = "https://demo.example", "ab" * 32, "12" * 32
        clock = [1800]
        wallet = ec.generate_private_key(ec.SECP256K1())
        public = wallet.public_key().public_bytes(serialization.Encoding.X962, serialization.PublicFormat.UncompressedPoint)
        sender = "0x" + keccak256(public[1:])[12:].hex()
        profile = {"device":board,"sender":sender,"generation":3,"leaseRevision":"2","expires":2200,
                   "dapp":{"origin":origin,"chain":"11155111","publicKey":public.hex()},
                   "networks":[{"chain":"11155111","genesis":"0x"+"aa"*32}]}

        def ask(service, name, transport=None, **arguments):
            return service.handle({"jsonrpc":"2.0","id":1,"method":name,"params":[arguments]},transport)

        with tempfile.TemporaryDirectory(prefix="nu54-siwe-service-") as name:
            path = Path(name) / "authority.pem"
            path.touch(mode=0o600)
            issuer = ec.generate_private_key(ec.SECP256R1())
            path.write_bytes(issuer.private_bytes(serialization.Encoding.PEM,serialization.PrivateFormat.PKCS8,serialization.NoEncryption()))
            service = WalletAuthority(profile,path,hashlib.sha256(token.encode()).hexdigest(),clock=lambda:clock[0])
            with self.assertRaises(ValueError):
                ask(service,"nu54_siweChallenge","https://other.example",account=sender)
            issued = ask(service,"nu54_siweChallenge",origin,account=sender)
            capsule = ask(service,"nu54_walletLogin",token=token,origin=origin,message=issued["message"])
            with self.assertRaisesRegex(ValueError, "already reviewed"):
                ask(service,"nu54_walletLogin",token=token,origin=origin,message=issued["message"])
            wire = bytes.fromhex(capsule["wire"])
            self.assertEqual(wire[0],15)
            parse_siwe_capsule(wire,issuer.public_key(),bytes.fromhex(board),bytes.fromhex(sender[2:]),3,clock[0])
            with self.assertRaises(ValueError):
                parse_siwe_capsule(bytes([13])+wire[1:],issuer.public_key(),bytes.fromhex(board),bytes.fromhex(sender[2:]),3,clock[0])
            with self.assertRaises(ValueError):
                ask(service,"nu54_walletLogin",token=token,origin=origin,message=issued["message"].replace("11155111","8283"))
            raw=issued["message"].encode()
            digest=keccak256(b"\x19Ethereum Signed Message:\n"+str(len(raw)).encode()+raw)
            der=wallet.sign(digest,ec.ECDSA(utils.Prehashed(hashes.SHA256())))
            r, s=utils.decode_dss_signature(der)
            signature=(r.to_bytes(32,"big")+s.to_bytes(32,"big")+bytes([27])).hex()
            proof={"account":sender,"nonce":issued["nonce"],"message":issued["message"],"signature":signature}
            self.assertEqual(ask(service,"nu54_siweVerify",origin,**proof),{"verified":True})
            with self.assertRaises(ValueError):
                ask(service,"nu54_siweVerify",origin,**proof)
            later=ask(service,"nu54_siweChallenge",origin,account=sender)
            clock[0]=later["expires"]
            with self.assertRaises(ValueError):
                ask(service,"nu54_walletLogin",token=token,origin=origin,message=later["message"])


if __name__ == "__main__":
    unittest.main()
