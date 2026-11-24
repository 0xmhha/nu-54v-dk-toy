import React from 'react';
import Renderer from 'react-test-renderer';
import {defaultReadState, readProfiles, type ReadProfileState} from '../src/wallet/readProfiles';
import {networkRows} from '../src/wallet/networks';
import {decodeWalletStorage, type WalletStorage} from '../src/wallet/profile';
import {amountText, reviewToken, tokensOf} from '../src/wallet/tokens';
import {WalletTab} from '../src/wallet/WalletTab';

import {faucetFailure, FaucetPage} from '../src/wallet/FaucetPage';
import {fromOperator} from '../src/wallet/walletAdvice';

let mockClient: object = {}, mockKey = 0;
jest.mock('../src/wallet/BoardLife.tsx', () => ({credential: async () => ({client: mockClient})}));
jest.mock('../src/merchant/customerClient.ts', () => ({...jest.requireActual('../src/merchant/customerClient.ts'), walletId: async () => `key-${mockKey++}`}));
jest.mock('../src/specs/NativeCommerceHttp.ts', () => ({__esModule: true, default: null}));
jest.mock('../src/specs/NativeSubscriptionAlerts.ts', () => ({__esModule: true, default: null}));

const store = '0x' + '11'.repeat(20), won = '0x' + '22'.repeat(20), fake = '0x' + '33'.repeat(20), account = '0x' + 'aa'.repeat(20), device = 'ab'.repeat(32);
const block = '0x' + 'b1'.repeat(32);
const word = (value: bigint) => '0x' + value.toString(16).padStart(64, '0');
const string = (value: string) => '0x' + (32n).toString(16).padStart(64, '0') + BigInt(value.length).toString(16).padStart(64, '0') +
  [...value].map(char => char.charCodeAt(0).toString(16)).join('').padEnd(64, '0');
let balances: Record<string, bigint>, symbols: Record<string, string>, down: boolean;
const exchange = async (url: string, packet: string) => {
  if (down) throw new Error('offline');
  const {method, params, id} = JSON.parse(packet) as {method: string; params: {to?: string; data?: string}[]; id: number};
  const eth = url.includes('sepolia'), chain = eth ? readProfiles.eth : readProfiles.stable;
  const call = () => {
    const {to = '', data = ''} = params[0];
    if (data.startsWith('0x70a08231')) return word(balances[to] ?? 0n);
    if (data === '0x313ce567') return word(to === won ? 18n : 6n);
    return symbols[to] ?? '0x';
  };
  const result = method === 'eth_chainId' ? '0x' + BigInt(chain.evm).toString(16)
    : method === 'eth_getBlockByNumber' ? (params[0] as unknown === '0x0' ? {hash: chain.anchor} : {hash: block, number: '0x10'})
    : method === 'eth_getBalance' ? '0x' + (balances.coin ?? 0n).toString(16) : call();
  return JSON.stringify({jsonrpc: '2.0', id, result});
};
const advice = {store: 'https://rpc.store.example/', signed: [{address: store, symbol: 'TestUSDC', decimals: 6}], listed: {stable: [{address: won, symbol: 'tKRWC', decimals: 18}]}};
let stored: WalletStorage, tree: Renderer.ReactTestRenderer;
const went: string[] = [];
const text = () => JSON.stringify(tree.toJSON());
const draw = () => <WalletTab t={ko => ko} stored={stored} advice={advice} exchange={exchange} active hide={() => went.push('hide')} receive={() => went.push('receive')} send={() => went.push('send')}
  giveBack={() => went.push('return')} networks={() => went.push('networks')} board={() => went.push('board')} openOrder={() => {}}
  save={async (change: (state: ReadProfileState) => ReadProfileState) => { stored = decodeWalletStorage(JSON.stringify({...stored, walletRead: change(stored.walletRead ?? defaultReadState)})); }} />;
const flush = async () => { for (let i = 0; i < 3; i++) { await Renderer.act(async () => { for (let j = 0; j < 60; j++) await Promise.resolve(); }); await Renderer.act(async () => { tree.update(draw()); }); } };
const pick = (label: string) => tree.root.findAll(node => typeof node.props.accessibilityLabel === 'string' && node.props.accessibilityLabel.startsWith(label) && typeof node.props.onPress === 'function')[0];
const tap = async (label: string) => {
  expect([label, !!pick(label)]).toEqual([label, true]);
  await Renderer.act(async () => { pick(label).props.onPress(); });
  await flush();
};
beforeEach(async () => {
  jest.useFakeTimers();
  went.length = 0; down = false;
  balances = {[store]: 82_000_000n, [won]: 10_000n * 10n ** 18n, coin: 0n};
  symbols = {[fake]: string('TestUSDC')};
  stored = {version: 2, exercised: true, mode: 'personal', verified: {boardFingerprint: device, address: account, generation: 1, network: '8283'},
    spendingBoard: {device, account, generation: 1, testedAt: 1}};
  await Renderer.act(async () => { tree = Renderer.create(draw()); });
  await flush();
});
afterEach(async () => { await Renderer.act(async () => tree.unmount()); jest.useRealTimers(); });

