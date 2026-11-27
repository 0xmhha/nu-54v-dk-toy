import React from 'react';
import Renderer from 'react-test-renderer';
import {Platform} from 'react-native';
import {toBase64 as mockBase64} from '@nu54/protocol';
import {orderRecords, type OpenOrder} from '../src/merchant/orderRecords';
import {ordersWithoutRegistration, routeWords} from '../src/merchant/orderRoute';
import {customerSubject} from '../src/merchant/storeAccess';
import {BoardLife, setupSteps, type BoardLifeProps} from '../src/wallet/BoardLife';
import {readResetBlock, resetBlocked} from '../src/wallet/boardReturn';
import {faucetFailure} from '../src/wallet/FaucetPage';
import {FirstSteps} from '../src/wallet/HomeParts';
import {sendFailure} from '../src/wallet/SendPage';
import {defaultReadState} from '../src/wallet/readProfiles';
import {decodeWalletStorage, type WalletStorage} from '../src/wallet/profile';
import {journalStore} from './fixtures/journal';
import {branch, generation, now, payer, policy as mockPolicy, sale} from './fixtures/order';

// The board pages in a build that orders from a store page without a registration: its customer entrance and its
// order contract are bound. No board is enrolled at a store there, nothing says to register, and an order made
// from a store page holds a reset back like any other. The build of the earlier route is boardLife.test.tsx.

const mockAnchor = {origin: 'https://store.example:8444', ca: mockBase64(new Uint8Array(96).fill(3)), pin: 'ab'.repeat(32)};
let mockPin: object | undefined;
jest.mock('../src/merchant/customerEntranceDeployment.ts', () => ({__esModule: true,
  get approvedCustomerEntrance() { return mockAnchor; }, get approvedOrderPayment() { return mockPin; }}));
const mockJournals = new Map<string, string>();
const mockJournal = journalStore(mockJournals);
const mockCalls: string[] = [];
let mockSeed = 1;
jest.mock('../src/wallet/evmTransfer.ts', () => ({...jest.requireActual('../src/wallet/evmTransfer.ts'),
  readEvmReceiveBalance: async (_rpc: unknown, _chain: string, _genesis: string, _account: string, asset: string, _decimals?: boolean, tag?: string) =>
    ({amount: tag ? '42000000' : '0', block: '1', asset})}));
jest.mock('../src/specs/NativeShopQr.ts', () => ({__esModule: true, default: null}));
jest.mock('../src/specs/NativeSubscriptionAlerts.ts', () => ({__esModule: true, default: null}));
// A phone that never registered at a store: no profile, and no registered connection to ask.
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: {
  loadProfile: async () => '',
  loadJournal: (name: string) => mockJournal.loadJournal(name), saveJournal: (name: string, value: string) => mockJournal.saveJournal(name, value),
  randomBytes: async (length: number) => mockBase64(new Uint8Array(length).fill(mockSeed++ % 250 + 1)),
  exchange: async (_role: string, raw: string) => { mockCalls.push(JSON.parse(raw).command.operation); throw new Error('a registered connection was used'); },
}}));

const Http = jest.requireMock<{default: Parameters<typeof orderRecords>[0]}>('../src/specs/NativeCommerceHttp.ts').default;
const device = 'ab'.repeat(32), home = '0x' + 'cd'.repeat(20);
const base: WalletStorage = {version: 2, exercised: false, mode: 'personal',
  verified: {boardFingerprint: device, address: payer, generation: Number(generation), network: '8283'},
  spendingBoard: {device, account: payer, generation: Number(generation), testedAt: now, returnAddress: home},
  walletRead: {...defaultReadState, endpoints: {stable: 'https://rpc.example'}}};
/** An order made from a store page: its customer is the board account's own. */
const made: OpenOrder = {payer, generation: generation.toString(), sale, branch, customer: customerSubject(payer), confirmer: '0x' + '2'.repeat(40), store: '그린 카페',
  asset: mockPolicy.asset, amount: '4500000', escrow: mockPolicy.contract, merchantId: mockPolicy.merchantId, orderKey: 'b'.repeat(64), sentAt: now, pickupNo: 17,
  stage: 'locked', pickup: 'preparing'};

