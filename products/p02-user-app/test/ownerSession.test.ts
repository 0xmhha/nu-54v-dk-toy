/// <reference types="node" />
import evmFixtures from './fixtures/w28-local-evm.json';
import {keccak_256} from '@noble/hashes/sha3';
import {persistWalletSignature} from '../src/wallet/walletStorage';
import type {WalletStorage} from '../src/wallet/profile';
import {execFileSync} from 'child_process';
import {copyFileSync,mkdtempSync,rmSync} from 'fs';
import {tmpdir} from 'os';
import {resolve,join} from 'path';
import {p256} from '@noble/curves/p256';
import {sha256} from '@noble/hashes/sha256';
import {bigToBytes,bytesToHex,concat,encodeV2StreamFrame,equalBytes,fromBase64,hexToBytes,toBase64,utf8Encode,V2Journal,V2Link,V2StreamDecoder} from '@nu54/protocol';
import type {Spec as Ble} from '../src/specs/NativeRenterBle';
import type {Spec as Native} from '../src/specs/NativeOwnerWallet';
import {OwnerSession} from '../src/wallet/ownerSession';
import {ownerLimitChangeApplied} from '../src/wallet/ownerLimitsWire';
import {bitcoinWatchDescriptors,bitcoinXpubCommand,hash160} from '../src/wallet/chainAccounts';
import type {Enrollment} from '../src/wallet/ownerWire';

const device = 'd1'.repeat(32);
const pin = 'd2'.repeat(32);
const secret = new Uint8Array(32).fill(3);
const key = concat(hexToBytes('3059301306072a8648ce3d020106082a8648ce3d030107034200'),p256.getPublicKey(secret,false));
const owner = sha256(key);
const noOwner = new Uint8Array(32);
const oldOwner = new Uint8Array(32).fill(16);
const address = new Uint8Array(20).fill(32);
const outcome = (state: number,generation: bigint, activeOwner: Uint8Array = noOwner) =>
  concat(Uint8Array.of(3,state,0,0,0),bigToBytes(generation,8),hexToBytes(device),activeOwner);

