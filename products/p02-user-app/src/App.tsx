/**
 * Renter phone app (P02 design 3). A state machine (src/setup/flow.ts) takes the renter from a
 * device in pairing mode to a checked wallet: scan, bond, setup on the device buttons, the
 * reconnect code, the passkey bond, the wallet check. A registered device reconnects on start.
 * The home screen turns payment mode on (P02-FR-08) and shows what the device is about to sign.
 * There is no approve button: the renter approves on the device (P02-FR-05) and enters the PIN on
 * the device buttons (P02-FR-06).
 */

import { useEffect, useRef, useState } from 'react';
import { Linking, PermissionsAndroid, Platform, Pressable, ScrollView, StatusBar, StyleSheet, Text, View } from 'react-native';
import { SafeAreaProvider, SafeAreaView } from 'react-native-safe-area-context';
import { explorerTxUrl, fromBase64, GATT, receiptAmount, toBase64, type DigitalReceipt } from '@nu54/protocol';
import { ConfirmLink, PAYMENT_MODE_SECONDS, type PaymentMode, type Screen } from './link/confirmLink.ts';
import { bodyLink } from './setup/channel.ts';
import { SetupFlow, type DeviceRecord, type FoundDevice, type PinEntry, type SetupPlatform, type SetupState } from './setup/flow.ts';
import RenterBle from './specs/NativeRenterBle.ts';

const OUTCOME_TEXT: Record<string, string> = {
  approved: '결제 완료',
  refused: '결제 거절',
  failed: '결제 실패',
  Checking: '확인 중',
};

async function bluetoothPermission(): Promise<boolean> {
  if (Platform.OS !== 'android' || Number(Platform.Version) < 31) return true;
  const wanted = [PermissionsAndroid.PERMISSIONS.BLUETOOTH_CONNECT, PermissionsAndroid.PERMISSIONS.BLUETOOTH_SCAN];
  const got = await PermissionsAndroid.requestMultiple(wanted);
  return wanted.every(p => got[p] === PermissionsAndroid.RESULTS.GRANTED);
}

/** The Turbo Module as the setup flow's platform. */
const nativePlatform: SetupPlatform = {
  async scan(onFound, seconds) {
    const sub = RenterBle.onScan(json => {
      try {
        onFound(JSON.parse(json) as FoundDevice);
      } catch {
        // not a scan result
      }
    });
    try {
      await RenterBle.scan(GATT.service, seconds);
    } finally {
      sub.remove();
    }
  },
  stopScan: () => RenterBle.stopScan(),
  bond: (address, passkey) => RenterBle.bond(address, passkey),
  isBonded: address => RenterBle.isBonded(address),
  removeBond: address => RenterBle.removeBond(address),
  async connect(address) {
    const mtu = await RenterBle.connect(address, GATT.service, GATT.rx, GATT.tx);
    return bodyLink({
      mtu,
      write: f => RenterBle.writeFragment(toBase64(f)),
      onFragment: h => {
        const sub = RenterBle.onFragment(b64 => h(fromBase64(b64)));
        return () => sub.remove();
      },
    });
  },
  disconnect: () => RenterBle.disconnect(),
  random: async n => fromBase64(await RenterBle.randomBytes(n)),
  async loadDevices() {
    const text = await RenterBle.loadDevices();
    try {
      return text ? (JSON.parse(text) as DeviceRecord[]) : [];
    } catch {
      return [];
    }
  },
  saveDevices: list => RenterBle.saveDevices(JSON.stringify(list)),
  now: () => Date.now(),
  sleep: ms => new Promise(r => setTimeout(r, ms)),
};

/** Seconds left until `until` (ms), ticking once a second. */
function useSecondsLeft(until: number | null): number {
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    if (until === null) return;
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [until]);
  return until === null ? 0 : Math.max(0, Math.ceil((until - now) / 1000));
}

const short = (a: string) => `${a.slice(0, 8)}…${a.slice(-6)}`;

/** The PIN as the device reports it: kept digits, the digit being entered (framed), zeros after. */
function PinBoxes({ entry }: { entry: PinEntry }) {
  return (
    <View style={styles.pinRow}>
      {entry.digits.split('').map((d, i) => (
        <View key={i} style={[styles.pinBox, i === entry.position && styles.pinCurrent, i < entry.position && styles.pinKept]}>
          <Text style={[styles.pinDigit, i > entry.position && styles.pinLater]}>{d}</Text>
        </View>
      ))}
    </View>
  );
}

