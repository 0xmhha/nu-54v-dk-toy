import {p256} from '@noble/curves/p256';
import {sha256} from '@noble/hashes/sha256';
import {
  bigToBytes, bytesToBig, bytesToHex, concat, equalBytes, encodeV2StreamFrame, encodeApprovalProgressOffer, encodePaymentReview, readPaymentReview, fromBase64, hexToBytes, toBase64, V2Journal, V2Link, V2StreamDecoder,
  verifyCommerceApproval, type VerifiedCommercePayment, type V2Request,
} from '@nu54/protocol';
import type {Spec as NativeTLS} from '../specs/NativeOwnerWallet.ts';
import type {Spec as NativeBLE} from '../specs/NativeRenterBle.ts';
import {verifyBip48Partial, type Bip48Policy} from './bip48Wallet.ts';
import {readDirectTransfer,verifySignedTransfer,verifyTransferAuthority} from './evmTransfer.ts';
import {bip48Participant, bitcoinWatchDescriptors, bitcoinXpubCommand, chainPublicCommand, readChainPublic, type ChainAccount} from './chainAccounts.ts';
import {verifyChainSigned, type ChainTransfer} from './chainTransfer.ts';
import type {PhoneReplacementCheckpoint, SigningReservation} from './profile.ts';
import {validateRecovery, type RecoveryContext, type RecoveryTrust} from './recoveryProof.ts';
import {readOwnerIdleLock, ownerIdleLockOperation, readOwnerLimits, ownerLimitChange, ownerLimitChangeApplied, ownerLimitInitialize, ownerFinalityReconcile, ownerFinalityReplacement, ownerFinalityEnrollment, ownerSetActivate, ownerSetCaps, ownerSetChange, ownerSetOps, readOwnerLimitSet, type OwnerLimits} from './ownerLimitsWire.ts';
import {OWNER_GATT, ownerBegin, ownerProof, readOwnerAddress, readStatus, validateChallenge, type Enrollment, type OwnerStatus} from './ownerWire.ts';

import {checkLoginSignature,readIssuedLogin} from './dapp/loginWire.ts';
import type {DappScope} from './dapp/gate.ts';
import {readRunningFirmware,runningFirmwareRequest,type RunningFirmware} from './firmwareStatusWire.ts';
import {firmwarePowerRequest,readFirmwareInputPower,type FirmwareInputPower} from './firmwarePowerWire.ts';

type TLSNotice = {out: string; clear: string; epoch: string; peer: string; closed?: boolean};
export type OwnerDisplayBinding = {type:'nu54-owner-display-v1'; operation:number; session:string; request:string; expires:string};
export type OwnerApprovalProgress = {
  device: string; owner: string; generation: bigint; request: string; expires: bigint;
};
type OwnershipProof = {publicKey: Uint8Array; fingerprint: string};
export type LostPhoneContext = {challenge:string; device:string; replacement:string; generation:bigint; address:string};
const time = () => BigInt(Math.floor(Date.now() / 1000));

/** One owner session on the native pinned TLS adapter. The BLE link grants no trust. */
export class OwnerSession {
  private link?: V2Link;
  private epoch?: Uint8Array;
  private recovery?: RecoveryContext;
  private backup?: RecoveryContext;
  private replacement?: {challenge:Uint8Array; own:OwnershipProof; expires:bigint; previous:LostPhoneContext};
  private writes = Promise.resolve();
  private listeners: {remove(): void}[] = [];
  private decode = new V2StreamDecoder();
  private ended = false;
  private mtu = 20;
  private ready?: {resolve: () => void; reject: (e: Error) => void};
  private pending?: {
    resolve: (response: V2Request) => void; reject: (error: Error) => void;
    progress?: (active: boolean) => void;
  };
  private approvalProgressGeneration?: bigint;

  constructor(private readonly ble: NativeBLE, private readonly native: NativeTLS, private readonly profile: Enrollment) {}

  async connect(handle: string): Promise<void> {
    try {
      const mtu = await this.ble.connectOwner(handle, OWNER_GATT.service, OWNER_GATT.input, OWNER_GATT.output);
      if (!Number.isInteger(mtu) || mtu < 23 || mtu > 517) throw new Error('INVALID_MTU');
      this.mtu = Math.min(mtu - 3, 244);
      this.listeners = [
        this.ble.onFragment(chunk => this.schedule(async () => this.accept(await this.native.feedTls(chunk)))),
        this.native.onTls(notice => this.schedule(() => this.accept(notice))),
        this.ble.onDisconnect(() => this.schedule(() => this.fail())),
      ];
      const ready = new Promise<void>((resolve,reject) => {this.ready={resolve,reject};});
      const timer = setTimeout(() => this.fail(), 10000);
      this.schedule(async () => this.accept(await this.native.startTls(this.profile.ca, this.profile.certificateChain, this.profile.boardSPKI)));
      try {
        await ready;
      } finally {
        clearTimeout(timer);
      }
    } catch (error) {
      this.fail();
      throw error;
    }
  }

  private schedule(step: () => Promise<void> | void) {
    this.writes = this.writes.then(async () => { if (!this.ended) await step(); }).catch(() => this.fail());
  }

