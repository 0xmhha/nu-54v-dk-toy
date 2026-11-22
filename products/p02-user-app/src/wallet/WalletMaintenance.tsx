import {inputPlaceholder} from '../ui/theme.ts';
import {useLayoutEffect,useRef,useState} from 'react';
import {Pressable,StyleSheet,Text,TextInput,View} from 'react-native';
import {RenterReturnStatus} from './RenterReturnStatus.tsx';
import {verifyFirmwareCandidate,type FirmwareCandidate,type FirmwareCandidateTrust} from './firmwareCandidate.ts';
import type {FirmwareInputPower} from './firmwarePowerWire.ts';
import type {RunningFirmware} from './firmwareStatusWire.ts';
import type {OwnerStatus} from './ownerWire.ts';
import {boundMaintenanceBoard,maintenanceDecision,type MaintenanceScope} from './walletMaintenance.ts';

type Device = {address:string;name?:string};
type Props = {
  language:'ko'|'en'; scope:MaintenanceScope; active:boolean; signing:boolean; unsettled:boolean;
  approvalHold?:'board'|'unknown';
  firmwareOnly?:boolean;
  networkBusy:boolean; devices:Device[]; scan:()=>void; read:(handle:string)=>Promise<OwnerStatus>;
  runningFirmware?:(handle:string,board:OwnerStatus)=>Promise<RunningFirmware|undefined>;
  candidateTrust?:FirmwareCandidateTrust;
  pickCandidate?:()=>Promise<string>;
  shareCandidateContext?:(context:string)=>Promise<void>;
  inputPower?:(handle:string,board:OwnerStatus)=>Promise<FirmwareInputPower>;
  tools?:(handle:string,board:OwnerStatus)=>Promise<boolean>;
  find?:(handle:string,board:OwnerStatus,start:boolean)=>Promise<boolean>;
  speak?:(text:string)=>Promise<void>;
  campaign:(handle:string,request:string,board:OwnerStatus)=>Promise<string>;
  back:()=>void; backLabel?:string; backup:()=>void; orders:()=>void; rental?:()=>void;
  rentalOriginalDetail?:boolean; navigateRentalOriginal?:(detail:boolean)=>void;
};

function Action({label,run,disabled=false}:{label:string;run:()=>void;disabled?:boolean}) {
  return <Pressable accessibilityRole="button" accessibilityLabel={label} accessibilityState={{disabled}}
    disabled={disabled} onPress={run} style={styles.action}><Text>{label}</Text></Pressable>;
}

