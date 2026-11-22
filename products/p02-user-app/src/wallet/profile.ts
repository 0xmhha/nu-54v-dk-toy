import {readStoredBip48,type StoredBip48} from './bip48Wallet.ts';
import {isWalletReadState, type ReadProfileState} from './readProfiles.ts';
import type {ConnectCheckpoint} from './dapp/walletConnectSession.ts';
import {validInstallCheckpoint, type InstallCheckpoint} from './boardInstallation.ts';
import {readPractice, type Practice} from './practiceFlow.ts';
import {chainAccountCommand,readStoredBitcoinWatch,type StoredBitcoinWatch,type ChainAccount} from './chainAccounts.ts';
import {readStoredChain, type StoredChain} from './chainTransfer.ts';
import {isHttpsEndpoint,readRegistryEntry,type EvmRegistryEntry} from './evmRegistry.ts';
import type {Enrollment} from './ownerWire.ts';
import {evmProfiles,unsignedEvmDigest,verifyPendingOriginal,type EvmChain} from './evmTransfer.ts';
import type {DeviceRecord} from '../setup/flow.ts';

export type PersonalMetadata = {
  boardFingerprint: string;
  address: string;
  network: '8283';
  generation: number;
};

export type WalletScreen =
  | {kind: 'choose'}
  | {kind: 'legacy'; device: DeviceRecord}
  | {kind: 'practice'}
  | {kind: 'locked'; metadata: PersonalMetadata}
  | {kind: 'personal'; metadata: PersonalMetadata}
  | {kind: 'recovering'; metadata: PersonalMetadata};

export type SourceBackupCheckpoint = {device:string; owner:string; generation:string};

export type PhoneReplacementCheckpoint = {
  device: string; replacement: string; generation: string; address: string;
};

export type PendingEvmTransfer = {chain:EvmChain; raw:string; hash:string; sender:string; expires:number; submitted:boolean; genesis?:string; profileRevision?:number; leaseRevision?:string; rpc?:string; evidence?:{board:string;generation:number;request:string;unsigned:string}};
export type EvmRecipient = {name:string;genesis:string;address:string};
export type StoredConnect={relay:string;seed:string;checkpoint:ConnectCheckpoint};
export type SigningEvmContext={request:string;unsigned:string;chain:EvmChain;genesis:string;signer:string};
export type SigningReservation = {kind: 'evm' | 'chain' | 'bip48'; digest: string; board: string; generation: number; expires: number; evm?:SigningEvmContext};
export type WalletStorage = {
  version: 2; exercised: boolean; language?: 'ko' | 'en'; practice?: Practice; installation?: InstallCheckpoint; mode?: 'personal' | 'legacy'; verified?: PersonalMetadata;
  pendingBip48?:StoredBip48; bitcoinWatch?:StoredBitcoinWatch; enrollment?: Enrollment; backupCheckpoint?: SourceBackupCheckpoint; phoneReplacement?: PhoneReplacementCheckpoint; pendingEvm?:PendingEvmTransfer; evmRegistry?:EvmRegistryEntry; evmRecipients?:EvmRecipient[]; pendingChain?:StoredChain; chainSelection?:ChainAccount; walletConnect?:StoredConnect; walletConnectSessions?:StoredConnect[]; walletRead?:ReadProfileState; signing?: SigningReservation;
};

const fingerprint = /^[a-f0-9]{64}$/;
const evmAddress = /^0x[0-9a-f]{40}$/;

export function isMetadata(value: unknown): value is PersonalMetadata {
  if (!value || typeof value !== 'object') return false;
  const m = value as PersonalMetadata;
  return typeof m.boardFingerprint === 'string' && fingerprint.test(m.boardFingerprint) &&
    typeof m.address === 'string' && evmAddress.test(m.address) && m.address !== '0x' + '0'.repeat(40) &&
    m.network === '8283' && Number.isSafeInteger(m.generation) && m.generation > 0 &&
    Object.keys(m).length === 4;
}

