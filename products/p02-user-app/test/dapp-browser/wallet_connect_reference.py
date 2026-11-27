"""Run a pinned official SignClient peer against the product wallet on loopback."""
import argparse
import asyncio
import base64
import hashlib
from contextlib import asynccontextmanager, contextmanager
import json
import os
from pathlib import Path
import shutil
import signal
import socket
import ssl
import subprocess
import sys
import tempfile
import time
from urllib.request import Request, urlopen

from websockets.asyncio.server import serve
from websockets.asyncio.client import connect
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.asymmetric.x25519 import X25519PrivateKey,X25519PublicKey
from cryptography.hazmat.primitives.kdf.hkdf import HKDF

from reference_lock import VERSION, validate_lock
from wallet_connect_peer import Dapp, Relay
from wallet_connect_spend import fixture, pairing_data, BOARD, OWNER, TOKEN, SPENDER
from test_revoke_anvil import cast
from evm_transfer import keccak256

HERE = Path(__file__).resolve().parent
TIMEOUT = 45
SANDBOX = ["/usr/bin/sandbox-exec", "-p",
           '(version 1)(allow default)(deny network-outbound)(allow network-outbound (remote ip "localhost:*"))']


def provision(directory, npm):
    target = directory / "reference"
    shutil.copytree(HERE / "reference", target, ignore=shutil.ignore_patterns("node_modules"))
    validate_lock(target / "package-lock.json")
    env = {key: value for key, value in os.environ.items() if not key.lower().startswith("npm_config_") and key != "NODE_OPTIONS"}
    user, global_config = directory / "user.npmrc", directory / "global.npmrc"
    user.touch()
    global_config.touch()
    subprocess.run([npm, "ci", "--ignore-scripts", "--no-audit", "--no-fund", "--userconfig", user,
                    "--globalconfig", global_config, "--registry=https://registry.npmjs.org", "--cache", directory / "npm-cache"],
                   cwd=target, env=env, capture_output=True, timeout=90, check=True)
    return target / "peer.cjs"


@asynccontextmanager
async def children(directory, cert):
    processes = []

    async def start(*args):
        child = await asyncio.create_subprocess_exec(*SANDBOX, *args, cwd=directory,
            env={"PATH": os.environ["PATH"], "NODE_EXTRA_CA_CERTS": str(cert)}, stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.DEVNULL)
        processes.append(child)
        return child

    try:
        yield start
    finally:
        for child in processes:
            if child.returncode is None:
                child.kill()
            await child.wait()


async def send(child, text):
    child.stdin.write((text + "\n").encode())
    await child.stdin.drain()


async def line(child, timeout=TIMEOUT):
    text = await asyncio.wait_for(child.stdout.readline(), timeout)
    if not text:
        code = await child.wait()
        raise RuntimeError(f"Reference process exited before the expected event (exit={code})")
    return text.decode().strip()


async def event(child, kind, timeout=TIMEOUT):
    for _ in range(16):
        try:
            raw = await line(child, timeout)
        except TimeoutError as error:
            raise RuntimeError(f"Timed out waiting for reference event: {kind}") from error
        if raw.startswith("{"):
            value = json.loads(raw)
            if value.get("type") == "failed":
                raise RuntimeError(f"Reference peer failed during {value['phase']} (code={value.get('code')})")
            if value.get("type") == kind:
                return value
    raise RuntimeError("Missing reference event: " + kind)


async def expect_driver(child, expected):
    for _ in range(16):
        observed = await line(child)
        if observed.startswith("error "):
            raise RuntimeError("Wallet driver rejected " + expected)
        if observed == expected:
            return
    raise RuntimeError("Missing wallet state: " + expected)


async def drain(child):
    while await child.stdout.readline():
        pass


