// One order scenario across the real product code; see scripts/test_order_flow.py. The flows run in
// order on one phone, one board, one kiosk and one store, and each prints one PASS or FAIL line.
import {execFileSync} from 'node:child_process';
import {generateKeyPairSync} from 'node:crypto';
import {writeFileSync, writeSync} from 'node:fs';
import {join} from 'node:path';
import React from 'react';
import {AppState, Platform} from 'react-native';
import Renderer from 'react-test-renderer';
import {encodeOrderLocator, hexToBytes, isWon, readPickupKitchen, type PickupTarget} from '@nu54/protocol';

jest.mock('../../../../p02-user-app/src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: jest.requireActual('./world').commerceHttp}));
jest.mock('../../../../p02-user-app/src/specs/NativeSubscriptionAlerts.ts', () => ({__esModule: true, default: jest.requireActual('./world').alerts}));
jest.mock('../../../../p02-user-app/src/specs/NativeNearby.ts', () => ({__esModule: true, default: jest.requireActual('./world').nearby}));
jest.mock('../../../../p02-user-app/src/specs/NativeOwnerWallet.ts', () => ({__esModule: true, default: jest.requireActual('./world').ownerWallet}));
jest.mock('../../../../p02-user-app/src/specs/NativeRenterBle.ts', () => ({__esModule: true, default: jest.requireActual('./world').renterBle}));
jest.mock('../../../../p02-user-app/src/specs/NativeShopQr.ts', () => ({__esModule: true, default: null}));

import {CustomerOrders} from '../../../../p02-user-app/src/merchant/CustomerOrders';
import {customerClient, refreshOpenOrders} from '../../../../p02-user-app/src/merchant/customerClient';
import {CustomerNearby} from '../../../../p02-user-app/src/merchant/nearbyClient';
import {orderRecords} from '../../../../p02-user-app/src/merchant/orderRecords';
import {lockForSubmit, type PaymentCheckpoint} from '../../../../p02-user-app/src/merchant/paymentCheckpoint';
import {BoardLife, type BoardLifeScreen} from '../../../../p02-user-app/src/wallet/BoardLife';
import {returnRecord} from '../../../../p02-user-app/src/wallet/boardReturn';
import {sendRecord} from '../../../../p02-user-app/src/wallet/boardSend';
import {OwnerSession} from '../../../../p02-user-app/src/wallet/ownerSession';
import {readEvmReceiveBalance} from '../../../../p02-user-app/src/wallet/evmTransfer';
import {decodeWalletStorage} from '../../../../p02-user-app/src/wallet/profile';
import {defaultReadState, readProfiles, walletReadRpc} from '../../../../p02-user-app/src/wallet/readProfiles';
import {orderProgress} from '../../../../p04-merchant-kiosk/src/kiosk/orderProgress';
import {
  anchor, balance, board, boardOpens, boardSPKI, commerceHttp, control, customerProfile, dropNearby, fixture, id, journals, kiosk, nearby, notices, now, offline, ownerWallet,
  pause, phoneCalls, post, token, radio, read, renterBle, startBoard, startKiosk, stopWorld, until, wallet,
} from './world';

/** Short, because a board waits for the end of the last quote it signed before it signs a return. */
const quoteLife = 40;
let lastQuote = 0;
const price = BigInt(fixture.amount);
const store = fixture.payment.beneficiary;
const won = isWon(token);
/** Amounts of these flows are the same for every token in the units its board counts. */
const unit = BigInt(fixture.unit);
/** What an owner types and sees. */
const shown = won ? {deposit: '900,000원', send: '30000', price: '80,000원'} : {deposit: '90.00 TestUSDC', send: '3', price: '8.00 TestUSDC'};
const check = (what: string, value: unknown) => { if (!value) throw new Error('expected ' + what); };
const same = (what: string, seen: unknown, wanted: unknown) => {
  if (String(seen) !== String(wanted)) throw new Error(`${what}: expected ${String(wanted)}, got ${String(seen)}`);
};
const phone = () => customerClient(customerProfile);
const records = () => orderRecords(commerceHttp);

// ---- the phone's screens ----
let tree: Renderer.ReactTestRenderer | undefined;
const texts = () => JSON.stringify(tree?.toJSON() ?? null);
const button = (label: string) => tree?.root.findAll(node => (node.props.label === label && typeof node.props.run === 'function') ||
  ((node.props.title === label || node.props.accessibilityLabel === label) && typeof node.props.onPress === 'function'))[0];
const sees = (text: string, seconds = 40) => until(`"${text}" on the phone (${seen()})`, () => texts().includes(text), seconds);
const seen = () => (texts().match(/[가-힣][^"\\]{3,}/g) ?? []).slice(0, 14).join(' | ');
async function press(label: string, seconds = 20) {
  const found = await until(`button ${label}`, () => { const item = button(label); return item && !item.props.disabled ? item : undefined; }, seconds)
    .catch(error => { throw new Error(`${(error as Error).message}; the phone shows: ${seen()}`); });
  (found.props.run ?? found.props.onPress)();
  await pause(50);
}
async function type(label: string, value: string) {
  const field = await until(`field ${label}`, () => tree?.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onChangeText === 'function')[0]);
  field.props.onChangeText(value);
  await pause(50);
}
async function leave() {
  tree?.unmount();
  tree = undefined;
  await pause(100);
}
async function show(element: React.ReactElement) {
  await leave();
  tree = Renderer.create(element);
  await pause(100);
}
const openOrders = (props: Partial<React.ComponentProps<typeof CustomerOrders>> = {}) =>
  show(<CustomerOrders back={() => {}} initialLocator="" permission={async () => true} {...props} />);

async function connectStore(props: Partial<React.ComponentProps<typeof CustomerOrders>> = {shopEntry: true}) {
  await openOrders(props);
  await press('근처 단말 찾기');
  await press('근거리 단말 후보 1 인증');
  await press('이 매장 메뉴 열기');
  await sees('Order flow cafe 메뉴');
}
/** The payment the phone is working on, as it would find it after a restart. */
const checkpoint = () => {
  const raw = [...journals].find(([name, value]) => /^customer-payment-/.test(name) && value.includes('"submitKey"'))?.[1];
  return raw ? JSON.parse(raw) as PaymentCheckpoint : undefined;
};
/** The board takes no return and no reset while an order it approved could still execute. */
const quotesOver = () => pause(Math.max(0, lastQuote + 4 - now()) * 1000);
/** A cart made on the phone, reserved through the kiosk. */
async function cartOrder() {
  await connectStore();
  await press('Escrow cafe plain 추가');
  await press('주문 접수 요청');
  await sees('결제 내용을 확인하세요');
  const entry = await until('a payment checkpoint', checkpoint);
  lastQuote = Math.max(lastQuote, entry.quote.expiresAt);
  return entry;
}
/** An order chosen on the kiosk's own screen: the phone scans its code and the store confirms the number. */
async function kioskOrder() {
  const {prices} = await kiosk.api.catalogWindow(0);
  const cart = {origin: 'menu' as const, originRef: id(), menuVersion: prices[0].version, items: [{sku: prices[0].sku, option_key: prices[0].optionKey, quantity: 1}]};
  const sale = kiosk.shown = await kiosk.api.reserve(cart, prices[0].asset, id());
  await connectStore({});
  await type('매장 주문 QR 내용', encodeOrderLocator({branchRef: fixture.branch, orderRef: sale, expiresAt: now() + 50}, now()));
  await press('QR 주문 연결');
  const code = await until('a code on the kiosk', () => kiosk.codes[0]);
  await sees(code);
  kiosk.relay.approveDisplayed(code);
  await press('단말 연결 상태 확인');
  await sees('결제 내용을 확인하세요');
  const entry = await until('a payment checkpoint', checkpoint);
  same('the order on the phone', entry.order.id, sale);
  check(`the price ${shown.price} on the phone`, texts().includes(shown.price));
  lastQuote = Math.max(lastQuote, entry.quote.expiresAt);
  return entry;
}
/** The server asks for the proof that nothing was locked and closes the order. Nobody helps it. */
async function closesByItself(entry: PaymentCheckpoint) {
  await pause(Math.max(0, entry.quote.expiresAt - now()) * 1000);
  // A status read at the moment of the close can say `closed` before it names the unpaid proof, so both are awaited.
  await until('an order closed as unpaid by the server', async () => {
    const order = await phone().status(entry.order.id);
    return order.state === 'closed' && order.payment?.stage === 'unpaid';
  }, 60);
  same('locks', (await control<{count: number}>('locks', {orderKey: entry.quote.orderKey})).count, 0);
}
async function checkBoard() {
  await press('내 보드 확인');
  await until('the board checked', () => button('보드로 승인'), 15).catch(error => { throw new Error(`${(error as Error).message}; the phone shows: ${seen()}`); });
}
async function approve() {
  await checkBoard();
  await press('보드로 승인');
}
async function orderNumber(sale: string) {
  await sees('주문이 접수되었습니다', 60);
  const order = (await records().list()).find(row => row.sale === sale);
  check('an order number on the phone', order?.pickupNo);
  check('the same number on the screen', texts().includes(`주문번호 ${order!.pickupNo}`));
  const onKiosk = await kiosk.api.status(sale);
  same('number on the kiosk', onKiosk.pickupNo, order!.pickupNo);
  same('public screen', orderProgress({sale: onKiosk}, now()), 'paid');
  await press('확인');
  await leave();
  return order!.pickupNo!;
}
/** A paid order, from the cart to its number. */
async function paidOrder() {
  const entry = await cartOrder();
  await approve();
  await orderNumber(entry.order.id);
  await oneLock(entry);
  return entry;
}
async function oneLock(entry: PaymentCheckpoint) {
  const lock = await control<{count: number; payer: string; confirmer: string}>('locks', {orderKey: entry.quote.orderKey});
  same('locks on chain', lock.count, 1);
  same('payer on chain', lock.payer, board.account);
  same('phone key on chain', lock.confirmer, entry.confirmer);
}
/** What the app does every 20 seconds for an order in progress. */
async function follow(sale: string) {
  return (await refreshOpenOrders(phone())).find(row => row.sale === sale)!;
}
const stage = (sale: string, wanted: string, seconds = 60) => until(`${wanted} on the phone`, async () => {
  const order = await follow(sale);
  return order.stage === wanted ? order : undefined;
}, seconds);

// ---- the store ----
async function kitchen(operation: string, more: object = {}) {
  const {status, raw} = await post(fixture.kitchen, '/v2/commerce', JSON.stringify({travel: {operation, branch: fixture.branch,
    ...(operation === 'pickup-queue' ? {includeCafe: true} : {}), ...more}}));
  same(operation, `${status} ${status === 200 ? '' : raw}`, '200 ');
  return raw;
}
const kitchenOrder = async (sale: string) => readPickupKitchen(await kitchen('pickup-queue')).find(row => row.saleRef === sale);
async function cook(sale: string, ...targets: PickupTarget[]) {
  for (const target of targets) {
    const row = await until('the order in the kitchen', async () => { const found = await kitchenOrder(sale); return found && found.state !== 'checking' ? found : undefined; });
    await kitchen('pickup-change', {sale, requestKey: id(), expectedRevision: row.revision, target});
  }
}
async function deliver(sale: string) {
  await cook(sale, 'preparing', 'ready');
  await until('a ready notice', async () => { await follow(sale); return notices.includes(sale); });
  await cook(sale, 'collected');
  return stage(sale, 'delivered');
}
/** The store cannot make it: the money goes back to the board account. */
async function cancelled(sale: string) {
  await cook(sale, 'preparing', 'unfulfillable');
  same('refund', (await stage(sale, 'refunded')).refund, 'store');
}
/** Finance replies with amounts name the store's token, and the token of no other store. */
const amountReplies = ['escrow-orders', 'escrow-claims', 'report', 'order'];
async function finance<T = Record<string, unknown>>(who: typeof fixture.finance1, operation: string, body: object, status = 200) {
  const reply = await post(who, '/v2/finance/' + operation, JSON.stringify({branchId: fixture.branch, ...body}));
  same(operation, `${reply.status} ${reply.status === status ? '' : reply.raw}`, `${status} `);
  if (status === 200 && amountReplies.includes(operation)) {
    const {token: named, tokenAsset} = JSON.parse(reply.raw) as {token?: object; tokenAsset?: string};
    same(`the token of ${operation}`, JSON.stringify(named), JSON.stringify(fixture.payment.token && (fixture.shop === 'refused' ? {name: '', symbol: '', label: '', decimals: 0} : token)));
    same(`the asset of ${operation}`, tokenAsset, fixture.payment.token && fixture.payment.asset);
  }
  return (status === 200 ? JSON.parse(reply.raw) : {}) as T;
}
/** The branches a finance operator may see, with the token of each. */
async function financeScopes() {
  const {status, raw} = await read(fixture.finance1, '/v2/finance/scopes');
  same('finance scopes', status, 200);
  return JSON.parse(raw) as {id: string; token?: object; asset?: string}[];
}
async function answer(sale: string, ...labels: string[]) {
  await openOrders({initialOrder: sale});
  for (const label of labels) await press(label);
  await sees('답을 확인하고 있습니다');
  await leave();
}

// ---- the board pages, with what WalletHome does around them ----
const life = {message: '', went: [] as string[], copied: '', seen: undefined as {account: string; generation: number; at: number} | undefined,
  found: [] as {address: string; name: string}[], busy: false, error: undefined as Error | undefined};
let screen: BoardLifeScreen = 'setup';
const stored = () => decodeWalletStorage(wallet.raw);
const enrollment = () => stored().enrollment!;
async function withBoard<T>(handle: string, run: (session: OwnerSession) => Promise<T>) {
  const session = new OwnerSession(renterBle as never, ownerWallet as never, enrollment());
  try {
    await session.connect(handle).catch(error => { throw session.reached === 'bluetooth' ? new Error('BOARD_PERMIT') : error; });
    return await run(session);
  } finally {
    session.close();
  }
}
function page() {
  return <BoardLife screen={screen} stored={stored()} native={ownerWallet as never} hasEnrollment found={life.found} busy={life.busy}
    scan={() => { life.found = [{address: 'board-1', name: 'NU54DK'}]; draw(); }} go={next => { life.went.push(next); if (['setup', 'return-address', 'deposit', 'send', 'return', 'reset', 'operator-reset'].includes(next)) { screen = next as BoardLifeScreen; draw(); } }}
    orders={() => { life.went.push('orders'); }} say={text => { life.message = text; }} wait={() => {}} copy={text => { life.copied = text; }}
    work={(run, failure) => {
      if (life.busy) return;
      life.busy = true;
      life.message = '';
      draw();
      run().catch(error => { life.message = failure(error) || (error as Error).message; life.error = error as Error; }).finally(() => { life.busy = false; draw(); });
    }}
    withBoard={withBoard} commit={async change => { wallet.raw = JSON.stringify(decodeWalletStorage(JSON.stringify(change(stored())))); draw(); }}
    seenBoard={life.seen} seen={(account, generation) => { life.seen = {account, generation, at: Date.now()}; draw(); }} />;
}
const draw = () => { tree?.update(page()); };
async function openLife(next: BoardLifeScreen) {
  screen = next;
  life.found = [];
  life.message = '';
  life.error = undefined;
  await show(page());
}
async function onBoard(label: string) {
  if (!button(label)) await press('내 보드 찾기');
  await press(label);
}
const says = (text: string, seconds = 30) => until(`"${text}" (message: ${life.message})`, () => life.message.includes(text), seconds)
  .catch(() => { throw new Error(`expected the message "${text}", got "${life.message}" ${life.error?.message ?? ''}; the page shows: ${seen()}`); });
async function register() {
  wallet.raw = JSON.stringify({version: 2, exercised: false, mode: 'personal',
    enrollment: {version: 2, device: board.device, boardSPKI, ca: 'AAAA', certificateChain: ['AAAA']},
    walletRead: defaultReadState});
  // A board without an owner holds no wallet key and advertises for its owner by itself; the registration makes the key.
  same('account of a board without an owner', board.account, '0x' + '0'.repeat(40));
  check('a board that waits for its owner', board.waitsForOwner);
  board.generation = (await withBoard('board-1', session => session.register(() => {}))).generation;
  check('an account made by the registration', /^0x[0-9a-f]{40}$/.test(board.account) && !/^0x0{40}$/.test(board.account));
  check('no waiting for an owner any more', !board.waitsForOwner);
}
async function enroll() {
  await openLife('setup');
  await onBoard('서명 시험 하기');
  await says('보드 서명을 확인했습니다');
  same('account on the phone', stored().verified?.address, board.account);
  await openLife('return-address');
  await type('원래 지갑 주소', fixture.home);
  await type('원래 지갑 주소 다시', fixture.home);
  await press('주소 확인');
  await onBoard('보드에 저장');
  await says('보드에 반납받을 주소를 저장했습니다');
  await openLife('setup');
  await onBoard('이 매장에 보드 등록');
  await says('이 매장에 보드 계정을 등록했습니다');
  const held = await phone().enrolledPayer();
  same('account at the store', held?.payer, board.account);
  same('generation at the store', held?.generation, board.generation);
  await leave();
}
async function deposit(amount: bigint) {
  const before = await balance(board.account);
  await control('deposit', {address: board.account, amount: amount.toString()});
  same('board account after the deposit', await balance(board.account), before + amount);
  // The app only counts money in final blocks.
  const rpc = walletReadRpc('https://rpc.test', (url, packet) => ownerWallet.evmRpc(url, packet));
  await until('a final deposit', async () => BigInt((await readEvmReceiveBalance(rpc, '8283', readProfiles.stable.anchor, board.account,
    fixture.payment.asset)).amount) === before + amount);
}
// ---- running ----
let authority = '';
let ready = false;
let failed = 0;
const started = Date.now();
/** One named flow. A flow longer than the usual three minutes says how long it may take. */
function flow(name: string, run: () => Promise<void>, timeout?: number) {
  test(name, async () => {
    const at = Date.now();
    let passed = false;
    const line = (word: string, detail = '') => writeSync(1, `ORDER-FLOW ${word} ${fixture.shops > 1 ? fixture.shop + ': ' : ''}${name} (${((Date.now() - at) / 1000).toFixed(1)}s)${detail}\n`);
    try {
      check('a started world', ready);
      await run();
      passed = true;
      line('PASS');
    } catch (error) {
      failed++;
      line('FAIL', ' ' + (error instanceof Error ? error.message : String(error)).replace(/\s+/g, ' '));
      throw error;
    } finally {
      await leave();
      board.answer = 'press';
      board.dropAfterSigning = false;
      radio.loseReplyTo = undefined;
      kiosk.shown = undefined;
      dropNearby();
      await control('quote-lifetime', {seconds: quoteLife}).catch(() => {});
      // A failed flow must not block the next one with its unfinished payment.
      if (!passed) for (const [key, value] of [...journals]) if (/^customer-payment-/.test(key) || value.includes('"originRef"')) journals.delete(key);
    }
  }, timeout);
}

beforeAll(async () => {
  Object.defineProperty(Platform, 'OS', {value: 'android', configurable: true});
  jest.spyOn(AppState, 'addEventListener').mockReturnValue({remove() {}} as never);
  jest.spyOn(console, 'error').mockImplementation(() => {}); // React asks for act(); these flows run in real time
  (globalThis as {IS_REACT_ACT_ENVIRONMENT?: boolean}).IS_REACT_ACT_ENVIRONMENT = false;
  anchor.hash = readProfiles.stable.anchor;
  const key = generateKeyPairSync('ec', {namedCurve: 'prime256v1'});
  authority = join(fixture.directory, 'authority.pem');
  writeFileSync(authority, key.privateKey.export({type: 'pkcs8', format: 'pem'}), {mode: 0o600});
  await startBoard(key.publicKey.export({type: 'spki', format: 'der'}).subarray(-65).toString('hex'));
  await startKiosk();
  await control('quote-lifetime', {seconds: quoteLife});
  ready = true;
});
afterAll(async () => {
  await leave();
  stopWorld();
  writeSync(1, `ORDER-FLOW DONE ${failed} failed, ${((Date.now() - started) / 1000).toFixed(0)}s, ${radio.seen.length} requests at the kiosk\n`);
});

flow('enroll the board account', async () => {
  await register();
  await enroll();
});

flow('deposit to the board account', async () => {
  await deposit(90_000_000n * unit);
  await openLife('deposit');
  // A token that other networks may also have is named before any address.
  if (won) {
    await sees(`${token.label}는 StableNet 테스트넷에만 있습니다`);
    check('no address before the confirmation', !texts().includes(board.account));
    await press('확인했습니다. 주소 보기');
  }
  if (!texts().includes(board.account)) await onBoard('보드 연결해 확인');
  // The page reads the finalized balance, a few blocks behind.
  await until('the deposit on the screen', async () => {
    if (texts().includes(shown.deposit)) return true;
    const again = button('잔액 다시 확인');
    if (again && !again.props.disabled) again.props.run();
    return pause(1000).then(() => false);
  });
  check('the address on the screen', texts().includes(board.account));
});

flow('kiosk order, paid to the store', async () => {
  const before = await balance(store);
  const entry = await kioskOrder(), sale = entry.order.id;
  const asked = board.prompts;
  await approve();
  const number = await orderNumber(sale);
  same('board prompts', board.prompts - asked, 1);
  same('locks through the kiosk', kiosk.locks, 1);
  check('no lock sent around the kiosk', !phoneCalls.includes('order.lock'));
  await oneLock(entry);
  const row = await until('the order in the kitchen', async () => { const found = await kitchenOrder(sale); return found?.state === 'accepted' ? found : undefined; });
  same('number in the kitchen', row.pickupNo, number);
  check('the goods in the kitchen', row.items?.length === 1 && row.escrow?.stage === 'locked');
  const delivered = await deliver(sale);
  check('a confirmation period', delivered.claimableAt! > now());
  await answer(sale, '받았어요', '네, 받았어요');
  const paid = await stage(sale, 'paid');
  same('payout', paid.payout, 'receipt');
  same('store income', await balance(store) - before, price);
});

flow('options picked on the phone, named in the kitchen', async () => {
  // The same amount goes in first, so the balance the later flows expect stays as it was.
  const dearer = price + price / 8n, before = await balance(store);
  await deposit(dearer);
  await connectStore();
  await press('옵션 라떼 옵션 고르기');
  await press('온도 차갑게');
  await press(won ? '크기 크게 +10,000원' : '크기 크게 +1.00 TestUSDC');
  await press(won ? '담기 · 90,000원' : '담기 · 9.00 TestUSDC');
  await press('주문 접수 요청');
  await sees('결제 내용을 확인하세요');
  const entry = await until('a payment checkpoint', checkpoint), sale = entry.order.id;
  lastQuote = Math.max(lastQuote, entry.quote.expiresAt);
  same('the amount of the combination', entry.order.amount, dearer);
  await approve();
  const number = await orderNumber(sale);
  await oneLock(entry);
  const row = await until('the order in the kitchen', async () => { const found = await kitchenOrder(sale); return found?.state === 'accepted' ? found : undefined; });
  same('number in the kitchen', row.pickupNo, number);
  same('the combination in the kitchen', JSON.stringify(row.items), JSON.stringify([{label: 'Option latte', option: 'ice-l', optionLabel: '차갑게 · 크게', quantity: 1}]));
  await deliver(sale);
  await answer(sale, '받았어요', '네, 받았어요');
  await stage(sale, 'paid');
  same('store income', await balance(store) - before, dearer);
});

flow('refused on the board, then a second order and store cancel', async () => {
  const before = await balance(board.account), submissions = (await control<{count: number}>('submissions')).count;
  board.answer = 'refuse';
  await control('quote-lifetime', {seconds: 12});
  const refused = await cartOrder();
  await control('quote-lifetime', {seconds: quoteLife});
  await checkBoard();
  const lamps = board.lamps.length;
  await press('보드로 승인');
  await sees('보드에서 거절했습니다');
  check('no signature lamp', !board.lamps.slice(lamps).includes(1));
  same('money after the refusal', await balance(board.account), before);
  same('requests at the server', (await control<{count: number}>('submissions')).count, submissions);
  same('locks of the refused order', (await control<{count: number}>('locks', {orderKey: refused.quote.orderKey})).count, 0);
  // The customer closes the page and orders again on the kiosk, without waiting for the first quote to end.
  board.answer = 'press';
  await press('결제 화면 닫기');
  const second = await kioskOrder();
  await approve();
  await orderNumber(second.order.id);
  await oneLock(second);
  check('another order', second.order.id !== refused.order.id);
  same('money held', await balance(board.account), before - price);
  await cancelled(second.order.id);
  // The refused cart is still on the phone; giving it up frees the cart.
  await connectStore();
  await press('새로 주문하기');
  await until('a free cart', () => button('주문 접수 요청') || button('Escrow cafe plain 추가'));
  await closesByItself(refused);
  same('money after the cancel', await balance(board.account), before);
});

flow('connection lost after signing', async () => {
  const entry = await cartOrder();
  const asked = board.prompts, sent = kiosk.locks;
  board.dropAfterSigning = true;
  await approve();
  await sees('보드 연결이 끊겼습니다');
  same('signatures', board.lamps.filter(lamp => lamp === 1).length > 0, true);
  same('locks before the answer arrived', kiosk.locks, sent);
  // The app is closed and opened again, away from the kiosk; the same board hands over the stored answer.
  await leave();
  await openOrders({shopEntry: true});
  await sees('보드 승인을 이어서 확인합니다');
  board.answer = 'ignore';
  await approve();
  await orderNumber(entry.order.id);
  same('board prompts', board.prompts - asked, 1);
  await oneLock(entry);
  await cancelled(entry.order.id);
});

flow('duplicate lock', async () => {
  const entry = await cartOrder();
  const sent = kiosk.locks;
  radio.loseReplyTo = 'lock';
  await approve();
  const open = await until('a sent payment', () => { const item = checkpoint(); return item?.forwardAttempted && kiosk.locks > sent ? item : undefined; });
  // The kiosk took the lock and its answer was lost. The page offers the same payment again, over the internet.
  const {payment, confirmer, lockDigest, signature} = lockForSubmit(open, fixture.payment, now());
  const request = {op: 'lock' as const, branch: fixture.branch, key: open.submitKey, order: open.order.id, quoteDigest: payment.digest, confirmer, lockDigest, signature};
  const peer = {kind: 'ready' as const, link: 'again', epoch: '', peer: '', principal: fixture.customer.subject, branch: fixture.branch, expiresAt: fixture.expiresAt};
  for (let n = 0; n < 2; n++) same('the same lock through the kiosk', (await kiosk.relay.handle(request, peer)).status, 'ok');
  // The page offers the same payment again, over the internet; the server holds the kiosk's request already.
  for (const key of [open.submitKey, open.submitKey, id()])
    same('the same lock from the phone', await phone().lock(open.order.id, payment.digest, confirmer, lockDigest, signature, key), open.order.id);
  const swapped = await kiosk.relay.handle({...request, confirmer: '0x' + '5'.repeat(40)}, peer);
  same('a lock with another phone key', swapped.status, 'denied');
  await orderNumber(entry.order.id);
  await oneLock(entry);
  same('records', (await records().list()).filter(row => row.sale === entry.order.id).length, 1);
  await cancelled(entry.order.id);
});

flow('same lock by both routes, phone first', async () => {
  const entry = await cartOrder();
  const sent = kiosk.locks;
  await checkBoard();
  dropNearby(); // the customer walks away from the kiosk before the board answers
  await pause(300);
  await press('보드로 승인');
  const open = await until('a sent payment', () => { const item = checkpoint(); return item?.forwardAccepted ? item : undefined; });
  same('locks through the kiosk', kiosk.locks, sent);
  check('the lock went over the internet', phoneCalls.includes('order.lock'));
  const {payment, confirmer, lockDigest, signature} = lockForSubmit(open, fixture.payment, now());
  const reply = await kiosk.relay.handle({op: 'lock', branch: fixture.branch, key: open.submitKey, order: open.order.id, quoteDigest: payment.digest, confirmer, lockDigest, signature},
    {kind: 'ready', link: 'again', epoch: '', peer: '', principal: fixture.customer.subject, branch: fixture.branch, expiresAt: fixture.expiresAt});
  same('the same lock through the kiosk', JSON.stringify(reply), JSON.stringify({status: 'ok', value: {submitted: open.order.id}}));
  await orderNumber(entry.order.id);
  await oneLock(entry);
  await cancelled(entry.order.id);
});

flow('refused quote closes by itself, new order at once', async () => {
  const stock = async () => (await kiosk.api.catalogWindow(0)).prices[0].available;
  const before = await stock();
  await control('quote-lifetime', {seconds: 12});
  board.answer = 'refuse';
  const refused = await cartOrder();
  await approve();
  await sees('보드에서 거절했습니다');
  same('stock held', await stock(), before - 1);
  board.answer = 'press';
  await control('quote-lifetime', {seconds: quoteLife});
  await press('새로 주문하기');
  await sees('새로 주문할 수 있습니다');
  await press('주문 접수 요청'); // the same cart is still chosen
  await sees('결제 내용을 확인하세요');
  const second = await until('a new order', () => { const item = checkpoint(); return item && item.order.id !== refused.order.id ? item : undefined; });
  lastQuote = Math.max(lastQuote, second.quote.expiresAt);
  await approve();
  await orderNumber(second.order.id);
  await oneLock(second);
  await closesByItself(refused);
  same('stock given back', await stock(), before - 1);
  await cancelled(second.order.id);
});

flow('quote expiry', async () => {
  const before = await balance(board.account), held = (await control<{amount: string}>('held')).amount;
  await control('quote-lifetime', {seconds: 8});
  const entry = await cartOrder();
  board.answer = 'ignore';
  await checkBoard();
  await press('보드로 승인');
  await sees('보드에서 승인해 주세요');
  await sees('결제 기한이 지났습니다', 30);
  await control('quote-lifetime', {seconds: quoteLife});
  same('money after the expiry', await balance(board.account), before);
  same('held money after the expiry', (await control<{amount: string}>('held')).amount, held);
  same('locks', (await control<{count: number}>('locks', {orderKey: entry.quote.orderKey})).count, 0);
  board.answer = 'press';
  // A late press on the board cannot sign it either.
  const lamps = board.lamps.length;
  await board.command('press');
  check('no late signature', !board.lamps.slice(lamps).includes(1));
  await press('새로 주문하기');
  await sees('새로 주문할 수 있습니다');
  await leave();
  await closesByItself(entry);
  same('money after the proof', await balance(board.account), before);
});

flow('a grant that took 12 seconds', async () => {
  const account = board.account;
  await board.boot();
  // A restarted board keeps the operator's grant, the owner and the owner's key. Its clock is late until the next grant.
  check('a session under the grant kept through the restart', await boardOpens());
  same('account after the restart', board.account, account);
  await board.grant(12);
  same('seconds the board\'s clock is ahead', board.ahead, 14);
  await withBoard('board-1', session => session.spendingAccount({generation: board.generation} as never));
  const entry = await paidOrder();
  await cancelled(entry.order.id);
  await board.boot();
  await board.grant(58); // almost the longest one the board accepts
  same('seconds the board\'s clock is ahead', board.ahead, 60);
  await withBoard('board-1', session => session.spendingAccount({generation: board.generation} as never));
  await board.boot();
  // The slow grant is kept through the restart, and its width with it: a minute ahead, no quote of this store can be
  // approved. A fresh grant is the operator's word about the time and sets both ends of the board's interval, so the
  // quick one brings the latest possible time down again and the orders of the flows after this one are taken.
  check('a session under the slow grant kept through the restart', await boardOpens());
  check('a board that is a minute ahead', board.ahead > quoteLife);
  await board.grant();
  same('seconds the board\'s clock is ahead', board.ahead, 4);
});

flow('40 requests in a row to the board', async () => {
  await withBoard('board-1', async session => { for (let n = 0; n < 40; n++) same('state', (await session.status()).state, 'active'); });
  for (let n = 0; n < 40; n++) await withBoard('board-1', session => session.status());
  const entry = await paidOrder();
  await cancelled(entry.order.id);
});

flow('140 nearby requests at the kiosk', async () => {
  const ask = async (times: number) => {
    const near = new CustomerNearby(nearby as never, fixture.branch);
    try {
      const link = await near.connect('kiosk');
      for (let n = 0; n < times; n++) same('reply', (await link.request({op: 'discover', branch: fixture.branch}, hexToBytes(id().replace(/-/g, '')))).status, 'ok');
    } finally {
      near.close();
    }
  };
  for (let n = 0; n < 14; n++) await ask(10);
  const own = radio.identity;
  for (let n = 0; n < 40; n++) { // more phones than the kiosk remembers
    radio.identity = (n + 16).toString(16).padStart(64, '0');
    await ask(2);
  }
  radio.identity = own;
  const entry = await paidOrder();
  await cancelled(entry.order.id);
});

flow('dispute, two people, refund to the board', async () => {
  const before = await balance(board.account);
  const entry = await paidOrder(), sale = entry.order.id;
  await deliver(sale);
  await answer(sale, '못 받았어요');
  await stage(sale, 'disputed');
  // Both finance pages read the order with its token.
  await finance(fixture.finance1, 'order', {saleId: sale});
  await finance(fixture.finance1, 'escrow-orders', {});
  const scopes = await financeScopes();
  same('finance branches', scopes.map(row => row.id).join(), fixture.branch);
  same('the token of the branch', JSON.stringify(scopes[0].token), JSON.stringify(fixture.payment.token && token));
  same('the asset of the branch', scopes[0].asset, fixture.payment.token && fixture.payment.asset);
  const propose = {saleId: sale, outcome: 'refund-buyer', reason: 'not_delivered', idempotencyKey: id()};
  const made = await finance<{decisionId: string; state: string}>(fixture.finance1, 'escrow-resolve', propose);
  await finance(fixture.finance1, 'escrow-resolve-approve', {decisionId: made.decisionId, idempotencyKey: id()}, 404);
  const approved = await finance(fixture.finance2, 'escrow-resolve-approve', {decisionId: made.decisionId, idempotencyKey: id()});
  same('decision', approved.state, 'approved');
  const done = await stage(sale, 'refunded');
  same('refund', done.refund, 'arbiter');
  same('money back on the board account', await balance(board.account), before);
});

if (fixture.shops > 1) {
  flow('test tokens from the faucet', async () => {
    const before = await balance(board.account);
    const given = async () => (await phone().faucetStatus()).tokens.find(item => item.token === fixture.payment.asset)!;
    const offered = await given();
    same('the token the faucet gives', `${offered.symbol} ${offered.decimals} ${offered.state}`, `${token.symbol} ${token.decimals} ready`);
    await phone().claimFaucet(offered.token, id());
    await until('test tokens on the board account', async () => await balance(board.account) === before + BigInt(offered.amount), 60);
    await until('a final claim', async () => (await given()).claim?.state === 'final', 60);
    const again = await phone().claimFaucet(offered.token, id()).then(() => 'accepted', error => (error as Error).message);
    check(`a second claim on one day refused (${again})`, again !== 'accepted');
    same('money after the second claim', await balance(board.account), before + BigInt(offered.amount));
  });
}

/** One customer's order through the kitchen and the customer's answer. */
async function servedOrder() {
  const entry = await paidOrder(), sale = entry.order.id;
  await deliver(sale);
  await answer(sale, '받았어요', '네, 받았어요');
  same('payout', (await stage(sale, 'paid')).payout, 'receipt');
  return sale;
}
/** The ready board the kiosk reads shows exactly the numbers the kitchen has ready. */
async function boardMatchesKitchen() {
  const board = await kiosk.api.readyBoard();
  const ready = readPickupKitchen(await kitchen('pickup-queue')).filter(row => row.state === 'ready').map(row => row.pickupNo);
  same('ready numbers on the board', [...board.ready].sort().join(','), [...ready].sort().join(','));
}

flow('thirty customers: served, cancelled by the store, and served by the automatic kitchen', async () => {
  const before = await balance(store);
  // Twenty-seven are paid out; the three cancelled come back to the board account.
  await deposit(27n * price);
  for (let i = 0; i < 3; i++) {
    const entry = await paidOrder();
    await cancelled(entry.order.id);
  }
  for (let i = 0; i < 25; i++) {
    const entry = await paidOrder(), sale = entry.order.id;
    await cook(sale, 'preparing', 'ready');
    if (i % 5 === 0) await boardMatchesKitchen();
    await cook(sale, 'collected');
    await stage(sale, 'delivered');
    await answer(sale, '받았어요', '네, 받았어요');
    await stage(sale, 'paid');
  }
  // A test store's automatic kitchen takes the last two from received to handed over by itself.
  await control('auto-kitchen', {on: true});
  try {
    const last = [await paidOrder(), await paidOrder()].map(entry => entry.order.id);
    for (const sale of last) {
      await stage(sale, 'delivered', 150);
      await answer(sale, '받았어요', '네, 받았어요');
      await stage(sale, 'paid');
    }
  } finally {
    await control('auto-kitchen', {on: false});
  }
  same('store income for 27 orders', await balance(store) - before, 27n * price);
}, 600_000);

flow('five interruptions: the server stops and comes back at each', async () => {
  const before = await balance(store);
  await deposit(4n * price);
  // After the refund is sent: the cancel reaches the chain, the worker dies, a new one finishes the refund.
  const refunded = await paidOrder();
  await control('crash-after-send', {method: 'cancel'});
  await cancelled(refunded.order.id);
  // After the lock is sent: the lock reaches the chain and the worker dies before it records the send.
  await control('crash-after-send', {method: 'lock'});
  await servedOrder();
  // After it is included, before it is recorded: the worker dies after the send and the server stays down while blocks come.
  await control('crash-after-send', {method: 'lock'});
  const included = await cartOrder();
  await approve();
  await until('the lock on chain', async () => (await control<{count: number}>('locks', {orderKey: included.quote.orderKey})).count === 1);
  await control('hold', {seconds: 8});
  await orderNumber(included.order.id);
  await oneLock(included);
  await deliver(included.order.id);
  await answer(included.order.id, '받았어요', '네, 받았어요');
  await stage(included.order.id, 'paid');
  // After the kitchen started: the server stops, then the order goes on from where it was.
  const started = await paidOrder();
  await cook(started.order.id, 'preparing');
  await control('hold', {seconds: 8});
  await pause(9000);
  await cook(started.order.id, 'ready');
  await until('a ready notice', async () => { await follow(started.order.id); return notices.includes(started.order.id); });
  await cook(started.order.id, 'collected');
  await stage(started.order.id, 'delivered');
  await answer(started.order.id, '받았어요', '네, 받았어요');
  await stage(started.order.id, 'paid');
  // After the hand-over is recorded: the delivery call reaches the chain and the worker dies; a new one records it.
  const handed = await paidOrder();
  await control('crash-after-send', {method: 'markDelivered'});
  await deliver(handed.order.id);
  await answer(handed.order.id, '받았어요', '네, 받았어요');
  await stage(handed.order.id, 'paid');
  same('the sends the worker died after', (await control<{methods: string[]}>('crashes')).methods.join(','), 'cancel,lock,lock,markDelivered');
  same('store income for 4 orders', await balance(store) - before, 4n * price);
});

flow('send to another address, refused on the board, then a lost answer', async () => {
  const friend = '0x' + 'f1'.repeat(20), before = await balance(board.account);
  const fill = async (amount: string) => {
    await openLife('send');
    await type('받는 주소', friend);
    await type('금액', amount);
    await press('다음');
    await press('이 내용으로 보내기');
  };
  board.answer = 'refuse';
  await fill(shown.send);
  await onBoard('보드로 승인');
  await says('보드에서 거절했습니다');
  board.answer = 'press';
  await press('보내기 취소');
  await says('보내지 않았습니다');
  same('nothing left the account', await balance(board.account), before);
  const asked = board.prompts;
  await fill(shown.send);
  offline.phone = true; // the board signs, the store cannot be reached
  await onBoard('보드로 승인');
  await until('a transfer on its way', async () => (await sendRecord(commerceHttp).load())?.stage === 'sent');
  await until('a quiet page', () => !life.busy);
  offline.phone = false;
  await press('같은 거래 다시 보내기');
  await until('the transfer to be final', async () => (await sendRecord(commerceHttp).load())?.stage === 'final' || (button('지금 확인') && !life.busy && (button('지금 확인')!.props.run(), false)), 60);
  same('board prompts', board.prompts - asked, 1);
  same('received', await balance(friend), 3_000_000n * unit);
  same('left on the board account', await balance(board.account), before - 3_000_000n * unit);
  await openLife('send');
  await press('확인');
  const held = await phone().walletSends();
  same('transfers left today', held.remaining, held.perDay - 1);
  await leave();
});

/** What the server would say at a start whose settings for this store name another contract, or no token. */
const takeBack = () => control<{anotherContract: string; withoutToken: string}>('take-back');
flow('return, blocked while an order is open, then final; owner reset', async () => {
  const entry = await paidOrder(), sale = entry.order.id;
  await openLife('return');
  await sees('끝나지 않은 주문이 있습니다');
  check('no start with another contract', /has \d+ open orders of another payment contract/.test((await takeBack()).anotherContract));
  // The server refuses it as well, whatever the phone thinks. The board signs once the payment it approved has run out.
  const before = await balance(board.account);
  await quotesOver();
  const transfer = {payer: board.account, to: fixture.home, amount: before, validBefore: now() + 300, nonce: '7'.repeat(64)};
  const signature = await withBoard('board-1', session => session.signReturn(board.generation, {...transfer, token: fixture.payment.asset}, () => {}, token.name, token));
  const refusal = await phone().returnWallet({...transfer, amount: before.toString()}, signature, id()).then(() => 'accepted', error => (error as Error).message);
  same('the server with an open order', refusal, 'CHECKOUT_CONFLICT');
  await leave();
  await deliver(sale);
  await answer(sale, '받았어요', '네, 받았어요');
  await stage(sale, 'paid');
  const home = await balance(fixture.home);
  await openLife('return');
  await press('반납 시작');
  await onBoard('보드로 승인');
  await until('the return to be final', async () => (await returnRecord(commerceHttp).load())?.stage === 'final' || (button('지금 확인') && !life.busy && (button('지금 확인')!.props.run(), false)), 60);
  same('returned to the ordinary wallet', await balance(fixture.home) - home, before);
  same('left on the board account', await balance(board.account), 0);
  // With every order over the store can be taken back, but never by removing only its token: it would sign as dollars.
  await until('a store that can be taken back', async () => !(await takeBack()).anotherContract);
  same('the same contract without its token', (await takeBack()).withoutToken, fixture.payment.token ? 'invalid' : 'unverified');
  const account = board.account, generation = board.generation;
  await openLife('return');
  await onBoard('보드 초기화');
  await says('보드를 초기화했습니다');
  await until('a restarted board', () => board.account !== account);
  check('a newer generation', board.generation > generation);
  check('no account on the phone', !stored().verified);
});

flow('no answer, claim batch', async () => {
  await board.grant(); // the reset restarted the board, so the operator permits it again
  await register();
  await enroll();
  await deposit(20_000_000n * unit);
  const entry = await paidOrder(), sale = entry.order.id;
  await deliver(sale);
  const before = await balance(store);
  type Batch = {batchId: string; state: string; paid?: string; skipped: {saleId: string; reason: string}[]};
  const claim = async () => {
    const {batchId} = await finance<Batch>(fixture.finance1, 'escrow-claim', {saleIds: [sale], idempotencyKey: id()});
    return until('the batch to be final', async () => (await finance<{items: Batch[]}>(fixture.finance1, 'escrow-claims', {})).items
      .find(item => item.batchId === batchId && item.state === 'final'), 60);
  };
  const early = await claim();
  same('paid while the customer can still answer', early.paid, 0);
  same('why', early.skipped.find(row => row.saleId === sale)?.reason, 'window-open');
  await control('warp', {seconds: 49 * 3600});
  same('paid by the batch', (await claim()).paid, price);
  const paid = await stage(sale, 'paid');
  same('payout', paid.payout, 'claim');
  same('store income', await balance(store) - before, price);
});

// Only with other stores on the same server, so that the run of one store stays as it was.
if (fixture.shops > 1) {
  flow('a branch refused at the start', async () => {
    same('this store', fixture.shop, 'refused');
    const {prices} = await kiosk.api.catalogWindow(0);
    const cart = {origin: 'menu' as const, originRef: id(), menuVersion: prices[0].version, items: [{sku: prices[0].sku, option_key: prices[0].optionKey, quantity: 1}]};
    const refusal = await kiosk.api.reserve(cart, prices[0].asset, id()).then(() => 'accepted', error => (error as Error).message);
    same('an order at a refused branch', refusal, 'UNSUPPORTED_CAPABILITY');
    same('stock kept', (await kiosk.api.catalogWindow(0)).prices[0].available, prices[0].available);
    await finance(fixture.finance1, 'escrow-orders', {}); // in base units
    const scopes = await financeScopes();
    same('finance branches', scopes.map(row => row.id).join(), fixture.branch);
    check('no token for the refused branch', !scopes[0].token && !scopes[0].asset);
  });
}

flow('operator reset statement', async () => {
  await quotesOver();
  await openLife('operator-reset');
  await onBoard('초기화 요청 만들기');
  const text = await until('a request', () => { const shown = tree!.root.findAll(node => node.props.testID === 'reset-request')[0]; return shown?.props.children as string | undefined; });
  check('the request names this board', text.startsWith(`NU54-RESET1:${board.device}:`));
  const reply = execFileSync('python3', ['-B', join(__dirname, '../../../../p01-device-firmware/scripts/enroll_board.py'), 'operator-reset', text,
    '--authority', authority, '--device', board.device, '--yes'], {encoding: 'utf8'}).trim();
  const account = board.account;
  await type('운영자의 답', reply);
  await onBoard('이 답으로 보드 초기화');
  await says('보드를 초기화했습니다');
  await until('a restarted board', () => board.account !== account);
});
