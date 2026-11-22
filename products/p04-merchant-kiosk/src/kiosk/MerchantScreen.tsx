import { sha256 } from '@noble/hashes/sha256';
import { menuCategories, MenuCategorySelector } from './menuCategories';
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ComponentRef,
} from 'react';
import {
  AccessibilityInfo,
  AppState,
  Linking,
  PermissionsAndroid,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import {
  CommerceClient,
  NearbyCommerceLink,
  V2Journal,
  readNearbyReady,
  commerceProfile,
  commerceAmount,
  commerceCartTotals,
  commerceLineKey,
  commerceLineCapacity,
  commerceNeedsOptionSupport,
  commerceCartAmount,
  commerceReservationRejected,
  reviewCommerceCart,
  commerceMenuLabels,
  commerceMenuDisclosure,
  commerceRequestId,
  encodeOrderLocator,
  makeEntranceQr,
  fromBase64,
  bytesToHex,
  hexToBytes,
  paymentProfile,
  preparePresentedPayment,
  presentationFingerprint,
  utf8Encode,
  type PresentationScope,
  quoteOrderRevision,
  verifyCommerceApproval,
  type CommerceCart,
  type CommerceOrder,
  type CommercePrice,
  type CommerceCartReview,
  type CommerceSelection,
  type StaffAccess,
  type TerminalMode,
  type TerminalSettings,
  type TerminalTransition,
  type CheckoutComparison,
  type CommerceQuote,
  type CommercePaymentProfile,
} from '@nu54/protocol';
import Http from '../specs/NativeCommerceHttp.ts';
import Nearby from '../specs/NativeNearby.ts';
import { NearbyRelay } from './nearbyRelay.ts';
import { PickupKitchen } from './PickupKitchen.tsx';
import { ReceiptPrint } from './ReceiptPrint.tsx';
import { pickupHash, pickupJournalName } from './pickupJournal.ts';
import { QrCode } from '../DigitalReceiptView.tsx';
import { boardPayment, findPaymentBoards, reviewRentalBoard } from './boardPayment.ts';
import { MerchantInbox } from './merchantInbox.ts';
import { encodeCachedMenu, menuCacheJournal, networkUnavailable, OfflineMenu, readCachedMenu, type CachedMenu } from './offlineMenu.tsx';
import {
  readPOSOrder,
  posAmount,
  validPOSHandoff,
  type POSOrder,
} from './pos.ts';

const slots = ['terminal', 'operator', 'finance'] as const;
type Slot = (typeof slots)[number];
type StaffView = 'public' | 'login' | 'checking' | 'denied' | 'work';
type PendingPayment = {
  branch: string;
  terminal: string;
  order: CommerceOrder;
  scope: PresentationScope;
  quote: CommerceQuote;
  request: string;
  key: string;
  signature?: string;
};
const paymentJournal = 'terminal-payment-v2';
const modeRequestJournal = 'terminal-mode-request-v2';
const modeAppliedJournal = 'terminal-mode-applied-v2';
const terminalRef =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
type ModeRequest = {
  branch: string;
  operator: string;
  terminal: string;
  request: string;
  revision: number;
  mode: TerminalMode['mode'];
};
function modeRequest(
  raw: string,
  client: CommerceClient,
): ModeRequest | undefined {
  if (!raw) return undefined;
  const value = JSON.parse(raw) as ModeRequest;
  if (
    !value ||
    value.branch !== client.profile.branchRef ||
    value.operator !== client.profile.subjectRef ||
    !terminalRef.test(value.terminal) ||
    !terminalRef.test(value.request) ||
    !Number.isSafeInteger(value.revision) ||
    value.revision < 1 ||
    (value.mode !== 'menu' && value.mode !== 'pos')
  )
    throw new Error('MODE_JOURNAL');
  return value;
}
type PublicScreen = {
  session: string;
  connectionEnded?: true;
  key?: string;
  sale?: CommerceOrder;
  locator?: string;
  pos?: POSOrder;
  selectedCode?: string;
  posSelection?: POSSelection;
  pairing?: {state: 'binding'; code: string} | {state: 'unknown'; checkedAt: number};
};
type POSSelection = {state: 'locking'; code: string} | {state: 'unknown' | 'conflict'};
type MenuDetail = { sku: string; option?: string; quantity: number; error?: 'option' | 'stock' };
type CartReplacement = {
  previous: CommerceSelection[string];
  next: CommerceSelection[string];
};
type SetupState = 'loading' | 'configure' | 'blocked' | Slot;
const now = () => Math.floor(Date.now() / 1000);
const id = async () =>
  commerceRequestId(fromBase64(await Http!.randomBytes(16)));

function Button({
  label,
  run,
  disabled = false,
}: {
  label: string;
  run: () => void;
  disabled?: boolean;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={label}
      accessibilityState={{ disabled }}
      onPress={run}
      disabled={disabled}
      style={[s.button, disabled && s.disabled]}
    >
      <Text style={s.buttonText}>{label}</Text>
    </Pressable>
  );
}

function Label({ children }: { children: React.ReactNode }) {
  return <Text style={s.label}>{children}</Text>;
}

function clientFor(raw: string, slot: Slot, profileDigest: string) {
  return new CommerceClient(
    commerceProfile(raw, slot),
    body => Http!.exchangeScoped(slot, profileDigest, body),
    id,
  );
}

async function forgetMenuCache() {
  if (Http) await Http.saveJournal(menuCacheJournal,'').catch(()=>undefined);
}

export function MerchantScreen({ legacy }: { legacy?: () => void }) {
  const [setup, setSetup] = useState<SetupState>('loading');
  const [pickupStaff, setPickupStaff] = useState(false);
  const [pickupOpen, setPickupOpen] = useState(false);
  const [printView, setPrintView] = useState<{ sale?: string }>();
  const closePrint = useCallback(() => setPrintView(undefined), []);
  const [staffView, setStaffView] = useState<StaffView>('public');
  const [registration, setRegistration] = useState<{
    slot: Slot;
    digest: string;
    endpoint: string;
    pin: string;
  }>();
  const [pickupJournal, setPickupJournal] = useState<string>();
  const [staff, setStaff] = useState<StaffAccess>();
  const staffEpoch = useRef(0);
  const staffChecking = useRef(false);
  const [orderPage, setOrderPage] = useState<string[]>();
  const [role, setRole] = useState<Slot>('terminal');
  const [client, setClient] = useState<CommerceClient>();
  const [inbox, setInbox] = useState<MerchantInbox>();
  const [enrollment, setEnrollment] = useState('');
  const [pin, setPin] = useState('');
  const pickupServerPinInput = useRef<ComponentRef<typeof TextInput>>(null);
  const pickupProofPinInput = useRef<ComponentRef<typeof TextInput>>(null);
  const [proofPin, setProofPin] = useState('');
  const [publicKey, setPublicKey] = useState('');
  const [nearbyKey, setNearbyKey] = useState('');
  const [nearbyProfile, setNearbyProfile] = useState('');
  const [paymentKey, setPaymentKey] = useState('');
  const [paymentEnrollment, setPaymentEnrollment] = useState('');
  const [policy, setPolicy] = useState<CommercePaymentProfile>();
  const [pendingPayment, setPendingPayment] = useState<PendingPayment>();
  const [boardOptions, setBoardOptions] = useState<{ handle: string }[]>([]);
  const abortRef = useRef<AbortController>(undefined);
  const [onlinePairs, setOnlinePairs] = useState<CheckoutComparison[]>([]);
  const [unresolvedComparisons, setUnresolvedComparisons] = useState(false);
  const [pairCode, setPairCode] = useState<string>();
  const [entrance, setEntrance] = useState<{label: string; qr: string}>();
  const relayRef = useRef<NearbyRelay>(undefined);
  const [target, setTarget] = useState('');
  const [mode, setMode] = useState<TerminalSettings>();
  const [modeReview, setModeReview] = useState<TerminalMode['mode']>();
  const [pendingMode, setPendingMode] = useState<ModeRequest>();
  const [menuLocale, setMenuLocale] = useState<'ko' | 'en'>('ko');
  const [stock, setStock] = useState<CommercePrice[]>([]);
  const [menuCategory, setMenuCategory] = useState('');
  const categories = menuCategories(stock);
  const selectedCategory = categories.some(({ key }) => key === menuCategory) ? menuCategory : '';
  const [offlineMenu,setOfflineMenu]=useState<CachedMenu>();
  const [menuBranchLabel,setMenuBranchLabel]=useState<string>();
  const menuScope=useRef<string>(undefined);
  const [choice, setChoice] = useState<CommerceSelection>({});
  const [basketOpen, setBasketOpen] = useState(false);
  const [publicGuide, setPublicGuide] = useState<'phone' | 'cancel'>();
  const publicScroll = useRef<ComponentRef<typeof ScrollView>>(null);
  const basketHeading = useRef<ComponentRef<typeof Text>>(null);
  const [menuDetail, setMenuDetail] = useState<MenuDetail>();
  const optionHeading = useRef<ComponentRef<typeof Text>>(null);
  const [cartReplacement, setCartReplacement] = useState<CartReplacement>();
  const [cartReview, setCartReview] = useState<CommerceCartReview>();
  const [pos, setPos] = useState('');
  const posFlights = useRef(new Map<string, string>());
  const [screen, setScreen] = useState<PublicScreen>();
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const active = useRef<PublicScreen>(undefined);
  const latest = useRef<symbol>(undefined);
  const screenExpiry = useRef<ReturnType<typeof setTimeout>>(undefined);
  const qrExpiry = useRef<ReturnType<typeof setTimeout>>(undefined);

  const show = useCallback((next: PublicScreen | undefined) => {
    active.current = next;
    setScreen(next);
  }, []);
  const retirePublicOrder = useCallback(() => {
    const current = active.current;
    if (current && !current.pos && (!current.sale || current.sale.state === 'preview') &&
        !current.sale?.menuCheckout?.payerBound && !current.selectedCode && inbox && client) {
      void inbox.cancelPublic(current.session, (session, key) => client.cancelPublicOrder(session, key)).catch(() => {});
    }
    inbox?.endPublic();
    relayRef.current?.clearPublic();
    abortRef.current?.abort();
    setBoardOptions([]);
    setOnlinePairs([]);
    setPairCode(undefined);
    if (qrExpiry.current) clearTimeout(qrExpiry.current);
    setChoice({});
    setBasketOpen(false);
    setPublicGuide(undefined);
    setMenuCategory('');
    setMenuDetail(undefined);
    setCartReplacement(undefined);
    setCartReview(undefined);
    setPos('');
    setMessage('');
  }, [client, inbox]);
  const clearPublic = useCallback(() => {
    retirePublicOrder();
    if (screenExpiry.current) clearTimeout(screenExpiry.current);
    show(undefined);
  }, [retirePublicOrder, show]);

  const closeStaff = useCallback(
    (next: StaffView = 'public') => {
      staffEpoch.current++;
      staffChecking.current = false;
      latest.current = undefined;
      setBusy(false);
      setStaff(undefined);
      setStaffView(next);
      setPickupOpen(false);
      closePrint();
      setPickupJournal(undefined);
      setTarget('');
      setMode(undefined);
      setModeReview(undefined);
      setPendingMode(undefined);
      setOrderPage(undefined);
      setStock([]);
      setMenuCategory('');
      setOfflineMenu(undefined);
      setMenuBranchLabel(undefined);
      clearPublic();
    },
    [clearPublic, closePrint],
  );

  async function checkStaff() {
    if (!client || !Http || !registration || staffChecking.current) return;
    const generation = ++staffEpoch.current;
    staffChecking.current = true;
    setStaff(undefined);
    setStaffView('checking');
    try {
      const access = await client.staffAccess();
      if (generation !== staffEpoch.current) return;
      if (access.permissions.includes('terminal_settings')) {
        const pending = modeRequest(
          await Http.loadJournal(modeRequestJournal),
          client,
        );
        if (generation !== staffEpoch.current) return;
        if (pending) {
          setTarget(pending.terminal);
          setPendingMode(pending);
        }
      }
      if (
        access.permissions.includes('pickup') ||
        access.permissions.includes('print_receipts')
      ) {
        const identity = await Http.publicIdentity(registration.slot);
        if (
          !identity ||
          pickupHash(await Http.loadProfile(registration.slot)) !==
            registration.digest
        )
          throw new Error('STAFF_ENROLLMENT_CHANGED');
        if (generation !== staffEpoch.current) return;
        setPickupJournal(
          pickupJournalName(
            client.profile,
            registration.endpoint,
            registration.pin,
            identity,
          ),
        );
      }
      setStaff(access);
      setStaffView('work');
    } catch {
      if (generation === staffEpoch.current) closeStaff('denied');
    } finally {
      if (generation === staffEpoch.current) staffChecking.current = false;
    }
  }

  useEffect(() => {
    if (!staff || !client) return;
    const generation = staffEpoch.current;
    function refresh() {
      client!
        .staffAccess()
        .then(current => {
          if (staffEpoch.current !== generation) return;
          if (
            current.console !== staff?.console ||
            current.permissions.join('/') !== staff?.permissions.join('/')
          )
            closeStaff('denied');
        })
        .catch(() => {
          if (staffEpoch.current === generation) closeStaff('denied');
        });
    }
    const timer = setInterval(refresh, 15_000);
    return () => clearInterval(timer);
  }, [staff, client, closeStaff]);

  function keepPublicActive() {
    if (!active.current || active.current.connectionEnded) return;
    if (screenExpiry.current) clearTimeout(screenExpiry.current);
    screenExpiry.current = setTimeout(clearPublic, 60_000);
  }

  async function load() {
    if (!Http) {
      setSetup('blocked');
      return;
    }
    const profiles = await Promise.all(
      slots.map(slot => Http!.loadProfile(slot)),
    );
    const enrolled = profiles.filter(Boolean).length;
    if (enrolled > 1) {
      await forgetMenuCache();
      setSetup('blocked');
      return;
    }
    if (!enrolled) {
      await forgetMenuCache();
      setSetup('configure');
      return;
    }
    const index = profiles.findIndex(Boolean);
    const slot = slots[index];
    const raw = profiles[index];
    const digest = pickupHash(raw);
    const next = clientFor(raw, slot, digest);
    const { endpoint, pin: serverPin } = JSON.parse(raw) as {
      endpoint: string;
      pin: string;
    };
    setRegistration({ slot, digest, endpoint, pin: serverPin });
    if (slot === 'terminal') {
      const terminal = profiles[index];
      const config = JSON.parse(terminal) as {
        nearby?: unknown;
        boardTransport?: unknown;
        pickupKitchen?: boolean;
      };
      setPickupStaff(config.pickupKitchen === true);
      if (config.boardTransport)
        setPaymentEnrollment(JSON.stringify(config.boardTransport));
      setPolicy(paymentProfile(terminal));
      const identity=await Http.publicIdentity(slot);
      menuScope.current=identity ? pickupHash(JSON.stringify([digest,identity])) : undefined;
      const cached=await Http.loadJournal(menuCacheJournal).catch(()=> '');
      if(cached && (!menuScope.current || !readCachedMenu(cached,menuScope.current,next.profile.branchRef,now()))) {
        await Http.saveJournal(menuCacheJournal,'').catch(()=>undefined);
      }
      const existing = await Http!.loadJournal(paymentJournal);
      if (existing) {
        const record = JSON.parse(existing) as PendingPayment;
        if (
          record.branch !== next.profile.branchRef ||
          record.terminal !== next.profile.subjectRef ||
          !/^[0-9a-f]{32}$/.test(record.request)
        )
          throw new Error('PAYMENT_JOURNAL');
        setPendingPayment(record);
      }
      if (config.nearby) setNearbyProfile(JSON.stringify(config.nearby));
      const journal = await MerchantInbox.open(
        {
          getSetting: name => Http!.loadJournal(name),
          putSetting: (name, value) => Http!.saveJournal(name, value),
        },
        next.profile.branchRef,
        next.profile.subjectRef,
      );
      setUnresolvedComparisons(journal.recovery().some(row => row.comparison));
      setInbox(journal);
    } else {
      // Staff profiles may also identify the deployment's display currency.
      // Legacy or incomplete display metadata still permits exact base units.
      try {
        setPolicy(slot === 'operator' ? paymentProfile(raw) : undefined);
      } catch {
        setPolicy(undefined);
      }
      await forgetMenuCache();
      setStaffView('login');
    }
    setClient(next);
    setSetup(slot);
  }

  useEffect(() => {
    load().catch(async () => {
      await forgetMenuCache();
      setSetup('blocked');
    });
  }, []);
  useEffect(
    () => () => {
      staffEpoch.current++;
    },
    [],
  );
  useEffect(() => {
    const sub = AppState.addEventListener('change', state => {
      if (state !== 'active') closeStaff();
    });
    return () => {
      sub.remove();
      inbox?.endPublic();
      if (screenExpiry.current) clearTimeout(screenExpiry.current);
      if (qrExpiry.current) clearTimeout(qrExpiry.current);
      active.current = undefined;
    };
  }, [inbox, closeStaff]);

  useEffect(() => {
    if (!Nearby || !client || !inbox || setup !== 'terminal' || !nearbyProfile)
      return;
    const module = Nearby;
    const journal = new V2Journal(128);
    const relay = new NearbyRelay(
      client,
      inbox,
      async () => fromBase64(await Http!.randomBytes(16)),
      () => (active.current?.locator ? active.current.sale?.id : undefined),
      check => setPairCode(check?.code),
      () => (active.current?.pos ? active.current.sale?.id : undefined),
      policy && paymentEnrollment ? {
        policy,
        read: (payment, authority, sourceSignal, decided, begin) => new Promise<string>((resolve, reject) => {
          if (sourceSignal.aborted || abortRef.current || !active.current) {
            reject(new Error('PAYMENT_SCOPE'));
            return;
          }
          const abort = new AbortController();
          const cancel = () => abort.abort();
          abortRef.current = abort;
          sourceSignal.addEventListener('abort', cancel);
          const timer = setTimeout(() => {
            reject(new Error('PAYMENT_EXPIRY'));
            cancel();
          }, Math.max(0, (authority.expiresAt - now()) * 1000));
          void (async () => {
            try {
              setBoardOptions([]);
              await reviewRentalBoard(module,paymentEnrollment,client.profile.branchRef,payment,abort.signal,
                {scope:authority,begin,reviewed:receipt=>{
                  if (abort.signal.aborted || authority.expiresAt <= now()) return;
                  clearTimeout(timer);
                  resolve(receipt);
                },decided});
              reject(new Error('PAYMENT_RESULT_UNKNOWN'));
            } catch (error) {
              reject(error);
            } finally {
              clearTimeout(timer);
              sourceSignal.removeEventListener('abort', cancel);
              if (abortRef.current === abort) abortRef.current = undefined;
            }
          })();
        }),
      } : undefined,
    );
    relayRef.current = relay;
    const links = new Map<string, NearbyCommerceLink>();
    const subscription = module.onNearby(raw => {
      try {
        const data = JSON.parse(raw);
        if (typeof data.link !== 'string') return;
        if (data.kind === 'ready') {
          const peer = readNearbyReady(data);
          if (
            peer.branch !== client.profile.branchRef ||
            links.has(peer.link) ||
            links.size >= 8
          ) {
            module.close(peer.link).catch(() => {});
            return;
          }
          relay.activate(peer.link);
          links.set(
            peer.link,
            new NearbyCommerceLink(
              peer,
              'terminal',
              module,
              journal,
              (req, auth) => relay.handle(req, auth),
            ),
          );
        } else if (data.kind === 'data' && typeof data.clear === 'string') {
          links.get(data.link)?.receive(data.clear);
        } else if (data.kind === 'closed') {
          links.get(data.link)?.close();
          links.delete(data.link);
          relay.retire(data.link);
        }
      } catch {
        // A malformed event cannot open a customer session.
      }
    });
    let open = true;
    (async () => {
      if (Platform.OS === 'android' && Number(Platform.Version) >= 31) {
        const required = [
          PermissionsAndroid.PERMISSIONS.BLUETOOTH_ADVERTISE,
          PermissionsAndroid.PERMISSIONS.BLUETOOTH_CONNECT,
        ];
        const grants = await PermissionsAndroid.requestMultiple(required);
        if (
          required.some(
            permission =>
              grants[permission] !== PermissionsAndroid.RESULTS.GRANTED,
          )
        )
          throw new Error('NEARBY_PERMISSION');
      }
      await module.configure(nearbyProfile);
      if (open) await module.start();
    })().catch(() => {
      if (open)
        setMessage(
          '근거리 주문 연결을 시작하지 못했습니다. 단말의 등록 정보를 확인하세요.',
        );
    });
    return () => {
      open = false;
      relay.close();
      subscription.remove();
      module.stop().catch(() => {});
      for (const link of links.values()) link.close();
      links.clear();
      relayRef.current = undefined;
    };
  }, [setup, client, inbox, nearbyProfile, policy, paymentEnrollment]);

  function work(run: () => Promise<void>, failure: string) {
    if (latest.current) return;
    const flight = Symbol();
    latest.current = flight;
    setMessage('');
    setBusy(true);
    const session = active.current?.session;
    if (session) keepPublicActive();
    run()
      .catch(() => {
        if (latest.current === flight && session === active.current?.session)
          setMessage(failure);
      })
      .finally(() => {
        if (latest.current !== flight) return;
        latest.current = undefined;
        setBusy(false);
      });
  }

  async function enroll() {
    if (!Http || setup !== 'configure') throw new Error('COMMERCE_ENROLLMENT');
    commerceProfile(enrollment, role, pin.trim());
    const payment = paymentProfile(enrollment);
    if (payment && presentationFingerprint(payment) !== proofPin.trim())
      throw new Error('PRESENTATION_TRUST');
    await Http.configure(role, enrollment);
    setEnrollment('');
    setPin('');
    setPublicKey('');
    await load();
  }

  async function publicMenuStatus() {
    if (!client || !registration || !Http || staffView !== 'public')
      throw new Error('TERMINAL_REQUIRED');
    const transport=Http;
    const generation=staffEpoch.current;
    const scope=menuScope.current;
    let observed:TerminalSettings|undefined;
    const enrolled=async()=>{
      if (!scope || client.profile.expiresAt<=now()) return false;
      const [raw,identity]=await Promise.all([
        transport.loadProfile('terminal'), transport.publicIdentity('terminal'),
      ]);
      return pickupHash(raw)===registration.digest && !!identity &&
        pickupHash(JSON.stringify([registration.digest,identity]))===scope;
    };
    try {
      if (!(await enrolled())) throw new Error('TERMINAL_PROFILE_CHANGED');
      observed=await client.terminalState();
      if (generation!==staffEpoch.current) return undefined;
      if (observed.mode!=='menu' || observed.transition?.state==='waiting') {
        await forgetMenuCache();
        if (generation!==staffEpoch.current) return undefined;
        setOfflineMenu(undefined);
        return {current:observed, menu:[]};
      }
      const {catalog,shop}=await client.publishedMenu();
      const confirmed=await client.terminalState();
      if (generation!==staffEpoch.current) return undefined;
      if (confirmed.mode!=='menu' || confirmed.revision!==observed.revision ||
          confirmed.transition?.state==='waiting' || !(await enrolled()))
        throw new Error('TERMINAL_MODE_CHANGED');
      const candidate=shop && encodeCachedMenu({scope:scope!,modeRevision:confirmed.revision,shop,checkedAt:now(),rows:catalog});
      if (candidate) {
        await transport.saveJournal(menuCacheJournal,candidate).catch(()=>forgetMenuCache());
      } else {
        await forgetMenuCache();
      }
      if (generation!==staffEpoch.current) return undefined;
      setOfflineMenu(undefined);
      return {current:confirmed,menu:catalog,shop};
    } catch (error) {
      const available=networkUnavailable(error) && await enrolled().catch(()=>false);
      const raw=available ? await transport.loadJournal(menuCacheJournal).catch(()=> '') : '';
      const cached=scope && raw ? readCachedMenu(raw,scope,client.profile.branchRef,now()) : undefined;
      if (generation!==staffEpoch.current) return undefined;
      if (cached && (!observed || observed.revision===cached.modeRevision)) {
        clearPublic();
        setEntrance(undefined);
        setMode(undefined);
        setMenuBranchLabel(undefined);
        setStock([]);
        setOfflineMenu(cached);
        return undefined;
      }
      await forgetMenuCache();
      if (generation===staffEpoch.current) {
        setOfflineMenu(undefined);
        if (!active.current?.key) {
          clearPublic();
          setMessage('인증된 메뉴 연결을 확인할 수 없습니다.');
        }
        throw error;
      }
      return undefined;
    }
  }

  async function retryMenuConnection() {
    const online=await publicMenuStatus();
    if (online) {
      setMode(online.current);
      setMenuBranchLabel(online.shop?.branchLabel);
      setMessage(online.current.mode==='menu' && !online.current.transition ?
        '현재 메뉴 동기화 완료' : '현재 단말 모드를 확인했습니다.');
    }
  }

  async function newCustomer() {
    if (!client || !inbox || staffView !== 'public')
      throw new Error('TERMINAL_REQUIRED');
    const generation = staffEpoch.current;
    void inbox.flushPublicCancellations((session, key) => client.cancelPublicOrder(session, key));
    await inbox.reclaimPublicOrders(sale => client.status(sale));
    const online=await publicMenuStatus();
    if (!online || generation!==staffEpoch.current) return;
    const {current,menu,shop}=online;
    if (current.transition?.state === 'waiting') {
      const applied = await applyTerminalVersion(current.transition);
      if (generation !== staffEpoch.current) return;
      setMode(applied);
      setMessage(
        applied.transition?.state === 'active'
          ? `결제 전용 v${applied.revision}이 적용됐습니다.`
          : '지점 인계와 단말 적용 확인이 끝날 때까지 신규 주문은 중지됩니다.',
      );
      return;
    }
    const handle = await id();
    if (
      generation !== staffEpoch.current ||
      AppState.currentState === 'background'
    )
      return;
    setEntrance(undefined);
    inbox.openPublic(handle);
    setMode(current);
    setMenuBranchLabel(shop?.branchLabel);
    setStock(menu);
    show({ session: handle });
    keepPublicActive();
  }

  function applyCartReview(
    review: CommerceCartReview,
    catalog: CommercePrice[],
  ) {
    setStock(catalog);
    setChoice(review.selection);
    setCartReview(review);
  }

  function putChoice(row: CommercePrice, quantity: number, add = false) {
    if (quantity < 1 || quantity > commerceLineCapacity(row, choice)) return;
    const next = { row, quantity };
    const previous = Object.values(choice).find(line => line.row.sku === row.sku);
    if (row.skuAvailable === undefined && previous && previous.row.optionKey !== row.optionKey) {
      setCartReplacement({ previous, next });
      return;
    }
    const key = commerceLineKey(row);
    setChoice(old => {
      const desired = add ? (old[key]?.quantity ?? 0) + 1 : quantity;
      return desired <= commerceLineCapacity(row, old)
        ? { ...old, [key]: { row, quantity: desired } }
        : old;
    });
    setMenuDetail(undefined);
  }

  function reduceChoice(row: CommercePrice) {
    keepPublicActive();
    const key = commerceLineKey(row);
    setChoice(old => {
      const existing = old[key];
      if (!existing) return old;
      const next = { ...old };
      if (existing.quantity === 1) delete next[key];
      else next[key] = { ...existing, quantity: existing.quantity - 1 };
      return next;
    });
  }

  function openMenuDetail(rows: CommercePrice[], required: boolean) {
    const previous = rows[0].skuAvailable === undefined
      ? rows.find(row => choice[commerceLineKey(row)])
      : undefined;
    const defaultOption = required ? undefined :
      (rows.length === 1 ? rows[0] : rows.find(row => row.optionKey === 'standard'));
    const selected = previous ?? defaultOption;
    keepPublicActive();
    setMenuDetail({
      sku: rows[0].sku,
      option: selected?.optionKey,
      quantity: selected ? choice[commerceLineKey(selected)]?.quantity ?? 1 : 1,
    });
  }

  function missingOptionMessage(group?: string) {
    if (menuLocale === 'en') return group ? `Choose an option for ${group}` : 'Choose a required option';
    if (!group) return '필수 옵션을 선택해 주세요';
    if (!/[가-힣]$/.test(group)) return `${group} 옵션을 선택해 주세요`;
    const particle = (group.charCodeAt(group.length - 1) - 0xac00) % 28 === 0 ? '를' : '을';
    return `${group}${particle} 선택해 주세요`;
  }

  function addMenuDetail(detail: MenuDetail, selected: CommercePrice | undefined, group?: string) {
    if (!selected) {
      setMenuDetail({ ...detail, error: 'option' });
      AccessibilityInfo.announceForAccessibility(missingOptionMessage(group));
      requestAnimationFrame(() => {
        const heading = optionHeading.current;
        if (heading) AccessibilityInfo.sendAccessibilityEvent(heading, 'focus');
      });
      return;
    }
    if (detail.quantity > commerceLineCapacity(selected, choice)) {
      setMenuDetail({ ...detail, error: 'stock' });
      return;
    }
    putChoice(selected, detail.quantity);
  }

  async function reserve() {
    const current = active.current;
    if (!current || current.key || mode?.mode !== 'menu' || !client || !inbox)
      throw new Error('PUBLIC_SESSION_CLOSED');
    const online=await publicMenuStatus();
    if (!online || active.current !== current) return;
    if (online.current.mode!=='menu' || online.current.revision!==mode.revision ||
        online.current.transition?.state==='waiting') {
      clearPublic();
      setMode(online.current);
      setMenuBranchLabel(undefined);
      setMessage('단말 모드가 변경되었습니다. 다시 시작하세요.');
      return;
    }
    const latestStock=online.menu;
    setMenuBranchLabel(online.shop?.branchLabel);
    const review = reviewCommerceCart(choice, latestStock);
    if (review.priceChanges.length || review.shortages.length) {
      applyCartReview(review, latestStock);
      return;
    }
    if (commerceNeedsOptionSupport(review.selection)) {
      applyCartReview(review, latestStock);
      setMessage(menuLocale === 'ko' ? '이 매장에서는 품목당 옵션을 하나씩 담을 수 있습니다. 담은 옵션을 확인하세요.'
        : 'This store accepts one option per item. Review the selected options.');
      return;
    }
    const selected = Object.values(review.selection);
    const first = selected[0];
    if (
      !first ||
      selected.some(
        ({ row }) =>
          row.asset !== first.row.asset || row.version !== first.row.version,
      )
    ) {
      applyCartReview(review, latestStock);
      setMessage('한 주문에는 같은 결제 통화의 품목을 선택하세요.');
      return;
    }
    const key = await id();
    if (active.current !== current) return;
    const cart: CommerceCart = {
      origin: mode.mode,
      originRef: current.session,
      menuVersion: first.row.version,
      items: selected.map(({ row, quantity }) => ({
        sku: row.sku,
        option_key: row.optionKey,
        quantity,
      })),
    };
    await inbox.stage(current.session, cart, first.row.asset, key);
    if (active.current !== current) return;
    setChoice(review.selection);
    setCartReview(undefined);
    show({ ...current, key });
    await reconcile(current.session, key, review.selection);
  }

  async function reconcile(session: string, key: string, previous = choice) {
    if (!client || !inbox) return;
    let sale: string;
    try {
      sale = await inbox.dispatch(session, (cart, asset, requestKey) =>
        client.reserve(cart, asset, requestKey),
      );
    } catch (error) {
      if (!commerceReservationRejected(error)) throw error;
      if (active.current?.session !== session || active.current.connectionEnded) return;
      setCartReview(reviewCommerceCart(previous, stock));
      show({ session });
      const refreshed = await client.publishedCatalog();
      if (active.current?.session !== session || active.current.connectionEnded) return;
      applyCartReview(reviewCommerceCart(previous, refreshed), refreshed);
      return;
    }
    if (active.current?.session !== session || active.current.connectionEnded) return;
    const original = await client.status(sale);
    const candidate = inbox
      .recovery()
      .find(row => row.requestKey === key)?.comparison;
    const result = candidate
      ? await reconcileComparison(key, original, candidate)
      : { order: original, selectedCode: undefined, pending: false };
    await clearFinishedPayment(result.order);
    publishOrder(
      session,
      key,
      result.order,
      undefined,
      result.selectedCode,
      result.pending ? {state: 'unknown', checkedAt: now()} : undefined,
    );
  }

  async function reconcilePOS(handoff: string, outcome?: POSSelection) {
    const slot = active.current;
    const transport = Http;
    if (!slot || !client || !transport || mode?.mode !== 'pos')
      throw new Error('PUBLIC_SESSION_CLOSED');
    try {
      const result = await readPOSOrder(
        raw => transport.exchange('terminal', raw),
        client.profile.branchRef,
        handoff,
      );
      if (active.current !== slot) return;
      if (policy && policy.asset !== result.order.asset)
        throw new Error('POS_ASSET_UNSUPPORTED');
      const inFlight = posFlights.current.get(result.order.id);
      const selection = outcome?.state === 'conflict' ||
        (outcome?.state === 'unknown' && !result.payerBound && result.order.state === 'preview')
        ? outcome
        : inFlight && result.order.state === 'preview'
        ? {state: 'locking' as const, code: inFlight}
        : undefined;
      publishOrder(slot.session, handoff, result.order, result,
        !selection ? result.selectedCode : undefined, undefined, selection);
    } catch (error) {
      if (active.current !== slot) return;
      if (error instanceof Error && error.message === 'POS_ASSET_UNSUPPORTED') {
        setMessage(
          'TestUSDC 결제만 가능합니다. POS의 자산과 네트워크를 확인하세요.',
        );
        return;
      }
      throw error;
    }
  }

  function publishOrder(
    session: string,
    key: string,
    latestSale: CommerceOrder,
    posOrder?: POSOrder,
    selectedCode?: string,
    pairing?: PublicScreen['pairing'],
    posSelection?: POSSelection,
  ) {
    if (!client || active.current?.session !== session || active.current.connectionEnded) return;
    const expiresAt = Math.min(
      now() + 40,
      Math.floor(Date.parse(latestSale.expiresAt) / 1000),
    );
    const previous =
      active.current.sale?.id === latestSale.id ? active.current : undefined;
    const finished = latestSale.state === 'paid' || latestSale.state === 'closed';
    const code = finished ? undefined : (
      selectedCode ??
      (!posOrder && latestSale.menuCheckout?.customerRef ===
      previous?.sale?.menuCheckout?.customerRef
        ? previous?.selectedCode
        : undefined)
    );
    const payerBound =
      posOrder?.payerBound ?? latestSale.menuCheckout?.payerBound;
    const canPair =
      latestSale.state === 'preview' && !payerBound && !code && !pairing && !posSelection;
    const locator =
      canPair && expiresAt > now()
        ? encodeOrderLocator(
            {
              branchRef: client.profile.branchRef,
              orderRef: latestSale.id,
              expiresAt,
            },
            now(),
          )
        : undefined;
    if (!locator) relayRef.current?.clearPublic();
    setOnlinePairs([]);
    show({
      session,
      key,
      sale: latestSale,
      locator,
      pos: posOrder,
      selectedCode: code,
      pairing: finished ? undefined : pairing,
      posSelection: finished ? undefined : posSelection,
    });
    keepPublicActive();
    if (qrExpiry.current) clearTimeout(qrExpiry.current);
    if (locator)
      qrExpiry.current = setTimeout(() => {
        const latestOrder = active.current;
        if (latestOrder?.session !== session || latestOrder.locator !== locator)
          return;
        if (!latestOrder.pos) {
          retirePublicOrder();
          latest.current = undefined;
          setBusy(false);
          show({ session, connectionEnded: true });
          return;
        }
        relayRef.current?.clearPublic();
        setOnlinePairs([]);
        show({ ...latestOrder, locator: undefined });
        setMessage(
          '주문 QR이 만료되었습니다. 현재 주문을 다시 확인해 새 코드를 표시하세요.',
        );
      }, Math.max(0, (expiresAt - now()) * 1000));
  }

  async function pollPairs() {
    if (!client || !active.current?.sale || !active.current.locator)
      throw new Error('PUBLIC_SESSION_CLOSED');
    const visible = active.current;
    const pairs = await client.pendingComparisons(visible.sale!.id);
    if (active.current !== visible) return;
    if (
      visible.pos !== undefined &&
      new Set(pairs.map(pair => pair.code)).size !== pairs.length
    ) {
      setOnlinePairs([]);
      throw new Error('POS_COMPARISON_AMBIGUOUS');
    }
    setOnlinePairs(pairs);
  }

  async function rememberPayment(value?: PendingPayment) {
    await Http!.saveJournal(paymentJournal, value ? JSON.stringify(value) : '');
    setPendingPayment(value);
  }

  async function clearFinishedPayment(order: CommerceOrder) {
    if (
      pendingPayment?.order.id === order.id &&
      (order.state === 'paid' || order.state === 'closed')
    )
      await rememberPayment();
  }

  async function reconcileComparison(
    key: string,
    original: CommerceOrder,
    candidate: CheckoutComparison,
    submitted?: Awaited<ReturnType<CommerceClient['confirmComparison']>>,
  ) {
    if (!client || !inbox) throw new Error('PUBLIC_SESSION_CLOSED');
    let order = original;
    let claim = submitted;
    if (
      !claim &&
      order.state === 'preview' &&
      !order.menuCheckout?.payerBound &&
      candidate.expiresAt > now()
    ) {
      claim = await client.confirmComparison(
        order.id,
        candidate,
        comparisonKey(order.id, candidate),
      );
      order = await client.status(order.id);
    }
    const selected = order.menuCheckout
      ? order.menuCheckout.customerRef === candidate.customerRef
      : !!claim;
    const pending =
      !selected &&
      !order.menuCheckout?.payerBound &&
      order.state !== 'paid' &&
      order.state !== 'closed';
    if (!pending) {
      await inbox.clearComparison(key, order.id);
      setUnresolvedComparisons(inbox.recovery().some(row => row.comparison));
    }
    return {
      order,
      selectedCode: selected ? candidate.code : undefined,
      pending,
    };
  }

  async function recoverComparisons() {
    if (!client || !inbox) throw new Error('PUBLIC_SESSION_CLOSED');
    for (const { requestKey, saleRef, comparison } of inbox.recovery()) {
      if (!saleRef || !comparison) continue;
      const order = await client.status(saleRef);
      await reconcileComparison(requestKey, order, comparison);
      await clearFinishedPayment(order);
    }
  }

  function comparisonKey(sale: string, candidate: CheckoutComparison) {
    return commerceRequestId(
      sha256(utf8Encode(sale + candidate.customerRef!)).subarray(0, 16),
    );
  }

  async function selectPOSComparison(candidate: CheckoutComparison) {
    const visible = active.current;
    const sale = visible?.sale;
    const original = visible?.pos;
    if (!client || !visible?.locator || !original || !sale || sale.state !== 'preview' ||
        posFlights.current.has(sale.id) || candidate.expiresAt <= now()) return;
    const key = comparisonKey(sale.id, candidate);
    const current = () => active.current?.session === visible.session && active.current.sale?.id === sale.id;
    const selection = {state: 'locking' as const, code: candidate.code};
    posFlights.current.set(sale.id, candidate.code);
    setMessage('');
    publishOrder(visible.session, original.handoff, sale, original, undefined, undefined, selection);
    let outcome: POSSelection | undefined;
    let confirmed = false;
    try {
      const claim = await client.confirmComparison(sale.id, candidate, key)
        .catch(error => {
          // Retry a lost response with the same candidate and original intent.
          if (error instanceof Error && ['CHECKOUT_CONFLICT', 'REPLAY_CONFLICT', 'NOT_AUTHORIZED', 'EXPIRED'].includes(error.message))
            throw error;
          return client.confirmComparison(sale.id, candidate, key);
        });
      confirmed = true;
      // A POS customer uses their own approval path. Terminal board enrollment
      // and a different order's pending board payment do not gate issuance.
      await client.quote(sale.id, claim.claimDigest, quoteOrderRevision(sale), key);
    } catch (error) {
      outcome = !confirmed && error instanceof Error && error.message === 'CHECKOUT_CONFLICT'
        ? {state: 'conflict'} : {state: 'unknown'};
    } finally {
      posFlights.current.delete(sale.id);
    }
    if (!current()) return;
    try {
      await reconcilePOS(original.handoff, outcome);
    } catch {
      if (current()) show({...active.current!, selectedCode: undefined, locator: undefined,
        posSelection: outcome ?? {state: 'unknown'}});
    }
  }

  async function acceptComparison(candidate: CheckoutComparison) {
    if (
      !client ||
      !inbox ||
      !active.current?.sale ||
      active.current.pos ||
      !active.current.locator ||
      candidate.expiresAt <= now()
    )
      throw new Error('PUBLIC_SESSION_CLOSED');
    const visible = active.current;
    const sale = visible.sale!;
    const key = comparisonKey(sale.id, candidate);
    if (!visible.key) throw new Error('PUBLIC_SESSION_CLOSED');
    await inbox.stageComparison(visible.session, sale.id, candidate);
    setUnresolvedComparisons(true);
    if (active.current !== visible) return;
    publishOrder(
      visible.session,
      visible.key,
      sale,
      undefined,
      undefined,
      {state: 'binding', code: candidate.code},
    );
    let claim: Awaited<ReturnType<CommerceClient['confirmComparison']>>;
    try {
      claim = await client.confirmComparison(sale.id, candidate, key);
      if (active.current?.session !== visible.session) return;
      setOnlinePairs([]);
      const checked = await client.status(sale.id);
      const result = await reconcileComparison(
        visible.key!,
        checked,
        candidate,
        claim,
      );
      await clearFinishedPayment(result.order);
      publishOrder(
        visible.session,
        visible.key!,
        result.order,
        undefined,
        result.selectedCode,
        result.pending ? {state: 'unknown', checkedAt: now()} : undefined,
      );
      if (
        !result.selectedCode ||
        result.order.state === 'paid' ||
        result.order.state === 'closed'
      )
        return;
    } catch (error) {
      if (active.current?.session === visible.session &&
          active.current.pairing?.state === 'binding') {
        show({...active.current, pairing: {state: 'unknown', checkedAt: now()}});
      }
      throw error;
    }
    if (active.current?.session !== visible.session) return;
    if (!policy || !paymentEnrollment || pendingPayment) {
      if (active.current?.session === visible.session)
        setMessage('고객 휴대전화 연결을 확인했습니다.');
      return;
    }
    const quote = await client.quote(
      sale.id,
      claim.claimDigest,
      quoteOrderRevision(sale),
      key,
    );
    const checked = await client.status(sale.id);
    const saved = inbox
      ?.recovery()
      .find(
        record =>
          record.requestKey === visible.key && record.saleRef === sale.id,
      );
    const origin = saved?.cart.originRef;
    if (!origin) throw new Error('PAYMENT_SCOPE');
    const scope = {
      branch: client.profile.branchRef,
      sale: sale.id,
      customer: candidate.customerRef!,
      origin,
      claim: claim.claimDigest,
    };
    preparePresentedPayment(quote, checked, policy, now(), scope);
    if (active.current?.session !== visible.session) return;
    await rememberPayment({
      branch: client.profile.branchRef,
      terminal: client.profile.subjectRef,
      order: checked,
      scope,
      quote,
      request: bytesToHex(fromBase64(await Http!.randomBytes(16)), false),
      key: await id(),
    });
    publishOrder(visible.session, visible.key!, checked);
    if (active.current?.session === visible.session)
      setMessage(
        '고객 휴대전화 연결을 확인했습니다. 원견적을 준비했습니다.',
      );
  }

  async function scanBoards() {
    const visible = active.current;
    if (
      !Nearby ||
      !client ||
      !policy ||
      visible?.sale?.state !== 'issued' ||
      !pendingPayment ||
      pendingPayment.order.id !== active.current?.sale?.id ||
      !paymentEnrollment
    )
      throw new Error('PAYMENT_SCOPE');
    const current = await client.status(pendingPayment.order.id);
    if (active.current !== visible) return;
    if (current.state !== 'issued') {
      setBoardOptions([]);
      show({ ...visible, sale: current, locator: undefined });
      await clearFinishedPayment(current);
      return;
    }
    if (Platform.OS === 'android' && Number(Platform.Version) >= 31) {
      const needed = [
        PermissionsAndroid.PERMISSIONS.BLUETOOTH_SCAN,
        PermissionsAndroid.PERMISSIONS.BLUETOOTH_CONNECT,
      ];
      const grants = await PermissionsAndroid.requestMultiple(needed);
      if (needed.some(p => grants[p] !== PermissionsAndroid.RESULTS.GRANTED))
        throw new Error('PAYMENT_PERMISSION');
    }
    if (active.current !== visible) return;
    if (abortRef.current) throw new Error('PAYMENT_SCOPE');
    const abort=new AbortController();
    abortRef.current=abort;
    setBoardOptions([]);
    try {
      const items=await findPaymentBoards(Nearby,paymentEnrollment);
      if (!abort.signal.aborted && active.current===visible) setBoardOptions(items);
    } finally {
      if (abortRef.current===abort) abortRef.current=undefined;
    }
  }

  async function approveAtBoard(handle: string) {
    if (
      !Nearby ||
      !policy ||
      !client ||
      !pendingPayment ||
      pendingPayment.signature ||
      active.current?.sale?.state !== 'issued' ||
      pendingPayment.order.id !== active.current?.sale?.id
    )
      throw new Error('PAYMENT_SCOPE');
    const visible = active.current;
    const exact = await client.status(pendingPayment.order.id);
    if (active.current !== visible) throw new Error('PAYMENT_SCOPE');
    const payment = preparePresentedPayment(
      pendingPayment.quote,
      exact,
      policy,
      now(),
      pendingPayment.scope,
    );
    if (abortRef.current) throw new Error('PAYMENT_SCOPE');
    const abort = new AbortController();
    abortRef.current = abort;
    try {
      const signature = await boardPayment(
        Nearby,
        handle,
        pendingPayment.branch,
        payment,
        hexToBytes(pendingPayment.request),
        () => {
          if (active.current === visible)
            setMessage(
              '서명된 원견적을 보드에 보내는 중입니다. 고객은 휴대전화에서 주문과 금액을 확인합니다.',
            );
        },
        abort.signal,
      );
      await rememberPayment({ ...pendingPayment, signature });
      if (active.current === visible)
        setMessage('보드 승인 확보. 같은 주문의 지급 결과를 조회하세요.');
    } catch (error) {
      if (!(error instanceof Error) || error.message !== 'PAYMENT_REFUSED')
        throw error;
      await rememberPayment();
      if (active.current === visible)
        setMessage(
          '보드에서 결제를 거절했습니다. 지급 요청을 보내지 않았습니다.',
        );
    } finally {
      if (abortRef.current === abort) abortRef.current = undefined;
      setBoardOptions([]);
    }
  }

  async function resumeApproval() {
    if (!client || !policy || !pendingPayment) throw new Error('PAYMENT_SCOPE');
    const visit = active.current;
    const current = await client.status(pendingPayment.order.id);
    if (
      pendingPayment.signature &&
      current.state === 'issued' &&
      pendingPayment.quote.expiresAt > now()
    ) {
      const payment = preparePresentedPayment(
        pendingPayment.quote,
        current,
        policy,
        now(),
        pendingPayment.scope,
      );
      const approval = verifyCommerceApproval(
        payment,
        hexToBytes(pendingPayment.signature),
        now(),
      );
      await client
        .forward(
          current.id,
          payment.digest,
          payment.approvalDigest,
          approval,
          pendingPayment.key,
        )
        .catch(() => {});
    }
    const checked = await client.status(current.id);
    if (checked.state === 'paid' || checked.state === 'closed')
      await rememberPayment();
    if (visit && active.current === visit && visit.sale?.id === checked.id) {
      show({ ...visit, sale: checked, locator: undefined });
      setMessage(
        checked.state === 'paid'
          ? '서버에서 지급을 확인했습니다.'
          : checked.state === 'closed'
          ? '주문 종료를 확인했습니다.'
          : '지급 결과를 확인 중입니다. 같은 주문을 다시 조회하세요.',
      );
    } else {
      setMessage(
        checked.state === 'paid' || checked.state === 'closed'
          ? '보류 결과가 정리되었습니다.'
          : '보류 결과를 확인 중입니다.',
      );
    }
  }

  async function applyTerminalVersion(transition: TerminalTransition) {
    if (!client || client.profile.role !== 'terminal')
      throw new Error('TERMINAL_REQUIRED');
    const applied = JSON.stringify({
      branch: client.profile.branchRef,
      terminal: client.profile.subjectRef,
      request: transition.requestRef,
      mode: transition.targetMode,
      revision: transition.targetRevision,
    });
    if ((await Http!.loadJournal(modeAppliedJournal)) !== applied) {
      await Http!.saveJournal(modeAppliedJournal, applied);
      if ((await Http!.loadJournal(modeAppliedJournal)) !== applied)
        throw new Error('MODE_APPLY_JOURNAL');
    }
    return client.confirmModeTransition(client.profile.subjectRef, transition);
  }

  async function clearModeRequest() {
    await Http!.saveJournal(modeRequestJournal, '');
    setPendingMode(undefined);
  }

  async function retryMode(request: ModeRequest) {
    if (
      !client ||
      !staff?.permissions.includes('terminal_settings') ||
      request.terminal !== target.trim()
    )
      throw new Error('STAFF_REQUIRED');
    const generation = staffEpoch.current;
    const value = await client.changeMode(
      request.terminal,
      request.mode,
      request.revision,
      request.request,
    );
    if (generation !== staffEpoch.current) return;
    if (request.mode === 'pos' && !value.transition)
      throw new Error('TRANSITION_REQUIRED');
    setMode(value);
    setOrderPage(undefined);
    if (value.transition?.state !== 'waiting') await clearModeRequest();
    setMessage(
      value.transition?.state === 'waiting'
        ? '같은 전환 요청을 확인했습니다. 인계와 단말 적용 확인을 기다립니다.'
        : '서버에서 단말 모드를 확인했습니다.',
    );
  }

  async function confirmModeRequest() {
    if (
      !client ||
      !mode ||
      !modeReview ||
      pendingMode ||
      !staff?.permissions.includes('terminal_settings') ||
      (modeReview === 'pos' && mode.orderCount === undefined)
    )
      throw new Error('MODE_REVIEW_REQUIRED');
    const generation = staffEpoch.current;
    const request: ModeRequest = {
      branch: client.profile.branchRef,
      operator: client.profile.subjectRef,
      terminal: target.trim(),
      request: await id(),
      revision: mode.revision,
      mode: modeReview,
    };
    if (generation !== staffEpoch.current) return;
    await Http!.saveJournal(modeRequestJournal, JSON.stringify(request));
    if (generation !== staffEpoch.current) return;
    setPendingMode(request);
    setModeReview(undefined);
    await retryMode(request);
  }

  async function confirmHandoff(transition: TerminalTransition) {
    if (!client || !staff?.permissions.includes('terminal_settings'))
      throw new Error('STAFF_REQUIRED');
    const generation = staffEpoch.current;
    const value = await client.confirmModeTransition(target.trim(), transition);
    if (generation !== staffEpoch.current) return;
    setMode(value);
    setOrderPage(undefined);
    if (
      value.transition?.state === 'active' &&
      pendingMode?.request === transition.requestRef
    )
      await clearModeRequest();
  }

  async function refreshTerminalSettings() {
    if (!client || !staff?.permissions.includes('terminal_settings')) return;
    const generation = staffEpoch.current;
    const value = await client.terminalState(target.trim());
    if (generation !== staffEpoch.current) return;
    setMode(value);
    setOrderPage(undefined);
  }

  async function loadNextOrders() {
    const cursor = (orderPage ?? mode?.orderRefs)?.at(-1);
    if (
      !client ||
      !mode ||
      !cursor ||
      !staff?.permissions.includes('terminal_settings')
    )
      return;
    const generation = staffEpoch.current;
    const refs = await client.terminalOrders(target.trim(), mode, cursor);
    if (generation === staffEpoch.current) setOrderPage(refs);
  }

  function modeOrders() {
    const refs = orderPage ?? mode?.orderRefs;
    return (
      <>
        {refs?.map(ref => (
          <Label key={ref}>{ref}</Label>
        ))}
        {refs?.length === 20 &&
          (orderPage !== undefined ||
            (mode?.orderCount ?? 0) > refs.length) && (
            <Button
              label="다음 인계 주문"
              disabled={busy}
              run={() =>
                work(
                  loadNextOrders,
                  '같은 단말의 인계 주문을 확인할 수 없습니다.',
                )
              }
            />
          )}
        {orderPage !== undefined && (
          <Button
            label="인계 목록 처음"
            disabled={busy}
            run={() => setOrderPage(undefined)}
          />
        )}
      </>
    );
  }

  async function openRefundConsole() {
    if (!client) return;
    const generation = staffEpoch.current;
    const access = await client.staffAccess();
    if (generation !== staffEpoch.current) return;
    if (!access.permissions.includes('refund_console') || !access.console)
      throw new Error('REFUND_CONSOLE');
    await Linking.openURL(access.console);
  }

  function showBasket(open: boolean) {
    keepPublicActive();
    publicScroll.current?.scrollTo({ y: 0, animated: false });
    setBasketOpen(open);
    if (open) requestAnimationFrame(() => {
      if (basketHeading.current) AccessibilityInfo.sendAccessibilityEvent(basketHeading.current, 'focus');
    });
  }

  function openPublicGuide(guide?: 'phone' | 'cancel') {
    keepPublicActive();
    publicScroll.current?.scrollTo({ y: 0, animated: false });
    setPublicGuide(guide);
  }

  const basketVisible = setup === 'terminal' && staffView === 'public' && screen &&
    !screen.key && !screen.connectionEnded && !offlineMenu && mode?.mode === 'menu';
  const basketUnits = Object.values(choice).reduce((total, item) => total + item.quantity, 0);
  const basketTotals = commerceCartTotals(choice);
  const menuGroups = new Map<string, CommercePrice[]>();
  for (const row of stock) {
    const group = menuGroups.get(row.sku);
    if (group) group.push(row);
    else menuGroups.set(row.sku, [row]);
  }

  return (
    <SafeAreaView style={s.root}>
      <ScrollView
        ref={publicScroll}
        style={s.viewport}
        contentContainerStyle={s.scroll}
        keyboardShouldPersistTaps="handled"
        keyboardDismissMode="on-drag"
        automaticallyAdjustKeyboardInsets
      >
        <Text accessibilityRole="header" style={s.title}>
          {staffView === 'public' ? '매장 주문' : '직원 인증'}
        </Text>
        {!!message && (
          <Text accessibilityLiveRegion="polite" style={s.notice}>
            {message}
          </Text>
        )}
        {setup === 'loading' && <Label>매장 등록 상태 확인 중</Label>}
        {setup === 'blocked' && (
          <Label>
            이 단말의 매장 등록을 확인할 수 없습니다. 설치 담당자에게
            문의하세요.
          </Label>
        )}
        {setup === 'configure' && (
          <>
            <Label>
              등록 승인 담당자에게 이 단말의 공개키를 전달하고 발급서를
              받으세요.
            </Label>
            <Button
              label={
                role === 'terminal'
                  ? '등록 권한 · 공용 단말'
                  : role === 'operator'
                  ? '등록 권한 · 직원 전용'
                  : '등록 권한 · 재무 전용'
              }
              run={() => {
                setRole(slots[(slots.indexOf(role) + 1) % slots.length]);
                setPublicKey('');
              }}
              disabled={busy}
            />
            <Button
              label="등록 공개키 확인"
              run={() =>
                work(
                  async () => setPublicKey(await Http!.publicIdentity(role)),
                  '보호된 공개키를 읽을 수 없습니다.',
                )
              }
              disabled={busy}
            />
            {role === 'terminal' && Nearby && (
              <Button
                label="근거리 연결 공개키 확인"
                run={() =>
                  work(
                    async () => setNearbyKey(await Nearby!.publicIdentity()),
                    '근거리 연결 공개키를 읽을 수 없습니다.',
                  )
                }
                disabled={busy}
              />
            )}
            {role === 'terminal' && Nearby && (
              <Button
                label="보드 결제 연결 공개키 확인"
                disabled={busy}
                run={() =>
                  work(
                    async () =>
                      setPaymentKey(await Nearby!.paymentPublicIdentity()),
                    '보드 연결 공개키를 읽을 수 없습니다.',
                  )
                }
              />
            )}
            {!!paymentKey && (
              <Text selectable style={s.mono}>
                {paymentKey}
              </Text>
            )}
            {!!nearbyKey && (
              <Text selectable style={s.mono}>
                {nearbyKey}
              </Text>
            )}
            {!!publicKey && (
              <Text selectable style={s.mono}>
                {publicKey}
              </Text>
            )}
            <TextInput
              accessibilityLabel="매장 발급서"
              placeholder="매장 발급서"
              maxLength={16384}
              multiline
              autoCapitalize="none"
              style={[s.input, s.registration]}
              returnKeyType="next"
              submitBehavior="submit"
              onSubmitEditing={() => pickupServerPinInput.current?.focus()}
              value={enrollment}
              onChangeText={setEnrollment}
            />
            <TextInput
              ref={pickupServerPinInput}
              accessibilityLabel="독립 확인한 서버 공개키 지문"
              placeholder="독립 확인한 서버 공개키 지문"
              maxLength={64}
              autoCapitalize="none"
              style={s.input}
              returnKeyType="next"
              onSubmitEditing={() => pickupProofPinInput.current?.focus()}
              value={pin}
              onChangeText={setPin}
            />
            <TextInput
              ref={pickupProofPinInput}
              accessibilityLabel="별도 발급한 비공개 주문 서명 지문"
              placeholder="별도 발급한 비공개 주문 서명 지문"
              style={s.input}
              maxLength={64}
              autoCapitalize="none"
              value={proofPin}
              onChangeText={setProofPin}
            />
            <Button
              label="등록서 확인"
              run={() => work(enroll, '등록서와 독립 설치 기록을 확인하세요.')}
              disabled={busy}
            />
            {legacy && (
              <Button label="기존 대여 결제" run={legacy} disabled={busy} />
            )}
          </>
        )}
        {(setup === 'operator' || setup === 'finance') &&
          staffView === 'public' && (
            <>
              <Label>고객 주문은 등록된 공용 단말에서 시작하세요.</Label>
              <Button label="직원 인증" run={() => setStaffView('login')} />
            </>
          )}
        {setup === 'terminal' &&
          pickupStaff &&
          !screen &&
          !pendingPayment &&
          staffView === 'public' && (
            <Button
              label="직원 픽업 접수"
              disabled={busy}
              run={() => closeStaff('login')}
            />
          )}
        {(staffView === 'login' ||
          staffView === 'checking' ||
          staffView === 'denied') && (
          <>
            {staffView === 'login' && (
              <>
                <Label>
                  등록된 인증서로 소속 지점과 운영 권한을 확인합니다.
                </Label>
                <Button label="직원 권한 확인" run={checkStaff} />
              </>
            )}
            {staffView === 'checking' && (
              <>
                <Label>지점 권한을 확인하고 있습니다.</Label>
                <Button label="인증 취소" run={() => closeStaff()} />
              </>
            )}
            {staffView === 'denied' && (
              <>
                <Label>
                  이 단말을 운영할 권한이 없습니다. 지점 관리자에게 직원 등록
                  상태를 문의하세요.
                </Label>
                <Button label="다시 인증" run={() => closeStaff('login')} />
              </>
            )}
            <Button label="고객 화면" run={() => closeStaff()} />
          </>
        )}
        {staffView === 'work' && (
          <Button label="고객 화면" run={() => closeStaff()} />
        )}
        {staff?.permissions.includes('refund_console') &&
          staffView === 'work' && (
            <>
              <Label>
                환불 업무는 P05 운영 콘솔에서 등록된 재무 인증서로 확인합니다.
              </Label>
              {staff.console ? (
                <Button
                  label="P05 환불 운영 화면"
                  disabled={busy}
                  run={() =>
                    work(
                      openRefundConsole,
                      '지점의 재무 권한이나 운영 콘솔을 확인할 수 없습니다.',
                    )
                  }
                />
              ) : (
                <Label>P05 운영 콘솔 주소를 확인할 수 없습니다.</Label>
              )}
            </>
          )}
        {staff?.permissions.includes('terminal_settings') &&
          staffView === 'work' && (
            <>
              <Text accessibilityRole="header" style={s.title}>
                직원 단말 설정
              </Text>
              <Label>
                직원 권한이 유지되는 지점의 단말만 변경할 수 있습니다.
              </Label>
              <TextInput
                accessibilityLabel="단말 식별값"
                placeholder="단말 식별값"
                autoCapitalize="none"
                maxLength={36}
                style={s.input}
                value={target}
                editable={!busy && !pendingMode && !modeReview}
                onChangeText={value => {
                  setTarget(value);
                  setMode(undefined);
                  setModeReview(undefined);
                  setOrderPage(undefined);
                }}
              />
              <Button
                label="현재 단말 모드 조회"
                run={() =>
                  work(
                    refreshTerminalSettings,
                    '단말 또는 직원 권한을 확인할 수 없습니다.',
                  )
                }
                disabled={
                  busy || !terminalRef.test(target.trim()) || !!modeReview
                }
              />
              {mode && (
                <Label>{`${
                  mode.mode === 'menu' ? '메뉴 주문 모드' : '결제 전용 모드'
                } · v${mode.revision}`}</Label>
              )}
              {mode &&
                !modeReview &&
                !pendingMode &&
                mode.transition?.state !== 'waiting' && (
                  <>
                    <Button
                      label="메뉴 주문 모드로 전환"
                      run={() => setModeReview('menu')}
                      disabled={busy || mode.mode === 'menu'}
                    />
                    <Button
                      label="결제 전용 모드로 전환"
                      run={() => setModeReview('pos')}
                      disabled={
                        busy ||
                        mode.mode === 'pos' ||
                        mode.orderCount === undefined
                      }
                    />
                  </>
                )}
              {mode && modeReview && (
                <>
                  <Label>전환 영향 검토</Label>
                  <Label>{`지점 · ${client?.profile.branchRef}`}</Label>
                  <Label>{`대상 단말 · ${target.trim()}`}</Label>
                  <Label>{`새 설정 · ${
                    modeReview === 'pos' ? '결제 전용' : '메뉴 주문'
                  } v${mode.revision + 1}`}</Label>
                  {modeReview === 'pos' && (
                    <Label>
                      신규 주문을 중지하고 진행 중인 원주문을 지점 운영 대기열로
                      인계합니다.
                    </Label>
                  )}
                  {modeReview === 'pos' && (
                    <Label>{`인계 대상 · ${mode.orderCount ?? 0}건`}</Label>
                  )}
                  {modeReview === 'pos' && modeOrders()}
                  <Button
                    label="전환 요청 저장"
                    disabled={busy}
                    run={() =>
                      work(
                        confirmModeRequest,
                        '전환 결과를 확인할 수 없습니다. 기존 요청을 다시 조회하세요.',
                      )
                    }
                  />
                  <Button
                    label="기존 모드로 돌아가기"
                    disabled={busy}
                    run={() => setModeReview(undefined)}
                  />
                </>
              )}
              {mode?.transition && (
                <>
                  <Label>{`결제 전용 v${mode.transition.targetRevision} · ${
                    mode.transition.state === 'active'
                      ? '적용 완료'
                      : '전환 대기'
                  }`}</Label>
                  <Label>{`지점 인계 · ${
                    mode.transition.handoff ? '확인' : '대기'
                  } · 단말 적용 · ${
                    mode.transition.ack ? '확인' : '대기'
                  }`}</Label>
                  {mode.transition.state === 'waiting' && (
                    <Label>신규 주문 · 중지</Label>
                  )}
                  <Label>{`지점 인계 대기열 · ${
                    mode.orderCount ?? 0
                  }건`}</Label>
                  {modeOrders()}
                  {mode.transition.state === 'waiting' &&
                    !mode.transition.handoff && (
                      <Button
                        label="지점 인계 확인"
                        disabled={busy}
                        run={() =>
                          work(
                            () => confirmHandoff(mode.transition!),
                            '인계 결과를 확인할 수 없습니다. 기존 요청을 다시 조회하세요.',
                          )
                        }
                      />
                    )}
                </>
              )}
              {pendingMode && (
                <Button
                  label="기존 요청 재조회"
                  disabled={busy}
                  run={() =>
                    work(
                      () => retryMode(pendingMode),
                      '전환 결과를 확인할 수 없습니다. 신규 주문 중지와 원요청을 유지합니다.',
                    )
                  }
                />
              )}
            </>
          )}
        {staff?.permissions.includes('print_receipts') &&
          staffView === 'work' &&
          registration?.slot === 'operator' &&
          pickupJournal &&
          (printView ? (
            <ReceiptPrint
              key={pickupJournal + (printView.sale ?? '')}
              journal={'print-v1-' + pickupJournal.slice('pickup-v2-'.length)}
              branch={staff.branchRef}
              sale={printView.sale}
              transport={(operation, reference, body) =>
                Http!.exchangePrint(
                  operation,
                  registration.digest,
                  reference,
                  body,
                )
              }
              close={closePrint}
            />
          ) : (
            <Button
              label="직원 영수증과 출력"
              run={() => setPrintView({})}
              disabled={busy}
            />
          ))}
        {!printView &&
          staff?.permissions.includes('pickup') &&
          staffView === 'work' &&
          registration &&
          pickupJournal &&
          (pickupOpen ? (
            <PickupKitchen
              key={pickupJournal}
              journal={pickupJournal}
              transport={body =>
                Http!.exchangeScoped(
                  registration.slot,
                  registration.digest,
                  body,
                )
              }
              branch={staff.branchRef}
              actor={staff.actorRef}
              allowCafe={registration.slot === 'operator'}
              paymentProfile={policy}
              openReceipt={
                staff.permissions.includes('print_receipts')
                  ? sale => setPrintView({ sale })
                  : undefined
              }
              close={() => setPickupOpen(false)}
            />
          ) : (
            <Button
              label="지점 픽업 대기열"
              disabled={busy}
              run={() => setPickupOpen(true)}
            />
          ))}
        {setup === 'terminal' &&
          staffView === 'public' &&
          !offlineMenu &&
          !screen &&
          unresolvedComparisons && (
            <Button
              label="보류 연결 상태 확인"
              disabled={busy}
              run={() =>
                work(
                  recoverComparisons,
                  '기존 주문의 연결 결과를 다시 확인하세요.',
                )
              }
            />
          )}
        {setup === 'terminal' &&
          staffView === 'public' &&
          !offlineMenu &&
          !screen &&
          pendingPayment && (
            <Button
              label="보류 지급 상태 확인"
              disabled={busy}
              run={() =>
                work(resumeApproval, '보류 결과를 확인할 수 없습니다.')
              }
            />
          )}
        {setup === 'terminal' && staffView === 'public' && !offlineMenu && !screen && !pickupStaff && (
          <>
            {entrance && (
              <>
                <Label>{entrance.label}</Label>
                <Label>{`주문 단말 ${client?.profile.subjectRef.slice(-8)}`}</Label>
                <View accessible accessibilityLabel="개인 주문을 시작하는 매장 입구 QR">
                  <QrCode text={entrance.qr} />
                </View>
              </>
            )}
            <Button label={entrance ? '매장 입구 QR 닫기' : '매장 입구 QR 보기'} disabled={busy}
              run={() => entrance ? setEntrance(undefined) : work(async () => {
                const online=await publicMenuStatus();
                if (!online) return;
                if (!online.shop) throw new Error('SHOP_MENU_REQUIRED');
                const shop=online.shop;
                setEntrance({label: shop.branchLabel, qr: makeEntranceQr(shop.branch)});
              }, '온라인 메뉴 단말의 등록 지점을 확인할 수 없습니다.')} />
          </>
        )}
        {setup === 'terminal' &&
          staffView === 'public' &&
          !offlineMenu &&
          !screen &&
          !pickupStaff && (
            <Button
              label="새 고객 주문"
              run={() =>
                work(
                  newCustomer,
                  '인증된 메뉴 연결을 확인할 수 없습니다.',
                )
              }
              disabled={busy}
            />
          )}
        {setup==='terminal' && staffView==='public' && offlineMenu && (
          <OfflineMenu cached={offlineMenu} policy={policy} busy={busy}
            retry={()=>work(retryMenuConnection,'인증된 메뉴 연결을 확인할 수 없습니다.')} />
        )}
        {setup === 'terminal' && staffView === 'public' && screen?.connectionEnded && (
          <>
            <Label>연결 시간이 지났습니다</Label>
            <Label>견적을 받은 경우 폰에서 원주문을 확인할 수 있습니다.</Label>
            <Label>공용 화면 · 자동 초기화 대기</Label>
            <Button label="공용 화면 닫기" run={clearPublic} />
          </>
        )}
        {setup === 'terminal' && staffView === 'public' && screen && !screen.connectionEnded && publicGuide === 'phone' && client && menuBranchLabel && (
          <>
            <Text accessibilityRole="header" style={s.title}>내 폰에서 주문하기</Text>
            <Label>{menuBranchLabel}</Label>
            <Label>{`주문 단말 ${client.profile.subjectRef.slice(-8)}`}</Label>
            <Label>매장 입구 QR을 스캔하거나 지갑 앱에서 근거리 매장을 찾으세요.</Label>
            <View accessible accessibilityLabel="개인 주문을 시작하는 매장 입구 QR">
              <QrCode text={makeEntranceQr(client.profile.branchRef)} />
            </View>
            <Button label="내 공용 장바구니로" run={() => {
              openPublicGuide();
              showBasket(true);
            }} />
          </>
        )}
        {setup === 'terminal' && staffView === 'public' && screen && !screen.connectionEnded && publicGuide === 'cancel' && (
          <>
            <Text accessibilityRole="header" style={s.title}>공용 주문을 닫을까요?</Text>
            <Label>휴대전화에 연결되지 않은 예약을 해제합니다.</Label>
            <Label>연결한 휴대전화에서 원주문을 확인할 수 있습니다.</Label>
            <Button label="계속 주문하기" run={() => openPublicGuide()} />
            <Button label="닫고 화면 지우기" run={clearPublic} />
          </>
        )}
        {setup === 'terminal' && staffView === 'public' && screen && !screen.connectionEnded && !publicGuide && (
          <>
            <Label>
              {mode?.mode === 'pos' ? 'POS 결제 전용' : (menuBranchLabel ?? '메뉴 주문')}
            </Label>
            {!screen.key && mode?.mode === 'pos' && (
              <>
                <Label>StableNet 테스트넷 8283 · TestUSDC</Label>
                <TextInput
                  accessibilityLabel="POS 인계 코드"
                  placeholder="등록된 POS에서 발급한 인계 코드"
                  style={s.input}
                  autoCapitalize="none"
                  autoCorrect={false}
                  maxLength={36}
                  value={pos}
                  onChangeText={setPos}
                />
                <Button
                  label="POS 원주문 조회"
                  disabled={busy || !validPOSHandoff(pos.trim())}
                  run={() =>
                    work(
                      () => reconcilePOS(pos.trim()),
                      '같은 POS 인계 코드로 다시 확인하세요.',
                    )
                  }
                />
              </>
            )}
            {!screen.key && mode?.mode === 'menu' && (
              <>
                {!!stock.length && (
                  <Button
                    label={
                      menuLocale === 'ko'
                        ? '메뉴 언어 English'
                        : 'Menu language 한국어'
                    }
                    run={() => {
                      keepPublicActive();
                      setMenuLocale(locale => (locale === 'ko' ? 'en' : 'ko'));
                    }}
                  />
                )}
                {!!menuBranchLabel && !menuDetail && !cartReplacement &&
                  <Button label="내 폰에서 주문하기" disabled={busy} run={() => openPublicGuide('phone')} />}
                {basketOpen && <>
                  <Text ref={basketHeading} accessibilityRole="header" style={s.title}>
                    {menuLocale === 'ko' ? '장바구니' : 'Basket'}
                  </Text>
                  {!basketUnits && <Label>{menuLocale === 'ko' ? '담은 메뉴가 없습니다.' : 'Your basket is empty.'}</Label>}
                  <Button label={menuLocale === 'ko' ? '메뉴로 돌아가기' : 'Back to menu'}
                    run={() => showBasket(false)} disabled={busy} />
                  <Button label="주문 그만하기" run={() => openPublicGuide('cancel')} disabled={busy} />
                </>}
                {!basketOpen && !menuDetail && !cartReplacement && (
                  <MenuCategorySelector categories={categories} selected={selectedCategory} locale={menuLocale}
                    select={(key) => { keepPublicActive(); setMenuCategory(key); }} />
                )}
                {!basketOpen && !menuDetail && !cartReplacement && stock.map(row => {
                  if (selectedCategory && row.category?.key !== selectedCategory) return null;
                  const productRows = menuGroups.get(row.sku) ?? [row];
                  const required = productRows.some(item => item.metadata?.optionRequired === true);
                  const first = productRows[0] === row;
                  if (required && !first) return null;
                  const names = commerceMenuLabels(row, menuLocale);
                  const quantity = required
                    ? productRows.reduce((sum, item) => sum + (choice[commerceLineKey(item)]?.quantity ?? 0), 0)
                    : choice[commerceLineKey(row)]?.quantity ?? 0;
                  const available = required
                    ? row.skuAvailable ?? Math.max(...productRows.map(item => item.available))
                    : commerceLineCapacity(row, choice);
                  return (
                    <View key={row.sku + '/' + row.optionKey} style={s.line}>
                      <Label>
                        {required
                          ? names.product
                          : `${names.product} · ${names.option} · ${commerceAmount(row.unitPrice, row.asset, policy)}`}
                      </Label>
                      {!required && <Label>{commerceMenuDisclosure(row, menuLocale)}</Label>}
                      <Label>
                        {available === 0
                          ? menuLocale === 'ko'
                            ? '품절'
                            : 'Sold out'
                          : menuLocale === 'ko'
                            ? `가능 수량 ${available} · 선택 ${quantity}`
                            : `Available ${available} · Selected ${quantity}`}
                      </Label>
                      {first && (
                        <Button
                          label={menuLocale === 'ko'
                            ? `${names.product} ${required ? '옵션 선택' : '상세'}`
                            : `${required ? 'Choose options for' : 'Details for'} ${names.product}`}
                          disabled={busy}
                          run={() => openMenuDetail(productRows, required)}
                        />
                      )}
                      {!required && <Button
                        label={`${names.product} ${names.option} ${
                          menuLocale === 'ko' ? '추가' : 'Add'
                        }`}
                        disabled={busy || quantity >= available}
                        run={() => {
                          keepPublicActive();
                          putChoice(row, quantity + 1, true);
                        }}
                      />}
                      {!required && !!quantity && (
                        <Button
                          label={`${names.product} ${names.option} ${menuLocale === 'ko' ? '빼기' : 'Remove'}`}
                          disabled={busy}
                          run={() => reduceChoice(row)}
                        />
                      )}
                    </View>
                  );
                })}
                {menuDetail && !cartReplacement && (() => {
                  const rows = menuGroups.get(menuDetail.sku) ?? [];
                  if (!rows.length) return null;
                  const selected = rows.find(row => row.optionKey === menuDetail.option);
                  const ko = menuLocale === 'ko';
                  const required = rows.some(row => row.metadata?.optionRequired === true);
                  const group = required ? rows[0].metadata?.optionGroupLabels?.[menuLocale] : undefined;
                  const groupTitle = `${group ?? (ko ? '옵션' : 'Options')}${required ? (ko ? ' (필수)' : ' (required)') : ''}`;
                  const maximum = selected ? commerceLineCapacity(selected, choice) :
                    Math.max(...rows.map(row => commerceLineCapacity(row, choice)));
                  return (
                    <View style={s.line}>
                      <Text accessibilityRole="header" style={s.title}>
                        {commerceMenuLabels(selected ?? rows[0], menuLocale).product}
                      </Text>
                      {(rows.length > 1 || required) && (
                        <View>
                          <Text ref={optionHeading} testID="menu-option-group-heading" accessible accessibilityRole="header" style={s.label}>{groupTitle}</Text>
                          {menuDetail.error === 'option' && (
                            <Text accessibilityRole="alert" style={s.label}>{missingOptionMessage(group)}</Text>
                          )}
                          {rows.map(row => {
                            const option = commerceMenuLabels(row, menuLocale).option;
                            return (
                              <View key={row.optionKey}>
                                <Label>{`${option} · ${commerceAmount(row.unitPrice, row.asset, policy)}`}</Label>
                                <Button
                                  label={ko ? `${option} 선택` : `Select ${option}`}
                                  disabled={commerceLineCapacity(row, choice) === 0 || row === selected}
                                  run={() => {
                                    keepPublicActive();
                                    const existing = choice[commerceLineKey(row)];
                                    setMenuDetail({
                                      ...menuDetail, option: row.optionKey, error: undefined,
                                      quantity: existing?.quantity ?? (menuDetail.option && row.skuAvailable !== undefined ? 1 : menuDetail.quantity),
                                    });
                                  }}
                                />
                                {row.available === 0 && <Label>{ko ? '품절' : 'Sold out'}</Label>}
                              </View>
                            );
                          })}
                        </View>
                      )}
                      {maximum === 0 && <Label>{ko ? '품절' : 'Sold out'}</Label>}
                      {selected && (
                        <>
                          <Label>{commerceMenuDisclosure(selected, menuLocale)}</Label>
                          {(!!selected.metadata?.allergens?.length || selected.metadata?.crossContact === 'possible') && (
                            <Label>{ko ? '알레르기가 있으면 주문 전에 직원에게 제조 환경을 확인하세요.' :
                              'If you have an allergy, ask staff about preparation before ordering.'}</Label>
                          )}
                          <Label>{`${ko ? '가격' : 'Price'} · ${commerceAmount(selected.unitPrice, selected.asset, policy)}`}</Label>
                          <Label>{`${ko ? '소계' : 'Subtotal'} · ${commerceCartAmount(
                            (BigInt(selected.unitPrice) * BigInt(menuDetail.quantity)).toString(), selected.asset, policy,
                          )}`}</Label>
                        </>
                      )}
                      <Label>{`${ko ? '수량' : 'Quantity'} · ${menuDetail.quantity}`}</Label>
                      <Button label={ko ? '수량 늘리기' : 'Increase quantity'}
                        disabled={menuDetail.quantity >= maximum}
                        run={() => {
                          keepPublicActive();
                          setMenuDetail({ ...menuDetail, quantity: menuDetail.quantity + 1 });
                        }} />
                      <Button label={ko ? '수량 줄이기' : 'Decrease quantity'}
                        disabled={menuDetail.quantity <= 1}
                        run={() => {
                          keepPublicActive();
                          setMenuDetail({ ...menuDetail, quantity: menuDetail.quantity - 1 });
                        }} />
                      {menuDetail.error === 'stock' && (
                        <Text accessibilityRole="alert">{ko ? '선택한 수량을 준비할 수 없습니다' : 'This quantity is unavailable'}</Text>
                      )}
                      <Button label={ko ? '장바구니에 담기' : 'Add to basket'}
                        disabled={maximum === 0}
                        run={() => { keepPublicActive(); addMenuDetail(menuDetail, selected, group); }} />
                      <Button label={ko ? '메뉴로 돌아가기' : 'Back to menu'}
                        run={() => { keepPublicActive(); setMenuDetail(undefined); }} />
                    </View>
                  );
                })()}
                {cartReplacement && (() => {
                  const ko = menuLocale === 'ko';
                  const line = ({ row, quantity }: CommerceSelection[string]) => {
                    const names = commerceMenuLabels(row, menuLocale);
                    const amount = commerceCartAmount((BigInt(row.unitPrice) * BigInt(quantity)).toString(), row.asset, policy);
                    return `${names.product} · ${names.option} ×${quantity} · ${amount}`;
                  };
                  return (
                    <View style={s.line}>
                      <Label>{ko ? '같은 메뉴의 옵션을 교체할까요?' : 'Replace the option for this item?'}</Label>
                      <Label>{line(cartReplacement.previous)}</Label>
                      <Label>{line(cartReplacement.next)}</Label>
                      <Button label={ko ? '기존 옵션 유지' : 'Keep current option'}
                        run={() => { keepPublicActive(); setCartReplacement(undefined); }} />
                      <Button label={ko ? '새 옵션으로 교체' : 'Replace with new option'}
                        run={() => {
                          keepPublicActive();
                          const next = cartReplacement.next;
                          setChoice(old => {
                            const updated = { ...old };
                            delete updated[commerceLineKey(cartReplacement.previous.row)];
                            updated[commerceLineKey(next.row)] = next;
                            return updated;
                          });
                          setCartReplacement(undefined);
                          setMenuDetail(undefined);
                        }} />
                    </View>
                  );
                })()}
                {Object.values(choice).map(({ row, quantity }) => {
                  const names = commerceMenuLabels(row, menuLocale);
                  const required = menuGroups.get(row.sku)?.some(item => item.metadata?.optionRequired);
                  return (
                    <View key={commerceLineKey(row)}>
                      <Label>{`${names.product} · ${names.option} ×${quantity}`}</Label>
                      {(required || basketOpen) && !menuDetail && !cartReplacement && <Button
                        label={`${names.product} ${names.option} ${menuLocale === 'ko' ? '빼기' : 'Remove'}`}
                        disabled={busy}
                        run={() => reduceChoice(row)}
                      />}
                    </View>
                  );
                })}
                {!menuDetail && !cartReplacement && <>
                {cartReview?.shortages.map(({ row, quantity }) => {
                  const names = commerceMenuLabels(row, menuLocale);
                  return (
                    <Label key={commerceLineKey(row)}>
                      {`확보 못한 품목 · ${names.product} · ${names.option} ×${quantity}`}
                    </Label>
                  );
                })}
                {!!cartReview?.priceChanges.length && (
                  <>
                    <Label>
                      가격이 바뀌었습니다. 변경된 옵션과 합계를 확인하세요.
                    </Label>
                    {cartReview.priceChanges.map(({ previous, current }) => {
                      const names = commerceMenuLabels(current, menuLocale);
                      return (
                        <Label key={commerceLineKey(current)}>{`${names.product} · ${
                          names.option
                        } · ${commerceCartAmount(
                          previous.unitPrice,
                          previous.asset,
                          policy,
                        )} → ${commerceCartAmount(
                          current.unitPrice,
                          current.asset,
                          policy,
                        )}`}</Label>
                      );
                    })}
                    {cartReview.previousTotals.map(({ asset, amount }) => (
                      <Label key={asset}>{`기존 합계 · ${commerceCartAmount(
                        amount,
                        asset,
                        policy,
                      )}`}</Label>
                    ))}
                  </>
                )}
                {basketTotals.map(({ asset, amount }) => (
                  <Label key={asset}>
                    {`${
                      cartReview?.priceChanges.length ? '새 합계' : '합계'
                    } · ${commerceCartAmount(amount, asset, policy)}`}
                  </Label>
                ))}
                <Button
                  label={
                    cartReview
                      ? '바뀐 장바구니 확인 후 주문 접수'
                      : '주문 접수 요청'
                  }
                  disabled={busy || !Object.keys(choice).length}
                  run={() =>
                    work(
                      reserve,
                      '이 주문을 접수했는지 확인할 수 없습니다. 같은 주문 상태를 확인하세요.',
                    )
                  }
                />
                </>}
              </>
            )}
            {screen.key && (
              <>
                {screen.pos && (
                  <Label>{`POS 주문 ${screen.pos.reference}`}</Label>
                )}
                {screen.pos && <Label>{screen.pos.branchLabel}</Label>}
                {!screen.pos && screen.sale && (
                  <>
                    <Label>
                      {screen.sale.menuCheckout?.branchLabel ??
                        `지점 · ${client?.profile.branchRef.slice(-8)}`}
                    </Label>
                    <Label>{`원주문 · ${screen.sale.id}`}</Label>
                  </>
                )}
                {(screen.pos ||
                  (screen.sale && policy?.asset === screen.sale.asset)) && (
                  <Label>StableNet 테스트넷 8283 · TestUSDC</Label>
                )}
                {screen.posSelection?.state === 'locking' && (
                  <Label>{`선택한 표식 · ${screen.posSelection.code}`}</Label>
                )}
                {screen.pairing?.state === 'binding' && (
                  <Label>{`선택한 표식 · ${screen.pairing.code}`}</Label>
                )}
                {screen.pairing?.state === 'unknown' && (
                  <Label>{`마지막 확인 · ${new Date(screen.pairing.checkedAt * 1000).toLocaleTimeString('ko-KR', {hour: '2-digit', minute: '2-digit'})}`}</Label>
                )}
                {screen.selectedCode && (
                  <Label>{`연결한 휴대전화 · ${screen.selectedCode}`}</Label>
                )}
                <Label>
                  {screen.sale?.menuCheckout?.finalOutcome === 'relay-paid'
                    ? '결제 완료'
                    : screen.sale?.menuCheckout?.finalOutcome === 'expired-unpaid'
                    ? '원견적 만료 · 지급 없음 확정'
                    : screen.sale?.state === 'paid'
                    ? '서버 지급 확인'
                    : screen.sale?.state === 'closed'
                    ? '주문 종료'
                    : screen.sale?.state === 'checking'
                    ? '결제 확인 중 · 재청구 보류'
                    : screen.posSelection?.state === 'locking'
                    ? '선택한 지급자의 견적을 기록하고 있습니다'
                    : screen.posSelection?.state === 'conflict'
                    ? '다른 지급자가 선택됐습니다'
                    : screen.posSelection?.state === 'unknown'
                    ? '결합 결과를 확인하지 못했습니다 · 같은 원주문을 조회하세요'
                    : screen.pos?.selectionHeld
                    ? '기존 지급자 결합을 확인해야 합니다 · 운영자 확인 대기'
                    : screen.pos?.payerBound && screen.sale?.state === 'preview'
                    ? '지급자 확인됨 · 견적 기록 대기'
                    : screen.pos?.payerBound && screen.sale?.state === 'issued'
                    ? '기존 견적 · 선택한 폰에서 결제를 계속하세요'
                    : screen.pos
                    ? '고객 폰의 연결 표식을 확인하세요'
                    : screen.pairing?.state === 'binding'
                    ? '이 폰의 결제 요청을 준비합니다'
                    : screen.pairing?.state === 'unknown'
                    ? '기존 결제 연결을 확인하고 있습니다'
                    : screen.sale?.state === 'issued'
                    ? '기존 견적 · 연결한 폰에서 결제를 계속하세요'
                    : screen.selectedCode ||
                      screen.sale?.menuCheckout?.payerBound
                    ? '휴대전화 연결 확인 · 연결한 폰에서 결제를 계속하세요'
                    : screen.locator
                    ? '폰에서 이 주문을 열어 주세요'
                    : screen.sale
                    ? '폰 연결 대기'
                    : '접수 여부 확인 중'}
                </Label>
                {screen.sale && (
                  <Label>
                    금액{' '}
                    {screen.pos
                      ? posAmount(screen.sale.amount)
                      : commerceAmount(
                          screen.sale.amount,
                          screen.sale.asset,
                          policy,
                        )}
                  </Label>
                )}
                {screen.locator && (
                  <>
                    <View
                      accessible
                      accessibilityLabel="이 주문을 여는 일회용 QR"
                    >
                      <QrCode text={screen.locator} />
                    </View>
                    <Button
                      label="휴대전화 확인 번호 조회"
                      disabled={busy}
                      run={() =>
                        work(pollPairs, '확인 번호를 조회하지 못했습니다.')
                      }
                    />
                    {onlinePairs.length > 0 && (
                      <Label>
                        휴대전화 화면에 표시된 번호와 같은 번호를 선택하세요.
                      </Label>
                    )}
                    {onlinePairs.map(row => (
                      <Button
                        key={row.code}
                        label={`확인 번호 ${row.code}와 일치`}
                        disabled={busy}
                        run={() =>
                          screen.pos
                            ? void selectPOSComparison(row)
                            : work(
                                () => acceptComparison(row),
                                '기존 연결 결과를 이 주문에서 다시 확인하세요.',
                              )
                        }
                      />
                    ))}
                  </>
                )}
                {screen.sale?.menuCheckout?.finalOutcome === 'relay-paid' &&
                  <Label>네트워크 수수료 · 가맹점 릴레이 부담</Label>}
                {screen.sale?.state === 'paid' &&
                  screen.sale.pickupNo !== undefined && (
                    <Label>픽업 번호 {screen.sale.pickupNo}</Label>
                  )}
                {screen.pos && screen.sale?.state === 'paid' && (
                  <Label>{`같은 영수증 ${screen.sale.receiptRef}`}</Label>
                )}
                {pendingPayment &&
                  pendingPayment.order.id === screen.sale?.id && (
                    <>
                      {screen.sale?.state === 'issued' && !pendingPayment.signature && (
                        <Button
                          label="직접 승인할 보드 찾기"
                          disabled={busy}
                          run={() =>
                            work(
                              scanBoards,
                              '인증된 결제 보드를 찾을 수 없습니다.',
                            )
                          }
                        />
                      )}
                      {screen.sale?.state === 'issued' && !pendingPayment.signature && boardOptions.map((row, index) => (
                        <Button
                          key={row.handle}
                          label={`보드 후보 ${index + 1} 인증 및 검토`}
                          disabled={busy}
                          run={() =>
                            work(
                              () => approveAtBoard(row.handle),
                              '보드 응답이 미확정입니다. 같은 주문을 유지하세요.',
                            )
                          }
                        />
                      ))}
                      <Button
                        label="같은 주문의 지급 결과 확인"
                        disabled={busy}
                        run={() =>
                          work(resumeApproval, '지급 상태를 다시 확인하세요.')
                        }
                      />
                    </>
                  )}
                {screen.locator && pairCode && (
                  <View style={s.line}>
                    <Label>
                      휴대전화 화면과 같은 확인 번호인지 확인하세요.
                    </Label>
                    <Text style={s.mono}>{pairCode}</Text>
                    <Button
                      label="확인 번호가 같은 휴대전화에 연결"
                      run={() =>
                        work(
                          async () =>
                            relayRef.current?.approveDisplayed(pairCode),
                          '인계 요청이 만료되었습니다.',
                        )
                      }
                    />
                  </View>
                )}
                {screen.pos && (
                  <Button label="다른 POS 원주문 열기" disabled={busy} run={() => {
                    relayRef.current?.clearPublic();
                    if (qrExpiry.current) clearTimeout(qrExpiry.current);
                    setOnlinePairs([]);
                    setPairCode(undefined);
                    setMessage('');
                    setPos('');
                    show({session: screen.session});
                  }} />
                )}
                <Button
                  label={
                    screen.pos ? 'POS 원주문 다시 조회' : screen.pairing?.state === 'unknown' ? '기존 연결 조회' : '이 주문 다시 확인'
                  }
                  disabled={busy}
                  run={() =>
                    work(
                      () =>
                        screen.pos
                          ? reconcilePOS(screen.pos.handoff)
                          : reconcile(screen.session, screen.key!),
                      '결과 확인 중입니다. 동일 주문을 다시 조회하세요.',
                    )
                  }
                />
              </>
            )}
            <Label>
              1분 동안 사용하지 않으면 이 주문 화면을 비웁니다. 주문을 더
              살펴보려면 시간을 연장하세요.
            </Label>
            <Button
              label="계속 주문하고 시간 연장"
              run={keepPublicActive}
              disabled={busy}
            />
            <Button label="화면을 비우고 대기"
              run={screen.key && !screen.pos && screen.sale?.state === 'preview' &&
                !screen.sale.menuCheckout?.payerBound && !screen.selectedCode
                ? () => openPublicGuide('cancel') : clearPublic} />
          </>
        )}
      </ScrollView>
      {basketVisible && !publicGuide && !basketOpen && !menuDetail && !cartReplacement && (
        <View style={s.basketBar}>
          <View accessibilityLiveRegion="polite">
            <Text style={s.label}>
              {menuLocale === 'ko' ? `담은 수량 ${basketUnits}` : `Items in your basket: ${basketUnits}`}
            </Text>
            {basketTotals.map(({ asset, amount }) => <Text style={s.label} key={asset}>
              {menuLocale === 'ko' ? '합계 · ' : 'Total · '}{commerceCartAmount(amount, asset, policy)}
            </Text>)}
          </View>
          <Button label={menuLocale === 'ko' ? '내 장바구니' : 'My basket'}
            run={() => showBasket(true)} disabled={busy} />
        </View>
      )}
    </SafeAreaView>
  );
}

const s = StyleSheet.create({
  root: { flex: 1, backgroundColor: '#fafaf6' },
  viewport: { flex: 1 },
  basketBar: {
    borderTopWidth: 1,
    borderTopColor: '#d3dfd4',
    backgroundColor: '#fafaf6',
    paddingHorizontal: 22,
    paddingVertical: 8,
    flexDirection: 'row',
    flexWrap: 'wrap',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: 8,
  },
  scroll: { padding: 22, gap: 12 },
  title: { fontSize: 26, color: '#122a22', fontWeight: '700' },
  label: { fontSize: 17, color: '#26382f', lineHeight: 24 },
  notice: { fontSize: 16, color: '#2d4b3e', lineHeight: 22 },
  mono: { fontFamily: 'monospace', fontSize: 14, color: '#193c2c' },
  registration: { height: 100, textAlignVertical: 'top' },
  input: {
    borderWidth: 1,
    borderColor: '#71897d',
    borderRadius: 8,
    minHeight: 48,
    padding: 12,
    color: '#182923',
  },
  line: {
    borderBottomWidth: 1,
    borderBottomColor: '#d3dfd4',
    paddingBottom: 12,
    gap: 8,
  },
  button: {
    minHeight: 45,
    borderRadius: 8,
    backgroundColor: '#1d4636',
    padding: 12,
    justifyContent: 'center',
  },
  buttonText: { fontSize: 16, color: '#fff', fontWeight: '600' },
  disabled: { opacity: 0.5 },
});