@contextmanager
def anvil_node(anvil, chain=11155111):
    with socket.socket() as reserved:
        reserved.bind(("127.0.0.1", 0))
        port = reserved.getsockname()[1]
    child = subprocess.Popen([*SANDBOX,anvil,"--host","127.0.0.1","--port",str(port),"--chain-id",str(chain),"--silent"],
                             stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    def rpc(method, params):
        data = json.dumps({"jsonrpc":"2.0","id":1,"method":method,"params":params}).encode()
        req = Request(f"http://127.0.0.1:{port}", data, {"Content-Type":"application/json"})
        with urlopen(req, timeout=3) as conn:
            reply = json.load(conn)
        if reply.get("error"):
            raise RuntimeError("Local Anvil RPC rejected " + method)
        return reply["result"]

    try:
        yield rpc
    finally:
        child.terminate()
        child.wait(timeout=8)


async def chain_ready(rpc):
    for _ in range(80):
        try:
            return rpc("eth_getBlockByNumber", ["0x0", False])["hash"]
        except OSError:
            await asyncio.sleep(.1)
    raise RuntimeError("Local Anvil did not become ready")


def finalized_payment(rpc, tx, before, board_calls):
    if not isinstance(tx,str) or len(tx) != 66:
        raise RuntimeError("Invalid local payment reference")
    rpc("anvil_mine", ["0x81"])
    receipt = rpc("eth_getTransactionReceipt", [tx])
    latest = rpc("eth_getBlockByNumber", ["finalized", False])
    delta = int(rpc("eth_getBalance",[SPENDER,"finalized"]),16) - before
    if (receipt["status"] != "0x1" or receipt["transactionHash"] != tx or delta != 9 or len(board_calls) != 1 or
            int(latest["number"],16) < int(receipt["blockNumber"],16)):
        raise RuntimeError("Local payment evidence mismatch")
    return {"refusal": True, "hostBoardSignatures": len(board_calls), "finalizedWei":delta, "localChainId":11155111,
            "txHash":tx,"peerDisconnected":True}


async def read_case(start, node, peer_source, relay, directory):
    peer = await start(node, peer_source, relay, "read")
    account = await event(peer,"pair")
    wallet = await start(node,HERE / "walletConnectDriver.mjs",relay,directory / "state")
    await send(wallet, "pair " + account["uri"])
    await expect_driver(wallet,"paired")
    logging = asyncio.create_task(drain(wallet))
    try:
        answer = await event(peer,"read")
    finally:
        if wallet.returncode is None:
            wallet.terminate()
            await asyncio.wait_for(wallet.wait(),8)
        await logging
    wallet = await start(node,HERE / "walletConnectDriver.mjs",relay,directory / "state")
    await send(wallet,"resume")
    await expect_driver(wallet,"resumed")
    await send(peer,"continue")
    resumed = await event(peer,"resumed")
    await event(peer,"closed")
    return {"chain":answer["chain"], "accountCount":answer["accountCount"], "unverifiedSpendDenied":answer["denied"],
            "sameTopicAfterRestart":resumed["sameTopic"],"peerDisconnected":True}


async def spend_case(start, node, peer_source, relay, directory, tls, anvil):
    with anvil_node(anvil) as rpc:
        genesis = await chain_ready(rpc)
        before = int(rpc("eth_getBalance", [SPENDER,"latest"]), 16)
        peer = await start(node,peer_source,relay,"spend")
        request = await event(peer,"pair")
        topic = request["uri"].split(":",1)[1].split("@",1)[0]
        server, thread, anchor, board_calls = fixture(directory,rpc,genesis,topic,request["pub"],tls)
        try:
            settings = directory / "wallet.json"
            settings.write_text(json.dumps({"socket":relay,"http":f"https://127.0.0.1:{server.server_port}",
                "authority":anchor,"token":TOKEN,"recipient":SPENDER,"scope":{
                    "board":BOARD,"account":OWNER,"generation":1,"chain":11155111,"genesis":genesis}}))
            wallet = await start(node,HERE / "externalSpendDriver.mjs",settings,directory / "spend-state")
            await send(wallet,"pair " + request["uri"])
            await expect_driver(wallet,"paired")
            payment = await event(peer,"tx")
            if not payment["rejected"]:
                raise RuntimeError("Unauthorized recipient was accepted")
            await event(peer,"closed")
            return finalized_payment(rpc,payment["hash"],before,board_calls)
        finally:
            server.shutdown()
            server.server_close()
            thread.join()


async def native_case(start, node, peer_source, url, relay, directory, tls, cert, anvil):
    with anvil_node(anvil) as rpc:
        genesis = await chain_ready(rpc)
        before = int(rpc("eth_getBalance",[SPENDER,"latest"]),16)
        reader = await start(node,peer_source,url,"read")
        first = await event(reader,"pair")
        config = {"relay":url,"cert":base64.b64encode(ssl.PEM_cert_to_DER_cert(cert.read_text())).decode(),
                  "case":{"first":first["uri"]}}
        server, thread, _, board_calls = fixture(directory,rpc,genesis,None,None,tls,config)
        try:
            print("Native reference fixture ready on https://127.0.0.1:18449",flush=True)
            read = await event(reader,"read")
            config["case"]["readPassed"] = read["denied"] and read["accountCount"] == 1
            await send(reader,"continue")
            await event(reader,"resumed")
            await event(reader,"closed")
            payer = await start(node,peer_source,url,"native-spend")
            second = await event(payer,"pair")
            topic = second["uri"].split(":",1)[1].split("@",1)[0]
            config["peer"] = (topic, second["pub"])
            config["case"]["second"] = second["uri"]
            config["case"]["readClosed"] = True
            payment = await event(payer,"tx")
            if not payment["rejected"]:
                raise RuntimeError("Unauthorized recipient was accepted")
            proof = finalized_payment(rpc,payment["hash"],before,board_calls)
            config["case"]["receipt"] = proof["txHash"]
            # Wait for the installed customer process to terminate and resubscribe.
            async with asyncio.timeout(TIMEOUT):
                while any(len(clients) >= 2 for key,clients in relay.listening.items() if key != topic):
                    await asyncio.sleep(.1)
                while not any(len(clients) >= 2 for key,clients in relay.listening.items() if key != topic):
                    await asyncio.sleep(.1)
            await send(payer,"continue")
            resumed = await event(payer,"resumed")
            await event(payer,"closed")
            config["case"]["done"] = True
            proof.pop("txHash")
            print(json.dumps({**proof,"readDenied":config["case"]["readPassed"],"sameTopicAfterRestart":resumed["sameTopic"]}),flush=True)
            await asyncio.get_running_loop().create_future()
        finally:
            server.shutdown()
            server.server_close()
            thread.join()


async def native_refusal_case(start, node, peer_source, url, directory, tls, cert, anvil, kind):
    chain = 84532 if kind == "batch" else 11155111
    with anvil_node(anvil, chain) as rpc:
        genesis = await chain_ready(rpc)
        before = rpc("eth_getTransactionCount", [OWNER, "latest"])
        peer = await start(node, peer_source, url, "native-refuse-" + kind)
        pairing = await event(peer, "pair")
        topic = pairing["uri"].split(":", 1)[1].split("@", 1)[0]
        submissions = []

        def read_only(method, params):
            if method in ("eth_sendRawTransaction", "eth_sendTransaction"):
                submissions.append(method)
                raise ValueError("refusal fixture cannot submit")
            return rpc(method, params)

        state = {"first":pairing["uri"],"refusalKind":kind,"submissions":0}
        config = {"relay":url,"evm_chain":chain,"case":state,
                  "cert":base64.b64encode(ssl.PEM_cert_to_DER_cert(cert.read_text())).decode()}
        server, thread, _, calls = fixture(directory, read_only, genesis, topic, pairing["pub"], tls, config)
        try:
            print("Native advanced refusal fixture ready",flush=True)
            response = await event(peer, "refused")
            if response["code"] != 5000 or calls or submissions or rpc("eth_getTransactionCount", [OWNER,"latest"]) != before:
                raise RuntimeError("advanced refusal reached a signing or submission boundary")
            state["refused"] = True
            print(json.dumps({"chain":chain,"refused":kind,"signatures":len(calls),"submissions":len(submissions)}),flush=True)
            await asyncio.get_running_loop().create_future()
        finally:
            try:
                if state.get("refused"):
                    await send(peer,"continue")
            finally:
                server.shutdown()
                server.server_close()
                thread.join()


async def native_allowance_case(start, node, peer_source, url, directory, tls, cert, anvil, port=18483):
    """One installed wallet, one real SDK session, and two separate state transitions."""
    artifact = HERE.parents[2] / "p06-stablenet-contracts/out/TestUSDC.sol/TestUSDC.json"
    code = json.loads(artifact.read_text())["bytecode"]["object"]
    with anvil_node(anvil) as rpc:
        genesis = await chain_ready(rpc)
        deployment = rpc("eth_sendTransaction", [{"from":OWNER, "data":code+OWNER[2:].rjust(64,"0"), "gas":"0x500000"}])
        rpc("anvil_mine", ["0x1"])
        contract = rpc("eth_getTransactionReceipt", [deployment])["contractAddress"].lower()
        initial = rpc("eth_sendTransaction", [{"from":OWNER, "to":contract, "gas":"0x30000",
            "data":"0x095ea7b3"+SPENDER[2:].rjust(64,"0")+format(42,"064x")}])
        rpc("anvil_mine", ["0x82"])
        assert rpc("eth_getTransactionReceipt", [initial])["status"] == "0x1"
        allowance = {"to":contract, "data":"0xdd62ed3e"+OWNER[2:].rjust(64,"0")+SPENDER[2:].rjust(64,"0")}
        nonce = int(rpc("eth_getTransactionCount", [OWNER,"latest"]),16)
        assert int(rpc("eth_call", [allowance,"finalized"]),16) == 42
        peer = await start(node,peer_source,url,"user-disconnect")
        first = await event(peer,"pair")
        state = {"first":first["uri"], "spender":SPENDER, "submitted":0}
        finished = asyncio.Event()
        loop = asyncio.get_running_loop()

        def submitted(hash_, calls):
            assert state.get("disconnected") and len(calls) == 1 and calls[0][0] == 14
            assert int(rpc("eth_getTransactionCount", [OWNER,"latest"]),16) == nonce+1
            receipt = rpc("eth_getTransactionReceipt", [hash_])
            assert receipt["status"] == "0x1" and receipt["transactionHash"] == hash_
            approval = "0x"+keccak256(b"Approval(address,address,uint256)").hex()
            assert [log["topics"][0] for log in receipt["logs"]] == [approval]
            rpc("anvil_mine", ["0x82"])
            assert int(rpc("eth_call", [allowance,"finalized"]),16) == 0
            state.update(receipt=hash_,submitted=1)
            loop.call_soon_threadsafe(finished.set)

        config = {"relay":url,"cert":base64.b64encode(ssl.PEM_cert_to_DER_cert(cert.read_text())).decode(),
                  "case":state,"revoke_token":contract,"submitted":submitted,"port":port}
        server, thread, _, calls = fixture(directory,rpc,genesis,None,None,tls,config)
        try:
            print(f"Native W15 fixture ready on https://127.0.0.1:{port}",flush=True)
            read = await event(peer,"read",180)
            assert read["denied"] and read["accountCount"] == 1
            state["readPassed"] = True
            await send(peer,"continue")
            closed = await event(peer,"disconnected")
            assert closed["activeSessions"] == 0 and closed["deniedAfterEnd"]
            assert int(rpc("eth_call", [allowance,"finalized"]),16) == 42
            assert int(rpc("eth_getTransactionCount", [OWNER,"latest"]),16) == nonce and not calls
            state["disconnected"] = True
            await asyncio.wait_for(finished.wait(), 180)
            print(json.dumps({"reference":VERSION,"activeSessions":0,"newRequestDenied":True,
                              "allowanceAfterDisconnect":42,"boardApprovals":1,"submissions":1,"finalizedAllowance":0}),flush=True)
            await asyncio.get_running_loop().create_future()
        finally:
            server.shutdown()
            server.server_close()
            thread.join()


async def native_positive_allowance_case(start, node, peer_source, url, relay, directory, tls, cert, anvil, gateway):
    artifact = HERE.parents[2] / "p06-stablenet-contracts/out/TestUSDC.sol/TestUSDC.json"
    code = json.loads(artifact.read_text())["bytecode"]["object"]
    with anvil_node(anvil) as rpc:
        genesis = await chain_ready(rpc)
        deployment = rpc("eth_sendTransaction", [{"from":OWNER,"data":code+OWNER[2:].rjust(64,"0"),"gas":"0x500000"}])
        rpc("anvil_mine", ["0x82"])
        contract = rpc("eth_getTransactionReceipt", [deployment])["contractAddress"].lower()
        allowance = {"to":contract,"data":"0xdd62ed3e"+OWNER[2:].rjust(64,"0")+SPENDER[2:].rjust(64,"0")}
        nonce = int(rpc("eth_getTransactionCount",[OWNER,"latest"]),16)
        assert int(rpc("eth_call",[allowance,"finalized"]),16)==0
        rpc("evm_setAutomine", [False])
        peer = await start(node,peer_source,url,"native-approve",contract)
        first = await event(peer,"pair")
        pairing = first["uri"].split("@",1)[0].removeprefix("wc:")
        state = {"first":first["uri"],"token":contract,"spender":SPENDER,"submitted":0,"queries":0}
        finalized = asyncio.Event()
        loop = asyncio.get_running_loop()

        def submitted(hash_, calls):
            assert not state["submitted"] and len(calls)==1 and calls[0][0]==36
            assert rpc("eth_getTransactionReceipt",[hash_]) is None
            assert int(rpc("eth_call",[allowance,"finalized"]),16)==0
            state.update(receipt=hash_,submitted=1)

        def queried(hash_):
            assert hash_==state["receipt"]
            state["queries"]+=1
            if state["queries"]!=1:
                return
            rpc("anvil_mine", ["0x82"])
            receipt = rpc("eth_getTransactionReceipt",[hash_])
            assert receipt["status"]=="0x1" and receipt["transactionHash"]==hash_
            assert int(rpc("eth_getTransactionCount",[OWNER,"latest"]),16)==nonce+1
            assert int(rpc("eth_call",[allowance,"finalized"]),16)==5000000
            loop.call_soon_threadsafe(finalized.set)

        config={"relay":url,"cert":base64.b64encode(ssl.PEM_cert_to_DER_cert(cert.read_text())).decode(),
                "case":state,"positive_token":contract,"gateway":gateway,"submitted":submitted,
                "observe_receipt":queried,"port":18483}
        server, thread, _, calls=fixture(directory,rpc,genesis,pairing,first["pub"],tls,config)
        try:
            print("Native W14 allowance fixture ready on https://127.0.0.1:18483",flush=True)
            viewed=await event(peer,"viewed",180)
            assert not calls and state["submitted"]==0
            state["viewed"]=True
            topic=viewed["topic"]
            async with asyncio.timeout(TIMEOUT):
                while len(relay.listening[topic])>=2:
                    await asyncio.sleep(.1)
                while len(relay.listening[topic])<2:
                    await asyncio.sleep(.1)
            await send(peer,"continue")
            tx=await event(peer,"tx",180)
            assert tx["hash"]==state["receipt"]
            state["peerReturnedOriginal"]=True
            await asyncio.wait_for(finalized.wait(),180)
            assert len(calls)==1 and state["submitted"]==1
            print(json.dumps({"reference":VERSION,"boardApprovals":1,"submissions":1,
                              "samePeerHash":True,"connectionViewRetiredReview":True,"finalizedAllowance":5000000}),flush=True)
            await asyncio.get_running_loop().create_future()
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=5)


