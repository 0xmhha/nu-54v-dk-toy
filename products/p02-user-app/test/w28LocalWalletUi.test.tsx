import React from 'react';
import Renderer from 'react-test-renderer';
import {TextInput} from 'react-native';
import {p256} from '@noble/curves/p256';
import {sha256} from '@noble/hashes/sha256';
import {bigToBytes,bytesToHex,concat,hexToBytes,utf8Encode} from '@nu54/protocol';
import {ChainWalletScreen} from '../src/wallet/ChainWalletScreen';
import {bitcoinHrp,bitcoinSignetCheckpoint,hash160,segwitAddress,xrplXAddress} from '../src/wallet/chainAccounts';
import {verifyChainSigned,type ChainTransfer,type StoredChain} from '../src/wallet/chainTransfer';
import type {OwnerSession} from '../src/wallet/ownerSession';
import {decodeWalletStorage} from '../src/wallet/profile';
import {readCommissionedChains} from '../src/wallet/commissionedChainProfiles';
import cases from './fixtures/w28-local-chains.json';
import signetFixture from './fixtures/w28-commissioned-bitcoin.json';
import xrplFixture from './fixtures/w28-commissioned-xrpl.json';

const commissioned = {
  1: {chain:1,profile:'bitcoin-signet',anchor:'00000008819873e925422c1ff0f99f7cc9bbb232af63a077a480a3633bee1ef6:'+bitcoinSignetCheckpoint.hash,rpc:'https://wallet-gateway.example/bitcoin'},
  2: {chain:2,profile:'solana-devnet',anchor:'EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG',rpc:'https://wallet-gateway.example/solana'},
  3: {chain:3,profile:'xrpl-testnet',anchor:'1:60691D3CA08E04064EBB014D0ADE8DA2CB2BFE0AE9D820D88502FB3A9A8F7149',rpc:'https://wallet-gateway.example/xrpl'},
};
const rows = [ ...cases.map(fixture=>({fixture, network:1 as const})),
  ...[{...signetFixture,replaceable:false,tag:undefined},...cases.filter(fixture=>fixture.chain===2), {...xrplFixture,replaceable:false,change:undefined,fingerprint:undefined}].map(fixture=>({fixture,network:2 as const})) ];

