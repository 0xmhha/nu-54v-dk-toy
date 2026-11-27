import React from 'react';
import Renderer from 'react-test-renderer';
import {BackHandler, Linking, Platform} from 'react-native';
import App from '../src/App';
import {orderRecords, type OpenOrder} from '../src/merchant/orderRecords';
import {customerSubject} from '../src/merchant/storeAccess';
import {journalStore} from './fixtures/journal';
import {policy as mockPolicy} from './fixtures/order';

let mockStored = '';
// The trust anchor a release would bind; the shipped build binds none.
let mockAnchor: {origin: string; ca: string; pin: string} | undefined;
const mockCustomerRequest = jest.fn<Promise<string>, [string, string, string, string]>();
let mockScan = '';
jest.mock('react-native-safe-area-context', () => require('react-native-safe-area-context/jest/mock').default);
jest.mock('../src/specs/NativeRenterBle.ts', () => ({__esModule: true, default: {loadDevices: async () => '', scan: async () => {}, stopScan: async () => {},
  disconnect: async () => {}, onScan: () => ({remove() {}})}}));
jest.mock('../src/specs/NativeOwnerWallet.ts', () => ({__esModule: true, default: {loadProfile: async () => mockStored, setHomeProtection: async () => {}}}));
jest.mock('../src/specs/NativeShopQr.ts', () => ({__esModule: true, default: {scan: async () => mockScan}}));
// The phone's registration at one store on the earlier route; empty on a phone that never registered.
let mockRegistration = '';
const mockJournals = new Map<string, string>();
const mockJournal = journalStore(mockJournals);
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: {
  customerRequest: (a: string, p: string, s: string, b: string) => mockCustomerRequest(a, p, s, b),
  loadJournal: (name: string) => mockJournal.loadJournal(name), saveJournal: (name: string, value: string) => mockJournal.saveJournal(name, value),
  loadProfile: async () => mockRegistration, randomBytes: async () => 'AAAAAAAAAAAAAAAAAAAAAA==',
  exchange: async () => { throw new Error('offline'); },
}}));
// The order contract a release would bind beside it.
let mockPin: {chain: number; contract: string; asset: string} | undefined;
jest.mock('../src/merchant/customerEntranceDeployment.ts', () => ({__esModule: true, get approvedCustomerEntrance() { return mockAnchor; },
  get approvedOrderPayment() { return mockPin; }}));

const cafe = '0fbc54ef-2827-4ab5-bfcd-3a27611c3002';
const status = {mode: 'open', orderable: true, weekday: 4, time: '10:31', timezone: 'Asia/Seoul'};
const personal = JSON.stringify({version: 2, exercised: true, mode: 'personal',
  verified: {boardFingerprint: 'ab'.repeat(32), address: '0x' + '12'.repeat(20), generation: 3, network: '8283'}});

let tree!: Renderer.ReactTestRenderer;
const text = () => JSON.stringify(tree.toJSON());
const pick = (label: string, role = 'button') => tree.root.findAll(node => node.props.accessibilityLabel === label &&
  typeof node.props.onPress === 'function' && node.props.accessibilityRole === role)[0];
const tap = (label: string, role?: string) => Renderer.act(async () => { pick(label, role).props.onPress(); });
const selected = () => [...new Set(tree.root.findAll(node => node.props.accessibilityRole === 'tab' && node.props.accessibilityState?.selected)
  .map(node => node.props.accessibilityLabel))];
afterEach(async () => {
  await Renderer.act(async () => tree.unmount());
  jest.restoreAllMocks();
  mockPin = undefined; mockRegistration = ''; mockJournals.clear(); mockStored = '';
});

/** App with the entrance of a release bound, answering one listed store. */
async function open(anchor = true, link?: string) {
  mockAnchor = anchor ? {origin: 'https://store.example:8444', ca: 'MIIB' + 'A'.repeat(64), pin: 'ab'.repeat(32)} : undefined;
  mockStored = mockStored || personal;
  mockCustomerRequest.mockReset();
  mockCustomerRequest.mockImplementation(async (_anchor, path) => JSON.stringify(
    path === '/v2/customer/stores' ? {status: 200, body: JSON.stringify({stores: [{branchRef: cafe, name: '여울 커피', kind: '카페', status}]})}
    : path === `/v2/customer/stores/${cafe}` ? {status: 200, body: JSON.stringify({branchRef: cafe, name: '여울 커피', kind: '카페', intro: '직접 볶은 원두', status})}
    : {status: 404, body: ''}));
  // Set every time: the link of an earlier test would otherwise open the next one.
  jest.spyOn(Linking, 'getInitialURL').mockResolvedValue(link ?? null);
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
}

