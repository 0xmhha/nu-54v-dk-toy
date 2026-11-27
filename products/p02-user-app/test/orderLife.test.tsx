import React from 'react';
import {AppState, Text, type AppStateStatus} from 'react-native';
import Renderer from 'react-test-renderer';
import {
  bindServiceRequestV2 as mockBind, hexToBytes, orderBuyerDigest, recoverSigner, toBase64 as mockBase64, validOrderEscrowStatus,
  type CommerceOrder, type OrderEscrowStatus, type ServiceRequestV2,
} from '@nu54/protocol';
import {CustomerOrders} from '../src/merchant/CustomerOrders';
import {isFormerBoard, OpenOrderCard, OrderDetail, remaining, useOpenOrders} from '../src/merchant/OpenOrders';
import {refreshOpenOrders} from '../src/merchant/customerClient';
import {orderRecords, type OpenOrder} from '../src/merchant/orderRecords';
import {demoOrders} from '../src/wallet/DemoWalletHome';
import {branch as mockBranch, customer as mockCustomer, escrowStatus, generation, now, order, orderKey, payer, policy as mockPolicy, sale as mockSale, terminal} from './fixtures/order';

const mockJournals = new Map<string, string>();
let mockEnrolled = true;
let mockExpires = now + 9_999_999;
let mockOrder: CommerceOrder;
let mockOffline = false;
let mockAnswerError = '';
let mockAnswerLost = false;
let mockAnswerRefused = false;
let mockSeed = 1;
const mockAnswers: (ServiceRequestV2 & {approvalSignature?: string})[] = [];
const mockSaved: {answers: number; journal: string}[] = [];
const mockNotice = jest.fn(async (_order: string) => true);
const mockAsk = jest.fn(async (_ask: boolean) => true);
jest.mock('../src/specs/NativeSubscriptionAlerts.ts', () => ({__esModule: true, default: {
  orderReady: (id: string) => mockNotice(id), permission: (ask: boolean) => mockAsk(ask), clear: async () => {}}}));
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: {
  loadProfile: async () => mockEnrolled ? JSON.stringify({scope: {role: 'customer', branchRef: mockBranch, subjectRef: mockCustomer, tenantRef: mockBranch, expiresAt: mockExpires},
    pin: 'a'.repeat(64), payment: mockPolicy}) : '',
  loadJournal: async (name: string) => mockJournals.get(name) ?? '',
  saveJournal: async (name: string, value: string) => { mockJournals.set(name, value); },
  randomBytes: async (length: number) => mockBase64(new Uint8Array(length).fill(mockSeed++ % 250 + 1)),
  exchange: async (_role: string, raw: string) => {
    const {command, approvalSignature} = JSON.parse(raw) as {command: ServiceRequestV2; approvalSignature?: string};
    if (mockOffline) throw new Error('offline');
    const {requestId, requestDigest} = mockBind(command, {role: 'customer', branchRef: mockBranch, subjectRef: mockCustomer, tenantRef: mockBranch, expiresAt: mockExpires,
      grants: [{domainRef: command.domainRef, objectRef: command.objectRef, operations: [`C3/${command.operation}`]}]} as never, Math.floor(Date.now() / 1000));
    const ok = {requestId, requestDigest, status: 'ok', resultRef: command.objectRef};
    if (command.operation === 'order.list') return JSON.stringify({...ok, orders: [{id: mockSale, amount: mockOrder.amount, asset: mockOrder.asset, state: mockOrder.state}]});
    if (command.operation === 'invoice.list') return JSON.stringify({...ok, invoices: []});
    if (command.operation === 'order.status') return JSON.stringify({...ok, order: mockOrder});
    if (command.operation === 'order.answer') {
      mockAnswers.push({...command, approvalSignature});
      mockSaved.push({answers: mockAnswers.length, journal: [...mockJournals.values()].find(value => value.includes('"answer"')) ?? ''});
      if (mockAnswerLost) throw new Error('lost');
      if (mockAnswerRefused) throw Object.assign(new Error('The enrolled commerce connection is unavailable'), {code: 'COMMERCE_REFUSED'});
      return JSON.stringify(mockAnswerError ? {...ok, status: 'error', error: mockAnswerError, resultRef: undefined} : ok);
    }
    throw new Error('unexpected ' + command.operation);
  },
}}));
jest.mock('../src/specs/NativeNearby.ts', () => ({__esModule: true, default: null}));
jest.mock('../src/specs/NativeRenterBle.ts', () => ({__esModule: true, default: {}}));
jest.mock('../src/specs/NativeOwnerWallet.ts', () => ({__esModule: true, default: {loadProfile: async () => '', setHomeProtection: async () => {}}}));

