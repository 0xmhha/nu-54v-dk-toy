import {bytesToHex, encodePickupQr, hexToBytes} from '@nu54/protocol';
import Http from '../specs/NativeCommerceHttp.ts';
import {publicOrigin} from '../travel/catalog.ts';
import type {OpenOrder} from './orderRecords.ts';
import {approvedCustomerEntrance, type CustomerEntranceAnchor} from './customerEntranceDeployment.ts';

// The customer entrance (opsd customer_entrance.go): a one-time challenge, the paying board's account proof over it,
// and for a ready order a pickup token that lives about 60 seconds. No client certificate and no enrollment: the
// server is trusted by the anchor of this build alone.

/** `retryAfter`: the seconds the server asked to wait before the next request (its Retry-After), when it said so. */
export type EntranceReply = {status: number; body: unknown; retryAfter?: number};
export type EntrancePath = '/v2/customer/challenge' | '/v2/customer/proof' | '/v2/customer/commerce' | '/v2/customer/checkout/cancel'
  | '/v2/customer/stores' | `/v2/customer/stores?after=${string}` | `/v2/customer/stores/${string}`;
/**
 * One request to the entrance. An empty body is a read; `session` goes only with `/v2/customer/commerce` and
 * `/v2/customer/checkout/cancel`.
 */
export type EntrancePost = (path: EntrancePath, body: string, session?: string) => Promise<EntranceReply>;

/**
 * Codes the screens act on. MISMATCH: the server did not accept the account proof for what was asked. COLLECTED: the
 * store already recorded the handover. BUSY: the server has too many requests now and nothing else is known; the
 * same request is sent again later. REFUSED: the server answered and did not take the request. UNAVAILABLE: no
 * entrance address, no answer, or an answer that is not the protocol.
 */
export type EntranceCode = 'MISMATCH' | 'NOT_READY' | 'COLLECTED' | 'EXPIRED' | 'BUSY' | 'REFUSED' | 'UNAVAILABLE';
export class EntranceError extends Error {
  /**
   * `at`: when the store recorded the handover, in seconds, when the server said so with COLLECTED. `retryAfter`: the
   * seconds the server asked to wait, with BUSY.
   */
  constructor(readonly code: EntranceCode, readonly at?: number, readonly retryAfter?: number) { super(`CUSTOMER_ENTRANCE_${code}`); }
}

/** The native half of the entrance connection (specs/NativeCommerceHttp.ts). */
export type EntranceTransport = {customerRequest?: (anchor: string, path: string, session: string, body: string) => Promise<string>};

/** Whether an answer is the server's "too many requests now": 429, or 503 with BUSY. It says nothing else. */
export function entranceBusy(reply: EntranceReply): boolean {
  return reply.status === 429 || (reply.status === 503 && (reply.body as {error?: unknown} | null)?.error === 'BUSY');
}

/** The longest wait a server's Retry-After is taken for, in seconds. The entrance asks for ten at most. */
export const longestWait = 120;

/**
 * The requests the phone waits for together (opsd customer_limits.go): a wait asked of one kind does not hold another.
 * The server counts a challenge and a proof apart (a challenge for its address; a proof for its account, and a
 * session start for its address too), but for the app they are one step: a challenge is of no use while its proof
 * would be told to wait, so a wait asked of either holds both.
 */
function waitKind(path: string): string {
  if (path === '/v2/customer/commerce' || path === '/v2/customer/checkout/cancel') return 'session';
  if (path === '/v2/customer/proof' || path === '/v2/customer/challenge') return 'proof';
  return /\/photos\//.test(path) ? 'photo' : 'read';
}

// When each kind of request may be sent to each server again (milliseconds). The phone waits out what the server asked
// for: until then a request of that kind is answered here as busy and is not sent.
const waits = new Map<string, number>();
/** For tests: forgets every wait. */
export function forgetEntranceWaits() { waits.clear(); }

/** The pinned keys of an anchor: one, or the current one and the next. Undefined when they are not that. */
export function entrancePins(pin: unknown): string[] | undefined {
  const pins = typeof pin === 'string' ? [pin] : Array.isArray(pin) ? pin : [];
  return pins.length >= 1 && pins.length <= 2 && new Set(pins).size === pins.length &&
    pins.every(value => typeof value === 'string' && /^[0-9a-f]{64}$/.test(value) && !/^0+$/.test(value)) ? pins as string[] : undefined;
}

