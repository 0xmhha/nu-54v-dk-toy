import type {ReadProfileState, ReadSelection} from './readProfiles.ts';
import {decodePublicBase58} from './chainAccounts.ts';
import {readSolanaMint, readXrplIssuer, xrplIssue} from './nonEvmAssets.ts';
import type {ChainRpc} from './chainTransfer.ts';

type Owner = {boardFingerprint:string;generation:number};
type Scope = {board:string;generation:number;account:number};
type Asset = {id:'sol';mint:string;decimals:number;slot:number} |
  {id:'xrp';currency:string;issuer:string;ledgerHash:string;ledgerIndex:number};
export type NonEvmAssetPreference = Scope & Asset & {hidden:boolean;favorite:boolean};
export type NonEvmAssetReview = Scope & Asset & {endpoint:string};
export const nonEvmAssetKey = (asset: Asset) => asset.id === 'sol' ? asset.mint : asset.currency + '@' + asset.issuer;

export function isNonEvmAssetId(id: string, key: string) {
  try {
    if (id === 'sol') {decodePublicBase58(key,32); return true;}
    if (id === 'xrp') {
      const [currency,issuer,...extra] = key.split('@');
      return !extra.length && !!xrplIssue(currency,issuer);
    }
    return false;
  } catch { return false; }
}

export function validNonEvmAssetPreference(input: unknown): input is NonEvmAssetPreference {
  const asset = input as NonEvmAssetPreference;
  if (!asset || !/^[a-f0-9]{64}$/.test(asset.board) || !Number.isSafeInteger(asset.generation) || asset.generation < 1 ||
      !Number.isInteger(asset.account) || asset.account < 0 || asset.account > 99 ||
      typeof asset.favorite !== 'boolean' || typeof asset.hidden !== 'boolean') return false;
  if (asset.id === 'sol') return Object.keys(asset).length === 9 && isNonEvmAssetId('sol',asset.mint) &&
    Number.isInteger(asset.decimals) && asset.decimals >= 0 && asset.decimals <= 36 &&
    Number.isSafeInteger(asset.slot) && asset.slot > 0;
  return asset.id === 'xrp' && Object.keys(asset).length === 10 && isNonEvmAssetId('xrp',nonEvmAssetKey(asset)) &&
    typeof asset.ledgerHash === 'string' && /^[A-Fa-f0-9]{64}$/.test(asset.ledgerHash) &&
    Number.isSafeInteger(asset.ledgerIndex) && asset.ledgerIndex > 0;
}

export function scopedNonEvmAssets(all: NonEvmAssetPreference[] | undefined, owner: Owner, selection: ReadSelection) {
  return (all ?? []).filter(item => item.board === owner.boardFingerprint && item.generation === owner.generation &&
    item.account === selection.account && item.id === selection.id);
}

export async function reviewNonEvmAsset(rpc:ChainRpc, owner:Owner, selection:ReadSelection, endpoint:string, anchor:string,
  token:string, extra:string):Promise<NonEvmAssetReview> {
  const scope = {board:owner.boardFingerprint,generation:owner.generation,account:selection.account,endpoint};
  if (selection.id === 'sol') {
    if (extra && !/^(0|[1-9][0-9]?)$/.test(extra)) throw new Error('SOL_TOKEN_MINT');
    return {...scope,id:'sol',...await readSolanaMint(rpc,anchor,token,extra ? Number(extra) : undefined)};
  }
  if (selection.id === 'xrp') return {...scope,id:'xrp',...await readXrplIssuer(rpc,anchor,token,extra)};
  throw new Error('WALLET_ASSET_ENGINE');
}

export function registerNonEvmAsset(state:ReadProfileState, owner:Owner, review:NonEvmAssetReview):ReadProfileState {
  const {selection} = state;
  if (review.id !== selection.id || review.board !== owner.boardFingerprint || review.generation !== owner.generation ||
      review.account !== selection.account || state.endpoints[review.id] !== review.endpoint || state.disabled?.includes(review.id))
    throw new Error('WALLET_ASSET_SCOPE');
  const key = nonEvmAssetKey(review);
  const prior = scopedNonEvmAssets(state.nonEvmAssets,owner,selection).find(item => nonEvmAssetKey(item) === key);
  if (prior?.id === 'sol' && review.id === 'sol' && (prior.decimals !== review.decimals || review.slot < prior.slot))
    throw new Error('SOL_TOKEN_MINT');
  const remaining = (state.nonEvmAssets ?? []).filter(item => item !== prior);
  if (remaining.length >= 12) throw new Error('WALLET_ASSET_CAPACITY');
  const scope={board:review.board,generation:review.generation,account:review.account};
  const preferences={hidden:prior?.hidden ?? false,favorite:prior?.favorite ?? false};
  const record:NonEvmAssetPreference=review.id==='sol' ?
    {...scope,...preferences,id:'sol',mint:review.mint,decimals:review.decimals,slot:review.slot} :
    {...scope,...preferences,id:'xrp',currency:review.currency,issuer:review.issuer,ledgerHash:review.ledgerHash,ledgerIndex:review.ledgerIndex};
  if (!validNonEvmAssetPreference(record)) throw new Error('WALLET_ASSET_SCHEMA');
  return {...state,nonEvmAssets:[...remaining,record],selection:record.hidden ? selection : {...selection,asset:key}};
}
