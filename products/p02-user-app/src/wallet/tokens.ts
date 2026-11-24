// The tokens of the wallet tab. Where a token comes from decides whether the board can send it; its symbol does not.
import {normalizeEvmAddress, readEvmReceiveBalance, type EvmChain} from './evmTransfer.ts';
import {boardNetwork, isReadProfile, type Exchange, type NetworkRow} from './networks.ts';
import {formatReadAmount, readCustomEvmBalance, readProfiles, walletReadRpc, type ReadProfileState} from './readProfiles.ts';
import {assetsForAccount, type AssetPreference} from './walletCatalog.ts';

export type WalletToken = {
  key: string; symbol: string; decimals: number;
  /** Undefined for the network's own coin. */
  contract?: string;
  /** The board signs transfers of it, so it may be received. */
  sends: boolean;
  /** Added by the owner on this phone. */
  added?: AssetPreference;
};
export type ListedToken = {address: string; symbol: string; decimals: number};

const coins: Record<EvmChain, string> = {'8283': 'WKRC', '11155111': 'ETH', '84532': 'ETH'};
/** The store token of the pilot; the board's firmware signs its name. */
export const storeSymbol = 'TestUSDC';

/**
 * The tokens one network shows: what the board signs, then the operator's list, the owner's own, and the coin.
 * `signed` is what the board can send: today the one token of the store this phone is registered with.
 */
export function tokensOf(row: NetworkRow, owner: {boardFingerprint: string; generation: number} | undefined, state: ReadProfileState | undefined,
  signed: ListedToken[] = [], listed: ListedToken[] = []): WalletToken[] {
  if (row.custom) return [{key: 'coin', symbol: row.custom.currency, decimals: row.custom.decimals, sends: false}];
  if (!isReadProfile(row.key)) return [];
  const profile = readProfiles[row.key];
  if (!('evm' in profile)) return [];
  const out = new Map<string, WalletToken>();
  const put = (item: ListedToken, more: Partial<WalletToken> = {}) => {
    const contract = item.address.toLowerCase();
    if (!out.has(contract)) out.set(contract, {key: contract, contract, symbol: item.symbol, decimals: item.decimals, sends: false, ...more});
  };
  if (row.key === boardNetwork) for (const item of signed) put(item, {sends: true});
  for (const item of listed) put(item);
  if (owner) for (const item of assetsForAccount(state?.assets, owner, {id: row.key, account: 0, hidden: false})) {
    if (!item.hidden) put({address: item.contract, symbol: item.symbol ?? 'ERC-20', decimals: item.decimals}, {added: item});
  }
  return [...out.values(), {key: 'coin', symbol: coins[profile.evm], decimals: 18, sends: false}];
}

export type TokenAmount = {units: string; text: string};
export const amountText = (units: string, decimals: number) => {
  const [whole, fraction = ''] = formatReadAmount(units, decimals).split('.');
  return whole.replace(/\B(?=(\d{3})+(?!\d))/g, ',') + '.' + fraction.padEnd(2, '0');
};

export async function readToken(row: NetworkRow, token: WalletToken, account: string, exchange: Exchange): Promise<TokenAmount> {
  const rpc = walletReadRpc(row.server, exchange);
  if (row.custom) {
    const {amount} = await readCustomEvmBalance(row.custom, account, rpc);
    return {units: amount, text: amountText(amount, token.decimals)};
  }
  const profile = isReadProfile(row.key) ? readProfiles[row.key] : undefined;
  if (!profile || !('evm' in profile)) throw new Error('WALLET_READ_BOARD_ACCOUNT');
  const {amount} = await readEvmReceiveBalance(rpc, profile.evm, profile.anchor, account, token.contract, false);
  return {units: amount, text: amountText(amount, token.decimals)};
}

