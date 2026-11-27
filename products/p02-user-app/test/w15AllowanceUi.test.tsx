import React from 'react';
import Renderer from 'react-test-renderer';
import {Alert,AppState,BackHandler,TextInput} from 'react-native';
import {p256} from '@noble/curves/p256';
import {sha256} from '@noble/hashes/sha256';
import {keccak_256} from '@noble/hashes/sha3';
import {bigToBytes,bytesToHex,concat,hexToBytes,toBase64,utf8Encode} from '@nu54/protocol';
import {WalletHome} from '../src/wallet/WalletHome';
import {WalletConnectScreen} from '../src/wallet/dapp/WalletConnectScreen';
import {readDirectTransfer,verifySignedTransfer} from '../src/wallet/evmTransfer';
import {secp256k1} from '@noble/curves/secp256k1';

let mockStored='';
const mockRpc=jest.fn();
const mockSigner=jest.fn();
const mockPublish=jest.fn(async(_topic:string,_cipher:string,_tag:number,_ttl:number)=>{});
jest.mock('../src/wallet/dapp/walletConnectRelay',()=>({IrnRelay:class {
  publish(...args:Parameters<typeof mockPublish>){return mockPublish(...args);}
  close(){}
}}));
jest.mock('../src/specs/NativeOwnerWallet.ts',()=>({__esModule:true,default:{
  randomBytes:async(length:number)=>Buffer.alloc(length,6).toString('base64'),
  setHomeProtection:async()=>{},loadProfile:async()=>mockStored,evmRpc:(url:string,packet:string)=>mockRpc(url,packet),
  compareAndSaveProfile:async(expected:string,next:string)=>{if(expected!==mockStored)return false;mockStored=next;return true;},
}}));
jest.mock('../src/specs/NativeRenterBle.ts',()=>({__esModule:true,default:{loadDevices:async()=>'',
  onScan:(listener:(value:string)=>void)=>{listener(JSON.stringify({name:'Board',address:'board'}));return{remove(){}};},
  scanOwner:async()=>{},
}}));
jest.mock('../src/wallet/ownerSession.ts',()=>({OwnerSession:class {
  async connect(){} close(){}
  async newSigningRequestId(){return '56'.repeat(16);}
  signDirectTransfer(wire:string,address:string){return mockSigner(wire,address);}
}}));

const field=(value:Uint8Array)=>value.length===1&&value[0]<128 ? value : concat(Uint8Array.of(128+value.length),value);
const list=(...values:Uint8Array[])=>{
  const body=concat(...values);
  return concat(Uint8Array.of(0xf8,body.length),body);
};
const hex=(value:string)=>hexToBytes(value);