function harness(options: {commerce?:{bytes:Uint8Array; mode:'ready'|'mismatch'|'refused'; calls:string[]; progress?:'live'|'loss'|'legacy'}; alreadyActive?:boolean; drop?:boolean; wrongPin?:boolean; staleFinal?:boolean; sourceEnabled?:boolean; sourceWrongEpoch?:boolean; lost?:'pending'|'committed'; wrongAddress?:boolean; approvalClock?:bigint; wrongApproval?:boolean; legacyApproval?:boolean; dropApproval?:boolean; policy?:'fresh'|'existing'; bridge?:{binary:string;state:string;deviceClock?:bigint}; bridgeDropChange?:boolean; bridgeDropActivation?:boolean; ownerBridge?:{binary:string;state:string;dropProof?:boolean;mode?:'lost'|'lost-no-proof'}; backupReady?:boolean; backupAddressWrongGeneration?:boolean; maintenance?:{busy?:boolean; moved?:boolean; onProof:()=>void}; running?:{mode:'current'|'unsupported'|'wrong-board'|'moved'; onRead:()=>void}; power?:{mode:'good'|'unknown'|'moved'; onRead:()=>void}; tools?:(opcode:number)=>Uint8Array; watch?:{proof:Uint8Array;roots:[Uint8Array,Uint8Array];onExport:()=>void}; evm?:{signer:string;anchor:Uint8Array;signed:Uint8Array;onRequest:(id:Uint8Array)=>void}} = {}) {
  const epoch = new Uint8Array(16).fill(7);
  const board = new V2Link('management','board',epoch,owner,new V2Journal(16));
  const decoder = new V2StreamDecoder();
  const credential: Enrollment = {version:2,device,boardSPKI:pin,ca:'MA==',certificateChain:['MA==']};
  let next = 0;
  let active = options.alreadyActive ?? false;
  let challenge: Uint8Array = new Uint8Array();
  let requestID = 0;
  let firmwareProofIssued=false;
  let runningRead=false;
  let powerRead=false;
  let wireDisplay: {operation:number;session:string;request:string;expires:string}|undefined;
  const inbound: number[] = [];
  const leading: number[] = [];
  const ble = {
    connectOwner:async () => 23,
    disconnect:async () => {},
    onDisconnect:() => ({remove(){}}),
    onFragment:() => ({remove(){}}),
    writeFragment:async (value: string) => {const bytes=fromBase64(value);leading.push(bytes[0]);return inbound.push(bytes.length);},
  } as unknown as Ble;
  const native = {
    onTls:() => ({remove(){}}),
    startTls:async () => JSON.stringify({out:toBase64(new Uint8Array(33)),epoch:toBase64(epoch),peer:options.wrongPin ? 'ee'.repeat(32) : pin}),
    stopTls:async () => {},
    randomBytes:async () => toBase64(new Uint8Array(16).fill(++requestID)),
    publicIdentity:async () => toBase64(key),
    signChallenge:async (record: string) => toBase64(p256.sign(sha256(fromBase64(record)),secret).toDERRawBytes()),
    sendTls:async (value: string) => {
      const replies: Uint8Array[] = [];
      let closed = false;
      decoder.feed(fromBase64(value),frame => {
        const message = board.receive(frame,BigInt(Math.floor(Date.now()/1000)));
        if (!message) return;
        const {id,expires,payload} = message.request;
        if ([24,26,28,33,34].includes(payload[0]))
          wireDisplay={operation:payload[0],session:bytesToHex(epoch,false),request:bytesToHex(id,false),expires:expires.toString()};
        let reply: Uint8Array;
        if (payload[0] === 0x76) {
          const commerce = options.commerce!;
          commerce.calls.push('probe');
          if (commerce.progress === 'legacy') {closed=true;return;}
          expect(payload).toEqual(concat(Uint8Array.of(0x76),bigToBytes(1n,8)));
          for(const response of board.send({command:'reply',id,expires,payload}))replies.push(encodeV2StreamFrame(response));
          board.enableApprovalProgress(1n);
          return;
        }
        if(options.ownerBridge && [1,2,3,7].includes(payload[0])) {
          const result=execFileSync(options.ownerBridge.binary,[options.ownerBridge.state,bytesToHex(payload,false),bytesToHex(owner,false),bytesToHex(id,false),expires.toString(),options.ownerBridge.mode??'new'],{encoding:'utf8'}).trim();
          if(payload[0]===2 && options.ownerBridge.dropProof){closed=true;return;}
          reply=hexToBytes(result);
          for(const response of board.send({command:'reply',id,expires,payload:reply}))replies.push(encodeV2StreamFrame(response));
          if(payload[0]===2)closed=true;
          return;
        }
        if (options.bridge && [23,24,25,26,32,33,34,35].includes(payload[0])) {
          const result=execFileSync(options.bridge.binary,[options.bridge.state,bytesToHex(payload,false),expires.toString(),bytesToHex(id,false),...(options.bridge.deviceClock!==undefined?[options.bridge.deviceClock.toString()]:[])],{encoding:'utf8'}).trim();
          reply=hexToBytes(result);
          if ([24,33,34].includes(payload[0]) && options.bridgeDropChange) {closed=true;return;}
          if (((payload[0]===25 && reply[1]===1) || (payload[0]===35 && reply[2]===1)) && options.bridgeDropActivation) {closed=true;return;}
          for (const response of board.send({command:'reply',id,expires,payload:reply})) replies.push(encodeV2StreamFrame(response));
          if ([24,26,33,34].includes(payload[0])) closed=true;
          return;
        }
        if (message.request.command === 'pay.prepare') {
          const commerce = options.commerce!;
          expect(payload).toEqual(commerce.bytes);
          commerce.calls.push('prepare');
          if (commerce.progress === 'live' || commerce.progress === 'loss') {
            for (const response of board.send({command:'approval.progress', id, expires, payload:concat(Uint8Array.of(1),bigToBytes(1n,8))}))
              replies.push(encodeV2StreamFrame(response));
            if (commerce.progress === 'loss') {closed=true;return;}
          }
          for (const response of board.send({command:'reply', id, expires, payload:Uint8Array.of(0)}))
            replies.push(encodeV2StreamFrame(response));
          return;
        }
        switch (payload[0]) {
          case 0x75: {
            const commerce = options.commerce!;
            expect(payload.subarray(1,17)).toEqual(new Uint8Array(16).fill(100));
            expect(payload.subarray(17)).toEqual(commerce.bytes);
            commerce.calls.push('verified');
            const digest = sha256(payload);
            if (commerce.mode==='mismatch') digest[0] ^= 1;
            reply = concat(Uint8Array.of(0x75),digest,Uint8Array.of(commerce.mode === 'refused' ? 0 : 1));
            break;
          }
          case 3:
            if(options.maintenance?.busy){closed=true;return;}
            reply = active ? outcome(1,(options.maintenance?.moved&&firmwareProofIssued || options.running?.mode==='moved'&&runningRead || options.power?.mode==='moved'&&powerRead)?2n:1n,owner) : outcome(0,0n);
            if(active && options.backupReady)reply.set([2,1,1],2);
            break;
          case 0x70:
          case 0x71:
          case 0x72:
          case 0x73:
            if(!options.tools)throw new Error('unexpected board tool');
            reply=options.tools(payload[0]);
            break;
          case 0x77: {
            if (!options.running) throw new Error('unexpected running firmware read');
            expect(payload).toEqual(concat(Uint8Array.of(0x77),bigToBytes(1n,8)));
            options.running.onRead();
            runningRead=true;
            const boardID = options.running.mode==='wrong-board' ? 'ee'.repeat(32) : device;
            reply=options.running.mode==='unsupported' ? Uint8Array.of(0x77) :
              hexToBytes('77010203010200010002000002000000000000000001'+boardID+'dc'.repeat(32));
            break;
          }
          case 0x78: {
            if (!options.power) throw new Error('unexpected power read');
            expect(payload).toEqual(concat(Uint8Array.of(0x78),bigToBytes(1n,8)));
            options.power.onRead();
            powerRead=true;
            const observation=options.power.mode==='unknown'?0:2;
            reply=concat(Uint8Array.of(0x78,1,observation),bigToBytes(1n,8),hexToBytes(device));
            break;
          }
          case 16:
            if(!options.maintenance)throw new Error('unexpected maintenance evidence');
            options.maintenance.onProof();
            firmwareProofIssued=true;
            reply=concat(Uint8Array.of(16),hexToBytes('4e353446'),payload.subarray(1),new Uint8Array(128));
            break;
          case 7:
            if(options.backupReady && !options.lost){
              const ready=outcome(1,options.backupAddressWrongGeneration?2n:1n,owner);
              ready.set([2,1,1],2);reply=concat(ready,address);break;
            }
            if(options.evm && payload.length===1){const ready=outcome(1,1n,owner);ready.set([2,1,1],2);reply=concat(ready,hexToBytes(options.evm.signer.slice(2)));break;}
            if (payload.length === 41 && !options.legacyApproval) {
              if (options.dropApproval) { closed = true; return; }
              const ready = outcome(1, 1n, owner);
              ready.set([2, 1, 1], 2);
              const fields = payload.slice(1);
              if (options.wrongApproval) fields[0] ^= 1;
              reply = concat(ready, address, fields, bigToBytes(options.approvalClock ?? 0n, 8));
              break;
            }
            reply = concat(options.lost === 'committed' ? outcome(1,2n,owner) : outcome(1,1n,oldOwner),
              options.wrongAddress ? new Uint8Array(20).fill(5) : address);
            break;
          case 13:
            if (!options.watch) throw new Error('unexpected watch export');
            options.watch.onExport();
            reply=options.watch.proof;
            break;
          case 10: {
            if (!options.watch) throw new Error('unexpected public account read');
            const child=options.watch.roots[payload[10]];
            reply=concat(payload,child,hash160(child),options.watch.proof.slice(7,11));
            break;
          }
          case 12:
            if(!options.evm)throw new Error('unexpected authority');reply=concat(Uint8Array.of(12),options.evm.anchor,bigToBytes(2n,8));break;
          case 9:
            if(!options.evm)throw new Error('unexpected signing');options.evm.onRequest(id);expect(options.evm.signed.length).toBeGreaterThan(0);closed=true;return;
          case 23:
            if (options.policy==='fresh') reply=Uint8Array.of(23,2);
            else if (options.policy==='existing') reply=concat(Uint8Array.of(23,1),hexToBytes(device),bigToBytes(1n,8),bigToBytes(1n,8),bigToBytes(8283n,8),
              new Uint8Array(20).fill(4),new Uint8Array(20).fill(5),bigToBytes(10000000n,8),bigToBytes(30000000n,8),bigToBytes(0n,8),Uint8Array.of(0));
            else reply=Uint8Array.of(23,0);
            break;
          case 28:
            expect(payload.length).toBe(298);reply=Uint8Array.of(28,1);closed=true;break;
          case 29:
            expect(payload).toEqual(Uint8Array.of(29));reply=Uint8Array.of(29,2);break;
          case 31:
            expect(payload.length).toBeGreaterThan(338);reply=Uint8Array.of(31,1);break;
          case 27:
            expect(payload.length).toBeGreaterThan(304);reply=Uint8Array.of(27,1);break;
          case 26:
            expect(options.policy).toBe('fresh');expect(payload.length).toBe(65);reply=Uint8Array.of(26,1);closed=true;break;
          case 24:
            expect(options.policy).toBe('existing');expect(payload.length).toBe(37);reply=concat(Uint8Array.of(24,1),bigToBytes(expires+86400n,8));closed=true;break;
          case 6: {
            expect(payload).toEqual(Uint8Array.of(6));
            reply = options.sourceEnabled ? concat(Uint8Array.of(6),new Uint8Array(32).fill(12),id,hexToBytes(device),
              owner,options.sourceWrongEpoch ? new Uint8Array(16).fill(19) : epoch,bigToBytes(1n,8),bigToBytes(expires,8)) : Uint8Array.of(6);
            break;
          }
          case 1:
            expect(payload.length).toBe(34);
            expect(equalBytes(payload.subarray(2),owner)).toBe(true);
            challenge = concat(Uint8Array.of(1,payload[1]),hexToBytes(device),bigToBytes(active || options.lost ? 1n : 0n,8),
              owner,new Uint8Array(32).fill(11),id,bigToBytes(expires,8));
            reply = challenge;
            break;
          case 2: {
            expect(equalBytes(payload.subarray(1,131),challenge)).toBe(true);
            const size = payload[131];
            expect(equalBytes(payload.subarray(132,132+size),key)).toBe(true);
            const signature = payload.subarray(133+size);
            expect(p256.verify(signature,sha256(challenge),key.subarray(26),{format:'der'})).toBe(true);
            active = challenge[1] === 1;
            next++;
            if (options.drop) {
              closed = true;
              return;
            }
            reply = challenge[1] === 5 ? outcome(1,2n,owner) :
              active ? outcome(1,options.staleFinal?0n:1n,owner) : outcome(3,2n);
            closed = true;
            break;
          }
          default: throw new Error('unexpected request');
        }
        for (const response of board.send({command:'reply',id,expires,payload:reply})) replies.push(encodeV2StreamFrame(response));
      });
      return JSON.stringify({clear:toBase64(concat(...replies)),closed});
    },
  } as unknown as Native;
  return {currentDisplay:()=>wireDisplay,session:new OwnerSession(ble,native,credential),inbound,leading,applied:() => next};
}