async def native_session_management_case(start, node, peer_source, url, directory, tls, cert, anvil):
    artifact = HERE.parents[2] / "p06-stablenet-contracts/out/TestUSDC.sol/TestUSDC.json"
    code = json.loads(artifact.read_text())["bytecode"]["object"]
    with anvil_node(anvil) as rpc:
        genesis = await chain_ready(rpc)
        deployment = rpc("eth_sendTransaction", [{"from":OWNER,"data":code+OWNER[2:].rjust(64,"0"),"gas":"0x500000"}])
        rpc("anvil_mine", ["0x1"])
        contract = rpc("eth_getTransactionReceipt", [deployment])["contractAddress"].lower()
        approval = rpc("eth_sendTransaction", [{"from":OWNER,"to":contract,"gas":"0x30000",
            "data":"0x095ea7b3"+SPENDER[2:].rjust(64,"0")+format(5000000,"064x")}])
        rpc("anvil_mine", ["0x82"])
        assert rpc("eth_getTransactionReceipt", [approval])["status"]=="0x1"
        nonce = rpc("eth_getTransactionCount", [OWNER,"latest"])
        allowance = {"to":contract,"data":"0xdd62ed3e"+OWNER[2:].rjust(64,"0")+SPENDER[2:].rjust(64,"0")}
        assert int(rpc("eth_call",[allowance,"finalized"]),16)==5000000
        first = await start(node,peer_source,url,"manage-first")
        second = await start(node,peer_source,url,"manage-second")
        state = {"first":(await event(first,"pair"))["uri"],"second":(await event(second,"pair"))["uri"],
                 "signingAttempts":0,"submissions":0}
        config = {"relay":url,"cert":base64.b64encode(ssl.PEM_cert_to_DER_cert(cert.read_text())).decode(),
                  "case":state,"positive_token":contract,"deny_signing":True,"port":18483}
        server, thread, _, calls = fixture(directory,rpc,genesis,None,None,tls,config)
        try:
            print("Native W15 management fixture ready on https://127.0.0.1:18483",flush=True)
            first_read=await event(first,"read",180)
            assert first_read["accountCount"]==1 and first_read["denied"]
            state["firstRead"]=True
            second_read=await event(second,"read",180)
            assert second_read["accountCount"]==1 and second_read["denied"]
            state["twoAuthorized"]=True
            await send(first,"continue")
            one=await event(first,"disconnected",180)
            assert one["activeSessions"]==0 and one["deniedAfterEnd"]
            state["selectedClosed"]=True
            await send(second,"check")
            await event(second,"unselected-active",180)
            state["unselectedActive"]=True
            await send(second,"continue")
            all_=await event(second,"disconnected",180)
            assert all_["activeSessions"]==0 and all_["deniedAfterEnd"]
            assert rpc("eth_getTransactionCount",[OWNER,"latest"])==nonce
            assert int(rpc("eth_call",[allowance,"finalized"]),16)==5000000
            assert not calls and state["signingAttempts"]==state["submissions"]==0
            state["allClosed"]=True
            print(json.dumps({"reference":VERSION,"authorizedSessions":2,"selectedClosed":True,
                              "unselectedRemainedAuthorized":True,"allClosed":True,"signingAttempts":0,
                              "submissions":0,"finalizedAllowance":5000000}),flush=True)
            await asyncio.get_running_loop().create_future()
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=5)


