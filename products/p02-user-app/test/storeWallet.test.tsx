import React from 'react';
import Renderer from 'react-test-renderer';
import {namedTokenAbiSha256 as mockNamed} from '@nu54/protocol';
import {orderRecords} from '../src/merchant/orderRecords';
import {defaultReadState, observationScope} from '../src/wallet/readProfiles';
import type {WalletStorage} from '../src/wallet/profile';
import {assetMoney, storeOf, tokenText} from '../src/wallet/tokens';
import {useWalletAdvice} from '../src/wallet/walletAdvice';
import {educationRate, stableTestUsdc} from '../src/wallet/walletEducationRate';
import {WalletTab, type WalletAdvice} from '../src/wallet/WalletTab';
import {branch, customer, orderKey, payer, generation, policy as mockPolicy, sale} from './fixtures/order';

// What a phone registered with a won store shows about its wallet.
const won = {name: 'NU54 Test KRW', symbol: 'tKRW', label: 'tKRW', decimals: 2};
const mockWon = {...mockPolicy, abiSha256: mockNamed, token: won}, elsewhere = '0x' + '7'.repeat(40), friend = '0x' + 'f1'.repeat(20);
const mockJournals = new Map<string, string>();
let mockEnrolled = true, mockSends: object[] = [];
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: {
  loadJournal: async (name: string) => mockJournals.get(name) ?? '', saveJournal: async (name: string, value: string) => { mockJournals.set(name, value); },
}}));
jest.mock('../src/specs/NativeSubscriptionAlerts.ts', () => ({__esModule: true, default: null}));
jest.mock('../src/wallet/BoardLife.tsx', () => ({credential: async () => {
  if (!mockEnrolled) throw new Error('NO_CREDENTIAL');
  return {policy: mockWon, client: {walletSends: async () => ({perDay: 3, remaining: 3, sends: mockSends}), networkList: async () => { throw new Error('none'); }}};
}}));
const Http = jest.requireMock<{default: Parameters<typeof orderRecords>[0]}>('../src/specs/NativeCommerceHttp.ts').default;
const device = 'ab'.repeat(32), account = {payer, generation: generation.toString()};
const stored: WalletStorage = {version: 2, exercised: true, mode: 'personal', verified: {boardFingerprint: device, address: payer, generation: Number(generation), network: '8283'},
  spendingBoard: {device, account: payer, generation: Number(generation), testedAt: 1}};
let tree: Renderer.ReactTestRenderer, advice: WalletAdvice | undefined;
const text = () => JSON.stringify(tree.toJSON());
const flush = () => Renderer.act(async () => { for (let i = 0; i < 80; i++) await Promise.resolve(); });
function Screen({language}: {language: 'ko' | 'en'}) {
  advice = useWalletAdvice(true, 0, 0, () => {});
  return <WalletTab t={ko => ko} language={language} stored={stored} advice={advice} active hide={() => {}} receive={() => {}} send={() => {}} giveBack={() => {}} networks={() => {}}
    board={() => {}} openOrder={() => {}} save={async () => {}} exchange={async () => { throw new Error('offline'); }} />;
}
const show = async (language: 'ko' | 'en' = 'ko') => { await Renderer.act(async () => { tree = Renderer.create(<Screen language={language} />); }); await flush(); await flush(); };
const keep = (asset: string, id: string, amount = '4500000') => orderRecords(Http).keep({...account, sale: id, branch, customer, confirmer: friend, store: '여울 커피', asset, amount,
  escrow: mockPolicy.contract, merchantId: mockPolicy.merchantId, orderKey, sentAt: 1_800_000_000, stage: 'locked'});
const sent = (token: string, nonce: string, amount = '7000000') => ({payer, to: friend, token, amount, validBefore: 1_800_000_500, nonce: nonce.repeat(64), createdAt: 1_800_000_000, state: 'final', txHash: '0x' + '1'.repeat(64)});

beforeEach(() => { mockJournals.clear(); mockEnrolled = true; mockSends = []; });
afterEach(async () => { await Renderer.act(async () => { tree?.unmount(); }); });

test('the registration names the store token for the wallet', async () => {
  await show();
  expect(advice).toMatchObject({signed: [{address: mockPolicy.asset, symbol: 'tKRW', decimals: 2}], registered: {asset: mockPolicy.asset, token: won}});
  expect(text()).toContain('tKRW');
});
test('activity is worded by the asset of each record, never by the present store alone', async () => {
  await keep(mockPolicy.asset, sale);
  await keep(elsewhere, '1'.repeat(8) + sale.slice(8), '3300000');
  mockSends = [sent(mockPolicy.asset, '8'), sent(elsewhere, '9', '8800000')];
  await show();
  expect(text()).toContain('45,000원');
  expect(text()).toContain('70,000원');
  expect(text()).toContain('· 확인 안 됨');
  for (const guess of ['33,000', '88,000', '3.30', '8.80']) expect(text()).not.toContain(guess);
  expect(text()).not.toContain('TestUSDC');
  await Renderer.act(async () => { tree.unmount(); });
  await show('en');
  expect(text()).toContain('₩45,000');
  expect(text()).toContain('₩70,000');
  expect(text()).toContain('· Not confirmed');
});
test('without a registration no record is worded as dollars', async () => {
  mockEnrolled = false;
  await keep(mockPolicy.asset, sale);
  await show();
  expect(text()).toContain('확인 안 됨');
  expect(text()).not.toContain('4.50');
});
test('lists cut dollars and keep won exact', () => {
  expect([tokenText('82009999', {symbol: 'TestUSDC', decimals: 6}), tokenText('1234567899999', {symbol: 'tUSDC', decimals: 6})]).toEqual(['82.00', '1,234,567.89']);
  expect(tokenText('1250001', {symbol: 'tKRW', decimals: 2})).toBe('12,500.01');
  const store = storeOf(mockWon);
  expect(assetMoney('1250001', mockPolicy.asset.toUpperCase().replace('0X', '0x'), store)).toBe('12,500.01원');
  expect(assetMoney('1250001', elsewhere, store)).toBe('확인 안 됨');
  expect(assetMoney('1250001', mockPolicy.asset, undefined)).toBe('확인 안 됨');
});
test('the training rate cuts both amounts off', () => {
  const owner = {boardFingerprint: device, generation: 1, address: payer, network: '8283'} as never;
  const selection = {...defaultReadState.selection, id: 'stable' as const, account: 0, hidden: false, asset: stableTestUsdc};
  const scope = observationScope(device, 1, selection);
  const state = {...defaultReadState, selection, assets: [{board: device, generation: 1, id: 'stable', account: 0, contract: stableTestUsdc, decimals: 6, block: '0x' + '11'.repeat(32), height: '70', hidden: false, favorite: false}],
    observations: [{scope, address: payer, amount: '1999999', decimals: 6, point: 'Finalized block 1', time: 1, unit: 'ERC-20'}]} as never;
  const rate = educationRate(state, owner);
  expect(rate && [rate.amount, rate.krw]).toEqual(['1.99', '2599']); // 1.999999 x 1,300 is 2,599.9987
});