test('W15 uses a separate zero-only review and reloads the original hash before checking receipt and allowance',async()=>{
  const time=BigInt(Math.floor(Date.now()/1000)), key=new Uint8Array(32).fill(7);
  const device='34'.repeat(32), genesis='0x'+'44'.repeat(32), hashBlock='0x'+'52'.repeat(32);
  const rpc='https://ledger.example/', endpoint='https://authority.example/wallet';
  const token='0x'+'12'.repeat(20), spender='0x'+'23'.repeat(20), chain='11155111';
  const sender=bytesToHex(keccak_256(secp256k1.getPublicKey(key,false).subarray(1)).subarray(12),true);
  const anchor=bytesToHex(p256.getPublicKey(key,false),false);
  const payload=utf8Encode(JSON.stringify({device,sender,generation:1,expires:Number(time)+120,leaseRevision:'2',
    networks:[{chain,genesis,rpc,feeLimit:'600000',assets:[{address:token,symbol:'DEMO',decimals:0}]}]}));
  mockStored=JSON.stringify({version:2,exercised:true,mode:'personal',
    verified:{boardFingerprint:device,address:sender,generation:1,network:'8283'},
    enrollment:{version:2,device,boardSPKI:'24'.repeat(32),ca:'MA==',certificateChain:['MA==']},
    evmRegistry:{endpoint,token:'55'.repeat(32),payload:toBase64(payload),anchor,
      proof:bytesToHex(p256.sign(sha256(concat(utf8Encode('N54G'),payload)),key).toCompactRawBytes(),false)}});
  const data=hex('095ea7b3'+'00'.repeat(12)+spender.slice(2)+'00'.repeat(32));
  const fields=[...['aa36a7','03','03','0c','a028',token.slice(2),''].map(value=>field(hex(value))),
    concat(Uint8Array.of(0xb8,data.length),data),Uint8Array.of(0xc0)];
  const unsigned=concat(Uint8Array.of(2),list(...fields));
  const signature=secp256k1.sign(keccak_256(unsigned),key);
  const signed=concat(Uint8Array.of(2),list(...fields,field(signature.recovery ? Uint8Array.of(1) : new Uint8Array()),
    field(bigToBytes(signature.r,32)),field(bigToBytes(signature.s,32))));
  const raw=bytesToHex(signed,true), hash=bytesToHex(keccak_256(signed),true);
  const head=concat(Uint8Array.of(14),bigToBytes(time+60n,8),bigToBytes(BigInt(chain),8),bigToBytes(600000n,8),
    bigToBytes(0n,8),hex(token.slice(2)),hex(genesis.slice(2)),bigToBytes(1n,8),bigToBytes(2n,8));
  const statement=concat(utf8Encode('N54V'),hex(device),bigToBytes(1n,8),hex(sender.slice(2)),head,keccak_256(unsigned));
  const wire=bytesToHex(concat(head,p256.sign(sha256(statement),key).toCompactRawBytes(),unsigned),false);
  mockSigner.mockImplementation(async (intent:string,address:string)=>{
    expect(intent).toBe(wire);
    return {...verifySignedTransfer(readDirectTransfer(intent,time),signed,address),
      evidence:{board:device,generation:1,request:'56'.repeat(16),unsigned:bytesToHex(unsigned,true)}};
  });
  let receipt=false, allowance=42n, reorganize=false, failRead=false;
  let gate:Promise<void>|undefined, quoteGate:Promise<void>|undefined;
  mockRpc.mockImplementation(async (url:string,packet:string)=>{
    const {method,params}=JSON.parse(packet);
    expect(url).toBe(method==='nu54_walletRevokeAllowance'?endpoint:rpc);
    let result:unknown;
    switch(method){
      case 'nu54_walletRevokeAllowance':
        await quoteGate;
        expect(params[0]).toMatchObject({chain,asset:token,recipient:spender});
        expect(params[0]).not.toHaveProperty('amount');result=wire;break;
      case 'eth_chainId':result='0xaa36a7';break;
      case 'eth_getBlockByNumber':result=params[0]==='0x0'?{hash:genesis}:{hash:hashBlock,number:'0x5'};break;
      case 'eth_call':
        await gate;
        expect(params[0]).toEqual({to:token,data:'0xdd62ed3e'+'00'.repeat(12)+sender.slice(2)+'00'.repeat(12)+spender.slice(2)});
        if (JSON.parse(mockStored).pendingEvm?.submitted) {
          expect(params[1]).toEqual({blockHash:hashBlock,requireCanonical:true});
          if (reorganize) receipt=false;
          if (failRead) throw new Error('Allowance provider unavailable');
        } else expect(params[1]).toBe('finalized');
        result='0x'+allowance.toString(16).padStart(64,'0');break;
      case 'eth_sendRawTransaction':
        expect(JSON.parse(mockStored).pendingEvm).toMatchObject({submitted:true,hash});
        expect(params).toEqual([raw]);result=hash;break;
      case 'eth_getTransactionReceipt':
        expect(params).toEqual([hash]);result=receipt?{transactionHash:hash,blockHash:hashBlock,blockNumber:'0x5',status:'0x1'}:null;break;
      default:throw Error('Unexpected '+method);
    }
    return JSON.stringify({jsonrpc:'2.0',id:1,result});
  });
  let tree:Renderer.ReactTestRenderer|undefined;
  const app=<WalletHome permission={async()=>true} honorExistingRental={false} rental={()=>{}} orders={()=>{}}/>;
  const tap=(label:string)=>Renderer.act(async()=>{
    const control=tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0];
    expect(control.props.disabled).not.toBe(true);
    control.props.onPress();
  });
  const ui=()=>JSON.stringify(tree!.toJSON());
  const methods=()=>mockRpc.mock.calls.map(([,body])=>JSON.parse(body).method);
  const listeners:(Parameters<typeof AppState.addEventListener>[1])[]=[];
  const events=jest.spyOn(AppState,'addEventListener').mockImplementation((_type, listener)=>{
    listeners.push(listener);
    return {remove(){}};
  });
  let hardwareBack:Parameters<typeof BackHandler.addEventListener>[1]|undefined;
  const backEvent=jest.spyOn(BackHandler,'addEventListener').mockImplementation((_type, listener)=>{
    hardwareBack=listener;
    return {remove(){hardwareBack=undefined;}};
  });
  try{
    await Renderer.act(async()=>{tree=Renderer.create(app);});
    await tap('온체인 토큰 권한');
    await Renderer.act(async()=>tree!.root.findAllByType(TextInput).find(input=>input.props.accessibilityLabel==='권한 대상 주소')!.props.onChangeText(spender));
    let resume!:()=>void;
    gate=new Promise(resolve=>{resume=resolve;});
    await tap('확정 한도 조회');
    expect(methods()).toContain('eth_call');
    await Renderer.act(async()=>{
      listeners.forEach(listener=>listener('background'));
      listeners.forEach(listener=>listener('active'));
      resume();
    });
    gate=undefined;
    expect(ui()).not.toContain('현재 확정 한도');
    await tap('확정 한도 조회');
    expect(ui()).toContain('42');
    quoteGate=new Promise(resolve=>{resume=resolve;});
    await tap('토큰 사용 권한 철회 검토');
    expect(methods()).toContain('nu54_walletRevokeAllowance');
    await Renderer.act(async()=>{
      listeners.forEach(listener=>listener('inactive'));
      listeners.forEach(listener=>listener('active'));
      resume();
    });
    quoteGate=undefined;
    expect(ui()).not.toContain('42 → 0');
    expect(mockSigner).not.toHaveBeenCalled();
    await tap('확정 한도 조회');
    quoteGate=new Promise(resolve=>{resume=resolve;});
    await tap('토큰 사용 권한 철회 검토');
    await Renderer.act(async()=>{expect(hardwareBack?.({type:'hardwareBackPress',timeStamp:Date.now()})).toBe(true); resume();});
    quoteGate=undefined;
    await tap('온체인 토큰 권한');
    expect(ui()).not.toContain('42 → 0');
    expect(ui()).not.toContain('현재 확정 한도');
    await Renderer.act(async()=>tree!.root.findAllByType(TextInput).find(input=>input.props.accessibilityLabel==='권한 대상 주소')!.props.onChangeText(spender));
    await tap('확정 한도 조회');
    await tap('토큰 사용 권한 철회 검토');
    expect(ui()).toContain('42 → 0');
    await tap('거절하고 초안으로');
    expect(mockSigner).not.toHaveBeenCalled();
    expect(methods()).not.toContain('eth_sendRawTransaction');
    await tap('토큰 사용 권한 철회 검토');
    await tap('개인 기기 찾기');
    allowance=41n;
    await tap('기기에 토큰 권한 철회 승인 요청');
    expect(mockSigner).not.toHaveBeenCalled();
    expect(ui()).toContain('확정 한도가 변경되었습니다. 다시 검토하세요.');
    allowance=42n;
    await tap('토큰 사용 권한 철회 검토');
    await tap('기기에 토큰 권한 철회 승인 요청');
    expect(mockSigner).toHaveBeenCalledTimes(1);
    await tap('이 네트워크에 전송');
    await Renderer.act(async()=>{tree!.unmount();tree=Renderer.create(app);});
    await tap('온체인 토큰 권한');
    expect(ui()).toContain(hash);
    expect(ui()).toContain('요청 한도');
    await tap('같은 철회 거래 조회');
    expect(ui()).not.toContain('현재 확정 한도');
    allowance=0n;
    await tap('같은 철회 거래 조회');
    expect(ui()).not.toMatch(/현재 확정 한도[^0-9]*0/);
    receipt=true;allowance=42n;
    await tap('같은 철회 거래 조회');
    expect(ui()).toMatch(/현재 확정 한도[^0-9]*42/);
    expect(ui()).not.toContain('철회 거래 확정 대기');
    allowance=0n;
    reorganize=true;
    await tap('같은 철회 거래 조회');
    expect(ui()).not.toContain('현재 확정 한도');
    expect(ui()).toContain(hash);
    reorganize=false;receipt=true;
    failRead=true;
    await tap('같은 철회 거래 조회');
    expect(ui()).not.toContain('현재 확정 한도');
    expect(ui()).toContain(hash);
    failRead=false;
    gate=new Promise(resolve=>{resume=resolve;});
    await tap('같은 철회 거래 조회');
    await Renderer.act(async()=>{
      listeners.forEach(listener=>listener('background'));
      listeners.forEach(listener=>listener('active'));
      resume();
    });
    gate=undefined;
    expect(ui()).not.toMatch(/현재 확정 한도[^0-9]*0/);
    expect(ui()).toContain(hash);
    await tap('같은 철회 거래 조회');
    expect(ui()).toMatch(/현재 확정 한도[^0-9]*0/);
    failRead=true;
    await tap('같은 철회 거래 조회');
    expect(ui()).not.toContain('현재 확정 한도');
    expect(ui()).toContain(hash);
    expect(JSON.parse(mockStored).pendingEvm).toMatchObject({raw,hash,submitted:true});
    expect(methods().filter(method=>method==='eth_sendRawTransaction')).toHaveLength(1);
  } finally {
    await Renderer.act(async()=>{tree?.unmount();});
    events.mockRestore();
    backEvent.mockRestore();
  }
},30000);

