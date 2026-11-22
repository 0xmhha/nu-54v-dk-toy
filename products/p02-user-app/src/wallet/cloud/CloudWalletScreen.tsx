import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {BackHandler, Keyboard, Linking, Pressable, StyleSheet, Text, TextInput, View} from 'react-native';
import {fromBase64} from '@nu54/protocol';
import NativeCloudProfile from '../../specs/NativeCloudWalletProfile.ts';
import NativeHttp from '../../specs/NativeCommerceHttp.ts';
import {
  CloudClient, CloudHttpError, closureJournalName, readClosureJournal, journalName, prepareTransfer, readCloudJournal, requestID, signedTransfer,
  type PendingClosure, type ClosureReceipt, type CloudAccount, type CloudCredential, type CloudFlow, type JournalEntry, type RecoveryHistoryItem, type CloudKind, type CloudOperation, type CloudProfile, type CloudProvider, type PendingCloud,
} from './client.ts';

const labels: Record<string,string> = {
  creating:'생성 확인 중', active:'사용 가능', blocked:'서명 동결', failed:'종료', closed:'계정 종료',
  reserved:'독립 확인 대기', running:'참여자 응답 대기', succeeded:'완료', held:'복구 보류', expired:'만료', denied:'독립 복구 승인 거절', refused:'서명 요청 종료',
};

type Flow = {kind:'login' | 'link' | 'approval' | 'close'; request:CloudFlow};

function Action({title, run, disabled}: {title:string; run:()=>void; disabled?:boolean}) {
  return <Pressable accessibilityRole="button" accessibilityLabel={title} disabled={disabled}
    accessibilityState={{disabled:Boolean(disabled)}} onPress={run} style={[styles.action,disabled&&styles.disabled]}>
    <Text style={styles.button}>{title}</Text>
  </Pressable>;
}

export type MarketCloudSession = {client:CloudClient; token:string; owner:string; account:CloudAccount};

