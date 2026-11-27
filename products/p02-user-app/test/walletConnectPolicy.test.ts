import {validateConnectCheckpoint,viewAllowanceConnection,WalletConnectSession} from '../src/wallet/dapp/walletConnectSession';
import {open,seal} from '../src/wallet/dapp/walletConnectCrypto';
import type {RelayMessage} from '../src/wallet/dapp/walletConnectRelay';
import {p256} from '@noble/curves/p256';
import {sha256} from '@noble/hashes/sha256';
import {bytesToHex,hexToBytes,utf8Encode} from '@nu54/protocol';
import {parsePairing, negotiate, verifyPeer, type VerifiedPeer} from '../src/wallet/dapp/walletConnectPolicy';

const key='22'.repeat(32), topic=bytesToHex(sha256(hexToBytes(key)),false), pub='33'.repeat(32);
const uri=`wc:${topic}@2?symKey=${key}&relay-protocol=irn&expiryTimestamp=1100`;
const scope={board:'44'.repeat(32),account:'0x'+'55'.repeat(20),generation:3,chain:11155111,genesis:'0x'+'66'.repeat(32)};
const proposal={proposer:{publicKey:pub,metadata:{url:'https://service.example/app'}},relays:[{protocol:'irn'}],
  requiredNamespaces:{eip155:{chains:['eip155:11155111'],methods:['eth_accounts'],events:[]}},
  optionalNamespaces:{eip155:{chains:['eip155:11155111'],methods:['personal_sign','eth_sendTransaction'],events:['accountsChanged']}}};
const proof:VerifiedPeer={origin:'https://service.example',topic,publicKey:pub,expires:1090};

test('pairing URI accepts only version 2 IRN with a live secret and no relay override',()=>{
  expect(parsePairing(uri,1000)).toEqual({topic,key,expiry:1100});
  for (const invalid of [uri.replace('@2','@1'),uri+'&symKey='+key,uri+'&relayUrl=wss://attacker.example',
    uri.replace('1100','999'),uri.replace('relay-protocol=irn','relay-protocol=foo')])
    expect(()=>parsePairing(invalid,1000)).toThrow();
});

test('unverified metadata cannot get signing permissions even when a claimed origin matches',()=>{
  const pairing=parsePairing(uri,1000);
  expect(negotiate(proposal,pairing,scope,1000,undefined,true).methods).toEqual(['eth_accounts']);
  expect(negotiate(proposal,pairing,scope,1000,proof,true).methods).toEqual(['eth_accounts','personal_sign','eth_sendTransaction']);
  const requiredSign={...proposal,requiredNamespaces:{eip155:{chains:['eip155:11155111'],methods:['personal_sign'],events:[]}}};
  expect(()=>negotiate(requiredSign,pairing,scope,1000,{...proof,publicKey:'77'.repeat(32)},true)).toThrow();
  for (const allow of [undefined,{...proof,topic:pub},{...proof,expires:1000},{...proof,origin:'https://other.example'}])
    expect(negotiate(proposal,pairing,scope,1000,allow,true).methods).toEqual(['eth_accounts']);
  const wrongChain={...proposal,requiredNamespaces:{eip155:{chains:['eip155:1'],methods:['eth_accounts'],events:[]}}};
  expect(()=>negotiate(wrongChain,pairing,scope,1000,proof,true)).toThrow();
});

test('authenticates a peer only under the enrolled authority and exact pairing scope',()=>{
  const authority=new Uint8Array(32).fill(2);
  const statement=utf8Encode('N54-walletconnect-v1\n'+topic+'\n'+pub+'\n'+proof.origin+'\n'+proof.expires);
  const signature=p256.sign(sha256(statement),authority).toCompactHex();
  expect(verifyPeer(proof,signature,p256.getPublicKey(authority),1000)).toEqual(proof);
  expect(()=>verifyPeer({...proof,origin:'https://else.example'},signature,p256.getPublicKey(authority),1000)).toThrow();
  expect(()=>verifyPeer(proof,signature,p256.getPublicKey(new Uint8Array(32).fill(3)),1000)).toThrow();
});

