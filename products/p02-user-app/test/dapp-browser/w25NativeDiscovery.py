"""Run W25 historical receive/change reads with actual disconnected Bitcoin Core."""
import argparse
import base64
import ctypes as C
import json
import os
from pathlib import Path
import signal
import ssl
import tempfile
import time

from wallet_connect_spend import BOARD, P01, fixture
from run_wallet_chain_nodes import B, P, SEED, FirmwareCore, available_port, b58, buffer, daemon, eventual, lab
from wallet_chain_review import segwit

ACCOUNT = 3
PORT = 18525
ENDPOINT = f"https://127.0.0.1:{PORT}/chain-node"


def main(options):
    core = FirmwareCore(options.oracle)
    core.lib.chain_host_watch.argtypes = [P, C.c_uint32, P]
    exported = (B * 80)()
    assert not core.lib.chain_host_watch(buffer(SEED), ACCOUNT, exported)
    public = bytes(exported)
    root = bytes.fromhex("043587cf03")
    serialized = root + public[11:15] + (0x80000000 + ACCOUNT).to_bytes(4, "big") + public[15:80]
    tpub = b58(serialized + lab.d256(serialized)[:4])
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(options.cert, options.key)
    cert = base64.b64encode(ssl.PEM_cert_to_DER_cert(options.cert.read_text())).decode()

    with tempfile.TemporaryDirectory(prefix="nu54-w25-native-") as directory:
        scratch = Path(directory)
        port = available_port()
        address = f"http://127.0.0.1:{port}"
        argv = [options.bitcoind,"-regtest",f"-datadir={scratch}",f"-rpcport={port}", "-networkactive=0",
                "-listen=0","-dnsseed=0","-rpcbind=127.0.0.1","-fallbackfee=0.00001"]
        with daemon(argv, scratch / "node.log"):
            cookie=scratch/"regtest/.cookie"
            eventual(cookie.exists)
            credential=cookie.read_text().strip()
            node=lab.rpc_client(address,credential)
            eventual(lambda:node("getblockchaininfo",[]))
            node("createwallet",{"wallet_name":"miner"})
            node("createwallet",{"wallet_name":"watch","disable_private_keys":True,"blank":True})
            miner=lab.rpc_client(address+"/wallet/miner",credential,timeout=60)
            watch=lab.rpc_client(address+"/wallet/watch",credential)
            local = {}
            selected = {}
            for branch, used in ((0,3),(1,5)):
                desc=node("getdescriptorinfo",[f"wpkh({tpub}/{branch}/*)"])["descriptor"]
                assert watch("importdescriptors",[[{"desc":desc,"range":[0,63],"timestamp":"now","internal":bool(branch)}]])[0]["success"]
                entries=node("deriveaddresses",[desc,[0,63]])
                for index,core_address in enumerate(entries):
                    program=bytes.fromhex(node("validateaddress",[core_address])["scriptPubKey"])
                    _,derived,_=core.bitcoin_public_at(ACCOUNT,branch,index)
                    assert program==b"\x00\x14"+derived
                    signet=segwit(program,"tb")
                    local[signet]=(branch,index,core_address)
                    if (branch,index) in ((0,0),(0,4),(branch,used)):
                        selected[(branch,index)]=signet
            payout=miner("getnewaddress",[])
            miner("generatetoaddress",[101,payout])
            for pair in ((0,3),(1,5)):
                miner("sendtoaddress",{"address":local[selected[pair]][2],"amount":0.01})
            miner("generatetoaddress",[6,payout])
            head=node("getblockchaininfo",[])
            checked=set()
            state={"account":ACCOUNT,"head":head["bestblockhash"],"height":head["blocks"],
                "initial":selected[0,0],"next":selected[0,4],"readCount":0,"exports":0,"signatures":0,"submissions":0}

            def capture(wire):
                if (len(wire) not in (11,15) or wire[:2]!=bytes([10,1]) or wire[2] not in (1,2) or
                    int.from_bytes(wire[3:7],"big")!=ACCOUNT or any(wire[7:10])):
                    raise ValueError("wrong W25 board account")
                index,branch=int.from_bytes(wire[11:15],"big"),wire[10]
                key,pkh,fingerprint=core.bitcoin_public_at(ACCOUNT,branch,index)
                return wire+key+pkh+fingerprint

            def capture_watch(wire):
                if wire!=public[:7]:
                    raise ValueError("wrong W25 descriptor scope")
                state["exports"]+=1
                return public

            def exchange(packet,_key):
                if tpub in str(packet):
                    raise ValueError("W25 must send individual addresses only")
                method,params=packet["method"],packet["params"]
                if method=="getblockchaininfo":
                    print("W25 verified head read",flush=True)
                    result={**node(method,params),"chain":"signet"}
                elif method=="getblockhash" and params in ([0],[321295]):
                    result=lab.COMMISSIONED[1].split(":")[int(params[0]==321295)]
                elif method in ("getaddressinfo","getreceivedbyaddress") and params[0] in local:
                    branch,index,core_address=local[params[0]]
                    if method=="getaddressinfo":
                        result={**watch(method,[core_address]),"address":params[0]}
                    else:
                        assert params[1:]==[1]
                        result=watch(method,[core_address,1])
                        checked.add((branch,index))
                        state["readCount"]=len(checked)
                        if len(checked) in (1,24,25):
                            print("W25 independently observed addresses",len(checked),flush=True)
                        if checked==({(0,i) for i in range(24)}|{(1,i) for i in range(26)}):
                            print(json.dumps({"receive":24,"change":26,"sameHeight":head["blocks"],
                                "signatures":0,"submissions":0}),flush=True)
                else:
                    if method.startswith(("send","submit","nu54_submit")):
                        state["submissions"]+=1
                    raise ValueError("W25 accepts read-only requests for the selected account")
                return {"jsonrpc":"2.0","id":1,"result":lab.wire_value(result)}

            def reject(*_):
                raise ValueError("W25 fixture denies signing")
            def reject_sign(*args):
                state["signatures"]+=1
                reject(*args)
            wallet_read={"selection":{"account":2,"id":"btc","hidden":False},"endpoints":{"btc":ENDPOINT},
                "contacts":[],"observations":[],"accounts":[
                {"board":BOARD,"generation":1,"id":"btc","account":ACCOUNT,"name":"Previous archive","hidden":True},
                {"board":BOARD,"generation":2,"id":"btc","account":ACCOUNT,"name":"Other generation","hidden":True}]}
            config={"cert":cert,"relay":f"wss://127.0.0.1:{PORT}","port":PORT,"case":state,"wallet_read":wallet_read,
                "chain_public":capture,"chain_watch":capture_watch,"chain_rpc":exchange,"chain_sign":reject_sign}
            server,worker,_,_=fixture(scratch,reject,"0x"+"12"*32,None,None,context,config)
            try:
                print("W25 discovery fixture ready",flush=True)
                while True:
                    time.sleep(1)
            finally:
                server.shutdown()
                server.server_close()
                worker.join()


def stop(_signal,_frame):
    raise KeyboardInterrupt


if __name__=="__main__":
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("cert",type=Path)
    parser.add_argument("key",type=Path)
    parser.add_argument("--bitcoind",type=Path,required=True)
    parser.add_argument("--oracle",type=Path,default=P01/"out/host/libwallet_chain_node_oracle.so")
    os.umask(0o077)
    signal.signal(signal.SIGTERM,stop)
    try:
        main(parser.parse_args())
    except KeyboardInterrupt:
        pass
