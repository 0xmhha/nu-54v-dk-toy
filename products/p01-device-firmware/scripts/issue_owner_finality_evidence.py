#!/usr/bin/env python3
"""Issue scoped 8283 original-transaction evidence from an operator-controlled finality node.

The profile and RPC endpoint are operator inputs, not phone inputs. A single node
is a trust assumption; production operators must independently qualify its finality
and incident handling. This tool never creates or enrolls a board trust key.
"""
import argparse
import json
import urllib.parse
import urllib.request
from pathlib import Path
from cryptography.hazmat.primitives import serialization
from enroll_board import exact_hex,point,signed
from evm_service import rlp
from evm_transfer import decode,keccak256

FIELDS={'board','generation','chain','signer','genesis','asset','request','raw','transaction','finality_key','rpc'}
def quantity(v):
    if not isinstance(v,str) or not v.startswith('0x') or len(v)<3:return_value_error()
    try:return int(v,16)
    except ValueError:return_value_error()
def return_value_error():raise ValueError('Malformed RPC quantity')
def bytes_hex(v,n=None):
    if not isinstance(v,str) or not v.startswith('0x'):raise ValueError('Malformed RPC bytes')
    try:b=bytes.fromhex(v[2:])
    except ValueError as e:raise ValueError('Malformed RPC bytes') from e
    if n is not None and len(b)!=n:raise ValueError('Malformed RPC bytes')
    return b

def evidence(profile,rpc):
    if not isinstance(profile,dict) or set(profile) not in (FIELDS,FIELDS|{'replacement_raw'}) or profile['chain']!=8283 or type(profile['generation']) is not int or not 0<profile['generation']<2**64:
        raise ValueError('Unsupported independent scope')
    board=exact_hex(profile['board'],32);signer=exact_hex(profile['signer'],20);genesis=exact_hex(profile['genesis'],32)
    token=exact_hex(profile['asset'],20);request=exact_hex(profile['request'],16);txhash=exact_hex(profile['transaction'],32)
    if not all(any(v) for v in (board,signer,genesis,token,request,txhash)):raise ValueError('Empty scope')
    try:raw=bytes.fromhex(profile['raw'])
    except (ValueError,TypeError):raise ValueError('Invalid original bytes') from None
    chain,nonce,_,_,amount,recipient=decode(raw,token)
    if chain!=8283 or not amount:raise ValueError('Unsupported original transfer')
    replacement=None
    if 'replacement_raw' in profile:
        try:replacement=bytes.fromhex(profile['replacement_raw'])
        except (ValueError,TypeError):raise ValueError('Invalid replacement bytes') from None
        other_chain,other_nonce,_,_,other_amount,other_recipient=decode(replacement,token)
        if (other_chain!=chain or other_nonce!=nonce or other_recipient!=recipient or not other_amount or
            other_amount>amount or keccak256(replacement)==keccak256(raw)):
            raise ValueError('Replacement is outside the enrolled safe scope')
    mined=raw if replacement is None else replacement
    if quantity(rpc('eth_chainId',[]))!=8283 or bytes_hex(rpc('eth_getBlockByNumber',['0x0',False])['hash'],32)!=genesis:
        raise ValueError('Wrong network')
    tx=rpc('eth_getTransactionByHash',['0x'+txhash.hex()]);receipt=rpc('eth_getTransactionReceipt',['0x'+txhash.hex()])
    if not isinstance(tx,dict) or not isinstance(receipt,dict):raise ValueError('No finalized original')
    if (bytes_hex(tx['hash'],32)!=txhash or bytes_hex(receipt['transactionHash'],32)!=txhash or bytes_hex(tx['from'],20)!=signer or
        quantity(tx['type'])!=2 or quantity(tx['chainId'])!=8283 or quantity(tx['nonce'])!=nonce or tx.get('accessList')!=[]):
        raise ValueError('Original transaction mismatch')
    values=(8283,nonce,quantity(tx['maxPriorityFeePerGas']),quantity(tx['maxFeePerGas']),quantity(tx['gas']),
            bytes_hex(tx['to'],20),quantity(tx['value']),bytes_hex(tx['input']))
    if b'\x02'+rlp([rlp(v) for v in values]+[rlp([],True)],True)!=mined:raise ValueError('Finalized transaction bytes differ')
    height=quantity(receipt['blockNumber']);blockhash=bytes_hex(receipt['blockHash'],32);status=quantity(receipt['status'])
    if (not height or status not in (0,1) or bytes_hex(tx['blockHash'],32)!=blockhash or
        quantity(tx['blockNumber'])!=height):raise ValueError('Invalid receipt')
    block=rpc('eth_getBlockByNumber',[receipt['blockNumber'],False]);final=rpc('eth_getBlockByNumber',['finalized',False])
    finalized=quantity(final['number']);stamp=quantity(block['timestamp'])
    rechecked=rpc('eth_getTransactionReceipt',['0x'+txhash.hex()])
    if (not isinstance(rechecked,dict) or bytes_hex(rechecked['transactionHash'],32)!=txhash or
        bytes_hex(rechecked['blockHash'],32)!=blockhash or quantity(rechecked['status'])!=status or
        quantity(rechecked['blockNumber'])!=height or bytes_hex(block['hash'],32)!=blockhash or finalized<height or
        bytes_hex(rpc('eth_getBlockByNumber',[receipt['blockNumber'],False])['hash'],32)!=blockhash or
        quantity(rpc('eth_getTransactionCount',['0x'+signer.hex(),'finalized']))<=nonce):
        raise ValueError('Finality or canonical chain not established')
    values=(profile['generation'],8283,nonce,height,finalized,stamp)
    domain=b'N54F' if replacement is None else b'N54R'
    outcome=(1 if status else 2) if replacement is None else (3 if status else 4)
    body=(domain+board+genesis+signer+token+request+keccak256(raw)+blockhash+
          b''.join(v.to_bytes(8,'big') for v in values)+bytes([outcome]))
    if replacement is not None:body+=keccak256(replacement)
    if len(body)!=(237 if replacement is None else 269):raise ValueError('Invalid evidence')
    return body,raw,replacement