test.each(['ready','mismatch','refused'] as const)('commerce gates prepare on exact authenticated board evidence (%s)', async mode => {
  const bytes = new Uint8Array(737).fill(4);
  const calls: string[] = [];
  const {session} = harness({alreadyActive:true, commerce:{bytes,mode,calls}});
  await session.connect('fixture');
  try {
    const payment = {
      bytes, payer:bytesToHex(address), generation:1n, digest:'1'.repeat(64),
      approvalDigest:'2'.repeat(64), deadline:Math.floor(Date.now()/1000)+60,
    };
    await expect(session.approvePayment(payment,new Uint8Array(16).fill(100),() => calls.push('ready')))
      .rejects.toThrow(mode==='mismatch' ? 'PAYMENT_BOARD_EVIDENCE' : 'PAYMENT_REFUSED');
    expect(calls).toEqual(mode==='ready' ? ['verified','ready','prepare'] : ['verified']);
  } finally {
    session.close();
  }
});

test.each(['live','loss','legacy'] as const)('owner progress preserves the original approval lifecycle (%s)', async progress => {
  const bytes = new Uint8Array(737).fill(4);
  const calls: string[] = [];
  const {session} = harness({alreadyActive:true,commerce:{bytes,mode:'ready',calls,progress}});
  await session.connect('fixture');
  const scope = jest.fn();
  const deadline = Math.floor(Date.now()/1000)+60;
  const payment = {bytes,payer:bytesToHex(address),generation:1n,digest:'1'.repeat(64),approvalDigest:'2'.repeat(64),deadline};
  const request = new Uint8Array(16).fill(100);
  try {
    if (progress === 'legacy') {
      await expect(session.negotiateApprovalProgress()).rejects.toThrow('OWNER_DISCONNECTED');
      expect(calls).toEqual(['probe']);
      expect(scope).not.toHaveBeenCalled();
      return;
    }
    await session.negotiateApprovalProgress();
    await expect(session.approvePayment(payment,request,()=>calls.push('local'),scope))
      .rejects.toThrow(progress === 'loss' ? 'OWNER_DISCONNECTED' : 'PAYMENT_REFUSED');
    expect(calls).toEqual(['probe','verified','local','prepare']);
    expect(scope.mock.calls).toEqual([[{
      device,owner:bytesToHex(owner,false),generation:1n,request:bytesToHex(request,false),expires:BigInt(deadline),
    }],[undefined]]);
  } finally {session.close();}
});

test('owner enrollment binds a P-256 proof to the exact challenge and handles final-and-close', async () => {
  const {session,inbound,leading,applied} = harness();
  await session.connect('synthetic');
  let asked = 0;
  expect(await session.register(() => {expect(applied()).toBe(0); asked++;})).toMatchObject({
    state:'active',generation:1n,owner:bytesToHex(owner,false),
  });
  expect(asked).toBe(1);
  // The first write carries the C1 route byte before the 33-byte TLS start; later writes do not.
  expect(inbound).toEqual([20,14]);
  expect(leading).toEqual([1,0]);
});

test('commit and dropped reply remain unknown to the phone, even with valid proof', async () => {
  const {session,applied} = harness({drop:true});
  await session.connect('synthetic');
  await expect(session.register(() => {})).rejects.toThrow('OWNER_DISCONNECTED');
  expect(applied()).toBe(1);
  const newConnection = harness({alreadyActive:true});
  await newConnection.session.connect('synthetic');
  expect(await newConnection.session.status()).toMatchObject({state:'active',generation:1n});
  newConnection.session.close();
});

