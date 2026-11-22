"""Isolated 8283 chain fixture, NOT a deployable finality authority."""
import json
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import time
from urllib.request import Request, urlopen
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec, utils

TEST=Path(__file__).resolve().parent
sys.path.insert(0,str(TEST.parent/'scripts'))
from evm_service import rlp
from evm_transfer import fields,keccak256
from issue_owner_finality_evidence import evidence
OWNER='0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266'
KEY='0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80'
RECIPIENT='0x'+'05'*20
BOARD=bytes.fromhex('01'*32)

def cast(*args):
    return subprocess.check_output(['cast',*args],text=True).strip()

def main(binary):
    with socket.socket() as sock:
        sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
    url=f'http://127.0.0.1:{port}'
    def rpc(method,args):
        packet={'jsonrpc':'2.0','id':1,'method':method,'params':args}
        with urlopen(Request(url,json.dumps(packet).encode(),{'Content-Type':'application/json'}),timeout=2) as res: reply=json.load(res)
        if 'error' in reply: raise ValueError(reply['error'])
        return reply['result']
    asset=TEST.parents[1]/'p06-stablenet-contracts/out/TestUSDC.sol/TestUSDC.json'
    with tempfile.TemporaryDirectory() as directory, open(Path(directory)/'anvil.log','w+') as log:
        node=subprocess.Popen(['anvil','--host','127.0.0.1','--port',str(port),'--chain-id','8283'],stdout=log,stderr=log)
        try:
            for _ in range(80):
                if node.poll() is not None: raise RuntimeError('Anvil failed to bind')
                try:
                    if rpc('eth_chainId',[])=='0x205b':break
                except OSError:time.sleep(.05)
            else:raise RuntimeError('Anvil unavailable')
            opts=('--rpc-url',url,'--private-key',KEY,'--json')
            deployment=json.loads(cast('send',*opts,'--create',json.loads(asset.read_text())['bytecode']['object'],'constructor(address)',OWNER))
            token=deployment['contractAddress'].lower()
            cast('send',*opts,token,'mint(address,uint256)',OWNER,'9')
            genesis=bytes.fromhex(rpc('eth_getBlockByNumber',['0x0',False])['hash'][2:])
            finality_key=ec.generate_private_key(ec.SECP256R1())
            public=finality_key.public_key().public_bytes(serialization.Encoding.DER,serialization.PublicFormat.SubjectPublicKeyInfo)
            for iteration,expected in enumerate((1,0,1,0)):
                if iteration==2:cast('send',*opts,token,'mint(address,uint256)',OWNER,'9')
                nonce=int(rpc('eth_getTransactionCount',[OWNER,'pending']),16)
                data=bytes.fromhex('a9059cbb'+'00'*12+RECIPIENT[2:]+'00'*31+'09')
                unsigned=b'\x02'+rlp([rlp(x) for x in (8283,nonce,1000000000,2000000000,100000,bytes.fromhex(token[2:]),0,data)]+[rlp([],True)],True)
                replacement=(b'\x02'+rlp([rlp(x) for x in (8283,nonce,0,2000000000,100000,bytes.fromhex(token[2:]),0,data)]+[rlp([],True)],True)) if iteration>=2 else None
                mined=unsigned if replacement is None else replacement
                digest=keccak256(mined)
                signature=bytes.fromhex(cast('wallet','sign','--private-key',KEY,'--no-hash','0x'+digest.hex())[2:])
                encoded=[rlp(value,group) if not group else rlp([],True) for value,group in fields(fields(mined[1:])[0][0])]
                signed=b'\x02'+rlp(encoded+[rlp(signature[64]-27),rlp(int.from_bytes(signature[:32],'big')),rlp(int.from_bytes(signature[32:64],'big'))],True)
                tx=rpc('eth_sendRawTransaction',['0x'+signed.hex()]);assert tx=='0x'+keccak256(signed).hex()
                rpc('anvil_mine',['0x1'])
                receipt=rpc('eth_getTransactionReceipt',[tx]);assert int(receipt['status'],16)==expected
                rpc('anvil_mine',['0x80'])
                finalized=rpc('eth_getBlockByNumber',['finalized',False]);height=int(receipt['blockNumber'],16)
                assert int(finalized['number'],16)>=height
                block=rpc('eth_getBlockByNumber',[receipt['blockNumber'],False]);assert block['hash']==receipt['blockHash']
                actual=rpc('eth_getTransactionByHash',[tx]);assert actual['from'].lower()==OWNER and int(actual['nonce'],16)==nonce and actual['to'].lower()==token
                assert actual['input'].lower()=='0x'+data.hex() and rpc('eth_getBlockByNumber',[receipt['blockNumber'],False])['hash']==receipt['blockHash']
                rid=bytes([iteration+1])*16
                from enroll_board import point
                profile=dict(board=BOARD.hex(),generation=1,chain=8283,signer=OWNER,genesis=genesis.hex(),asset=token,
                  request=rid.hex(),raw=unsigned.hex(),transaction=tx,finality_key=point(finality_key.public_key()).hex(),rpc='independent-test-fixture')
                if replacement is not None:
                    profile['replacement_raw']=replacement.hex()
                    for position in (6,len(replacement)-1-32-20,len(replacement)-2):
                        altered=bytearray(replacement);altered[position]^=1
                        bad=dict(profile);bad['replacement_raw']=altered.hex()
                        try:evidence(bad,rpc);raise AssertionError('altered replacement accepted')
                        except ValueError:pass
                body,original,verified_replacement=evidence(profile,rpc)
                assert original==unsigned and verified_replacement==replacement and body[236]==((1 if expected else 2) if replacement is None else (3 if expected else 4))
                bad=dict(profile);bad['genesis']='ff'*32
                try:evidence(bad,rpc);raise AssertionError('wrong genesis accepted')
                except ValueError:pass
                bad=dict(profile);bad['signer']='ee'*20
                try:evidence(bad,rpc);raise AssertionError('wrong signer accepted')
                except ValueError:pass
                r,s=utils.decode_dss_signature(finality_key.sign(body,ec.ECDSA(hashes.SHA256())))
                proof=r.to_bytes(32,'big')+s.to_bytes(32,'big')
                arguments=[binary,unsigned.hex(),body.hex(),proof.hex(),public.hex()]
                if replacement is not None:arguments.append(replacement.hex())
                subprocess.run(arguments,check=True)
            print('isolated 8283 original and same-nonce replacement success/revert finalized; C PS adapter verified signed evidence')
        finally:
            node.terminate();node.wait(timeout=5)
if __name__=='__main__':main(sys.argv[1])