test('the explore tab opens the store finder, a store opens its page, and Back returns to explore', async () => {
  Object.defineProperty(Platform, 'OS', {value: 'android', configurable: true});
  const handlers: Parameters<typeof BackHandler.addEventListener>[1][] = [];
  jest.spyOn(BackHandler, 'addEventListener').mockImplementation((_name, run) => {
    handlers.push(run);
    return {remove: () => { if (handlers.includes(run)) handlers.splice(handlers.indexOf(run), 1); }} as never;
  });
  await open();
  await tap('둘러보기', 'tab');
  await tap('매장 찾기');
  expect(selected()).toEqual(['둘러보기']);
  expect(text()).toContain('모든 매장');
  await tap('여울 커피');
  expect(text()).toContain('직접 볶은 원두');
  expect(mockCustomerRequest.mock.calls.map(call => [call[1], call[2], call[3]])).toEqual([['/v2/customer/stores', '', ''], [`/v2/customer/stores/${cafe}`, '', '']]);
  await Renderer.act(async () => { (handlers[handlers.length - 1] as () => unknown)(); });
  expect(pick('매장 찾기')).toBeDefined();
  expect(text()).not.toContain('모든 매장');
});

test('a store shortcut link opens that store\'s page when the build has the entrance address', async () => {
  await open(true, `nu54://shop/${cafe}`);
  await Renderer.act(async () => {});
  expect(text()).toContain('직접 볶은 원두');
  expect(mockCustomerRequest.mock.calls.map(call => call[1])).toEqual([`/v2/customer/stores/${cafe}`]);
});

test('the shortcut scanner in the finder opens the page, and the order button goes on to the menu flow', async () => {
  await open();
  await tap('둘러보기', 'tab');
  await tap('매장 찾기');
  mockScan = `nu54://shop/${cafe}`;
  await tap('매장 바로가기 QR 찍기');
  expect(text()).toContain('직접 볶은 원두');
  await tap('메뉴 보고 주문');
  expect(selected()).toEqual(['결제']);
  expect(text()).not.toContain('직접 볶은 원두');
});

test('without the entrance address the finder says so, and a shortcut link keeps the earlier route', async () => {
  await open(false);
  await tap('둘러보기', 'tab');
  await tap('매장 찾기');
  expect(text()).toContain('이 앱에는 매장 목록을 불러올 매장 서버 주소가 아직 없습니다.');
  expect(mockCustomerRequest).not.toHaveBeenCalled();
  await Renderer.act(async () => tree.unmount());
  await open(false, `nu54://shop/${cafe}`);
  await Renderer.act(async () => {});
  expect(selected()).toEqual(['결제']);
  expect(mockCustomerRequest).not.toHaveBeenCalled();
});

test('with the order contract bound, the order button opens the store\'s menu with no registration and no proof, and goes back to the page', async () => {
  mockPin = {chain: 8283, contract: '0x' + '3'.repeat(40), asset: '0x' + '2'.repeat(40)};
  await open();
  const page = {branchRef: cafe, name: '여울 커피', kind: '카페', intro: '직접 볶은 원두', status, merchantId: 'e1' + '0'.repeat(60) + '07'};
  mockCustomerRequest.mockImplementation(async (_anchor, path) => JSON.stringify(
    path === '/v2/customer/stores' ? {status: 200, body: JSON.stringify({stores: [{branchRef: cafe, name: '여울 커피', kind: '카페', status}]})}
    : path === `/v2/customer/stores/${cafe}` ? {status: 200, body: JSON.stringify(page)}
    : path === `/v2/customer/stores/${cafe}/menu/0` ? {status: 200, body: JSON.stringify({shop: {branch: cafe, branchLabel: '여울 커피'}, orderState: {mode: 'open'},
      catalogVersion: 1, catalog: [{sku: 'tea', optionKey: 'standard', label: '차', asset: mockPin!.asset, version: 1, unitPrice: '3500000', available: 2, skuAvailable: 2,
        expiresAt: new Date(Date.now() + 3_600_000).toISOString()}]})}
    : {status: 404, body: ''}));
  await tap('둘러보기', 'tab');
  await tap('매장 찾기');
  await tap('여울 커피');
  await tap('메뉴 보고 주문');
  await Renderer.act(async () => {});
  expect(selected()).toEqual(['결제']);
  expect(text()).toContain('여울 커피 메뉴');
  expect(text()).toContain('3.50 TestUSDC');
  expect(text()).not.toContain('주문 등록'); // no registration step on this route
  expect(text()).not.toContain('근처 단말 찾기');
  // Only public reads went out: the list, the page and the menu. No challenge, proof or session.
  const calls = mockCustomerRequest.mock.calls.map(call => [call[1], call[2], call[3]]);
  expect(calls.filter(call => call[0] === `/v2/customer/stores/${cafe}/menu/0`)).toEqual([[`/v2/customer/stores/${cafe}/menu/0`, '', '']]);
  expect(calls.every(call => call[0].startsWith('/v2/customer/stores') && call[1] === '' && call[2] === '')).toBe(true);
  await tap('매장 화면으로');
  await Renderer.act(async () => {});
  expect(text()).toContain('직접 볶은 원두');
});

