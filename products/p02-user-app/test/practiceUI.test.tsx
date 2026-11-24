import React from 'react';
import ReactTestRenderer from 'react-test-renderer';
import {AppState, type AppStateStatus} from 'react-native';
import {WalletHome} from '../src/wallet/WalletHome';

let mockSaved = '';
const mockSign = jest.fn();
const mockRpc = jest.fn();
const mockProtection = jest.fn(async (_enabled:boolean) => {});
jest.mock('../src/specs/NativeOwnerWallet.ts', () => ({__esModule:true, default:{
  setHomeProtection: (enabled:boolean) => mockProtection(enabled),
  loadProfile: async () => mockSaved, saveProfile: async (value:string) => {mockSaved = value;},
  compareAndSaveProfile: async (expected:string, value:string) => {if (expected !== mockSaved) return false; mockSaved = value; return true;},
  signChallenge: (...args:unknown[]) => mockSign(...args), evmRpc: (...args:unknown[]) => mockRpc(...args),
}}));
jest.mock('../src/specs/NativeRenterBle.ts', () => ({__esModule:true, default:{loadDevices: async () => ''}}));
const mockRoute = jest.fn();
const props = {rental:jest.fn(), orders:jest.fn(), permission:async()=>true, honorExistingRental:false, onPracticeChange:mockRoute};
async function press(tree:ReactTestRenderer.ReactTestRenderer, label:string) {
  // Home navigation moved to the bottom tab bar, whose entries carry the tab role.
  const button = tree.root.find(node => ['button', 'tab'].includes(node.props.accessibilityRole) && node.props.accessibilityLabel === label);
  await ReactTestRenderer.act(async () => {button.props.onPress(); await Promise.resolve();});
}
test('wrong sample rejects, survives remount, and never signs or sends', async () => {
  mockSaved = ''; let tree:ReactTestRenderer.ReactTestRenderer;
  await ReactTestRenderer.act(async () => {tree = ReactTestRenderer.create(<WalletHome {...props}/>);});
  await press(tree!, '결제 연습');
  expect(mockRoute).toHaveBeenLastCalledWith(true);
  await press(tree!, '틀린 금액 찾아보기');
  await press(tree!, '연습 승인');
  expect(JSON.stringify(tree!.toJSON())).toContain('금액이 다릅니다');
  await press(tree!, '나중에 이어하기');
  expect(mockRoute).toHaveBeenLastCalledWith(false);
  await ReactTestRenderer.act(async () => {tree!.unmount();});
  await ReactTestRenderer.act(async () => {tree = ReactTestRenderer.create(<WalletHome {...props}/>);});
  await press(tree!, '연습 이어하기');
  await press(tree!, '연습 거절');
  expect(JSON.stringify(tree!.toJSON())).toContain('실제 지급 0건');
  expect(mockSign).not.toHaveBeenCalled(); expect(mockRpc).not.toHaveBeenCalled();
});
test('English matching sample can be approved without a transaction', async () => {
  mockSaved = ''; let tree:ReactTestRenderer.ReactTestRenderer;
  await ReactTestRenderer.act(async () => {tree = ReactTestRenderer.create(<WalletHome {...props}/>);});
  await press(tree!, 'English');
  await press(tree!, 'Payment practice');
  await press(tree!, 'Compare matching amounts');
  await press(tree!, 'Continue later');
  await ReactTestRenderer.act(async () => {tree!.unmount();});
  await ReactTestRenderer.act(async () => {tree = ReactTestRenderer.create(<WalletHome {...props}/>);});
  await press(tree!, 'Resume practice');
  await press(tree!, 'Approve practice');
  expect(JSON.stringify(tree!.toJSON())).toContain('Practice approved');
  expect(mockSign).not.toHaveBeenCalled(); expect(mockRpc).not.toHaveBeenCalled();
});
test('corrupt wrong-amount completion cannot appear as approved', async () => {
  mockSaved = JSON.stringify({version:2, exercised:true, practice:{scenario:'wrongAmount',state:'done'}});
  let tree:ReactTestRenderer.ReactTestRenderer;
  await ReactTestRenderer.act(async () => {tree = ReactTestRenderer.create(<WalletHome {...props}/>);});
  const text = JSON.stringify(tree!.toJSON());
  expect(text).not.toContain('연습을 승인했습니다');
  expect(text).toContain('보호된 지갑 기록을 열 수 없습니다');
});