test('wrong board pin and a proof reply with stale generation are refused', async () => {
  const wrong = harness({wrongPin:true});
  await expect(wrong.session.connect('synthetic')).rejects.toThrow('OWNER_DISCONNECTED');
  const stale = harness({staleFinal:true});
  await stale.session.connect('synthetic');
  await expect(stale.session.register(() => {})).rejects.toThrow('OWNER_PHYSICAL_CONFIRMATION');
});

test('revocation accepts only a final reply for the next owner generation with zero owner', async () => {
  const {session} = harness({alreadyActive:true});
  await session.connect('synthetic');
  expect(await session.revoke(() => {})).toMatchObject({state:'revoked',generation:2n,owner:'00'.repeat(32)});
});

test('source backup exposes only a public request on the current owner TLS session', async () => {
  const {session} = harness({alreadyActive:true,sourceEnabled:true});
  await session.connect('synthetic');
  const backup = await session.beginBackup();
  expect(Object.keys(backup).sort()).toEqual(['checkpoint','expires','requestId']);
  expect(backup.checkpoint).toEqual({device,owner:bytesToHex(owner,false),generation:'1'});
  expect(backup.requestId).toBe('02'.repeat(16));
  session.close();
});

test('backup completion requires the same active board generation and its verified recovery state',async()=>{
 const waiting=harness({alreadyActive:true,sourceEnabled:true});await waiting.session.connect('original');await waiting.session.beginBackup();
 await expect(waiting.session.checkBackup()).resolves.toEqual({verified:false});waiting.session.close();
 const complete=harness({alreadyActive:true,sourceEnabled:true,backupReady:true});await complete.session.connect('original');await complete.session.beginBackup();
 await expect(complete.session.checkBackup()).resolves.toMatchObject({verified:true,address:bytesToHex(address,true),status:{generation:1n}});complete.session.close();
 await expect(complete.session.checkBackup()).rejects.toThrow('BACKUP_SESSION_REQUIRED');
 const changed=harness({alreadyActive:true,sourceEnabled:true,backupReady:true,backupAddressWrongGeneration:true});
 await changed.session.connect('original');await changed.session.beginBackup();
 await expect(changed.session.checkBackup()).rejects.toThrow('OWNER_BACKUP_ADDRESS_BINDING');changed.session.close();
});

test('source backup rejects a disabled board, wrong epoch and missing ownership', async () => {
  for (const [opts,reason] of [
    [{alreadyActive:true},'BACKUP_UNAVAILABLE'],
    [{alreadyActive:true,sourceEnabled:true,sourceWrongEpoch:true},'RECOVERY_CONTEXT'],
    [{sourceEnabled:true},'OWNER_MISMATCH'],
  ] as const) {
    const {session} = harness(opts);
    await session.connect('synthetic');
    await expect(session.beginBackup()).rejects.toThrow(reason);
    session.close();
  }
});


test('lost phone proof and fresh status bind the next generation and original address', async () => {
  const before = harness({lost:'pending',drop:true});
  await before.session.connect('original');
  const previous = await before.session.beginLostPhone();
  expect(previous).toMatchObject({device,replacement:bytesToHex(owner,false),generation:1n,address:bytesToHex(address,true)});
  await expect(before.session.proveLostPhone(() => {})).rejects.toThrow('OWNER_DISCONNECTED');
  expect(before.applied()).toBe(1);

  const checkpoint = {...previous,generation:previous.generation.toString()};
  const committed = harness({lost:'committed'});
  await committed.session.connect('original');
  expect(await committed.session.verifyLostPhone(checkpoint)).toMatchObject({state:'active',generation:2n});
  committed.session.close();

  const otherAddress = harness({lost:'committed',wrongAddress:true});
  await otherAddress.session.connect('original');
  await expect(otherAddress.session.verifyLostPhone(checkpoint)).rejects.toThrow('OWNER_REPLACEMENT_UNVERIFIED');
  otherAddress.session.close();

  const unchanged = harness({lost:'pending'});
  await unchanged.session.connect('original');
  await expect(unchanged.session.verifyLostPhone(checkpoint)).rejects.toThrow('OWNER_REPLACEMENT_NOT_COMMITTED');
  unchanged.session.close();
});

test('unresolved approval requires the exact authenticated board expiry even across app sessions', async () => {
  const reservation = {kind: 'chain' as const, board: device, generation: 1, expires: 500, digest: 'ec'.repeat(32)};
  const expectedAddress = bytesToHex(address,true);
  for (const options of [{approvalClock: 499n}, {approvalClock: 500n, wrongApproval: true},
    {approvalClock: 500n, legacyApproval: true}, {approvalClock: 500n, dropApproval: true}]) {
    const {session} = harness(options);
    await session.connect('board');
    // The handset clock is far past 500. The board's trusted lower bound governs.
    await expect(session.confirmSigningExpiry(reservation, expectedAddress)).rejects.toThrow();
    session.close();
  }
  const {session} = harness({approvalClock: 500n});
  await session.connect('board');
  await expect(session.confirmSigningExpiry(reservation, expectedAddress)).resolves.toBeUndefined();
  await expect(session.confirmSigningExpiry({...reservation, generation: 2}, expectedAddress)).rejects.toThrow('WALLET_SIGNING_BINDING');
  await expect(session.confirmSigningExpiry({...reservation,kind:'evm'},expectedAddress)).rejects.toThrow('EVM_SIGNING_FINALITY_REQUIRED');
  session.close();
});

test('owner policy requests use the authenticated management session and exact previously read policy',async()=>{
  const stale=harness({alreadyActive:true,policy:'fresh'});await stale.session.connect('synthetic');
  await expect(stale.session.ownerLimits(2n)).rejects.toThrow('OWNER_LIMITS_OWNER');stale.session.close();
  const fresh=harness({alreadyActive:true,policy:'fresh'});await fresh.session.connect('synthetic');
  let asked=0;
  await fresh.session.initializeOwnerLimits(1n,'8283','0x'+'04'.repeat(20),'0x'+'05'.repeat(20),'10000000','30000000',()=>{asked++;});
  expect(asked).toBe(1);
  const changed=harness({alreadyActive:true,policy:'existing'});await changed.session.connect('synthetic');
  const policy=await changed.session.ownerLimits();
  expect(policy && typeof policy!=='string' && policy.perTransfer).toBe(10000000n);
  if(!policy || typeof policy==='string')throw new Error('fixture');
  await expect(changed.session.requestOwnerLimitChange({...policy,revision:2n},'12000000','30000000',policy.destination,()=>{asked++;})).rejects.toThrow('OWNER_LIMITS_UNAVAILABLE');
  expect(asked).toBe(1);
  const bindings: unknown[]=[];
  const due=await changed.session.requestOwnerLimitChange(policy,'12000000','30000000',policy.destination,binding=>{asked++;bindings.push(binding);});
  expect(bindings).toEqual([{type:'nu54-owner-display-v1',...changed.currentDisplay()}]);
  expect(due).toBeGreaterThan(BigInt(Math.floor(Date.now()/1000))+86400n);
  expect(asked).toBe(2);
});