  private async accept(serialized: string) {
    const data = JSON.parse(serialized) as TLSNotice;
    if (data.out) {
      const encrypted = fromBase64(data.out);
      for (let index=0; index<encrypted.length; index+=this.mtu) {
        await this.ble.writeFragment(toBase64(encrypted.subarray(index,index+this.mtu)));
      }
    }
    if (data.epoch && !this.link) {
      const epoch = fromBase64(data.epoch);
      if (data.peer !== this.profile.boardSPKI || epoch.length !== 16 || !epoch.some(Boolean)) throw new Error('OWNER_TLS_PIN');
      this.epoch = epoch.slice();
      this.link = new V2Link('management','owner',epoch,hexToBytes(data.peer),new V2Journal(16));
      const notify = this.ready;
      this.ready = undefined;
      notify?.resolve();
    }
    if (data.clear) {
      const link = this.link;
      if (!link) throw new Error('OWNER_TLS_ORDER');
      this.decode.feed(fromBase64(data.clear), frame => {
        const reply = link.receive(frame,time());
        if (reply && reply.request.command === 'approval.progress' && this.pending?.progress) {
          this.pending.progress(true);
        } else if (reply && reply.request.command === 'reply' && this.pending) {
          const pending = this.pending;
          this.pending = undefined;
          pending.progress?.(false);
          pending.resolve(reply.request);
        } else if (reply) {
          throw new Error('OWNER_REPLY');
        }
      });
    }
    if (data.closed) this.fail();
  }

  private fail() {
    if (this.ended) return;
    this.ended = true;
    this.link?.close();
    this.decode.close();
    this.epoch?.fill(0);
    this.epoch = undefined;
    this.recovery = undefined;
    this.backup = undefined;
    this.replacement = undefined;
    this.approvalProgressGeneration = undefined;
    const error = new Error('OWNER_DISCONNECTED');
    this.ready?.reject(error);
    this.pending?.progress?.(false);
    this.pending?.reject(error);
    this.ready = undefined;
    this.pending = undefined;
    for (const listener of this.listeners) listener.remove();
    this.listeners = [];
    this.native.stopTls().catch(() => {});
    this.ble.disconnect().catch(() => {});
  }

  close() {this.fail();}

  private async invoke(payload: Uint8Array, ttl = 10, replyTimeout = ttl,
    command:V2Request['command']='manage', fixed?:{id:Uint8Array;expires:bigint},
    display?:(binding:OwnerDisplayBinding)=>void, progress?:(active:boolean)=>void) {
    if (!this.link || this.ended || this.pending) throw new Error('OWNER_SESSION_STATE');
    const id = fixed?.id ?? fromBase64(await this.native.randomBytes(16));
    const expires = fixed?.expires ?? time()+BigInt(ttl);
    if (this.ended || this.pending) throw new Error('OWNER_SESSION_STATE');
    const frames = this.link.send({command, id, expires, payload});
    const result = new Promise<V2Request>((resolve,reject)=>{this.pending={resolve,reject,progress};});
    this.schedule(async () => {
      if (display && this.pending && this.epoch && !this.ended) {
        display({type:'nu54-owner-display-v1',operation:payload[0],session:bytesToHex(this.epoch,false),
          request:bytesToHex(id,false),expires:expires.toString()});
      }
      for (const frame of frames) await this.accept(await this.native.sendTls(toBase64(encodeV2StreamFrame(frame))));
    });
    const timer = setTimeout(() => this.fail(),replyTimeout*1000);
    try {
      return {reply:await result, id, expires};
    } finally {
      clearTimeout(timer);
    }
  }

  /** Opt in before any original approval has been sent. Legacy peers may close this idle session. */
  async negotiateApprovalProgress(): Promise<void> {
    const status = await this.maintenanceStatus();
    const link = this.link;
    if (!link || this.ended || status.state !== 'active') throw new Error('OWNER_MISMATCH');
    const {reply} = await this.invoke(encodeApprovalProgressOffer(status.generation), 8, 6);
    if (this.ended || this.link !== link) throw new Error('OWNER_DISCONNECTED');
    link.acceptApprovalProgressOffer(reply.payload, status.generation);
    this.approvalProgressGeneration = status.generation;
  }

  async approvePayment(payment:VerifiedCommercePayment,request:Uint8Array,onBoard:()=>void,
    observeProgress?:(progress:OwnerApprovalProgress|undefined)=>void) {
    const current=await this.status();
    const own=await this.identity();
    const remaining=payment.deadline-Number(time());
    if(current.state!=='active'||current.owner!==own.fingerprint||current.generation!==payment.generation||
        remaining<=0||remaining>120) throw new Error('PAYMENT_BOARD_BINDING');
    const original=request.slice();
    const quote=payment.bytes.slice();
    const review=encodePaymentReview(original,quote);
    const evidence=await this.invoke(review,Math.min(remaining,10));
    if(readPaymentReview(review,evidence.reply.payload)==='refused') throw new Error('PAYMENT_REFUSED');
    const live=payment.deadline-Number(time());
    if(live<=0) throw new Error('PAYMENT_BOARD_BINDING');
    onBoard();
    const expires = BigInt(payment.deadline);
    let observed = false;
    const progress = this.approvalProgressGeneration === current.generation ? (active: boolean) => {
      if (active === observed) return;
      observed = active;
      // A view callback cannot interrupt or resolve the board's original approval.
      try {
        observeProgress?.(active ? {device:current.device,owner:current.owner,generation:current.generation,
          request:bytesToHex(original,false),expires} : undefined);
      } catch { /* Ignore a view failure and keep the approval transport intact. */ }
    } : undefined;
    const {reply}=await this.invoke(quote,live,live,'pay.prepare',{id:original,expires},undefined,progress);
    if(reply.payload.length===1&&reply.payload[0]===0) throw new Error('PAYMENT_REFUSED');
    return verifyCommerceApproval(payment,reply.payload,Number(time()));
  }

