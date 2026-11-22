import {inputPlaceholder} from '../../ui/theme.ts';
import {useCallback,useEffect,useRef,useState} from 'react';
import {AppState,Keyboard,Pressable,Text,TextInput,View} from 'react-native';
import {bytesToHex,fromBase64} from '@nu54/protocol';
import NativeBrowser from '../../specs/NativeDappBrowser.ts';
import NativeWallet from '../../specs/NativeOwnerWallet.ts';
import {authorizeDappRequest,httpsOrigin,issueDappGrant,refusedAdvancedRequest,type DappGrant,type DappScope} from './gate.ts';
import {browserRpc,parseBrowserRpc} from './browserRpc.ts';
import {browserProvider} from './provider.ts';
import {evmProfiles,type DappAllowancePolicy,type EvmChain} from '../evmTransfer.ts';
import {advancedRequestRefused} from '../walletCapabilities.ts';

const now=()=>Math.floor(Date.now()/1000);

export function DappBrowserScreen({scope,boards,scan,verify,back,initialSite='',onSiteChange,openExternal,openConnections,openTravel,openMarket,openNetwork,loginOrigin,prepareLogin,signLogin,prepareSpend,prepareAllowance,allowancePolicy,signSpend,submitSpend,onAllowanceSubmitted,onViewConnection,abortLogin}:{
  scope?:DappScope;
  boards:readonly {address:string;name:string}[];
  scan:()=>void;
  verify:(handle:string)=>Promise<void>;
  back:()=>void;
  initialSite?:string;
  onSiteChange?:(site:string)=>void;
  openExternal?:()=>void;
  openConnections?:()=>void;
  openTravel?:()=>void;
  openMarket?:()=>void;
  openNetwork?:()=>void;
  loginOrigin?:string;
  prepareLogin?:(message:string,origin:string)=>Promise<{issued:unknown;nonce:string;expires:number}>;
  signLogin?:(handle:string,issued:unknown,message:string,origin:string,signal:AbortSignal)=>Promise<string>;
  prepareSpend?:(recipient:string,value:bigint)=>Promise<{wire:string;fee:bigint}>;
  allowancePolicy?:DappAllowancePolicy;
  prepareAllowance?:(token:string,spender:string,amount:bigint,origin:string,session:string,expires:number)=>
    Promise<{wire:string;fee:bigint;amountLabel:string}>;
  signSpend?:(handle:string,wire:string,signal:AbortSignal)=>Promise<string>;
  submitSpend?:(hash:string,signal:AbortSignal)=>Promise<void>;
  onAllowanceSubmitted?:()=>void;
  onViewConnection?:(origin:string)=>void;
  abortLogin:()=>void;
}) {
  const [site,setSite]=useState(initialSite);
  const [review,setReview]=useState('');
  const [allowSpend,setAllowSpend]=useState(false);
  const [message,setMessage]=useState('');
  const [loginNonce,setLoginNonce]=useState('');
  const [reviewRequired,setReviewRequired]=useState(false);
  const [busy,setBusy]=useState(false);
  const grant=useRef<DappGrant|undefined>(undefined);
  const selectedBoard=useRef('');
  const login=useRef<AbortController|undefined>(undefined);
  const deadline=useRef<ReturnType<typeof setTimeout>|undefined>(undefined);
  const revision=useRef(0);
  const operations=useRef({prepareLogin,signLogin,prepareSpend,prepareAllowance,allowancePolicy,signSpend,submitSpend,onAllowanceSubmitted,onViewConnection,abortLogin});
  operations.current={prepareLogin,signLogin,prepareSpend,prepareAllowance,allowancePolicy,signSpend,submitSpend,onAllowanceSubmitted,onViewConnection,abortLogin};
  const {board,account,generation,chain,genesis}=scope??{};
  const remove=useCallback(()=>{
    revision.current++;
    grant.current=undefined;
    login.current?.abort();
    login.current=undefined;
    operations.current.abortLogin();
    clearTimeout(deadline.current);
  },[]);
  const requireReview=useCallback(()=>{
    remove();
    setReview('');
    setAllowSpend(false);
    setMessage('');
    setReviewRequired(true);
  },[remove]);
  useEffect(()=>{
    if (board===undefined || account===undefined || generation===undefined || chain===undefined || genesis===undefined) return;
    const authorizedScope={board,account,generation,chain,genesis};
    const native=NativeBrowser;
    if (!native) return;
    const incoming=native.onRequest(frame=>{
      const captured=grant.current;
      if (!captured) return;
      if (captured.expires<=now()) {
        requireReview();
        native.close().catch(()=>{});
        return;
      }
      const handle=async()=>{
        const {prepareLogin,signLogin,prepareSpend,prepareAllowance,allowancePolicy,signSpend,submitSpend,onAllowanceSubmitted,onViewConnection}=operations.current;
        const {channel,request,document}=parseBrowserRpc(frame);
        const current=()=>grant.current===captured && captured.expires>now();
        const refused=refusedAdvancedRequest(captured,channel,authorizedScope,request,now());
        if (refused && current()) {
          setMessage(advancedRequestRefused[refused]+' 서명 0건 · 제출 0건');
          await native.respond(channel.session,document,JSON.stringify({id:request.id,
            error:{code:4200,message:'The wallet cannot verify this transaction method'}}));
          return;
        }
        if (request.method==='personal_sign' && prepareLogin && signLogin && channel.origin===loginOrigin &&
            captured.methods.includes('personal_sign') && !login.current) {
          const pending=new AbortController();
          login.current=pending;
          try {
            const action=authorizeDappRequest(captured,channel,authorizedScope,request,now());
            if (action.kind!=='login') throw new Error('DAPP_LOGIN');
            const proof=await prepareLogin(action.message,channel.origin);
            if (!current() || pending.signal.aborted || proof.expires<=now()) throw new Error('DAPP_CLOSED');
            const decision=await native.confirmAllowance(channel.session,document,'사이트 로그인 · '+channel.origin,
              account+'\nChain '+chain+'\nNonce '+proof.nonce+'\n기한 '+new Date(proof.expires*1000).toISOString()+
              '\n\n'+action.message);
            if (!current() || pending.signal.aborted || proof.expires<=now()) throw new Error('DAPP_CLOSED');
            if (decision==='connections') {
              await native.respond(channel.session,document,JSON.stringify({id:request.id,error:{code:4001,message:'Login review ended'}}));
              if (current() && !pending.signal.aborted) {
                remove();
                native.close().catch(()=>{});
                onViewConnection?.(channel.origin);
              }
              return;
            }
            if (decision!=='approve') throw new Error('DAPP_REFUSED');
            const signature=await signLogin(selectedBoard.current,proof.issued,action.message,channel.origin,pending.signal);
            if (!current() || pending.signal.aborted || proof.expires<=now()) throw new Error('DAPP_CLOSED');
            await native.respond(channel.session,document,JSON.stringify({id:request.id,result:signature}));
            if (current()) setLoginNonce(proof.nonce);
          } catch {
            if (current()) await native.respond(channel.session,document,JSON.stringify({id:request.id,error:{code:4001,message:'Login was not approved'}}));
          } finally {
            if (login.current===pending) login.current=undefined;
          }
          return;
        }
        if (request.method==='eth_sendTransaction' && captured.methods.includes('eth_sendTransaction') &&
            prepareSpend && signSpend && submitSpend && !login.current) {
          const pending=new AbortController();
          login.current=pending;
          try {
            const action=authorizeDappRequest(captured,channel,authorizedScope,request,now(),
              channel.origin===loginOrigin ? allowancePolicy : undefined);
            const check='\nCheck these details on the independent display before approving on the board.';
            let quote:{wire:string;fee:bigint}, title:string, terms:string;
            if (action.kind==='allowance' && prepareAllowance) {
              const allowance=await prepareAllowance(action.token,action.spender,action.amount,channel.origin,channel.session,captured.expires);
              quote=allowance;
              title='Approve token allowance · '+channel.origin;
              terms='Chain '+evmProfiles[String(chain) as EvmChain].label+' · '+chain+'\nToken '+action.token+'\nSpender '+action.spender+'\nAllowance '+allowance.amountLabel;
            } else if (action.kind==='spend') {
              quote=await prepareSpend(action.recipient,action.value);
              title='Transfer · '+channel.origin;
              terms='Chain '+chain+'\nTo '+action.recipient+'\nAmount '+action.value+' wei';
            } else throw new Error('DAPP_SPEND');
            if (!current() || pending.signal.aborted) throw new Error('DAPP_CLOSED');
            const details=terms+'\nMaximum fee '+quote.fee+' wei'+check;
            if (action.kind==='allowance') {
              const decision=await native.confirmAllowance(channel.session,document,title,details);
              if (decision==='connections' && current() && !pending.signal.aborted) {
                await native.respond(channel.session,document,JSON.stringify({id:request.id,
                  error:{code:4001,message:'Allowance review ended. Send a new request to continue.'}}));
                if (current() && !pending.signal.aborted) {
                  remove();
                  native.close().catch(()=>{});
                  onViewConnection?.(channel.origin);
                }
                return;
              }
              if (decision!=='approve') throw new Error('DAPP_REFUSED');
            } else if (!await native.confirm(channel.session,document,title,details)) throw new Error('DAPP_REFUSED');
            if (!current() || pending.signal.aborted) throw new Error('DAPP_CLOSED');
            const hash=await signSpend(selectedBoard.current,quote.wire,pending.signal);
            if (!current() || pending.signal.aborted || !await native.confirm(channel.session,document,'Submit signed transaction',
                'Chain '+chain+'\n'+hash)) throw new Error('DAPP_REFUSED');
            if (!current() || pending.signal.aborted) throw new Error('DAPP_CLOSED');
            await submitSpend(hash,pending.signal);
            if (current() && !pending.signal.aborted) {
              await native.respond(channel.session,document,JSON.stringify({id:request.id,result:hash}));
              if (action.kind==='allowance' && current() && !pending.signal.aborted) onAllowanceSubmitted?.();
            }
          } catch {
            if (current()) await native.respond(channel.session,document,JSON.stringify({id:request.id,error:{code:4001,message:'Request not completed. Check the wallet for the transaction status.'}}));
          } finally {
            if (login.current===pending) login.current=undefined;
          }
          return;
        }
        const output=browserRpc(captured,authorizedScope,frame,now());
        await native.respond(output.session,output.document,output.reply);
      };
      handle().catch(()=>{});
    });
    const closed=native.onClosed(session=>{
      if (grant.current?.channel.session===session) requireReview();
    });
    const background=AppState.addEventListener('change',state=>{
      if (state!=='active') {
        if (grant.current) requireReview();
        else remove();
        native.close().catch(()=>{});
      }
    });
    return ()=>{remove();incoming.remove();closed.remove();background.remove();native.close().catch(()=>{});};
  },[board,account,generation,chain,genesis,loginOrigin,remove,requireReview]);

  async function open(handle:string) {
    if (busy || !review || reviewRequired || !scope || !NativeBrowser || !NativeWallet) return;
    const operation=++revision.current;
    setBusy(true);
    try {
      await verify(handle);
      if (operation!==revision.current) return;
      const session=bytesToHex(fromBase64(await NativeWallet.randomBytes(32)),false);
      if (operation!==revision.current) return;
      selectedBoard.current=handle;
      const issued=issueDappGrant({transport:'browser',session,origin:review,topLevel:true},scope,
        ['eth_accounts','eth_chainId',
          ...(review===loginOrigin && prepareLogin && signLogin ? ['personal_sign' as const] : []),
          ...(allowSpend && prepareSpend && signSpend && submitSpend ? ['eth_sendTransaction' as const] : [])],now(),now()+600);
      grant.current=issued;
      deadline.current=setTimeout(()=>{
        if (grant.current!==issued) return;
        requireReview();
        NativeBrowser?.close().catch(()=>{});
      },Math.max(0,issued.expires*1000-Date.now()));
      await NativeBrowser.open(site.trim(),session,browserProvider);
      if (grant.current===issued) setMessage('선택한 사이트에 계정과 네트워크를 공개했습니다.');
    } catch {
      if (operation===revision.current) {
        remove();
        setMessage('사이트 또는 현재 보드 계정을 확인하지 못했습니다.');
      }
    } finally {
      setBusy(false);
    }
  }

  const tap=(title:string,action:()=>void,disabled=false)=><Pressable accessibilityRole="button" accessibilityLabel={title}
    style={{padding:12}} disabled={disabled||busy} onPress={action}><Text style={{color:'#2658a5'}}>{title}</Text></Pressable>;
  const loginResult=loginNonce ? <>
    <Text accessibilityRole="header">사이트 로그인 서명을 반환했습니다</Text>
    <Text selectable>메시지 · {loginNonce}</Text>
    <Text>온체인 지출 · 0건. 다음 지출 요청은 별도로 확인합니다.</Text>
    {openConnections && tap('연결 목록',openConnections)}
  </> : null;
  if (reviewRequired) return <View>
    {loginResult}
    <Text accessibilityRole="header">다시 동의 필요</Text>
    <Text accessibilityLiveRegion="polite">기존 연결로 요청할 수 없습니다. 출처와 요청 범위를 다시 검토하세요.</Text>
    {tap('새 연결 검토',()=>{
      setReviewRequired(false);
      try {setReview(httpsOrigin(site.trim()));} catch {setReview('');}
    })}
    {tap('종료',back)}
  </View>;
  return <View>
    {loginResult}
    <Text accessibilityRole="header">DApp 둘러보기</Text>
    {!review && <Text>사이트와 서비스를 선택하세요. 열기 전에 HTTPS 출처와 공개 범위를 확인합니다.</Text>}
    <TextInput placeholderTextColor={inputPlaceholder} accessibilityLabel="HTTPS 사이트 주소" value={site} editable={!busy}
      maxLength={320} autoCorrect={false} autoCapitalize="none" returnKeyType="done" onSubmitEditing={Keyboard.dismiss}
      onChangeText={value=>{setSite(value);onSiteChange?.(value);setReview('');setAllowSpend(false);}} />
    {tap('사이트 범위 검토',()=>{
      Keyboard.dismiss();
      try {setReview(httpsOrigin(site.trim()));setMessage('');}
      catch {setMessage('검증 가능한 HTTPS 사이트 주소를 입력하세요.');}
    },!site)}
    {!!review && <>
      <Text accessibilityRole="header">접속 범위</Text>
      <Text selectable>브라우저 출처 · {review}</Text>
      {!scope && <Text>확인된 계정과 네트워크가 있어야 사이트에 연결할 수 있습니다.</Text>}
      {!NativeBrowser && <Text>이 배포에서 내장 브라우저를 사용할 수 없습니다.</Text>}
      {!scope && openNetwork && tap('네트워크 및 자산 설정',openNetwork)}
      {!!scope && <Text selectable>{scope.account}</Text>}
      {!!scope && <Text>체인 {scope.chain} · 10분 · 계정과 체인 조회</Text>}
      {review===loginOrigin && <Text>로그인 메시지마다 확인과 보드 승인이 필요합니다.</Text>}
      {review===loginOrigin && allowancePolicy && allowSpend && <Text>등록된 토큰 권한 요청마다 한도와 대상을 확인합니다.</Text>}
      {prepareSpend && signSpend && submitSpend && tap(allowSpend ? '이 사이트에서 송금 요청 허용됨 · 취소' : '이 사이트에서 송금 요청 허용',()=>setAllowSpend(!allowSpend))}
      {!!scope && tap('현재 보드 찾기',scan)}
      {scope && boards.map(board=><View key={board.address}>
        <Text>{board.name}</Text>
        {tap('계정 공개 동의 및 사이트 열기',()=>{open(board.address);},!NativeBrowser)}
      </View>)}
      {tap('거절',()=>{setReview('');setAllowSpend(false);})}
    </>}
    {!review && <>
      {openExternal ? tap('WalletConnect',openExternal) : <Text>이 지갑에 WalletConnect 연결 설정이 없습니다.</Text>}
      {openConnections && tap('연결 관리',openConnections)}
      {openTravel && tap('여행 지도',openTravel)}
      {openMarket && tap('학습 시장',openMarket)}
    </>}
    {tap('연결 해제',()=>{remove();NativeBrowser?.close().catch(()=>{});setMessage('사이트 연결을 닫았습니다.');})}
    {tap('지갑으로 돌아가기',back)}
    {!!message && <Text accessibilityLiveRegion="polite">{message}</Text>}
  </View>;
}