test('auto-lock disclosure requires an active current owner and a device capability', async()=>{
  let reads=0;
  const supported=harness({alreadyActive:true,tools:op=>{
    expect(op).toBe(0x73);
    reads++;
    return Uint8Array.of(op,1,3,0,120);
  }});
  await supported.session.connect('synthetic');
  await expect(supported.session.ownerIdleLock(2n)).rejects.toThrow('OWNER_LIMITS_OWNER');
  expect(reads).toBe(0);
  await expect(supported.session.ownerIdleLock(1n)).resolves.toEqual({idleSeconds:120,disconnect:true});
  expect(reads).toBe(1);
  supported.session.close();

  const older=harness({alreadyActive:true,tools:op=>Uint8Array.of(op,0)});
  await older.session.connect('synthetic');
  await expect(older.session.ownerIdleLock(1n)).rejects.toThrow('OWNER_IDLE_LOCK_UNAVAILABLE');
  older.session.close();
});

test('C protected storage and authenticated W27 wire preserve all eight recipients during cap edits',async()=>{
  const dir=mkdtempSync(join(tmpdir(),'nu54-owner-set-bridge-'));
  try {
    execFileSync('cmake',['-S',resolve(__dirname,'../../p01-device-firmware/test'),'-B',join(dir,'build')],{stdio:'pipe'});
    execFileSync('cmake',['--build',join(dir,'build'),'--target','owner_limits_host_bridge'],{stdio:'pipe'});
    const bridge={binary:join(dir,'build','owner_limits_host_bridge'),state:join(dir,'state')};
    const actor=async(deviceClock?:bigint,options:{bridgeDropChange?:boolean;bridgeDropActivation?:boolean}={})=>{
      const {session}=harness({alreadyActive:true,bridge:{...bridge,deviceClock},...options});
      await session.connect('synthetic');
      return session;
    };
    let session=await actor();
    await session.initializeOwnerLimits(1n,'8283','0x'+'04'.repeat(20),'0x'+'05'.repeat(20),'10','30',()=>{});
    session.close();
    session=await actor(undefined,{bridgeDropChange:true});
    const initial=await session.ownerLimitsSet(1n);
    if(!initial)throw new Error('expected active policy');
    const recipients=Array.from({length:8},(_,i)=>'0x'+(i+1).toString(16).padStart(2,'0').repeat(20));
    const display=jest.fn();
    await expect(session.requestOwnerSetChange(initial,{recipients},display)).rejects.toThrow();
    expect(display).toHaveBeenCalledWith(expect.objectContaining({operation:33}));
    session.close();
    session=await actor();
    const pending=await session.ownerLimitsSet(1n);
    if(!pending?.pending)throw new Error('expected durable candidate');
    expect(pending.pending.recipients).toEqual(recipients);
    session.close();
    session=await actor(pending.pending.notBefore-1n);
    await expect(session.activateOwnerSet(pending)).rejects.toThrow('OWNER_LIMITS_ACTIVATE');
    session.close();
    session=await actor(pending.pending.notBefore,{bridgeDropActivation:true});
    await expect(session.activateOwnerSet(pending)).rejects.toThrow();
    session.close();
    session=await actor();
    const active=await session.ownerLimitsSet(1n);
    if(!active)throw new Error('expected activated set');
    expect(ownerLimitChangeApplied(pending,active)).toBe(true);
    await expect(session.activateOwnerSet(pending)).rejects.toThrow('OWNER_LIMITS_UNAVAILABLE');
    const now=jest.spyOn(Date,'now').mockReturnValue(Number(pending.pending.notBefore)*1000);
    try { await session.requestOwnerSetChange(active,{per:8n,daily:20n},display); }
    finally { now.mockRestore(); }
    expect(display).toHaveBeenLastCalledWith(expect.objectContaining({operation:34}));
    session.close();
    session=await actor();
    const pendingCaps=await session.ownerLimitsSet(1n);
    if(!pendingCaps?.pending)throw new Error('expected pending caps');
    expect(pendingCaps.pending).toMatchObject({recipients,perTransfer:8n,daily:20n});
    session.close();
    session=await actor(pendingCaps.pending.notBefore);
    await expect(session.activateOwnerSet(pendingCaps)).resolves.toMatchObject({revision:3n,recipients,perTransfer:8n,daily:20n});
    session.close();
  } finally {rmSync(dir,{recursive:true,force:true});}
},60000);

