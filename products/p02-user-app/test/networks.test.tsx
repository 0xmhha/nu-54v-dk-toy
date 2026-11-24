import React from 'react';
import Renderer from 'react-test-renderer';
import {BackHandler, Platform} from 'react-native';
import {addCustom, builtInServers, changeBuiltIn, checkServer, describeCustom, networkRows, removeCustom, serverFailure, shownNetwork, type CustomDraft} from '../src/wallet/networks';
import {NetworksScreen} from '../src/wallet/NetworksScreen';
import {decodeWalletStorage} from '../src/wallet/profile';
import {defaultReadState, isWalletReadState, readEndpoint, readProfiles, type ReadProfileState} from '../src/wallet/readProfiles';

const genesis = '0x' + 'a1'.repeat(32);
type Chain = {id: string; genesis: string | null};
const chains: Record<string, Chain> = {};
const asked: string[] = [];
const exchange = async (url: string, packet: string) => {
  const {method, id} = JSON.parse(packet) as {method: string; id: number};
  asked.push(`${url} ${method}`);
  const chain = chains[url];
  if (!chain) throw new Error('offline');
  const result = method === 'eth_chainId' ? '0x' + BigInt(chain.id).toString(16) : method === 'eth_getBlockByNumber' ? chain.genesis && {hash: chain.genesis} : undefined;
  return JSON.stringify({jsonrpc: '2.0', id, result});
};
const draft: CustomDraft = {name: 'Arbitrum Sepolia', server: 'https://rpc.arb.example/', chainId: '421614', currency: 'eth', explorer: ''};
beforeEach(() => {
  for (const key of Object.keys(chains)) delete chains[key];
  asked.length = 0;
  chains['https://rpc.arb.example/'] = {id: '421614', genesis};
  chains['https://rpc.own.example/'] = {id: '8283', genesis: readProfiles.stable.anchor};
});

describe('networks', () => {
  test('six built in, and only the board network may receive', () => {
    const rows = networkRows();
    expect(rows.map(row => [row.key, row.sends])).toEqual([['stable', true], ['eth', false], ['base', false], ['btc', false], ['sol', false], ['xrp', false]]);
    expect(rows.every(row => row.server === builtInServers[row.key as 'eth'] && row.source === 'built-in')).toBe(true);
    for (const server of Object.values(builtInServers)) expect(readEndpoint(server)).toBe(server);
  });
  test('the owner, the store, the operator and the app are asked in that order, and only StableNet follows the store', () => {
    const own = changeBuiltIn(defaultReadState, 'stable', {server: 'https://rpc.own.example/'});
    const advice = {store: 'https://rpc.store.example/', operator: {stable: 'https://rpc.op.example/', eth: 'https://eth.op.example/'}};
    const source = (state: ReadProfileState, key: string) => networkRows(state, advice).find(row => row.key === key)!.source;
    expect([source(own, 'stable'), source(defaultReadState, 'stable'), source(defaultReadState, 'eth'), source(defaultReadState, 'base')]).toEqual(['own', 'store', 'operator', 'built-in']);
    expect(networkRows(changeBuiltIn(own, 'stable', {server: ''}), advice)[0].source).toBe('store');
  });
  test('a server of another network, one without the first block and one that is not https are refused', async () => {
    chains['https://other.example/'] = {id: '8453', genesis};
    chains['https://pruned.example/'] = {id: '8283', genesis: null};
    const fails = (url: string) => checkServer({key: 'stable'}, url, exchange).then(() => '', serverFailure);
    expect(await checkServer({key: 'stable'}, 'https://rpc.own.example', exchange)).toBe('https://rpc.own.example/');
    expect(await fails('https://other.example/')).toContain('다른 네트워크');
    expect(await fails('https://pruned.example/')).toContain('첫 블록');
    expect(await fails('http://rpc.own.example/')).toContain('https://');
    expect(await fails('https://nowhere.example/')).toContain('연결하지 못했습니다');
    expect(asked.some(line => line.includes('getBalance'))).toBe(false);
  });
  test('a hidden or removed network falls back to the board network', () => {
    const state = changeBuiltIn({...defaultReadState, shown: 'eth'}, 'eth', {hidden: true});
    expect(shownNetwork(networkRows(state), state).key).toBe('stable');
    expect(changeBuiltIn(defaultReadState, 'stable', {hidden: true}).disabled).toEqual([]);
    const gone = {...defaultReadState, shown: 'evm:5'};
    expect(shownNetwork(networkRows(gone), gone).key).toBe('stable');
  });
  test('adding keeps the first block as the identity, and refuses twins, built-ins and wrong chain IDs', async () => {
    const item = await describeCustom(draft, exchange);
    expect(item).toMatchObject({chainId: '421614', genesis, currency: 'ETH', explorer: '', capability: 'read-only'});
    const state = addCustom(defaultReadState, item);
    expect(isWalletReadState(JSON.parse(JSON.stringify(state)))).toBe(true);
    expect(() => addCustom(state, item)).toThrow('WALLET_CUSTOM_EXISTS');
    await expect(describeCustom({...draft, chainId: '1'}, exchange)).rejects.toThrow('WALLET_CUSTOM_IDENTITY');
    await expect(describeCustom({...draft, server: 'https://rpc.own.example/', chainId: '8283'}, exchange)).rejects.toThrow('WALLET_CUSTOM_SCHEMA');
    expect(networkRows(state).at(-1)).toMatchObject({key: 'evm:421614', sends: false, short: 'ETH'});
    expect(removeCustom({...state, shown: 'evm:421614'}, '421614')).toEqual({...defaultReadState, custom: []});
  });
  test('what is stored is checked again when it is loaded', () => {
    const raw = (walletRead: object) => JSON.stringify({version: 2, exercised: false, walletRead: {...defaultReadState, ...walletRead}});
    // Networks can be set before any board is registered.
    expect(decodeWalletStorage(raw({shown: 'base', links: {eth: {explorer: 'https://sepolia.etherscan.io/'}}})).walletRead?.shown).toBe('base');
    for (const bad of [{shown: 'evm:0'}, {shown: 5}, {links: {eth: {explorer: 'javascript:1'}}}, {links: {eth: {other: 'https://a.example/'}}}, {links: {nope: {}}}])
      expect(() => decodeWalletStorage(raw(bad))).toThrow('WALLET_STORAGE_CORRUPT');
  });
});

