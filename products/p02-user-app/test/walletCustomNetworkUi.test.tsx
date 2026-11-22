import React from 'react';
import Renderer from 'react-test-renderer';
import {TextInput} from 'react-native';
import {WalletCustomNetworkScreen} from '../src/wallet/WalletCustomNetworkScreen';
import {readCustomEvmDescriptor} from '../src/wallet/readProfiles';

test('W26 cancels a pending HTTPS verification and ignores its late response before retrying', async () => {
  const genesis = '0x' + '12'.repeat(32);
  const existing = readCustomEvmDescriptor({engine:'evm',name:'Dev net',chainId:'10001',genesis,rpc:'https://old.example/',
    explorer:'https://explorer.example/',currency:'DEV',decimals:18,feeCap:'50',confirmations:0});
  const requests: ((value: string) => void)[] = [];
  const save = jest.fn(async () => {});
  const exchange = jest.fn(async () => new Promise<string>(resolve => requests.push(resolve)));
  let tree: Renderer.ReactTestRenderer | undefined;
  const press = (label:string) => Renderer.act(async () => {
    const control = tree!.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onPress === 'function')[0];
    expect(control?.props.disabled).toBe(false);
    control.props.onPress();
  });
  const response = (result:unknown) => JSON.stringify({jsonrpc:'2.0',id:1,result});
  try {
    await Renderer.act(async () => {
      tree = Renderer.create(<WalletCustomNetworkScreen profiles={[existing]} save={save} readAddress={async()=>''}
        found={[]} discover={()=>{}} exchange={exchange} work={run=>{run().catch(()=>{});}} busy={false} back={()=>{}} t={value=>value}/>);
    });
    await press(existing.name);
    await Renderer.act(async () => tree!.root.findAllByType(TextInput).find(node=>node.props.accessibilityLabel==='검증된 HTTPS RPC 주소')!
      .props.onChangeText('https://candidate.example/'));
    await press('지문 확인 후 읽기 전용 RPC 저장');
    expect(JSON.stringify(tree!.toJSON())).toContain('RPC 네트워크 지문 확인 중');
    await press('검증 취소');
    await Renderer.act(async () => requests[0](response('0x2711')));
    expect(exchange).toHaveBeenCalledTimes(1);
    expect(save).not.toHaveBeenCalled();
    expect(JSON.stringify(tree!.toJSON())).toContain('검증을 취소했습니다.');
    await press('지문 확인 후 읽기 전용 RPC 저장');
    await Renderer.act(async () => requests[1](response('0x2711')));
    await press('검증 취소');
    await Renderer.act(async () => requests[2](response({hash:genesis})));
    expect(save).not.toHaveBeenCalled();
    await press('지문 확인 후 읽기 전용 RPC 저장');
    await Renderer.act(async () => requests[3](response('0x2711')));
    await Renderer.act(async () => requests[4](response({hash:genesis})));
    expect(save).toHaveBeenCalledTimes(1);
    expect(save).toHaveBeenCalledWith([{...existing,rpc:'https://candidate.example/'}]);
  } finally {
    await Renderer.act(async () => tree?.unmount());
  }
});
