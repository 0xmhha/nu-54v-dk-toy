"""Installed W17 profile with an independent, synthetic BIP48 signer."""
import argparse
import base64
import ctypes
from decimal import Decimal
import hmac
import json
import os
from pathlib import Path
import signal
import ssl
import struct
import tempfile
import time

from wallet_connect_spend import BOARD, P01, TOKEN, fixture
from run_wallet_chain_nodes import lab
from test_bip48 import VECTOR, sign
from wallet_chain_review import review_bip48


def main(args):
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(args.cert, args.key)
    cert = base64.b64encode(ssl.PEM_cert_to_DER_cert(args.cert.read_text())).decode()
    anchor = lab.COMMISSIONED[1]
    endpoint = "https://wallet.localhost:18449/bitcoin"
    token = hmac.digest(bytes.fromhex(TOKEN), ("N54P\n" + endpoint).encode(), "sha256").hex()
    sample = json.loads((P01 / "test/bip48_two_input_vector.json").read_text()) if args.wrong_change else VECTOR
    psbt = base64.b64decode((args.bridge / "original.psbt").read_text() if args.bridge else sample["unsignedPSBT"],validate=True)
    if args.wrong_change:
        from test_bip48 import maps
        fields = maps(psbt)
        for key in fields[-1]:
            if key[0] == 2:
                path = fields[-1][key]
                fields[-1][key] = path[:20] + bytes(4) + path[24:]
        psbt = b"psbt\xff" + b"".join(b"".join(lab.kv(k, v) for k, v in row.items()) + b"\0" for row in fields)
        if args.document_out:
            if args.document_out.name != args.document or args.document_out.suffix != ".psbt":
                raise ValueError("expected a dedicated synthetic PSBT path")
            args.document_out.write_bytes(psbt)
    info = review_bip48(bytes([18, 1, 2])+bytes(12)+struct.pack(">Q",100000)+bytes(194)+psbt)
    assert len(info["allOutputs"]) == 2
    recipient = info["allOutputs"][0]["address"]
    amount = str(Decimal(info["allOutputs"][0]["satoshis"])/10**8)
    case = {"register":0,"signatures":0,"destination":recipient,"amount":amount,"bridge":bool(args.bridge),"inputs":len(info["inputs"]),
            "document":args.document,"wrongChange":args.wrong_change,"reviewOnly":args.review_only,"reviewDetails":info}
    if args.bridge:
        case["original"] = (args.bridge / "original-id").read_text().strip()
        (args.bridge / "expected-review.json").write_text(json.dumps(info))
    oracle = ctypes.CDLL(str(P01 / "out/host/libwallet_chain_node_oracle.so"))
    latest = None

    with tempfile.TemporaryDirectory(prefix="w17-native.") as directory:
        scratch = Path(directory)
        scope_path = scratch / "policy.json"
        scope_path.write_text(json.dumps({"account":0,"maxFee":100000,"owner":0,"descriptors":VECTOR["descriptors"]}))
        policy, descriptors = lab.bip48_configuration(scope_path, 2)
        pin = lab.h256(policy)

        def board(wire):
            if wire and wire[0] in (17,18,19):
                print("W17 board stage", wire[0], flush=True)
            if wire == bytes([19,1,2,0,0,0,0]):
                return bytes([19]) + (pin if case["register"] else bytes(32)) + bytes(35)
            if wire == policy and not case["register"]:
                case["register"] = 1
                return bytes([17]) + pin
            if wire == latest and wire[:3] == bytes([18,1,2]) and case["register"] and not case["signatures"]:
                if args.review_only:
                    raise ValueError("inspection-only fixture cannot sign")
                if int.from_bytes(wire[7:15],"big") <= int(time.time()):
                    raise ValueError("Approval expired")
                partial = sign(oracle, wire[217:], policy[17:], network=2)
                if not partial:
                    raise ValueError("Host multisig refusal")
                case["signatures"] = 1
                if args.bridge:
                    target = args.bridge / "native-partial.psbt"
                    scratch_partial = target.with_suffix(".scratch")
                    scratch_partial.write_bytes(base64.b64encode(partial))
                    scratch_partial.rename(target)
                return partial
            raise ValueError("Invalid or duplicate W17 request")

        def exchange(packet,key):
            nonlocal latest
            if not hmac.compare_digest(packet.get("walletToken",""),token):
                raise ValueError("Wrong W17 route credential")
            method, params = packet["method"], packet["params"]
            if method in ("getblockchaininfo","getblockhash","nu54_bip48Policy","nu54_bip48Prepare","getrawtransaction"):
                print("W17 ledger stage", method, flush=True)
            if method == "getblockchaininfo":
                result = {"chain":"signet"}
            elif method == "getblockhash" and params in ([0],[321295]):
                result = anchor.split(":")[int(params[0]==321295)]
            elif method == "getrawtransaction" and args.bridge:
                source = args.bridge / "confirmed.json"
                if source.exists():
                    result = json.loads(source.read_text())
                    if params != [result["txid"],True]:
                        raise ValueError("W17 original transaction ID mismatch")
                    case["requeried"] = True
                else:
                    result = None
            elif method in ("nu54_bip48Policy","nu54_bip48Prepare"):
                request, = params
                if any(request.get(name)!=value for name,value in
                       {"anchor":anchor,"board":BOARD,"generation":"1","leaseRevision":"1","account":0,"network":2}.items()):
                    raise ValueError("W17 enrollment mismatch")
                if method == "nu54_bip48Policy":
                    result = {"policy":policy.hex(),"descriptors":descriptors}
                else:
                    if args.document:
                        if request.get("counterpart") != base64.b64encode(psbt).decode():
                            raise ValueError("W17 imported document changed")
                    elif request.get("recipient")!=recipient or Decimal(request.get("amount","0"))!=Decimal(amount):
                        raise ValueError("W17 raw intent differed")
                    header = struct.pack(">BBBIQQ",18,1,2,0,int(time.time())+55,100000) + pin + bytes(58) + lab.h256(anchor.encode()) + struct.pack(">Q",1)
                    latest = header + lab.sign(key,b"N54M"+bytes.fromhex(BOARD)+struct.pack(">Q",1)+header+lab.h256(psbt)) + psbt
                    result = {"grant":latest.hex()}
            else:
                raise ValueError("No payment-network egress in this fixture")
            return {"jsonrpc":"2.0","id":1,"result":result}

        def refuse(*_):
            raise ValueError("W17 has no sending RPC")

        opts = {"cert":cert,"relay":"wss://127.0.0.1:18449","case":case,"port":args.listen,"joint_board":board,"chain_rpc":exchange,
                "chain_route":"/bitcoin","commissioned":{"chain":1,"profile":"bitcoin-signet","anchor":anchor,"rpc":endpoint}}
        server, worker, _, _ = fixture(scratch,refuse,"0x"+"12"*32,None,None,context,opts)
        print("W17 fixture ready",flush=True)
        try:
            while True:
                time.sleep(1)
        finally:
            server.shutdown()
            server.server_close()
            worker.join()
            print({"register":case["register"],"signatures":case["signatures"],"requeried":case.get("requeried",False)},flush=True)


def interrupt(_signal,_frame):
    raise KeyboardInterrupt


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("cert", type=Path)
    parser.add_argument("key", type=Path)
    parser.add_argument("--bridge",type=Path)
    parser.add_argument("--document")
    parser.add_argument("--wrong-change",action="store_true")
    parser.add_argument("--document-out",type=Path)
    parser.add_argument("--review-only",action="store_true")
    parser.add_argument("--listen",type=int,default=18449)
    os.umask(0o077)
    signal.signal(signal.SIGTERM,interrupt)
    try:
        main(parser.parse_args())
    except KeyboardInterrupt:
        pass