function isBackupCheckpoint(value: unknown): value is SourceBackupCheckpoint {
  if (!value || typeof value !== 'object' || Object.keys(value).length !== 3) return false;
  const entry = value as SourceBackupCheckpoint;
  return typeof entry.device === 'string' && fingerprint.test(entry.device) &&
    typeof entry.owner === 'string' && fingerprint.test(entry.owner) &&
    typeof entry.generation === 'string' && /^[1-9][0-9]{0,19}$/.test(entry.generation) &&
    BigInt(entry.generation) < 2n ** 64n;
}

function isCheckpoint(value: unknown): value is PhoneReplacementCheckpoint {
  if (!value || typeof value !== 'object' || Object.keys(value).length !== 4) return false;
  const entry = value as PhoneReplacementCheckpoint;
  return typeof entry.device === 'string' && fingerprint.test(entry.device) &&
    typeof entry.replacement === 'string' && fingerprint.test(entry.replacement) &&
    typeof entry.generation === 'string' && /^[1-9][0-9]{0,19}$/.test(entry.generation) &&
    BigInt(entry.generation) < (2n ** 64n) - 1n && typeof entry.address === 'string' &&
    evmAddress.test(entry.address) && entry.address !== '0x' + '0'.repeat(40);
}

function isPendingEvm(value:unknown): value is PendingEvmTransfer {
  if (!value || typeof value !== 'object') return false;
  const entry = value as PendingEvmTransfer;
  const keys = Object.keys(value),allowed=['chain','raw','hash','sender','expires','submitted','genesis','profileRevision','leaseRevision','rpc','evidence'];
  if(keys.some(key=>!allowed.includes(key)))return false;
  const bound = typeof entry.genesis === 'string' && /^0x[a-f0-9]{64}$/.test(entry.genesis) && entry.genesis !== '0x'+'0'.repeat(64) &&
    entry.profileRevision === 1 && typeof entry.leaseRevision === 'string' && /^[1-9][0-9]{0,19}$/.test(entry.leaseRevision) && BigInt(entry.leaseRevision) < 2n**64n &&
    (entry.rpc===undefined || isHttpsEndpoint(entry.rpc));
  const legacy = keys.length === 6 && keys.every(key=>['chain','raw','hash','sender','expires','submitted'].includes(key));
  if(!(bound || legacy) || !Object.prototype.hasOwnProperty.call(evmProfiles,entry.chain) || !/^0x02(?:[a-f0-9]{2}){1,322}$/.test(entry.raw) ||
     !/^0x[a-f0-9]{64}$/.test(entry.hash) || !evmAddress.test(entry.sender) || !Number.isSafeInteger(entry.expires) || entry.expires<=0 || typeof entry.submitted!=='boolean')return false;
  if(entry.evidence!==undefined){
    const e=entry.evidence;
    if(!bound || !e || Object.keys(e).sort().join(',')!=='board,generation,request,unsigned' || !fingerprint.test(e.board) ||
       !Number.isSafeInteger(e.generation) || e.generation<=0 || !/^[a-f0-9]{32}$/.test(e.request) || !/[1-9a-f]/.test(e.request))return false;
    try{verifyPendingOriginal(entry.raw,entry.hash,entry.chain,entry.sender,e.unsigned);}catch{return false;}
  }
  return true;
}

function validChainSelection(value: ChainAccount) {
  try {
    return !!value && Object.keys(value).length === 3 && (value.network === 1 || value.network === 2) &&
      chainAccountCommand(10, value).length === 7;
  } catch {
    return false;
  }
}

