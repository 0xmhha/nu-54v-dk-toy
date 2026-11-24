// The networks of the wallet: which server each one is read from, and what the board can do on it.
// Reading balances and sending are different things. A network is only offered for receiving when the
// board can also send from it, so money never reaches an address it cannot leave.
import {verifyPublicSignet, publicSignetIndexer, signetIndexer} from './signetIndexer.ts';
import {
  defaultReadState, readCustomEvmDescriptor, readEndpoint, readProfiles, setCustomEvmDescriptor, verifyCustomEvmNetwork, verifyReadNetwork, walletReadRpc,
  type CustomEvmDescriptor, type ReadProfileId, type ReadProfileState,
} from './readProfiles.ts';

/** Public servers that answer for the first block, which is how a network is recognized. Checked 2026-11-24. */
export const builtInServers: Record<ReadProfileId, string> = {
  stable: 'https://api.test.stablenet.network/',
  eth: 'https://ethereum-sepolia-rpc.publicnode.com/',
  base: 'https://base-sepolia.gateway.tenderly.co/',
  btc: publicSignetIndexer,
  sol: 'https://api.devnet.solana.com/',
  xrp: 'https://s.altnet.rippletest.net:51234/',
};

/** The spending firmware signs for this network only. */
export const boardNetwork: ReadProfileId = 'stable';

export type ServerSource = 'own' | 'store' | 'operator' | 'built-in';
/** What others chose for the owner: the store registration's server (StableNet) and the operator's signed list. */
export type NetworkAdvice = {store?: string; operator?: Partial<Record<ReadProfileId, string>>; pages?: Partial<Record<ReadProfileId, {explorer?: string; gasCoin?: string}>>};
export type NetworkKey = ReadProfileId | `evm:${string}`;
export type NetworkRow = {
  key: NetworkKey; label: string; short: string; chainId?: string; server: string; source: ServerSource; explorer?: string; gasCoin?: string;
  /** The board can send from it, so its address may be shown. */
  sends: boolean; hidden: boolean; custom?: CustomEvmDescriptor;
};

export const isReadProfile = (key: string): key is ReadProfileId => Object.hasOwn(readProfiles, key);
const shorts: Record<ReadProfileId, string> = {stable: 'SN', eth: 'ETH', base: 'BASE', btc: 'BTC', sol: 'SOL', xrp: 'XRP'};

/** The owner's choice first, then the store's server for StableNet, the operator's list, and the built-in one. */
export function serverFor(id: ReadProfileId, state: ReadProfileState = defaultReadState, advice: NetworkAdvice = {}): {server: string; source: ServerSource} {
  const own = state.endpoints[id];
  if (own) return {server: own, source: 'own'};
  if (id === boardNetwork && advice.store) return {server: advice.store, source: 'store'};
  const operator = advice.operator?.[id];
  return operator ? {server: operator, source: 'operator'} : {server: builtInServers[id], source: 'built-in'};
}

export function networkRows(state: ReadProfileState = defaultReadState, advice: NetworkAdvice = {}): NetworkRow[] {
  const built = (Object.keys(readProfiles) as ReadProfileId[]).map((id): NetworkRow => {
    const profile = readProfiles[id];
    return {key: id, label: profile.label, short: shorts[id], ...('evm' in profile ? {chainId: profile.evm} : {}), ...serverFor(id, state, advice),
      ...advice.pages?.[id], ...state.links?.[id], sends: id === boardNetwork, hidden: !!state.disabled?.includes(id)};
  });
  const custom = (state.custom ?? []).map((item): NetworkRow => ({key: `evm:${item.chainId}`, label: item.name, short: item.currency.slice(0, 4), chainId: item.chainId,
    server: item.rpc, source: 'own', ...(item.explorer ? {explorer: item.explorer} : {}), sends: false, hidden: false, custom: item}));
  return [...built, ...custom];
}

/** The network the wallet tab shows. A hidden or removed one falls back to the board's. */
export function shownNetwork(rows: NetworkRow[], state: ReadProfileState = defaultReadState): NetworkRow {
  return rows.find(row => row.key === state.shown && !row.hidden) ?? rows.find(row => row.key === boardNetwork)!;
}

export type Exchange = (url: string, packet: string) => Promise<string>;
/** Fails unless the server answers for this very network. No address is sent. */
export async function checkServer(row: Pick<NetworkRow, 'key' | 'custom'>, server: string, exchange: Exchange) {
  const endpoint = readEndpoint(server);
  if (row.custom) await verifyCustomEvmNetwork(row.custom, walletReadRpc(endpoint, exchange));
  else if (row.key === 'btc' && endpoint === publicSignetIndexer) await verifyPublicSignet(signetIndexer(exchange), readProfiles.btc.anchor);
  else if (isReadProfile(row.key)) await verifyReadNetwork(walletReadRpc(endpoint, exchange), row.key);
  else throw new Error('WALLET_READ_CHAIN');
  return endpoint;
}

