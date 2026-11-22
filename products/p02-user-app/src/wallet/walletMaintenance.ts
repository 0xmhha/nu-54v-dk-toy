import type {OwnerStatus} from './ownerWire.ts';
import type {WalletStorage} from './profile.ts';

export type MaintenanceScope = {
  mode: 'personal' | 'rental';
  board?: string;
  generation?: number;
};

export function maintenanceScope(storage:WalletStorage|undefined):MaintenanceScope {
  if(storage?.mode!=='personal')return {mode:'rental'};
  const board=storage.enrollment?.device;
  return {mode:'personal',board:storage.verified&&storage.verified.boardFingerprint!==board?undefined:board,
    generation:storage.verified?.generation};
}

export function sameMaintenanceScope(a:MaintenanceScope,b:MaintenanceScope) {
  return a.mode===b.mode&&a.board===b.board&&a.generation===b.generation;
}

export function boundMaintenanceBoard(scope:MaintenanceScope,board:OwnerStatus) {
  return scope.mode==='personal'&&scope.board===board.device&&board.generation>0n&&
    (scope.generation===undefined||BigInt(scope.generation)===board.generation);
}

export function maintenanceDecision(scope:MaintenanceScope,board:OwnerStatus|undefined,signing:boolean,unsettled:boolean) {
  if(scope.mode==='rental')return {update:'rental-only',erase:'rental-only'} as const;
  const scoped=board&&boundMaintenanceBoard(scope,board);
  const active=scoped&&board.state==='active'&&board.generation>0n;
  const update=signing||scoped&&board.state==='pending'?'approval-busy':active?'ready-to-read':'verify-board';
  const erase=signing||unsettled?'unresolved':!active?'verify-board':
    !board.backedUp?'backup-unverified':'physical-not-verified';
  return {update,erase} as const;
}