test('restored wallet grants fail closed on old board, same-chain different genesis, or expiry',()=>{
  const sessionKey='aa'.repeat(32);
  const sessionTopic=bytesToHex(sha256(hexToBytes(sessionKey)),false);
  const saved={topic:sessionTopic,key:sessionKey,scope,proposal:negotiate(proposal,parsePairing(uri,1000),scope,1000),expires:1090,lastId:102};
  expect(validateConnectCheckpoint(saved,scope,1001)).toEqual(saved);
  expect(()=>validateConnectCheckpoint(saved,{...scope,genesis:'0x'+'88'.repeat(32)},1001)).toThrow();
  expect(()=>validateConnectCheckpoint(saved,{...scope,generation:4},1001)).toThrow();
  expect(()=>validateConnectCheckpoint(saved,scope,1090)).toThrow();
  expect(()=>validateConnectCheckpoint({...saved,proposal:{...saved.proposal,methods:['personal_sign']}},scope,1001)).toThrow();
});

test('W26 refuses authenticated NFT and multicall envelopes before a broadcaster or approval callback',async()=>{
  const sessionKey=new Uint8Array(32).fill(0xaa);
  const sessionTopic=bytesToHex(sha256(sessionKey),false);
  const saved={topic:sessionTopic,key:bytesToHex(sessionKey,false),scope,
    proposal:negotiate(proposal,parsePairing(uri,1000),scope,1000,proof,true),expires:1090,lastId:102};
  let receive:((message:RelayMessage)=>void)|undefined;
  let response:((raw:string)=>void)|undefined;
  const request=jest.fn(async()=>'unexpected');
  const approve=jest.fn(async()=>true);
  const save=jest.fn(async()=>{});
  const refused=jest.fn();
  const session=new WalletConnectSession({scope,clock:()=>1001,random:async length=>new Uint8Array(length).fill(1),
    save,approve,request,advancedRefused:refused,allowSpend:true,trust:async()=>undefined,
    rtc:{subscribe:async()=>{},listen:handler=>{receive=handler;},close:()=>{},
      publish:async(_,packet)=>{response?.(packet);}}});
  try {
    await session.resume(saved);
    for (const [idx,selector] of ['42842e0e','ac9650d8'].entries()) {
      const next=new Promise<string>(resolve=>{response=resolve;});
      receive?.({topic:sessionTopic,tag:1108,message:seal(sessionKey,new Uint8Array(12).fill(idx+2),
        {jsonrpc:'2.0',id:103+idx,method:'wc_sessionRequest',params:{chainId:'eip155:11155111',
          request:{method:'eth_sendTransaction',params:[{from:scope.account,to:'0x'+'77'.repeat(20),data:'0x'+selector+'00'.repeat(96)}]}}})});
      expect(open(sessionKey,await next)).toMatchObject({id:103+idx,error:{code:5000}});
      await new Promise(resolve=>setTimeout(resolve,0));
    }
    expect(refused.mock.calls).toEqual([['nft'],['batch']]);
    expect(request).not.toHaveBeenCalled();
    expect(approve).not.toHaveBeenCalled();
    expect(save).not.toHaveBeenCalled();
  } finally {
    session.suspend();
  }
});

