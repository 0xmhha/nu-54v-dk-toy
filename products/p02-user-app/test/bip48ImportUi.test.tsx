import React from 'react';
import Renderer from 'react-test-renderer';
import {Bip48WalletScreen} from '../src/wallet/Bip48WalletScreen';
import {loadBip48Policy,prepareBip48} from '../src/wallet/bip48Wallet';
import type {OwnerSession} from '../src/wallet/ownerSession';

jest.mock('../src/wallet/bip48Wallet',()=>({
  ...jest.requireActual('../src/wallet/bip48Wallet'),
  loadBip48Policy:jest.fn(async()=>({bytes:new Uint8Array(224),hash:new Uint8Array(32),descriptors:[],account:0,owner:0})),
  prepareBip48:jest.fn(async()=>({fingerprints:[],inputs:[],outputs:[],id:'reviewed'})),
}));

test('W17 invalidates old review when a replacement picker opens, is canceled or returns after an owner change',async()=>{
  const sample=require('../../p01-device-firmware/test/bip48_vector.json') as {unsignedPSBT:string};
  const owner={boardFingerprint:'ab'.repeat(32),address:'0x'+'34'.repeat(20),generation:1,network:'8283' as const};
  const signBip48=jest.fn();
  const device={verifiedEvmAddress:async()=>owner.address,evmAuthority:async()=>({key:new Uint8Array(65),revision:1n}),
    bip48Status:async()=>true,signBip48} as unknown as OwnerSession;
  const pickFile=jest.fn<Promise<string>,[]>();
  const persist=jest.fn(async()=>{});
  let task:Promise<void>|undefined;
  let tree:Renderer.ReactTestRenderer|undefined;
  const render=(generation=1)=><Bip48WalletScreen verified={{...owner,generation}} found={[{address:'board',name:'Board'}]}
    busy={false} scan={()=>{}} back={()=>{}} prompt={()=>{}} rpc={async()=>''} t={value=>value}
    session={async(_handle,action)=>action(device)} work={run=>{task=run().catch(()=>{});}}
    pickFile={pickFile} persist={persist}/>;
  const button=(name:string)=>tree!.root.findAll(node=>node.props.accessibilityLabel===name && typeof node.props.onPress==='function')[0];
  const press=(name:string,wait=true)=>Renderer.act(async()=>{
    const action=button(name);
    expect(action.props.disabled).toBe(false);
    action.props.onPress();
    if (wait) await task;
  });
  const picker='PSBT 파일 또는 QR 이미지 선택';
  const review='PSBT 원문 검토';
  const sign='보드에 부분 서명 요청';
  try {
    await Renderer.act(async()=>{tree=Renderer.create(render());});
    await Renderer.act(async()=>{
      tree!.root.findAll(node=>node.props.accessibilityLabel==='네트워크 식별자' && typeof node.props.onChangeText==='function')[0]
        .props.onChangeText('12'.repeat(32));
    });
    await press('공동서명 지갑 확인 · Board');
    await press('공동서명 PSBT 가져오기');
    pickFile.mockResolvedValueOnce(sample.unsignedPSBT);
    await press(picker);
    await press(review);
    expect(button(sign)).toBeDefined();
    expect(prepareBip48).toHaveBeenCalledWith(expect.any(Function),expect.anything(),expect.anything(),
      expect.anything(),expect.objectContaining({counterpart:sample.unsignedPSBT}));

    let cancel:((error:unknown)=>void)|undefined;
    pickFile.mockImplementationOnce(()=>new Promise((_,reject)=>{cancel=reject;}));
    await press(picker,false);
    expect(button(sign)).toBeUndefined();
    expect(button(review).props.disabled).toBe(true);
    await Renderer.act(async()=>{cancel?.({code:'FILE_CANCELLED'});await task;});
    expect(button(review).props.disabled).toBe(true);

    let finish:((content:string)=>void)|undefined;
    pickFile.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;}));
    await press(picker,false);
    await Renderer.act(async()=>tree!.update(render(2)));
    await Renderer.act(async()=>tree!.update(render(1)));
    await Renderer.act(async()=>{finish?.(sample.unsignedPSBT);await task;});
    expect(JSON.stringify(tree!.toJSON())).not.toContain('PSBT 파일을 불러왔습니다.');
    expect(button(review)).toBeUndefined();
    expect(button(sign)).toBeUndefined();
    await press('공동서명 지갑 확인 · Board');
    pickFile.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;}));
    await press(picker,false);
    await Renderer.act(async()=>tree?.unmount());
    tree=undefined;
    await Renderer.act(async()=>{finish?.(sample.unsignedPSBT);await task;});
    expect(signBip48).not.toHaveBeenCalled();
    expect(persist).not.toHaveBeenCalled();
    expect(loadBip48Policy).toHaveBeenCalledTimes(2);
    expect(prepareBip48).toHaveBeenCalledTimes(1);
  } finally {await Renderer.act(async()=>tree?.unmount());}
});