function validSigning(value: SigningReservation) {
  if(!value || !['evm','chain','bip48'].includes(value.kind) || typeof value.digest!=='string' || !fingerprint.test(value.digest) ||
     typeof value.board!=='string' || !fingerprint.test(value.board) || !Number.isSafeInteger(value.generation) || value.generation<=0 ||
     !Number.isSafeInteger(value.expires) || value.expires<=0 || Object.keys(value).some(k=>!['kind','digest','board','generation','expires','evm'].includes(k)))return false;
  if(value.evm!==undefined){const e=value.evm;
    if(value.kind!=='evm' || !e || Object.keys(e).sort().join(',')!=='chain,genesis,request,signer,unsigned' ||
       typeof e.request!=='string' || !/^[a-f0-9]{32}$/.test(e.request) || !/[1-9a-f]/.test(e.request) ||
       !Object.hasOwn(evmProfiles,e.chain) || typeof e.genesis!=='string' || !/^0x[a-f0-9]{64}$/.test(e.genesis) || /^0x0{64}$/.test(e.genesis) ||
       typeof e.signer!=='string' || !evmAddress.test(e.signer))return false;
    try{unsignedEvmDigest(e.unsigned,e.chain);}catch{return false;}
  }
  return true;
}

function validConnectOwner(value:StoredConnect, owner:PersonalMetadata|undefined) {
  return value && typeof value.relay==='string' && value.relay.length<=320 && fingerprint.test(value.seed) &&
    fingerprint.test(value.checkpoint?.topic) && owner &&
    value.checkpoint.scope?.board===owner.boardFingerprint && value.checkpoint.scope?.account===owner.address &&
    value.checkpoint.scope?.generation===owner.generation;
}

export function storedWalletConnects(record:WalletStorage|undefined):StoredConnect[] {
  const sessions=new Map<string,StoredConnect>();
  for (const item of [...(record?.walletConnect ? [record.walletConnect] : []),...(record?.walletConnectSessions ?? [])]) {
    const earlier=sessions.get(item.checkpoint.topic);
    if (!earlier || item.checkpoint.lastId>=earlier.checkpoint.lastId) sessions.set(item.checkpoint.topic,item);
  }
  return [...sessions.values()];
}

export function decodeWalletStorage(raw: string): WalletStorage {
  if (!raw) return {version: 2, exercised: false};
  const value = JSON.parse(raw) as WalletStorage;
  if (value?.evmRegistry) {
    const {registry} = readRegistryEntry(value.evmRegistry);
    if (!value.evmRegistry.anchor || registry.device !== value.verified?.boardFingerprint || registry.sender !== value.verified?.address ||
        registry.generation !== value.verified?.generation) throw new Error('WALLET_STORAGE_CORRUPT');
  }
  if (!value || value.version !== 2 || typeof value.exercised !== 'boolean' ||
      ![undefined, 'legacy', 'personal'].includes(value.mode) ||
      (value.practice !== undefined && !readPractice(JSON.stringify(value.practice))) ||
      (value.installation !== undefined && !validInstallCheckpoint(value.installation)) ||
      (value.language !== undefined && value.language !== 'ko' && value.language !== 'en') ||
      (value.verified !== undefined && (!isMetadata(value.verified) || value.mode !== 'personal')) ||
      (value.enrollment !== undefined && value.mode !== 'personal') ||
      (value.backupCheckpoint !== undefined && (value.mode !== 'personal' || !isBackupCheckpoint(value.backupCheckpoint) ||
        value.enrollment?.device !== value.backupCheckpoint.device)) ||
      (value.phoneReplacement !== undefined && (value.mode !== 'personal' || !isCheckpoint(value.phoneReplacement) ||
        value.enrollment?.device !== value.phoneReplacement.device)) ||
      (value.pendingEvm !== undefined && (value.mode !== 'personal' || !isPendingEvm(value.pendingEvm))) ||
      (value.pendingChain !== undefined && (value.mode !== 'personal' || !readStoredChain(value.pendingChain) ||
        (!value.pendingChain.attempted && (!value.verified || value.pendingChain.ownerAddress !== value.verified.address ||
          (value.pendingChain.clock && value.pendingChain.clock.board !== value.verified.boardFingerprint) ||
          value.pendingChain.ownerGeneration !== value.verified.generation)))) ||
      (value.pendingBip48 !== undefined && (value.mode !== 'personal' || !readStoredBip48(value.pendingBip48))) ||
      (value.bitcoinWatch !== undefined && (value.mode !== 'personal' || !readStoredBitcoinWatch(value.bitcoinWatch))) ||
      (value.chainSelection !== undefined && (value.mode !== 'personal' || !validChainSelection(value.chainSelection))) ||
      (value.signing !== undefined && (value.mode !== 'personal' || !validSigning(value.signing))) ||
      (value.walletRead !== undefined && (value.mode !== 'personal' || !isWalletReadState(value.walletRead))) ||
      (value.walletConnect !== undefined && (value.mode!=='personal' || !validConnectOwner(value.walletConnect,value.verified))) ||
      (value.walletConnectSessions !== undefined && (value.mode!=='personal' || !Array.isArray(value.walletConnectSessions) ||
        value.walletConnectSessions.some(session=>!validConnectOwner(session,value.verified)))) ||
      (value.evmRecipients !== undefined && (!Array.isArray(value.evmRecipients) || value.evmRecipients.length > 12 ||
        value.evmRecipients.some(item=>!item || typeof item.name !== 'string' || item.name.length<1 || item.name.length>32 ||
          !/^0x[a-f0-9]{64}$/.test(item.genesis) || !evmAddress.test(item.address)))) ||
      Object.keys(value).some(key => !['version', 'exercised', 'practice', 'language', 'installation', 'mode', 'verified', 'enrollment', 'backupCheckpoint', 'phoneReplacement', 'pendingEvm', 'pendingChain', 'chainSelection', 'bitcoinWatch', 'pendingBip48', 'evmRegistry', 'evmRecipients','walletConnect','walletConnectSessions','walletRead','signing'].includes(key))) {
    throw new Error('WALLET_STORAGE_CORRUPT');
  }
  return value;
}

