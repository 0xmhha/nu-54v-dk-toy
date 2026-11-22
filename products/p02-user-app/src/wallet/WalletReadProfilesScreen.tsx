import {WalletNonEvmAssets} from './WalletNonEvmAssets.tsx';
import {nonEvmAssetKey, scopedNonEvmAssets} from './nonEvmAssetCatalog.ts';
import {unknownConnectivity, type ReadProof, type WalletConnectivity} from './walletConnectivity.ts';
import {unsupportedWalletMethods, walletCapabilities} from './walletCapabilities.ts';
import {accountPreference, assetsForAccount, registerReviewedAsset, reviewEvmAsset, saveAccountPreference, type ReviewedAsset} from './walletCatalog.ts';
import {evmProfiles, storedAllowanceChange, verifyPendingOriginal, verifyStoredEvmWire} from './evmTransfer.ts';
import {publicSignetIndexer, signetIndexer, verifyPublicSignet} from './signetIndexer.ts';
import {WalletCustomNetworkScreen} from './WalletCustomNetworkScreen.tsx';
import {WalletEducationRateCard} from './WalletEducationRateCard.tsx';
import {assetSearchMatches, educationRate, stableTestUsdc} from './walletEducationRate.ts';
import {useCallback, useEffect, useRef, useState} from 'react';
import {AppState, InputAccessoryView, Keyboard, Platform, Pressable, Share, StyleSheet, Text, TextInput, View} from 'react-native';
import {presentReceive, receiveQr} from './receivePresentation.ts';
import {Image} from 'react-native';
import {hash160,segwitAddress,type ChainPublicAccount} from './chainAccounts.ts';
import type {OwnerSession} from './ownerSession.ts';
import type {PendingEvmTransfer, PersonalMetadata} from './profile.ts';
import {BoardStepPanel} from './BoardStepPanel.tsx';
import {discoverSignetBranches, formatReadAmount, isXrpTag, observationScope, readContact, readEndpoint, readProfiles, readSelectedBalance, receiveAddressForProfile, updateReadObservation, verifyReadNetwork, walletReadRpc,
  type ReadProfileId, type ReadProfileState, type ReadSelection} from './readProfiles.ts';

type Props = {
  verified: PersonalMetadata;
  language?: 'ko' | 'en';
  pendingEvm?: PendingEvmTransfer;
  onOpenPending?: () => void;
  connectivity?: WalletConnectivity;
  onReadProof?: (proof: ReadProof | undefined) => void;
  onWatchSignet?: (account:number)=>void;
  reviewableNetworks?:readonly ReadProfileId[];
  onReviewNetwork?: (id: ReadProfileId) => void;
  value: ReadProfileState;
  save: (next: ReadProfileState) => Promise<void>;
  found: {address: string; name: string}[];
  scan: () => void;
  back: () => void;
  busy: boolean;
  work: (operation: () => Promise<void>, failure: string | ((error: unknown) => string)) => void;
  session: <T>(handle: string, action: (session: OwnerSession) => Promise<T>) => Promise<T>;
  rpc: (url: string, packet: string) => Promise<string>;
  t: (message: string) => string;
};

function Button({text, disabled, click}: {text: string; disabled?: boolean; click: () => void}) {
  return <Pressable accessibilityRole="button" accessibilityLabel={text} accessibilityState={{disabled: !!disabled}}
    style={[styles.button, disabled && styles.disabled]} disabled={disabled} onPress={click}>
    <Text style={styles.buttonText}>{text}</Text>
  </Pressable>;
}

