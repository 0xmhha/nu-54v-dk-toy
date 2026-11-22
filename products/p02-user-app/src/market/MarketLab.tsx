import {useEffect, useRef, useState} from 'react';
import {Alert, Linking, Pressable, ScrollView, StyleSheet, Text, TextInput, View} from 'react-native';
import MarketVault from '../specs/NativeCommerceHttp.ts';
import NativeCloudProfile from '../specs/NativeCloudWalletProfile.ts';
import {bytesToBig, fromBase64, hexToBytes} from '@nu54/protocol';
import {reviewPosition, verifiedPositionPrice} from './positionRisk';
import {readLabBoard,scanLabBoards,signLabBoard,type LabBoard} from './board.ts';
import {CloudWalletScreen, type MarketCloudSession} from '../wallet/cloud/CloudWalletScreen.tsx';
import {CloudHttpError, cloudMarketProfile, readCloudProfile, readPending, requestID, signedTransfer,
  type CloudFlow, type CloudOperation, type CloudProfile, type PendingCloud} from '../wallet/cloud/client.ts';
import {
  deleteIsolatedWallet, displayAmount, installedMarketOperatorOrigin, installedMarketPolicy, isolatedWallet, makeIntent, marketAPI, operations, requiredAllowance, sameDeployment,
  prepareCloudMarketTransfer, signAllowance, signExecution, signedCloudCheckpoint, validAddr, validHash, verifyDeploymentPolicy, verifyApproval, verifyCheckpoint,
  type Approval, type Checkpoint, type Draft, type Intent, type Operation,
  type PositionEvent, type PositionView, type Profile, type Result, type Snapshot, type Wallet,
} from './lab.ts';

const journal = 'market-lab-8283-v1';
const groups = ['교환', '유동성', '모의 환전', '포지션'] as const;
type MarketCloudBinding = {realm:string; owner:string; wallet:string};
type Session = {user: string; profile: Profile; pending?: Checkpoint; receipt?: Result; positions: string[];
  cloud?:MarketCloudBinding; board?:LabBoard; threshold?:{request:PendingCloud; digest?:string; fundingAt?:number}};
type Offered = {intent: Intent; quote: Approval; risk?: NonNullable<ReturnType<typeof reviewPosition>>};

function Action({label, caption = label, run, disabled}: {label: string; caption?: string; run: () => void; disabled?: boolean}) {
  return <Pressable accessibilityRole="button" accessibilityLabel={label} accessibilityState={{disabled: !!disabled}}
    disabled={disabled} style={[styles.button, disabled && styles.disabled]} onPress={run}><Text>{caption}</Text></Pressable>;
}

function timeLabel(seconds: number) {
  return seconds ? new Date(seconds * 1000).toLocaleTimeString() : '미관측';
}

function outputLabel(action: Operation, first: string, second: string) {
  const output = displayAmount(first, action.out[1]) + ' ' + action.out[0];
  if (action.id === 'lp-remove') return output + ' · ' + displayAmount(second, 6) + ' LAQ';
  if (action.id === 'open-long' || action.id === 'open-short') return output + ' · ' + displayAmount(second, 8) + ' LAB';
  return output;
}

function liquidationLabel(bounds: {lower?: bigint; higher?: bigint}, long: boolean) {
  const price = (value: bigint) => displayAmount(value.toString(),6) + ' LAQ/LAB';
  const lower = bounds.lower ? price(bounds.lower) + ' 이하' : '양의 하락 임계 없음';
  const higher = bounds.higher ? price(bounds.higher) + ' 이상' : '등록 가격 범위 안에는 임계 없음';
  if (!long) return higher;
  return bounds.higher ? lower + ' · 평가액 상한 적용 시 ' + higher : lower;
}

