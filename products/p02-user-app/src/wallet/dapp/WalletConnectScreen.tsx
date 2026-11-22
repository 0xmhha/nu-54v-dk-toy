import {useCallback,useEffect,useRef,useState} from 'react';
import {Alert,AppState,Keyboard,Pressable,StyleSheet,Text,TextInput,View} from 'react-native';
import {bytesToHex,hexToBytes} from '@nu54/protocol';
import type {DappScope} from './gate.ts';
import {parsePairing} from './walletConnectPolicy.ts';
import {relayAuthorization,seal} from './walletConnectCrypto.ts';
import {IrnRelay} from './walletConnectRelay.ts';
import {type SessionOptions,WalletConnectSession} from './walletConnectSession.ts';
import type {StoredConnect} from '../profile.ts';
import {advancedRequestRefused} from '../walletCapabilities.ts';
import PairingCamera from '../../specs/NativeShopQr.ts';

const now=()=>Math.floor(Date.now()/1000);
const styles=StyleSheet.create({button:{padding:12,minHeight:44,justifyContent:'center'},label:{color:'#2658a5'},
  input:{minHeight:44,borderWidth:1,borderColor:'#78828e',borderRadius:6,paddingHorizontal:10}});

export function reviewPrompt(title:string,body:string,signal:AbortSignal) {
  return new Promise<boolean>(resolve=>{
    if (signal.aborted) {resolve(false);return;}
    const done=(approved:boolean)=>{
      signal.removeEventListener('abort',abort);
      resolve(approved&&!signal.aborted);
    };
    const abort=()=>done(false);
    signal.addEventListener('abort',abort,{once:true});
    Alert.alert(title,body,[{text:'거절',style:'cancel',onPress:()=>done(false)},{text:'승인',onPress:()=>done(true)}],
      {cancelable:true,onDismiss:()=>done(false)});
  });
}

export function reviewAllowancePrompt(title:string,body:string,signal:AbortSignal,connectionLabel='연결 보기') {
  return new Promise<'approve'|'reject'|'connections'>(resolve=>{
    if (signal.aborted) {resolve('reject');return;}
    const done=(decision:'approve'|'reject'|'connections')=>{
      signal.removeEventListener('abort',abort);
      resolve(signal.aborted ? 'reject' : decision);
    };
    const abort=()=>done('reject');
    signal.addEventListener('abort',abort,{once:true});
    Alert.alert(title,body,[
      {text:connectionLabel,onPress:()=>done('connections')},
      {text:'거절',style:'cancel',onPress:()=>done('reject')},
      {text:'승인',onPress:()=>done('approve')},
    ],{cancelable:true,onDismiss:abort});
  });
}

function openRelay(seed:Uint8Array,relay:string,projectId:string|undefined,nonce:Uint8Array) {
  const authorization=relayAuthorization(seed,nonce,relay,now());
  return new IrnRelay(relay+'?auth='+encodeURIComponent(authorization)+(projectId ? '&projectId='+projectId : ''));
}

export async function notifyStoredDisconnect(stored:StoredConnect,relay:string,projectId:string|undefined,random:SessionOptions['random']) {
  const {checkpoint}=stored;
  if (stored.relay!==relay || checkpoint.expires<=now()) return;
  const transport=openRelay(hexToBytes(stored.seed),relay,projectId,await random(32));
  try {
    const packet={jsonrpc:'2.0',id:now()*1000,method:'wc_sessionDelete',
      params:{code:6000,message:'Wallet disconnected'}};
    const message=seal(hexToBytes(checkpoint.key),await random(12),packet);
    await transport.publish(checkpoint.topic,message,1112,86400);
  } finally {transport.close();}
}

