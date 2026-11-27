import {inputPlaceholder} from '../ui/theme.ts';
import {useCallback, useEffect, useRef, useState} from 'react';
import {AppState, PermissionsAndroid, Platform, Pressable, ScrollView, StyleSheet, Text, TextInput, View} from 'react-native';
import {fromBase64, utf8Decode} from '@nu54/protocol';
import Native from '../specs/NativeCommerceHttp.ts';
import {consentScope, opaqueTravelID, newErasureInstallationProof, privateTravel, privateTravelPolicies, travelID, type Consent, type Purpose, type TravelPolicy} from './privateClient.ts';
import {TravelItinerary} from './TravelItinerary.tsx';
import {TravelVisits, type VisitScreen} from './TravelVisits.tsx';
import {deviceJournal, maxLocalRecordings, readErasureIntent, readLocalRecordings, writeErasureIntent, writeLocalRecordings, type ErasureIntent, type LocalRecording} from './localRecordings.ts';
import type {Place} from './catalog.ts';

type RecordItem = {id: string; purpose: Purpose; consentRevision: number; parent?: string; deleteAfter: string};
type ErasureReceipt = {request: string; requestedAt: string};
type ErasureHistory = {receipts: ErasureReceipt[]; next?: string};
type Erasure = {request: string; state: string; localDeviceAck: boolean; localDeviceEvidence?: 'none'|'subject-reported'|'installation-reported'|'not-applicable'; providerPending: boolean; backupExpiry: string};
const labels: Record<Purpose, string> = {
  location: '위치 기록 공유', voice: '음성 공유', transcription: '전사·요약',
  recommendation: '여행 추천과 일정', review: '후기 공개', challenge: '방문 챌린지',
};
const recordDate = (timestamp: string) => new Intl.DateTimeFormat('ko-KR', {
  month: 'long', day: 'numeric', hour: 'numeric', minute: 'numeric',
}).format(new Date(timestamp));
function Button({title, action, disabled}: {title: string; action: () => void; disabled?: boolean}) {
  return <Pressable accessibilityRole="button" accessibilityLabel={title} onPress={action} disabled={disabled}
    style={[styles.button, disabled && styles.disabled]}><Text>{title}</Text></Pressable>;
}
function Input({label, value, update, limit = 128, secure = false}: {label: string; value: string; update: (v: string) => void; limit?: number; secure?: boolean}) {
  return <TextInput placeholderTextColor={inputPlaceholder} accessibilityLabel={label} placeholder={label} value={value} onChangeText={update}
    autoCapitalize="none" autoCorrect={!secure} secureTextEntry={secure} returnKeyType="done" maxLength={limit} style={styles.input} />;
}

function sameRecommendation(left?: Consent, right?: Consent) {
  return left?.accepted===right?.accepted && left?.receipt===right?.receipt && left?.revision===right?.revision &&
    left?.policyVersion===right?.policyVersion && left?.recipient===right?.recipient && left?.expiresAt===right?.expiresAt;
}

export type PrivateTravelScreen = 'records' | 'itinerary' | VisitScreen;