export function WalletReadProfilesScreen({verified, value, save, found, scan, back, busy, work, session, rpc, t, language = 'ko', pendingEvm, onOpenPending, connectivity = unknownConnectivity, onReadProof, onReviewNetwork, onWatchSignet,reviewableNetworks}: Props) {
  const [custom, setCustom] = useState(false);
  const [capability, setCapability] = useState(false);
  const [unsupported, setUnsupported] = useState<(typeof unsupportedWalletMethods)[number]>();
  const [switchTarget, setSwitchTarget] = useState<ReadProfileId>();
  const [endpointCheck, setEndpointCheck] = useState<'checking' | 'error' | 'passed'>();
  const [candidate, setCandidate] = useState(value.endpoints[value.selection.id] ?? '');
  const [asset, setAsset] = useState(value.selection.asset ?? '');
  const [expectedDecimals, setExpectedDecimals] = useState('');
  const [assetReview, setAssetReview] = useState<ReviewedAsset>();
  const [accountName, setAccountName] = useState('');
  const [assetFilter, setAssetFilter] = useState<'all' | 'favorites' | 'hidden'>('all');
  const [assetSearch, setAssetSearch] = useState('');
  const [name, setName] = useState('');
  const [contactTag, setContactTag] = useState('');
  const [tag, setTag] = useState(value.selection.tag ?? '');
  const [address, setAddress] = useState('');
  const discovery = useRef<AbortController | undefined>(undefined);
  const [discovering,setDiscovering]=useState<'approval'|'addresses'>();
  const [discoveryResult,setDiscoveryResult]=useState<{scope:string;endpoint:string;proof:Awaited<ReturnType<typeof discoverSignetBranches>>}>();
  const [verifiedReceive, setVerifiedReceive] = useState<{scope: string; address: string}>();
  const generation = useRef({revision:0});
  const watch = useRef<{scope: string; endpoint: string; account: ChainPublicAccount | string} | undefined>(undefined);
  useEffect(() => {
    const lifecycle=generation.current;
    const background = AppState.addEventListener('change', state => {
      if (state === 'active') return;
      lifecycle.revision++;
      watch.current = undefined;
      discovery.current?.abort();
      setDiscoveryResult(undefined);
      setDiscovering(undefined);
      setFresh(undefined);
      setVerifiedReceive(undefined);
      setAssetReview(undefined);
    });
    return () => {
      lifecycle.revision++;
      watch.current = undefined;
      discovery.current?.abort();
      background.remove();
    };
  }, []);
  const [fresh, setFresh] = useState<{scope: string; time: number}>();
  const selection = value.selection;
  const profile = readProfiles[selection.id];
  const accountRow = accountPreference(value.accounts, verified, selection);
  const hiddenAccounts=(value.accounts??[]).filter(row=>row.hidden && row.board===verified.boardFingerprint &&
    row.generation===verified.generation && row.id===selection.id);
  const assets = assetsForAccount(value.assets, verified, selection);
  const nativeAsset = !selection.asset || /^0x0{40}$/.test(selection.asset);
  const selectedToken = assets.find(item => item.contract === selection.asset && !item.hidden);
  const nonEvmToken = scopedNonEvmAssets(value.nonEvmAssets,verified,selection)
    .find(item => nonEvmAssetKey(item) === selection.asset && !item.hidden);
  const assetReady = nativeAsset || !!selectedToken || !!nonEvmToken;
  const assetName = (contract:string, decimals:number) =>
    selection.id === 'stable' && contract === stableTestUsdc && decimals === 6 ? 'TestUSDC' : 'ERC-20';
  const shownAssets = assets.filter(row => (assetFilter === 'hidden' ? row.hidden :
    !row.hidden && (assetFilter !== 'favorites' || row.favorite)) &&
    assetSearchMatches(assetSearch, row.contract, assetName(row.contract,row.decimals), profile.label));
  const reviewed = assetReview?.board === verified.boardFingerprint && assetReview.generation === verified.generation &&
    assetReview.account === selection.account && assetReview.id === selection.id &&
    assetReview.endpoint === value.endpoints[selection.id] ? assetReview : undefined;
  const enabled = !value.disabled?.includes(selection.id);
  const pendingAsset = (() => {
    if (!enabled || accountRow?.hidden || selection.hidden || selection.account !== 0 || !('evm' in profile) ||
        !pendingEvm || pendingEvm.chain !== profile.evm || pendingEvm.genesis !== profile.anchor || pendingEvm.sender !== verified.address ||
        pendingEvm.evidence?.board !== verified.boardFingerprint || pendingEvm.evidence.generation !== verified.generation) return;
    try {
      if (storedAllowanceChange(pendingEvm.raw)) return;
      verifyPendingOriginal(pendingEvm.raw,pendingEvm.hash,pendingEvm.chain,pendingEvm.sender,pendingEvm.evidence.unsigned);
      return verifyStoredEvmWire(pendingEvm.raw,pendingEvm.hash,pendingEvm.chain);
    } catch {return;}
  })();
  useEffect(() => {
    setAccountName('');
    setAssetReview(undefined);
    setSwitchTarget(undefined);
  }, [verified.boardFingerprint, verified.generation]);
  const scope = observationScope(verified.boardFingerprint, verified.generation, selection);
  const endpoint = value.endpoints[selection.id];
  useEffect(() => {
    const lifecycle=generation.current;
    lifecycle.revision++;
    watch.current = undefined;
    discovery.current?.abort();
    setDiscoveryResult(undefined);
    setDiscovering(undefined);
    setFresh(undefined);
    return () => {lifecycle.revision++;};
  }, [scope, endpoint, enabled, assetReady, connectivity.revision]);
  const directory = discoveryResult?.scope===scope && discoveryResult.endpoint===endpoint ? discoveryResult.proof : undefined;
  const snapshot = enabled && assetReady && !accountRow?.hidden ? value.observations.find(item => item.scope === scope && (!('evm' in profile) || item.address === verified.address) &&
    (!selectedToken || item.decimals === selectedToken.decimals) &&
    (nonEvmToken?.id !== 'sol' || item.decimals === nonEvmToken.decimals)) : undefined;
  const failure = useCallback((error: unknown) => t(error instanceof Error && error.message === 'EVM_RPC_GENESIS_UNAVAILABLE' ?
    'RPC에서 제네시스 블록을 조회할 수 없습니다. 네트워크 설정에서 대체 RPC를 직접 확인하세요.' :
    '확정 입금 잔액을 확인하지 못했습니다.'), [t]);
  const update = (next: ReadSelection) => work(async () => {
    if (observationScope(verified.boardFingerprint, verified.generation, next) !== scope) watch.current = undefined;
    await save({...value, selection: next});
    setCandidate(value.endpoints[next.id] ?? '');
    setAsset(next.asset ?? '');
    setAccountName('');
    setAssetReview(undefined);
    setExpectedDecimals('');
    setAssetFilter('all');
    setAssetSearch('');
    setEndpointCheck(undefined);
    setSwitchTarget(undefined);
    setName('');
    setAddress('');
    setContactTag('');
    setTag(next.tag ?? '');
    setFresh(undefined);
  }, t('계정 선택을 저장하지 못했습니다.'));

  function verifyEndpoint() {
    work(async () => {
      setEndpointCheck('checking');
      try {
        const endpoint = readEndpoint(candidate.trim());
        if (selection.id === 'btc' && endpoint === publicSignetIndexer) {
          await verifyPublicSignet(signetIndexer(rpc), readProfiles.btc.anchor);
        } else await verifyReadNetwork(walletReadRpc(endpoint, rpc), selection.id);
        await save({...value, endpoints: {...value.endpoints, [selection.id]: endpoint},
          observations: value.observations.filter(row => row.scope.split('/')[2] !== selection.id)});
        setFresh(undefined);
        watch.current = undefined;
        setAssetReview(undefined);
        setEndpointCheck('passed');
      } catch (error) {
        setEndpointCheck('error');
        throw error;
      }
    }, failure);
  }

  async function boardAccount(handle: string) {
    return session<string | ChainPublicAccount>(handle, active => 'evm' in profile ?
      active.verifiedEvmAddress(verified.address, BigInt(verified.generation)) :
      active.verifiedChainAccount({chain: profile.chain, network: 1, account: selection.account},
        verified.address, BigInt(verified.generation), 0, selection.index ?? 0));
  }

  function verifyReceive(handle: string) {
    work(async () => {
      const operation = generation.current.revision;
      const account = await boardAccount(handle);
      if (operation !== generation.current.revision || AppState.currentState === 'background') return;
      setVerifiedReceive({scope, address: receiveAddressForProfile(selection, account)});
    }, t('보드가 수신 주소를 확인하지 못했습니다.'));
  }

  function refresh(handle: string) {
    work(async () => {
      if (!endpoint || !enabled || !assetReady || connectivity.link === 'offline')
        throw new Error('WALLET_READ_RPC_REQUIRED');
      const operation = ++generation.current.revision;
      const isCurrent = () => operation === generation.current.revision && AppState.currentState !== 'background';
      const checkpoint = value.observations.filter(row => row.scope !== scope);
      setFresh(undefined);
      onReadProof?.(undefined);
      try {
        const publicAccount = await boardAccount(handle);
        if (!isCurrent()) return;
        setVerifiedReceive({scope, address: receiveAddressForProfile(selection, publicAccount)});
        const result = await readSelectedBalance(walletReadRpc(endpoint, rpc), selection, publicAccount,
          selection.id === 'btc' && endpoint === publicSignetIndexer ? signetIndexer(rpc) : undefined, nonEvmToken);
        if (selectedToken && result.decimals !== selectedToken.decimals) throw new Error('WALLET_ASSET_DECIMALS');
        const time = Date.now();
        if (!isCurrent()) return;
        await save({...value, observations: [{...result, scope, time}, ...checkpoint].slice(0, 24)});
        if (!isCurrent()) return;
        watch.current = {scope, endpoint, account: publicAccount};
        onReadProof?.({scope, endpoint, time, revision:connectivity.revision});
        setFresh({scope, time});
      } catch (error) {
        if (!isCurrent()) return;
        watch.current = undefined;
        throw error;
      }
    }, failure);
  }

  useEffect(() => {
    if (!selection.automatic || !enabled || !assetReady || custom || connectivity.link !== 'online') return;
    const handle = setInterval(() => {
      const current = watch.current;
      if (!current || current.scope !== scope || current.endpoint !== value.endpoints[selection.id] ||
          busy || AppState.currentState !== 'active') return;
      work(async () => {
        const operation = generation.current.revision;
        const isCurrent = () => operation === generation.current.revision && watch.current === current && AppState.currentState === 'active';
        try {
          const observed = await readSelectedBalance(walletReadRpc(current.endpoint, rpc), selection, current.account,
            selection.id === 'btc' && current.endpoint === publicSignetIndexer ? signetIndexer(rpc) : undefined, nonEvmToken);
          if (selectedToken && observed.decimals !== selectedToken.decimals) throw new Error('WALLET_ASSET_DECIMALS');
          if (!isCurrent()) return;
          const time = Date.now();
          const next = updateReadObservation(value.observations, {...observed, scope, time});
          if (next === value.observations) return;
          await save({...value, observations: next});
          if (!isCurrent()) return;
          onReadProof?.({scope, endpoint:current.endpoint, time, revision:connectivity.revision});
          setFresh({scope, time});
        } catch (error) {
          if (!isCurrent()) return;
          watch.current = undefined;
          setFresh(undefined);
          onReadProof?.(undefined);
          throw error;
        }
      }, failure);
    }, 30000);
    return () => clearInterval(handle);
  }, [selection, scope, value, busy, rpc, save, work, failure, enabled, assetReady, selectedToken, nonEvmToken, custom, connectivity.link, connectivity.revision, onReadProof]);

  function discoverBitcoin(handle: string) {
    work(async () => {
      const targetEndpoint=value.endpoints.btc;
      if (!targetEndpoint || targetEndpoint===publicSignetIndexer || selection.id!=='btc' || connectivity.link==='offline')
        throw new Error('BTC_DISCOVERY_RPC');
      const operation=generation.current.revision;
      setDiscoveryResult(undefined);
      const controller = new AbortController();
      discovery.current = controller;
      try {
        const watchProfile={chain:1 as const,network:2 as const,account:selection.account};
        const current=()=>!controller.signal.aborted && generation.current.revision===operation &&
          AppState.currentState!=='background';
        const watchAccount=await session(handle,active=>active.exportBitcoinWatchAccount(watchProfile,verified.address,
          BigInt(verified.generation),()=>{if (current()) setDiscovering('approval');}));
        if (!current()) return;
        setDiscovering('addresses');
        const proof=await discoverSignetBranches(walletReadRpc(targetEndpoint,rpc),selection.account,async(branch,index)=>{
          const publicKey=watchAccount.publicChild(branch,index);
          return {...watchProfile,publicKey,address:segwitAddress('tb',Uint8Array.of(0,20,...hash160(publicKey)))};
        },controller.signal);
        if (controller.signal.aborted || generation.current.revision!==operation || AppState.currentState==='background') return;
        setDiscoveryResult({scope,endpoint:targetEndpoint,proof});
      } finally {
        if (discovery.current===controller) {discovery.current=undefined;setDiscovering(undefined);}
      }
    }, error => t(error instanceof Error && error.message === 'BTC_DISCOVERY_CANCELLED' ?
      '주소 재발견을 중단했습니다.' : '검증된 주소 재발견을 마치지 못했습니다.'));
  }

  function addContact() {
    work(async () => {
      const entry = readContact(selection.id, name, address, contactTag || undefined);
      const previous = value.contacts.filter(item => item.id !== entry.id || item.address !== entry.address || item.tag !== entry.tag);
      if (previous.length >= 30) throw new Error('WALLET_CONTACT_CAPACITY');
      await save({...value, contacts: [...previous, entry]});
      setName('');
      setAddress('');
    }, t('주소록 항목을 저장하지 못했습니다.'));
  }

  function reviewAsset() {
    work(async () => {
      setAssetReview(undefined);
      const endpoint = value.endpoints[selection.id];
      if (!endpoint || !enabled) throw new Error('WALLET_ASSET_SCOPE');
      if (expectedDecimals && !/^(?:[0-9]|[12][0-9]|3[0-6])$/.test(expectedDecimals))
        throw new Error('WALLET_ASSET_DECIMALS');
      const metadata = await reviewEvmAsset(walletReadRpc(endpoint,rpc), verified, selection, endpoint, asset,
        expectedDecimals ? Number(expectedDecimals) : undefined);
      if (AppState.currentState !== 'background') setAssetReview(metadata);
    }, error => t(error instanceof Error && error.message === 'WALLET_ASSET_DECIMALS' ?
      '검증한 토큰의 소수 자릿수가 입력값과 다릅니다.' : '선택한 네트워크에서 토큰을 검증하지 못했습니다.'));
  }

  const currentAddress = enabled && verifiedReceive?.scope === scope ? verifiedReceive.address : undefined;
  const card = currentAddress && assetReady && !accountRow?.hidden ? presentReceive(
    'evm' in profile ? {kind:'evm', network:profile.label, chainId:profile.evm, symbol:evmProfiles[profile.evm].native,
      token:selection.asset === '0x' + '0'.repeat(40) ? undefined : selection.asset} :
    profile.chain === 1 ? {kind:'btc', network:profile.label, hrp:'tb'} :
    profile.chain === 2 ? {kind:'sol', network:profile.label, mint:nonEvmToken?.id === 'sol' ? nonEvmToken.mint : undefined} :
    {kind:'xrp', network:profile.label, test:true, tag:selection.tag,
      issue:nonEvmToken?.id === 'xrp' ? {currency:nonEvmToken.currency,issuer:nonEvmToken.issuer} : undefined}, currentAddress) : undefined;
  if (capability) return <View>
    <Text accessibilityRole="header" style={styles.label}>{t('체인별 기능표')}</Text>
    {(Object.keys(readProfiles) as ReadProfileId[]).map(id => <View key={id}>
      <Text accessibilityRole="header" style={styles.label}>{readProfiles[id].label}</Text>
      <Text style={styles.label}>{t(walletCapabilities[id].fee)} · {t(walletCapabilities[id].read)}</Text>
      <Text style={styles.label}>{t(walletCapabilities[id].approvals)}</Text>
      {onReviewNetwork && <>
        {reviewableNetworks && !reviewableNetworks.includes(id) && <Text style={styles.label}>{t('등록 프로필이 필요합니다.')}</Text>}
        <Button text={t('체인별 거래 원문 확인') + ' · ' + readProfiles[id].label}
          disabled={busy || !!value.disabled?.includes(id) || !!reviewableNetworks && !reviewableNetworks.includes(id)}
          click={() => onReviewNetwork(id)} />
      </>}

    </View>)}
    {unsupportedWalletMethods.map(method => <Button key={method.id} text={t('지원 여부 확인') + ' · ' + method.name} disabled={busy}
      click={() => setUnsupported(method)} />)}
    {unsupported && <Text accessibilityLiveRegion="polite" style={styles.label}>
      {t('지원되지 않는 요청')} · {unsupported.name} · {t(unsupported.description)} · {t('서명 0건 · 제출 0건')}
    </Text>}
    <Button text={t('네트워크 선택으로 돌아가기')} disabled={busy} click={() => {setCapability(false); setUnsupported(undefined);}} />
  </View>;
  if (custom) return <WalletCustomNetworkScreen profiles={value.custom ?? []} save={list => save({...value, custom: list})}
    readAddress={handle => session(handle, active => active.verifiedEvmAddress(verified.address, BigInt(verified.generation)))}
    found={found} discover={scan} exchange={rpc} work={work} busy={busy} back={() => setCustom(false)} t={t} />;

  return <View>
    <Text accessibilityRole="header" style={styles.label}>{t('6개 체인 · 읽기 전용')}</Text>
    <View style={styles.row}>{(Object.entries(readProfiles) as [ReadProfileId, typeof profile][]).map(([id, network]) =>
      <Button key={id} text={network.label} disabled={busy || selection.id === id}
        click={() => setSwitchTarget(id)} />)}</View>
    {switchTarget && <View>
      <Text accessibilityRole="header" style={styles.label}>{t('활성 조회 네트워크 전환')}</Text>
      <Text style={styles.label}>{readProfiles[switchTarget].label} · {t('기존 미결 거래는 원래 네트워크에서 조회합니다.')}</Text>
      <Button text={t('조회 네트워크 전환 동의')} disabled={busy} click={() => {
        setVerifiedReceive(undefined);
        update({id:switchTarget,account:0,hidden:selection.hidden});
      }} />
      <Button text={t('조회 네트워크 전환 취소')} disabled={busy} click={() => setSwitchTarget(undefined)} />
    </View>}
    <Button text={t('체인별 기능표')} disabled={busy} click={() => {watch.current = undefined; setCapability(true);}} />
    <View style={styles.row}>
      <Button text={enabled ? t('이 내장 망 끄기') : t('이 내장 망 켜기')} disabled={busy}
        click={() => work(async () => {
          watch.current = undefined;
          setFresh(undefined);
          const rest = (value.disabled ?? []).filter(id => id !== selection.id);
          await save({...value, disabled: enabled ? [...rest, selection.id] : rest});
        }, t('계정 선택을 저장하지 못했습니다.'))} />
      <Button text={t('사용자 지정 EVM 망 · 읽기 전용')} disabled={busy}
        click={() => {watch.current = undefined; setCustom(true);}} />
    </View>
    <Text selectable style={styles.label}>{profile.label} · {profile.anchor}{'ledger' in profile ? '\n' + profile.ledger : ''}</Text>
    {'chain' in profile && <View style={styles.row}>
      <Button text={t('이전 계정')} disabled={busy || selection.account === 0}
        click={() => update({...selection, account: selection.account - 1, index: undefined})} />
      <Text style={styles.label}>{t('계정')} {selection.account + 1} · {t('파생 인덱스')} {selection.account}</Text>
      <Button text={t('다음 계정')} disabled={busy || selection.account === 99}
        click={() => update({...selection, account: selection.account + 1, index: undefined})} />
    </View>}
    {hiddenAccounts.length>0 && <View>
      <Text accessibilityRole="header" style={styles.label}>{t('숨긴 계정')}</Text>
      {hiddenAccounts.map(row=><Button key={row.account} text={t('숨긴 계정 선택')+' · '+(row.name ?? t('계정'))+' · '+(row.account+1)}
        disabled={busy || row.account===selection.account}
        click={()=>update({...selection,account:row.account,index:undefined})} />)}
    </View>}
    <Text style={styles.label}>{accountRow?.name ?? t('계정') + ' ' + (selection.account + 1)}
      {accountRow?.hidden ? ' · ' + t('숨김') : ''}</Text>
    <TextInput value={accountName} onChangeText={setAccountName} maxLength={32} editable={!busy}
      placeholder={accountRow?.name} accessibilityLabel={t('계정 이름')} style={styles.input} />
    {onWatchSignet && selection.id==='btc' && <Button text={t('Signet 읽기 전용 계정 내보내기')}
      disabled={busy || !enabled || !!accountRow?.hidden} click={()=>onWatchSignet(selection.account)} />}
    <Button text={t('검증한 계정 이름 저장')} disabled={busy || !currentAddress || !accountName.trim()} click={() => work(async () => {
      await save({...value,accounts:saveAccountPreference(value.accounts,verified,selection,accountName,accountRow?.hidden ?? false)});
      setAccountName('');
    }, t('계정 이름을 저장하지 못했습니다.'))} />
    {accountRow?.name && <Button text={t('계정 이름 삭제')} disabled={busy || !currentAddress}
      click={()=>work(async()=>{
        await save({...value,accounts:saveAccountPreference(value.accounts,verified,selection,undefined,accountRow.hidden)});
        setAccountName('');
      },t('계정 이름을 삭제하지 못했습니다.'))} />}
    {accountRow && <Button text={accountRow.hidden ? t('숨긴 계정 표시') : t('이름 붙인 계정 숨기기')}
      disabled={busy || !currentAddress} click={() => work(async () => {
        await save({...value,accounts:saveAccountPreference(value.accounts,verified,selection,accountRow.name,!accountRow.hidden)});
      }, t('계정 표시를 저장하지 못했습니다.'))} />}
    {'evm' in profile && <>
      {!assetReady && <Text style={styles.label}>{t('이전 토큰을 공유하거나 조회하려면 메타데이터를 다시 검증하세요.')}</Text>}
      <TextInput value={asset} onChangeText={input => {setAsset(input); setAssetReview(undefined);}} autoCapitalize="none"
        editable={!busy} maxLength={42} accessibilityLabel={t('읽을 토큰 주소')} style={styles.input} />
      <TextInput value={expectedDecimals} onChangeText={input => {setExpectedDecimals(input); setAssetReview(undefined);}}
        keyboardType="number-pad" maxLength={2} editable={!busy} placeholder={t('알고 있는 소수 자릿수 (선택)')}
        accessibilityLabel={t('예상 소수 자릿수')} style={styles.input} />
      <Button text={t('토큰 정보 검토')} disabled={busy || !enabled || !asset.trim() || !value.endpoints[selection.id]} click={reviewAsset} />
      <Button text={t('기본 자산 보기')} disabled={busy || !selection.asset} click={() => update({...selection,asset:undefined})} />
      {reviewed && <>
        <Text selectable style={styles.label}>{profile.label} · {reviewed.contract} · {t('소수 자릿수')} {reviewed.decimals}
          {'\n'}{t('확정 블록')} {reviewed.height} · {reviewed.block}</Text>
        <Button text={t('검증된 토큰 추가')} disabled={busy || !enabled} click={() => work(async () => {
          await save(registerReviewedAsset(value,verified,reviewed));
          watch.current = undefined;
          setFresh(undefined);
          setAssetReview(undefined);
        }, t('검증한 토큰을 저장하지 못했습니다.'))} />
        <Button text={t('토큰 검토 취소')} disabled={busy} click={() => setAssetReview(undefined)} />
      </>}
      <View style={styles.row}>
        <Button text={t('등록 토큰')} disabled={busy || assetFilter === 'all'} click={() => setAssetFilter('all')} />
        <Button text={t('즐겨찾기')} disabled={busy || assetFilter === 'favorites'} click={() => setAssetFilter('favorites')} />
        <Button text={t('숨긴 자산')} disabled={busy || assetFilter === 'hidden'} click={() => setAssetFilter('hidden')} />
      </View>
      <TextInput accessibilityLabel={language === 'ko' ? '등록 자산 검색' : 'Search registered assets'} value={assetSearch} maxLength={64}
        style={styles.input} autoCapitalize="none" autoCorrect={false} onChangeText={setAssetSearch} />
      {assetSearch.trim() && !shownAssets.length ? <Text style={styles.label}>{language === 'ko' ? '일치하는 등록 자산이 없습니다.' : 'No matching registered assets.'}</Text> :
        assetFilter === 'favorites' && !shownAssets.length && <Text style={styles.label}>{t('즐겨찾는 자산이 없습니다')}</Text>}
      {shownAssets.map((item,index) => {
        const target = {...selection,asset:item.contract};
        const assetScope = observationScope(verified.boardFingerprint,verified.generation,target);
        const mayInspect = !item.hidden || assetFilter === 'hidden';
        const cached = enabled && !accountRow?.hidden && !selection.hidden && mayInspect ? value.observations.find(row =>
          row.scope === assetScope && row.address === verified.address && row.decimals === item.decimals) : undefined;
        const displayAsset = assetName(item.contract,item.decimals);
        const rate = educationRate(value,verified,target);
        return <View key={item.contract}>
        <Text selectable style={styles.label}>{displayAsset} · {profile.label} · {item.contract} · {t('소수 자릿수')} {item.decimals}</Text>
        {cached?.amount !== undefined && <Text style={styles.label}>{formatReadAmount(cached.amount,item.decimals)} {displayAsset} · {cached.point}</Text>}
        {mayInspect && pendingAsset?.asset === item.contract && <>
          <Text selectable style={styles.label}>
          {language === 'ko' ? '미결 송금' : 'Pending transfer'} · {formatReadAmount(pendingAsset.amount.toString(),item.decimals)} {displayAsset} · {pendingEvm?.hash}
          </Text>
          {onOpenPending && <Button text={language === 'ko' ? '기존 송금 조회' : 'Open original transfer'} disabled={busy} click={onOpenPending} />}
        </>}
        {rate && <WalletEducationRateCard key={rate.scope} rate={rate} stale={connectivity.link !== 'online'} language={language} />}
        <Button text={t('조회 토큰 선택') + ' ' + (index + 1)} disabled={busy || item.hidden || !enabled}
          click={() => update({...selection,asset:item.contract})} />
        <Button text={t(item.favorite ? '즐겨찾기 해제' : '즐겨찾기에 추가') + ' ' + (index + 1)} disabled={busy || item.hidden}
          click={() => work(async () => {
            await save({...value,assets:value.assets?.map(row => row === item ? {...row,favorite:!row.favorite} : row)});
          }, t('즐겨찾기를 저장하지 못했습니다.'))} />
        <Button text={t(item.hidden ? '목록에 다시 표시' : '목록에서 숨기기') + ' ' + (index + 1)} disabled={busy}
          click={() => work(async () => {
            const hideCurrent = !item.hidden && selection.asset === item.contract;
            if (hideCurrent) watch.current = undefined;
            await save({...value,assets:value.assets?.map(row => row === item ? {...row,hidden:!row.hidden} : row),
              selection:hideCurrent ? {...selection,asset:undefined} : selection});
          }, t('자산 표시를 저장하지 못했습니다.'))} />
      </View>;})}
    </>}
    {(selection.id === 'sol' || selection.id === 'xrp') && <WalletNonEvmAssets
      key={[verified.boardFingerprint,verified.generation,selection.id,selection.account,endpoint,connectivity.revision].join('/')}
      owner={verified} value={value} busy={busy} enabled={enabled && connectivity.link !== 'offline'} rpc={rpc} work={work} save={save} t={t} language={language} />}
    {selection.id === 'xrp' && <>
      <TextInput value={tag} onChangeText={setTag} keyboardType="number-pad" editable={!busy} maxLength={10}
        inputAccessoryViewID={Platform.OS === 'ios' ? 'wallet-receive-tag' : undefined}
        accessibilityLabel={t('목적지 태그')} placeholder={t('목적지 태그 (선택)')} style={styles.input} />
      {Platform.OS === 'ios' && <InputAccessoryView nativeID="wallet-receive-tag">
        <Button text={language === 'ko' ? '태그 입력 완료' : 'Done entering tag'} click={Keyboard.dismiss} />
      </InputAccessoryView>}
      <Button text={t('수신 태그 저장')} disabled={busy || (!!tag && !isXrpTag(tag))}
        click={() => update({...selection, tag: tag || undefined})} />
    </>}
    {selection.id === 'btc' && <>
      <Text style={styles.label}>{t('공개 Signet 인덱서로 조회하면 현재 수신 주소 한 개가 mempool.space에 전달됩니다. 잔액과 사용 이력은 인덱서가 보고합니다.')}</Text>
      <Button text={t('공개 Signet 인덱서 사용 준비')} disabled={busy || !enabled}
        click={() => {setCandidate(publicSignetIndexer); setEndpointCheck(undefined);}} />
    </>}
    <TextInput value={candidate} onChangeText={next => {setCandidate(next); setEndpointCheck(undefined);}} autoCapitalize="none" maxLength={512}
      editable={!busy} style={styles.input} accessibilityLabel={t('검증된 HTTPS RPC 주소')} />
    <Button text={t('지문 확인 후 읽기 전용 RPC 저장')} disabled={busy || !enabled || !candidate.trim()} click={verifyEndpoint} />
    {endpointCheck && <Text accessibilityLiveRegion="polite" style={styles.label}>{t(endpointCheck === 'checking' ?
      '기존 설정을 유지하며 새 서버의 지문을 확인하는 중입니다.' : endpointCheck === 'passed' ?
        '네트워크 지문을 확인하고 서버를 저장했습니다.' : '서버를 등록하지 못했습니다. 기존 네트워크 설정을 유지합니다.')}</Text>}
    <Button text={t('개인 기기 찾기')} disabled={busy} click={scan} />
    {found.map(item => <Button key={'receive-' + item.address} text={t('보드와 수신 주소 확인') + ' · ' + item.name}
      disabled={busy || !enabled} click={() => verifyReceive(item.address)} />)}
    {value.endpoints[selection.id] && <Text style={styles.label}>
      {t('입금 잔액을 다시 확인하면 이 주소와 선택한 자산을 저장한 HTTPS 서버에 조회합니다.')}
    </Text>}
    {found.map(item => <Button key={item.address} text={t('입금 잔액 다시 확인') + ' · ' + item.name}
      disabled={busy || !enabled || !assetReady || connectivity.link === 'offline' || !endpoint} click={() => refresh(item.address)} />)}
    {selection.id === 'btc' && <>
      <Text style={styles.label}>{t('현재 수신 주소 인덱스')} {selection.index ?? 0}</Text>
      {endpoint && endpoint!==publicSignetIndexer && <Text style={styles.label}>
        {t('재발견을 시작하면 이 계정의 공개 파생 정보를 기기에서 승인합니다. 저장한 개인 노드는 개별 주소만 조회합니다.')}
      </Text>}
      {found.map(item => <Button key={'discover-' + item.address}
        text={t('사용한 Bitcoin 주소 재발견') + ' · ' + item.name} disabled={busy || !enabled || connectivity.link==='offline' || !value.endpoints.btc || value.endpoints.btc === publicSignetIndexer}
        click={() => discoverBitcoin(item.address)} />)}
      {discovering==='approval' && <BoardStepPanel wait={{flow:'sign',step:'display'}} t={t} />}
      {discovering && <Text accessibilityLiveRegion="polite" style={styles.label}>{t(discovering==='approval'
        ? '독립 화면에서 이 계정의 공개키 범위를 확인한 후 보드에서 승인하세요.'
        : '확정된 수신·거스름돈 이력을 조회하고 있습니다.')}</Text>}
      {discovering && <Button text={t('주소 재발견 취소')} click={() => discovery.current?.abort()} />}
      {directory && <>
        <Text accessibilityLiveRegion="polite" style={styles.label}>{t('사용한 주소와 파생경로 확인')}</Text>
        <Text selectable style={styles.label}>{t('수신 분기')} · {"m/84'/1'/"+directory.account+"'/0/*"} · {' '}
          {directory.receive.lastUsed<0 ? t('확정 사용 기록 없음') : t('마지막 사용 인덱스')+' '+directory.receive.lastUsed}</Text>
        <Text selectable style={styles.label}>{t('거스름돈 분기')} · {"m/84'/1'/"+directory.account+"'/1/*"} · {' '}
          {directory.change.lastUsed<0 ? t('확정 사용 기록 없음') : t('마지막 사용 인덱스')+' '+directory.change.lastUsed}</Text>
        <Text selectable style={styles.label}>{t('검증한 블록')} {directory.head}</Text>
        <Button text={t('다음 수신 주소 선택')+' · '+directory.receive.nextIndex}
          disabled={busy || accountRow?.hidden || directory.receive.nextIndex<=(selection.index??0)}
          click={()=>update({...selection,index:directory.receive.nextIndex})} />
      </>}
    </>}
    <Button text={selection.automatic ? t('확정 입금 자동 조회 끄기') : t('확정 입금 자동 조회 켜기')}
      disabled={busy || !enabled || !assetReady || !value.endpoints[selection.id]} click={() => update({...selection, automatic: !selection.automatic})} />
    {selection.automatic && <Text style={styles.label}>
      {t('보드 확인 후 앱이 열려 있는 동안 선택한 RPC에 이 주소만 조회합니다.')}
    </Text>}
    <Button text={selection.hidden ? t('잔액 표시') : t('잔액 숨기기')} disabled={busy}
      click={() => update({...selection, hidden: !selection.hidden})} />
    {connectivity.link === 'offline' && <Text accessibilityLiveRegion="polite" style={styles.label}>
      {t('오프라인 · 마지막 조회')}
    </Text>}
    {snapshot && <>
      <Text selectable style={styles.label}>{snapshot.address}</Text>
      {!selection.hidden && <Text selectable style={styles.label}>
        {snapshot.inactive ? t('첫 입금 대기 · XRPL 계정 미활성') : snapshot.unconfigured ? t('설정된 발행 자산 신뢰선 없음') :
            (snapshot.issued ?? formatReadAmount(snapshot.amount!, snapshot.decimals)) + ' ' + snapshot.unit}
      </Text>}
      {!selection.hidden && snapshot.used !== undefined && <Text style={styles.label}>{snapshot.used ?
        t('인덱서가 보고한 확정 사용 이력 있음') : t('인덱서가 보고한 확정 사용 이력 없음')}</Text>}
      <Text style={styles.label}>{snapshot.point} · {new Date(snapshot.time).toISOString()} · {
        fresh?.scope === scope && fresh.time === snapshot.time ?
          (snapshot.used !== undefined ? t('방금 확인한 인덱서 조회') : t('방금 확인한 확정 조회')) : t('저장된 이전 조회')}</Text>
    </>}
    {card && <>
      <Text selectable style={styles.label}>{card.detail}</Text>
      <Image accessibilityLabel={t('수신 QR 코드')} source={{uri: receiveQr(card.payload)}} style={styles.qr} />
      <Button text={t('받기 정보 공유')} disabled={busy} click={() => work(async () => {
        await Share.share({message: card.share});
      }, t('받기 정보를 공유하지 못했습니다.'))} />
    </>}
    <Text accessibilityRole="header" style={styles.label}>{t('네트워크별 주소록')}</Text>
    {value.contacts.filter(item => item.id === selection.id).map(item =>
      <Text key={item.address + (item.tag ?? '')} style={styles.label} selectable>
        {item.name} · {item.address}{item.tag !== undefined ? ' · DestinationTag: ' + item.tag : ''}
      </Text>)}
    <TextInput value={name} onChangeText={setName} maxLength={32} editable={!busy}
      accessibilityLabel={t('주소록 이름')} style={styles.input} />
    <TextInput value={address} onChangeText={setAddress} autoCapitalize="none" maxLength={90}
      editable={!busy} accessibilityLabel={t('주소록 주소')} style={styles.input} />
    {selection.id === 'xrp' && <TextInput value={contactTag} onChangeText={setContactTag}
      editable={!busy} keyboardType="number-pad" maxLength={10}
      accessibilityLabel={t('주소록 목적지 태그')} style={styles.input} />}
    <Button text={t('주소록에 저장')} disabled={busy || !name.trim() || !address.trim() || (!!contactTag && !isXrpTag(contactTag))} click={addContact} />
    <Button text={t('등록 상태로 돌아가기')} disabled={busy} click={back} />
  </View>;
}

const styles = StyleSheet.create({
  button: {backgroundColor: '#19446b', borderRadius: 7, margin: 4, padding: 11},
  buttonText: {color: '#fff', fontWeight: '600'},
  disabled: {opacity: 0.55},
  label: {color: '#0b1730', padding: 7},
  input: {borderColor: '#71839a', borderWidth: 1, backgroundColor: '#fff', color: '#0b1730', margin: 4, padding: 9},
  qr: {alignSelf: 'center', height: 220, margin: 8, width: 220},
  row: {flexDirection: 'row', flexWrap: 'wrap'},
});
