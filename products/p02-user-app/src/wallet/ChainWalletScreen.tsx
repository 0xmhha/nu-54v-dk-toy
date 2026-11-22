import {commissionedProfile,type CommissionedChain} from './commissionedChainProfiles.ts';
import {xrplIssue} from './nonEvmAssets.ts';
import type {ReadProof, WalletConnectivity} from './walletConnectivity.ts';
import {useEffect, useRef, useState} from 'react';
import {WalletReadProfilesScreen} from './WalletReadProfilesScreen.tsx';
import {defaultReadState, type ReadProfileId, type ReadProfileState} from './readProfiles.ts';
import type {StoredBip48} from './bip48Wallet.ts';
import {Bip48WalletScreen} from './Bip48WalletScreen.tsx';
import {advancedRequestRefused} from './walletCapabilities.ts';
import {bytesToHex,hexToBytes} from '@nu54/protocol';
import {AppState, Image, InputAccessoryView, Keyboard, Platform, Pressable, Share, StyleSheet, Text, TextInput, View} from 'react-native';
import {presentReceive, receiveQr} from './receivePresentation.ts';
import {bitcoinHrp, bitcoinWatchDescriptors, chainLabels, type ChainAccount, type ChainPublicAccount, type WalletChain, type StoredBitcoinWatch} from './chainAccounts.ts';
import {bitcoinReplacementDraft, checkBitcoinReplacement, checkChainNetwork, ensureBitcoinPredecessor, loopbackChainRpc, prepareChainTransfer, queryChainTransfer, storedChainRpc, readChainReceiveBalance, submitChainTransfer, verifyChainGrantWindow,
  type ChainDraft, type ChainResult, type StoredChain} from './chainTransfer.ts';
import type {OwnerSession} from './ownerSession.ts';
import type {PendingEvmTransfer, PersonalMetadata} from './profile.ts';

type Props = {
  pickBip48File?:()=>Promise<string>;
  pendingBip48?:StoredBip48;
  persistBip48?:(wire:string,expires:number,sign:()=>Promise<StoredBip48>)=>Promise<void>;
  onWatchSignet?: (account:number)=>void;
  savedWatch?:StoredBitcoinWatch;
  saveWatch?: (snapshot:StoredBitcoinWatch)=>Promise<void>;
  commissioned?: CommissionedChain[];
  commissionedToken?: string;
  checkCommissioned?: (scope:Pick<StoredChain,'chain'|'network'|'anchor'|'endpoint'>,key:Uint8Array,revision:bigint)=>void;
  language?: 'ko' | 'en';
  pendingEvm?: PendingEvmTransfer;
  onOpenPending?: () => void;
  initial: 'receive' | 'send' | 'profiles';
  readState?: ReadProfileState;
  connectivity?: WalletConnectivity;
  onReadProof?: (proof: ReadProof | undefined) => void;
  reviewableNetworks?:readonly ReadProfileId[];
  onReviewNetwork?: (id: ReadProfileId) => void;
  saveRead?: (next: ReadProfileState) => Promise<void>;
  verified: PersonalMetadata;
  preferred?: ChainAccount;
  choose: (value: ChainAccount) => Promise<void>;
  found: {address: string; name: string}[];
  scan: () => void;
  back: () => void;
  busy: boolean;
  pending?: StoredChain;
  save: (pending?: StoredChain) => Promise<void>;
  persistSignature: (wire: string, expires: number, approve: () => Promise<StoredChain>) => Promise<void>;
  work: (operation: () => Promise<void>, failure: string | ((error: unknown) => string)) => void;
  session: <T>(handle: string, action: (session: OwnerSession) => Promise<T>) => Promise<T>;
  /** `onBoard` marks a prompt shown while the request waits on the independent display and the board. */
  prompt: (message: string, onBoard?: boolean) => void;
  challenge: () => Promise<Uint8Array>;
  rpc: (endpoint: string, packet: string) => Promise<string>;
  t: (message: string) => string;
};

function Button({text, run, disabled = false}: {text: string; run: () => void; disabled?: boolean}) {
  return <Pressable accessibilityRole="button" accessibilityLabel={text} accessibilityState={{disabled}} disabled={disabled} onPress={run}
    style={[styles.button, disabled && styles.disabled]}><Text style={styles.buttonLabel}>{text}</Text></Pressable>;
}

