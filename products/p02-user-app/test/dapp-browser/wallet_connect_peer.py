"""A separate WalletConnect v2 DApp peer using Python/OpenSSL over a disposable WSS relay."""
import asyncio
import base64
from collections import defaultdict
import hashlib
import json
import os
from pathlib import Path
import secrets
import ssl
import subprocess
import tempfile
import time

from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.asymmetric.x25519 import X25519PrivateKey, X25519PublicKey
from cryptography.hazmat.primitives.ciphers.aead import ChaCha20Poly1305
from cryptography.hazmat.primitives.kdf.hkdf import HKDF
from websockets.asyncio.client import connect
from websockets.asyncio.server import serve
from websockets.exceptions import ConnectionClosed

HERE = Path(__file__).resolve().parent


def seal(key, packet):
    nonce = secrets.token_bytes(12)
    return base64.b64encode(b"\0" + nonce + ChaCha20Poly1305(key).encrypt(nonce, json.dumps({"jsonrpc":"2.0", **packet}).encode(), None)).decode()


def unseal(key, ciphertext):
    envelope = base64.b64decode(ciphertext, validate=True)
    assert envelope[0] == 0
    return json.loads(ChaCha20Poly1305(key).decrypt(envelope[1:13], envelope[13:], None))


class Relay:
    def __init__(self, defer_rejection=True):
        self.defer_rejection = defer_rejection
        self.listening = defaultdict(dict)
        self.backlog = defaultdict(list)
        self.id = 1
        self.deferred_rejection = {}

    async def notify(self, topic, websocket, message):
        self.id += 1
        await websocket.send(json.dumps({"jsonrpc":"2.0","id":str(self.id),"method":"irn_subscription",
            "params":{"id":self.listening[topic][websocket],"data":message}}))

    def drain_backlog(self, topic, receiver):
        entries = self.backlog.pop(topic, [])
        retained = [entry for entry in entries if entry[0] == receiver]
        if retained:
            self.backlog[topic] = retained
        return [message for sender, message in entries if sender != receiver]

    async def accept(self, websocket):
        try:
            async for encoded in websocket:
                event = json.loads(encoded)
                if "method" not in event:
                    continue
                body = event["params"]
                method = event["method"]
                if method in {"irn_subscribe", "irn_batchSubscribe"}:
                    topics = [body["topic"]] if method == "irn_subscribe" else body["topics"]
                    subscriptions = []
                    for topic in topics:
                        subscription = secrets.token_hex(16)
                        self.listening[topic][websocket] = subscription
                        subscriptions.append(subscription)
                    result = subscriptions[0] if method == "irn_subscribe" else subscriptions
                    await websocket.send(json.dumps({"jsonrpc":"2.0","id":event["id"],"result":result}))
                    old_ack = self.deferred_rejection.pop(websocket, None)
                    if old_ack:
                        await websocket.send(old_ack)
                    for topic in topics:
                        for prior in self.drain_backlog(topic, websocket):
                            await self.notify(topic, websocket, prior)
                elif method == "irn_unsubscribe":
                    self.listening[body["topic"]].pop(websocket, None)
                    await websocket.send(json.dumps({"jsonrpc":"2.0","id":event["id"],"result":True}))
                elif method == "irn_batchFetchMessages":
                    topics = body["topics"]
                    messages = [message for topic in topics if websocket in self.listening.get(topic, {})
                                for message in self.drain_backlog(topic, websocket)]
                    await websocket.send(json.dumps({"jsonrpc":"2.0","id":event["id"],"result":{"messages":messages}}))
                elif method in {"irn_publish", "wc_proposeSession"}:
                    if method == "wc_proposeSession":
                        # Newer SignClient submits the encrypted proposal through a custom verb.
                        body = {"topic":body["pairingTopic"],"message":body["sessionProposal"],
                                "ttl":body["ttl"],"tag":1100}
                    topic = body["topic"]
                    ack = json.dumps({"jsonrpc":"2.0","id":event["id"],"result":True})
                    if self.defer_rejection and body["tag"] == 1120:
                        self.deferred_rejection[websocket] = ack
                    else:
                        await websocket.send(ack)
                    data = {**body, "publishedAt":int(time.time())}
                    receivers = [receiver for receiver in self.listening[topic] if receiver != websocket]
                    if not receivers:
                        self.backlog[topic].append((websocket, data))
                    for receiver in receivers:
                        await self.notify(topic, receiver, data)
                else:
                    raise ValueError("Unsupported fixture relay method")
        except ConnectionClosed:
            pass
        finally:
            for sockets in self.listening.values():
                sockets.pop(websocket, None)


