"""Run only against a fresh disposable Anvil. Uses the standard Anvil test account."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import time
from urllib.request import Request, urlopen

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import ec

TEST = Path(__file__).resolve().parent
sys.path.insert(0, str(TEST.parent / "scripts"))
from evm_service import WalletAuthority, rlp
from evm_transfer import allowance_context, external_context, fields, keccak256, parse
from walletconnect_peer import issue as issue_peer

ROOT = TEST.parents[2]
PORT = 18761
RPC = f"http://127.0.0.1:{PORT}"
OWNER = "0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266"
KEY = "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
SPENDER = "0x" + "ab" * 20
BOARD, TOKEN = "11" * 32, "aa" * 32


def rpc(method, args):
    packet = {"jsonrpc":"2.0","id":1,"method":method,"params":args}
    response = json.load(urlopen(Request(RPC, json.dumps(packet).encode(), {"Content-Type":"application/json"}), timeout=2))
    if "error" in response:
        raise ValueError(response["error"])
    return response["result"]


def cast(*args):
    return subprocess.check_output(["cast", *args], text=True).strip()


def gateway_sign(gateway, external, public, board, owner, generation, now):
    start = allowance_context(external)[0] if external[0] == 36 else external_context(external)[0]
    sender = bytes.fromhex(owner[2:])
    device = bytes.fromhex(board)
    terms = parse(external,public,device,sender,generation,now)
    statement = b"N54V"+device+generation.to_bytes(8,"big")+sender+external[:101]+keccak256(external[start:])+external[165:start]
    raw = subprocess.check_output([str(gateway),external.hex(),statement.hex(),external[101:165].hex(),board,owner[2:],
      KEY[2:],str(now),str(generation)],text=True).strip()
    return terms,raw


def main(binary):
    asset = ROOT / "products/p06-stablenet-contracts/out/TestUSDC.sol/TestUSDC.json"
    with tempfile.TemporaryDirectory() as directory, open(Path(directory) / "anvil.log", "w+") as log:
        node = subprocess.Popen(["anvil","--host","127.0.0.1","--port",str(PORT),"--chain-id","11155111"], stdout=log, stderr=log)
        try:
            for _ in range(80):
                if node.poll() is not None:
                    raise RuntimeError("Disposable Anvil failed to bind")
                try:
                    assert rpc("eth_chainId",[]) == hex(11155111)
                    break
                except OSError:
                    time.sleep(0.05)
            else:
                raise RuntimeError("Disposable Anvil failed to start")
            opts = ("--rpc-url",RPC,"--private-key",KEY,"--json")
            deployment = json.loads(cast("send",*opts,"--create",json.loads(asset.read_text())["bytecode"]["object"],"constructor(address)",OWNER))
            contract = deployment["contractAddress"].lower()
            for amount in ("21","42"):
                cast("send",*opts,contract,"approve(address,uint256)",SPENDER,amount)
            allowance_call = {"to":contract,"data":"0xdd62ed3e"+OWNER[2:].rjust(64,"0")+SPENDER[2:].rjust(64,"0")}
            rpc("anvil_mine",["0x80"])
            assert int(rpc("eth_call",[allowance_call,"finalized"]),16)==42
            signer = ec.generate_private_key(ec.SECP256R1())
            path=Path(directory) / "authority.pem"
            path.touch(mode=0o600)
            path.write_bytes(signer.private_bytes(serialization.Encoding.PEM,serialization.PrivateFormat.PKCS8,serialization.NoEncryption()))
            now=int(time.time())
            genesis=rpc("eth_getBlockByNumber",["0x0",False])["hash"]
            profile={"device":BOARD,"sender":OWNER,"generation":1,"leaseRevision":"1","expires":now+120,
                     "networks":[{"chain":"11155111","rpc":RPC,"genesis":genesis,"feeLimit":str(10**17),"assets":[{"address":contract,"symbol":"DEMO","decimals":6}]}]}
            service=WalletAuthority(profile,path,hashlib.sha256(TOKEN.encode()).hexdigest(),lambda _:rpc,lambda:now)
            packet={"jsonrpc":"2.0","id":1,"method":"nu54_walletRevokeAllowance","params":[{"token":TOKEN,"chain":"11155111","asset":contract,"recipient":SPENDER}]}
            envelope=bytes.fromhex(service.handle(packet))
            parsed=parse(envelope,signer.public_key(),bytes.fromhex(BOARD),bytes.fromhex(OWNER[2:]),1,now)
            assert envelope[0]==14 and parsed[3]==0 and parsed[4].hex()==SPENDER[2:]
            raw=envelope[165:]
            digest=keccak256(raw)
            signature=bytes.fromhex(cast("wallet","sign","--private-key",KEY,"--no-hash","0x"+digest.hex())[2:])
            encoded=[rlp(value,group) if not group else rlp([],True) for value,group in fields(fields(raw[1:])[0][0])]
            signed_tx=b"\x02"+rlp(encoded+[rlp(signature[64]-27),rlp(int.from_bytes(signature[:32],"big")),rlp(int.from_bytes(signature[32:64],"big"))],True)
            subprocess.run([binary,"11155111",str(parsed[5]),"0",contract[2:],raw.hex(),digest.hex(),signature.hex(),signed_tx.hex(),"revoke"],check=True)
            gateway = Path(binary).with_name("test_device_evm")
            reviewed, board_signed = gateway_sign(gateway, envelope, signer.public_key(), BOARD, OWNER, 1, now)
            assert reviewed[2].hex() == contract[2:] and reviewed[3] == 0 and reviewed[4].hex() == SPENDER[2:]
            assert board_signed == signed_tx.hex()
            tx=rpc("eth_sendRawTransaction",["0x"+board_signed])
            assert tx=="0x"+keccak256(signed_tx).hex()
            rpc("anvil_mine",["0x1"])
            assert rpc("eth_getTransactionReceipt",[tx])["status"]=="0x1"
            rpc("anvil_mine",["0x80"])
            assert int(rpc("eth_call",[allowance_call,"finalized"]),16)==0
            print("Disposable Anvil: finalized ERC20 allowance 42 → 0 using issuer draft, independent display proof and one C gateway signature")
            cast("send", *opts, contract, "mint(address,uint256)", OWNER, "100")
            rpc("anvil_mine", ["0x80"])
            origin, pairing, topic, pubkey = "https://independent.example", "44"*32, "55"*32, "66"*32
            profile["dapp"] = {"origin":origin,"chain":"11155111",
                               "allowance":{"token":contract,"spender":SPENDER,"maximum":"10"}}
            profile["walletConnect"] = {"relay":"wss://relay.example","peer":None}
            profile["walletConnect"]["peer"] = issue_peer(profile,path,pairing,pubkey,now)
            approval = {"token":TOKEN,"chain":"11155111","asset":contract,"recipient":SPENDER,
                        "amount":"9","origin":origin,"sessionTopic":topic,"sessionExpires":now+45,"transport":"browser"}

            def allowance_quote(request):
                return bytes.fromhex(service.handle({"jsonrpc":"2.0","id":1,"method":"nu54_walletDappAllowance","params":[request]}))

            finite = allowance_quote(approval)
            terms, signed_finite = gateway_sign(gateway,finite,signer.public_key(),BOARD,OWNER,1,now)
            assert terms[3] == 9 and terms[4].hex() == SPENDER[2:]
            assert allowance_context(finite)[1][1:] == (10,6)
            def balances():
                return [int(rpc("eth_call", [{"to":contract, "data":"0x70a08231"+account[2:].rjust(64,"0")},
                                              "finalized"]), 16) for account in (OWNER, SPENDER)]

            assert balances() == [100, 0]
            tx = rpc("eth_sendRawTransaction", ["0x"+signed_finite])
            assert tx == "0x"+keccak256(bytes.fromhex(signed_finite)).hex()
            rpc("anvil_mine", ["0x80"])
            receipt = rpc("eth_getTransactionReceipt", [tx])
            assert receipt["status"] == "0x1"
            assert [log["topics"][0] for log in receipt["logs"]] == ["0x"+keccak256(b"Approval(address,address,uint256)").hex()]
            assert int(rpc("eth_call", [allowance_call, "finalized"]), 16) == 9
            assert balances() == [100, 0]
            print("Disposable Anvil: normal C gateway finalized browser allowance 0 → 9 without transferring ERC-20 balance")
            wc = allowance_quote({**approval, "amount":"5", "transport":"walletconnect",
                                  "pairingTopic":pairing, "peerKey":pubkey})
            terms, wc_signed = gateway_sign(gateway,wc,signer.public_key(),BOARD,OWNER,1,now)
            assert terms[3] == 5 and allowance_context(wc)[2][1:] == tuple(bytes.fromhex(i) for i in (pairing,topic,pubkey))
            tx = rpc("eth_sendRawTransaction", ["0x"+wc_signed])
            assert tx == "0x"+keccak256(bytes.fromhex(wc_signed)).hex()
            rpc("anvil_mine", ["0x80"])
            assert rpc("eth_getTransactionReceipt", [tx])["status"] == "0x1"
            assert int(rpc("eth_call", [allowance_call, "finalized"]), 16) == 5
            assert balances() == [100, 0]
            print("Disposable Anvil: normal C gateway finalized WalletConnect allowance 9 → 5 with the same owner and spender")
            profile["networks"][0]["assets"].append({"address":"0x"+"00"*20})
            transfer = {"token":TOKEN,"chain":"11155111","asset":"0x"+"00"*20,"recipient":SPENDER,"amount":"9",
                        "origin":origin,"pairingTopic":pairing,"sessionTopic":topic,"peerKey":pubkey}
            external = bytes.fromhex(service.handle({"jsonrpc":"2.0","id":1,"method":"nu54_walletExternalSpend","params":[transfer]}))
            _, context = external_context(external)
            assert context[0] == origin
            terms,signed_transfer = gateway_sign(gateway,external,signer.public_key(),BOARD,OWNER,1,now)
            assert terms[3] == 9
            balance = int(rpc("eth_getBalance",[SPENDER,"latest"]),16)
            tx = rpc("eth_sendRawTransaction",["0x"+signed_transfer])
            rpc("anvil_mine",["0x1"])
            assert tx=="0x"+keccak256(bytes.fromhex(signed_transfer)).hex()
            assert rpc("eth_getTransactionReceipt",[tx])["status"]=="0x1"
            assert int(rpc("eth_getBalance",[SPENDER,"latest"]),16) == balance+9
            print("Disposable Anvil: op20 authority/display-gated firmware device_evm signed a standard native transfer and recipient received exactly 9 wei")

        finally:
            node.terminate()
            node.wait(timeout=5)


if __name__=="__main__":
    main(sys.argv[1])
