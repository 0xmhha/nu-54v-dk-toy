import {accountProofDigest, addressOfPrivateKey, bytesToHex, signDigest} from '@nu54/protocol';
import {EntranceError, type BoardLink, type EntrancePost, type PickupBoard} from '../src/merchant/customerEntrance';
import {openCustomerSession, sessionClient, SessionEnded, SessionEntrance, type CustomerSession, type SessionScope} from '../src/merchant/customerSession';

jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: null}));

const key = new Uint8Array(32).fill(7), otherKey = new Uint8Array(32).fill(9);
const payer = addressOfPrivateKey(key), stranger = addressOfPrivateKey(otherKey);
const branch = '0fbc54ef-2827-4ab5-bfcd-3a27611c3002', sale = '0fbc54ef-2827-4ab5-bfcd-3a27611c3001';
const tenant = '0fbc54ef-2827-4ab5-bfcd-3a27611c3005', subject = '0fbc54ef-2827-4ab5-bfcd-3a27611c3006';
const challengeHex = 'c1'.repeat(32), token = 'a7'.repeat(32);
const checkout: SessionScope = {purpose: 'checkout', branch}, paid: SessionScope = {purpose: 'order', branch, sale};
const granted = (scope: SessionScope) => ({status: 200, body: {purpose: scope.purpose, branchRef: branch,
  ...(scope.purpose === 'order' ? {saleRef: sale} : {}), tenantRef: tenant, subjectRef: subject, session: token, expiresAt: 1_764_000_900}});

/** A board that signs the account proof the way the firmware does, with `secret`'s account. */
function board(secret: Uint8Array | 'absent' | 'silent' = key) {
  const closed: string[] = [];
  const value: PickupBoard = {async open() {
    if (secret === 'absent') throw new Error('BOARD_NOT_FOUND');
    const account = addressOfPrivateKey(secret === 'silent' ? key : secret);
    const link: BoardLink = {account, generation: '3', close: () => { closed.push(account); }, prove: async challenge => {
      if (secret === 'silent') throw new Error('OWNER_DISCONNECTED');
      return {account, signature: bytesToHex(signDigest(accountProofDigest(account, challenge), secret), false)};
    }};
    return link;
  }};
  return Object.assign(value, {closed});
}
function entrance(proofs: {status: number; body: unknown}[]) {
  const calls: {path: string; body: Record<string, string>; session?: string}[] = [];
  const post: EntrancePost = async (path, body, session) => {
    calls.push({path, body: JSON.parse(body), ...(session ? {session} : {})});
    if (path === '/v2/customer/challenge') return {status: 200, body: {challenge: challengeHex, expiresAt: 1_764_000_120}};
    return proofs.shift() ?? {status: 503, body: {status: 'error', error: 'UNKNOWN'}};
  };
  return {client: new SessionEntrance(post), calls, post};
}
const session: CustomerSession = {token, purpose: 'checkout', branch, tenant, subject, expiresAt: 1_764_000_900};