const Http = jest.requireMock<{default: Parameters<typeof orderRecords>[0]}>('../src/specs/NativeCommerceHttp.ts').default;
const records = orderRecords(Http);
const account = {payer, generation: generation.toString()};
let confirmer = '';
let row: OpenOrder;
let appState: ((state: AppStateStatus) => void)[] = [];
let tree: Renderer.ReactTestRenderer | undefined;
const screen = () => JSON.stringify(tree?.toJSON());
const button = (label: string) => tree?.root.findAll(node => node.props.label === label && typeof node.props.run === 'function')[0];
const press = async (label: string) => {
  const found = button(label);
  expect([label, !!found, found?.props.disabled ?? false]).toEqual([label, true, false]);
  await Renderer.act(async () => { found!.props.run(); });
};
const tick = (ms: number) => Renderer.act(async () => { await jest.advanceTimersByTimeAsync(ms); });
/** What P05 reports for the escrow order. */
function served(stage: OrderEscrowStatus['stage'], change: Partial<OrderEscrowStatus> = {}, pickupStage?: 'preparing' | 'ready' | 'collected' | 'unfulfillable'): CommerceOrder {
  const escrow = {...escrowStatus(stage, confirmer), ...change};
  expect(validOrderEscrowStatus(escrow)).toBe(true);
  return {...order, quoteDigest: undefined, orderKey: undefined, state: 'paid', receiptRef: terminal, pickupNo: 17, escrow,
    payment: {stage: 'finalized', chainId: 8283, txHash: '0x' + '1'.repeat(64), includedAt: new Date(now * 1000).toISOString(), pickupStage,
      ...(pickupStage === 'ready' || pickupStage === 'collected' ? {finalizedAt: new Date(now * 1000).toISOString(), pickupReadyAt: new Date(now * 1000).toISOString()} : {})}};
}
async function open(sale = mockSale) {
  await Renderer.act(async () => { tree = Renderer.create(<CustomerOrders back={() => {}} initialLocator="" initialOrder={sale} permission={async () => true} />); });
}

beforeEach(async () => {
  jest.useFakeTimers({now: now * 1000});
  appState = [];
  jest.spyOn(AppState, 'addEventListener').mockImplementation((_name, run) => { appState.push(run); return {remove() { appState = appState.filter(item => item !== run); }}; });
  mockJournals.clear(); mockAnswers.length = 0; mockSaved.length = 0;
  mockEnrolled = true; mockExpires = now + 9_999_999; mockOffline = false; mockAnswerError = ''; mockAnswerLost = false;
  mockNotice.mockClear(); mockAsk.mockClear();
  confirmer = await records.confirmer(account);
  row = {...account, sale: mockSale, branch: mockBranch, customer: mockCustomer, confirmer, store: '그린 카페', asset: mockPolicy.asset, amount: order.amount,
    escrow: mockPolicy.contract, merchantId: mockPolicy.merchantId, orderKey, sentAt: now, stage: 'locked', pickupNo: 17};
  await records.keep(row);
  mockOrder = served('locked', {}, 'preparing');
});
afterEach(async () => {
  await Renderer.act(async () => { tree?.unmount(); });
  tree = undefined;
  jest.useRealTimers();
  jest.restoreAllMocks();
});

