// What the phone decides and remembers while a spending board is set up, returned and reset.
// The board and the server have the last word; this file only keeps their steps in order.
import {type CommerceClient, type CommerceOrder, hexToBytes, recoverSigner, orderTokenName, walletReturnDigestFor, type WalletReturn} from '@nu54/protocol';
import {AccountAskError} from '../merchant/customerEntrance.ts';
import {customerDigest} from '../merchant/customerJournal.ts';
import {isOpenOrder, type OpenOrder, type OrderAccount} from '../merchant/orderRecords.ts';

type Storage = {loadJournal(name: string): Promise<string>; saveJournal(name: string, value: string): Promise<void>};
const address = /^0x[0-9a-f]{40}$/, hash = /^[0-9a-f]{64}$/;
const nonzero = (value: string) => !/^(0x)?0+$/.test(value);

/** One return: the transfer home, its confirmation, then the reset. Written down before every step. */
export type ReturnRecord = OrderAccount & {
  device: string; token: string; to: string; amount: string; validBefore: number; nonce: string; key: string;
  stage: 'signing' | 'signed' | 'sent' | 'final' | 'resetting';
  signature?: string; sentAt?: number; txHash?: string;
  /** The store that was asked to send it. Only that store can say what became of it. */
  branch?: string;
};
const stages = ['signing', 'signed', 'sent', 'final', 'resetting'];
const fields = ['payer', 'generation', 'device', 'token', 'to', 'amount', 'validBefore', 'nonce', 'key', 'stage', 'signature', 'sentAt', 'txHash', 'branch'];
const name = `customer-order-${customerDigest('nu54-wallet-return-v1')}`;

export function validReturn(value: ReturnRecord) {
  return !!value && Object.keys(value).every(key => fields.includes(key)) &&
    [value.payer, value.to].every(entry => address.test(entry) && nonzero(entry)) && address.test(value.token) && value.payer !== value.to &&
    /^[1-9][0-9]{0,19}$/.test(value.generation) && hash.test(value.device) && /^(0|[1-9][0-9]{0,77})$/.test(value.amount) &&
    Number.isSafeInteger(value.validBefore) && value.validBefore > 0 && hash.test(value.nonce) && nonzero(value.nonce) &&
    /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(value.key) && stages.includes(value.stage) &&
    (value.signature === undefined || /^[0-9a-f]{128}(1b|1c)$/.test(value.signature)) &&
    (value.amount === '0' ? ['final', 'resetting'].includes(value.stage) : value.stage === 'signing' || value.signature !== undefined) &&
    (value.sentAt === undefined || (Number.isSafeInteger(value.sentAt) && value.sentAt > 0)) &&
    (value.txHash === undefined || /^0x[0-9a-f]{64}$/.test(value.txHash)) && (value.branch === undefined || /^[0-9a-f-]{36}$/.test(value.branch));
}

/** Sent through another store: this one knows nothing of it, and its silence means nothing. */
export const atOtherStore = (record: {branch?: string}, branch: string | undefined) => !!record.branch && !!branch && record.branch !== branch;

const watchers = new Set<() => void>();
/** Called after the return on this phone changed. */
export function onReturnChanged(run: () => void) {
  watchers.add(run);
  return () => { watchers.delete(run); };
}

export function returnRecord(storage: Storage) {
  return {
    async load(): Promise<ReturnRecord | undefined> {
      const raw = await storage.loadJournal(name);
      const value = raw ? JSON.parse(raw) as ReturnRecord | null : null;
      if (value && !validReturn(value)) throw new Error('RETURN_RECORD');
      return value ?? undefined;
    },
    async save(value?: ReturnRecord) {
      if (value && !validReturn(value)) throw new Error('RETURN_RECORD');
      await storage.saveJournal(name, JSON.stringify(value ?? null));
      watchers.forEach(run => run());
    },
  };
}

/** After a reset only a transfer already on its way is still worth following. */
export async function forgetReturn(storage: Storage) {
  const records = returnRecord(storage);
  if ((await records.load())?.stage !== 'sent') await records.save(undefined);
}

// The board takes up to 600 seconds counted from a time that may lag behind the phone's, so a short life leaves
// room for that lag. It still covers the press, the PIN and the sending. Counted anew at every board request.
const lifetime = 170;
/** A signed transfer that was never sent and can no longer be sent in time. */
export const returnStale = (record: ReturnRecord, now: number) => record.stage === 'signed' && record.sentAt === undefined && record.validBefore - now < 20;
/** The same transfer with a new deadline. The nonce stays, so only one signature over it can ever run. */
export const renewReturn = (record: ReturnRecord, now: number): ReturnRecord => ({...record, validBefore: now + lifetime});