  async status(): Promise<OwnerStatus> {
    const {reply} = await this.invoke(Uint8Array.of(3),60,10);
    return readStatus(reply.payload, this.profile.device);
  }

  async ownerLimits(expectedGeneration?:bigint) {
    const before=await this.status();
    const own=await this.identity();
    if(before.state!=='active' || before.owner!==own.fingerprint ||
       (expectedGeneration!==undefined && before.generation!==expectedGeneration)) throw new Error('OWNER_LIMITS_OWNER');
    const {reply}=await this.invoke(Uint8Array.of(23),60,10);
    const limits=readOwnerLimits(reply.payload,this.profile.device,before.generation);
    const after=await this.status();
    if(after.state!=='active' || after.owner!==own.fingerprint || after.generation!==before.generation)
      throw new Error('OWNER_LIMITS_OWNER');
    return limits;
  }

  async ownerLimitsSet(generation:bigint) {
    const before=await this.maintenanceStatus();
    if(before.state!=='active' || before.generation!==generation)throw new Error('OWNER_LIMITS_OWNER');
    const {reply}=await this.invoke(Uint8Array.of(ownerSetOps.read,2),60,10);
    const policy=readOwnerLimitSet(reply.payload,this.profile.device,generation);
    const after=await this.maintenanceStatus();
    if(after.state!=='active' || after.generation!==generation || after.owner!==before.owner)
      throw new Error('OWNER_LIMITS_OWNER');
    return policy;
  }

  async requestOwnerSetChange(expected:OwnerLimits,change:{recipients:string[]}|{per:bigint;daily:bigint},
    onBoard:(binding:OwnerDisplayBinding)=>void) {
    const existing=await this.ownerLimitsSet(expected.generation);
    if(!existing || !expected.recipients || existing.pending || existing.board!==expected.board ||
       existing.revision!==expected.revision || existing.chain!==expected.chain || existing.asset!==expected.asset ||
       existing.recipients?.join(',')!==expected.recipients.join(',') || existing.perTransfer!==expected.perTransfer ||
       existing.daily!==expected.daily)throw new Error('OWNER_LIMITS_UNAVAILABLE');
    const wire='recipients' in change ? ownerSetChange(existing.revision,change.recipients) :
      ownerSetCaps(existing.revision,change.per,change.daily);
    const {reply,expires}=await this.invokeOwnerDisplay(wire,onBoard);
    if(reply.payload.length!==11 || reply.payload[0]!==wire[0] || reply.payload[1]!==2 || reply.payload[2]!==1 ||
       bytesToBig(reply.payload.slice(3))!==expires+86400n)throw new Error('OWNER_LIMITS_CONFIRM');
    return bytesToBig(reply.payload.slice(3));
  }

  async activateOwnerSet(expected:OwnerLimits) {
    const existing=await this.ownerLimitsSet(expected.generation),pending=expected.pending;
    if(!existing || !pending?.request || !existing.pending || existing.board!==expected.board ||
       existing.revision!==expected.revision || existing.chain!==expected.chain || existing.asset!==expected.asset ||
       existing.recipients?.join(',')!==expected.recipients?.join(',') || existing.perTransfer!==expected.perTransfer ||
       existing.daily!==expected.daily || existing.pending.request!==pending.request ||
       existing.pending.notBefore!==pending.notBefore || existing.pending.kind!==pending.kind ||
       existing.pending.recipients?.join(',')!==pending.recipients?.join(',') ||
       existing.pending.perTransfer!==pending.perTransfer || existing.pending.daily!==pending.daily)
      throw new Error('OWNER_LIMITS_UNAVAILABLE');
    const {reply}=await this.invoke(ownerSetActivate(expected),60,10);
    if(reply.payload.length!==3 || reply.payload[0]!==ownerSetOps.activate || reply.payload[1]!==2 || reply.payload[2]!==1)
      throw new Error('OWNER_LIMITS_ACTIVATE');
    const applied=await this.ownerLimitsSet(expected.generation);
    if(!applied || !ownerLimitChangeApplied(expected,applied))throw new Error('OWNER_LIMITS_READBACK');
    return applied;
  }

  async ownerIdleLock(generation:bigint) {
    const current=await this.maintenanceStatus();
    if(current.state!=='active' || current.generation!==generation)throw new Error('OWNER_LIMITS_OWNER');
    const {reply}=await this.invoke(Uint8Array.of(ownerIdleLockOperation),8,6);
    return readOwnerIdleLock(reply.payload);
  }

  private invokeOwnerDisplay(payload:Uint8Array, onBoard:(binding:OwnerDisplayBinding)=>void) {
    return this.invoke(payload,110,110,'manage',undefined,onBoard);
  }

  async initializeOwnerLimits(generation:bigint,chain:string,asset:string,destination:string,per:string,daily:string,onBoard:(binding:OwnerDisplayBinding)=>void) {
    if(await this.ownerLimits(generation)!=='fresh')throw new Error('OWNER_LIMITS_UNAVAILABLE');
    const wire=ownerLimitInitialize(chain,asset,destination,per,daily);
    const {reply}=await this.invokeOwnerDisplay(wire,onBoard);
    if(reply.payload.length!==2 || reply.payload[0]!==26 || reply.payload[1]!==1)
      throw new Error('OWNER_LIMITS_CONFIRM');
  }