/** The 6-digit pairing code (the BLE passkey), not the 4-digit device PIN. */
const PAIRING_CODE_NOTE = '폰의 블루투스 창에 "PIN"이라고 나와도, 기기 PIN 4자리가 아니라 이 6자리 페어링 코드를 입력하세요.';

/** The pairing code, shown once; the renter types it into the system pairing dialog next. */
function CodeShow({ passkey, wallet, retry, onDone }: { passkey: string; wallet: string; retry?: boolean; onDone: () => void }) {
  return (
    <View>
      <Text style={styles.title}>지갑 설정 3/3: 페어링 코드</Text>
      <Text style={styles.label}>지갑 주소</Text>
      <Text style={styles.mono}>{wallet}</Text>
      <Text style={styles.label}>페어링 코드 (6자리)</Text>
      <Text style={styles.code}>{passkey}</Text>
      <Text style={styles.body}>
        이 폰과 기기를 다시 연결하거나 다른 폰에 연결할 때 쓰는 코드입니다. 방금 정한 기기 PIN(4자리)과는 다릅니다. 앱은 이 코드를 저장하지 않으니 지금 적어 두세요.
      </Text>
      <Text style={styles.body}>다음을 누르면 폰의 블루투스 페어링 창이 뜹니다. 그 창에 이 6자리 코드를 입력하세요.</Text>
      <Text style={styles.hint}>{PAIRING_CODE_NOTE}</Text>
      {retry && <Text style={styles.error}>본딩하지 못했습니다. 6자리 페어링 코드를 확인하고 다시 입력하세요. 기기 페어링 모드가 끝났으면 SW3을 길게 누르세요.</Text>}
      <Button label="코드를 적어 두었습니다. 다음" onPress={onDone} />
    </View>
  );
}