describe('a customer session from the board\'s account proof', () => {
  test('before payment: a challenge for the store, the proof with the board generation, a session for that store', async () => {
    const {client, calls} = entrance([granted(checkout)]);
    const near = board();
    expect(await openCustomerSession(client, near, payer, checkout)).toEqual({kind: 'session', session});
    expect(calls[0]).toEqual({path: '/v2/customer/challenge', body: {purpose: 'checkout', branchRef: branch}});
    expect(calls[1].path).toBe('/v2/customer/proof');
    expect(calls[1].body).toMatchObject({challenge: challengeHex, account: payer, generation: '3'});
    expect(calls[1].body.signature).toMatch(/^[0-9a-f]{130}$/);
    expect(near.closed).toEqual([payer]);
  });

  test('after payment: a challenge for the order, no generation, a session for that order only', async () => {
    const {client, calls} = entrance([granted(paid)]);
    expect(await openCustomerSession(client, board(), payer, paid)).toEqual({kind: 'session', session: {...session, purpose: 'order', sale}});
    expect(calls[0].body).toEqual({purpose: 'order', branchRef: branch, saleRef: sale});
    expect(calls[1].body.generation).toBeUndefined();
  });

  test('no board, a silent board and another board are said before or without the server', async () => {
    const none = entrance([]);
    expect(await openCustomerSession(none.client, board('absent'), payer, checkout)).toEqual({kind: 'no-board'});
    expect(await openCustomerSession(none.client, board(otherKey), payer, checkout)).toEqual({kind: 'other-board', expected: payer, board: stranger});
    expect(none.calls).toEqual([]);
    const silent = board('silent');
    expect(await openCustomerSession(entrance([]).client, silent, payer, checkout)).toEqual({kind: 'no-board'});
    expect(silent.closed).toEqual([payer]);
    expect(await openCustomerSession(undefined, board(), payer, checkout)).toEqual({kind: 'unavailable'});
  });

  test('the server\'s refusals: another account, an account held by an open order, a limit, no answer', async () => {
    const answer = (status: number, error: string) => openCustomerSession(entrance([{status, body: {status: 'error', error}}]).client, board(), payer, checkout);
    expect(await answer(400, 'MISMATCHED_EVIDENCE')).toEqual({kind: 'other-board', expected: payer, board: payer});
    expect(await answer(409, 'CHECKOUT_CONFLICT')).toEqual({kind: 'held'});
    expect(await answer(429, 'BUSY')).toEqual({kind: 'busy'});
    expect(await answer(503, 'UNKNOWN')).toEqual({kind: 'unavailable'});
    expect(await answer(404, '')).toEqual({kind: 'unavailable'});
  });

  test('a challenge that ran out is asked once more, not forever', async () => {
    const expired = {status: 400, body: {status: 'error', error: 'EXPIRED'}};
    const twice = entrance([expired, granted(checkout)]);
    expect((await openCustomerSession(twice.client, board(), payer, checkout)).kind).toBe('session');
    expect(twice.calls.map(call => call.path)).toEqual(['/v2/customer/challenge', '/v2/customer/proof', '/v2/customer/challenge', '/v2/customer/proof']);
    expect(await openCustomerSession(entrance([expired, expired]).client, board(), payer, checkout)).toEqual({kind: 'unavailable'});
  });

  test('a session for another purpose, store or order, a pickup token, or a malformed one is not taken', async () => {
    const proof = {account: payer, signature: 'ab'.repeat(65)}, value = new Uint8Array(32).fill(1);
    const good = granted(paid).body;
    for (const body of [{...good, purpose: 'checkout'}, {...good, branchRef: sale}, {...good, saleRef: branch}, {...good, saleRef: undefined},
      {...good, session: token.toUpperCase()}, {...good, session: undefined, pickupToken: token}, {...good, pickupToken: token},
      {...good, tenantRef: 'x'}, {...good, subjectRef: undefined}, {...good, expiresAt: '1'}, null])
      await expect(entrance([{status: 200, body}]).client.prove(value, proof, '', paid)).rejects.toEqual(new EntranceError('UNAVAILABLE'));
    await expect(entrance([{status: 200, body: granted(paid).body}]).client.prove(value, proof, '3', checkout)).rejects.toEqual(new EntranceError('UNAVAILABLE'));
    // A checkout proof without the board generation, or a malformed scope, never leaves the phone.
    const quiet = entrance([]);
    await expect(quiet.client.prove(value, proof, '', checkout)).rejects.toEqual(new EntranceError('REFUSED'));
    await expect(quiet.client.prove(value, proof, '0', checkout)).rejects.toEqual(new EntranceError('REFUSED'));
    await expect(quiet.client.challenge({purpose: 'order', branch, sale: 'x'})).rejects.toEqual(new EntranceError('REFUSED'));
    expect(quiet.calls).toEqual([]);
  });
});

describe('C3 requests of a session', () => {
  const newId = async () => '0fbc54ef-2827-4ab5-bfcd-3a27611c3009';
  const now = () => 1_764_000_000;

  test('go to the entrance with the session token, scoped as the server\'s answer said', async () => {
    const sent: {path: string; body: {command: Record<string, unknown>}; token?: string}[] = [];
    const post: EntrancePost = async (path, body, bearer) => { sent.push({path, body: JSON.parse(body), token: bearer}); return {status: 200, body: {status: 'unknown'}}; };
    const client = sessionClient(post, session, newId, now);
    expect(client.profile).toEqual({role: 'customer', branchRef: branch, tenantRef: tenant, subjectRef: subject, expiresAt: 1_764_000_900});
    await expect(client.status(sale)).rejects.toThrow();
    expect(sent).toHaveLength(1);
    expect(sent[0].path).toBe('/v2/customer/commerce');
    expect(sent[0].token).toBe(token);
    expect(sent[0].body.command).toMatchObject({version: 2, contract: 'C3', operation: 'order.status', domainRef: branch, objectRef: sale, expiresAt: 1_764_000_030});
  });

  test('a session that ended, a limit and an answer outside the protocol are told apart', async () => {
    const failing = (status: number, body: unknown) => sessionClient(async () => ({status, body}), session, newId, now).status(sale);
    await expect(failing(404, null)).rejects.toBeInstanceOf(SessionEnded);
    await expect(failing(404, null)).rejects.toMatchObject({code: 'CUSTOMER_SESSION_ENDED'});
    await expect(failing(429, {status: 'error', error: 'BUSY'})).rejects.toThrow('BUSY');
    await expect(failing(503, null)).rejects.toThrow('BAD_RESPONSE');
    await expect(failing(200, 'ok')).rejects.toThrow('BAD_RESPONSE');
  });

  test('a request never outlives its session', async () => {
    const sent: {command: {expiresAt: number}}[] = [];
    const client = sessionClient(async (_path, body) => { sent.push(JSON.parse(body)); return {status: 404, body: null}; },
      {...session, expiresAt: 1_764_000_010}, newId, now);
    await expect(client.status(sale)).rejects.toBeInstanceOf(SessionEnded);
    expect(sent[0].command.expiresAt).toBe(1_764_000_010);
  });
});
