import React from 'react';
import Renderer from 'react-test-renderer';
import {namedTokenAbiSha256 as mockNamed} from '@nu54/protocol';
import {readProfiles} from '../src/wallet/readProfiles';
import type {WalletStorage} from '../src/wallet/profile';
import {useWalletAdvice} from '../src/wallet/walletAdvice';
import {WalletTab, type WalletAdvice} from '../src/wallet/WalletTab';
import {payer, generation, policy as mockPolicy} from './fixtures/order';

// The wallet of a build that orders from store pages. No store registers the phone, so the token a board of this
// build pays in, and where its chain is read, come from the order contract the build binds.

const won = {name: 'NU54 Test KRW', symbol: 'tKRW', label: 'tKRW', decimals: 2};
const mockAnchor = {origin: 'https://store.example:8444', ca: 'MIIB' + 'A'.repeat(64), pin: 'ab'.repeat(32)};
let mockPin: object | undefined;
jest.mock('../src/merchant/customerEntranceDeployment.ts', () => ({__esModule: true, get approvedCustomerEntrance() { return mockAnchor; },
  get approvedOrderPayment() { return mockPin; }}));
// A registration of the earlier route, when the phone still holds one.
let mockRegistration = '';
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: {
  loadProfile: async () => mockRegistration, loadJournal: async () => '', saveJournal: async () => {},
}}));
jest.mock('../src/specs/NativeSubscriptionAlerts.ts', () => ({__esModule: true, default: null}));
jest.mock('../src/wallet/BoardLife.tsx', () => ({credential: async () => { throw new Error('NO_CREDENTIAL'); }}));

const device = 'ab'.repeat(32);
const stored: WalletStorage = {version: 2, exercised: true, mode: 'personal', verified: {boardFingerprint: device, address: payer, generation: Number(generation), network: '8283'},
  spendingBoard: {device, account: payer, generation: Number(generation), testedAt: 1}};
let balance = 0n;
const asked: string[] = [];
const word = (value: bigint) => value.toString(16).padStart(64, '0');
/** The chain: the token's balance of the board account, and nothing of anything else. */
const exchange = async (url: string, packet: string) => {
  const {method, params} = JSON.parse(packet) as {method: string; params: unknown[]};
  asked.push(url);
  const result = method === 'eth_chainId' ? '0x205b'
    : method === 'eth_getBlockByNumber' ? {hash: params[0] === '0x0' ? readProfiles.stable.anchor : '0x' + '5c'.repeat(32), number: '0x64'}
    : method === 'eth_getBalance' ? '0x0'
    : '0x' + word((params as [{to: string}])[0].to === mockPolicy.asset ? balance : 0n);
  return JSON.stringify({jsonrpc: '2.0', id: 1, result});
};
let tree: Renderer.ReactTestRenderer, advice: WalletAdvice | undefined;
const text = () => JSON.stringify(tree.toJSON());
const flush = () => Renderer.act(async () => { for (let i = 0; i < 120; i++) await Promise.resolve(); });
function Screen() {
  advice = useWalletAdvice(true, 0, 0, () => {});
  return <WalletTab t={ko => ko} language="ko" stored={stored} advice={advice} active hide={() => {}} receive={() => {}} send={() => {}} giveBack={() => {}} networks={() => {}}
    board={() => {}} openOrder={() => {}} save={async () => {}} exchange={exchange} />;
}
const show = async () => { await Renderer.act(async () => { tree = Renderer.create(<Screen />); }); await flush(); await flush(); };
const row = (start: string) => tree.root.findAll(node => typeof node.props.accessibilityLabel === 'string' && node.props.accessibilityLabel.startsWith(start)
  && typeof node.props.onPress === 'function').map(node => node.props.accessibilityLabel as string);

beforeEach(() => { mockPin = {chain: 8283, contract: mockPolicy.contract, asset: mockPolicy.asset, token: won, rpc: 'https://rpc.store.example'}; mockRegistration = ''; balance = 0n; asked.length = 0; });
afterEach(async () => { await Renderer.act(async () => { tree?.unmount(); }); });

test('without a registration the build\'s order contract names the token, and the wallet lists what the board account holds of it', async () => {
  balance = 299_999n; // 2,999.99 won
  await show();
  expect(advice).toMatchObject({store: 'https://rpc.store.example', signed: [{address: mockPolicy.asset, symbol: 'tKRW', decimals: 2, won: true}],
    registered: {asset: mockPolicy.asset, token: won}});
  // Whole won, cut off: never 3,000원 and never the part of a won.
  expect(row('tKRW ')).toEqual(['tKRW 2,999원, 매장 결제에 씀']);
  expect(text()).not.toContain('2,999.99');
  expect(text()).not.toContain('3,000원');
  // Once the contract is known its own read server is the one asked.
  expect(asked.at(-1)).toBe('https://rpc.store.example/');
});

test('less than one won reads as 0 won', async () => {
  balance = 99n;
  await show();
  expect(row('tKRW ')).toEqual(['tKRW 0원, 매장 결제에 씀']);
});

test('a dollar contract lists dollars cut to two decimals', async () => {
  mockPin = {chain: 8283, contract: mockPolicy.contract, asset: mockPolicy.asset};
  balance = 2_999_999n; // 2.999999 dollars
  await show();
  expect(advice?.registered).toEqual({asset: mockPolicy.asset, token: expect.objectContaining({label: 'TestUSDC', decimals: 6})});
  expect(row('TestUSDC ')).toEqual(['TestUSDC 2.99, 매장 결제에 씀']);
});

test('a registration the phone still holds names its own store, as before', async () => {
  mockRegistration = JSON.stringify({scope: {role: 'customer', branchRef: '0fbc54ef-2827-4ab5-bfcd-3a27611c39ea', subjectRef: '0fbc54ef-2827-4ab5-bfcd-3a27611c39eb',
    tenantRef: '0fbc54ef-2827-4ab5-bfcd-3a27611c39ea', expiresAt: 4_000_000_000}, pin: 'a'.repeat(64),
    payment: {...mockPolicy, asset: '0x' + '7'.repeat(40), abiSha256: mockNamed, token: won}});
  await show();
  expect(advice?.registered).toMatchObject({asset: '0x' + '7'.repeat(40)});
  expect(advice?.signed).toBeUndefined();
});

test('a build that binds no order contract names no token', async () => {
  mockPin = undefined;
  balance = 299_999n;
  await show();
  expect(advice).toEqual({});
  expect(row('tKRW ')).toEqual([]);
});
