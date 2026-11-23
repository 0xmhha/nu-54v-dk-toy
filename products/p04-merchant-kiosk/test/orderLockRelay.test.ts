/// <reference types="node" />
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  addressOfPrivateKey, bytesToBig, bytesToHex, concat, hexToBytes, orderLock, quoteDigest, quoteV2Layout as q, signDigest,
  type CommerceClient, type CommerceOrder, type CommerceQuote, type NearbyReady, type NearbyRequest,
} from '@nu54/protocol';
import { MerchantInbox } from '../src/kiosk/merchantInbox';
import { NearbyRelay, type RelayProgress } from '../src/kiosk/nearbyRelay';

const ref = (n: number) => `f0551c76-7592-4850-a414-54464d24d${n.toString(16).padStart(3, '0')}`;
const branch = ref(1), terminal = ref(2), buyer = ref(3), other = ref(4), sale = ref(5), key = ref(6);
const vectors = Object.fromEntries(
  readFileSync(resolve(__dirname, '../../../packages/protocol/v2/testdata/order-lock.vectors'), 'utf8')
    .trim().split('\n').map(line => line.split('=')),
) as Record<string, string>;
// The shared vector's payer key is not published, so this quote names a payer the test can sign for.
const payerKey = new Uint8Array(32).fill(6);
const raw = hexToBytes(vectors.unsigned);
raw.set(hexToBytes(addressOfPrivateKey(payerKey)), q.payerOffset + 12);
const digest = bytesToHex(quoteDigest(raw), false);
const expires = Number(bytesToBig(raw.subarray(q.expiresAtOffset, q.expiresAtOffset + 32)));
const confirmer = vectors.confirmer;
const lock = orderLock(raw, confirmer);
const sign = (hex: string, secret = payerKey) => bytesToHex(signDigest(hexToBytes(hex), secret), false);
const quote: CommerceQuote = {
  saleRef: sale, branchRef: branch, orderKey: bytesToHex(raw.subarray(q.orderIdOffset, q.orderIdOffset + 32), false),
  quoteDigest: digest, expiresAt: expires,
  signedQuote: bytesToHex(concat(raw, signDigest(quoteDigest(raw), new Uint8Array(32).fill(9))), false),
};
const peer = (principal: string, expiresAt = expires + 20_000): NearbyReady => ({
  kind: 'ready', link: 'L-' + principal, principal, branch, expiresAt, epoch: '', peer: '',
});
const request: Extract<NearbyRequest, { op: 'lock' }> = {
  op: 'lock', branch, key, order: sale, quoteDigest: digest, confirmer, lockDigest: lock.digest, signature: sign(lock.digest),
};
const paid: CommerceOrder = {
  id: sale, revision: 3, state: 'paid', asset: '0x' + '1'.repeat(40), amount: '7', pickupNo: 17, receiptRef: ref(7),
  expiresAt: new Date(expires * 1000).toISOString(),
  escrow: {
    stage: 'locked', escrow: '0x' + '1'.repeat(40), merchantId: '7'.padStart(64, '0'), orderKey: quote.orderKey,
    payer: addressOfPrivateKey(payerKey), confirmer, lockTxHash: '0x' + '1'.repeat(64), deliverBy: expires + 600,
  },
};