async def native_explore_case(start, node, peer_source, url, directory, tls, cert, anvil):
    with anvil_node(anvil) as rpc:
        genesis = await chain_ready(rpc)
        before = rpc("eth_getTransactionCount", [OWNER, "latest"])
        peer = await start(node, peer_source, url, "native-explore")
        state = {"first": (await event(peer, "pair"))["uri"], "signingAttempts": 0, "submissions": 0}
        config = {"relay": url, "case": state, "deny_signing": True, "explore_site": True, "port": 18483,
                  "cert": base64.b64encode(ssl.PEM_cert_to_DER_cert(cert.read_text())).decode()}
        server, thread, _, calls = fixture(directory, rpc, genesis, None, None, tls, config)
        try:
            print("Native W13 explore fixture ready", flush=True)
            read = await event(peer, "read", 170)
            assert read["accountCount"] == 1 and read["denied"]
            assert not calls and state["signingAttempts"] == state["submissions"] == 0
            assert rpc("eth_getTransactionCount", [OWNER, "latest"]) == before
            state["externalApproved"] = True
            print(json.dumps({"reference": VERSION, "externalApproved": True, "signingAttempts": 0, "submissions": 0}), flush=True)
            await asyncio.get_running_loop().create_future()
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=5)

async def native_browser_login_case(directory, tls, cert, anvil, gateway):
    with anvil_node(anvil) as rpc:
        genesis = await chain_ready(rpc)
        nonce = rpc("eth_getTransactionCount", [OWNER, "latest"])
        balance = rpc("eth_getBalance", [OWNER, "latest"])
        origin = "https://127.0.0.1:18483"
        chain = "0xaa36a7"
        state = {"site": origin + "/site", "submissions": 0}
        finished = asyncio.Event()
        loop = asyncio.get_running_loop()
        challenges = {}

        def browser_request(path, body, calls):
            assert body["account"] == OWNER
            if path == "/siwe/challenge":
                assert body == {"account": OWNER, "chain": chain} and not calls and len(challenges) < 2
                challenge = "8abc002" + str(len(challenges) + 2)
                message = config["login_message"](challenge, int(time.time())+70)
                challenges[challenge] = message
                return {"nonce": challenge, "message": message}
            if path == "/siwe/verify":
                assert set(body) == {"account","nonce","message","signature"} and len(calls)==1
                assert len(challenges)==2 and body["nonce"]=="8abc0023" and body["message"]==challenges["8abc0023"]
                assert cast("wallet","verify","--address",OWNER,body["message"],body["signature"])
                result = config["wallet"].handle({"jsonrpc":"2.0","id":1,"method":"nu54_siweVerify","params":[body]},origin)
                assert result=={"verified":True}
                state["accountVerified"]=True
                return result
            assert path=="/siwe/complete" and state["accountVerified"]
            assert body=={"account":OWNER,"chain":chain,"replayCode":4001}
            assert len(calls)==1 and calls[0][0]==15 and state["submissions"]==0
            assert rpc("eth_getTransactionCount",[OWNER,"latest"])==nonce
            assert rpc("eth_getBalance",[OWNER,"latest"])==balance
            state["finished"]=True
            loop.call_soon_threadsafe(finished.set)
            return {"complete":True}

        config={"relay":"wss://127.0.0.1","case":state,"login":True,"browser_login":True,
                "browser_request":browser_request,"gateway":gateway,"port":18483,
                "cert":base64.b64encode(ssl.PEM_cert_to_DER_cert(cert.read_text())).decode()}
        server,thread,_,_=fixture(directory,rpc,genesis,None,None,tls,config)
        try:
            print("Native W14 browser login fixture ready",flush=True)
            await asyncio.wait_for(finished.wait(),140)
            print(json.dumps({"nonce":"8abc0023","accountVerified":True,"accountScopeUnchanged":True,
                              "replayRefused":True,"boardApprovals":1,"submissions":0,"onChainApprovals":0}),flush=True)
            await asyncio.get_running_loop().create_future()
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=5)


