import React from 'react';
import Renderer from 'react-test-renderer';
import {AppState, Platform} from 'react-native';
import {namedTokenAbiSha256 as mockNamed} from '@nu54/protocol';
import {orderRecords} from '../src/merchant/orderRecords';
import {WalletHome} from '../src/wallet/WalletHome';
import {branch as mockBranch, customer as mockCustomer, orderKey, policy as mockPolicy, sale} from './fixtures/order';

// The wallet home of a phone registered with a won store.
const mockDevice = '34'.repeat(32), mockOwner = 'ab'.repeat(32), oldAccount = '0x' + '12'.repeat(20);
let mockStored = '';
const mockWonPolicy = {...mockPolicy, abiSha256: mockNamed, token: {name: 'NU54 Test KRW', symbol: 'tKRW', label: 'tKRW', decimals: 2}};
let mockEnrolled = true;
const mockJournals = new Map<string, string>();
jest.mock('../src/specs/NativeSubscriptionAlerts.ts', () => ({__esModule: true, default: null}));
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: {
  loadProfile: async () => mockEnrolled ? JSON.stringify({scope: {role: 'customer', branchRef: mockBranch, subjectRef: mockCustomer, tenantRef: mockBranch, expiresAt: 4_000_000_000},
    pin: 'a'.repeat(64), payment: mockWonPolicy}) : '', loadJournal: async (name: string) => mockJournals.get(name) ?? '', saveJournal: async (name: string, value: string) => { mockJournals.set(name, value); }, randomBytes: async () => 'AAAA',
  exchange: async () => { throw new Error('offline'); }}}));
let mockBoard = {state: 'released', generation: 1n};
jest.mock('react-native-safe-area-context', () => require('react-native-safe-area-context/jest/mock').default);
jest.mock('../src/specs/NativeOwnerWallet.ts', () => ({__esModule: true, default: {setHomeProtection: async () => {}, loadProfile: async () => mockStored,
  compareAndSaveProfile: async (expected: string, next: string) => { if (expected !== mockStored) return false; mockStored = next; return true; }}}));
jest.mock('../src/specs/NativeRenterBle.ts', () => ({__esModule: true, default: {loadDevices: async () => '[]', scanOwner: async () => {},
  onScan: (listener: (value: string) => void) => { listener(JSON.stringify({name: 'NU54V', address: 'board'})); return {remove() {}}; }}}));
jest.mock('../src/wallet/ownerSession.ts', () => ({OwnerSession: class {
  reached = 'board';
  async connect() {}
  close() {}
  secondsLeft() { return 110; }
  async identity() { return {fingerprint: mockOwner}; }
  async status() { return {...mockBoard, device: mockDevice, owner: mockOwner, root: 0, profile: 0, backedUp: false}; }
  async register() { mockBoard = {state: 'active', generation: mockBoard.generation + 1n}; return this.status(); }
  async spendingAccount() { return '0x' + '12'.repeat(20); }
  async spendingStatus() { return {account: '0x' + '12'.repeat(20), perPayment: 30_000_000n, daily: 100_000_000n, pinWait: 0, orderOpen: false, leftToday: 95_500_000n}; }
}}));

const profile = () => JSON.stringify({version: 2, exercised: true, mode: 'personal',
  verified: {boardFingerprint: mockDevice, address: oldAccount, generation: 1, network: '8283'},
  spendingBoard: {device: mockDevice, account: oldAccount, generation: 1, testedAt: 1_800_000_000},
  enrollment: {version: 2, device: mockDevice, boardSPKI: '24'.repeat(32), ca: 'MA==', certificateChain: ['MA==']}});
let tree!: Renderer.ReactTestRenderer, nonce = 0;
const text = () => JSON.stringify(tree.toJSON());
const flush = () => Renderer.act(async () => { for (let i = 0; i < 20; i++) await Promise.resolve(); });
const show = (screen: 'owner' | 'board' | 'wallet') => <WalletHome permission={async () => true} honorExistingRental={false} rental={() => {}} orders={() => {}} tabbed
  requestScreen={{screen: screen as 'board', nonce: ++nonce}} />;
const go = async (screen: 'owner' | 'board' | 'wallet') => { await Renderer.act(async () => { const next = show(screen); if (tree) tree.update(next); else tree = Renderer.create(next); }); await flush(); };
const tap = async (label: string) => {
  const found = tree.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onPress === 'function')[0];
  expect([label, !!found]).toEqual([label, true]);
  await Renderer.act(async () => { found.props.onPress(); });
  await flush();
};

beforeEach(async () => {
  Object.defineProperty(Platform, 'OS', {value: 'android', configurable: true});
  Object.defineProperty(AppState, 'currentState', {value: 'active', configurable: true});
  mockJournals.clear();
  mockStored = profile();
  mockBoard = {state: 'active', generation: 1n};
  mockEnrolled = true;
  tree = undefined as never;
});
afterEach(async () => { await Renderer.act(async () => { tree.unmount(); }); });

const limits = async () => {
  await go('board');
  await tap('보드 연결·상태 확인');
  await tap('개인 기기 찾기');
  await tap('등록 상태 확인');
  await go('board');
};

test('the limits follow the registered token', async () => {
  await limits();
  for (const shown of ['300,000원', '1,000,000원', '955,000원']) expect(text()).toContain(shown);
  expect(text()).not.toContain('TestUSDC');
});
test('without a registration the limits are not guessed', async () => {
  mockEnrolled = false;
  await limits();
  expect(text()).toContain('확인 안 됨');
  expect(text()).not.toContain('TestUSDC');
  expect(text()).not.toContain('30.00');
});
test('the address page asks about the network first, then names the token', async () => {
  await go('board');
  await tap('받기');
  await tap('내 보드 찾기');
  await tap('보드 연결해 확인');
  expect(text()).toContain('StableNet 테스트넷으로만 보내세요');
  expect(text()).not.toContain(oldAccount);
  await tap('확인했습니다. 주소 보기');
  expect(text()).toContain(oldAccount);
  expect(text()).toContain('tKRW');
  expect(text()).not.toContain('TestUSDC');
});
test('the wallet tab follows the language of the home', async () => {
  mockStored = JSON.stringify({...JSON.parse(profile()), language: 'en'});
  const Http = jest.requireMock<{default: Parameters<typeof orderRecords>[0]}>('../src/specs/NativeCommerceHttp.ts').default;
  await orderRecords(Http).keep({payer: oldAccount, generation: '1', sale, branch: mockBranch, customer: mockCustomer, confirmer: '0x' + 'f1'.repeat(20), store: 'Yeoul', asset: mockPolicy.asset,
    amount: '4500000', escrow: mockPolicy.contract, merchantId: mockPolicy.merchantId, orderKey, sentAt: 1_800_000_000, stage: 'locked'});
  await go('wallet');
  expect(text()).toContain('₩45,000');
  expect(text()).not.toContain('45,000원');
});