  async requestOwnerLimitChange(expected:OwnerLimits,per:string,daily:string,destination:string,onBoard:(binding:OwnerDisplayBinding)=>void) {
    const existing=await this.ownerLimits(expected.generation);
    if(!existing || existing==='fresh' || (existing==='finality-fresh' || existing==='finality-pending') || existing.mode==='finality' || existing.pending || existing.board!==expected.board ||
       existing.generation!==expected.generation || existing.revision!==expected.revision || existing.chain!==expected.chain ||
       existing.asset!==expected.asset || existing.destination!==expected.destination || existing.perTransfer!==expected.perTransfer ||
       existing.daily!==expected.daily || existing.spent!==expected.spent)throw new Error('OWNER_LIMITS_UNAVAILABLE');
    const wire=ownerLimitChange(per,daily,destination);
    const {reply,expires}=await this.invokeOwnerDisplay(wire,onBoard);
    if(reply.payload.length!==10 || reply.payload[0]!==24 || reply.payload[1]!==1 ||
       bytesToBig(reply.payload.slice(2))<expires+86400n)throw new Error('OWNER_LIMITS_CONFIRM');
    return bytesToBig(reply.payload.slice(2));
  }

  /** The delegated verifier key still requires the board display and physical approval. */
  async enrollOwnerFinality(generation:bigint,grant:Uint8Array,signature:Uint8Array,onBoard:(binding:OwnerDisplayBinding)=>void) {
    const before=await this.status();const own=await this.identity();
    if(before.state!=='active' || before.owner!==own.fingerprint || before.generation!==generation)throw new Error('OWNER_LIMITS_OWNER');
    const signer=await this.ownerAddress();
    const wire=ownerFinalityEnrollment(grant,signature,this.profile.device,generation,signer.address);
    const {reply}=await this.invokeOwnerDisplay(wire,onBoard);
    if(reply.payload.length!==2 || reply.payload[0]!==28 || ![1,2].includes(reply.payload[1]))throw new Error('OWNER_FINALITY_CONFIRM');
    return reply.payload[1]===2?'already':'applied';
  }

  /** Completes only a previously approved and durably prepared enrollment. */
  async resumeOwnerFinality(generation:bigint) {
    const before=await this.status();const own=await this.identity();
    if(before.state!=='active' || before.owner!==own.fingerprint || before.generation!==generation)throw new Error('OWNER_LIMITS_OWNER');
    const {reply}=await this.invoke(Uint8Array.of(29),60,10);
    if(reply.payload.length!==2 || reply.payload[0]!==29 || ![1,2].includes(reply.payload[1]))throw new Error('OWNER_FINALITY_CONFIRM');
    return reply.payload[1]===2?'already':'applied';
  }

  /** A finalized claim is verified by the board, never trusted from phone RPC. */
  async reconcileOwnerFinality(generation:bigint,body:Uint8Array,signature:Uint8Array,raw:Uint8Array) {
    const before=await this.status();const own=await this.identity();
    if(before.state!=='active' || before.owner!==own.fingerprint || before.generation!==generation)throw new Error('OWNER_LIMITS_OWNER');
    const wire=ownerFinalityReconcile(body,signature,raw,this.profile.device,generation);
    const {reply}=await this.invoke(wire,60,10);
    if(reply.payload.length!==2 || reply.payload[0]!==27 || ![1,2].includes(reply.payload[1]))throw new Error('OWNER_FINALITY_CONFIRM');
    return reply.payload[1]===2?'already':'applied';
  }

  async reconcileOwnerReplacement(generation:bigint,body:Uint8Array,signature:Uint8Array,raw:Uint8Array,replacement:Uint8Array) {
    const before=await this.status();const own=await this.identity();
    if(before.state!=='active' || before.owner!==own.fingerprint || before.generation!==generation)throw new Error('OWNER_LIMITS_OWNER');
    const wire=ownerFinalityReplacement(body,signature,raw,replacement,this.profile.device,generation);
    const {reply}=await this.invoke(wire,60,10);
    if(reply.payload.length!==2 || reply.payload[0]!==31 || ![1,2].includes(reply.payload[1]))throw new Error('OWNER_FINALITY_CONFIRM');
    return reply.payload[1]===2?'already':'applied';
  }

  async activateOwnerLimitChange(expected:OwnerLimits) {
    const pending=expected.pending;
    const existing=await this.ownerLimits(expected.generation);
    if(!pending || !existing || typeof existing==='string' || existing.mode==='finality' ||
       existing.board!==expected.board || existing.revision!==expected.revision ||
       existing.chain!==expected.chain || existing.asset!==expected.asset ||
       existing.destination!==expected.destination || existing.perTransfer!==expected.perTransfer ||
       existing.daily!==expected.daily || !existing.pending || existing.pending.notBefore!==pending.notBefore ||
       existing.pending.destination!==pending.destination || existing.pending.perTransfer!==pending.perTransfer ||
       existing.pending.daily!==pending.daily) throw new Error('OWNER_LIMITS_UNAVAILABLE');
    const {reply}=await this.invoke(Uint8Array.of(25),60,10);
    if(reply.payload.length!==2 || reply.payload[0]!==25 || reply.payload[1]!==1)
      throw new Error('OWNER_LIMITS_ACTIVATE');
    const applied=await this.ownerLimits(expected.generation);
    if(!applied || typeof applied==='string' || !ownerLimitChangeApplied(expected,applied))
      throw new Error('OWNER_LIMITS_READBACK');
    return applied;
  }

  private async ownerAddress() {
    const {reply} = await this.invoke(Uint8Array.of(7),60,10);
    return readOwnerAddress(reply.payload,this.profile.device);
  }

