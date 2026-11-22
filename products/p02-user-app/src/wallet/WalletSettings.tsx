import React, {useEffect, useRef, useState} from 'react';
import {AccessibilityInfo, AppState, Linking, Platform, Share, StyleSheet, Text, TextInput, View} from 'react-native';
import {WalletAccessibility} from './WalletAccessibility.tsx';
import Alerts from '../specs/NativeSubscriptionAlerts.ts';
import {normalizePolicyRecipients, type OwnerIdleLock, type OwnerLimits} from './ownerLimitsWire.ts';
import {formatReadAmount} from './readProfiles.ts';
import {KitButton, Section, type ButtonVariant} from '../ui/kit.tsx';
import {color, font} from '../ui/theme.ts';
import {stableTestUsdc} from './walletEducationRate.ts';

export function WalletSettings({tabbed=false, backup, replacePhone, firmwareEvidence, assetNetworks, language, setLanguage, back, policyRoute, applied, idleLock, checkIdleLock, practice, pendingOrders, pendingEvm, pendingChain, device, pending, boardStatus, boardSuffix, recover, credentials, networks, privacy, policy, policyDecimals, policyFresh, policyFinalityFresh, policyFinalityPending, finalityEnroll, finalityReconcile, finalityResume, freshAsset, chooseAsset, initialize, change, changeSet, policyContacts, activate, readPolicy, policyReadAt, section, accessibility, diagnostics, rejectPractice, resetAccessibilityScroll}: {
  /** Inside the app tabs the 설정 tab replaces the back-to-wallet button. */
  tabbed?:boolean;
  /** Entries that live in the 설정 tab once the app has tabs: backup, phone replacement, network registry and developer evidence. */
  backup?:()=>void; replacePhone?:()=>void; firmwareEvidence?:()=>void; assetNetworks?:()=>void;
  language:'ko'|'en'; section?:'home'|'access'|'diagnostics'; accessibility?:()=>void; diagnostics?:()=>void; policyRoute?:boolean; applied?:boolean; idleLock?:OwnerIdleLock; checkIdleLock?:()=>void; setLanguage:(language:'ko'|'en')=>void; back:()=>void; practice:()=>void;
  rejectPractice?:()=>void; resetAccessibilityScroll?:()=>void; pendingOrders:()=>void; pendingEvm:()=>void; pendingChain:()=>void; device:()=>void;
  pending?:{evm:boolean;chain:boolean;signing:boolean;evmExpires?:number}; boardStatus?:'active'|'unavailable'; boardSuffix?:string; recover?:()=>void; credentials?:()=>void; networks?:()=>void; privacy?:()=>void; policy?:OwnerLimits|null; policyDecimals?:number; policyReadAt?:number; policyFresh?:boolean; policyFinalityFresh?:boolean; policyFinalityPending?:boolean; finalityEnroll?:()=>void; finalityReconcile?:()=>void; finalityResume?:()=>void; freshAsset?:{chain:string;address:string;decimals:number}; chooseAsset?:()=>void; initialize?:(destination:string,per:string,daily:string)=>void; change?:(destination:string,per:string,daily:string)=>void; activate?:()=>void; readPolicy?:()=>void;
  changeSet?:(change:{recipients:string[]}|{per:string;daily:string})=>void;
  policyContacts?:{name:string;address:string}[];
}) {
  const ko = language === 'ko';
  const [reader,setReader] = useState<boolean>();
  const [motion,setMotion] = useState<boolean>();
  const [preview,setPreview] = useState(false);
  const [policyDestination,setPolicyDestination] = useState('');
  const [policyPer,setPolicyPer] = useState('');
  const [policyDaily,setPolicyDaily] = useState('');
  const [policyReview,setPolicyReview] = useState(false);
  const [setEditor,setSetEditor] = useState<'recipients'|'caps'>();
  const [recipientDraft,setRecipientDraft] = useState<string[]>([]);
  const [recipientInput,setRecipientInput] = useState('');
  const [recipientError,setRecipientError] = useState(false);
  const [failure,setFailure] = useState(false);
  const [notification,setNotification] = useState<boolean>();
  const [requestingNotifications,setRequestingNotifications] = useState(false);
  const notificationRefresh=useRef<(() => void) | undefined>(undefined);
  useEffect(()=>{
    let mounted=true;
    AccessibilityInfo.isScreenReaderEnabled().then(v=>{if(mounted)setReader(v);}).catch(()=>undefined);
    AccessibilityInfo.isReduceMotionEnabled().then(v=>{if(mounted)setMotion(v);}).catch(()=>undefined);
    let permissionRead=0;
    const refreshNotification=()=>{
      const read=++permissionRead;
      Alerts?.permission(false).then(value=>{
        if(mounted && read===permissionRead)setNotification(value);
      }).catch(()=>{
        if(mounted && read===permissionRead)setNotification(undefined);
      });
    };
    notificationRefresh.current=refreshNotification;
    refreshNotification();
    const appListener=AppState.addEventListener('change', state=>{if(state==='active')refreshNotification();});
    const readerListener=AccessibilityInfo.addEventListener('screenReaderChanged',setReader);
    const motionListener=AccessibilityInfo.addEventListener('reduceMotionChanged',setMotion);
    return ()=>{mounted=false;notificationRefresh.current=undefined;appListener.remove();readerListener.remove();motionListener.remove();};
  },[]);
  useEffect(()=>{
    setPolicyReview(false);
    setSetEditor(undefined);
    setRecipientDraft(policy?.recipients??[]);
    setRecipientInput('');
    setRecipientError(false);
    if(policy && policyDecimals!==undefined) {
      setPolicyDestination(policy.destination);
      setPolicyPer(formatReadAmount(policy.perTransfer.toString(),policyDecimals));
      setPolicyDaily(formatReadAmount(policy.daily.toString(),policyDecimals));
    }
  },[policy,policyDecimals]);
  const quantity=(n:bigint)=>policyDecimals===undefined ? n.toString() : formatReadAmount(n.toString(),policyDecimals);
  const policyUnit=policy?.chain===8283n && policy.asset===stableTestUsdc && policyDecimals===6 ? 'TestUSDC' : undefined;
  const amountUnit=policyUnit ?? (policyDecimals===undefined ? (ko?'기본 단위':'base units') : 'ERC-20');
  const amount=(n:bigint)=>quantity(n)+' '+amountUnit;
  const activation=(n:bigint)=>n<=8640000000000n ? new Date(Number(n)*1000).toLocaleString(ko?'ko-KR':'en-US') : (ko?'확인 불가':'Unavailable');
  const diagnostic = 'NU54 · ' + Platform.OS + ' ' + String(Platform.Version);
  const button=(label:string,action:()=>void,variant:ButtonVariant='row')=><KitButton label={label} run={action} variant={variant} />;
  const recipientList=(label:string,entries:string[])=><>
    <Text style={styles.text}>{label}</Text>
    {entries.length ? entries.map(entry=><Text key={entry} selectable style={styles.text}>{entry}</Text>) :
      <Text style={styles.text}>{ko?'허용된 주소가 없습니다.':'No addresses are allowed.'}</Text>}
  </>;
  const addRecipient=(address:string)=>{
    try{
      setRecipientDraft(normalizePolicyRecipients([...recipientDraft,address.trim()]));
      setRecipientInput('');
      setRecipientError(false);
      setPolicyReview(false);
    }catch{setRecipientError(true);}
  };
  const alerts=Alerts;
  const requestNotifications=Platform.OS==='ios' && alerts && notification!==true ? async()=>{
    if(requestingNotifications)return;
    setRequestingNotifications(true);
    setFailure(false);
    try {
      await alerts.permission(true);
    } catch {
      if(notificationRefresh.current)setFailure(true);
    } finally {
      if(notificationRefresh.current){
        setRequestingNotifications(false);
        notificationRefresh.current();
      }
    }
  } : undefined;
  const accessibilityControls=<WalletAccessibility {...{language, reader, motion, notification, setLanguage, practice, pendingOrders, rejectPractice, requestNotifications, requestingNotifications, resetAccessibilityScroll}}
    openDeviceSettings={()=>{Linking.openSettings().catch(()=>setFailure(true));}}/>;
  const diagnosticControls=<>
      <Text accessibilityRole="header" style={styles.heading}>{ko?'도움말·진단':'Help and diagnostics'}</Text>
    {button(ko?'진단 항목 검토':'Review diagnostics',()=>setPreview(true))}
    {preview && <>
      <Text accessibilityRole="header" style={styles.heading}>{ko?'공유할 진단 미리보기':'Diagnostic preview'}</Text>
      <Text selectable style={styles.text}>{diagnostic}</Text>
      <Text style={styles.text}>{ko ? '검증된 펌웨어 버전: 제공되지 않음 · 최근 오류 코드: 제공되지 않음' : 'Verified firmware version: unavailable · Last error code: unavailable'}</Text>
      <Text style={styles.text}>{ko?'주소, 잔액, PIN과 키는 포함하지 않습니다.':'No addresses, balances, PINs or keys are included.'}</Text>
      {button(ko?'검토한 진단 공유':'Share reviewed diagnostics',()=>{Share.share({message:diagnostic}).catch(()=>setFailure(true));})}
      {button(ko?'공유 취소':'Cancel sharing',()=>setPreview(false))}
    </>}
  </>;
  if(section==='access'||section==='diagnostics')return <View style={styles.card}>
    {section==='access' ? accessibilityControls : diagnosticControls}
    {failure && <Text accessibilityLiveRegion="polite" style={styles.text}>{ko?'기기 기능을 열지 못했습니다.':'Could not open the device feature.'}</Text>}
    {button(ko?'설정으로 돌아가기':'Return to Settings',back,'secondary')}
  </View>;
  if(applied && policy)return <View style={styles.card}>
    <Text accessibilityRole="header" style={styles.heading}>{ko?'기기에서 새 활성 정책을 확인했습니다':'New active policy verified on device'}</Text>
    <Text style={styles.text}>{ko?'건별 한도':'Per transfer'}: {amount(policy.perTransfer)} · {ko?'일별 한도':'Daily'}: {amount(policy.daily)}</Text>
    {policy.recipients && <Text style={styles.text}>{ko?'정책 버전':'Policy revision'} {policy.revision.toString()}</Text>}
    {policy.recipients && recipientList(ko?'활성 수취인':'Active recipients',policy.recipients)}
    {button(ko?'설정으로 돌아가기':'Return to Settings',back,'secondary')}
  </View>;
  // First page of the 설정 tab: grouped entries per the P02 IA (내 보드, 보안과 복구, 네트워크와 개인정보, 앱).
  if(section==='home')return <View style={styles.home}>
    <Section title={ko?'내 보드':'My board'} aside={boardStatus === 'active' ? (ko?'상태 확인됨':'Status verified')+(boardSuffix?' · '+boardSuffix:'') : (ko?'상태 미확인':'Status not checked')}>
      {button(ko?'기기·업데이트':'Device and updates',device)}
      {readPolicy && button(ko?'한도와 잠금':'Limits and auto-lock',readPolicy)}
      {checkIdleLock && button(ko?'기기 잠금 규칙 확인':'Verify device auto-lock',checkIdleLock)}
    </Section>
    {(recover || credentials || backup || replacePhone) && <Section title={ko?'보안과 복구':'Security and recovery'}>
      {backup && button(ko?'독립 백업 시작':'Start independent backup',backup)}
      {replacePhone && button(ko?'원래 보드에서 분실한 휴대전화 교체':'Replace a lost phone on the original board',replacePhone)}
      {recover && button(ko?'분실 복구':'Wallet recovery',recover)}
      {credentials && button(ko?'인증수단':'Sign-in methods',credentials)}
    </Section>}
    {(networks || privacy || assetNetworks) && <Section title={ko?'네트워크와 개인정보':'Networks and privacy'}>
      {networks && button(ko?'네트워크':'Networks',networks)}
      {assetNetworks && button(ko?'네트워크 및 자산 설정':'Network and asset settings',assetNetworks)}
      {privacy && button(ko?'개인정보와 여행 동의':'Privacy and travel consent',privacy)}
    </Section>}
    <Section title={ko?'앱':'App'}>
      {accessibility && button(ko?'접근성':'Accessibility',accessibility)}
      {diagnostics && button(ko?'도움말·진단':'Help and diagnostics',diagnostics)}
      {button(ko?'연습 다시 보기':'Replay practice',practice)}
    </Section>
    {firmwareEvidence && <Section title={ko?'개발자 정보':'Developer information'}>
      {button(ko?'펌웨어 적용 증거 확인':'Check firmware evidence',firmwareEvidence)}
    </Section>}
    <Section title={ko?'미결 요청':'Pending requests'}>
      {button(ko?'미결 결제':'Pending payments',pendingOrders)}
      {pending?.evm && button(ko?'EVM 전송 확인':'Review EVM transfer',pendingEvm)}
      {pending?.chain && button(ko?'체인 전송 확인':'Review chain transfer',pendingChain)}
    </Section>
    {pending?.signing && <Text style={styles.text}>{ko?'이전 승인 결과가 불확실합니다. 원래 기기에서 만료를 확인하세요.':'A prior approval is unresolved. Verify expiry on the original device.'}</Text>}
    <Text style={styles.caption}>{ko?'미결 상태는 원래 요청에서 다시 확인합니다. 시계나 만료 시각만으로 완료를 판단하지 않습니다.':'Review pending status at its original source. A clock or expiry alone does not establish completion.'}</Text>
    {failure && <Text accessibilityLiveRegion="polite" style={styles.text}>{ko?'기기 기능을 열지 못했습니다.':'Could not open the device feature.'}</Text>}
    {!tabbed && button(ko?'지갑으로':'Back to wallet',back,'secondary')}
  </View>;
  return <View style={styles.card}>
    <Text accessibilityRole="header" style={styles.heading}>{ko?'기기와 미결 요청':'Device and pending requests'}</Text>
    <Text style={styles.text}>{boardStatus === 'active' ? (ko?'등록 기기 상태 확인됨':'Registered device status verified') : (ko?'등록 기기 상태 미확인':'Registered device status not checked')}</Text>
    {boardSuffix && <Text style={styles.text}>{ko?'인증된 활성 보드 끝자리':'Verified active board ending'} · {boardSuffix}</Text>}
    <Text style={styles.text}>{ko?'저장된 프로필 형식 확인됨. 기기 소유권과 정책은 연결 후 별도로 확인해야 합니다.':'Stored profile format checked. Ownership and policy require separate device verification.'}</Text>
    {policy === null && !policyFresh && !policyFinalityFresh && !policyFinalityPending && <Text style={styles.text}>{ko?'기기가 보호 정책을 제공하지 않습니다. 기존 주소와 이전 서명은 그대로 유지됩니다.':'This device does not provide a protection policy. Existing addresses and prior signatures remain unchanged.'}</Text>}
    {policy && <>
      <Text style={styles.text}>{ko?'기기에서 마지막으로 확인한 정책':'Last policy read from device'}{policyReadAt ? ' · ' + new Date(policyReadAt).toLocaleString(ko?'ko-KR':'en-US') : ''} · {ko?'세대':'Generation'} {policy.generation.toString()} · {ko?'체인':'Chain'} {policy.chain.toString()}{policy.recipients ? ' · '+(ko?'정책 버전':'Policy revision')+' '+policy.revision.toString() : ''}</Text>
      <Text selectable style={styles.text}>{ko?'자산':'Asset'}: {policyUnit ? policyUnit+' · ' : ''}{policy.asset}</Text>
      {policy.recipients ? recipientList(ko?'활성 수취인':'Active recipients',policy.recipients) :
        <Text selectable style={styles.text}>{ko?'허용 주소':'Allowed address'}: {policy.destination}</Text>}
      {!policy.recipients && <Text style={styles.text}>{policy.mode==='finality' ? (ko?'이 8283 정책은 한 주소를 허용하며 현재 변경할 수 없습니다.':'This 8283 policy allows one address and cannot currently be changed.') : (ko?'현재 정책은 한 주소만 허용합니다. 변경하면 대기 후 기존 주소를 교체합니다.':'This policy allows one address; a change replaces it after the waiting period.')}</Text>}
      {policy.mode!=='finality' && <Text style={styles.text}>{ko?'기존 정책은 이 기기와 주소에서 계속 사용합니다. 모든 과거 서명 nonce를 증명할 기록이 없어 새 정산 방식으로 이관하지 않습니다. 기존 한도와 미결 기록은 유지되며, 앱을 다시 연 뒤 같은 기기에서 확인할 수 있습니다.':'This device and address keep their existing policy. Migration is on hold because a complete record of all previously signed nonces is unavailable. Existing limits and pending records remain; reconnect the same device after restarting.'}</Text>}
      <Text style={styles.text}>{policy.mode==='finality' ? (ko?'미결 금액은 매일 한도에 반영됩니다. 신뢰된 발급자의 확정 증거로 정리할 수 있습니다.':'Unresolved amounts count each day. Use evidence from the trusted issuer to reconcile.') : (ko?'확인되지 않은 승인 금액은 다음 날 한도에도 그대로 반영됩니다. 예약 정리 기능이 없어 남은 한도가 부족하거나 16건이 쌓이면 새 서명이 막힙니다.':'Unreconciled approvals count against each new day. Without reconciliation, signing stops if the remaining limit is too small or all 16 reservation slots are full.')}</Text>
      {policy.mode==='finality' && finalityReconcile && button(ko?'확정 증거 파일 선택':'Choose finalized evidence',finalityReconcile)}
      <Text style={styles.text}>{ko?'건별 한도':'Per transfer'}: {amount(policy.perTransfer)} · {ko?'일별 한도':'Daily'}: {amount(policy.daily)} · {policy.mode==='finality' ? (ko?'오늘 사용 또는 예약':'Used or reserved today') : (ko?'예약된 금액':'Reserved')}: {amount(policy.spent)}</Text>
      {policy.pending && <Text accessibilityRole="header" style={styles.heading}>{ko?'보호 정책 변경 대기':'Protection policy change pending'}</Text>}
      {policy.pending && <Text style={styles.text}>{ko?'위의 기존 한도가 계속 적용됩니다.':'The current limits above remain in force.'}</Text>}
      {policy.pending && <Text style={styles.text}>{ko?'변경 대기':'Change pending'}: {amount(policy.pending.perTransfer)} / {amount(policy.pending.daily)} · {ko?'기기 확인 시각 이후':'Device activation not before'} {activation(policy.pending.notBefore)}</Text>}
      {policy.pending?.recipients && recipientList(ko?'대기 수취인':'Pending recipients',policy.pending.recipients)}
      {policy.pending && activate && button(ko?'대기 완료 확인':'Check delayed activation',activate)}
      {policy.mode!=='finality' && !policy.pending && policyDecimals===undefined && <Text style={styles.text}>{ko?'변경하려면 같은 자산의 소수 자릿수를 먼저 검증하세요.':'Verify decimals for this asset before changing the policy.'}</Text>}
      {policy.recipients && !policy.pending && changeSet && <>
        {!setEditor && button(ko?'허용 수취인 편집':'Edit allowed recipients',()=>setSetEditor('recipients'))}
        {!setEditor && policyDecimals!==undefined && button(ko?'한도만 편집':'Edit limits only',()=>setSetEditor('caps'))}
        {setEditor==='recipients' && <>
          {recipientList(ko?'제안한 수취인':'Proposed recipients',recipientDraft)}
          {!policyReview && <>
            {recipientDraft.map(entry=><View key={entry}>{button((ko?'제거 ':'Remove ')+entry,
              ()=>setRecipientDraft(recipientDraft.filter(item=>item!==entry)))}</View>)}
            <TextInput placeholderTextColor={color.mute} accessibilityLabel={ko?'추가할 주소':'Address to add'} maxLength={42} autoCapitalize="none"
              autoCorrect={false} value={recipientInput} onChangeText={setRecipientInput} style={styles.input}/>
            {button(ko?'주소 추가':'Add address',()=>addRecipient(recipientInput))}
            {policyContacts?.filter(contact=>!recipientDraft.includes(contact.address.toLowerCase())).map(contact=>
              <View key={contact.address}>{button((ko?'주소록에서 추가 ':'Add from contacts ')+contact.name,
                ()=>addRecipient(contact.address))}</View>)}
            {recipientError && <Text accessibilityLiveRegion="polite" style={styles.text}>{ko?
              '서로 다른 유효 주소를 최대 8개 입력하세요.':'Use at most eight distinct valid addresses.'}</Text>}
            {recipientDraft.join(',')!==policy.recipients.join(',') &&
              button(ko?'수취인 변경 검토':'Review recipient changes',()=>setPolicyReview(true))}
          </>}
          {policyReview && button(ko?'수취인 변경 독립 표시 요청':'Review recipients on independent display',
            ()=>changeSet({recipients:[...recipientDraft]}))}
        </>}
        {setEditor==='caps' && <>
          <TextInput placeholderTextColor={color.mute} accessibilityLabel={ko?'새 건별 한도':'New per-transfer limit'} keyboardType="decimal-pad"
            value={policyPer} onChangeText={value=>{setPolicyPer(value);setPolicyReview(false);}} style={styles.input}/>
          <TextInput placeholderTextColor={color.mute} accessibilityLabel={ko?'새 일별 한도':'New daily limit'} keyboardType="decimal-pad"
            value={policyDaily} onChangeText={value=>{setPolicyDaily(value);setPolicyReview(false);}} style={styles.input}/>
          {policyReview ? button(ko?'한도 변경 독립 표시 요청':'Review limits on independent display',
            ()=>changeSet({per:policyPer,daily:policyDaily})) :
            button(ko?'한도 변경 검토':'Review cap changes',()=>setPolicyReview(true))}
        </>}
        {setEditor && button(ko?'편집 취소':'Cancel policy edit',()=>{
          setSetEditor(undefined);setPolicyReview(false);setRecipientDraft(policy.recipients!);
          setRecipientInput('');setRecipientError(false);setPolicyPer(quantity(policy.perTransfer));setPolicyDaily(quantity(policy.daily));
        })}
      </>}
      {!policy.recipients && !applied && policy.mode!=='finality' && !policy.pending && policyDecimals!==undefined && change && <>
        <Text style={styles.text}>{ko?'변경은 독립 표시와 보드 승인을 거쳐 대기 시간이 지난 뒤에 적용됩니다.':'Changes require independent display and board approval, then a waiting period.'}</Text>
        <Text style={styles.text}>{ko?'새 허용 주소':'New allowed address'}</Text>
        <TextInput placeholderTextColor={color.mute} accessibilityLabel={ko?'새 허용 주소':'New allowed address'} autoCapitalize="none" value={policyDestination} onChangeText={value=>{setPolicyDestination(value);setPolicyReview(false);}} style={styles.input}/>
        <Text style={styles.text}>{ko?'새 건별 한도':'New per-transfer limit'}</Text>
        <TextInput placeholderTextColor={color.mute} accessibilityLabel={ko?'새 건별 한도':'New per-transfer limit'} keyboardType="decimal-pad" value={policyPer} onChangeText={value=>{setPolicyPer(value);setPolicyReview(false);}} style={styles.input}/>
        <Text style={styles.text}>{ko?'새 일별 한도':'New daily limit'}</Text>
        <TextInput placeholderTextColor={color.mute} accessibilityLabel={ko?'새 일별 한도':'New daily limit'} keyboardType="decimal-pad" value={policyDaily} onChangeText={value=>{setPolicyDaily(value);setPolicyReview(false);}} style={styles.input}/>
        {button(ko?'변경 내용 검토':'Review change',()=>setPolicyReview(true))}
        {policyReview && <>
          <Text selectable style={styles.text}>{policyDestination}{'\n'}{policyPer} {amountUnit} / {policyDaily} {amountUnit}</Text>
          <Text style={styles.text}>{ko?'독립 표시에서 같은 내용을 확인한 뒤 보드의 SW1을 누르세요.':'Confirm the same details on the independent display, then press SW1 on the board.'}</Text>
          {button(ko?'독립 표시로 변경 요청':'Request change on independent display',()=>change(policyDestination,policyPer,policyDaily))}
          {button(ko?'변경 검토 취소':'Cancel change review',()=>{
            setPolicyDestination(policy.destination);
            setPolicyPer(quantity(policy.perTransfer));
            setPolicyDaily(quantity(policy.daily));
            setPolicyReview(false);
          })}
        </>}
      </>}
    </>}
    {policyFresh && <>
      <Text style={styles.text}>{ko?'새 기기의 첫 서명 전에 보호 정책을 설정합니다. 기존 지갑의 이전 서명은 자동으로 무효화되지 않습니다.':'Set a protection policy before the new device signs. Previous signatures on an existing wallet are not automatically revoked.'}</Text>
      {!freshAsset ? <>
        <Text style={styles.text}>{ko?'먼저 검증한 StableNet 자산을 선택하세요.':'First select a reviewed StableNet asset.'}</Text>
        {chooseAsset && button(ko?'자산 선택':'Choose asset',chooseAsset)}
      </> : <>
        <Text selectable style={styles.text}>{ko?'검증한 자산':'Reviewed asset'}: {freshAsset.address} · {ko?'소수 자릿수':'Decimals'} {freshAsset.decimals}</Text>
        <Text style={styles.text}>{ko?'허용 주소':'Allowed address'}</Text>
        <TextInput placeholderTextColor={color.mute} accessibilityLabel={ko?'허용 주소':'Allowed address'} autoCapitalize="none" value={policyDestination} onChangeText={value=>{setPolicyDestination(value);setPolicyReview(false);}} style={styles.input}/>
        <Text style={styles.text}>{ko?'건별 한도':'Per-transfer limit'}</Text>
        <TextInput placeholderTextColor={color.mute} accessibilityLabel={ko?'건별 한도':'Per-transfer limit'} keyboardType="decimal-pad" value={policyPer} onChangeText={value=>{setPolicyPer(value);setPolicyReview(false);}} style={styles.input}/>
        <Text style={styles.text}>{ko?'일별 한도':'Daily limit'}</Text>
        <TextInput placeholderTextColor={color.mute} accessibilityLabel={ko?'일별 한도':'Daily limit'} keyboardType="decimal-pad" value={policyDaily} onChangeText={value=>{setPolicyDaily(value);setPolicyReview(false);}} style={styles.input}/>
        {button(ko?'설정 검토':'Review setup',()=>setPolicyReview(true))}
        {policyReview && <>
          <Text style={styles.text}>{ko?'설정할 주소와 금액을 독립 표시에서도 대조하고 보드에서 승인하세요.':'Compare the destination and limits on the independent display and approve on the board.'}</Text>
          <Text selectable style={styles.text}>{policyDestination}{'\n'}{policyPer} / {policyDaily}</Text>
          {initialize && button(ko?'독립 표시에서 확인':'Confirm on independent display',()=>initialize(policyDestination,policyPer,policyDaily))}
          {button(ko?'설정 검토 취소':'Cancel setup review',()=>setPolicyReview(false))}
        </>}
      </>}
    </>}
    {policyFinalityFresh && <>
      <Text style={styles.text}>{ko?'새 8283 기기의 독립 최종성 발급자 설정 파일을 선택하고 별도 표시에서 키 지문과 한도를 확인하세요. 기존 기기의 키, 주소와 미결은 바뀌지 않습니다.':'Choose the new 8283 device’s finality issuer setup file, then compare the key fingerprint and limits on the independent display. Existing devices, addresses and pending requests remain unchanged.'}</Text>
      {finalityEnroll && button(ko?'발급자 설정 파일 선택':'Choose issuer setup file',finalityEnroll)}
    </>}
    {policyFinalityPending && <>
      <Text style={styles.text}>{ko?'이전에 승인한 설정이 준비되었습니다. 같은 기기에서 계속할 수 있습니다.':'A previously approved setup is prepared. Resume on the same device.'}</Text>
      {finalityResume && button(ko?'승인한 설정 재개':'Resume approved setup',finalityResume)}
    </>}
    {!policyFresh && !policyFinalityFresh && !policyFinalityPending && !policy && <Text style={styles.text}>{ko?'지출 한도와 허용 주소 정책 변경은 현재 이 화면에서 지원하지 않습니다. 독립 확인과 기기 확인 전에는 적용되지 않습니다.':'Policy changes are unavailable here. Independent review and device confirmation are required before any change.'}</Text>}
    {(policyRoute || boardStatus || idleLock) && <Text style={styles.text}>{idleLock
      ? (ko?'기기가 확인한 자동 잠금 · 유휴 2분 또는 분리 즉시':'Device-verified auto-lock · after 2 minutes idle or upon disconnect')
      : (ko?'자동 잠금 규칙 미확인':'Auto-lock rule unavailable')}</Text>}
    {checkIdleLock && button(ko?'기기 잠금 규칙 확인':'Verify device auto-lock',checkIdleLock)}
    {readPolicy && button(section==='home'?(ko?'한도와 잠금':'Limits and auto-lock'):(ko?'보호 정책 읽기':'Read device policy'),readPolicy)}
    {button(section==='home'?(ko?'기기·업데이트':'Device and updates'):(ko?'기기 상태 확인':'Check device status'),device)}
    {section==='home'&&accessibility&&button(ko?'접근성':'Accessibility',accessibility)}
    {section==='home'&&diagnostics&&button(ko?'도움말·진단':'Help and diagnostics',diagnostics)}
    {section==='home'&&button(ko?'연습 다시 보기':'Replay practice',practice)}
    {recover && button(ko?'분실 복구':'Wallet recovery',recover)}
    {credentials && button(ko?'인증수단':'Sign-in methods',credentials)}
    {networks && button(ko?'네트워크':'Networks',networks)}
    {privacy && button(ko?'개인정보와 여행 동의':'Privacy and travel consent',privacy)}
    <Text style={styles.text}>{ko?'원래 요청에서 미결 상태를 다시 확인하세요. 시계나 만료 시각만으로 처리 완료를 판단하지 않습니다.':'Review pending status at its original source. A clock or expiry alone does not establish completion.'}</Text>
    {pending?.signing && <Text style={styles.text}>{ko?'이전 승인 결과가 불확실합니다. 원래 기기에서 만료를 확인하세요.':'A prior approval is unresolved. Verify expiry on the original device.'}</Text>}
    {pending?.evm && <>
      <Text style={styles.text}>{ko?'저장된 EVM 전송':'Saved EVM transfer'}{pending.evmExpires ? ' · ' + new Date(pending.evmExpires * 1000).toLocaleString(ko?'ko-KR':'en-US') : ''}</Text>
      {button(ko?'EVM 전송 확인':'Review EVM transfer',pendingEvm)}
    </>}
    {pending?.chain && button(ko?'체인 전송 확인':'Review chain transfer',pendingChain)}
    {button(section==='home'?(ko?'미결 결제':'Pending payments'):(ko?'매장 주문 확인':'Review store orders'),pendingOrders)}
    {!section && <>{accessibilityControls}{diagnosticControls}</>}
    {failure && <Text accessibilityLiveRegion="polite" style={styles.text}>{ko?'기기 기능을 열지 못했습니다.':'Could not open the device feature.'}</Text>}
    {!tabbed && button(policyRoute?(ko?'설정으로 돌아가기':'Return to Settings'):(ko?'지갑으로':'Back to wallet'),back,'secondary')}
  </View>;
}
const styles=StyleSheet.create({
  home:{gap:16},
  card:{gap:12,padding:16,borderRadius:18,borderWidth:1,borderColor:color.line,backgroundColor:color.sheet},
  heading:{fontFamily:font,fontSize:20,fontWeight:'800',color:color.ink},
  text:{fontFamily:font,fontSize:16,lineHeight:24,color:color.ink},
  caption:{fontFamily:font,fontSize:13,lineHeight:19,color:color.mute},
  input:{fontFamily:font,borderWidth:1,borderColor:'#8fa497',borderRadius:12,minHeight:50,padding:12,color:color.ink,backgroundColor:color.sheet},
});
