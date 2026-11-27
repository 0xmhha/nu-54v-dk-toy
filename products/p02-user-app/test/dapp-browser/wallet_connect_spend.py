"""Join an independent WalletConnect peer, production op20 C gateway, and disposable Anvil."""
import asyncio
import base64
import datetime
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import secrets
import ssl
import subprocess
import sys
import tempfile
import threading
import time
from urllib.request import Request, urlopen

from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.primitives.asymmetric.x25519 import X25519PrivateKey, X25519PublicKey
from cryptography.hazmat.primitives.kdf.hkdf import HKDF
from websockets.asyncio.client import connect
from websockets.asyncio.server import serve

from wallet_connect_peer import Relay, Dapp, command

HERE = Path(__file__).resolve().parent
P01 = HERE.parents[2] / "p01-device-firmware"
sys.path.insert(0,str(P01 / "test"))
from test_revoke_anvil import BOARD, OWNER, TOKEN, SPENDER, KEY, WalletAuthority, gateway_sign, issue_peer
from evm_transfer import parse, parse_siwe_capsule

CHAIN = "eip155:11155111"
ORIGIN = "https://independent.example"


def fixture(directory, rpc, genesis, pairing, public, tls, native=None):
    issuer = ec.generate_private_key(ec.SECP256R1())
    key = directory / "authority.pem"
    key.touch(mode=0o600)
    key.write_bytes(issuer.private_bytes(serialization.Encoding.PEM,serialization.PrivateFormat.PKCS8,serialization.NoEncryption()))
    now = int(time.time())
    port = native.get("port",18449) if native else 0
    origin = f"https://127.0.0.1:{port}"
    chain = str(native.get("evm_chain", 11155111)) if native else "11155111"
    network_url=origin+"/rpc-native" if native else "loopback"
    relay=native["relay"] if native else "wss://127.0.0.1"
    profile = {"device":BOARD,"sender":OWNER,"generation":1,"leaseRevision":"1","expires":now+(290 if native else 160),
        "networks":[{"chain":chain,"rpc":network_url,"genesis":genesis,"feeLimit":str(10**17),"assets":[{"address":"0x"+"00"*20,"symbol":"ETH","decimals":0}]}],
        "dapp":{"chain":chain,"origin":origin if native and native.get("browser_login") else ORIGIN},"walletConnect":{"relay":relay}}
    positive_token = native.get("positive_token") if native else None
    revoke_token = native.get("revoke_token") if native else None
    gateway = native.get("gateway") if native else None
    gateway = gateway or P01/"out/host/test_device_evm"
    if positive_token:
        profile["dapp"]["allowance"] = {"token":positive_token,"spender":SPENDER,"maximum":"5000000"}
        profile["networks"][0]["assets"].append({"address":positive_token,"symbol":"DEMO","decimals":6})
    if revoke_token:
        profile["networks"][0]["assets"].append({"address":revoke_token,"symbol":"TUSD","decimals":6})
    if native and native.get("evm_profiles"):
        profile["networks"] = native["evm_profiles"]
    if native and native.get("commissioned_profiles"):
        profile["chainNetworks"] = native["commissioned_profiles"]
    elif native and native.get("commissioned"):
        profile["chainNetworks"] = [native["commissioned"]]
    proof_profile = {**profile, "expires": now + native["proof_lifetime"]} if native and "proof_lifetime" in native else profile
    profile["walletConnect"]["peer"] = issue_peer(proof_profile,key,pairing,public,now) if pairing else None
    if native and native.get("login"):
        public_key = ec.derive_private_key(int(KEY[2:],16),ec.SECP256K1()).public_key()
        profile["dapp"]["publicKey"] = public_key.public_bytes(serialization.Encoding.X962,serialization.PublicFormat.UncompressedPoint).hex()
    wallet = WalletAuthority(profile,key,hashlib.sha256(TOKEN.encode()).hexdigest(),lambda _:rpc)
    board_calls = []
    board_lock = threading.Lock()
    registry = wallet.handle({"jsonrpc":"2.0","id":1,"method":"nu54_walletRegistry","params":[{"token":TOKEN}]})
    if native and native.get("login"):
        native["wallet"] = wallet

        def login_message(nonce, expires):
            def stamp(epoch):
                return datetime.datetime.fromtimestamp(epoch,datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
            site = profile["dapp"]["origin"]
            host = site.removeprefix("https://")
            message = "\n".join((f"{host} wants you to sign in with your Ethereum account:", OWNER, "", "",
                                f"URI: {site}/sign-in", "Version: 1", f"Chain ID: {chain}", f"Nonce: {nonce}",
                                f"Issued At: {stamp(int(time.time()))}", f"Expiration Time: {stamp(expires)}"))
            with wallet.login_lock:
                wallet.login_challenges[nonce] = (expires,message)
            return message

        native["login_message"] = login_message

    def owner_state():
        anchor = issuer.public_key().public_bytes(serialization.Encoding.X962,serialization.PublicFormat.UncompressedPoint).hex()
        state = {"version":2,"exercised":True,"mode":"personal","verified":{
            "boardFingerprint":BOARD,"address":OWNER,"generation":1,"network":"8283"},
            "enrollment":{"version":2,"device":BOARD,"boardSPKI":"77"*32,"ca":native["cert"],"certificateChain":[native["cert"]]},
            "evmRegistry":{**registry,"anchor":anchor,"endpoint":origin+"/wallet-native","token":TOKEN}}
        if native.get("commissioned"):
            state["chainSelection"] = {"chain":native["commissioned"]["chain"],"network":2,"account":0}
        if native.get("wallet_read"):
            state["walletRead"] = native["wallet_read"]
        return state

    class Handler(BaseHTTPRequestHandler):
        def send_json(self,result):
            data = json.dumps(result).encode()
            self.send_response(200)
            self.send_header("Content-Type","application/json")
            self.send_header("Content-Length",str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def do_GET(self):
            if native and (native.get("explore_site") or native.get("browser_login")) and self.path=="/site":
                page = ((HERE / "browser-siwe.html").read_bytes() if native.get("browser_login") else
                        b"<!doctype html><html><body><h1>Local DApp preview</h1></body></html>")
                self.send_response(200)
                self.send_header("Content-Type","text/html; charset=utf-8")
                self.send_header("Content-Length",str(len(page)))
                self.end_headers()
                self.wfile.write(page)
            elif native and self.path=="/profile":
                self.send_json(owner_state())
            elif native and self.path=="/case":
                self.send_json({**native["case"],"boardCalls":len(board_calls)})
            else:
                self.send_error(404)

        def do_POST(self):
            phase = "request"
            try:
                size = int(self.headers["Content-Length"])
                assert 0 < size <= (6144 if native and native.get("joint_board") else 4000)
                body = json.loads(self.rfile.read(size))
                if native and native.get("browser_login") and self.path in ("/siwe/challenge","/siwe/verify","/siwe/complete"):
                    phase = "browser login"
                    if self.headers.get("Origin") != origin:
                        raise ValueError("Unexpected browser origin")
                    result = native["browser_request"](self.path,body,board_calls)
                elif self.path == "/board":
                    phase = "board"
                    if native and native.get("login"):
                        wire = bytes.fromhex(body["wire"])
                        with board_lock:
                            assert not board_calls and wire[0] == 15
                            parse_siwe_capsule(wire,issuer.public_key(),bytes.fromhex(BOARD),bytes.fromhex(OWNER[2:]),1,int(time.time()))
                            statement = b"N54S"+bytes.fromhex(BOARD)+(1).to_bytes(8,"big")+bytes.fromhex(OWNER[2:])+wire[:-64]
                            result = subprocess.check_output([str(gateway),wire.hex(),statement.hex(),wire[-64:].hex(),
                                BOARD,OWNER[2:],KEY[2:],str(int(time.time())),"1"],text=True).strip()
                            board_calls.append(wire)
                        self.send_json(result)
                        return
                    if native and native.get("deny_signing"):
                        native["case"]["signingAttempts"] += 1
                        raise ValueError("Session management cannot sign")
                    wire = bytes.fromhex(body["wire"])
                    with board_lock:
                        if wire[0] == 14:
                            assert revoke_token and not any(previous[0]==14 for previous in board_calls)
                            reviewed = parse(wire, issuer.public_key(), bytes.fromhex(BOARD), bytes.fromhex(OWNER[2:]), 1, int(time.time()))
                            assert "0x"+reviewed[2].hex()==revoke_token and reviewed[3]==0
                            assert "0x"+reviewed[4].hex()==SPENDER and reviewed[0]==int(chain)
                            assert "0x"+reviewed[7].hex()==genesis and reviewed[5]>0
                        if native and native.get("direct") and wire[0] == 9:
                            terms, result = native["direct"](P01/"out/host/test_device_evm",wire,issuer.public_key(),int(time.time()))
                            result = result.removeprefix("0x")
                        else:
                            terms, result = gateway_sign(gateway,wire,issuer.public_key(),BOARD,OWNER,1,int(time.time()))
                        if wire[0] == 14:
                            assert terms == reviewed
                        elif wire[0] == 36:
                            assert positive_token and not any(previous[0]==36 for previous in board_calls)
                            assert "0x"+terms[2].hex()==positive_token and terms[3]==5000000
                            assert "0x"+terms[4].hex()==SPENDER and terms[0]==int(chain)
                        else:
                            assert wire[0] in (9,20) and terms[3]==9 and "0x"+terms[4].hex()==SPENDER
                        board_calls.append(wire)
                elif self.path == "/chain-public" and native and native.get("chain_public"):
                    phase = "chain-public"
                    result = native["chain_public"](bytes.fromhex(body["wire"])).hex()
                elif self.path == "/chain-watch" and native and native.get("chain_watch"):
                    phase = "chain-watch"
                    result = native["chain_watch"](bytes.fromhex(body["wire"])).hex()
                elif self.path == "/joint-board" and native and native.get("joint_board"):
                    phase = "joint-board"
                    result = native["joint_board"](bytes.fromhex(body["wire"])).hex()
                elif self.path == "/chain-sign" and native and native.get("chain_sign"):
                    phase = "chain-sign"
                    result = native["chain_sign"](bytes.fromhex(body["wire"])).hex()
                elif native and native.get("chain_rpc") and self.path in {"/chain-node",native.get("chain_route")}:
                    phase = "chain-node"
                    result = native["chain_rpc"](body, key)
                elif self.path in ("/wallet","/wallet-native"):
                    phase = "peer" if body.get("method") == "nu54_walletConnectPeer" else "authority"
                    if native and native.get("peer") and profile["walletConnect"]["peer"] is None:
                        peer = native["peer"]
                        profile["walletConnect"]["peer"] = issue_peer(profile,key,*peer,now)
                    result = wallet.handle(body)
                elif self.path in ("/rpc","/rpc-native"):
                    phase = "ledger"
                    if native and (native.get("deny_signing") or native.get("login")) and body["method"] == "eth_sendRawTransaction":
                        native["case"]["submissions"] += 1
                        raise ValueError("Session management cannot submit")
                    if body["method"] == "eth_call":
                        allowance = "0xdd62ed3e"+OWNER[2:].rjust(64,"0")+SPENDER[2:].rjust(64,"0")
                        assert (revoke_token or positive_token) and body["params"][0]=={"to":revoke_token or positive_token,"data":allowance}
                        block = body["params"][1]
                        assert block=="finalized" or (isinstance(block,dict) and set(block)=={"blockHash","requireCanonical"} and
                              block["requireCanonical"] is True and block["blockHash"]==rpc("eth_getBlockByNumber",["finalized",False])["hash"])
                    else:
                        assert body["method"] in {"eth_chainId","eth_getBlockByNumber","eth_sendRawTransaction","eth_getTransactionReceipt"}
                    result = rpc(body["method"],body["params"])
                    if native and native.get("observe_receipt") and body["method"] == "eth_getTransactionReceipt":
                        native["observe_receipt"](body["params"][0])
                    if native and native.get("submitted") and body["method"] == "eth_sendRawTransaction":
                        native["submitted"](result, board_calls)
                else:
                    raise ValueError("Unexpected path")
                if self.path.endswith("-native"):
                    result = {"jsonrpc":"2.0","id":1,"result":result}
                self.send_json(result)
            except Exception as error:
                print("Test boundary",phase,type(error).__name__,flush=True)
                self.send_error(400)

        def log_message(self,*_):
            pass

    server = ThreadingHTTPServer(("127.0.0.1",port),Handler)
    server.socket = tls.wrap_socket(server.socket,server_side=True)
    thread = threading.Thread(target=server.serve_forever,daemon=True)
    thread.start()
    anchor = issuer.public_key().public_bytes(serialization.Encoding.X962,serialization.PublicFormat.UncompressedPoint).hex()
    return server, thread, anchor, board_calls


async def scenario(directory, socket, http, cert, anchor, rpc, genesis, key, private, board_calls):
    pair = hashlib.sha256(key).hexdigest()
    checkpoint = directory/"session.json"
    settings = directory/"settings.json"
    settings.write_text(json.dumps({"socket":socket,"http":http,"authority":anchor,"token":TOKEN,"recipient":SPENDER,
        "scope":{"board":BOARD,"account":OWNER,"generation":1,"chain":11155111,"genesis":genesis}}))
    tls = ssl.create_default_context(cafile=cert)

    async def client():
        return await asyncio.create_subprocess_exec("node",str(HERE/"externalSpendDriver.mjs"),str(settings),str(checkpoint),
            stdin=asyncio.subprocess.PIPE,stdout=asyncio.subprocess.PIPE,env={**os.environ,"NODE_EXTRA_CA_CERTS":str(cert)})

    async with connect(socket,ssl=tls) as connection:
        peer = Dapp(connection)
        await peer.rpc("irn_subscribe",{"topic":pair})
        phone = await client()
        try:
            await command(phone,f"pair wc:{pair}@2?symKey={key.hex()}&relay-protocol=irn","paired")
            await peer.publish(pair,1100,key,{"id":201,"method":"wc_sessionPropose","params":{
                "relays":[{"protocol":"irn"}],"proposer":{"publicKey":private.public_key().public_bytes_raw().hex(),
                "metadata":{"url":ORIGIN,"name":"Independent spend peer","icons":[]}},
                "requiredNamespaces":{"eip155":{"chains":[CHAIN],"methods":["eth_accounts","eth_sendTransaction"],"events":[]}}}})
            approved = (await peer.receive(1101,key))["result"]
            shared = private.exchange(X25519PublicKey.from_public_bytes(bytes.fromhex(approved["responderPublicKey"])))
            secret = HKDF(algorithm=hashes.SHA256(),length=32,salt=None,info=b"").derive(shared)
            topic = hashlib.sha256(secret).hexdigest()
            await peer.rpc("irn_subscribe",{"topic":topic})
            settle = await peer.receive(1102,secret)
            assert settle["params"]["namespaces"]["eip155"]["accounts"]==[CHAIN+":"+OWNER]
            await peer.publish(topic,1103,secret,{"id":settle["id"],"result":True})

            async def send(number, recipient, **extra):
                await peer.publish(topic,1108,secret,{"id":number,"method":"wc_sessionRequest","params":{
                    "chainId":CHAIN,"request":{"method":"eth_sendTransaction","params":[{
                        "from":OWNER,"to":recipient,"value":"0x9",**extra}]}}})
                return await peer.receive(1109,secret)

            assert (await send(202,SPENDER,data="0x095ea7b3"))["error"]
            assert (await send(203,"0x"+"cc"*20))["error"]
            assert not board_calls
            before = int(rpc("eth_getBalance",[SPENDER,"latest"]),16)
            receipt_hash = (await send(204,SPENDER))["result"]
            rpc("anvil_mine",["0x1"])
            receipt = rpc("eth_getTransactionReceipt",[receipt_hash])
            rpc("anvil_mine",["0x80"])
            final = rpc("eth_getBlockByNumber",["finalized",False])
            assert receipt["status"]=="0x1" and int(final["number"],16)>=int(receipt["blockNumber"],16)
            assert int(rpc("eth_getBalance",[SPENDER,"finalized"]),16)==before+9
            assert len(board_calls)==1 and board_calls[0][165:229]==bytes.fromhex(pair+topic)
            await command(phone,"suspend")
            phone.stdin.close()
            await phone.wait()
            phone=await client()
            await command(phone,"resume","resumed")
            assert (await send(204,SPENDER))["error"] and len(board_calls)==1
            await peer.publish(topic,1112,secret,{"id":205,"method":"wc_sessionDelete","params":{"code":6000,"message":"Peer disconnected"}})
            assert (await peer.receive(1113,secret))["result"] is True
            assert json.loads(checkpoint.read_text()) is None
            print("Independent verified DApp → WSS → op20 authority → production C gateway host substitute → finalized +9 wei → restart replay denied")
        finally:
            if phone.returncode is None:
                phone.terminate()
                await phone.wait()


def pairing_data():
    key=secrets.token_bytes(32)
    topic=hashlib.sha256(key).hexdigest()
    return key,topic,f"wc:{topic}@2?symKey={key.hex()}&relay-protocol=irn"


async def native_scenario(relay, config, cert, rpc, first, second, private, board_calls):
    def make_proposal(identifier, methods):
        return {"id":identifier,"method":"wc_sessionPropose","params":{"relays":[{"protocol":"irn"}],
            "proposer":{"publicKey":private.public_key().public_bytes_raw().hex(),"metadata":{"url":ORIGIN,"name":"Independent peer"}},
            "requiredNamespaces":{"eip155":{"chains":[CHAIN],"methods":methods,"events":[]}}}}
    context=ssl.create_default_context(cafile=cert)
    async with connect(config["relay"],ssl=context) as connection:
        peer=Dapp(connection)
        for (key,topic,_),identifier,methods in [(first,201,["eth_accounts"]),(second,202,["eth_accounts","eth_sendTransaction"])]:
            await peer.rpc("irn_subscribe",{"topic":topic})
            while len(relay.listening[topic]) < 2:
                await asyncio.sleep(0.05)
            await peer.publish(topic,1100,key,make_proposal(identifier,methods))
            if identifier==201:
                assert (await peer.receive(1120,key))["error"]
                config["case"]["declined"]=True
        key,pairing,_=second
        approved=(await peer.receive(1101,key))["result"]
        shared=private.exchange(X25519PublicKey.from_public_bytes(bytes.fromhex(approved["responderPublicKey"])))
        secret=HKDF(algorithm=hashes.SHA256(),length=32,salt=None,info=b"").derive(shared)
        topic=hashlib.sha256(secret).hexdigest()
        await peer.rpc("irn_subscribe",{"topic":topic})
        settle=await peer.receive(1102,secret)
        assert settle["params"]["namespaces"]["eip155"]["accounts"]==[CHAIN+":"+OWNER]
        await peer.publish(topic,1103,secret,{"id":settle["id"],"result":True})

        async def send(identifier, recipient, **extra):
            await peer.publish(topic,1108,secret,{"id":identifier,"method":"wc_sessionRequest","params":{
                "chainId":CHAIN,"request":{"method":"eth_sendTransaction","params":[{
                    "from":OWNER,"to":recipient,"value":"0x9",**extra}]}}})
            return await peer.receive(1109,secret)

        assert (await send(203,SPENDER,data="0x095ea7b3"))["error"]
        assert (await send(204,"0x"+"cc"*20))["error"] and not board_calls
        before=int(rpc("eth_getBalance",[SPENDER,"latest"]),16)
        tx=(await send(205,SPENDER))["result"]
        rpc("anvil_mine",["0x80"])
        receipt=rpc("eth_getTransactionReceipt",[tx])
        assert receipt["status"]=="0x1" and int(rpc("eth_getBalance",[SPENDER,"finalized"]),16)==before+9
        assert len(board_calls)==1 and board_calls[0][165:229]==bytes.fromhex(pairing+topic)
        config["case"]["receipt"]=tx
        while len(relay.listening[topic]) != 1:
            await asyncio.sleep(0.05)
        while len(relay.listening[topic]) != 2:
            await asyncio.sleep(0.05)
        assert (await send(205,SPENDER))["error"] and len(board_calls)==1
        config["case"]["replayed"]=True
        await peer.publish(topic,1112,secret,{"id":206,"method":"wc_sessionDelete","params":{"code":6000,"message":"Peer disconnected"}})
        assert (await peer.receive(1113,secret))["result"] is True
        config["case"]["done"]=True
        print("Installed P02 QA UI and independent WSS peer passed refusal, commissioned spend, final receipt, restart replay and disconnect",flush=True)
        await asyncio.get_running_loop().create_future()


async def main():
    with tempfile.TemporaryDirectory() as temp:
        directory=Path(temp)
        native_run=len(sys.argv)==4 and sys.argv[1]=="--native"
        cert,key=(Path(sys.argv[2]),Path(sys.argv[3])) if native_run else (directory/"relay.crt",directory/"relay.key")
        if not native_run:
            subprocess.run(["openssl","req","-x509","-newkey","rsa:2048","-nodes","-days","1","-subj","/CN=localhost",
            "-addext","subjectAltName=IP:127.0.0.1","-keyout",str(key),"-out",str(cert)],check=True,capture_output=True)
        tls=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        tls.load_cert_chain(cert,key)
        import socket
        with socket.socket() as reserved:
            reserved.bind(("127.0.0.1",0))
            port=reserved.getsockname()[1]

        def rpc(method,params):
            packet={"jsonrpc":"2.0","id":1,"method":method,"params":params}
            call=Request(f"http://127.0.0.1:{port}",json.dumps(packet).encode(),{"Content-Type":"application/json"})
            with urlopen(call,timeout=2) as response:
                result=json.load(response)
            if "error" in result:
                raise ValueError(result["error"])
            return result["result"]

        with (directory/"anvil.log").open("w") as log:
            anvil=subprocess.Popen(["anvil","--host","127.0.0.1","--port",str(port),"--chain-id","11155111"],stdout=log,stderr=log)
            try:
                for _ in range(80):
                    if anvil.poll() is not None:
                        raise RuntimeError("Disposable Anvil failed to bind")
                    try:
                        genesis=rpc("eth_getBlockByNumber",["0x0",False])["hash"]
                        break
                    except OSError:
                        await asyncio.sleep(0.05)
                else:
                    raise RuntimeError("Disposable Anvil failed to start")
                peer=X25519PrivateKey.generate()
                first, second=pairing_data(),pairing_data()
                relay=Relay()
                async with serve(relay.accept,"127.0.0.1",0,ssl=tls) as listener:
                    socket_url=f"wss://127.0.0.1:{listener.sockets[0].getsockname()[1]}"
                    config=None
                    if native_run:
                        config={"relay":socket_url,"cert":base64.b64encode(ssl.PEM_cert_to_DER_cert(cert.read_text())).decode(),
                            "case":{"first":first[2],"second":second[2]}}
                    operator,thread,anchor,board_calls=fixture(directory,rpc,genesis,second[1],
                        peer.public_key().public_bytes_raw().hex(),tls,config)
                    try:
                        if native_run:
                            print("Native P02 QA service ready on https://127.0.0.1:18449",flush=True)
                            await native_scenario(relay,config,cert,rpc,first,second,peer,board_calls)
                        else:
                            await asyncio.wait_for(scenario(directory,socket_url,f"https://127.0.0.1:{operator.server_port}",
                                cert,anchor,rpc,genesis,second[0],peer,board_calls),25)
                    finally:
                        operator.shutdown()
                        operator.server_close()
                        thread.join()
            finally:
                anvil.terminate()
                anvil.wait(timeout=5)


if __name__=="__main__":
    asyncio.run(main())
