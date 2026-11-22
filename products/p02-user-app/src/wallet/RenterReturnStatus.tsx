import {useCallback,useLayoutEffect,useRef,useState} from 'react';
import {Pressable,Text,TextInput,View} from 'react-native';
import {sha256} from '@noble/hashes/sha256';
import {bytesToHex,utf8Encode} from '@nu54/protocol';
import {RentalOriginalOrderDetail,type RentalOriginalPayment} from '../merchant/RentalOriginalOrderDetail.tsx';
import Http from '../specs/NativeCommerceHttp.ts';

const assignmentRef=/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const accountRef=/^0x[0-9a-f]{40}$/;
const maxUint256=2n**256n-1n;
type Assignment={assignment:string;account:string;generation:string;assignmentUntil:string;slot:'unconfirmed'|'open'};
type CredentialTerm={expiresAt:number;evidenceUntil:number};
type SlotObservation={checkedAt:number;evidenceUntil:number};
const credentialEvidenceWindowMs=30_000;
type VerifiedReturn=Assignment&{credential?:CredentialTerm;slotObservation?:SlotObservation}&({settlement:'unknown'}|{settlement:'verified';pendingCount:number;pendingPayments:number;pendingLimits:number;pendingOrders?:RentalOriginalPayment[];tokenDecimals?:number;tokenSymbol?:string;basisAt:string;basisUntil:string});
const baseKeys='account,assignment,assignmentUntil,generation,settlement,slot';
const orderKeys='account,assignment,assignmentUntil,basisAt,basisUntil,generation,pendingCount,pendingLimits,pendingOrders,pendingPayments,settlement,slot';
const metadataKeys=orderKeys+',tokenDecimals,tokenSymbol';
const countKeys='account,assignment,assignmentUntil,basisAt,basisUntil,generation,pendingCount,pendingLimits,pendingPayments,settlement,slot';

function parseCredentialTime(value:unknown):number|undefined {
  if(typeof value!=='string'||!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/.test(value))return;
  const instant=Date.parse(value);
  if(Number.isFinite(instant)&&new Date(instant).toISOString().replace('.000Z','Z')===value)return instant;
}