/** The anchor as the native module takes it, or undefined when any part is not what a release may bind. */
export function entranceAnchor(anchor: CustomerEntranceAnchor | undefined): string | undefined {
  try {
    const pins = entrancePins(anchor?.pin);
    if (!anchor || !pins || !/^[A-Za-z0-9+/]{64,5460}={0,2}$/.test(anchor.ca) || anchor.ca.length % 4) return undefined;
    return JSON.stringify({origin: publicOrigin(anchor.origin), ca: anchor.ca, pin: typeof anchor.pin === 'string' ? anchor.pin : pins});
  } catch {
    return undefined;
  }
}

/**
 * Sends to the entrance through the native commerce module: TLS 1.3, no client certificate, and a server accepted
 * only by the CA and a key pin of this build (customerEntranceDeployment.ts). No enrollment profile and no system
 * certificate store take part. A build without an anchor or without the native method, or any native failure,
 * counts as no answer.
 */
export function nativeEntrance(anchor: CustomerEntranceAnchor | undefined, http: EntranceTransport | null | undefined = Http): EntrancePost {
  const trust = entranceAnchor(anchor);
  if (!trust) return async () => { throw new EntranceError('UNAVAILABLE'); };
  const origin = (JSON.parse(trust) as {origin: string}).origin;
  return async (path, body, session = '') => {
    const kind = `${origin} ${waitKind(path)}`;
    const left = (waits.get(kind) ?? 0) - Date.now();
    if (left > 0) throw new EntranceError('BUSY', undefined, Math.ceil(left / 1000));
    let reply: {status?: unknown; body?: unknown; retryAfter?: unknown} | null;
    try {
      if (typeof http?.customerRequest !== 'function') throw new Error('no native entrance');
      reply = JSON.parse(await http.customerRequest(trust, path, session, body));
    } catch {
      throw new EntranceError('UNAVAILABLE');
    }
    if (!Number.isSafeInteger(reply?.status) || typeof reply?.body !== 'string') throw new EntranceError('UNAVAILABLE');
    let answer: unknown = null;
    try { answer = JSON.parse(reply.body); } catch { /* not JSON, such as a plain 404 */ }
    const result: EntranceReply = {status: reply.status as number, body: answer};
    if (Number.isSafeInteger(reply.retryAfter) && (reply.retryAfter as number) > 0) result.retryAfter = Math.min(reply.retryAfter as number, longestWait);
    // A refusal for the rate alone holds the next request of its kind. One that names a reason (a cart that was not
    // held) is about that one request, and whoever sent it decides (storeAccess.ts).
    if (entranceBusy(result) && result.retryAfter !== undefined && (answer as {reason?: unknown} | null)?.reason === undefined)
      waits.set(kind, Date.now() + result.retryAfter * 1000);
    return result;
  };
}

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const hex32 = /^[0-9a-f]{64}$/;
const account = /^0x[0-9a-f]{40}$/;
const signature = /^[0-9a-f]{130}$/;

/** What an answer other than 200 to a challenge or a proof means. */
export function entranceRefusal(reply: EntranceReply): EntranceError {
  const body = reply.body as {error?: unknown; reason?: unknown; recordedAt?: unknown} | null;
  const code = body?.error;
  if (reply.status === 400 && code === 'MISMATCHED_EVIDENCE') return new EntranceError('MISMATCH');
  if (reply.status === 409 && code === 'CHECKOUT_CONFLICT') {
    // An order already handed over says so and when. A server that names no reason is an older one.
    if (body?.reason === 'collected')
      return new EntranceError('COLLECTED', Number.isSafeInteger(body.recordedAt) && (body.recordedAt as number) > 0 ? body.recordedAt as number : undefined);
    return new EntranceError('NOT_READY');
  }
  if (reply.status === 400 && code === 'EXPIRED') return new EntranceError('EXPIRED');
  if (entranceBusy(reply)) return new EntranceError('BUSY', undefined, reply.retryAfter);
  if (reply.status === 400 || reply.status === 404) return new EntranceError('REFUSED');
  return new EntranceError('UNAVAILABLE');
}

/**
 * Reads the answer to the proof of an `account` challenge (opsd customerProof): how many orders of the proven
 * account are unfinished, at any store of this server. The answer holds that number and nothing else; anything
 * else is not the protocol.
 */
export function readOpenOrders(reply: EntranceReply): number {
  if (reply.status !== 200) throw entranceRefusal(reply);
  const body = reply.body as Record<string, unknown> | null;
  if (!body || typeof body !== 'object' || Object.keys(body).length !== 2 || body.purpose !== 'account' ||
      !Number.isSafeInteger(body.openOrders) || (body.openOrders as number) < 0) throw new EntranceError('UNAVAILABLE');
  return body.openOrders as number;
}