test('with the entrance bound and no order contract, a registered phone\'s order button goes on by the earlier route and does not come back to the page', async () => {
  // The phone is registered at another store. The earlier route checks a store QR against that registration.
  mockRegistration = JSON.stringify({scope: {role: 'customer', branchRef: '0fbc54ef-2827-4ab5-bfcd-3a27611c39ea', subjectRef: '0fbc54ef-2827-4ab5-bfcd-3a27611c39eb',
    tenantRef: '0fbc54ef-2827-4ab5-bfcd-3a27611c39ea', expiresAt: 4_000_000_000}, pin: 'a'.repeat(64), payment: mockPolicy});
  await open();
  await tap('둘러보기', 'tab');
  await tap('매장 찾기');
  await tap('여울 커피');
  await tap('메뉴 보고 주문');
  for (let i = 0; i < 5; i++) await Renderer.act(async () => {});
  expect(selected()).toEqual(['결제']);
  expect(text()).not.toContain('직원이 찍으면'); // not a pickup page
  expect(text()).not.toContain('직접 볶은 원두'); // and not the store page again
  expect(text()).toContain('등록 지점의 매장 입구 QR을 확인하세요.');
  // The store page was read once, when the customer opened it. Nothing sent the app back to it.
  expect(mockCustomerRequest.mock.calls.filter(call => call[1] === `/v2/customer/stores/${cafe}`)).toHaveLength(1);
});