  async beginLostPhone(): Promise<LostPhoneContext> {
    if (this.replacement || this.recovery || this.backup) throw new Error('RECOVERY_SESSION_REQUIRED');
    const own = await this.identity();
    const current = await this.ownerAddress();
    if (current.status.state !== 'active' || current.status.owner === own.fingerprint) throw new Error('OWNER_MISMATCH');
    const {reply,id,expires} = await this.invoke(ownerBegin(5,own.fingerprint),110,10);
    validateChallenge(reply.payload,{operation:5,device:this.profile.device,owner:own.fingerprint,
      generation:current.status.generation,id,expires,now:time()});
    const previous = {challenge:bytesToHex(reply.payload,false),device:this.profile.device,
      replacement:own.fingerprint,generation:current.status.generation,address:current.address};
    this.replacement = {challenge:reply.payload.slice(),own,expires,previous};
    return previous;
  }

  async proveLostPhone(onConfirm:() => void) {
    const replacement = this.replacement;
    if (!replacement || replacement.expires <= time() || this.ended) throw new Error('RECOVERY_SESSION_REQUIRED');
    this.replacement = undefined;
    const {challenge,own,expires,previous} = replacement;
    const signed = fromBase64(await this.native.signChallenge(toBase64(challenge)));
    const remaining = Number(expires-time());
    if (remaining <= 0) throw new Error('OWNER_CHALLENGE_EXPIRED');
    onConfirm();
    const {reply} = await this.invoke(ownerProof(challenge,own.publicKey,signed),remaining);
    const changed = readStatus(reply.payload,this.profile.device);
    if (changed.state !== 'active' || changed.owner !== own.fingerprint || changed.generation !== previous.generation + 1n) {
      throw new Error('OWNER_PHYSICAL_CONFIRMATION');
    }
  }

  async verifyLostPhone(previous:PhoneReplacementCheckpoint): Promise<OwnerStatus> {
    const own = await this.identity();
    const current = await this.ownerAddress();
    const oldGeneration = BigInt(previous.generation);
    if (previous.device !== this.profile.device || previous.replacement !== own.fingerprint ||
        current.status.state !== 'active' || current.address !== previous.address) {
      throw new Error('OWNER_REPLACEMENT_UNVERIFIED');
    }
    if (current.status.generation === oldGeneration && current.status.owner !== own.fingerprint) {
      throw new Error('OWNER_REPLACEMENT_NOT_COMMITTED');
    }
    if (current.status.owner !== own.fingerprint || current.status.generation !== oldGeneration + 1n) {
      throw new Error('OWNER_REPLACEMENT_UNVERIFIED');
    }
    return current.status;
  }

  async beginRecovery(expected?:OwnerStatus) {
    const active = await this.beginOffline(5,expected);
    this.recovery = active;
    return {challenge:bytesToHex(active.challenge,false),requestId:bytesToHex(active.id,false),expires:active.expires.toString()};
  }

  async beginBackup() {
    const active = await this.beginOffline(6);
    this.backup = active;
    return {requestId:bytesToHex(active.id,false),expires:active.expires.toString(),
      checkpoint:{device:active.target,owner:active.owner,generation:active.generation.toString()}};
  }

  async checkBackup() {
    const active = this.backup;
    if (!active || active.expires <= time() || this.ended) throw new Error('BACKUP_SESSION_REQUIRED');
    const checked = await this.status();
    if (this.ended || checked.state !== 'active' || checked.owner !== active.owner ||
        checked.generation !== active.generation || checked.device !== active.target) throw new Error('OWNER_MISMATCH');
    if (checked.root!==2 || checked.profile!==1 || !checked.backedUp) return {verified:false} as const;
    const address = await this.backedUpAddress(checked);
    if (this.ended || active.expires <= time()) throw new Error('BACKUP_SESSION_REQUIRED');
    return {verified:true,status:checked,address} as const;
  }

  async backedUpAddress(expected:OwnerStatus) {
    const {status,address} = await this.ownerAddress();
    if (this.ended || status.state !== 'active' || status.device !== expected.device ||
        status.owner !== expected.owner || status.generation !== expected.generation ||
        status.root !== 2 || status.profile !== 1 || !status.backedUp) {
      throw new Error('OWNER_BACKUP_ADDRESS_BINDING');
    }
    return address;
  }

  private async beginOffline(operation:5 | 6, expected?:OwnerStatus): Promise<RecoveryContext> {
    const own = await this.identity();
    const current = await this.status();
    if(current.state!=='active'||current.owner!==own.fingerprint||expected&&
        (current.device!==expected.device||current.owner!==expected.owner||current.generation!==expected.generation)) {
      throw new Error('OWNER_MISMATCH');
    }
    const epoch = this.epoch;
    if (!epoch || this.ended || this.recovery || this.backup) throw new Error('RECOVERY_SESSION_REQUIRED');
    const {reply,id,expires} = await this.invoke(Uint8Array.of(operation),300,10);
    const packet = reply.payload;
    if (packet.length === 1 && packet[0] === operation) throw new Error(operation===6 ? 'BACKUP_UNAVAILABLE' : 'RECOVERY_UNAVAILABLE');
    if (packet.length !== 145 || packet[0] !== operation) throw new Error('RECOVERY_CONTEXT');
    const expiry = bytesToBig(packet.subarray(137));
    if (this.ended || !packet.subarray(1,33).some(Boolean) ||
        !equalBytes(packet.subarray(33,49),id) ||
        bytesToHex(packet.subarray(49,81),false) !== current.device ||
        bytesToHex(packet.subarray(81,113),false) !== current.owner ||
        !equalBytes(packet.subarray(113,129),epoch) ||
        bytesToBig(packet.subarray(129,137)) !== current.generation || expiry > expires || expiry <= time()) {
      throw new Error('RECOVERY_CONTEXT');
    }
    return {
      challenge:packet.slice(1,33),id,target:current.device,owner:current.owner,
      epoch:epoch.slice(),generation:current.generation,expires:expiry,
    };
  }