function Renter() {
  const [state, setState] = useState<SetupState>({ kind: 'loading' });
  const flow = useRef<SetupFlow | null>(null);

  useEffect(() => {
    const f = new SetupFlow(nativePlatform, setState);
    flow.current = f;
    bluetoothPermission().then(ok => {
      if (ok) f.start();
      else setState({ kind: 'failed', message: '블루투스 권한이 필요합니다' });
    });
    return () => {
      f.closeLink();
    };
  }, []);

  const f = flow.current;
  const until = state.kind === 'verify' || state.kind === 'setupPin' ? state.until : null;
  const left = useSecondsLeft(until);

  switch (state.kind) {
    case 'loading':
      return <Text style={styles.body}>불러오는 중</Text>;
    case 'scan':
      return (
        <View>
          <Text style={styles.title}>기기 찾기</Text>
          <Text style={styles.body}>기기를 켜고 SW3을 길게 눌러 페어링 모드로 두세요. LED가 2초마다 두 번 깜빡이면 페어링 모드입니다.</Text>
          {state.found.map(d => (
            <Pressable key={d.address} style={styles.device} onPress={() => f?.choose(d)}>
              <Text style={styles.value}>{d.name || '이름 없는 기기'}</Text>
              <Text style={styles.small}>{d.address}  신호 {d.rssi} dBm</Text>
              <Text style={styles.connect}>연결</Text>
            </Pressable>
          ))}
          {state.scanning && <Text style={styles.hint}>찾는 중…</Text>}
          {!state.scanning && state.found.length === 0 && <Text style={styles.hint}>찾은 기기가 없습니다.</Text>}
          {!state.scanning && <Button label="다시 찾기" onPress={() => f?.scan()} />}
        </View>
      );
    case 'bonding':
      return (
        <Step title="본딩 중" device={state.device.name}>
          시스템 창이 뜨면 페어링을 허용하세요. 이미 설정한 기기라면 설정 때 받은 6자리 페어링 코드를 입력하세요(기기 PIN 4자리가 아닙니다).
        </Step>
      );
    case 'connecting':
      return <Step title="연결 중" device={state.device.name}>기기에 연결하고 상태를 확인합니다.</Step>;
    case 'reconnecting':
      return <Step title="기기에 다시 연결 중" device={state.record.name}>등록된 기기에 연결하고 지갑 주소를 확인합니다.</Step>;
    case 'setupConfirm':
      return (
        <Step title="지갑 설정 1/3" device={state.device.name}>
          새 지갑을 만듭니다. 기기의 SW1을 눌러 승인하세요 (거절은 SW2). 키는 기기 안에서만 만들어지고 밖으로 나오지 않습니다.
        </Step>
      );
    case 'setupPin':
      return (
        <View>
          <Text style={styles.title}>지갑 설정 2/3: PIN</Text>
          <Text style={styles.small}>{state.device.name}</Text>
          <Text style={styles.cta}>{state.entry.position < 4 ? `기기 버튼으로 PIN 4자리를 정하세요 (${left}초)` : 'PIN을 저장하고 있습니다'}</Text>
          <PinBoxes entry={state.entry} />
          <Text style={styles.body}>{state.entry.position < 4 ? `${state.entry.position + 1}번째 자리를 입력하는 중입니다.` : '네 자리를 모두 확정했습니다.'}</Text>
          <Text style={styles.body}>SW1: 숫자를 1씩 올립니다 (9 다음은 0). LED가 짧게 깜빡입니다.</Text>
          <Text style={styles.body}>SW3: 이 자리를 확정하고 다음 자리로 갑니다. LED가 길게 깜빡입니다.</Text>
          <Text style={styles.body}>SW2: 0000부터 다시 입력합니다.</Text>
          <Text style={styles.hint}>PIN은 기기 안에만 저장됩니다. 입력 중인 숫자는 이 화면에 보이도록 폰으로 전달되지만 앱은 저장하지 않습니다. 결제 한도를 바꿀 때 다시 입력합니다.</Text>
        </View>
      );
    case 'passkey':
      return <CodeShow passkey={state.passkey} wallet={state.wallet} retry={state.retry} onDone={() => f?.passkeyNoted()} />;
    case 'rebonding':
      return (
        <View>
          <Text style={styles.title}>페어링 코드 입력</Text>
          <Text style={styles.small}>{state.device.name}</Text>
          <Text style={styles.cta}>폰에 블루투스 페어링 창이 뜨면 아래 6자리 페어링 코드를 입력하고 확인을 누르세요.</Text>
          <Text style={styles.code}>{state.passkey}</Text>
          <Text style={styles.hint}>{PAIRING_CODE_NOTE}</Text>
        </View>
      );
    case 'verifyReady':
      return (
        <View>
          <Text style={styles.title}>지갑 확인</Text>
          <Text style={styles.small}>{state.device.name}</Text>
          <Text style={styles.label}>지갑 주소</Text>
          <Text style={styles.mono}>{state.wallet}</Text>
          <Text style={styles.body}>
            기기가 이 지갑 주소의 키로 서명하는지 확인합니다. 시작하면 60초 안에 기기의 SW1을 누르세요 (거절은 SW2).
          </Text>
          <Button label="지갑 확인 시작" onPress={() => f?.startVerify()} />
        </View>
      );
    case 'verify':
      return (
        <Step title="지갑 확인" device={state.device.name}>
          {`기기의 SW1을 눌러 지갑을 확인하세요 (${left}초). 기기가 서명한 값을 앱이 지갑 주소 ${short(state.wallet)}와 대조합니다.`}
        </Step>
      );
    case 'failed':
      return (
        <View>
          <Text style={styles.title}>진행하지 못했습니다</Text>
          <Text style={styles.error}>{state.message}</Text>
          <Button label="처음부터 다시" onPress={() => f?.start()} />
          {state.record && <Button label="기기 지우기" onPress={() => f?.forget(state.record!)} />}
        </View>
      );
    case 'home':
      return <Home record={state.record} flow={f!} />;
  }
}

function Step({ title, device, children }: { title: string; device: string; children: React.ReactNode }) {
  return (
    <View>
      <Text style={styles.title}>{title}</Text>
      <Text style={styles.small}>{device}</Text>
      <Text style={styles.cta}>{children}</Text>
    </View>
  );
}

