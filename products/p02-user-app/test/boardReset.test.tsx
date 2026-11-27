import React from 'react';
import Renderer from 'react-test-renderer';
import {Platform} from 'react-native';
import {WalletHome} from '../src/wallet/WalletHome';

// A board that was reset by its buttons or by the operator, seen by the phone of its former account.
const mockDevice = '34'.repeat(32), mockOwner = 'ab'.repeat(32), oldAccount = '0x' + '12'.repeat(20);
const mockNewAccount = '0x' + '56'.repeat(20);
let mockStored = '';
let mockBoard = {state: 'released', generation: 1n};
let mockAtRegister: {verified?: unknown; spendingBoard?: unknown} | undefined;
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
  async register() { mockAtRegister = JSON.parse(mockStored); mockBoard = {state: 'active', generation: mockBoard.generation + 1n}; return this.status(); }
  async spendingAccount() { return mockBoard.generation === 1n ? '0x' + '12'.repeat(20) : mockNewAccount; }
  async spendingStatus() { return {account: mockNewAccount, perPayment: 1n, daily: 1n, pinWait: 0, orderOpen: false, leftToday: 1n}; }
}}));

const profile = () => JSON.stringify({version: 2, exercised: true, mode: 'personal',
  verified: {boardFingerprint: mockDevice, address: oldAccount, generation: 1, network: '8283'},
  spendingBoard: {device: mockDevice, account: oldAccount, generation: 1, testedAt: 1_800_000_000},
  enrollment: {version: 2, device: mockDevice, boardSPKI: '24'.repeat(32), ca: 'MA==', certificateChain: ['MA==']}});
let tree!: Renderer.ReactTestRenderer, nonce = 0;
const text = () => JSON.stringify(tree.toJSON());
const flush = () => Renderer.act(async () => { for (let i = 0; i < 20; i++) await Promise.resolve(); });
const show = (screen: 'owner' | 'board') => <WalletHome permission={async () => true} honorExistingRental={false} rental={() => {}} orders={() => {}} tabbed
  requestScreen={{screen, nonce: ++nonce}} />;
/** The connection page of a spending board is part of 내 보드. */
const connect = async () => { await go('board'); await tap('보드 연결·상태 확인'); };
const go = async (screen: 'owner' | 'board') => { await Renderer.act(async () => { const next = show(screen); if (tree) tree.update(next); else tree = Renderer.create(next); }); await flush(); };
const tap = async (label: string) => {
  const found = tree.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onPress === 'function')[0];
  expect([label, !!found]).toEqual([label, true]);
  await Renderer.act(async () => { found.props.onPress(); });
  await flush();
};
const saved = () => JSON.parse(mockStored);

beforeEach(async () => {
  Object.defineProperty(Platform, 'OS', {value: 'android', configurable: true});
  mockStored = profile();
  mockBoard = {state: 'released', generation: 1n};
  mockAtRegister = undefined;
  tree = undefined as never;
});
afterEach(async () => { await Renderer.act(async () => { tree.unmount(); }); });

test('a released board makes the phone forget the destroyed account', async () => {
  await connect();
  await tap('개인 기기 찾기');
  await tap('등록 상태 확인');
  expect([saved().verified, saved().spendingBoard]).toEqual([undefined, undefined]);
  expect(saved().enrollment.device).toBe(mockDevice);
});

test('another generation of the same board replaces the account and asks for the signing test again', async () => {
  mockBoard = {state: 'active', generation: 2n};
  await connect();
  await tap('개인 기기 찾기');
  await tap('등록 상태 확인');
  expect(saved().verified).toMatchObject({address: mockNewAccount, generation: 2});
  expect(saved().spendingBoard).toBeUndefined();
});

test('registration forgets the old account before it starts and needs a new signing test', async () => {
  await connect();
  await tap('개인 기기 찾기');
  await tap('기기 등록 시작');
  expect([mockAtRegister?.verified, mockAtRegister?.spendingBoard]).toEqual([undefined, undefined]);
  expect([saved().verified, saved().spendingBoard]).toEqual([undefined, undefined]);
});

test('the same registration keeps its account', async () => {
  mockBoard = {state: 'active', generation: 1n};
  await connect();
  await tap('개인 기기 찾기');
  await tap('등록 상태 확인');
  expect(saved()).toMatchObject({verified: {address: oldAccount, generation: 1}, spendingBoard: {account: oldAccount}});
});

test('the deposit address needs a board seen just now, and says so after it was reset', async () => {
  await go('board');
  await tap('받기');
  expect(text()).not.toContain(oldAccount);
  expect(text()).toContain('보드를 다시 연결해 확인하세요');
  await tap('내 보드 찾기');
  await tap('보드 연결해 확인');
  expect(text()).not.toContain(oldAccount);
  expect([saved().verified, saved().spendingBoard]).toEqual([undefined, undefined]);
});

test('the deposit address appears for ten minutes after the board proved itself', async () => {
  jest.useFakeTimers({now: 1_800_000_000_000});
  try {
    mockBoard = {state: 'active', generation: 1n};
    await go('board');
    await tap('받기');
    await tap('내 보드 찾기');
    await tap('보드 연결해 확인');
    // Without a registration nobody knows the board's token, so the network is confirmed before the address.
    expect(text()).not.toContain(oldAccount);
    await tap('확인했습니다. 주소 보기');
    expect(text()).toContain(oldAccount);
    expect(text()).not.toContain('TestUSDC');
    expect(text()).toContain('버튼으로 초기화했다면');
    await Renderer.act(async () => { await jest.advanceTimersByTimeAsync(601_000); });
    expect(text()).not.toContain(oldAccount);
  } finally {
    jest.useRealTimers();
  }
});
