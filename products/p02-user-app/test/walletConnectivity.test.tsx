import React from 'react';
import Renderer from 'react-test-renderer';
import {Text} from 'react-native';
import {useWalletConnectivity} from '../src/wallet/walletConnectivity';

test('a late OS snapshot cannot overwrite default-network events', async () => {
  let dispatch: ((link:string)=>void) | undefined;
  let resolveSnapshot: ((link:string)=>void) | undefined;
  const removed=jest.fn();
  const source = {
    getNetworkLink: () => new Promise<string>(resolve => {resolveSnapshot=resolve;}),
    onNetworkLink: (listener: (link:string)=>void) => {
      dispatch=listener;
      return {remove:removed};
    },
  };
  let tree:Renderer.ReactTestRenderer | undefined;
  function State() {
    const value=useWalletConnectivity(source,true);
    return <Text>{value.link}:{value.revision}</Text>;
  }
  try {
    await Renderer.act(async()=>{tree=Renderer.create(<State/>);});
    await Renderer.act(async()=>{dispatch?.('offline');});
    await Renderer.act(async()=>{resolveSnapshot?.('online');});
    expect(JSON.stringify(tree!.toJSON())).toContain('offline');
    await Renderer.act(async()=>{dispatch?.('online');});
    expect(JSON.stringify(tree!.toJSON())).toContain('online');
  } finally {
    await Renderer.act(async()=>tree?.unmount());
    expect(removed).toHaveBeenCalledTimes(1);
  }
});