export type PickupChallenge = {challenge: Uint8Array; expiresAt: number};
export type PickupToken = {token: string; expiresAt: number};

export class CustomerEntrance {
  constructor(private readonly post: EntrancePost) {}

  /** A challenge for handing over this ready order, which only the board that paid it can answer. */
  async pickupChallenge(branch: string, sale: string): Promise<PickupChallenge> {
    if (!uuid.test(branch) || !uuid.test(sale)) throw new EntranceError('REFUSED');
    const reply = await this.post('/v2/customer/challenge', JSON.stringify({purpose: 'pickup', branchRef: branch, saleRef: sale}));
    if (reply.status !== 200) throw entranceRefusal(reply);
    const body = reply.body as {challenge?: unknown; expiresAt?: unknown} | null;
    if (typeof body?.challenge !== 'string' || !hex32.test(body.challenge) || /^0+$/.test(body.challenge) ||
        !Number.isSafeInteger(body.expiresAt)) throw new EntranceError('UNAVAILABLE');
    return {challenge: hexToBytes(body.challenge), expiresAt: body.expiresAt as number};
  }

  /** The board's proof over the challenge; the answer must be a pickup token for the same branch and order. */
  async pickupToken(challenge: Uint8Array, proof: {account: string; signature: string}, order: {branch: string; sale: string}): Promise<PickupToken> {
    if (challenge.length !== 32 || !account.test(proof.account) || !signature.test(proof.signature)) throw new EntranceError('REFUSED');
    const reply = await this.post('/v2/customer/proof',
      JSON.stringify({challenge: bytesToHex(challenge, false), account: proof.account, signature: proof.signature}));
    if (reply.status !== 200) throw entranceRefusal(reply);
    const body = reply.body as {purpose?: unknown; branchRef?: unknown; saleRef?: unknown; pickupToken?: unknown; expiresAt?: unknown} | null;
    if (body?.purpose !== 'pickup' || body.branchRef !== order.branch || body.saleRef !== order.sale ||
        typeof body.pickupToken !== 'string' || !hex32.test(body.pickupToken) || !Number.isSafeInteger(body.expiresAt))
      throw new EntranceError('UNAVAILABLE');
    return {token: body.pickupToken, expiresAt: body.expiresAt as number};
  }

  /** A challenge about the account alone, at no store: only the board of that account can answer it. */
  async accountChallenge(): Promise<Uint8Array> {
    const reply = await this.post('/v2/customer/challenge', JSON.stringify({purpose: 'account'}));
    if (reply.status !== 200) throw entranceRefusal(reply);
    const body = reply.body as {challenge?: unknown} | null;
    if (typeof body?.challenge !== 'string' || !hex32.test(body.challenge) || /^0+$/.test(body.challenge)) throw new EntranceError('UNAVAILABLE');
    return hexToBytes(body.challenge);
  }

  /** The board's proof over an account challenge; the answer is the number of unfinished orders of that account. */
  async openOrders(challenge: Uint8Array, proof: {account: string; signature: string}): Promise<number> {
    if (challenge.length !== 32 || !account.test(proof.account) || !signature.test(proof.signature)) throw new EntranceError('REFUSED');
    return readOpenOrders(await this.post('/v2/customer/proof',
      JSON.stringify({challenge: bytesToHex(challenge, false), account: proof.account, signature: proof.signature})));
  }
}

/**
 * A board that answered: its spending account and generation, read without a button, and the account proof over a
 * challenge.
 */
export type BoardLink = {account: string; generation: string; prove(challenge: Uint8Array): Promise<{account: string; signature: string}>; close(): void};
/** Opens the board near the phone, preferring the one whose account is `expected`. Throws when none answers. */
export type PickupBoard = {open(expected: string): Promise<BoardLink>};

export type PickupProblem =
  | {kind: 'no-board'}
  /** The board near the phone has another account than the order's payer; nothing was asked of the server. */
  | {kind: 'other-board'; payer: string; board: string}
  /** The paying board proved its account and the server still did not accept it for this order. */
  | {kind: 'mismatch'}
  | {kind: 'not-ready'}
  /** The store already recorded the handover, at `at` (seconds) when the server said when. */
  | {kind: 'collected'; at?: number}
  | {kind: 'busy'}
  /** The server answered and did not take the request. */
  | {kind: 'refused'}
  /** No answer from the server. */
  | {kind: 'unavailable'};
export type IssuedPickup = {kind: 'qr'; qr: string; expiresAt: number} | PickupProblem;

