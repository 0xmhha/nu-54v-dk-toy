/**
 * Kiosk payment screen (P04 design 3): amount -> the renter approves on the device -> the
 * kiosk submits settle and shows one of approved, refused, failed or Checking.
 *
 * Development builds take settings and keys from scripts/provision-dev.ts (N32).
 */

import { useCallback, useEffect, useState } from 'react';
import {
  PermissionsAndroid,
  Platform,
  Pressable,
  StatusBar,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import { SafeAreaProvider, SafeAreaView } from 'react-native-safe-area-context';
import { fromBase64 } from '@nu54/protocol';
import { findDevice, openTransport } from './ble/central.ts';
import { FramedLink } from './ble/framing.ts';
import { JsonRpcChain } from './chain/rpc.ts';
import { anchorUsed, loadKiosk, pendingAnchor, type Loaded } from './kiosk/config.ts';
import { ANCHOR_MAX_AGE_S, WAIT_MS, type TimeAnchor } from './payment/session.ts';
import { changeLimits, pay, resumeOrders, submitContext, type LimitResult, type PayDeps, type PayResult, type Phase } from './kiosk/pay.ts';
import { resume } from './payment/submit.ts';
import { OrderStore } from './kiosk/orders.ts';
import { blockTimeText, fetchReceipt, type ReceiptResult } from './kiosk/receipt.ts';
import Vault from './specs/NativeKioskVault.ts';

const PHASE_TEXT: Record<Phase, string> = {
  checkingGas: '가스 잔액 확인 중',
  connecting: '결제 기기를 찾는 중',
  anchor: '기기 시각 설정 중',
  opening: '결제 세션 여는 중',
  identifying: '가맹점 확인 중',
  waitingDevice: '기기에서 결제를 승인해 주세요',
  submitting: '결제 처리 중',
};

/** Whole seconds left until `until` (ms), refreshed while shown; never below 0. */
function useSecondsLeft(until: number | undefined): number | null {
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    if (until === undefined) return;
    const t = setInterval(() => setNow(Date.now()), 250);
    return () => clearInterval(t);
  }, [until]);
  return until === undefined ? null : Math.max(0, Math.ceil((until - now) / 1000));
}

/** Token amount as the screens show it: truncated to two decimals (N31). */
function shown(amount: bigint, decimals: number): string {
  const unit = 10n ** BigInt(decimals);
  const cents = ((amount % unit) * 100n) / unit;
  return `${amount / unit}.${cents.toString().padStart(2, '0')}`;
}

/** "4.5" -> 4500000 for 6 decimals; null when not a positive amount with at most `decimals` places. */
function parseAmount(text: string, decimals: number): bigint | null {
  const m = /^(\d+)(?:\.(\d+))?$/.exec(text.trim());
  if (!m || (m[2]?.length ?? 0) > decimals) return null;
  const v = BigInt(m[1]) * 10n ** BigInt(decimals) + BigInt((m[2] ?? '').padEnd(decimals, '0') || '0');
  return v > 0n ? v : null;
}

async function blePermissions(): Promise<boolean> {
  if (Platform.OS !== 'android') return true;
  const wanted =
    Number(Platform.Version) >= 31
      ? [PermissionsAndroid.PERMISSIONS.BLUETOOTH_SCAN, PermissionsAndroid.PERMISSIONS.BLUETOOTH_CONNECT]
      : [PermissionsAndroid.PERMISSIONS.ACCESS_FINE_LOCATION];
  const got = await PermissionsAndroid.requestMultiple(wanted);
  return wanted.every(p => got[p] === PermissionsAndroid.RESULTS.GRANTED);
}

function deps(kiosk: Loaded, orders?: OrderStore): Omit<PayDeps, 'random'> {
  return {
    kiosk,
    orders,
    chain: new JsonRpcChain(kiosk.config.rpc),
    connect: async () => {
      const found = await findDevice();
      return new FramedLink(await openTransport(found.address, () => {}));
    },
    anchor: () => pendingAnchor(Vault),
    anchorUsed: () => anchorUsed(Vault),
    // With the development anchor server, every payment asks for a fresh anchor instead.
    ...(kiosk.config.anchorUrl
      ? {
          anchor: async () => undefined,
          anchorFor: async (device: string) => {
            const r = await fetch(`${kiosk.config.anchorUrl}?device=${device}`);
            return r.ok ? ((await r.json()) as TimeAnchor) : undefined;
          },
        }
      : {}),
  };
}