export function MarketLab({back, permission, openTravel}: {
  back: () => void; permission: () => Promise<boolean>; openTravel?: () => void;
}) {
  const [operatorOrigin,setOperatorOrigin] = useState('');
  const [loaded,setLoaded] = useState(false);
  const [session,setSession] = useState<Session>();
  const [snapshot,setSnapshot] = useState<Snapshot>();
  const sampledAt = useRef(0);
  const [reviewClock,setReviewClock] = useState(0);
  const [pin,setPin] = useState('');
  const [wallet,setWallet] = useState<Wallet>();
  const [group,setGroup] = useState<(typeof groups)[number]>('교환');
  const [liquidityReview,setLiquidityReview] = useState(false);
  const [operation,setOperation] = useState<Operation>(operations[0]);
  const [draft,setDraft] = useState<Partial<Draft>>({});
  const [offered,setOffered] = useState<Offered>();
  const [previous,setPrevious] = useState<Offered>();
  const [receiptActive,setReceiptActive] = useState(false);
  const [receiptOpen,setReceiptOpen] = useState(false);
  const result = session?.receipt;
  const [position,setPosition] = useState<{id: string; view: PositionView; events: PositionEvent[]}>();
  const [message,setMessage] = useState('');
  const [busy,setBusy] = useState(false);
  const [cloudProfile,setCloudProfile]=useState<CloudProfile>();
  const [cloudShown,setCloudShown]=useState(false);
  const [cloudAuth,setCloudAuth]=useState<MarketCloudSession>();
  const [cloudOp,setCloudOp]=useState<CloudOperation>();
  const [cloudFlow,setCloudFlow]=useState<CloudFlow>();
  const [boardOptions,setBoardOptions]=useState<string[]>([]);
  const [boardHandle,setBoardHandle]=useState<string>();
  const running = useRef(false);
  const user = session?.user ?? '';
  const pending = session?.pending;
  const threshold=session?.threshold;

  useEffect(() => {
    let active = true;
    async function load() {
      try {
        if (!MarketVault) throw new Error('LAB_VAULT');
        const policy = await installedMarketPolicy();
        if (!policy) {
          if (active) setMessage('승인된 테스트 시장 배포 설정이 필요합니다.');
          return;
        }
        const data = await MarketVault.loadJournal(journal);
        if (data) {
          const saved = JSON.parse(data) as Session;
          verifyDeploymentPolicy(policy,saved.profile);
          if ('key' in saved || !validAddr(saved.user) || !Array.isArray(saved.positions) ||
              saved.positions.some(id => !/^[1-9][0-9]{0,77}$/.test(id))) throw new Error('LAB_JOURNAL');
          if (!saved.cloud && !saved.board && await isolatedWallet(saved.profile,false) !== saved.user) throw new Error('LAB_JOURNAL_IDENTITY');
          if (saved.threshold) {
            const item=readPending(JSON.stringify(saved.threshold.request));
            if (!saved.cloud || !item?.transfer?.market || item.transfer.market.user!==saved.user ||
                item.transfer.market.deploymentHash!==saved.profile.deploymentHash.toLowerCase() ||
                (saved.threshold.digest!==undefined && !validHash(saved.threshold.digest)) ||
                (saved.threshold.fundingAt !== undefined &&
                  (!Number.isSafeInteger(saved.threshold.fundingAt) || saved.threshold.fundingAt <= 0))) throw new Error('LAB_CLOUD_JOURNAL');
          }
          if (saved.pending) verifyCheckpoint(saved.pending);
          if (saved.receipt && (!validHash(saved.receipt.txHash) || !saved.receipt.blockHash ||
              !validHash(saved.receipt.blockHash) || !['confirmed','rejected'].includes(saved.receipt.state))) {
            throw new Error('LAB_RECEIPT');
          }
          if (active) setSession(saved);
        }
        if (active) setLoaded(true);
      } catch {
        if (active) setMessage('격리 학습 지갑 기록을 열 수 없습니다.');
      }
    }
    load();
    installedMarketOperatorOrigin().then(origin => {
      if (active) setOperatorOrigin(origin);
    }).catch(() => {});
    return () => { active = false; };
  }, []);

  useEffect(() => {
    if (group !== '포지션') return;
    setReviewClock(Date.now());
    const timer = setInterval(() => setReviewClock(Date.now()),1000);
    return () => clearInterval(timer);
  },[group]);


  function work(run: () => Promise<void>, error: string) {
    if (running.current) return;
    running.current = true;
    setBusy(true);
    setMessage('');
    run().catch(() => setMessage(error)).finally(() => {
      running.current = false;
      setBusy(false);
    });
  }

  async function store(next: Session) {
    if (!MarketVault) throw new Error('LAB_VAULT');
    await MarketVault.saveJournal(journal, JSON.stringify(next));
    setSession(next);
  }

  async function readSnapshot(expected?: Profile) {
    let current: Snapshot;
    try {
      current = await marketAPI<Snapshot>('/v1/profile');
      verifyDeploymentPolicy(await installedMarketPolicy(),current.profile);
      if (expected && !sameDeployment(expected,current.profile)) throw new Error('LAB_DEPLOYMENT_CHANGED');
    } catch (error) {
      setSnapshot(undefined);
      setOffered(value => value?.risk ? undefined : value);
      throw error;
    }
    if (!Number.isSafeInteger(current.time) || !Array.isArray(current.observations) ||
        current.observations.length !== 2 || current.observations.some(item =>
          typeof item?.price !== 'string' || !/^[0-9]{1,9}$/.test(item.price) ||
          typeof item?.observedAt !== 'string' || !/^[0-9]{1,15}$/.test(item.observedAt)) ||
        (current.price !== undefined && !/^[0-9]{1,9}$/.test(current.price))) {
      setSnapshot({...current, state:'price-unavailable', price:undefined, observations:[]});
      setOffered(value => value?.risk ? undefined : value);
      return {...current, state:'price-unavailable' as const, price:undefined, observations:[]};
    }
    sampledAt.current = Date.now();
    setReviewClock(sampledAt.current);
    setOffered(value => {
      if (!value?.risk) return value;
      const risk = reviewPosition(value.quote,current);
      return risk?.nextFunding === value.risk.nextFunding && current.time < value.quote.deadline ? value : undefined;
    });
    setSnapshot(current);
    return current;
  }

  async function connect() {
    if (!MarketVault) throw new Error('LAB_VAULT');
    const current = await readSnapshot();
    if (!validHash(pin) || pin.toLowerCase() !== current.profile.deploymentHash.toLowerCase()) throw new Error('LAB_PIN');
    const enrolled = await isolatedWallet(current.profile,true);
    const next = {user:enrolled,profile:current.profile,positions:[]};
    await store(next);
    setWallet(await marketAPI<Wallet>('/v1/wallet/' + enrolled));
  }

  async function pickBoard() {
    if (!await permission()) throw new Error('LAB_BOARD_PERMISSION');
    const options=await scanLabBoards();
    setBoardOptions(options);
    if (!options.length) setMessage('등록된 보드가 검색되지 않았습니다. 보드를 켠 뒤 다시 시도하세요.');
  }

  async function attachBoard(handle:string) {
    const current=await readSnapshot(session?.profile);
    if (!session && (!validHash(pin) || pin.toLowerCase()!==current.profile.deploymentHash.toLowerCase())) throw new Error('LAB_PIN');
    const board=await readLabBoard(handle);
    if(session && (!session.board || session.board.fingerprint!==board.fingerprint ||
      session.board.generation!==board.generation || session.user!==board.user)) throw new Error('LAB_BOARD_ACCOUNT');
    const balance=await marketAPI<Wallet>('/v1/wallet/'+board.user);
    if(!session) await store({user:board.user, profile:current.profile,positions:[],board});
    setBoardOptions([]);
    setBoardHandle(handle);
    setWallet(balance);
  }

  async function pickCloud() {
    if (!NativeCloudProfile) throw new Error('LAB_CLOUD_UNAVAILABLE');
    const configured=readCloudProfile(await NativeCloudProfile.profile());
    setCloudProfile(configured);
    setCloudShown(true);
  }

  async function attachCloud(auth:MarketCloudSession) {
    if (!cloudProfile) return;
    const current=await readSnapshot(session?.profile);
    const profile=current.profile;
    if (!session && (!validHash(pin) || pin.toLowerCase()!==profile.deploymentHash.toLowerCase())) throw new Error('LAB_PIN');
    const account=auth.account;
    if (!account.address || account.state!=='active' || account.profile!==cloudMarketProfile+':'+profile.deploymentHash.slice(2).toLowerCase()) {
      throw new Error('LAB_CLOUD_DEPLOYMENT');
    }
    const binding={realm:cloudProfile.api+'|'+cloudProfile.identity,owner:auth.owner,wallet:account.id};
    if (session && (!session.cloud || session.user!==account.address || session.cloud.owner!==binding.owner ||
      session.cloud.wallet!==binding.wallet || session.cloud.realm!==binding.realm)) throw new Error('LAB_CLOUD_WALLET');
    const balance=await marketAPI<Wallet>('/v1/wallet/'+account.address);
    if (!session) await store({user:account.address,profile,positions:[],cloud:binding});
    setCloudAuth(auth);
    setWallet(balance);
    setCloudShown(false);
  }

  async function liveCloud() {
    if (!cloudAuth || !session?.cloud) throw new Error('LAB_CLOUD_LOGIN');
    const {client,token}=cloudAuth;
    const principal=await client.subject(token);
    const account=await client.account(token);
    if (principal.linked===false || principal.subject!==session.cloud.owner || account.id!==session.cloud.wallet ||
      account.address!==session.user || account.profile!==cloudMarketProfile+':'+session.profile.deploymentHash.slice(2).toLowerCase()) {
      throw new Error('LAB_CLOUD_WALLET');
    }
    return {...cloudAuth,account};
  }

  async function acceptThreshold(authorizedOperation:CloudOperation) {
    if (!session || !threshold) throw new Error('LAB_CLOUD_UNAVAILABLE');
    if (['refused','expired','failed'].includes(authorizedOperation.state)) {
      await store({...session,threshold:undefined});
      setCloudFlow(undefined);
      setCloudOp(undefined);
      if (authorizedOperation.state==='refused') setOffered(undefined);
      setMessage(authorizedOperation.state==='refused'?'이 원문을 거절했습니다. 새 견적을 받으세요.':'공동 서명 시간이 지났습니다. 다시 견적을 받으세요.');
      return;
    }
    setCloudOp(authorizedOperation);
    if (authorizedOperation.state!=='succeeded') return;
    const auth=await liveCloud();
    const raw=signedTransfer(authorizedOperation,threshold.request,auth.account);
    const offer = threshold.request.transfer?.market?.offer;
    if (!offer) throw new Error('LAB_CLOUD_ORDER');
    const item={...signedCloudCheckpoint(raw,threshold.digest,offer),fundingAt:threshold.fundingAt};
    setCloudOp(undefined);
    setCloudFlow(undefined);
    await keepSigned(item,{...session,threshold:undefined});
  }

  async function advanceThreshold() {
    if (!session || !threshold) return;
    const raw = threshold.request.transfer?.market?.offer;
    if (!raw) throw new Error('LAB_CLOUD_ORDER');
    const expiry = bytesToBig(hexToBytes(raw.slice(2)).slice(260,292));
    if (expiry <= BigInt(Math.floor(Date.now()/1000))) {
      await store({...session,threshold:undefined});
      setCloudFlow(undefined);
      setCloudOp(undefined);
      setOffered(undefined);
      setMessage('이 견적의 승인 기한이 지났습니다. 새 견적을 받으세요.');
      return;
    }
    const auth=await liveCloud();
    const {client,token}=auth;
    if (cloudFlow) {
      const approval=await client.poll(cloudFlow);
      if (approval.state==='pending') {setMessage('독립 승인 브라우저에서 확인한 뒤 다시 조회하세요.');return;}
      setCloudFlow(undefined);
      if (approval.state==='denied' || approval.state==='resolved') {
        setCloudOp(undefined);
        await acceptThreshold(await client.lookup(token,threshold.request,auth.owner));
        return;
      }
      if (approval.state!=='ready' || !approval.proof) {setMessage('이번 독립 승인 요청이 종료됐습니다.');return;}
      await requireDispatchOffer(session.profile,raw,threshold.fundingAt);
      let result:CloudOperation;
      try {result=await client.approve(token,threshold.request,approval.proof);}
      catch(error){
        if (!(error instanceof CloudHttpError) || error.status!==409) throw error;
        setCloudOp(undefined);
        result=await client.lookup(token,threshold.request,auth.owner);
      }
      await acceptThreshold(result);
    } else if (cloudOp?.state==='reserved') {
      await requireDispatchOffer(session.profile,raw,threshold.fundingAt);
      setCloudFlow(await client.authorize(cloudOp,threshold.request,auth.owner));
    } else {
      await acceptThreshold(await client.lookup(token,threshold.request));
    }
  }

  function erase() {
    const storage = MarketVault;
    if (!session || pending || threshold || !storage) return;
    Alert.alert('격리 학습 지갑 삭제', session.cloud || session.board ?
      '이 기기의 학습 지갑 연결과 거래 기록을 삭제합니다.' : '학습 전용 키와 이 기기의 학습 거래 기록을 삭제합니다.', [
      {text:'취소',style:'cancel'},
      {text:'삭제',style:'destructive',onPress: () => work(async () => {
        if (!session.cloud && !session.board) await deleteIsolatedWallet(session.profile);
        await storage.saveJournal(journal, '');
        setSession(undefined);
        setPin('');
        setBoardHandle(undefined);
        setCloudAuth(undefined);
        setCloudProfile(undefined);
        setWallet(undefined);
        setReceiptActive(false);
        setPosition(undefined);
        select(operations[0]);
      },'격리 학습 기록을 삭제할 수 없습니다.')},
    ]);
  }

  async function refresh() {
    if (!session) return;
    await readSnapshot(session.profile);
    setWallet(await marketAPI<Wallet>('/v1/wallet/' + user));
  }

  function select(action: Operation) {
    setLiquidityReview(action.group === '유동성');
    setReceiptActive(false);
    setOperation(action);
    setGroup(action.group);
    setDraft({});
    setOffered(undefined);
    setPrevious(undefined);
    setMessage('');
  }

  function openPool() {
    select(operations[2]);
    setLiquidityReview(false);
    setSnapshot(undefined);
    setWallet(undefined);
    work(refresh,'검증된 테스트 풀과 학습 지갑 잔액을 다시 확인하세요.');
  }

  async function getQuote() {
    if (!session) return;
    setReceiptActive(false);
    const intent = makeIntent(operation,draft,user);
    setPrevious(current => offered ?? current);
    setOffered(undefined);
    const current = await readSnapshot(session.profile);
    if (current.state !== 'ready') throw new Error('LAB_MARKET_STOPPED');
    const isOpen = intent.action === 'open-long' || intent.action === 'open-short';
    if (isOpen && !verifiedPositionPrice(current)) throw new Error('LAB_POSITION_PRICE');
    const q = await marketAPI<Approval>('/v1/quotes',intent);
    const received = await readSnapshot(session.profile);
    verifyApproval(session.profile,intent,q,received.time);
    if (received.state !== 'ready' || q.priceVersion !== current.priceVersion || q.price !== current.price ||
        q.priceVersion !== received.priceVersion || q.price !== received.price) throw new Error('LAB_PRICE_MOVED');
    const risk = isOpen ? reviewPosition(q,received) : undefined;
    if (isOpen && !risk) throw new Error('LAB_POSITION_REVIEW');
    setOffered({intent,quote:q,risk});
    setWallet(await marketAPI<Wallet>('/v1/wallet/' + user));
  }

  async function send(item: Checkpoint) {
    const reply = await marketAPI<{txHash: string; state: string}>('/v1/transactions',{raw:item.raw,...(item.offer ? {offer:item.offer} : {})});
    if (reply.txHash.toLowerCase() !== item.hash || !['submitted','unknown'].includes(reply.state)) throw new Error('LAB_HASH');
    setMessage(reply.state === 'submitted' ? '보냈습니다. 원래 거래 해시로 확정 여부를 확인하세요.' :
      '응답을 확인할 수 없습니다. 원래 거래 해시로 상태를 확인하세요.');
  }

  async function requireDispatchOffer(profile: Profile, offer: string, fundingAt?: number) {
    if (!/^0x[0-9a-f]{904}$/i.test(offer)) throw new Error('LAB_RELAY_OFFER');
    const wire = hexToBytes(offer.slice(2));
    const version = bytesToBig(wire.subarray(196,228)).toString();
    const deadline = bytesToBig(wire.subarray(260,292));
    const current = await readSnapshot(profile);
    const opensPosition = wire[67] === 6 || wire[67] === 7;
    const risk = opensPosition && verifiedPositionPrice(current);
    if (current.state !== 'ready' || current.priceVersion !== version || BigInt(current.time) >= deadline ||
        (opensPosition && (!risk || (fundingAt !== undefined && risk.nextFunding !== fundingAt)))) {
      setOffered(undefined);
      throw new Error('LAB_RELAY_OFFER_CHANGED');
    }
  }

  async function dispatch(item: Checkpoint, next: Session) {
    if (!item.offer) {
      // A legacy journal can only recover its original known hash at the relay.
      await send(item);
      return;
    }
    try {
      await requireDispatchOffer(next.profile,item.offer,item.fundingAt);
      if (item.phase === 'staged') {
        await store({...next,pending:{...item,phase:'attempted'}});
        await requireDispatchOffer(next.profile,item.offer,item.fundingAt);
      }
    } catch (error) {
      if (item.phase === 'staged') await store({...next,pending:undefined});
      throw error;
    }
    await send(item);
  }

  async function keepSigned(item: Checkpoint, next: Session) {
    if (!item.offer) throw new Error('LAB_RELAY_OFFER');
    try {
      await requireDispatchOffer(next.profile,item.offer,item.fundingAt);
    } catch (error) {
      if (session?.threshold) await store(next);
      throw error;
    }
    const sealed = {...item,phase:'staged' as const};
    await store({...next,pending:sealed});
    await dispatch(sealed,next);
  }

  async function submit(allowance: boolean) {
    if (!session || session.pending || threshold || !offered) return;
    const {profile} = session;
    const {intent,quote:q,risk:originalRisk} = offered;
    async function readApprovalHead() {
      const current = await readSnapshot(profile);
      const risk = originalRisk && reviewPosition(q,current);
      if (current.state !== 'ready' || q.priceVersion !== current.priceVersion || q.price !== current.price ||
          (originalRisk && (!risk || risk.nextFunding !== originalRisk.nextFunding))) {
        if (originalRisk) setOffered(undefined);
        throw new Error('LAB_PRICE_CHANGED');
      }
      verifyApproval(profile,intent,q,current.time);
      return current;
    }
    let current = await readApprovalHead();
    const nextWallet = await marketAPI<Wallet>('/v1/wallet/' + user);
    setWallet(nextWallet);
    if (session.cloud) {
      const auth=await liveCloud();
      if (!MarketVault) throw new Error('LAB_CLOUD_STORAGE');
      if (originalRisk) current = await readApprovalHead();
      const fresh = q;
      const transfer=prepareCloudMarketTransfer(session.profile,user,nextWallet,offered.intent,fresh,current.time,allowance);
      const request:PendingCloud={id:requestID(fromBase64(await MarketVault.randomBytes(16))),kind:'sign',transfer};
      await requireDispatchOffer(profile,q.data,originalRisk?.nextFunding);
      await store({...session,threshold:{request,digest:allowance?undefined:fresh.digest,fundingAt:originalRisk?.nextFunding}});
      await requireDispatchOffer(profile,q.data,originalRisk?.nextFunding);
      setCloudOp(await auth.client.reserve(auth.token,request));
      setReceiptActive(false);
      if (!allowance) setOffered(undefined);
      return;
    }
    if (originalRisk) current = await readApprovalHead();
    const fresh = q;
    if (session.board) {
      if (!boardHandle) throw new Error('LAB_BOARD_RECONNECT');
      const transfer=prepareCloudMarketTransfer(session.profile,user,nextWallet,offered.intent,fresh,current.time,allowance);
      const raw=await signLabBoard(boardHandle,session.board,session.profile,transfer,fresh,
        ()=>setMessage('독립 화면의 시장 주문을 확인하고 보드에서 승인하세요.'),
        () => requireDispatchOffer(profile,q.data,originalRisk?.nextFunding));
      const item=signedCloudCheckpoint(raw,allowance?undefined:fresh.digest,q.data);
      setReceiptActive(false);
      if(!allowance) setOffered(undefined);
      await keepSigned({...item,fundingAt:originalRisk?.nextFunding},session);
      return;
    }
    const item = allowance ? await signAllowance(session.profile,user,nextWallet,offered.intent,fresh,current.time) :
      await signExecution(session.profile,user,nextWallet,offered.intent,fresh,current.time);
    setReceiptActive(false);
    if (!allowance) setOffered(undefined);
    await keepSigned({...item,fundingAt:originalRisk?.nextFunding},session);
  }

  async function reconcile() {
    if (!session || !pending) return;
    await readSnapshot(session.profile);
    const route = pending.digest ? '/v1/executions/' + pending.hash + '/' + pending.digest :
      '/v1/approvals/' + pending.hash + '/' + user;
    const evidence = await marketAPI<Result>(route);
    if (evidence.txHash.toLowerCase() !== pending.hash) throw new Error('LAB_HASH');
    if (evidence.state === 'pending') {
      setMessage('아직 확정되지 않았습니다. 같은 해시로 다시 확인할 수 있습니다.');
      return;
    }
    if (!['confirmed','rejected'].includes(evidence.state) || !evidence.blockHash || !validHash(evidence.blockHash)) {
      throw new Error('LAB_EVIDENCE');
    }
    const positions = session.positions.slice();
    if (evidence.state === 'confirmed' && (evidence.action === 'open-long' || evidence.action === 'open-short')) {
      if (!evidence.first || !/^[1-9][0-9]{0,77}$/.test(evidence.first)) throw new Error('LAB_POSITION_ID');
      if (!positions.includes(evidence.first)) positions.push(evidence.first);
    }
    await store({...session,pending:undefined,positions,receipt:evidence});
    setReceiptActive(true);
    if (pending.digest) setPrevious(undefined);
    await refresh();
  }

  async function readPosition(id: string) {
    if (!session) return;
    await readSnapshot(session.profile);
    const view = await marketAPI<PositionView>('/v1/positions/' + id);
    if (view.position.owner.toLowerCase() !== user) throw new Error('LAB_POSITION_OWNER');
    const events = await marketAPI<PositionEvent[]>('/v1/positions/' + id + '/history');
    setPosition({id,view,events});
  }

  const need = offered && wallet ? requiredAllowance(offered.intent,wallet) : undefined;
  const eligible = snapshot?.state === 'ready' && !pending && !threshold && !busy;
  const poolSelection = group === '유동성' && !liquidityReview;
  const poolVerified = !!snapshot && wallet && /^[0-9]{1,78}$/.test(wallet.shares);
  const elapsed = Math.max(0,Math.floor((reviewClock - sampledAt.current)/1000));
  const positionPrice = snapshot && verifiedPositionPrice(snapshot,elapsed);
  const positionStale = group === '포지션' && !positionPrice;
  const reviewedRisk = offered?.risk;
  const expiredRisk = !!reviewedRisk &&
    (!positionPrice || !snapshot || snapshot.time + elapsed >= offered.quote.deadline);
  const shown = positionStale || expiredRisk ? undefined : offered?.quote;
  const resultType = operations.find(item => item.id === result?.action);
  const openResult = result?.action === 'open-long' || result?.action === 'open-short';
  const pos = position?.view.position;

  useEffect(() => {
    if (expiredRisk) setOffered(undefined);
  },[expiredRisk]);

  const receipt = result && <View key="market-receipt" collapsable={false} style={styles.card}>
    <Text>{result.state === 'confirmed' ? '거래 확정' : '거래 거절'} · 블록 {result.block}</Text>
    <Text selectable>{result.txHash}</Text>
    {result.gasFee && <Text>실제 가스 비용 {displayAmount(result.gasFee,18)} 실습 가스 단위</Text>}
    {openResult && result.first ? <Text>포지션 번호 {result.first}</Text> :
      resultType && result.first && result.second && <Text>실제 {outputLabel(resultType,result.first,result.second)}</Text>}
    {result.addedBase && result.addedQuote && <Text>지급 {displayAmount(result.addedBase,8)} LAB · {displayAmount(result.addedQuote,6)} LAQ</Text>}
  </View>;

  return <>
    {cloudProfile && <View style={cloudShown ? styles.cloudPanel : styles.cloudHidden}>
      <CloudWalletScreen profile={cloudProfile} back={()=>setCloudShown(false)}
        selectMarket={attachCloud} />
    </View>}
    {receiptOpen && receipt && <ScrollView key="market-receipt-page" contentContainerStyle={styles.container}>
      <Action label="시장 주문으로 돌아가기" run={()=>setReceiptOpen(false)} />
      <Text accessibilityRole="header" style={styles.title}>최근 거래 영수증</Text>
      {receipt}
    </ScrollView>}

    {!cloudShown && !receiptOpen && <ScrollView key={session ? 'market-wallet' : 'market-enrollment'}
      automaticallyAdjustKeyboardInsets keyboardShouldPersistTaps="handled" keyboardDismissMode="on-drag" contentContainerStyle={styles.container}>
    <Action label="지갑으로 돌아가기" run={back} disabled={busy} />
    <Text accessibilityRole="header" style={styles.title}>테스트 자산으로 연습하기</Text>
    <View style={styles.card}>
      <Text style={styles.heading}>StableNet 테스트넷 · 테스트 자산만</Text>
      <Text>실물 자산이나 실거래가 아닙니다.</Text>
      <Text>격리 테스트 체인 8283 · 앱 내 학습 전용 지갑</Text>
    </View>
    {groups.map(label => <Action key={label} label={label}
      caption={label === '모의 환전' ? 'TEST FX · 모의 환전' : label === '포지션' ? '모의 포지션' : label}
      disabled={busy || !!pending || !!threshold} run={() => {
        const selected = operations.find(item => item.group === label)!;
        if (session && label === '유동성') openPool();
        else if (session) select(selected);
        else {
          setGroup(label);
          setOperation(selected);
        }
      }} />)}
    {openTravel && <Action label="여행" disabled={busy} run={openTravel} />}
    <Action label="시장 운영 모니터" caption="운영자 인증" disabled={busy || !operatorOrigin} run={() => {
      if (operatorOrigin) work(() => Linking.openURL(operatorOrigin), '설치된 시장 운영 모니터 주소를 브라우저에서 열 수 없습니다.');
    }} />
    {!operatorOrigin && <Text>이 설치에는 승인된 운영자 주소가 없습니다.</Text>}
    {!session && <Text>{`${group} 연습 · 승인된 학습 지갑을 연결하면 시작할 수 있습니다.`}</Text>}
    {!!message && <Text accessibilityLiveRegion="polite">{message}</Text>}
    {loaded && !session && <>
      <Text>로컬 학습 배포의 블록 해시를 확인해 앱에서 전용 지갑을 만듭니다.</Text>
      <TextInput accessibilityLabel="학습 배포 블록 해시" placeholder="학습 배포 블록 해시" value={pin} onChangeText={setPin}
        autoCapitalize="none" autoCorrect={false} maxLength={66} editable={!busy} style={styles.input} />
      <Action label="P03 격리 학습 지갑 선택" disabled={busy || !validHash(pin)}
        run={()=>work(pickCloud,'이 앱에는 격리 P03 등록 프로필이 없습니다.')} />
      <Action label="P01 격리 하드웨어 지갑 선택" disabled={busy || !validHash(pin)}
        run={()=>work(pickBoard,'등록된 P01 보드와 블루투스 권한을 확인하세요.')} />
      <Action label="격리 학습 지갑 만들기" disabled={busy || !validHash(pin)}
        run={() => work(connect,'배포 해시와 승인된 학습 시장 연결을 확인하세요.')} />
    </>}
    {boardOptions.map((handle,index)=><Action key={handle} label={`P01 보드 후보 ${index+1} 확인`} disabled={busy}
      run={()=>work(()=>attachBoard(handle),'독립 등록과 일치하는 보드를 확인할 수 없습니다.')} />)}
    {boardOptions.length>0 && <Action label="보드 선택 취소" disabled={busy} run={()=>setBoardOptions([])} />}
    {session && <>
      <Text selectable accessibilityLabel={'전용 학습 지갑 ' + user}>전용 학습 지갑 {user}</Text>
      {session.board && <>
        <Text>P01 보드 학습 계정 · 독립 화면과 기기 승인 필요</Text>
        <Action label="등록 P01 보드 다시 연결" disabled={busy} run={()=>work(pickBoard,'등록된 P01 보드를 찾을 수 없습니다.')} />
      </>}
      {session.cloud && <>
        <Text>P03 두 참여자 공동 서명 · 독립 승인 필요</Text>
        <Action label="P03 로그인 확인" disabled={busy} run={()=>work(pickCloud,'독립 P03 프로필을 읽을 수 없습니다.')} />
      </>}
      {threshold && <View style={styles.card}>
        <Text>P03 승인 요청 {threshold.request.id}</Text>
        <Action label={cloudFlow ? '독립 승인 결과 가져오기' : cloudOp?.state==='reserved' ? '독립 승인 열기' : 'P03 원래 요청 조회'}
          disabled={busy || !cloudAuth} run={()=>work(advanceThreshold,'P03의 원래 요청을 확인하세요.')} />
        {!cloudFlow && !cloudOp && cloudAuth && <Action label="같은 P03 요청 다시 제출" disabled={busy}
          run={()=>work(async()=>{
            const auth=await liveCloud();
            const offer = threshold.request.transfer?.market?.offer;
            if (!offer) throw new Error('LAB_CLOUD_ORDER');
            await requireDispatchOffer(session.profile,offer,threshold.fundingAt);
            await acceptThreshold(await auth.client.reserve(auth.token,threshold.request));
          },'기존 P03 요청을 다시 확인할 수 없습니다.')} />}
      </View>}
      <Action label="가격과 잔액 확인" disabled={busy} run={() => work(refresh,'학습 환경과 잔액을 확인할 수 없습니다.')} />
      {snapshot && !positionStale && <View style={styles.card}>
        <Text>{snapshot.state === 'ready' ? '정상 가격' : snapshot.state === 'paused' ? '시장 중지' : '가격 관측 보류'}</Text>
        {snapshot.price && <Text>1 LAB = {displayAmount(snapshot.price,6)} LAQ · 버전 {snapshot.priceVersion}</Text>}
        {snapshot.observations.map((entry,index) => <Text key={index}>
          관측 {index + 1} · {displayAmount(entry.price,6)} LAQ · {timeLabel(Number(entry.observedAt))}
        </Text>)}
      </View>}
      {wallet && <Text>LAB {displayAmount(wallet.baseBalance,8)} · LAQ {displayAmount(wallet.quoteBalance,6)} · LP {displayAmount(wallet.shares,8)}</Text>}
      {poolSelection ? <View style={styles.card}>
        <Text accessibilityRole="header" style={styles.heading}>테스트 풀 선택</Text>
        {poolVerified ? <>
          <Text>검증된 LAB / LAQ 테스트 풀</Text>
          <Text selectable>풀 주소 {snapshot.profile.venue.address}</Text>
          <Text>보유 LP {displayAmount(wallet.shares,8)} LP</Text>
          <Action label="유동성 공급" caption="유동성 추가" disabled={busy || !!pending || !!threshold} run={() => select(operations[2])} />
          <Action label="유동성 회수" caption="유동성 제거" disabled={busy || !!pending || !!threshold} run={() => select(operations[3])} />
        </> : <Text>검증된 테스트 풀과 잔액을 확인한 뒤 작업을 선택하세요.</Text>}
        <Action label="학습 홈" disabled={busy} run={() => select(operations[0])} />
      </View> : <>
      {group === '유동성' ? <Action label="풀로 돌아가기" disabled={busy || !!pending || !!threshold} run={() => {
        select(operations[2]);
        setLiquidityReview(false);
      }} /> : operations.filter(item => item.group === group).map(item =>
        <Action key={item.id} label={item.label} disabled={busy || !!pending} run={() => select(item)} />)}
      <Text accessibilityRole="header" style={styles.heading}>{operation.label}</Text>
      {group === '교환' && <Text>LAB ↔ LAQ 단일 풀 · 풀 수수료 0.30%</Text>}
      {group === '모의 환전' && <Text>실습 가격 환산 · 토큰 잔액 변화 없이 결과 기록 · 환전 수수료 0</Text>}
      {group === '포지션' && <>
        {positionStale && <View style={styles.card}>
          <Text accessibilityRole="header" style={styles.heading}>신규 위험 증가가 중지됐어요</Text>
          <Text>두 기준 가격을 검증할 때까지 신규 승인과 자동 청산을 보류합니다. 기존 포지션과 확정 이력은 조회할 수 있습니다.</Text>
        </View>}
        <Text>LAQ 격리 증거금 · 진입 레버리지 최대 1배 · 펀딩 5분마다 0.10% · 유지증거금 10%</Text>
        {session.positions.map(id => <Action key={id} label={'포지션 ' + id + ' 확인'} disabled={busy}
          run={() => work(() => readPosition(id),'격리 포지션을 확인할 수 없습니다.')} />)}
        {pos && position && <View style={styles.card}>
          <Text>포지션 {position.id} · {pos.long ? '매수' : '매도'} · {pos.closed ? '종료' : '열림'} · 버전 {pos.revision}</Text>
          <Text>증거금 {displayAmount(pos.margin,6)} LAQ · 진입 가격 {displayAmount(pos.entryPrice,6)} LAQ</Text>
          <Text>크기 {displayAmount(pos.size,8)} LAB · 다음 펀딩 {timeLabel(Number(pos.nextPeriod)*300)}</Text>
          {!positionStale && position.view.equity && position.view.maintenance &&
            <Text>조회 시 평가액 {displayAmount(position.view.equity,6)} LAQ · 청산 기준 {displayAmount(position.view.maintenance,6)} LAQ 이하</Text>}
          {position.events.map(event => <View key={event.txHash}>
            <Text>{event.kind === 'opened' ? '포지션 개설' : event.kind === 'funded' ? '펀딩 반영' :
              event.kind === 'closed' ? '수동 종료' : '자동 청산'} · 확정 블록 {event.block}</Text>
            {event.payment && <Text>정산 기간 {event.period} · {event.payment.startsWith('-') ? '차감 ' : '가산 '}
              {displayAmount(event.payment.replace(/^-/,'') ,6)} LAQ</Text>}
            {event.payout && <Text>반환 {displayAmount(event.payout,6)} LAQ · 미반영 펀딩 기간 {event.fundingGap}</Text>}
            <Text selectable>{event.txHash}</Text>
          </View>)}
          {!pos.closed && <Action label="이 포지션 종료 견적 준비" disabled={busy || !!pending} run={() => {
            select(operations[8]);
            setDraft({a:position.id,b:pos.revision,minA:'0'});
          }} />}
        </View>}
      </>}
      {operation.fields.map(([field,label]) => <TextInput key={field} accessibilityLabel={label} placeholder={label}
        keyboardType="decimal-pad" value={draft[field] ?? ''} editable={!busy && !pending} style={styles.input}
        onChangeText={value => {
          setDraft({...draft,[field]:value});
          setOffered(undefined);
          setPrevious(undefined);
        }} />)}
      <Action label={offered || previous ? '새 견적 받기' : '견적 받기'} disabled={!eligible || positionStale}
        run={() => work(getQuote,'현재 가격과 입력 한도를 확인해 새 견적을 받으세요.')} />
      {shown && <View key="market-offer" collapsable={false} style={styles.card}>
        {reviewedRisk ? <View style={styles.card}>
          <Text accessibilityRole="header" style={styles.heading}>위험 조건을 확인하세요</Text>
          <Text>{reviewedRisk.long ? '매수' : '매도'} · {displayAmount(reviewedRisk.size.toString(),8)} LAB</Text>
          <Text>격리 증거금 {displayAmount(reviewedRisk.margin.toString(),6)} LAQ · 최대 {displayAmount(reviewedRisk.sizeLimit.toString(),8)} LAB (1배)</Text>
          <Text>기준 가격 {displayAmount(reviewedRisk.price.toString(),6)} LAQ/LAB · 관측 {timeLabel(reviewedRisk.observedAt)}</Text>
          <Text>다음 예상 펀딩 {timeLabel(reviewedRisk.nextFunding)} · 5분마다 0.10% · 예상 {reviewedRisk.long ? '차감' : '수령'} {displayAmount(reviewedRisk.funding.toString(),6)} LAQ</Text>
          <Text>현재 기준 가격에서의 예상 펀딩입니다. 실제 개설 블록과 적용 가격에 따라 달라집니다.</Text>
          <Text>펀딩 전 예상 청산 조건 · {liquidationLabel(reviewedRisk.beforeFunding,reviewedRisk.long)}</Text>
          <Text>첫 예상 펀딩 후 · {liquidationLabel(reviewedRisk.afterFunding,reviewedRisk.long)}</Text>
          <Text>유지증거금 10% · 현재 학습 배포의 가격 범위와 계약 반올림 적용</Text>
          <Action label="포지션으로 돌아가기" disabled={busy} run={() => {
            setOffered(undefined);
            setPrevious(undefined);
          }} />
        </View> : <Text>예상 {outputLabel(operation,shown.first,shown.second)}</Text>}
        <Text>가격 시각 {timeLabel(shown.observedAt)} · 승인 기한 {timeLabel(shown.deadline)}</Text>
        {previous && <Text>직전 예상 {outputLabel(operation,previous.quote.first,previous.quote.second)} · 새 견적 승인 필요</Text>}
        {wallet && <Text>거래 가스 최대 {displayAmount((BigInt(wallet.gasPrice)*600000n).toString(),18)} 실습 가스 단위</Text>}
        {need ? <Action label={`${need.token === 'base' ? 'LAB' : 'LAQ'} 최대 ${displayAmount(need.amount,need.token === 'base' ? 8 : 6)} 사용 허용 승인`}
          disabled={!eligible || positionStale} run={() => work(() => submit(true),'허용 승인을 완료하지 못했습니다. 기존 거래를 확인하세요.')} /> :
          <Action label="견적 확인 및 학습 지갑에서 승인" disabled={!eligible || positionStale || !wallet}
            run={() => work(() => submit(false),'견적 조건이 달라졌거나 네트워크 응답을 확인할 수 없습니다.')} />}
      </View>}
      </>}
      {pending && <View key="market-checkpoint" collapsable={false} style={styles.card}>
        <Text selectable>원래 거래 해시 {pending.hash}</Text>
        <Action label="원래 거래 확정 확인" disabled={busy} run={() => work(reconcile,'원래 거래의 확정 여부를 확인할 수 없습니다.')} />
        <Action label="같은 서명 거래 다시 보내기" disabled={busy} run={() => work(async () => {
          await dispatch(verifyCheckpoint(pending),session);
        },'원래 거래를 다시 보내지 못했습니다.')} />
      </View>}
      {result && !pending && <>
        {receiptActive && !shown && receipt}
        {receiptActive && shown && <Text>{result.state === 'confirmed' ? '거래 확정' : '거래 거절'} · 블록 {result.block}</Text>}
        <Action label="최근 거래 영수증 보기" disabled={busy} run={()=>setReceiptOpen(true)} />
      </>}
      <Action label="격리 학습 지갑 삭제" disabled={busy || !!pending || !!threshold} run={erase} />
    </>}
  </ScrollView>}</>;
}

const styles = StyleSheet.create({
  cloudPanel:{flex:1},
  cloudHidden:{display:'none'},
  container: {padding:8,gap:8},
  title: {fontSize:22,fontWeight:'700'},
  heading: {fontSize:17,fontWeight:'600'},
  card: {borderWidth:1,borderColor:'#676767',borderRadius:8,padding:8,gap:4},
  button: {padding:11,borderRadius:6,backgroundColor:'#e6eef8'},
  disabled: {opacity:0.5},
  input: {borderWidth:1,borderColor:'#676767',borderRadius:6,padding:8},
});
