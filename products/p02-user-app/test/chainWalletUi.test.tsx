import React from 'react';
import {ed25519} from "@noble/curves/ed25519";
import {p256} from "@noble/curves/p256";
import {sha256} from "@noble/hashes/sha256";
import {bigToBytes, bytesToHex, concat, hexToBytes, utf8Encode} from "@nu54/protocol";
import {base58,bitcoinWatchDescriptors,bitcoinXpubCommand,decodePublicBase58} from "../src/wallet/chainAccounts";

import ReactTestRenderer from 'react-test-renderer';
import {Image, Share, TextInput} from 'react-native';
import {ChainWalletScreen} from '../src/wallet/ChainWalletScreen';
import {decodeWalletStorage} from '../src/wallet/profile';
import type {OwnerSession} from '../src/wallet/ownerSession';

test('normal chain routes expose recipient form and XRP tagged receive QR/share', async () => {
  const posted = jest.spyOn(Share, 'share').mockResolvedValue({action: Share.dismissedAction, activityType: undefined});
  const destination = 'rwhKUCPbWBrj7pHZuNy57L8gb7hbfcc7gK';
  const session = {verifiedChainAccount: async () => ({chain: 3, network: 1, account: 0,
    address: destination, publicKey: new Uint8Array(33)})} as unknown as OwnerSession;
  let active = Promise.resolve();
  let tree: ReactTestRenderer.ReactTestRenderer | undefined;
  const trigger = (label: string) => {
    const button = tree!.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onPress === 'function')[0];
    expect(button.props.disabled).toBe(false);
    button.props.onPress();
    return active;
  };
  const field = (label: string) => tree!.root.findAllByType(TextInput).find(input => input.props.accessibilityLabel === label);
  try {
    await ReactTestRenderer.act(async () => {
      tree = ReactTestRenderer.create(<ChainWalletScreen initial="send" preferred={{chain: 3, network: 1, account: 0}}
        verified={{boardFingerprint: 'ab'.repeat(32), address: '0x' + 'ab'.repeat(20), network: '8283', generation: 1}}
        choose={async () => {}} found={[{name: 'Board', address: 'board'}]} scan={() => {}} back={() => {}}
        busy={false} save={async () => {}} persistSignature={async () => {}} work={run => {active = run();}}
        session={async (_address, operation) => operation(session)} prompt={() => {}}
        challenge={async () => new Uint8Array(32)} rpc={async () => ''} t={text => text} />);
    });
    expect(field('수취인 주소')).toBeDefined();
    expect(field('보낼 금액')).toBeDefined();
    expect(field('서명된 전송 정책')).toBeUndefined();
    await ReactTestRenderer.act(async () => { await trigger('받기'); });
    await ReactTestRenderer.act(async () => { await trigger('Board'); });
    await ReactTestRenderer.act(async () => { field('목적지 태그')?.props.onChangeText('0'); });
    expect(tree!.root.findByType(Image).props.source.uri).toMatch(/^data:image\/gif;base64,/);
    await ReactTestRenderer.act(async () => { await trigger('받기 정보 공유'); });
    expect(posted).toHaveBeenCalledWith({message: expect.stringContaining(
      'XRP Ledger · local laboratory\nAddress: ' + destination + '\nDestinationTag: 0')});
    await ReactTestRenderer.act(async () => { await trigger('Bitcoin 공동서명'); });
    expect(field('공동서명 계정 번호')).toBeDefined();
    expect(field('서명된 전송 정책')).toBeUndefined();

  } finally {
    await ReactTestRenderer.act(async () => tree?.unmount());
    posted.mockRestore();
  }
});

