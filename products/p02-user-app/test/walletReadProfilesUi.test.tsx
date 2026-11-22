import React from 'react';
import {toBase64} from "@nu54/protocol";
import {classicTokenProgram} from "../src/wallet/nonEvmAssets";

import ReactTestRenderer from 'react-test-renderer';
import {Image, Share, TextInput} from 'react-native';
import {WalletReadProfilesScreen} from '../src/wallet/WalletReadProfilesScreen';
import {defaultReadState, observationScope, readProfiles, type ReadProfileId, type ReadProfileState} from '../src/wallet/readProfiles';
import type {OwnerSession} from '../src/wallet/ownerSession';
import type {WalletConnectivity} from '../src/wallet/walletConnectivity';
import pendingVector from './fixtures/w25-stable-pending.json';



test('W25 validates a mixed-case EVM asset before normalizing and saving it', async () => {
  const verified = {network: '8283' as const, boardFingerprint: 'af'.repeat(32), generation: 1, address: '0x' + 'a3'.repeat(20)};
  const save = jest.fn(async () => {});
  const state = {...defaultReadState,endpoints:{stable:'https://read.example/'}};
  const rpc = jest.fn(async (_endpoint: string, raw: string) => {
    const {method,params} = JSON.parse(raw);
    const result = method === 'eth_chainId' ? '0x205b' :
      method === 'eth_call' ? '0x' + '0'.repeat(63) + '6' :
      {hash:params[0] === '0x0' ? readProfiles.stable.anchor : '0x' + 'ab'.repeat(32),number:'0x1'};
    return JSON.stringify({id:1,result});
  });
  let active: Promise<void> | undefined;
  let tree: ReactTestRenderer.ReactTestRenderer | undefined;
  const token = '0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed';
  const enter = async (text: string) => ReactTestRenderer.act(async () => {
    tree!.root.findAllByType(TextInput).find(node => node.props.accessibilityLabel === '읽을 토큰 주소')!.props.onChangeText(text);
  });
  const choose = (label = '토큰 정보 검토') => ReactTestRenderer.act(async () => {
    tree!.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onPress === 'function')[0].props.onPress();
    await active;
  });
  try {
    await ReactTestRenderer.act(async () => {
      tree = ReactTestRenderer.create(<WalletReadProfilesScreen verified={verified} value={state} save={save}
        found={[]} scan={() => {}} back={() => {}} busy={false} rpc={rpc} session={async () => {throw Error('no board');}}
        work={run => {active = run().catch(() => {});}} t={text => text} />);
    });
    await enter(token.replace('aA', 'AA'));
    await choose();
    expect(save).not.toHaveBeenCalled();
    expect(rpc).not.toHaveBeenCalled();
    await enter(token);
    await choose();
    expect(save).not.toHaveBeenCalled();
    expect(rpc).toHaveBeenCalledTimes(4);
    await choose('검증된 토큰 추가');
    expect(save).toHaveBeenCalledWith(expect.objectContaining({
      selection:{...defaultReadState.selection,asset:token.toLowerCase()},
      assets:[expect.objectContaining({contract:token.toLowerCase(),decimals:6})]
    }));
  } finally {
    await ReactTestRenderer.act(async () => tree?.unmount());
  }
});

test('W07 verifies the receiving account without disclosing it to a public RPC or sharing automatically', async () => {
  const verified = {network:'8283' as const, boardFingerprint:'bb'.repeat(32), generation:1, address:'0x'+'a3'.repeat(20)};
  const rpc = jest.fn(async () => {throw Error('no public disclosure');});
  const share = jest.spyOn(Share, 'share').mockResolvedValue({action: Share.dismissedAction, activityType: undefined});
  const save = jest.fn(async () => {});
  let active: Promise<void> | undefined;
  let tree: ReactTestRenderer.ReactTestRenderer | undefined;
  const press = (label: string) => ReactTestRenderer.act(async () => {
    const node = tree!.root.findAll(item => item.props.accessibilityLabel === label && typeof item.props.onPress === 'function')[0];
    expect(node).toBeDefined();
    expect(node.props.disabled).not.toBe(true);
    node.props.onPress();
    await active;
  });
  try {
    await ReactTestRenderer.act(async () => {
      tree = ReactTestRenderer.create(<WalletReadProfilesScreen verified={verified} value={defaultReadState} save={save}
        found={[{address:'board', name:'Board'}]} scan={() => {}} back={() => {}} busy={false} rpc={rpc}
        session={async (_handle, action) => action({verifiedEvmAddress:async () => verified.address} as unknown as OwnerSession)}
        work={run => {active = run();}} t={text => text} />);
    });
    await press('보드와 수신 주소 확인 · Board');
    expect(rpc).not.toHaveBeenCalled();
    expect(save).not.toHaveBeenCalled();
    expect(share).not.toHaveBeenCalled();
    expect(tree!.root.findByType(Image).props.source.uri).toMatch(/^data:image\/gif;base64,/);
    await press('받기 정보 공유');
    expect(share).toHaveBeenCalledWith({message:expect.stringContaining('ethereum:' + verified.address + '@8283')});
  } finally {
    share.mockRestore();
    await ReactTestRenderer.act(async () => tree?.unmount());
  }
});

