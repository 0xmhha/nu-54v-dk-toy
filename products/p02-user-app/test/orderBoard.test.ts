import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {sha256} from '@noble/hashes/sha256';
import {p256} from '@noble/curves/p256';
import {
  accountProofDigest, addressOfPrivateKey, bigToBytes, bytesToBig, bytesToHex, concat, encodeV2StreamFrame, equalBytes, fromBase64, hexToBytes, orderBoardRequest,
  orderLock, payerEnrollChallenge, quoteV2Layout as q, recoverSigner, walletReturnDigest, signDigest, toBase64, V2Journal, V2Link, V2StreamDecoder, type V2Request,
} from '@nu54/protocol';
import type {Spec as Ble} from '../src/specs/NativeRenterBle';
import type {Spec as Native} from '../src/specs/NativeOwnerWallet';
import {OwnerSession} from '../src/wallet/ownerSession';
import {readSpendingStatus} from '../src/wallet/ownerWire';
import {generation, now, payer, payerKey, payment, quote} from './fixtures/order';

// A stand-in for the spending firmware's owner session (device_pilot.c order_request, order_button,
// receipt_status and the 0x7f/0x79 reads). Records outlive a connection, as the board's storage does.
const device = 'd1'.repeat(32), pin = 'd2'.repeat(32);
const phone = new Uint8Array(32).fill(3);
const spki = concat(hexToBytes('3059301306072a8648ce3d020106082a8648ce3d030107034200'), p256.getPublicKey(phone, false));
const owner = sha256(spki);
const confirmer = addressOfPrivateKey(new Uint8Array(32).fill(9));
const request = new Uint8Array(16).fill(0xc);

const stableToken = '0x' + '5'.repeat(40), home = '0x' + 'ab'.repeat(20);
type Board = {
  press: 'approve' | 'refuse' | 'ignore' | 'other-phone'; loseReply?: boolean; cap: bigint; daily: bigint; spent: bigint; clock: boolean; blindDaily?: boolean; wait: number; account: string; state: number;
  prompts: number; spending: boolean; home?: string; far?: boolean; orderOpen?: boolean; nonce?: Uint8Array; records: {request: Uint8Array; digest: Uint8Array; expires: bigint; answer?: Uint8Array}[]; seen: V2Request[]; journal: V2Journal;
};
const board = (change: Partial<Board> = {}): Board => ({press: 'approve', cap: 30_000_000n, daily: 100_000_000n, spent: 0n, clock: true, wait: 0, account: payer, state: 1, prompts: 0,
  spending: true, records: [], seen: [], journal: new V2Journal(32), ...change});

