import {addressOfPrivateKey, toBase64} from '@nu54/protocol';
import {lockForSubmit, paymentCheckpoint, paymentReviewStop, type PaymentCheckpoint} from '../src/merchant/paymentCheckpoint';
import {isOpenOrder, orderRecords, type OpenOrder} from '../src/merchant/orderRecords';
import {orderProgress} from '../src/merchant/customerClient';
import {boardSignature, branch, customer, escrowStatus, generation, now, order, orderKey, payer, policy, quote, sale, scope} from './fixtures/order';

jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: {}}));

function memory() {
  const values = new Map<string, string>();
  let seed = 1;
  return {
    values, broken: false, writes: 0,
    async loadJournal(name: string) { if (this.broken) throw new Error('unavailable'); return values.get(name) ?? ''; },
    async saveJournal(name: string, value: string) { this.writes++; values.set(name, value); },
    async randomBytes(size: number) { return toBase64(new Uint8Array(size).fill(seed++)); },
  };
}
const account = {payer, generation: generation.toString()};
const open: Omit<OpenOrder, 'confirmer'> = {...account, sale, branch, customer, store: '그린 카페', asset: policy.asset, amount: '4500000', escrow: policy.contract, merchantId: policy.merchantId, orderKey, sentAt: now};
const seedIn = (storage: ReturnType<typeof memory>) => [...storage.values.keys()].find(name => storage.values.get(name)!.includes('"seed"'))!;

describe('phone confirmer key', () => {
  test('is made once from one stored seed, survives a restart and is never returned', async () => {
    const storage = memory();
    const first = await orderRecords(storage).confirmer(account);
    expect(first).toMatch(/^0x[0-9a-f]{40}$/);
    expect(await orderRecords(storage).confirmer(account)).toBe(first);
    expect(storage.values.size).toBe(1);
    // Only the journal names the native store already seals with the Keystore key are used.
    for (const name of storage.values.keys()) expect(name).toMatch(/^customer-order-[a-f0-9]{64}$/);
    expect(JSON.stringify(await Promise.all([orderRecords(storage).list(), orderRecords(storage).confirmer(account)]))).not.toContain('01'.repeat(32));
  });
  test('differs for another board account or generation, and the seed itself is no key', async () => {
    const storage = memory();
    const records = orderRecords(storage);
    const keys = [await records.confirmer(account), await records.confirmer({...account, generation: '99'}),
      await records.confirmer({...account, payer: '0x' + '5'.repeat(40)}), addressOfPrivateKey(new Uint8Array(32).fill(1))];
    expect(new Set(keys).size).toBe(4);
    expect(storage.values.size).toBe(1);
    await expect(records.confirmer({payer: 'nobody', generation: '1'})).rejects.toThrow('ORDER_ACCOUNT');
    await expect(records.confirmer({...account, generation: '0'})).rejects.toThrow('ORDER_ACCOUNT');
  });
  test('two screens asking at once share one key', async () => {
    const storage = memory();
    const [first, second] = await Promise.all([orderRecords(storage).confirmer(account), orderRecords(storage).confirmer(account)]);
    expect(first).toBe(second);
  });
  test('a lost seed is reported while an order of any account is bound to it', async () => {
    const storage = memory();
    const records = orderRecords(storage);
    const confirmer = await records.confirmer(account);
    await records.keep({...open, confirmer});
    const seedName = seedIn(storage);
    storage.values.delete(seedName);
    await expect(records.confirmer(account)).rejects.toThrow('ORDER_KEY_LOST');
    await expect(records.confirmer({...account, generation: '99'})).rejects.toThrow('ORDER_KEY_LOST');
    expect(storage.values.has(seedName)).toBe(false);
    await records.update(sale, {stage: 'refunded'});
    await expect(records.confirmer(account)).resolves.not.toBe(confirmer);
  });
  test('a replaced seed is reported, and so is one only the checkpoint still needs', async () => {
    const storage = memory();
    const records = orderRecords(storage);
    await expect(records.confirmer(account, '0x' + '9'.repeat(40))).rejects.toThrow('ORDER_KEY_LOST');
    const confirmer = await records.confirmer(account);
    expect(await records.confirmer(account, confirmer)).toBe(confirmer);
    await records.keep({...open, confirmer});
    storage.values.set(seedIn(storage), JSON.stringify({version: 1, seed: 'ab'.repeat(32)}));
    await expect(records.confirmer(account)).rejects.toThrow('ORDER_KEY_LOST');
  });
  test('an unreadable record is never overwritten', async () => {
    const storage = memory();
    const records = orderRecords(storage);
    await records.confirmer(account);
    storage.values.set(seedIn(storage), '{"version":1,"seed":"00"}');
    const writes = storage.writes;
    await expect(records.confirmer(account)).rejects.toThrow('ORDER_KEY_UNREADABLE');
    storage.broken = true;
    await expect(records.confirmer(account)).rejects.toThrow();
    expect(storage.writes).toBe(writes);
  });
});