type Contact=Pick<SessionOptions,'random'|'trust'|'request'|'allowancePolicy'|'allowanceSubmitted'|'viewAllowanceConnection'>;
export function WalletConnectScreen({scope,relay,projectId,stored,save,boards,scan,back,reviewNewConnection,openConnections,verify,random,trust,request,allowancePolicy,allowanceSubmitted,viewAllowanceConnection}:Contact&{
  scope:DappScope;
  relay:string;
  projectId?:string;
  stored?:StoredConnect;
  save:(value:StoredConnect|undefined,topic?:string,stillCurrent?:()=>boolean)=>Promise<void>;
  boards:readonly {address:string;name:string}[];
  scan:()=>void;
  back:()=>void;
  reviewNewConnection?:()=>void;
  openConnections?:()=>void;
  verify:(handle:string)=>Promise<void>;
  request:SessionOptions['request'];
}) {
  const [uri,setUri]=useState('');
  const [status,setStatus]=useState('');
  const [loginNonce,setLoginNonce]=useState('');
  const [reviewRequired,setReviewRequired]=useState(false);
  const [active,setActive]=useState(false);
  const [spend,setSpend]=useState(false);
  const current=useRef<WalletConnectSession|undefined>(undefined);
  const camera=useRef(false);
  const lifecycle=useRef({revision:0}).current;
  const actions=useRef({save,trust,request,allowanceSubmitted,viewAllowanceConnection});
  actions.current={save,trust,request,allowanceSubmitted,viewAllowanceConnection};
  const {board,generation,account,chain,genesis}=scope;
  const storedDeadline=stored?.checkpoint.expires;
  const liveStored=stored && stored.checkpoint.expires>now() ? stored : undefined;
  const requireReview=useCallback(()=>{
    lifecycle.revision++;
    setActive(false);
    setUri('');
    setSpend(false);
    setStatus('');
    setReviewRequired(true);
  },[lifecycle]);
  useEffect(()=>{
    if (storedDeadline===undefined) return;
    const timer=setTimeout(()=>{
      if (!current.current) requireReview();
    },Math.max(0,storedDeadline*1000-Date.now()));
    return ()=>clearTimeout(timer);
  },[storedDeadline,requireReview]);
  useEffect(()=>{
    const background=AppState.addEventListener('change',state=>{
      if (state==='active' || camera.current) return;
      lifecycle.revision++;
      current.current?.suspend();
      current.current=undefined;
      setActive(false);
      setStatus('연결을 일시 중지했습니다. 다시 연결하려면 기기를 확인하세요.');
    });
    return ()=>{lifecycle.revision++;background.remove();current.current?.suspend();current.current=undefined;};
  },[board,generation,account,chain,genesis,relay,lifecycle]);

  async function removeStored() {
    if (!stored) return;
    await actions.current.save(undefined,stored.checkpoint.topic);
    setActive(false);
    setStatus('저장된 외부 DApp 권한을 삭제했습니다.');
    try {await notifyStoredDisconnect(stored,relay,projectId,random);}
    catch {setStatus('이 기기의 권한을 삭제했습니다. 상대 연결은 만료될 때 종료됩니다.');}
  }

  async function scanPairing() {
    if (!PairingCamera || camera.current || active || liveStored || reviewRequired) return;
    const operation=lifecycle.revision;
    camera.current=true;
    try {
      const value=await PairingCamera.scan();
      if (!value || operation!==lifecycle.revision) return;
      const input=value.trim();
      parsePairing(input,now());
      setUri(input);
      setStatus('연결 주소를 읽었습니다. 기기를 확인한 뒤 요청 범위를 검토하세요.');
    } catch {
      if (operation===lifecycle.revision) setStatus('QR을 읽을 수 없습니다. 연결 주소를 입력하거나 붙여 넣으세요.');
    } finally {camera.current=false;}
  }

  async function start(handle:string,resume:boolean) {
    if (camera.current || active || reviewRequired) return;
    const operation=++lifecycle.revision;
    const input=uri.trim();
    Keyboard.dismiss();
    setUri('');
    setActive(true);
    try {
      if (!resume) parsePairing(input,now());
      await verify(handle);
      if (operation!==lifecycle.revision) return;
      if (!resume && current.current) {
        setStatus('외부 DApp의 연결 제안을 기다리고 있습니다.');
        await current.current.pair(input,spend);
        return;
      }
      const saved=resume && liveStored?.relay===relay ? liveStored : undefined;
      if (resume && !saved) throw new Error('WC_RELAY_CHANGED');
      const seed=saved ? hexToBytes(saved.seed) : await random(32);
      const rtc=openRelay(seed,relay,projectId,await random(32));
      if (operation!==lifecycle.revision) {rtc.close();return;}
      const session=new WalletConnectSession({rtc,scope,clock:now,random,allowSpend:spend,allowancePolicy,
        allowanceSubmitted:()=>actions.current.allowanceSubmitted?.(),
        viewAllowanceConnection:origin=>actions.current.viewAllowanceConnection?.(origin),
        loginReturned:setLoginNonce,
        pairingRefused:()=>{
          setActive(false);
          setStatus('연결 제안을 거절했습니다. 새 연결 주소를 사용할 수 있습니다.');
        },
        expired:()=>{
          if (current.current===session) requireReview();
        },
        advancedRefused:reason=>setStatus(advancedRequestRefused[reason]+' 서명 0건 · 제출 0건'),
        save:async (checkpoint,topic)=>{
          if (current.current!==session) throw new Error('WC_CLOSED');
          await actions.current.save(checkpoint ? {relay,seed:bytesToHex(seed,false),checkpoint} : undefined,topic,()=>current.current===session);
          if (!checkpoint && current.current===session) {
            setActive(false);setStatus('외부 DApp 연결이 종료되었습니다.');current.current=undefined;
          }
        },
        trust:(pairing,proposal)=>actions.current.trust(pairing,proposal),
        approve:(proposal,signal)=>reviewPrompt('외부 DApp 연결',
          proposal.origin+'\n'+(proposal.verified?'등록된 출처':'출처 미확인 · 계정 조회만')+'\n'+
          proposal.account+'\n'+proposal.methods.join(', ')+'\n최대 10분',signal),
        request:(action,proposal,signal,topic,expiry)=>actions.current.request(action,proposal,signal,topic,expiry)});
      current.current=session;
      if (saved) {
        await session.resume(saved.checkpoint);
        if (operation===lifecycle.revision) setStatus('외부 DApp에 다시 연결했습니다.');
      } else {
        setStatus('외부 DApp의 연결 제안을 기다리고 있습니다.');
        await session.pair(input,spend);
      }
    } catch (error) {
      if (operation!==lifecycle.revision) return;
      if (error instanceof Error && error.message==='WC_PAIRING_DECIDED') {
        setActive(false);
        setStatus('이미 검토한 주소입니다. 외부 DApp에서 새 연결 주소를 받으세요.');
        return;
      }
      current.current?.suspend();
      current.current=undefined;
      setActive(false);
      setStatus('연결 주소, 기기 또는 만료 시간을 확인하세요.');
    }
  }

  const button=(label:string,onPress:()=>void,disabled=false)=><Pressable accessibilityRole="button"
    accessibilityState={{disabled}} accessibilityLabel={label} onPress={onPress} disabled={disabled} style={styles.button}>
    <Text style={styles.label}>{label}</Text></Pressable>;
  if (reviewRequired) return <View>
    <Text accessibilityRole="header">다시 동의 필요</Text>
    <Text accessibilityLiveRegion="polite">기존 연결로 요청할 수 없습니다. 출처와 요청 범위를 다시 검토하세요.</Text>
    {button('새 연결 검토',()=>{
      current.current=undefined;
      setStatus('');
      setReviewRequired(false);
      reviewNewConnection?.();
    })}
    {button('종료',back)}
  </View>;
  return <View>
    <Text accessibilityRole="header">외부 DApp 연결</Text>
    <Text>체인 {chain} · {account}</Text>
    {!!loginNonce && <>
      <Text accessibilityRole="header">사이트 로그인 서명을 반환했습니다</Text>
      <Text selectable>메시지 · {loginNonce}</Text>
      <Text>온체인 지출 · 0건. 다음 지출 요청은 별도로 확인합니다.</Text>
      {openConnections && button('연결 목록',openConnections)}
    </>}
    {!active && <>
      <TextInput style={styles.input} accessibilityLabel="WalletConnect 연결 주소" value={uri} onChangeText={setUri} maxLength={1200}
        autoCorrect={false} autoCapitalize="none" secureTextEntry returnKeyType="done" onSubmitEditing={Keyboard.dismiss} />
      {PairingCamera ? button('WalletConnect QR 읽기',()=>{Keyboard.dismiss();scanPairing();},!!liveStored) :
        <Text>이 배포에서 QR을 읽을 수 없습니다. 연결 주소를 입력하거나 붙여 넣으세요.</Text>}
      {button('주소 입력 완료',Keyboard.dismiss,!uri.trim())}
      {button(spend?'송금 요청 허용 중 · 취소':'등록된 출처의 송금 요청 허용',()=>setSpend(!spend))}
      {button('현재 보드 찾기',()=>{Keyboard.dismiss();scan();})}
      {boards.map(item=><View key={item.address}><Text>{item.name}</Text>
        {button('외부 DApp 연결 검토',()=>{start(item.address,false);},!uri || !!liveStored)}
        {!!liveStored && button('외부 DApp 다시 연결',()=>{start(item.address,true);})}
      </View>)}
    </>}
    {(active || liveStored) && button('외부 DApp 연결 해제',()=>{
      lifecycle.revision++;
      const session=current.current;
      if (session) session.disconnect().then(()=>{
        if (current.current===session) {
          current.current=undefined;
          setActive(false);
          setStatus('외부 DApp 연결이 종료되었습니다.');
        }
      }).catch(()=>{setStatus('연결 해제 시 네트워크 확인이 필요합니다.');});
      else removeStored().catch(()=>setStatus('연결 해제를 저장하지 못했습니다.'));
    })}
    {button('지갑으로 돌아가기',back)}
    <Text accessibilityLiveRegion="polite">{status}</Text>
  </View>;
}