export function TravelPrivate({back, place, catalogVersion, places, backLabel, initialScreen = 'records'}: {
  back: () => void; backLabel?: string; place?: Place; catalogVersion?: number; places: Place[]; initialScreen?: PrivateTravelScreen;
}) {
  const [policies,setPolicies]=useState<TravelPolicy[]>([]);
  const [consents,setConsents]=useState<Consent[]>([]);
  const [recommendationChecked,setRecommendationChecked]=useState(false);
  const consentRead=useRef(0);
  const foreground=useRef(true);
  const changingRecommendation=useRef(false);
  const recommendationEpoch=useRef(0);
  const [draftEpoch,setDraftEpoch]=useState(0);
  const verifiedRecommendation=useRef<Consent | undefined>(undefined);
  const [records,setRecords]=useState<RecordItem[]>([]);
  const [status,setStatus]=useState('');
  const [busy,setBusy]=useState(false);
  const lock=useRef(false);
  const [profile,setProfile]=useState('');
  const [identity,setIdentity]=useState('');
  const [target,setTarget]=useState('');
  const [declaring,setDeclaring]=useState<TravelPolicy>();
  const [recording,setRecording]=useState(false);
  const [local,setLocal]=useState('');
  const [localRecordings,setLocalRecordings]=useState<LocalRecording[]>([]);
  const [legacyLookup,setLegacyLookup]=useState(false);
  const [legacyID,setLegacyID]=useState('');
  const [hasSample,setHasSample]=useState(false);
  const [transcript,setTranscript]=useState('');
  const [eraseSelection,setEraseSelection]=useState('');
  const [erasure,setErasure]=useState<Erasure>();
  const [receipts,setReceipts]=useState<ErasureReceipt[]>([]);
  const [nextReceipt,setNextReceipt]=useState<string>();
  const [erasureIntent,setErasureIntent]=useState<ErasureIntent>();
  const isCurrentErasure=!!erasure && erasure.request===erasureIntent?.receipt;
  const [screen,setScreen]=useState<PrivateTravelScreen>(initialScreen);
  const currentScreen=useRef<typeof screen | undefined>(initialScreen);
  function navigate(next: typeof screen) {
    if (currentScreen.current==='itinerary' && next!=='itinerary') ++recommendationEpoch.current;
    currentScreen.current=next;
    setScreen(next);
  }
  const scopeEpoch=recommendationEpoch.current;
  const currentRecommendationScope=useCallback(()=>foreground.current && currentScreen.current==='itinerary' && scopeEpoch===recommendationEpoch.current,[scopeEpoch]);
  const current=(purpose: Purpose) => consents.find(c=>c.purpose===purpose);
  const granted=(purpose: Purpose) => {
    const consent=current(purpose);
    return consent?.accepted && Date.parse(consent.expiresAt)>Date.now() ? consent : undefined;
  };

  async function run(job: () => Promise<void>, error: string) {
    if (lock.current) return;
    lock.current=true;setBusy(true);setStatus('');
    try {await job();}
    catch {setStatus(error);}
    finally {setBusy(false);lock.current=false;}
  }
  const invalidateConsentRead=useCallback(()=>++consentRead.current,[]);
  const suspendRecommendation=useCallback((discardDraft=true)=>{
    invalidateConsentRead();
    ++recommendationEpoch.current;
    verifiedRecommendation.current=undefined;
    setRecommendationChecked(false);
    if (discardDraft) setDraftEpoch(value=>value+1);
  },[invalidateConsentRead]);
  const readConsents=useCallback(async () => {
    if (changingRecommendation.current) return;
    const read=++consentRead.current;
    try {
      const result=await privateTravel<{consents:Consent[]}>('/consents');
      if (read!==consentRead.current || !foreground.current) return;
      const recommendation=result.consents.find(consent=>consent.purpose==='recommendation');
      const previous=verifiedRecommendation.current;
      if (!sameRecommendation(previous,recommendation)) {
        ++recommendationEpoch.current;
        if (previous) setDraftEpoch(value=>value+1);
      }
      verifiedRecommendation.current=recommendation;
      setConsents(result.consents);
      setRecommendationChecked(true);
      return result.consents;
    } catch (error) {
      if(read!==consentRead.current || !foreground.current) return;
      suspendRecommendation();
      throw error;
    }
  },[suspendRecommendation]);
  async function verifyRecommendation(expected: Consent) {
    const latest=(await readConsents())?.find(consent=>consent.purpose==='recommendation');
    return !!latest?.accepted && sameRecommendation(latest,expected) && Date.parse(latest.expiresAt)>Date.now();
  }
  const refresh=useCallback(async () => {
    const enabled=await privateTravelPolicies();setPolicies(enabled);
    setReceipts([]);
    setNextReceipt(undefined);
    if (!enabled.length) {
      suspendRecommendation();
      return;
    }
    if (!await readConsents()) return;
    const item=await privateTravel<{records:RecordItem[]}>('/records');
    setRecords(item.records);
    try {
      const history=await privateTravel<ErasureHistory>('/erasure');
      setReceipts(history.receipts);
      setNextReceipt(history.next);
    } catch {
      // Legacy records servers do not expose a receipt index.
    }
  },[readConsents,suspendRecommendation]);
  useEffect(()=>{
    foreground.current=true;
    refresh().catch(()=>setStatus('여행 기록의 등록 또는 연결 상태를 확인해 주세요.'));
    readErasureIntent().then(setErasureIntent).catch(()=>setStatus('이 기기의 삭제 요청을 확인할 수 없습니다.'));
    readLocalRecordings().then(setLocalRecordings).catch(()=>setStatus('이 기기의 녹음 목록을 확인할 수 없습니다.'));
    const sub=AppState.addEventListener('change',state=>{
      const wasForeground=foreground.current;
      foreground.current=state==='active';
      if (foreground.current) {
        if (!wasForeground) readConsents().catch(()=>{});
        return;
      }
      suspendRecommendation();
      Native?.discardTravelRecording().catch(()=>{});
      setRecording(false);
      setTranscript('');
      setHasSample(false);
      setLocal('');
    });
    return ()=>{foreground.current=false;invalidateConsentRead();sub.remove();Native?.discardTravelRecording().catch(()=>{});};
  },[invalidateConsentRead,readConsents,refresh,suspendRecommendation]);
  const task=(action: () => Promise<void>, message='현재 처리 상태를 확인할 수 없습니다. 잠시 후 다시 시도해 주세요.') =>
    () => {run(action,message);};

  async function updateRecommendationAuthority(change: () => Promise<void>) {
    changingRecommendation.current=true;
    suspendRecommendation();
    try {await change();}
    finally {changingRecommendation.current=false;}
  }
  async function changeConsent(policy:TravelPolicy, accept:boolean) {
    const change=async () => {
      await privateTravel('/consents',consentScope(policy,current(policy.purpose),policy.purpose==='transcription'?target:'',await travelID(),accept));
    };
    if (policy.purpose==='recommendation') await updateRecommendationAuthority(change);
    else await change();
    setDeclaring(undefined);
    await refresh();
  }
  const recommendationPolicy=policies.find(policy=>policy.purpose==='recommendation' && policy.recipient==='p09' && !policy.targetRequired);
  async function approveItineraryRecommendation() {
    if (!recommendationChecked || !recommendationPolicy || changingRecommendation.current || !foreground.current) return;
    const previous=current('recommendation');
    changingRecommendation.current=true;
    suspendRecommendation(false);
    const before=recommendationEpoch.current;
    let receipt: string;
    try {
      const key=await travelID();
      if (!foreground.current || currentScreen.current!=='itinerary' || before!==recommendationEpoch.current) return;
      const result=await privateTravel<{receipt:string}>('/consents',consentScope(recommendationPolicy,previous,'',key,true));
      receipt=result.receipt;
      if (!foreground.current || currentScreen.current!=='itinerary' || before!==recommendationEpoch.current) return;
    } finally {
      changingRecommendation.current=false;
    }
    const latest=(await readConsents())?.find(consent=>consent.purpose==='recommendation');
    if (currentScreen.current==='itinerary' && latest?.accepted && latest.receipt===receipt && Date.parse(latest.expiresAt)>Date.now()) return latest;
  }

  async function capture() {
    if(!Native||localRecordings.length>=maxLocalRecordings)throw new Error();
    if(Platform.OS==='android' &&
      await PermissionsAndroid.request(PermissionsAndroid.PERMISSIONS.RECORD_AUDIO)!==PermissionsAndroid.RESULTS.GRANTED)throw new Error();
    await Native.startTravelRecording();
    setRecording(true);
  }
  async function finish() {
    if(!Native)throw new Error();
    try {
      const sample=await Native.finishTravelRecording();
      const id=await travelID();
      const entries = [{id, capturedAt: new Date().toISOString()}, ...await readLocalRecordings()];
      await writeLocalRecordings(entries);
      try { await Native.saveJournal(deviceJournal(id),sample); }
      catch (error) {
        await writeLocalRecordings(entries.slice(1));
        throw error;
      }
      setLocalRecordings(entries);
      setLocal(id);
      setHasSample(true);
    } finally {setRecording(false);}
  }
  async function importLegacy() {
    if (!Native || !opaqueTravelID.test(legacyID) || !await Native.loadJournal(deviceJournal(legacyID))) throw new Error();
    const entries=await readLocalRecordings();
    if (!entries.some(item=>item.id===legacyID)) {
      entries.unshift({id:legacyID});
      await writeLocalRecordings(entries);
    }
    setLocalRecordings(entries);
    setLocal(legacyID);
    setHasSample(true);
    setLegacyLookup(false);
    setLegacyID('');
  }
  async function share() {
    const consent=granted('voice');
    if(!Native||!consent||!opaqueTravelID.test(local))throw new Error();
    const data=await Native.loadJournal(deviceJournal(local));
    if(!data)throw new Error();
    await privateTravel('/records',{id:local,purpose:'voice',consentRevision:consent.revision,
      recipient:consent.recipient,deleteAfter:consent.expiresAt,data});
    await refresh();
  }
  async function sendTranscript(id:string) {
    const consent=granted('transcription');
    if(!consent||consent.object!==id)throw new Error();
    await privateTravel('/transcriptions',{job:await travelID(),source:id,consentRevision:consent.revision,recipient:consent.recipient});
    setStatus('선택한 녹음 한 건의 전사를 요청했습니다.');
  }
  async function requestDeletion(id:string) {
    const previous=await readErasureIntent();
    if (previous && previous.object!==id) throw new Error('Another deletion is pending');
    const intent=previous??{object:id,key:await travelID(),proof:await newErasureInstallationProof()};
    await writeErasureIntent(intent);
    setErasureIntent(intent);
    const result=await privateTravel<Erasure>('/erasure',{
      object:intent.object,requestKey:intent.key,installationProof:intent.proof,
    });
    const written=result.request===intent.key ? {...intent,receipt:result.request} : undefined;
    await writeErasureIntent(written);
    setErasureIntent(written);
    setErasure(result);
    await refresh();
  }
  async function refreshDeletion() {
    if(!erasureIntent)throw new Error();
    if(!erasureIntent.receipt) {
      await requestDeletion(erasureIntent.object);
      return;
    }
    setErasure(await privateTravel<Erasure>(`/erasure/${erasureIntent.receipt}`));
  }
  async function localErase(id:string) {
    if(!Native||!opaqueTravelID.test(id))throw new Error();
    await Native.saveJournal(deviceJournal(id),'');
    if(await Native.loadJournal(deviceJournal(id)))throw new Error();
    const entries=(await readLocalRecordings()).filter(item=>item.id!==id);
    await writeLocalRecordings(entries);
    setLocalRecordings(entries);
    if(local===id){setHasSample(false);setLocal('');}
    if(erasureIntent?.object===id && erasureIntent.receipt && isCurrentErasure) {
      const result=await privateTravel<Erasure>(`/erasure/${erasureIntent.receipt}`,{localDeviceErased:true,installationProof:erasureIntent.proof});
      setErasure(result);
    }
  }
  return <ScrollView keyboardShouldPersistTaps="handled" keyboardDismissMode="on-drag">
    <Button title={backLabel ?? "장소 검색으로 돌아가기"} action={()=>{currentScreen.current=undefined;back();}} />
    <Text style={styles.title}>여행 기록과 동의</Text>
    {!!policies.length && <View style={styles.preferences}>
      <Button title="여행 기록" disabled={screen==='records'} action={()=>navigate('records')} />
      <Button title="후기와 이용 자격" disabled={screen==='review'} action={()=>navigate('review')} />
      <Button title="스탬프와 혜택" disabled={screen==='stamps'} action={()=>navigate('stamps')} />
      <Button title="따라하기 챌린지" disabled={screen==='challenge'} action={()=>navigate('challenge')} />
      <Button title="내 일정과 추천" disabled={screen==='itinerary'} action={()=>navigate('itinerary')} />
    </View>}
    {screen==='itinerary' && !recommendationChecked && <Button title="추천 동의 상태 다시 확인" disabled={busy} action={task(async()=>{await readConsents();})} />}
    {screen==='itinerary' && <TravelItinerary key={draftEpoch} scopeEpoch={scopeEpoch} place={place} places={places} catalogVersion={catalogVersion}
      consent={recommendationChecked ? granted('recommendation') : undefined} scopeCurrent={currentRecommendationScope}
      verifyConsent={verifyRecommendation} policy={recommendationChecked ? recommendationPolicy : undefined}
      approveRecommendation={approveItineraryRecommendation} manageConsent={()=>navigate('records')} />}
    {(screen==='review' || screen==='stamps' || screen==='challenge') &&
      <TravelVisits key={screen === 'review' ? place?.id ?? screen : screen} screen={screen} place={place} places={places} policies={policies} />}
    {(screen==='records' || !policies.length) && <>
    {!policies.length && <View style={styles.section}>
      <Text>개인 기록은 상점 등록과 별도의 발급 정보가 필요합니다.</Text>
      <Button title="동의하지 않음" action={()=>{currentScreen.current=undefined;back();}} />
      <Button title="개인 기록 등록 공개키" disabled={busy} action={task(async()=>{setIdentity(await Native!.publicIdentity('travel'));})} />
      {!!identity&&<Text selectable>{identity}</Text>}
      <Input label="승인된 개인 기록 등록서" value={profile} update={setProfile} limit={16_384} secure />
      <Button title="개인 기록 등록 확인" disabled={busy||!profile} action={task(async()=>{await updateRecommendationAuthority(()=>Native!.configure('travel',profile));setProfile('');await refresh();})} />
    </View>}
    <View style={styles.section}>
      <Text style={styles.heading}>음성 기록</Text>
      <Button title={recording?'녹음 종료하고 이 기기에만 저장':'음성 녹음 시작'} disabled={busy||(!recording&&localRecordings.length>=maxLocalRecordings)}
        action={task(recording?finish:capture,'마이크 권한 또는 녹음 상태를 확인해 주세요.')} />
      {!localRecordings.length&&<Text>이 기기에 저장한 녹음이 없습니다.</Text>}
      {localRecordings.length>=maxLocalRecordings&&<Text>녹음을 더 저장하려면 기존 녹음 한 건을 선택해 삭제해 주세요.</Text>}
      {localRecordings.map(item=><Button key={item.id} title={item.capturedAt? `녹음 · ${recordDate(item.capturedAt)}` : '이전 버전에서 가져온 녹음'}
        disabled={busy||recording} action={task(async()=>{
          setLocal(item.id);
          setHasSample(!!(await Native!.loadJournal(deviceJournal(item.id))));
        })} />)}
      <Button title="이전 버전 녹음 찾기" disabled={busy||recording} action={()=>setLegacyLookup(!legacyLookup)} />
      {legacyLookup&&<>
        <Input label="이전에 저장한 식별 정보" value={legacyID} update={setLegacyID} limit={36} />
        <Button title="이전 녹음을 목록에 추가" disabled={busy||!opaqueTravelID.test(legacyID)} action={task(importLegacy)} />
      </>}
      {!!local&&!hasSample&&<Button title="누락된 녹음 목록 항목 지우기" disabled={busy} action={task(()=>localErase(local))} />}
      {hasSample&&<>
        <Button title="선택한 녹음 한 건 공유" disabled={busy||!granted('voice')} action={task(share)} />
        <Button title="이 기기 녹음 삭제" disabled={busy} action={task(()=>localErase(local))} />
      </>}
    </View>
    {!!policies.length && <>
      <Text style={styles.heading}>목적별 선택</Text>
      {!!target&&<Text>목록에서 선택한 녹음에만 전사 동의가 적용됩니다.</Text>}
      {policies.filter(policy=>policy.purpose!=='review' && policy.purpose!=='challenge').map(policy=><View style={styles.section} key={`${policy.purpose}/${policy.recipient}`}>
        <Text>{labels[policy.purpose]} · {policy.recipient} · 정책 {policy.version}</Text>
        <Text>최대 {Math.round(policy.maxLifetimeSeconds/3600)}시간</Text>
        {!!current(policy.purpose)?.receipt&&<Text selectable>동의 기록 {current(policy.purpose)!.receipt}</Text>}
        {declaring===policy && policy.purpose!=='transcription' ? <>
          <Button title="이 목적과 수신 범위에 동의" disabled={busy} action={task(()=>changeConsent(policy,true))} />
          <Button title="동의하지 않음" action={()=>setDeclaring(undefined)} />
        </> : policy.purpose!=='transcription' && <Button title={`${labels[policy.purpose]} 선택`} disabled={busy} action={()=>setDeclaring(policy)} />}
        {!!granted(policy.purpose)&&<Button title={`${labels[policy.purpose]} 철회`} disabled={busy}
          action={task(()=>changeConsent(policy,false))} />}
      </View>)}
      <Button title="여행 기록 다시 읽기" disabled={busy} action={task(refresh)} />
      {records.map(item=><View key={item.id} style={styles.section}>
        <Text>{item.purpose==='voice'?'공유한 녹음':labels[item.purpose]} · 보관 종료 {recordDate(item.deleteAfter)}</Text>
        {item.purpose==='voice' && policies.some(policy=>policy.purpose==='transcription') &&
          <View>
            {target===item.id && declaring?.purpose==='transcription' ? <>
              <Text>선택한 녹음 한 건 · {declaring.recipient} · 정책 {declaring.version}</Text>
              <Button title="이 녹음의 전사에 동의" disabled={busy} action={task(()=>changeConsent(declaring,true))} />
              <Button title="전사 동의 닫기" disabled={busy} action={()=>setDeclaring(undefined)} />
            </> : <Button title="이 녹음의 전사 동의 검토" disabled={busy} action={()=>{
              setTarget(item.id);
              setDeclaring(policies.find(policy=>policy.purpose==='transcription'));
            }} />}
          </View>}
        {item.purpose==='voice' && <Button title="이 녹음 한 건 전사 요청" disabled={busy||granted('transcription')?.object!==item.id}
           action={task(()=>sendTranscript(item.id))} />}
        {item.purpose==='transcription' && <Button title="전사 내용 보기" disabled={busy} action={task(async()=>{
          const value=await privateTravel<{data:string}>(`/records/${item.id}`);
          setTranscript(utf8Decode(fromBase64(value.data)));
        })} />}
        <Button title="이 기록 삭제 검토" disabled={busy||!!erasureIntent} action={()=>setEraseSelection(item.id)} />
        {eraseSelection===item.id&&!erasureIntent&&<Button title="이 기록과 파생물 삭제 요청" disabled={busy}
          action={task(()=>requestDeletion(item.id))} />}
      </View>)}
      {!!transcript&&<Text>{transcript}</Text>}
      {erasureIntent&&!isCurrentErasure&&<Button title="선택한 기록의 삭제 상태 열기" disabled={busy} action={task(refreshDeletion)} />}
      {receipts.length>0&&<View style={styles.section}>
        <Text style={styles.heading}>삭제 영수증</Text>
        {receipts.map(receipt=><View key={receipt.request}>
          <Text>{recordDate(receipt.requestedAt)}</Text>
          <Button title="삭제 영수증 보기" disabled={busy} action={task(async()=>{
            setErasure(await privateTravel<Erasure>(`/erasure/${receipt.request}`));
          })} />
        </View>)}
        {!!nextReceipt&&<Button title="이전 삭제 영수증 더 보기" disabled={busy} action={task(async()=>{
          const page=await privateTravel<ErasureHistory>('/erasure',{before:nextReceipt});
          setReceipts(previous=>[...previous,...page.receipts]);
          setNextReceipt(page.next);
        })} />}
      </View>}
      {erasure&&<View style={styles.section}>
        <Text>{erasure.state==='journal-pending'?'삭제 표식 기록 대기 중':'서버 접근 차단 확인'}</Text>
        {erasure.providerPending&&<Text>수탁자 종료·삭제 확인 대기 중</Text>}
        <Text>{Date.parse(erasure.backupExpiry)>Date.now()?'백업 만료 예정':'백업 만료 예정일 지남'} {recordDate(erasure.backupExpiry)}</Text>
        {erasure.localDeviceEvidence==='not-applicable' ? <Text>서버에 저장한 여행 내역 삭제 요청</Text> : erasure.localDeviceAck ? <Text>{erasure.localDeviceEvidence==='installation-reported'
          ? '삭제를 요청한 설치본이 복사본 삭제를 보고했습니다.'
          : '이 계정에서 복사본 삭제를 보고했습니다.'}</Text> :
          isCurrentErasure && erasureIntent ?
            <Button title="이 기기의 복사본 삭제와 확인" disabled={busy} action={task(()=>localErase(erasureIntent.object))} /> :
            <Text>요청한 기기의 복사본 삭제 확인 대기 중</Text>}
        <Button title="삭제 상태 다시 조회" disabled={busy} action={task(async()=>{
          setErasure(await privateTravel<Erasure>(`/erasure/${erasure.request}`));
        })} />
        {!isCurrentErasure&&<Button title="삭제 영수증 닫기" disabled={busy} action={()=>setErasure(undefined)} />}
        {isCurrentErasure && erasure.state==='access-fenced' && erasure.localDeviceAck &&
          <Button title="완료한 삭제 상태 닫기" disabled={busy} action={task(async()=>{
            await writeErasureIntent();
            setErasureIntent(undefined);
            setErasure(undefined);
            setEraseSelection('');
          })} />}
      </View>}
    </>}
    </>}
    {!!status&&<Text accessibilityRole="alert">{status}</Text>}
  </ScrollView>;
}

const styles=StyleSheet.create({
  title:{fontSize:24,fontWeight:'600',marginVertical:14},
  heading:{fontSize:18,fontWeight:'600',marginVertical:10},
  section:{borderBottomWidth:1,borderColor:'#cad8ce',paddingVertical:10},
  input:{borderBottomWidth:1,borderColor:'#9aada0',padding:10,marginBottom:8},
  button:{backgroundColor:'#e5ede9',padding:11,marginVertical:4,alignItems:'center',borderRadius:5},
  disabled:{opacity:0.5},
  preferences:{flexDirection:'row',flexWrap:'wrap',gap:4},
});