/** The wallet's home: payment mode, the payment screens, and the registered devices. */
function Home({ record, flow }: { record: DeviceRecord; flow: SetupFlow }) {
  const [screen, setScreen] = useState<Screen>({ kind: 'waiting' });
  const [mode, setMode] = useState<PaymentMode | null>(null);
  const confirm = useRef<ConfirmLink | null>(null);

  useEffect(() => {
    flow.releaseLink(); // the setup flow stops reading; the payment screens read the link from here
    let bonded = true;
    const lost = RenterBle.onBondLost(() => {
      bonded = false;
    });
    RenterBle.connect(record.address, GATT.service, GATT.rx, GATT.tx)
      .then(mtu => {
        const link = new ConfirmLink(
          {
            bonded: () => bonded,
            onFragment: h => {
              const sub = RenterBle.onFragment(b64 => h(fromBase64(b64)));
              return () => sub.remove();
            },
            write: f => RenterBle.writeFragment(toBase64(f)),
            mtu,
          },
          setScreen,
        );
        link.onPaymentMode = setMode;
        confirm.current = link;
      })
      .catch(() => setMode({ on: false, accepted: false, reason: '기기에 연결하지 못했습니다' }));
    return () => {
      lost.remove();
      confirm.current?.close();
    };
  }, [flow, record.address]);

  const togglePaymentMode = async () => {
    const on = !(mode?.accepted && mode.on);
    try {
      await confirm.current?.setPaymentMode(on);
    } catch (e) {
      setMode({ on, accepted: false, reason: String(e instanceof Error ? e.message : e) });
    }
  };

  return (
    <ScrollView>
      {screen.kind === 'waiting' && (
        <>
          <Text style={styles.title}>내 지갑</Text>
          <Text style={styles.label}>지갑 주소</Text>
          <Text style={styles.mono}>{record.wallet}</Text>
          <Text style={styles.body}>키오스크에서 결제를 시작하면 기기가 보낸 결제 내용이 여기에 나옵니다.</Text>
          <Text style={styles.hint}>PIN이 필요할 때는 기기 버튼으로 입력하세요. LED가 자릿수와 누를 버튼을 안내합니다.</Text>
          <Button
            label={mode?.accepted && mode.on ? '결제 모드 끄기' : `결제 모드 켜기 (${PAYMENT_MODE_SECONDS / 60}분)`}
            onPress={togglePaymentMode}
          />
          {mode?.accepted && mode.on && <Text style={styles.body}>결제 모드: 키오스크가 기기를 찾을 수 있습니다</Text>}
          {mode && !mode.accepted && <Text style={styles.error}>결제 모드를 바꾸지 못했습니다{mode.reason ? ` (${mode.reason})` : ''}</Text>}
          <Text style={styles.label}>등록된 기기</Text>
          {flow.devices().map(d => (
            <View key={d.address} style={styles.device}>
              <Text style={styles.value}>{d.name}</Text>
              <Text style={styles.small}>{d.address}  지갑 {short(d.wallet)}</Text>
              <Pressable onPress={() => flow.forget(d)}>
                <Text style={styles.remove}>기기 지우기</Text>
              </Pressable>
            </View>
          ))}
        </>
      )}
      {screen.kind === 'confirming' && (
        <>
          <Text style={styles.label}>가맹점</Text>
          <Text style={styles.value}>{screen.view.merchantName}</Text>
          <Text style={styles.label}>금액</Text>
          <Text style={styles.amount}>{screen.view.amount}</Text>
          {!screen.view.known && <Text style={styles.error}>알 수 없는 토큰 {screen.view.token}: 금액은 base unit입니다</Text>}
          <Text style={styles.label}>받는 주소</Text>
          <Text style={styles.mono}>{screen.view.payout}</Text>
          <Text style={styles.label}>주문</Text>
          <Text style={styles.mono}>{screen.view.orderShort}</Text>
          <Text style={styles.cta}>기기 버튼을 눌러 승인하세요</Text>
        </>
      )}
      {screen.kind === 'limit' && (
        <>
          <Text style={styles.title}>한도 변경</Text>
          <Text style={styles.label}>1회 한도</Text>
          <Text style={styles.value}>{screen.view.perPayment}</Text>
          <Text style={styles.label}>하루 한도</Text>
          <Text style={styles.value}>{screen.view.daily}</Text>
          {screen.pin && <PinBoxes entry={screen.pin} />}
          <Text style={styles.cta}>기기 버튼으로 PIN을 입력한 뒤 승인 버튼을 누르세요</Text>
        </>
      )}
      {screen.kind === 'result' && (
        <>
          <Text style={styles.title}>{OUTCOME_TEXT[screen.outcome] ?? screen.outcome}</Text>
          {screen.receipt ? (
            <Receipt receipt={screen.receipt} txHash={screen.txHash} />
          ) : (
            <>
              {screen.view && <Text style={styles.amount}>{screen.view.amount}</Text>}
              {screen.view && <Text style={styles.body}>{screen.view.merchantName}</Text>}
            </>
          )}
          {screen.reason && <Text style={styles.body}>{screen.reason}</Text>}
        </>
      )}
    </ScrollView>
  );
}

