import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {
  bindServiceRequestV2, bytesToHex, CommerceClient, CommerceHoldRefusal, commerceLineCapacity, hexToBytes, type CommerceCart, type CommerceProfile, type ServiceRequestV2,
} from '@nu54/protocol';
import {
  accountOpenOrders, CustomerEntrance, EntranceError, entranceBusy, entranceRefusal, forgetEntranceWaits, issuePickupQr, nativeEntrance, readOpenOrders,
  type BoardLink, type EntrancePost, type EntranceReply,
} from '../src/merchant/customerEntrance';
import {SessionEntrance} from '../src/merchant/customerSession';
import {orderPaymentPin} from '../src/merchant/orderPaymentTrust';
import {readCartCancel, StoreAccess, StoreBusy} from '../src/merchant/storeAccess';
import {StoreDirectory, storeNow} from '../src/merchant/storeDirectory';
import {storeStopped} from '../src/merchant/StoreNotice';
import {journalStore} from './fixtures/journal';

// The phone's parsers against what the store server really answers. The answers are in one file,
// packages/protocol/v2/testdata/customer-entrance.json, written by the server's own tests from its routes
// (p05 internal/commerce/customer_contract_test.go) and compared with them on every run of those tests. This test
// reads the same file, so neither side can change an answer without the other side's test failing.

type Answer = {status: number; retryAfter?: number; body: Record<string, unknown> | null};
const file = JSON.parse(readFileSync(resolve(__dirname, '../../../packages/protocol/v2/testdata/customer-entrance.json'), 'utf8')) as {answers: Record<string, Answer>};
const answers = file.answers;

const mockJournal = journalStore();
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: {
  loadJournal: (name: string) => mockJournal.loadJournal(name), saveJournal: (name: string, value: string) => mockJournal.saveJournal(name, value),
  randomBytes: async (size: number) => Buffer.from(new Uint8Array(size).fill(7)).toString('base64')}}));
jest.mock('../src/specs/NativeOwnerWallet.ts', () => ({__esModule: true, default: null}));

// The samples of the file.
const cafe = '0fbc54ef-2827-4ab5-bfcd-3a27611c3002', bakery = '0fbc54ef-2827-4ab5-bfcd-3a27611c3003', sale = '0fbc54ef-2827-4ab5-bfcd-3a27611c3001';
const tenant = '0fbc54ef-2827-4ab5-bfcd-3a27611c39e0', subject = '0fbc54ef-2827-4ab5-bfcd-3a27611c3004';
const token = 'ab'.repeat(32), seconds = 1_764_201_600;
// Before the sample time a menu row is on sale until.
const clock = Math.floor(Date.parse('2026-11-27T11:00:00Z') / 1000);
const account = '0x' + '7'.repeat(40), signature = 'c'.repeat(130);

const reply = (name: string): EntranceReply => {
  const answer = answers[name];
  if (!answer) throw new Error(`the server's answers do not hold ${name}`);
  return {status: answer.status, body: answer.body, ...(answer.retryAfter ? {retryAfter: answer.retryAfter} : {})};
};
/** An entrance that gives one answer of the file to whatever is asked. */
const answering = (name: string, sent: [string, string, string | undefined][] = []): EntrancePost =>
  async (path, body, session) => { sent.push([path, body, session]); return reply(name); };
const rejection = async (run: Promise<unknown>) => run.then(() => { throw new Error('answered'); }, (error: unknown) => error);

const profile: CommerceProfile = {role: 'customer', branchRef: cafe, tenantRef: tenant, subjectRef: subject, expiresAt: clock + 900};
/** A client of a session: the server binds its answer to the request, so the sample binding is replaced by the real one. */
function sessionClient(name: string) {
  return new CommerceClient(profile, async raw => {
    const {command} = JSON.parse(raw) as {command: ServiceRequestV2};
    const {requestId, requestDigest} = bindServiceRequestV2(command,
      {...profile, grants: [{domainRef: command.domainRef, objectRef: command.objectRef, operations: [`C3/${command.operation}`]}]}, clock);
    return JSON.stringify({...answers[name].body, requestId, requestDigest});
  }, async () => '0fbc54ef-2827-4ab5-bfcd-3a27611c3005', () => clock);
}
const cart: CommerceCart = {origin: 'menu', originRef: '0fbc54ef-2827-4ab5-bfcd-3a27611c3006', menuVersion: 1, items: [{sku: 'tea', option_key: 'standard', quantity: 1}]};
const asset = '0x' + '2'.repeat(40);
const reserve = (name: string) => rejection(sessionClient(name).reserve(cart, asset, '0fbc54ef-2827-4ab5-bfcd-3a27611c3007'));