class Dapp:
    def __init__(self, websocket):
        self.socket = websocket
        self.id = 10
        self.incoming = []

    async def rpc(self, method, params):
        self.id += 1
        await self.socket.send(json.dumps({"jsonrpc":"2.0","id":str(self.id),"method":method,"params":params}))
        while True:
            event = json.loads(await self.socket.recv())
            if event["id"] == str(self.id) and "result" in event:
                return event["result"]
            assert event["method"] == "irn_subscription"
            self.incoming.append(event["params"]["data"])
            await self.socket.send(json.dumps({"jsonrpc":"2.0","id":event["id"],"result":True}))

    async def publish(self, topic, tag, key, body):
        return await self.rpc("irn_publish", {"topic":topic,"tag":tag,"ttl":300,"message":seal(key,body)})

    async def receive(self, tag, key):
        if self.incoming:
            data = self.incoming.pop(0)
        else:
            event = json.loads(await self.socket.recv())
            data = event["params"]["data"]
            await self.socket.send(json.dumps({"jsonrpc":"2.0","id":event["id"],"result":True}))
        assert data["tag"] == tag, data["tag"]
        return unseal(key, data["message"])


async def start(url, directory, cert):
    return await asyncio.create_subprocess_exec("node",str(HERE / "walletConnectDriver.mjs"),url,str(directory / "checkpoint.json"),
      stdin=asyncio.subprocess.PIPE,stdout=asyncio.subprocess.PIPE,env={**os.environ,"NODE_EXTRA_CA_CERTS":str(cert)})


async def command(child, line, expected=None):
    child.stdin.write((line+"\n").encode())
    await child.stdin.drain()
    if expected:
        while True:
            value = (await child.stdout.readline()).decode().strip()
            if value == expected:
                return
            assert value and not value.startswith("error "), value