let ids = 0; // the board's journal outlives a connection, so request ids must not repeat
async function connect(b: Board, spending = false) {
  const epoch = new Uint8Array(16).fill(7 + b.seen.length % 200);
  const link = new V2Link('management', 'board', epoch, owner, b.journal);
  const decoder = new V2StreamDecoder();
  const ble = {connectOwner: async () => 247, disconnect: async () => {}, onDisconnect: () => ({remove() {}}),
    onFragment: () => ({remove() {}}), writeFragment: async () => {}} as unknown as Ble;
  const left = () => b.clock && b.spent < b.daily ? b.daily - b.spent : 0n;
  // nu54_order_find: the same request or the same order alone, before its quote ended, is an error.
  const find = (id: Uint8Array, digest: Uint8Array) => {
    if (!b.clock) return 'drop';
    const clash = b.records.some(row => equalBytes(row.request, id) !== equalBytes(row.digest, digest) && row.expires > BigInt(Math.floor(Date.now() / 1000)));
    return clash ? 'drop' : b.records.find(row => equalBytes(row.request, id) && equalBytes(row.digest, digest));
  };
  const answer = (message: V2Request): Uint8Array | 'drop' | 'wait' => {
    const body = message.payload;
    if (message.command === 'status') {
      const known = find(body.subarray(1, 17), body.subarray(17));
      if (known === 'drop') return 'drop';
      return body.length === 49 && body[0] === 1 ? known?.answer ?? Uint8Array.of(0) : 'drop';
    }
    if (message.command === 'pay.prepare') {
      if (bytesToHex(body.subarray(0, 4), false) !== '4e35344f' || body.length !== 4 + 20 + 737) return 'drop';
      const lock = orderLock(body.subarray(24, 24 + q.unsignedSize), bytesToHex(body.subarray(4, 24)));
      const digest = sha256(lock.preimage);
      const known = find(message.id, digest);
      if (known === 'drop') return 'drop';
      if (known) return known.answer ?? Uint8Array.of(0);
      const amount = bytesToBig(body.subarray(24 + q.amountOffset, 24 + q.amountOffset + 32));
      if (amount > left()) return 'drop'; // device_order_within_daily, before the prompt
      b.prompts++;
      if (b.press === 'ignore') return 'wait';
      const signed = b.press === 'refuse' ? undefined : signDigest(hexToBytes(b.press === 'other-phone'
        ? orderLock(body.subarray(24, 24 + q.unsignedSize), '0x' + '1'.repeat(40)).digest : lock.digest), payerKey);
      if (signed) b.spent += amount; // device_order_charge commits before the signature exists
      b.records.push({request: message.id, digest, expires: bytesToBig(body.subarray(24 + q.expiresAtOffset, 24 + q.expiresAtOffset + 32)), answer: signed});
      return b.loseReply ? 'drop' : signed ?? Uint8Array.of(0);
    }
    if (body[0] === 3) return concat(Uint8Array.of(3, b.state, 0, 0, 0), bigToBytes(generation, 8), hexToBytes(device), owner);
    if (!b.spending) return 'drop';
    // spending_request: 0x7a once per wallet; 0x7b, 0x7c and 0x7e not while an approved order can execute; SW2 answers one 0.
    if ([0x7a, 0x7b, 0x7c, 0x7e].includes(body[0])) {
      if ((body[0] !== 0x7a && b.orderOpen) || (body[0] === 0x7a && b.home) || (body[0] === 0x7b && (!b.home || b.far))) return 'drop';
      if (body[0] === 0x7e) {
        const live = b.nonce;
        b.nonce = undefined;
        if (!live || body.length !== 97 || !equalBytes(body.subarray(1, 33), live) || body[33] !== 0x5a) return 'drop';
      }
      b.prompts++;
      if (b.press !== 'approve') return Uint8Array.of(0);
      if (body[0] === 0x7a) { b.home = bytesToHex(body.subarray(1)); return body; }
      if (body[0] === 0x7b) {
        const transfer = {token: stableToken, from: b.account, to: b.home!, amount: bytesToBig(body.subarray(1, 9)), validBefore: bytesToBig(body.subarray(9, 17)), nonce: bytesToHex(body.subarray(17), false)};
        return concat(Uint8Array.of(0x7b), hexToBytes(b.home!), signDigest(walletReturnDigest(transfer), payerKey));
      }
      b.state = 4;
      return Uint8Array.of(body[0]);
    }
    if (body[0] === 0x7d) { b.nonce = new Uint8Array(32).fill(0x3c); return concat(Uint8Array.of(0x7d), b.nonce); }
    if (body[0] === 0x7f) return concat(Uint8Array.of(0x7f, b.home ? 1 : 0), hexToBytes(b.account), b.home ? hexToBytes(b.home) : new Uint8Array(20),
      bigToBytes(b.cap, 8), bigToBytes(b.daily, 8), bigToBytes(BigInt(b.wait), 4), Uint8Array.of(b.orderOpen ? 1 : 0),
      bigToBytes(b.blindDaily ? b.daily : left(), 8));
    if (body[0] === 0x79) return concat(Uint8Array.of(0x79), hexToBytes(b.account),
      signDigest(accountProofDigest(b.account, body.subarray(1)), payerKey), new Uint8Array(64));
    return 'drop';
  };
  const native = {
    onTls: () => ({remove() {}}), stopTls: async () => {},
    startTls: async () => JSON.stringify({out: '', epoch: toBase64(epoch), peer: pin}),
    randomBytes: async (size: number) => toBase64(new Uint8Array(size).fill(20 + ++ids % 230)),
    publicIdentity: async () => toBase64(spki),
    sendTls: async (value: string) => {
      const out: Uint8Array[] = [];
      let closed = false;
      decoder.feed(fromBase64(value), frame => {
        const got = link.receive(frame, BigInt(Math.floor(Date.now() / 1000)));
        if (!got) return;
        b.seen.push(got.request);
        const reply = answer(got.request);
        if (reply === 'drop') closed = true;
        else if (reply !== 'wait') for (const part of link.send({command: 'reply', id: got.request.id, expires: got.request.expires, payload: reply}))
          out.push(encodeV2StreamFrame(part));
      });
      return JSON.stringify({out: '', clear: out.length ? toBase64(concat(...out)) : '', closed});
    },
  } as unknown as Native;
  const session = new OwnerSession(ble, native, {version: 2, device, boardSPKI: pin, ca: 'MA==', certificateChain: ['MA==']}, spending);
  await session.connect('board');
  return session;
}
const approve = async (b: Board, onBoard = () => {}) => {
  const session = await connect(b);
  try { return await session.approveOrder(payment, confirmer, request, onBoard); } finally { session.close(); }
};
const prepared = (b: Board) => b.seen.filter(message => message.command === 'pay.prepare');