/** One store of this build, read through the entrance without a board or a registration. */
const access = (post: EntrancePost) => StoreAccess.open({branch: cafe, name: '여울 커피'}, {post, origin: 'https://store.example:8444',
  pin: orderPaymentPin({chain: 8283, contract: '0x' + '3'.repeat(40), asset})!, board: {open: async () => { throw new Error('BOARD_NOT_FOUND'); }},
  wallet: async () => undefined, now: () => clock});

const board = (): BoardLink => ({account, generation: '7', prove: async () => ({account, signature}), close: () => {}});
const pickup = (name: string) => issuePickupQr(new CustomerEntrance(async path => path === '/v2/customer/challenge' ? reply('challenge') : reply(name)),
  {open: async () => board()}, {branch: cafe, sale, payer: account});

// Every answer of the file is read below. A new answer on the server needs a line here saying what the app makes of it.
const read: Record<string, () => Promise<void> | void> = {
  // --- too many requests: "again shortly", with the time to wait, and never a word about the session or the order ---
  'busy-rate': () => busy('busy-rate', 429, 3),
  'busy-rate-session': () => busy('busy-rate-session', 429, 3),
  'busy-in-flight': () => busy('busy-in-flight', 503, 1),

  // --- the account proof ---
  challenge: async () => {
    expect(bytesToHex(await new SessionEntrance(answering('challenge')).challenge({purpose: 'checkout', branch: cafe}), false)).toBe(token);
    expect(await new CustomerEntrance(answering('challenge')).pickupChallenge(cafe, sale)).toEqual({challenge: hexToBytes(token), expiresAt: seconds});
  },
  'proof-checkout': async () => {
    const session = await new SessionEntrance(answering('proof-checkout')).prove(hexToBytes(token), {account, signature}, '7', {purpose: 'checkout', branch: cafe});
    expect(session).toEqual({token, purpose: 'checkout', branch: cafe, tenant, subject, expiresAt: seconds});
  },

  // --- the store list, a page at a time ---
  'store-list-page': async () => {
    const sent: [string, string, string | undefined][] = [];
    const page = await new StoreDirectory(answering('store-list-page', sent)).list('0fbc54ef-2827-4ab5-bfcd-3a27611c3009');
    expect(sent).toEqual([['/v2/customer/stores?after=0fbc54ef-2827-4ab5-bfcd-3a27611c3009', '', undefined]]);
    expect(page.next).toBe(cafe); // the place to go on from: the page's last store
    expect(page.stores).toEqual([{branch: cafe, name: '여울 커피', kind: '카페', address: '시험시 시험로 1', hours: expect.any(Array),
      status: {mode: 'open', orderable: true, weekday: 4, time: '10:31'}}]);
  },
  'store-list-last': async () => {
    const page = await new StoreDirectory(answering('store-list-last')).list(cafe);
    expect(page.next).toBeUndefined(); // no `next`: the list is complete
    expect(page.stores.map(store => [store.branch, store.status.orderable, store.status.reason])).toEqual([[bakery, false, 'no_menu']]);
  },

  // --- a store page: whether an order can be placed is the server's word, with its reason ---
  'store-page': async () => {
    const page = await new StoreDirectory(answering('store-page')).page(cafe);
    expect(page).toEqual({branch: cafe, name: '여울 커피', kind: '카페', address: '시험시 시험로 1', hours: expect.any(Array),
      status: {mode: 'open', orderable: true, weekday: 4, time: '10:31'}, intro: '직접 볶은 원두로 내립니다', directions: '역 2번 출구에서 걸어서 3분',
      phone: '02-555-0100', amenities: '무선 인터넷', parking: '건물 뒤 2대', closedNote: '매주 일요일',
      merchantId: 'e1' + '0'.repeat(60) + '07', operatorContact: 'help@operator.example'});
    // All a page says about paying is the store's number in the order contract: no token, address, chain or signer.
    expect(Object.keys(answers['store-page'].body!).filter(key => /token|asset|contract|signer|beneficiary|chain|rpc/i.test(key))).toEqual([]);
    expect(storeNow(page)).toEqual({line: '영업 중 · 21:00에 영업 종료', shut: false, dayOff: false, note: ''});
  },
  'store-page-busy': async () => {
    const page = await new StoreDirectory(answering('store-page-busy')).page(cafe);
    expect(page.status).toEqual({mode: 'busy', orderable: true, busyMinutes: 20, weekday: 4, time: '10:31'});
    expect(storeNow(page)).toMatchObject({line: '영업 중 · 바쁨, 약 20분 · 21:00에 영업 종료', shut: false});
  },
  'store-page-no-menu': async () => {
    const page = await new StoreDirectory(answering('store-page-no-menu')).page(bakery);
    // `mode` is closed whenever no order can be placed; that alone is not a day off.
    expect(page.status).toEqual({mode: 'closed', orderable: false, reason: 'no_menu', weekday: 4, time: '10:31'});
    expect(storeNow(page)).toEqual({line: '판매 중인 메뉴 없음', shut: true, dayOff: false, note: '판매 중인 메뉴 없음', why: '지금 판매 중인 메뉴가 없어 주문을 받지 않습니다.'});
  },
  'store-page-closed-day': async () => {
    const page = await new StoreDirectory(answering('store-page-closed-day')).page(bakery);
    expect(page.status).toEqual({mode: 'closed', orderable: false, reason: 'closed_day', weekday: 4, time: '10:31'});
    expect(storeNow(page)).toEqual({line: '오늘 쉽니다', shut: true, dayOff: true, note: '오늘 쉽니다', why: '오늘은 쉬는 날이라 주문을 받지 않습니다.'});
  },
  'store-page-paused': async () => {
    const page = await new StoreDirectory(answering('store-page-paused')).page(bakery);
    expect(page.status).toEqual({mode: 'paused', orderable: false, reason: 'paused', resumeAt: '11:00', weekday: 4, time: '10:31'});
    expect(storeNow(page)).toMatchObject({line: '주문 잠시 멈춤 · 11:00쯤 다시 받습니다', shut: true, dayOff: false,
      why: '매장이 주문을 잠시 멈췄습니다. 11:00쯤 다시 받는다고 알렸습니다.'});
  },

  // --- a store that did not publish its page is no store: page, menu and photo alike ---
  'store-page-unpublished': async () => {
    expect(await rejection(new StoreDirectory(answering('store-page-unpublished')).page(cafe))).toMatchObject({code: 'REFUSED'});
  },
  'menu-unpublished': async () => {
    expect(await rejection((await access(answering('menu-unpublished'))).client.publishedMenu())).toMatchObject({message: 'STORE_NOT_PUBLIC'});
  },
  'photo-unpublished': async () => {
    // A photo that cannot be read is a product shown without its photo: no word about a busy server or a session.
    const error = await rejection((await access(answering('photo-unpublished'))).client.catalogPhoto('ab'.repeat(32)));
    expect(error).toMatchObject({message: 'BAD_RESPONSE'});
    expect(error).not.toBeInstanceOf(StoreBusy);
  },

  // --- the menu: the public one tells on sale or sold out, the session's tells how many are left ---
  'menu-public': async () => {
    const sent: [string, string, string | undefined][] = [];
    const menu = await (await access(answering('menu-public', sent))).client.publishedMenu();
    expect(sent).toEqual([[`/v2/customer/stores/${cafe}/menu/0`, '', undefined]]);
    expect(menu.shop).toMatchObject({branch: cafe, branchLabel: '여울 커피'});
    expect(menu.orderState).toEqual({mode: 'open'});
    expect(menu.catalog.map(row => [row.sku, row.available, row.skuAvailable, row.uncounted])).toEqual([['cake', 0, 0, true], ['tea', 1000, 1000, true]]);
    // 1000 is the server's word for "on sale" and the most one line may ask for; every public row is marked, so no
    // screen shows it as a count. A sold-out row still takes nothing into the basket.
    expect(menu.catalog.map(row => commerceLineCapacity(row, {}))).toEqual([0, 1000]);
  },
  // A store that takes no order now still shows its menu to read: the same rows, and the state that says why.
  'menu-public-closed-day': async () => {
    const menu = await (await access(answering('menu-public-closed-day'))).client.publishedMenu();
    expect(menu.orderState).toEqual({mode: 'closed'});
    expect(storeStopped(menu.orderState)).toBe(true);
    expect(menu.catalog.map(row => [row.sku, row.available, row.skuAvailable, row.uncounted])).toEqual([['cake', 0, 0, true], ['tea', 1000, 1000, true]]);
  },
  'menu-public-paused': async () => {
    const menu = await (await access(answering('menu-public-paused'))).client.publishedMenu();
    expect(menu.orderState).toEqual({mode: 'paused', resumeAt: '11:00'});
    expect(storeStopped(menu.orderState)).toBe(true);
    expect(menu.catalog.map(row => [row.sku, row.available, row.skuAvailable, row.uncounted])).toEqual([['cake', 0, 0, true], ['tea', 1000, 1000, true]]);
  },
  'menu-session': async () => {
    const menu = await sessionClient('menu-session').publishedMenu();
    expect(menu.catalog.map(row => [row.sku, row.available, row.skuAvailable, row.uncounted])).toEqual([['cake', 0, 0, undefined], ['tea', 5, 5, undefined]]);
    expect(menu.catalog.map(row => commerceLineCapacity(row, {}))).toEqual([0, 5]);
  },

  // --- a cart the server did not hold ---
  'hold-balance-short': async () => {
    const refusal = await reserve('hold-balance-short');
    expect(refusal).toBeInstanceOf(CommerceHoldRefusal);
    expect(refusal).toMatchObject({message: 'CHECKOUT_CONFLICT', reason: 'balance_short', required: '300000', balance: '299999'});
    expect(reply('hold-balance-short')).toMatchObject({status: 409});
    expect(reply('hold-balance-short').retryAfter).toBeUndefined();
  },
  'hold-balance-unavailable': async () => {
    const refusal = await reserve('hold-balance-unavailable');
    expect(refusal).toBeInstanceOf(CommerceHoldRefusal);
    expect(refusal).toMatchObject({message: 'BUSY', reason: 'balance_unavailable', required: undefined, balance: undefined});
    expect(reply('hold-balance-unavailable')).toMatchObject({status: 503, retryAfter: 3});
    expect(await heldBack('hold-balance-unavailable')).toBe(false); // about this cart only: the session's other requests go on
  },
  'hold-limit': async () => {
    const refusal = await reserve('hold-limit');
    expect(refusal).toBeInstanceOf(CommerceHoldRefusal);
    expect(refusal).toMatchObject({message: 'BUSY', reason: 'hold_limit'});
    expect(reply('hold-limit')).toMatchObject({status: 429, retryAfter: 10});
    expect(await heldBack('hold-limit')).toBe(false);
  },
  'hold-orders-closed': async () => {
    // The store cannot be paid by a board until its operator changes that: no wait is named, and none is kept.
    const refusal = await reserve('hold-orders-closed');
    expect(refusal).toBeInstanceOf(CommerceHoldRefusal);
    expect(refusal).toMatchObject({message: 'CHECKOUT_CONFLICT', reason: 'orders_closed', required: undefined, balance: undefined});
    expect(reply('hold-orders-closed')).toMatchObject({status: 409});
    expect(reply('hold-orders-closed').retryAfter).toBeUndefined();
  },
  'hold-cart-released': async () => {
    // The cart this request made is closed without a payment: the request is over, and a new one is needed.
    const refusal = await reserve('hold-cart-released');
    expect(refusal).toBeInstanceOf(CommerceHoldRefusal);
    expect(refusal).toMatchObject({message: 'CHECKOUT_CONFLICT', reason: 'cart_released', required: undefined, balance: undefined});
    // The status is part of the answer: 404 would read as a session that ended (the board would be asked to prove the
    // account again), 429 or 503 as "again shortly". 409 is neither, and names no time to wait.
    expect(reply('hold-cart-released')).toMatchObject({status: 409});
    expect(reply('hold-cart-released').retryAfter).toBeUndefined();
    expect(entranceBusy(reply('hold-cart-released'))).toBe(false);
    expect(await heldBack('hold-cart-released')).toBe(false); // the new request the app makes is sent at once
  },
  'hold-other-cart': async () => {
    // One unpaid cart for each account: the plain code, with no reason and no numbers.
    const refusal = await reserve('hold-other-cart');
    expect(refusal).not.toBeInstanceOf(CommerceHoldRefusal);
    expect(refusal).toMatchObject({message: 'CHECKOUT_CONFLICT'});
    expect(reply('hold-other-cart')).toMatchObject({status: 409});
  },

  // --- an unpaid cart taken back ---
  'cancel-released': () => { expect(readCartCancel(reply('cancel-released'), sale)).toBe('released'); },
  'cancel-quote-issued': () => { expect(readCartCancel(reply('cancel-quote-issued'), sale)).toBe('quote-issued'); },
  'cancel-paid': () => { expect(readCartCancel(reply('cancel-paid'), sale)).toBe('paid'); },
  'cancel-not-found': () => { expect(readCartCancel(reply('cancel-not-found'), sale)).toBe('not-found'); },

  // --- a store that is not public gives no challenge: the same answer as for no store, and no session follows ---
  'challenge-not-public': async () => {
    await expect(new SessionEntrance(answering('challenge-not-public')).challenge({purpose: 'checkout', branch: cafe})).rejects.toMatchObject({code: 'REFUSED'});
  },

  // --- the account alone: how many of its orders are unfinished ---
  'challenge-account': async () => {
    expect(bytesToHex(await new CustomerEntrance(answering('challenge-account')).accountChallenge(), false)).toBe(token);
  },
  'proof-account': async () => {
    expect(readOpenOrders(reply('proof-account'))).toBe(1);
    expect(Object.keys(answers['proof-account'].body!).sort()).toEqual(['openOrders', 'purpose']); // the number and nothing else
    const asked: string[] = [];
    const entrance = new CustomerEntrance(async (path, body) => { asked.push(`${path} ${body}`); return path === '/v2/customer/challenge' ? reply('challenge-account') : reply('proof-account'); });
    expect(await accountOpenOrders(entrance, {open: async () => board()}, account)).toBe(1);
    expect(asked).toEqual(['/v2/customer/challenge {"purpose":"account"}', `/v2/customer/proof {"challenge":"${token}","account":"${account}","signature":"${signature}"}`]);
  },

  // --- the pickup proof ---
  'pickup-not-ready': async () => { expect(await pickup('pickup-not-ready')).toEqual({kind: 'not-ready'}); },
  'pickup-collected': async () => { expect(await pickup('pickup-collected')).toEqual({kind: 'collected', at: seconds}); },
  'pickup-token': async () => { expect(await pickup('pickup-token')).toEqual({kind: 'qr', qr: `nu54://pickup/${token}`, expiresAt: seconds}); },
};