/** `0x7a3c…91e2`: enough to tell two accounts apart on a screen. */
export const shortAccount = (value: string) => `${value.slice(0, 6)}…${value.slice(-4)}`;

/**
 * One pickup QR for a ready order: open the board, compare its account with the order's payer before the server is
 * asked, take a challenge, let the board prove its account, and trade the proof for a token. The server decides; the
 * comparison here only spares a challenge and says what is wrong sooner.
 */
export async function issuePickupQr(entrance: CustomerEntrance | undefined, board: PickupBoard,
  order: Pick<OpenOrder, 'branch' | 'sale' | 'payer'>): Promise<IssuedPickup> {
  if (!entrance) return {kind: 'unavailable'};
  let link: BoardLink;
  try {
    link = await board.open(order.payer);
  } catch {
    return {kind: 'no-board'};
  }
  try {
    if (link.account !== order.payer) return {kind: 'other-board', payer: order.payer, board: link.account};
    // A challenge that ran out before the board answered is asked once more.
    for (let attempt = 0; ; attempt++) {
      const {challenge} = await entrance.pickupChallenge(order.branch, order.sale);
      let proof;
      try {
        proof = await link.prove(challenge);
      } catch {
        return {kind: 'no-board'};
      }
      if (proof.account !== order.payer) return {kind: 'other-board', payer: order.payer, board: proof.account};
      try {
        const issued = await entrance.pickupToken(challenge, proof, order);
        return {kind: 'qr', qr: encodePickupQr(issued.token), expiresAt: issued.expiresAt};
      } catch (error) {
        if (error instanceof EntranceError && error.code === 'EXPIRED' && attempt === 0) continue;
        throw error;
      }
    }
  } catch (error) {
    if (!(error instanceof EntranceError)) return {kind: 'unavailable'};
    switch (error.code) {
      // The board near is the payer's (compared above), so this is not another board: the server does not hold
      // this order as one this account can take now.
      case 'MISMATCH': return {kind: 'mismatch'};
      case 'NOT_READY': return {kind: 'not-ready'};
      case 'COLLECTED': return {kind: 'collected', ...(error.at === undefined ? {} : {at: error.at})};
      case 'BUSY': return {kind: 'busy'};
      // A challenge that ran out twice is also an answer of the server, not a lost connection.
      case 'REFUSED': case 'EXPIRED': return {kind: 'refused'};
      default: return {kind: 'unavailable'};
    }
  } finally {
    link.close();
  }
}

/** Why the server could not be asked about an account: no board of it answered, or the server gave no usable answer. */
export class AccountAskError extends Error {
  constructor(readonly reason: 'no-board' | 'server') { super(`ACCOUNT_ASK_${reason === 'no-board' ? 'NO_BOARD' : 'SERVER'}`); }
}

/**
 * How many orders of `expected` are unfinished, as the store server holds them: the board of that account proves it
 * over a challenge, with no button and no PIN, and the server answers the number alone. A phone that was set up
 * again holds no record of the orders its board account paid; this is how it still learns of them. Throws
 * AccountAskError when the number could not be learnt: that is "not known", never "none".
 */
export async function accountOpenOrders(entrance: CustomerEntrance | undefined, board: PickupBoard, expected: string): Promise<number> {
  if (!entrance) throw new AccountAskError('server');
  let link: BoardLink;
  try {
    link = await board.open(expected);
  } catch {
    throw new AccountAskError('no-board');
  }
  try {
    if (link.account !== expected) throw new AccountAskError('no-board');
    for (let attempt = 0; ; attempt++) {
      const challenge = await entrance.accountChallenge();
      let proof;
      try {
        proof = await link.prove(challenge);
      } catch {
        throw new AccountAskError('no-board');
      }
      if (proof.account !== expected) throw new AccountAskError('no-board');
      try {
        return await entrance.openOrders(challenge, proof);
      } catch (error) {
        // A challenge that ran out before the board answered is asked once more.
        if (error instanceof EntranceError && error.code === 'EXPIRED' && attempt === 0) continue;
        throw error;
      }
    }
  } catch (error) {
    throw error instanceof AccountAskError ? error : new AccountAskError('server');
  } finally {
    link.close();
  }
}

/** The entrance of this build, or undefined while no trust anchor is bound (customerEntranceDeployment.ts). */
export const configuredEntrance = (anchor: CustomerEntranceAnchor | undefined = approvedCustomerEntrance) =>
  entranceAnchor(anchor) ? new CustomerEntrance(nativeEntrance(anchor)) : undefined;
