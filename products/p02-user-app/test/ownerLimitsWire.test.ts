import {bigToBytes,concat,hexToBytes} from '@nu54/protocol';
import {readOwnerIdleLock,readOwnerLimits,ownerLimitChange,ownerLimitInitialize,ownerFinalityReconcile,ownerFinalityReplacement,ownerFinalityEnrollment,readOwnerFinalityFile} from '../src/wallet/ownerLimitsWire';
const board='ab'.repeat(32), token='11'.repeat(20), destination='22'.repeat(20);
const bytes=concat(Uint8Array.of(23,1),hexToBytes(board),bigToBytes(7n,8),bigToBytes(1n,8),bigToBytes(8283n,8),
 hexToBytes(token),hexToBytes(destination),bigToBytes(10000000n,8),bigToBytes(30000000n,8),bigToBytes(1000000n,8),Uint8Array.of(0));
test('policy read binds owner, board, numeric units and exact length',()=>{
 expect(readOwnerLimits(bytes,board,7n)).toMatchObject({revision:1n,perTransfer:10000000n,daily:30000000n,spent:1000000n});
 expect(()=>readOwnerLimits(bytes,board,8n)).toThrow();
 expect(()=>readOwnerLimits(bytes,'cd'.repeat(32),7n)).toThrow();
 expect(()=>readOwnerLimits(concat(bytes,Uint8Array.of(0)),board,7n)).toThrow();
 expect(readOwnerLimits(Uint8Array.of(23,0),board,7n)).toBeUndefined();
 expect(readOwnerLimits(Uint8Array.of(23,2),board,7n)).toBe('fresh');
 expect(readOwnerLimits(Uint8Array.of(23,3),board,7n)).toBe('finality-fresh');
 expect(readOwnerLimits(Uint8Array.of(23,4),board,7n)).toBe('finality-pending');
 expect(readOwnerLimits(concat(bytes,Uint8Array.of(2)),board,7n)).toMatchObject({mode:'finality'});
 expect(()=>readOwnerLimits(concat(bytes,Uint8Array.of(9)),board,7n)).toThrow();
});
test('change requests reject malformed and overflow values',()=>{
 const initial=ownerLimitInitialize('8283','0x'+token,'0x'+destination,'10000000','30000000');
 expect(initial).toEqual(concat(Uint8Array.of(26),bigToBytes(8283n,8),hexToBytes(token),hexToBytes(destination),bigToBytes(10000000n,8),bigToBytes(30000000n,8)));
 expect(initial).toHaveLength(65);
 expect(ownerLimitChange('12000000','30000000','0x'+destination)).toHaveLength(37);
 expect(()=>ownerLimitChange('12.0','30000000','0x'+destination)).toThrow();
 expect(()=>ownerLimitChange('18446744073709551616','30000000','0x'+destination)).toThrow();
 expect(()=>ownerLimitChange('12','10','0x'+destination)).toThrow();
});

test('finality forwarding binds board, owner generation and supported network',()=>{
 const body=new Uint8Array(237);body.set(hexToBytes('4e353446'),0);body.set(hexToBytes(board),4);body.set(bigToBytes(7n,8),188);body.set(bigToBytes(8283n,8),196);body[236]=1;
 const signature=new Uint8Array(64).fill(7),raw=Uint8Array.of(2,3);
 const result=ownerFinalityReconcile(body,signature,raw,board,7n);
 expect(result[0]).toBe(27);expect(result.length).toBe(306);expect(result.slice(-2)).toEqual(raw);
 expect(()=>ownerFinalityReconcile(body,signature,raw,'cd'.repeat(32),7n)).toThrow();
 expect(()=>ownerFinalityReconcile(body,signature,raw,board,8n)).toThrow();
 const other=body.slice();other.set(bigToBytes(84532n,8),196);expect(()=>ownerFinalityReconcile(other,signature,raw,board,7n)).toThrow();
 expect(()=>ownerFinalityReconcile(body,signature,new Uint8Array(257),board,7n)).toThrow();
});

test('finality enrollment binds issuer grant to board, signer and 8283',()=>{
 const grant=new Uint8Array(233);grant.set(hexToBytes('4e353444'));grant.set(hexToBytes(board),4);grant.set(bigToBytes(7n,8),36);grant.set(bigToBytes(8283n,8),44);grant.set(hexToBytes(token),52);
 const sig=new Uint8Array(64);expect(ownerFinalityEnrollment(grant,sig,board,7n,'0x'+token)).toHaveLength(298);
 expect(()=>ownerFinalityEnrollment(grant,sig,board,7n,'0x'+destination)).toThrow();
 const other=grant.slice();other.set(bigToBytes(84532n,8),44);expect(()=>ownerFinalityEnrollment(other,sig,board,7n,'0x'+token)).toThrow();
});

test('file import accepts only bounded typed signed evidence and grants',()=>{
 const enrollment=JSON.stringify({type:'nu54-finality-enrollment-v1',grant:'11'.repeat(233),signature:'22'.repeat(64)});
 expect(readOwnerFinalityFile(enrollment).kind).toBe('enrollment');
 const evidence=JSON.stringify({type:'nu54-finality-evidence-v1',body:'33'.repeat(237),signature:'44'.repeat(64),raw:'aa'.repeat(20)});
 expect(readOwnerFinalityFile(evidence).kind).toBe('evidence');
 expect(()=>readOwnerFinalityFile(evidence.replace('aa'.repeat(20),'aa'.repeat(257)))).toThrow();
 expect(()=>readOwnerFinalityFile(JSON.stringify({type:'nu54-finality-evidence-v1',body:'33'.repeat(237),signature:'44'.repeat(64),raw:'aa',extra:'x'}))).toThrow();
 expect(()=>readOwnerFinalityFile('{invalid')).toThrow();
});

test('replacement evidence carries both exact transactions and rejects the wrong scope',()=>{
 const body=new Uint8Array(269);body.set(hexToBytes('4e353452'));body.set(hexToBytes(board),4);body.set(bigToBytes(7n,8),188);body.set(bigToBytes(8283n,8),196);body[236]=3;
 const result=ownerFinalityReplacement(body,new Uint8Array(64),Uint8Array.of(2,3),Uint8Array.of(2,4),board,7n);
 expect(result[0]).toBe(31);expect(result.length).toBe(342);expect(result.slice(-4)).toEqual(Uint8Array.of(2,3,2,4));
 expect(()=>ownerFinalityReplacement(body,new Uint8Array(64),Uint8Array.of(2),new Uint8Array(257),board,7n)).toThrow();
 expect(readOwnerFinalityFile(JSON.stringify({type:'nu54-finality-replacement-v1',body:'11'.repeat(269),signature:'22'.repeat(64),raw:'aa',replacement:'bb'})).kind).toBe('replacement');
});

test('owner idle-lock capability requires the versioned device response', () => {
  expect(readOwnerIdleLock(Uint8Array.of(0x73, 1, 3, 0, 120))).toEqual({idleSeconds:120,disconnect:true});
  for (const unsupported of [[0x73,0], [0x73,2,3,0,120], [0x73,1,1,0,120], [0x73,1,3,0,119]])
    expect(() => readOwnerIdleLock(Uint8Array.from(unsupported))).toThrow('OWNER_IDLE_LOCK_UNAVAILABLE');
});