  async checkRecovery(trust: RecoveryTrust) {
    const active = this.recovery;
    if (!active || active.expires <= time() || this.ended) throw new Error('RECOVERY_SESSION_REQUIRED');
    const {reply} = await this.invoke(Uint8Array.of(4));
    if (this.ended || reply.payload[0] !== 4) throw new Error('RECOVERY_RECEIPT');
    if (reply.payload.length === 1) return undefined;
    const result = validateRecovery(reply.payload.subarray(1),active,trust,time());
    this.recovery = undefined;
    return result;
  }

  async identity(): Promise<OwnershipProof> {
    const publicKey = fromBase64(await this.native.publicIdentity());
    if (publicKey.length !== 91) throw new Error('OWNER_IDENTITY_SHAPE');
    return {publicKey, fingerprint:bytesToHex(sha256(publicKey),false)};
  }

  async verifiedEvmAddress(expectedAddress:string, generation:bigint) {
    const {status,address} = await this.ownerAddress();
    const own = await this.identity();
    if (status.state !== 'active' || status.owner !== own.fingerprint || status.generation !== generation ||
        status.root !== 2 || status.profile !== 1 || !status.backedUp || address !== expectedAddress)
      throw new Error('EVM_ACCOUNT_BINDING');
    return address;
  }

  async confirmSigningExpiry(reservation: SigningReservation, expectedAddress: string) {
    if (reservation.kind==='evm')throw new Error('EVM_SIGNING_FINALITY_REQUIRED');
    if (reservation.board !== this.profile.device) throw new Error('WALLET_SIGNING_BINDING');
    const fields = concat(hexToBytes(reservation.digest), bigToBytes(BigInt(reservation.expires), 8));
    const own = await this.identity();
    const {reply} = await this.invoke(concat(Uint8Array.of(7), fields), 60, 10);
    const payload = reply.payload;
    if (payload.length !== 145 || !equalBytes(payload.subarray(97, 137), fields) ||
        bytesToBig(payload.subarray(137)) < BigInt(reservation.expires))
      throw new Error('WALLET_SIGNING_UNRESOLVED');
    const {status, address} = readOwnerAddress(payload.subarray(0, 97), reservation.board);
    if (status.state !== 'active' || status.owner !== own.fingerprint || status.generation !== BigInt(reservation.generation) ||
        status.root !== 2 || status.profile !== 1 || !status.backedUp || address !== expectedAddress)
      throw new Error('WALLET_SIGNING_BINDING');
  }

  async evmAuthority() {
    const {reply} = await this.invoke(Uint8Array.of(12),60,10);
    if (reply.payload.length !== 74 || reply.payload[0] !== 12) throw new Error('EVM_TRANSFER_AUTHORITY');
    return {key:reply.payload.slice(1,66),revision:bytesToBig(reply.payload.subarray(66))};
  }

  async newSigningRequestId() {
    const id=fromBase64(await this.native.randomBytes(16));
    if(id.length!==16 || !id.some(Boolean))throw new Error('EVM_REQUEST_ID');
    return bytesToHex(id,false);
  }

  async signDirectTransfer(wire:string, expectedAddress:string, generation:bigint, onBoard:()=>void,requestId:string) {
    const transfer = readDirectTransfer(wire,time());
    const address = await this.verifiedEvmAddress(expectedAddress,generation);
    const trust = await this.evmAuthority();
    verifyTransferAuthority(transfer,trust.key,this.profile.device,generation,address,trust.revision);
    const ttl = Number(transfer.expires-time());
    if (ttl <= 0) throw new Error('EVM_TRANSFER_EXPIRED');
    if(!/^[a-f0-9]{32}$/.test(requestId) || !/[1-9a-f]/.test(requestId))throw new Error('EVM_REQUEST_ID');
    onBoard();
    const {reply,id} = await this.invoke(transfer.body,ttl,ttl,'manage',{id:hexToBytes(requestId),expires:transfer.expires});
    if (reply.payload.length === 1 && reply.payload[0] === 0) throw new Error('EVM_REFUSED');
    if(generation>BigInt(Number.MAX_SAFE_INTEGER))throw new Error('EVM_GENERATION');
    return {...verifySignedTransfer(transfer,reply.payload,address),evidence:{board:this.profile.device,generation:Number(generation),
      request:bytesToHex(id,false),unsigned:bytesToHex(transfer.raw,true)}};
  }

  async marketAddress(expectedAddress:string,generation:bigint) {
    await this.verifiedEvmAddress(expectedAddress,generation);
    const {reply}=await this.invoke(Uint8Array.of(22));
    if(reply.payload.length!==21 || reply.payload[0]!==22 || !reply.payload.slice(1).some(Boolean)) throw new Error('MARKET_BOARD_ADDRESS');
    return bytesToHex(reply.payload.slice(1),true);
  }