test('W14 forwards only a valid WalletConnect allowance and reports the original after the relay reply',async()=>{
  const sessionKey=new Uint8Array(32).fill(0xaa);
  const sessionTopic=bytesToHex(sha256(sessionKey),false);
  const saved={topic:sessionTopic,key:bytesToHex(sessionKey,false),scope,
    proposal:negotiate(proposal,parsePairing(uri,1000),scope,1000,proof,true),expires:1090,lastId:102};
  const token='0x'+'11'.repeat(20), spender='0x'+'22'.repeat(20);
  const hash='0x'+'ab'.repeat(32);
  const policy={token,spender,maximum:'5000000'};
  let receive:((message:RelayMessage)=>void)|undefined;
  let response:((raw:string)=>void)|undefined;
  const request=jest.fn<Promise<string|typeof viewAllowanceConnection>,[]>(async()=>hash).mockResolvedValueOnce(viewAllowanceConnection);
  const submitted=jest.fn(), view=jest.fn();
  const session=new WalletConnectSession({scope,clock:()=>1001,random:async length=>new Uint8Array(length).fill(1),
    save:async()=>{},approve:async()=>true,request,allowancePolicy:policy,allowanceSubmitted:submitted,viewAllowanceConnection:view,
    allowSpend:true,trust:async()=>undefined,rtc:{subscribe:async()=>{},listen:handler=>{receive=handler;},close:()=>{},
      publish:async(_,packet)=>{response?.(packet);}}});
  const send=(id:number,amount:bigint,expiry:number,chainId='eip155:11155111')=>{
    const reply=new Promise<string>(resolve=>{response=resolve;});
    const transaction={from:scope.account,to:token,value:'0x0',
      data:'0x095ea7b3'+'00'.repeat(12)+spender.slice(2)+amount.toString(16).padStart(64,'0')};
    receive?.({topic:sessionTopic,tag:1108,message:seal(sessionKey,new Uint8Array(12).fill(id),
      {jsonrpc:'2.0',id,method:'wc_sessionRequest',params:{chainId,
        request:{method:'eth_sendTransaction',expiry,params:[transaction]}}})});
    return reply;
  };
  try {
    await session.resume(saved);
    expect(open(sessionKey,await send(103,5000001n,1030))).toMatchObject({id:103,error:{code:5000}});
    await new Promise(resolve=>setTimeout(resolve,0));
    expect(request).not.toHaveBeenCalled();
    expect(open(sessionKey,await send(104,5000000n,1030))).toMatchObject({id:104,error:{code:5000,message:expect.stringContaining('Send a new request')}});
    await new Promise(resolve=>setTimeout(resolve,0));
    expect(view).toHaveBeenCalledWith(saved.proposal.origin);
    expect(submitted).not.toHaveBeenCalled();
    expect(open(sessionKey,await send(104,5000000n,1030))).toMatchObject({id:104,error:{code:5000}});
    expect(open(sessionKey,await send(105,5000000n,1030,'eip155:84532'))).toMatchObject({id:105,error:{code:5000}});
    await new Promise(resolve=>setTimeout(resolve,0));
    expect(open(sessionKey,await send(106,5000000n,1001))).toMatchObject({id:106,error:{code:5000}});
    await new Promise(resolve=>setTimeout(resolve,0));
    expect(request).toHaveBeenCalledTimes(1);
    expect(open(sessionKey,await send(107,5000000n,1030))).toMatchObject({id:107,result:hash});
    await new Promise(resolve=>setTimeout(resolve,0));
    expect(submitted).toHaveBeenCalledTimes(1);
    expect(request).toHaveBeenCalledTimes(2);
    expect(request).toHaveBeenLastCalledWith({kind:'allowance',token,spender,amount:5000000n},saved.proposal,
      expect.any(AbortSignal),sessionTopic,1030);
  } finally {session.suspend();}
});

test('W13 drops approval callbacks when a request expires while a durable session write is pending',async()=>{
  const sessionKey=new Uint8Array(32).fill(0xaa);
  const sessionTopic=bytesToHex(sha256(sessionKey),false);
  const saved={topic:sessionTopic,key:bytesToHex(sessionKey,false),scope,
    proposal:negotiate(proposal,parsePairing(uri,1000),scope,1000,proof,true),expires:1090,lastId:102};
  const token='0x'+'11'.repeat(20), spender='0x'+'22'.repeat(20);
  let receive!:((message:RelayMessage)=>void);
  let completeSave!:()=>void;
  let startSave!:()=>void;
  const started=new Promise<void>(resolve=>{startSave=resolve;});
  const request=jest.fn(async()=>'unexpected');
  const publish=jest.fn(async()=>{});
  let clock=1001;
  const session=new WalletConnectSession({scope,clock:()=>clock,random:async length=>new Uint8Array(length).fill(1),
    save:()=>new Promise(resolve=>{completeSave=resolve;startSave();}),
    approve:async()=>true,request,allowancePolicy:{token,spender,maximum:'5000000'},
    allowSpend:true,trust:async()=>undefined,rtc:{subscribe:async()=>{},listen:handler=>{receive=handler;},close:()=>{},publish}});
  try {
    await session.resume(saved);
    receive({topic:sessionTopic,tag:1108,message:seal(sessionKey,new Uint8Array(12).fill(3),
      {jsonrpc:'2.0',id:103,method:'wc_sessionRequest',params:{chainId:'eip155:11155111',
        request:{method:'eth_sendTransaction',expiry:1002,params:[{from:scope.account,to:token,value:'0x0',
          data:'0x095ea7b3'+'00'.repeat(12)+spender.slice(2)+'1'.padStart(64,'0')}]}}})});
    await started;
    clock=1002;
    completeSave();
    await new Promise(resolve=>setTimeout(resolve,0));
    expect(request).not.toHaveBeenCalled();
    expect(publish).not.toHaveBeenCalled();
  } finally {session.suspend();}
});