async def native_login_case(url, relay, directory, tls, cert, anvil, gateway):
    with anvil_node(anvil) as rpc:
        genesis = await chain_ready(rpc)
        owner_nonce = rpc("eth_getTransactionCount",[OWNER,"latest"])
        owner_balance = rpc("eth_getBalance",[OWNER,"latest"])
        key, pairing, uri = pairing_data()
        private = X25519PrivateKey.generate()
        public = private.public_key().public_bytes_raw().hex()
        state = {"first":uri,"submissions":0}
        config = {"relay":url,"case":state,"login":True,"gateway":gateway,"port":18483,
                  "cert":base64.b64encode(ssl.PEM_cert_to_DER_cert(cert.read_text())).decode()}
        server,thread,_,calls = fixture(directory,rpc,genesis,pairing,public,tls,config)
        try:
            ssl_client=ssl.create_default_context(cafile=cert)
            async with connect(url,ssl=ssl_client) as stream:
                peer=Dapp(stream)
                await peer.rpc("irn_subscribe",{"topic":pairing})
                print("Native W14 login fixture ready",flush=True)
                async with asyncio.timeout(100):
                    while len(relay.listening[pairing]) < 2:
                        await asyncio.sleep(.1)
                    chain="eip155:11155111"
                    await peer.publish(pairing,1100,key,{"id":201,"method":"wc_sessionPropose","params":{
                        "relays":[{"protocol":"irn"}],"proposer":{"publicKey":public,"metadata":{"url":"https://independent.example","name":"Local login peer"}},
                        "requiredNamespaces":{"eip155":{"chains":[chain],"methods":["eth_accounts","personal_sign"],"events":[]}}}})
                    reply=(await peer.receive(1101,key))["result"]
                    shared=private.exchange(X25519PublicKey.from_public_bytes(bytes.fromhex(reply["responderPublicKey"])))
                    secret=HKDF(algorithm=hashes.SHA256(),length=32,salt=None,info=b"").derive(shared)
                    topic=hashlib.sha256(secret).hexdigest()
                    await peer.rpc("irn_subscribe",{"topic":topic})
                    settle=await peer.receive(1102,secret)
                    assert settle["params"]["namespaces"]["eip155"]["accounts"]==[chain+":"+OWNER]
                    await peer.publish(topic,1103,secret,{"id":settle["id"],"result":True})

                    async def request(number,message):
                        await peer.publish(topic,1108,secret,{"id":number,"method":"wc_sessionRequest","params":{
                            "chainId":chain,"request":{"method":"personal_sign","params":["0x"+message.encode().hex(),OWNER]}}})
                        response=await peer.receive(1109,secret)
                        assert response["id"]==number
                        await asyncio.sleep(.3)
                        return response

                    expires=int(time.time())+70
                    early=config["login_message"]("8abc0022",expires)
                    original=config["login_message"]("8abc0023",expires)
                    expired=config["login_message"]("8abc0000",int(time.time())-1)
                    assert (await request(202,early.replace("8abc0022","8abc0099")))["error"]
                    assert (await request(203,expired))["error"]
                    assert not calls
                    assert (await request(204,early))["error"]
                    assert not calls
                    state["refused"]=True
                    signature=(await request(205,original))["result"]
                    assert (await request(206,original))["error"]
                    assert len(calls)==1 and calls[0][0]==15
                    assert cast("wallet","verify","--address",OWNER,original,signature)
                    signed={"account":OWNER,"nonce":"8abc0023","message":original,"signature":signature}
                    envelope={"jsonrpc":"2.0","id":1,"method":"nu54_siweVerify","params":[signed]}
                    assert config["wallet"].handle(envelope,"https://independent.example")=={"verified":True}
                    try:
                        config["wallet"].handle(envelope,"https://independent.example")
                    except ValueError:
                        pass
                    else:
                        raise AssertionError("Verified login was reusable")
                    assert state["submissions"]==0
                    assert rpc("eth_getTransactionCount",[OWNER,"latest"])==owner_nonce
                    assert rpc("eth_getBalance",[OWNER,"latest"])==owner_balance
                    state["verified"]=True
                    print(json.dumps({"nonce":"8abc0023","accountVerified":True,"replayRefused":True,
                        "boardApprovals":1,"submissions":0,"onChainApprovals":0}),flush=True)
                await asyncio.get_running_loop().create_future()
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=5)


