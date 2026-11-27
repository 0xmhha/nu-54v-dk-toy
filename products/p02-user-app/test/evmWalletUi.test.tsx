import React from 'react';
import ReactTestRenderer from 'react-test-renderer';
import {PixelRatio,TextInput} from 'react-native';
import {p256} from '@noble/curves/p256';
import {sha256} from '@noble/hashes/sha256';
import {keccak_256} from '@noble/hashes/sha3';
import {bigToBytes,bytesToHex,concat,hexToBytes,toBase64} from '@nu54/protocol';
import {WalletHome} from '../src/wallet/WalletHome';

const mockLoad=jest.fn(), mockRpc=jest.fn();
jest.mock('../src/specs/NativeOwnerWallet.ts',()=>({__esModule:true,default:{
  setHomeProtection:async()=>{},loadProfile:()=>mockLoad(),evmRpc:(endpoint:string,body:string)=>mockRpc(endpoint,body),saveProfile:async()=>{},compareAndSaveProfile:async()=>true,
}}));
jest.mock('../src/specs/NativeRenterBle.ts',()=>({__esModule:true,default:{loadDevices:async()=>''}}));

test.each([
  ['8283','e182205b','252000'],['11155111','e283aa36a7','252000'],['84532','e283014a34','253000'],
])('normal network %s previews the exact quote at a 200% scale and declines without signing',async(chain,prefix,total)=>{
  const fontScale=jest.spyOn(PixelRatio,'getFontScale').mockReturnValue(2);
  const now=BigInt(Math.floor(Date.now()/1000));
  const asset='0x'+'0'.repeat(40), sender='0x'+'13'.repeat(20), recipient='0x'+'12'.repeat(20);
  const device='34'.repeat(32), rpc='https://rpc.example.test', genesis='0x'+'44'.repeat(32), endpoint='https://authority.example.test/wallet';
  const key=new Uint8Array(32).fill(7), anchor=bytesToHex(p256.getPublicKey(key,false),false);
  const raw=hexToBytes('02'+prefix+'03030c82520894'+recipient.slice(2)+'0980c0');
  const l1Fee=chain==='84532'?1000n:0n;
  mockRpc.mockReset();
  const registry={device,sender,generation:1,expires:Number(now)+120,leaseRevision:'2',networks:[{
    chain,genesis,rpc,feeLimit:'300000',assets:[{address:asset,symbol:'ETH',decimals:0}],
  }]};
  const payload=Uint8Array.from(JSON.stringify(registry),char=>char.charCodeAt(0));
  mockLoad.mockResolvedValue(JSON.stringify({version:2,exercised:true,mode:'personal',
    verified:{boardFingerprint:device,address:sender,generation:1,network:'8283'},
    evmRegistry:{endpoint,token:'55'.repeat(32),payload:toBase64(payload),anchor,
      proof:bytesToHex(p256.sign(sha256(concat(Uint8Array.of(0x4e,0x35,0x34,0x47),payload)),key).toCompactRawBytes(),false)},
  }));
  mockRpc.mockImplementation(async (url:string,body:string)=>{
    expect(url).toBe(endpoint);
    expect(JSON.parse(body)).toMatchObject({method:'nu54_walletQuote',params:[{chain,asset,recipient,amount:'9'}]});
    const head=concat(Uint8Array.of(9),bigToBytes(now+60n,8),bigToBytes(BigInt(chain),8),bigToBytes(300000n,8),
      bigToBytes(l1Fee,8),new Uint8Array(20),hexToBytes(genesis.slice(2)),bigToBytes(1n,8),bigToBytes(2n,8));
    const statement=concat(Uint8Array.of(0x4e,0x35,0x34,0x56),hexToBytes(device),bigToBytes(1n,8),
      hexToBytes(sender.slice(2)),head,keccak_256(raw));
    const signed=p256.sign(sha256(statement),key).toCompactRawBytes();
    return JSON.stringify({jsonrpc:'2.0',id:1,result:bytesToHex(concat(head,signed,raw),false)});
  });

  let tree:ReactTestRenderer.ReactTestRenderer | undefined;
  const tap=(title:string)=>{
    const button=tree!.root.findAll(node=>node.props.accessibilityLabel===title && typeof node.props.onPress==='function')[0];
    expect(button).toBeDefined();
    expect(button.props.disabled).toBe(false);
    button.props.onPress();
  };
  const text=(title:string)=>tree!.root.findAllByType(TextInput).find(input=>input.props.accessibilityLabel===title);
  try {
    await ReactTestRenderer.act(async()=>{tree=ReactTestRenderer.create(<WalletHome permission={async()=>true} honorExistingRental={false} rental={()=>{}} orders={()=>{}}/>);});
    expect(JSON.stringify(tree!.toJSON())).toContain('개인 전송');
    await ReactTestRenderer.act(async()=>{tap('개인 전송');});
    expect(text('서명된 전송 정책')).toBeUndefined();
    expect(text('검증된 창세 블록 해시')).toBeUndefined();
    await ReactTestRenderer.act(async()=>{
      text('수취인')!.props.onChangeText(recipient);
      text('전송 금액')!.props.onChangeText('9');
    });
    await ReactTestRenderer.act(async()=>{tap('전송 내용 검토');});
    expect(mockRpc).toHaveBeenCalledTimes(1);
    const ui=JSON.stringify(tree!.toJSON());
    expect(ui).toContain(recipient);
    expect(ui).toContain(total);
    expect(ui).toContain('Nonce');
    if (l1Fee) expect(ui).toContain('Base L1 + operator');
    await ReactTestRenderer.act(async()=>{tap('거절하고 초안으로');});
    expect(mockRpc).toHaveBeenCalledTimes(1);
    expect(JSON.stringify(tree!.toJSON())).toContain('서명 0건 · 제출 0건');
    expect(ui).toContain('개인 기기 찾기');
  } finally {
    await ReactTestRenderer.act(async()=>{tree?.unmount();});
    fontScale.mockRestore();
  }
});
