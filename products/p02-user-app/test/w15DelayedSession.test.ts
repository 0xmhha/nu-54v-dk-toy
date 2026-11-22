import {bytesToHex} from '@nu54/protocol';
import {seal,topicOf} from '../src/wallet/dapp/walletConnectCrypto';
import type {RelayMessage} from '../src/wallet/dapp/walletConnectRelay';
import {WalletConnectSession} from '../src/wallet/dapp/walletConnectSession';

test('W15 navigation retires a spending callback waiting on the session checkpoint write',async()=>{
  const key=new Uint8Array(32).fill(7), topic=topicOf(key);
  const scope={board:'aa'.repeat(32),account:'0x'+'11'.repeat(20),generation:1,chain:11155111,genesis:'0x'+'ee'.repeat(32)};
  const caip='eip155:11155111';
  const proposal={origin:'https://registered.example',chain:caip,account:caip+':'+scope.account,verified:true,
    methods:['eth_accounts','eth_sendTransaction'],events:[],peerKey:'44'.repeat(32),pairingTopic:'55'.repeat(32)};
  let receive:((event:RelayMessage)=>void)|undefined;
  let entered!:()=>void, release!:()=>void;
  const saving=new Promise<void>(resolve=>{entered=resolve;});
  const finishSave=new Promise<void>(resolve=>{release=resolve;});
  const save=jest.fn(async()=>{entered();await finishSave;});
  const request=jest.fn(async()=>'0x'+'bb'.repeat(32));
  const publish=jest.fn(async()=>{});
  const session=new WalletConnectSession({scope,clock:()=>1000,random:async length=>new Uint8Array(length),allowSpend:true,
    save,request,approve:async()=>true,trust:async()=>undefined,rtc:{listen:handler=>{receive=handler;},
      subscribe:async()=>{},close:()=>{},publish}});
  await session.resume({topic,key:bytesToHex(key,false),scope,proposal,lastId:1,expires:1060});
  const packet={jsonrpc:'2.0',id:2,method:'wc_sessionRequest',params:{chainId:caip,
    request:{method:'eth_sendTransaction',params:[{from:scope.account,to:'0x'+'22'.repeat(20),value:'0x1'}]}}};
  receive?.({topic,tag:1108,message:seal(key,new Uint8Array(12).fill(1),packet)});
  await saving;
  session.suspend();
  release();
  await new Promise(resolve=>setTimeout(resolve,0));
  receive?.({topic,tag:1108,message:seal(key,new Uint8Array(12).fill(2),{...packet,id:3})});
  await new Promise(resolve=>setTimeout(resolve,0));
  expect(save).toHaveBeenCalledTimes(1);
  expect(request).not.toHaveBeenCalled();
  expect(publish).not.toHaveBeenCalled();
});