test('verified Bitcoin watch descriptors only leave through an explicit system share', async () => {
  const board = {boardFingerprint:'ab'.repeat(32), address:'0x'+'ab'.repeat(20), generation:1, network:'8283' as const};
  const watch = {receive:'wpkh(public/0/*)#example1',change:'wpkh(public/1/*)#example2',
    firstReceive:'bcrt1-receive',firstChange:'bcrt1-change'};
  const exportAccount = jest.fn(async () => watch);
  const posted = jest.spyOn(Share, 'share').mockResolvedValue({action:Share.dismissedAction,activityType:undefined});
  const rpc = jest.fn(async () => '');
  let active: Promise<void> | undefined;
  let tree: ReactTestRenderer.ReactTestRenderer | undefined;
  const press = (label: string) => ReactTestRenderer.act(async () => {
    tree!.root.findAll(node => node.props.accessibilityLabel === label && typeof node.props.onPress === 'function')[0].props.onPress();
    await active;
  });
  try {
    await ReactTestRenderer.act(async () => {
      tree = ReactTestRenderer.create(<ChainWalletScreen initial="receive" verified={board} found={[{name:'Board',address:'board'}]}
        choose={async () => {}} scan={() => {}} back={() => {}} busy={false} save={async () => {}}
        persistSignature={async () => {}} work={run => {active = run();}}
        session={async (_handle, action) => action({exportBitcoinWatchAccount:exportAccount} as unknown as OwnerSession)}
        prompt={() => {}} challenge={async () => new Uint8Array(32)} rpc={rpc} t={text => text} />);
    });
    expect(posted).not.toHaveBeenCalled();
    await press('읽기 전용 Bitcoin 계정 내보내기 · Board');
    expect(exportAccount).not.toHaveBeenCalled();
    await press('공개 범위 동의');
    expect(exportAccount).toHaveBeenCalledWith({chain:1,network:1,account:0},board.address,1n,expect.any(Function));
    expect(posted).not.toHaveBeenCalled();
    expect(rpc).not.toHaveBeenCalled();
    await press('공개 디스크립터 공유');
    expect(posted).toHaveBeenCalledWith({message: watch.receive+'\n'+watch.change});
  } finally {
    posted.mockRestore();
    await ReactTestRenderer.act(async () => tree?.unmount());
  }
});