type Screen =
  | { kind: 'loading' }
  | { kind: 'unprovisioned'; error?: string }
  | { kind: 'idle' }
  | { kind: 'limits' }
  | { kind: 'limitBusy'; text: string }
  | { kind: 'limitResult'; result: LimitResult }
  | { kind: 'paying'; phase: Phase; amount: bigint; deadline?: number }
  | { kind: 'result'; result: PayResult; amount: bigint };

/** What to do about a refusal the person at the kiosk can fix. */
const REFUSAL_HINT: Record<string, string> = {
  TIME_ANCHOR_MISSING: '기기에 시각 기준이 없습니다. Mac에서 TimeAnchor를 새로 발급해 넣고 바로 다시 요청하세요.',
  TIME_ANCHOR_STALE: 'TimeAnchor가 90초 넘게 지났습니다. 새로 발급해 넣고 바로 다시 요청하세요.',
  ATTESTATION_EXPIRED: '기기 시각이나 가맹점 인증 기간이 맞지 않습니다. TimeAnchor를 새로 발급해 넣으세요.',
  USER_REJECTED: '기기에서 거절했습니다.',
};

function resultText(r: PayResult): { title: string; detail?: string; tone: 'ok' | 'bad' | 'wait' } {
  switch (r.status) {
    case 'approved':
      return { title: '결제 완료', detail: r.txHash ? `tx ${r.txHash.slice(0, 10)}…${r.txHash.slice(-4)}` : '이미 정산된 주문', tone: 'ok' };
    case 'refused':
      return { title: '결제 거절', detail: REFUSAL_HINT[r.reason] ? `${r.reason}: ${REFUSAL_HINT[r.reason]}` : r.reason, tone: 'bad' };
    case 'failed':
      return { title: '결제 실패', detail: r.reason, tone: 'bad' };
    case 'Checking':
      return { title: '확인 중', detail: '체인에서 결과를 확인하고 있습니다. 이 주문은 다시 결제하지 마세요.', tone: 'wait' };
    case 'cancelled':
      return { title: '시간 초과로 취소', detail: '기기 승인이 없어 주문을 취소했습니다. 다시 결제할 수 있습니다.', tone: 'bad' };
    case 'busy':
      return { title: '주문을 받을 수 없음', detail: '키오스크 가스 잔액이 부족합니다.', tone: 'bad' };
    case 'noDevice':
      return { title: '결제 기기를 찾지 못함', detail: `${r.reason}. 기기 SW4를 길게 눌러 결제 모드를 켠 뒤(2분 동안 유지) 다시 요청하세요.`, tone: 'bad' };
  }
}