test('actual OwnerSession exchanges policy messages with the C PS adapter on the host',async()=>{
  // BLE/TLS, counter, time, independent display and button are synthetic; this
  // exercises the wire and C persistence, not physical-device qualification.
  const dir=mkdtempSync(join(tmpdir(),'nu54-owner-bridge-'));
  try {
    const source=resolve(__dirname,'../../p01-device-firmware/test');
    execFileSync('cmake',['-S',source,'-B',join(dir,'build')],{stdio:'pipe'});
    execFileSync('cmake',['--build',join(dir,'build'),'--target','owner_limits_host_bridge'],{stdio:'pipe'});
    const bridge={binary:join(dir,'build','owner_limits_host_bridge'),state:join(dir,'state')};
    const first=harness({alreadyActive:true,bridge});await first.session.connect('synthetic');
    expect(await first.session.ownerLimits(1n)).toBe('fresh');
    await first.session.initializeOwnerLimits(1n,'8283','0x'+'04'.repeat(20),'0x'+'05'.repeat(20),'10000000','30000000',()=>{});
    first.session.close();
    const second=harness({alreadyActive:true,bridge,bridgeDropChange:true});await second.session.connect('synthetic');
    const policy=await second.session.ownerLimits(1n);
    if(!policy || typeof policy==='string')throw new Error('fixture');
    expect(policy.perTransfer).toBe(10000000n);expect(policy.daily).toBe(30000000n);
    await expect(second.session.requestOwnerLimitChange(policy,'12000000','30000000',policy.destination,()=>{})).rejects.toThrow();
    second.session.close();
    const expiry=(BigInt(Math.floor(Date.now()/1000))+110n).toString();
    const request='aa'.repeat(16);
    const sendEvm=(amount:bigint)=>execFileSync(bridge.binary,[bridge.state,'63'+bytesToHex(bigToBytes(amount,8),false),expiry,request],{encoding:'utf8'}).trim();
    const noTrustedTime=harness({alreadyActive:true,bridge:{...bridge,deviceClock:0n}});
    await noTrustedTime.session.connect('synthetic');
    const candidate=await noTrustedTime.session.ownerLimits(1n);
    if(!candidate || typeof candidate==='string' || !candidate.pending)throw new Error('fixture');
    expect(candidate).toMatchObject({revision:1n,perTransfer:10000000n,daily:30000000n,pending:{perTransfer:12000000n,daily:30000000n}});
    await expect(noTrustedTime.session.activateOwnerLimitChange(candidate)).rejects.toThrow('OWNER_LIMITS_ACTIVATE');
    noTrustedTime.session.close();
    const early=harness({alreadyActive:true,bridge:{...bridge,deviceClock:candidate.pending.notBefore-1n}});
    await early.session.connect('synthetic');
    await expect(early.session.activateOwnerLimitChange(candidate)).rejects.toThrow('OWNER_LIMITS_ACTIVATE');
    expect(await early.session.ownerLimits(1n)).toEqual(candidate);
    early.session.close();

    // Clone the protected-storage fixture so the successful activation and the
    // signing gate below both start from the same durable pending revision.
    const dueBridge={...bridge,state:bridge.state+'-activation',deviceClock:candidate.pending.notBefore};
    copyFileSync(bridge.state,dueBridge.state);
    const lost={...dueBridge,state:bridge.state+'-lost-activation'};
    copyFileSync(bridge.state,lost.state);
    const uncertain=harness({alreadyActive:true,bridge:lost,bridgeDropActivation:true});
    await uncertain.session.connect('synthetic');
    await expect(uncertain.session.activateOwnerLimitChange(candidate)).rejects.toThrow();
    uncertain.session.close();
    const confirmed=harness({alreadyActive:true,bridge:lost});await confirmed.session.connect('synthetic');
    const active=await confirmed.session.ownerLimits(1n);
    if(!active || typeof active==='string')throw new Error('fixture');
    expect(ownerLimitChangeApplied(candidate,active)).toBe(true);
    expect(ownerLimitChangeApplied({...candidate, pending:{...candidate.pending,perTransfer:13000000n}},active)).toBe(false);
    confirmed.session.close();

    const due=harness({alreadyActive:true,bridge:dueBridge});
    await due.session.connect('synthetic');
    const stale={...candidate,pending:{...candidate.pending,perTransfer:13000000n}};
    await expect(due.session.activateOwnerLimitChange(stale)).rejects.toThrow('OWNER_LIMITS_UNAVAILABLE');
    expect(await due.session.ownerLimits(1n)).toEqual(candidate);
    const applied=await due.session.activateOwnerLimitChange(candidate);
    expect(applied).toMatchObject({revision:2n,perTransfer:12000000n,daily:30000000n,pending:undefined});
    due.session.close();
    const afterRestart=harness({alreadyActive:true,bridge:dueBridge});
    await afterRestart.session.connect('synthetic');
    expect(await afterRestart.session.ownerLimits(1n)).toEqual(applied);
    afterRestart.session.close();
    // Pending policy leaves the old 10-unit cap active. The dummy address
    // makes signing fail after reservation; a retry cannot release or re-sign it.
    expect(sendEvm(11000000n)).toBe('6300');
    expect(sendEvm(9000000n)).toBe('6301');
    expect(sendEvm(9000000n)).toBe('6300');
    const third=harness({alreadyActive:true,bridge});await third.session.connect('synthetic');
    const pending=await third.session.ownerLimits(1n);
    if(!pending || typeof pending==='string')throw new Error('fixture');
    expect(pending.perTransfer).toBe(10000000n);expect(pending.pending?.perTransfer).toBe(12000000n);
    expect(pending.spent).toBe(9000000n);
    third.session.close();
    const reserved=harness({alreadyActive:true,bridge:{...bridge,deviceClock:candidate.pending.notBefore}});
    await reserved.session.connect('synthetic');
    await expect(reserved.session.activateOwnerLimitChange(pending)).rejects.toThrow('OWNER_LIMITS_ACTIVATE');
    expect(await reserved.session.ownerLimits(1n)).toEqual(pending);
    reserved.session.close();
  } finally {rmSync(dir,{recursive:true,force:true});}
},30000);


test('owner finality forwarding requires the active bound owner and 8283 evidence',async()=>{
 const body=new Uint8Array(237);body.set(hexToBytes('4e353446'));body.set(hexToBytes(device),4);body.set(bigToBytes(1n,8),188);body.set(bigToBytes(8283n,8),196);body[236]=1;
 const h=harness({alreadyActive:true});await h.session.connect('synthetic');
 await expect(h.session.reconcileOwnerFinality(2n,body,new Uint8Array(64),Uint8Array.of(2))).rejects.toThrow('OWNER_LIMITS_OWNER');
 await expect(h.session.reconcileOwnerFinality(1n,body,new Uint8Array(64),Uint8Array.of(2))).resolves.toBe('applied');h.session.close();
});

test('finality enrollment and resume require authenticated owner and signer',async()=>{
 const grant=new Uint8Array(233);grant.set(hexToBytes('4e353444'));grant.set(hexToBytes(device),4);grant.set(bigToBytes(1n,8),36);grant.set(bigToBytes(8283n,8),44);grant.set(address,52);
 const h=harness({alreadyActive:true});await h.session.connect('synthetic');let asked=0;
 await expect(h.session.enrollOwnerFinality(1n,grant,new Uint8Array(64),()=>{asked++;})).resolves.toBe('applied');expect(asked).toBe(1);h.session.close();
 const resumed=harness({alreadyActive:true});await resumed.session.connect('synthetic');await expect(resumed.session.resumeOwnerFinality(1n)).resolves.toBe('already');resumed.session.close();
});

