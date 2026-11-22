import {formatReadAmount, observationScope, type ReadProfileState, type ReadSelection} from './readProfiles.ts';
import {accountPreference, assetsForAccount} from './walletCatalog.ts';

export const stableTestUsdc = '0x500ef69da230e42bff34487b578ea0cbefbda2ba'; // StableNet 8283 v1 deployment
const trainingKrw = 1300n;
const testUsdcScale = 1_000_000n;

export function assetSearchMatches(query: string, ...values: string[]) {
  const needle = query.trim().toLowerCase();
  return !needle || values.some(value => value.toLowerCase().includes(needle));
}

export function educationRate(state: ReadProfileState, owner: {boardFingerprint:string;generation:number;address:string},
  selection: ReadSelection = state.selection) {
  if (selection.id !== 'stable' || selection.account !== 0 || selection.hidden || selection.asset !== stableTestUsdc ||
      state.disabled?.includes('stable') || accountPreference(state.accounts,owner,selection)?.hidden) return undefined;
  const registered = assetsForAccount(state.assets,owner,selection).find(asset => asset.contract === stableTestUsdc);
  if (!registered || registered.hidden || registered.decimals !== 6) return undefined;
  const scope = observationScope(owner.boardFingerprint,owner.generation,selection);
  const observed = state.observations.find(row => row.scope === scope && row.address === owner.address);
  if (!observed || observed.decimals !== 6 || !observed.point.startsWith('Finalized block ') || !observed.amount ||
      !/^(?:0|[1-9][0-9]{0,77})$/.test(observed.amount)) return undefined;
  const won = BigInt(observed.amount) * trainingKrw + testUsdcScale / 2n;
  return {scope,time:observed.time, amount:formatReadAmount(observed.amount,6),
    krw:(won / testUsdcScale).toString(), basis:'2026-11-20T05:20:00.000Z'};
}
