import {bytesToHex,toBase64,utf8Encode} from '@nu54/protocol';
import {sha256} from '@noble/hashes/sha256';
import {readChainTransfer} from './chainTransfer.ts';
import {readDirectTransfer} from './evmTransfer.ts';
import {decodeWalletStorage, type SigningEvmContext, type SigningReservation, type WalletStorage} from './profile.ts';

export const profileLimit = 32768;
const cacheBudget = 8192;
const signatureReserve = 8192;
const json = (value: unknown) => JSON.stringify(value);
const size = (value: string) => utf8Encode(value).length;
const same = (left: unknown, right: unknown) => json(left) === json(right);

export function encodeWalletProfile(record: WalletStorage) {
  const limit = profileLimit - (record.signing ? signatureReserve : 0);
  const read = record.walletRead;
  if (!read) {
    const raw = json(record);
    if (size(raw) > limit) throw new Error('WALLET_STORAGE_FULL');
    return raw;
  }
  const observations = [...read.observations];
  const raw = () => json({...record, walletRead: {...read, observations}});
  while (observations.length && (size(json(observations)) > cacheBudget || size(raw()) > limit)) {
    observations.pop();
  }
  const encoded = raw();
  if (size(encoded) > limit) throw new Error('WALLET_STORAGE_FULL');
  return encoded;
}

type NativeProfile = {
  loadProfile(): Promise<string>;
  compareAndSaveProfile(expected: string, value: string): Promise<boolean>;
};

function rebaseProfile(base: WalletStorage, next: WalletStorage, head: WalletStorage): WalletStorage {
  if (!same(base.verified, head.verified) || !same(base.enrollment, head.enrollment) || base.mode !== head.mode) {
    throw new Error('WALLET_STORAGE_CONFLICT');
  }
  if ((!same(base.signing, next.signing) || !same(base.pendingEvm, next.pendingEvm) ||
      !same(base.pendingChain, next.pendingChain)) &&
      (!same(base.signing, head.signing) || !same(base.pendingEvm, head.pendingEvm) ||
       !same(base.pendingChain, head.pendingChain))) throw new Error('WALLET_STORAGE_CONFLICT');
  const result = {...head};
  // Top-level sections are independent. A stale update to the same section
  // fails instead of merging transaction bytes, consent, or session state.
  const keys = new Set([...Object.keys(base), ...Object.keys(next)] as (keyof WalletStorage)[]);
  for (const key of keys) {
    if (same(base[key], next[key])) continue;
    if (!same(base[key], head[key])) throw new Error('WALLET_STORAGE_CONFLICT');
    Object.assign(result, {[key]: next[key]});
  }
  if (!same(base.verified, result.verified)) {
    if (head.pendingEvm) result.pendingEvm = {...head.pendingEvm, submitted: true};
    if (head.pendingChain) result.pendingChain = {...head.pendingChain, attempted: true};
  }
  return result;
}

export class WalletProfileStore {
  constructor(private readonly native: NativeProfile) {}
  async load() {
    return decodeWalletStorage(await this.native.loadProfile());
  }

  async save(base: WalletStorage, next: WalletStorage) {
    for (let attempt = 0; attempt < 3; attempt++) {
      const expected = await this.native.loadProfile();
      const updated = rebaseProfile(base, next, decodeWalletStorage(expected));
      const encoded = encodeWalletProfile(updated);
      const result = decodeWalletStorage(encoded);
      if (expected === encoded) return result;
      try {
        if (await this.native.compareAndSaveProfile(expected, encoded)) return result;
      } catch (error) {
        // A lost response is safe to accept only if protected storage
        // now contains the exact bytes we intended to commit.
        if (await this.native.loadProfile() === encoded) return result;
        throw error;
      }
    }
    throw new Error('WALLET_STORAGE_CONFLICT');
  }
}

export async function persistWalletSignature(
  base: WalletStorage, kind: SigningReservation['kind'], wire: string, expires: number,
  commit: (next: WalletStorage, before: WalletStorage) => Promise<WalletStorage>,
  approve: () => Promise<Pick<WalletStorage, 'pendingEvm' | 'pendingChain' | 'pendingBip48'>>,
  evm?:SigningEvmContext,
) {
  if (!base.verified || base.signing) throw new Error('WALLET_SIGNING_UNRESOLVED');
  if(kind==='evm') {
    if(!evm || evm.signer!==base.verified.address || !/^[a-f0-9]{32}$/.test(evm.request) || !/[1-9a-f]/.test(evm.request))throw new Error('WALLET_SIGNING_CONTEXT');
    const transfer=readDirectTransfer(wire,BigInt(expires)-1n);
    if(transfer.expires!==BigInt(expires) || transfer.chain!==evm.chain || transfer.genesis!==evm.genesis ||
       bytesToHex(transfer.raw,true)!==evm.unsigned)throw new Error('WALLET_SIGNING_CONTEXT');
  } else if(evm)throw new Error('WALLET_SIGNING_CONTEXT');
  const multisig=kind==='bip48' ? readChainTransfer(wire,expires-1,18) : undefined;
  if (multisig && (multisig.expires!==expires || multisig.chain!==1 || ![1,2].includes(multisig.network)))
    throw new Error('WALLET_SIGNING_CONTEXT');
  if (kind==='bip48' && base.pendingBip48?.board===base.verified.boardFingerprint &&
      base.pendingBip48.generation===base.verified.generation) throw new Error('WALLET_SIGNING_UNRESOLVED');
  const signing: SigningReservation = {kind, digest: bytesToHex(sha256(utf8Encode(wire)), false),
    board: base.verified.boardFingerprint, generation: base.verified.generation, expires,...(evm ? {evm} : {})};
  const staged = await commit({...base, signing}, base);
  let checkpoint: Awaited<ReturnType<typeof approve>>;
  try {
    checkpoint = await approve();
  } catch (error) {
    // Only an authenticated board refusal is definitive. A disconnected
    // or otherwise uncertain signature exchange keeps the reservation.
    if (error instanceof Error && ['CHAIN_USER_REFUSED', 'EVM_REFUSED'].includes(error.message)) {
      await commit({...staged, signing: undefined}, staged);
    }
    throw error;
  }
  if (kind==='evm' && (!checkpoint.pendingEvm || checkpoint.pendingEvm.submitted) ||
      kind==='chain' && (!checkpoint.pendingChain || checkpoint.pendingChain.attempted)) throw new Error('WALLET_SIGNING_CHECKPOINT');
  if (multisig) {
    const transfer=multisig, result=checkpoint.pendingBip48;
    if (!result || base.pendingBip48?.id===result.id || transfer.expires!==expires || transfer.chain!==1 ||
        result.account!==transfer.account || result.network!==transfer.network || result.board!==signing.board ||
        result.generation!==signing.generation || result.original!==toBase64(transfer.raw) ||
        result.policy!==bytesToHex(transfer.body.subarray(23,55),false)) throw new Error('WALLET_SIGNING_CHECKPOINT');
  }
  if(kind==='evm') {
    const p=checkpoint.pendingEvm,e=p?.evidence;
    if(!evm || !p || !e || e.board!==signing.board || e.generation!==signing.generation || e.request!==evm.request ||
       e.unsigned!==evm.unsigned || p.chain!==evm.chain || p.genesis!==evm.genesis || p.sender!==evm.signer || p.expires!==expires)
      throw new Error('WALLET_SIGNING_CHECKPOINT');
  }
  return commit({...staged, ...checkpoint, signing: undefined}, staged);
}