test('W13 starts a new durable connection after a selected stored topic expires',async()=>{
  jest.useFakeTimers();
  const now=Math.floor(Date.now()/1000), key=new Uint8Array(32).fill(7);
  const device='34'.repeat(32), sender='0x'+'11'.repeat(20), genesis='0x'+'44'.repeat(32);
  const chain='11155111', relay='wss://relay.example';
  const payload=utf8Encode(JSON.stringify({device,sender,generation:1,expires:now+120,leaseRevision:'2',walletConnect:{relay},
    networks:[{chain,genesis,rpc:'https://ledger.example/',feeLimit:'600000',
      assets:[{address:'0x'+'12'.repeat(20),symbol:'DEMO',decimals:6}]}]}));
  function connection(origin:string, secret: number, expires:number) {
    const sessionKey=new Uint8Array(32).fill(secret);
    return {relay,seed:'55'.repeat(32),checkpoint:{topic:bytesToHex(sha256(sessionKey),false),key:bytesToHex(sessionKey,false),
      scope:{board:device,chain:Number(chain),genesis,account:sender,generation:1},lastId:1,expires,
      proposal:{origin,chain:'eip155:'+chain,account:'eip155:'+chain+':'+sender,
        peerKey:'31'.repeat(32),pairingTopic:'41'.repeat(32),methods:['eth_accounts' as const],events:[],verified:false}}};
  }
  const expired=connection('https://old.example',4,now+10), fresh=connection('https://new.example',5,now+60);
  mockStored=JSON.stringify({version:2,exercised:true,mode:'personal',
    verified:{boardFingerprint:device,address:sender,generation:1,network:'8283'},walletConnect:expired,
    enrollment:{version:2,device,boardSPKI:'24'.repeat(32),ca:'MA==',certificateChain:['MA==']},
    evmRegistry:{endpoint:'https://authority.example/wallet',token:'55'.repeat(32),payload:toBase64(payload),
      anchor:bytesToHex(p256.getPublicKey(key,false),false),
      proof:bytesToHex(p256.sign(sha256(concat(utf8Encode('N54G'),payload)),key).toCompactRawBytes(),false)}});
  let tree:Renderer.ReactTestRenderer|undefined;
  const tap=(label:string)=>Renderer.act(async()=>{
    tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0].props.onPress();
  });
  const events=jest.spyOn(AppState,'addEventListener').mockImplementation(()=>({remove(){}}));
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<WalletHome permission={async()=>true}
      honorExistingRental={false} rental={()=>{}} orders={()=>{}}/>);});
    await tap('온체인 토큰 권한');
    await tap('외부 DApp 연결 관리');
    const selected=tree!.root.findByType(WalletConnectScreen).props;
    expect(selected.stored).toEqual(expired);
    await Renderer.act(async()=>{jest.advanceTimersByTime(11_000);});
    await tap('새 연결 검토');
    const next=tree!.root.findByType(WalletConnectScreen).props;
    await Renderer.act(async()=>{await next.save(fresh,fresh.checkpoint.topic,()=>true);});
    await expect(selected.save(expired,expired.checkpoint.topic,()=>false)).rejects.toThrow('WC_CLOSED');
    await tap('온체인 토큰 권한');
    const output=JSON.stringify(tree!.toJSON());
    expect(output).toContain('https://new.example');
    expect(output).not.toContain('https://old.example');
  } finally {
    await Renderer.act(async()=>tree?.unmount());
    events.mockRestore();
    jest.useRealTimers();
  }
},30000);