  async signMarket(wire:Uint8Array, expectedOwner:string, account:string, generation:bigint, onBoard:()=>void) {
    if(wire.length!==746 && wire.length!==814 && wire.length!==846) throw new Error('MARKET_BOARD_PACKET');
    const actual=await this.marketAddress(expectedOwner,generation);
    const trust=await this.evmAuthority();
    const deadline=bytesToBig(wire.slice(1,9));
    const ttl=deadline-time();
    if (actual!==account || wire[0]!==21 || ttl<=0n || ttl>60n || bytesToBig(wire.slice(9,17))!==trust.revision) throw new Error('MARKET_BOARD_LEASE');
    const signed=concat(Uint8Array.of(78,53,52,74),hexToBytes(this.profile.device),bigToBytes(generation,8),
      hexToBytes(actual.slice(2)),wire.slice(0,-64));
    if(!p256.verify(wire.slice(-64),sha256(signed),trust.key,{lowS:false})) throw new Error('MARKET_BOARD_AUTHORITY');
    onBoard();
    const {reply}=await this.invoke(wire,Number(ttl),Number(ttl));
    if(reply.payload.length===1 && reply.payload[0]===0) throw new Error('MARKET_BOARD_REFUSED');
    return reply.payload;
  }

  async signDappLogin(issued:unknown,message:string,origin:string,scope:DappScope) {
    if (scope.board!==this.profile.device) throw new Error('DAPP_ACCOUNT');
    await this.verifiedEvmAddress(scope.account,BigInt(scope.generation));
    const trust=await this.evmAuthority();
    const login=readIssuedLogin(issued,message,origin,scope,trust.revision,trust.key,Number(time()));
    const ttl=login.expires-Number(time());
    if (ttl<=0) throw new Error('DAPP_LOGIN_EXPIRED');
    const {reply}=await this.invoke(login.body,ttl,ttl);
    return checkLoginSignature(reply.payload,login.digest,scope.account);
  }

  async verifiedChainAccount(account: ChainAccount, expectedAddress: string, generation: bigint, branch: 0 | 1 = 0, index = 0) {
    await this.verifiedEvmAddress(expectedAddress, generation);
    const {reply} = await this.invoke(chainPublicCommand(account, branch, index));
    return readChainPublic(reply.payload, account, branch, index);
  }

  async exportBip48Participant(account: number, onBoard: () => void, network:1|2=1) {
    const profile = {chain: 1 as const, network, account};
    onBoard();
    const {reply} = await this.invoke(Uint8Array.of(...bitcoinXpubCommand(profile), 48), 60, 60);
    return bip48Participant(reply.payload, profile);
  }

  async bip48Status(policy: Bip48Policy) {
    const {reply} = await this.invoke(Uint8Array.of(19, 1, policy.bytes[2], 0, 0, 0, policy.account));
    const bytes = reply.payload;
    if (bytes.length !== 68 || bytes[0] !== 19 || bytes.subarray(33).some(Boolean)) throw new Error('BIP48_STATUS');
    if (!bytes.subarray(1, 33).some(Boolean)) return false;
    if (!equalBytes(bytes.subarray(1, 33), policy.hash)) throw new Error('BIP48_REGISTERED_DIFFERENT');
    return true;
  }

  async registerBip48(policy: Bip48Policy, onBoard: () => void) {
    if (await this.bip48Status(policy)) return;
    onBoard();
    const {reply} = await this.invoke(policy.bytes, 60, 60);
    if (reply.payload.length === 1 && reply.payload[0] === 0) throw new Error('CHAIN_USER_REFUSED');
    if (reply.payload.length !== 33 || reply.payload[0] !== 17 ||
        !equalBytes(reply.payload.subarray(1), policy.hash)) throw new Error('BIP48_REGISTRATION_RECEIPT');
  }

  async signBip48(policy: Bip48Policy, grant: import('./chainTransfer.ts').ChainTransfer, onBoard: () => void) {
    if (!(await this.bip48Status(policy))) throw new Error('BIP48_REGISTRATION_REQUIRED');
    const ttl = grant.expires - Number(time());
    if (ttl < 1 || ttl > 60) throw new Error('CHAIN_TRANSFER_EXPIRED');
    onBoard();
    const {reply} = await this.invoke(grant.body, ttl, ttl);
    const {count} = verifyBip48Partial(grant.raw, reply.payload, policy);
    return {psbt: toBase64(reply.payload), count};
  }

  async exportBitcoinWatchAccount(account: ChainAccount, expectedAddress: string, generation: bigint, onBoard: () => void) {
    await this.verifiedEvmAddress(expectedAddress, generation);
    onBoard();
    const {reply} = await this.invoke(bitcoinXpubCommand(account), 60, 60);
    const exported = bitcoinWatchDescriptors(reply.payload, account);
    const receive = await this.verifiedChainAccount(account, expectedAddress, generation, 0);
    const change = await this.verifiedChainAccount(account, expectedAddress, generation, 1);
    if (receive.address !== exported.firstReceive || change.address !== exported.firstChange ||
        receive.fingerprint !== exported.masterFingerprint || change.fingerprint !== exported.masterFingerprint) throw new Error('BTC_XPUB_SCOPE');
    return {...exported,proof:bytesToHex(reply.payload,false)};
  }

  async signChainTransfer(transfer: ChainTransfer, expectedAddress: string, generation: bigint, onBoard: () => void) {
    const publicAccount = await this.verifiedChainAccount(transfer, expectedAddress, generation);
    const ttl = transfer.expires - Number(time());
    if (ttl < 1 || ttl > 60) throw new Error('CHAIN_TRANSFER_EXPIRED');
    onBoard();
    const {reply} = await this.invoke(transfer.body, ttl, ttl);
    return verifyChainSigned(transfer, publicAccount, reply.payload);
  }

  async maintenanceStatus():Promise<OwnerStatus> {
    const owner=await this.identity();
    const status=await this.status();
    if(status.owner!==owner.fingerprint||status.generation<=0n||!['active','pending'].includes(status.state)) {
      throw new Error('OWNER_MISMATCH');
    }
    return status;
  }