test('W16 Signet scope cancellation shares nothing and reopens a protected watch-only record', async()=>{
  const owner={boardFingerprint:'bc'.repeat(32),address:'0x'+'12'.repeat(20),generation:5,network:'8283' as const};
  const account={chain:1,network:2,account:1} as const;
  const raw=concat(bitcoinXpubCommand(account),hexToBytes('1122334455667788'),new Uint8Array(32).fill(55),
    hexToBytes('0330d54fd0dd420a6e5f8d3624f5f3482cae350f79d5f0753bf5beef9c2d91af3c'));
  const watch=bitcoinWatchDescriptors(raw,account);
  expect(watch.firstReceive).toMatch(/^tb1q/);
  expect(watch.firstChange).toMatch(/^tb1q/);
  expect(()=>bitcoinWatchDescriptors(raw,{...account,network:1})).toThrow('BTC_XPUB_REPLY');
  const config={chain:1,profile:'bitcoin-signet',anchor:'synthetic',rpc:'https://wallet.example/bitcoin'} as const;
  const posted=jest.spyOn(Share,'share').mockResolvedValue({action:Share.dismissedAction,activityType:undefined});
  const approved=jest.fn(async()=>({...watch,proof:bytesToHex(raw,false)}));
  const signer=jest.fn();
  const rpc=jest.fn();
  let active:Promise<void>|undefined, tree:ReactTestRenderer.ReactTestRenderer|undefined, stored:string|undefined;
  const props={initial:'receive' as const,verified:owner,preferred:account,commissioned:[config],found:[{name:'Board',address:'board'}],
    choose:async()=>{},scan:()=>{},back:()=>{},busy:false,save:async()=>{},persistSignature:async()=>{},
    work:(run:()=>Promise<void>)=>{active=run();}, checkCommissioned:jest.fn(),
    session:async<T,>(_handle:string,action:(session:OwnerSession)=>Promise<T>)=>action({
      evmAuthority:async()=>({key:new Uint8Array(65),revision:1n}),exportBitcoinWatchAccount:approved,signChainTransfer:signer,
    } as unknown as OwnerSession),prompt:()=>{},challenge:async()=>new Uint8Array(32),rpc,t:(label:string)=>label};
  const click=(label:string)=>ReactTestRenderer.act(async()=>{
    tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0].props.onPress();
    await active;
  });
  try {
    await ReactTestRenderer.act(async()=>{tree=ReactTestRenderer.create(<ChainWalletScreen {...props} saveWatch={async bitcoinWatch=>{
      stored=JSON.stringify({version:2,mode:'personal',exercised:true,verified:owner,bitcoinWatch});
    }}/>);});
    await click('읽기 전용 Bitcoin 계정 내보내기 · Board');
    await click('취소');
    expect(approved).not.toHaveBeenCalled();
    expect(posted).not.toHaveBeenCalled();

    let finish:((value:{proof:string}&typeof watch)=>void)|undefined;
    approved.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;}));
    await click('읽기 전용 Bitcoin 계정 내보내기 · Board');
    await ReactTestRenderer.act(async()=>{
      tree!.root.findAll(node=>node.props.accessibilityLabel==='공개 범위 동의'&&typeof node.props.onPress==='function')[0].props.onPress();
      await Promise.resolve();
    });
    expect(finish).toBeDefined();
    await ReactTestRenderer.act(async()=>{
      tree!.root.findAll(node=>node.props.accessibilityLabel==='취소'&&typeof node.props.onPress==='function')[0].props.onPress();
      finish!({...watch,proof:bytesToHex(raw,false)});
      await active;
    });
    expect(stored).toBeUndefined();
    expect(posted).not.toHaveBeenCalled();
    approved.mockClear();

    await click('읽기 전용 Bitcoin 계정 내보내기 · Board');
    await click('공개 범위 동의');
    expect(approved).toHaveBeenCalledTimes(1);
    expect(stored).toBeDefined();
    expect(posted).not.toHaveBeenCalled();
    const recovered=decodeWalletStorage(stored!);
    await ReactTestRenderer.act(async()=>{
      tree!.unmount();
      tree=ReactTestRenderer.create(<ChainWalletScreen {...props} found={[]} commissioned={[]} savedWatch={recovered.bitcoinWatch}/>);
    });
    await click('저장된 읽기 전용 Bitcoin 계정');
    expect(JSON.stringify(tree!.toJSON())).toContain(watch.receive);
    expect(JSON.stringify(tree!.toJSON())).toContain(watch.change);
    expect(tree!.root.findAll(node=>node.props.accessibilityLabel==='보내기')).toHaveLength(0);
    await click('공개 디스크립터 공유');
    expect(posted).toHaveBeenCalledWith({message:watch.receive+'\n'+watch.change});
    expect(approved).toHaveBeenCalledTimes(1);
    expect(signer).not.toHaveBeenCalled();
    expect(rpc).not.toHaveBeenCalled();
  } finally {
    posted.mockRestore();
    await ReactTestRenderer.act(async()=>tree?.unmount());
  }
});