async function setup(quoted = true) {
  const disk = new Map<string, string>();
  const inbox = await MerchantInbox.open({
    getSetting: async name => disk.get(name) ?? '',
    putSetting: async (name, value) => { disk.set(name, value); },
  }, branch, terminal);
  const locks = jest.fn(async () => 'submitted-ref');
  const state: { order: CommerceOrder; reserved?: string } = { order: { ...paid, state: 'preview', escrow: undefined } };
  const api = {
    profile: { branchRef: branch, subjectRef: terminal },
    authorize: async () => ({ claimDigest: 'a'.repeat(64), expiresAt: expires }),
    quote: async () => quote,
    status: async () => state.order,
    nearbyReservation: jest.fn(async (_order: string, customer: string) => state.reserved === customer ? sale : undefined),
    lock: locks,
  } as unknown as CommerceClient;
  const progress: RelayProgress[] = [];
  const relay = new NearbyRelay(api, inbox, async () => new Uint8Array(16).fill(8), () => sale, () => {},
    undefined, undefined, event => progress.push(event));
  if (quoted) {
    const checkout = { op: 'checkout' as const, branch, key, order: sale };
    const pending = await relay.handle(checkout, peer(buyer));
    relay.approveDisplayed((pending.value as { code: string }).code);
    expect((await relay.handle(checkout, peer(buyer))).status).toBe('ok');
  }
  return { relay, inbox, locks, progress, state };
}

let time: jest.SpyInstance;
const setTime = (seconds: number) => time.mockReturnValue(seconds * 1000);
beforeEach(() => {
  time = jest.spyOn(Date, 'now');
  setTime(expires - 60);
});
afterEach(() => time.mockRestore());

test('a board signature over the issued quote is sent to the server and reported to the screen', async () => {
  const { relay, locks, progress } = await setup();
  expect(progress).toEqual([{ sale, stage: 'quoted', until: expires }]);
  expect(await relay.handle(request, peer(buyer))).toEqual({ status: 'ok', value: { submitted: 'submitted-ref' } });
  expect(locks.mock.calls).toEqual([[sale, digest, confirmer, lock.digest, request.signature, key]]);
  expect(progress.at(-1)).toEqual({ sale, stage: 'submitting', until: expires });
});

test('another customer cannot lock the order', async () => {
  const { relay, locks, state } = await setup();
  expect(await relay.handle(request, peer(other))).toEqual({ status: 'denied' });
  // Even a server reservation does not let a customer answer a quote handed to somebody else.
  state.reserved = other;
  expect(await relay.handle(request, peer(other))).toEqual({ status: 'denied' });
  expect(await relay.handle({ ...request, branch: ref(9) }, peer(buyer))).toEqual({ status: 'denied' });
  expect(locks).not.toHaveBeenCalled();
});

test('a tampered confirmer, signature, lock digest or quote digest never reaches the server', async () => {
  const { relay, locks, progress } = await setup();
  const swapped = '0x' + '5'.repeat(40);
  const bad: (typeof request)[] = [
    { ...request, confirmer: swapped },
    { ...request, confirmer: swapped, lockDigest: orderLock(raw, swapped).digest },
    { ...request, signature: sign(lock.digest, new Uint8Array(32).fill(5)) },
    { ...request, signature: request.signature.slice(0, 2) + (request.signature[2] === '0' ? '1' : '0') + request.signature.slice(3) },
    { ...request, lockDigest: 'b'.repeat(64) },
    { ...request, quoteDigest: 'c'.repeat(64) },
  ];
  for (const item of bad) expect(await relay.handle(item, peer(buyer))).toEqual({ status: 'denied' });
  expect(locks).not.toHaveBeenCalled();
  expect(progress).toHaveLength(1);
  expect((await relay.handle(request, peer(buyer))).status).toBe('ok');
});

test('an expired quote is refused, but a lock already sent is asked again with the same values', async () => {
  const late = await setup();
  setTime(expires);
  // The customer's claim ends with the quote; the server-side reservation still names the owner.
  late.state.reserved = buyer;
  expect(await late.relay.handle(request, peer(buyer))).toEqual({ status: 'expired' });
  expect(late.locks).not.toHaveBeenCalled();

  setTime(expires - 60);
  const lost = await setup();
  lost.locks.mockRejectedValueOnce(new Error('NETWORK'));
  expect(await lost.relay.handle(request, peer(buyer))).toEqual({ status: 'pending' });
  setTime(expires + 5);
  expect(await lost.relay.handle(request, peer(buyer))).toEqual({ status: 'ok', value: { submitted: 'submitted-ref' } });
  expect(lost.locks.mock.calls[0]).toEqual(lost.locks.mock.calls[1]);
});