def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--profile',type=Path,required=True)
    parser.add_argument('--key',type=Path,required=True);parser.add_argument('--out',type=Path,required=True)
    args=parser.parse_args();profile=json.loads(args.profile.read_text());url=urllib.parse.urlsplit(profile.get('rpc',''))
    if url.scheme!='https' or not url.hostname or url.username or url.password or url.fragment:raise ValueError('Operator RPC must be HTTPS')
    key=serialization.load_pem_private_key(args.key.read_bytes(),None)
    if point(key.public_key())!=exact_hex(profile['finality_key'],65):raise ValueError('Wrong delegated finality key')
    def rpc(method,params):
        payload=json.dumps({'jsonrpc':'2.0','id':1,'method':method,'params':params}).encode()
        with urllib.request.urlopen(urllib.request.Request(profile['rpc'],payload,{'Content-Type':'application/json'}),timeout=10) as response:
            if response.geturl()!=profile['rpc']:raise ValueError('Finality node redirected')
            encoded=response.read(1048577)
        if len(encoded)>1048576:raise ValueError('Finality node response too large')
        reply=json.loads(encoded)
        if not isinstance(reply,dict) or reply.get('id')!=1 or 'error' in reply or 'result' not in reply:raise ValueError('Invalid finality node response')
        return reply['result']
    body,raw,replacement=evidence(profile,rpc)
    document={'type':'nu54-finality-evidence-v1' if replacement is None else 'nu54-finality-replacement-v1',
              'body':body.hex(),'signature':signed(args.key,body).hex(),'raw':raw.hex()}
    if replacement is not None:document['replacement']=replacement.hex()
    args.out.write_text(json.dumps(document,indent=2)+'\n')
if __name__=='__main__':main()
