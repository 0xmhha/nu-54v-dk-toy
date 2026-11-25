import React, {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {Image, StyleSheet, Text, TextInput, View} from 'react-native';
import {
  boardUnit, bytesToHex, dollarToken, fromBase64, payerEnrollChallenge, paymentProfile, paymentToken, paysThroughEscrow, stableNetCoin, tokenAmount, type PayerEnrollment, type PaymentToken,
} from '@nu54/protocol';
import Http from '../specs/NativeCommerceHttp.ts';
import ShopQr from '../specs/NativeShopQr.ts';
import type {Spec as NativeWallet} from '../specs/NativeOwnerWallet.ts';
import {customerClient, walletId} from '../merchant/customerClient.ts';
import {orderRecords} from '../merchant/orderRecords.ts';
import {AmountLine} from '../merchant/paymentParts.tsx';
import {Card, KitButton, Row} from '../ui/kit.tsx';
import {color, font, inputPlaceholder, radius, type} from '../ui/theme.ts';
import {
  atOtherStore, boardFailure, forgetReturn, newReturn, pendingReset, readResetAnswer, readResetBlock, renewReturn, resetBlocked, resetLifetime, resetRequestText, resetUnknown,
  returnRecord, returnStale, serverFailure, settleReturn, submitReturn, verifyReturn, type PendingReset, type ResetBlock, type ReturnRecord,
} from './boardReturn.ts';
import {forgetSend, type SendRecord} from './boardSend.ts';
import type {BoardWait} from './boardSteps.ts';
import {normalizeEvmAddress, readEvmReceiveBalance} from './evmTransfer.ts';
import type {OwnerSession} from './ownerSession.ts';
import type {SpendingStatus} from './ownerWire.ts';
import {SendPage} from './SendPage.tsx';
import {testedBoard, withoutBoardAccount, type WalletStorage} from './profile.ts';
import {readProfiles, walletReadRpc} from './readProfiles.ts';
import {networkRows, serverFor} from './networks.ts';
import {registeredStore} from './registeredStore.ts';
import {receiveQr} from './receivePresentation.ts';
import {readToken, sameStore, storeOf, storeToken, tokensOf, unknownAmount, type Store, type ListedToken} from './tokens.ts';

// Setting up, filling, returning and resetting a spending board. WalletHome owns the board link,
// the protected profile and the busy state; these pages only run one step at a time over them.

export type BoardLifeScreen = 'setup' | 'return-address' | 'deposit' | 'send' | 'return' | 'reset' | 'operator-reset';
export type BoardLifeProps = {
  screen: BoardLifeScreen; stored: WalletStorage; native: NativeWallet | null | undefined; hasEnrollment: boolean; registered?: boolean; spending?: SpendingStatus;
  found: {address: string; name: string}[]; busy: boolean;
  scan: () => void; go: (screen: BoardLifeScreen | 'owner' | 'board' | 'networks') => void; orders: () => void;
  /** One exclusive step; its message replaces the last one. */
  work: (run: () => Promise<void>, failure: (error: unknown) => string) => void;
  say: (message: string) => void;
  withBoard: <T>(handle: string, run: (session: OwnerSession) => Promise<T>) => Promise<T>;
  wait: (wait?: BoardWait) => void;
  commit: (change: (head: WalletStorage) => WalletStorage) => Promise<unknown>;
  copy: (text: string) => void;
  /** The last time the board account proved itself over a connection, and how to note it. */
  seenBoard?: {account: string; generation: number; at: number};
  seen: (account: string, generation: number, limits?: SpendingStatus) => void;
  /** Shown instead of anything read from a server, a chain or the phone's records (design review only). */
  fixture?: {balance?: Balance; open?: number; record?: ReturnRecord; request?: PendingReset; send?: SendRecord; others?: OtherAsset[]};
  /** The store of this phone's registration; these pages read it themselves unless it is given (design review).
   * Without one nobody knows which token the board signs. */
  store?: Store;
  /** Whether the registration was read; set by BoardLife for its pages. */
  storeRead?: 'loading' | 'done'; readStore?: () => void;
  /** Tokens the operator lists for StableNet. */
  listed?: ListedToken[];
};

const pinNote = 'SW1을 짧게 누른 뒤 보드 버튼으로 PIN을 입력하세요. 거절하려면 SW2를 누르세요.';
const now = () => Math.floor(Date.now() / 1000);
const short = (value: string) => value.slice(0, 8) + '…' + value.slice(-6);
const onlyEscrow = (raw: string) => {
  const policy = paymentProfile(raw);
  if (!policy || !paysThroughEscrow(policy)) throw new Error('NO_CREDENTIAL');
  return policy;
};

/** The store credential of this phone, when there is one for an escrow store. */
export async function credential() {
  const raw = Http ? await Http.loadProfile('customer') : '';
  if (!raw) throw new Error('NO_CREDENTIAL');
  return {client: customerClient(raw), policy: onlyEscrow(raw)};
}
const noCredential = '먼저 매장 주문 등록이 필요합니다. 결제 탭에서 매장 담당자가 준 등록서를 입력하세요.';
const explain = (error: unknown, other: (error: unknown) => string) =>
  error instanceof Error && error.message === 'NO_CREDENTIAL' ? noCredential : error instanceof Error && tokenFailures[error.message] || other(error);

const storeText = (units: bigint, store?: Store) => store ? tokenAmount(units, store.token) : unknownAmount.ko;
const assetText = (units: bigint, asset: string, store?: Store) => store?.asset === asset.toLowerCase() ? tokenAmount(units, store.token) : unknownAmount.ko;

/** The token a transfer of this phone was made for. A registration with another store cannot name it. */
export async function signingToken(asset: string): Promise<PaymentToken> {
  const {policy} = await credential();
  if (policy.asset.toLowerCase() !== asset.toLowerCase()) throw new Error('OTHER_STORE');
  return paymentToken(policy);
}
/** Before the board is asked: what it signs must be what this screen shows. */
export async function shownToken(asset: string, shown?: Store): Promise<PaymentToken> {
  const token = await signingToken(asset);
  if (!sameStore(shown, {asset: asset.toLowerCase(), token})) throw new Error('TOKEN_CHANGED');
  return token;
}
export const tokenFailures: Record<string, string> = {
  OTHER_STORE: '다른 매장에 등록되어 있어 이 거래를 이어서 할 수 없습니다. 이 거래를 만든 매장의 등록서가 필요합니다.',
  SENT_AT_OTHER_STORE: '이 거래는 다른 매장을 통해 보냈습니다. 돈이 이미 나갔을 수 있습니다. 그 매장에 다시 등록해 결과를 확인하고, 다시 보내지 마세요.',
  RETURN_SIGNATURE: '이 거래의 서명이 매장 등록의 토큰과 맞지 않아 보내지 않았습니다. 이 거래를 만든 매장의 등록서가 필요합니다.',
  SEND_SIGNATURE: '이 거래의 서명이 매장 등록의 토큰과 맞지 않아 보내지 않았습니다. 이 거래를 만든 매장의 등록서가 필요합니다.',
  TOKEN_CHANGED: '화면의 토큰과 매장 등록의 토큰이 다르거나 확인되지 않았습니다. 이 화면을 닫고 다시 여세요. 빠져나간 돈은 없습니다.',
};

/** Finalized and newest balance of one token (the store's unless named), or undefined when either cannot be read. */
type Balance = {units: bigint; latest: bigint; text: string; /** There is also less than the board can count, which it cannot send. */ dust?: boolean};
async function tokenBalance(props: Pick<BoardLifeProps, 'stored' | 'native' | 'store'>, token?: string): Promise<Balance | undefined> {
  const {verified, walletRead} = props.stored, native = props.native;
  if (!verified || !native) return undefined;
  try {
    const policy = token && walletRead?.endpoints.stable ? undefined
      : await credential().then(value => value.policy, error => { if (token) return undefined; throw error; });
    const asset = token ?? policy!.asset;
    const rpc = walletReadRpc(serverFor('stable', walletRead, {store: policy?.rpc}).server, (url, packet) => native.evmRpc(url, packet));
    const read = async (tag: 'finalized' | 'latest') => BigInt((await readEvmReceiveBalance(rpc, '8283', readProfiles.stable.anchor,
      verified.address, asset, false, tag)).amount);
    const store = policy ? storeOf(policy) : props.store, unit = store?.asset === asset.toLowerCase() ? boardUnit(store.token) : 1n;
    const held = await read('finalized'), newest = await read('latest');
    const units = held - held % unit;
    return {units, latest: newest - newest % unit, text: assetText(units, asset, store), ...(held % unit || newest % unit ? {dust: true} : {})};
  } catch {
    return undefined;
  }
}
export function useBalance(props: Pick<BoardLifeProps, 'stored' | 'native' | 'fixture' | 'store'>) {
  const [balance, setBalance] = useState<Balance | null | undefined>(props.fixture ? props.fixture.balance ?? null : undefined);
  const latest = useRef(props);
  latest.current = props;
  const read = useCallback(async (token?: string) => {
    if (latest.current.fixture) return latest.current.fixture.balance;
    const value = await tokenBalance(latest.current, token);
    setBalance(value ?? null);
    return value;
  }, []);
  useEffect(() => { read().catch(() => {}); }, [read]);
  return {balance, read};
}

export function Chooser({props, label, run, disabled = false}: {props: BoardLifeProps; label: string; run: (handle: string) => void; disabled?: boolean}) {
  return <View style={styles.group}>
    {props.found.map((item, index) => <KitButton key={item.address} disabled={props.busy || disabled}
      label={`${label}${props.found.length > 1 ? ` · ${item.name || '이름 없는 보드'} ${index + 1}` : ''}`} run={() => run(item.address)} />)}
    <KitButton label={props.found.length ? '내 보드 다시 찾기' : '내 보드 찾기'} run={props.scan} disabled={props.busy || disabled}
      variant={props.found.length ? 'secondary' : 'primary'} />
  </View>;
}

export function Notice({title, body, tone = 'wait', children}: {title?: string; body: string; tone?: 'wait' | 'stop' | 'done'; children?: React.ReactNode}) {
  return <View style={[styles.notice, tone === 'stop' && styles.noticeStop, tone === 'done' && styles.noticeDone]}>
    {!!title && <Text accessibilityRole="header" style={[styles.noticeTitle, tone === 'stop' && styles.stop]}>{title}</Text>}
    <Text style={styles.noticeBody}>{body}</Text>{children}
  </View>;
}

export type SetupStep = {label: string; detail: string; state: 'done' | 'now' | 'later'; optional?: true};
/** The one line of first use. Every step is read from what is stored or reported, so it resumes anywhere. */
export function setupSteps(stored: WalletStorage, hasEnrollment: boolean, payer?: PayerEnrollment | null, registered = false): SetupStep[] {
  const board = testedBoard(stored);
  const enrolled = !!board && payer?.payer === board.account && payer.generation === String(board.generation) && payer.expiresAt > now();
  const done = [hasEnrollment && (!!stored.verified || registered), !!board, !!board?.returnAddress, enrolled, false];
  const rows: [string, string][] = [
    ['보드 연결과 PIN', '보드 버튼으로 이 휴대전화를 승인하고 PIN을 정합니다'],
    ['서명 시험', '보드 계정의 서명을 확인합니다. 버튼은 누르지 않습니다'],
    ['반납받을 주소', board?.returnAddress ? short(board.returnAddress) : '남은 돈을 돌려받을 원래 지갑 주소를 보드에 저장합니다'],
    ['매장 등록', payer && board && !enrolled ? '다른 보드 계정이 등록되어 있습니다. 다시 등록하세요' : '결제할 매장에 보드 계정을 등록합니다. 나중에 해도 됩니다'],
    ['입금', '쓸 만큼만 원래 지갑에서 보냅니다'],
  ];
  const first = done.findIndex((value, index) => !value && index !== 3);
  return rows.map(([label, detail], index) => ({label, detail, state: done[index] ? 'done' : index === first || (index === 3 && !!board) ? 'now' : 'later',
    ...(index === 3 ? {optional: true as const} : {})}));
}

export function SetupList({steps}: {steps: SetupStep[]}) {
  return <Card style={styles.steps}>{steps.map((step, index) => <View key={step.label} style={styles.step} accessible
    accessibilityLabel={`${index + 1}. ${step.label}, ${{done: '완료', now: step.optional ? '선택' : '지금 할 일', later: '다음'}[step.state]}. ${step.detail}`}>
    <View style={[styles.number, step.state === 'done' && styles.numberDone, step.state === 'now' && styles.numberNow]}>
      <Text style={[styles.numberText, step.state !== 'later' && styles.numberTextOn]}>{index + 1}</Text></View>
    <View style={styles.words}><Text style={type.label}>{step.label}{step.state === 'done' ? ' · 완료' : step.optional ? ' · 선택' : ''}</Text>
      <Text style={type.small}>{step.detail}</Text></View>
  </View>)}</Card>;
}

/** `undefined` while asking, null when there is none or it cannot be read. */
export function usePayer(active: boolean) {
  const [payer, setPayer] = useState<PayerEnrollment | null>(), [unregistered, setUnregistered] = useState(false);
  const read = useCallback(async () => {
    let missing = false;
    const value = await credential().then(({client}) => client.enrolledPayer())
      .catch(error => { missing = error instanceof Error && error.message === 'NO_CREDENTIAL'; return undefined; });
    setUnregistered(missing);
    setPayer(value ?? null);
    return value;
  }, []);
  useEffect(() => { if (active) read().catch(() => {}); }, [active, read]);
  return {payer, read, unregistered};
}

/** Setup progress on 내 보드. */
export function SetupStatus({stored, hasEnrollment, registered, open, live = true}: {stored: WalletStorage; hasEnrollment: boolean; registered?: boolean; open: () => void; live?: boolean}) {
  const {payer} = usePayer(live && !!testedBoard(stored));
  return <View style={styles.group}><Text accessibilityRole="header" style={type.section}>보드 설정</Text>
    <SetupList steps={setupSteps(stored, hasEnrollment, payer, registered)} />
    <KitButton label="설정 열기" run={open} variant="secondary" /></View>;
}

function SetupPage(props: BoardLifeProps) {
  const {stored, work, withBoard, commit, say} = props;
  const {payer, read, unregistered} = usePayer(!props.fixture);
  const steps = setupSteps(stored, props.hasEnrollment, payer, props.registered);
  const board = testedBoard(stored), verified = stored.verified;
  const test = (handle: string) => work(async () => {
    await withBoard(handle, async session => {
      const status = await session.status(), own = await session.identity();
      if (status.state === 'released' || status.state === 'new') throw new Error('BOARD_NOT_REGISTERED');
      if (status.state !== 'active' || status.owner !== own.fingerprint || status.generation > BigInt(Number.MAX_SAFE_INTEGER)) throw new Error('PAYMENT_BOARD_BINDING');
      const account = await session.spendingAccount(status);
      const generation = Number(status.generation);
      // Same registration, another account: not the board this phone knows. Another registration was cleared on connecting.
      if (verified && verified.generation === generation && verified.address !== account) throw new Error('ACCOUNT_CHANGED');
      const limits = await session.spendingStatus(status.generation);
      await commit(head => ({...(head.verified && head.verified.generation !== generation ? withoutBoardAccount(head) : head),
        verified: {boardFingerprint: status.device, address: account, generation, network: '8283'},
        spendingBoard: {device: status.device, account, generation, testedAt: now(), ...(limits.returnAddress ? {returnAddress: limits.returnAddress} : {})}}));
      props.seen(account, generation, limits);
    });
    say('보드 서명을 확인했습니다. 이제 입금 주소를 볼 수 있습니다.');
  }, boardFailure);
  const enroll = (handle: string) => work(async () => {
    const {client} = await credential();
    const key = await walletId();
    const generation = BigInt(board!.generation);
    const {signature} = await withBoard(handle, async session => {
      const status = await session.maintenanceStatus();
      if (status.generation !== generation) throw new Error('PAYMENT_BOARD_BINDING');
      return session.proveAccount(board!.account, payerEnrollChallenge({tenantRef: client.profile.tenantRef, branchRef: client.profile.branchRef,
        subjectRef: client.profile.subjectRef, payer: board!.account, generation, idempotencyKey: key}));
    }).catch(error => { throw new Error(boardFailure(error)); });
    await client.enrollPayer(board!.account, generation, signature, key).catch(async error => {
      // A lost reply is settled by what the server reports, not by asking the board again.
      const held = await read();
      if (held?.payer !== board!.account || held.generation !== String(board!.generation)) throw new Error(serverFailure(error));
    });
    await read();
    say('이 매장에 보드 계정을 등록했습니다.');
  }, error => error instanceof Error && /[가-힣]/.test(error.message) ? error.message : explain(error, serverFailure));
  const step = steps.find(item => item.state === 'now' && !item.optional)?.label;
  return <>
    <SetupList steps={steps} />
    {step === '보드 연결과 PIN' && <KitButton label="보드 연결하기" run={() => props.go('owner')} disabled={props.busy} />}
    {(step === '서명 시험' || (props.hasEnrollment && !board && step === '보드 연결과 PIN')) && <>
      <Text style={type.body}>보드 계정이 정말 이 보드의 것인지 확인합니다. 통과하기 전에는 입금 주소를 보여 주지 않습니다.</Text>
      <Chooser props={props} label="서명 시험 하기" run={test} />
    </>}
    {step === '반납받을 주소' && <KitButton label="반납받을 주소 등록" run={() => props.go('return-address')} disabled={props.busy} />}
    {step === '입금' && <KitButton label="입금 안내 보기" run={() => props.go('deposit')} disabled={props.busy} />}
    {board && steps[3].state !== 'done' && <>
      <Text style={type.section}>매장 등록</Text>
      <Text style={type.body}>이 휴대전화의 주문 등록을 쓰는 매장에 보드 계정을 알립니다. 보드 버튼은 누르지 않습니다.</Text>
      {unregistered && <KitButton label="결제 탭에서 주문 등록하기" run={props.orders} disabled={props.busy} variant="secondary" />}
      <Chooser props={props} label="이 매장에 보드 등록" run={enroll} />
    </>}
  </>;
}

function ReturnAddressPage(props: BoardLifeProps) {
  const board = testedBoard(props.stored);
  const [first, setFirst] = useState(''), [second, setSecond] = useState(''), [checked, setChecked] = useState('');
  if (!board) return <Notice body="먼저 서명 시험을 마치세요." />;
  if (board.returnAddress) return <><Notice title="반납받을 주소가 저장되어 있습니다" body="보드에 한 번만 저장할 수 있습니다. 바꾸려면 보드를 초기화해야 합니다." />
    <Text selectable style={styles.address}>{board.returnAddress}</Text></>;
  const review = () => {
    try {
      const [a, b] = [normalizeEvmAddress(first.trim()), normalizeEvmAddress(second.trim())];
      if (a !== b) { props.say('두 주소가 서로 다릅니다. 다시 확인하세요.'); return; }
      if (a === board.account || /^0x0+$/.test(a)) { props.say('보드 계정이 아닌 원래 지갑 주소를 입력하세요.'); return; }
      props.say('');
      setChecked(a);
    } catch { props.say('주소 형식이 맞지 않습니다. 원래 지갑에서 주소를 다시 복사하세요.'); }
  };
  const store = (handle: string) => props.work(async () => {
    await props.withBoard(handle, session => session.setReturnAddress(BigInt(board.generation), board.account, checked,
      () => props.wait({flow: 'address', step: 'approve', note: pinNote})));
    await props.commit(head => head.spendingBoard ? {...head, spendingBoard: {...head.spendingBoard, returnAddress: checked}} : head);
    props.say('보드에 반납받을 주소를 저장했습니다.');
  }, boardFailure);
  const field = (label: string, value: string, change: (text: string) => void) => <TextInput accessibilityLabel={label} placeholder={label} placeholderTextColor={inputPlaceholder}
    value={value} onChangeText={text => { change(text); setChecked(''); }} style={styles.input} maxLength={42} autoCapitalize="none" autoCorrect={false} editable={!props.busy} />;
  return <>
    <Text style={type.body}>반납하면 남은 돈이 이 주소로만 갑니다. 원래 지갑의 주소를 두 번 입력하세요. 한 번 저장하면 초기화하기 전에는 바꿀 수 없습니다.</Text>
    {field('원래 지갑 주소', first, setFirst)}
    {field('원래 지갑 주소 다시', second, setSecond)}
    {!checked ? <KitButton label="주소 확인" run={review} disabled={props.busy || !first || !second} /> : <>
      <Notice title="이 주소가 맞나요?" body="원래 지갑 앱의 주소와 한 글자씩 비교하세요."><Text selectable style={styles.address}>{checked}</Text></Notice>
      <Chooser props={props} label="보드에 저장" run={store} />
    </>}
  </>;
}

/** How long the deposit address stays on screen after the board proved itself. */
const seenFor = 600_000;
function DepositPage(props: BoardLifeProps) {
  const board = testedBoard(props.stored);
  const {balance, read} = useBalance(props);
  const [clock, setClock] = useState(Date.now());
  useEffect(() => {
    const timer = setInterval(() => setClock(Date.now()), 5000);
    return () => clearInterval(timer);
  }, []);
  const [networkSeen, setNetworkSeen] = useState(false);
  if (!board) return <Notice title="아직 입금하지 마세요" body="이 등록에서 서명 시험을 통과해야 입금 주소를 보여 줍니다.">
    <KitButton label="설정 이어서 하기" run={() => props.go('setup')} variant="secondary" /></Notice>;
  const age = props.seenBoard && props.seenBoard.account === board.account && props.seenBoard.generation === board.generation
    ? Math.max(clock, props.seenBoard.at) - props.seenBoard.at : Infinity;
  const warning = <Notice tone="stop" body="보드를 버튼으로 초기화했다면 이 휴대전화가 알던 주소로 보내지 마세요. 그 주소의 열쇠는 지워져 돈을 되찾을 수 없습니다." />;
  if (age >= seenFor) return <>
    {warning}
    <Notice title="주소를 숨겼습니다" body="보드를 다시 연결해 확인하세요. 확인한 뒤 10분 동안 주소를 보여 줍니다." />
    <Chooser props={props} label="보드 연결해 확인" run={handle => props.work(async () => {
      await props.withBoard(handle, async session => {
        const status = await session.status();
        if (status.state === 'released' || status.state === 'new' || status.generation !== BigInt(board.generation)) throw new Error('BOARD_WAS_RESET');
        if (await session.spendingAccount(status) !== board.account) throw new Error('ACCOUNT_CHANGED');
        props.seen(board.account, board.generation);
      });
      setClock(Date.now());
    }, boardFailure)} /></>;
  // The same address exists on every EVM network, and what arrives there on another one cannot be moved.
  const label = props.store?.token.label;
  if (props.storeRead === 'loading') return <Text style={type.body}>매장 등록을 확인하고 있습니다.</Text>;
  if (props.store?.token.symbol !== dollarToken.symbol && !networkSeen) return <>
    {warning}
    <Notice tone="stop" title="StableNet 테스트넷으로만 보내세요" body={`${label ? `${label}는 StableNet 테스트넷에만 있습니다.` : '매장 등록을 확인하지 못해 이 보드가 쓰는 토큰을 알 수 없습니다. 확인하기 전에는 보내지 않는 것을 권장합니다.'} 다른 네트워크나 다른 토큰으로 보낸 것은 이 보드가 다시 보낼 수 없습니다. 시험용이며 실제 돈이 아닙니다.`} />
    <KitButton label="확인했습니다. 주소 보기" run={() => setNetworkSeen(true)} disabled={props.busy} /></>;
  return <>
    {warning}
    <Text style={type.body}>원래 지갑에서 아래 주소로 {label ? `${label}만` : '매장이 알려 준 토큰만'} 보내세요. 다른 토큰이나 코인은 쓸 수 없습니다. 보통 송금이면 되고, 다른 승인이나 등록은 필요 없습니다. 쓸 만큼만 보내세요. 보드를 잃어버리면 남은 돈을 되찾을 수 없습니다.</Text>
    <Card><Row label="네트워크" value="StableNet 테스트넷" /><Row label="토큰" value={label ?? unknownAmount.ko} /></Card>
    <View style={styles.qr}><Image accessibilityLabel="입금 주소 QR" source={{uri: receiveQr(board.account)}} style={styles.qrImage} /></View>
    <Text selectable style={styles.address}>{board.account}</Text>
    <KitButton label="주소 공유" run={() => props.copy(board.account)} variant="secondary" disabled={props.busy} />
    <Card><Row label="보드 계정 잔액" value={balance === undefined ? '확인하는 중' : balance?.text ?? '확인 안 됨'} />
      {balance === null && <Text style={type.small}>네트워크 주소나 매장 등록이 없어 잔액을 읽지 못했습니다. 보낸 돈은 원래 지갑의 거래 기록에서 확인하세요.</Text>}</Card>
    <KitButton label="잔액 다시 확인" run={() => props.work(async () => { await read(); }, () => '잔액을 확인하지 못했습니다.')} disabled={props.busy} variant="secondary" />
    {balance === null && <KitButton label="네트워크 주소 설정" run={() => props.go('networks')} disabled={props.busy} variant="secondary" />}
    {!board.returnAddress && <Notice body="반납받을 주소를 아직 저장하지 않았습니다. 입금하기 전에 저장하는 것을 권장합니다.">
      <KitButton label="반납받을 주소 등록" run={() => props.go('return-address')} variant="secondary" /></Notice>}
  </>;
}

const unfinished = (stored: WalletStorage) => !!(stored.signing || stored.pendingEvm || stored.pendingChain || stored.pendingBip48);

/** What still needs the key of this account, from this phone's records and the store's server. */
async function askResetBlock(stored: WalletStorage): Promise<ResetBlock> {
  const {verified} = stored;
  if (!Http || !verified) return {orders: 0, returning: false, unreadable: false, server: 'none'};
  return readResetBlock(Http, orderRecords(Http), {payer: verified.address, generation: String(verified.generation)}, () =>
    credential().then(({client}) => client, error => {
      if (error instanceof Error && error.message === 'NO_CREDENTIAL') return undefined;
      throw error;
    }));
}
function useResetBlock(props: BoardLifeProps) {
  const {stored, fixture} = props;
  const [block, setBlock] = useState<ResetBlock | undefined>(fixture ? {orders: fixture.open ?? 0, returning: false, unreadable: false, server: 'ok'} : undefined);
  const read = useCallback(async () => {
    const value = await askResetBlock(stored);
    setBlock(value);
    return value;
  }, [stored]);
  useEffect(() => { if (!fixture) read().catch(() => {}); }, [fixture, read]);
  return {block, read};
}

// Answers that say nothing was erased. Anything else leaves the result open.
const notErased = ['RESET_BLOCKED', 'RESET_UNKNOWN', 'BOARD_REFUSED', 'BOARD_ORDER_OPEN', 'ORDER_PIN_WAIT', 'ACCOUNT_CHANGED', 'BOARD_PERMIT', 'BOARD_SESSION_SHORT'];
/** Asks for an erase. Until the result is known the deposit address is withheld, since its key may be gone. */
async function erase(props: BoardLifeProps, run: () => Promise<void>) {
  const withhold = (on: boolean) => props.commit(head => {
    if (!head.spendingBoard) return head;
    const board = {...head.spendingBoard};
    if (on) board.withheld = true; else delete board.withheld;
    return {...head, spendingBoard: board};
  });
  const doubted = !!props.stored.spendingBoard?.withheld;
  await withhold(true);
  try {
    await run();
  } catch (error) {
    if (!doubted && error instanceof Error && notErased.includes(error.message)) await withhold(false);
    throw error;
  }
}

/** Clears what the phone kept for the account of a board that was reset. Orders stay, under their former account. */
async function finishReset(props: BoardLifeProps) {
  if (Http) { await forgetReturn(Http); await forgetSend(Http); await pendingReset(Http).save(undefined); }
  await props.commit(withoutBoardAccount);
  props.go('setup');
  props.say('보드를 초기화했습니다. 다시 쓰려면 운영자 허가를 받은 뒤 처음부터 등록하세요.');
}

const unsettled = '아직 확정되지 않은 입출금이 있습니다. 잠시 뒤 다시 시도하세요.';
const blockedText = '끝나지 않은 주문이나 진행 중인 반납이 있거나, 있는지 확인하지 못했습니다. 주문과 반납을 먼저 확인하세요.';
const resetFailure = (error: unknown) => error instanceof Error && error.message === 'ASSETS_LEFT' ? assetsLeft : error instanceof Error && ['RESET_BLOCKED', 'RESET_UNKNOWN'].includes(error.message) ? blockedText : boardFailure(error);

function OpenOrderNotice({props, orders}: {props: BoardLifeProps; orders: number}) {
  return <Notice tone="stop" title="끝나지 않은 주문이 있습니다" body={`주문 ${orders}건을 먼저 끝내세요. 보드의 키를 지우면 그 주문의 환불금을 받을 수 없습니다.`}>
    <KitButton label="진행 중인 주문 보기" run={props.orders} variant="secondary" /></Notice>;
}

/** What still needs the board key. Drawn instead of the way on; unknown parts need their own confirmation. */
function useResetGate(props: BoardLifeProps) {
  const {block, read} = useResetBlock(props);
  const [accepted, setAccepted] = useState(false);
  const unknown = !!block && resetUnknown(block);
  const open = !!block && !resetBlocked(block) && (!unknown || accepted);
  const gate = !block ? <Text style={type.body}>끝나지 않은 주문과 반납을 확인하고 있습니다.</Text>
    : block.orders > 0 ? <OpenOrderNotice props={props} orders={block.orders} />
    : block.returning ? <Notice tone="stop" title="끝나지 않은 반납이 있습니다" body="반납이 확정되거나 실패할 때까지 초기화할 수 없습니다.">
      <KitButton label="반납 보기" run={() => props.go('return')} variant="secondary" /></Notice>
    : unknown && !accepted ? <Notice tone="stop" title="끝나지 않은 주문이 있는지 알 수 없습니다"
      body={`${block.unreadable ? '이 휴대전화의 주문 기록을 읽을 수 없습니다.' : '매장 서버에 확인하지 못했습니다.'} 끝나지 않은 주문이 있으면 그 환불금을 영영 받을 수 없습니다.`}>
      <KitButton label="다시 확인" run={() => { read().catch(() => {}); }} variant="secondary" disabled={props.busy} />
      <KitButton label="환불을 잃을 수 있음을 알고 계속" run={() => setAccepted(true)} variant="secondary" disabled={props.busy} /></Notice>
    : null;
  /** Asked again right before the board is, since an order can appear while the page is open. */
  const verify = async () => {
    if (props.fixture) return;
    const fresh = await read();
    if (resetBlocked(fresh)) throw new Error('RESET_BLOCKED');
    if (resetUnknown(fresh) && !accepted) throw new Error('RESET_UNKNOWN');
  };
  return {open, gate, verify};
}

export type OtherAsset = {key: string; symbol: string; text: string};
/** What the board account holds besides the token it can return. `null` when a balance could not be read. */
async function otherAssets(props: BoardLifeProps, returned?: string): Promise<OtherAsset[] | null> {
  const {stored, native} = props, {verified, walletRead} = stored;
  if (!verified || !native) return [];
  const policy = await credential().then(value => value.policy, () => undefined);
  const row = networkRows(walletRead, {store: policy?.rpc}).find(item => item.key === 'stable')!;
  // Without a return in progress the store's token counts as money the reset page warns of by itself.
  const kept = (returned ?? policy?.asset)?.toLowerCase();
  const tokens = tokensOf(row, verified, walletRead, policy ? [storeToken(policy)] : [], props.listed).filter(token => (token.contract ?? (kept === stableNetCoin.asset ? kept : undefined)) !== kept || kept === undefined);
  const found = await Promise.all(tokens.map(async token => ({token, amount: await readToken(row, token, verified.address, (url, packet) => native.evmRpc(url, packet))})))
    .catch(() => null);
  return found && found.filter(({amount}) => amount.units !== '0').map(({token, amount}) => ({key: token.key, symbol: token.symbol, text: token.won ? amount.text : `${amount.text} ${token.symbol}`}));
}

/** Assets that go with the key when the board is reset. Each one has to be given up by name. */
function useLossGate(props: BoardLifeProps, returned?: string) {
  const {fixture} = props;
  const [others, setOthers] = useState<OtherAsset[] | null | undefined>(fixture ? fixture.others ?? [] : undefined);
  const [given, setGiven] = useState<string[]>([]);
  const latest = useRef(props);
  latest.current = props;
  const read = useCallback(async () => {
    const value = await otherAssets(latest.current, returned);
    setOthers(value);
    return value;
  }, [returned]);
  useEffect(() => { if (!fixture) read().catch(() => setOthers(null)); }, [fixture, read]);
  const unknown = others === null;
  const left = (others ?? []).filter(item => !given.includes(item.text));
  const open = others !== undefined && (unknown ? given.includes('unknown') : !left.length);
  const gate = others === undefined ? <Text style={type.body}>보드 계정의 다른 자산을 확인하고 있습니다.</Text>
    : unknown ? <Notice tone="stop" title="다른 자산이 있는지 확인하지 못했습니다" body="보드 계정에 다른 토큰이나 코인이 있으면 초기화할 때 잃습니다.">
      <KitButton label="다른 자산 다시 확인" run={() => { read().catch(() => {}); }} variant="secondary" disabled={props.busy} />
      {!open && <KitButton label="다른 자산을 잃을 수 있음을 알고 계속" run={() => setGiven(['unknown'])} variant="secondary" disabled={props.busy} />}</Notice>
    : others.length ? <Notice tone="stop" title="보드가 보낼 수 없는 자산이 있습니다" body="초기화하면 되찾을 수 없습니다. 잃어도 되는지 자산마다 확인하세요.">
      {others.map(item => given.includes(item.text) ? <Text key={item.key} style={styles.risk}>{`잃게 될 자산: ${item.text}`}</Text>
        : <KitButton key={item.key} label={`${item.text} · 잃어도 됩니다`} run={() => setGiven([...given, item.text])} variant="secondary" disabled={props.busy} />)}</Notice>
    : null;
  /** Asked again right before the board is: something may have arrived meanwhile. */
  const verify = async () => {
    if (fixture) return;
    const fresh = await read();
    if (fresh ? fresh.some(item => !given.includes(item.text)) : !given.includes('unknown')) throw new Error('ASSETS_LEFT');
  };
  return {open, gate, verify};
}
const assetsLeft = '보드 계정에 남은 다른 자산이 있거나 확인하지 못했습니다. 위 내용을 확인한 뒤 다시 시도하세요.';

function ReturnPage(props: BoardLifeProps) {
  const {stored, work, withBoard, say} = props;
  const {verified} = stored, board = testedBoard(stored);
  const {balance, read} = useBalance(props);
  const {block} = useResetBlock(props);
  const [record, setRecord] = useState<ReturnRecord | null | undefined>(props.fixture ? props.fixture.record ?? null : undefined);
  const keep = useCallback(async (next?: ReturnRecord) => { await returnRecord(Http!).save(next); setRecord(next ?? null); }, []);
  useEffect(() => {
    let live = true;
    if (props.fixture) return;
    if (!Http) { setRecord(null); return; }
    returnRecord(Http).load().then(value => { if (live) setRecord(value ?? null); }).catch(() => { if (live) say('반납 기록을 읽을 수 없습니다. 지원을 요청하세요.'); });
    return () => { live = false; };
  }, [say, props.fixture]);
  const check = useCallback(async () => {
    const {client} = await credential();
    const next = await settleReturn(Http!, client, now);
    setRecord(next ?? null);
    if (!next) say('반납이 실행되지 않은 것을 확인했습니다. 돈은 보드 계정에 그대로 있습니다. 다시 시작할 수 있습니다.');
  }, [say]);
  const loss = useLossGate(props, record?.token);
  const waiting = record?.stage === 'sent' && !props.fixture;
  useEffect(() => {
    if (!waiting) return;
    const timer = setInterval(() => { check().catch(() => {}); }, 10_000);
    return () => clearInterval(timer);
  }, [waiting, check]);
  if (record === undefined || block === undefined) return <Text style={type.body}>반납 상태를 확인하고 있습니다.</Text>;
  if (!record) {
    if (!verified || !board) return <Notice body="이 휴대전화에 등록된 보드 계정이 없습니다." />;
    if (block.unreadable) return <Notice tone="stop" title="반납할 수 없습니다" body="주문 기록을 읽을 수 없어 끝나지 않은 주문이 있는지 알 수 없습니다. 지원을 요청하세요." />;
    if (block.orders > 0) return <OpenOrderNotice props={props} orders={block.orders} />;
    if (block.returning) return <Notice title="진행 중인 반납이 있습니다" body="매장 서버가 이 계정의 반납을 처리하고 있습니다. 끝난 뒤 다시 확인하세요." />;
    if (unfinished(stored)) return <Notice tone="stop" title="반납할 수 없습니다" body="결과를 모르는 다른 전송이나 서명이 있습니다. 먼저 그 결과를 확인하세요." />;
    if (!board.returnAddress) return <Notice title="반납받을 주소가 없습니다" body="원래 지갑 주소를 보드에 먼저 저장하세요.">
      <KitButton label="반납받을 주소 등록" run={() => props.go('return-address')} variant="secondary" /></Notice>;
    const start = () => work(async () => {
      const money = await read();
      if (!money) { say('보드 계정의 잔액을 확인하지 못해 반납을 시작하지 않았습니다.'); return; }
      if (money.latest !== money.units) { say(unsettled); return; }
      const fresh = await askResetBlock(stored);
      // With nothing to send the server is not asked to accept anything, so its silence counts against it.
      if (resetBlocked(fresh) || fresh.unreadable || (money.units === 0n && resetUnknown(fresh))) throw new Error('RESET_BLOCKED');
      const {client, policy} = await credential();
      const payer = await client.enrolledPayer();
      if (money.units > 0n && (payer?.payer !== verified.address || payer.generation !== String(verified.generation)))
        throw new Error('PAYER_NOT_ENROLLED');
      if (money.units > 0n) await shownToken(policy.asset, props.store);
      const nonce = bytesToHex(fromBase64(await Http!.randomBytes(32)), false);
      await keep(newReturn({payer: verified.address, generation: String(verified.generation), device: board.device, token: policy.asset, to: board.returnAddress!},
        money.units, nonce, await walletId(), now()));
    }, error => error instanceof Error && error.message === 'PAYER_NOT_ENROLLED' ? '이 매장에 보드 계정이 등록되어 있지 않습니다. 설정에서 매장 등록을 먼저 하세요.'
      : error instanceof Error && error.message === 'RESET_BLOCKED' ? blockedText : explain(error, serverFailure));
    return <>
      <Text style={type.body}>남은 돈을 모두 원래 지갑으로 보내고, 확정된 뒤에 보드를 초기화합니다.</Text>
      <Card><AmountLine amount={balance === undefined ? '확인하는 중' : balance?.text ?? '확인 안 됨'} label="돌려받을 금액" />
        {balance?.dust && <Text style={type.small}>보드가 셀 수 있는 가장 작은 금액보다 작은 나머지는 보내지 못하고 남습니다.</Text>}
        <Text style={type.small}>받는 주소 (보드에 저장된 원래 지갑)</Text><Text selectable style={styles.address}>{board.returnAddress}</Text></Card>
      {balance === null && <Notice body="보드 계정의 잔액을 확인할 수 없습니다. 네트워크 주소와 매장 등록을 확인하세요.">
        <KitButton label="잔액 다시 확인" run={() => work(async () => { await read(); }, () => '잔액을 확인하지 못했습니다.')} variant="secondary" /></Notice>}
      <KitButton label={balance?.units === 0n ? '잔액 없이 초기화로 진행' : '반납 시작'} run={start} disabled={props.busy || !balance} />
    </>;
  }
  const summary = record.amount !== '0' && <Card><AmountLine amount={assetText(BigInt(record.amount), record.token, props.store)} label="돌려받을 금액" />
    <Text style={type.small}>받는 주소</Text><Text selectable style={styles.address}>{record.to}</Text></Card>;
  const restart = () => work(async () => { await keep(undefined); await read(); say('반납을 처음부터 다시 시작합니다. 빠져나간 돈은 없습니다.'); }, () => '반납 기록을 바꾸지 못했습니다.');
  const former = !verified || verified.address !== record.payer || String(verified.generation) !== record.generation;
  if (former && record.stage === 'final') return <>{summary}<Notice title="반납이 확정되었습니다" body="이 반납을 한 보드는 이미 초기화되었습니다." />
    <KitButton label="확인" disabled={props.busy} run={() => work(() => keep(undefined), () => '반납 기록을 바꾸지 못했습니다.')} /></>;
  if (returnStale(record, now())) return <>{summary}<Notice body="승인할 수 있는 시간이 지났습니다. 이 반납은 보내지 않았고 빠져나간 돈도 없습니다." />
    <KitButton label="다시 시작" run={restart} disabled={props.busy} /></>;
  if (record.stage === 'signing') return <>{summary}
    <Text style={type.body}>보드에서 위 금액과 주소의 송금을 승인하세요.</Text>
    <Chooser props={props} label="보드로 승인" run={handle => work(async () => {
      const token = await shownToken(record.token, props.store), {name} = token;
      const signed = await withBoard(handle, async session => {
        const asked = renewReturn(record, now());
        await keep(asked);
        const signature = await session.signReturn(BigInt(asked.generation), {...asked, amount: BigInt(asked.amount)},
          () => props.wait({flow: 'return', step: 'approve', note: pinNote}), name, token);
        const next = verifyReturn(asked, signature, name);
        await keep(next);
        return next;
      }).catch(error => { throw new Error(boardFailure(error)); });
      // The signature is written down and lives for minutes only, so it is sent at once.
      const {client} = await credential();
      await submitReturn(client, signed, async next => keep(next), now(), (await signingToken(record.token)).name);
      await check();
    }, error => error instanceof Error && /[가-힣]/.test(error.message) ? error.message : explain(error, serverFailure))} />
    <KitButton label="반납 취소" run={restart} disabled={props.busy} variant="secondary" /></>;
  if (record.stage === 'signed') return <>{summary}
    <Text style={type.body}>보드가 승인한 반납을 아직 보내지 않았습니다. 수수료는 매장이 냅니다.</Text>
    <KitButton label="반납 보내기" disabled={props.busy} run={() => work(async () => {
      const {client} = await credential();
      await submitReturn(client, record, async next => keep(next), now(), (await signingToken(record.token)).name);
      await check();
    }, error => explain(error, serverFailure))} />
    {record.sentAt === undefined && <KitButton label="반납 취소" run={restart} disabled={props.busy} variant="secondary" />}</>;
  if (record.stage === 'sent' && atOtherStore(record, props.store?.branch)) return <>{summary}
    <Notice tone="stop" title="결과 확인 안 됨" body={tokenFailures.SENT_AT_OTHER_STORE} /></>;
  if (record.stage === 'sent') return <>{summary}
    <Notice title="반납을 확인하고 있습니다" body="거래가 확정될 때까지 기다립니다. 두 번 보내지 않습니다. 앱을 열어 두면 다른 화면에서도 이어서 확인합니다." />
    <KitButton label="지금 확인" run={() => work(check, error => explain(error, serverFailure))} disabled={props.busy} />
    <KitButton label="같은 반납 다시 보내기" variant="secondary" disabled={props.busy || record.validBefore <= now()} run={() => work(async () => {
      const {client} = await credential();
      await submitReturn(client, record, async next => keep(next), now(), (await signingToken(record.token)).name);
      await check();
    }, error => explain(error, serverFailure))} /></>;
  const reset = (handle: string) => work(async () => {
    // Money that arrived after the transfer would be lost with the key.
    const money = await read(record.token);
    if (!money) throw new Error('BALANCE_UNKNOWN');
    if (money.units > 0n && money.latest > 0n) { await keep(undefined); say('새로 들어온 돈이 있어 초기화하지 않았습니다. 그 돈도 반납하세요.'); return; }
    if (money.units > 0n || money.latest > 0n) { say(unsettled); return; }
    const fresh = await askResetBlock(stored);
    if (resetBlocked(fresh)) throw new Error('RESET_BLOCKED');
    if (resetUnknown(fresh)) throw new Error('RESET_UNKNOWN');
    await loss.verify();
    await keep({...record, stage: 'resetting'});
    await erase(props, () => withBoard(handle, session => session.resetBoard(BigInt(record.generation), record.payer,
      () => props.wait({flow: 'reset', step: 'approve', note: pinNote})))).catch(async error => {
      if (error instanceof Error && notErased.includes(error.message)) await keep({...record, stage: 'final'});
      throw error;
    });
    await finishReset(props);
  }, error => error instanceof Error && error.message === 'BALANCE_UNKNOWN' ? '남은 돈이 없는지 확인하지 못해 초기화하지 않았습니다. 다시 시도하세요.' : resetFailure(error));
  if (record.stage === 'final') return <>
    {summary}
    <Notice title={record.amount === '0' ? '보낼 돈이 없습니다' : '반납이 확정되었습니다'} body="이제 보드를 초기화하면 끝납니다. 보드의 키, PIN, 등록이 지워집니다. 끝난 주문 기록은 이 휴대전화에 남습니다." />
    {loss.gate}
    {loss.open && <Chooser props={props} label="보드 초기화" run={reset} />}</>;
  return <>
    <Notice title="초기화 결과를 확인하고 있습니다" body="보드의 LED가 1.5초씩 두 번 켜졌다면 초기화가 끝난 것입니다. 보드는 운영자 허가를 다시 받을 때까지 연결되지 않습니다." />
    <Chooser props={props} label="보드 상태 확인" run={handle => work(async () => {
      const state = await withBoard(handle, async session => (await session.status()).state);
      if (state === 'released' || state === 'new') await finishReset(props);
      else { await keep({...record, stage: 'final'}); say('보드가 아직 초기화되지 않았습니다. 다시 시도하세요.'); }
    }, boardFailure)} />
    <KitButton label="LED로 초기화를 확인했어요" run={() => work(() => finishReset(props), () => '기록을 정리하지 못했습니다.')} disabled={props.busy} variant="secondary" /></>;
}

const buttonReset = ['보드의 SW2와 SW3을 함께 5초 동안 누릅니다.', '보드 버튼으로 PIN을 입력합니다.', 'LED가 아주 빠르게 깜박이면 10초 안에 SW1을 길게 누릅니다.', 'LED가 1.5초씩 두 번 켜지면 끝난 것입니다.'];

function ResetPage(props: BoardLifeProps) {
  const {stored, work, withBoard} = props;
  const {verified} = stored;
  const {balance} = useBalance(props);
  const {open: ordersDone, gate, verify} = useResetGate(props);
  const loss = useLossGate(props);
  const open = ordersDone && loss.open;
  const [sure, setSure] = useState(false);
  const wait = props.spending?.pinWait ?? 0;
  const risk = [
    balance && (balance.units > 0n || balance.latest > 0n) ? props.store ? `남은 돈 ${storeText(balance.units > balance.latest ? balance.units : balance.latest, props.store)}을 잃습니다.` : '보드 계정에 돈이 남아 있습니다. 금액은 확인하지 못했고, 초기화하면 모두 잃습니다.' : balance === null ? '남은 돈이 있는지 확인하지 못했습니다. 있으면 잃습니다.' : '',
    unfinished(stored) ? '결과를 모르는 다른 전송이나 서명이 있습니다.' : '',
  ].filter(Boolean);
  return <>
    <Notice tone="stop" title="공장 초기화" body="보드의 키와 PIN, 등록을 지웁니다. 되돌릴 수 없고, 남은 돈은 되찾을 수 없습니다. 돈이 남아 있으면 먼저 반납하세요.">
      {risk.map(text => <Text key={text} style={styles.risk}>{text}</Text>)}
      <KitButton label="먼저 반납하기" run={() => props.go('return')} variant="secondary" disabled={props.busy} /></Notice>
    {wait > 0 && <Notice title="PIN 입력을 기다려야 합니다" body={`약 ${Math.ceil(wait / 60)}분 뒤에 다시 입력할 수 있습니다. 전원을 껐다 켜면 처음부터 다시 기다립니다.`} />}
    {gate}
    {verified && loss.gate}
    {verified && open && (!sure ? <KitButton label="앱에서 초기화 계속" run={() => setSure(true)} disabled={props.busy || balance === undefined} variant="secondary" /> : <>
      <Text style={styles.risk}>{risk.length ? '위 내용을 모두 알고 초기화합니다.' : '보드를 초기화합니다.'}</Text>
      <Chooser props={props} label="보드 초기화" run={handle => work(async () => {
        await verify();
        await loss.verify();
        await erase(props, () => withBoard(handle, session => session.resetBoard(BigInt(verified.generation), verified.address,
          () => props.wait({flow: 'reset', step: 'approve', note: pinNote}))));
        await finishReset(props);
      }, resetFailure)} />
      <KitButton label="초기화하지 않기" run={() => setSure(false)} disabled={props.busy} variant="secondary" /></>)}
    <Card><Text style={type.section}>앱 없이 버튼으로 초기화</Text>
      {buttonReset.map((text, index) => <Text key={text} style={type.body}>{`${index + 1}. ${text}`}</Text>)}
      <Text style={type.small}>끝나지 않은 주문이나 반납이 있으면 버튼으로도 초기화하지 마세요.</Text></Card>
    <Card><Text style={type.section}>PIN을 잊었을 때</Text><Text style={type.body}>운영자가 서명한 초기화로 지웁니다. 남은 돈은 잃습니다.</Text>
      <KitButton label="운영자 초기화" run={() => props.go('operator-reset')} variant="secondary" disabled={props.busy} /></Card>
  </>;
}

/** The board keeps a request for 15 minutes and over reconnects; the phone keeps the same one, and only one. */
function OperatorResetPage(props: BoardLifeProps) {
  const {work, withBoard, fixture} = props;
  const {open: ordersDone, gate: ordersGate, verify: ordersVerify} = useResetGate(props);
  const loss = useLossGate(props);
  const known = !!props.stored.verified;
  const open = ordersDone && (!known || loss.open), gate = <>{ordersGate}{known && loss.gate}</>;
  const verify = async () => { await ordersVerify(); if (known) await loss.verify(); };
  const [request, setRequest] = useState<PendingReset | null | undefined>(fixture ? fixture.request ?? null : undefined);
  const [answer, setAnswer] = useState('');
  const [left, setLeft] = useState(0);
  useEffect(() => {
    let live = true;
    if (fixture) return;
    if (!Http) { setRequest(null); return; }
    pendingReset(Http).load().then(value => { if (live) setRequest(value ?? null); }).catch(() => { if (live) setRequest(null); });
    return () => { live = false; };
  }, [fixture]);
  useEffect(() => {
    if (!request) return;
    const tick = () => setLeft(Math.max(0, request.until - now()));
    tick();
    const timer = setInterval(tick, 1000);
    return () => clearInterval(timer);
  }, [request]);
  const keep = async (next?: PendingReset) => { await pendingReset(Http!).save(next); setRequest(next ?? null); };
  const start = (handle: string) => work(async () => {
    await verify();
    const asked = await withBoard(handle, session => session.operatorResetRequest());
    setAnswer('');
    await keep({...asked, until: now() + resetLifetime});
  }, resetFailure);
  const send = (handle: string) => work(async () => {
    const signature = readResetAnswer(answer, request!.nonce);
    await verify();
    let asked = false, waitKnown = false;
    await withBoard(handle, async session => {
      const status = await session.status();
      if (status.device !== request!.device || status.generation !== request!.generation) throw new Error('PAYMENT_BOARD_BINDING');
      // Only the enrolled phone can read whether an approved payment still holds the board.
      const board = await session.spendingStatus(status.generation).catch(error => {
        if (error instanceof Error && error.message === 'PAYMENT_BOARD_BINDING') return undefined;
        throw error;
      });
      if (board?.orderOpen) throw new Error('BOARD_ORDER_OPEN');
      waitKnown = !!board;
      await erase(props, () => session.operatorReset(request!.nonce, signature, () => {
        asked = true;
        props.wait({flow: 'reset', step: 'approve', note: '운영자 초기화를 승인하려면 SW1을 짧게 누르세요. PIN은 묻지 않습니다.'});
      }));
    }).catch(async error => {
      // The board takes one try per request. A dropped link could also be its wait for an approved payment, which takes none.
      if (asked && (waitKnown || (error instanceof Error && error.message === 'BOARD_REFUSED'))) await keep(undefined);
      throw error;
    });
    await finishReset(props);
  }, error => error instanceof Error && error.message === 'RESET_ANSWER_OTHER' ? '이전 요청의 답입니다. 지금 요청의 답을 받으세요.'
    : error instanceof Error && error.message === 'RESET_ANSWER' ? '운영자의 답 형식이 맞지 않습니다.'
    : error instanceof Error && error.message === 'OWNER_DISCONNECTED' ? '보드가 답하지 않았습니다. 답이 틀렸거나 요청이 끝났을 수 있습니다. 보드의 LED를 확인하고, 초기화되지 않았다면 새 요청을 만드세요.' : resetFailure(error));
  const scan = async () => {
    if (!ShopQr) return;
    try { const raw = await ShopQr.scan(); if (raw) setAnswer(raw); } catch { props.say('QR을 읽지 못했습니다. 답을 붙여 넣으세요.'); }
  };
  // The countdown redraws this page every second; the QR must not be drawn again with it.
  const text = request ? resetRequestText(request) : '';
  const qr = useMemo(() => text ? receiveQr(text) : '', [text]);
  if (request === undefined) return <Text style={type.body}>초기화 요청을 확인하고 있습니다.</Text>;
  const intro = <Text style={type.body}>PIN을 잊은 보드를 운영자의 서명으로 지웁니다. 남은 돈은 되찾을 수 없습니다. 요청은 15분 동안 쓸 수 있고, 보드를 껐다 켜면 새로 만들어야 합니다.</Text>;
  if (!open) return <>{intro}{gate}</>;
  if (!request) return <>{intro}<Chooser props={props} label="초기화 요청 만들기" run={start} /></>;
  if (left === 0 && request.until <= now()) return <>{intro}<Notice body="이전 요청의 시간이 끝났습니다. 새 요청을 만드세요." />
    <Chooser props={props} label="초기화 요청 만들기" run={start} /></>;
  const shown = remainingTime(left);
  return <>
    <View style={styles.left} accessible accessibilityLabel={`남은 시간 ${shown}`}><Text style={styles.leftLabel}>운영자의 답을 쓸 수 있는 시간</Text><Text style={styles.leftTime}>{shown}</Text></View>
    <Text style={type.body}>운영자에게 아래 QR이나 글을 보내세요. 이 화면을 닫아도 요청은 남습니다.</Text>
    <View style={styles.qr}><Image accessibilityLabel="운영자 초기화 요청 QR" source={{uri: qr}} style={styles.qrImage} /></View>
    <Text selectable testID="reset-request" style={styles.mono}>{text}</Text>
    <KitButton label="요청 공유" run={() => props.copy(text)} variant="secondary" />
    <Text style={type.section}>운영자의 답</Text>
    <TextInput accessibilityLabel="운영자의 답" placeholder="NU54-RESET1-OK:…" placeholderTextColor={inputPlaceholder} value={answer} onChangeText={setAnswer}
      style={[styles.input, styles.multi]} multiline maxLength={260} autoCapitalize="none" autoCorrect={false} />
    {ShopQr && <KitButton label="카메라로 답 QR 읽기" run={() => { scan().catch(() => {}); }} variant="secondary" disabled={props.busy} />}
    <Chooser props={props} label="이 답으로 보드 초기화" run={send} disabled={!answer} />
    <KitButton label="새 요청 만들기" run={() => work(() => keep(undefined), () => '요청을 지우지 못했습니다.')} variant="secondary" disabled={props.busy} />
  </>;
}
const remainingTime = (seconds: number) => seconds >= 60 ? `${Math.floor(seconds / 60)}분 ${seconds % 60}초` : `${seconds}초`;

export const boardLifeTitles: Record<BoardLifeScreen, string> = {
  setup: '보드 설정', 'return-address': '반납받을 주소', deposit: '받기', send: '보내기', return: '반납', reset: '공장 초기화', 'operator-reset': '운영자 초기화',
};

/** Read when these pages open, and again on request. `undefined` while it is read, without one, or when it cannot be read. */
function useStore(fixture?: Store) {
  const [store, setStore] = useState(fixture), [storeRead, setRead] = useState<'loading' | 'done'>(fixture ? 'done' : 'loading');
  const live = useRef(true);
  const readStore = useCallback(() => {
    registeredStore().catch(() => undefined).then(value => { if (live.current) { setStore(value); setRead('done'); } });
  }, []);
  useEffect(() => {
    live.current = true;
    if (!fixture) readStore();
    return () => { live.current = false; };
  }, [fixture, readStore]);
  return {store, storeRead, readStore};
}

export function BoardLife(given: BoardLifeProps) {
  const props = {...given, ...useStore(given.store)};
  const Page = {setup: SetupPage, 'return-address': ReturnAddressPage, deposit: DepositPage, send: SendPage, return: ReturnPage, reset: ResetPage, 'operator-reset': OperatorResetPage}[props.screen];
  return <View style={styles.page} testID={'board-life-' + props.screen}>
    <Page {...props} />
    <KitButton label={props.screen === 'operator-reset' ? '초기화 안내로' : '돌아가기'} run={() => props.go(props.screen === 'operator-reset' ? 'reset' : props.screen === 'return-address' ? 'setup' : 'board')} variant="secondary" />
  </View>;
}

const styles = StyleSheet.create({
  page: {gap: 14}, group: {gap: 10},
  steps: {gap: 0, paddingVertical: 4}, step: {flexDirection: 'row', alignItems: 'center', gap: 14, paddingVertical: 12},
  number: {minWidth: 30, height: 30, borderRadius: 15, backgroundColor: color.line, alignItems: 'center', justifyContent: 'center'},
  numberDone: {backgroundColor: '#2f7a55'}, numberNow: {backgroundColor: color.green},
  numberText: {fontFamily: font, fontSize: 14, fontWeight: '800', color: color.ink}, numberTextOn: {color: color.onGreen},
  words: {flex: 1, gap: 2},
  notice: {gap: 8, borderRadius: radius.m, padding: 14, backgroundColor: color.amberBg}, noticeStop: {backgroundColor: color.dangerBg}, noticeDone: {backgroundColor: color.mintSoft},
  noticeTitle: {fontFamily: font, fontSize: 17, fontWeight: '800', color: '#5a3b05'}, stop: {color: color.danger},
  noticeBody: {fontFamily: font, fontSize: 15, lineHeight: 22, color: color.ink},
  risk: {fontFamily: font, fontSize: 15, lineHeight: 22, fontWeight: '700', color: color.danger},
  address: {fontFamily: 'monospace', fontSize: 14, lineHeight: 20, color: color.ink, backgroundColor: color.sheet, borderRadius: 8, padding: 10},
  mono: {fontFamily: 'monospace', fontSize: 12, lineHeight: 18, color: color.ink, backgroundColor: color.sheet, borderRadius: 8, padding: 10},
  qr: {alignItems: 'center'}, qrImage: {width: 240, height: 240},
  input: {fontFamily: font, fontSize: 15, borderWidth: 1, borderColor: '#8fa497', borderRadius: radius.m, padding: 12, minHeight: 50, color: color.ink, backgroundColor: color.sheet},
  multi: {minHeight: 90, textAlignVertical: 'top'},
  left: {flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 12, backgroundColor: color.amberBg, borderRadius: radius.m, paddingHorizontal: 14, paddingVertical: 10},
  leftLabel: {fontFamily: font, fontSize: 14, fontWeight: '600', color: '#5a3b05', flexShrink: 1},
  leftTime: {fontFamily: font, fontSize: 18, fontWeight: '800', color: '#5a3b05', fontVariant: ['tabular-nums']},
});