test('replacement forwarding requires authenticated owner and exact 8283 scope',async()=>{
 const body=new Uint8Array(269);body.set(hexToBytes('4e353452'));body.set(hexToBytes(device),4);body.set(bigToBytes(1n,8),188);body.set(bigToBytes(8283n,8),196);body[236]=3;
 const h=harness({alreadyActive:true});await h.session.connect('synthetic');
 await expect(h.session.reconcileOwnerReplacement(1n,body,new Uint8Array(64),Uint8Array.of(2),Uint8Array.of(3))).resolves.toBe('applied');h.session.close();
});


test('lost signed EVM reply retains the exact precommitted frame identity across restart',async()=>{
 const f=evmFixtures[0],now=BigInt(Math.floor(Date.now()/1000)),issuerSecret=new Uint8Array(32).fill(7);
 const anchor=p256.getPublicKey(issuerSecret,false),unsigned=hexToBytes(f.unsigned),expiry=now+60n;
 const header=concat(Uint8Array.of(9),bigToBytes(expiry,8),bigToBytes(8283n,8),bigToBytes(300000n,8),new Uint8Array(8+20),hexToBytes(f.genesis.slice(2)),bigToBytes(1n,8),bigToBytes(2n,8));
 const signed=concat(utf8Encode('N54V'),hexToBytes(device),bigToBytes(1n,8),hexToBytes(f.sender.slice(2)),header,keccak_256(unsigned));
 const wire=bytesToHex(concat(header,p256.sign(sha256(signed),issuerSecret).toCompactRawBytes(),unsigned),false);
 let saved:WalletStorage={version:2,exercised:true,mode:'personal',verified:{boardFingerprint:device,address:f.sender,network:'8283',generation:1}};
 const h=harness({alreadyActive:true,evm:{signer:f.sender,anchor,signed:hexToBytes(f.raw.slice(2)),onRequest:id=>{
   expect(saved.signing?.evm?.request).toBe(bytesToHex(id,false));expect(saved.signing?.evm?.unsigned).toBe('0x'+f.unsigned);
 }}});
 await h.session.connect('synthetic');const request=await h.session.newSigningRequestId();
 const context={request,unsigned:'0x'+f.unsigned,chain:'8283' as const,genesis:f.genesis,signer:f.sender};
 const before=saved;const commit=async(next:WalletStorage)=>{saved=JSON.parse(JSON.stringify(next));return saved;};
 const approve=jest.fn(async()=>{await h.session.signDirectTransfer(wire,f.sender,1n,()=>{},request);return {};});
 await expect(persistWalletSignature(before,'evm',wire,Number(expiry),commit,approve,context)).rejects.toThrow('OWNER_DISCONNECTED');
 const restored=JSON.parse(JSON.stringify(saved)) as WalletStorage;expect(restored.signing?.evm).toEqual(context);
 await expect(persistWalletSignature(restored,'evm',wire,Number(expiry),commit,approve,context)).rejects.toThrow('WALLET_SIGNING_UNRESOLVED');
 expect(approve).toHaveBeenCalledTimes(1);h.session.close();
});


test('W04 actual P01 owner core commits once and survives a lost response and reconnect',async()=>{
 const dir=mkdtempSync(join(tmpdir(),'nu54-owner-core-'));
 try{
  const source=resolve(__dirname,'../../p01-device-firmware/test');
  execFileSync('cmake',['-S',source,'-B',join(dir,'build')],{stdio:'pipe'});
  execFileSync('cmake',['--build',join(dir,'build'),'--target','owner_core_host_bridge'],{stdio:'pipe'});
  const binary=join(dir,'build','owner_core_host_bridge');
  for(const dropProof of [false,true]){
   const state=join(dir,dropProof?'lost':'normal'),first=harness({ownerBridge:{binary,state,dropProof}});
   await first.session.connect('synthetic');
   if(dropProof)await expect(first.session.register(()=>{})).rejects.toThrow('OWNER_DISCONNECTED');
   else await expect(first.session.register(()=>{})).resolves.toMatchObject({state:'active',generation:1n});
   first.session.close();const resumed=harness({ownerBridge:{binary,state}});await resumed.session.connect('synthetic');
   await expect(resumed.session.status()).resolves.toMatchObject({state:'active',generation:1n,owner:bytesToHex(owner,false)});
   await expect(resumed.session.register(()=>{throw new Error('must not register again');})).resolves.toMatchObject({state:'active',generation:1n});
   resumed.session.close();
  }
 }finally{rmSync(dir,{recursive:true,force:true});}
},30000);


test('W06 actual P01 owner core preserves address and generation on lost phone response',async()=>{
 const dir=mkdtempSync(join(tmpdir(),'nu54-owner-lost-core-'));
 try{
  const source=resolve(__dirname,'../../p01-device-firmware/test');execFileSync('cmake',['-S',source,'-B',join(dir,'build')],{stdio:'pipe'});
  execFileSync('cmake',['--build',join(dir,'build'),'--target','owner_core_host_bridge'],{stdio:'pipe'});
  const binary=join(dir,'build','owner_core_host_bridge'),state=join(dir,'lost');
  const before=harness({ownerBridge:{binary,state,mode:'lost',dropProof:true}});await before.session.connect('synthetic');
  const previous=await before.session.beginLostPhone();expect(previous.address).toBe(bytesToHex(address,true));
  await expect(before.session.proveLostPhone(()=>{})).rejects.toThrow('OWNER_DISCONNECTED');before.session.close();
  const resumed=harness({ownerBridge:{binary,state,mode:'lost'}});await resumed.session.connect('synthetic');
  await expect(resumed.session.verifyLostPhone({...previous,generation:previous.generation.toString()})).resolves.toMatchObject({state:'active',generation:2n});
  resumed.session.close();
  const blockedState=join(dir,'missing-independent-proof'),blocked=harness({ownerBridge:{binary,state:blockedState,mode:'lost-no-proof'}});
  await blocked.session.connect('synthetic');const original=await blocked.session.beginLostPhone();
  await expect(blocked.session.proveLostPhone(()=>{})).rejects.toThrow();blocked.session.close();
  const check=harness({ownerBridge:{binary,state:blockedState,mode:'lost-no-proof'}});await check.session.connect('synthetic');
  await expect(check.session.verifyLostPhone({...original,generation:original.generation.toString()})).rejects.toThrow('OWNER_REPLACEMENT_NOT_COMMITTED');
  check.session.close();
 }finally{rmSync(dir,{recursive:true,force:true});}
},30000);