describe('the life of an order on the phone', () => {
  test('follows preparation, readiness, hand-over and the end without a tap', async () => {
    await open();
    expect(screen()).toContain('매장이 준비하고 있습니다');
    expect(button('받았어요')).toBeUndefined();
    mockOrder = served('locked', {}, 'ready');
    await tick(15_000);
    expect(screen()).toContain('준비가 끝났습니다');
    mockOrder = served('locked', {}, 'collected');
    await tick(15_000);
    expect(screen()).toContain('매장이 전달을 기록했습니다');
    expect(button('받았어요')).toBeUndefined();
    mockOrder = served('delivered', {}, 'collected');
    await tick(15_000);
    expect(screen()).toContain('확인 기간이 2일');
    expect(button('받았어요')).toBeDefined();
    expect(button('못 받았어요')).toBeDefined();
    expect(button('이의 취소하고 지급')).toBeUndefined();
    mockOrder = served('paid', {payout: 'claim'}, 'collected');
    await tick(15_000);
    expect(screen()).toContain('확인 기간이 끝나 매장에 지급되었습니다');
    expect(button('받았어요')).toBeUndefined();
    expect(screen()).not.toMatch(/0x[0-9a-f]{8}/);
  });
  test('a refund names its reason, and finished orders stay in the list', async () => {
    mockOrder = served('refunded', {refund: 'undelivered'});
    await open();
    expect(screen()).toContain('기한 안에 전달되지 않아서');
    await press('결제 목록으로');
    const listed = tree!.root.findAll(node => node.props.testID === '매장 주문 ' + mockSale && node.props.label)[0];
    expect(listed.props.label).toBe('그린 카페 · 4.50 TestUSDC');
    expect(listed.props.detail).toContain('환불됨');
    expect(await records.list()).toHaveLength(1);
  });
  test('close to the end of the confirmation period the phone clock hides nothing', async () => {
    for (const claimableAt of [now - 10, now + 10]) {
      mockOrder = served('delivered', {claimableAt}, 'collected');
      await records.update(mockSale, {});
      await open();
      expect(button('못 받았어요')).toBeDefined();
      expect(button('받았어요')).toBeDefined();
      await Renderer.act(async () => { tree!.unmount(); });
    }
  });
  test('after the confirmation period only receipt is offered', async () => {
    mockOrder = served('delivered', {claimableAt: now - 301}, 'collected');
    await open();
    expect(screen()).toContain('확인 기간이 끝났습니다');
    expect(button('받았어요')).toBeDefined();
    expect(button('못 받았어요')).toBeUndefined();
  });
});

