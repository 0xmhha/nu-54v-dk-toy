import {decodeWalletStorage, type WalletStorage} from "../src/wallet/profile";
import {persistWalletSignature} from "../src/wallet/walletStorage";

import {p256} from "@noble/curves/p256";
import {sha256} from "@noble/hashes/sha256";

import {bigToBytes, bytesToHex, concat, fromBase64, hexToBytes, toBase64, utf8Encode} from '@nu54/protocol';
import {loadBip48Policy, prepareBip48, readBip48Import, verifyBip48Partial} from '../src/wallet/bip48Wallet';

const vector = require('../../p01-device-firmware/test/bip48_vector.json') as {
  unsignedPSBT: string; partialPSBTs: string[]; descriptors: string[];
};

function registration() {
  const alphabet = '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz';
  const roots = [...vector.descriptors[0].matchAll(/\[([a-f0-9]{8})\/48h\/1h\/0h\/2h\](tpub[1-9A-HJ-NP-Za-km-z]+)\/0\/\*/g)]
    .map(([, fp, key]) => {
      const value = Array.from(key).reduce((n, char) => n * 58n + BigInt(alphabet.indexOf(char)), 0n);
      return concat(hexToBytes(fp), bigToBytes(value, 82).subarray(13, 78));
    });
  return concat(Uint8Array.of(17, 1, 1, 0, 0, 0, 0), bigToBytes(100000n, 8), Uint8Array.of(0, 2), ...roots);
}

test('W17 verifies the three descriptor branches and the matching independent BIP143 partial', async () => {
  const bytes = registration();
  const scope = {board: '11'.repeat(32), generation: '1', leaseRevision: '2', anchor: '22'.repeat(32)};
  const payload = {policy: bytesToHex(bytes, false), descriptors: vector.descriptors};
  const rpc = jest.fn(async () => payload);
  const registered = await loadBip48Policy(rpc, scope);
  expect(registered.owner).toBe(0);
  expect(rpc).toHaveBeenCalledWith('nu54_bip48Policy', [scope]);
  const raw = fromBase64(vector.unsignedPSBT), own = fromBase64(vector.partialPSBTs[0]);
  expect(verifyBip48Partial(raw, own, registered)).toEqual({count: 1});
  expect(() => verifyBip48Partial(raw, fromBase64(vector.partialPSBTs[1]), registered)).toThrow('BIP48_SIGNER');
  expect(() => verifyBip48Partial(own, own, registered)).toThrow('BIP48_SIGNED_MISMATCH');
  expect(() => verifyBip48Partial(raw, Uint8Array.of(0), registered)).toThrow('CHAIN_USER_REFUSED');
  await expect(loadBip48Policy(async () => ({...payload, descriptors: [payload.descriptors[0], payload.descriptors[0]]}),
    scope)).rejects.toThrow('BIP48_DESCRIPTOR');
});

test('W17 refuses a gateway substitute for the user-imported original PSBT', async()=>{
  const key=new Uint8Array(32).fill(7), board='11'.repeat(32), anchor='22'.repeat(32);
  const scope={board,anchor,generation:'1',leaseRevision:'2'};
  const policy=await loadBip48Policy(async()=>({policy:bytesToHex(registration(),false),descriptors:vector.descriptors}),scope);
  const raw=fromBase64(vector.unsignedPSBT);
  const header=concat(Uint8Array.of(18,1,1,0,0,0,0),bigToBytes(BigInt(Math.floor(Date.now()/1000)+45),8),
    bigToBytes(100000n,8),policy.hash,new Uint8Array(58),sha256(utf8Encode(anchor)),bigToBytes(2n,8));
  const signed=sha256(concat(utf8Encode('N54M'),hexToBytes(board),bigToBytes(1n,8),header,sha256(raw)));
  const grant=bytesToHex(concat(header,p256.sign(signed,key).toCompactRawBytes(),raw),false);
  const prepare=(counterpart:string)=>prepareBip48(async()=>({grant}),scope,policy,p256.getPublicKey(key,false),{recipient:'',amount:'',counterpart});
  const approved = await prepare(vector.unsignedPSBT);
  expect(approved).toMatchObject({
    inputTotal: '100000000', outputTotal: '99999622', fee: '378',
    maximumVsize: 189, minimumFeeRate: '2.00',
    inputs: [{amount: '100000000', script: expect.stringMatching(/^0020[0-9a-f]{64}$/)}],
    outputs: [{satoshis: '99000000', script: expect.stringMatching(/^0014[0-9a-f]{40}$/)},
      {satoshis: '999622', change: true, path: "m/48'/1'/0'/2'/1/0"}],
  });
  await expect(prepare(vector.partialPSBTs[1])).rejects.toThrow('BIP48_ORIGINAL_MISMATCH');
});