async def scenario(url, directory, cert):
    trust = ssl.create_default_context(cafile=cert)
    async with connect(url, ssl=trust) as websocket:
        dapp = Dapp(websocket)
        pairing_key = secrets.token_bytes(32)
        pairing = hashlib.sha256(pairing_key).hexdigest()
        pairing_uri = f"wc:{pairing}@2?symKey={pairing_key.hex()}&relay-protocol=irn&expiryTimestamp={int(time.time())+200}"
        await dapp.rpc("irn_subscribe",{"topic":pairing})
        child = await start(url,directory,cert)
        try:
            await command(child,"pair "+pairing_uri,"paired")
            private = X25519PrivateKey.generate()
            chain = "eip155:11155111"
            def proposal(number,origin):
                return {"id":number,"method":"wc_sessionPropose","params":{
                    "relays":[{"protocol":"irn"}],"proposer":{"publicKey":private.public_key().public_bytes_raw().hex(),
                    "metadata":{"url":origin,"name":"Separate DApp","icons":[]}},
                    "requiredNamespaces":{"eip155":{"chains":[chain],"methods":["eth_accounts"],"events":[]}},
                    "optionalNamespaces":{"eip155":{"chains":[chain],"methods":["personal_sign","eth_sendTransaction"],"events":[]}}}}
            await dapp.publish(pairing,1100,pairing_key,proposal(101,"https://denied.example"))
            assert (await dapp.receive(1120,pairing_key))["error"]
            await command(child,"pair "+pairing_uri,"error WC_PAIRING_DECIDED")
            refused_topic, refused_key = pairing, pairing_key
            pairing_key=secrets.token_bytes(32)
            pairing=hashlib.sha256(pairing_key).hexdigest()
            pairing_uri=f"wc:{pairing}@2?symKey={pairing_key.hex()}&relay-protocol=irn&expiryTimestamp={int(time.time())+200}"
            await dapp.rpc("irn_subscribe",{"topic":pairing})
            await command(child,"pair "+pairing_uri,"paired")
            await dapp.publish(refused_topic,1100,refused_key,proposal(101,"https://wrong.example"))
            await dapp.publish(pairing,1100,pairing_key,proposal(102,"https://independent.example"))
            response = await dapp.receive(1101,pairing_key)
            assert response["id"] == 102
            public = bytes.fromhex(response["result"]["responderPublicKey"])
            secret = HKDF(algorithm=hashes.SHA256(),length=32,salt=None,info=b"").derive(private.exchange(X25519PublicKey.from_public_bytes(public)))
            topic = hashlib.sha256(secret).hexdigest()
            await dapp.rpc("irn_subscribe",{"topic":topic})
            settle = await dapp.receive(1102,secret)
            namespaces = settle["params"]["namespaces"]
            assert namespaces["eip155"]["methods"] == ["eth_accounts"]
            await dapp.publish(topic,1103,secret,{"id":settle["id"],"result":True})
            async def request(number, method, network=chain, expiry=None):
                await dapp.publish(topic,1108,secret,{"id":number,"method":"wc_sessionRequest","params":{"chainId":network,
                     "request":{"method":method,"params":[],**({"expiry":expiry} if expiry is not None else {})}}})
                return await dapp.receive(1109,secret)
            assert (await request(103,"personal_sign"))["error"]
            assert (await request(104,"eth_accounts","eip155:1"))["error"]
            assert (await request(105,"eth_accounts",expiry=int(time.time())-1))["error"]
            assert (await request(106,"eth_accounts"))["result"] == ["0x"+"22"*20]
            await command(child,"suspend")
            child.stdin.close()
            await child.wait()
            await dapp.publish(topic,1108,secret,{"id":106,"method":"wc_sessionRequest","params":{"chainId":chain,
                "request":{"method":"eth_accounts","params":[]}}})
            child = await start(url,directory,cert)
            await command(child,"resume","resumed")
            assert (await dapp.receive(1109,secret))["error"]
            assert (await request(107,"eth_accounts"))["result"] == ["0x"+"22"*20]
            await dapp.publish(topic,1112,secret,{"id":108,"method":"wc_sessionDelete","params":{"code":6000,"message":"Peer disconnected"}})
            assert (await dapp.receive(1113,secret))["result"] is True
            assert json.loads((directory / "checkpoint.json").read_text()) is None
            print("Independent WSS DApp peer passed pairing, proposal refusal, v2 settlement, scoped request denial, restart replay, and peer disconnect")
        finally:
            if child.returncode is None:
                child.terminate()
                await child.wait()


async def run():
    with tempfile.TemporaryDirectory() as path:
        directory = Path(path)
        cert, key = directory / "relay.crt", directory / "relay.key"
        subprocess.run(["openssl","req","-x509","-newkey","rsa:2048","-nodes","-days","1","-subj","/CN=localhost",
          "-addext","subjectAltName=IP:127.0.0.1","-keyout",str(key),"-out",str(cert)],check=True,capture_output=True)
        tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        tls.load_cert_chain(cert,key)
        async with serve(Relay().accept,"127.0.0.1",0,ssl=tls) as server:
            port = server.sockets[0].getsockname()[1]
            await asyncio.wait_for(scenario(f"wss://127.0.0.1:{port}",directory,cert),20)


if __name__ == "__main__":
    asyncio.run(run())