function Kiosk() {
  const [kiosk, setKiosk] = useState<Loaded | null>(null);
  const [screen, setScreen] = useState<Screen>({ kind: 'loading' });
  const [amountText, setAmountText] = useState('1');
  const [receipt, setReceipt] = useState<ReceiptResult | 'loading' | null>(null);
  const [orders, setOrders] = useState<OrderStore | undefined>();
  const [openCount, setOpenCount] = useState(0);

  /** Picks up orders left open by a restart (P04-NFR-05): same signature only, never a new one. */
  const resumeOpen = useCallback(async (k: Loaded, store: OrderStore) => {
    if (store.open().length === 0) return;
    await resumeOrders({ kiosk: k, chain: new JsonRpcChain(k.config.rpc), orders: store }).catch(() => undefined);
    setOpenCount(store.open().length);
  }, []);

  const load = useCallback(async () => {
    try {
      const k = await loadKiosk(Vault);
      setKiosk(k);
      setScreen(k ? { kind: 'idle' } : { kind: 'unprovisioned' });
      if (k) {
        const store = await OrderStore.open(Vault);
        setOrders(store);
        setOpenCount(store.open().length);
        resumeOpen(k, store);
      }
    } catch (e) {
      setScreen({ kind: 'unprovisioned', error: String(e) });
    }
  }, [resumeOpen]);
  useEffect(() => {
    load();
  }, [load]);

  const decimals = kiosk?.config.tokenDecimals ?? 6;
  const amount = parseAmount(amountText, decimals);

  /** SecureRandom bytes fetched once per session; a session draws well under 4 KiB. */
  const randomPool = async () => {
    const pool = await Vault.randomBytes(4096).then(fromBase64);
    let used = 0;
    return (n: number) => {
      if (used + n > pool.length) throw new Error('random pool exhausted');
      used += n;
      return pool.slice(used - n, used);
    };
  };

  const [perText, setPerText] = useState('');
  const [dailyText, setDailyText] = useState('');
  const startLimits = async () => {
    if (!kiosk) return;
    // An empty field is 0, which keeps the register cap (payment-protocol.md 2).
    const per = perText.trim() ? parseAmount(perText, decimals) : 0n;
    const daily = dailyText.trim() ? parseAmount(dailyText, decimals) : 0n;
    if (per === null || daily === null || !(await blePermissions())) return;
    const result = await changeLimits({ ...deps(kiosk), random: await randomPool() }, per, daily, phase =>
      setScreen({ kind: 'limitBusy', text: phase === 'waitingDevice' ? '기기 버튼으로 PIN을 입력하고 승인하세요' : phase === 'submitting' ? '한도 변경 제출 중' : '결제 기기를 찾는 중' }),
    ).catch((e): LimitResult => ({ status: 'failed', reason: e instanceof Error ? e.message : String(e) }));
    setScreen({ kind: 'limitResult', result });
  };

  const start = async () => {
    if (!kiosk || amount === null) return;
    if (!(await blePermissions())) {
      setScreen({ kind: 'result', amount, result: { status: 'noDevice', reason: '블루투스 권한이 필요합니다' } });
      return;
    }
    setScreen({ kind: 'paying', phase: 'checkingGas', amount });
    const result = await pay(
      { ...deps(kiosk, orders), random: await randomPool() },
      amount,
      // The press must come within WAIT_MS of the request: count it down on the screen.
      phase => setScreen({ kind: 'paying', phase, amount, deadline: phase === 'waitingDevice' ? Date.now() + WAIT_MS : undefined }),
    ).catch((e): PayResult => ({ status: 'failed', reason: e instanceof Error ? e.message : String(e) }));
    setOpenCount(orders?.open().length ?? 0);
    setScreen({ kind: 'result', result, amount });
  };

  const check = async (r: Extract<PayResult, { status: 'Checking' }>, amt: bigint) => {
    if (!kiosk) return;
    // The same signature only: re-simulated and resent when it would still settle (P04-FR-14).
    const out = await resume(submitContext({ ...deps(kiosk, orders), random: () => new Uint8Array() }), r.signed);
    if (orders?.get(r.signed.auth.orderId)) await orders.apply(r.signed.auth.orderId, { type: 'outcome', outcome: out });
    setOpenCount(orders?.open().length ?? 0);
    setScreen({ kind: 'result', amount: amt, result: out.status === 'Checking' ? { ...out, signed: r.signed } : out });
  };

  // The pushed TimeAnchor can be used for ANCHOR_MAX_AGE_S after it was signed (the phone's
  // clock stands in for the chain's; they agree within seconds on a synced phone).
  const [anchorUntil, setAnchorUntil] = useState<number | undefined>();
  useEffect(() => {
    if (screen.kind !== 'idle') return;
    let live = true;
    const read = () =>
      pendingAnchor(Vault)
        .then(a => live && setAnchorUntil(a ? (Number(a.timestamp) + ANCHOR_MAX_AGE_S) * 1000 : undefined))
        .catch(() => undefined);
    read();
    const t = setInterval(read, 5000);
    return () => {
      live = false;
      clearInterval(t);
    };
  }, [screen.kind]);
  const anchorLeft = useSecondsLeft(anchorUntil);
  const waitLeft = useSecondsLeft(screen.kind === 'paying' ? screen.deadline : undefined);

  const symbol = kiosk?.config.tokenSymbol ?? '';
  return (
    <SafeAreaView style={styles.root}>
      <Text style={styles.merchant}>{kiosk?.config.attestation.name ?? 'NU54 키오스크'}</Text>
      {screen.kind === 'loading' && <Text style={styles.body}>불러오는 중</Text>}
      {screen.kind === 'unprovisioned' && (
        <View>
          <Text style={styles.title}>설정 필요</Text>
          <Text style={styles.body}>
            Mac에서 scripts/provision-dev.ts --attestation att.json 을 실행한 뒤 다시 불러오세요.
          </Text>
          {screen.error && <Text style={styles.error}>{screen.error}</Text>}
          <Button label="다시 불러오기" onPress={load} />
        </View>
      )}
      {screen.kind === 'idle' && (
        <View>
          <Text style={styles.label}>결제 금액 ({symbol})</Text>
          <TextInput style={styles.input} value={amountText} onChangeText={setAmountText} keyboardType="decimal-pad" />
          <Button label={amount ? `${shown(amount, decimals)} ${symbol} 결제 요청` : '금액을 입력하세요'} onPress={start} disabled={!amount} />
          <Button label="한도 변경" onPress={() => setScreen({ kind: 'limits' })} />
          {anchorLeft !== null && (
            <Text style={anchorLeft > 0 ? styles.body : styles.error}>
              {anchorLeft > 0
                ? `기기 시각 기준(TimeAnchor) 사용 가능: ${anchorLeft}초 남음`
                : 'TimeAnchor가 오래됐습니다. Mac에서 새로 발급해 넣으세요.'}
            </Text>
          )}
          {openCount > 0 && kiosk && orders && (
            <View style={styles.receipt}>
              <Text style={styles.body}>확인 중인 주문 {openCount}건. 같은 주문은 다시 결제하지 마세요.</Text>
              <Button label="다시 확인" onPress={() => resumeOpen(kiosk, orders)} />
            </View>
          )}
        </View>
      )}
      {screen.kind === 'paying' && (
        <View>
          <Text style={styles.amount}>{shown(screen.amount, decimals)} {symbol}</Text>
          <Text style={screen.phase === 'waitingDevice' ? styles.title : styles.body}>{PHASE_TEXT[screen.phase]}</Text>
          {waitLeft !== null && <Text style={styles.countdown}>{waitLeft}초</Text>}
        </View>
      )}
      {screen.kind === 'limits' && (
        <View>
          <Text style={styles.label}>1회 한도 ({symbol}, 비우면 상한)</Text>
          <TextInput style={styles.input} value={perText} onChangeText={setPerText} keyboardType="decimal-pad" />
          <Text style={styles.label}>하루 한도 ({symbol}, 비우면 상한)</Text>
          <TextInput style={styles.input} value={dailyText} onChangeText={setDailyText} keyboardType="decimal-pad" />
          <Button label="기기에 요청" onPress={startLimits} />
          <Button label="취소" onPress={() => setScreen({ kind: 'idle' })} />
        </View>
      )}
      {screen.kind === 'limitBusy' && <Text style={styles.title}>{screen.text}</Text>}
      {screen.kind === 'limitResult' && (
        <View>
          <Text style={[styles.title, screen.result.status === 'approved' ? styles.ok : styles.bad]}>
            {screen.result.status === 'approved' ? '한도 변경 완료' : '한도 변경 안 됨'}
          </Text>
          {'reason' in screen.result && <Text style={styles.body}>{screen.result.reason}</Text>}
          <Button label="처음으로" onPress={() => setScreen({ kind: 'idle' })} />
        </View>
      )}
      {screen.kind === 'result' && (() => {
        const t = resultText(screen.result);
        const r = screen.result;
        return (
          <View>
            <Text style={styles.amount}>{shown(screen.amount, decimals)} {symbol}</Text>
            <Text style={[styles.title, styles[t.tone]]}>{t.title}</Text>
            {t.detail && <Text style={styles.body}>{t.detail}</Text>}
            {r.status === 'approved' && kiosk?.config.indexerUrl && (
              <ReceiptView
                state={receipt}
                onLoad={async () => {
                  setReceipt('loading');
                  setReceipt(await fetchReceipt(kiosk.config.indexerUrl!, r.event.merchant, r.event.orderId));
                }}
                format={a => `${shown(BigInt(a), decimals)} ${symbol}`}
              />
            )}
            {r.status === 'Checking' ? (
              <Button label="다시 확인" onPress={() => check(r, screen.amount)} />
            ) : (
              <Button label="새 주문" onPress={() => { setReceipt(null); setScreen({ kind: 'idle' }); }} />
            )}
          </View>
        );
      })()}
    </SafeAreaView>
  );
}