export function beginWallet(legacy: DeviceRecord[], local: WalletStorage | null): WalletScreen {
  // A protected personal record is historical metadata. A fresh board session
  // rechecks the active owner generation before exposing any approval.
  if (local?.mode === 'personal') {
    return local.verified ? {kind: 'locked', metadata: local.verified} : {kind: 'choose'};
  }
  if (legacy[0]) return {kind: 'legacy', device: legacy[0]};
  return {kind: 'choose'};
}

export function sameAccount(a: PersonalMetadata, b: PersonalMetadata): boolean {
  return a.network === b.network && a.address === b.address;
}

// The board/independent proof protocol explicitly grants these states after its
// own credential verification. JSON or UI caller values never enter this map.
const currentApprovals = new WeakMap<object, {metadata: PersonalMetadata; until: number; intent: string}>();
const verifiedRecovery = new WeakMap<object, {from: PersonalMetadata; to: PersonalMetadata}>();

export function verifyRecovery(previous: Extract<WalletScreen, {kind: 'recovering'}>, receipt: unknown): WalletScreen {
  if (!receipt || typeof receipt !== 'object') return previous;
  const binding = verifiedRecovery.get(receipt);
  if (!binding || !sameAccount(previous.metadata, binding.from) || !sameAccount(binding.from, binding.to) ||
      previous.metadata.boardFingerprint !== binding.from.boardFingerprint ||
      previous.metadata.generation !== binding.from.generation) return previous;
  return {kind: 'locked', metadata: binding.to};
}

export function allowPersonalApproval(current: WalletScreen, proof: unknown, expectedIntent?: string): boolean {
  if (current.kind !== 'personal' || !proof || typeof proof !== 'object' || typeof expectedIntent !== 'string') return false;
  const binding = currentApprovals.get(proof);
  return !!binding && binding.until > Date.now() && sameAccount(current.metadata, binding.metadata) &&
    current.metadata.generation === binding.metadata.generation &&
    current.metadata.boardFingerprint === binding.metadata.boardFingerprint && binding.intent === expectedIntent;
}