test.each(rows)('W28 network $network chain $fixture.chain previews, declines, signs and resumes by exact terminal ID',async ({fixture,network})=>{
  const owner={boardFingerprint:'ee'.repeat(32),address:'0x'+'ab'.repeat(20),generation:3,network:'8283' as const};
  const chain=fixture.chain as 1|2|3;
  const name=({1:'BTC',2:'SOL',3:'XRP'} as const)[chain];
  const profile={chain,network,account:0};
  const selected=network===2 ? readCommissionedChains([commissioned[chain]])[0] : undefined;
  const anchor=selected?.anchor ?? fixture.anchor;
  const publicKey=hexToBytes(fixture.publicKey);
  const source={...profile,publicKey,address:fixture.address,fingerprint:fixture.fingerprint};
  const key=new Uint8Array(32).fill(12), trust=p256.getPublicKey(key,false), revision=2n;
  const input=hexToBytes(fixture.reviewed), tx=input.slice(217);
  input[2]=network;
  const head=concat(input.subarray(0,7),bigToBytes(BigInt(Math.floor(Date.now()/1000)+58),8),input.subarray(15,113),
    sha256(utf8Encode(anchor)),bigToBytes(revision,8));
  const statement=concat(utf8Encode('N54M'),hexToBytes(owner.boardFingerprint),bigToBytes(3n,8),head,sha256(tx));
  const grant=bytesToHex(concat(head,p256.sign(sha256(statement),key).toCompactRawBytes(),tx),false);
  const signed=hexToBytes(fixture.signed);
  const sign=jest.fn(async(transfer:ChainTransfer)=>verifyChainSigned(transfer,source,signed));
  const board={evmAuthority:async()=>({key:trust,revision}),
    signChainTransfer:sign,
    verifiedChainAccount:async(_account:unknown,_address:string,_generation:bigint,branch?:number)=>{
      if (!branch) return source;
      const change=hexToBytes(fixture.change!);
      return {...source,publicKey:change,address:segwitAddress(bitcoinHrp(network),Uint8Array.of(0,20,...hash160(change)))};
    }} as unknown as OwnerSession;
  let submitted = false;
  const rpc=jest.fn(async(_endpoint:string,packet:string)=>{
    const {method,params,walletToken}=JSON.parse(packet);
    if (selected) {
      expect(_endpoint).toBe(selected.rpc);
      expect(walletToken).toMatch(/^[a-f0-9]{64}$/);
      expect(walletToken).not.toBe('42'.repeat(32));
    } else expect(walletToken).toBeUndefined();
    let result:unknown;
    switch(method) {
      case 'getblockchaininfo':result={chain:selected?'signet':'regtest'};break;
      case 'getblockhash':result=selected ? anchor.split(':')[params[0]===0?0:1] : anchor;break;
      case 'getGenesisHash':result=anchor;break;
      case 'ledger':result={validated:true,ledger_hash:anchor.slice(2),ledger_index:22470370};break;
      case 'isBlockhashValid':result={value:true};break;
      case 'getBlockHeight':result=1;break;
      case 'nu54_prepareWalletTransfer':result={grant};break;
      case 'nu54_walletTime': {
        const time=Math.floor(Date.now()/1000);
        const digest=sha256(concat(utf8Encode('N54C'),hexToBytes(owner.boardFingerprint),bigToBytes(3n,8),bigToBytes(revision,8),
          hexToBytes(params[0].nonce),bigToBytes(BigInt(time),8)));
        result={time,proof:bytesToHex(p256.sign(digest,key).toCompactRawBytes(),false)};
        break;
      }
      case 'nu54_submitCommissionedTransfer':
        expect(params).toEqual([{grant:bytesToHex(sha256(hexToBytes(grant)),false),id:fixture.id,signed:fixture.signed}]);
        expect(persisted?.attempted).toBe(true);
        expect(submitted).toBe(false);
        submitted=true;
        result=fixture.id;
        break;
      case 'ledger_current':result={ledger_current_index:5};break;
      case 'server_info':result={info:{network_id:1,validated_ledger:{seq:4}}};break;
      case 'getrawtransaction':
        expect(params[0]).toBe(fixture.id);
        result=selected && !submitted ? null : fixture.observed;
        break;
      case 'getSignatureStatuses':
        expect(params[0]).toEqual([fixture.id]);
        result=selected && !submitted ? {value:[null]} : fixture.observed;
        break;
      case 'tx':
        expect(params[0].transaction).toBe(fixture.id);
        result=selected && !submitted ? {error:'txnNotFound'} : fixture.observed;
        break;
      default:throw new Error('Unexpected method '+method);
    }
    return JSON.stringify({id:1,result});
  });
  let active:Promise<void>|undefined, tree:Renderer.ReactTestRenderer|undefined, persisted:StoredChain|undefined;
  const errors:string[]=[];
  const persistSignature=jest.fn(async(_wire:string,_expiry:number,signing:()=>Promise<StoredChain>)=>{persisted=await signing();});
  const actions={initial:'send' as const,verified:owner,preferred:profile,found:[{name:'Board',address:'board'}],
    commissioned:selected?[selected]:undefined, commissionedToken:'42'.repeat(32), checkCommissioned:jest.fn(),
    busy:false,rpc,persistSignature,choose:async()=>{},save:async(next?:StoredChain)=>{persisted=next;},scan:()=>{},back:()=>{},
    work:(run:()=>Promise<void>)=>{active=run().catch(error=>{errors.push(String(error));});},
    session:async<T,>(_handle:string,action:(session:OwnerSession)=>Promise<T>)=>action(board),
    prompt:jest.fn(),challenge:async()=>new Uint8Array(32).fill(56),t:(text:string)=>text};
  const press=(label:string)=>Renderer.act(async()=>{
    const button=tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0];
    expect(button.props.disabled).not.toBe(true);
    button.props.onPress();
    await active;
  });
  const type=(label:string,text:string)=>Renderer.act(async()=>{
    tree!.root.findAllByType(TextInput).find(input=>input.props.accessibilityLabel===label)!.props.onChangeText(text);
  });
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<ChainWalletScreen {...actions}/>);});
    if (!selected) await type('네트워크 식별자',anchor);
    await press(selected ? '등록된 시험망 확인' : '로컬 네트워크 확인');
    await type('수취인 주소',selected && chain===3 ? xrplXAddress(fixture.destination,'0',true) : fixture.destination);
    await type('보낼 금액',fixture.amount);
    if (chain===1 && !selected) await press('수수료 교체 없음');
    if (chain===3 && !selected) await type('목적지 태그','0');
    await press('거래 원문 검토 · Board');
    expect(JSON.stringify(tree!.toJSON())).toContain(fixture.destination);
    expect(JSON.stringify(tree!.toJSON())).toContain(name);
    expect(sign).not.toHaveBeenCalled();
    await press('거절하고 초안으로');
    expect(sign).not.toHaveBeenCalled();
    expect(persistSignature).not.toHaveBeenCalled();
    expect(submitted).toBe(false);
    await press('거래 원문 검토 · Board');
    if (chain===2) {
      const future=Date.now()+60000;
      const wall=jest.spyOn(Date,'now').mockReturnValue(future);
      try {await press('기기에 전송 승인 요청 · Board');} finally {wall.mockRestore();}
      expect(errors.pop()).toContain('CHAIN_REVIEW_EXPIRED');
      expect(persistSignature).not.toHaveBeenCalled();
      await press('거래 원문 검토 · Board');
    }
    await press('기기에 전송 승인 요청 · Board');
    expect(sign).toHaveBeenCalledTimes(1);
    expect(persisted).toMatchObject({id:fixture.id,signed:fixture.signed,review:{destination:fixture.destination}});
    if (selected) {
      await Renderer.act(async()=>{tree!.update(<ChainWalletScreen {...actions} pending={persisted}/>);});
      await press('이 네트워크에 전송');
      expect(errors).toEqual([]);
      expect(submitted).toBe(true);
    }
    const restored=decodeWalletStorage(JSON.stringify({version:2,exercised:true,mode:'personal',verified:owner,
      pendingChain:{...persisted,attempted:true}}));
    await Renderer.act(async()=>{tree!.unmount(); tree=Renderer.create(<ChainWalletScreen {...actions} pending={restored.pendingChain}/>);});
    expect(JSON.stringify(tree!.toJSON())).toContain(fixture.id);
    await press('전송 상태 재조회');
    expect(JSON.stringify(tree!.toJSON())).toContain('선택한 네트워크에서 확정되었습니다.');
    expect(sign).toHaveBeenCalledTimes(1);
  } finally {await Renderer.act(async()=>tree?.unmount());}
});
