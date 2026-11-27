/**
 * Renter phone app (P02 design 3). A state machine (src/setup/flow.ts) takes the renter from a
 * device in pairing mode to a checked wallet: scan, bond, setup on the device buttons, the
 * reconnect code, the passkey bond, the wallet check. A registered device reconnects on start.
 * The home screen turns payment mode on (P02-FR-08) and shows what the device is about to sign.
 * There is no approve button: the renter approves on the device (P02-FR-05) and enters the PIN on
 * the device buttons (P02-FR-06).
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { BackHandler, Keyboard, Linking, PermissionsAndroid, Platform, Pressable, ScrollView, StatusBar, StyleSheet, Text, View } from 'react-native';
import { SafeAreaProvider, SafeAreaView } from 'react-native-safe-area-context';
import { explorerTxUrl, fromBase64, GATT, makeEntranceQr, readEntranceQr, receiptAmount, toBase64, type DigitalReceipt } from '@nu54/protocol';
import { ConfirmLink, PAYMENT_MODE_SECONDS, type PaymentMode, type Screen } from './link/confirmLink.ts';
import { bodyLink } from './setup/channel.ts';
import { SetupFlow, type DeviceRecord, type FoundDevice, type PinEntry, type SetupPlatform, type SetupState } from './setup/flow.ts';
import RenterBle from './specs/NativeRenterBle.ts';
import {ConsentGate} from './legal/ConsentGate.tsx';
import { WalletHome } from './wallet/WalletHome.tsx';
import {readCommissioningQr, type CommissioningPins} from './wallet/ownerWire.ts';
import { CustomerOrders } from './merchant/CustomerOrders.tsx';
import {useOpenOrders} from './merchant/OpenOrders.tsx';
import {MarketLab} from './market/MarketLab.tsx';
import {color, font} from './ui/theme.ts';
import {ExploreHome} from './explore/ExploreHome.tsx';
import {StoreFinder} from './merchant/StoreFinder.tsx';
import {orderPaymentPin} from './merchant/orderPaymentTrust.ts';
import {customerSubject, type StoreChoice} from './merchant/storeAccess.ts';
import {configuredDirectory} from './merchant/storeDirectory.ts';
import ShopQr from './specs/NativeShopQr.ts';
import {KitButton, TabBar, type ButtonVariant} from './ui/kit.tsx';
import { TravelHome, type TravelBookingScope } from './travel/TravelHome.tsx';

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

function Button({ label, onPress, disabled, variant = 'primary' }: { label: string; onPress: () => void; disabled?: boolean; variant?: ButtonVariant }) {
  return <View style={styles.buttonGap}><KitButton label={label} run={onPress} disabled={disabled} variant={variant} /></View>;
}

/** Height the on-screen keyboard covers. The edge-to-edge window is not resized for it on Android. */
function useKeyboardHeight(): number {
  const [height, setHeight] = useState(0);
  useEffect(() => {
    if (Platform.OS !== 'android') return;
    const shown = Keyboard.addListener('keyboardDidShow', event => setHeight(event.endCoordinates.height));
    const hidden = Keyboard.addListener('keyboardDidHide', () => setHeight(0));
    return () => { shown.remove(); hidden.remove(); };
  }, []);
  return height;
}

/** The required agreements come first; nothing behind them is started before. */
export default function App() {
  return <ConsentGate><Shell /></ConsentGate>;
}