async def native_expiry_case(start, node, peer_source, url, relay, directory, tls, cert, anvil):
    with anvil_node(anvil) as rpc:
        genesis = await chain_ready(rpc)
        nonce = rpc("eth_getTransactionCount", [OWNER, "latest"])
        before = rpc("eth_getBalance", [SPENDER, "latest"])
        first = await start(node, peer_source, url, "native-expiry")
        initial = await event(first, "pair")
        pairing = initial["uri"].split("@", 1)[0].removeprefix("wc:")
        second = await start(node, peer_source, url, "expiry-fresh")
        fresh = await event(second, "pair")
        state = {"first": initial["uri"], "second": fresh["uri"], "signingAttempts": 0, "submissions": 0}
        config = {"relay": url, "case": state, "proof_lifetime": 100, "deny_signing": True, "port": 18483,
                  "cert": base64.b64encode(ssl.PEM_cert_to_DER_cert(cert.read_text())).decode()}
        server, thread, _, calls = fixture(directory, rpc, genesis, pairing, initial["pub"], tls, config)
        try:
            print("Native W13 expiry fixture ready on https://127.0.0.1:18483", flush=True)
            linked = await event(first, "initial-read", 140)
            state["firstRead"] = True
            async with asyncio.timeout(90):
                while len(relay.listening[linked["topic"]]) >= 2:
                    await asyncio.sleep(.05)
                while len(relay.listening[linked["topic"]]) < 2:
                    await asyncio.sleep(.05)
            await send(first, "resume")
            pending = await event(first, "awaiting-expiry", 140)
            assert pending["expiry"] > int(time.time()) and len(relay.listening[pending["topic"]]) >= 2
            state["expires"] = pending["expiry"]
            async with asyncio.timeout(140):
                while time.time() < pending["expiry"] or len(relay.listening[pending["topic"]]) >= 2:
                    await asyncio.sleep(.1)
            await send(first, "check")
            await event(first, "expired", 30)
            state["oldSessionDenied"] = True
            read = await event(second, "read", 100)
            assert read["accountCount"] == 1 and read["denied"]
            state["freshScopeApproved"] = True
            async with asyncio.timeout(90):
                while len(relay.listening[read["topic"]]) >= 2:
                    await asyncio.sleep(.05)
                while len(relay.listening[read["topic"]]) < 2:
                    await asyncio.sleep(.05)
            await send(second, "resume")
            await event(second, "fresh-resumed", 30)
            assert not calls and state["signingAttempts"] == state["submissions"] == 0
            assert rpc("eth_getTransactionCount", [OWNER, "latest"]) == nonce
            assert rpc("eth_getBalance", [SPENDER, "latest"]) == before
            state["freshResumedAfterRestart"] = True
            print(json.dumps({"reference": VERSION, "expiredSessionDenied": True, "freshScopeApproved": True,
                              "freshResumedAfterRestart": True, "signingAttempts": 0, "submissions": 0}), flush=True)
            await asyncio.get_running_loop().create_future()
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=5)


