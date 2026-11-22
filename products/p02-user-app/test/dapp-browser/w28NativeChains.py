"""Installed P02 non-EVM approvals against fresh disconnected reference ledgers."""
import argparse
import base64
from contextlib import contextmanager, redirect_stdout
from decimal import Decimal
from io import StringIO
import hmac
import json
import os
from pathlib import Path
import signal
import ssl
import tempfile
import time
from types import SimpleNamespace

from wallet_connect_spend import BOARD, P01, TOKEN, fixture
from w07NativeReceive import chain_public
from run_wallet_chain_nodes import FirmwareCore, XRPL_IMAGE, available_port, b58, command, daemon, eventual, lab, ripple

NAMES = ("Bitcoin", "Solana", "XRP Ledger")
SUBMIT = {1:"sendrawtransaction",2:"sendTransaction",3:"submit"}
LOOKUP = {1:"getrawtransaction",2:"getSignatureStatuses",3:"tx"}


@contextmanager
def bitcoin(binary, scratch, core):
    port = available_port()
    root = f"http://127.0.0.1:{port}"
    argv = [binary,"-regtest",f"-datadir={scratch}",f"-rpcport={port}","-networkactive=0","-listen=0","-dnsseed=0",
            "-txindex=1","-rpcbind=127.0.0.1","-rpcallowip=127.0.0.1","-fallbackfee=0.00001"]
    with daemon(argv,scratch/"node.log"):
        cookie=scratch/"regtest/.cookie"
        eventual(cookie.exists)
        token=cookie.read_text().strip()
        node=lab.rpc_client(root,token)
        eventual(lambda:node("getblockchaininfo",[]))
        node("createwallet",{"wallet_name":"miner"})
        node("createwallet",{"wallet_name":"watch","disable_private_keys":True,"blank":True})
        miner=lab.rpc_client(root+"/wallet/miner",token,timeout=60)
        watch=lab.rpc_client(root+"/wallet/watch",token)
        owner=core.account(1,0)[0].hex()
        descriptor=node("getdescriptorinfo",[f"wpkh({owner})"])["descriptor"]
        address,=node("deriveaddresses",[descriptor])
        assert watch("importdescriptors",[[{"desc":descriptor,"timestamp":"now"}]])[0]["success"]
        destination=miner("getnewaddress",[])
        miner("generatetoaddress",[101,destination])
        for _ in range(2):
            miner("sendtoaddress",{"address":address,"amount":0.006,"fee_rate":2})
        miner("generatetoaddress",[6,destination])

        def verify(txid):
            miner("generatetoaddress",[6,destination])
            receipt=node("getrawtransaction",[txid,True])
            assert receipt["txid"]==txid and receipt["confirmations"]>=6
            assert sum(row["value"] for row in receipt["vout"] if row["scriptPubKey"].get("address")==destination)==Decimal("0.01")
            return {"confirmations":receipt["confirmations"],"sats":1_000_000}

        yield watch,node("getblockhash",[0]),destination,"0.01",verify


@contextmanager
def solana(binary, scratch, core, slow=False):
    port=available_port()
    payer=b58(core.account(2,0)[0])
    destination=b58(core.account(2,1)[0])
    argv=[binary,"--ledger",scratch/"ledger","--reset","--bind-address","127.0.0.1","--rpc-port",str(port),
          "--faucet-port",str(available_port()),"--gossip-port",str(available_port()),"--mint",payer,"--log"]
    if slow:
        argv += ["--ticks-per-slot", "128"]
    with daemon(argv,scratch/"node.log"):
        rpc=lab.rpc_client(f"http://127.0.0.1:{port}",None)
        anchor=eventual(lambda:rpc("getGenesisHash",[]),90)
        eventual(lambda:rpc("getBlockHeight",[{"commitment":"confirmed"}])>0,90)

        def verify(txid):
            def finalized():
                state=rpc("getSignatureStatuses",[[txid],{"searchTransactionHistory":True}])["value"][0]
                return state and state["confirmationStatus"]=="finalized" and state["err"] is None
            eventual(finalized,90)
            assert rpc("getBalance",[destination,{"commitment":"finalized"}])["value"]==10_000_000
            return {"lamports":10_000_000}

        yield rpc,anchor,destination,"0.01",verify