describe("the buyer's answer", () => {
  test('receipt asks once, is written down first and is signed by this account key for this delivery', async () => {
    mockOrder = served('delivered', {}, 'collected');
    await open();
    await press('받았어요');
    expect(mockAnswers).toHaveLength(0);
    expect(screen()).toContain('취소할 수 없습니다');
    mockOrder = served('delivered', {pendingAction: 'receipt'}, 'collected');
    await press('네, 받았어요');
    expect(mockAnswers).toHaveLength(1);
    expect(mockSaved[0].journal).toContain(mockAnswers[0].approvalSignature);
    const claimableAt = now + 172800;
    expect(mockAnswers[0]).toMatchObject({operation: 'order.answer', body: {action: 'receipt', claimableAt: String(claimableAt)}});
    expect(recoverSigner(orderBuyerDigest('receipt', {escrow: mockPolicy.contract, merchantId: mockPolicy.merchantId, orderKey, claimableAt}),
      hexToBytes(mockAnswers[0].approvalSignature!))).toBe(confirmer);
    expect(screen()).toContain('답을 확인하고 있습니다');
    expect(button('받았어요')).toBeUndefined();
    expect(button('못 받았어요')).toBeUndefined();
    mockOrder = served('paid', {}, 'collected');
    await tick(15_000);
    expect(screen()).toContain('받았다고 확인해서 매장에 지급되었습니다');
    expect((await records.list())[0].answer).toBeUndefined();
  });
  test('a lost reply offers only the identical answer, also after a restart', async () => {
    mockOrder = served('delivered', {}, 'collected');
    mockAnswerLost = true;
    await open();
    await press('못 받았어요');
    expect(screen()).toContain('같은 답만 다시 보낼 수 있습니다');
    expect(button('받았어요')).toBeUndefined();
    await Renderer.act(async () => { tree!.unmount(); });
    await open();
    expect(button('받았어요')).toBeUndefined();
    mockAnswerLost = false;
    mockOrder = served('delivered', {pendingAction: 'dispute'}, 'collected');
    await press('같은 답 다시 보내기');
    expect(mockAnswers).toHaveLength(2);
    expect(mockAnswers[1].idempotencyKey).toBe(mockAnswers[0].idempotencyKey);
    expect(mockAnswers[1].approvalSignature).toBe(mockAnswers[0].approvalSignature);
    expect(mockAnswers[1].body).toEqual(mockAnswers[0].body);
  });
  test('two different answers are never in flight', async () => {
    const key = async () => terminal;
    await records.signAnswer(mockSale, 'dispute', now + 172800, key);
    await expect(records.signAnswer(mockSale, 'receipt', now + 172800, key)).rejects.toThrow('ORDER_ANSWER_PENDING');
    await records.update(mockSale, {stage: 'delivered', claimableAt: now + 172800, pendingAction: undefined});
    expect((await records.list())[0].answer).toBeDefined();
    // The server holds another answer, or the order moved on, or the dispute can no longer execute.
    for (const change of [{pendingAction: 'receipt' as const}, {stage: 'paid' as const, payout: 'claim' as const}, {claimableAt: now + 999_999}]) {
      mockJournals.forEach((value, name) => { if (value.startsWith('[{') && value.includes('"sale"')) mockJournals.delete(name); });
      await records.keep({...row, stage: 'delivered', claimableAt: now + 172800});
      await records.signAnswer(mockSale, 'dispute', now + 172800, key);
      await records.update(mockSale, change);
      expect((await records.list())[0].answer).toBeUndefined();
    }
    await records.keep({...row, stage: 'delivered', claimableAt: now + 172800});
    await records.signAnswer(mockSale, 'dispute', now + 172800, key);
    await records.update(mockSale, {}, now + 172800 + 60);
    expect((await records.list())[0].answer).toBeDefined();
    await records.update(mockSale, {}, now + 172800 + 121);
    expect((await records.list())[0].answer).toBeUndefined();
  });
  test('withdrawing a dispute asks once and pays', async () => {
    mockOrder = served('disputed', {}, 'collected');
    await open();
    expect(screen()).toContain('받지 못했다는 이의를 접수했습니다');
    await press('이의 취소하고 지급');
    await press('이의 취소하고 지급하기');
    expect(mockAnswers[0].body.action).toBe('withdraw');
  });
  test.each([
    ['escrow contract', {escrow: '0x' + '9'.repeat(40)}], ['merchant', {merchantId: 'f'.repeat(64)}], ['order key', {orderKey: 'f'.repeat(64)}],
    ['phone key', {confirmer: '0x' + '9'.repeat(40)}], ['payer', {payer: '0x' + '9'.repeat(40)}],
  ] as [string, Partial<OrderEscrowStatus>][])('nothing is signed when the reported %s differs from the stored one', async (_name, change) => {
    await records.update(mockSale, {stage: 'delivered', claimableAt: now + 172800});
    mockOrder = served('delivered', change, 'collected');
    await open();
    await press('못 받았어요');
    expect(mockAnswers).toHaveLength(0);
    expect(screen()).toContain('결제할 때와 달라');
    expect((await records.list())[0].answer).toBeUndefined();
  });
  test('a lost phone key refuses to answer instead of making a new key', async () => {
    mockOrder = served('delivered', {}, 'collected');
    const seed = [...mockJournals].find(([, value]) => value.includes('"seed"'))![0];
    mockJournals.delete(seed);
    await open();
    await press('못 받았어요');
    expect(mockAnswers).toHaveLength(0);
    expect(screen()).toContain('주문 확인 키를 찾을 수 없어');
    expect(mockJournals.has(seed)).toBe(false);
  });
  test('an order already settled or held by another answer says so', async () => {
    mockOrder = served('delivered', {}, 'collected');
    mockAnswerError = 'CHECKOUT_CONFLICT';
    await open();
    await press('못 받았어요');
    expect(screen()).toContain('주문이 이미 정해졌거나');
    expect(button('같은 답 다시 보내기')).toBeUndefined();
    expect(button('못 받았어요')).toBeDefined();
    mockAnswerError = 'REPLAY_CONFLICT';
    await press('못 받았어요');
    expect(screen()).toContain('다른 답은 보낼 수 없습니다');
  });
  test.each(['CHECKOUT_CONFLICT', 'MISMATCHED_EVIDENCE', 'EXPIRED', 'BAD_REQUEST', 'NOT_AUTHORIZED'])('%s offers the answers again', async code => {
    mockOrder = served('delivered', {}, 'collected');
    mockAnswerError = code;
    await open();
    await press('못 받았어요');
    expect((await records.list())[0].answer).toBeUndefined();
    expect(button('같은 답 다시 보내기')).toBeUndefined();
    expect(button('못 받았어요')).toBeDefined();
  });
  test('an answer outside the protocol is checked against the server before it is dropped', async () => {
    mockOrder = served('delivered', {}, 'collected');
    mockAnswerError = '';
    mockAnswerRefused = true;
    await open();
    await press('못 받았어요');
    expect((await records.list())[0].answer).toBeUndefined();
    mockOrder = served('delivered', {pendingAction: 'dispute'}, 'collected');
    await press('못 받았어요');
    expect((await records.list())[0].answer?.action).toBe('dispute');
    mockAnswerRefused = false;
  });
  test('an answer the server never took is dropped once its request has ended', async () => {
    await records.update(mockSale, {stage: 'delivered', claimableAt: now + 172800});
    await records.signAnswer(mockSale, 'dispute', now + 172800, async () => terminal);
    await records.markAnswerSent(mockSale, now);
    await records.update(mockSale, {}, now + 60);
    expect((await records.list())[0].answer).toBeDefined();
    await records.update(mockSale, {}, now + 121);
    expect((await records.list())[0].answer).toBeUndefined();
  });
  test('a refused answer may be given again, also after a lost one', async () => {
    mockOrder = served('delivered', {}, 'collected');
    mockAnswerError = 'NOT_AUTHORIZED';
    await open();
    await press('못 받았어요');
    expect((await records.list())[0].answer).toBeUndefined();
    mockAnswerError = '';
    mockAnswerLost = true;
    await press('못 받았어요');
    mockAnswerLost = false;
    mockAnswerError = 'NOT_AUTHORIZED';
    await press('같은 답 다시 보내기');
    expect((await records.list())[0].answer).toBeUndefined();
  });
  test('without an internet registration the page says how to connect', async () => {
    mockEnrolled = false;
    await open();
    expect(screen()).toContain('진행 중인 주문이 1건 있습니다');
    expect(screen()).toContain('인터넷 주문 등록이 필요합니다');
    await Renderer.act(async () => { tree!.unmount(); });
    mockEnrolled = true;
    mockExpires = now - 1;
    await open();
    expect(screen()).toContain('인터넷 주문 등록이 만료되어');
    expect(button('받았어요')).toBeUndefined();
  });
  test('offline, the last known state shows and nothing can be answered', async () => {
    await records.update(mockSale, {stage: 'delivered', claimableAt: now + 172800, pickup: 'collected'});
    mockOffline = true;
    await open();
    expect(screen()).toContain('인터넷에 연결되어 있는지 확인');
    expect(screen()).toContain('확인 기간이 2일');
    expect(button('받았어요')).toBeUndefined();
  });
});

