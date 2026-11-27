// The tokens of the wallet tab. Where a token comes from decides whether the board can send it; its symbol does not.
import {commerceAmount, commerceCartAmount, dollarText, dollarToken, isWon, paymentToken, stableNetCoin, tokenAmount, type CommercePaymentProfile, type PaymentToken} from '@nu54/protocol';
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
  /** The store's own token, and it is won. Another token with the same symbol is not shown as won. */
  won?: true;
};
export type ListedToken = {address: string; symbol: string; decimals: number; won?: true};

const coins: Record<EvmChain, string> = {'8283': stableNetCoin.label, '11155111': 'ETH', '84532': 'ETH'};
/** The token of the store this phone is registered with, as the wallet lists it. */
export const storeToken = (policy: CommercePaymentProfile): ListedToken => {
  const token = paymentToken(policy), {label, decimals} = token;
  return {address: policy.asset, symbol: label, decimals, ...(isWon(token) ? {won: true as const} : {})};
};

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
    // The coin has one name on every screen, whatever its token is called on chain.
    const symbol = row.key === boardNetwork && contract === stableNetCoin.asset ? stableNetCoin.label : item.symbol;
    if (!out.has(contract)) out.set(contract, {key: contract, contract, symbol, decimals: item.decimals, sends: false, ...more, ...(item.won && more.sends ? {won: true as const} : {})});
  };
  if (row.key === boardNetwork) for (const item of signed) put(item, {sends: true});
  for (const item of listed) put(item);
  if (owner) for (const item of assetsForAccount(state?.assets, owner, {id: row.key, account: 0, hidden: false})) {
    if (!item.hidden) put({address: item.contract, symbol: item.symbol ?? 'ERC-20', decimals: item.decimals}, {added: item});
  }
  // StableNet's coin is also a token: one balance, so it is listed once.
  const listedCoin = row.key === boardNetwork && out.has(stableNetCoin.asset);
  return [...out.values(), ...(listedCoin ? [] : [{key: 'coin', symbol: coins[profile.evm], decimals: 18, sends: false}])];
}

export type TokenAmount = {units: string; text: string};
export const amountText = (units: string, decimals: number) => {
  const [whole, fraction = ''] = formatReadAmount(units, decimals).split('.');
  return whole.replace(/\B(?=(\d{3})+(?!\d))/g, ',') + '.' + fraction.padEnd(2, '0');
};

/**
 * What an amount is to the customer, which decides how the phone makes it whole (owner decision 2026-11-27: every
 * won amount on the phone is a whole number of won; the operations web keeps the parts). What the customer has or
 * gets back is cut off, so no screen shows more than there is. What the customer pays or lacks is rounded up,
 * so no screen shows less than what leaves the wallet or less than what must be added. The tax inside a price is cut
 * off. This is for the screen alone: what is sent, signed or compared stays in exact base units.
 */
export type Shown = 'balance' | 'refund' | 'tax' | 'charge' | 'shortfall';
type Counted = Pick<PaymentToken, 'decimals' | 'symbol'>;
const roundedUp: Shown[] = ['charge', 'shortfall'];
/** Base units at a whole `unit`: cut off, or rounded up for what is paid or missing. Below zero it does the same. */
function wholeUnits(units: bigint, unit: bigint, shown: Shown): bigint {
  const rest = ((units % unit) + unit) % unit;
  return rest === 0n ? units : units - rest + (roundedUp.includes(shown) ? unit : 0n);
}
/** The smallest amount of a store token a screen shows, in base units: one won, or one cent of a dollar token. */
const shownUnit = (token: Counted) => 10n ** BigInt(isWon(token) ? token.decimals : Math.max(0, token.decimals - 2));
/**
 * An amount of a store token in base units, as the phone shows it: whole won. A dollar token keeps its own rule (its
 * text is cut off at the cent), so its amount is returned as it is. Only for a screen, never for a request.
 */
export const shownUnits = (units: string | bigint, token: Counted, shown: Shown): bigint =>
  isWon(token) ? wholeUnits(BigInt(units), shownUnit(token), shown) : BigInt(units);
/**
 * What a basket lacks, in base units, for the screen: counted in the smallest unit the basket shows (a whole won, or
 * a cent) and never less than what is missing, so that adding it is enough. A balance is cut off, never rounded up
 * (N31); what is missing is the other side of the same cut. Won are whole on a screen, so a part of a won that is
 * missing reads as one won.
 */
export function missingUnits(required: bigint, balance: bigint, token: Counted): string {
  return wholeUnits(required > balance ? required - balance : 0n, shownUnit(token), 'shortfall').toString();
}
/** A balance as the basket shows it, in base units: cut off at a whole won, or at a cent. Never rounded up. */
export function shownBalance(balance: bigint, token: Counted): string {
  return wholeUnits(balance, shownUnit(token), 'balance').toString();
}
/**
 * The lines of a receipt in the base units the screen shows, so that the rows add up on it. Every amount is made
 * whole on its running sum: the lines add up to the amount paid (rounded up) and their taxes to the tax (cut off).
 * The supply value of a line is what is left of its shown amount, so supply value and tax add up to it as well.
 * Without a token, or with a dollar token, nothing changes.
 */
