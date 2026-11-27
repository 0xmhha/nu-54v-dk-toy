import React from 'react';
import Renderer from 'react-test-renderer';
import {BackHandler, Linking, Platform} from 'react-native';
import App from '../src/App';

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
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: {
  customerRequest: (a: string, p: string, s: string, b: string) => mockCustomerRequest(a, p, s, b),
  loadJournal: async () => '', saveJournal: async () => {}, loadProfile: async () => '', randomBytes: async () => 'AAAAAAAAAAAAAAAAAAAAAA==',
}}));
// The order contract a release would bind beside it.
let mockPin: {chain: number; contract: string; asset: string} | undefined;
jest.mock('../src/merchant/customerEntranceDeployment.ts', () => ({__esModule: true, get approvedCustomerEntrance() { return mockAnchor; },
  get approvedOrderPayment() { return mockPin; }}));

const cafe = '0fbc54ef-2827-4ab5-bfcd-3a27611c3002';
const status = {mode: 'open', weekday: 4, time: '10:31', timezone: 'Asia/Seoul'};
const personal = JSON.stringify({version: 2, exercised: true, mode: 'personal',
  verified: {boardFingerprint: 'ab'.repeat(32), address: '0x' + '12'.repeat(20), generation: 3, network: '8283'}});

let tree!: Renderer.ReactTestRenderer;
const text = () => JSON.stringify(tree.toJSON());
const pick = (label: string, role = 'button') => tree.root.findAll(node => node.props.accessibilityLabel === label &&
  typeof node.props.onPress === 'function' && node.props.accessibilityRole === role)[0];
const tap = (label: string, role?: string) => Renderer.act(async () => { pick(label, role).props.onPress(); });
const selected = () => [...new Set(tree.root.findAll(node => node.props.accessibilityRole === 'tab' && node.props.accessibilityState?.selected)
  .map(node => node.props.accessibilityLabel))];
afterEach(async () => { await Renderer.act(async () => tree.unmount()); jest.restoreAllMocks(); mockPin = undefined; });

/** App with the entrance of a release bound, answering one listed store. */
async function open(anchor = true, link?: string) {
  mockAnchor = anchor ? {origin: 'https://store.example:8444', ca: 'MIIB' + 'A'.repeat(64), pin: 'ab'.repeat(32)} : undefined;
  mockStored = personal;
  mockCustomerRequest.mockReset();
  mockCustomerRequest.mockImplementation(async (_anchor, path) => JSON.stringify(
    path === '/v2/customer/stores' ? {status: 200, body: JSON.stringify({stores: [{branchRef: cafe, name: '여울 커피', kind: '카페', status}]})}
    : path === `/v2/customer/stores/${cafe}` ? {status: 200, body: JSON.stringify({branchRef: cafe, name: '여울 커피', kind: '카페', intro: '직접 볶은 원두', status})}
    : {status: 404, body: ''}));
  if (link) jest.spyOn(Linking, 'getInitialURL').mockResolvedValue(link);
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
