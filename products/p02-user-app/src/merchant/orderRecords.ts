// What the phone must keep for orders paid through OrderEscrow: one random seed per installation,
// the orders still bound to its keys, and the board requests that may already have an answer. All
// live in the Keystore-wrapped commerce journal, beside the payment checkpoint (customerJournal.ts).
import {mapHashToField} from '@noble/curves/abstract/modular';
import {secp256k1} from '@noble/curves/secp256k1';
import {hkdf} from '@noble/hashes/hkdf';
import {sha256} from '@noble/hashes/sha256';
import {
  addressOfPrivateKey, bytesToHex, fromBase64, hexToBytes, signOrderBuyerAction, utf8Encode,
  type CommerceOrderPayment, type OrderBuyerAction, type OrderEscrowStatus,
} from '@nu54/protocol';
import {customerDigest} from './customerJournal.ts';

type Storage = {
  loadJournal(name: string): Promise<string>;
  saveJournal(name: string, value: string): Promise<void>;
  randomBytes(size: number): Promise<string>;
};
/** The board account an order is paid from. Another account gets another phone key. */
export type OrderAccount = {payer: string; generation: string};
/** The buyer's signed answer to one delivery. It is kept until the order shows it or can no longer take it. */
export type OrderAnswer = {action: OrderBuyerAction; claimableAt: number; key: string; signature: string; sentAt?: number};
/** What the server last reported about the order. */
export type OrderProgress = Partial<Pick<OrderEscrowStatus, 'stage' | 'claimableAt' | 'resolveBy' | 'payout' | 'refund' | 'pendingAction'>> &
  {pickupNo?: number; pickup?: CommerceOrderPayment['pickupStage']};
export type OpenOrder = OrderAccount & OrderProgress & {
  sale: string; branch: string; customer: string; confirmer: string; store?: string;
  asset: string; amount: string; escrow: string; merchantId: string; orderKey: string; sentAt: number;
  notified?: true; answer?: OrderAnswer;
};

/**
 * One board request per quote and phone key. The board keeps its answer until the quote ends and
 * drops the link when the same order arrives under another request id (nu54_order_find).
 */
export type BoardRequest = {digest: string; confirmer: string; request: string; expires: number; answered?: 'refused' | 'approved'};

const seedName = `customer-order-${customerDigest('nu54-order-confirmer-seed-v1')}`;
const requestsName = `customer-order-${customerDigest('nu54-board-requests-v1')}`;
const ordersName = `customer-order-${customerDigest('nu54-open-orders-v1')}`;
const limit = 64;
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const address = /^0x[0-9a-f]{40}$/;
const hash = /^[0-9a-f]{64}$/;
const stages = ['received', 'submitted', 'locked', 'delivered', 'disputed', 'paid', 'refunded', 'failed'];
const settled = ['paid', 'refunded', 'failed'];
const fields = ['payer', 'generation', 'sale', 'branch', 'customer', 'confirmer', 'store', 'asset', 'amount', 'escrow', 'merchantId', 'orderKey', 'sentAt',
  'stage', 'claimableAt', 'resolveBy', 'payout', 'refund', 'pendingAction', 'pickupNo', 'pickup', 'notified', 'answer'];
const actions = ['receipt', 'dispute', 'withdraw'];

/** The buyer's key is still needed for this order. */
export const isOpenOrder = (order: OpenOrder) => !settled.includes(order.stage ?? '');
/** The store still has something to do: the order number, the goods, or the hand-over. */
export const awaitsStore = (order: OpenOrder) => isOpenOrder(order) &&
  (order.pickupNo === undefined || (order.stage === 'locked' && order.pickup !== 'unfulfillable'));

function validOrder(value: OpenOrder) {
  return !!value && Object.keys(value).every(key => fields.includes(key)) &&
    [value.sale, value.branch, value.customer].every(id => uuid.test(id)) &&
    address.test(value.payer) && /^[1-9][0-9]{0,19}$/.test(value.generation) && address.test(value.confirmer) && address.test(value.asset) && /^[1-9][0-9]{0,77}$/.test(value.amount) &&
    address.test(value.escrow) && hash.test(value.merchantId) && hash.test(value.orderKey) && Number.isSafeInteger(value.sentAt) && value.sentAt > 0 &&
    (value.store === undefined || (typeof value.store === 'string' && value.store.length > 0 && value.store.length <= 240)) &&
    (value.stage === undefined || stages.includes(value.stage)) &&
    (value.pickupNo === undefined || (Number.isSafeInteger(value.pickupNo) && value.pickupNo > 0)) &&
    [value.claimableAt, value.resolveBy].every(at => at === undefined || (Number.isSafeInteger(at) && at > 0)) &&
    (value.pickup === undefined || ['preparing', 'ready', 'collected', 'unfulfillable'].includes(value.pickup)) &&
    (value.payout === undefined || ['receipt', 'claim', 'arbiter', 'dispute-withdrawn'].includes(value.payout)) &&
    (value.refund === undefined || ['store', 'undelivered', 'arbiter', 'dispute-expired'].includes(value.refund)) &&
    (value.pendingAction === undefined || actions.includes(value.pendingAction)) &&
    (value.notified === undefined || value.notified === true) &&
    (value.answer === undefined || (actions.includes(value.answer.action) && Number.isSafeInteger(value.answer.claimableAt) &&
      value.answer.claimableAt > 0 && uuid.test(value.answer.key) && /^[0-9a-f]{128}(1b|1c)$/.test(value.answer.signature) &&
      (value.answer.sentAt === undefined || (Number.isSafeInteger(value.answer.sentAt) && value.answer.sentAt > 0))));
}

