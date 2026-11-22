import React from 'react';
import Renderer from 'react-test-renderer';
import {WalletHomeSummary} from '../src/wallet/WalletHomeSummary';
import {defaultReadState, observationScope} from '../src/wallet/readProfiles';

const owner = {boardFingerprint:'ab'.repeat(32), generation:3, address:'0x'+'12'.repeat(20), network:'8283'};
const selection = {...defaultReadState.selection};
const observation = {scope:observationScope(owner.boardFingerprint,3,selection), address:owner.address, amount:'1250000', unit:'TEST', decimals:6, point:'block 45', time:1700000000000};
const state = {...defaultReadState,selection,observations:[observation]};
const props = {owner, state, language:'ko' as const, active:true, onHide:jest.fn(), onAccounts:jest.fn(), onReceive:jest.fn()};
function render(p: Partial<React.ComponentProps<typeof WalletHomeSummary>> = {}) { let tree: Renderer.ReactTestRenderer; Renderer.act(()=>{tree=Renderer.create(<WalletHomeSummary {...props} {...p}/>);}); return tree!; }
const text=(tree:Renderer.ReactTestRenderer)=>JSON.stringify(tree.toJSON());
test('only matching owner and selection is shown, with historical point and time',()=>{
  const tree=render(); expect(text(tree)).toContain('1.25'); expect(text(tree)).toContain('block 45'); expect(text(tree)).toContain('마지막 조회');
  expect(text(render({owner:{...owner,generation:4}}))).not.toContain('1.25');
  expect(text(render({state:{...state,selection:{...selection,id:'eth'}}}))).not.toContain('1.25');
  expect(text(render({state:{...state,observations:[]}}))).toContain('조회 기록 없음');
});
test('hidden and inactive states contain no balance in rendered or accessible tree',()=>{
  expect(text(render({state:{...state,selection:{...selection,hidden:true}}}))).not.toContain('1.25');
  expect(text(render({active:false}))).not.toContain('1.25');
});
test('disabled network, hidden account and unreviewed token never reveal an old observation',()=>{
  expect(text(render({state:{...state,disabled:['stable']}}))).not.toContain('1.25');
  expect(text(render({state:{...state,accounts:[{board:owner.boardFingerprint,generation:3,id:'stable',account:0,name:'Private',hidden:true}]}}))).not.toContain('1.25');
  const tokenSelection = {...selection,asset:'0x'+'34'.repeat(20)};
  const tokenObservation = {...observation,scope:observationScope(owner.boardFingerprint,3,tokenSelection)};
  expect(text(render({state:{...state,selection:tokenSelection,observations:[tokenObservation]}}))).not.toContain('1.25');
});
test('OS offline and restored link leave the same cached observation until a fresh bound proof',()=>{
  const offline = render({connectivity:{link:'offline',revision:2}});
  expect(text(offline)).toContain('오프라인');
  expect(text(offline)).toContain('1.25');
  expect(text(offline)).toContain('block 45');
  const online = {link:'online' as const,revision:3};
  const proof = {scope:observation.scope,time:observation.time,revision:1,endpoint:'https://read.example/'};
  const routed = {...state,endpoints:{stable:proof.endpoint}};
  const restored = render({state:routed,connectivity:online,proof});
  expect(text(restored)).toContain('연결됨 · 원장 재확인 필요');
  expect(text(restored)).not.toContain('원장 재확인 완료');
  expect(text(render({state:routed,connectivity:online,proof:{...proof,revision:3}}))).toContain('원장 재확인 완료');
});

test('W25 education rate on W01 is available only for the exact scoped TestUSDC and retains its cached time offline',()=>{
  const asset='0x500ef69da230e42bff34487b578ea0cbefbda2ba';
  const selected={...state.selection,asset};
  const wallet={...state,selection:selected,assets:[{board:owner.boardFingerprint,generation:owner.generation,id:'stable' as const,
    account:0,contract:asset,decimals:6,block:'0x'+'11'.repeat(32),height:'70',hidden:false,favorite:false}],observations:[{...observation,
    scope:observationScope(owner.boardFingerprint,owner.generation,selected),amount:'42000000',decimals:6,unit:'ERC-20',point:'Finalized block 70'}]};
  const offline=render({state:wallet,connectivity:{link:'offline',revision:2}});
  expect(text(offline)).toContain('마지막 환산 시세');
  Renderer.act(()=>offline.root.findAll(node=>node.props.accessibilityLabel==='마지막 환산 시세'&&!!node.props.onPress)[0].props.onPress());
  expect(text(offline)).toContain('54,600');
  expect(text(offline)).toContain('2023-11-14T22:13:20.000Z');
  expect(text(offline)).toContain('2026-11-20T05:20:00.000Z');
  expect(text(render({owner:{...owner,generation:owner.generation+1},state:wallet}))).not.toContain('환산 시세');
  expect(text(render({state:{...wallet,selection:{...selected,hidden:true}}}))).not.toContain('환산 시세');
  expect(text(render({state:{...wallet,selection:{...selected,asset:'0x'+'ab'.repeat(20)}}}))).not.toContain('환산 시세');
});