export function receiptLines(items: {total: string | bigint; tax?: string | bigint}[], token?: Counted) {
  const whole = (sum: bigint, shown: Shown) => token ? shownUnits(sum, token, shown) : sum;
  let paid = 0n, taxed = 0n, shownPaid = 0n, shownTaxed = 0n;
  return items.map(item => {
    paid += BigInt(item.total);
    taxed += BigInt(item.tax ?? 0);
    const total = whole(paid, 'charge') - shownPaid, tax = whole(taxed, 'tax') - shownTaxed;
    shownPaid += total;
    shownTaxed += tax;
    return {total, tax, supply: total - tax};
  });
}

const wonText = (units: string, decimals: number, language: 'ko' | 'en') =>
  tokenAmount(wholeUnits(BigInt(units), 10n ** BigInt(decimals), 'balance'), {name: '', symbol: stableNetCoin.symbol, label: '', decimals}, language);
const dollarSymbols = ['TestUSDC', 'tUSDC', 'tUSDT', 'USDC', 'USDT'];
/** A token's amount in a list: what is held or received. Won are whole and cut off, dollars are cut off after two decimals, anything else is in full. */
export const tokenText = (units: string, token: Pick<ListedToken, 'symbol' | 'decimals' | 'won'>, language: 'ko' | 'en' = 'ko') => token.won
  ? wonText(units, token.decimals, language) : dollarSymbols.includes(token.symbol)
  ? dollarText(units, token.decimals).replace(/\B(?=(\d{3})+\.)/g, ',') : amountText(units, token.decimals);

/** The store this phone is registered with: its token's address and how that token signs and is shown. */
export type Store = {asset: string; token: PaymentToken; branch?: string};
export const storeOf = (policy: CommercePaymentProfile, branch?: string): Store => ({asset: policy.asset.toLowerCase(), token: paymentToken(policy), ...(branch ? {branch} : {})});
export const sameStore = (a: Store | undefined, b: Store) => !!a && a.asset === b.asset && JSON.stringify(a.token) === JSON.stringify(b.token);
export const unknownAmount = {ko: '확인 안 됨', en: 'Not confirmed'};
/** An amount of one asset. Only the registered store's token is known; nothing else is shown as if it were dollars. */
export const assetMoney = (units: string | bigint, asset: string, store: Store | undefined, language: 'ko' | 'en' = 'ko', shown: Shown = 'balance') =>
  store?.asset === asset.toLowerCase() ? storeMoney(units, store.token, language, shown) : unknownAmount[language];

/**
 * An amount of the store's token. Without a registration nobody knows which token a board signs. Won are whole: cut
 * off unless `shown` says the amount is paid or missing.
 */
export const storeMoney = (units: string | bigint, token: PaymentToken | undefined, language: 'ko' | 'en' = 'ko', shown: Shown = 'balance'): string =>
  !token ? unknownAmount[language] : token.symbol === dollarToken.symbol ? `${tokenText(units.toString(), token)} ${token.label}`
    : tokenAmount(shownUnits(units, token, shown), token, language);

const counted = /^(0|[1-9][0-9]*)$/;
/** An order's amount in base units as the phone shows it: whole won of the registered store's token, anything else as it is. */
export const orderUnits = (units: string | bigint, asset: string, policy: CommercePaymentProfile | undefined, shown: Shown): bigint =>
  policy?.asset === asset ? shownUnits(units, paymentToken(policy), shown) : BigInt(units);
const orderText = (units: string, asset: string, policy: CommercePaymentProfile | undefined, shown: Shown) =>
  counted.test(units) ? orderUnits(units, asset, policy, shown).toString() : units;
/** An amount of an order on the phone: the shared wording, in whole won. */
export const orderMoney = (units: string, asset: string, policy: CommercePaymentProfile | undefined, shown: Shown, language: 'ko' | 'en' = 'ko') =>
  commerceAmount(orderText(units, asset, policy, shown), asset, policy, language);
/** An amount of a basket on the phone: the shared wording, in whole won. */
export const cartMoney = (units: string, asset: string, policy: CommercePaymentProfile | undefined, shown: Shown, language: 'ko' | 'en' = 'ko') =>
  commerceCartAmount(orderText(units, asset, policy, shown), asset, policy, language);

export async function readToken(row: NetworkRow, token: WalletToken, account: string, exchange: Exchange): Promise<TokenAmount> {
  const rpc = walletReadRpc(row.server, exchange);
  if (row.custom) {
    const {amount} = await readCustomEvmBalance(row.custom, account, rpc);
    return {units: amount, text: tokenText(amount, token)};
  }
  const profile = isReadProfile(row.key) ? readProfiles[row.key] : undefined;
  if (!profile || !('evm' in profile)) throw new Error('WALLET_READ_BOARD_ACCOUNT');
  const {amount} = await readEvmReceiveBalance(rpc, profile.evm, profile.anchor, account, token.contract, false);
  return {units: amount, text: tokenText(amount, token)};
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
  return {contract, symbol, decimals, balance: tokenText(amount, {symbol, decimals}), block: head.hash, height};
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