beforeEach(() => jest.spyOn(Date, 'now').mockReturnValue(now * 1000));
afterEach(() => jest.restoreAllMocks());

test('the request and the accepted signature match the vectors shared with the firmware and the contract', () => {
  const vectors = Object.fromEntries(readFileSync(resolve(__dirname, '../../../packages/protocol/v2/testdata/order-lock.vectors'), 'utf8')
    .trim().split('\n').map(line => line.split('='))) as Record<string, string>;
  const signed = hexToBytes(vectors.unsigned + vectors.merchantSignature.slice(2));
  const body = orderBoardRequest(vectors.confirmer, signed);
  expect(body).toHaveLength(761);
  expect(body.subarray(24)).toEqual(signed);
  expect(bytesToHex(orderLock(signed.subarray(0, q.unsignedSize), vectors.confirmer).preimage)).toBe(vectors.preimage);
});

test('one order is sent as the firmware expects it and the board signature is verified', async () => {
  const b = board(), shown = jest.fn();
  expect(await approve(b, shown)).toMatch(/^[0-9a-f]{128}(1b|1c)$/);
  expect(shown).toHaveBeenCalledTimes(1);
  const [sent] = prepared(b);
  expect(sent.payload).toEqual(orderBoardRequest(confirmer, hexToBytes(quote.signedQuote)));
  expect(sent.id).toEqual(request);
  expect(sent.expires).toBe(BigInt(quote.expiresAt));
});

test('a lost reply is read back without a second press or a second signature', async () => {
  const b = board({loseReply: true});
  await expect(approve(b)).rejects.toThrow('OWNER_DISCONNECTED');
  const shown = jest.fn();
  const recovered = await approve(b, shown);
  expect(recovered).toBe(bytesToHex(b.records[0].answer!, false));
  expect(shown).not.toHaveBeenCalled();
  expect(b.prompts).toBe(1);
  expect(prepared(b)).toHaveLength(1);
});

test('a request the board never answered is asked again with the same id and deadline', async () => {
  jest.useFakeTimers({now: now * 1000});
  const b = board({press: 'ignore'});
  const session = await connect(b);
  const first = session.approveOrder(payment, confirmer, request, () => {});
  const failed = expect(first).rejects.toThrow('OWNER_DISCONNECTED');
  await jest.advanceTimersByTimeAsync(10);
  session.close();
  await failed;
  b.press = 'approve';
  const again = await connect(b);
  await again.approveOrder(payment, confirmer, request, () => {});
  again.close();
  const [one, two] = prepared(b);
  expect(two).toEqual(one);
  jest.useRealTimers();
});

test('a board that gives no answer by the quote deadline ends the session without a signature', async () => {
  jest.useFakeTimers({now: now * 1000});
  const b = board({press: 'ignore'});
  const session = await connect(b);
  const waiting = expect(session.approveOrder(payment, confirmer, request, () => {})).rejects.toThrow('OWNER_DISCONNECTED');
  await jest.advanceTimersByTimeAsync((quote.expiresAt - now) * 1000 + 100);
  await waiting;
  expect(b.records).toHaveLength(0);
  jest.useRealTimers();
});

test('the same order under another request id makes the board drop the link, as after a refusal or a cancel', async () => {
  for (const press of ['refuse', 'approve'] as const) {
    const b = board({press});
    await approve(b).catch(() => {});
    const session = await connect(b);
    await expect(session.approveOrder(payment, confirmer, new Uint8Array(16).fill(0xd), () => {})).rejects.toThrow('OWNER_DISCONNECTED');
    expect(b.prompts).toBe(1);
  }
});