@contextmanager
def xrpl(_binary, scratch, core, network=1):
    config=scratch/"xrpld.cfg"
    config.write_text("[server]\nlocal\n[local]\nport=5005\nip=127.0.0.1\nadmin=127.0.0.1\nprotocol=http\n"
                      "[node_db]\ntype=NuDB\npath=/tmp/xrpl-data/nudb\n[database_path]\n/tmp/xrpl-data/sqlite\n"
                      "[debug_logfile]\n/tmp/xrpl-data/debug.log\n[network_id]\n1\n")
    name="nu54-native-"+scratch.name.lower()
    command(["docker","run","--detach","--rm","--platform","linux/amd64","--pull","never","--network","none",
             "--name",name,"-v",str(config)+":/tmp/local.cfg:ro","--entrypoint","/usr/bin/xrpld",XRPL_IMAGE,
             "--conf","/tmp/local.cfg","--standalone","--start"])
    try:
        def rpc(method,params,relay=False):
            payload,=params
            result=json.loads(command(["docker","exec",name,"/usr/bin/xrpld","--silent","--conf","/tmp/local.cfg",
                                       "json",method,json.dumps(payload)]))["result"]
            return {"result":result} if relay else result

        eventual(lambda:rpc("server_info",[{}]).get("info"),90)
        destination=ripple(core.account(3,1)[1])
        master="rHb9CJAWyB4rj91VRWn96DkukG4bwdtyTh"
        for address in (ripple(core.account(3,0,network=network)[1]),destination):
            funded=rpc("submit",[{"secret":"masterpassphrase","tx_json":{"TransactionType":"Payment",
                       "Account":master,"Destination":address,"Amount":"100000000"}}])
            assert funded["engine_result"]=="tesSUCCESS"
            rpc("ledger_accept",[{}])

        def verify(txid):
            rpc("ledger_accept",[{}])
            result=rpc("tx",[{"transaction":txid,"binary":False,"api_version":1}])
            assert result["validated"] and result["hash"]==txid and result["Destination"]==destination
            assert result["DestinationTag"]==0 and result["meta"]["delivered_amount"]=="5000000"
            return {"drops":5_000_000,"destinationTag":0}

        yield rpc,"1",destination,"5",verify
    finally:
        command(["docker","stop","-t","2",name])


def commissioned_node(chain, node, destination):
    from wallet_chain_review import b58 as encode58, segwit
    anchor = lab.COMMISSIONED[chain]
    local_destination = destination
    candidate = destination
    if chain == 1:
        script = bytes.fromhex(node("validateaddress", [destination])["scriptPubKey"])
        candidate = segwit(script, "tb")
    if chain == 3:
        data = bytes((4,147)) + lab.xrpl_id(destination) + bytes((1,)) + bytes(8)
        candidate = encode58(data + lab.d256(data)[:4], lab.RIPPLE)

    def isolated(method, params, relay=False):
        if chain == 1 and method == "getblockchaininfo":
            result = {**node(method,params),"chain":"signet"}
        elif chain == 1 and method == "getblockhash" and params in ([0],[321295]):
            result = anchor.split(":")[int(params[0] == 321295)]
        elif chain == 1 and method == "validateaddress" and params == [candidate]:
            result = node(method,[local_destination])
        elif chain == 2 and method == "getGenesisHash":
            result = anchor
        elif chain == 3 and method == "ledger" and params[0].get("ledger_hash") == anchor.split(":")[1]:
            result = {"validated":True,"ledger_hash":anchor.split(":")[1],"ledger_index":22470370}
        else:
            return node(method, params, relay=relay)
        return {"result":result} if relay else result

    return isolated,anchor,candidate