async def main(options):
    if sys.platform != "darwin":
        raise RuntimeError("This runner requires a loopback-only macOS sandbox")
    task = asyncio.current_task()
    asyncio.get_running_loop().add_signal_handler(signal.SIGTERM, task.cancel)
    os.umask(0o077)
    with tempfile.TemporaryDirectory(prefix="nu54-walletconnect-reference-") as path:
        directory = Path(path)
        peer = None if options.native_login or options.native_browser_login else provision(directory,options.npm)
        native = (options.native or options.native_allowance or options.native_positive_allowance or
                  options.native_session_management or options.native_expiry or options.native_explore or
                  options.native_login or options.native_browser_login or options.native_refusal)
        cert, key = native if native else (directory / "local.pem", directory / "local.key")
        if not native:
            subprocess.run(["openssl","req","-x509","-newkey","rsa:2048","-nodes","-days","1",
                "-subj","/CN=localhost","-addext","subjectAltName=IP:127.0.0.1","-keyout",key,"-out",cert],
                check=True,capture_output=True)
        tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        tls.load_cert_chain(cert,key)
        if options.native_browser_login:
            await native_browser_login_case(directory,tls,cert,options.anvil,options.gateway)
            return
        relay = Relay(defer_rejection=False)
        async with serve(relay.accept,"127.0.0.1",0,ssl=tls) as listener, children(directory,cert) as start:
            url = f"wss://127.0.0.1:{listener.sockets[0].getsockname()[1]}"
            if options.native_allowance:
                await native_allowance_case(start,options.node,peer,url,directory,tls,cert,options.anvil)
            elif options.native_positive_allowance:
                await native_positive_allowance_case(start,options.node,peer,url,relay,directory,tls,cert,options.anvil,options.gateway)
            elif options.native_session_management:
                await native_session_management_case(start,options.node,peer,url,directory,tls,cert,options.anvil)
            elif options.native_expiry:
                await native_expiry_case(start,options.node,peer,url,relay,directory,tls,cert,options.anvil)
            elif options.native_explore:
                await native_explore_case(start,options.node,peer,url,directory,tls,cert,options.anvil)
            elif options.native_login:
                await native_login_case(url,relay,directory,tls,cert,options.anvil,options.gateway)
            elif options.native_refusal:
                await native_refusal_case(start,options.node,peer,url,directory,tls,cert,options.anvil,options.refusal_kind)
            elif options.native:
                await native_case(start,options.node,peer,url,relay,directory,tls,cert,options.anvil)
            else:
                result = {"reference":"@walletconnect/sign-client@" + VERSION}
                result["read"] = await read_case(start,options.node,peer,url,directory)
                result["spend"] = await spend_case(start,options.node,peer,url,directory,tls,options.anvil)
                result["spend"].pop("txHash")
                print(json.dumps(result),flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--node", default="node")
    parser.add_argument("--npm", default="npm")
    parser.add_argument("--anvil", default="anvil")
    native_modes = parser.add_mutually_exclusive_group()
    native_modes.add_argument("--native", nargs=2, type=Path, metavar=("CERT","KEY"))
    native_modes.add_argument("--native-allowance", nargs=2, type=Path, metavar=("CERT","KEY"))
    native_modes.add_argument("--native-positive-allowance", nargs=2, type=Path, metavar=("CERT","KEY"))
    native_modes.add_argument("--native-session-management", nargs=2, type=Path, metavar=("CERT","KEY"))
    native_modes.add_argument("--native-expiry", nargs=2, type=Path, metavar=("CERT","KEY"))
    native_modes.add_argument("--native-explore", nargs=2, type=Path, metavar=("CERT","KEY"))
    native_modes.add_argument("--native-login", nargs=2, type=Path, metavar=("CERT","KEY"))
    native_modes.add_argument("--native-browser-login", nargs=2, type=Path, metavar=("CERT","KEY"))
    parser.add_argument("--gateway", type=Path)
    native_modes.add_argument("--native-refusal", nargs=2, type=Path, metavar=("CERT","KEY"))
    parser.add_argument("--refusal-kind",choices=("nft","batch"),default="nft")
    try:
        asyncio.run(main(parser.parse_args()))
    except (asyncio.CancelledError, KeyboardInterrupt):
        pass