describe('a build that orders without a registration', () => {
  const account = '0x' + '12'.repeat(20), device = 'ab'.repeat(32);
  /** A phone whose board is connected and tested, as the wallet keeps it. */
  const tested = (address = account, generation = 3) => JSON.stringify({version: 2, exercised: true, mode: 'personal',
    verified: {boardFingerprint: device, address, generation, network: '8283'},
    spendingBoard: {device, account: address, generation, testedAt: 1_800_000_000},
    enrollment: {version: 2, device, boardSPKI: '24'.repeat(32), ca: 'MA==', certificateChain: ['MA==']}});
  const sale = '0fbc54ef-2827-4ab5-bfcd-3a27611c3001';
  const made: OpenOrder = {payer: account, generation: '3', sale, branch: cafe, customer: customerSubject(account), confirmer: '0x' + '2'.repeat(40), store: '여울 커피',
    asset: '0x' + '2'.repeat(40), amount: '3500000', escrow: '0x' + '3'.repeat(40), merchantId: 'e1' + '0'.repeat(60) + '07', orderKey: 'b'.repeat(64),
    sentAt: 1_764_000_000, pickupNo: 17, stage: 'locked', pickup: 'preparing'};
  const keep = (order: OpenOrder) => orderRecords(jest.requireMock('../src/specs/NativeCommerceHttp.ts').default).keep(order);
  const settle = async () => { for (let i = 0; i < 8; i++) await Renderer.act(async () => {}); };
  const bound = async () => {
    mockPin = {chain: 8283, contract: '0x' + '3'.repeat(40), asset: '0x' + '2'.repeat(40)};
    await open();
    await settle();
  };
  const registrationWords = ['주문 등록', '등록서', '매장 등록', '공개키'];
  const none = (words: string[]) => words.filter(word => text().includes(word));

  test('the payments tab asks for no registration and leads to the store finder', async () => {
    await bound();
    await tap('결제', 'tab');
    await settle();
    expect(none(registrationWords)).toEqual([]);
    expect(text()).toContain('진행 중인 주문이 없습니다.');
    expect(text()).toContain('메뉴는 누구나 볼 수 있습니다. 주문할 때 보드로 본인을 확인합니다.');
    expect(pick('주문 등록 공개키 보기')).toBeUndefined();
    expect(tree.root.findAll(node => node.props.testID === 'customer-commerce-registration')).toHaveLength(0);
    await tap('매장 찾기');
    expect(selected()).toEqual(['둘러보기']);
    expect(text()).toContain('모든 매장');
  });

  test('the home shortcuts and the explore entry for stores all open the store finder', async () => {
    await bound();
    for (const label of ['매장 QR 결제', '근처 매장']) {
      await tap('홈', 'tab');
      await settle();
      await tap(label);
      await settle();
      expect([label, text().includes('모든 매장'), selected()]).toEqual([label, true, ['둘러보기']]);
      expect(none(registrationWords)).toEqual([]);
    }
    await tap('둘러보기', 'tab');
    await tap('가까운 매장에서 주문');
    await settle();
    expect(text()).toContain('모든 매장');
    expect(text()).not.toContain('근처 단말 찾기');
  });

  test('a build of the earlier route keeps its registration guidance and its shortcuts', async () => {
    await open(false);
    await settle();
    await tap('결제', 'tab');
    await settle();
    expect(text()).toContain('매장 결제를 시작하려면');
    expect(pick('주문 등록 공개키 보기')).toBeDefined();
    expect(pick('매장 찾기')).toBeUndefined();
    await tap('홈', 'tab');
    await settle();
    await tap('매장 QR 결제');
    await settle();
    expect(selected()).toEqual(['결제']);
    expect(text()).not.toContain('모든 매장');
  });

  test('an order made from a store page opens with its store from the payments tab and from home, never with a registration', async () => {
    mockStored = tested();
    await keep(made);
    await bound();
    await tap('결제', 'tab');
    await settle();
    expect(text()).not.toContain('인터넷 주문 등록이 필요합니다');
    expect(none(registrationWords)).toEqual([]);
    await tap('여울 커피 주문번호 17 준비 중');
    await settle();
    // The order's own page, of the store's flow. Without the board near it shows what was written down.
    expect(tree.root.findAll(node => node.props.testID === 'order-detail').length).toBeGreaterThan(0);
    expect(text()).toContain('여울 커피');
    expect(none(registrationWords)).toEqual([]);
    expect(tree.root.findAll(node => node.props.testID === 'former-order')).toHaveLength(0);
    await tap('홈', 'tab');
    await settle();
    await tap('여울 커피 주문번호 17 준비 중');
    await settle();
    expect(selected()).toEqual(['결제']);
    expect(tree.root.findAll(node => node.props.testID === 'order-detail').length).toBeGreaterThan(0);
  });

  test('an order of a board account the phone no longer holds opens a read-only page that says what cannot be done', async () => {
    // The board was reset and registered again: the phone now holds another account.
    mockStored = tested('0x' + '56'.repeat(20), 4);
    await keep(made);
    await bound();
    mockCustomerRequest.mockClear();
    await tap('결제', 'tab');
    await settle();
    await tap('여울 커피 주문번호 17 이전 보드 · 준비 중');
    await settle();
    expect(tree.root.findAll(node => node.props.testID === 'former-order').length).toBeGreaterThan(0);
    expect(text()).toContain('이전 보드의 주문');
    expect(text()).toContain('마지막으로 확인한 상태 · 준비 중');
    expect(text()).toContain('결제한 보드 계정의 확인이 필요해서 이 앱에서는 할 수 없습니다.');
    expect(text()).toContain('환불되면 이전 보드 계정으로 가서 되찾을 수 없습니다.');
    expect(text()).not.toContain('받았다는 답은 보낼 수 있지만');
    // Nothing is asked of a board or of the store's server for it: no challenge, no proof, no session request.
    expect(mockCustomerRequest.mock.calls.filter(call => !call[1].startsWith('/v2/customer/stores'))).toEqual([]);
    expect(pick('수령 QR 받기')).toBeUndefined();
    expect(pick('받았어요')).toBeUndefined();
    await tap('매장 화면으로');
    await settle();
    expect(text()).toContain('직접 볶은 원두');
  });

  test('a wallet that cannot be read does not make an order a former board\'s', async () => {
    mockStored = tested();
    await keep(made);
    await bound();
    await tap('결제', 'tab');
    await settle();
    const {default: wallet} = jest.requireMock('../src/specs/NativeOwnerWallet.ts') as {default: {loadProfile: () => Promise<string>}};
    jest.spyOn(wallet, 'loadProfile').mockRejectedValue(new Error('locked'));
    await tap('여울 커피 주문번호 17 준비 중');
    await settle();
    expect(tree.root.findAll(node => node.props.testID === 'former-order')).toHaveLength(0);
    expect(text()).not.toContain('이전 보드의 주문');
  });

  test('orders of the earlier route without a registration are listed as what this app cannot open, with no word about registering', async () => {
    mockStored = tested();
    await keep({...made, customer: '0fbc54ef-2827-4ab5-bfcd-3a27611c39eb'}); // a customer a store issued, not the account's own
    await bound();
    await tap('결제', 'tab');
    await settle();
    expect(text()).toContain('진행 중인 주문이 1건 있습니다');
    expect(text()).toContain('이 앱에서는 이 주문의 상태를 볼 수 없습니다. 매장에 문의하세요.');
    expect(text()).not.toContain('진행 중인 주문이 없습니다.');
    expect(none(registrationWords)).toEqual([]);
  });
});
