import {maintenanceDecision, sameMaintenanceScope, type MaintenanceScope} from '../src/wallet/walletMaintenance.ts';
import type {OwnerStatus} from '../src/wallet/ownerWire.ts';

const scope:MaintenanceScope={mode:'personal',board:'a'.repeat(64),generation:4};
const board:OwnerStatus={device:scope.board!,state:'active',generation:4n,root:2,profile:1,owner:'b'.repeat(64),backedUp:true};

test('refuses update evidence during a persisted signing reservation and refuses reset while payment is unresolved',()=>{
  expect(maintenanceDecision(scope,board,true,false)).toEqual({update:'approval-busy',erase:'unresolved'});
  expect(maintenanceDecision(scope,board,false,true)).toEqual({update:'ready-to-read',erase:'unresolved'});
  expect(maintenanceDecision(scope,board,false,false)).toEqual({update:'ready-to-read',erase:'physical-not-verified'});
});

test('missing/changed authenticated state or rental never enables private maintenance',()=>{
  expect(maintenanceDecision(scope,undefined,false,false)).toEqual({update:'verify-board',erase:'verify-board'});
  expect(maintenanceDecision(scope,{...board,generation:3n},false,false)).toEqual({update:'verify-board',erase:'verify-board'});
  expect(maintenanceDecision(scope,{...board,state:'pending'},false,false)).toEqual({update:'approval-busy',erase:'verify-board'});
  expect(maintenanceDecision({mode:'rental'},board,false,false)).toEqual({update:'rental-only',erase:'rental-only'});
  expect(sameMaintenanceScope(scope,{...scope,generation:5})).toBe(false);
  expect(sameMaintenanceScope(scope,{mode:'rental'})).toBe(false);
  expect(sameMaintenanceScope(scope,{...scope})).toBe(true);
});
