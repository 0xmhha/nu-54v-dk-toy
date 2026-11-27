/// <reference types="node" />
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  addressOfPrivateKey, bytesToBig, bytesToHex, concat, hexToBytes, orderLockFor, orderTokenName, quoteDigest, quoteV2Layout as q, signDigest,
  type CommerceClient, type CommerceOrder, type CommerceQuote, type NearbyReady, type NearbyRequest,
} from '@nu54/protocol';
import { MerchantInbox } from '../src/kiosk/merchantInbox';
import { NearbyRelay } from '../src/kiosk/nearbyRelay';

const won = 'NU54 Test KRW';
const ref = (n: number) => `f0551c76-7592-4850-a414-54464d24d${n.toString(16).padStart(3, '0')}`;
const branch = ref(1), buyer = ref(3), sale = ref(5), key = ref(6);
const unsigned = readFileSync(resolve(__dirname, '../../../packages/protocol/v2/testdata/order-lock-won.vectors'), 'utf8').split('\n')[0].split('=')[1];
const payerKey = new Uint8Array(32).fill(6);
const raw = hexToBytes(unsigned);
raw.set(hexToBytes(addressOfPrivateKey(payerKey)), q.payerOffset + 12);
const digest = bytesToHex(quoteDigest(raw), false);
const expires = Number(bytesToBig(raw.subarray(q.expiresAtOffset, q.expiresAtOffset + 32)));
const confirmer = '0x0000000000000000000000000000000000001004';
const quote: CommerceQuote = {
  saleRef: sale, branchRef: branch, orderKey: bytesToHex(raw.subarray(q.orderIdOffset, q.orderIdOffset + 32), false), quoteDigest: digest,
  expiresAt: expires, signedQuote: bytesToHex(concat(raw, signDigest(quoteDigest(raw), new Uint8Array(32).fill(9))), false),
};
const peer: NearbyReady = { kind: 'ready', link: 'L', principal: buyer, branch, expiresAt: expires + 20_000, epoch: '', peer: '' };
const request = (name: string): Extract<NearbyRequest, { op: 'lock' }> => {
  const lock = orderLockFor(name, raw, confirmer);
  return { op: 'lock', branch, key, order: sale, quoteDigest: digest, confirmer, lockDigest: lock.digest,
    signature: bytesToHex(signDigest(hexToBytes(lock.digest), payerKey), false) };
};

async function relayFor(tokenName?: string) {
  const inbox = await MerchantInbox.open({ getSetting: async () => '', putSetting: async () => {} }, branch, ref(2));
  const locks = jest.fn(async () => 'submitted-ref');
  const api = {
    profile: { branchRef: branch, subjectRef: ref(2) }, authorize: async () => ({ claimDigest: 'a'.repeat(64), expiresAt: expires }),
    quote: async () => quote, status: async () => ({ id: sale, revision: 3, state: 'preview' }) as CommerceOrder,
    nearbyReservation: async () => undefined, lock: locks,
  } as unknown as CommerceClient;
  const relay = new NearbyRelay(api, inbox, async () => new Uint8Array(16).fill(8), () => sale, () => {}, undefined, undefined, undefined, tokenName);
  const checkout = { op: 'checkout' as const, branch, key, order: sale };
  relay.approveDisplayed(((await relay.handle(checkout, peer)).value as { code: string }).code);
  expect((await relay.handle(checkout, peer)).status).toBe('ok');
  return { relay, locks };
}

beforeEach(() => { jest.spyOn(Date, 'now').mockReturnValue((expires - 60) * 1000); });
afterEach(() => { jest.restoreAllMocks(); });

test.each([
  ['won at a won store', won, won, 'ok'],
  ['dollar at a won store', won, orderTokenName, 'denied'],
  ['won at a dollar store', undefined, won, 'denied'],
  ['dollar at a dollar store', undefined, orderTokenName, 'ok'],
  ['the coin at a coin store', 'WKRC', 'WKRC', 'ok'],
  ['its screen name at a coin store', 'WKRC', 'KRWC', 'denied'],
  ['the test won at a coin store', 'WKRC', won, 'denied'],
  ['the coin at a won store', won, 'WKRC', 'denied'],
] as const)('%s', async (_label, store, signed, status) => {
  const { relay, locks } = await relayFor(store);
  expect((await relay.handle(request(signed), peer)).status).toBe(status);
  expect(locks).toHaveBeenCalledTimes(status === 'ok' ? 1 : 0);
});
