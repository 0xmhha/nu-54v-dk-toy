import React from 'react';
import ReactTestRenderer from 'react-test-renderer';
import {Alert,PixelRatio,TextInput} from 'react-native';
import {bytesToHex} from '@nu54/protocol';
import {x25519} from '@noble/curves/ed25519';
import {reviewAllowancePrompt,WalletConnectScreen} from '../src/wallet/dapp/WalletConnectScreen';
import {seal,topicOf} from '../src/wallet/dapp/walletConnectCrypto';
import type {RelayMessage} from '../src/wallet/dapp/walletConnectRelay';
import PairingCamera from '../src/specs/NativeShopQr';

jest.mock('../src/specs/NativeShopQr',()=>({__esModule:true,default:{scan:jest.fn()}}));

let mockReceive:(message:RelayMessage)=>void;
let mockOldAck:()=>void;
const mockSubscribe=jest.fn(async (_topic:string)=>{});
const mockPublish=jest.fn(async (_topic:string,_message:string,tag:number)=>{
  if(tag===1120) await new Promise<void>(done=>{mockOldAck=done;});
});
jest.mock('../src/wallet/dapp/walletConnectRelay',()=>({
  IrnRelay:class {
    listen(handler:(message:RelayMessage)=>void) {mockReceive=handler;}
    subscribe(topic:string) {return mockSubscribe(topic);}
    publish(topic:string,message:string,tag:number) {return mockPublish(topic,message,tag);}
    close() {}
  },
}));

test('the wallet screen accepts a new URI while the relay still owes an acknowledgment for a declined proposal',async()=>{
  const scope={board:'11'.repeat(32),account:'0x'+'22'.repeat(20),generation:3,chain:11155111,genesis:'0x'+'33'.repeat(32)};
  const proposedKey=bytesToHex(x25519.getPublicKey(new Uint8Array(32).fill(31)),false);
  let rng=0;
  const verify=jest.fn(async (_board:string)=>{});
  const scale=jest.spyOn(PixelRatio,'getFontScale').mockReturnValue(2);
  const prompt=jest.spyOn(Alert,'alert').mockImplementation(()=>{});
  let tree:ReactTestRenderer.ReactTestRenderer|undefined;
  const hop=async(tap:()=>void)=>ReactTestRenderer.act(async()=>{tap();});
  const pair=async(key:Uint8Array)=>{
    const topic=topicOf(key);
    await hop(()=>tree!.root.findByType(TextInput).props.onChangeText('wc:'+topic+'@2?symKey='+bytesToHex(key,false)+'&relay-protocol=irn'));
    const button=tree!.root.findAll(node=>node.props.accessibilityLabel==='외부 DApp 연결 검토'&&typeof node.props.onPress==='function')[0];
    expect(button.props.disabled).toBe(false);
    await hop(()=>button.props.onPress());
    expect(mockSubscribe).toHaveBeenCalledWith(topic);
    return topic;
  };
  const proposal=(topic:string,key:Uint8Array,id:number)=>mockReceive({
    topic,tag:1100,message:seal(key,new Uint8Array(12).fill(id),{jsonrpc:'2.0',id,method:'wc_sessionPropose',
      params:{relays:[{protocol:'irn'}],proposer:{publicKey:proposedKey,metadata:{url:'https://test.example'}},
        requiredNamespaces:{eip155:{chains:['eip155:11155111'],methods:['eth_accounts'],events:[]}}}}),
  });
  try {
    await hop(()=>{tree=ReactTestRenderer.create(<WalletConnectScreen
      scope={scope} relay="wss://localhost" boards={[{address:'A',name:'Local board'}]}
      random={async length=>new Uint8Array(length).fill(++rng)} trust={async()=>undefined} request={async()=>''}
      save={async()=>{}} verify={verify} scan={()=>{}} back={()=>{}} />);});
    const first=new Uint8Array(32).fill(1), second=new Uint8Array(32).fill(2);
    const firstTopic=await pair(first);
    await hop(()=>proposal(firstTopic,first,1));
    expect(prompt).toHaveBeenCalledTimes(1);
    await hop(()=>prompt.mock.calls[0][2]![0].onPress?.());
    expect(mockPublish).toHaveBeenCalledWith(firstTopic,expect.any(String),1120);
    const secondTopic=await pair(second);
    await hop(()=>proposal(firstTopic,first,1));
    expect(prompt).toHaveBeenCalledTimes(1);
    await hop(()=>proposal(secondTopic,second,2));
    expect(prompt).toHaveBeenCalledTimes(2);
    await hop(()=>prompt.mock.calls[1][2]![1].onPress?.());
    await hop(()=>mockOldAck());
    expect(mockPublish).toHaveBeenCalledWith(secondTopic,expect.any(String),1101);
    expect(verify).toHaveBeenCalledTimes(2);
  } finally {
    await hop(()=>tree?.unmount());
    prompt.mockRestore();
    scale.mockRestore();
  }
});

test('the allowance connection action cannot authorize a dismissed or expired review',async()=>{
  const alert=jest.spyOn(Alert,'alert').mockImplementation(()=>{});
  try {
    const valid=new AbortController();
    const view=reviewAllowancePrompt('Allowance','exact terms',valid.signal);
    alert.mock.lastCall?.[2]?.find(button=>button.text==='연결 보기')?.onPress?.();
    await expect(view).resolves.toBe('connections');
    alert.mock.lastCall?.[2]?.find(button=>button.text==='승인')?.onPress?.();
    await expect(view).resolves.toBe('connections');
    const expired=new AbortController();
    const pending=reviewAllowancePrompt('Allowance','exact terms',expired.signal);
    expired.abort();
    alert.mock.lastCall?.[2]?.find(button=>button.text==='승인')?.onPress?.();
    await expect(pending).resolves.toBe('reject');
  } finally {alert.mockRestore();}
});

test('W13 scanning a valid external QR fills the URI without subscribing, signing or saving',async()=>{
  const key=new Uint8Array(32).fill(3);
  const uri='wc:'+topicOf(key)+'@2?symKey='+bytesToHex(key,false)+'&relay-protocol=irn';
  const scan=jest.mocked(PairingCamera!.scan).mockResolvedValue(uri);
  mockSubscribe.mockClear();
  const verify=jest.fn(async()=>{}), save=jest.fn(async()=>{}), request=jest.fn(async()=>'');
  let tree:ReactTestRenderer.ReactTestRenderer|undefined;
  try {
    await ReactTestRenderer.act(async()=>{tree=ReactTestRenderer.create(<WalletConnectScreen
      scope={{board:'11'.repeat(32),account:'0x'+'22'.repeat(20),generation:3,chain:11155111,genesis:'0x'+'33'.repeat(32)}}
      relay="wss://localhost" boards={[{address:'A',name:'Local board'}]} random={async length=>new Uint8Array(length)}
      trust={async()=>undefined} request={request} save={save} verify={verify} scan={()=>{}} back={()=>{}} />);});
    await ReactTestRenderer.act(async()=>tree!.root.findAll(node=>node.props.accessibilityLabel==='WalletConnect QR 읽기'
      && typeof node.props.onPress==='function')[0].props.onPress());
    expect(scan).toHaveBeenCalledTimes(1);
    expect(tree!.root.findByType(TextInput).props.value).toBe(uri);
    for (const action of [verify,save,request,mockSubscribe]) expect(action).not.toHaveBeenCalled();
  } finally {await ReactTestRenderer.act(async()=>tree?.unmount());}
});
