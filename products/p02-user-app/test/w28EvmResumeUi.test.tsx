import React from 'react';
import Renderer from 'react-test-renderer';
import {Share,TextInput} from 'react-native';
import {p256} from '@noble/curves/p256';
import {sha256} from '@noble/hashes/sha256';
import {keccak_256} from '@noble/hashes/sha3';
import {bigToBytes,bytesToHex,concat,hexToBytes,toBase64,utf8Encode} from '@nu54/protocol';
import {WalletHome} from '../src/wallet/WalletHome';
import {readDirectTransfer,verifySignedTransfer} from '../src/wallet/evmTransfer';
import fixtures from './fixtures/w28-local-evm.json';

let mockStored='';
const mockRpc=jest.fn();
const mockSigner=jest.fn();
jest.mock('../src/specs/NativeOwnerWallet.ts',()=>({__esModule:true,default:{
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

test.each(fixtures)('W28 EVM $chain approves checked raw bytes and reloads the original finalized hash',async fixture=>{
  mockRpc.mockReset();mockSigner.mockReset();
  const now=BigInt(Math.floor(Date.now()/1000));
  const device='34'.repeat(32), provider='https://ledger.example/', authority='https://authority.example/wallet';
  const key=new Uint8Array(32).fill(7), anchor=bytesToHex(p256.getPublicKey(key,false),false);
  const asset='0x'+'0'.repeat(40);
  const l1Fee=fixture.chain==='84532'?1000n:0n;
  const registry={device,sender:fixture.sender,generation:1,expires:Number(now)+120,leaseRevision:'2',networks:[{
    chain:fixture.chain,genesis:fixture.genesis,rpc:provider,feeLimit:'300000',assets:[{address:asset,symbol:'ETH',decimals:0}]
  }]};
  const payload=utf8Encode(JSON.stringify(registry));
  mockStored=JSON.stringify({version:2,exercised:true,mode:'personal',
    verified:{boardFingerprint:device,address:fixture.sender,generation:1,network:'8283'},
    enrollment:{version:2,device,boardSPKI:'24'.repeat(32),ca:'MA==',certificateChain:['MA==']},
    evmRegistry:{endpoint:authority,token:'55'.repeat(32),payload:toBase64(payload),anchor,
      proof:bytesToHex(p256.sign(sha256(concat(utf8Encode('N54G'),payload)),key).toCompactRawBytes(),false)}});
  const raw=hexToBytes(fixture.unsigned);
  const header=concat(Uint8Array.of(9),bigToBytes(now+60n,8),bigToBytes(BigInt(fixture.chain),8),bigToBytes(300000n,8),
    bigToBytes(l1Fee,8),new Uint8Array(20),hexToBytes(fixture.genesis.slice(2)),bigToBytes(1n,8),bigToBytes(2n,8));
  const message=concat(utf8Encode('N54V'),hexToBytes(device),bigToBytes(1n,8),hexToBytes(fixture.sender.slice(2)),header,keccak_256(raw));
  const grant=bytesToHex(concat(header,p256.sign(sha256(message),key).toCompactRawBytes(),raw),false);
  mockSigner.mockImplementation(async (intent:string,sender:string)=>{
    expect(intent).toBe(grant);
    return {...verifySignedTransfer(readDirectTransfer(intent,now),hexToBytes(fixture.raw.slice(2)),sender),
      evidence:{board:device,generation:1,request:'56'.repeat(16),unsigned:'0x'+fixture.unsigned}};
  });
  mockRpc.mockImplementation(async (url:string,packet:string)=>{
    const {method,params}=JSON.parse(packet);
    let result:unknown;
    switch(method){
      case 'nu54_walletQuote':
        expect(url).toBe(authority);
        expect(params[0]).toMatchObject({chain:fixture.chain,amount:'9',recipient:fixture.recipient});
        result=grant;break;
      case 'eth_chainId':result='0x'+Number(fixture.chain).toString(16);break;
      case 'eth_getBlockByNumber':result=params[0]==='0x0'?{hash:fixture.genesis}:
        params[0]==='finalized'?fixture.finalized:fixture.block;break;
      case 'eth_sendRawTransaction':
        expect(JSON.parse(mockStored).pendingEvm).toMatchObject({submitted:true,hash:fixture.hash});
        expect(params).toEqual([fixture.raw]); result=fixture.hash;break;
      case 'eth_getTransactionReceipt':
        expect(params).toEqual([fixture.hash]);result=fixture.receipt;break;
      default:throw Error('Unexpected '+method);
    }
    if(method!=='nu54_walletQuote')expect(url).toBe(provider);
    return JSON.stringify({jsonrpc:'2.0',id:1,result});
  });
  let tree:Renderer.ReactTestRenderer|undefined;
  const app=<WalletHome permission={async()=>true} honorExistingRental={false} rental={()=>{}} orders={()=>{}}/>;
  const tap=(label:string)=>Renderer.act(async()=>{
    const control=tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0];
    expect(control.props.disabled).not.toBe(true);
    control.props.onPress();
  });
  try{
    await Renderer.act(async()=>{tree=Renderer.create(app);});
    await tap('개인 전송');
    await Renderer.act(async()=>{
      const fields=tree!.root.findAllByType(TextInput);
      fields.find(item=>item.props.accessibilityLabel==='수취인')!.props.onChangeText(fixture.recipient);
      fields.find(item=>item.props.accessibilityLabel==='전송 금액')!.props.onChangeText('9');
    });
    await tap('전송 내용 검토');
    expect(mockSigner).not.toHaveBeenCalled();
    await tap('개인 기기 찾기');
    await tap('기기에 전송 승인 요청');
    expect(mockSigner).toHaveBeenCalledTimes(1);
    expect(JSON.parse(mockStored).pendingEvm).toMatchObject({raw:fixture.raw,hash:fixture.hash,submitted:false,
      evidence:{board:device,generation:1,request:'56'.repeat(16),unsigned:'0x'+fixture.unsigned}});
    const share=jest.spyOn(Share,'share').mockResolvedValue({action:Share.dismissedAction,activityType:undefined});
    await tap('저장된 거래 내보내기');
    expect(JSON.parse(String(share.mock.calls[0][0].message))).toMatchObject({raw:fixture.raw,hash:fixture.hash});share.mockRestore();
    const saved=JSON.parse(mockStored);saved.pendingEvm.expires=Number(now)-1;mockStored=JSON.stringify(saved);
    await Renderer.act(async()=>{tree!.unmount();tree=Renderer.create(app);});await tap('개인 전송');
    expect(tree!.root.findAll(node=>node.props.accessibilityLabel==='만료된 미전송 초안 지우기')).toHaveLength(0);
    expect(JSON.parse(mockStored).pendingEvm.raw).toBe(fixture.raw);
    saved.pendingEvm.expires=Number(now)+60;mockStored=JSON.stringify(saved);
    await Renderer.act(async()=>{tree!.unmount();tree=Renderer.create(app);});await tap('개인 전송');
    await tap('이 네트워크에 전송');
    await Renderer.act(async()=>{tree!.unmount();tree=Renderer.create(app);});
    await tap('개인 전송');
    expect(JSON.stringify(tree!.toJSON())).toContain(fixture.hash);
    await tap('전송 상태 재조회');
    expect(JSON.stringify(tree!.toJSON())).toContain('선택한 RPC가 확정 결과를 보고했습니다.');
    expect(JSON.parse(mockStored).pendingEvm).toMatchObject({raw:fixture.raw,hash:fixture.hash,submitted:true});
    await Renderer.act(async()=>{tree!.unmount();tree=Renderer.create(app);});
    await tap('개인 전송');
    expect(JSON.stringify(tree!.toJSON())).toContain(fixture.hash);
    expect(JSON.stringify(tree!.toJSON())).toContain('휴대전화 조회는 보드 정산이 아니며');
    expect(tree!.root.findAll(node=>node.props.accessibilityLabel==='만료된 미전송 초안 지우기')).toHaveLength(0);
    expect(mockSigner).toHaveBeenCalledTimes(1);
    expect(mockRpc.mock.calls.filter(([,packet])=>JSON.parse(packet).method==='eth_sendRawTransaction')).toHaveLength(1);
  }finally{await Renderer.act(async()=>tree?.unmount());}
});

test.each(['legacy','8283','84532'] as const)('expired unresolved %s EVM never exposes a time-only unlock after restart',async kind=>{
 const f=fixtures[kind==='84532'?2:0],device='34'.repeat(32);
 const evm=kind==='legacy'?undefined:{request:'56'.repeat(16),unsigned:'0x'+f.unsigned,chain:f.chain,genesis:f.genesis,signer:f.sender};
 mockStored=JSON.stringify({version:2,exercised:true,mode:'personal',verified:{boardFingerprint:device,address:f.sender,generation:1,network:'8283'},
  enrollment:{version:2,device,boardSPKI:'24'.repeat(32),ca:'MA==',certificateChain:['MA==']},
  signing:{kind:'evm',digest:'ab'.repeat(32),board:device,generation:1,expires:1,...(evm?{evm}:{})}});
 const app=<WalletHome permission={async()=>true} honorExistingRental={false} rental={()=>{}} orders={()=>{}}/>;
 let tree:Renderer.ReactTestRenderer;
 try{
  await Renderer.act(async()=>{tree=Renderer.create(app);});
  expect(JSON.stringify(tree!.toJSON())).toContain('요청 만료만으로 서명이 무효가 되지는 않습니다');
  expect(tree!.root.findAll(node=>String(node.props.accessibilityLabel).includes('만료된 승인 확인'))).toHaveLength(0);
  expect(JSON.parse(mockStored).signing).toBeDefined();
  if(evm){const share=jest.spyOn(Share,'share').mockResolvedValue({action:Share.dismissedAction,activityType:undefined});
   await Renderer.act(async()=>{tree!.root.find(node=>node.props.accessibilityLabel==='미결 요청 내보내기'&&node.props.onPress).props.onPress();});
   expect(JSON.parse(String(share.mock.calls[0][0].message))).toMatchObject(evm);share.mockRestore();}
  await Renderer.act(async()=>{tree!.unmount();tree=Renderer.create(app);});
  expect(JSON.parse(mockStored).signing).toBeDefined();
 }finally{await Renderer.act(async()=>tree?.unmount());}
});
