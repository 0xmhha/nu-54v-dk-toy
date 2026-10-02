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
import { fromBase64 } from './ble/base64.ts';
import { findDevice, openTransport } from './ble/central.ts';
import { FramedLink } from './ble/framing.ts';
import { JsonRpcChain } from './chain/rpc.ts';
import { loadKiosk, takeAnchor, type Loaded } from './kiosk/config.ts';
import { pay, submitContext, type PayDeps, type PayResult, type Phase } from './kiosk/pay.ts';
import { recheck } from './payment/submit.ts';
import Vault from './specs/NativeKioskVault.ts';

const PHASE_TEXT: Record<Phase, string> = {
  checkingGas: '가스 잔액 확인 중',
  connecting: '결제 기기를 찾는 중',
  anchor: '기기 시각 설정 중',
  opening: '결제 세션 여는 중',
  identifying: '가맹점 확인 중',
  waitingDevice: '기기에서 결제를 승인해 주세요 (10초)',
  submitting: '결제 처리 중',
};

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

function deps(kiosk: Loaded): Omit<PayDeps, 'random'> {
  return {
    kiosk,
    chain: new JsonRpcChain(kiosk.config.rpc),
    connect: async () => {
      const found = await findDevice();
      return new FramedLink(await openTransport(found.address, () => {}));
    },
    anchor: () => takeAnchor(Vault),
  };
}

type Screen =
  | { kind: 'loading' }
  | { kind: 'unprovisioned'; error?: string }
  | { kind: 'idle' }
  | { kind: 'paying'; phase: Phase; amount: bigint }
  | { kind: 'result'; result: PayResult; amount: bigint };

function resultText(r: PayResult): { title: string; detail?: string; tone: 'ok' | 'bad' | 'wait' } {
  switch (r.status) {
    case 'approved':
      return { title: '결제 완료', detail: r.txHash ? `tx ${r.txHash.slice(0, 10)}…${r.txHash.slice(-4)}` : '이미 정산된 주문', tone: 'ok' };
    case 'refused':
      return { title: '결제 거절', detail: r.reason, tone: 'bad' };
    case 'failed':
      return { title: '결제 실패', detail: r.reason, tone: 'bad' };
    case 'Checking':
      return { title: '확인 중', detail: '체인에서 결과를 확인하고 있습니다. 이 주문은 다시 결제하지 마세요.', tone: 'wait' };
    case 'cancelled':
      return { title: '시간 초과로 취소', detail: '기기 승인이 없어 주문을 취소했습니다. 다시 결제할 수 있습니다.', tone: 'bad' };
    case 'busy':
      return { title: '주문을 받을 수 없음', detail: '키오스크 가스 잔액이 부족합니다.', tone: 'bad' };
    case 'noDevice':
      return { title: '결제 기기를 찾지 못함', detail: r.reason, tone: 'bad' };
  }
}

function Kiosk() {
  const [kiosk, setKiosk] = useState<Loaded | null>(null);
  const [screen, setScreen] = useState<Screen>({ kind: 'loading' });
  const [amountText, setAmountText] = useState('1');

  const load = useCallback(async () => {
    try {
      const k = await loadKiosk(Vault);
      setKiosk(k);
      setScreen(k ? { kind: 'idle' } : { kind: 'unprovisioned' });
    } catch (e) {
      setScreen({ kind: 'unprovisioned', error: String(e) });
    }
  }, []);
  useEffect(() => {
    load();
  }, [load]);

  const decimals = kiosk?.config.tokenDecimals ?? 6;
  const amount = parseAmount(amountText, decimals);

  const start = async () => {
    if (!kiosk || amount === null) return;
    if (!(await blePermissions())) {
      setScreen({ kind: 'result', amount, result: { status: 'noDevice', reason: '블루투스 권한이 필요합니다' } });
      return;
    }
    setScreen({ kind: 'paying', phase: 'checkingGas', amount });
    const random = await Vault.randomBytes(4096).then(fromBase64);
    let used = 0;
    const result = await pay(
      {
        ...deps(kiosk),
        // SecureRandom bytes fetched once per payment; the session draws well under 4 KiB.
        random: n => {
          if (used + n > random.length) throw new Error('random pool exhausted');
          used += n;
          return random.slice(used - n, used);
        },
      },
      amount,
      phase => setScreen({ kind: 'paying', phase, amount }),
    ).catch((e): PayResult => ({ status: 'failed', reason: e instanceof Error ? e.message : String(e) }));
    setScreen({ kind: 'result', result, amount });
  };

  const check = async (r: Extract<PayResult, { status: 'Checking' }>, amt: bigint) => {
    if (!kiosk) return;
    const out = await recheck(submitContext({ ...deps(kiosk), random: () => new Uint8Array() }), r.signed);
    setScreen({ kind: 'result', amount: amt, result: out.status === 'Checking' ? { ...out, signed: r.signed } : out });
  };

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
        </View>
      )}
      {screen.kind === 'paying' && (
        <View>
          <Text style={styles.amount}>{shown(screen.amount, decimals)} {symbol}</Text>
          <Text style={screen.phase === 'waitingDevice' ? styles.title : styles.body}>{PHASE_TEXT[screen.phase]}</Text>
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
            {r.status === 'Checking' ? (
              <Button label="다시 확인" onPress={() => check(r, screen.amount)} />
            ) : (
              <Button label="새 주문" onPress={() => setScreen({ kind: 'idle' })} />
            )}
          </View>
        );
      })()}
    </SafeAreaView>
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
  body: { fontSize: 18, color: '#333', marginBottom: 16 },
  error: { fontSize: 14, color: '#b00020', marginBottom: 16 },
  ok: { color: '#1b7f3a' },
  bad: { color: '#b00020' },
  wait: { color: '#a15c00' },
  button: { backgroundColor: '#111', paddingVertical: 16, borderRadius: 8, alignItems: 'center', marginTop: 8 },
  disabled: { backgroundColor: '#999' },
  buttonText: { color: '#fff', fontSize: 18, fontWeight: '600' },
});