test('an order over the daily total is dropped by the board even if its status was stale', async () => {
  const b = board({spent: 99_000_000n, blindDaily: true});
  await expect(approve(b)).rejects.toThrow('OWNER_DISCONNECTED');
  expect(b.prompts).toBe(0);
});

test('an approved order counts today once, also when it is read back', async () => {
  const b = board({loseReply: true});
  await approve(b).catch(() => {});
  expect(b.spent).toBe(4_500_000n);
  await approve(b);
  expect(b.spent).toBe(4_500_000n);
});

test('a refusal is final for this request', async () => {
  const b = board({press: 'refuse'});
  await expect(approve(b)).rejects.toThrow('PAYMENT_REFUSED');
  b.press = 'approve';
  await expect(approve(b)).rejects.toThrow('PAYMENT_REFUSED');
  expect(b.prompts).toBe(1);
});

test.each([
  ['another account', {account: '0x' + '5'.repeat(40)}, 'PAYMENT_BOARD_BINDING'],
  ['over the per-payment limit', {cap: 4_499_999n}, 'ORDER_OVER_LIMIT'],
  ['over what is left today', {spent: 96_000_000n}, 'ORDER_OVER_DAILY'],
  ['without trusted time', {clock: false}, 'OWNER_DISCONNECTED'],
  ['a PIN wait', {wait: 60}, 'ORDER_PIN_WAIT'],
  ['a revoked phone', {state: 3}, 'PAYMENT_BOARD_BINDING'],
  ['another firmware profile', {spending: false}, 'OWNER_DISCONNECTED'],
] as [string, Partial<Board>, string][])('%s is refused before the board is asked', async (_name, change, error) => {
  const b = board(change);
  await expect(approve(b)).rejects.toThrow(error);
  expect(prepared(b)).toHaveLength(0);
});

test('a released board is not read as a usable one', async () => {
  const b = board({state: 4});
  await expect(approve(b)).rejects.toThrow('PAYMENT_BOARD_BINDING');
  expect(prepared(b)).toHaveLength(0);
});

test('a signature for another phone key is rejected', async () => {
  await expect(approve(board({press: 'other-phone'}))).rejects.toThrow('ORDER_LOCK_MISMATCH');
});

test('an expired quote is not sent', async () => {
  jest.spyOn(Date, 'now').mockReturnValue(quote.expiresAt * 1000);
  const b = board();
  await expect(approve(b)).rejects.toThrow('PAYMENT_BOARD_BINDING');
  expect(prepared(b)).toHaveLength(0);
});

test('the board account is accepted only with its own signature over a fresh value', async () => {
  const session = await connect(board());
  const status = await session.status();
  expect(await session.spendingAccount(status)).toBe(payer);
  session.close();
  const other = await connect(board({account: '0x' + '5'.repeat(40)}));
  await expect(other.spendingAccount(status)).rejects.toThrow('ACCOUNT_PROOF');
  other.close();
});

test('spending status is strict about its 71 bytes', () => {
  const good = concat(Uint8Array.of(0x7f, 1), hexToBytes(payer), new Uint8Array(20).fill(6), bigToBytes(5n, 8), bigToBytes(9n, 8),
    bigToBytes(61n, 4), Uint8Array.of(1), bigToBytes(7n, 8));
  expect(readSpendingStatus(good)).toEqual({account: payer, returnAddress: '0x' + '06'.repeat(20), perPayment: 5n, daily: 9n, pinWait: 61, orderOpen: true, leftToday: 7n});
  for (const bad of [good.slice(1), good.slice(0, 63), concat(good, Uint8Array.of(0)), good.map((byte, i) => i === 0 ? 0x7e : byte),
    good.map((byte, i) => i === 1 ? 0 : byte), good.map((byte, i) => i > 1 && i < 22 ? 0 : byte), good.map((byte, i) => i === 62 ? 2 : byte)])
    expect(() => readSpendingStatus(bad)).toThrow('SPENDING_STATUS');
});

