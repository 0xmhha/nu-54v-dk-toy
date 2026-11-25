// One transfer to another address: written down before every step, like a return, and followed by its nonce.
import {type CommerceClient, hexToBytes, recoverSigner, orderTokenName, walletReturnDigestFor, type WalletSend} from '@nu54/protocol';
import {customerDigest} from '../merchant/customerJournal.ts';
import type {OrderAccount} from '../merchant/orderRecords.ts';
import {atOtherStore, returnRefusals} from './boardReturn.ts';

type Storage = {loadJournal(name: string): Promise<string>; saveJournal(name: string, value: string): Promise<void>};
const address = /^0x[0-9a-f]{40}$/, hash = /^[0-9a-f]{64}$/;
const nonzero = (value: string) => !/^(0x)?0+$/.test(value);

export type SendRecord = OrderAccount & {
  device: string; token: string; to: string; amount: string; validBefore: number; nonce: string; key: string;
  stage: 'signing' | 'signed' | 'sent' | 'final' | 'failed';
  signature?: string; sentAt?: number; txHash?: string;
  /** The store that was asked to send it. Only that store can say what became of it. */
  branch?: string;
};
const stages = ['signing', 'signed', 'sent', 'final', 'failed'];
const fields = ['payer', 'generation', 'device', 'token', 'to', 'amount', 'validBefore', 'nonce', 'key', 'stage', 'signature', 'sentAt', 'txHash', 'branch'];
const name = `customer-order-${customerDigest('nu54-wallet-send-v1')}`;

export function validSend(value: SendRecord) {
  return !!value && Object.keys(value).every(key => fields.includes(key)) &&
    [value.payer, value.to, value.token].every(entry => address.test(entry) && nonzero(entry)) && value.payer !== value.to && value.to !== value.token &&
    /^[1-9][0-9]{0,19}$/.test(value.generation) && hash.test(value.device) && /^[1-9][0-9]{0,19}$/.test(value.amount) &&
    Number.isSafeInteger(value.validBefore) && value.validBefore > 0 && hash.test(value.nonce) && nonzero(value.nonce) &&
    /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(value.key) && stages.includes(value.stage) &&
    (value.signature === undefined || /^[0-9a-f]{128}(1b|1c)$/.test(value.signature)) && (value.stage === 'signing' || value.signature !== undefined) &&
    (value.sentAt === undefined || (Number.isSafeInteger(value.sentAt) && value.sentAt > 0)) &&
    (value.txHash === undefined || /^0x[0-9a-f]{64}$/.test(value.txHash)) && (value.branch === undefined || /^[0-9a-f-]{36}$/.test(value.branch));
}

export function sendRecord(storage: Storage) {
  return {
    async load(): Promise<SendRecord | undefined> {
      const raw = await storage.loadJournal(name);
      const value = raw ? JSON.parse(raw) as SendRecord | null : null;
      if (value && !validSend(value)) throw new Error('SEND_RECORD');
      return value ?? undefined;
    },
    async save(value?: SendRecord) {
      if (value && !validSend(value)) throw new Error('SEND_RECORD');
      await storage.saveJournal(name, JSON.stringify(value ?? null));
    },
  };
}

/** After a reset only a transfer already on its way is still worth following. */
export async function forgetSend(storage: Storage) {
  const records = sendRecord(storage);
  if ((await records.load())?.stage !== 'sent') await records.save(undefined);
}

// As for a return: room for a board clock that lags behind, and time for the press, the PIN and the sending.
const lifetime = 170;
export const newSend = (base: Pick<SendRecord, 'payer' | 'generation' | 'device' | 'token' | 'to'>, amount: bigint, nonce: string, key: string, now: number): SendRecord =>
  ({...base, amount: amount.toString(), validBefore: now + lifetime, nonce, key, stage: 'signing'});
/** The same transfer with a new deadline. The nonce stays, so only one signature over it can ever run. */
export const renewSend = (record: SendRecord, now: number): SendRecord => ({...record, validBefore: now + lifetime});
/** Signed, never sent, and too late to send. */
export const sendStale = (record: SendRecord, now: number) => record.stage === 'signed' && record.sentAt === undefined && record.validBefore - now < 20;

/** The signature must be the board account's, over exactly this transfer. */
export function verifySend(record: SendRecord, signature: string, tokenName = orderTokenName): SendRecord {
  const digest = walletReturnDigestFor(tokenName, {token: record.token, from: record.payer, to: record.to, amount: BigInt(record.amount),
    validBefore: BigInt(record.validBefore), nonce: record.nonce});
  if (recoverSigner(digest, hexToBytes(signature)) !== record.payer) throw new Error('SEND_SIGNATURE');
  return {...record, signature, stage: 'signed'};
}

const sameSend = (record: SendRecord, seen: WalletSend) => seen.nonce === record.nonce && seen.payer === record.payer && seen.to === record.to &&
  seen.token === record.token && seen.amount === record.amount && seen.validBefore === record.validBefore;

/** What the server's list means for the record. A transfer it does not know ends only when it can no longer run. */
export function followSend(record: SendRecord, sends: WalletSend[], now: number): SendRecord {
  const seen = sends.find(item => sameSend(record, item));
  if (seen) return {...record, stage: seen.state === 'final' ? 'final' : seen.state === 'failed' ? 'failed' : 'sent', ...(seen.txHash ? {txHash: seen.txHash} : {})};
  return now > record.validBefore + 120 ? {...record, stage: 'failed'} : record;
}

/** Reads what the server holds for a transfer on its way. A record that moved on meanwhile is left alone. */
export async function settleSend(storage: Storage, client: CommerceClient, now: () => number) {
  const records = sendRecord(storage), current = await records.load();
  if (current?.stage !== 'sent' || atOtherStore(current, client.profile?.branchRef)) return current;
  const next = followSend(current, (await client.walletSends()).sends, now());
  const latest = await records.load();
  if (JSON.stringify(latest) !== JSON.stringify(current)) return latest;
  if (JSON.stringify(next) !== JSON.stringify(current)) await records.save(next);
  return next;
}

/** Sends the signed transfer once it is written down. A first refusal that left nothing behind is thrown. */
export async function submitSend(client: CommerceClient, record: SendRecord, save: (next?: SendRecord) => Promise<void>, now: number, tokenName = orderTokenName): Promise<SendRecord> {
  if (!record.signature || verifySend(record, record.signature, tokenName).signature !== record.signature) throw new Error('SEND_SIGNATURE');
  if (record.validBefore <= now) throw new Error('SEND_EXPIRED');
  if (record.stage === 'sent' && atOtherStore(record, client.profile?.branchRef)) throw new Error('SENT_AT_OTHER_STORE');
  const first = record.sentAt === undefined;
  const sent: SendRecord = {...record, stage: 'sent', sentAt: now, ...(client.profile ? {branch: client.profile.branchRef} : {})};
  await save(sent);
  try {
    const {payer, to, token, amount, validBefore, nonce} = sent;
    await client.sendWallet({payer, to, token, amount, validBefore, nonce}, record.signature, record.key);
    return sent;
  } catch (error) {
    const reason = error instanceof Error ? error.message : '';
    if (!first || !(returnRefusals.includes(reason) || (error as {code?: string}).code === 'COMMERCE_REFUSED')) return sent;
    // Refused on the first try: the server holds nothing, unless its answer only came back late.
    const held = await client.walletSends().catch(() => undefined);
    if (!held) return sent;
    if (held.sends.some(item => sameSend(record, item))) return followSend(sent, held.sends, now);
    await save(record);
    throw error;
  }
}