test('one quote and phone key keep one board request until the quote ends', async () => {
  const storage = memory();
  const records = orderRecords(storage);
  const asked = {digest: quote.quoteDigest, confirmer: '0x' + '7'.repeat(40), expires: quote.expiresAt};
  const first = await records.claimRequest(asked, now);
  expect(first).toMatchObject({reused: false, answered: undefined});
  expect(await orderRecords(storage).claimRequest(asked, now)).toMatchObject({request: first.request, reused: true});
  expect((await records.claimRequest({...asked, confirmer: '0x' + '8'.repeat(40)}, now)).request).not.toBe(first.request);
  await records.answerRequest(first.request, 'refused');
  expect(await records.claimRequest(asked, now)).toMatchObject({request: first.request, answered: 'refused'});
  expect((await records.claimRequest(asked, quote.expiresAt)).request).not.toBe(first.request);
});

test('order records keep unfinished orders and refuse a malformed row', async () => {
  const storage = memory();
  const records = orderRecords(storage);
  const confirmer = await records.confirmer(account);
  await records.keep({...open, confirmer});
  await records.keep({...open, confirmer, sentAt: now + 1});
  expect(await records.list()).toHaveLength(1);
  expect(await records.update(sale, {stage: 'locked', pickupNo: 17})).toMatchObject({pickupNo: 17});
  expect((await records.list()).every(isOpenOrder)).toBe(true);
  await expect(records.keep({...open, confirmer: 'nobody'})).rejects.toThrow('ORDER_RECORDS');
  await expect(records.update(sale, {pickupNo: -1})).rejects.toThrow('ORDER_RECORDS');
});

test('only a status of the same order changes a record', () => {
  const confirmer = '0x' + '7'.repeat(40);
  const row = {...open, confirmer};
  const view = {...order, state: 'paid' as const, pickupNo: 17, escrow: escrowStatus('locked', confirmer)};
  expect(orderProgress(row, view)).toMatchObject({stage: 'locked', pickupNo: 17});
  for (const wrong of [{...view, id: branch}, {...view, amount: '1'}, {...view, escrow: undefined},
    {...view, escrow: escrowStatus('locked', '0x' + '8'.repeat(40))},
    {...view, escrow: {...view.escrow, payer: '0x' + '9'.repeat(40)}}, {...view, escrow: {...view.escrow, merchantId: 'f'.repeat(64)}},
    {...view, escrow: {...view.escrow, escrow: '0x' + '9'.repeat(40)}}, {...view, escrow: {...view.escrow, orderKey: 'f'.repeat(64)}}]) expect(orderProgress(row, wrong)).toBeUndefined();
});

describe('order checkpoint', () => {
  const confirmer = addressOfPrivateKey(new Uint8Array(32).fill(9));
  const entry: PaymentCheckpoint = {version: 2, branch, customer, order, quote, scope, request: 'c'.repeat(32), submitKey: sale,
    route: 'nearby', relayKey: sale, confirmer, store: '그린 카페'};
  const read = (value: object) => paymentCheckpoint(JSON.stringify(value), branch, customer);
  test('is read back after a restart with its phone key and signature', () => {
    const signed = {...entry, boardStarted: true as const, authorization: boardSignature(confirmer)};
    expect(read(signed)).toEqual(signed);
    expect(lockForSubmit(read(signed)!, policy, now)).toMatchObject({confirmer, signature: signed.authorization});
    for (const bad of [{confirmer: 'x'}, {confirmer: '0x' + '0'.repeat(40)}, {confirmer: undefined}, {store: ''}])
      expect(() => read({...signed, ...bad})).toThrow('PAYMENT_JOURNAL');
  });
  test('never submits a signature for another phone, board or amount', () => {
    const signed = {...entry, authorization: boardSignature(confirmer)};
    const other = addressOfPrivateKey(new Uint8Array(32).fill(8));
    expect(() => lockForSubmit({...signed, confirmer: other}, policy, now)).toThrow('ORDER_LOCK_MISMATCH');
    expect(() => lockForSubmit({...signed, authorization: boardSignature(confirmer, new Uint8Array(32).fill(5))}, policy, now)).toThrow('ORDER_LOCK_MISMATCH');
    expect(() => lockForSubmit({...signed, order: {...order, amount: '1'}}, policy, now)).toThrow('PAYMENT_QUOTE_MISMATCH');
    expect(() => lockForSubmit(signed, {...policy, contract: '0x' + '4'.repeat(40)}, now)).toThrow('PAYMENT_QUOTE_MISMATCH');
    expect(() => lockForSubmit(signed, policy, quote.expiresAt)).toThrow();
    expect(() => lockForSubmit({...signed, authorization: undefined}, policy, now)).toThrow('PAYMENT_APPROVAL_PENDING');
  });
  test('an interrupted approval asks the same board again, and only an unsent signature may expire', () => {
    expect(paymentReviewStop({...entry, boardStarted: true}, now)).toBeUndefined();
    expect(paymentReviewStop({...entry, boardStarted: true, confirmer: undefined}, now)).toBe('interrupted');
    expect(paymentReviewStop(entry, quote.expiresAt)).toBe('expired');
    const signed = {...entry, authorization: boardSignature(confirmer)};
    expect(paymentReviewStop(signed, now)).toBeUndefined();
    expect(paymentReviewStop(signed, quote.expiresAt)).toBe('expired');
    expect(paymentReviewStop({...signed, forwardAttempted: true}, quote.expiresAt + 9999)).toBeUndefined();
    expect(paymentReviewStop({...signed, confirmer: undefined}, quote.expiresAt)).toBeUndefined();
  });
});