let stored: WalletStorage, tree: Renderer.ReactTestRenderer | undefined, message = '', session: Record<string, jest.Mock>;
const went: string[] = [];
let props: BoardLifeProps;
const text = () => tree?.toJSON() ? JSON.stringify(tree.toJSON()) : '';
const flush = () => Renderer.act(async () => { for (let i = 0; i < 40; i++) await Promise.resolve(); });
const render = async () => { await Renderer.act(async () => { const next = <BoardLife {...props} stored={stored} />; if (tree) tree.update(next); else tree = Renderer.create(next); }); await flush(); };
async function open(screen: BoardLifeProps['screen']) {
  if (tree) await Renderer.act(async () => { tree!.unmount(); });
  tree = undefined;
  message = '';
  props = {screen, stored, native: {evmRpc: async () => ''} as never, hasEnrollment: true, found: [{address: 'board', name: 'NU54'}], busy: false,
    scan() {}, go: next => { went.push(next); }, orders() { went.push('orders'); }, say: value => { message = value; }, wait() {}, copy() {}, seen() {},
    seenBoard: {account: payer, generation: Number(generation), at: Date.now()},
    work: (run, failure) => { run().catch(error => { message = failure(error); }); },
    withBoard: async (_handle, run) => run(session as never),
    commit: async change => { stored = decodeWalletStorage(JSON.stringify(change(stored))); }};
  await render();
}
const button = (label: string) => tree!.root.findAll(node => node.props.label === label && typeof node.props.run === 'function')[0];
const press = async (label: string) => {
  expect([label, !!button(label), button(label)?.props.disabled ?? false]).toEqual([label, true, false]);
  await Renderer.act(async () => { button(label).props.run(); });
  await flush();
  await render();
};
const registrationWords = ['매장 등록', '주문 등록', '등록서', '매장에 보드'];
const said = (value: string) => registrationWords.filter(word => value.includes(word));

beforeEach(() => {
  jest.useFakeTimers({now: now * 1000});
  Object.defineProperty(Platform, 'OS', {value: 'android', configurable: true});
  mockJournals.clear(); mockCalls.length = 0; went.length = 0; mockSeed = 1;
  mockPin = {chain: 8283, contract: mockPolicy.contract, asset: mockPolicy.asset, token: {name: 'NU54 Test KRW', symbol: 'tKRW', label: 'tKRW', decimals: 2}};
  stored = base;
  session = {resetBoard: jest.fn(async () => {}), spendingStatus: jest.fn(async () => ({account: payer, orderOpen: false})), status: jest.fn(async () => ({state: 'released'})),
    operatorResetRequest: jest.fn(async () => ({device, generation, nonce: '3c'.repeat(32)})), secondsLeft: jest.fn(() => 110)};
});
afterEach(async () => { await Renderer.act(async () => { tree?.unmount(); }); tree = undefined; jest.useRealTimers(); });