test('tokens show their amounts, and only what the board signs can be received', async () => {
  for (const shown of ['82.00', '10,000.00', 'tKRWC', 'WKRC']) expect(text()).toContain(shown);
  await tap('TestUSDC 82.00');
  expect(pick('받기')).toBeDefined();
  await tap('지갑으로 돌아가기');
  await tap('tKRWC 10,000.00');
  expect(pick('받기')).toBeUndefined();
  expect(text()).toContain('이 자산을 보드 계정으로 보내지 마세요');
});
test('another network shows its balance and no way to receive', async () => {
  balances.coin = 5n * 10n ** 17n;
  await tap('네트워크 StableNet Testnet');
  await tap('Ethereum Sepolia');
  expect(stored.walletRead?.shown).toBe('eth');
  expect(text()).toContain('0.50');
  expect(pick('받기')).toBeUndefined();
  await tap('네트워크 Ethereum Sepolia');
  await tap('Bitcoin Signet');
  expect(text()).toContain('보드가 이 네트워크의 계정을 만들지 않습니다');
  expect(text()).not.toContain('0.50');
});
test('a hidden balance shows nothing, and a silent server says so', async () => {
  stored = {...stored, walletRead: {...defaultReadState, selection: {...defaultReadState.selection, hidden: true}}};
  await flush();
  expect(text()).not.toContain('82.00');
  stored = {...stored, walletRead: defaultReadState};
  down = true;
  await Renderer.act(async () => { jest.advanceTimersByTime(30_000); });
  await flush();
  expect(text()).not.toContain('82.00');
  expect(text()).toContain('서버에서 잔액을 읽지 못했습니다');
});
test('a token is added by its address, and one with a known name is pointed out', async () => {
  balances[fake] = 7_000_000n;
  await tap('토큰 추가');
  const field = () => tree.root.findAll(node => node.props.accessibilityLabel === '토큰 계약 주소' && node.props.onChangeText)[0];
  await Renderer.act(async () => { field().props.onChangeText(fake); });
  await tap('토큰 정보 확인');
  expect(text()).toContain('기호가 같은 토큰이 이미 있습니다');
  await tap('토큰 추가');
  expect(stored.walletRead?.assets).toMatchObject([{contract: fake, symbol: 'TestUSDC', decimals: 6}]);
  expect(text()).toContain('7.00');
  await tap('TestUSDC 7.00');
  expect(pick('받기')).toBeUndefined();
  await tap('목록에서 지우기');
  expect(stored.walletRead?.assets).toEqual([]);
});
test('an address that is no token is refused', async () => {
  const row = networkRows(defaultReadState, advice)[0];
  await expect(reviewToken(row, won, account, exchange)).rejects.toThrow('TOKEN_UNREADABLE');
  await expect(reviewToken(row, '0x12', account, exchange)).rejects.toThrow('EVM_RECIPIENT');
  expect(await reviewToken(row, fake, account, exchange)).toMatchObject({symbol: 'TestUSDC', decimals: 6, balance: '0.00'});
});
test('an operator list cannot make a token sendable', () => {
  const row = networkRows(defaultReadState, advice)[0];
  expect(tokensOf(row, undefined, defaultReadState, [], [{address: store, symbol: 'TestUSDC', decimals: 6}])[0].sends).toBe(false);
  expect(tokensOf(networkRows()[1], undefined, defaultReadState, [{address: store, symbol: 'TestUSDC', decimals: 6}]).map(token => token.symbol)).toEqual(['ETH']);
  expect([amountText('1234567000000', 6), amountText('1', 18), amountText('0', 6)]).toEqual(['1,234,567.00', '0.000000000000000001', '0.00']);
});
test('an operator list names servers and tokens of known networks only, and nothing it names can be sent', () => {
  const advice = fromOperator({issuedAt: 1, expiresAt: 2, sequence: 3, networks: [
    {chainId: 11155111, name: 'Sepolia', rpc: 'https://eth.list.example', explorer: 'https://scan.example', gasCoinUrl: 'https://gas.example',
      tokens: [{address: won, symbol: 'tKRWC', name: 'Test won', decimals: 18}]},
    {id: 'bitcoin-signet', name: 'Signet', rpc: 'https://btc.list.example/api'},
    {chainId: 1, name: 'Mainnet', rpc: 'https://main.list.example'},
    {id: 'solana-devnet', name: 'Solana', rpc: 'https://sol.list.example/?key=1'}]});
  expect(advice.operator).toEqual({eth: 'https://eth.list.example/', btc: 'https://btc.list.example/api'});
  const row = networkRows(defaultReadState, advice)[1];
  expect(row).toMatchObject({server: 'https://eth.list.example/', source: 'operator', explorer: 'https://scan.example/', gasCoin: 'https://gas.example/', sends: false});
  expect(tokensOf(row, undefined, defaultReadState, [], advice.listed?.eth).map(token => [token.symbol, token.sends])).toEqual([['tKRWC', false], ['ETH', false]]);
});