export function ChainWalletScreen({initial,pickBip48File,pendingBip48,persistBip48,onWatchSignet,savedWatch,saveWatch, readState, saveRead, connectivity, language, commissioned, commissionedToken, checkCommissioned, pendingEvm, onOpenPending, onReadProof, onReviewNetwork, reviewableNetworks, verified, preferred, choose, found, scan, back, busy, pending, save, persistSignature, work,
  session, prompt, challenge, rpc, t}: Props) {
  const [mode, setMode] = useState<'receive' | 'send' | 'bip48' | 'profiles'>(initial);
  const [selected, setSelected] = useState<ChainAccount>(preferred ?? {chain: 1, network: 1, account: 0});
  const [boardAccount, setBoardAccount] = useState<ChainPublicAccount>();
  const [receiveBalance, setReceiveBalance] = useState<Awaited<ReturnType<typeof readChainReceiveBalance>>>();
  const [tag, setTag] = useState('');
  const watchEpoch=useRef({revision:0});
  const [watchStage,setWatchStage]=useState<{step:'scope'|'waiting'|'export'; handle?:string}>();
  const [watch, setWatch] = useState<ReturnType<typeof bitcoinWatchDescriptors>>();
  const [destination, setDestination] = useState('');
  const [amount, setAmount] = useState('');
  const [asset, setAsset] = useState('');
  const [source, setSource] = useState('');
  const [issuer, setIssuer] = useState('');
  const [outpoint, setOutpoint] = useState('');
  const [replaceable, setReplaceable] = useState(false);
  const defaultRpc = (chain: WalletChain) => 'https://127.0.0.1:8443/' + ({1: 'bitcoin', 2: 'solana', 3: 'xrpl'}[chain]);
  const approvedNetwork = selected.network===2 ? commissioned?.find(row=>row.chain===selected.chain) : undefined;
  const [endpoint, setEndpoint] = useState(defaultRpc(selected.chain));
  const [anchor, setAnchor] = useState('');
  const [networkOk, setNetworkOk] = useState(false);
  const [result, setResult] = useState<ChainResult>();
  const [replacementFee, setReplacementFee] = useState('');
  const [history, setHistory] = useState<Record<string, ChainResult>>({});
  const generation = BigInt(verified.generation);
  const reviewEpoch=useRef({revision:0});
  const [prepared,setPrepared] = useState<{transfer: Awaited<ReturnType<typeof prepareChainTransfer>>;
    endpoint:string; anchor:string; board:string; generation:number;
    authority:string; revision:string; original?:StoredChain; fee?:number}>();
  useEffect(() => {
    const lifecycle=reviewEpoch.current;
    const watchLifecycle=watchEpoch.current;
    lifecycle.revision++;
    setPrepared(undefined);
    const app=AppState.addEventListener('change',state=>{
      if (state === 'active') return;
      lifecycle.revision++;
      watchLifecycle.revision++;
      setWatch(undefined);
      setWatchStage(undefined);
      setPrepared(undefined);
    });
    return () => {lifecycle.revision++; watchLifecycle.revision++; app.remove();};
  },[verified.boardFingerprint, verified.generation, mode]);
  const select = (value: ChainAccount) => work(async () => {
    await choose(value);
    reviewEpoch.current.revision++;
    setPrepared(undefined);
    setSelected(value);
    setBoardAccount(undefined);
    setReceiveBalance(undefined);
    setTag('');
    setWatch(undefined);
    setDestination('');
    setAmount('');
    setAsset('');
    setSource('');
    setIssuer('');
    setOutpoint('');
    setReplaceable(false);
    setNetworkOk(false);
    if (selected.chain !== value.chain) {
      setAnchor('');
      setEndpoint(defaultRpc(value.chain));
    }
  }, t('계정 선택을 저장하지 못했습니다.'));

  let card: ReturnType<typeof presentReceive> | undefined;
  try {
    if (boardAccount) {
      const network = chainLabels[boardAccount.chain] + ' · ' + (boardAccount.network===2 ? commissionedProfile(boardAccount.chain) : boardAccount.network ? 'local laboratory' : 'mainnet');
      card = presentReceive(boardAccount.chain === 1 ? {kind:'btc', network, hrp:bitcoinHrp(boardAccount.network)} :
        boardAccount.chain === 2 ? {kind:'sol', network} : {kind:'xrp', network, test:boardAccount.network !== 0, tag:tag || undefined},
        boardAccount.address);
    }
  } catch { /* Invalid data keeps the QR and share action hidden. */ }

  function receive(handle: string) {
    work(async () => {
      setBoardAccount(undefined);
    setReceiveBalance(undefined);
      const value = await session(handle, active => active.verifiedChainAccount(selected, verified.address, generation));
      setBoardAccount(value);
      if (value.chain === 3) setTag('');
    }, t('보드의 계정 주소를 확인하지 못했습니다.'));
  }

  const matchingWatch = savedWatch?.board===verified.boardFingerprint && savedWatch.generation===verified.generation &&
    savedWatch.account.chain===selected.chain && savedWatch.account.network===selected.network &&
    savedWatch.account.account===selected.account ? savedWatch : undefined;
  const watchAvailable=selected.chain===1 && (selected.network===1 || !!approvedNetwork);
  const stopWatch=()=>{
    ++watchEpoch.current.revision;
    setWatch(undefined);
    setWatchStage(undefined);
  };
  function watchOnly(handle: string) {
    if (!watchAvailable) return;
    const sequence=++watchEpoch.current.revision;
    setWatchStage({step:'waiting'});
    work(async () => {
      try {
        const descriptors = await session(handle, async active => {
          if (selected.network===2) {
            if (!approvedNetwork || !checkCommissioned) throw new Error('CHAIN_COMMISSIONED_SCOPE');
            const trust=await active.evmAuthority();
            checkCommissioned({...selected,endpoint:approvedNetwork.rpc,anchor:approvedNetwork.anchor},trust.key,trust.revision);
          }
          if (sequence!==watchEpoch.current.revision) throw new Error('BTC_XPUB_CANCELLED');
          return active.exportBitcoinWatchAccount(selected,verified.address,generation,
            () => prompt(t('독립 화면에서 읽기 전용 Bitcoin 계정을 확인한 뒤 보드에서 승인하세요.'), true));
        });
        if (sequence!==watchEpoch.current.revision || AppState.currentState==='background') return;
        await saveWatch?.({account:selected, board:verified.boardFingerprint,generation:verified.generation,proof:descriptors.proof});
        if (sequence!==watchEpoch.current.revision || AppState.currentState==='background') return;
        setWatch(descriptors);
        setWatchStage({step:'export'});
      } catch (error) {
        if (sequence===watchEpoch.current.revision) stopWatch();
        throw error;
      }
    }, error => t(error instanceof Error && error.message === 'CHAIN_USER_REFUSED' ?
      '보드에서 읽기 전용 내보내기를 거절했습니다.' : '읽기 전용 계정을 확인하지 못했습니다.'));
  }

  function prepare(handle:string, original?:StoredChain) {
    work(async () => {
      setPrepared(undefined);
      const epoch=++reviewEpoch.current.revision;
      if (original ? original.ownerGeneration !== verified.generation || original.clock?.board !== verified.boardFingerprint ||
        !/^[1-9][0-9]{0,5}$/.test(replacementFee) : pending || !networkOk) throw new Error('CHAIN_NETWORK_REQUIRED');
      const targetTag = tag.trim();
      if (!original && targetTag && (selected.chain !== 3 || !/^(0|[1-9][0-9]{0,9})$/.test(targetTag) ||
          BigInt(targetTag) > 0xffffffffn)) throw new Error('XRPL_DESTINATION_TAG');
      if (!original && selected.chain === 3 && asset.trim()) xrplIssue(asset.trim(),issuer.trim());
      const scope = original ?? {...selected,endpoint:approvedNetwork?.rpc ?? endpoint.trim(),anchor:approvedNetwork?.anchor ?? anchor.trim()};
      if (scope.network===2 && !approvedNetwork) throw new Error('CHAIN_COMMISSIONED_SCOPE');
      const url=scope.endpoint, networkAnchor=scope.anchor;
      const connect=storedChainRpc(scope,rpc,commissionedToken);
      const draft:ChainDraft = original ? bitcoinReplacementDraft(original,Number(replacementFee)) : {
        destination:destination.trim(),amount:amount.trim(),asset:asset.trim() || undefined,source:source.trim() || undefined,issuer:issuer.trim() || undefined,
        outpoint:outpoint.trim() || undefined,tag:targetTag ? Number(targetTag) : undefined,replaceable:selected.network===1 && replaceable || undefined,
      };
      await checkChainNetwork(connect, original ?? selected, networkAnchor);
      if (original) await ensureBitcoinPredecessor(connect,original.id);
      const safe = await session(handle,async active => {
        const account = await active.verifiedChainAccount(original ?? selected,verified.address,generation);
        const change = account.chain === 1 ? await active.verifiedChainAccount(account,verified.address,generation,1) : undefined;
        const trust = await active.evmAuthority();
        if (scope.network===2) {
          if (!checkCommissioned) throw new Error('CHAIN_COMMISSIONED_SCOPE');
          checkCommissioned(scope,trust.key,trust.revision);
        }
        const transfer = await prepareChainTransfer(connect,account,change,draft,networkAnchor,
          verified.boardFingerprint,generation,trust.revision,trust.key);
        if (original) checkBitcoinReplacement(transfer,original,draft);
        await verifyChainGrantWindow(connect,transfer);
        return {transfer,authority:bytesToHex(trust.key,false),revision:trust.revision.toString()};
      });
      if (epoch!==reviewEpoch.current.revision || AppState.currentState === 'background') return;
      setPrepared({...safe, endpoint:url,anchor:networkAnchor,board:verified.boardFingerprint,
        generation:verified.generation,original,fee:draft.fee});
    }, error=>error instanceof Error && error.message==='SOL_UNSUPPORTED_STAKE'
      ? t(advancedRequestRefused.stake)+' '+t('서명 0건 · 제출 0건') : t('거래 원문을 확인하지 못했습니다. 다시 검토하세요.'));
  }

  function signReviewed(handle:string) {
    work(async () => {
      if (!prepared) throw new Error('CHAIN_REVIEW_REQUIRED');
      const {transfer, board, original, authority, revision} = prepared;
      const epoch=reviewEpoch.current.revision;
      if (prepared.generation !== verified.generation || board !== verified.boardFingerprint ||
          transfer.expires <= Math.floor(Date.now()/1000) || original?.id !== pending?.id) {
        setPrepared(undefined);
        throw new Error('CHAIN_REVIEW_EXPIRED');
      }
      const scope={...transfer,endpoint:prepared.endpoint,anchor:prepared.anchor};
      const connection=storedChainRpc(scope,rpc,commissionedToken);
      await checkChainNetwork(connection,transfer,prepared.anchor);
      if (original) await ensureBitcoinPredecessor(connection,original.id);
      await verifyChainGrantWindow(connection,transfer);
      if (epoch!==reviewEpoch.current.revision || AppState.currentState==='background') throw new Error('CHAIN_REVIEW_EXPIRED');
      await session(handle,async active => {
        const trust=await active.evmAuthority();
        if (bytesToHex(trust.key,false) !== authority || trust.revision.toString() !== revision)
          throw new Error('CHAIN_GRANT_SCOPE');
        if (scope.network===2) {
          if (!checkCommissioned) throw new Error('CHAIN_COMMISSIONED_SCOPE');
          checkCommissioned(scope,trust.key,trust.revision);
        }
        if (epoch!==reviewEpoch.current.revision) throw new Error('CHAIN_REVIEW_EXPIRED');
        await persistSignature(bytesToHex(transfer.body,false),transfer.expires,async () => {
          if (epoch!==reviewEpoch.current.revision) throw new Error('CHAIN_REVIEW_EXPIRED');
          const signed=await active.signChainTransfer(transfer,verified.address,generation,() =>
            prompt(t('독립 화면에서 수취인, 금액, 수수료를 확인하고 보드에서 승인 또는 거절하세요.'), true));
          if (original && (!signed.btc || signed.btc.fee !== prepared.fee || signed.id === original.id))
            throw new Error('BTC_REPLACEMENT_SIGNED');
          return {...signed,...(original && signed.btc && original.btc ?
            {btc:{...signed.btc,previous:[...original.btc.previous,original.id]}} : {}),
            endpoint:prepared.endpoint,anchor:prepared.anchor,ownerAddress:verified.address,
            ownerGeneration:verified.generation,review:transfer.review,clock:{key:authority,board,revision}};
        });
      });
      setPrepared(undefined);
      setDestination('');
      setAmount('');
      setTag('');
      setReplacementFee('');
      setResult(undefined);
      prompt(t('승인된 원문과 거래 ID를 저장했습니다.'));
    }, error => {
      setPrepared(undefined);
      return t(error instanceof Error && error.message === 'CHAIN_USER_REFUSED' ?
        '보드에서 전송을 거절했습니다.' : '보드에서 승인된 전송을 확인하지 못했습니다.');
    });
  }

  function track(action: 'submit' | 'query') {
    if (!pending) return;
    work(async () => {
      if (pending.network===2 && !commissioned?.some(row=>row.chain===pending.chain && row.anchor===pending.anchor && row.rpc===pending.endpoint))
        throw new Error('CHAIN_COMMISSIONED_SCOPE');
      const send = storedChainRpc(pending,rpc,commissionedToken);
      if (action === 'submit') {
        if (pending.network===2) {
          if (!checkCommissioned || !pending.clock || pending.ownerGeneration!==verified.generation ||
              pending.clock.board!==verified.boardFingerprint || pending.ownerAddress!==verified.address) throw new Error('CHAIN_COMMISSIONED_SCOPE');
          checkCommissioned(pending,hexToBytes(pending.clock.key),BigInt(pending.clock.revision));
        }
        const outcome = await submitChainTransfer(send, pending, {challenge, saveAttempt: save,
          monotonic: () => (globalThis as {performance?: {now: () => number}}).performance?.now() ?? Number.NaN});
        setResult(outcome);
        if (outcome === 'pending') prompt(t('전송을 요청했습니다. 같은 거래 ID로 상태를 확인하세요.'));
      } else {
        await checkChainNetwork(send, pending, pending.anchor);
        setResult(await queryChainTransfer(send, pending));
        if (pending.btc?.previous.length) {
          const previous = await Promise.all(pending.btc.previous.map(async id =>
            [id, await queryChainTransfer(send, {...pending, id})] as const));
          setHistory(Object.fromEntries(previous));
        }
      }
    }, error => {
      if (error instanceof Error && error.message === 'CHAIN_SUBMISSION_EXPIRED') {
        if (!pending.attempted) setResult('expired');
        return t('전송 승인 시간이 지났습니다. 같은 거래 ID로 상태를 확인할 수 있습니다.');
      }
      return t('거래 결과가 불확실합니다. 같은 거래 ID의 상태를 다시 확인하세요.');
    });
  }

  if (mode === 'profiles' && saveRead) return <WalletReadProfilesScreen verified={verified} value={readState ?? defaultReadState} connectivity={connectivity} language={language} pendingEvm={pendingEvm} onOpenPending={onOpenPending} onReadProof={onReadProof} onReviewNetwork={onReviewNetwork} reviewableNetworks={reviewableNetworks} onWatchSignet={onWatchSignet}
    save={saveRead} found={found} scan={scan} busy={busy} back={initial === 'profiles' ? back : () => setMode(initial)} work={work} session={session} rpc={rpc} t={t} />;

  if (watchStage) return <View>
    <Text accessibilityRole="header" style={styles.label}>{t(watchStage.step==='export' ? '읽기 전용 Bitcoin 계정' : 'Bitcoin 공개 범위')}</Text>
    <Text style={styles.label}>Bitcoin {selected.network===2?'Signet':'regtest'} · Native SegWit · {t('계정 인덱스')} {selected.account}</Text>
    <Text style={styles.label}>{t('수신 및 거스름돈 주소 · 서명 기능 없음')}</Text>
    {watchStage.step==='scope' && <>
      <Text style={styles.label}>{t('공유받은 사람이 계정의 과거와 미래 주소를 서로 연결할 수 있습니다.')}</Text>
      <Button text={t('공개 범위 동의')} disabled={busy} run={()=>{if (watchStage.handle) watchOnly(watchStage.handle);}} />
    </>}
    {watchStage.step==='waiting' && <Text style={styles.label}>{t('보드 확인 대기')}</Text>}
    {watchStage.step==='export' && watch && <>
      <Text selectable style={styles.label}>{watch.firstReceive+'\n'+watch.firstChange}</Text>
      <Text selectable style={styles.label}>{watch.receive+'\n'+watch.change}</Text>
      <Button text={t('공개 디스크립터 공유')} disabled={busy} run={()=>work(async()=>{
        await Share.share({message:watch.receive+'\n'+watch.change});
      },t('공개 디스크립터를 공유하지 못했습니다.'))} />
    </>}
    <Button text={t(watchStage.step==='export'?'계정 관리':'취소')} run={stopWatch} />
  </View>;

  if (mode === 'bip48') return <Bip48WalletScreen pickFile={pickBip48File} accountIndex={selected.account} registered={approvedNetwork}
    token={commissionedToken} verifyRegistered={checkCommissioned} saved={pendingBip48} persist={persistBip48} verified={verified} found={found} scan={scan} busy={busy}
    back={() => setMode(initial)} work={work} session={session} prompt={prompt} rpc={rpc} t={t} />;

  return <View>
    <View style={styles.row}>
      <Button text={t('받기')} disabled={busy || mode === 'receive'} run={() => setMode('receive')} />
      <Button text={t('보내기')} disabled={busy || mode === 'send'} run={() => setMode('send')} />
      {(selected.network===1 || (selected.chain===1 && approvedNetwork)) && <Button text={t('Bitcoin 공동서명')} disabled={busy} run={() => setMode('bip48')} />}
      {saveRead && <Button text={t('6개 체인 · 읽기 전용')} disabled={busy} run={() => {
        setDestination('');
        setAmount('');
        setAsset('');
        setSource('');
    setIssuer('');
        setTag('');
        setOutpoint('');
        setReplacementFee('');
        setReplaceable(false);
        setNetworkOk(false);
        setMode('profiles');
      }} />}
    </View>
    {(mode === 'receive' || !pending) && <>
      <Text style={styles.label}>{chainLabels[selected.chain]} · {t(selected.network===2 ? approvedNetwork?.profile ?? '등록 프로필 필요' : '로컬 시험망')} · {t('계정')} {selected.account + 1}</Text>
      {selected.network===1 && <View style={styles.row}>{([1, 2, 3] as WalletChain[]).map(chain => <Button key={chain} text={chainLabels[chain]}
        disabled={busy || chain === selected.chain} run={() => select({...selected, chain, account: 0})} />)}</View>}
      <View style={styles.row}>
        <Button text={t('이전 계정')} disabled={busy || selected.account === 0}
          run={() => select({...selected, account: selected.account - 1})} />
        <Button text={t('다음 계정')} disabled={busy || selected.account === 99}
          run={() => select({...selected, account: selected.account + 1})} />
      </View>
    </>}
    {mode === 'receive' && <>
      <Button text={t('개인 기기 찾기')} disabled={busy} run={scan} />
      {found.map(item => <Button key={item.address} text={item.name || t('보드와 수신 주소 확인')}
        disabled={busy} run={() => receive(item.address)} />)}
      {matchingWatch && <Button text={t('저장된 읽기 전용 Bitcoin 계정')} disabled={busy} run={()=>{
        setWatch(bitcoinWatchDescriptors(hexToBytes(matchingWatch.proof),selected));
        setWatchStage({step:'export'});
      }} />}
      {watchAvailable && found.map(item => <Button key={'watch-' + item.address}
        text={t('읽기 전용 Bitcoin 계정 내보내기') + ' · ' + item.name}
        disabled={busy} run={() => setWatchStage({step:'scope',handle:item.address})} />)}
      {boardAccount && <>
        <Text selectable style={styles.label}>{boardAccount.address}</Text>
        {boardAccount.network===1 && <>
        <TextInput style={styles.field} value={endpoint} maxLength={512} editable={!busy}
          accessibilityLabel={t('로컬 HTTPS RPC 주소')} onChangeText={value => {setEndpoint(value); setReceiveBalance(undefined);}} />
        <TextInput style={styles.field} value={anchor} maxLength={64} editable={!busy}
          accessibilityLabel={t('네트워크 식별자')} placeholder={t('네트워크 식별자')}
          onChangeText={value => {setAnchor(value); setReceiveBalance(undefined);}} />
        <Button text={t('입금 잔액 다시 확인')} disabled={busy || !anchor.trim()} run={() => work(async () => {
          setReceiveBalance(undefined);
          setReceiveBalance(await readChainReceiveBalance(loopbackChainRpc(endpoint.trim(), rpc), boardAccount, anchor.trim()));
        }, error => t(error instanceof Error && error.message === 'XRPL_BALANCE_LEDGER' ?
          'XRPL 확정 원장을 다시 확인하지 못했습니다.' : '확정 입금 잔액을 확인하지 못했습니다.'))} />
        </>}
        {receiveBalance && <Text selectable style={styles.label}>
          {'status' in receiveBalance && receiveBalance.status === 'inactive' ?
            t('첫 입금 대기 · XRPL 계정 미활성') : receiveBalance.amount + ' ' + receiveBalance.unit} · {receiveBalance.observation}
        </Text>}

        {boardAccount.chain === 3 && <TextInput style={styles.field} value={tag} maxLength={10}
          accessibilityLabel={t('목적지 태그')} placeholder={t('목적지 태그 (선택)')} keyboardType="number-pad" onChangeText={setTag} />}
        {card && <>
          <Text selectable style={styles.label}>{card.detail}</Text>
          <Image accessibilityLabel={t('수신 QR 코드')} source={{uri: receiveQr(card.payload)}} style={styles.qr} />
          <Button text={t('받기 정보 공유')} run={() => work(async () => {
            await Share.share({message: card.share});
          }, t('받기 정보를 공유하지 못했습니다.'))} />
        </>}
      </>}
    </>}
    {mode === 'send' && !pending && !prepared && <>
      {Platform.OS === 'ios' && <InputAccessoryView nativeID="wallet-chain-send-input">
        <Button text={language === 'en' ? 'Done entering transfer details' : '전송 입력 완료'} run={Keyboard.dismiss} />
      </InputAccessoryView>}
      {selected.network===1 ? <><TextInput style={styles.field} value={endpoint} maxLength={512} autoCorrect={false} autoCapitalize="none"
        accessibilityLabel={t('로컬 HTTPS RPC 주소')} placeholder={t('로컬 HTTPS RPC 주소')}
        onChangeText={value => { setEndpoint(value); setNetworkOk(false); }} />
      <TextInput style={styles.field} value={anchor} maxLength={64} autoCorrect={false} autoCapitalize="none"
        accessibilityLabel={t('네트워크 식별자')} placeholder={selected.chain === 3 ? 'Network ID' : 'Genesis hash'}
        onChangeText={value => { setAnchor(value); setNetworkOk(false); }} /></> : <Text selectable style={styles.label}>{approvedNetwork?.rpc} · {approvedNetwork?.anchor}</Text>}
      <Button text={networkOk ? t('네트워크 확인 완료') : t(selected.network===2 ? '등록된 시험망 확인' : '로컬 네트워크 확인')} disabled={busy || networkOk || (selected.network===2 ? !approvedNetwork : !anchor)}
        run={() => work(async () => {
          const scope={...selected,endpoint:approvedNetwork?.rpc ?? endpoint.trim(),anchor:approvedNetwork?.anchor ?? anchor.trim()};
          await checkChainNetwork(storedChainRpc(scope,rpc,commissionedToken),selected,scope.anchor);
          setNetworkOk(true);
        }, t('로컬 네트워크의 식별자가 일치하지 않습니다.'))} />
      <TextInput style={styles.field} value={destination} maxLength={85} onChangeText={setDestination}
        editable={!busy} autoCorrect={false} autoCapitalize="none" placeholder={t('수취인 주소')}
        accessibilityLabel={t('수취인 주소')} />
      <TextInput style={styles.field} value={amount} maxLength={34} onChangeText={setAmount}
        inputAccessoryViewID={Platform.OS === 'ios' ? 'wallet-chain-send-input' : undefined}
        editable={!busy} keyboardType="decimal-pad" placeholder={t('보낼 금액')} accessibilityLabel={t('보낼 금액')} />
      {selected.chain === 1 && selected.network===1 && <>
        <TextInput style={styles.field} value={outpoint} maxLength={74} onChangeText={setOutpoint}
          editable={!busy} autoCapitalize="none" placeholder={t('지출할 UTXO (선택)')} accessibilityLabel={t('지출할 UTXO')} />
        <Button text={t(replaceable ? '수수료 교체 가능' : '수수료 교체 없음')} disabled={busy}
          run={() => setReplaceable(!replaceable)} />
      </>}
      {selected.network===1 && selected.chain !== 1 && <TextInput style={styles.field} value={asset} maxLength={50} onChangeText={setAsset}
        editable={!busy} autoCorrect={false} autoCapitalize="none"
        accessibilityLabel={t('자산')} placeholder={selected.chain === 2 ? t('SPL 민트 (SOL은 비워 두세요)') :
          t('발행 자산 통화 코드 (XRP는 비워 두세요)')} />}
      {selected.chain === 2 && !!asset && <TextInput style={styles.field} value={source} maxLength={45} onChangeText={setSource}
        editable={!busy} autoCorrect={false} autoCapitalize="none" accessibilityLabel={t('토큰 출금 계정')}
        placeholder={t('토큰 출금 계정')} />}
      {selected.chain === 3 && !!asset && <TextInput style={styles.field} value={issuer} maxLength={35}
        editable={!busy} autoCorrect={false} autoCapitalize="none" accessibilityLabel={t('XRPL 발행자 주소')}
        placeholder={t('XRPL 발행자 주소')} onChangeText={setIssuer} />}
      {selected.chain === 3 && <TextInput style={styles.field} value={tag} maxLength={10} onChangeText={setTag}
        inputAccessoryViewID={Platform.OS === 'ios' ? 'wallet-chain-send-input' : undefined}
        editable={!busy} keyboardType="number-pad" accessibilityLabel={t('목적지 태그')}
        placeholder={t('목적지 태그 (선택)')} />}
      <Button text={t('개인 기기 찾기')} disabled={busy || !networkOk} run={scan} />
      {found.map(item => <Button key={item.address} text={t('거래 원문 검토') + ' · ' + item.name}
        disabled={busy || !destination.trim() || !amount.trim() || !networkOk || (selected.chain===3 && !!asset.trim() && !issuer.trim())} run={() => prepare(item.address)} />)}
    </>}
    {mode === 'send' && prepared && <View>
      <Text accessibilityRole="header" style={styles.label}>{t('검증된 거래 원문')}</Text>
      <Text selectable style={styles.label}>{chainLabels[prepared.transfer.chain]} · {t(prepared.transfer.network===2 ? approvedNetwork?.profile ?? '등록 프로필 필요' : '로컬 시험망')} · {prepared.anchor}</Text>
      <Text selectable style={styles.label}>{t('보내는 계정')} {prepared.transfer.review.sender}</Text>
      <Text selectable style={styles.label}>{t('수취인 주소')} {prepared.transfer.review.destination}</Text>
      <Text style={styles.label}>{t('승인 요청 만료')} {new Date(prepared.transfer.expires * 1000).toISOString()}</Text>
      <Text selectable style={styles.label}>{t('보낼 금액')} {prepared.transfer.review.amount} · {prepared.transfer.review.asset}</Text>
      <Text selectable style={styles.label}>{t('수수료')} {prepared.transfer.review.fee} {prepared.transfer.review.feeUnit}
        {' · '}{t('수수료 한도')} {prepared.transfer.review.maximumFee}</Text>
      {prepared.transfer.review.fields.map(field => <Text key={field.label} selectable style={styles.label}>
        {t(field.label)} · {field.value}</Text>)}
      {prepared.original && <Text selectable style={styles.label}>{t('원 거래 ID')} {prepared.original.id}</Text>}
      {found.map(item => <Button key={'review-'+item.address} text={t('기기에 전송 승인 요청') + ' · ' + item.name}
        disabled={busy} run={() => signReviewed(item.address)} />)}
      <Button text={t('거절하고 초안으로')} disabled={busy} run={() => {
        reviewEpoch.current.revision++;
        setPrepared(undefined);
        prompt(t('이 원문을 거절했습니다. 서명 0건 · 제출 0건'));
      }} />
    </View>}
    {mode === 'send' && pending && !prepared && <>
      <Text style={styles.label}>{chainLabels[pending.chain]} · {t(pending.network===2 ? commissionedProfile(pending.chain) : '로컬 시험망')} · {t('계정')} {pending.account + 1}</Text>
      <Text selectable style={styles.label}>{pending.id}</Text>
      <Text accessibilityLiveRegion="polite" style={styles.label}>
        {t(pending.attempted ? '기존 거래 상태 확인 필요' : '서명됨 · 전송 전')}
      </Text>
      {!!pending.btc?.vsize && <Text style={styles.label}>
        {(pending.btc.fee / pending.btc.vsize).toFixed(2)} sat/vB · {pending.btc.vsize} vB
      </Text>}
      {pending.review && <>
        <Text selectable style={styles.label}>{t('수취인 주소')} {pending.review.destination}</Text>
        <Text style={styles.label}>{pending.review.amount} · {pending.review.asset} · {t('수수료')} {pending.review.fee} {pending.review.feeUnit}</Text>
        {pending.review.fields.map(field => <Text key={field.label} selectable style={styles.label}>{t(field.label)} · {field.value}</Text>)}
      </>}
      {pending.btc?.previous.map(id => <Text key={id} selectable style={styles.label}>
        {t('원 거래 ID')} {id} {history[id] && t(({pending: '미확정', included: '블록에 포함됨',
          finalized: '확정됨', failed: '실패', expired: '만료'})[history[id]])}
      </Text>)}
      {pending.attempted && pending.btc?.replaceable && pending.ownerGeneration === verified.generation && pending.clock?.board === verified.boardFingerprint &&
        (result === undefined || result === 'pending') && <>
          <Text style={styles.label}>{t('현재 수수료 (sats)')} {pending.btc.fee}</Text>
          <TextInput style={styles.field} value={replacementFee} onChangeText={setReplacementFee}
            editable={!busy} accessibilityLabel={t('새 전체 수수료 (sats)')} placeholder={t('새 전체 수수료 (sats)')}
            maxLength={6} keyboardType="number-pad" />
          <Button text={t('개인 기기 찾기')} disabled={busy} run={scan} />
          {found.map(item => <Button key={'replace-' + item.address} text={t('교체 원문 검토') + ' · ' + item.name}
            disabled={busy || !replacementFee} run={() => prepare(item.address,pending)} />)}
        </>}

      {result && <Text style={styles.label}>{t({pending: '아직 네트워크에서 확인되지 않습니다.',
        included: '블록에 포함되었습니다.', finalized: '선택한 네트워크에서 확정되었습니다.',
        failed: '선택한 네트워크에서 실패가 확정되었습니다.', expired: '거래 유효 기간이 지났습니다.'}[result])}</Text>}
      {!pending.attempted && !!pending.clock && (result === undefined || result === 'pending') &&
        <Button text={t('이 네트워크에 전송')} disabled={busy} run={() => track('submit')} />}
      <Button text={t('전송 상태 재조회')} disabled={busy} run={() => track('query')} />
      {(result === 'finalized' || result === 'failed' || result === 'expired') &&
        <Button text={t('거래 기록 정리')} disabled={busy} run={() => work(async () => {
          await save();
          setResult(undefined);
        }, t('거래 기록을 저장하지 못했습니다.'))} />}
    </>}
    <Button text={t('등록 상태로 돌아가기')} disabled={busy} run={back} />
  </View>;
}

const styles = StyleSheet.create({
  button: {backgroundColor: '#19446b', borderRadius: 7, margin: 4, padding: 11},
  disabled: {opacity: 0.55},
  buttonLabel: {color: '#fff', fontWeight: '600'},
  field: {borderColor: '#71839a', borderWidth: 1, backgroundColor: '#fff', color: '#0b1730', margin: 4, padding: 9},
  label: {color: '#0b1730', padding: 7},
  row: {flexDirection: 'row', flexWrap: 'wrap'},
  qr: {alignSelf: 'center', height: 220, margin: 8, width: 220},
});
