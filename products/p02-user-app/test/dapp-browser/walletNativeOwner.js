/* global BigInt */
// QA seeds a disposable owner and substitutes the simulator's BLE/mTLS transport.
import {Platform,TurboModuleRegistry} from 'react-native';
import {bigToBytes,bytesToHex,concat,encodeV2StreamFrame,fromBase64,hexToBytes,toBase64,V2Journal,V2Link,V2StreamDecoder} from '@nu54/protocol';
import {p256} from '@noble/curves/p256';
import {sha256} from '@noble/hashes/sha256';
import api from './walletNativeEndpoint.js';

const platform=TurboModuleRegistry.getEnforcing('OwnerWallet');
let enrollment, link, stream;
const noopEvent=()=>({remove(){}});
const clock=()=>BigInt(Math.floor(Date.now()/1000));
async function qaPublicIdentity() {
  try { return await platform.publicIdentity(); }
  catch(error) {
    if (Platform.OS!=='android') throw error;
    // The Android emulator may lack hardware P-256. This disposable public
    // identifier is used only by the synthetic BLE/mTLS bridge.
    const scalar=new Uint8Array(32); scalar[31]=1;
    return toBase64(concat(hexToBytes('3059301306072a8648ce3d020106082a8648ce3d030107034200'),p256.getPublicKey(scalar,false)));
  }
}

async function request(path,body) {
  const response=await fetch(api+path,body===undefined ? {} : {method:'POST',
    headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
  if (!response.ok) throw new Error('Native wallet QA service rejected request');
  return response.json();
}
async function fixtureProfile() {
  if (!enrollment) enrollment=await request('/profile');
  return enrollment;
}

export default {
  loadProfile:async()=>{
    const fixture=await fixtureProfile();
    const stored=await platform.loadProfile();
    const registry=stored ? JSON.parse(stored).evmRegistry : undefined;
    if (registry?.payload===fixture.evmRegistry.payload && registry.anchor===fixture.evmRegistry.anchor) return stored;
    const state=JSON.stringify(fixture);
    if (!await platform.compareAndSaveProfile(stored, state)) throw new Error('QA wallet profile changed');
    return state;
  },
  saveProfile:value=>platform.saveProfile(value),
  compareAndSaveProfile:(expected,value)=>platform.compareAndSaveProfile(expected,value),
  setHomeProtection:enabled=>platform.setHomeProtection(enabled),
  getNetworkLink:()=>platform.getNetworkLink(),
  onNetworkLink:listener=>platform.onNetworkLink(listener),
  publicIdentity:qaPublicIdentity,
  signChallenge:record=>platform.signChallenge(record),
  randomBytes:length=>platform.randomBytes(length),
  speak:(text,lang)=>platform.speak(text,lang),
  pickPsbtFile:()=>platform.pickPsbtFile(),
  evmRpc:(endpoint,packet)=>platform.evmRpc(endpoint,packet),
  onTls:noopEvent,
  startTls:async(ca,chain,pin)=>{
    const fixture=await fixtureProfile();
    if (pin!==fixture.enrollment.boardSPKI || ca!==fixture.enrollment.ca ||
        chain.length!==1 || chain[0]!==fixture.enrollment.certificateChain[0]) throw new Error('QA enrollment mismatch');
    const epoch=fromBase64(await platform.randomBytes(16));
    const publicIdentity=fromBase64(await qaPublicIdentity());
    link=new V2Link('management','board',epoch,sha256(publicIdentity),new V2Journal(16));
    stream=new V2StreamDecoder();
    return JSON.stringify({epoch:toBase64(epoch),peer:pin});
  },
  feedTls:async()=>'{ }',
  stopTls:async()=>{link?.close();stream?.close();link=undefined;stream=undefined;},
  sendTls:async text=>{
    if (!link || !stream) throw new Error('QA session missing');
    let requestFrame;
    stream.feed(fromBase64(text),frame=>{requestFrame=link.receive(frame,clock())?.request;});
    if (!requestFrame) return '{}';
    const fixture=await fixtureProfile();
    const registry=fixture.evmRegistry;
    const identity=sha256(fromBase64(await qaPublicIdentity()));
    const op=requestFrame.payload[0];
    let payload;
    if (op===7) payload=concat(Uint8Array.of(3,1,2,1,1),bigToBytes(1n,8),
      hexToBytes(fixture.verified.boardFingerprint),identity,hexToBytes(fixture.verified.address.slice(2)));
    else if (op===10) payload=hexToBytes(await request('/chain-public',{wire:bytesToHex(requestFrame.payload,false)}));
    else if (op===11) payload=hexToBytes(await request('/chain-sign',{wire:bytesToHex(requestFrame.payload,false)}));
    else if ([17,18,19].includes(op)) payload=hexToBytes(await request('/joint-board',{wire:bytesToHex(requestFrame.payload,false)}));
    else if (op===13) payload=hexToBytes(await request('/chain-watch',{wire:bytesToHex(requestFrame.payload,false)}));
    else if (op===12) payload=concat(Uint8Array.of(12),hexToBytes(registry.anchor),bigToBytes(1n,8));
    else if ([9,14,15,20,36].includes(op)) payload=hexToBytes(await request('/board',{wire:bytesToHex(requestFrame.payload,false)}));
    else throw new Error('QA board unexpected operation');
    const reply=link.send({...requestFrame,command:'reply',payload});
    return JSON.stringify({clear:toBase64(concat(...reply.map(encodeV2StreamFrame)))});
  },
};