const validRequest = (value: BoardRequest) => !!value && hash.test(value.digest) && address.test(value.confirmer) &&
  /^[0-9a-f]{32}$/.test(value.request) && !/^0+$/.test(value.request) && Number.isSafeInteger(value.expires) && value.expires > 0 &&
  (value.answered === undefined || value.answered === 'refused' || value.answered === 'approved');

/** The order key of one board account. Keys of two accounts cannot be linked without the seed. */
function accountKey(seed: Uint8Array, {payer, generation}: OrderAccount) {
  if (!address.test(payer) || !/^[1-9][0-9]{0,19}$/.test(generation)) throw new Error('ORDER_ACCOUNT');
  return mapHashToField(hkdf(sha256, seed, utf8Encode('NU54-order-confirmer-v1'), utf8Encode(payer + '/' + generation), 48), secp256k1.CURVE.n);
}

// Screens share these records, so every change waits for the one before it.
let queue: Promise<unknown> = Promise.resolve();
const announcing = new Set<string>();
const watchers = new Set<() => void>();
/** Runs after any recorded order changed. */
export function onOrdersChanged(run: () => void) {
  watchers.add(run);
  return () => { watchers.delete(run); };
}
function inOrder<T>(step: () => Promise<T>): Promise<T> {
  const result = queue.then(step, step);
  queue = result.catch(() => undefined);
  return result;
}