def main(options):
    chain=options.chain
    core=FirmwareCore(options.oracle)
    tls=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    tls.load_cert_chain(options.cert, options.key)
    cert=base64.b64encode(ssl.PEM_cert_to_DER_cert(options.cert.read_text())).decode()
    with tempfile.TemporaryDirectory(prefix="w28native-") as path:
        scratch=Path(path)
        factory,binary=((bitcoin,options.bitcoind),(solana,options.validator),(xrpl,None))[chain-1]
        network=2 if options.commissioned else 1
        arguments = (binary,scratch,core,network) if chain==3 else (binary,scratch,core,options.hold_finality) if chain==2 else (binary,scratch,core)
        with factory(*arguments) as (node,anchor,destination,amount,verify):
            scope = None
            if options.commissioned:
                node,anchor,destination = commissioned_node(chain,node,destination)
                route = ("/bitcoin","/solana","/xrpl")[chain-1]
                endpoint = "https://wallet.localhost:18449" + route
                scope = {"chain":chain,"profile":("bitcoin-signet","solana-devnet","xrpl-testnet")[chain-1],"rpc":endpoint,"anchor":anchor}
                token = hmac.digest(bytes.fromhex(TOKEN),("N54P\n"+endpoint).encode(),"sha256").hex()
            control={"chain":chain,"name":NAMES[chain-1],"anchor":anchor,"destination":destination,"amount":amount,
                     "signatures":0,"submissions":0,"requeries":0,"commissioned":bool(scope),"holdFinality":options.hold_finality}
            config=SimpleNamespace(board=BOARD,generation=1,lease_revision=1,review_dir=scratch,
                spl_mint=None,spl_decimals=6,xrpl_network=1,xrpl_currency=None,xrpl_issuer=None,
                wall_start=int(time.time()),mono_start=time.monotonic(),
                commissioned_token_file=bool(scope),commissioned_expires=int(time.time())+285)
            reviewed=None
            authorized=None

            def sign(wire):
                nonlocal authorized
                if options.staking_refusal:
                    raise ValueError("staking refusal fixture cannot sign")
                if wire != reviewed or control["signatures"] or wire[1]!=chain or int.from_bytes(wire[7:15],"big")<=int(time.time()):
                    raise ValueError("Unexpected native signing request")
                authorized=core.sign(wire)
                control["signatures"]+=1
                return authorized

            def exchange(packet,key):
                nonlocal reviewed
                if packet.get("id")!=1 or packet.get("jsonrpc")!="2.0":
                    raise ValueError("Unexpected RPC envelope")
                method,params=packet["method"],packet["params"]
                if method in {"nu54_walletTime", "nu54_prepareWalletTransfer", LOOKUP[chain], SUBMIT[chain],
                              "server_info", "ledger_current", "isBlockhashValid", "getBlockHeight", "getGenesisHash", "nu54_submitCommissionedTransfer"}:
                    print("Wallet request",method,flush=True)
                config.authority=key
                if scope:
                    if not hmac.compare_digest(packet.get("walletToken", ""), token):
                        raise ValueError("Unexpected commissioned credential")
                    lab.verify_commissioned_node(chain,node)
                    if method=="nu54_submitCommissionedTransfer":
                        request,=params
                        if authorized is None or request["signed"]!=authorized.hex() or control["submissions"]:
                            raise ValueError("Unexpected native submission")
                        txid=lab.submit_commissioned(config,chain,node,request)
                        control.update(submissions=1,receipt=txid)
                        return {"jsonrpc":"2.0","id":1,"result":txid}
                    if method==SUBMIT[chain]:
                        raise ValueError("Direct commissioned submit is forbidden")
                if method=="nu54_prepareWalletTransfer":
                    request,=params
                    assert request["chain"]==chain and request["anchor"]==anchor and request["destination"]==(destination if chain!=3 or not scope else lab.xrpl_destination(destination,None,True)[1])
                    with redirect_stdout(StringIO()):
                        grant=lab.prepare(config,node,request)
                    reviewed=bytes.fromhex(grant["grant"])
                    if options.staking_refusal:
                        payer=bytes.fromhex(request["publicKey"])
                        peer=lab.b58decode(destination)
                        program=lab.b58decode("Stake11111111111111111111111111111111111111")
                        raw=bytes.fromhex("01000507")+payer+peer+b"".join(bytes([value])*32 for value in (61,62,63,64))
                        raw+=program+reviewed[23:55]+bytes.fromhex("0106060102030405000402000000")
                        header=reviewed[:153]
                        statement=b"N54M"+bytes.fromhex(BOARD)+int(request["generation"]).to_bytes(8,"big")
                        grant={"grant":(header+lab.sign(key,statement+header+lab.h256(raw))+raw).hex()}
                        reviewed=bytes.fromhex(grant["grant"])
                        control["stakingRefusal"]=True
                    result={"result":grant}
                elif method=="nu54_walletTime":
                    result={"result":lab.trusted_submission_time(config,params[0])}
                elif method in lab.METHODS[chain]:
                    if method==SUBMIT[chain]:
                        submitted=(bytes.fromhex(params[0]) if chain==1 else base64.b64decode(params[0],validate=True)
                                   if chain==2 else bytes.fromhex(params[0]["tx_blob"]))
                        if authorized is None or submitted!=authorized or control["submissions"]:
                            raise ValueError("Unexpected native submission")
                    result=node(method,params,relay=True)
                    if chain==1 and method==LOOKUP[chain] and result.get("error"):
                        print(json.dumps({"lookupError":result["error"]["code"],"hasResult":"result" in result}),flush=True)
                    if method==SUBMIT[chain]:
                        received=result["result"]
                        txid=received["tx_json"]["hash"] if chain==3 else received
                        control["submissions"]=1
                        control["receipt"]=txid
                    if method==LOOKUP[chain] and control.get("receipt"):
                        query=params[0]["transaction"] if chain==3 else params[0][0] if chain==2 else params[0]
                        assert query==control["receipt"]
                        control["requeries"]+=1
                        if options.hold_finality and control["requeries"]==1:
                            data = result.get("result")
                            if chain==1:
                                pending = isinstance(data,dict) and data.get("txid")==query and data.get("confirmations",0)<6
                            elif chain==2:
                                status = data["value"][0]
                                pending = status is None or status["confirmationStatus"]!="finalized"
                            else:
                                pending = data.get("validated") is not True or data.get("error")=="txnNotFound"
                            control["pendingObserved"] = bool(pending)
                else:
                    raise ValueError("Unexpected native RPC")
                return {"jsonrpc":"2.0","id":1,**lab.wire_value(result)}

            # The common profile transport supplies only enrollment and native identity.
            # The non-EVM RPC anchor and all approval evidence belong to the selected node.
            def public(wire):
                print("Board public account",wire[1],wire[10],flush=True)
                return chain_public(core,wire,network)

            native={"cert":cert,"relay":"wss://127.0.0.1:18449","case":control,
                    "chain_public":public,"chain_sign":sign,"chain_rpc":exchange,
                    "commissioned":scope,"chain_route":route if scope else None}
            server,worker,_,_=fixture(scratch,node,"0x"+"12"*32,None,None,tls,native)
            try:
                print("Non-EVM fixture ready",flush=True)
                if options.staking_refusal:
                    eventual(lambda:control.get("stakingRefusal"),120)
                    print(json.dumps({"chain":chain,"stakingRefusal":True,"signatures":control["signatures"],
                                      "submissions":control["submissions"]}),flush=True)
                    while True:
                        time.sleep(1)
                txid=eventual(lambda:control.get("receipt"),240)
                if options.hold_finality:
                    eventual(lambda:control.get("pendingObserved") is not None,90)
                    assert control["pendingObserved"]
                proof=verify(txid)
                control["finalized"]=True
                eventual(lambda:control["requeries"]>=(2 if options.hold_finality else 1),100)
                print(json.dumps({"chain":chain,"signatures":1,"submissions":1,"originalIdRequeried":True,"priorPendingObserved":control.get("pendingObserved"),**proof}),flush=True)
                while True:
                    time.sleep(1)
            finally:
                server.shutdown()
                server.server_close()
                worker.join()


def interrupt(_signal, _frame):
    raise KeyboardInterrupt


if __name__=="__main__":
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("chain",type=int,choices=(1,2,3))
    parser.add_argument("cert",type=Path)
    parser.add_argument("key",type=Path)
    parser.add_argument("--commissioned",action="store_true")
    parser.add_argument("--hold-finality",action="store_true")
    parser.add_argument("--staking-refusal",action="store_true")
    parser.add_argument("--oracle",type=Path,default=P01/"out/host/libwallet_chain_node_oracle.so")
    parser.add_argument("--bitcoind",type=Path)
    parser.add_argument("--validator",type=Path)
    options=parser.parse_args()
    if options.chain == 1 and not options.bitcoind:
        parser.error("Bitcoin lane requires --bitcoind")
    if options.chain == 2 and not options.validator:
        parser.error("Solana lane requires --validator")
    if options.staking_refusal and (options.chain!=2 or not options.commissioned or options.hold_finality):
        parser.error("staking refusal requires the commissioned Solana lane")
    os.umask(0o077)
    signal.signal(signal.SIGTERM,interrupt)
    try:
        main(options)
    except KeyboardInterrupt:
        pass