/** The merchant's digital receipt (payment-protocol.md 6); the explorer link is built here from the transaction hash. */
function Receipt({ receipt: r, txHash }: { receipt: DigitalReceipt; txHash?: string }) {
  const amount = (v: string | bigint) => `${receiptAmount(v, r.token.decimals)} ${r.token.symbol}`;
  const url = txHash ? explorerTxUrl(r.chainId, txHash) : null;
  const d = new Date(r.time * 1000);
  const p = (n: number) => String(n).padStart(2, '0');
  return (
    <ScrollView style={styles.paper}>
      <Text style={styles.store}>{r.store}</Text>
      {r.representative && <Text style={styles.small}>대표 {r.representative}</Text>}
      {r.businessNumber && <Text style={styles.small}>사업자등록번호 {r.businessNumber}</Text>}
      {r.address && <Text style={styles.small}>{r.address}</Text>}
      {r.phone && <Text style={styles.small}>전화 {r.phone}</Text>}
      <View style={styles.rule} />
      <Text style={styles.label}>주문번호 {r.orderNumber}</Text>
      <Text style={styles.small}>{`${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`}</Text>
      <View style={styles.rule} />
      {r.items.map((i, k) => (
        <View key={k} style={styles.line}>
          <Text style={styles.value}>{i.name} × {i.qty}</Text>
          <Text style={styles.value}>{amount(BigInt(i.unitPrice) * BigInt(i.qty))}</Text>
        </View>
      ))}
      <View style={styles.rule} />
      <View style={styles.line}>
        <Text style={styles.total}>합계</Text>
        <Text style={styles.total}>{amount(r.total)}</Text>
      </View>
      <Text style={styles.small}>결제수단 {r.token.symbol} (StableNet, chain {r.chainId})</Text>
      {txHash && <Text style={styles.mono}>거래 {txHash.slice(0, 10)}…{txHash.slice(-4)}</Text>}
      {url && <Button label="탐색기에서 거래 보기" onPress={() => Linking.openURL(url)} />}
    </ScrollView>
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
      <SafeAreaView style={styles.root}>
        <Renter />
      </SafeAreaView>
    </SafeAreaProvider>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, padding: 24, backgroundColor: '#fafafa' },
  title: { fontSize: 26, fontWeight: '600', color: '#111', marginBottom: 12 },
  body: { fontSize: 17, color: '#333', marginBottom: 12 },
  hint: { fontSize: 15, color: '#666', marginTop: 12 },
  label: { fontSize: 14, color: '#666', marginTop: 12 },
  value: { fontSize: 20, color: '#111' },
  amount: { fontSize: 36, fontWeight: '600', color: '#111' },
  mono: { fontSize: 15, color: '#111', fontFamily: 'monospace' },
  cta: { fontSize: 22, fontWeight: '600', color: '#1b4fa0', marginTop: 28 },
  error: { fontSize: 14, color: '#b00020', marginBottom: 12 },
  input: { fontSize: 15, borderBottomWidth: 1, borderColor: '#333', paddingVertical: 8, marginBottom: 16, color: '#111' },
  button: { backgroundColor: '#111', paddingVertical: 16, borderRadius: 8, alignItems: 'center', marginTop: 8 },
  disabled: { backgroundColor: '#999' },
  paper: { backgroundColor: '#fff', borderRadius: 8, padding: 16, marginTop: 8 },
  store: { fontSize: 22, fontWeight: '700', color: '#111' },
  small: { fontSize: 13, color: '#555', marginTop: 2 },
  rule: { borderBottomWidth: StyleSheet.hairlineWidth, borderColor: '#999', marginVertical: 8 },
  line: { flexDirection: 'row', justifyContent: 'space-between' },
  total: { fontSize: 20, fontWeight: '700', color: '#111', marginVertical: 4 },
  buttonText: { color: '#fff', fontSize: 18, fontWeight: '600' },
  device: { backgroundColor: '#fff', borderRadius: 8, padding: 14, marginTop: 10, borderWidth: StyleSheet.hairlineWidth, borderColor: '#bbb' },
  connect: { fontSize: 16, fontWeight: '600', color: '#1b4fa0', marginTop: 6 },
  remove: { fontSize: 15, color: '#b00020', marginTop: 6 },
  pinRow: { flexDirection: 'row', justifyContent: 'center', marginVertical: 16 },
  pinBox: { width: 56, height: 68, marginHorizontal: 6, borderRadius: 8, borderWidth: 1, borderColor: '#bbb', alignItems: 'center', justifyContent: 'center', backgroundColor: '#fff' },
  pinCurrent: { borderWidth: 3, borderColor: '#1b4fa0' },
  pinKept: { backgroundColor: '#e8eef8', borderColor: '#1b4fa0' },
  pinDigit: { fontSize: 34, fontWeight: '700', color: '#111', fontFamily: 'monospace' },
  pinLater: { color: '#bbb' },
  code: { fontSize: 44, fontWeight: '700', letterSpacing: 6, color: '#111', fontFamily: 'monospace', marginVertical: 8 },
});
