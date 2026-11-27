import {bytesToHex, CommerceClient, hexToBytes, type CommerceProfile} from '@nu54/protocol';
import {EntranceError, type BoardLink, type EntrancePost, type PickupBoard} from './customerEntrance.ts';

// Customer sessions of the entrance (traveller-services.md chapter 9 item 2). A phone registers at no store: before
// payment the board proves its account once and gets a session for that store's menu, cart, quote and payment; after
// payment it proves the paying account once and gets a session for that one order until the order ends. A session
// is a bearer token for C3 requests. It never hands over goods: a pickup QR needs a new proof each time.

export type CustomerSession = {
  token: string; purpose: 'checkout' | 'order'; branch: string; sale?: string;
  tenant: string; subject: string; expiresAt: number;
};
/** What the session is for: a store before payment, or one paid order of that store. */
export type SessionScope = {purpose: 'checkout'; branch: string} | {purpose: 'order'; branch: string; sale: string};

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const hex32 = /^[0-9a-f]{64}$/;
const account = /^0x[0-9a-f]{40}$/;
const signature = /^[0-9a-f]{130}$/;
const generation = /^[1-9][0-9]{0,17}$/;

function refusal(reply: {status: number; body: unknown}): EntranceError {
  const code = (reply.body as {error?: unknown} | null)?.error;
  if (reply.status === 400 && code === 'MISMATCHED_EVIDENCE') return new EntranceError('MISMATCH');
  if (reply.status === 409 && code === 'CHECKOUT_CONFLICT') return new EntranceError('NOT_READY');
  if (reply.status === 400 && code === 'EXPIRED') return new EntranceError('EXPIRED');
  if (reply.status === 429 || (reply.status === 503 && code === 'BUSY')) return new EntranceError('BUSY');
  if (reply.status === 400 || reply.status === 404) return new EntranceError('REFUSED');
  return new EntranceError('UNAVAILABLE');
}

export class SessionEntrance {
  constructor(private readonly post: EntrancePost) {}

  /** A one-time value for the board to sign, bound to the store and, after payment, to the order. */
  async challenge(scope: SessionScope): Promise<Uint8Array> {
    if (!uuid.test(scope.branch) || (scope.purpose === 'order' && !uuid.test(scope.sale))) throw new EntranceError('REFUSED');
    const reply = await this.post('/v2/customer/challenge', JSON.stringify({purpose: scope.purpose, branchRef: scope.branch,
      ...(scope.purpose === 'order' ? {saleRef: scope.sale} : {})}));
    if (reply.status !== 200) throw refusal(reply);
    const body = reply.body as {challenge?: unknown} | null;
    if (typeof body?.challenge !== 'string' || !hex32.test(body.challenge) || /^0+$/.test(body.challenge)) throw new EntranceError('UNAVAILABLE');
    return hexToBytes(body.challenge);
  }

  /**
   * The board's proof over the challenge. A checkout proof carries the board's generation, which the quote is issued
   * for. The answer must be a session of the same purpose, store and order.
   */
  async prove(challenge: Uint8Array, proof: {account: string; signature: string}, boardGeneration: string, scope: SessionScope): Promise<CustomerSession> {
    if (challenge.length !== 32 || !account.test(proof.account) || !signature.test(proof.signature) ||
        (scope.purpose === 'checkout' && !generation.test(boardGeneration))) throw new EntranceError('REFUSED');
    const reply = await this.post('/v2/customer/proof', JSON.stringify({challenge: bytesToHex(challenge, false), account: proof.account,
      signature: proof.signature, ...(scope.purpose === 'checkout' ? {generation: boardGeneration} : {})}));
    if (reply.status !== 200) throw refusal(reply);
    const body = reply.body as Record<string, unknown> | null;
    const sale = scope.purpose === 'order' ? scope.sale : undefined;
    if (body?.purpose !== scope.purpose || body.branchRef !== scope.branch || body.saleRef !== sale || body.pickupToken !== undefined ||
        typeof body.session !== 'string' || !hex32.test(body.session) || typeof body.tenantRef !== 'string' || !uuid.test(body.tenantRef) ||
        typeof body.subjectRef !== 'string' || !uuid.test(body.subjectRef) || !Number.isSafeInteger(body.expiresAt))
      throw new EntranceError('UNAVAILABLE');
    return {token: body.session, purpose: scope.purpose, branch: scope.branch, ...(sale ? {sale} : {}),
      tenant: body.tenantRef, subject: body.subjectRef, expiresAt: body.expiresAt as number};
  }
}

export type SessionProblem =
  | {kind: 'no-board'}
  | {kind: 'other-board'; expected: string; board: string}
  /** The account cannot start here now: its board changed while an order of it is still open. */
  | {kind: 'held'}
  | {kind: 'busy'}
  | {kind: 'unavailable'};
export type OpenedSession = {kind: 'session'; session: CustomerSession} | SessionProblem;

/**
 * One session: open the board of `expected`, take a challenge, let the board prove its account, and trade the proof
 * for the session. The board needs no button for this. The server decides; the comparison here only says sooner
 * which board is near.
 */
export async function openCustomerSession(entrance: SessionEntrance | undefined, board: PickupBoard, expected: string,
  scope: SessionScope): Promise<OpenedSession> {
  if (!entrance) return {kind: 'unavailable'};
  let link: BoardLink;
  try {
    link = await board.open(expected);
  } catch {
    return {kind: 'no-board'};
  }
  try {
    if (link.account !== expected) return {kind: 'other-board', expected, board: link.account};
    // A challenge that ran out before the board answered is asked once more.
    for (let attempt = 0; ; attempt++) {
      const challenge = await entrance.challenge(scope);
      let proof;
      try {
        proof = await link.prove(challenge);
      } catch {
        return {kind: 'no-board'};
      }
      if (proof.account !== expected) return {kind: 'other-board', expected, board: proof.account};
      try {
        return {kind: 'session', session: await entrance.prove(challenge, proof, link.generation, scope)};
      } catch (error) {
        if (error instanceof EntranceError && error.code === 'EXPIRED' && attempt === 0) continue;
        throw error;
      }
    }
  } catch (error) {
    const code = error instanceof EntranceError ? error.code : 'UNAVAILABLE';
    if (code === 'MISMATCH') return {kind: 'other-board', expected, board: link.account};
    if (code === 'NOT_READY') return {kind: 'held'};
    if (code === 'BUSY') return {kind: 'busy'};
    return {kind: 'unavailable'};
  } finally {
    link.close();
  }
}

/** The session is over, or the request is outside what it covers. The caller proves the board again. */
export class SessionEnded extends Error {
  readonly code = 'CUSTOMER_SESSION_ENDED';
  constructor() { super('CUSTOMER_SESSION_ENDED'); }
}

/**
 * The C3 client of one session: the same requests as an enrolled customer connection, sent to the entrance with the
 * session token. Its scope comes from the server's answer to the proof, never from the phone.
 */
export function sessionClient(post: EntrancePost, session: CustomerSession, newId: () => Promise<string>, clock?: () => number): CommerceClient {
  const profile: CommerceProfile = {role: 'customer', branchRef: session.branch, tenantRef: session.tenant, subjectRef: session.subject,
    expiresAt: session.expiresAt};
  return new CommerceClient(profile, async body => {
    const reply = await post('/v2/customer/commerce', body, session.token);
    if (reply.status === 404) throw new SessionEnded();
    if (reply.status === 429) throw new Error('BUSY');
    if (reply.body === null || typeof reply.body !== 'object') throw new Error('BAD_RESPONSE');
    return JSON.stringify(reply.body);
  }, newId, clock);
}