test('W17 binds two complete input signatures to the same registered Signet policy',async()=>{
  const both=require('../../p01-device-firmware/test/bip48_two_input_vector.json') as typeof vector;
  const bytes=registration();
  bytes[2]=2;
  const profile={board:'11'.repeat(32),generation:'1',leaseRevision:'2',anchor:'33'.repeat(32),network:2 as const,account:0};
  const policy=await loadBip48Policy(async()=>({policy:bytesToHex(bytes,false),descriptors:both.descriptors}),profile);
  expect(readBip48Import(both.unsignedPSBT)).toBe(both.unsignedPSBT);
  expect(()=>readBip48Import('b'.repeat(2400))).toThrow('BIP48_PSBT_FORMAT');
  expect(verifyBip48Partial(fromBase64(both.unsignedPSBT),fromBase64(both.partialPSBTs[0]),policy)).toEqual({count:1});
  expect(()=>verifyBip48Partial(fromBase64(both.unsignedPSBT),fromBase64(both.partialPSBTs[1]),policy)).toThrow('BIP48_SIGNER');
});

test('W17 persists its two-input reservation before one board call and blocks a competing receipt',async()=>{
  const both=require('../../p01-device-firmware/test/bip48_two_input_vector.json') as typeof vector & {id:string};
  const owner={network:'8283' as const,address:'0x'+'ab'.repeat(20),generation:3,boardFingerprint:'11'.repeat(32)};
  const base:WalletStorage={version:2,exercised:true,mode:'personal',verified:owner};
  let protectedRecord=base;
  const commit=async(next:WalletStorage,expected:WalletStorage)=>{
    expect(expected).toEqual(protectedRecord);
    protectedRecord=decodeWalletStorage(JSON.stringify(next));
    return protectedRecord;
  };
  const raw=fromBase64(both.unsignedPSBT), expires=Math.floor(Date.now()/1000)+55, policy='22'.repeat(32);
  const header=concat(Uint8Array.of(18,1,2,0,0,0,0),bigToBytes(BigInt(expires),8),bigToBytes(100000n,8),
    hexToBytes(policy),new Uint8Array(58+32+8+64));
  const wire=bytesToHex(concat(header,raw),false);
  const issue=jest.fn(async()=>{
    expect(protectedRecord.signing?.kind).toBe('bip48');
    return {pendingBip48:{board:owner.boardFingerprint,generation:owner.generation,network:2 as const,account:0,
      id:both.id,original:both.unsignedPSBT,psbt:both.partialPSBTs[0],policy,count:1 as const}};
  });
  await persistWalletSignature(base,'bip48',wire,expires,commit,issue);
  expect(protectedRecord.signing).toBeUndefined();
  expect(protectedRecord.pendingBip48).toHaveProperty('id',both.id);
  await expect(persistWalletSignature(protectedRecord,'bip48',wire,expires,commit,issue))
    .rejects.toThrow('WALLET_SIGNING_UNRESOLVED');
  expect(issue).toHaveBeenCalledTimes(1);
});


test('W17 rejects both transport sizes before any signing path',async()=>{
  const oversized=toBase64(concat(hexToBytes('70736274ff'),new Uint8Array(1776)));
  expect(fromBase64(oversized)).toHaveLength(1781);
  expect(()=>readBip48Import(oversized)).toThrow('BIP48_PSBT_FORMAT');
  const scope={board:'11'.repeat(32),generation:'1',leaseRevision:'2',anchor:'33'.repeat(32)};
  const policy=await loadBip48Policy(async()=>({policy:bytesToHex(registration(),false),descriptors:vector.descriptors}),scope);
  const rpc=jest.fn();
  await expect(prepareBip48(rpc,scope,policy,new Uint8Array(65),{recipient:'',amount:'',counterpart:oversized}))
    .rejects.toThrow('BIP48_PSBT_FORMAT');
  expect(rpc).not.toHaveBeenCalled();
  expect(()=>verifyBip48Partial(fromBase64(vector.unsignedPSBT),new Uint8Array(1999),policy)).toThrow('BIP48_PSBT_FORMAT');
});