test('W26 keeps the previous profile until explicit approval and exposes no unsupported signing action', async () => {
  const verified = {network:'8283' as const, boardFingerprint:'bc'.repeat(32), generation:1, address:'0x'+'a3'.repeat(20)};
  const value = {...defaultReadState, endpoints:{stable:'https://old.example/',sol:'https://new.example/'}};
  const save = jest.fn(async () => {});
  const rpc = jest.fn(async () => {throw Error('unexpected');});
  const session = jest.fn(async () => {throw Error('unexpected');});
  let active: Promise<void> | undefined;
  let tree: ReactTestRenderer.ReactTestRenderer | undefined;
  const press = (label: string) => ReactTestRenderer.act(async () => {
    tree!.root.findAll(item => item.props.accessibilityLabel === label && typeof item.props.onPress === 'function')[0].props.onPress();
    await active;
  });
  try {
    await ReactTestRenderer.act(async () => {
      tree = ReactTestRenderer.create(<WalletReadProfilesScreen verified={verified} value={value} save={save}
        found={[]} scan={() => {}} back={() => {}} busy={false} rpc={rpc} session={session}
        work={run => {active = run();}} t={text => text} />);
    });
    await press('Solana Devnet');
    expect(save).not.toHaveBeenCalled();
    await press('조회 네트워크 전환 취소');
    await press('Solana Devnet');
    await press('조회 네트워크 전환 동의');
    expect(save).toHaveBeenCalledWith({...value,selection:{id:'sol',account:0,hidden:false}});
    await press('체인별 기능표');
    await press('지원 여부 확인 · Staking · DelegateStake');
    expect(JSON.stringify(tree!.toJSON())).toContain('서명 0건 · 제출 0건');
    expect(rpc).not.toHaveBeenCalled();
    expect(session).not.toHaveBeenCalled();
  } finally {
    await ReactTestRenderer.act(async () => tree?.unmount());
  }
});

test('W25 keeps the selected token when favorites change and recovers it after hiding', async () => {
  const verified = {network:'8283' as const, boardFingerprint:'bc'.repeat(32), generation:1, address:'0x'+'a3'.repeat(20)};
  const contract = '0x'+'cf'.repeat(20);
  const value = {...defaultReadState,selection:{...defaultReadState.selection,asset:contract},
    assets:[{board:verified.boardFingerprint,generation:1,id:'stable' as const,account:0,contract,decimals:6,
      block:'0x'+'42'.repeat(32),height:'1234',favorite:false,hidden:false}]};
  const saves: (typeof value)[] = [];
  let active: Promise<void> | undefined;
  let tree: ReactTestRenderer.ReactTestRenderer | undefined;
  function ViewUnderTest() {
    const [state, setState] = React.useState<ReadProfileState>(value);
    return <WalletReadProfilesScreen verified={verified} value={state} save={async next => {
      saves.push(next as typeof value);
      setState(next);
    }} found={[]} scan={() => {}} back={() => {}} busy={false} rpc={async () => ''}
    session={async () => {throw Error('unexpected');}} work={run => {active=run();}} t={text=>text} />;
  }
  const press = (label: string) => ReactTestRenderer.act(async () => {
    tree!.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onPress === 'function')[0].props.onPress();
    await active;
  });
  try {
    await ReactTestRenderer.act(async () => {tree = ReactTestRenderer.create(<ViewUnderTest />);});
    await press('즐겨찾기에 추가 1');
    expect(saves.at(-1)).toMatchObject({selection:{asset:contract},assets:[{favorite:true,hidden:false}]});
    await press('목록에서 숨기기 1');
    expect(saves.at(-1)?.selection.asset).toBeUndefined();
    expect(saves.at(-1)?.assets[0]).toMatchObject({favorite:true,hidden:true,contract});
    await press('숨긴 자산');
    await press('목록에 다시 표시 1');
    expect(saves.at(-1)?.assets[0]).toMatchObject({favorite:true,hidden:false,contract});
  } finally {
    await ReactTestRenderer.act(async () => tree?.unmount());
  }
});