test('personal home persists balance visibility and routes to account selection', async () => {
  AppState.currentState = 'active';
  const {defaultReadState, observationScope} = require('../src/wallet/readProfiles');
  const board = 'ab'.repeat(32), address = '0x'+'12'.repeat(20);
  const walletRead = {...defaultReadState, observations:[{scope:observationScope(board,3,defaultReadState.selection),
    address,amount:'1250000',unit:'TEST',decimals:6,point:'block 45',time:1700000000000}]};
  mockSaved = JSON.stringify({version:2,exercised:true,mode:'personal',verified:{boardFingerprint:board,address,generation:3,network:'8283'},walletRead});
  let tree:ReactTestRenderer.ReactTestRenderer;
  await ReactTestRenderer.act(async () => {tree = ReactTestRenderer.create(<WalletHome {...props}/>);});
  expect(JSON.stringify(tree!.toJSON())).toContain('1.25');
  await press(tree!, '잔액 숨기기');
  expect(JSON.stringify(tree!.toJSON())).not.toContain('1.25');
  expect(JSON.parse(mockSaved).walletRead.selection.hidden).toBe(true);
  await ReactTestRenderer.act(async () => {tree!.unmount();});
  await ReactTestRenderer.act(async () => {tree = ReactTestRenderer.create(<WalletHome {...props}/>);});
  expect(JSON.stringify(tree!.toJSON())).not.toContain('1.25');
  await press(tree!, '계정 및 자산 선택');
  expect(JSON.stringify(tree!.toJSON())).toContain('6개 체인');
  await ReactTestRenderer.act(async () => {tree!.unmount();});
});

test('home remains redacted if native protection cannot start', async () => {
  const board = 'ab'.repeat(32), address = '0x'+'12'.repeat(20);
  mockSaved = JSON.stringify({version:2,exercised:true,mode:'personal',verified:{boardFingerprint:board,address,generation:3,network:'8283'}});
  mockProtection.mockRejectedValueOnce(new Error('no window'));
  let tree:ReactTestRenderer.ReactTestRenderer;
  await ReactTestRenderer.act(async () => {tree = ReactTestRenderer.create(<WalletHome {...props}/>);});
  expect(JSON.stringify(tree!.toJSON())).toContain('지갑 화면 보호를 시작하지 못했습니다');
  expect(JSON.stringify(tree!.toJSON())).not.toContain(address);
  await ReactTestRenderer.act(async () => {tree!.unmount();});
  expect(mockProtection).toHaveBeenLastCalledWith(false);
});

test('background removes the observed balance from the home accessibility tree', async () => {
  let changed: ((state: AppStateStatus) => void) | undefined;
  const listener = jest.spyOn(AppState, 'addEventListener').mockImplementation((_type, callback) => {
    changed = callback as (state:AppStateStatus)=>void; return {remove:()=>{}};
  });
  AppState.currentState = 'active';
  const {defaultReadState, observationScope} = require('../src/wallet/readProfiles');
  const board = 'ab'.repeat(32), address = '0x'+'12'.repeat(20);
  mockSaved = JSON.stringify({version:2,exercised:true,mode:'personal',verified:{boardFingerprint:board,address,generation:3,network:'8283'},
    walletRead:{...defaultReadState, observations:[{scope:observationScope(board,3,defaultReadState.selection),address,
      amount:'1250000',unit:'TEST',decimals:6,point:'block 45',time:1700000000000}]}});
  let tree:ReactTestRenderer.ReactTestRenderer;
  await ReactTestRenderer.act(async () => {tree = ReactTestRenderer.create(<WalletHome {...props}/>);});
  expect(JSON.stringify(tree!.toJSON())).toContain('1.25');
  await ReactTestRenderer.act(async () => {changed?.('background');});
  expect(JSON.stringify(tree!.toJSON())).not.toContain('1.25');
  await ReactTestRenderer.act(async () => {changed?.('active');});
  expect(JSON.stringify(tree!.toJSON())).toContain('1.25');
  await ReactTestRenderer.act(async () => {tree!.unmount();});
  listener.mockRestore();
});