test('W15 consolidates legacy and saved connections, disconnects exact identities, and keeps allowance separate',async()=>{
  const now=Math.floor(Date.now()/1000), signingKey=new Uint8Array(32).fill(7);
  const device='34'.repeat(32), genesis='0x'+'44'.repeat(32), sender='0x'+'11'.repeat(20);
  const token='0x'+'12'.repeat(20), spender='0x'+'23'.repeat(20), chain='11155111';
  const rpc='https://ledger.example/', endpoint='https://authority.example/wallet', relay='wss://relay.example';
  const origin='https://selected.example', otherOrigin='https://remaining.example';
  const anchor=bytesToHex(p256.getPublicKey(signingKey,false),false);
  const payload=utf8Encode(JSON.stringify({device,sender,generation:1,expires:now+120,leaseRevision:'2',
    dapp:{origin,chain,allowance:{token,spender,maximum:'5000000'}},walletConnect:{relay},
    networks:[{chain,genesis,rpc,feeLimit:'600000',assets:[{address:token,symbol:'DEMO',decimals:6}]}]}));
  function connect(site:string,keyByte:number,lastId:number) {
    const key=new Uint8Array(32).fill(keyByte);
    return {relay,seed:'55'.repeat(32),checkpoint:{topic:bytesToHex(sha256(key),false),key:bytesToHex(key,false),
      scope:{board:device,chain:Number(chain),genesis,account:sender,generation:1},lastId,expires:now+100,
      proposal:{origin:site,chain:'eip155:'+chain,account:'eip155:'+chain+':'+sender,
        peerKey:'31'.repeat(32),pairingTopic:'41'.repeat(32),methods:['eth_accounts'],events:[],verified:false}}};
  }
  const first=connect(origin,4,1), second=connect(otherOrigin,5,2);
  mockStored=JSON.stringify({version:2,exercised:true,mode:'personal',
    verified:{boardFingerprint:device,address:sender,generation:1,network:'8283'},
    walletConnect:first,walletConnectSessions:[connect(origin,4,3),second],
    evmRegistry:{endpoint,token:'55'.repeat(32),payload:toBase64(payload),anchor,
      proof:bytesToHex(p256.sign(sha256(concat(utf8Encode('N54G'),payload)),signingKey).toCompactRawBytes(),false)}});
  mockSigner.mockClear();
  mockPublish.mockClear();
  mockRpc.mockReset().mockImplementation(async (_url:string,packet:string)=>{
    const {method}=JSON.parse(packet);
    const result=method==='eth_chainId' ? '0xaa36a7' : method==='eth_getBlockByNumber' ? {hash:genesis} :
      method==='eth_call' ? '0x'+(5000000).toString(16).padStart(64,'0') : undefined;
    if (result===undefined) throw new Error('Unexpected signing or submission');
    return JSON.stringify({jsonrpc:'2.0',id:1,result});
  });
  let tree:Renderer.ReactTestRenderer|undefined;
  const app=<WalletHome permission={async()=>true} honorExistingRental={false} rental={()=>{}} orders={()=>{}}/>;
  const tap=(label:string)=>Renderer.act(async()=>{
    tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0].props.onPress();
  });
  const ui=()=>JSON.stringify(tree!.toJSON());
  const dialog=jest.spyOn(Alert,'alert').mockImplementation(()=>{});
  const events=jest.spyOn(AppState,'addEventListener').mockImplementation(()=>({remove(){}}));
  try {
    await Renderer.act(async()=>{tree=Renderer.create(app);});
    await tap('온체인 토큰 권한');
    expect(ui()).toContain('허용된 연결');
    expect(ui()).toContain(origin);
    expect(ui()).toContain(otherOrigin);
    expect(tree!.root.findAll(node=>node.props.accessibilityLabel==='세션 해제 · '+origin && typeof node.props.onPress==='function')).toHaveLength(1);
    await tap('확정 한도 조회');
    expect(ui()).toContain('5.000000 DEMO');
    await tap('전체 연결 해제');
    expect(dialog.mock.lastCall?.[1]).toContain('온체인');
    await Renderer.act(async()=>dialog.mock.lastCall?.[2]?.[0].onPress?.());
    expect(mockPublish).not.toHaveBeenCalled();
    await tap('세션 해제 · '+origin);
    expect(ui()).not.toContain(origin);
    expect(ui()).toContain(otherOrigin);
    expect(ui()).toContain('5.000000 DEMO');
    expect(JSON.parse(mockStored).walletConnect).toBeUndefined();
    expect(JSON.parse(mockStored).walletConnectSessions).toEqual([second]);
    expect(mockPublish).toHaveBeenCalledTimes(1);
    expect(mockPublish.mock.calls[0][0]).toBe(first.checkpoint.topic);
    await tap('전체 연결 해제');
    const staleConfirm=dialog.mock.lastCall?.[2]?.[1].onPress;
    await tap('등록 상태로 돌아가기');
    await Renderer.act(async()=>staleConfirm?.());
    expect(JSON.parse(mockStored).walletConnectSessions).toEqual([second]);
    await tap('온체인 토큰 권한');
    await tap('전체 연결 해제');
    const confirm=dialog.mock.lastCall?.[2]?.[1].onPress;
    await Renderer.act(async()=>{confirm?.();confirm?.();});
    expect(ui()).not.toContain(otherOrigin);
    expect(JSON.parse(mockStored).walletConnectSessions).toEqual([]);
    expect(mockPublish).toHaveBeenCalledTimes(2);
    expect(mockSigner).not.toHaveBeenCalled();
    expect(mockRpc.mock.calls.filter(([,packet])=>JSON.parse(packet).method==='eth_call')).toHaveLength(1);
  } finally {
    await Renderer.act(async()=>{tree?.unmount();});
    dialog.mockRestore();
    events.mockRestore();
  }
},30000);