test('W25 retains cached amount and time on failure and ignores a read completed after link loss', async () => {
  const owner = {network:'8283' as const,boardFingerprint:'a1'.repeat(32),generation:1,address:'0x'+'a3'.repeat(20)};
  const selection = defaultReadState.selection;
  const cached = {scope:observationScope(owner.boardFingerprint,owner.generation,selection),address:owner.address,
    amount:'1250000000000000000',decimals:18,unit:'TEST',point:'Finalized block 45',time:1700000000000};
  const value = {...defaultReadState,observations:[cached],endpoints:{stable:'https://rpc.example/'}};
  const save = jest.fn(async () => {});
  const onReadProof = jest.fn();
  const board = {verifiedEvmAddress:async () => owner.address} as unknown as OwnerSession;
  let active:Promise<void> | undefined, tree:ReactTestRenderer.ReactTestRenderer | undefined;
  let connection: WalletConnectivity = {link:'online',revision:1};
  let release: ((value: string) => void) | undefined;
  let arrives: (() => void) | undefined;
  let fail = true;
  const rpc = jest.fn(async (_url: string, raw: string) => {
    if (fail) throw Error('transport interrupted');
    const {method,params} = JSON.parse(raw);
    if (method === 'eth_getBalance') {
      arrives?.();
      return await new Promise<string>(resolve => {release = resolve;});
    }
    const result = method === 'eth_chainId' ? '0x205b' :
      {hash:params[0] === '0x0' ? readProfiles.stable.anchor : '0x'+'ba'.repeat(32),number:'0x46'};
    return JSON.stringify({id:1,result});
  });
  const view = () => <WalletReadProfilesScreen verified={owner} value={value} save={save}
    connectivity={connection} onReadProof={onReadProof} found={[{name:'Board',address:'board'}]} scan={() => {}} back={() => {}}
    busy={false} rpc={rpc} session={async (_handle, action) => action(board)}
    work={run => {active=run().catch(() => {});}} t={text => text} />;
  const press = () => tree!.root.findAll(item => item.props.accessibilityLabel === '입금 잔액 다시 확인 · Board' &&
    typeof item.props.onPress === 'function')[0].props.onPress();
  try {
    await ReactTestRenderer.act(async () => {tree=ReactTestRenderer.create(view());});
    expect(rpc).not.toHaveBeenCalled();
    await ReactTestRenderer.act(async () => {press(); await active;});
    expect(save).not.toHaveBeenCalled();
    expect(JSON.stringify(tree!.toJSON())).toContain('1.25');
    expect(JSON.stringify(tree!.toJSON())).toContain('2023-11-14T22:13:20.000Z');
    fail=false;
    const started = new Promise<void>(resolve => {arrives=resolve;});
    await ReactTestRenderer.act(async () => {press(); await started;});
    connection={link:'offline',revision:2};
    await ReactTestRenderer.act(async () => {tree!.update(view());});
    await ReactTestRenderer.act(async () => {release?.(JSON.stringify({id:1,result:'0x10'})); await active;});
    expect(save).not.toHaveBeenCalled();
    expect(onReadProof.mock.calls.every(([proof]) => proof === undefined)).toBe(true);
    expect(JSON.stringify(tree!.toJSON())).toContain('오프라인');
    expect(JSON.stringify(tree!.toJSON())).toContain('1.25');
    connection={link:'online',revision:3};
    await ReactTestRenderer.act(async () => {tree!.update(view());});
    expect(save).not.toHaveBeenCalled();
    const nextRead = new Promise<void>(resolve => {arrives=resolve;});
    await ReactTestRenderer.act(async () => {press(); await nextRead;});
    await ReactTestRenderer.act(async () => {release?.(JSON.stringify({id:1,result:'0x10'})); await active;});
    expect(save).toHaveBeenCalledWith(expect.objectContaining({observations:[expect.objectContaining({
      scope:cached.scope,point:'Finalized block 70',amount:'16'
    })]}));
    expect(onReadProof).toHaveBeenLastCalledWith(expect.objectContaining({
      scope:cached.scope,endpoint:value.endpoints.stable,revision:3
    }));
  } finally {
    await ReactTestRenderer.act(async () => tree?.unmount());
  }
});