/** The P07 receipt for an approved payment; its absence never changes the result above. */
function ReceiptView({ state, onLoad, format }: { state: ReceiptResult | 'loading' | null; onLoad: () => void; format: (amount: string) => string }) {
  if (state === null) return <Button label="영수증 보기" onPress={onLoad} />;
  if (state === 'loading') return <Text style={styles.body}>영수증 조회 중</Text>;
  if (state.status !== 'found') {
    const why = state.status === 'notIndexed' ? '아직 indexer에 없습니다' : state.status === 'stale' ? 'indexer가 뒤처져 있습니다' : state.reason;
    return <Text style={styles.body}>영수증을 가져오지 못했습니다({why}). 결제 결과는 그대로입니다.</Text>;
  }
  const v = state.receipt;
  return (
    <View style={styles.receipt}>
      <Text style={styles.body}>영수증</Text>
      <Text style={styles.small}>금액 {format(v.amount)}</Text>
      <Text style={styles.small}>블록 {v.blockNumber} · {blockTimeText(v.blockTime)}</Text>
      <Text style={styles.small}>tx {v.txHashShort} · 기기 {v.device.slice(0, 8)}…{v.device.slice(-4)}</Text>
      {v.duplicate && <Text style={styles.error}>같은 주문의 로그가 둘 이상입니다(가장 이른 것을 표시)</Text>}
    </View>
  );
}