describe('setup without a store registration', () => {
  test('has no store step, no enrollment button and no word about registering', async () => {
    expect(ordersWithoutRegistration()).toBe(true);
    await open('setup');
    expect(button('이 매장에 보드 등록')).toBeUndefined();
    expect(button('결제 탭에서 주문 등록하기')).toBeUndefined();
    expect(said(text())).toEqual([]);
    expect(text()).toContain('4. 입금, 지금 할 일');
    expect(button('입금 안내 보기')).toBeDefined();
    // Nobody is asked which board a store has enrolled: there is no such thing here.
    expect(mockCalls).toEqual([]);
    expect(setupSteps(base, true).map(step => step.label)).toEqual(['보드 연결과 PIN', '서명 시험', '반납받을 주소', '입금']);
  });

  test('the first steps of a new phone name no store step either', async () => {
    let first!: Renderer.ReactTestRenderer;
    await Renderer.act(async () => { first = Renderer.create(<FirstSteps t={value => value} connect={() => {}} disabled={false} />); });
    expect(said(JSON.stringify(first.toJSON()))).toEqual([]);
    expect(JSON.stringify(first.toJSON())).toContain('원래 지갑에서 입금');
    await Renderer.act(async () => { first.unmount(); });
  });

  test('a build of the earlier route keeps the store step and the enrollment button', async () => {
    mockPin = undefined;
    expect(ordersWithoutRegistration()).toBe(false);
    expect(setupSteps(base, true).map(step => step.label)).toEqual(['보드 연결과 PIN', '서명 시험', '반납받을 주소', '매장 등록', '입금']);
    await open('setup');
    expect(button('이 매장에 보드 등록')).toBeDefined();
    expect(button('결제 탭에서 주문 등록하기')).toBeDefined(); // this phone has no registration yet
    expect(text()).toContain('결제할 매장에 보드 계정을 등록합니다. 나중에 해도 됩니다');
  });
});

describe('the words of the two routes', () => {
  const sentences = ['먼저 매장 주문 등록이 필요합니다. 결제 탭에서 매장 담당자가 준 등록서를 입력하세요.',
    '이 매장에 보드 계정이 등록되어 있지 않습니다. 설정에서 매장 등록을 먼저 하세요.', '다른 보드 계정이 이 매장에 등록되어 있습니다. 설정에서 매장 등록을 다시 하세요.',
    '이 매장에 등록된 보드 계정이 아니거나 서버가 서명을 받아들이지 않았습니다. 설정에서 매장 등록을 다시 하세요.',
    '매장 등록을 확인하지 못했습니다. 등록했다면 다시 확인하고, 아니면 결제 탭에서 매장 담당자가 준 등록서를 입력하세요.',
    '이 휴대전화의 주문 등록으로는 받을 수 없습니다. 매장 담당자에게 새 등록서를 받으세요.', '이 휴대전화의 주문 등록으로는 할 수 없습니다. 매장 담당자에게 새 등록서를 받으세요.',
    '주문 등록이 만료되었습니다. 매장 담당자에게 새 등록서를 받으세요.', '매장 등록을 확인하고 있습니다.',
    '네트워크 주소나 매장 등록이 없어 잔액을 읽지 못했습니다. 보낸 돈은 원래 지갑의 거래 기록에서 확인하세요.', '보드 계정의 잔액을 확인할 수 없습니다. 네트워크 주소와 매장 등록을 확인하세요.',
    '서명 시험, 반납 주소, 매장 등록'];

  test('without a registration nothing sends the customer to register; on the earlier route every sentence is as it was', () => {
    for (const sentence of sentences) {
      expect([sentence, said(routeWords(sentence, true))]).toEqual([sentence, []]);
      expect(routeWords(sentence, true)).not.toContain('등록');
      expect(routeWords(sentence, false)).toBe(sentence);
    }
    expect(routeWords('오늘 받을 수 있는 만큼 받았거나 서버가 바쁩니다. 나중에 다시 시도하세요.', true)).toBe('오늘 받을 수 있는 만큼 받았거나 서버가 바쁩니다. 나중에 다시 시도하세요.');
  });

  test('test tokens, sending and returning say they are not in this app yet, not that a registration is missing', async () => {
    for (const reason of ['NO_CREDENTIAL', 'NOT_AUTHORIZED', 'MISMATCHED_EVIDENCE', 'EXPIRED'])
      expect([reason, said(faucetFailure(new Error(reason)))]).toEqual([reason, []]);
    expect(faucetFailure(new Error('NO_CREDENTIAL'))).toBe('이 앱에서는 아직 쓸 수 없는 기능입니다.');
    expect(faucetFailure(new Error('MISMATCHED_EVIDENCE'))).toBe('매장 서버가 이 보드 계정을 알지 못해 진행할 수 없습니다.');
    expect(said(sendFailure(new Error('NO_CREDENTIAL')))).toEqual([]);
    await open('return');
    expect(said(text())).toEqual([]);
    await press('반납 시작');
    expect(message).toBe('이 앱에서는 아직 쓸 수 없는 기능입니다.');
    mockPin = undefined;
    expect(faucetFailure(new Error('MISMATCHED_EVIDENCE'))).toBe('이 매장에 보드 계정이 등록되어 있지 않습니다. 설정에서 매장 등록을 먼저 하세요.');
    expect(faucetFailure(new Error('NO_CREDENTIAL'))).toBe('먼저 매장 주문 등록이 필요합니다. 결제 탭에서 매장 담당자가 준 등록서를 입력하세요.');
  });

  test('the deposit page names the token of the build\'s order contract and reads its balance, with no registration', async () => {
    await open('deposit');
    await press('확인했습니다. 주소 보기');
    expect(text()).toContain('tKRW만');
    expect(text()).not.toContain('확인 안 됨');
    expect(said(text())).toEqual([]);
    expect(mockCalls).toEqual([]);
  });
});