test('personal settings is reachable and language preference survives return to home', async () => {
  const listener = jest.spyOn(AppState, 'addEventListener').mockReturnValue({remove:()=>{}});
  AppState.currentState = 'active';
  mockSaved = JSON.stringify({version:2,exercised:true,mode:'personal',verified:{boardFingerprint:'ab'.repeat(32),address:'0x'+'12'.repeat(20),generation:3,network:'8283'}});
  let tree:ReactTestRenderer.ReactTestRenderer;
  await ReactTestRenderer.act(async () => {tree = ReactTestRenderer.create(<WalletHome {...props}/>);});
  await press(tree!, '설정');
  expect(mockProtection).toHaveBeenLastCalledWith(true);
  expect(JSON.stringify(tree!.toJSON())).toContain('접근성');
  expect(JSON.stringify(tree!.toJSON())).toContain('도움말·진단');
  await press(tree!, '알림과 접근성');
  await press(tree!, 'English 선택');
  expect(JSON.parse(mockSaved).language).toBe('en');
  await press(tree!, 'Return to Settings');
  await press(tree!, 'Back to wallet');
  expect(mockProtection).toHaveBeenLastCalledWith(true);
  expect(JSON.stringify(tree!.toJSON())).toContain('My assets');
  await ReactTestRenderer.act(async () => {tree!.unmount();});
  listener.mockRestore();
});

test('W27 decline opens W02 and the same local outcome survives restart without live wallet operations', async()=>{
  AppState.currentState='active';
  const stateListener=jest.spyOn(AppState,'addEventListener').mockReturnValue({remove:()=>{}});
  mockSaved=JSON.stringify({version:2,exercised:false,mode:'personal',
    verified:{boardFingerprint:'ab'.repeat(32),address:'0x'+'12'.repeat(20),generation:3,network:'8283'}});
  const verified=JSON.parse(mockSaved).verified;
  const permission=jest.fn(async()=>true);
  let tree:ReactTestRenderer.ReactTestRenderer;
  const mount=()=>ReactTestRenderer.act(async()=>{tree=ReactTestRenderer.create(<WalletHome {...props} permission={permission}/>);});
  try {
    await mount();
    await press(tree!,'설정');
    await press(tree!,'알림과 접근성');
    await press(tree!,'화면 읽기 순서');
    await press(tree!,'거절 연습');
    expect(JSON.parse(mockSaved).practice).toEqual({scenario:'matching',state:'reject'});
    expect(JSON.parse(mockSaved).verified).toEqual(verified);
    expect(JSON.stringify(tree!.toJSON())).toContain('실제 지급 0건');
    await ReactTestRenderer.act(async()=>tree!.unmount());
    await mount();
    await press(tree!,'결제 연습');
    expect(JSON.stringify(tree!.toJSON())).toContain('연습 주문을 거절했습니다');
    await press(tree!,'같은 연습 다시 보기');
    expect(JSON.stringify(tree!.toJSON())).toContain('2 연습 코인');
    await press(tree!,'연습 거절');
    await press(tree!,'실물 보드 등록');
    expect(JSON.stringify(tree!.toJSON())).toContain('개인 지갑');
    expect(permission).not.toHaveBeenCalled();
    expect(mockSign).not.toHaveBeenCalled();
    expect(mockRpc).not.toHaveBeenCalled();
  } finally {
    if(tree!)await ReactTestRenderer.act(async()=>tree.unmount());
    stateListener.mockRestore();
  }
});
