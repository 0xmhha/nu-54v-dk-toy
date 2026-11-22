"""Check the independent W17 display against disconnected Core and the board host oracle."""
import base64
import json
from pathlib import Path
import struct
import sys
import time

sys.dont_write_bytecode = True
p01=Path(__file__).resolve().parents[1]
sys.path.insert(0, str(p01.parents[1] / "scripts"))
from bitcoin_core_workspace import bitcoin_core_workspace
from integration_workspace import run_command
sys.path.insert(0,str(p01/'scripts'))
sys.path.insert(0,str(p01/'test'))
import wallet_chain_lab as lab
from wallet_chain_review import review_bip48
from test_bip48 import maps
from run_bip48_node import sign
import ctypes

core=Path(sys.argv[1]).resolve()
oracle_path=Path(sys.argv[2]).resolve()
vector=json.loads((p01/'test/bip48_two_input_vector.json').read_text())
raw=base64.b64decode(vector['unsignedPSBT'])
with bitcoin_core_workspace(core, 'nu54-w17-actual-display-', 'signet') as node:
    scratch=node.directory
    cfg=scratch/'policy.json'
    cfg.write_text(json.dumps(dict(account=0,maxFee=100000,owner=0,descriptors=vector['descriptors'])))
    policy,_=lab.bip48_configuration(cfg,2)
    display_key=scratch/'display.pem'
    run_command(['openssl','ecparam','-name','prime256v1','-genkey','-noout','-out',str(display_key)],check=True)
    def review_file(psbt):
        grant = struct.pack('>BBBIQQ',18,1,2,0,int(time.time())+55,100000)+lab.h256(policy)+bytes(58)+lab.h256(lab.COMMISSIONED[1].encode())+struct.pack('>Q',1)+bytes(64)+psbt
        path=scratch/(lab.h256(grant).hex()+'.json')
        path.write_text(json.dumps(dict(grant=grant.hex(),review=dict(unsignedHex=psbt.hex()))))
        return path
    def display(psbt, response='APPROVE'):
        return run_command([sys.executable, '-B', str(p01/'scripts/wallet_chain_lab.py'),'display','--review',str(review_file(psbt)),
            '--bitcoin',f'http://127.0.0.1:{node.port}','--bitcoin-cookie',str(node.cookie),
            '--challenge','33'*32,'--display-key',str(display_key),'--bip48-policy',str(cfg)],
            input=response+'\n',capture_output=True,text=True,timeout=15)
    node.start()
    valid=display(raw)
    assert valid.returncode==0, valid.stderr
    review=json.JSONDecoder().raw_decode(valid.stdout)[0]
    assert [i['satoshis'] for i in review['inputs']]==[60000000,60000000]
    assert all(i['scriptPubKey'].startswith('0020') for i in review['inputs'])
    assert review['allOutputs'][1]['verifiedChangePath']=="m/48'/1'/0'/2'/1/0"
    assert review['feeSatoshis']==588 and review['maximumSignedVbytes']==294 and review['minimumFeeRateSatPerVbyte']=='2.00'
    refusal=display(raw,'REFUSE')
    assert refusal.returncode and 'approval declined' in refusal.stderr
    entries=maps(raw)
    for k in list(entries[-1]):
        if k[0]==2:
            entries[-1][k]=entries[-1][k][:20]+bytes(4)+entries[-1][k][24:]
    broken=b'psbt\xff'+b''.join(b''.join(lab.kv(k,v) for k,v in fields.items())+b'\0' for fields in entries)
    bad=display(broken)
    assert bad.returncode and 'invalid registered branch' in bad.stderr
    oracle=ctypes.CDLL(str(oracle_path))
    assert sign(oracle,broken,network=2) is None
    print('PASS actual disconnected Signet Core descriptor engine, two prevout scripts, verified change, 588 sat and 2.00 sat/vB lower bound, invalid change refusal and zero board partial')