function verifiedReturn(raw:string, expected:string, account:string):VerifiedReturn {
  const parsed=JSON.parse(raw) as (Assignment&{settlement?:string;pendingCount?:number;pendingPayments?:number;pendingLimits?:number;pendingOrders?:unknown;tokenDecimals?:unknown;tokenSymbol?:unknown;basisAt?:string;basisUntil?:string;
    credentialExpiresAt?:unknown;credentialEvidenceUntil?:unknown;slotCheckedAt?:unknown;slotEvidenceUntil?:unknown})|null;
  if(!parsed||typeof parsed!=='object'||Array.isArray(parsed))throw Error('RETURN_NOT_VERIFIED');
  const {credentialExpiresAt,credentialEvidenceUntil,slotCheckedAt,slotEvidenceUntil,...data}=parsed;
  const now=Date.now();
  if(data.assignment!==expected || typeof data.generation!=='string'|| !/^[1-9][0-9]{0,18}$/.test(data.generation) ||
    typeof data.account!=='string'||!accountRef.test(data.account)||(account&&data.account!==account)||
    typeof data.assignmentUntil!=='string'||!(Date.parse(data.assignmentUntil)>now)||
    (data.slot!=='unconfirmed'&&data.slot!=='open'))throw Error('RETURN_NOT_VERIFIED');
  const expiresAt=parseCredentialTime(credentialExpiresAt);
  const evidenceUntil=parseCredentialTime(credentialEvidenceUntil);
  const credential=!account&&expiresAt!==undefined&&evidenceUntil!==undefined&&
    now<evidenceUntil&&evidenceUntil<=expiresAt&&evidenceUntil<=Date.parse(data.assignmentUntil)&&
    evidenceUntil<=now+credentialEvidenceWindowMs?{expiresAt,evidenceUntil}:undefined;
  const checkedAt=parseCredentialTime(slotCheckedAt);
  const slotUntil=parseCredentialTime(slotEvidenceUntil);
  const slotObservation=!account&&data.slot==='open'&&checkedAt!==undefined&&slotUntil!==undefined&&
    checkedAt<=now&&now<slotUntil&&slotUntil<=Date.parse(data.assignmentUntil)&&
    slotUntil-checkedAt<=credentialEvidenceWindowMs?{checkedAt,evidenceUntil:slotUntil}:undefined;
  const validated=()=>({...data,slot:'unconfirmed',credential,slotObservation}) as VerifiedReturn;
  const keys=Object.keys(data).sort().join();
  if(data.settlement==='unknown'&&keys===baseKeys)return validated();
  if(data.settlement!=='verified'||(keys!==countKeys&&keys!==orderKeys&&keys!==metadataKeys)||typeof data.pendingCount!=='number'||!Number.isInteger(data.pendingCount)||
    data.pendingCount<0||data.pendingCount>8||
    typeof data.pendingPayments!=='number'||!Number.isInteger(data.pendingPayments)||data.pendingPayments<0||
    typeof data.pendingLimits!=='number'||!Number.isInteger(data.pendingLimits)||data.pendingLimits<0||
    data.pendingPayments+data.pendingLimits!==data.pendingCount||typeof data.basisAt!=='string'||typeof data.basisUntil!=='string'||
    !(Date.parse(data.basisAt)<=now)||!(Date.parse(data.basisUntil)>now)||
    Date.parse(data.basisUntil)>Date.parse(data.assignmentUntil))throw Error('RETURN_NOT_VERIFIED');
  if(keys===metadataKeys&&(typeof data.tokenDecimals!=='number'||!Number.isInteger(data.tokenDecimals)||
    data.tokenDecimals<0||data.tokenDecimals>36||typeof data.tokenSymbol!=='string'||!/^[A-Za-z0-9][A-Za-z0-9._-]{0,11}$/.test(data.tokenSymbol)))throw Error('RETURN_NOT_VERIFIED');
  if(keys===orderKeys||keys===metadataKeys){
    if(!Array.isArray(data.pendingOrders)||data.pendingOrders.length>data.pendingPayments)throw Error('RETURN_NOT_VERIFIED');
    const seen=new Set<string>();
    for(const order of data.pendingOrders){
      if(!order||Object.keys(order).sort().join()!=='amount,asset,origin'||
        typeof order.origin!=='string'||!assignmentRef.test(order.origin)||seen.has(order.origin)||
        typeof order.asset!=='string'||!accountRef.test(order.asset)||
        typeof order.amount!=='string'||!/^[1-9][0-9]{0,77}$/.test(order.amount)||BigInt(order.amount)>maxUint256)throw Error('RETURN_NOT_VERIFIED');
      seen.add(order.origin);
    }
  }
  return validated();
}