/** Why a server was not taken, in words the owner can act on. */
export function serverFailure(error: unknown) {
  switch (error instanceof Error ? error.message : '') {
    case 'WALLET_READ_HTTPS': return '주소 형식이 맞지 않습니다. https://로 시작하는 주소를 입력하세요.';
    case 'WALLET_READ_CHAIN': case 'WALLET_CUSTOM_IDENTITY': case 'EVM_RPC_CHAIN': return '이 서버는 다른 네트워크의 서버입니다.';
    case 'EVM_RPC_GENESIS_UNAVAILABLE': return '이 서버는 첫 블록을 알려 주지 않아 네트워크를 확인할 수 없습니다. 다른 서버를 쓰세요.';
    case 'WALLET_CUSTOM_SCHEMA': case 'WALLET_CUSTOM_BUILT_IN': return '입력한 값을 확인하세요. 앱에 이미 있는 네트워크는 추가할 수 없습니다.';
    case 'WALLET_CUSTOM_CAPACITY': return '네트워크는 8개까지 추가할 수 있습니다.';
    case 'WALLET_CUSTOM_EXISTS': return '같은 체인 ID의 네트워크가 이미 있습니다.';
    default: return '서버에 연결하지 못했습니다. 주소와 인터넷 연결을 확인하세요.';
  }
}

export type CustomDraft = {name: string; server: string; chainId: string; currency: string; explorer: string};
/** Reads the first block of the network being added and keeps it as the network's identity. */
export async function describeCustom(draft: CustomDraft, exchange: Exchange): Promise<CustomEvmDescriptor> {
  const server = readEndpoint(draft.server.trim()), chainId = draft.chainId.trim();
  if (!/^[1-9][0-9]{0,19}$/.test(chainId)) throw new Error('WALLET_CUSTOM_SCHEMA');
  const rpc = walletReadRpc(server, exchange);
  const told = await rpc('eth_chainId', []);
  if (typeof told !== 'string' || !/^0x[0-9a-f]{1,16}$/.test(told) || BigInt(told).toString() !== chainId) throw new Error('WALLET_CUSTOM_IDENTITY');
  const first = await rpc('eth_getBlockByNumber', ['0x0', false]) as {hash?: unknown} | null;
  if (first === null) throw new Error('EVM_RPC_GENESIS_UNAVAILABLE');
  if (typeof first?.hash !== 'string') throw new Error('WALLET_CUSTOM_IDENTITY');
  return readCustomEvmDescriptor({engine: 'evm', name: draft.name.trim(), chainId, genesis: first.hash, rpc: server, explorer: draft.explorer.trim(),
    currency: draft.currency.trim().toUpperCase(), decimals: 18, feeCap: '1', confirmations: 0});
}

export function addCustom(state: ReadProfileState, item: CustomEvmDescriptor): ReadProfileState {
  if (state.custom?.some(row => row.chainId === item.chainId)) throw new Error('WALLET_CUSTOM_EXISTS');
  return {...state, custom: setCustomEvmDescriptor(state.custom ?? [], item)};
}

export function removeCustom(state: ReadProfileState, chainId: string): ReadProfileState {
  const {shown, ...rest} = state;
  return {...rest, custom: (state.custom ?? []).filter(item => item.chainId !== chainId), ...(shown && shown !== `evm:${chainId}` ? {shown} : {})};
}

/** An empty server goes back to what the store, the operator or the app offers. */
export function changeBuiltIn(state: ReadProfileState, id: ReadProfileId, change: {server?: string; explorer?: string; gasCoin?: string; hidden?: boolean}): ReadProfileState {
  const endpoints = {...state.endpoints}, links = {...state.links};
  if (change.server !== undefined) {
    if (change.server) endpoints[id] = readEndpoint(change.server); else delete endpoints[id];
  }
  if (change.explorer !== undefined || change.gasCoin !== undefined) {
    const next = {...links[id]};
    for (const key of ['explorer', 'gasCoin'] as const) {
      const value = change[key]?.trim();
      if (value === undefined) continue;
      if (value) next[key] = readEndpoint(value); else delete next[key];
    }
    if (Object.keys(next).length) links[id] = next; else delete links[id];
  }
  const disabled = change.hidden === undefined ? state.disabled
    : [...(state.disabled ?? []).filter(item => item !== id), ...(change.hidden && id !== boardNetwork ? [id] : [])];
  // Balances read from another server are not carried over.
  const observations = change.server === undefined ? state.observations : state.observations.filter(row => row.scope.split('/')[2] !== id);
  const next: ReadProfileState = {...state, endpoints, observations, ...(disabled ? {disabled} : {}), links};
  if (!Object.keys(links).length) delete next.links;
  return next;
}