describe('setup, return and reset on the board', () => {
  const transfer = {token: stableToken, payer, to: home, amount: 42_000_000n, validBefore: now + 420, nonce: '7'.repeat(64)};
  const on = async <T>(b: Board, run: (session: OwnerSession) => Promise<T>) => {
    const session = await connect(b);
    try { return await run(session); } finally { session.close(); }
  };

  test('the signing test needs no button and gives the proof the store asks for', async () => {
    const b = board();
    const challenge = payerEnrollChallenge({tenantRef: 't', branchRef: 'b', subjectRef: 's', payer, generation, idempotencyKey: 'dededede-dede-4ede-9ede-dededededede'});
    const {signature} = await on(b, session => session.proveAccount(payer, challenge));
    expect(recoverSigner(accountProofDigest(payer, challenge), hexToBytes(signature))).toBe(payer);
    await expect(on(b, session => session.proveAccount('0x' + '1'.repeat(40), challenge))).rejects.toThrow('ACCOUNT_PROOF');
    expect(b.prompts).toBe(0);
  });
  test('the return address is stored once, and the same one is not asked again', async () => {
    const b = board(), shown = jest.fn();
    expect(await on(b, session => session.setReturnAddress(generation, payer, home, shown))).toBe(home);
    expect(await on(b, session => session.setReturnAddress(generation, payer, home, shown))).toBe(home);
    expect([b.prompts, shown.mock.calls.length]).toEqual([1, 1]);
    await expect(on(b, session => session.setReturnAddress(generation, payer, '0x' + 'cd'.repeat(20), shown))).rejects.toThrow('RETURN_ADDRESS_SET');
    await expect(on(board(), session => session.setReturnAddress(generation, payer, payer, shown))).rejects.toThrow('RETURN_ADDRESS');
    await expect(on(board({press: 'refuse'}), session => session.setReturnAddress(generation, payer, home, shown))).rejects.toThrow('BOARD_REFUSED');
  });
  test('a return is signed for the stored address only and the signature is checked', async () => {
    const b = board({home});
    const signature = await on(b, session => session.signReturn(generation, transfer, () => {}));
    const digest = walletReturnDigest({...transfer, from: payer, validBefore: BigInt(transfer.validBefore)});
    expect(recoverSigner(digest, hexToBytes(signature))).toBe(payer);
    await expect(on(board(), session => session.signReturn(generation, transfer, () => {}))).rejects.toThrow('RETURN_ADDRESS');
    await expect(on(b, session => session.signReturn(generation, {...transfer, token: '0x' + '6'.repeat(40)}, () => {}))).rejects.toThrow('RETURN_SIGNATURE');
    for (const [change, code] of [[{orderOpen: true}, 'BOARD_ORDER_OPEN'], [{wait: 60}, 'ORDER_PIN_WAIT'], [{account: '0x' + '1'.repeat(40)}, 'ACCOUNT_CHANGED'], [{state: 4}, 'PAYMENT_BOARD_BINDING']] as const) {
      const stopped = board({home, ...change});
      await expect(on(stopped, session => session.signReturn(generation, transfer, () => {}))).rejects.toThrow(code);
      expect(stopped.prompts).toBe(0);
    }
  });
  test('a board whose clock does not fit the deadline gets its own error', async () => {
    const b = board({home, far: true});
    await expect(on(b, session => session.signReturn(generation, transfer, () => {}))).rejects.toThrow('RETURN_NOT_TAKEN');
    expect(b.prompts).toBe(0);
  });
  test('a reset is refused by SW2 or done, and the board then reports a released owner', async () => {
    const refused = board({press: 'refuse'});
    await expect(on(refused, session => session.resetBoard(generation, payer, () => {}))).rejects.toThrow('BOARD_REFUSED');
    expect(refused.state).toBe(1);
    const b = board();
    await on(b, session => session.resetBoard(generation, payer, () => {}));
    expect((await on(b, session => session.status())).state).toBe('released');
  });
  test('the operator reset works within one session and once per request', async () => {
    const b = board(), signature = '5a'.repeat(64);
    await on(b, async session => {
      const asked = await session.operatorResetRequest();
      expect(asked).toEqual({device, generation, nonce: '3c'.repeat(32)});
      await session.operatorReset(asked.nonce, signature, () => {});
    });
    expect(b.state).toBe(4);
    const wrong = board();
    await expect(on(wrong, async session => {
      const asked = await session.operatorResetRequest();
      await session.operatorReset(asked.nonce, '00'.repeat(64), () => {});
    })).rejects.toThrow('OWNER_DISCONNECTED');
    expect(wrong.prompts).toBe(0);
  });
  test('an action is not started when the board is about to end the session', async () => {
    const b = board({home});
    const session = await connect(b);
    jest.spyOn(Date, 'now').mockReturnValue((now + 100) * 1000);
    expect(session.secondsLeft()).toBe(18);
    await expect(session.resetBoard(generation, payer, () => {})).rejects.toThrow('BOARD_SESSION_SHORT');
    expect(b.prompts).toBe(0);
    session.close();
  });
});