/** The answer as the native module hands it over: the status, the body as text, and the Retry-After in seconds. */
const native = (name: string) => JSON.stringify({status: answers[name].status, body: answers[name].body === null ? 'Not Found\n' : JSON.stringify(answers[name].body),
  ...(answers[name].retryAfter ? {retryAfter: answers[name].retryAfter} : {})});
const anchor = {origin: 'https://store.example:8444', ca: 'MIIB' + 'A'.repeat(64), pin: 'ab'.repeat(32)};

/** Whether the answer makes the phone hold back the next request of a session. */
async function heldBack(name: string) {
  forgetEntranceWaits();
  const post = nativeEntrance(anchor, {customerRequest: async () => native(name)});
  await post('/v2/customer/commerce', '{"command":"x"}', token);
  return post('/v2/customer/commerce', '{"command":"x"}', token).then(() => false, (error: unknown) => error instanceof EntranceError && error.code === 'BUSY');
}

async function busy(name: string, status: number, wait: number) {
  const answer = reply(name);
  expect(answer).toEqual({status, retryAfter: wait, body: {requestId: '', requestDigest: '', status: 'error', error: 'BUSY'}});
  expect(entranceBusy(answer)).toBe(true);
  expect(entranceRefusal(answer)).toMatchObject({code: 'BUSY', retryAfter: wait});
  // It is "again shortly" wherever it comes: a challenge, a proof, the store list, a store page, the menu, a pickup QR.
  expect(await rejection(new SessionEntrance(answering(name)).challenge({purpose: 'checkout', branch: cafe}))).toMatchObject({code: 'BUSY', retryAfter: wait});
  expect(await rejection(new StoreDirectory(answering(name)).list())).toMatchObject({code: 'BUSY', retryAfter: wait});
  expect(await rejection(new StoreDirectory(answering(name)).page(cafe))).toMatchObject({code: 'BUSY', retryAfter: wait});
  const menu = await rejection((await access(answering(name))).client.publishedMenu());
  expect(menu).toBeInstanceOf(StoreBusy);
  expect(menu).toMatchObject({retryAfter: wait});
  expect(await issuePickupQr(new CustomerEntrance(answering(name)), {open: async () => board()}, {branch: cafe, sale, payer: account})).toEqual({kind: 'busy'});
  // And the phone waits it out: the next request of the same kind is not sent before the time the server named.
  expect(await heldBack(name)).toBe(true);
}

beforeEach(() => { forgetEntranceWaits(); jest.useFakeTimers({now: clock * 1000}); });
afterEach(() => { jest.useRealTimers(); });

test('every answer of the server\'s file is read by the phone, and nothing here reads an answer the file does not hold', () => {
  expect(Object.keys(read).sort()).toEqual(Object.keys(answers).sort());
});

test.each(Object.keys(answers).sort())('the phone reads the server\'s answer %s', async name => {
  const check = read[name];
  expect(typeof check).toBe('function');
  await check();
});
