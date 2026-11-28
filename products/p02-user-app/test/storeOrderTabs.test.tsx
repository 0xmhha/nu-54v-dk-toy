import React from 'react';
import Renderer from 'react-test-renderer';
import {Linking} from 'react-native';
import App from '../src/App';
import {orderRecords, type OpenOrder} from '../src/merchant/orderRecords';
import {customerSubject} from '../src/merchant/storeAccess';
import {journalStore} from './fixtures/journal';

// A build that orders from store pages: what becomes of a basket when the customer leaves the payment tab. The app, its tabs and the store's page are real; the store server answers
// the public reads of one cafe.

let mockStored = '';
const mockCustomerRequest = jest.fn<Promise<string>, [string, string, string, string]>();
jest.mock('react-native-safe-area-context', () => require('react-native-safe-area-context/jest/mock').default);
jest.mock('../src/specs/NativeRenterBle.ts', () => ({__esModule: true, default: {loadDevices: async () => '', scan: async () => {}, stopScan: async () => {},
  disconnect: async () => {}, onScan: () => ({remove() {}})}}));
jest.mock('../src/specs/NativeOwnerWallet.ts', () => ({__esModule: true, default: {loadProfile: async () => mockStored, setHomeProtection: async () => {},
  evmRpc: async () => { throw new Error('offline'); }}}));
jest.mock('../src/specs/NativeShopQr.ts', () => ({__esModule: true, default: null}));
const mockJournals = new Map<string, string>();
const mockJournal = journalStore(mockJournals);
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: {
  customerRequest: (a: string, p: string, s: string, b: string) => mockCustomerRequest(a, p, s, b),
  loadJournal: (name: string) => mockJournal.loadJournal(name), saveJournal: (name: string, value: string) => mockJournal.saveJournal(name, value),
  loadProfile: async () => '', randomBytes: async () => 'AAAAAAAAAAAAAAAAAAAAAA==',
  exchange: async () => { throw new Error('offline'); },
}}));
const mockAnchor = {origin: 'https://store.example:8444', ca: 'MIIB' + 'A'.repeat(64), pin: 'ab'.repeat(32)};
const mockPin = {chain: 8283, contract: '0x' + '3'.repeat(40), asset: '0x' + '2'.repeat(40)};
jest.mock('../src/merchant/customerEntranceDeployment.ts', () => ({__esModule: true, get approvedCustomerEntrance() { return mockAnchor; },
  get approvedOrderPayment() { return mockPin; }}));

const cafe = '0fbc54ef-2827-4ab5-bfcd-3a27611c3002', sale = '0fbc54ef-2827-4ab5-bfcd-3a27611c3001';
const account = '0x' + '12'.repeat(20), device = 'ab'.repeat(32);
const status = {mode: 'open', orderable: true, weekday: 4, time: '10:31', timezone: 'Asia/Seoul'};
const tested = JSON.stringify({version: 2, exercised: true, mode: 'personal',
  verified: {boardFingerprint: device, address: account, generation: 3, network: '8283'},
  spendingBoard: {device, account, generation: 3, testedAt: 1_800_000_000},
  enrollment: {version: 2, device, boardSPKI: '24'.repeat(32), ca: 'MA==', certificateChain: ['MA==']}});
const made: OpenOrder = {payer: account, generation: '3', sale, branch: cafe, customer: customerSubject(account), confirmer: '0x' + '2'.repeat(40), store: '여울 커피',
  asset: mockPin.asset, amount: '3500000', escrow: mockPin.contract, merchantId: 'e1' + '0'.repeat(60) + '07', orderKey: 'b'.repeat(64),
  sentAt: 1_764_000_000, pickupNo: 17, stage: 'locked', pickup: 'preparing'};
const menuRow = {sku: 'latte', optionKey: 'standard', label: '라떼', asset: mockPin.asset, version: 1, unitPrice: '4500000', available: 1000, skuAvailable: 1000,
  expiresAt: new Date(Date.now() + 3_600_000).toISOString()};

let tree!: Renderer.ReactTestRenderer;
const text = () => JSON.stringify(tree.toJSON());
const pick = (label: string, role = 'button') => tree.root.findAll(node => node.props.accessibilityLabel === label &&
  typeof node.props.onPress === 'function' && node.props.accessibilityRole === role)[0];
const settle = async () => { for (let i = 0; i < 12; i++) await Renderer.act(async () => {}); };
const tap = async (label: string, role?: string) => {
  expect([label, !!pick(label, role)]).toEqual([label, true]);
  await Renderer.act(async () => { pick(label, role).props.onPress(); });
  await settle();
};
const selected = () => [...new Set(tree.root.findAll(node => node.props.accessibilityRole === 'tab' && node.props.accessibilityState?.selected)
  .map(node => node.props.accessibilityLabel))];

beforeEach(async () => {
  mockStored = tested;
  mockJournals.clear();
  mockCustomerRequest.mockReset();
  mockCustomerRequest.mockImplementation(async (_anchor, path) => JSON.stringify(
    path === '/v2/customer/stores' ? {status: 200, body: JSON.stringify({stores: [{branchRef: cafe, name: '여울 커피', kind: '카페', status}]})}
    : path === `/v2/customer/stores/${cafe}` ? {status: 200, body: JSON.stringify({branchRef: cafe, name: '여울 커피', kind: '카페', intro: '직접 볶은 원두', status,
      payment: {board: true, merchantId: made.merchantId}})}
    : path === `/v2/customer/stores/${cafe}/menu/0` ? {status: 200, body: JSON.stringify({shop: {branch: cafe, branchLabel: '여울 커피'}, orderState: {mode: 'open'},
      catalogVersion: 1, catalog: [menuRow]})}
    : {status: 404, body: ''}));
  jest.spyOn(Linking, 'getInitialURL').mockResolvedValue(null);
  await orderRecords(jest.requireMock('../src/specs/NativeCommerceHttp.ts').default).keep(made);
  await Renderer.act(async () => { tree = Renderer.create(<App />); });
  await settle();
});
afterEach(async () => {
  await Renderer.act(async () => tree.unmount());
  jest.restoreAllMocks();
});

async function toMenu() {
  await tap('둘러보기', 'tab');
  await tap('매장 찾기');
  await tap('여울 커피');
  await tap('메뉴 보고 주문');
  expect(text()).toContain('여울 커피 메뉴');
}

test('a basket stays when the customer leaves the payment tab and comes back to it', async () => {
  await toMenu();
  await tap('라떼 추가');
  expect(text()).toContain('선택 1');
  for (const tab of ['지갑', '홈', '설정']) {
    await tap(tab, 'tab');
    await tap('결제', 'tab');
    expect([tab, selected(), text().includes('여울 커피 메뉴'), text().includes('선택 1')]).toEqual([tab, ['결제'], true, true]);
  }
});