export function orderRecords(storage: Storage) {
  async function loadSeed() {
    const raw = await storage.loadJournal(seedName).catch(() => { throw new Error('ORDER_KEY_UNREADABLE'); });
    if (!raw) return undefined;
    try {
      const {version, seed} = JSON.parse(raw) as {version: number; seed: string};
      if (version !== 1 || !hash.test(seed) || /^0+$/.test(seed)) throw new Error();
      return hexToBytes(seed);
    } catch {
      throw new Error('ORDER_KEY_UNREADABLE');
    }
  }
  async function loadOrders(): Promise<OpenOrder[]> {
    const raw = await storage.loadJournal(ordersName);
    if (!raw) return [];
    const rows = JSON.parse(raw) as OpenOrder[];
    if (!Array.isArray(rows) || rows.length > limit || !rows.every(validOrder)) throw new Error('ORDER_RECORDS');
    return rows;
  }
  const saveOrders = async (rows: OpenOrder[]) => {
    await storage.saveJournal(ordersName, JSON.stringify(rows));
    for (const run of watchers) run();
  };
  /** Changes one order in place; undefined when there is no such order. */
  async function change(sale: string, edit: (order: OpenOrder) => OpenOrder) {
    const rows = await loadOrders();
    const index = rows.findIndex(row => row.sale === sale);
    if (index < 0) return undefined;
    const next = edit(rows[index]);
    if (!validOrder(next)) throw new Error('ORDER_RECORDS');
    if (JSON.stringify(next) !== JSON.stringify(rows[index])) await saveOrders(rows.map((row, i) => i === index ? next : row));
    return next;
  }

  const keyAddress = (seed: Uint8Array, account: OrderAccount) => addressOfPrivateKey(accountKey(seed, account));

  /** The phone key address for a board account. `bound` adds what only the payment checkpoint knows. */
  const confirmer = (account: OrderAccount, bound?: string) => inOrder(async () => {
    const open = (await loadOrders()).filter(isOpenOrder);
    const existing = await loadSeed();
    if (existing) {
      if (open.some(order => keyAddress(existing, order) !== order.confirmer) ||
          (bound !== undefined && keyAddress(existing, account) !== bound)) throw new Error('ORDER_KEY_LOST');
      return keyAddress(existing, account);
    }
    // An unfinished order still needs the key it was signed with, so no new seed may replace it.
    if (open.length || bound !== undefined) throw new Error('ORDER_KEY_LOST');
    const seed = fromBase64(await storage.randomBytes(32));
    const created = keyAddress(seed, account);
    await storage.saveJournal(seedName, JSON.stringify({version: 1, seed: bytesToHex(seed, false)}));
    seed.fill(0);
    const kept = await loadSeed();
    if (!kept || keyAddress(kept, account) !== created) throw new Error('ORDER_KEY_UNREADABLE');
    return created;
  });

  async function loadRequests(): Promise<BoardRequest[]> {
    const raw = await storage.loadJournal(requestsName);
    if (!raw) return [];
    const rows = JSON.parse(raw) as BoardRequest[];
    if (!Array.isArray(rows) || rows.length > limit || !rows.every(validRequest)) throw new Error('ORDER_RECORDS');
    return rows;
  }

  /** The one request id for this quote, with the board's answer when it already gave one. */
  const claimRequest = (quote: Pick<BoardRequest, 'digest' | 'confirmer' | 'expires'>, now: number) => inOrder(async () => {
    const live = (await loadRequests()).filter(row => row.expires > now);
    const known = live.find(row => row.digest === quote.digest && row.confirmer === quote.confirmer);
    if (known) return {...known, reused: true};
    if (live.length >= limit) throw new Error('ORDER_RECORDS_FULL');
    const created = {...quote, request: bytesToHex(fromBase64(await storage.randomBytes(16)), false)};
    if (!validRequest(created)) throw new Error('ORDER_RECORDS');
    await storage.saveJournal(requestsName, JSON.stringify([...live, created]));
    return {...created, answered: undefined, reused: false};
  });

  const answerRequest = (request: string, answered: 'refused' | 'approved') => inOrder(async () => {
    const rows = await loadRequests();
    if (rows.some(row => row.request === request && row.answered !== answered))
      await storage.saveJournal(requestsName, JSON.stringify(rows.map(row => row.request === request ? {...row, answered} : row)));
  });

  const list = () => inOrder(loadOrders);

  /** Writes the order before its signature leaves the phone. */
  const keep = (order: OpenOrder) => inOrder(async () => {
    if (!validOrder(order)) throw new Error('ORDER_RECORDS');
    const rows = (await loadOrders()).filter(row => row.sale !== order.sale);
    // Finished orders stay as history; the oldest makes room for a new one.
    const spare = rows.map(isOpenOrder).lastIndexOf(false);
    if (rows.length >= limit && spare < 0) throw new Error('ORDER_RECORDS_FULL');
    if (rows.length >= limit) rows.splice(spare, 1);
    await saveOrders([order, ...rows]);
  });

  /** Takes over what the server reported. An answer ends when the order left the state it answers. */
  const update = (sale: string, progress: OrderProgress, now = Math.floor(Date.now() / 1000)) => inOrder(() => change(sale, order => {
    const next: OpenOrder = {...order, ...progress};
    const {answer} = next;
    if (answer && (next.stage !== (answer.action === 'withdraw' ? 'disputed' : 'delivered') || next.claimableAt !== answer.claimableAt ||
        (next.pendingAction !== undefined && next.pendingAction !== answer.action) ||
        // The contract takes no dispute after this; the margin covers a transaction already on its way.
        (answer.action === 'dispute' && next.pendingAction === undefined && now > answer.claimableAt + 120) ||
        // A request names its own end (30 seconds), so one the server does not hold by now never arrives.
        (answer.sentAt !== undefined && next.pendingAction === undefined && now > answer.sentAt + 120))) delete next.answer;
    return next;
  }));

  /** Signs one answer with the order's own key. While it is in flight only the same answer comes back. */
  const signAnswer = (sale: string, action: OrderBuyerAction, claimableAt: number, newKey: () => Promise<string>) => inOrder(async () => {
    const order = (await loadOrders()).find(row => row.sale === sale);
    if (!order) throw new Error('ORDER_RECORDS');
    if (order.answer) {
      if (order.answer.action !== action || order.answer.claimableAt !== claimableAt) throw new Error('ORDER_ANSWER_PENDING');
      return order.answer;
    }
    const seed = await loadSeed();
    const key = seed && accountKey(seed, order);
    if (!key || addressOfPrivateKey(key) !== order.confirmer) throw new Error('ORDER_KEY_LOST');
    const answer: OrderAnswer = {action, claimableAt, key: await newKey(), signature: signOrderBuyerAction(action, {...order, claimableAt}, key)};
    key.fill(0);
    await change(sale, row => ({...row, answer}));
    return answer;
  });
  const markAnswerSent = (sale: string, now: number) => inOrder(() => change(sale, row => row.answer ? {...row, answer: {...row.answer, sentAt: now}} : row));
  /** Only after the server turned the answer away and reported that it holds none. */
  const dropAnswer = (sale: string) => inOrder(() => change(sale, row => ({...row, answer: undefined})));

  /** Shows a ready order once. It counts as shown only when `show` succeeded, so a refusal is tried again. */
  async function announceReady(sale: string, show: () => Promise<boolean>) {
    if (announcing.has(sale)) return;
    announcing.add(sale);
    try {
      const order = (await list()).find(row => row.sale === sale);
      if (order?.stage === 'locked' && order.pickup === 'ready' && !order.notified && await show())
        await inOrder(() => change(sale, row => ({...row, notified: true})));
    } finally {
      announcing.delete(sale);
    }
  }

  /** Orders that still hold this board account's money. Returning or resetting the board must find none. */
  const openFor = ({payer, generation}: OrderAccount) => inOrder(async () =>
    (await loadOrders()).filter(order => isOpenOrder(order) && order.payer === payer && order.generation === generation));

  return {confirmer, claimRequest, answerRequest, list, keep, update, signAnswer, markAnswerSent, dropAnswer, announceReady, openFor};
}