describe('the ready notice', () => {
  function Watch() { return <>{useOpenOrders().map(item => <OpenOrderCard key={item.sale} order={item} open={() => {}} />)}</>; }
  const watch = () => Renderer.act(async () => { tree = Renderer.create(<Watch />); });
  test('is shown once, also after a restart, and carries only the local order reference', async () => {
    await watch();
    expect(mockNotice).not.toHaveBeenCalled();
    mockOrder = served('locked', {}, 'ready');
    await tick(20_000);
    expect(mockNotice.mock.calls).toEqual([[mockSale]]);
    expect(screen()).toContain('준비 완료');
    await tick(60_000);
    await Renderer.act(async () => { tree!.unmount(); });
    await watch();
    await tick(60_000);
    expect(mockNotice).toHaveBeenCalledTimes(1);
  });
  test('an order that became ready while the app was closed is announced when it opens', async () => {
    mockOrder = served('locked', {}, 'ready');
    await watch();
    await tick(0);
    expect(mockNotice).toHaveBeenCalledTimes(1);
  });
  test('asks nothing in the background and asks again in front', async () => {
    await watch();
    await Renderer.act(async () => { appState.forEach(run => run('background')); });
    mockOrder = served('locked', {}, 'ready');
    await tick(300_000);
    expect(mockNotice).not.toHaveBeenCalled();
    await Renderer.act(async () => { appState.forEach(run => run('active')); });
    await tick(0);
    expect(mockNotice).toHaveBeenCalledTimes(1);
  });
  test('two readers announce it once', async () => {
    await records.update(mockSale, {pickup: 'ready'});
    const show = jest.fn(async () => true);
    await Promise.all([records.announceReady(mockSale, show), records.announceReady(mockSale, show)]);
    expect(show).toHaveBeenCalledTimes(1);
  });
  test('a notice that could not be shown is tried again', async () => {
    await records.update(mockSale, {pickup: 'ready'});
    const show = jest.fn(async () => false);
    await records.announceReady(mockSale, show);
    expect((await records.list())[0].notified).toBeUndefined();
    show.mockResolvedValue(true);
    await records.announceReady(mockSale, show);
    await records.announceReady(mockSale, show);
    expect(show).toHaveBeenCalledTimes(2);
  });
  test('every open order is asked in turn, not only the first eight', async () => {
    const asked = new Set<string>();
    const status = jest.fn(async (sale: string) => { asked.add(sale); throw new Error('offline'); });
    for (let index = 0; index < 11; index++) await records.keep({...row, sale: `dededede-dede-4ede-9ede-${String(index).padStart(12, '0')}`});
    const before = (await records.list()).length;
    const client = {profile: {branchRef: mockBranch, subjectRef: mockCustomer}, status} as never;
    for (let round = 0; round < 2; round++) { status.mockClear(); await refreshOpenOrders(client).catch(() => {}); expect(status.mock.calls.length).toBeLessThanOrEqual(8); }
    expect(asked.size).toBe(before);
  });
  test('a home card disappears when the order is settled', async () => {
    await watch();
    expect(tree!.root.findAllByType(Text).length).toBeGreaterThan(0);
    await Renderer.act(async () => { await records.update(mockSale, {stage: 'refunded', refund: 'store'}); });
    await tick(0);
    expect(tree!.toJSON()).toBeNull();
  });
});