test('a duplicate uses the same key and a second different lock for the order is refused', async () => {
  const { relay, locks } = await setup();
  const replies = await Promise.all([relay.handle(request, peer(buyer)), relay.handle(request, peer(buyer))]);
  expect(replies.map(reply => reply.status)).toEqual(['ok', 'ok']);
  expect(locks.mock.calls[0]).toEqual(locks.mock.calls[1]);
  const second = '0x' + '6'.repeat(40);
  const next = orderLock(raw, second).digest;
  expect(await relay.handle({ ...request, confirmer: second, lockDigest: next, signature: sign(next) }, peer(buyer)))
    .toEqual({ status: 'denied' });
  expect(locks).toHaveBeenCalledTimes(2);
});

test.each([['FORBIDDEN', 'denied'], ['REPLAY_CONFLICT', 'denied'], ['BAD_REQUEST', 'denied'], ['BUSY', 'pending'], ['EXPIRED', 'pending']])(
  'the server answer %s becomes %s like forward', async (error, status) => {
    const { relay, locks } = await setup();
    locks.mockRejectedValueOnce(new Error(error));
    expect(await relay.handle(request, peer(buyer))).toEqual({ status });
  },
);

test('a restarted terminal without the quote leaves the check to the server for the same owner only', async () => {
  const { relay, locks, state } = await setup(false);
  expect(await relay.handle(request, peer(buyer))).toEqual({ status: 'denied' });
  state.reserved = buyer;
  expect(await relay.handle(request, peer(other))).toEqual({ status: 'denied' });
  expect((await relay.handle(request, peer(buyer))).status).toBe('ok');
  expect(locks).toHaveBeenCalledTimes(1);
});

test('the same customer reads the order number and escrow over the link after the claim would have ended', async () => {
  const { relay, state } = await setup();
  await relay.handle(request, peer(buyer));
  state.order = { ...paid, menuCheckout: { branchLabel: 'A', payerBound: true, customerRef: buyer } };
  setTime(expires + 3 * 3_600);
  const status = { op: 'status' as const, branch, key, order: sale };
  const answer = await relay.handle(status, peer(buyer));
  expect(answer).toMatchObject({ status: 'ok', value: { state: 'paid', pickupNo: 17, escrow: { stage: 'locked' } } });
  expect(answer.value).not.toHaveProperty('menuCheckout');
  expect(JSON.stringify(answer).length).toBeLessThan(1_800);
  expect(await relay.handle(status, peer(other))).toEqual({ status: 'denied' });
});

test('every waiting phone is offered to the customer at the terminal', async () => {
  const shown: (string[] | undefined)[] = [];
  const { inbox } = await setup(false);
  let seed = 0;
  const relay = new NearbyRelay({ profile: { branchRef: branch } } as CommerceClient, inbox,
    async () => new Uint8Array(16).fill(++seed), () => sale, pair => shown.push(pair?.codes));
  for (const [n, customer] of [buyer, other].entries())
    await relay.handle({ op: 'checkout', branch, key: ref(20 + n), order: sale }, peer(customer));
  expect(shown.at(-1)).toHaveLength(2);
  relay.approveDisplayed(shown.at(-1)![1]);
  expect(shown.at(-1)).toEqual([shown[1]![0]]);
});