test('a board without a live operator permit answers Bluetooth and ends the handshake', async () => {
  const attempt = async (bluetooth: boolean) => {
    const ble = {connectOwner: async () => { if (!bluetooth) throw new Error('gone'); return 247; }, disconnect: async () => {}, onDisconnect: () => ({remove() {}}),
      onFragment: () => ({remove() {}}), writeFragment: async () => { throw new Error('rejected'); }} as unknown as Ble;
    const native = {onTls: () => ({remove() {}}), stopTls: async () => {}, startTls: async () => JSON.stringify({out: 'AA=='})} as unknown as Native;
    const session = new OwnerSession(ble, native, {version: 2, device, boardSPKI: pin, ca: 'MA==', certificateChain: ['MA==']});
    await expect(session.connect('board')).rejects.toThrow();
    return session.reached;
  };
  expect(await attempt(true)).toBe('bluetooth');
  expect(await attempt(false)).toBe('none');
});

test('requests that move no money outlive a board clock that runs ahead; money requests end with their deadline', async () => {
  const b = board({home: home}), clock = BigInt(now);
  const session = await connect(b);
  await session.spendingAccount(await session.status());
  await session.operatorResetRequest();
  await session.orderAnswer(payment, confirmer, request);
  expect(b.seen.map(message => message.expires - clock)).toEqual(b.seen.map(() => 90n));
  b.seen.length = 0;
  await session.signReturn(generation, {token: stableToken, payer, to: home, amount: 1n, validBefore: now + 170, nonce: '7'.repeat(64)}, () => {});
  expect(b.seen[b.seen.length - 1].expires).toBe(BigInt(now + 170));
  await session.approveOrder(payment, confirmer, request, () => {});
  expect(prepared(b)[0].expires).toBe(BigInt(quote.expiresAt));
  session.close();
});

test('tool, firmware and progress requests live 90 s on a spending wallet and 8 s on any other board', async () => {
  for (const [spending, ttl] of [[true, 90], [false, 8]] as const) {
    const b = board();
    type Status = Awaited<ReturnType<OwnerSession['status']>>;
    const requests: [number, (session: OwnerSession, status: Status) => Promise<unknown>][] = [
      [0x70, (session, status) => session.deviceTool(status, 'read')], [0x77, (session, status) => session.runningFirmware(status)],
      [0x78, (session, status) => session.firmwareInputPower(status)], [0x76, session => session.negotiateApprovalProgress()],
    ];
    for (const [op, run] of requests) {
      const session = await connect(b, spending);
      await run(session, await session.status()).catch(() => {}); // the stand-in has none of these
      expect([b.seen[b.seen.length - 1].payload[0], Number(b.seen[b.seen.length - 1].expires) - now]).toEqual([op, ttl]);
      session.close();
    }
  }
  const known = await connect(board());
  await known.spendingStatus(generation);
  expect((known as unknown as {spending: boolean}).spending).toBe(true);
  known.close();
});

test('an order with more than 120 s left is not sent, and a return request never outlives 295 s', async () => {
  const b = board({home});
  jest.spyOn(Date, 'now').mockReturnValue((quote.expiresAt - 121) * 1000);
  await expect(approve(b)).rejects.toThrow('PAYMENT_BOARD_BINDING');
  expect(prepared(b)).toHaveLength(0);
  const session = await connect(b);
  await session.signReturn(generation, {token: stableToken, payer, to: home, amount: 1n, validBefore: quote.expiresAt + 420, nonce: '7'.repeat(64)}, () => {});
  expect(Number(b.seen[b.seen.length - 1].expires)).toBe(quote.expiresAt - 121 + 295);
  session.close();
});