export function WalletMaintenance(props:Props) {
  const {language,scope,active,signing,unsettled,approvalHold,networkBusy,devices,scan,read,runningFirmware,inputPower,tools,find,speak,campaign,back,backup,orders,rental}=props;
  const t=(ko:string,en:string)=>language==='ko'?ko:en;
  const [route,setRoute]=useState<'update'|'erase'|'tools'>('update');
  const [preparing,setPreparing]=useState(false);
  const [board,setBoard]=useState<{handle:string;status:OwnerStatus}>();
  const [running,setRunning]=useState<RunningFirmware>();
  const [candidate,setCandidate]=useState<FirmwareCandidate>();
  const [candidateDocument,setCandidateDocument]=useState('');
  const [power,setPower]=useState<FirmwareInputPower>('unknown');
  const [request,setRequest]=useState('');
  const [findAvailable,setFindAvailable]=useState<boolean>();
  const [signal,setSignal]=useState(false);
  const [proof,setProof]=useState('');
  const [issue,setIssue]=useState('');
  const [busy,setBusy]=useState(false);
  const revision=useRef({value:0}).current;
  useLayoutEffect(()=>{
    revision.value++;
    setBoard(undefined);setRunning(undefined);setCandidate(undefined);setCandidateDocument('');setPower('unknown');setRequest('');setProof('');setIssue('');setBusy(false);setFindAvailable(undefined);setSignal(false);
    return ()=>{revision.value++;};
  },[active,revision,scope.board,scope.generation,scope.mode,signing,approvalHold,unsettled,props.candidateTrust]);
  const signedWorkSummary=approvalHold==='board'||board?.status.state==='pending'
    ? t('기기 승인 진행 · 업데이트 적용 보류','Approval in progress. Firmware update is on hold.')
    : t('승인 진행 또는 결과 미확인 · 업데이트 적용 보류','Approval in progress or unresolved. Firmware update is on hold.');
  const verdict=maintenanceDecision(scope,board?.status,signing,unsettled);
  const working=busy||networkBusy;
  const candidateNow=Math.floor(Date.now()/1000);
  const currentCandidate=active&&!unsettled&&verdict.update==='ready-to-read'&&candidate&&
    candidate.issuedAt<=candidateNow&&candidateNow<candidate.expiresAt?candidate:undefined;
  const task=async(run:(current:()=>boolean)=>Promise<void>)=>{
    const generation=++revision.value;
    const current=()=>revision.value===generation;
    setBusy(true);setIssue('');
    try{await run(current);}
    catch{if(current()){setBoard(undefined);setRunning(undefined);setCandidate(undefined);setCandidateDocument('');setPower('unknown');setProof('');setFindAvailable(undefined);setSignal(false);setIssue(t(
      '인증된 기기 상태를 확인할 수 없습니다. 진행 중인 작업을 확인하고 다시 조회하세요.',
      'Could not verify device status. Check any current approval and retry.'));}}
    finally{if(current())setBusy(false);}
  };
  useLayoutEffect(()=>{
    if(!signal)return;
    const timeout=setTimeout(()=>setSignal(false),4500);
    return ()=>clearTimeout(timeout);
  },[signal]);

  useLayoutEffect(()=>{
    if(!candidate)return;
    const timeout=setTimeout(()=>setCandidate(undefined),Math.max(0,candidate.expiresAt*1000-Date.now()));
    return ()=>clearTimeout(timeout);
  },[candidate]);

  function inspectCandidate(readDocument:()=>Promise<string>) {
    const trust=props.candidateTrust;
    if(!board||!running||!runningFirmware||!trust||working||!active||unsettled||
        verdict.update!=='ready-to-read')return;
    setCandidate(undefined);
    setCandidateDocument('');
    task(async current=>{
      const raw=await readDocument().catch(()=>'');
      if(!raw||!current())return;
      const latest=await runningFirmware(board.handle,board.status).catch(()=>undefined);
      if(!current())return;
      setRunning(latest);
      if(!latest)return;
      try{
        setCandidate(verifyFirmwareCandidate(raw,trust,board.status,latest,Math.floor(Date.now()/1000)));
      }catch{
        setIssue(t('서명 후보가 현재 기기 및 배포 정책과 일치하지 않습니다.',
          'The signed candidate does not match the current device and deployment policy.'));
      }
    });
  }

  function locate(start:boolean) {
    if(!find||!board||!findAvailable||!active||signing||working||board.status.state!=='active')return;
    task(async current=>{
      const accepted=await find(board.handle,board.status,start);
      if(!current())return;
      setSignal(start&&accepted);
      if(!accepted)setIssue(t('보드가 LED 요청을 보류했습니다.','The board deferred the LED request.'));
    });
  }

  if(approvalHold)return <View style={styles.section}>
    <Text accessibilityRole="header">{t('개인 보드 유지관리','Personal board maintenance')}</Text>
    <Text accessibilityLiveRegion="polite">{signedWorkSummary}</Text>
    <Text>{t('기존 작업이 끝날 때까지 기다리세요.','Wait for the original approval to finish.')}</Text>
    <Action label={props.backLabel ?? t('등록 상태로 돌아가기','Return to enrollment status')} run={back}/>
  </View>;

  if(scope.mode==='rental')return <View style={styles.section}>
    {!props.rentalOriginalDetail&&<>
    <Text accessibilityRole="header">{t('대여 보드 반납 상태','Rental board return')}</Text>
    <Text>{t('대여 반납은 인증된 운영자가 원래 대여 지급을 대사한 뒤 처리합니다. 개인 슬롯은 이 화면의 권한에 포함되지 않습니다.',
      'An authenticated operator handles the return after reconciling the original rental payments. This view has no access to personal slots.')}</Text>
    <Text accessibilityLiveRegion="polite">{t('대여 슬롯의 종료 확인을 받지 못했습니다. 대여 담당자에게 반납 상태를 확인하세요.',
      'Closure of the rental slot has not been confirmed. Check the return status with the rental operator.')}</Text>
    </>}
    <RenterReturnStatus language={language} active={active&&!networkBusy}
      detailVisible={props.rentalOriginalDetail} navigateOriginal={props.navigateRentalOriginal}/>
    {!props.rentalOriginalDetail&&<>
    {rental&&<Action label={t('기존 대여 보드 연결','Connect the existing rental board')} run={rental}/>}
    <Action label={t('프로필 선택','Wallet selection')} run={back}/>
    </>}
  </View>;
  return <View style={styles.section}>
    <Text accessibilityRole="header">{t('개인 보드 유지관리','Personal board maintenance')}</Text>
    {!props.firmwareOnly&&<>
      <Action label={t('펌웨어 업데이트 준비','Prepare firmware update')} disabled={working} run={()=>{setPreparing(true);setRoute('update');}}/>
      <Action label={t('개인 초기화 조건 확인','Check personal reset conditions')} disabled={working} run={()=>{setCandidate(undefined);setCandidateDocument('');setRoute('erase');}}/>
      <Action label={t('기기 부가기능','Device tools')} disabled={working} run={()=>{setCandidate(undefined);setCandidateDocument('');setRoute('tools');}}/>
      <Action label={t('등록 기기 찾기','Find enrolled device')} disabled={working||!active||signing||!scope.board} run={scan}/>
    </>}
    {devices.map(device=><Action key={device.address} disabled={working||!active||signing}
      label={t('인증된 기기 상태 조회','Verify device status')+' · '+(device.name||device.address.slice(-6))}
      run={()=>{setBoard(undefined);setRunning(undefined);setCandidate(undefined);setCandidateDocument('');setPower('unknown');setProof('');setFindAvailable(undefined);setSignal(false);task(async current=>{
        const status=await read(device.address);
        if(!boundMaintenanceBoard(scope,status))throw new Error('MAINTENANCE_SCOPE');
        if(current())setBoard({handle:device.address,status});
        if(route==='update'&&preparing&&runningFirmware&&status.state==='active'&&current()) {
          const evidence=await runningFirmware(device.address,status).catch(()=>undefined);
          if(current())setRunning(evidence);
        }
        if(route==='tools'&&tools&&status.state==='active'&&current()) {
          const capability=await tools(device.address,status).catch(()=>undefined);
          if(current())setFindAvailable(capability);
        }
      });}}/>)}
    {board&&<Text>{t('인증된 소유 세대','Verified owner generation')} · {board.status.generation.toString()}</Text>}
    {!!issue&&<Text accessibilityLiveRegion="polite">{issue}</Text>}
    {route==='tools'?<>
      <Text accessibilityRole="header">{t('지원 기능','Supported device tools')}</Text>
      {board&&<Text>{t('인증된 기기 지문 끝자리','Verified device fingerprint ending')} · {board.status.device.slice(-4).toUpperCase()}</Text>}
      <Text>{findAvailable===true?t('찾기 · 보드 LED','Find · board LED'):
        findAvailable===false?t('보드 LED 찾기 미지원','Board LED finding unavailable'):
          t('이 보드의 찾기 기능을 확인하지 못했습니다. 상태를 다시 조회하세요.','Board find capability is unverified. Verify device status again.')}</Text>
      {findAvailable!==undefined&&<Text>{t('보드 오디오 전송 미지원','Board audio transfer unavailable')}</Text>}
      {signal&&<Text accessibilityLiveRegion="polite">{t('보드가 LED 신호 요청을 받았습니다. 보드에서 직접 확인하세요.','The board acknowledged the LED request. Check the board directly.')}</Text>}
      <Action label={t('찾기 신호 요청','Request find signal')} disabled={working||!active||signing||!findAvailable||!board||signal} run={()=>locate(true)}/>
      <Action label={t('찾기 신호 취소','Cancel find signal')} disabled={working||!active||signing||!signal} run={()=>locate(false)}/>
      <Action label={t('휴대전화로 상태 듣기','Hear status on this phone')} disabled={working||!active||!speak||!board}
        run={()=>{if(speak)task(async()=>{await speak(signal?t('보드 LED 신호 요청을 받았습니다.','Board LED request acknowledged.'):
          t('휴대전화에서 상태를 읽습니다. 보드 오디오 전송은 진행 중이 아닙니다.','This phone is reading the status. There is no board audio transfer.'));});}}/>
    </>:route==='update'?<>
      {preparing&&<>
      <Text accessibilityRole="header">{t('업데이트 준비','Update preparation')}</Text>
      <Text accessibilityLiveRegion="polite">{t('현재 실행 버전','Currently running version')} · {
        running && active && verdict.update==='ready-to-read' ? running.version : t('확인 불가','Unconfirmed')}</Text>
      {running&&active&&verdict.update==='ready-to-read'&&
        <Text>{t('실행 이미지 보안 카운터','Running image security counter')} · {running.counter}</Text>}
      <Text accessibilityLiveRegion="polite">{currentCandidate
        ? `${t('서명 후보','Signed candidate')} · ${currentCandidate.version} · ${currentCandidate.channel} · ${t('보안 카운터','Security counter')} ${currentCandidate.counter}`
        : t('서명 후보 · 확인 불가','Signed candidate · Unconfirmed')}</Text>
      {props.candidateTrust&&props.candidateTrust.board===board?.status.device&&props.shareCandidateContext&&
        <Action label={t('이 보드의 후보 대조 자료 공유','Share this board’s candidate comparison data')}
          disabled={working||!active||unsettled||!running||verdict.update!=='ready-to-read'} run={()=>{
            if(!board||!running||working||!active||unsettled||verdict.update!=='ready-to-read')return;
            const {device,generation}=board.status;
            const {counter,measurement}=running;
            const context=JSON.stringify({board:device,generation:generation.toString(),counter,measurement});
            task(async()=>{await props.shareCandidateContext?.(context).catch(()=>{});});
          }}/>}
      {props.candidateTrust&&props.candidateTrust.board===board?.status.device&&running&&active&&
        !unsettled&&verdict.update==='ready-to-read'&&<>
        {props.pickCandidate&&<Action label={t('서명된 후보 확인서 선택','Choose signed candidate receipt')}
          disabled={working} run={()=>{if(props.pickCandidate)inspectCandidate(props.pickCandidate);}}/>}
        <TextInput placeholderTextColor={inputPlaceholder} style={styles.input} accessibilityLabel={t('서명된 후보 확인서','Signed candidate receipt')}
          value={candidateDocument} autoCapitalize="none" autoCorrect={false} multiline maxLength={8192} editable={!working}
          onChangeText={value=>{setCandidate(undefined);setCandidateDocument(value);}}/>
        <Action label={t('후보 확인서 검증','Verify candidate receipt')} disabled={working||!candidateDocument}
          run={()=>inspectCandidate(async()=>candidateDocument)}/>
      </>}
      {currentCandidate&&<Action label={t('후보 선택 취소','Clear candidate selection')} run={()=>setCandidate(undefined)}/>}
      <Text>{t('기기 전원 · 확인 불가','Device power · Unconfirmed')}</Text>
      <Text accessibilityLiveRegion="polite">{t('PMIC 입력','PMIC input')} · {
        active&&verdict.update==='ready-to-read'&&power!=='unknown'
          ? power==='vin-good' ? t('VIN 정상 범위','VIN within range') : t('VIN 조건 미충족','VIN condition not met')
          : t('확인 불가','Unconfirmed')}</Text>
      {inputPower&&<Action label={t('PMIC 입력 확인','Check PMIC input')}
        disabled={working||!active||!board||verdict.update!=='ready-to-read'} run={()=>{
          if (!board || working || !active || verdict.update!=='ready-to-read') return;
          setPower('unknown');
          task(async current=>{
            const value=await inputPower(board.handle,board.status).catch(()=>'unknown' as const);
            if(current())setPower(value);
          });
        }} />}
      <Text>{t('배터리 잔량 · 확인 불가','Battery charge · Unconfirmed')}</Text>
      <Text>{t('업데이트 적용 권한 · 보류','Firmware update authorization · On hold')}</Text>
      <Text>{currentCandidate
        ? t('기기 전원과 업데이트 적용 권한을 확인할 때까지 적용을 보류합니다.','Applying an update remains on hold until device power and update authorization are verified.')
        : t('후보와 기기 전원을 확인하기 전까지 업데이트 준비를 보류합니다.','Update preparation is on hold until the candidate and device power are verified.')}</Text>
      </>}
      {verdict.update==='approval-busy'
        ? <Text accessibilityLiveRegion="polite">{signedWorkSummary}</Text>
        : <Text>{t('운영 캠페인의 전원·모델·최소 허용 버전과 배포 서명을 확인하세요. 실행 버전은 기기의 적용 증거를 받아 승인된 캠페인에서 대조합니다.',
          'Check power, model, minimum version and release signature in the approved campaign. Verify the running version against authenticated board evidence there.')}</Text>}
      <Text accessibilityRole="header">{t('설치 후 캠페인 증거','Post-installation campaign evidence')}</Text>
      <TextInput placeholderTextColor={inputPlaceholder} style={styles.input} accessibilityLabel={t('펌웨어 적용 증거 요청','Firmware evidence request')}
        editable={!working&&active} autoCapitalize="none" autoCorrect={false} maxLength={98} value={request}
        onChangeText={value=>{setRequest(value);setProof('');}}/>
      <Action label={t('이 기기 적용 증거 읽기','Read this device rollout evidence')}
        disabled={working||!active||!board||verdict.update!=='ready-to-read'||!/^10[0-9a-f]{96}$/.test(request.trim())}
        run={()=>{if(!board)return;task(async current=>{
          const evidence=await campaign(board.handle,request.trim(),board.status);
          if(current())setProof(evidence);
        });}}/>
      {!!proof&&active&&verdict.update==='ready-to-read'&&<><Text>{t('캠페인 검증용 기기 증거','Board evidence for campaign verification')}</Text><Text selectable>{proof}</Text></>}
    </>:<>
      <Text accessibilityRole="header">{t('개인 슬롯 삭제 보류','Personal slot reset on hold')}</Text>
      <Text>{verdict.erase==='unresolved'?t('미결 지급 또는 서명 요청을 원래 주문에서 먼저 대사하세요.','Reconcile unresolved payments or approvals from their original requests.'):
        verdict.erase==='backup-unverified'?t('기기가 독립 복원 시험을 확인하지 않았습니다.','The board has not verified recovery.'):
        verdict.erase==='verify-board'?t('등록된 개인 보드 상태를 먼저 인증하세요.','Verify the enrolled personal board first.'):
        t('기기 정산과 별도 물리 확인이 필요합니다. 체인 자산은 원래 주소에 남습니다.','Board reconciliation and separate physical confirmation are required. Chain assets stay at the original address.')}</Text>
      <Action label={t('백업 확인','Check backup')} run={backup} disabled={working}/>
      <Action label={t('미결 지급 조회','View pending payments')} run={orders} disabled={working}/>
    </>}
    <Action label={props.backLabel ?? t('등록 상태로 돌아가기','Return to enrollment status')} run={back} disabled={working}/>
  </View>;
}

const styles=StyleSheet.create({
  section:{gap:12},
  action:{minHeight:44,borderWidth:1,borderColor:'#718399',justifyContent:'center',borderRadius:5,padding:8},
  input:{borderWidth:1,borderColor:'#718399',minHeight:44,padding:8},
});