function Shell() {
  const keyboard = useKeyboardHeight();
  const [renting, setRenting] = useState(false);
  const [commissioningPins, setCommissioningPins] = useState<CommissioningPins>();
  const [shopping, setShopping] = useState(false);
  const [travel, setTravel] = useState(false);
  const [travelBooking, setTravelBooking] = useState<TravelBookingScope>();
  const [marketLab, setMarketLab] = useState(false);
  const [marketTravel, setMarketTravel] = useState(false);
  const [exploreSite,setExploreSite]=useState('');
  const [exploreService,setExploreService]=useState<'travel'|'market'>();
  const resumeExplore=useRef(false);
  const [practicing, setPracticing] = useState(false);
  // App-level bottom tabs (홈 · 결제 · 지갑 · 둘러보기 · 설정).
  const [exploring, setExploring] = useState(false);
  const [walletScreen, setWalletScreen] = useState('');
  const [walletRequest, setWalletRequest] = useState<{screen:'home'|'board'|'owner'|'settings'|'dapp'|'allowance'|'walletconnect'; nonce:number}>();
  const requestWallet = (screen:'home'|'board'|'owner'|'settings'|'dapp'|'allowance'|'walletconnect') => setWalletRequest(previous => ({screen, nonce: (previous?.nonce ?? 0) + 1}));
  const rentalActive = useRef(false);
  rentalActive.current = renting;
  const marketReturnAllowed = useRef(true);
  marketReturnAllowed.current = !shopping && !travel && !marketTravel && !practicing;
  const marketLearningRoute = 'nu54://market-learning';
  const [orderLink, setOrderLink] = useState('');
  const [incomingPaymentLink, setIncomingPaymentLink] = useState(0);
  const [subscriptionLink, setSubscriptionLink] = useState('');
  const [counterQr, setCounterQr] = useState('');
  const [shopQr, setShopQr] = useState('');
  const [shopEntry, setShopEntry] = useState(false);
  // Runs while the app is in front on any tab, so a ready order is announced wherever the owner is.
  const openOrders = useOpenOrders();
  const [shownOrder, setShownOrder] = useState('');
  // A store picked from its store page, ordered from without a registration (merchant/storeAccess.ts).
  const [storeOrder, setStoreOrder] = useState<StoreChoice>();
  const openShopQr = useCallback((raw: string) => {
    setStoreOrder(undefined);
    setOrderLink('');
    setSubscriptionLink('');
    setCounterQr('');
    setShopQr(raw.slice(0, 97));
    setShopEntry(true);
    setIncomingPaymentLink(value => value + 1);
    setShopping(true);
  }, []);
  const openNearbyOrders = () => {
    openShopQr('');
  };
  // The store list and store pages of this build; undefined while no entrance address is bound.
  const storeDirectory = useMemo(() => configuredDirectory(), []);
  // The store finder, or one store's page first when a shortcut QR named it. Each opening is a new screen.
  const [finding, setFinding] = useState<{branch?: string; nonce: number}>();
  const openStores = useCallback((branch?: string) => {
    setShopping(false);
    setExploring(false);
    setFinding(previous => ({branch, nonce: (previous?.nonce ?? 0) + 1}));
  }, []);
  useEffect(()=>{
    let present=true;
    let receivedLiveLink=false;
    const offer=(url:string)=>{
      if(!present || rentalActive.current)return;
      const pins = readCommissioningQr(url);
      if (pins) {
        setCommissioningPins(pins);
        return;
      }
      if (url === marketLearningRoute) {
        if (marketReturnAllowed.current) setMarketLab(true);
        return;
      }
      if (url.startsWith('nu54://shop/')) {
        // A store shortcut opens that store's page. A build without the entrance address keeps the earlier menu route.
        let branch: string | undefined;
        try { branch = readEntranceQr(url); } catch {}
        if (storeDirectory && branch) openStores(branch);
        else openShopQr(url);
        return;
      }
      const subscription = /^nu54:\/\/subscriptions\/([0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12})$/.exec(url);
      if (subscription) {
        setIncomingPaymentLink(value => value + 1);
        setOrderLink('');
        setCounterQr('');
        setShopQr('');
        setShopEntry(false);
        setSubscriptionLink(subscription[1]);
        setShopping(true);
        return;
      }
      const ready = /^nu54:\/\/order\/ready\/([0-9a-f]{8}-(?:[0-9a-f]{4}-){3}[0-9a-f]{12})$/.exec(url);
      if (ready) {
        setIncomingPaymentLink(value => value + 1);
        setOrderLink('');
        setSubscriptionLink('');
        setCounterQr('');
        setShopQr('');
        setShopEntry(false);
        setShownOrder(ready[1]);
        setShopping(true);
        return;
      }
      if (!url.startsWith('nu54://order')) return;
      setIncomingPaymentLink(value => value + 1);
      setSubscriptionLink('');
      setCounterQr('');
      setShopQr('');
      setShopEntry(false);
      setOrderLink(url === 'nu54://order' ? '' : url.slice(0, 133));
      setShopping(true);
    };
    Linking.getInitialURL().then(url=>{if(url && !receivedLiveLink)offer(url);}).catch(()=>{});
    const callback=Linking.addEventListener('url',event=>{
      if (event.url === marketLearningRoute || event.url.startsWith('nu54://order') || event.url.startsWith('nu54://subscriptions/') || event.url.startsWith('nu54://shop/') || readCommissioningQr(event.url))
        receivedLiveLink=true;
      offer(event.url);
    });
    return ()=>{present=false;callback.remove();};
  },[marketLearningRoute,openShopQr,openStores,storeDirectory]);
  const [honorExistingRental, setHonorExistingRental] = useState(true);
  const rental = useCallback(() => setRenting(true), []);
  const profiles = () => {
    setHonorExistingRental(false);
    setRenting(false);
  };
  const openPaymentList = (qr = '') => {
    setStoreOrder(undefined);
    setOrderLink('');
    setSubscriptionLink('');
    setCounterQr(qr);
    setShopQr('');
    setShopEntry(false);
    setIncomingPaymentLink(value => value + 1);
  };
  const closePayments = () => {
    const left = storeOrder;
    openPaymentList();
    setShownOrder('');
    setShopping(false);
    // An order started from a store page goes back to that page.
    if (left && storeDirectory) openStores(left.branch);
  };
  /** The menu of a store page, and from there the order. `sale` opens one of its orders first. */
  const openStoreOrder = (store: StoreChoice, sale = '') => {
    openPaymentList();
    setFinding(undefined);
    setStoreOrder(store);
    setShopEntry(true);
    setShownOrder(sale);
    setShopping(true);
  };
  const openTravelBookings = (scope: TravelBookingScope) => {
    setTravelBooking(scope);
    setShopping(false);
    setTravel(true);
  };
  const closeTravel = useCallback(() => {
    if (exploreService==='travel') resumeExplore.current=true;
    setExploreService(undefined);
    setTravel(false);
    if (travelBooking) setShopping(true);
    setTravelBooking(undefined);
  },[exploreService,travelBooking]);
  const closeMarket=useCallback(()=>{
    if (exploreService==='market') resumeExplore.current=true;
    setExploreService(undefined);
    setMarketTravel(false);
    setMarketLab(false);
  },[exploreService]);
  const onExploreRestored=useCallback(()=>{resumeExplore.current=false;},[]);
  useEffect(()=>{
    if ((!exploreService && !marketTravel) || shopping || renting) return;
    const listener=BackHandler.addEventListener('hardwareBackPress',()=>{
      if (marketTravel) setMarketTravel(false);
      else if (exploreService==='travel') closeTravel();
      else closeMarket();
      return true;
    });
    return ()=>listener.remove();
  },[exploreService, marketTravel, shopping, renting, closeTravel,closeMarket]);
  const closeOverlays = () => { setShopping(false); setTravel(false); setMarketLab(false); setMarketTravel(false); setExploring(false); setFinding(undefined); };
  const settingsScreens = ['settings', 'networks', 'legal', 'board', 'setup', 'return-address', 'reset', 'operator-reset', 'settings-access', 'settings-diagnostics', 'settings-device', 'policy-picker', 'lock-picker',
    'credentials', 'travel-privacy', 'policy', 'policy-applied', 'policy-device'];
  const currentTab = shopping ? 'pay' : finding || travel || marketLab || exploring || ['dapp', 'walletconnect', 'allowance'].includes(walletScreen) ? 'explore'
    : settingsScreens.includes(walletScreen) ? 'settings'
    : ['', 'home', 'profiles', 'practice', 'install'].includes(walletScreen) ? 'home' : 'wallet';
  const openSettings = () => { closeOverlays(); requestWallet('settings'); };
  const openBoard = () => { closeOverlays(); requestWallet('board'); };
  // Tabs stay out of rental mode, practice and the secure wallet subflows that hide navigation.
  const showTabs = !renting && !practicing;
  useEffect(() => {
    // Travel and the learning market opened from the explore tab have no Back handling of their own:
    // Back returns to the explore page, or to payments for a booking opened from there.
    if (!showTabs || !(travel || marketLab) || exploreService || marketTravel) return;
    const listener = BackHandler.addEventListener('hardwareBackPress', () => {
      if (travel && travelBooking) { closeTravel(); return true; }
      closeOverlays(); setExploring(true); return true;
    });
    return () => listener.remove();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [showTabs, travel, marketLab, exploreService, marketTravel, travelBooking]);
  useEffect(() => {
    // The store finder opens from the explore tab, and system Back returns there.
    if (!showTabs || !finding || shopping) return;
    const listener = BackHandler.addEventListener('hardwareBackPress', () => { closeOverlays(); setExploring(true); return true; });
    return () => listener.remove();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [showTabs, finding, shopping]);
  useEffect(() => {
    // In the rental (trial) screens system Back returns to the wallet choice instead of leaving the app.
    if (!renting) return;
    const listener = BackHandler.addEventListener('hardwareBackPress', () => { profiles(); return true; });
    return () => listener.remove();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [renting]);
  useEffect(() => {
    // On the payments and explore tabs' first pages, system Back returns to the home tab instead of leaving the app.
    if (!showTabs || !(shopping || exploring)) return;
    const listener = BackHandler.addEventListener('hardwareBackPress', () => { closeOverlays(); requestWallet('home'); return true; });
    return () => listener.remove();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [showTabs, shopping, exploring]);
  return (
    <SafeAreaProvider>
      <StatusBar barStyle="dark-content" />
      <SafeAreaView style={styles.root}>
        <View style={[styles.page, keyboard > 0 && {paddingBottom: keyboard}]}>
        {shopping ? <CustomerOrders tabbed={showTabs} initialOrder={shownOrder} openBoard={openBoard} key={incomingPaymentLink} initialCounterQr={counterQr} initialShopQr={shopQr} shopEntry={shopEntry} openShopQr={openShopQr} store={storeOrder}
          openPayments={openPaymentList} openTravel={openTravelBookings} back={closePayments}
          initialLocator={orderLink} subscriptionLink={subscriptionLink} permission={bluetoothPermission} /> : finding && !renting ? <ScrollView
          contentContainerStyle={styles.finder} keyboardShouldPersistTaps="handled" testID="store-finder">
          <StoreFinder key={finding.nonce} directory={storeDirectory} initialBranch={finding.branch}
            scan={ShopQr ? () => ShopQr!.scan() : undefined}
            back={() => { closeOverlays(); requestWallet('home'); }}
            order={store => {
              // A build with its order contract bound orders without a registration; one without keeps the earlier route.
              if (orderPaymentPin()) openStoreOrder(store);
              else { setFinding(undefined); openShopQr(makeEntranceQr(store.branch)); }
            }}
            payQr={raw => { setFinding(undefined); openPaymentList(raw); setShopping(true); }} />
        </ScrollView> : renting ? <View style={styles.root}>
          <Button label="지갑 프로필 선택" onPress={profiles} variant="secondary" />
          <Renter />
        </View> : marketLab ? <View style={styles.home}>
          <View style={marketTravel ? styles.hidden : styles.home} accessibilityElementsHidden={marketTravel}
            importantForAccessibility={marketTravel ? 'no-hide-descendants' : 'auto'}>
            <MarketLab back={closeMarket} permission={bluetoothPermission} openTravel={() => setMarketTravel(true)} />
          </View>
          {marketTravel && <TravelHome back={() => setMarketTravel(false)} backLabel="학습 시장으로 돌아가기"
            permission={bluetoothPermission} />}
        </View> : travel ? <TravelHome bookingScope={travelBooking} back={closeTravel} permission={bluetoothPermission} /> : exploring ? <ExploreHome
          places={[
            {label: '여행지 찾기', detail: '지도와 목록에서 장소를 찾고 예약합니다', run: () => { setExploring(false); setTravel(true); }},
            {label: '매장 찾기', detail: '이름으로 매장을 찾아 매장 화면을 보고 미리 주문합니다', run: () => openStores()},
            {label: '가까운 매장에서 주문', detail: '근처 매장의 메뉴를 보고 주문합니다', run: () => { setExploring(false); openNearbyOrders(); }},
          ]}
          apps={[
            {label: 'DApp 둘러보기', detail: '연결한 사이트에서 로그인과 결제를 요청받습니다', run: () => { setExploring(false); requestWallet('dapp'); }},
            {label: '외부 DApp 연결', detail: '다른 앱의 사이트와 WalletConnect로 연결합니다', run: () => { setExploring(false); requestWallet('walletconnect'); }},
            {label: '온체인 토큰 권한', detail: '사이트에 준 토큰 사용 권한을 확인하고 철회합니다', run: () => { setExploring(false); requestWallet('allowance'); }},
            {label: '테스트 시장 학습', detail: '테스트넷에서 교환과 유동성을 연습합니다', run: () => { setExploring(false); setMarketLab(true); }},
          ]} /> : <View style={styles.home}>
          {/* Services open from the 둘러보기 tab; home keeps three shortcuts. */}
          <WalletHome tabbed openOrders={openOrders} openOrder={sale => {
            // An order made from a store page belongs to the board account's own customer, and opens with its store.
            const made = openOrders.find(row => row.sale === sale);
            if (made && storeDirectory && orderPaymentPin() && made.customer === customerSubject(made.payer))
              openStoreOrder({branch: made.branch, name: made.store ?? '매장', merchantId: made.merchantId}, sale);
            else { openPaymentList(); setShownOrder(sale); setShopping(true); }
          }} services={[
              {label: '매장 QR 결제', run: () => { openPaymentList(); setShopping(true); }},
              {label: '근처 매장', run: openNearbyOrders},
              {label: '여행 예약', run: () => { setTravel(true); }},
            ]} requestScreen={walletRequest} onScreenChange={setWalletScreen} onPracticeChange={setPracticing} rental={rental} orders={()=>setShopping(true)} permission={bluetoothPermission} honorExistingRental={honorExistingRental}
            initialExplore={resumeExplore.current} onExploreRestored={onExploreRestored}
            exploreSite={exploreSite} onExploreSiteChange={setExploreSite}
            openExploreTravel={()=>{setExploreService('travel');setTravel(true);}}
            openExploreMarket={()=>{setExploreService('market');setMarketLab(true);}}
            commissioningPins={commissioningPins} dismissCommissioningPins={()=>setCommissioningPins(undefined)} />
        </View>}
        </View>
        {showTabs && !keyboard && <TabBar current={currentTab} disabled={false} tabs={[
          {key: 'home', label: '홈', run: () => { closeOverlays(); requestWallet('home'); }},
          {key: 'pay', label: '결제', run: () => { closeOverlays(); setShownOrder(''); setShopping(true); }},
          {key: 'wallet', label: '지갑', run: () => { closeOverlays(); requestWallet('owner'); }},
          {key: 'explore', label: '둘러보기', run: () => { closeOverlays(); setExploring(true); }},
          {key: 'settings', label: '설정', run: openSettings},
        ]} />}
      </SafeAreaView>
    </SafeAreaProvider>
  );
}

// Rental (trial) screens use the same tokens and typeface as the wallet.
const styles = StyleSheet.create({
  home: { flex: 1 },
  finder: { paddingBottom: 24 },
  hidden: { display: 'none' },
  root: { flex: 1, backgroundColor: color.ground },
  page: { flex: 1, paddingHorizontal: 16, paddingTop: 12 },
  buttonGap: { marginTop: 8 },
  title: { fontFamily: font, fontSize: 28, fontWeight: '800', letterSpacing: -0.8, color: color.ink, marginVertical: 12 },
  body: { fontFamily: font, fontSize: 16, lineHeight: 24, color: color.ink, marginBottom: 12 },
  hint: { fontFamily: font, fontSize: 14, lineHeight: 20, color: color.mute, marginTop: 12 },
  label: { fontFamily: font, fontSize: 14, color: color.mute, marginTop: 12 },
  value: { fontFamily: font, fontSize: 20, fontWeight: '700', color: color.ink },
  amount: { fontFamily: font, fontSize: 36, fontWeight: '800', letterSpacing: -1.2, color: color.ink },
  mono: { fontSize: 14, color: color.ink, fontFamily: 'monospace' },
  cta: { fontFamily: font, fontSize: 20, fontWeight: '800', color: color.green, marginTop: 28 },
  error: { fontFamily: font, fontSize: 15, color: color.danger, marginBottom: 12 },
  input: { fontFamily: font, fontSize: 16, borderWidth: 1, borderColor: '#8fa497', borderRadius: 12, padding: 12, minHeight: 50, marginBottom: 16, color: color.ink, backgroundColor: color.sheet },
  paper: { backgroundColor: color.sheet, borderRadius: 18, borderWidth: 1, borderColor: color.line, padding: 16, marginTop: 8 },
  store: { fontFamily: font, fontSize: 20, fontWeight: '800', color: color.ink },
  small: { fontFamily: font, fontSize: 13, color: color.mute, marginTop: 2 },
  rule: { borderBottomWidth: 1, borderColor: color.line, marginVertical: 8 },
  line: { flexDirection: 'row', justifyContent: 'space-between' },
  total: { fontFamily: font, fontSize: 20, fontWeight: '800', color: color.ink, marginVertical: 4 },
  device: { backgroundColor: color.sheet, borderRadius: 14, padding: 14, marginTop: 10, borderWidth: 1, borderColor: color.line },
  connect: { fontFamily: font, fontSize: 16, fontWeight: '700', color: color.green, marginTop: 6 },
  remove: { fontFamily: font, fontSize: 15, color: color.danger, marginTop: 6 },
  pinRow: { flexDirection: 'row', justifyContent: 'center', marginVertical: 16 },
  pinBox: { width: 56, height: 68, marginHorizontal: 6, borderRadius: 12, borderWidth: 1, borderColor: color.line, alignItems: 'center', justifyContent: 'center', backgroundColor: color.sheet },
  pinCurrent: { borderWidth: 3, borderColor: color.green },
  pinKept: { backgroundColor: color.mintSoft, borderColor: color.green },
  pinDigit: { fontSize: 34, fontWeight: '700', color: color.ink, fontFamily: 'monospace' },
  pinLater: { color: color.line },
  code: { fontSize: 44, fontWeight: '700', letterSpacing: 6, color: color.ink, fontFamily: 'monospace', marginVertical: 8 },
});