export function CloudWalletScreen({profile, back, backLabel, selectMarket, recoveryLanguage}: {
  profile:CloudProfile; back:()=>void; backLabel?:string; selectMarket?: (session:MarketCloudSession)=>Promise<void>;
  recoveryLanguage?:'ko'|'en';
}) {
  const recoveryOnly = recoveryLanguage !== undefined;
  const recoveryText = (ko:string,en:string) => recoveryLanguage==='en' ? en : ko;
  const client=useMemo(()=>new CloudClient(profile,async size=>{
    if (!NativeHttp) throw new Error('P03_STORAGE');
    return fromBase64(await NativeHttp.randomBytes(size));
  },url=>Linking.openURL(url),(url,method,body,token)=>{
    if(!NativeCloudProfile) throw new Error('P03_NETWORK');
    return NativeCloudProfile.request(url,method,body,token);
  }),[profile]);
  const [token,setToken]=useState('');
  const [enrollment,setEnrollment]=useState('');
  const [subject,setSubject]=useState('');
  const realm=profile.api+'|'+profile.identity;
  const journalEntry=useRef<JournalEntry|undefined>(undefined);
  const [account,setAccount]=useState<CloudAccount>();
  const [pending,setPending]=useState<PendingCloud>();
  const [operation,setOperation]=useState<CloudOperation>();
  const [recoveryMissing,setRecoveryMissing]=useState(false);
  const [recoveryHistory,setRecoveryHistory]=useState<RecoveryHistoryItem[]>();
  const [archivedRefusal,setArchivedRefusal]=useState<CloudOperation>();
  const [flow,setFlow]=useState<Flow>();
  const [notice,setNotice]=useState('');
  const [busy,setBusy]=useState(false);
  const [loaded,setLoaded]=useState(false);
  const [profilePending,setProfilePending]=useState(false);
  const [localReady,setLocalReady]=useState(!profile.deviceParticipant);
  const [editing,setEditing]=useState(false);
  const [closing,setClosing]=useState(false);
  const [closingRequest,setClosingRequest]=useState<PendingClosure>();
  const [closureReceipt,setClosureReceipt]=useState<ClosureReceipt>();
  const [closureMissing,setClosureMissing]=useState(false);
  const [to,setTo]=useState('');
  const [amount,setAmount]=useState('');
  const [nonce,setNonce]=useState('');
  const [fee,setFee]=useState('');
  const [priority,setPriority]=useState('');
  const [credentials,setCredentials]=useState<CloudCredential[]>([]);
  const running=useRef<symbol|undefined>(undefined);
  const session=useRef({generation:0,expires:0,participantStarted:false,timer:undefined as ReturnType<typeof setTimeout>|undefined});
  useEffect(()=>()=> {
    session.current.generation++;
    running.current=undefined;
    clearTimeout(session.current.timer);
    if(profile.deviceParticipant && (!recoveryOnly || session.current.participantStarted))
      NativeCloudProfile?.stopParticipant().catch(()=>undefined);
  },[profile.deviceParticipant,recoveryOnly]);

  useEffect(()=>{
    if(!recoveryOnly) return;
    const listener=BackHandler.addEventListener('hardwareBackPress',()=>{
      if(!busy) back();
      return true;
    });
    return ()=>listener.remove();
  },[recoveryOnly,busy,back]);

  const clearSession=useCallback(async()=>{
    session.current.generation++;
    session.current.expires=0;
    clearTimeout(session.current.timer);
    setToken('');setEnrollment('');setSubject('');setAccount(undefined);setCredentials([]);
    setPending(undefined);setOperation(undefined);setRecoveryMissing(false);setClosureMissing(false);setRecoveryHistory(undefined);setArchivedRefusal(undefined);setFlow(undefined);setLoaded(false);setProfilePending(false);setLocalReady(!profile.deviceParticipant);
    setEditing(false);setClosing(false);setClosingRequest(undefined);setClosureReceipt(undefined);setTo('');setAmount('');setNonce('');setFee('');setPriority('');
    journalEntry.current=undefined;
    if(profile.deviceParticipant && (!recoveryOnly || session.current.participantStarted)) {
      if(await NativeCloudProfile?.stopParticipant()!==true) throw new Error('P03_PARTICIPANT_STOP');
      session.current.participantStarted=false;
    }
  },[profile.deviceParticipant,recoveryOnly]);

  const expireSession=useCallback(()=>{
    running.current=undefined;
    setBusy(false);
    return clearSession();
  },[clearSession]);

  function currentSession(generation:number) {
    const {generation:active,expires}=session.current;
    return active===generation && (expires===0 || Date.now()<expires*1000);
  }

  useEffect(()=>{
    if(!profile.deviceParticipant||!token||!loaded)return;
    let verifying=false;
    const generation=session.current.generation;
    const pulse=setInterval(()=>{
      if(verifying || generation!==session.current.generation)return;
      verifying=true;
      client.subject(token).then(current=>{
        if(current.linked===false)throw new CloudHttpError(403);
      }).catch((error:unknown)=>{
        if(generation===session.current.generation&&error instanceof CloudHttpError&&(error.status===401||error.status===403)) {
          return expireSession().catch(()=>undefined);
        }
      }).finally(()=>{verifying=false;});
    },15000);
    return ()=>clearInterval(pulse);
  },[expireSession,client,loaded,profile.deviceParticipant,token]);

  function perform(fn:()=>Promise<void>) {
    if(running.current)return;
    const task=Symbol();
    running.current=task;
    setBusy(true);
    setNotice('');
    fn().catch(async (error:unknown)=>{
      if(running.current!==task)return;
      if(error instanceof CloudHttpError){
        if(error.status===401){await clearSession().catch(()=>undefined);setNotice('클라우드 로그인이 만료됐습니다. 같은 요청은 다시 로그인한 뒤 조회할 수 있습니다.');return;}
        if(error.status===409){setNotice('요청과 현재 계정 상태를 확인하세요. 마지막 인증수단은 해제할 수 없습니다.');return;}
        if(error.status===403){
          if(token&&session.current.expires>0) {
            const linked=await client.subject(token).then(()=>true,(failure:unknown)=>
              !(failure instanceof CloudHttpError && (failure.status===401||failure.status===403)));
            if(running.current!==task)return;
            if(!linked) await clearSession().catch(()=>undefined);
          }
          setNotice('클라우드 자격 또는 독립 확인이 필요합니다.');return;}
      }
      setNotice('요청을 확인할 수 없습니다. 저장한 요청 번호로 다시 조회하세요.');
    }).finally(()=>{if(running.current===task){running.current=undefined;setBusy(false);}});
  }

  async function readAccount(access:string, generation=session.current.generation) {
    if(!currentSession(generation))return undefined;
    let current:CloudAccount|undefined;
    try{current=await client.account(access);}
    catch(error){if(!(error instanceof CloudHttpError&&error.status===404))throw error;}
    if(!currentSession(generation))return undefined;
    const links=await client.credentials(access);
    if(!currentSession(generation))return undefined;
    setAccount(current);
    setCredentials(links);
    return current;
  }

  async function activate(access:string) {
    const storage=NativeHttp;
    if(!storage)throw new Error('P03_STORAGE');
    const generation=session.current.generation;
    const {subject:owner,expires,linked}=await client.subject(access);
    if(generation!==session.current.generation)return false;
    session.current.expires=expires;
    clearTimeout(session.current.timer);
    session.current.timer=setTimeout(()=>{expireSession().catch(()=>undefined);},Math.max(0,expires*1000-Date.now()));
    setProfilePending(linked===false);
    if(linked===false){
      setSubject(owner);
      if(recoveryOnly) setNotice(recoveryText('기존에 연결한 클라우드 계정으로 다시 로그인하세요.','Sign in again with an existing linked cloud account.'));
      return false;
    }
    const current=await readAccount(access);
    if(generation!==session.current.generation)return false;
    if(recoveryOnly && (!current || !['active','blocked'].includes(current.state))){
      setSubject(owner);
      setLoaded(true);
      setNotice(recoveryText('이 계정에는 복구할 수 있는 클라우드 지갑이 없습니다.','This account has no cloud wallet eligible for recovery.'));
      return true;
    }
    const request=current
      ? readClosureJournal(await storage.loadJournal(closureJournalName(realm,owner,current.id)),realm,owner,current)
      : undefined;
    if(!currentSession(generation))return false;
    const receipt=current && (current.state==='closed' || request)
      ? await client.closure(access,current,request?.request).catch(()=>undefined)
      : undefined;
    if(!currentSession(generation))return false;
    setClosingRequest(request);
    setClosureReceipt(receipt);
    setClosureMissing(false);
    if(current?.state==='closed') {
      await storage.saveJournal(journalName(realm,owner,current.id),'');
      if(!currentSession(generation))return false;
      await storage.saveJournal(journalName(realm,owner),'');
      if(generation!==session.current.generation)return false;
      setLocalReady(false);
      journalEntry.current=undefined;
      setSubject(owner);
      setPending(undefined);
      setOperation(undefined);
      setLoaded(true);
      return true;
    }
    const scoped=async(wallet?:string)=>readCloudJournal(await storage.loadJournal(journalName(realm,owner,wallet)),realm,owner,wallet);
    const walletEntry=await scoped(current?.id);
    const initial=current&&(!walletEntry||walletEntry.pending.kind==='dkg') ? await scoped() : undefined;
    const existing=walletEntry??initial;
    if (!recoveryOnly && walletEntry?.wallet && initial && walletEntry.pending.id===initial.pending.id) {
      await storage.saveJournal(journalName(realm,owner),'');
    }
    if(generation!==session.current.generation)return false;
    if(recoveryOnly && existing && existing.pending.kind!=='refresh'){
      setSubject(owner);
      setPending(existing.pending);
      setLoaded(true);
      setNotice(recoveryText('기존 요청을 클라우드 지갑에서 먼저 확인하세요.','Check the existing request in your cloud wallet first.'));
      return true;
    }
    if(profile.deviceParticipant) {
      session.current.participantStarted=true;
      const ready=await NativeCloudProfile?.startParticipant(owner,expires,access).catch(()=>false);
      if(generation!==session.current.generation)return false;
      setLocalReady(ready===true);
    }
    const resumed=existing?.pending.kind==='refresh'
      ? await client.lookup(access,existing.pending,owner).catch(()=>undefined)
      : undefined;
    if(generation!==session.current.generation)return false;
    journalEntry.current=existing;
    setSubject(owner);
    setPending(existing?.pending);
    setOperation(resumed?.session.Wallet===current?.id ? resumed : undefined);
    setLoaded(true);
    return true;
  }

  async function provision() {
    if (!token || !subject || !NativeCloudProfile) throw new Error('P03_PARTICIPANT');
    const csr=await NativeCloudProfile.prepareParticipant(subject);
    const result=await client.roster(token,csr);
    if(result.state==='pending'){setNotice(profilePending?'이 휴대전화의 공개 등록 요청을 보냈습니다. 승인 뒤 계정 연계를 다시 확인하세요.':'이 단말의 등록 승인을 기다립니다. 같은 단말에서 다시 확인하세요.');return;}
    if(profilePending) throw new Error('P03_PROFILE_PENDING');
    if(result.state!=='ready'||!result.roster||!await NativeCloudProfile.installParticipant(subject,JSON.stringify(result.roster))) throw new Error('P03_ROSTER');
    session.current.participantStarted=true;
    const started=await NativeCloudProfile.startParticipant(subject,session.current.expires,token);
    setLocalReady(started);
    if(!started) throw new Error('P03_PARTICIPANT');
  }

  async function remember(value?:PendingCloud,wallet=account?.id) {
    if(!NativeHttp||!subject||!loaded)throw new Error('P03_STORAGE');
    const generation=session.current.generation;
    const prior=journalEntry.current;
    if (value && !wallet && value.kind!=='dkg') throw new Error('P03_JOURNAL_SCOPE');
    const next=value?{realm,subject,wallet,pending:value}:undefined;
    if (next) {
      await NativeHttp.saveJournal(journalName(realm,subject,wallet),JSON.stringify(next));
    }
    if(!currentSession(generation))return;
    if (prior && (!next || next.wallet!==prior.wallet)) {
      await NativeHttp.saveJournal(journalName(realm,subject,prior.wallet),'');
    }
    if(!currentSession(generation))return;
    journalEntry.current=next;
    setPending(value);
    setRecoveryMissing(false);
  }

  async function accept(request:PendingCloud,result:CloudOperation) {
    const current=await readAccount(token);
    if(!current || current.id!==result.session.Wallet)throw new Error('P03_WALLET_SCOPE');
    const existing=journalEntry.current;
    if(!existing || existing.pending.id!==request.id || existing.wallet&&existing.wallet!==current.id)throw new Error('P03_JOURNAL_SCOPE');
    if(!existing.wallet) await remember(request,current.id);
    setOperation(result);
    setRecoveryMissing(false);
  }

  async function start(kind:CloudKind) {
    if(!NativeHttp||!loaded||!localReady||pending || (recoveryOnly && (kind!=='refresh' ||
        !!closingRequest || !account?.recoveryAvailable || !['active','blocked'].includes(account.state))))throw new Error('P03_PENDING');
    const id=requestID(fromBase64(await NativeHttp.randomBytes(16)));
    const transfer=kind==='sign'?prepareTransfer(to,amount,nonce,fee,priority):undefined;
    const request:PendingCloud={id,kind,transfer};
    await remember(request);
    setEditing(false);
    await accept(request,await client.reserve(token,request,subject));
  }

  async function query() {
    if(!pending)return;
    try{await accept(pending,await client.lookup(token,pending,subject));}
    catch(error){
      if(error instanceof CloudHttpError&&error.status===404){setOperation(undefined);setRecoveryMissing(pending.kind==='refresh');setNotice('서비스에 이 요청이 없습니다. 같은 요청 번호로 제출할 수 있습니다.');return;}
      throw error;
    }
  }

  async function requestReview() {
    if(!NativeHttp||pending?.kind!=='refresh'||operation?.state!=='denied'||!operation.reviewAvailable) {
      throw new Error('P03_REVIEW');
    }
    const successor:PendingCloud={kind:'refresh',id:requestID(fromBase64(await NativeHttp.randomBytes(16))),reviewOf:pending.id};
    await remember(successor);
    setOperation(undefined);
    await accept(successor,await client.reserve(token,successor,subject));
  }

  async function showRecoveryHistory() {
    setArchivedRefusal(undefined);
    setRecoveryHistory(await client.recoveryHistory(token));
  }

  async function readRefusal(requestId:string) {
    const record=await client.lookup(token,{kind:'refresh',id:requestId},subject);
    if(record.state!=='denied'||record.session.Wallet!==account?.id) throw new Error('P03_REFUSAL');
    setArchivedRefusal(record);
  }

  async function beginClosure() {
    if(!NativeHttp||!account||!subject||!loaded||account.state==='closed'||closingRequest?.submitted&&!closureMissing)throw new Error('P03_CLOSURE_SCOPE');
    const generation=session.current.generation;
    const request=closingRequest??{wallet:account.id,epoch:account.epoch,request:requestID(fromBase64(await NativeHttp.randomBytes(16)))};
    if(request.wallet!==account.id||request.epoch!==account.epoch||!currentSession(generation))throw new Error('P03_CLOSURE_SCOPE');
    await NativeHttp.saveJournal(closureJournalName(realm,subject,account.id),JSON.stringify({...request,realm,subject}));
    if(!currentSession(generation))return;
    setClosingRequest(request);
    const authorization=await client.closeAuthorization(account,subject,request.request);
    if(currentSession(generation))setFlow({kind:'close',request:authorization});
  }

  async function cancelClosure() {
    const generation=session.current.generation;
    if(closingRequest?.submitted||closureReceipt)throw new Error('P03_CLOSURE_SCOPE');
    if(closingRequest&&account&&NativeHttp)await NativeHttp.saveJournal(closureJournalName(realm,subject,account.id),'');
    if(!currentSession(generation))return;
    setClosingRequest(undefined);
    setClosing(false);
  }

  async function acceptClosure(receipt:ClosureReceipt,generation:number) {
    if(!currentSession(generation))return;
    setClosureReceipt(receipt);
    setClosureMissing(false);
    const stopLocal=localReady&&profile.deviceParticipant;
    setLocalReady(false);
    if(stopLocal&&await NativeCloudProfile?.stopParticipant()!==true)throw new Error('P03_PARTICIPANT_STOP');
    if(!currentSession(generation))return;
    if(journalEntry.current)await remember();
    if(!currentSession(generation))return;
    setOperation(undefined);
    await readAccount(token,generation);
    if(currentSession(generation))setClosing(false);
  }

  async function checkClosure() {
    if(!account||!subject)return;
    const generation=session.current.generation;
    try{await acceptClosure(await client.closure(token,account,closingRequest?.request),generation);}
    catch(error){
      if(!currentSession(generation))return;
      if(error instanceof CloudHttpError&&error.status===404&&closingRequest){
        setClosureMissing(true);
        setNotice('서비스에 종료 접수 기록이 없습니다. 같은 요청으로 독립 확인을 다시 시작할 수 있습니다.');
        return;
      }
      throw error;
    }
  }

  async function poll() {
    if(!flow)return;
    if(Date.now()>=flow.request.expiresAt){setFlow(undefined);setNotice('확인 시간이 지났습니다. 기존 요청에서 독립 확인을 다시 시작하세요.');return;}
    const generation=session.current.generation;
    const result=await client.poll(flow.request);
    if(!currentSession(generation))return;
    if(result.state==='pending'){setNotice('브라우저에서 절차를 마친 뒤 다시 확인하세요.');return;}
    if(result.state==='ready' && flow.kind==='link') {
      if(typeof result.enrollmentToken!=='string'||result.enrollmentToken.length>8192)throw new Error('P03_ENROLLMENT');
      await client.link(token,result.enrollmentToken);
      if(!currentSession(generation))return;
      setFlow(undefined);
      await readAccount(token,generation);
      return;
    }
    if(flow.kind==='approval' && (result.state==='denied' && pending?.kind==='refresh' ||
        (result.state==='denied'||result.state==='resolved') && pending?.kind==='sign')) {
      setFlow(undefined);
      setOperation(undefined);
      await query();
      return;
    }
    if(result.state!=='ready'){setFlow(undefined);setNotice('확인 요청이 종료됐습니다.');return;}
    if(flow.kind==='login'&&typeof result.token==='string'&&result.token.length<=8192){
      await clearSession();
      setToken(result.token);
      setEnrollment(typeof result.enrollmentToken==='string'&&result.enrollmentToken.length<=8192?result.enrollmentToken:'');
      try{if(await activate(result.token)) setEnrollment('');}
      catch(error){if(!(error instanceof CloudHttpError&&error.status===403&&result.enrollmentToken))throw error;}
      return;
    }
    if(typeof result.proof!=='string'||!/^[A-Za-z0-9+/]{86}==$/.test(result.proof))throw new Error('P03_PROOF');
    if(flow.kind==='approval'&&pending){
      try{await accept(pending,await client.approve(token,pending,result.proof));}
      catch(error){
        if(pending.kind!=='sign'||!(error instanceof CloudHttpError)||error.status!==409)throw error;
        setFlow(undefined);
        setOperation(undefined);
        await query();
        return;
      }
    }else if(flow.kind==='close'&&account&&closingRequest&&NativeHttp){
      const submitted={...closingRequest,submitted:true};
      await NativeHttp.saveJournal(closureJournalName(realm,subject,account.id),JSON.stringify({...submitted,realm,subject}));
      if(!currentSession(generation))return;
      setClosingRequest(submitted);
      setClosureMissing(false);
      setFlow(undefined);
      await acceptClosure(await client.close(token,account,result.proof,closingRequest.request),generation);
    }
    if(currentSession(generation))setFlow(undefined);
  }

  const entryScope=selectMarket ? 'isolated-market-8283-v1' : 'sepolia-native-test-v1';
  const scope=profile.onboarding?.network??entryScope;
  const profileMatches=scope===entryScope;
  const providers=profile.onboarding?.providers;
  const beginLogin=(kind:'login'|'link',provider?:CloudProvider)=>perform(async()=>{
    if(!profileMatches) throw new Error('P03_ONBOARDING');
    try {
      setFlow({kind,request:await client.login(provider)});
    } catch {
      setNotice('이 배포의 클라우드 인증을 시작할 수 없습니다. 등록된 프로필을 확인한 뒤 다시 시도하세요.');
    }
  });
  const signFlow=flow?.kind==='approval'&&pending?.kind==='sign';
  const confirmed=operation && ['succeeded','failed','held','expired','refused'].includes(operation.state);
  let signed:string|undefined;
  let invalidSignature=false;
  if(operation?.signature&&pending&&account){
    try{signed=signedTransfer(operation,pending,account);}
    catch{invalidSignature=true;}
  }
  return <View style={styles.container}>
    <Text accessibilityRole="header" style={styles.title}>{recoveryOnly
      ? recoveryText('기존 클라우드 계정 인증','Authenticate existing cloud account') : '클라우드 지갑'}</Text>
    <Text>{scope==='isolated-market-8283-v1' || account?.profile?.startsWith('isolated-market-8283-v1:')
      ? 'StableNet 격리 학습 지갑 · 8283' : 'Ethereum Sepolia · 시험 프로필'}</Text>
    {selectMarket && token && subject && localReady && account?.state==='active' &&
      account.profile?.startsWith('isolated-market-8283-v1:') &&
      <Action title="이 클라우드 지갑을 격리 시장에 연결" disabled={busy}
        run={()=>perform(async()=>{
          try { await selectMarket({client,token,owner:subject,account}); }
          catch { setNotice('현재 배포와 일치하는 승인된 클라우드 학습 지갑을 선택하세요.'); }
        })}/>}
    <Action title={backLabel ?? "지갑 선택"} run={backLabel ? back : ()=>perform(async()=>{await clearSession();back();})} disabled={busy}/>
    {notice&&<Text accessibilityLiveRegion="polite">{notice}</Text>}
    {!token&&<>
      {recoveryOnly ? <Text style={styles.section}>{recoveryText(
        '이미 연결한 계정으로 로그인해 복구 자격을 확인하세요.',
        'Sign in with your linked account to check recovery eligibility.')}</Text> : <View style={styles.onboarding}>
        <Text style={styles.section}>별도 클라우드 MPC 지갑</Text>
        <Text>개인 보드와 별개의 계정과 키를 사용합니다.</Text>
        <Text>지원 프로필 · {scope}</Text>
        <Text>참여자 · 모바일 A · 서비스 B · 독립 복구 C</Text>
        <Text>{providers?'배포에 등록된 인증 제공자로 클라우드 계정에 로그인합니다.':'이 배포의 클라우드 인증 서비스에서 별도로 로그인합니다.'}</Text>
        <Text>새 지갑은 계정 연계 후 독립 사용자 증명과 분산 키 생성이 모두 끝날 때까지 활성 주소 0개입니다.</Text>
      </View>}
      {!profileMatches&&<Text accessibilityLiveRegion="polite">이 화면에서 사용할 수 없는 클라우드 배포 프로필입니다.</Text>}
      {!flow&&profileMatches&&(providers ? providers.map(provider=>
        <Action key={provider.id} title={recoveryText(`${provider.name}로 별도 로그인`,`Sign in with ${provider.name}`)} disabled={busy||!NativeHttp} run={()=>beginLogin('login',provider)}/>
      ):<Action title={recoveryText('클라우드 서비스에 별도 로그인','Sign in to the cloud service')} disabled={busy||!NativeHttp} run={()=>beginLogin('login')}/>)}
    </>}
    {token&&<Action title="클라우드 로그아웃" disabled={busy} run={()=>perform(clearSession)}/>}
    {flow&&!signFlow&&<>
      <Action title={flow.kind==='login'||flow.kind==='link'?'브라우저 로그인 확인':'독립 확인 결과 가져오기'} run={()=>perform(poll)} disabled={busy}/>
      <Action title="확인 취소" disabled={busy} run={()=>setFlow(undefined)}/>
    </>}
    {token&&!loaded&&<Action title="인증 연계 상태 재확인" disabled={busy||!!flow} run={()=>perform(async()=>{
      if(await activate(token)) setEnrollment('');
    })}/>}
    {!recoveryOnly&&token&&enrollment&&<Action title="이 계정을 클라우드 지갑에 연계" disabled={busy} run={()=>perform(async()=>{
      await client.enroll(enrollment);setEnrollment('');await activate(token);
    })}/>}
    {token&&subject&&(loaded||profilePending)&&(!recoveryOnly||(account?.recoveryAvailable && (!pending||pending.kind==='refresh')))&&!localReady&&account?.state!=='closed'&&<>
      <Text>이 휴대전화의 참여자 등록을 확인하세요.</Text>
      <Action title="휴대전화 참가자 등록 · 재확인" disabled={busy||!!flow} run={()=>perform(provision)}/>
    </>}
    {token&&account&&<>
      <Text>계정 상태 · {pending?.kind==='refresh'&&!operation ? '동결 여부 확인 대기'
        : account.state==='closed'&&(closingRequest||closureReceipt)&&!closureReceipt?.completedAt
          ? '서명 이용 종료 · 참여자 확인 중' : labels[account.state]??account.state}</Text>
      {account.epoch>0&&<Text>세대 · {account.epoch}</Text>}
      {account.address&&<Text selectable>수신 주소 · {account.address}</Text>}
    </>}
    {!recoveryOnly&&token&&loaded&&localReady&&!pending&&!enrollment&&(!account||account.state==='failed')&&
      <Action title="클라우드 지갑 생성" disabled={busy||!!flow} run={()=>perform(()=>start('dkg'))}/>}
    {token&&loaded&&localReady&&!pending&&!closingRequest&&account?.state==='active'&&<>
      <Action title="Sepolia 전송 요청" disabled={busy||!!flow} run={()=>setEditing(!editing)}/>
      {editing&&<>
        <Action title="키보드 닫기" run={Keyboard.dismiss}/>
        <TextInput accessibilityLabel="받는 주소" placeholder="받는 주소" style={styles.input} autoCapitalize="none" autoCorrect={false}
          returnKeyType="done" submitBehavior="blurAndSubmit"
          maxLength={42} value={to} onChangeText={setTo}/>
        {([
          ['전송액 (wei)',amount,setAmount],['거래 nonce',nonce,setNonce],
          ['최대 가스 단가 (wei)',fee,setFee],['우선 가스 단가 (wei)',priority,setPriority],
        ] as const).map(([label,value,set])=><TextInput key={label} accessibilityLabel={label} placeholder={label}
          style={styles.input} keyboardType="number-pad" maxLength={38} value={value} onChangeText={set}/>)}
        <Action title="원문 검토" disabled={busy||!!flow} run={()=>perform(()=>start('sign'))}/>
      </>}
    </>}
    {token&&loaded&&localReady&&!pending&&(!recoveryOnly||!closingRequest)&&account?.recoveryAvailable===true&&['active','blocked'].includes(account.state)&&
      <Action title={recoveryText('분실 휴대전화 교체 접수 · 서명 동결','Request replacement and freeze signing')} disabled={busy||!!flow} run={()=>perform(()=>start('refresh'))}/>}
    {token&&subject&&loaded&&pending&&(!recoveryOnly||pending.kind==='refresh')&&<>
      <Text selectable>요청 번호 · {pending.id}</Text>
      {pending.kind==='refresh'&&<Text accessibilityLiveRegion="polite">{!operation
        ? '동결 접수 · 서버 확인 대기'
        : operation.state==='denied' ? '동결 유지 · 독립 복구 거절'
        : operation.state==='running' ? `동결 접수 · 참여자 응답 ${operation.acknowledged} / ${operation.session.Peers.length}`
        : operation.state==='succeeded' ? '동결 해제 · 새 세대 활성' : '동결 접수 · 서명 차단 유지'}</Text>}
      {pending.transfer&&<>
        <Text>거래 체인 ID · {pending.transfer.chain}</Text>
        <Text selectable>받는 주소 · {pending.transfer.to}</Text>
        <Text>전송액 · {pending.transfer.valueWei} wei</Text>
        <Text>최대 가스비 · {BigInt(pending.transfer.maxFeeWei)*21000n} wei</Text>
        <Text>거래 nonce · {pending.transfer.nonce}</Text>
      </>}
      {operation&&<>
        <Text>요청 상태 · {labels[operation.state]}</Text>
        {pending.kind==='sign'&&<>
          <Text>이 원문의 세대 · {operation.session.Epoch}</Text>
          <Text selectable>이 원문의 지갑 · {operation.session.Wallet}</Text>
          <Text>서명 참여 조합 · {operation.session.Peers.map(peer=>peer.Role).join(' + ')}</Text>
          {operation.state==='refused'&&<Text accessibilityLiveRegion="polite">이 원문을 거절했습니다. 서명 반환 · 0건</Text>}
        </>}
        {operation.refusal&&<>
          <Text>휴대전화 교체를 승인하지 않았습니다.</Text>
          <Text>독립 확인 · 거절 · {new Date(operation.refusal.recordedAt).toLocaleString()}</Text>
          <Text>후보 세대 {operation.session.Epoch} 활성화 차단</Text>
          {operation.reviewAvailable&&
            <Action title="허가된 복구 재심사 시작" disabled={busy||!!flow||!localReady} run={()=>perform(requestReview)}/>}
          {operation.successorRequest&&
            <Text selectable>연결된 재심사 요청 · {operation.successorRequest}</Text>}
        </>}
        <Text>응답 · {operation.acknowledged} / {operation.session.Peers.length}</Text>
        <Text>만료 · {pending.kind==='sign' ? new Date(operation.session.Expires*1000).toISOString() : new Date(operation.session.Expires*1000).toLocaleString()}</Text>
        {Date.now()<Date.parse(operation.notBefore)&&<Text>독립 확인 시작 · {new Date(operation.notBefore).toLocaleString()}</Text>}
        {operation.state==='reserved'&&token&&localReady&&!flow&&<Action title={pending.kind==='refresh'?'독립 복구 확인':'요청 원문 독립 승인'}
          disabled={busy||Date.now()<Date.parse(operation.notBefore)||Date.now()>=operation.session.Expires*1000}
          run={()=>perform(async()=>setFlow({kind:'approval',request:await client.authorize(operation,pending,subject)}))}/>}
        {signFlow&&<>
          <Action title="독립 확인 결과 가져오기" run={()=>perform(poll)} disabled={busy}/>
          <Action title="확인 취소" disabled={busy} run={()=>setFlow(undefined)}/>
        </>}
      </>}
      {token&&<Action title="같은 요청 재조회" disabled={busy||!!flow} run={()=>perform(query)}/>}
      {token&&localReady&&!operation&&(pending.kind!=='refresh'||recoveryMissing)&&<Action title="같은 번호로 제출" disabled={busy||!!flow} run={()=>perform(async()=>{
        await accept(pending,await client.reserve(token,pending,subject));
      })}/>}
      {signed&&<><Text>검증된 서명 거래 · 체인 전송과 확정은 별도</Text><Text selectable>{signed}</Text></>}
      {invalidSignature&&<Text>서명 검증에 실패했습니다.</Text>}
      {confirmed&&<Action title="요청 확인 마침" disabled={busy||!!flow} run={()=>perform(async()=>{
        setOperation(undefined);await remember();await readAccount(token);
      })}/>}
    </>}
    {token&&account&&account.epoch>0&&<>
      <Action title="복구 거절 기록 조회" disabled={busy||!!flow} run={()=>perform(showRecoveryHistory)}/>
      {recoveryHistory&&recoveryHistory.length===0&&<Text>기록된 복구 거절이 없습니다.</Text>}
      {recoveryHistory?.map(item=>
        <Action key={item.requestId} title={`거절 영수증 · ${item.requestId.slice(-8)}`} disabled={busy||!!flow}
          run={()=>perform(()=>readRefusal(item.requestId))}/>)}
      {archivedRefusal?.refusal&&<View>
        <Text selectable>원래 복구 요청 · {archivedRefusal.requestId??archivedRefusal.session.ID}</Text>
        <Text>독립 확인 · 거절 · {new Date(archivedRefusal.refusal.recordedAt).toLocaleString()}</Text>
        <Text>후보 세대 {archivedRefusal.session.Epoch} 활성화 차단</Text>
        {archivedRefusal.successorRequest&&<Text selectable>연결된 재심사 요청 · {archivedRefusal.successorRequest}</Text>}
      </View>}
    </>}
    {!recoveryOnly&&token&&loaded&&(closingRequest||closureReceipt)&&<>
      <Text selectable>종료 요청 · {closureReceipt?.request??closingRequest?.request}</Text>
      <Text accessibilityLiveRegion="polite">{!closureReceipt ? closingRequest?.submitted
        ? '종료 접수 · 서버 확인 대기' : '종료 접수 · 독립 확인 대기'
        : closureReceipt.completedAt ? '종료 완료 · 참여자 확인 완료' : '종료 요청을 접수했습니다 · 참여자 확인 대기'}</Text>
      {closureReceipt&&<Text>자산 이동 · 0건</Text>}
      <Action title="같은 종료 요청 조회" disabled={busy||!!flow} run={()=>perform(checkClosure)}/>
    </>}
    {!recoveryOnly&&token&&account&&account.epoch>0&&account.state!=='closed'&&!flow&&<>
      {credentials.map(credential=><View key={credential.id}>
        <Text selectable>연계 수단 · {credential.id}</Text>
        <Action title={`연결 해제 · ${credential.id.slice(-8)}`} disabled={busy||credentials.length<=1} run={()=>perform(async()=>{
          await client.unlink(token,credential.id);await clearSession();setNotice('연결을 해제했습니다. 다시 로그인해 상태를 확인하세요.');
        })}/>
      </View>)}
      {providers ? providers.map(provider=>
        <Action key={provider.id} title={`${provider.name} 인증수단 연계 시작`} disabled={busy} run={()=>beginLogin('link',provider)}/>
      ):<Action title="다른 인증수단 연계 시작" disabled={busy} run={()=>beginLogin('link')}/>}
      {!closing&&!closingRequest?<Action title="클라우드 계정 종료 검토" disabled={busy} run={()=>setClosing(true)}/>:<>
        <Text>계정을 종료하면 클라우드 지갑의 서명 기능을 사용할 수 없습니다. 자산은 자동으로 이전되지 않습니다.</Text>
        {(!closingRequest?.submitted||closureMissing)&&<Action title="서명 이용 종료 확인 및 독립 인증" disabled={busy} run={()=>perform(beginClosure)}/>}
        {!closingRequest?.submitted&&<Action title="종료 취소" disabled={busy} run={()=>perform(cancelClosure)}/>}
      </>}
    </>}
  </View>;
}

const styles=StyleSheet.create({
  container:{gap:12,paddingBottom:20},
  title:{fontSize:23,fontWeight:'700',color:'#10283f'},
  onboarding:{gap:8,padding:14,borderRadius:8,backgroundColor:'#eef5f7'},
  section:{fontWeight:'700',color:'#10283f'},
  action:{backgroundColor:'#124864',padding:12,borderRadius:7,alignItems:'center'},
  button:{color:'#fff',fontWeight:'600'},
  disabled:{opacity:.45},
  input:{borderWidth:1,borderColor:'#8199a5',padding:10,borderRadius:6,color:'#142f3b'},
});