test('status stays open until the order is collected, then only for a short grace', async () => {
  const { relay, state, inbox } = await setup();
  await relay.handle(request, peer(buyer, expires + 90_000));
  const status = { op: 'status' as const, branch, key, order: sale };
  const ask = (customer = buyer) => relay.handle(status, peer(customer, expires + 90_000)).then(reply => reply.status);
  const at = (pickupStage: 'ready' | 'collected'): CommerceOrder => ({
    ...paid, payment: { stage: 'finalized', chainId: 8283, txHash: '0x' + '1'.repeat(64), includedAt: new Date(expires * 1000).toISOString(), pickupStage },
  });
  state.order = at('ready');
  setTime(expires + 6 * 3_600);
  expect(await ask()).toBe('ok');
  expect(await ask(other)).toBe('denied');
  // A repeated lock does not push the limit further out.
  const limit = inbox.statusUntil(sale, buyer);
  await relay.handle(request, peer(buyer, expires + 90_000));
  expect(inbox.statusUntil(sale, buyer)).toBe(limit);
  state.order = at('collected');
  expect(await ask()).toBe('ok');
  setTime(expires + 6 * 3_600 + 599);
  expect(await ask()).toBe('ok');
  setTime(expires + 6 * 3_600 + 601);
  expect(await ask()).toBe('denied');
});

test('an order nobody collects also ends', async () => {
  const { relay, state } = await setup();
  await relay.handle(request, peer(buyer, expires + 90_000));
  state.order = paid;
  setTime(expires - 60 + 12 * 3_600);
  expect(await relay.handle({ op: 'status', branch, key, order: sale }, peer(buyer, expires + 90_000))).toEqual({ status: 'denied' });
});

test('kept orders never block a new payment, and an unpaid claim is never dropped for one', async () => {
  const { inbox } = await setup(false);
  const until = expires + 10;
  for (let n = 0; n < 64; n++) {
    await inbox.grantClaim(ref(300 + n), ref(500 + n), until);
    if (n >= 2) await inbox.keepStatus(ref(300 + n), ref(500 + n), until + n);
  }
  await inbox.grantClaim(ref(400), ref(600), until);
  expect(inbox.ownsSale(ref(400), ref(600))).toBe(true);
  expect([300, 301, 303].every((n, i) => inbox.ownsSale(ref(n), ref([500, 501, 503][i])))).toBe(true);
  expect(inbox.ownsSale(ref(302), ref(502))).toBe(false);
  await inbox.keepStatus(ref(402), ref(602), until);
  expect(inbox.ownsSale(ref(402), ref(602))).toBe(false);
  // A late claim for the same customer cannot shorten what the customer already has.
  await inbox.grantClaim(ref(303), ref(503), until - 5);
  expect(inbox.statusUntil(ref(303), ref(503))).toBe(until + 3);
});

test('the screen learns that the quoted phone left and came back, and nothing after the quote ends', async () => {
  const { relay, progress } = await setup();
  relay.retire('L-somebody');
  expect(progress).toHaveLength(1);
  relay.retire('L-' + buyer);
  expect(progress.at(-1)).toEqual({ sale, stage: 'disconnected' });
  const back = { ...peer(buyer), link: 'again' };
  await relay.handle({ op: 'status', branch, key, order: sale }, peer(other));
  expect(progress).toHaveLength(2);
  await relay.handle({ op: 'status', branch, key, order: sale }, back);
  expect(progress.at(-1)).toEqual({ sale, stage: 'connected' });
  setTime(expires);
  relay.retire('again');
  expect(progress).toHaveLength(3);
});

test('a phone chosen by its mark can lock the quote this terminal fetched for it', async () => {
  const { relay, inbox, locks } = await setup(false);
  expect(await relay.handle(request, peer(buyer))).toEqual({ status: 'denied' });
  await inbox.grantClaim(sale, buyer, expires);
  relay.rememberIssued(quote, buyer);
  expect(await relay.handle({ ...request, lockDigest: 'b'.repeat(64) }, peer(buyer))).toEqual({ status: 'denied' });
  expect(await relay.handle(request, peer(other))).toEqual({ status: 'denied' });
  expect((await relay.handle(request, peer(buyer))).status).toBe('ok');
  expect(locks).toHaveBeenCalledTimes(1);
});