  async deviceTool(expected:OwnerStatus, operation:'read'|'find'|'cancel'):Promise<boolean> {
    const current=await this.maintenanceStatus();
    if(current.state!=='active'||expected.state!=='active'||current.device!==expected.device||
        current.owner!==expected.owner||current.generation!==expected.generation||this.ended)
      throw new Error('OWNER_MISMATCH');
    const op=operation==='read'?0x70:operation==='find'?0x71:0x72;
    const {reply}=await this.invoke(Uint8Array.of(op),8,6);
    if(this.ended||reply.payload.length!==2||reply.payload[0]!==op||reply.payload[1]>1)
      throw new Error('DEVICE_TOOL_REPLY');
    return reply.payload[1]===1;
  }

  private async currentFirmwareOwner(expected:OwnerStatus):Promise<OwnerStatus> {
    const status=await this.maintenanceStatus();
    if (this.ended || status.state!=='active' || expected.state!=='active' ||
        status.device!==expected.device || status.owner!==expected.owner || status.generation!==expected.generation)
      throw new Error('OWNER_MISMATCH');
    return status;
  }

  async runningFirmware(expected:OwnerStatus):Promise<RunningFirmware|undefined> {
    const before=await this.currentFirmwareOwner(expected);
    const {reply}=await this.invoke(runningFirmwareRequest(before.generation),8,7);
    const firmware=readRunningFirmware(reply.payload,before.device,before.generation);
    await this.currentFirmwareOwner(expected);
    return firmware;
  }

  async firmwareInputPower(expected:OwnerStatus):Promise<FirmwareInputPower> {
    const before=await this.currentFirmwareOwner(expected);
    const {reply}=await this.invoke(firmwarePowerRequest(before.generation),8,7);
    const observation=readFirmwareInputPower(reply.payload,before.device,before.generation);
    await this.currentFirmwareOwner(expected);
    return observation;
  }

  async firmwareProof(command: string, expected?:OwnerStatus): Promise<string> {
    if (!/^10[0-9a-f]{96}$/.test(command)) throw new Error('FIRMWARE_CHALLENGE_SHAPE');
    const owner = await this.identity();
    const status = await this.status();
    if (status.state !== 'active' || status.owner !== owner.fingerprint || expected&&
        (expected.state!=='active'||status.device!==expected.device||status.owner!==expected.owner||status.generation!==expected.generation)) {
      throw new Error('OWNER_MISMATCH');
    }
    const body = hexToBytes(command);
    const { reply } = await this.invoke(body, 30, 15);
    const payload = reply.payload;
    if (payload.length !== 181 || payload[0] !== 16 ||
        bytesToHex(payload.subarray(1, 5), false) !== '4e353446' ||
        !equalBytes(payload.subarray(5, 53), body.subarray(1))) {
      throw new Error('FIRMWARE_PROOF_BINDING');
    }
    if(expected) {
      const after=await this.maintenanceStatus();
      if(after.state!=='active'||after.owner!==status.owner||after.generation!==status.generation)throw new Error('OWNER_MISMATCH');
    }
    return bytesToHex(payload.subarray(1), false);
  }

  async reclaimPaymentReceipt(raw: string): Promise<'reclaimed' | 'absent'> {
    if (!/^08[0-9a-f]{1506}$/.test(raw)) throw new Error('RECEIPT_DISPOSITION_SHAPE');
    const owner = await this.identity();
    const status = await this.status();
    if (status.state !== 'active' || status.owner !== owner.fingerprint) throw new Error('OWNER_MISMATCH');
    const {reply} = await this.invoke(hexToBytes(raw), 60, 10);
    if (reply.payload.length !== 1 || reply.payload[0] > 1) throw new Error('RECEIPT_DISPOSITION_REPLY');
    return reply.payload[0] ? 'reclaimed' : 'absent';
  }

  async register(onConfirm: () => void): Promise<OwnerStatus> {
    const own = await this.identity();
    const status = await this.status();
    if (status.state === 'active' && status.owner === own.fingerprint) return status;
    if (status.state !== 'new' || status.generation !== 0n) throw new Error('OWNER_ALREADY_BOUND');
    return this.proveOwner(1,own,status,onConfirm);
  }

  async revoke(onConfirm: () => void): Promise<OwnerStatus> {
    const own = await this.identity();
    const status = await this.status();
    if (status.state !== 'active' || status.owner !== own.fingerprint) throw new Error('OWNER_MISMATCH');
    return this.proveOwner(3,own,status,onConfirm);
  }

  private async proveOwner(operation: 1 | 3, own: OwnershipProof, previous: OwnerStatus, onConfirm: () => void) {
    const {reply,id,expires} = await this.invoke(ownerBegin(operation,own.fingerprint),60);
    validateChallenge(reply.payload, {operation, device:this.profile.device, owner:own.fingerprint,
      generation:previous.generation, id, expires, now:time()});
    const signed = fromBase64(await this.native.signChallenge(toBase64(reply.payload)));
    const remaining = Number(expires - time());
    if (remaining <= 0) throw new Error('OWNER_CHALLENGE_EXPIRED');
    onConfirm();
    const proven = await this.invoke(ownerProof(reply.payload,own.publicKey,signed),remaining);
    const final = readStatus(proven.reply.payload,this.profile.device);
    const expectedState = operation === 1 ? 'active' : 'revoked';
    const expectedOwner = operation === 1 ? own.fingerprint : '0'.repeat(64);
    if (final.state !== expectedState || final.generation !== previous.generation+1n || final.owner !== expectedOwner) {
      throw new Error('OWNER_PHYSICAL_CONFIRMATION');
    }
    return final;
  }
}