describe('test tokens', () => {
  const token = (change: object = {}) => ({token: store, symbol: 'TestUSDC', decimals: 6, amount: '5000000', perDay: 1, remaining: 1, state: 'ready', ...change});
  const client = {faucetStatus: jest.fn(), claimFaucet: jest.fn()};
  const draw = async () => {
    await Renderer.act(async () => tree.unmount());
    await Renderer.act(async () => { tree = Renderer.create(<FaucetPage t={ko => ko} account={account} sends={[store]} />); });
    await Renderer.act(async () => { for (let i = 0; i < 40; i++) await Promise.resolve(); });
  };
  beforeEach(() => { mockClient = client; client.faucetStatus.mockReset(); client.claimFaucet.mockReset(); });
  test('only a token the board can send is offered, and only for the enrolled account', async () => {
    client.faucetStatus.mockResolvedValue({payer: account, tokens: [token(), token({token: won, symbol: 'tKRWC', decimals: 18})]});
    await draw();
    expect([!!pick('TestUSDC 받기'), !!pick('tKRWC 받기')]).toEqual([true, false]);
    expect(text()).toContain('보드가 아직 이 토큰을 보낼 수 없어 받지 않습니다');
    client.faucetStatus.mockResolvedValue({payer: won, tokens: [token()]});
    await draw();
    expect(pick('TestUSDC 받기')).toBeUndefined();
    expect(text()).toContain('다른 보드 계정이 이 매장에 등록되어 있습니다');
  });
  test('a lost reply is asked again under the same key, an answer under a new one', async () => {
    client.faucetStatus.mockResolvedValue({payer: account, tokens: [token()]});
    await draw();
    const claim = async () => { await Renderer.act(async () => { pick('TestUSDC 받기').props.onPress(); for (let i = 0; i < 40; i++) await Promise.resolve(); }); };
    client.claimFaucet.mockRejectedValueOnce(new Error('network down'));
    await claim();
    expect(text()).toContain('서버에 연결하지 못했습니다');
    client.claimFaucet.mockRejectedValueOnce(new Error('OUT_OF_STOCK'));
    await claim();
    expect(text()).toContain('모자랍니다');
    client.claimFaucet.mockResolvedValueOnce(undefined);
    await claim();
    const [first, second, third] = client.claimFaucet.mock.calls.map(call => call[1]);
    expect([first === second, second === third]).toEqual([true, false]);
    expect(text()).toContain('보내 달라고 했습니다');
  });
  test('each state and each refusal has its own words', async () => {
    for (const [change, shown] of [[{state: 'open', remaining: 0}, '보내는 중'], [{state: 'limit', remaining: 0, availableAt: 1_900_000_000}, '이후 다시 받을 수 있습니다'],
      [{state: 'empty'}, '모자람'], [{claim: {amount: '1', to: account, createdAt: 1, state: 'unknown'}}, '지난번 결과를 확인하지 못했습니다']] as const) {
      client.faucetStatus.mockResolvedValue({payer: account, tokens: [token(change)]});
      await draw();
      expect(text()).toContain(shown);
    }
    const codes = ['NO_CREDENTIAL', 'UNSUPPORTED_CAPABILITY', 'NOT_AUTHORIZED', 'MISMATCHED_EVIDENCE', 'REPLAY_CONFLICT', 'CHECKOUT_CONFLICT', 'BUSY', 'OUT_OF_STOCK', 'EXPIRED', 'anything else'];
    expect(new Set(codes.map(code => faucetFailure(new Error(code)))).size).toBe(codes.length);
    client.faucetStatus.mockRejectedValue(new Error('UNSUPPORTED_CAPABILITY'));
    await draw();
    expect(text()).toContain('이 매장에서는 시험 토큰을 주지 않습니다');
  });
});