test('W28 reviews exact SOL bytes before a separate signing action and refusal signs or submits nothing', async () => {
  const key=ed25519.getPublicKey(new Uint8Array(32).fill(5)), target=ed25519.getPublicKey(new Uint8Array(32).fill(8));
  const address=base58(key), destination=base58(target), anchor=base58(new Uint8Array(32).fill(7));
  const owner={boardFingerprint:'ab'.repeat(32),address:'0x'+'34'.repeat(20),generation:1,network:'8283' as const};
  const blockhash=new Uint8Array(32).fill(42);
  const authority=new Uint8Array(32).fill(33);
  const header=concat(Uint8Array.of(11,2,1,0,0,0,0),bigToBytes(BigInt(Math.floor(Date.now()/1000)+50),8),
    bigToBytes(5000n,8),blockhash,bigToBytes(1n,8),bigToBytes(180n,8),bigToBytes(5000n,8),new Uint8Array(34),
    sha256(utf8Encode(anchor)),bigToBytes(1n,8));
  const raw=concat(hexToBytes('01000103'),key,target,new Uint8Array(32),blockhash,
    hexToBytes('01020200010c02000000'),bigToBytes(10000000n,8).reverse());
  const proof=p256.sign(sha256(concat(utf8Encode('N54M'),hexToBytes(owner.boardFingerprint),bigToBytes(1n,8),header,sha256(raw))),authority)
    .toCompactRawBytes();
  let grant=bytesToHex(concat(header,proof,raw),false);
  const rpc=jest.fn(async (_endpoint:string, packet:string)=>{
    const {method}=JSON.parse(packet);
    const result=method==='nu54_prepareWalletTransfer' ? {grant} : method==='getGenesisHash' ? anchor :
      method==='isBlockhashValid' ? {value:true} : method==='getBlockHeight' ? 80 : undefined;
    if (result === undefined) throw Error('Unexpected RPC '+method);
    return JSON.stringify({id:1,result});
  });
  const signChainTransfer=jest.fn(async()=>{throw Error('Unexpected board signature');});
  const board={verifiedChainAccount:async()=>({chain:2,network:1,account:0,address,publicKey:key}),
    evmAuthority:async()=>({key:p256.getPublicKey(authority,false),revision:1n}),signChainTransfer} as unknown as OwnerSession;
  const persistSignature=jest.fn(async()=>{});
  let active:Promise<void>|undefined,tree:ReactTestRenderer.ReactTestRenderer|undefined;
  let refused:string|undefined;
  const press=(label:string)=>ReactTestRenderer.act(async()=>{
    tree!.root.findAll(node=>node.props.accessibilityLabel===label&&typeof node.props.onPress==='function')[0].props.onPress();
    await active;
  });
  const value=(label:string,text:string)=>ReactTestRenderer.act(async()=>{
    tree!.root.findAllByType(TextInput).find(item=>item.props.accessibilityLabel===label)!.props.onChangeText(text);
  });
  try {
    await ReactTestRenderer.act(async()=>{tree=ReactTestRenderer.create(<ChainWalletScreen initial="send"
      preferred={{chain:2,network:1,account:0}} verified={owner} found={[{name:'Board',address:'board'}]} scan={()=>{}} back={()=>{}}
      busy={false} save={async()=>{}} choose={async()=>{}} rpc={rpc} persistSignature={persistSignature}
      work={(run,failure)=>{active=run().catch(error=>{refused=typeof failure==='string'?failure:failure(error);});}}
      session={async(_handle,fn)=>fn(board)} prompt={()=>{}}
      challenge={async()=>new Uint8Array(32)} t={text=>text} />);});
    await value('네트워크 식별자',anchor);
    await press('로컬 네트워크 확인');
    await value('수취인 주소',destination);
    await value('보낼 금액','0.01');
    await press('거래 원문 검토 · Board');
    const screen=JSON.stringify(tree!.toJSON());
    expect(screen).toContain('System Program · Transfer');
    expect(screen).toContain('5000');
    expect(screen).toContain(destination);
    expect(screen).toContain('Last valid block height');
    expect(signChainTransfer).not.toHaveBeenCalled();
    expect(persistSignature).not.toHaveBeenCalled();
    await press('거절하고 초안으로');
    const stake=concat(hexToBytes('01000507'),key,target,new Uint8Array(32).fill(61),new Uint8Array(32).fill(62),
      new Uint8Array(32).fill(63),new Uint8Array(32).fill(64),decodePublicBase58('Stake11111111111111111111111111111111111111',32),
      blockhash,hexToBytes('0106060102030405000402000000'));
    const signedStake=p256.sign(sha256(concat(utf8Encode('N54M'),hexToBytes(owner.boardFingerprint),bigToBytes(1n,8),
      header,sha256(stake))),authority).toCompactRawBytes();
    grant=bytesToHex(concat(header,signedStake,stake),false);
    await press('거래 원문 검토 · Board');
    expect(refused).toBe('스테이킹 요청을 거절했습니다. 서명 0건 · 제출 0건');
    expect(JSON.stringify(tree!.toJSON())).not.toContain('보드에 서명 요청');
    expect(signChainTransfer).not.toHaveBeenCalled();
    expect(persistSignature).not.toHaveBeenCalled();
    expect(rpc.mock.calls.every(([,packet])=>JSON.parse(packet).method!=='sendTransaction')).toBe(true);
  } finally {await ReactTestRenderer.act(async()=>tree?.unmount());}
});