test('W25 review of a public Solana mint verifies scope without disclosing the receiving account', async () => {
  const owner = {network:'8283' as const,boardFingerprint:'bf'.repeat(32),generation:1,address:'0x'+'a3'.repeat(20)};
  const mint = 'So11111111111111111111111111111111111111112';
  const state = {...defaultReadState,selection:{id:'sol' as const,account:0,hidden:false},endpoints:{sol:'https://rpc.example/'}};
  const data = new Uint8Array(82); data[44]=9; data[45]=1;
  const rpc = jest.fn(async (_url:string, raw:string) => {
    const {method,params}=JSON.parse(raw);
    if (method === 'getGenesisHash') return JSON.stringify({id:1,result:readProfiles.sol.anchor});
    expect(method).toBe('getAccountInfo');
    expect(params[0]).toBe(mint);
    return JSON.stringify({id:1,result:{context:{slot:23},value:{owner:classicTokenProgram,executable:false,data:[toBase64(data),'base64']}}});
  });
  const save = jest.fn(async () => {});
  const session = jest.fn(async () => {throw Error('unexpected');});
  let active: Promise<void> | undefined, tree:ReactTestRenderer.ReactTestRenderer | undefined;
  const press = (label:string) => ReactTestRenderer.act(async()=>{
    tree!.root.findAll(item=>item.props.accessibilityLabel===label && typeof item.props.onPress==='function')[0].props.onPress();
    await active;
  });
  try {
    await ReactTestRenderer.act(async()=>{
      tree=ReactTestRenderer.create(<WalletReadProfilesScreen verified={owner} value={state}
        save={save} rpc={rpc} session={session} found={[]} scan={()=>{}} back={()=>{}} busy={false}
        t={text=>text} work={run=>{active=run();}} />);
    });
    await ReactTestRenderer.act(async()=>{
      tree!.root.findAllByType(TextInput).find(input=>input.props.accessibilityLabel==='SPL 민트 주소')!.props.onChangeText(mint);
    });
    await press('토큰 정보 검토');
    expect(save).not.toHaveBeenCalled();
    expect(session).not.toHaveBeenCalled();
    expect(JSON.stringify(tree!.toJSON())).toContain('소수 자릿수 9');
    await press('검증된 토큰 추가');
    expect(save).toHaveBeenCalledWith(expect.objectContaining({selection:{...state.selection,asset:mint},
      nonEvmAssets:[expect.objectContaining({mint,slot:23,decimals:9})]}));
  } finally {
    await ReactTestRenderer.act(async()=>tree?.unmount());
  }
});