test('W12 uses authenticated OwnerSession status before and after the exact firmware campaign',async()=>{
  const command='10'+'ab'.repeat(48);
  const measured=jest.fn();
  const {session}=harness({alreadyActive:true,maintenance:{onProof:measured}});
  await session.connect('synthetic');
  const status=await session.maintenanceStatus();
  await expect(session.firmwareProof(command,{...status,state:'pending'})).rejects.toThrow('OWNER_MISMATCH');
  expect(measured).not.toHaveBeenCalled();
  expect(await session.firmwareProof(command,status)).toMatch(/^4e353446ab/);
  expect(measured).toHaveBeenCalledTimes(1);
  session.close();

  const busy=harness({alreadyActive:true,maintenance:{busy:true,onProof:measured}}).session;
  await busy.connect('synthetic');
  await expect(busy.maintenanceStatus()).rejects.toThrow();
  expect(measured).toHaveBeenCalledTimes(1);
  busy.close();

  const moved=harness({alreadyActive:true,maintenance:{moved:true,onProof:measured}}).session;
  await moved.connect('synthetic');
  const before=await moved.maintenanceStatus();
  await expect(moved.firmwareProof(command,before)).rejects.toThrow('OWNER_MISMATCH');
  moved.close();
});

test.each(['current','unsupported','wrong-board','moved'] as const)('W12 reads only current owner firmware scope (%s)',async mode=>{
  const onRead=jest.fn();
  const {session}=harness({alreadyActive:true,running:{mode,onRead}});
  await session.connect('synthetic');
  try {
    const original=await session.maintenanceStatus();
    await expect(session.runningFirmware({...original,generation:2n})).rejects.toThrow('OWNER_MISMATCH');
    expect(onRead).not.toHaveBeenCalled();
    const pending=session.runningFirmware(original);
    if (mode==='current') await expect(pending).resolves.toMatchObject({version:'2.3.258+65538',counter:512,confidence:'confirmed-running'});
    else if (mode==='unsupported') await expect(pending).resolves.toBeUndefined();
    else await expect(pending).rejects.toThrow(mode==='wrong-board'?'FIRMWARE_STATUS_SCOPE':'OWNER_MISMATCH');
    expect(onRead).toHaveBeenCalledTimes(1);
  }finally{session.close();}
});

test.each(['good','unknown','moved'] as const)('W12 authenticates a separate PMIC read (%s)',async mode=>{
  const onRead=jest.fn();
  const {session}=harness({alreadyActive:true,power:{mode,onRead}});
  await session.connect('synthetic');
  try {
    const original=await session.maintenanceStatus();
    expect(onRead).not.toHaveBeenCalled();
    await expect(session.firmwareInputPower({...original,generation:2n})).rejects.toThrow('OWNER_MISMATCH');
    expect(onRead).not.toHaveBeenCalled();
    const pending=session.firmwareInputPower(original);
    if (mode==='moved') await expect(pending).rejects.toThrow('OWNER_MISMATCH');
    else await expect(pending).resolves.toBe(mode==='good'?'vin-good':'unknown');
    expect(onRead).toHaveBeenCalledTimes(1);
  }finally{session.close();}
});

test('W06 refuses to open on a generation different from the authenticated selection',async()=>{
  const {session}=harness({alreadyActive:true});
  await session.connect('synthetic');
  const selected=await session.maintenanceStatus();
  await expect(session.beginRecovery({...selected,generation:selected.generation+1n})).rejects.toThrow('OWNER_MISMATCH');
  session.close();
});


test('W25 approved tpub must match the current owner generation, exact network and both independent board branches',async()=>{
  const profile={chain:1 as const,network:2 as const,account:3};
  const manifest=concat(bitcoinXpubCommand(profile),hexToBytes('1122334455667788'),new Uint8Array(32).fill(15),
    hexToBytes('0330d54fd0dd420a6e5f8d3624f5f3482cae350f79d5f0753bf5beef9c2d91af3c'));
  const approved=bitcoinWatchDescriptors(manifest,profile);
  const roots:[Uint8Array,Uint8Array]=[approved.publicChild(0,0),approved.publicChild(1,0)];
  const otherAccount=manifest.slice();otherAccount[6]=4;
  const relabeledRoot=manifest.slice();relabeledRoot[15]=9;
  const otherNetwork=manifest.slice();otherNetwork[2]=1;
  for (const [proof,epoch,expected] of [[manifest,1n,undefined],[otherAccount,1n,'BTC_XPUB_REPLY'],
      [relabeledRoot,1n,'BTC_XPUB_SCOPE'],[otherNetwork,1n,'BTC_XPUB_REPLY'],[manifest,2n,'EVM_ACCOUNT_BINDING']] as const) {
    const exportCall=jest.fn();
    const {session}=harness({backupReady:true,watch:{proof,roots,onExport:exportCall}});
    try {
      await session.connect('synthetic');
      const result=session.exportBitcoinWatchAccount(profile,'0x'+bytesToHex(address,false),epoch,()=>{});
      if (expected) await expect(result).rejects.toThrow(expected);
      else await expect(result).resolves.toMatchObject({firstReceive:approved.firstReceive,firstChange:approved.firstChange});
      expect(exportCall).toHaveBeenCalledTimes(epoch===2n?0:1);
    } finally {session.close();}
  }
});

test('W12 uses authenticated owner scope and rejects malformed LED acknowledgments',async()=>{
  const tools=jest.fn((opcode:number)=>Uint8Array.of(opcode,1));
  const {session}=harness({alreadyActive:true,tools});
  await session.connect('synthetic');
  const current=await session.maintenanceStatus();
  await expect(session.deviceTool({...current,generation:2n},'find')).rejects.toThrow('OWNER_MISMATCH');
  expect(tools).not.toHaveBeenCalled();
  await expect(session.deviceTool(current,'read')).resolves.toBe(true);
  await expect(session.deviceTool(current,'find')).resolves.toBe(true);
  await expect(session.deviceTool(current,'cancel')).resolves.toBe(true);
  expect(tools.mock.calls).toEqual([[0x70],[0x71],[0x72]]);
  tools.mockReturnValueOnce(Uint8Array.of(0x70,1));
  await expect(session.deviceTool(current,'find')).rejects.toThrow('DEVICE_TOOL_REPLY');
  session.close();
});