export function RenterReturnStatus({language,active,detailVisible=false,navigateOriginal}:{
  language:'ko'|'en';active:boolean;detailVisible?:boolean;navigateOriginal?:(detail:boolean)=>void;
}) {
  const t=(ko:string,en:string)=>language==='ko'?ko:en;
  const [assignment,setAssignment]=useState('');
  const [account,setAccount]=useState('');
  const [openOrigin,setOpenOrigin]=useState<string>();
  const [status,setStatus]=useState<VerifiedReturn>();
  const [busy,setBusy]=useState(false);
  const [failed,setFailed]=useState(false);
  const revision=useRef(0);
  const profile=useRef<string|null|undefined>(undefined);
  const invalidate=useCallback(()=>{revision.current++;},[]);
  const clearIdentity=useCallback(()=>{
    invalidate();
    setStatus(undefined);setOpenOrigin(undefined);setAssignment('');setAccount('');setBusy(false);setFailed(false);
  },[invalidate]);

  useLayoutEffect(()=>{
    if(!active){clearIdentity();profile.current=undefined;return;}
    let alive=true,checking=false;
    async function checkProfile() {
      if(checking)return;
      checking=true;
      try {
        const current=await Http?.loadProfile('customer')||null;
        if(!alive)return;
        if(profile.current!==undefined&&profile.current!==current)clearIdentity();
        profile.current=current;
      }catch{
        if(alive){clearIdentity();profile.current=null;}
      }finally{checking=false;}
    }
    void checkProfile();
    const timer=setInterval(()=>{void checkProfile();},1000);
    return ()=>{alive=false;clearInterval(timer);invalidate();};
  },[active,clearIdentity,invalidate]);

  useLayoutEffect(()=>{
    if(!status)return;
    const expiry=Date.parse(status.assignmentUntil);
    const timer=setTimeout(clearIdentity,Math.min(2_147_483_647,Math.max(0,expiry-Date.now())));
    return ()=>clearTimeout(timer);
  },[status,clearIdentity]);

  useLayoutEffect(()=>{
    if(status?.settlement!=='verified')return;
    const basisUntil=status.basisUntil;
    const timer=setTimeout(()=>setStatus(current=>{
      if(current?.settlement!=='verified'||current.basisUntil!==basisUntil)return current;
      const {assignment:reference,account:address,generation,assignmentUntil,slot,credential,slotObservation}=current;
      return {assignment:reference,account:address,generation,assignmentUntil,slot,credential,slotObservation,settlement:'unknown'};
    }),Math.min(2_147_483_647,Math.max(0,Date.parse(basisUntil)-Date.now())));
    return ()=>clearTimeout(timer);
  },[status]);

  const credential=status?.credential;
  useLayoutEffect(()=>{
    if(!credential)return;
    const timer=setTimeout(()=>setStatus(current=>current?.credential===credential?
      {...current,credential:undefined}:current),Math.max(0,credential.evidenceUntil-Date.now()));
    return ()=>clearTimeout(timer);
  },[credential]);

  const slotObservation=status?.slotObservation;
  useLayoutEffect(()=>{
    if(!slotObservation)return;
    const timer=setTimeout(()=>setStatus(current=>current?.slotObservation===slotObservation?
      {...current,slotObservation:undefined}:current),Math.max(0,slotObservation.evidenceUntil-Date.now()));
    return ()=>clearTimeout(timer);
  },[slotObservation]);

  function clear() {
    invalidate();
    setStatus(undefined);setOpenOrigin(undefined);setFailed(false);setBusy(false);
  }

  async function read(accept:boolean,origin?:string) {
    const expected=assignment.trim().toLowerCase();
    const confirmed=accept?account.trim().toLowerCase():'';
    if(!active||busy||!assignmentRef.test(expected)||(accept&&!accountRef.test(confirmed)))return;
    const now=++revision.current;
    setStatus(undefined);setOpenOrigin(undefined);setFailed(false);setBusy(true);
    try {
      if(!Http) throw Error('RETURN_NOT_VERIFIED');
      const raw=await Http.loadProfile('customer');
      if(revision.current!==now)return;
      if(!raw||(profile.current!==undefined&&profile.current!==raw))throw Error('RETURN_PROFILE_CHANGED');
      profile.current=raw;
      const reply=await Http.exchangeRenterReturn(bytesToHex(sha256(utf8Encode(raw)),false),expected,confirmed);
      if(await Http.loadProfile('customer')!==raw)throw Error('RETURN_PROFILE_CHANGED');
      const verified=verifiedReturn(reply,expected,confirmed);
      if(revision.current===now){
        setStatus(verified);setAccount('');
        if(origin&&verified.settlement==='verified'&&verified.pendingOrders?.some(order=>order.origin===origin)){
          setOpenOrigin(origin);
          navigateOriginal?.(true);
        }
      }
    }catch{
      if(revision.current===now){clearIdentity();setFailed(true);}
    }finally{
      if(revision.current===now)setBusy(false);
    }
  }

  const button=(label:string,enabled:boolean,onPress:()=>void)=><Pressable accessibilityRole="button"
    accessibilityLabel={label} accessibilityState={{disabled:!enabled}} disabled={!enabled} onPress={onPress}
    style={{minHeight:44,justifyContent:'center'}}><Text>{label}</Text></Pressable>;
  const reference=assignmentRef.test(assignment.trim().toLowerCase());
  const currentCredential=active&&credential&&Date.now()<credential.evidenceUntil?credential:undefined;
  const renderTime=Date.now();
  const currentSlot=active&&slotObservation&&slotObservation.checkedAt<=renderTime&&
    renderTime<slotObservation.evidenceUntil?slotObservation:undefined;
  const linked=status?.settlement==='verified'?(status.pendingOrders??[]):[];
  const original=linked.find(order=>order.origin===openOrigin);
  useLayoutEffect(()=>{
    if(detailVisible&&(!original||!active))navigateOriginal?.(false);
    if(!detailVisible&&openOrigin)setOpenOrigin(undefined);
  },[active,detailVisible,navigateOriginal,openOrigin,original]);
  if(detailVisible && original && status?.settlement==='verified'){
    const token=typeof status.tokenDecimals==='number'&&status.tokenSymbol?
      {decimals:status.tokenDecimals,symbol:status.tokenSymbol}:undefined;
    return <RentalOriginalOrderDetail order={original} language={language} token={token}
      back={button(t('대여 반납 상태로 돌아가기','Back to rental status'),active,()=>{
        setOpenOrigin(undefined);navigateOriginal?.(false);
      })}/>;
  }
  return <View style={{gap:8}}>
    <TextInput accessibilityLabel={t('대여 인도 참조','Rental handover reference')} value={assignment}
      onChangeText={next=>{clear();setAssignment(next);}} autoCapitalize="none" autoCorrect={false} maxLength={36}
      placeholder={t('대여 인도 참조','Rental handover reference')}/>
    {button(t('같은 대여 상태 조회','Check this rental'),active&&!busy&&reference,()=>{void read(false);})}
    <TextInput accessibilityLabel={t('인도받은 대여 계정','Handed-over rental account')} value={account}
      onChangeText={next=>{clear();setAccount(next);}} autoCapitalize="none" autoCorrect={false} maxLength={42}
      placeholder={t('인도받은 대여 계정','Handed-over rental account')}/>
    {button(t('이 계정의 대여 인도 수락','Accept handover for this rental account'),active&&!busy&&reference&&accountRef.test(account.trim().toLowerCase()),()=>{void read(true);})}
    {failed&&<Text accessibilityLiveRegion="polite">{t('이 대여의 상태를 확인할 수 없습니다.','This rental status could not be verified.')}</Text>}
    {status&&<View>
      <Text>{t('원래 대여 세대','Original rental generation')} · {status.generation}</Text>
      <Text>{t('원래 대여 계정','Original rental account')} · {status.account}</Text>
      <Text>{t('인도 확인 유효기한','Handover confirmation expires')} · {new Date(status.assignmentUntil).toLocaleString()}</Text>
      {currentSlot?<>
        <Text>{t('대여 슬롯 · 열림','Rental slot · Open')}</Text>
        <Text>{t('슬롯 확인 시각','Slot checked at')} · {new Date(currentSlot.checkedAt).toLocaleString(t('ko-KR','en-US'))}</Text>
        <Text>{t('슬롯 확인 유효기한','Slot evidence valid until')} · {new Date(currentSlot.evidenceUntil).toLocaleString(t('ko-KR','en-US'))}</Text>
      </>:<Text>{t('대여 슬롯 · 확인 불가','Rental slot · Unconfirmed')}</Text>}
      {status.settlement==='verified'?
        <>
          <Text>{t('미결 지급','Pending payments')} · {status.pendingPayments}</Text>
          <Text>{t('미결 한도 변경','Pending limit changes')} · {status.pendingLimits}</Text>
          <Text>{t('미결 근거 시각','Pending assessment time')} · {new Date(status.basisAt).toLocaleString()}</Text>
        </>:<Text>{t('지급 미결 건수를 확인할 수 없습니다.','Pending payments could not be verified.')}</Text>}

    </View>}
    {currentCredential?<>
      <Text>{t('대여 패스키 사용기한','Rental passkey expires')} · {new Date(currentCredential.expiresAt).toLocaleString(t('ko-KR','en-US'))}</Text>
      <Text>{t('패스키 근거 유효기한','Passkey evidence valid until')} · {new Date(currentCredential.evidenceUntil).toLocaleString(t('ko-KR','en-US'))}</Text>
    </>:<>
      <Text>{t('대여 패스키 사용기한 · 확인 불가','Rental passkey validity · Unconfirmed')}</Text>
      <Text>{t('이 대여의 기한을 확인할 수 있는 유효한 증거가 없습니다. 대여 담당자에게 이 기기와 대여 세대의 패스키 사용기한을 확인하세요.',
        'Current evidence does not establish this rental passkey’s validity. Ask the rental operator to verify the passkey validity for this device and rental generation.')}</Text>
    </>}
    {linked.map(order=><View key={order.origin}>
      {button(t('미결 원거래 '+order.origin,'Open original pending payment '+order.origin),active&&!busy,()=>{void read(false,order.origin);})}
    </View>)}
    {status?.settlement==='verified'?
      (status.pendingPayments===linked.length&&linked.length>0?null:<Text>{status.pendingPayments===0?
        t('조회할 미결 지급이 없습니다.','There are no pending payments to open.'):
        t('미결 지급의 원거래 연결을 확인 중입니다.','Links to the original pending payments are awaiting verification.')}</Text>):
      <Text>{t('이 대여에 연결된 원주문을 확인할 수 없습니다.','Original orders for this rental are not verified.')}</Text>}
  </View>;
}