export function newReturn(base: Pick<ReturnRecord, 'payer' | 'generation' | 'device' | 'token' | 'to'>, amount: bigint, nonce: string, key: string, now: number): ReturnRecord {
  return {...base, amount: amount.toString(), validBefore: now + lifetime, nonce, key, stage: amount ? 'signing' : 'final'};
}

/** The signature must be the board account's, over exactly this transfer. */
export function verifyReturn(record: ReturnRecord, signature: string, tokenName = orderTokenName): ReturnRecord {
  const digest = walletReturnDigestFor(tokenName, {token: record.token, from: record.payer, to: record.to, amount: BigInt(record.amount),
    validBefore: BigInt(record.validBefore), nonce: record.nonce});
  if (recoverSigner(digest, hexToBytes(signature)) !== record.payer) throw new Error('RETURN_SIGNATURE');
  return {...record, signature, stage: 'signed'};
}

const sameReturn = (record: ReturnRecord, seen: WalletReturn) => seen.payer === record.payer && seen.to === record.to &&
  seen.amount === record.amount && seen.validBefore === record.validBefore;

/** What the server's answer means for the record. `undefined` ends it: nothing moved and nothing can move. */
export function followReturn(record: ReturnRecord, seen: WalletReturn | undefined, now: number): ReturnRecord | undefined {
  if (seen && sameReturn(record, seen)) {
    if (seen.state === 'failed') return undefined;
    return {...record, stage: seen.state === 'final' ? 'final' : 'sent', txHash: seen.txHash ?? record.txHash};
  }
  // The request ends after 30 seconds and the authorization at validBefore, so neither can arrive later.
  return now > record.validBefore + 120 ? undefined : record;
}

/** Reads what the server holds for a sent return. A record that moved on meanwhile is left alone. */
export async function settleReturn(storage: Storage, client: CommerceClient, now: () => number) {
  const records = returnRecord(storage), current = await records.load();
  if (current?.stage !== 'sent' || atOtherStore(current, client.profile?.branchRef)) return current;
  const next = followReturn(current, await client.walletReturn(), now());
  const latest = await records.load();
  if (JSON.stringify(latest) !== JSON.stringify(current)) return latest;
  if (JSON.stringify(next) !== JSON.stringify(current)) await records.save(next);
  return next;
}

/** Why a board key must stay for now, and what could not be found out. */
export type ResetBlock = {orders: number; returning: boolean; /** Order or return records on this phone. */ unreadable: boolean; server: 'ok' | 'none' | 'failed';
  /** Money of a top-up is still on its way to this account (topUpUnfinished). */ toppingUp?: boolean;
  /** The server was not asked because no board of the account answered to prove it (a build without a registration). */ boardAway?: boolean};
export const resetBlocked = (block: ResetBlock) => block.orders > 0 || block.returning || !!block.toppingUp;

/**
 * Whether money of a top-up is still on its way to this board account. A reset before it arrives would send it to
 * an account whose key is gone, so the reset and return pages wait for this the way they wait for an open order.
 * The app keeps no top-up state yet (traveller-exchange.md), so the answer is no. The top-up flow answers here once
 * it exists; nothing else needs to change for the pages to hold a reset back.
 */
export async function topUpUnfinished(_storage: Storage, _account: OrderAccount): Promise<boolean> {
  return false;
}
export const resetUnknown = (block: ResetBlock) => block.unreadable || block.server === 'failed';

const openEscrow = (order: CommerceOrder, account: OrderAccount) => order.escrow ?
  order.escrow.payer === account.payer && isOpenOrder({stage: order.escrow.stage} as OpenOrder) : order.state === 'checking';
// More unknown orders than this are not asked one by one; the answer is then "not known".
const asked = 60;

/**
 * Asks this phone's records and the server. The records hold every order this phone paid with the account, also
 * those made from a store page without a registration; but a phone that was set up again, or another phone, holds
 * none of them, so the records alone never say "no order is open".
 *
 * With a store credential the server is asked through it (`connect`). A build that orders without a registration
 * has none; it asks the customer entrance how many orders of the account are unfinished (`askEntrance`, the board's
 * account proof and the number alone). The number held back is the larger of what the phone knows and what the
 * server counts: the same order is on both sides, so they are not added. When the entrance could not be asked the
 * answer is "not known" (`server: 'failed'`), which the pages treat as they do on the earlier route.
 */