const asciiWord = /^[A-Za-z0-9.-]{1,12}$/;
/** An ABI string, or the bytes32 that older tokens answer with. */
function symbolOf(raw: unknown) {
  if (typeof raw !== 'string' || !/^0x(?:[0-9a-f]{64}){1,16}$/.test(raw)) return undefined;
  const bytes = raw.slice(2).match(/../g)!.map(pair => parseInt(pair, 16));
  const size = bytes.length >= 96 ? Number(BigInt('0x' + raw.slice(66, 130))) : 32;
  const body = bytes.length >= 96 ? bytes.slice(64, 64 + Math.min(size, 64)) : bytes.slice(0, bytes.indexOf(0) < 0 ? 32 : bytes.indexOf(0));
  const text = String.fromCharCode(...body);
  return asciiWord.test(text) ? text : undefined;
}

export type TokenReview = {contract: string; symbol: string; decimals: number; balance: string; block: string; height: string};
/** What a contract says about itself at one finalized block. Anyone can choose a name, so only the address identifies it. */
export async function reviewToken(row: NetworkRow, input: string, account: string, exchange: Exchange): Promise<TokenReview> {
  const contract = normalizeEvmAddress(input.trim());
  const profile = isReadProfile(row.key) ? readProfiles[row.key] : undefined;
  if (!profile || !('evm' in profile) || /^0x0+$/.test(contract)) throw new Error('TOKEN_NETWORK');
  const rpc = walletReadRpc(row.server, exchange);
  const {amount, decimals, block: height} = await readEvmReceiveBalance(rpc, profile.evm, profile.anchor, account, contract, true);
  const head = await rpc('eth_getBlockByNumber', ['finalized', false]) as {hash?: unknown} | null;
  if (typeof head?.hash !== 'string' || !/^0x[0-9a-f]{64}$/.test(head.hash)) throw new Error('TOKEN_BLOCK');
  const symbol = symbolOf(await rpc('eth_call', [{to: contract, data: '0x95d89b41'}, {blockHash: head.hash, requireCanonical: true}]).catch(() => undefined));
  if (decimals === undefined || !symbol) throw new Error('TOKEN_UNREADABLE');
  return {contract, symbol, decimals, balance: amountText(amount, decimals), block: head.hash, height};
}

export function addToken(state: ReadProfileState, owner: {boardFingerprint: string; generation: number}, row: NetworkRow, review: TokenReview): ReadProfileState {
  if (!isReadProfile(row.key)) throw new Error('TOKEN_NETWORK');
  const scope = {board: owner.boardFingerprint, generation: owner.generation, id: row.key, account: 0};
  const others = (state.assets ?? []).filter(item => !(item.board === scope.board && item.generation === scope.generation && item.id === scope.id &&
    item.account === 0 && item.contract === review.contract));
  if (others.length >= 12) throw new Error('TOKEN_FULL');
  return {...state, assets: [...others, {...scope, contract: review.contract, decimals: review.decimals, symbol: review.symbol, block: review.block, height: review.height,
    favorite: false, hidden: false}]};
}

export const removeToken = (state: ReadProfileState, token: AssetPreference): ReadProfileState =>
  ({...state, assets: (state.assets ?? []).filter(item => JSON.stringify(item) !== JSON.stringify(token))});

export const tokenFailure = (error: unknown) => {
  switch (error instanceof Error ? error.message : '') {
    case 'EVM_RECIPIENT': case 'EVM_RECIPIENT_CHECKSUM': case 'TOKEN_NETWORK': return '토큰 계약 주소가 맞지 않습니다.';
    case 'TOKEN_FULL': return '토큰은 12개까지 추가할 수 있습니다.';
    case 'TOKEN_UNREADABLE': case 'EVM_ASSET_DECIMALS': case 'EVM_BALANCE_SCHEMA': case 'TOKEN_BLOCK': return '이 주소에서 토큰 정보를 읽을 수 없습니다. 네트워크와 주소를 확인하세요.';
    default: return '서버에서 토큰 정보를 읽지 못했습니다. 잠시 뒤 다시 시도하세요.';
  }
};
export const isEvmRow = (row: NetworkRow) => !!row.custom || (isReadProfile(row.key) && 'evm' in readProfiles[row.key]);
