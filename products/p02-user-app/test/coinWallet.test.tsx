import React from 'react';
import Renderer from 'react-test-renderer';
import {namedTokenAbiSha256, stableNetCoin} from '@nu54/protocol';
import {FaucetPage} from '../src/wallet/FaucetPage';
import {networkRows} from '../src/wallet/networks';
import {defaultReadState} from '../src/wallet/readProfiles';
import {WalletTab} from '../src/wallet/WalletTab';
import {storeToken, tokensOf, tokenText} from '../src/wallet/tokens';
import {policy} from './fixtures/order';

// StableNet's own coin as a store's won: one balance that is both the gas coin and what the board pays with.
const coin = {name: 'WKRC', symbol: 'WKRC', label: 'KRWC', decimals: 18, boardDecimals: 2}, wei = 10n ** 18n;
const coinPolicy = {...policy, abiSha256: namedTokenAbiSha256, asset: stableNetCoin.asset, token: coin, maxBaseUnits: (300_000n * wei).toString()};
const wonPolicy = {...policy, abiSha256: namedTokenAbiSha256, token: {name: 'NU54 Test KRW', symbol: 'tKRW', label: 'tKRW', decimals: 2}};
const [stable, sepolia] = networkRows();
const list = (signed: ReturnType<typeof storeToken>[], listed: ReturnType<typeof storeToken>[] = []) =>
  tokensOf(stable, undefined, defaultReadState, signed, listed).map(token => [token.symbol, token.sends, !!token.won]);
let mockClient: object;
jest.mock('../src/wallet/BoardLife.tsx', () => ({credential: async () => ({client: mockClient})}));
jest.mock('../src/merchant/customerClient.ts', () => ({walletId: async () => 'dededede-dede-4ede-9ede-dededededede'}));

test('the coin is listed once, as what the board pays with', () => {
  expect(list([storeToken(coinPolicy)])).toEqual([['KRWC', true, true]]);
  expect(list([storeToken(wonPolicy)])).toEqual([['tKRW', true, true], ['KRWC', false, false]]);
  expect(list([storeToken(policy)])).toEqual([['TestUSDC', true, false], ['KRWC', false, false]]);
  // On another network the same address is another contract.
  // Added by its address, it is still the same coin under the same name.
  expect(list([storeToken(policy)], [{address: stableNetCoin.asset, symbol: 'WKRC', decimals: 18}])).toEqual([['TestUSDC', true, false], ['KRWC', false, false]]);
  expect(tokensOf(sepolia, undefined, defaultReadState, [], [storeToken(coinPolicy)]).map(token => [token.symbol, !!token.won])).toEqual([['KRWC', false], ['ETH', false]]);
});
test('the coin has one name at a faucet of any store', async () => {
  mockClient = {faucetStatus: async () => ({payer: policy.beneficiary, tokens: [{token: stableNetCoin.asset, symbol: 'WKRC', decimals: 18, amount: wei.toString(), perDay: 1, remaining: 1, state: 'ready'}]})};
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<FaucetPage t={ko => ko} account={policy.beneficiary} sends={[policy.asset]} />); for (let i = 0; i < 40; i++) await Promise.resolve(); });
  expect(JSON.stringify(tree.toJSON())).toContain('KRWC');
  expect(JSON.stringify(tree.toJSON())).not.toContain('WKRC');
  await Renderer.act(async () => tree.unmount());
});
test('only the store\'s own token is worded as won', () => {
  expect(list([storeToken(policy)], [storeToken(wonPolicy)].map(token => ({...token, address: '0x' + '9'.repeat(40)})))).toEqual([['TestUSDC', true, false], ['tKRW', false, false], ['KRWC', false, false]]);
  const amount = (900_000n * wei).toString();
  expect(tokenText(amount, {symbol: 'KRWC', decimals: 18, won: true})).toBe('900,000원');
  expect(tokenText(amount, {symbol: 'KRWC', decimals: 18, won: true}, 'en')).toBe('₩900,000');
  expect(tokenText(amount, {symbol: 'KRWC', decimals: 18})).toBe('900,000.00');
  expect(tokenText('90000000', {symbol: 'tKRW', decimals: 2, won: true})).toBe('900,000원');
  expect(tokenText((6_500n * wei - 1n).toString(), {symbol: 'KRWC', decimals: 18, won: true})).toBe('6,499.99원');
});
test('the faucet words the coin as the wallet does', async () => {
  const claims: string[] = [];
  const faucet = (token: string, symbol: string) => ({token, symbol, decimals: 18, amount: (2_000n * wei).toString(), perDay: 1, remaining: 1, state: 'ready'});
  mockClient = {faucetStatus: async () => ({payer: policy.beneficiary, tokens: [faucet(stableNetCoin.asset, 'WKRC'), faucet('0x' + '9'.repeat(40), 'tKRWC')]}), claimFaucet: async (token: string) => { claims.push(token); }};
  let tree!: Renderer.ReactTestRenderer;
  await Renderer.act(async () => { tree = Renderer.create(<FaucetPage t={ko => ko} account={policy.beneficiary} sends={[stableNetCoin.asset]} won={{[stableNetCoin.asset]: 'KRWC'}} />);
    for (let i = 0; i < 40; i++) await Promise.resolve(); });
  const text = JSON.stringify(tree.toJSON());
  expect(text).toContain('2,000원');
  expect(text).not.toContain('2,000.00');
  expect(text).toContain('보드가 아직 이 토큰을 보낼 수 없어 받지 않습니다');
  const button = tree.root.findAll(node => node.props.label === 'KRWC 받기' && typeof node.props.run === 'function')[0];
  await Renderer.act(async () => { button.props.run(); for (let i = 0; i < 40; i++) await Promise.resolve(); });
  expect(claims).toEqual([stableNetCoin.asset]);
  expect(JSON.stringify(tree.toJSON())).toContain('KRWC · 보내 달라고 했습니다');
  await Renderer.act(async () => tree.unmount());
});
test('the coin\'s page also gives the name an explorer uses, and no other token\'s does', async () => {
  const device = 'ab'.repeat(32), stored = {version: 2, exercised: true, mode: 'personal', verified: {boardFingerprint: device, address: policy.beneficiary, generation: 1, network: '8283'}} as const;
  const open = async (signed: typeof policy | typeof coinPolicy, name: string) => {
    let tree!: Renderer.ReactTestRenderer;
    await Renderer.act(async () => { tree = Renderer.create(<WalletTab t={ko => ko} language="ko" stored={stored as never} advice={{signed: [storeToken(signed as never)]} as never} active hide={() => {}} receive={() => {}} send={() => {}}
      giveBack={() => {}} networks={() => {}} board={() => {}} openOrder={() => {}} save={async () => {}} exchange={async () => { throw new Error('offline'); }} />); for (let i = 0; i < 40; i++) await Promise.resolve(); });
    const row = tree.root.findAll(node => String(node.props.accessibilityLabel ?? '').startsWith(name + ' ') && typeof node.props.onPress === 'function')[0];
    await Renderer.act(async () => { row.props.onPress(); for (let i = 0; i < 20; i++) await Promise.resolve(); });
    const text = JSON.stringify(tree.toJSON());
    await Renderer.act(async () => tree.unmount());
    return text;
  };
  for (const text of [await open(coinPolicy, 'KRWC'), await open(policy, 'KRWC')]) { expect(text).toContain('탐색기의 이름'); expect(text).toContain('WKRC'); }
  expect(await open(policy, 'TestUSDC')).not.toContain('탐색기의 이름');
});