export async function readResetBlock(storage: Storage, records: {list(): Promise<OpenOrder[]>}, account: OrderAccount,
  connect: () => Promise<CommerceClient | undefined>, toppingUp: typeof topUpUnfinished = topUpUnfinished,
  askEntrance?: () => Promise<number>): Promise<ResetBlock> {
  const block: ResetBlock = {orders: 0, returning: false, unreadable: false, server: 'none'};
  try {
    if (await toppingUp(storage, account)) block.toppingUp = true;
  } catch { block.unreadable = true; }
  let known: OpenOrder[] = [];
  const open = new Set<string>();
  try {
    known = await records.list();
    for (const order of known)
      if (isOpenOrder(order) && order.payer === account.payer && order.generation === account.generation) open.add(order.sale);
  } catch { block.unreadable = true; }
  try {
    block.returning = ['signed', 'sent'].includes((await returnRecord(storage).load())?.stage ?? '');
  } catch { block.unreadable = true; }
  try {
    const client = await connect();
    if (client) {
      const held = await client.walletReturn();
      if (held && held.payer === account.payer && (held.state === 'queued' || held.state === 'submitted')) block.returning = true;
      const ended = new Set(known.filter(order => !isOpenOrder(order)).map(order => order.sale));
      const all = [];
      for (let page = await client.orders(), count = 1; ; page = await client.orders(page[page.length - 1].id), count++) {
        all.push(...page);
        if (page.length < 50) break;
        if (count === 20) throw new Error('TOO_MANY_ORDERS');
      }
      const rows = all.filter(row => ['checking', 'paid'].includes(row.state) && !ended.has(row.id) && !open.has(row.id));
      for (const row of rows.slice(0, asked)) if (openEscrow(await client.status(row.id), account)) open.add(row.id);
      block.server = rows.length > asked ? 'failed' : 'ok';
    }
  } catch { block.server = 'failed'; }
  block.orders = open.size;
  if (askEntrance && block.server === 'none') {
    try {
      block.orders = Math.max(open.size, await askEntrance());
      block.server = 'ok';
    } catch (error) {
      block.server = 'failed';
      if (error instanceof AccountAskError && error.reason === 'no-board') block.boardAway = true;
    }
  }
  return block;
}

export const returnRefusals = ['CHECKOUT_CONFLICT', 'MISMATCHED_EVIDENCE', 'EXPIRED', 'BAD_REQUEST', 'NOT_AUTHORIZED', 'UNSUPPORTED_CAPABILITY', 'REPLAY_CONFLICT', 'BUSY'];

/** Sends the signed transfer once it is written down, and reads what the server holds. */
export async function submitReturn(client: CommerceClient, record: ReturnRecord, save: (next?: ReturnRecord) => Promise<void>, now: number, tokenName = orderTokenName) {
  if (!record.signature || verifyReturn(record, record.signature, tokenName).signature !== record.signature) throw new Error('RETURN_SIGNATURE');
  if (record.validBefore <= now) throw new Error('RETURN_EXPIRED');
  if (record.stage === 'sent' && atOtherStore(record, client.profile?.branchRef)) throw new Error('SENT_AT_OTHER_STORE');
  const first = record.sentAt === undefined;
  const sent: ReturnRecord = {...record, stage: 'sent', sentAt: now, ...(client.profile ? {branch: client.profile.branchRef} : {})};
  await save(sent);
  try {
    const {payer, to, amount, validBefore, nonce} = sent;
    await client.returnWallet({payer, to, amount, validBefore, nonce}, record.signature, record.key);
    return sent;
  } catch (error) {
    const reason = error instanceof Error ? error.message : '';
    if (!first || !(returnRefusals.includes(reason) || (error as {code?: string}).code === 'COMMERCE_REFUSED')) return sent;
    // Refused on the first try: the server holds nothing, unless its answer only came back late.
    const seen = await client.walletReturn().catch(() => null);
    if (seen === null) return sent;
    if (seen && sameReturn(record, seen)) return followReturn(sent, seen, now) ?? sent;
    await save({...record, sentAt: undefined});
    throw error;
  }
}

/** NU54-RESET1: what the operator signs for a board whose PIN is forgotten (nu54_spend_operator_statement). */
export type ResetRequest = {device: string; generation: bigint; nonce: string};
export function resetRequestText({device, generation, nonce}: ResetRequest) {
  if (!hash.test(device) || !hash.test(nonce) || generation < 1n || generation >= 2n ** 64n) throw new Error('RESET_REQUEST');
  return `NU54-RESET1:${device}:${generation}:${nonce}`;
}
/** The board keeps a request for 15 minutes; the phone stops a minute earlier. */
export const resetLifetime = 840;
export type PendingReset = ResetRequest & {until: number};
const resetName = `customer-order-${customerDigest('nu54-operator-reset-v1')}`;
export function pendingReset(storage: Storage) {
  return {
    async load(): Promise<PendingReset | undefined> {
      const raw = await storage.loadJournal(resetName);
      const value = raw ? JSON.parse(raw) as {device: string; generation: string; nonce: string; until: number} | null : null;
      if (!value) return undefined;
      if (!/^[1-9][0-9]{0,19}$/.test(value.generation) || !Number.isSafeInteger(value.until)) throw new Error('RESET_REQUEST');
      const request = {device: value.device, generation: BigInt(value.generation), nonce: value.nonce, until: value.until};
      resetRequestText(request);
      return request;
    },
    async save(value?: PendingReset) {
      await storage.saveJournal(resetName, JSON.stringify(value ? {...value, generation: value.generation.toString()} : null));
    },
  };
}
export function readResetAnswer(raw: string, nonce: string) {
  const match = /^NU54-RESET1-OK:([0-9a-f]{64}):([0-9a-f]{128})$/.exec(raw.trim());
  if (!match) throw new Error('RESET_ANSWER');
  if (match[1] !== nonce) throw new Error('RESET_ANSWER_OTHER');
  return match[2];
}

