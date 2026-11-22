import {checkEvmNetwork, normalizeEvmAddress} from './evmTransfer.ts';
import {readProfiles, type ReadProfileId, type ReadProfileState, type ReadSelection} from './readProfiles.ts';

type Owner = {boardFingerprint: string; generation: number};
type Scope = {board: string; generation: number; id: ReadProfileId; account: number};
export type AccountPreference = Scope & {name?: string; hidden: boolean};
export type ReviewedAsset = Scope & {endpoint: string; contract: string; decimals: number; block: string; height: string};
export type AssetPreference = Omit<ReviewedAsset, 'endpoint'> & {favorite: boolean; hidden: boolean};
const hash = /^0x[a-f0-9]{64}$/;
const validName = (value: string) => value.length >= 1 && value.length <= 32 &&
  ![...value].some(char => char.charCodeAt(0) < 32 || char.charCodeAt(0) === 127);
export const catalogKey = (scope: Scope) => [scope.board, scope.generation, scope.id, scope.account].join('/');

const scopeOf = (owner: Owner, selection: ReadSelection): Scope => ({
  board:owner.boardFingerprint, generation:owner.generation, id:selection.id, account:selection.account,
});
const equalScope = (a: Scope, b: Scope) =>
  a.board === b.board && a.generation === b.generation && a.id === b.id && a.account === b.account;

export function accountPreference(records: AccountPreference[] | undefined, owner: Owner, selection: ReadSelection) {
  const scope = scopeOf(owner, selection);
  return records?.find(item => equalScope(item, scope));
}

export function saveAccountPreference(records: AccountPreference[] = [], owner: Owner, selection: ReadSelection, name: string|undefined, hidden: boolean) {
  const label = name?.trim();
  if (label!==undefined && !validName(label))
    throw new Error('WALLET_ACCOUNT_NAME');
  const scope = scopeOf(owner, selection);
  const remaining = records.filter(item => !equalScope(item, scope));
  if (!label && !hidden) return remaining;
  if (remaining.length >= 12) throw new Error('WALLET_ACCOUNT_CAPACITY');
  return [...remaining, {...scope, ...(label ? {name:label} : {}), hidden}];
}

export function assetsForAccount(records: AssetPreference[] | undefined, owner: Owner, selection: ReadSelection) {
  const scope = scopeOf(owner, selection);
  return (records ?? []).filter(item => equalScope(item, scope));
}

export async function reviewEvmAsset(rpc: Parameters<typeof checkEvmNetwork>[0], owner: Owner,
  selection: ReadSelection, endpoint: string, input: string, expectedDecimals?: number): Promise<ReviewedAsset> {
  const profile = readProfiles[selection.id];
  if (!('evm' in profile) || selection.account !== 0) throw new Error('WALLET_ASSET_ENGINE');
  const contract = normalizeEvmAddress(input.trim());
  await checkEvmNetwork(rpc, profile.evm, profile.anchor);
  const block = await rpc('eth_getBlockByNumber', ['finalized', false]) as {hash?: unknown; number?: unknown} | null;
  if (!block || typeof block.hash !== 'string' || !hash.test(block.hash) ||
      typeof block.number !== 'string' || !/^0x(0|[1-9a-f][0-9a-f]{0,15})$/.test(block.number))
    throw new Error('WALLET_ASSET_BLOCK');
  const result = await rpc('eth_call', [{to:contract,data:'0x313ce567'}, {blockHash:block.hash,requireCanonical:true}]);
  if (typeof result !== 'string' || !hash.test(result) || BigInt(result) > 36n ||
      (expectedDecimals !== undefined && expectedDecimals !== Number(BigInt(result)))) throw new Error('WALLET_ASSET_DECIMALS');
  return {...scopeOf(owner, selection), endpoint, contract, decimals:Number(BigInt(result)), block:block.hash,
    height:BigInt(block.number).toString()};
}

export function registerReviewedAsset(value: ReadProfileState, owner: Owner, asset: ReviewedAsset): ReadProfileState {
  const {endpoint, ...record} = asset;
  if (!equalScope(record, scopeOf(owner, value.selection)) || endpoint !== value.endpoints[record.id] ||
      value.disabled?.includes(record.id)) throw new Error('WALLET_ASSET_SCOPE');
  const existing = assetsForAccount(value.assets, owner, value.selection).find(item => item.contract === record.contract);
  if (existing && existing.decimals !== record.decimals) throw new Error('WALLET_ASSET_DECIMALS');
  const assets = (value.assets ?? []).filter(item => !(equalScope(item, record) && item.contract === record.contract));
  if (assets.length >= 12) throw new Error('WALLET_ASSET_CAPACITY');
  return {...value, assets:[...assets, {...record, favorite:existing?.favorite ?? false, hidden:existing?.hidden ?? false}],
    selection:existing?.hidden ? value.selection : {...value.selection,asset:record.contract}};
}

export function validCatalogScope(item: unknown): item is Scope {
  const value = item as Scope;
  return !!value && typeof value.board === 'string' && /^[a-f0-9]{64}$/.test(value.board) &&
    Number.isSafeInteger(value.generation) && value.generation > 0 && Object.hasOwn(readProfiles,value.id) &&
    Number.isInteger(value.account) && value.account >= 0 && value.account <= 99 &&
    (!('evm' in readProfiles[value.id]) || value.account === 0);
}

export function validAccountPreference(value: unknown): value is AccountPreference {
  if (!validCatalogScope(value)) return false;
  const item = value as AccountPreference;
  if (typeof item.hidden!=='boolean') return false;
  if (typeof item.name==='string')
    return validName(item.name) && item.name===item.name.trim() && Object.keys(item).length===6;
  return item.hidden && !Object.hasOwn(item,'name') && Object.keys(item).length===5;
}

export function validAssetPreference(value: unknown): value is AssetPreference {
  if (!validCatalogScope(value)) return false;
  const item = value as AssetPreference;
  return 'evm' in readProfiles[item.id] && Object.keys(item).length === 10 &&
    typeof item.contract === 'string' && /^0x[a-f0-9]{40}$/.test(item.contract) && !/^0x0{40}$/.test(item.contract) &&
    Number.isInteger(item.decimals) && item.decimals >= 0 && item.decimals <= 36 &&
    typeof item.block === 'string' && hash.test(item.block) &&
    typeof item.height === 'string' && /^[0-9]{1,20}$/.test(item.height) &&
    typeof item.favorite === 'boolean' && typeof item.hidden === 'boolean';
}