describe('the networks page', () => {
  let tree: Renderer.ReactTestRenderer, state: ReadProfileState, left = 0;
  const text = () => JSON.stringify(tree.toJSON());
  const draw = () => <NetworksScreen t={ko => ko} state={state} exchange={exchange} back={() => { left++; }} save={async change => { state = change(state); }} />;
  const flush = async () => { await Renderer.act(async () => { for (let i = 0; i < 40; i++) await Promise.resolve(); }); await Renderer.act(async () => { tree.update(draw()); }); };
  const pick = (label: string) => tree.root.findAll(node => typeof node.props.accessibilityLabel === 'string' && node.props.accessibilityLabel.startsWith(label) && typeof node.props.onPress === 'function')[0];
  const tap = async (label: string) => {
    expect([label, !!pick(label), !!pick(label)?.props.disabled]).toEqual([label, true, false]);
    await Renderer.act(async () => { pick(label).props.onPress(); });
    await flush();
  };
  const type = (label: string, value: string) => Renderer.act(async () => { tree.root.findAll(node => node.props.accessibilityLabel === label && node.props.onChangeText)[0].props.onChangeText(value); });
  beforeEach(async () => {
    Object.defineProperty(Platform, 'OS', {value: 'android', configurable: true});
    state = defaultReadState; left = 0;
    await Renderer.act(async () => { tree = Renderer.create(draw()); });
    await flush();
  });
  afterEach(async () => { await Renderer.act(async () => tree.unmount()); });

  test('a network is added, changed and removed without a build', async () => {
    await tap('네트워크 추가');
    for (const [label, key] of [['네트워크 이름', 'name'], ['서버 주소 (https)', 'server'], ['체인 ID', 'chainId'], ['코인 기호', 'currency']] as const) await type(label, draft[key]);
    await flush();
    await tap('확인하고 저장');
    expect(text()).toContain('네트워크를 추가했습니다');
    expect(text()).toContain('Arbitrum Sepolia');
    chains['https://rpc2.arb.example/'] = {id: '421614', genesis: '0x' + '00'.repeat(31) + '01'};
    await tap('Arbitrum Sepolia');
    await type('서버 주소 (https)', 'https://rpc2.arb.example/');
    await flush();
    await tap('저장');
    expect(text()).toContain('다른 네트워크의 서버');
    chains['https://rpc2.arb.example/'].genesis = genesis;
    await tap('저장');
    expect(state.custom![0].rpc).toBe('https://rpc2.arb.example/');
    await tap('Arbitrum Sepolia');
    await tap('네트워크 삭제');
    expect(text()).toContain('지워지지 않는 것');
    await tap('네트워크 삭제');
    expect(state.custom).toEqual([]);
    expect(text()).toContain('네트워크를 삭제했습니다');
  });
  test('a built-in network takes another server, can be hidden, and cannot be removed', async () => {
    await tap('StableNet Testnet');
    expect(pick('네트워크 삭제')).toBeUndefined();
    expect(pick('목록에서 숨기기')).toBeUndefined();
    await type('서버 주소 (https)', 'https://rpc.own.example');
    await flush();
    await tap('저장');
    expect(state.endpoints.stable).toBe('https://rpc.own.example/');
    await tap('StableNet Testnet');
    await tap('기본 서버로 되돌리기');
    expect(state.endpoints.stable).toBeUndefined();
    await tap('Ethereum Sepolia');
    await tap('목록에서 숨기기');
    expect(state.disabled).toEqual(['eth']);
    await tap('Ethereum Sepolia');
    await tap('목록에 다시 보이기');
    expect(state.disabled).toEqual([]);
  });
  test('each server says whether it answers, and system Back leaves one page at a time', async () => {
    expect(text()).toContain('연결 안 됨');
    const handlers: Parameters<typeof BackHandler.addEventListener>[1][] = [];
    const spy = jest.spyOn(BackHandler, 'addEventListener').mockImplementation((_name, run) => {
      handlers.push(run);
      return {remove: () => { handlers.splice(handlers.indexOf(run), 1); }} as never;
    });
    await tap('Base Sepolia');
    expect(handlers).toHaveLength(1);
    await Renderer.act(async () => { handlers.at(-1)!({} as never); });
    await flush();
    expect(text()).toContain('잔액 보기만');
    expect([handlers.length, left]).toEqual([0, 0]);
    spy.mockRestore();
  });
});