/** Text for a failed step on the board. */
export function boardFailure(error: unknown): string {
  const reason = error instanceof Error ? error.message : '';
  return ({
    BOARD_PERMIT: '보드가 연결을 받지 않습니다. 운영자 허가가 필요합니다. 보드를 켤 때마다 운영자가 허가를 넣어야 합니다.',
    BOARD_REFUSED: '보드에서 거절했습니다. 바뀐 것은 없습니다.',
    BOARD_ORDER_OPEN: '보드가 방금 승인한 결제의 기한이 끝나기를 기다립니다. 최대 4분 뒤에 다시 시도하세요.',
    BOARD_SESSION_SHORT: '보드 연결 시간이 거의 끝났습니다. 다시 연결하세요.',
    ORDER_PIN_WAIT: 'PIN을 여러 번 틀려 보드가 입력을 막았습니다. 내 보드에서 남은 시간을 확인하세요.',
    BOARD_WAS_RESET: '보드가 초기화되었거나 다시 등록되었습니다. 이전 주소로 보내지 말고 설정을 다시 하세요.',
    BOARD_NOT_REGISTERED: '이 보드에는 아직 이 휴대전화가 등록되어 있지 않습니다. 보드 연결에서 먼저 등록하세요.',
    ACCOUNT_CHANGED: '보드 계정이 이 휴대전화에 저장된 계정과 다릅니다. 내 보드가 맞는지 확인하세요.',
    ACCOUNT_PROOF: '보드의 서명이 계정 주소와 맞지 않습니다. 이 보드로 입금하지 마세요.',
    PAYMENT_BOARD_BINDING: '이 휴대전화에 등록된 보드가 아니거나 등록이 바뀌었습니다.',
    RETURN_ADDRESS_SET: '보드에 다른 반납 주소가 이미 있습니다. 바꾸려면 초기화해야 합니다.',
    RETURN_ADDRESS: '반납 주소가 보드에 저장된 주소와 다릅니다.',
    RETURN_NOT_TAKEN: '보드가 반납 요청을 받지 않았습니다. 보드의 시각이 휴대전화와 몇 분 넘게 다르면 이렇게 됩니다. 운영자에게 새 허가를 받은 뒤 다시 시도하세요. 빠져나간 돈은 없습니다.',
    RETURN_SIGNATURE: '보드의 반납 서명을 확인할 수 없어 보내지 않았습니다.',
  } as Record<string, string>)[reason] ?? '보드의 답을 받지 못했습니다. 다시 연결해 상태를 확인하세요.';
}

export function serverFailure(error: unknown): string {
  const reason = error instanceof Error ? error.message : '';
  return ({
    CHECKOUT_CONFLICT: '끝나지 않은 주문이나 진행 중인 반납이 있어 서버가 받지 않았습니다. 주문을 먼저 끝내세요.',
    NOT_AUTHORIZED: '이 휴대전화의 주문 등록으로는 할 수 없습니다. 매장 담당자에게 새 등록서를 받으세요.',
    EXPIRED: '요청 시간이 지났습니다. 휴대전화 시각이 맞는지 확인하고 다시 시도하세요.',
    MISMATCHED_EVIDENCE: '이 매장에 등록된 보드 계정이 아니거나 서버가 서명을 받아들이지 않았습니다. 설정에서 매장 등록을 다시 하세요.',
    REPLAY_CONFLICT: '이미 진행 중인 반납이 있습니다. 그 결과를 기다리세요.',
    BUSY: '오늘 반납을 세 번 요청했거나 서버가 바쁩니다. 나중에 다시 시도하세요.',
    BAD_REQUEST: '서버가 요청을 받지 않았습니다. 휴대전화 시각이 맞는지 확인하고 다시 시도하세요.',
    UNSUPPORTED_CAPABILITY: '이 매장은 지금 이 요청을 처리할 수 없습니다. 매장이나 운영자에게 문의하세요.',
    RETURN_EXPIRED: '반납 서명의 기한이 지났습니다. 처음부터 다시 시도하세요. 빠져나간 돈은 없습니다.',
  } as Record<string, string>)[reason] ?? '서버의 답을 받지 못했습니다. 인터넷 연결을 확인하고 다시 시도하세요.';
}