function Button({ label, onPress, disabled }: { label: string; onPress: () => void; disabled?: boolean }) {
  return (
    <Pressable style={[styles.button, disabled && styles.disabled]} onPress={onPress} disabled={disabled}>
      <Text style={styles.buttonText}>{label}</Text>
    </Pressable>
  );
}

export default function App() {
  return (
    <SafeAreaProvider>
      <StatusBar barStyle="dark-content" />
      <Kiosk />
    </SafeAreaProvider>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, padding: 24, backgroundColor: '#fafafa' },
  merchant: { fontSize: 18, color: '#555', marginBottom: 24 },
  label: { fontSize: 16, color: '#333', marginBottom: 8 },
  input: { fontSize: 32, borderBottomWidth: 2, borderColor: '#333', paddingVertical: 8, marginBottom: 24, color: '#111' },
  amount: { fontSize: 40, fontWeight: '600', color: '#111', marginBottom: 16 },
  title: { fontSize: 26, fontWeight: '600', color: '#111', marginBottom: 12 },
  countdown: { fontSize: 56, fontWeight: '700', color: '#1b4fa0', marginTop: 8 },
  body: { fontSize: 18, color: '#333', marginBottom: 16 },
  error: { fontSize: 14, color: '#b00020', marginBottom: 16 },
  small: { fontSize: 14, color: '#333', marginBottom: 4 },
  receipt: { borderTopWidth: 1, borderColor: '#ddd', paddingTop: 12, marginBottom: 12 },
  ok: { color: '#1b7f3a' },
  bad: { color: '#b00020' },
  wait: { color: '#a15c00' },
  button: { backgroundColor: '#111', paddingVertical: 16, borderRadius: 8, alignItems: 'center', marginTop: 8 },
  disabled: { backgroundColor: '#999' },
  buttonText: { color: '#fff', fontSize: 18, fontWeight: '600' },
});
