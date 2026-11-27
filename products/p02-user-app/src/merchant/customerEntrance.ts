import {bytesToHex, encodePickupQr, hexToBytes} from '@nu54/protocol';
import {publicOrigin} from '../travel/catalog.ts';
import type {OpenOrder} from './orderRecords.ts';
import {approvedCustomerEntranceOrigin} from './customerEntranceDeployment.ts';

// The customer entrance (opsd customer_entrance.go): a one-time challenge, the paying board's account proof over it,
// and for a ready order a pickup token that lives about 60 seconds. No client certificate and no enrollment.

export type EntranceReply = {status: number; body: unknown};
export type EntrancePost = (path: '/v2/customer/challenge' | '/v2/customer/proof', body: string) => Promise<EntranceReply>;

/** Codes the screens act on. UNAVAILABLE: no entrance address, no answer, or an answer that is not the protocol. */
export type EntranceCode = 'MISMATCH' | 'NOT_READY' | 'EXPIRED' | 'BUSY' | 'REFUSED' | 'UNAVAILABLE';
export class EntranceError extends Error {
  constructor(readonly code: EntranceCode) { super(`CUSTOMER_ENTRANCE_${code}`); }
}

/** Posts JSON to the entrance origin; a redirect or a reply from another address counts as no answer. */
export function fetchEntrance(origin: string, send: typeof fetch = fetch): EntrancePost {
  let base: string;
  try { base = publicOrigin(origin); } catch { return async () => { throw new EntranceError('UNAVAILABLE'); }; }
  return async (path, body) => {
    const url = base + path;
    let response: Response;
    try {
      response = await send(url, {method: 'POST', credentials: 'omit', redirect: 'error',
        headers: {'Content-Type': 'application/json', 'Cache-Control': 'no-store'}, body});
    } catch {
      throw new EntranceError('UNAVAILABLE');
    }
    if (response.url && response.url !== url) throw new EntranceError('UNAVAILABLE');
    return {status: response.status, body: await response.json().catch(() => null)};
  };
}

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const hex32 = /^[0-9a-f]{64}$/;
const account = /^0x[0-9a-f]{40}$/;
const signature = /^[0-9a-f]{130}$/;

function refusal(reply: EntranceReply): EntranceError {
  const code = (reply.body as {error?: unknown} | null)?.error;
  if (reply.status === 400 && code === 'MISMATCHED_EVIDENCE') return new EntranceError('MISMATCH');
  if (reply.status === 409 && code === 'CHECKOUT_CONFLICT') return new EntranceError('NOT_READY');
  if (reply.status === 400 && code === 'EXPIRED') return new EntranceError('EXPIRED');
  if (reply.status === 429 || (reply.status === 503 && code === 'BUSY')) return new EntranceError('BUSY');
  if (reply.status === 400 || reply.status === 404) return new EntranceError('REFUSED');
  return new EntranceError('UNAVAILABLE');
}

export type PickupChallenge = {challenge: Uint8Array; expiresAt: number};
export type PickupToken = {token: string; expiresAt: number};

export class CustomerEntrance {
  constructor(private readonly post: EntrancePost) {}

  /** A challenge for handing over this ready order, which only the board that paid it can answer. */
  async pickupChallenge(branch: string, sale: string): Promise<PickupChallenge> {
    if (!uuid.test(branch) || !uuid.test(sale)) throw new EntranceError('REFUSED');
    const reply = await this.post('/v2/customer/challenge', JSON.stringify({purpose: 'pickup', branchRef: branch, saleRef: sale}));
    if (reply.status !== 200) throw refusal(reply);
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
    if (reply.status !== 200) throw refusal(reply);
    const body = reply.body as {purpose?: unknown; branchRef?: unknown; saleRef?: unknown; pickupToken?: unknown; expiresAt?: unknown} | null;
    if (body?.purpose !== 'pickup' || body.branchRef !== order.branch || body.saleRef !== order.sale ||
        typeof body.pickupToken !== 'string' || !hex32.test(body.pickupToken) || !Number.isSafeInteger(body.expiresAt))
      throw new EntranceError('UNAVAILABLE');
    return {token: body.pickupToken, expiresAt: body.expiresAt as number};
  }
}

/** A board that answered: its spending account, read without a button, and the account proof over a challenge. */
export type BoardLink = {account: string; prove(challenge: Uint8Array): Promise<{account: string; signature: string}>; close(): void};
/** Opens the board near the phone, preferring the one whose account is `expected`. Throws when none answers. */
export type PickupBoard = {open(expected: string): Promise<BoardLink>};

export type PickupProblem =
  | {kind: 'no-board'}
  | {kind: 'other-board'; payer: string; board: string}
  | {kind: 'not-ready'}
  | {kind: 'busy'}
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
    const code = error instanceof EntranceError ? error.code : 'UNAVAILABLE';
    if (code === 'MISMATCH') return {kind: 'other-board', payer: order.payer, board: link.account};
    if (code === 'NOT_READY') return {kind: 'not-ready'};
    if (code === 'BUSY') return {kind: 'busy'};
    return {kind: 'unavailable'};
  } finally {
    link.close();
  }
}

/** The entrance of this build, or undefined while no address is bound (customerEntranceDeployment.ts). */
export const configuredEntrance = (origin = approvedCustomerEntranceOrigin) =>
  origin ? new CustomerEntrance(fetchEntrance(origin)) : undefined;