test('W25 searches only registered assets and restores the same contract and balance after hiding', async () => {
  const verified={network:'8283' as const, boardFingerprint:'2a'.repeat(32),generation:2,address:pendingVector.sender};
  const token='0x500ef69da230e42bff34487b578ea0cbefbda2ba';
  const another='0x'+'69'.repeat(20);
  const selection={...defaultReadState.selection,asset:token};
  const observation={scope:observationScope(verified.boardFingerprint,verified.generation,selection),address:verified.address,
    time:1700000000000,point:'Finalized block 90',amount:'42000000',unit:'ERC-20',decimals:6};
  let value: ReadProfileState={...defaultReadState,selection,observations:[observation],assets:[token,another].map(contract=>({
    board:verified.boardFingerprint,generation:2,id:'stable',account:0,contract,decimals:6,block:'0x'+'aa'.repeat(32),height:'90',
    hidden:false,favorite:false,
  }))};
  let tree:ReactTestRenderer.ReactTestRenderer|undefined, operation:Promise<void>|undefined;
  const rpc=jest.fn();
  const onOpenPending=jest.fn();
  const pendingEvm={chain:'8283' as const, genesis:pendingVector.genesis, raw:pendingVector.raw, hash:pendingVector.hash,
    sender:pendingVector.sender, expires:2000000000, submitted:true,
    evidence:{board:verified.boardFingerprint,generation:verified.generation,request:'16'.repeat(16),unsigned:pendingVector.unsigned}};
  const view=()=> <WalletReadProfilesScreen verified={verified} value={value} save={async next=>{value=next;}} found={[]}
    back={()=>{}} scan={()=>{}} busy={false} pendingEvm={pendingEvm} onOpenPending={onOpenPending} rpc={rpc} session={async()=>{throw Error('Unexpected board access');}}
    work={run=>{operation=run();}} t={label=>label}/>;
  const tap=(label:string)=>ReactTestRenderer.act(async()=>{
    tree!.root.findAll(node=>node.props.accessibilityLabel===label&&!!node.props.onPress)[0].props.onPress();
    await operation;
    tree!.update(view());
  });
  const search=(term:string)=>ReactTestRenderer.act(async()=>{
    tree!.root.findAllByType(TextInput).find(field=>field.props.accessibilityLabel==='등록 자산 검색')!.props.onChangeText(term);
  });
  const text=()=>JSON.stringify(tree!.toJSON());
  try {
    await ReactTestRenderer.act(async()=>{tree=ReactTestRenderer.create(view());});
    await search('TESTusdc');
    expect(text()).toContain(token);
    expect(text()).not.toContain(another);
    expect(text()).toContain('42');
    expect(text()).toContain(pendingVector.hash);
    await tap('즐겨찾기에 추가 1');
    await tap('목록에서 숨기기 1');
    expect(value.observations).toEqual([observation]);
    expect(text()).toContain('일치하는 등록 자산이 없습니다.');
    expect(text()).not.toContain(pendingVector.hash);
    await tap('숨긴 자산');
    expect(text()).toContain(pendingVector.hash);
    expect(text()).toContain('Finalized block 90');
    await tap('기존 송금 조회');
    expect(onOpenPending).toHaveBeenCalledTimes(1);
    await tap('목록에 다시 표시 1');
    await ReactTestRenderer.act(async()=>{tree!.unmount();tree=ReactTestRenderer.create(view());});
    await search('500ef69');
    expect(text()).toContain(token);
    expect(text()).toContain('Finalized block 90');
    expect(text()).toContain(pendingVector.hash);
    expect(value.assets?.[0]).toMatchObject({favorite:true,hidden:false,decimals:6});
    expect(rpc).not.toHaveBeenCalled();
  } finally {await ReactTestRenderer.act(async()=>tree?.unmount());}
});

test('W28 lists and scopes every built-in review route',async()=>{
  const all=Object.keys(readProfiles) as ReadProfileId[];
  const onReviewNetwork=jest.fn();
  const rpc=jest.fn();
  const owner={network:'8283' as const,boardFingerprint:'12'.repeat(32),generation:1,address:'0x'+'ab'.repeat(20)};
  const props={verified:owner,value:defaultReadState,save:async()=>{},found:[],scan:()=>{},back:()=>{},busy:false,rpc,
    session:async()=>{throw Error('No physical operation');},work:jest.fn(),t:(label:string)=>label,onReviewNetwork};
  let tree:ReactTestRenderer.ReactTestRenderer|undefined;
  const button=(label:string)=>tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0];
  try {
    await ReactTestRenderer.act(async()=>{tree=ReactTestRenderer.create(<WalletReadProfilesScreen {...props} reviewableNetworks={all}/>);});
    await ReactTestRenderer.act(async()=>button('체인별 기능표').props.onPress());
    for (const id of all) {
      const control=button('체인별 거래 원문 확인 · '+readProfiles[id].label);
      expect(control.props.disabled).toBe(false);
      control.props.onPress();
    }
    expect(onReviewNetwork.mock.calls).toEqual(all.map(id=>[id]));
    await ReactTestRenderer.act(async()=>{tree!.update(<WalletReadProfilesScreen {...props} reviewableNetworks={['btc']}/>);});
    expect(button('체인별 거래 원문 확인 · Bitcoin Signet').props.disabled).toBe(false);
    expect(button('체인별 거래 원문 확인 · XRP Ledger Testnet').props.disabled).toBe(true);
    expect(rpc).not.toHaveBeenCalled();
  } finally {await ReactTestRenderer.act(async()=>tree?.unmount());}
});