test('an order of a former board stays visible and says so', async () => {
  const card = async (former: boolean) => {
    await Renderer.act(async () => { tree = Renderer.create(<OpenOrderCard order={row} open={() => {}} former={former} />); });
    return screen();
  };
  expect(isFormerBoard(row, {address: payer, generation: Number(generation)})).toBe(false);
  expect(isFormerBoard(row, {address: payer, generation: Number(generation) + 1})).toBe(true);
  expect(isFormerBoard(row, null)).toBe(true);
  expect(isFormerBoard(row)).toBe(false);
  expect(await card(true)).toContain('이전 보드');
  expect(await card(false)).not.toContain('이전 보드');
});

test('open orders of a board account are known, so its money is not returned or reset', async () => {
  expect((await records.openFor(account)).map(item => item.sale)).toEqual([mockSale]);
  expect(await records.openFor({...account, generation: '99'})).toEqual([]);
  expect(await records.openFor({...account, payer: '0x' + '9'.repeat(40)})).toEqual([]);
  for (const stage of ['delivered', 'disputed'] as const) {
    await records.update(mockSale, {stage, claimableAt: now + 1});
    expect(await records.openFor(account)).toHaveLength(1);
  }
  await records.update(mockSale, {stage: 'paid', payout: 'claim'});
  expect(await records.openFor(account)).toEqual([]);
  const name = [...mockJournals].find(([, value]) => value.startsWith('[{') && value.includes('"sale"'))![0];
  mockJournals.set(name, '[{}]');
  await expect(records.openFor(account)).rejects.toThrow('ORDER_RECORDS'); // unreadable is not "none"
});

test('finished orders are kept and the oldest makes room', async () => {
  for (let index = 0; index < 64; index++)
    await records.keep({...row, sale: `f0551c76-7592-4850-a414-5446${String(index).padStart(8, '0')}`, stage: 'paid', payout: 'claim', sentAt: now + index + 1});
  const rows = await records.list();
  expect(rows).toHaveLength(64);
  expect(rows.some(item => item.sale === mockSale)).toBe(true);
  expect(rows.some(item => item.sale.endsWith('00000000'))).toBe(false);
});

test('the time left is short Korean text', () => {
  expect([remaining(172800), remaining(90000), remaining(3660), remaining(59)]).toEqual(['2일', '1일 1시간', '1시간 1분', '1분']);
});

test.each(demoOrders(now))('detail state %s', async (_label, shown) => {
  await Renderer.act(async () => { tree = Renderer.create(<OrderDetail order={shown} amount="4.50 TestUSDC" fresh now={now}
    actions={{answer: () => {}, refresh: () => {}, back: () => {}}} />); });
  expect(screen()).toContain('4.50 TestUSDC');
  expect(screen()).not.toMatch(/0x[0-9a-f]{6}/);
  expect(!!button('받았어요')).toBe(shown.stage === 'delivered' && !shown.pendingAction);
  expect(!!button('이의 취소하고 지급')).toBe(shown.stage === 'disputed');
});