describe('a reset while an order made from a store page is open', () => {
  test('is not offered by the app, with or without the operator, and the board-button way is warned against', async () => {
    await orderRecords(Http).keep(made);
    await open('reset');
    expect(text()).toContain('끝나지 않은 주문이 있습니다');
    expect(text()).toContain('주문 1건을 먼저 끝내세요. 보드의 키를 지우면 그 주문의 환불금을 받을 수 없습니다.');
    expect(button('앱에서 초기화 계속')).toBeUndefined();
    expect(button('보드 초기화')).toBeUndefined();
    // The buttons of the board cannot be held back by the app, so the page says not to use them now.
    expect(text()).toContain('끝나지 않은 주문이나 반납이 있으면 버튼으로도 초기화하지 마세요.');
    await press('진행 중인 주문 보기');
    expect(went).toEqual(['orders']);
    await open('operator-reset');
    expect(text()).toContain('끝나지 않은 주문이 있습니다');
    expect(button('초기화 요청 만들기')).toBeUndefined();
    await open('return');
    expect(text()).toContain('끝나지 않은 주문이 있습니다');
    expect(button('반납 시작')).toBeUndefined();
    expect(session.resetBoard).not.toHaveBeenCalled();
    expect(mockCalls).toEqual([]); // no registered connection exists to ask; the phone's own record is what holds it
  });

  test('is offered again once that order has ended', async () => {
    await orderRecords(Http).keep({...made, stage: 'paid', payout: 'receipt'});
    await open('reset');
    expect(text()).not.toContain('끝나지 않은 주문이 있습니다');
    expect(button('앱에서 초기화 계속')).toBeDefined();
  });

  test('a reset through the app also empties the stores this phone opened', async () => {
    await mockJournal.saveJournal('customer-stores-v1', JSON.stringify([{branch, name: '그린 카페'}]));
    await open('reset');
    await press('앱에서 초기화 계속');
    await press('보드 초기화');
    expect(session.resetBoard).toHaveBeenCalledTimes(1);
    expect(await mockJournal.loadJournal('customer-stores-v1')).toBe('');
    expect(stored.verified).toBeUndefined();
  });

  test('an unfinished top-up would hold it back the same way, once the app keeps one', async () => {
    const account = {payer, generation: generation.toString()};
    const none = await readResetBlock(Http, orderRecords(Http), account, async () => undefined);
    expect([none.toppingUp, resetBlocked(none)]).toEqual([undefined, false]);
    const held = await readResetBlock(Http, orderRecords(Http), account, async () => undefined, async () => true);
    expect([held.toppingUp, resetBlocked(held)]).toEqual([true, true]);
    const unknown = await readResetBlock(Http, orderRecords(Http), account, async () => undefined, async () => { throw new Error('unreadable'); });
    expect([unknown.unreadable, resetBlocked(unknown)]).toEqual([true, false]);
  });
});
