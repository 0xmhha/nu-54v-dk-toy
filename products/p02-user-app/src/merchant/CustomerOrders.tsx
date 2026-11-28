import {inputPlaceholder} from '../ui/theme.ts';
import {
  Fragment,
  useCallback,
  useEffect,
  useEffectEvent,
  useMemo,
  useRef,
  useState,
  type ComponentRef,
} from 'react';
import {sha256} from '@noble/hashes/sha256';
import {
  AppState,
  BackHandler,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import {
  type CommerceClient,
  assertCommerceCatalogPage,
  assertCommerceCatalogVersion,
  appendCommerceCatalogPage,
  assertCommerceChoices,
  commerceCatalogLimit,
  bytesToHex,
  hexToBytes,
  commerceProfile,
  fromBoard, paymentToken,
  commerceCartTotals,
  commerceLineKey,
  commerceLineCapacity,
  commerceHasSiblingOption,
  commerceNeedsOptionSupport,
  CommercePhotoCache,
  commerceCartTax,
  commerceLineTax,
  commerceReservationRejected,
  CommerceHoldRefusal,
  reviewCommerceCart,
  commerceMenuLabels,
  commerceMenuDisclosure,
  quoteOrderRevision,
  encodeCommerceCart,
  fromBase64,
  parseOrderLocator,
  readShopIdentity,
  paymentProfile,
  verifyPresentedGasSponsor,
  verifyPresentedRentalEscrow,
  verifyPresentedRentalReviewAuthority,
  verifyRentalPaymentReview,
  verifyRentalPaymentDecision,
  validOrderEscrowStatus,
  type RentalReviewAuthority,
  type RentalReviewChallenge,
  unpackNearbyPresentation,
  packNearbyPresentation,
  toBase64,
  presentationFingerprint,
  type CommerceCart,
  type CommerceCartLine,
  type CommerceOrder,
  type CommerceOrderPayment,
  type OrderBuyerAction,
  type CustomerOrderSummary,
  type SubscriptionInvoice,
  type CommercePrice,
  type CommerceOrderState,
  type CommerceCartReview,
  type CommerceSelection,
  type NearbyCommerceLink,
  type CommerceQuote,
  paysThroughEscrow,
  type CommercePaymentProfile,
} from '@nu54/protocol';
import Http from '../specs/NativeCommerceHttp.ts';
import {StoreNotice, storeStopped} from './StoreNotice.tsx';
import {BookingPaymentTerms, type TravelBookingPayment, type TravelBookingScope} from '../travel/BookingPaymentTerms.tsx';
import {TravelPickupDetail} from '../travel/TravelPickupDetail.tsx';
import Nearby from '../specs/NativeNearby.ts';
import {
  isolateSubscriptionReminders,
  SubscriptionScreen,
} from './subscriptions/SubscriptionScreen.tsx';
import { ShopMenuCategories, shopMenuCategories } from './ShopMenuCategories';
import { ChoiceCard, MenuChoices } from './MenuChoices.tsx';
import { CustomerNearby } from './nearbyClient.ts';
import { ShopDiscovery, type ShopMatch } from './ShopDiscovery.tsx';
import ShopQr from '../specs/NativeShopQr.ts';
import OwnerWallet from '../specs/NativeOwnerWallet.ts';
import Ble from '../specs/NativeRenterBle.ts';
import { OwnerSession, type OwnerApprovalProgress } from '../wallet/ownerSession.ts';
import { decodeWalletStorage } from '../wallet/profile.ts';
import { readEvmReceiveBalance } from '../wallet/evmTransfer.ts';
import { serverFor } from '../wallet/networks.ts';
import { readProfiles, walletReadRpc } from '../wallet/readProfiles.ts';
import { OWNER_GATT, type Enrollment, type OwnerStatus } from '../wallet/ownerWire.ts';
import { WalletMaintenance } from '../wallet/WalletMaintenance.tsx';
import { maintenanceScope, sameMaintenanceScope, type MaintenanceScope } from '../wallet/walletMaintenance.ts';
import {
  approvalForForward,
  lockForSubmit,
  neverSent, matchesReviewedOrder,
  needsMerchantSponsorship,
  paymentReviewStop,
  paymentCheckpoint,
  type PaymentCheckpoint,
  reviewedPayment,
  type PaymentReviewStop,
} from './paymentCheckpoint.ts';

import {BoardStepPanel} from '../wallet/BoardStepPanel.tsx';
import {english as walletEnglish} from '../wallet/walletEnglish.ts';
import {Card, KitButton, Section, TabHeader, type ButtonVariant} from '../ui/kit.tsx';
import {AmountLine, PickupTicket} from './paymentParts.tsx';
import {color, font, radius} from '../ui/theme.ts';
import {customerJournals} from './customerJournal.ts';
import {askOrderNotices, customerClient, openOrderLabel, recordOrder, refreshOpenOrders, sendOrderAnswer, walletId} from './customerClient.ts';
import {isOpenOrder, orderRecords, type OpenOrder, type OrderItem} from './orderRecords.ts';
import {useScreenProtection} from '../ui/screenProtection.ts';
import {FormerOrderDetail, isFormerBoard, OpenOrderCard, OrderDetail} from './OpenOrders.tsx';
import {OrderPayment, type OrderBlock, type OrderBoard, type OrderPaymentView} from './OrderPayment.tsx';
import {configuredEntrance, issuePickupQr, shortAccount} from './customerEntrance.ts';
import {isPinnedPolicy} from './orderPaymentTrust.ts';
import {madeAtStorePage, openStoreAccess, SessionNeeded, StoreBusy, type AccessProblem, type StoreAccess, type StoreChoice} from './storeAccess.ts';
import type {StoreVisits} from './storeVisit.ts';
import {ownerPickupBoard} from './pickupBoard.ts';
import {PickupQrScreen} from './PickupQr.tsx';
import {cartMoney, missingUnits, orderMoney, receiptLines, shownBalance} from '../wallet/tokens.ts';

type RentalBoardReview = RentalReviewChallenge & {
  checkpoint: string;
  terminal: NearbyCommerceLink;
  expiresAt: number;
} & ({verified: false} | {verified: true; receipt: string; authority: RentalReviewAuthority});

type ApprovalDevice = {
  handle: string;
  request: string;
  scope: MaintenanceScope;
  enrollment: Enrollment;
  address: string;
};

type Checkpoint = {
  version: 2;
  branch: string;
  subject: string;
  key: string;
  asset: string;
  cart: CommerceCart;
  route: 'direct' | 'nearby';
  terminal?: string;
  deadline?: number;
  selection?: CommerceSelection;
  order?: string;
  /** When the store stops holding this order's stock unless it is paid (seconds), as the store last said. */
  holdUntil?: number;
};
const orderStateLabel: Record<CommerceOrder['state'], string> = {
  preview: '주문 확인 대기', issued: '견적 발행', checking: '지급 확인 중',
  paid: '지급 확인', closed: '주문 종료',
};
const settlementLabel: Record<CommerceOrderPayment['stage'], string> = {
  received: '매장이 지급 요청을 받았습니다.',
  relaySigned: '릴레이 서명이 완료되었습니다. 네트워크 제출을 기다립니다.',
  submitted: '원거래가 네트워크에 제출되었습니다.',
  checking: '같은 거래의 지급 결과 확인 중',
  finalized: '주문 원장과 체인 지급 결과가 일치합니다.',
  unpaid: '최종 미지급이 확인되었습니다.',
};

const fulfillmentLabel: Record<NonNullable<CommerceOrderPayment['pickupStage']>, string> = {
  preparing: '픽업 준비 중',
  ready: '픽업 준비 완료',
  collected: '픽업 수령 완료',
  unfulfillable: '매장에서 픽업을 이행할 수 없습니다.',
};

const reviewStopLabel: Record<PaymentReviewStop, string> = {
  cancelled: '이 기기의 승인을 종료했습니다.',
  expired: '원견적이 만료되었습니다.',
  interrupted: '보드 연결이 끊겼습니다.',
  changed: '견적이 변경되었습니다. 새 견적을 확인하세요.',
  refused: '보드에서 결제를 거절했습니다.',
};
/** The server's list has no time; an order this phone sent has one. */
function orderRowDetail(row: CustomerOrderSummary, sent?: OpenOrder) {
  const state = sent?.stage ? openOrderLabel(sent) : orderStateLabel[row.state];
  return sent ? `${new Date(sent.sentAt * 1000).toLocaleString('ko-KR', {month: 'long', day: 'numeric', hour: '2-digit', minute: '2-digit'})} · ${state}` : state;
}
/**
 * Why an order at a store reached without a registration cannot go on, in the customer's words. `kept`: the page
 * came back to a payment it had kept, so the order was started before.
 */
const storeUnreached = '매장 서버에 연결하지 못했습니다. 연결을 확인하고 다시 시도하세요.';
function accessProblemText(problem: AccessProblem | {kind: 'no-build'}, kept = false) {
  switch (problem.kind) {
    case 'no-build': return '이 앱에는 주문을 받을 매장 서버와 주문 계약이 아직 정해지지 않았습니다.';
    case 'no-account': return '주문하려면 먼저 지갑 탭에서 보드를 연결하고 서명 시험을 마치세요.';
    case 'no-board': return '결제할 보드를 찾지 못했습니다. 보드를 켜고 휴대전화 가까이 둔 뒤 다시 시도하세요.';
    case 'other-board': return `가까이 있는 보드(${shortAccount(problem.board)})는 이 휴대전화의 보드(${shortAccount(problem.expected)})가 아닙니다. 내 보드를 가까이 두고 다시 시도하세요.`;
    case 'board-changed': return '보드 등록이 바뀌었습니다. 지갑 탭의 보드 설정에서 서명 시험을 다시 한 뒤 주문하세요.';
    case 'held': return '이 보드 계정에 끝나지 않은 주문이 있어 지금은 새로 시작할 수 없습니다. 진행 중인 주문을 먼저 확인하세요.';
    case 'busy': return '요청이 많습니다. 잠시 뒤 다시 시도하세요.';
    // The board near is this phone's own and proved its account; the server still did not accept it for what was asked.
    case 'mismatch': return '매장 서버가 이 보드 계정의 확인을 받아들이지 않았습니다. 주문 상태를 다시 확인하고, 같은 안내가 나오면 매장에 문의하세요.';
    case 'refused': return '매장 서버가 이 요청을 받지 않았습니다. 다시 시도해도 같으면 매장에 문의하세요.';
    // The board proved the paying account and the server still said nothing of the order; the board rests for a while.
    case 'order-unread': return '매장 서버가 이 주문의 상태를 지금 알려 주지 않습니다. 아래는 이 휴대전화가 마지막으로 확인한 상태입니다. 몇 분 뒤 다시 확인하세요.';
    case 'payment': return problem.code === 'NO_MERCHANT' ? '이 매장은 아직 앱에서 주문 결제를 받지 않습니다.'
      : problem.code === 'CHAIN_UNREADABLE' ? (kept ? '결제 정보를 체인에서 다시 확인하지 못했습니다. 연결을 확인하고 다시 시도하세요.'
        : '결제 정보를 체인에서 확인하지 못해 주문을 시작하지 않았습니다. 잠시 뒤 다시 시도하세요.')
      : '이 매장의 결제 정보가 이 앱의 주문 계약과 맞지 않아 주문할 수 없습니다. 운영자에게 문의하세요.';
    default: return storeUnreached;
  }
}
// What a basket lacks and holds, in the unit its screen shows: the phone's one rule for whole won (wallet/tokens.ts).
export {missingUnits, shownBalance};
const now = () => Math.floor(Date.now() / 1000);
/** How long after the hold's end the phone takes the hold for over: its clock and the store's may differ by this much. */
const holdMargin = 120;
/**
 * Whether the store's hold of the order on this page is over by this phone's clock: the time the store named for it,
 * and the time of a quote this phone holds for it, both passed more than two minutes ago. Without a known time it is
 * not. A clock that is further ahead than that says "over" too early, so the store is asked first whenever it can be
 * (startNew), and this alone never lets the page say that the store closed the order.
 */
export function holdOver(point: {order?: string; holdUntil?: number} | undefined, payment: {order: {id: string}; quote: {expiresAt: number}} | undefined, at: number): boolean {
  if (!point?.order || point.holdUntil === undefined) return false;
  const quoted = payment?.order.id === point.order ? payment.quote.expiresAt : 0;
  return Math.max(point.holdUntil, quoted) + holdMargin < at;
}
const identifier =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

function invoiceCanApprove(order: CommerceOrder) {
  const invoice = order.subscription;
  const stage = invoice?.stage ?? order.state;
  const deadline = invoice?.quoteExpiresAt ?? invoice?.dueAt ?? order.expiresAt;
  return (stage === 'preview' || stage === 'issued') && Date.parse(deadline) > Date.now();
}

function invoiceStatus(order: CommerceOrder) {
  switch (order.subscription?.stage ?? order.state) {
    case 'expired': return '견적 만료가 확정되었습니다. 기한 내 지급이 없었습니다.';
    case 'reconcile': return '기한 내 지급 여부를 대사 중입니다. 추가 제출은 닫혔습니다.';
    case 'awaitingClosure': return '납기가 지났습니다. 이 회차의 종료를 확인 중입니다.';
    case 'closed': return '회차가 종료되었습니다.';
    case 'paid': return '서버 지급 확인';
    case 'checking': return '지급 결과 확인 중';
    default: return '회차가 연결되었습니다. 보드 승인 대기 중입니다.';
  }
}

function Button({
  label,
  run,
  disabled = false,
  variant = 'primary',
  detail,
  testID,
}: {
  label: string;
  run: () => void;
  disabled?: boolean;
  variant?: ButtonVariant;
  detail?: string;
  testID?: string;
}) {
  return <KitButton testID={testID} label={label} run={run} disabled={disabled} variant={variant} detail={detail} />;
}

function isMenuNetworkOffline(error: unknown) {
  return Boolean(error && typeof error === 'object' &&
    'code' in error && error.code === 'COMMERCE_NETWORK_OFFLINE');
}

function checkpoint(raw: string, client: CommerceClient) {
  if (!raw) return undefined;
  const point = JSON.parse(raw) as Checkpoint;
  if (
    point.version !== 2 ||
    point.branch !== client.profile.branchRef ||
    point.subject !== client.profile.subjectRef ||
    !['direct', 'nearby'].includes(point.route) ||
    !identifier.test(point.key) ||
    (point.terminal !== undefined && (point.route !== 'nearby' || !identifier.test(point.terminal))) ||
    (point.deadline !== undefined && (point.route !== 'nearby' || !Number.isSafeInteger(point.deadline) || point.deadline < 1)) ||
    (point.order !== undefined && !identifier.test(point.order)) ||
    (point.holdUntil !== undefined && (point.order === undefined || !Number.isSafeInteger(point.holdUntil) || point.holdUntil < 1)) ||
    !/^0x[0-9a-f]{40}$/.test(point.asset)
  )
    throw new Error('ORDER_CHECKPOINT');
  encodeCommerceCart(point.cart);
  return point;
}

type PickupCartReview = CommerceCartReview & {
  pickupChanges?: {previous: CommercePrice; current: CommercePrice}[];
};

type OrderOutcome = {sale: string; store: string; amount: string} & ({kind: 'done'; number: number} | {kind: 'failed'});

/** The lines of a receipt. The tax is the one inside each line's price, when the store said it. */
function receiptItems(selection: CommerceSelection, policy: CommercePaymentProfile, taxFreeShop?: string): OrderItem[] | undefined {
  const lines = Object.values(selection);
  if (!lines.length || lines.length > 20) return undefined;
  return lines.map(({row, quantity}) => {
    const line = commerceLineTax(row, quantity, policy);
    return {name: commerceMenuLabels(row, 'ko').name.slice(0, 200), quantity, total: (BigInt(row.unitPrice) * BigInt(quantity)).toString(),
      ...(line ? {tax: line.tax.toString(), taxKind: row.metadata!.tax!} : {}),
      ...(taxFreeShop && line && row.metadata?.tax === 'vat10' && row.metadata.takeAway && row.metadata.taxRefund ? {refund: true as const} : {})};
  });
}

export function CustomerOrders({
  back,
  tabbed = false,
  openBoard,
  initialOrder,
  openTravel,
  openPayments,
  permission,
  initialLocator,
  initialCounterQr,
  initialShopQr,
  shopEntry,
  openShopQr,
  travelCheckout,
  subscriptionLink,
  store,
  storeVisits,
  openStorePage,
  storeRoute,
}: {
  /**
   * Where the app keeps what the customer left at a store page: the basket comes back from it when the page opens
   * again, and the page writes down what it holds (storeVisit.ts). Without it a page starts empty every time.
   */
  storeVisits?: StoreVisits;
  /** Opens a store's page from its shortcut QR; undefined unless the build takes the order from that page. */
  openStorePage?: (branch: string) => void;
  /**
   * Given only in a build that orders without a registration (orderRoute.ts): the store finder, and the page of one
   * store with one of its orders first. With it this screen shows no registration form and no way to one.
   */
  storeRoute?: {find: () => void; open: (store: StoreChoice, sale: string) => void};
  /**
   * A store picked from its store page. The phone orders there through the customer entrance without registering:
   * the menu is public, and the board proves its account when the order is asked for (storeAccess.ts).
   */
  store?: StoreChoice;
  /** Inside the app tabs the 결제 tab replaces the separate back button. */
  tabbed?: boolean;
  openBoard?: () => void;
  /** An order in progress to open first. */
  initialOrder?: string;
  back: () => void;
  openTravel?: (booking: TravelBookingScope) => void;
  openPayments?: (qr?: string) => void;
  permission: () => Promise<boolean>;
  initialLocator: string;
  initialCounterQr?: string;
  initialShopQr?: string;
  shopEntry?: boolean;
  openShopQr?: (raw: string) => void;
  travelCheckout?: { branch: string; sale?: string; bookingPayment?: TravelBookingPayment; pickup?: true | {window: string; startsAt: string; label: string} };
  subscriptionLink?: string;
}) {
  const [phase, setPhase] = useState<'check' | 'setup' | 'active' | 'locked'>(
    'check',
  );
  const [profile, setProfile] = useState('');
  const [pin, setPin] = useState('');
  const [proofPin, setProofPin] = useState('');
  const serverPinInput = useRef<ComponentRef<typeof TextInput>>(null);
  const presentationPinInput = useRef<ComponentRef<typeof TextInput>>(null);
  const [pub, setPub] = useState('');
  const [journals, setJournals] = useState<ReturnType<typeof customerJournals>>();
  const [client, setClient] = useState<CommerceClient>();
  const [showSubscriptions, setShowSubscriptions] = useState(
    Boolean(subscriptionLink),
  );
  useEffect(() => {
    if (subscriptionLink) setShowSubscriptions(true);
  }, [subscriptionLink]);
  const [menuLocale, setMenuLocale] = useState<'ko' | 'en'>('ko');
  const [catalog, setCatalog] = useState<CommercePrice[]>([]);
  // What the store said about taking orders, read with the menu from the server; refused marks a payment it stopped.
  const [storeState, setStoreState] = useState<{state?: CommerceOrderState; refused?: boolean; checkedAt?: string}>({});
  const [menuCategory, setMenuCategory] = useState('');
  const [menuReadOnly, setMenuReadOnly] = useState(false);
  const menuCheckedAt = useRef<number | undefined>(undefined);
  const categories = shopMenuCategories(catalog);
  const selectedCategory = categories.some(({key}) => key === menuCategory) ? menuCategory : '';
  const [cart, setCart] = useState<CommerceSelection>({});
  const [choosing, setChoosing] = useState('');
  // The menu may come next to the kiosk, but photos are read from the server; without it the page shows names.
  const photos = useMemo(() => client ? new CommercePhotoCache(name => client.catalogPhoto(name)) : undefined, [client]);
  const [cartReview, setCartReview] = useState<PickupCartReview>();
  const [point, setPoint] = useState<Checkpoint>();
  const [nearbyKey, setNearbyKey] = useState('');
  const [nearbyEnrollment, setNearbyEnrollment] = useState('');
  const [shop, setShop] = useState<ShopMatch>();
  const shopBranch = shop?.identity.branch;
  useEffect(() => setChoosing(''), [shopBranch, point]);
  const [shopOffline, setShopOffline] = useState(false);
  const [shopPermissionDenied, setShopPermissionDenied] = useState(false);
  const [entrancePending, setEntrancePending] = useState(Boolean(initialShopQr));
  const pendingShopLink = useRef<NearbyCommerceLink>(undefined);
  const [link, setLink] = useState<NearbyCommerceLink>();
  const [pair, setPair] = useState<{
    order: string;
    key: string;
    code?: string;
  }>();
  const adapter = useRef<CustomerNearby>(undefined);
  const [locator, setLocator] = useState(initialCounterQr ?? '');
  const [ticket, setTicket] = useState<{
    order: string;
    key: string;
    code?: string;
  }>();
  const [approved, setApproved] = useState<PaymentCheckpoint>();
  const [reviewDetails, setReviewDetails] = useState(false);
  const [recipientQuote, setRecipientQuote] = useState<string>();
  const [reviewWaiting, setReviewWaiting] = useState(false);
  const [boardVerified, setBoardVerified] = useState<string>();
  const [rentalBoardReview, setRentalBoardReview] = useState<RentalBoardReview>();
  const rentalContext = useRef({link, approved, order: undefined as CommerceOrder | undefined});
  const [approvalDevice,setApprovalDevice]=useState<ApprovalDevice>();
  const [approvalProgress,setApprovalProgress]=useState<OwnerApprovalProgress>();
  const [showApprovalMaintenance,setShowApprovalMaintenance]=useState(false);
  const [reviewClock, setReviewClock] = useState(now);
  const boardReview = useRef<object | undefined>(undefined);
  const [policy, setPolicy] = useState<CommercePaymentProfile>();
  // A store page's policy is confirmed from the chain in the middle of a step and is then a new object, while the
  // step that is running still holds the one its page was drawn with. The two differ only in who is paid (the
  // merchant record), so whatever checks a payment reads the policy here, at the time of the check.
  const policyNow = useRef<CommercePaymentProfile | undefined>(undefined);
  const keepPolicy = useCallback((next?: CommercePaymentProfile) => { policyNow.current = next; setPolicy(next); }, []);
  const trusted = (drawn: CommercePaymentProfile) => policyNow.current ?? drawn;
  const storeBranch = store?.branch, storeName = store?.name, storeMerchant = store?.merchantId;
  const registrationFree = !!storeRoute;
  const [storeAccess, setStoreAccess] = useState<StoreAccess>();
  // The access whose page has taken back the basket left at its store; from then on the page writes its own down.
  const [basketOf, setBasketOf] = useState<StoreAccess>();
  useEffect(() => {
    if (!storeAccess || basketOf !== storeAccess) return;
    storeVisits?.keep(storeAccess.store.branch, {basket: point ? undefined : cart, checkout: !!point || !!approved});
  }, [storeVisits, storeAccess, basketOf, cart, point, approved]);
  const [storeProblem, setStoreProblem] = useState<AccessProblem | {kind: 'no-build'}>();
  // The board account's balance of the store's token: undefined before it is read, null when it cannot be read.
  const [boardBalance, setBoardBalance] = useState<bigint | null>();
  // The store's server read the balance from the chain and did not hold the cart: what the cart costs and what the
  // account held then, in base units. It is about that basket only, and stands until the balance is read again.
  const [shortHold, setShortHold] = useState<{required: bigint; balance: bigint; basket: CommerceSelection}>();
  // An order paid into the escrow takes the whole page until the owner closes it.
  const [orderFocus, setOrderFocus] = useState(true);
  const [orderBoard, setOrderBoard] = useState<{request: string; handle?: string; view?: OrderBoard; block?: OrderBlock; pinWait?: number}>();
  const [orderBlocked, setOrderBlocked] = useState<{block: OrderBlock; sale: CommerceOrder; until?: number; spent?: boolean}>();
  const [orderOutcome, setOrderOutcome] = useState<OrderOutcome>();
  const [orderSilent, setOrderSilent] = useState(false);
  const [openOrders, setOpenOrders] = useState<OpenOrder[]>([]);
  const page = useRef<React.ComponentRef<typeof ScrollView>>(null);
  const [detailSale, setDetailSale] = useState(initialOrder ?? '');
  // The ready order whose pickup QR is on screen.
  const [pickupSale, setPickupSale] = useState('');
  const [stranded, setStranded] = useState<OpenOrder[]>([]);
  // An order opened from outside that was paid with a board account this phone no longer holds (FormerOrderDetail).
  const [formerOrder, setFormerOrder] = useState<OpenOrder>();
  const [boardAccount, setBoardAccount] = useState<{address: string; generation: number} | null>();
  const [boards, setBoards] = useState<{ handle: string; name: string }[]>([]);
  const ownerConnection = useRef<OwnerSession>(undefined);
  const [travelOrderObservedAt, setTravelOrderObservedAt] = useState<number>();
  const [order, setOrder] = useState<CommerceOrder>();
  rentalContext.current = {link, approved, order};
  const [unpaidNearby, setUnpaidNearby] = useState<CommerceOrder>();
  const [pendingNearby, setPendingNearby] = useState<{order: CommerceOrder; branch: string; label?: string}>();
  const [lookupOrder, setLookupOrder] = useState<string>();
  const [nearbySoldOut, setNearbySoldOut] = useState<CommerceCartLine[]>();
  const [invoiceDetail, setInvoiceDetail] = useState(false);
  const [message, setMessage] = useState('');
  // The board has no screen: show its step while the payment approval waits on it.
  const [boardWaiting, setBoardWaiting] = useState(false);
  const [orderRows, setOrderRows] = useState<CustomerOrderSummary[]>([]);
  const [invoiceRows, setInvoiceRows] = useState<SubscriptionInvoice[]>([]);
  const [historyState, setHistoryState] = useState<'loading' | 'ready' | 'unavailable'>('loading');
  const [moreOrders, setMoreOrders] = useState(false);
  const [deniedLink, setDeniedLink] = useState(false);
  const [counterQrAllowed, setCounterQrAllowed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [stepsDone, setStepsDone] = useState(0);
  const performing = useRef<object | undefined>(undefined);
  const mounted = useRef(true);

  const loadRevision = useRef(0);
  const registration = useRef<string | undefined>(undefined);
  const invalidateLoad = useCallback(() => {
    loadRevision.current++;
    performing.current = undefined;
  }, []);
  const travelBranch = travelCheckout?.branch;
  const travelSale = travelCheckout?.sale;
  const travelPayment = travelCheckout?.bookingPayment;
  const matchesTravelOrder = useCallback((view: CommerceOrder) => !travelPayment || (
    view.id === travelSale && view.asset === travelPayment.booking.asset &&
    view.amount === travelPayment.booking[travelPayment.kind] && !view.subscription &&
    (view.payment === undefined || travelPayment.booking.chainId === undefined ||
      view.payment.chainId === travelPayment.booking.chainId)
  ), [travelPayment, travelSale]);
  const pickupMode = travelCheckout?.pickup !== undefined;
  const pickupChoice = travelCheckout?.pickup === true ? undefined : travelCheckout?.pickup;
  const loadHistory = useCallback(async (actor: CommerceClient, revision: number) => {
    setHistoryState('loading');
    const [merchant, invoices] = await Promise.allSettled([actor.orders(), actor.invoices()]);
    if (!mounted.current || revision !== loadRevision.current) return;
    setOrderRows(merchant.status === 'fulfilled' ? merchant.value : []);
    setMoreOrders(merchant.status === 'fulfilled' && merchant.value.length === 50);
    setInvoiceRows(invoices.status === 'fulfilled' ? invoices.value : []);
    setHistoryState(merchant.status === 'fulfilled' && invoices.status === 'fulfilled' ? 'ready' : 'unavailable');
  }, []);

  const load = useCallback(async () => {
    const revision = ++loadRevision.current;
    const current = () => mounted.current && revision === loadRevision.current;
    setPhase('check');
    registration.current = undefined;
    performing.current = undefined;
    boardReview.current = undefined;
    ownerConnection.current?.close();
    ownerConnection.current = undefined;
    setBusy(false);
    setClient(undefined);
    setJournals(undefined);
    setApproved(undefined);
    setReviewDetails(false);
    setReviewWaiting(false);
    setApprovalDevice(undefined);
    setApprovalProgress(undefined);
    setShowApprovalMaintenance(false);
    setReviewClock(now());
    setPoint(undefined);
    setNearbySoldOut(undefined);
    setTicket(undefined);
    keepPolicy(undefined);
    setOrderFocus(true);
    setOrderBoard(undefined);
    setOrderBlocked(undefined);
    setOrderOutcome(undefined);
    setOrderSilent(false);
    setOpenOrders([]);
    setStranded([]);
    setFormerOrder(undefined);
    setCatalog([]);
    setMenuCategory('');
    setMenuReadOnly(false);
    menuCheckedAt.current = undefined;
    setCart({});
    setCartReview(undefined);
    adapter.current?.close();
    adapter.current = undefined;
    setLink(undefined);
    pendingShopLink.current = undefined;
    setShop(undefined);
    setShopOffline(false);
    setShopPermissionDenied(false);
    setPair(undefined);
    setOrder(undefined);
    setPendingNearby(undefined);
    setUnpaidNearby(undefined);
    setLookupOrder(undefined);
    setInvoiceDetail(false);
    setOrderRows([]);
    setInvoiceRows([]);
    setHistoryState('loading');
    setMoreOrders(false);
    setDeniedLink(false);
    setCounterQrAllowed(false);
    setMessage('');
    setStoreAccess(undefined);
    setBasketOf(undefined);
    setStoreProblem(undefined);
    setBoardBalance(undefined);
    setShortHold(undefined);
    // Without a working registration the phone still knows its open orders and can say how to reach them.
    const showStranded = () => {
      if (Http) orderRecords(Http).list().then(rows => { if (current()) setStranded(rows.filter(isOpenOrder)); }).catch(() => {});
    };
    try {
      if (!Http) {
        setPhase('locked');
        return;
      }
      if (storeBranch && storeName) {
        // No registration takes part: the menu is a public read, and nothing is asked of a board until the order.
        const access = await openStoreAccess({branch: storeBranch, name: storeName, merchantId: storeMerchant}, ownerPickupBoard);
        if (!current()) return;
        if (!access) {
          setStoreProblem({kind: 'no-build'});
          setPhase('locked');
          return;
        }
        const actor = access.client;
        // The order this page was opened for may belong to a board account the phone no longer holds. Its session
        // needs that account's proof, so it is shown as last written down, and nothing is asked for it. A wallet that
        // could not be read says nothing about which account the phone holds; the order then opens the usual way.
        const asked = initialOrder ? (await orderRecords(Http).list().catch(() => [])).find(row => row.sale === initialOrder && row.branch === storeBranch) : undefined;
        const holder = asked && madeAtStorePage(asked) && OwnerWallet
          ? await OwnerWallet.loadProfile().then(wallet => decodeWalletStorage(wallet).verified?.address ?? null, () => undefined) : undefined;
        const former = asked && holder !== undefined && holder !== asked.payer ? asked : undefined;
        const pending = checkpoint(await access.journals.read('order'), actor);
        const payment = paymentCheckpoint(await access.journals.read('payment'), actor.profile.branchRef, actor.profile.subjectRef);
        // A payment kept from before was checked against the store's record in the order contract. The record is
        // read again here, with no board near, so the page goes on with the policy that payment was checked against.
        const unconfirmed = payment ? await access.confirm() : undefined;
        if (!current()) return;
        keepPolicy(access.policy);
        if (unconfirmed) setStoreProblem(unconfirmed);
        setFormerOrder(former);
        setJournals(access.journals);
        setApproved(payment);
        setPoint(pending);
        setClient(actor);
        setStoreAccess(access);
        setShop({identity: {branch: storeBranch, branchLabel: storeName}, route: 'direct'});
        setEntrancePending(false);
        setPhase('active');
        registration.current = access.id;
        if (OwnerWallet) OwnerWallet.loadProfile().then(wallet => {
          if (current()) setBoardAccount(decodeWalletStorage(wallet).verified ?? null);
        }).catch(() => {});
        // An order with a session of its own is read again; the others keep what the phone last wrote down.
        void refreshOpenOrders(actor).then(rows => { if (current()) setOpenOrders(rows); }).catch(() => {});
        void access.balance().then(value => { if (current()) setBoardBalance(value ?? null); });
        if (pending) setBasketOf(access);
        else {
          try {
            const menu = await actor.publishedMenu();
            if (!current()) return;
            const at = new Date(now() * 1000);
            menuCheckedAt.current = now();
            setCatalog(menu.catalog);
            if (menu.shop) setShop({identity: menu.shop, route: 'direct'});
            setStoreState({state: menu.orderState, checkedAt: `${String(at.getHours()).padStart(2, '0')}:${String(at.getMinutes()).padStart(2, '0')}`});
            // The basket the customer left here (to top up, or for another tab) comes back, read against the menu
            // as it is now: a price that changed or a product that sold out meanwhile is said before any order.
            const left = storeVisits?.basket(storeBranch);
            if (left) {
              const review = reviewCommerceCart(left, menu.catalog);
              setCart(review.selection);
              if (review.priceChanges.length || review.shortages.length) setCartReview(review);
            }
            setBasketOf(access);
          } catch (error) {
            if (!current()) return;
            if (error instanceof StoreBusy) setStoreProblem({kind: 'busy'});
            else setMessage(error instanceof Error && error.message === 'STORE_NOT_PUBLIC'
              ? '이 매장을 찾지 못했습니다. 매장이 문을 닫았거나, 이 앱에서 주문을 받지 않는 매장입니다.'
              : '메뉴를 불러오지 못했습니다. 연결을 확인하고 다시 시도하세요.');
          }
        }
        return;
      }
      const raw = await Http.loadProfile('customer');
      if (!current()) return;

      if (!raw) {
        if (initialLocator) {setDeniedLink(true);setPhase('active');return;}
        registration.current = raw;
        setPhase('setup');
        showStranded();
        if (registrationFree && OwnerWallet) OwnerWallet.loadProfile().then(wallet => {
          if (current()) setBoardAccount(decodeWalletStorage(wallet).verified ?? null);
        }).catch(() => {});
        return;
      }
      const actor = customerClient(raw);
      if (
        travelBranch !== undefined &&
        (travelBranch !== actor.profile.branchRef || travelPayment &&
          (travelPayment.scope.customer !== actor.profile.subjectRef || travelPayment.scope.tenant !== actor.profile.tenantRef))
      ) {
        setPhase('locked');
        return;
      }
      const scopedJournals = customerJournals(raw, actor.profile);
      if (initialLocator && !travelBranch) {
        try {
          const located = parseOrderLocator(initialLocator, now());
          if (located.branchRef !== actor.profile.branchRef) throw new Error('WRONG_BRANCH');
          if (current()) setCounterQrAllowed(true);
          const detail = await actor.status(located.orderRef);
          const payment = paymentCheckpoint(
            await scopedJournals.read('payment'), actor.profile.branchRef, actor.profile.subjectRef,
          );
          if (await Http.loadProfile('customer') !== raw) throw new Error('COMMERCE_PROFILE_CHANGED');
          if (parseOrderLocator(initialLocator, now()).orderRef !== detail.id)
            throw new Error('WRONG_ORDER');
          if (current()) {
            keepPolicy(paymentProfile(raw));
            setOrder(detail);
            setClient(actor);
            setJournals(scopedJournals);
            setApproved(payment?.order.id === detail.id ? payment : undefined);
          }
        } catch {
          if (current()) {
            setDeniedLink(true);
            try { parseOrderLocator(initialLocator, now()); }
            catch { setCounterQrAllowed(false); }
          }
        }
        if (current()) setPhase('active');
        return;
      }
      const config = JSON.parse(raw) as { nearby?: unknown };
      const pending = checkpoint(await scopedJournals.read('order'), actor);
      let payment = paymentCheckpoint(
        await scopedJournals.read('payment'),
        actor.profile.branchRef,
        actor.profile.subjectRef,
      );
      if (travelPayment && payment && payment.order.id !== travelSale) {
        // An earlier leg can settle while the app is closed. Keep unresolved
        // work protected, but release its payment checkpoint once P05 closes it.
        const earlier = await actor.status(payment.order.id).catch(() => undefined);
        if (!current()) return;
        if (earlier?.id === payment.order.id && earlier.asset === payment.order.asset &&
            earlier.amount === payment.order.amount && ['paid', 'closed'].includes(earlier.state)) {
          await scopedJournals.write('payment', undefined);
          payment = undefined;
        }
      }
      const kept = await scopedJournals.read('qr');
      if (!current()) return;
      const liveProfile = await Http.loadProfile('customer');
      if (!current()) return;
      if (liveProfile !== raw) throw new Error('COMMERCE_PROFILE_CHANGED');
      setNearbyEnrollment(config.nearby ? JSON.stringify(config.nearby) : '');
      if (kept) {
        const value = JSON.parse(kept) as {
          branch: string;
          subject: string;
          order: string;
          key: string;
          code?: string;
        };
        if (
          value.branch !== actor.profile.branchRef ||
          value.subject !== actor.profile.subjectRef ||
          !identifier.test(value.order) ||
          !identifier.test(value.key) ||
          (value.code !== undefined && !/^[0-9a-f]{8}$/.test(value.code))
        )
          throw new Error('QR_JOURNAL');
        setTicket(value);
      } else setTicket(undefined);
      keepPolicy(paymentProfile(raw));
      setJournals(scopedJournals);
      setApproved(payment);
      setPoint(pending);
      setClient(actor);
      setPhase('active');
      registration.current = raw;
      if (!travelBranch && OwnerWallet) OwnerWallet.loadProfile().then(wallet => {
        if (current()) setBoardAccount(decodeWalletStorage(wallet).verified ?? null);
      }).catch(() => {});
      if (!travelBranch) void refreshOpenOrders(actor).then(rows => {
        if (current()) setOpenOrders(rows);
      }).catch(() => {});
      if (!travelBranch && !subscriptionLink && !shopEntry) await loadHistory(actor, revision);
      if (travelSale) {
        if (!identifier.test(travelSale)) throw new Error('TRAVEL_ORDER');
        if (payment && payment.order.id !== travelSale) {
          setMessage('먼저 다른 주문의 보류된 지급을 확인하세요.');
          if (!travelPayment) return;
        }
        if (travelPayment) setLookupOrder(travelSale);
        try {
          const result = await actor.status(travelSale);
          if (!matchesTravelOrder(result)) throw new Error('TRAVEL_ORDER');
          if (current()) {
            setOrder(result);
            if (travelPayment) setTravelOrderObservedAt(Date.now());
          }
        } catch (error) {
          if (!travelPayment) throw error;
          if (current()) setMessage('네트워크 응답 또는 원주문을 확인할 수 없습니다. 같은 주문을 다시 조회하세요.');
        }
      }
    } catch {
      if (current()) { setPhase('locked'); showStranded(); }
    }
  }, [initialLocator, loadHistory, shopEntry, subscriptionLink, travelBranch, travelSale, travelPayment, matchesTravelOrder, storeBranch, storeName, storeMerchant, keepPolicy,
    initialOrder, registrationFree, storeVisits]);

  useEffect(() => {
    mounted.current = true;
    const sub = AppState.addEventListener('change', state => {
      if (state === 'active') {
        const revision = ++loadRevision.current;
        // A store page's order keeps its basket while the app is away; it holds no registration to read again.
        if (storeBranch) {
          if (registration.current) setPhase('active');
          else load();
          return;
        }
        Http?.loadProfile('customer').then(raw => {
          if (!mounted.current || revision !== loadRevision.current) return;
          if (raw === registration.current && !initialLocator && !subscriptionLink) setPhase(raw ? 'active' : 'setup');
          else load();
        }).catch(() => {
          if (mounted.current && revision === loadRevision.current) setPhase('locked');
        });
        return;
      }
      invalidateLoad();
      setPhase('check');
      setBusy(false);
      boardReview.current = undefined;
      setReviewWaiting(false);
      setReviewDetails(false);
      setRentalBoardReview(undefined);
      setApprovalDevice(undefined);
      setApprovalProgress(undefined);
      setShowApprovalMaintenance(false);
      ownerConnection.current?.close();
      ownerConnection.current = undefined;
      setBoards([]);
      adapter.current?.close();
      adapter.current = undefined;
      setLink(undefined);
      setPair(undefined);
      pendingShopLink.current = undefined;
      setShop(current => current?.route === 'relay' ? undefined : current);
    });
    return () => {
      mounted.current = false;
      adapter.current?.close();
      ownerConnection.current?.close();
      sub.remove();
    };
  }, [initialLocator, invalidateLoad, load, subscriptionLink, storeBranch]);

  useEffect(() => {
    load();
    return invalidateLoad;
  }, [load, invalidateLoad]);

  useEffect(() => {
    if (!approved || approved.reviewStop) return;
    const sponsorDeadline = needsMerchantSponsorship(approved) ? approved.quote.presentation?.gasSponsor?.expiresAt : undefined;
    const liveDeadlines = [approved.quote.expiresAt, sponsorDeadline, approved.quote.presentation?.rentalEscrow?.expiresAt].filter(
      (deadline): deadline is number => typeof deadline === 'number' && Number.isSafeInteger(deadline) && deadline * 1000 > Date.now(),
    );
    if (!liveDeadlines.length) return;
    const timer = setTimeout(() => {
      setReviewClock(now());
      ownerConnection.current?.close();
    }, Math.min(...liveDeadlines) * 1000 - Date.now());
    return () => clearTimeout(timer);
  }, [approved, reviewClock]);

  useEffect(() => {
    if (!rentalBoardReview || rentalBoardReview.terminal !== link) return;
    const delay = rentalBoardReview.expiresAt * 1000 - Date.now();
    const timer = setTimeout(() => {
      setRentalBoardReview(current => current === rentalBoardReview ? undefined : current);
    }, Math.max(0, delay));
    return () => clearTimeout(timer);
  }, [rentalBoardReview, link]);

  useEffect(() => {
    if (!approvalProgress) return;
    const expiresIn = Number(approvalProgress.expires) * 1000 - Date.now();
    const timer = setTimeout(() => setApprovalProgress(current => current === approvalProgress ? undefined : current),
      Math.max(0, expiresIn));
    return () => clearTimeout(timer);
  }, [approvalProgress]);

  useEffect(() => {
    // Messages are drawn above the page; a button far below must bring them into view.
    if (message) page.current?.scrollTo({y: 0, animated: true});
  }, [message, stepsDone]);
  useEffect(() => {
    // So is what a store page says about the board, the account or the store's server: the answer to "주문 접수 요청"
    // at the end of a long menu must not stay out of sight above it.
    if (storeProblem) page.current?.scrollTo({y: 0, animated: true});
  }, [storeProblem]);
  // An order the store now holds is drawn at the top as well, in place of the basket the button was under.
  const heldOrder = point?.order;
  useEffect(() => {
    if (heldOrder && storeBranch) page.current?.scrollTo({y: 0, animated: true});
  }, [heldOrder, storeBranch]);

  const closeDetail = () => { setDetailSale(''); setPickupSale(''); setOrder(undefined); setLookupOrder(undefined); setMessage(''); };
  const detailShown = useRef(false);
  detailShown.current = false;
  useEffect(() => {
    if (!detailSale && !order) return;
    const backAction = BackHandler.addEventListener('hardwareBackPress', () => {
      if (detailShown.current) closeDetail();
      return detailShown.current;
    });
    return () => backAction.remove();
  }, [detailSale, order]);

  useEffect(() => {
    if (!showApprovalMaintenance) return;
    const backAction = BackHandler.addEventListener('hardwareBackPress', () => {
      setShowApprovalMaintenance(false);
      return true;
    });
    return () => backAction.remove();
  }, [showApprovalMaintenance]);

  function canReplaceReview(sale: string) {
    return !approved || (sale === approved.order.id && !boardReview.current &&
      paymentReviewStop(approved, now()) !== undefined);
  }

  /** Takes over the policy the store access confirmed, for the next drawing and for the step that is running. */
  function adoptStorePolicy() {
    if (storeAccess && storeAccess.policy !== policyNow.current) keepPolicy(storeAccess.policy);
  }

  /**
   * A kept payment of a store page is used only with the policy it was checked against: the store's record in the
   * order contract. When that could not be read as the page came back, it is read again here, with no board. False
   * when it still cannot be read; the page then says so and the step does not run.
   */
  async function storePolicyReady() {
    if (!storeAccess || isPinnedPolicy(policyNow.current)) return true;
    const revision = loadRevision.current;
    const problem = await storeAccess.confirm();
    if (!mounted.current || revision !== loadRevision.current) return false;
    adoptStorePolicy();
    setStoreProblem(problem);
    return !problem;
  }

  function work(action: () => Promise<void>, failure: string | ((error: unknown) => string)) {
    if (performing.current) return;
    const token = {};
    performing.current = token;
    setMessage('');
    // A busy server was said for the step before; this one starts clean.
    setStoreProblem(shown => shown?.kind === 'busy' ? undefined : shown);
    setBusy(true);
    const revision = loadRevision.current;
    const live = () => mounted.current && revision === loadRevision.current;
    action()
      .catch(async (error: unknown) => {
        // A request that needs the board's account proof asks for it once and runs again. The client, the journals
        // and the policy of a store page are the same objects before and after, so the same step can be repeated.
        if (!(error instanceof SessionNeeded) || !storeAccess) throw error;
        const problem = await storeAccess.ensure(error.scope);
        if (!live()) return;
        adoptStorePolicy();
        setStoreProblem(problem);
        if (!problem) await action();
      })
      .catch((error: unknown) => {
        if (!live()) return;
        // Too many requests at the store's server: nothing is known to have failed, and the session stands.
        if (storeAccess && error instanceof StoreBusy) setStoreProblem({kind: 'busy'});
        else setMessage(typeof failure === 'string' ? failure : failure(error));
      })
      .finally(() => {
        if (performing.current === token) {
          performing.current = undefined;
          if (mounted.current) setBusy(false);
        }
        // What the step said is drawn at the top of the page, also when it says the same as the step before.
        if (live()) setStepsDone(count => count + 1);
      });
  }

  async function enroll() {
    if (!Http || phase !== 'setup') throw new Error('COMMERCE_ENROLLMENT');
    const scope = commerceProfile(profile, 'customer', pin.trim());
    const payment = paymentProfile(profile);
    if (payment && presentationFingerprint(payment) !== proofPin.trim())
      throw new Error('PRESENTATION_TRUST');
    const revision = loadRevision.current;
    const current = () => mounted.current && revision === loadRevision.current;
    try {
      await isolateSubscriptionReminders(scope.branchRef, scope.subjectRef, current);
    } catch (error) {
      const reason = error instanceof Error ? error.message : '';
      const failures: Record<string, string> = {
        CLEAR_BUSY: '다른 알림 작업이 진행 중입니다.',
        CLEAR_DEADLINE: '시스템 알림을 확인하는 시간이 초과됐습니다.',
        CLEAR_REMAINS: '이전 구독 알림이 남아 있습니다.',
        REMINDER_JOURNAL_WRITE: '구독 알림 기록을 저장할 수 없습니다.',
      };
      const failure = Object.hasOwn(failures, reason) ? failures[reason] : '이전 구독 알림을 정리할 수 없습니다.';
      if (current()) setMessage(`${failure} 등록을 다시 시도하세요.`);
      return;
    }
    if (!current()) return;
    await Http.configure('customer', profile);
    if (!current()) return;
    setProfile('');
    setPin('');
    setPub('');
    await load();
  }

  function submitEnrollment() {
    work(enroll, '등록서를 확인할 수 없습니다. 발급 기록을 대조하세요.');
  }

  async function save(next?: Checkpoint) {
    const revision = loadRevision.current;
    if (!journals) throw new Error('ORDER_JOURNAL');
    await journals.write('order', next);
    if (mounted.current && revision === loadRevision.current) setPoint(next);
  }

  function reviewCart(selection: CommerceSelection, rows: CommercePrice[]): PickupCartReview {
    const review = reviewCommerceCart(selection, rows);
    if (!pickupMode) return review;
    const pickupChanges = Object.entries(review.selection).flatMap(([key, {row}]) => {
      const previous = selection[key].row;
      return previous.version !== row.version ? [{previous, current: row}] : [];
    });
    return {...review, pickupChanges};
  }

  function cartNeedsReview(review: PickupCartReview) {
    return Boolean(review.priceChanges.length || review.shortages.length || review.pickupChanges?.length);
  }

  function applyCartReview(review: PickupCartReview, rows: CommercePrice[]) {
    setCatalog(rows);
    setCart(review.selection);
    setCartReview(review);
  }

  async function reserve() {
    if (!client || point || menuReadOnly || (pickupMode ? !pickupChoice : !shop))
      throw new Error('ORDER_PENDING');
    const revision = loadRevision.current;
    if (storeAccess) {
      // The first step that needs the customer: the store's merchant record is read from the order contract, then
      // the board proves its account. Stock is held only by the order that follows.
      const problem = await storeAccess.ensure({purpose: 'checkout', branch: storeAccess.store.branch});
      if (!mounted.current || revision !== loadRevision.current) return;
      adoptStorePolicy();
      setStoreProblem(problem);
      if (problem) return;
    }
    let rows: CommercePrice[];
    try {
      rows = await readCatalog();
    } catch (error) {
      if (!pickupMode && mounted.current && revision === loadRevision.current) {
        const disconnected = isMenuNetworkOffline(error) ||
          (link && error instanceof Error && error.message === 'NEARBY_DISCONNECTED');
        if (disconnected && showLastShopMenu()) return;
        loseShop();
      }
      throw error;
    }
    if (!mounted.current || revision !== loadRevision.current) return;
    menuCheckedAt.current = now();
    // After the proof the menu is the session's, which tells how many are left; the page shows that one from now on.
    if (storeAccess) setCatalog(rows);
    const review = reviewCart(cart, rows);
    if (cartNeedsReview(review)) {
      applyCartReview(review, rows);
      return;
    }
    if (commerceNeedsOptionSupport(review.selection)) {
      applyCartReview(review, rows);
      setMessage(menuLocale === 'ko' ? '이 매장에서는 품목당 옵션을 하나씩 담을 수 있습니다. 담은 옵션을 확인하세요.'
        : 'This store accepts one option per item. Review the selected options.');
      return;
    }
    const items = Object.values(review.selection);
    const first = items[0];
    if (
      !first ||
      items.some(
        ({ row }) =>
          row.asset !== first.row.asset || row.version !== first.row.version,
      )
    ) {
      applyCartReview(review, rows);
      setMessage('한 주문에는 같은 결제 통화의 품목을 선택하세요.');
      return;
    }
    const next: Checkpoint = {
      version: 2,
      branch: client.profile.branchRef,
      subject: client.profile.subjectRef,
      key: await walletId(),
      asset: first.row.asset,
      route: link && !pickupMode ? 'nearby' : 'direct',
      ...(link && !pickupMode ? {
        terminal: link.peer.principal,
        deadline: Math.min(now() + 300, ...items.map(({row}) => Math.floor(Date.parse(row.expiresAt) / 1000))),
      } : {}),
      selection: review.selection,
      cart: {
        origin: 'menu',
        originRef: await walletId(),
        menuVersion: first.row.version,
        ...(pickupChoice ? {pickup: {window: pickupChoice.window, startsAt: pickupChoice.startsAt}} : {}),
        items: items.map(({ row, quantity }) => ({
          sku: row.sku,
          option_key: row.optionKey,
          quantity,
        })),
      },
    };
    if (!mounted.current || revision !== loadRevision.current) return;
    await save(next);
    setCartReview(undefined);
    await reconcile(next, true);
  }

  async function showRejectedCart(
    check: Checkpoint,
    stillCurrent: () => boolean,
    expired = false,
  ) {
    const rows = await readCatalog();
    // A pause or a closed day refuses the order before any money moves; the basket stays for later.
    setStoreState(previous => ({...previous, refused: storeStopped(previous.state)}));
    if (!stillCurrent()) return;
    const previous =
      check.selection ??
      Object.fromEntries(
        check.cart.items.map(({ sku, option_key, quantity }) => {
          const row = rows.find(
            current => current.sku === sku && current.optionKey === option_key,
          );
          return [
            commerceLineKey({sku, optionKey: option_key}),
            {
              row: row ?? {
                sku,
                optionKey: option_key,
                label: sku,
                available: 0,
                version: check.cart.menuVersion,
                asset: check.asset,
                unitPrice: '0',
                expiresAt: '',
              },
              quantity,
            },
          ];
        }),
      );
    await save();
    if (!stillCurrent()) return;
    applyCartReview(reviewCart(previous, rows), rows);
    setMessage(expired
      ? '원래 요청의 유효 기간이 끝났습니다. 현재 옵션과 합계를 확인하세요.'
      : check.cart.pickup
      ? '접수되지 않았습니다. 현재 가격과 재고를 다시 확인하거나 장소에서 다른 픽업 시간을 선택하세요.'
      : '접수되지 않았습니다. 현재 옵션과 합계를 확인하세요.');
  }

  /**
   * The store's server did not hold the cart of this board account (customer_funds.go): no stock is held and no order
   * exists, so the request is dropped and the basket stays. Only the limit on carts is counted before the server looks
   * at the request, so a request that had been sent before is kept for that one and asked again later.
   */
  /**
   * The cart of this request is closed at the store without a payment: it was taken back (and the app closed before
   * it wrote that down), or its hold ran out. The request is over; the basket is ordered again with a new one.
   */
  async function cartReleased(check: Checkpoint) {
    await save();
    const basket = check.selection ?? cart;
    if (basket !== cart) setCart(basket);
    setOrder(undefined);
    setOrderFocus(true);
    setMessage('이전 주문은 결제되지 않은 채 매장에서 취소되었거나 결제 시간이 지나 닫혔습니다. 잡아 둔 재고는 없습니다. 장바구니를 확인하고 다시 주문하세요.');
  }

  async function holdRefused(check: Checkpoint, refusal: CommerceHoldRefusal, first: boolean) {
    if (refusal.reason === 'cart_released') {
      await cartReleased(check);
      return;
    }
    if (refusal.reason === 'hold_limit' && !first) {
      setMessage('짧은 시간에 주문 요청이 많아 접수 여부를 확인하지 못했습니다. 조금 뒤 저장한 주문을 다시 확인하세요.');
      return;
    }
    await save();
    // A request found again after the page was closed brings its basket back.
    const basket = !first && check.selection ? check.selection : cart;
    if (basket !== cart) setCart(basket);
    if (refusal.reason === 'balance_short') {
      const balance = BigInt(refusal.balance!);
      setBoardBalance(balance);
      setShortHold({required: BigInt(refusal.required!), balance, basket});
      setMessage('보드 잔액이 모자라 주문을 접수하지 않았습니다. 모자란 금액을 확인하고 지갑에서 충전하세요.');
    } else setMessage(refusal.reason === 'balance_unavailable'
      ? '매장 서버가 보드 잔액을 확인하지 못해 주문을 접수하지 않았습니다. 잠시 뒤 다시 시도하세요.'
      // The store has no way to be paid by a board yet. Trying again does not change that; its operator has to.
      : refusal.reason === 'orders_closed' ? '이 매장은 지금 보드 결제 주문을 받을 수 없습니다. 다시 시도해도 접수되지 않습니다. 매장이나 운영자에게 문의하세요.'
      : '짧은 시간에 주문 요청이 많아 주문을 접수하지 않았습니다. 조금 뒤 다시 시도하세요.');
  }

  async function reconcile(check: Checkpoint, initial = false) {
    if (!client) throw new Error('CLIENT_REQUIRED');
    const revision = loadRevision.current;
    const stillCurrent = () =>
      mounted.current && revision === loadRevision.current;
    if (check.route === 'nearby') {
      if (!link || (check.terminal && check.terminal !== link.peer.principal))
        throw new Error('NEARBY_REQUIRED');
      const resend = async () => link.request({
        op: 'reserve',
        branch: check.branch,
        key: check.key,
        asset: check.asset,
        cart: JSON.parse(encodeCommerceCart(check.cart)) as CommerceCart,
        ...(check.deadline === undefined ? {} : {deadline: check.deadline}),
      }, fromBase64(await Http!.randomBytes(16)));
      let answer = initial ? await resend() : await link.request({
        op: 'status', branch: check.branch, key: check.key, order: check.order,
        ...(check.deadline === undefined ? {} : {authoritative: true}),
      }, fromBase64(await Http!.randomBytes(16)));
      // The first frame may have been lost before the relay saved the cart.
      // The same authenticated intent is safe to send again.
      if (!initial && !check.order && answer.status === 'denied' && stillCurrent())
        answer = await resend();
      if (!stillCurrent()) return;
      if (
        !check.order &&
        ['sold-out', 'catalog-changed', 'expired'].includes(answer.status)
      ) {
        if (answer.status === 'sold-out') {
          const unavailable = Array.isArray(answer.value)
            ? check.cart.items.filter(line => (answer.value as CommerceCartLine[]).some(denied =>
                denied?.sku === line.sku && denied.option_key === line.option_key && denied.quantity === line.quantity))
            : [];
          setNearbySoldOut(unavailable);
          return;
        }
        await showRejectedCart(check, stillCurrent, answer.status === 'expired');
        return;
      }
      if (answer.status !== 'ok') throw new Error(answer.status);
      const found = answer.value as CommerceOrder;
      const expectedAmount = check.selection && commerceCartTotals(check.selection)
        .find(total => total.asset === check.asset)?.amount;
      if (
        !found ||
        !identifier.test(found.id) ||
        (check.order && found.id !== check.order) ||
        found.asset !== check.asset ||
        (expectedAmount !== undefined && found.amount !== expectedAmount)
      )
        throw new Error('NEARBY_ORDER');
      if (!check.order) await save({ ...check, order: found.id });
      if (!stillCurrent()) return;
      if (policy && canReplaceReview(found.id) &&
        ['preview', 'issued'].includes(found.state)) {
        await nearbyCheckout({order: found.id, key: check.key},
          {origin: check.cart.originRef, order: found, authoritative: check.deadline !== undefined});
      } else {
        setOrder(found.payment?.stage === 'unpaid' ? {...found, payment: undefined} : found);
      }
      return;
    }
    let sale = check.order;
    if (!sale) {
      try {
        sale = await client.reserve(check.cart, check.asset, check.key);
      } catch (error) {
        if (storeAccess && error instanceof CommerceHoldRefusal) {
          if (stillCurrent()) await holdRefused(check, error, initial);
          return;
        }
        // One board account holds one unpaid cart at a time, at any store. A first request refused for that left
        // nothing behind; the basket stays for when the other order has ended.
        if (storeAccess && initial && error instanceof Error && error.message === 'CHECKOUT_CONFLICT') {
          if (!stillCurrent()) return;
          await save();
          setMessage('이 보드 계정이 잡아 둔 다른 미결제 주문이 있어 새 주문을 접수하지 못했습니다. 그 주문을 취소하거나 결제 시간이 끝난 뒤 다시 시도하세요.');
          return;
        }
        if (!commerceReservationRejected(error)) throw error;
        if (!stillCurrent()) return;
        await showRejectedCart(check, stillCurrent);
        return;
      }
      if (!stillCurrent()) return;
      await save({ ...check, order: sale });
    }
    const found = await client.status(sale);
    if (!stillCurrent()) return;
    // An order of a store page that the store closed, with no payment of it at the store and none ever sent from
    // this phone, was released: the same as the answer to its request sent again.
    if (storeAccess && found.id === sale && found.state === 'closed' && !found.payment && !found.escrow &&
        (!approved || approved.order.id !== sale || neverSent(approved)) &&
        !(await orderRecords(Http!).list().then(rows => rows.some(row => row.sale === sale && row.stage !== 'failed'), () => true))) {
      if (stillCurrent()) await cartReleased(check);
      return;
    }
    setOrder(found);
    // When the hold ends is written down with the order, so the order can be left after that time with no word
    // from the store (startNew).
    const holdUntil = Math.floor(Date.parse(found.expiresAt) / 1000);
    if (storeAccess && found.id === sale && ['preview', 'issued'].includes(found.state) && Number.isSafeInteger(holdUntil) && holdUntil > 0 &&
        point?.holdUntil !== holdUntil) await save({ ...check, order: sale, holdUntil });
  }

  function resetShop() {
    setShopPermissionDenied(false);
    adapter.current?.close();
    adapter.current = undefined;
    pendingShopLink.current = undefined;
    setLink(undefined);
    setPair(undefined);
    setShop(undefined);
    setCatalog([]);
    setMenuCategory('');
    setMenuReadOnly(false);
    menuCheckedAt.current = undefined;
    setCart({});
    setCartReview(undefined);
    setNearbySoldOut(undefined);
  }

  function showLastShopMenu() {
    if (menuCheckedAt.current === undefined) return false;
    setMenuReadOnly(true);
    setLink(undefined);
    setPair(undefined);
    return true;
  }

  function loseShop() {
    resetShop();
    setShopOffline(true);
  }

  async function findNearby() {
    if (!Nearby || !nearbyEnrollment || !client) return undefined;
    const revision = loadRevision.current;
    const raw = registration.current;
    const allowed = await permission().catch(() => false);
    if (!allowed && mounted.current && raw === registration.current) {
      // The system permission dialog can suspend and remount the search view.
      setShopPermissionDenied(true);
    }
    if (!allowed || revision !== loadRevision.current || !mounted.current) return undefined;
    const running = new CustomerNearby(Nearby, client.profile.branchRef, () => {
      if (adapter.current === running && !showLastShopMenu()) loseShop();
    });
    adapter.current = running;
    return running.scan(nearbyEnrollment);
  }

  async function connectNearby(handle: string): Promise<ShopMatch> {
    if (!adapter.current || !client || !Http) throw new Error('NEARBY_UNAVAILABLE');
    const running = adapter.current;
    const open = await running.connect(handle);
    try {
      const branch = client.profile.branchRef;
      const answer = await open.request({op: 'discover', branch}, fromBase64(await Http.randomBytes(16)));
      if (answer.status === 'pending') {
        running.disconnect();
        try {
          return await checkOnlineShop();
        } catch {
          throw new Error('SHOP_RELAY_OFFLINE');
        }
      }
      if (answer.status !== 'ok' || adapter.current !== running)
        throw new Error('NEARBY_TRUST_REQUIRED');
      const identity = readShopIdentity(answer.value, branch, open.peer.principal);
      pendingShopLink.current = open;
      return {identity, route: 'relay'};
    } catch (error) {
      running.disconnect();
      throw error;
    }
  }

  async function checkOnlineShop(branch?: string): Promise<ShopMatch> {
    if (!client) throw new Error('CLIENT_REQUIRED');
    if (branch && branch !== client.profile.branchRef) throw new Error('SHOP_WRONG_BRANCH');
    return {identity: await client.shopIdentity(), route: 'direct'};
  }

  async function openConfirmedShop(match: ShopMatch) {
    const revision = loadRevision.current;
    const routeLink = match.route === 'relay' ? pendingShopLink.current : undefined;
    if (match.route === 'relay' && (!routeLink || routeLink.peer.principal !== match.identity.terminal))
      throw new Error('NEARBY_REQUIRED');
    const rows = point ? undefined : await readCatalog(routeLink, true);
    if (revision !== loadRevision.current || !mounted.current) return;
    if (match.route === 'direct' && !rows) await client?.shopIdentity();
    if (revision !== loadRevision.current || !mounted.current) return;
    setShop(match);
    setEntrancePending(false);
    setShopOffline(false);
    setLink(routeLink);
    if (rows) {
      menuCheckedAt.current = now();
      setCatalog(rows);
    }
    setMenuReadOnly(false);
  }

  async function scanShopQr() {
    if (!ShopQr) throw new Error('SHOP_CAMERA');
    try {
      const raw = await ShopQr.scan();
      if (raw.startsWith('wc:')) throw new Error('SHOP_CAMERA');
      if (raw.startsWith('nu54://order') && openPayments) { openPayments(raw.slice(0, 133)); return ''; }
      if (!raw || !openShopQr) return raw;
      // The Android camera activity can suspend the order view while it captures.
      openShopQr(raw);
      return '';
    } catch {
      throw new Error('SHOP_CAMERA');
    }
  }

  /** The kiosk's code; a store's entrance code opens its menu instead. */
  async function scanOrderQr() {
    const raw = await scanShopQr();
    if (raw) throw new Error('SHOP_CAMERA');
  }

  async function recoverOriginal() {
    if (!client) throw new Error('CLIENT_REQUIRED');
    const original = point?.order ?? ticket?.order ?? approved?.order.id;
    if (!original) throw new Error('ORDER_PENDING');
    const revision = loadRevision.current;
    const result = await client.status(original);
    if (result.id !== original) throw new Error('WRONG_ORDER');
    if (mounted.current && revision === loadRevision.current) setOrder(result);
  }

  async function nearbyCheckout(current: {
    order: string;
    key: string;
    code?: string;
  }, privateCart?: {origin: string; order: CommerceOrder; authoritative: boolean}) {
    if (!link || !client) throw new Error('NEARBY_REQUIRED');
    const revision = loadRevision.current;
    const stillCurrent = () => mounted.current && revision === loadRevision.current;
    const answer = await link.request(
      {
        op: 'checkout',
        branch: client.profile.branchRef,
        order: current.order,
        key: current.key,
        ...(privateCart?.authoritative ? {authoritative: true} : {}),
      },
      fromBase64(await Http!.randomBytes(16)),
    );
    if (!stillCurrent()) return;
    if (answer.status === 'pending') {
      if (privateCart) throw new Error('NEARBY_QUOTE_PENDING');
      const code = (answer.value as { code?: string } | undefined)?.code;
      if (code && !/^[0-9a-f]{8}$/.test(code)) throw new Error('NEARBY_CODE');
      setPair({ ...current, code });
      setMessage(
        code
          ? '같은 확인 번호를 단말에서 확인하세요.'
          : '단말의 응답이 보류 중입니다.',
      );
      return;
    }
    if (answer.status !== 'ok' || !policy || !canReplaceReview(current.order))
      throw new Error('NEARBY_SCOPE');
    const value = unpackNearbyPresentation(
      answer.value,
      client.profile.branchRef,
      fromBase64,
    );
    if (privateCart &&
      (value.order.asset !== privateCart.order.asset ||
        value.order.amount !== privateCart.order.amount ||
        (privateCart.order.state === 'issued' && value.order.quoteDigest !== privateCart.order.quoteDigest)))
      throw new Error('NEARBY_QUOTE_CHANGED');
    await setUpPayment(
      value.order,
      value.quote,
      'nearby',
      {
        sale: current.order,
        origin:
          privateCart?.origin ?? (point?.order === current.order ? point.cart.originRef : undefined),
      },
      current.key,
      privateCart?.authoritative ? true : undefined,
    );
    if (!stillCurrent()) return;
    setPair(undefined);
    setOrder(value.order);
    setLocator('');
  }

  async function readCatalog(routeLink: NearbyCommerceLink | null | undefined = link, opening = false): Promise<CommercePrice[]> {
    if (!client) throw new Error('CLIENT_REQUIRED');
    if (!pickupMode && !opening && !shop) throw new Error('SHOP_NOT_CONFIRMED');
    if (!routeLink) {
      const menu = await client.publishedMenu();
      const at = new Date(now() * 1000);
      setStoreState(previous => ({
        state: menu.orderState,
        refused: previous.refused && storeStopped(menu.orderState),
        checkedAt: `${String(at.getHours()).padStart(2, '0')}:${String(at.getMinutes()).padStart(2, '0')}`,
      }));
      return menu.catalog;
    }
    const rows: CommercePrice[] = [];
    let offset = 0;
    let snapshot: number | undefined;
    while (true) {
      const result = await routeLink.request(
        { op: 'catalog', branch: client.profile.branchRef, offset },
        fromBase64(await Http!.randomBytes(16)),
      );
      const page = result.value as
        | { rows?: CommercePrice[]; next?: number; snapshotVersion?: number }
        | undefined;
      if (result.status !== 'ok' || !page) throw new Error('NEARBY_CATALOG');
      assertCommerceCatalogPage(page.rows, now(), 4);
      assertCommerceCatalogVersion(page.snapshotVersion, page.rows);
      if (offset === 0) snapshot = page.snapshotVersion;
      else if (snapshot !== page.snapshotVersion) throw new Error('CATALOG_CHANGED');
      appendCommerceCatalogPage(rows, page.rows);
      if (page.next === undefined) break;
      if (
        !page.rows.length ||
        page.next !== offset + page.rows.length ||
        page.next > commerceCatalogLimit
      )
        throw new Error('NEARBY_CATALOG');
      offset = page.next;
    }
    if (rows.some(row => Date.parse(row.expiresAt) <= Date.now()))
      throw new Error('NEARBY_CATALOG');
    assertCommerceChoices(rows);
    return rows;
  }

  async function getCatalog() {
    const revision = loadRevision.current;
    const rows = await readCatalog();
    if (!mounted.current || revision !== loadRevision.current) return;
    const review = reviewCart(cart, rows);
    if (cartNeedsReview(review))
      applyCartReview(review, rows);
    else setCatalog(rows);
  }

  async function refreshShopMenu() {
    if (!shop) throw new Error('SHOP_NOT_CONFIRMED');
    const revision = loadRevision.current;
    try {
      const verified = await checkOnlineShop(shop.identity.branch);
      const rows = await readCatalog(null);
      if (!mounted.current || revision !== loadRevision.current) return;
      const review = reviewCommerceCart(cart, rows);
      if (review.priceChanges.length || review.shortages.length) applyCartReview(review, rows);
      else setCatalog(rows);
      menuCheckedAt.current = now();
      setShop(verified);
      setMenuReadOnly(false);
    } catch (error) {
      if (!mounted.current || revision !== loadRevision.current) return;
      if (isMenuNetworkOffline(error) && showLastShopMenu()) return;
      loseShop();
      throw error;
    }
  }

  async function keepPayment(next?: PaymentCheckpoint) {
    if (!journals) throw new Error('PAYMENT_JOURNAL');
    const revision = loadRevision.current;
    await journals.write('payment', next);
    if (mounted.current && revision === loadRevision.current) setApproved(next);
  }

  async function setUpPayment(
    sale: CommerceOrder,
    quote: CommerceQuote,
    route: 'direct' | 'nearby',
    expected: { sale: string; origin?: string; claim?: string },
    requestKey?: string,
    authoritative?: true,
  ) {
    if (!policy || !client || !canReplaceReview(sale.id))
      throw new Error('PAYMENT_PROFILE');
    const revision = loadRevision.current;
    const escrow = paysThroughEscrow(policy);
    const block = (reason: OrderBlock, more: {until?: number; spent?: boolean} = {}) => {
      if (mounted.current && revision === loadRevision.current) setOrderBlocked({block: reason, sale, ...more});
    };
    setOrderBlocked(undefined);
    if (escrow && quote.expiresAt <= now()) return block('expired');
    if (escrow && BigInt(sale.amount) > BigInt(policy.maxBaseUnits)) return block('over-limit');
    const payment = reviewedPayment(quote, sale, trusted(policy), now(), {
      branch: client.profile.branchRef,
      customer: client.profile.subjectRef,
      ...expected,
    });
    if (escrow && (payment.payerRole !== 'owner' || sale.subscription || travelPayment)) return block('unsupported');
    if (!(route === 'direct' && sale.cafeReview && payment.payerRole === 'renter')) {
      if (!OwnerWallet) throw new Error('PAYMENT_PROFILE');
      const local = decodeWalletStorage(await OwnerWallet.loadProfile());
      if (!mounted.current || revision !== loadRevision.current) return;
      if (escrow && (local.mode !== 'personal' || !local.enrollment || !local.verified)) return block('no-board');
      if (escrow && (local.verified!.address !== payment.payer || BigInt(local.verified!.generation) !== payment.generation))
        return block('wrong-account');
      if (
        local.mode !== 'personal' ||
        !local.enrollment ||
        local.verified?.network !== '8283' ||
        local.verified.boardFingerprint !== local.enrollment.device ||
        local.verified.address !== payment.payer ||
        BigInt(local.verified.generation) !== payment.generation
      )
        throw new Error('PAYMENT_WALLET_BINDING');
    }
    let confirmer: string | undefined;
    let claimed: Awaited<ReturnType<ReturnType<typeof orderRecords>['claimRequest']>> | undefined;
    if (escrow) {
      const records = orderRecords(Http!);
      try {
        confirmer = await records.confirmer({payer: payment.payer, generation: payment.generation.toString()});
      } catch {
        return block('key-lost');
      }
      // A store issues one quote per order. The board keeps its answer under the first request id.
      claimed = await records.claimRequest({digest: quote.quoteDigest, confirmer, expires: quote.expiresAt}, now());
      if (claimed.answered) return block('answered', {until: quote.expiresAt, spent: claimed.answered === 'approved'});
    }
    const request = claimed?.request ?? bytesToHex(fromBase64(await Http!.randomBytes(16)), false);
    const submitKey = await walletId();
    if (!mounted.current || revision !== loadRevision.current) return;
    setReviewClock(now());
    setReviewDetails(false);
    setOrderBoard(undefined);
    setOrderFocus(true);
    await keepPayment({
      version: 2,
      branch: client.profile.branchRef,
      customer: client.profile.subjectRef,
      order: {
        id: sale.id, revision: sale.revision, state: sale.state, asset: sale.asset,
        amount: sale.amount, expiresAt: sale.expiresAt, quoteDigest: sale.quoteDigest,
        orderKey: sale.orderKey, subscription: sale.subscription, cafeReview: sale.cafeReview,
      },
      quote,
      request,
      submitKey,
      route,
      relayKey: route === 'nearby' ? requestKey : undefined,
      quoteKey: route === 'direct' && sale.cafeReview ? requestKey : undefined,
      authoritative,
      confirmer,
      boardStarted: claimed?.reused ? true : undefined,
      store: !confirmer ? undefined : shop?.identity.branch === client.profile.branchRef ? shop.identity.branchLabel : sale.cafeReview?.branchLabel,
      scope: {
        branch: client.profile.branchRef,
        customer: client.profile.subjectRef,
        ...expected,
      },
    });
  }

  async function preparePayment(sale: CommerceOrder) {
    if (!client || !policy || !canReplaceReview(sale.id)) throw new Error('PAYMENT_PROFILE');
    const subscription = invoiceDetail || !!sale.subscription;
    const escrow = paysThroughEscrow(policy);
    setOrderBlocked(undefined);
    if (escrow && (subscription || travelPayment || pickupMode || point?.order === sale.id && point.cart.pickup)) {
      setOrderBlocked({block: 'unsupported', sale});
      return;
    }
    if (point?.route === 'nearby' && point.order === sale.id && !subscription) {
      if (point.terminal && link?.peer.principal !== point.terminal) throw new Error('NEARBY_REQUIRED');
      await nearbyCheckout({order: sale.id, key: point.key},
        {origin: point.cart.originRef, order: sale, authoritative: point.deadline !== undefined});
      return;
    }
    if (link && !subscription && !sale.cafeReview) {
      await nearbyCheckout({ order: sale.id, key: await walletId() });
      return;
    }
    const key =
      point?.order === sale.id
        ? point.key
        : ticket?.order === sale.id
        ? ticket.key
        : await walletId();
    const exact = await client.status(sale.id);
    if (travelPayment) {
      if (!matchesTravelOrder(exact) || exact.asset !== policy.asset ||
          travelPayment.booking.chainId !== undefined && travelPayment.booking.chainId !== policy.chain)
        throw new Error('TRAVEL_ORDER');
      if (!['preview', 'issued'].includes(exact.state)) {
        setOrder(exact);
        return;
      }
    }
    if (subscription && !invoiceCanApprove(exact)) {
      setOrder(exact);
      return;
    }
    let claim, quote;
    try {
      claim =
        (await client.claim(sale.id)) ??
        (await client.authorize(sale.id, client.profile.subjectRef, key));
      quote = await client.quote(
        sale.id,
        claim.claimDigest,
        quoteOrderRevision(exact),
        key,
      );
    } catch (error) {
      // An escrow store's server refuses to quote what it cannot lock, such as a rented payer.
      if (!escrow || !(error instanceof Error) || error.message !== 'UNSUPPORTED_CAPABILITY') throw error;
      setOrderBlocked({block: 'unsupported', sale: exact});
      return;
    }
    const issued = await client.status(exact.id);
    if (!matchesTravelOrder(issued)) throw new Error('TRAVEL_ORDER');
    await setUpPayment(issued, quote, 'direct', {
      sale: sale.id,
      claim: claim.claimDigest,
      origin: exact.subscription?.invoiceRef ?? (point?.order === sale.id ? point.cart.originRef : undefined),
    }, key);
    setOrder(issued);
  }

  async function freshSponsorship(entry: PaymentCheckpoint, liveOrder?: CommerceOrder, rentalReview?: RentalReviewChallenge) {
    if (!policy || !client) throw new Error('PAYMENT_PROFILE');
    const revision = loadRevision.current;
    let current: CommerceOrder;
    let quote: CommerceQuote;
    if (entry.route === 'nearby') {
      if (!link || !entry.relayKey) throw new Error('NEARBY_REQUIRED');
      const response = await link.request({
        op: 'checkout', branch: entry.branch, order: entry.order.id, key: entry.relayKey,
        ...(entry.authoritative ? {authoritative: true} : {}),
      }, fromBase64(await Http!.randomBytes(16)));
      if (response.status !== 'ok') throw new Error('GAS_SPONSOR_UNAVAILABLE');
      const refreshed = unpackNearbyPresentation(response.value, entry.branch, fromBase64);
      current = refreshed.order;
      quote = refreshed.quote;
    } else {
      current = liveOrder ?? await client.status(entry.order.id);
      if (!matchesReviewedOrder(entry, current) || !current.cafeReview?.payerBound || !entry.scope.claim)
        throw new Error('PAYMENT_ORDER_STATE');
      quote = await client.quote(current.id, entry.scope.claim, quoteOrderRevision(current), entry.quoteKey ?? entry.submitKey, rentalReview);
    }
    if (!matchesReviewedOrder(entry, current) || quote.signedQuote !== entry.quote.signedQuote)
      throw new Error('PAYMENT_ORDER_STATE');
    reviewedPayment(quote, current, trusted(policy), now(), entry.scope);
    if (!mounted.current || revision !== loadRevision.current) throw new Error('PAYMENT_REVIEW_CHANGED');
    return {...entry, quote};
  }

  async function reviewRentalBoard() {
    const entry = approved;
    const terminal = link;
    if (!entry || entry.route !== 'direct' || !entry.order.cafeReview || entry.authorization ||
        !terminal || !policy || !Http || paymentReviewStop(entry, now()))
      throw new Error('RENTAL_REVIEW_SCOPE');
    const revision = loadRevision.current;
    const current = () => mounted.current && revision === loadRevision.current &&
      rentalContext.current.link === terminal && rentalContext.current.approved?.request === entry.request &&
      (!rentalContext.current.order ||
        (rentalContext.current.order.id === entry.order.id && rentalContext.current.order.state === 'issued'));
    const continuing = rentalBoardReview?.checkpoint === entry.request && rentalBoardReview.terminal === terminal &&
      rentalBoardReview.expiresAt > now() ? rentalBoardReview : undefined;
    const expected = continuing ?? {
      request: bytesToHex(fromBase64(await Http.randomBytes(16)), false),
      challenge: bytesToHex(fromBase64(await Http.randomBytes(32)), false),
    };
    if (!current()) return;
    const refreshed = await freshSponsorship(entry, undefined, {request: expected.request, challenge: expected.challenge});
    if (!current()) return;
    const verified = reviewedPayment(refreshed.quote, refreshed.order, policy, now(), refreshed.scope);
    const sponsorship = verifyPresentedGasSponsor(refreshed.quote, refreshed.order, policy, now(), refreshed.scope);
    const authority = verifyPresentedRentalReviewAuthority(refreshed.quote, refreshed.order, policy, now(), refreshed.scope, expected);
    const expiresAt = Math.min(authority.expiresAt, sponsorship.expiresAt);
    if (!continuing) {
      const {customer, origin, claim, signature} = refreshed.quote.presentation!;
      const original = {...refreshed.quote, presentation: {customer, origin, claim, signature}};
      const loaded = await terminal.request({
        op: 'rental-review-source', branch: entry.branch, key: entry.submitKey, order: entry.order.id,
        value: packNearbyPresentation(refreshed.order, original, toBase64),
      }, fromBase64(await Http.randomBytes(16)));
      if (!current()) return;
      if (loaded.status !== 'ok' || expiresAt <= now())
        throw new Error('RENTAL_REVIEW_SCOPE');
    }
    const reply = await terminal.request({
      op: 'rental-review', branch: entry.branch, key: entry.submitKey, order: entry.order.id,
      request: expected.request, challenge: expected.challenge, quoteDigest: verified.digest, authority,
    }, fromBase64(await Http.randomBytes(16)));
    if (!current()) return;
    if (expiresAt <= now()) throw new Error('RENTAL_REVIEW_EXPIRED');
    const next = {request: expected.request, challenge: expected.challenge, checkpoint: entry.request, terminal, expiresAt};
    const privateQuote = refreshed.quote.presentation &&
      {...refreshed.quote, presentation: {...refreshed.quote.presentation, rentalReview: undefined}};
    if (!privateQuote) throw new Error('RENTAL_REVIEW_SCOPE');
    if (reply.status === 'pending') {
      await keepPayment({...refreshed, quote: privateQuote});
      if (current()) {
        setRentalBoardReview({...next, verified: false});
        setMessage('매장 단말에서 원견적을 보내면 보드 검증 결과를 확인하세요.');
      }
      return;
    }
    const receipt = (reply.value as {receipt?: unknown} | undefined)?.receipt;
    if (reply.status !== 'ok' || typeof receipt !== 'string' || !/^[0-9a-f]{592}$/.test(receipt))
      throw new Error('RENTAL_REVIEW_SCOPE');
    verifyRentalPaymentReview(hexToBytes(receipt), verified, {...authority, now: now()});
    await keepPayment({...refreshed, quote: privateQuote});
    if (current() && expiresAt > now()) {
      setRentalBoardReview({...next, verified: true, receipt, authority});
      setMessage('앱에서 원주문, 매장과 금액을 확인한 뒤 보드 SW1로 승인하거나 SW2로 거절하세요.');
    }
  }

  async function confirmRentalDecision() {
    const entry = approved;
    const review = rentalBoardReview;
    if (!entry || entry.route !== 'direct' || !entry.order.cafeReview || !review?.verified ||
        link !== review.terminal || review.checkpoint !== entry.request ||
        review.expiresAt <= now() || !client || !policy || !Http ||
        entry.authorization || paymentReviewStop(entry, now()))
      throw new Error('RENTAL_REVIEW_SCOPE');
    const revision = loadRevision.current;
    const current = () => mounted.current && revision === loadRevision.current &&
      rentalContext.current.link === review.terminal && rentalContext.current.approved?.request === entry.request &&
      !rentalContext.current.approved.authorization && !paymentReviewStop(rentalContext.current.approved, now()) &&
      (!rentalContext.current.order ||
        (rentalContext.current.order.id === entry.order.id && rentalContext.current.order.state === 'issued'));
    const live = await client.status(entry.order.id);
    if (!current()) return;
    if (!matchesReviewedOrder(entry, live)) {
      await observePayment(live, client);
      return;
    }
    const payment = reviewedPayment(entry.quote, live, policy, now(), entry.scope);
    const receipt = hexToBytes(review.receipt);
    const reply = await review.terminal.request({
      op: 'rental-decision', branch: entry.branch, key: entry.submitKey, order: entry.order.id,
      request: review.request, challenge: review.challenge, quoteDigest: payment.digest,
      reviewDigest: bytesToHex(sha256(receipt), false),
    }, fromBase64(await Http.randomBytes(16)));
    if (!current()) return;
    if (review.expiresAt <= now()) throw new Error('RENTAL_REVIEW_EXPIRED');
    if (reply.status === 'pending') {
      setMessage('보드에서 SW1 또는 SW2를 누른 뒤 같은 결정 결과를 확인하세요.');
      return;
    }
    const decision = (reply.value as {decision?: unknown} | undefined)?.decision;
    if (reply.status !== 'ok' || typeof decision !== 'string' || !/^[0-9a-f]{332}$/.test(decision))
      throw new Error('RENTAL_REVIEW_SCOPE');
    const verified = verifyRentalPaymentDecision(hexToBytes(decision), receipt, payment, {...review.authority, now: now()});
    if (verified.decision === 'refused') {
      await keepPayment({...entry, reviewStop: 'refused'});
      setMessage('보드에서 이 원견적의 지급을 거절했습니다.');
    } else {
      await keepPayment({...entry, authorization: verified.signature});
      setMessage('보드에서 이 원견적의 지급을 승인했습니다. 지급 요청을 제출하세요.');
    }
    setRentalBoardReview(undefined);
  }

  async function findBoard() {
    if (
      !approved ||
      approved.authorization ||
      paymentReviewStop(approved, now()) !== undefined ||
      !OwnerWallet ||
      !policy ||
      !(await permission())
    )
      throw new Error('PAYMENT_BOARD_REQUIRED');
    const verified = reviewedPayment(
      approved.quote,
      approved.order,
      trusted(policy),
      now(),
      approved.scope,
    );
    if (needsMerchantSponsorship(approved))
      verifyPresentedGasSponsor(approved.quote, approved.order, policy, now(), approved.scope);
    if (approved.order.cafeReview && verified.payerRole === 'renter')
      throw new Error('PAYMENT_RENTER_BOARD_REQUIRED');
    const revision = loadRevision.current;
    const found = new Set<string>();
    setBoards([]);
    const receipt = Ble.onScan(json => {
      if (!mounted.current || revision !== loadRevision.current) return;
      try {
        const value = JSON.parse(json);
        if (
          typeof value.address !== 'string' ||
          value.address.length > 64 ||
          typeof value.name !== 'string'
        )
          return;
        found.add(value.address);
        setBoards(old =>
          old.some(item => item.handle === value.address)
            ? old
            : [...old, { handle: value.address, name: value.name }],
        );
      } catch {}
    });
    try {
      await Ble.scanOwner(OWNER_GATT.service, 6);
    } finally {
      receipt.remove();
    }
    return [...found];
  }

  /** Reads what the board will sign for before the owner is asked to press anything. */
  async function checkOrderBoard(handle: string) {
    const entry = approved;
    if (!entry?.confirmer || !OwnerWallet || !policy || paymentReviewStop(entry, now())) throw new Error('PAYMENT_BOARD_REQUIRED');
    const revision = loadRevision.current;
    if (!(await storePolicyReady())) return;
    const payment = reviewedPayment(entry.quote, entry.order, trusted(policy), now(), entry.scope);
    const local = decodeWalletStorage(await OwnerWallet.loadProfile());
    if (!local.enrollment || local.verified?.address !== payment.payer || BigInt(local.verified.generation) !== payment.generation) {
      setOrderBoard({request: entry.request, block: 'wrong-account'});
      return;
    }
    const session = new OwnerSession(Ble, OwnerWallet, local.enrollment);
    ownerConnection.current = session;
    let board;
    try {
      await session.connect(handle);
      board = await session.spendingStatus(payment.generation);
    } catch {
      if (mounted.current && revision === loadRevision.current) setOrderBoard({request: entry.request, block: 'board-unready'});
      return;
    } finally {
      session.close();
      if (ownerConnection.current === session) ownerConnection.current = undefined;
    }
    const endpoint = serverFor('stable', local.walletRead, {store: policy.rpc}).server;
    const wallet = OwnerWallet;
    const held = await readEvmReceiveBalance(
      walletReadRpc(endpoint, (url, packet) => wallet.evmRpc(url, packet)), '8283', readProfiles.stable.anchor,
      payment.payer, policy.asset, !!policy.token,
    ).catch((error: unknown) => error instanceof Error && error.message === 'EVM_ASSET_DECIMALS' ? {amount: undefined, decimals: -1} : undefined);
    const balance = held?.amount === undefined ? undefined : BigInt(held.amount);
    if (!mounted.current || revision !== loadRevision.current) return;
    const amount = BigInt(entry.order.amount);
    const show = (value: bigint) => orderMoney(value.toString(), policy.asset, policy, 'balance');
    const perPayment = fromBoard(board.perPayment, paymentToken(policy)), left = fromBoard(board.leftToday, paymentToken(policy));
    setOrderBoard({
      request: entry.request, handle, pinWait: board.pinWait,
      view: {perPayment: show(perPayment), left: show(left), balance: balance === undefined ? undefined : show(balance)},
      block: board.account !== payment.payer ? 'wrong-account' : policy.token && held && held.decimals !== policy.token.decimals ? 'token' : amount > perPayment ? 'over-limit' :
        amount > left ? 'over-daily' : board.pinWait > 0 ? 'pin-wait' : balance !== undefined && balance < amount ? 'balance' : undefined,
    });
  }

  function cancelOrder() {
    if (orderBlocked) { setOrderBlocked(undefined); return; }
    if (approved?.confirmer && approved.authorization && !approved.forwardAttempted) {
      const {request} = approved;
      const entry = {...approved, authorization: undefined, boardSigned: true as const, reviewStop: 'cancelled' as const};
      orderRecords(Http!).answerRequest(request, 'approved').then(() => keepPayment(entry)).catch(() => {
        if (mounted.current) setMessage('취소 상태를 저장하지 못했습니다.');
      });
      return;
    }
    cancelApproval();
  }

  function cancelApproval() {
    if (!approved || approved.authorization || paymentReviewStop(approved, now(), reviewWaiting)) return;
    boardReview.current = undefined;
    ownerConnection.current?.close();
    ownerConnection.current = undefined;
    setReviewWaiting(false);
    setReviewDetails(false);
    setBoards([]);
    keepPayment({...approved, reviewStop: 'cancelled'}).catch(() => {
      if (mounted.current) setMessage('승인 종료 상태를 저장하지 못했습니다.');
    });
  }

  async function acceptBoard(handle: string, observeDevice = false) {
    const reviewed = approved;
    if (!client || !reviewed || reviewed.authorization || !OwnerWallet || !policy || paymentReviewStop(reviewed, now()))
      throw new Error('PAYMENT_BOARD_REQUIRED');
    const token = {};
    const revision = loadRevision.current;
    boardReview.current = token;
    const currentReview = () => mounted.current && revision === loadRevision.current && boardReview.current === token;
    let activeReview = reviewed;
    let session: OwnerSession | undefined;
    let boardStarted: true | undefined;
    try {
      if (reviewed.confirmer) await orderRecords(Http!).confirmer(orderAccount(reviewed), reviewed.confirmer);
      const current = await freshPaymentOrder(reviewed);
      if (!currentReview()) return;
      if (reviewed.route === 'nearby' && ['checking', 'paid', 'closed'].includes(current.state)) {
        const observed = await client.status(reviewed.order.id);
        if (!currentReview()) return;
        if (!['checking', 'paid', 'closed'].includes(observed.state)) throw new Error('PAYMENT_PENDING');
        await observePayment(observed, client);
        return;
      }
      if (current.state === 'paid' || current.state === 'closed') {
        await observePayment(current, client);
        return;
      }
      if (!matchesReviewedOrder(reviewed, current)) {
        setOrder(current);
        await keepPayment({...reviewed, reviewStop: current.state === 'checking' ? 'interrupted' : 'changed'});
        return;
      }
      const payment = reviewedPayment(reviewed.quote, current, trusted(policy), now(), reviewed.scope);
      if (needsMerchantSponsorship(reviewed)) {
        const before = verifyPresentedGasSponsor(reviewed.quote, current, policy, now(), reviewed.scope);
        try {
          activeReview = await freshSponsorship(reviewed, current);
          if (!currentReview()) return;
          await keepPayment(activeReview);
          if (!currentReview()) return;
          const witness = verifyPresentedGasSponsor(activeReview.quote, current, policy, now(), reviewed.scope);
          if (before.account !== witness.account || before.grant !== witness.grant) {
            setMessage('가스 부담 조건이 바뀌었습니다. 아래 원견적을 확인하고 다시 검토하세요.');
            return;
          }
          payment.deadline = Math.min(payment.deadline, witness.expiresAt);
        } catch (error) {
          if (currentReview())
            setMessage(error instanceof Error && error.message === 'PAYMENT_ORDER_STATE'
              ? '원견적이 바뀌었습니다. 같은 주문의 지급 상태를 확인하세요.'
              : '등록된 가스 부담을 다시 확인한 뒤 보드를 이용하세요.');
          return;
        }
      }
      if (current.cafeReview && payment.payerRole === 'renter')
        throw new Error('PAYMENT_RENTER_BOARD_REQUIRED');
      const walletRaw = await OwnerWallet.loadProfile();
      const local = decodeWalletStorage(walletRaw);
      if (!currentReview()) return;
      if (
        !local.enrollment ||
        local.verified?.address !== payment.payer ||
        BigInt(local.verified.generation) !== payment.generation ||
        local.verified.boardFingerprint !== local.enrollment.device
      )
        throw new Error('PAYMENT_WALLET_BINDING');
      setBoardVerified(undefined);
      setApprovalProgress(undefined);
      setReviewWaiting(true);
      session = new OwnerSession(Ble, OwnerWallet, local.enrollment, !!local.spendingBoard);
      ownerConnection.current = session;
      if (observeDevice) {
        try {
          await session.connect(handle);
          await session.negotiateApprovalProgress();
        } catch {
          if (currentReview()) setMessage('기기 상태 확인을 시작하지 못했습니다. 원승인은 요청하지 않았습니다. 같은 보드에서 결제 검토를 직접 다시 시작하세요.');
          return;
        }
      }
      if (!currentReview()) return;
      await keepPayment({...activeReview, boardStarted: true});
      boardStarted = true;
      if (!currentReview()) return;
      if (observeDevice) {
        setApprovalDevice({handle,request:reviewed.request,scope:maintenanceScope(local),enrollment:local.enrollment,address:local.verified.address});
      } else {
        await session.connect(handle);
      }
      if (!currentReview()) return;
      let activeProgress: OwnerApprovalProgress | undefined;
      const {confirmer} = reviewed;
      const signed = confirmer ? await (async () => { try { return await session.approveOrder(
        payment, confirmer, hexToBytes(reviewed.request), () => { if (currentReview()) setBoardWaiting(true); }, paymentToken(policy).name, paymentToken(policy),
      ); } finally { setBoardWaiting(false); } })() : await (async () => { try { return await session.approvePayment(
        payment,
        hexToBytes(reviewed.request),
        () => {
          if (currentReview()) {
            setBoardVerified(reviewed.request);
            setBoardWaiting(true);
            setMessage('독립 표시기에 주문과 금액이 표시되면 대조한 뒤 SW1로 승인하거나 SW2로 거절하세요.');
          }
        },
        observeDevice ? progress => {
          activeProgress = progress;
          if (!currentReview() || !OwnerWallet) return;
          if (!progress) {
            setApprovalProgress(undefined);
          } else if (progress.request === reviewed.request && progress.device === local.enrollment?.device &&
              progress.generation === payment.generation) {
            OwnerWallet.loadProfile().then(latest => {
              if (currentReview() && activeProgress === progress && latest === walletRaw)
                setApprovalProgress(progress);
            }).catch(()=>{});
          }
        } : undefined,
      ); } finally { setBoardWaiting(false); } })();
      if (!currentReview()) return;
      if (payment.deadline <= now()) {
        await keepPayment({...activeReview, reviewStop: 'expired'});
        return;
      }
      await keepPayment({...activeReview, boardStarted: true, authorization: signed});
      if (confirmer) {
        session.close();
        if (currentReview()) await submitLock({...activeReview, boardStarted: true, authorization: signed});
        return;
      }
      if (currentReview()) setMessage('보드가 승인했습니다. 같은 주문의 지급 결과를 확인하세요.');
    } catch (error) {
      if (!currentReview()) return;
      // A store page's session that ran out, or a busy server, is not an answer about this payment: the step asks
      // the board for the account proof and runs again, or says the server is busy.
      if (error instanceof SessionNeeded || error instanceof StoreBusy) throw error;
      if (error instanceof Error && error.message === 'PAYMENT_REFUSED') {
        if (reviewed.confirmer) await orderRecords(Http!).answerRequest(reviewed.request, 'refused');
        await keepPayment({...activeReview, boardStarted, reviewStop: 'refused'});
        return;
      }
      if (reviewed.confirmer) {
        const reason = error instanceof Error ? error.message : '';
        const blocked = ({ORDER_OVER_LIMIT: 'over-limit', ORDER_OVER_DAILY: 'over-daily', ORDER_PIN_WAIT: 'pin-wait', PAYMENT_BOARD_BINDING: 'board-unready',
          PAYMENT_WALLET_BINDING: 'wrong-account', ORDER_KEY_LOST: 'key-lost', ORDER_KEY_UNREADABLE: 'key-lost'} as Record<string, OrderBlock>)[reason];
        if (reviewed.quote.expiresAt <= now()) await keepPayment({...activeReview, boardStarted, reviewStop: 'expired'});
        else if (blocked) setOrderBoard(shown => ({...(shown?.request === reviewed.request ? shown : undefined), request: reviewed.request, block: blocked}));
        else setMessage(reason === 'ORDER_LOCK_MISMATCH' ? '보드 응답을 확인할 수 없어 결제를 보내지 않았습니다. 내 보드가 맞는지 확인하세요.'
          : session ? '보드 연결이 끊겼습니다. 같은 보드에서 다시 확인하면 결과를 이어서 가져옵니다.'
          : '주문 상태를 확인하지 못했습니다. 키오스크 연결을 확인하고 다시 시도하세요.');
        return;
      }
      if (session || reviewed.quote.expiresAt <= now()) {
        await keepPayment({...activeReview, boardStarted, reviewStop: reviewed.quote.expiresAt <= now() ? 'expired' : 'interrupted'});
        return;
      }
      throw error;
    } finally {
      session?.close();
      if (ownerConnection.current === session) ownerConnection.current = undefined;
      if (currentReview()) {
        boardReview.current = undefined;
        setBoardVerified(undefined);
        setApprovalProgress(undefined);
        setReviewWaiting(false);
        setBoards([]);
      }
    }
  }

  async function readApprovalDevice<T>(target:ApprovalDevice, run:(session:OwnerSession)=>Promise<T>) {
    if (!OwnerWallet || ownerConnection.current || AppState.currentState !== 'active')
      throw new Error('MAINTENANCE_SCOPE');
    const revision = loadRevision.current;
    const raw = await OwnerWallet.loadProfile();
    const stored = decodeWalletStorage(raw);
    if (stored.signing || !sameMaintenanceScope(target.scope,maintenanceScope(stored)) ||
        stored.verified?.address !== target.address || stored.enrollment?.boardSPKI !== target.enrollment.boardSPKI ||
        stored.enrollment.ca !== target.enrollment.ca ||
        stored.enrollment.certificateChain.join() !== target.enrollment.certificateChain.join() ||
        ownerConnection.current || revision !== loadRevision.current) throw new Error('MAINTENANCE_SCOPE');
    const session = new OwnerSession(Ble,OwnerWallet,stored.enrollment,!!stored.spendingBoard);
    ownerConnection.current=session;
    try {
      await session.connect(target.handle);
      const result = await run(session);
      const latest = await OwnerWallet.loadProfile();
      if (revision !== loadRevision.current || ownerConnection.current !== session ||
          AppState.currentState !== 'active' || latest !== raw) throw new Error('MAINTENANCE_SCOPE');
      return result;
    } finally {
      if (ownerConnection.current === session) ownerConnection.current=undefined;
      session.close();
    }
  }

  async function freshPaymentOrder(
    entry: PaymentCheckpoint,
  ): Promise<CommerceOrder> {
    if (!client) throw new Error('PAYMENT_PROFILE');
    if (entry.route === 'direct' || invoiceDetail || (entry.confirmer && !link)) return client.status(entry.order.id);
    if (!link || !entry.relayKey) throw new Error('NEARBY_REQUIRED');
    const result = await link.request(
      {
        op: 'status',
        branch: entry.branch,
        key: entry.relayKey,
        order: entry.order.id,
        ...(entry.authoritative ? {authoritative: true} : {}),
      },
      fromBase64(await Http!.randomBytes(16)),
    );
    const current = result.value as CommerceOrder | undefined;
    if (result.status !== 'ok' || !current || current.id !== entry.order.id)
      throw new Error('PAYMENT_PENDING');
    return current;
  }

  async function reconcilePayment() {
    if (!approved || !client) throw new Error('PAYMENT_MISSING');
    const revision = loadRevision.current;
    const view = await client.status(approved.order.id);
    if (!mounted.current || revision !== loadRevision.current) return;
    if (!matchesTravelOrder(view)) throw new Error('TRAVEL_ORDER');
    if (travelPayment) setTravelOrderObservedAt(Date.now());
    await observePayment(view, client);
  }

  async function forwardPayment() {
    const entry = approved;
    if (!client || !entry?.authorization || !policy) throw new Error('PAYMENT_MISSING');
    const revision = loadRevision.current;
    const live = () => mounted.current && revision === loadRevision.current;
    const orderView = await freshPaymentOrder(entry);
    if (!live()) return;
    if (!matchesTravelOrder(orderView)) throw new Error('TRAVEL_ORDER');
    if (travelPayment) setTravelOrderObservedAt(Date.now());
    if (orderView.state !== 'issued' || entry.quote.expiresAt <= now() || entry.forwardAccepted) {
      const observed = entry.route === 'nearby' ? await client.status(entry.order.id) : orderView;
      if (live()) await observePayment(observed, client);
      return;
    }
    if (!matchesReviewedOrder(entry, orderView)) {
      if (entry.forwardAttempted) throw new Error('PAYMENT_ORDER_STATE');
      setOrder(orderView);
      await keepPayment({...entry, authorization: undefined, reviewStop: 'changed'});
      return;
    }
    let forwardReview = entry;
    if (needsMerchantSponsorship(entry)) {
      const prior = verifyPresentedGasSponsor(entry.quote, orderView, policy,
        Math.min(now(), (entry.quote.presentation?.gasSponsor?.expiresAt ?? 0) - 1), entry.scope);
      const refreshed = await freshSponsorship(entry, orderView);
      if (!live()) return;
      const witness = verifyPresentedGasSponsor(refreshed.quote, orderView, policy, now(), entry.scope);
      if (prior.account !== witness.account || prior.grant !== witness.grant) throw new Error('GAS_SPONSOR_CHANGED');
      forwardReview = refreshed;
    }
    const {payment, approval} = approvalForForward(forwardReview, trusted(policy), now());
    if (orderView.cafeReview && payment.payerRole === 'renter')
      verifyPresentedRentalEscrow(forwardReview.quote, orderView, policy, now(), entry.scope);
    const attempt = {...forwardReview, forwardAttempted: true as const};
    await keepPayment(attempt);
    if (!live()) return;
    try {
      if (entry.route === 'nearby') {
        if (!link || !entry.relayKey) throw new Error('NEARBY_REQUIRED');
        const reply = await link.request({
          op: 'forward', branch: entry.branch, key: entry.submitKey, order: entry.order.id,
          quoteDigest: payment.digest, approvalDigest: payment.approvalDigest, signature: approval,
          ...(entry.authoritative ? {authoritative: true} : {}),
        }, fromBase64(await Http!.randomBytes(16)));
        if (reply.status !== 'ok' || (reply.value as {submitted?: string} | undefined)?.submitted !== entry.order.id)
          throw new Error('PAYMENT_PENDING');
      } else {
        const accepted = await client.forward(entry.order.id, payment.digest, payment.approvalDigest, approval, entry.submitKey);
        if (accepted !== entry.order.id) throw new Error('PAYMENT_PENDING');
      }
    } catch {
      if (live()) setMessage('네트워크 응답을 기다립니다. 같은 원주문을 조회하세요.');
      return;
    }
    if (!live()) return;
    await keepPayment({...attempt, forwardAccepted: true});
    if (!live()) return;
    const confirmed = await client.status(entry.order.id);
    if (live()) await observePayment(confirmed, client);
  }

  /** The board account the reviewed order is paid from. It throws when the payment does not verify by the policy. */
  function orderAccount(entry: PaymentCheckpoint) {
    const {payer, generation} = reviewedPayment(entry.quote, entry.order, trusted(policy!), entry.quote.expiresAt - 1, entry.scope);
    return {payer, generation: generation.toString()};
  }

  /** Sends the board's order signature once the phone can prove it and has written down that it did. */
  async function submitLock(entry: PaymentCheckpoint) {
    if (!client || !policy || !Http || !entry.confirmer || !entry.authorization) throw new Error('PAYMENT_MISSING');
    const revision = loadRevision.current;
    const live = () => mounted.current && revision === loadRevision.current;
    const records = orderRecords(Http);
    if (!(await storePolicyReady())) return;
    // A payment that does not verify by the policy is not a lost key: only the key store's own answer says that.
    const account = orderAccount(entry);
    if (!entry.forwardAttempted) {
      try {
        await records.confirmer(account, entry.confirmer);
      } catch {
        if (live()) setOrderBoard({request: entry.request, block: 'key-lost'});
        return;
      }
    }
    const current = await freshPaymentOrder(entry);
    if (!live()) return;
    if (current.state !== 'issued') {
      await observeOrder(await client.status(entry.order.id), entry, true);
      return;
    }
    // After the deadline only the server's answer counts; an unsent signature is dropped.
    if (entry.quote.expiresAt <= now()) {
      if (entry.forwardAttempted) await observeOrder(await client.status(entry.order.id), entry, true);
      return;
    }
    if (!matchesReviewedOrder(entry, current)) {
      if (entry.forwardAttempted) throw new Error('PAYMENT_ORDER_STATE');
      await records.answerRequest(entry.request, 'approved');
      await keepPayment({...entry, authorization: undefined, boardSigned: true, reviewStop: 'changed'});
      return;
    }
    const {payment, confirmer, lockDigest, signature} = lockForSubmit(entry, trusted(policy), now());
    await records.keep({
      ...account, escrow: policy.contract, merchantId: policy.merchantId, sale: entry.order.id, branch: entry.branch, customer: entry.customer, confirmer, store: entry.store,
      asset: entry.order.asset, amount: entry.order.amount, orderKey: entry.quote.orderKey, sentAt: now(),
      // What was bought, kept for the receipt: the lines of the basket this order was made from.
      ...(point?.order === entry.order.id && point.selection ? (() => {
        const items = receiptItems(point.selection, policy, storeState.state?.taxFreeShop);
        return {items, ...(items?.some(item => item.refund) ? {taxFreeShop: storeState.state!.taxFreeShop} : {})};
      })() : {}),
    });
    const attempt = {...entry, forwardAttempted: true as const};
    await keepPayment(attempt);
    if (!live()) return;
    try {
      if (entry.route === 'nearby' && link) {
        const reply = await link.request({
          op: 'lock', branch: entry.branch, key: entry.submitKey, order: entry.order.id,
          quoteDigest: payment.digest, confirmer, lockDigest, signature,
        }, fromBase64(await Http.randomBytes(16)));
        if (reply.status !== 'ok' || (reply.value as {submitted?: string} | undefined)?.submitted !== entry.order.id)
          throw new Error('PAYMENT_PENDING');
      } else if (await client.lock(entry.order.id, payment.digest, confirmer, lockDigest, signature, entry.submitKey) !== entry.order.id) {
        throw new Error('PAYMENT_PENDING');
      }
    } catch {
      return; // The page says the result is unknown and keeps asking for it.
    }
    if (live()) await keepPayment({...attempt, forwardAccepted: true});
  }

  /**
   * Applies one status of the escrow order under payment; true once it is settled for this page.
   * A kiosk can show the order number, but only the server may say that nothing was paid.
   */
  async function observeOrder(view: CommerceOrder, entry: PaymentCheckpoint, authoritative: boolean) {
    if (!policy || !Http) throw new Error('PAYMENT_PROFILE');
    const revision = loadRevision.current;
    const escrow = view.escrow;
    if (!(await storePolicyReady())) return false;
    const payment = reviewedPayment(entry.quote, entry.order, trusted(policy), entry.quote.expiresAt - 1, entry.scope);
    if (view.id !== entry.order.id || view.asset !== entry.order.asset || view.amount !== entry.order.amount ||
        (escrow && (!validOrderEscrowStatus(escrow) || escrow.confirmer !== entry.confirmer || escrow.payer !== payment.payer ||
          escrow.orderKey !== entry.quote.orderKey || escrow.escrow !== policy.contract)))
      throw new Error('PAYMENT_ORDER_STATE');
    const records = orderRecords(Http);
    const record = (await records.list()).find(row => row.sale === view.id);
    const locked = view.state === 'paid' && !!escrow && view.pickupNo !== undefined;
    const failed = authoritative && view.state === 'closed' &&
      (escrow?.stage === 'failed' || view.payment?.stage === 'unpaid' || (!escrow && !entry.forwardAttempted));
    if (record && failed) await records.update(view.id, {stage: 'failed'});
    else if (record) await recordOrder(record, view);
    if (locked || failed) await keepPayment();
    // While the board is still near, the paid order gets the session that follows it to its end (ready notice,
    // receipt, dispute). Without the board now, the order's page asks for the same proof later.
    if (locked && storeAccess) void storeAccess.ensure({purpose: 'order', branch: entry.branch, sale: view.id}).catch(() => {});
    if (!mounted.current || revision !== loadRevision.current) return locked || failed;
    setOpenOrders((await records.list()).filter(row => row.branch === entry.branch && row.customer === entry.customer));
    if (locked || failed) {
      const shown = {sale: view.id, store: entry.store ?? '매장', amount: orderMoney(view.amount, view.asset, policy, 'charge')};
      setOrderOutcome(locked ? {...shown, kind: 'done', number: view.pickupNo!} : {...shown, kind: 'failed'});
      setPendingNearby(undefined);
      setOrderFocus(true);
    }
    return locked || failed;
  }

  const registrationExpired = !!client && client.profile.expiresAt <= now();
  const orderOffline = registrationExpired
    ? '인터넷 주문 등록이 만료되어 상태를 보거나 답할 수 없습니다. 매장 담당자에게 새 등록서를 받아 다시 등록하세요.'
    : '주문 상태를 확인하지 못했습니다. 인터넷에 연결되어 있는지 확인하고 다시 시도하세요.';
  // The open detail page follows its order; App.tsx follows the others.
  const followDetail = useEffectEvent(async () => {
    if (!client || performing.current) return;
    const revision = loadRevision.current;
    const status = await client.status(detailSale).catch(() => undefined);
    if (!status || !mounted.current || revision !== loadRevision.current || performing.current) return;
    await keepStatus(status);
    if (mounted.current && revision === loadRevision.current) setOrder(current => current?.id === status.id ? status : current);
  });
  const following = phase === 'active' && !!client && !!detailSale && openOrders.some(row => row.sale === detailSale && isOpenOrder(row));
  useEffect(() => {
    if (!following) return;
    const timer = setInterval(() => { followDetail().catch(() => {}); }, 15_000);
    return () => clearInterval(timer);
  }, [following]);
  const openFirst = useEffectEvent(() => { if (initialOrder && !formerOrder) work(() => openOrderDetail(initialOrder), orderOffline); });
  useEffect(() => { if (client) openFirst(); }, [client]);

  const pollOrder = useEffectEvent(async () => {
    const entry = approved;
    if (!entry?.confirmer || !entry.forwardAttempted || !client || performing.current) return;
    const revision = loadRevision.current;
    if (entry.route === 'nearby' && link) {
      const seen = await freshPaymentOrder(entry).catch(() => undefined);
      if (!mounted.current || revision !== loadRevision.current) return;
      if (seen?.escrow && await observeOrder(seen, entry, false).catch(() => false)) return;
      if (!mounted.current || revision !== loadRevision.current) return;
    }
    try {
      const view = await client.status(entry.order.id);
      if (!mounted.current || revision !== loadRevision.current) return;
      await observeOrder(view, entry, true);
      setOrderSilent(false);
    } catch {
      if (mounted.current && revision === loadRevision.current) setOrderSilent(true);
    }
  });
  // The page shows amounts, limits and the account's orders, so it stays out of screenshots.
  const protection = useScreenProtection(!!client, OwnerWallet);
  const sentOrder = phase === 'active' && approved?.confirmer && approved.forwardAttempted ? approved.request : undefined;
  useEffect(() => {
    if (!sentOrder) return;
    let live = true;
    const started = Date.now();
    let timer: ReturnType<typeof setTimeout>;
    // Every 5 seconds for 2 minutes, then every 30 seconds for half an hour; the page keeps a manual check.
    const next = (wait: number) => {
      timer = setTimeout(async () => {
        await pollOrder();
        const passed = Date.now() - started;
        if (live && passed < 1_800_000) next(passed < 120_000 ? 5000 : 30_000);
      }, wait);
    };
    next(2000);
    return () => { live = false; clearTimeout(timer); };
  }, [sentOrder]);

  async function closeOrder() {
    const outcome = orderOutcome;
    setOrderOutcome(undefined);
    setOrderBlocked(undefined);
    setOrder(undefined);
    setMessage('');
    // The connection to the kiosk's order has done its work.
    const finished = outcome?.sale ?? (approved?.confirmer && paymentReviewStop(approved, now()) ? approved.order.id : undefined);
    if (ticket && ticket.order === finished) await keepTicket();
    if (outcome) {
      if (point?.order === outcome.sale) { await save(); setCart({}); }
      // Asked here, once: the system dialog would end a kiosk connection that is still needed.
      if (outcome.kind === 'done' && openOrders.length === 1) await askOrderNotices().catch(() => false);
      return;
    }
    if (approved?.confirmer && paymentReviewStop(approved, now())) { await keepPayment(); return; }
    setOrderFocus(false);
  }

  /** The order on this page can be left: only this phone carries the board's signature, and it never sent one. */
  function canStartNew() {
    const sale = point?.order;
    if (!sale || !client || !policy || !paysThroughEscrow(policy) || pickupMode || travelBranch || point.cart.pickup) return false;
    if (order?.id === sale && ['checking', 'paid'].includes(order.state)) return false;
    if (openOrders.some(row => row.sale === sale && row.stage !== 'failed')) return false;
    return !approved || (approved.order.id === sale && neverSent(approved));
  }

  /**
   * Leaves the unpaid order of this page. `again` puts its lines back into the basket to change them; without it the
   * basket is emptied. An order made from a store page is taken back at the store first (storeAccess.cancelCart), so
   * its stock is released and the account's one unpaid cart is free at once, not when the hold runs out. A paid
   * order is not left here.
   */
  async function startNew(again = true) {
    const sale = point?.order;
    if (!sale || !Http || !canStartNew()) throw new Error('ORDER_PENDING');
    const revision = loadRevision.current;
    // The record is written before a signature leaves; one that cannot be read counts as sent.
    const sent = await orderRecords(Http).list().then(rows => rows.some(row => row.sale === sale && row.stage !== 'failed'), () => true);
    if (!mounted.current || revision !== loadRevision.current) return;
    if (sent) throw new Error('ORDER_PENDING');
    // The store is asked whenever this phone can ask without the board: the hold's end is the store's time, and a
    // phone whose clock is ahead would otherwise forget a cart the store still holds. Only without a session does
    // the phone go by its own clock (holdOver): such a hold is over at the store without a word from this phone,
    // nothing was sent that could still pay the order (checked above), and the board is not asked to prove its
    // account just to cancel it. What the page then says is the phone's reading, not the store's answer.
    const over = holdOver(point, approved, now());
    const taken = !storeAccess ? undefined
      : over && !storeAccess.live({purpose: 'checkout', branch: storeAccess.store.branch}) ? 'expired' as const
      : await storeAccess.cancelCart(sale).catch((error: unknown) => {
        if (over && error instanceof SessionNeeded) return 'expired' as const; // the store no longer knew the session
        throw error;
      });
    if (!mounted.current || revision !== loadRevision.current) return;
    // A payment arrived or is being checked: the order stays, and its result is looked at first.
    if (taken === 'paid') throw new Error('ORDER_PENDING');
    boardReview.current = undefined;
    ownerConnection.current?.close();
    ownerConnection.current = undefined;
    const selection = point.selection ?? {};
    if (approved) await keepPayment();
    await save();
    if (!mounted.current || revision !== loadRevision.current) return;
    setOrder(undefined);
    setOrderBlocked(undefined);
    setOrderBoard(undefined);
    setOrderFocus(true);
    setCart(again ? selection : {});
    setMessage(`${again ? '새로 주문할 수 있습니다.' : '주문을 그만두었습니다.'} ${taken === 'released' ? '이전 주문은 취소되어 잡아 둔 재고가 풀렸습니다.'
      : taken === 'expired' ? '이전 주문은 결제 시간이 지난 것으로 보입니다. 매장의 취소 확인은 받지 않았습니다. 매장이 아직 닫지 않았다면 곧 닫고, 그때까지는 새 주문이 접수되지 않을 수 있습니다.'
      : taken === 'quote-issued' ? '이전 주문은 결제 견적이 나와 있어 취소되지 않았습니다. 견적 시간이 끝나면 매장이 닫고, 그때까지는 새 주문이 접수되지 않을 수 있습니다.'
      : '이전 주문은 결제되지 않았고 매장이 닫습니다.'}`);
  }

  /**
   * Leaves the order, and says what is true when it cannot be left. Only an order whose payment may have gone out
   * is "pending"; a cancel that did not reach the store is said as that.
   */
  async function leaveOrder(again: boolean) {
    try {
      await startNew(again);
    } catch (error) {
      if (!(error instanceof Error) || error.message !== 'ORDER_PENDING') throw error;
      if (mounted.current) setMessage(`이 주문의 결제 요청이 나갔을 수 있어 ${again ? '새로 주문할' : '그만둘'} 수 없습니다. 결과를 먼저 확인하세요.`);
    }
  }
  const leaveFailed = '매장에 주문 취소를 전하지 못했습니다. 주문은 그대로 있습니다. 연결을 확인하고 다시 시도하세요.';

  function orderPaymentView(): OrderPaymentView | undefined {
    if (orderOutcome) return orderOutcome;
    if (orderBlocked) return {kind: 'confirm', store: shop?.identity.branchLabel ?? orderBlocked.sale.cafeReview?.branchLabel ?? '매장',
      amount: orderMoney(orderBlocked.sale.amount, orderBlocked.sale.asset, policy, 'charge'), items: [],
      deadline: orderBlocked.until, spent: orderBlocked.spent, block: orderBlocked.block};
    if (travelBranch) return undefined;
    const entry = visibleReview ? approved : undefined;
    if (!entry?.confirmer || !orderFocus) return undefined;
    const shown = {store: entry.store ?? '매장', amount: orderMoney(entry.order.amount, entry.order.asset, policy, 'charge')};
    const board = orderBoard?.request === entry.request ? orderBoard : undefined;
    if (stoppedReview) {
      const signed = !!entry.boardSigned || !!entry.authorization;
      const answered = stoppedReview === 'refused' || (stoppedReview === 'cancelled' && signed);
      return {...shown, kind: 'stopped', reason: stoppedReview, spent: signed, until: answered ? entry.quote.expiresAt : undefined,
        fresh: point?.order === entry.order.id && canStartNew()};
    }
    if (entry.authorization && !(board?.block === 'key-lost' && !entry.forwardAttempted)) {
      if (!entry.forwardAttempted) return busy ? {...shown, kind: 'submitting'} : {...shown, kind: 'unknown', unsent: true};
      return busy || (entry.forwardAccepted && !orderSilent) ? {...shown, kind: 'submitting'} : {...shown, kind: 'unknown'};
    }
    if (boardWaiting) return {...shown, kind: 'waiting', deadline: entry.quote.expiresAt};
    return {
      ...shown, kind: 'confirm', deadline: entry.quote.expiresAt, block: board?.block, pinWait: board?.pinWait,
      board: board?.view, boards, searching: busy && !boards.length && !board, resumed: !!entry.boardStarted && !busy,
      items: point?.order === entry.order.id ? Object.values(point.selection ?? {}).map(({row, quantity}) => {
        const names = commerceMenuLabels(row, menuLocale);
        const line = commerceLineTax(row, quantity, policy);
        return {label: `${names.name} ×${quantity}${line ? ` · 세금 ${orderMoney(line.tax.toString(), row.asset, policy, 'tax')} 포함` : ''}`,
          amount: cartMoney((BigInt(row.unitPrice) * BigInt(quantity)).toString(), row.asset, policy, 'charge')};
      }) : [],
    };
  }

  async function findOrderBoard() {
    setOrderBoard(undefined);
    const revision = loadRevision.current;
    if (!(await storePolicyReady())) return;
    const found = await findBoard();
    if (!mounted.current || revision !== loadRevision.current) return;
    if (found.length === 1) await checkOrderBoard(found[0]);
    else if (!found.length) setMessage('주변에서 내 보드를 찾지 못했습니다. 보드 전원을 켜고 가까이 두세요.');
  }

  function recordOwnerReceipt(view: CommerceOrder, actor: CommerceClient) {
    const review = approved;
    const original = review?.route === 'nearby' && review.order.id === view.id ? review.order : undefined;
    if (original && (view.amount !== original.amount || view.asset !== original.asset ||
        (pendingNearby?.order.id === view.id && (view.state === 'preview' || view.state === 'issued'))))
      throw new Error('PAYMENT_ORDER_STATE');
    setUnpaidNearby(view.state === 'closed' && view.payment?.stage === 'unpaid' && view.payment.nearbyUnpaid ? view : undefined);
    if (!original || view.state !== 'checking' || !view.payment) return;

    const branch = actor.profile.branchRef;
    const revision = loadRevision.current;
    const label = shop?.identity.branch === branch ? shop.identity.branchLabel : undefined;
    setPendingNearby({order: view, branch, label});
    if (label) return;
    void actor.shopIdentity().then(identity => {
      if (!mounted.current || revision !== loadRevision.current) return;
      setPendingNearby(current => current?.order === view ? {...current, label: identity.branchLabel} : current);
    }).catch(() => {}); // The authenticated branch reference remains available.
  }

  function paymentReceiptEndsReview(view: CommerceOrder) {
    return view.state === 'paid' || (view.state === 'closed' &&
      (approved?.route !== 'nearby' || approved.order.id !== view.id || view.payment?.stage === 'unpaid'));
  }

  async function observePayment(view: CommerceOrder, actor: CommerceClient) {
    if (approved?.confirmer && approved.order.id === view.id) {
      await observeOrder(view, approved, true);
      return;
    }
    const revision = loadRevision.current;
    recordOwnerReceipt(view, actor);
    const terminal = paymentReceiptEndsReview(view);
    if (terminal) await keepPayment();
    if (!mounted.current || revision !== loadRevision.current) return;
    if (terminal) setPendingNearby(current => current?.order.id === view.id ? undefined : current);
    setLookupOrder(view.id);
    setOrder(view);
  }

  async function readHistoryOrder(sale: string) {
    if (!client || (travelPayment && sale !== travelSale)) return;
    const revision = loadRevision.current;
    setLookupOrder(sale);
    if (!travelPayment) setOrder(undefined);
    const status = await client.status(sale);
    if (!matchesTravelOrder(status)) throw new Error('TRAVEL_ORDER');
    if (!mounted.current || revision !== loadRevision.current) return;
    if (approved?.confirmer && approved.order.id === sale) {
      await observeOrder(status, approved, true);
      setOrderFocus(true);
      return;
    }
    recordOwnerReceipt(status, client);
    await keepStatus(status);
    const terminal = paymentReceiptEndsReview(status);
    if (approved?.order.id === sale && terminal) await keepPayment();
    if (mounted.current && revision === loadRevision.current) {
      if (terminal) setPendingNearby(current => current?.order.id === status.id ? undefined : current);
      setOrder(status);
      if (travelPayment) setTravelOrderObservedAt(Date.now());
    }
  }

  /** Records a status read over the authenticated server connection. */
  async function keepStatus(status: CommerceOrder) {
    if (!Http || !client || !status.escrow) return;
    const revision = loadRevision.current;
    const records = orderRecords(Http);
    const recorded = (await records.list()).find(row => row.sale === status.id);
    if (recorded) await recordOrder(recorded, status);
    const rows = (await records.list()).filter(row => row.branch === client.profile.branchRef && row.customer === client.profile.subjectRef);
    if (mounted.current && revision === loadRevision.current) setOpenOrders(rows);
  }

  async function openOrderDetail(sale: string) {
    setDetailSale(sale);
    await readHistoryOrder(sale);
  }

  async function answerOrder(action: OrderBuyerAction) {
    const recorded = openOrders.find(row => row.sale === detailSale);
    if (!client || !recorded || order?.id !== recorded.sale) throw new Error('ORDER_ANSWER_SCOPE');
    const revision = loadRevision.current;
    if (storeAccess) {
      // An answer goes with the session of this one order; the board that paid proves its account for it once.
      const problem = await storeAccess.ensure({purpose: 'order', branch: recorded.branch, sale: recorded.sale});
      if (!mounted.current || revision !== loadRevision.current) return;
      adoptStorePolicy();
      setStoreProblem(problem);
      if (problem) return;
    }
    let failure = '';
    try {
      await sendOrderAnswer(client, recorded, order, action, now());
    } catch (error) {
      const reason = error instanceof Error ? error.message : '';
      failure = reason === 'ORDER_KEY_LOST' || reason === 'ORDER_KEY_UNREADABLE' ? '이 휴대전화의 주문 확인 키를 찾을 수 없어 답을 보낼 수 없습니다. 매장이나 운영자에게 문의하세요.'
        : reason === 'CHECKOUT_CONFLICT' ? '주문이 이미 정해졌거나 지금은 답할 수 없습니다. 아래 상태를 확인하세요.'
        : reason === 'ORDER_ANSWER_CLOSED' ? '지금은 보낼 수 없는 답입니다. 아래 상태를 확인하세요.'
        : reason === 'EXPIRED' ? '요청 시간이 지나 답을 보내지 못했습니다. 휴대전화 시각이 맞는지 확인하고 다시 답하세요.'
        : reason === 'MISMATCHED_EVIDENCE' ? '서버가 이 휴대전화의 서명을 받아들이지 않았습니다. 매장이나 운영자에게 문의하세요.'
        : reason === 'NOT_AUTHORIZED' ? '이 휴대전화의 주문 등록으로는 답할 수 없습니다. 매장 담당자에게 새 등록서를 받으세요.'
        : reason === 'BAD_REQUEST' || reason === 'UNSUPPORTED_CAPABILITY' || (error as {code?: string}).code === 'COMMERCE_REFUSED'
        ? '이 매장의 서버가 답을 받지 않았습니다. 잠시 뒤 다시 답하거나 매장에 문의하세요.'
        : reason === 'REPLAY_CONFLICT' || reason === 'ORDER_ANSWER_PENDING' ? '먼저 보낸 답을 확인하고 있습니다. 다른 답은 보낼 수 없습니다.'
        : reason === 'ORDER_ANSWER_SCOPE' ? '주문 정보가 결제할 때와 달라 답을 보내지 않았습니다. 매장이나 운영자에게 문의하세요.'
        : registrationExpired ? orderOffline : '답이 도착했는지 확인하지 못했습니다. 인터넷 연결을 확인하고 같은 답을 다시 보내세요.';
    }
    const status = await client.status(recorded.sale).catch(() => undefined);
    if (!mounted.current || revision !== loadRevision.current) return;
    if (status) { await keepStatus(status); setOrder(status); }
    else if (Http) setOpenOrders((await orderRecords(Http).list()).filter(row => row.branch === client.profile.branchRef && row.customer === client.profile.subjectRef));
    if (mounted.current && revision === loadRevision.current && failure) setMessage(failure);
  }

  async function nextOrderPage() {
    if (!client || !moreOrders) return;
    const revision = loadRevision.current;
    const rows = await client.orders(orderRows.at(-1)?.id);
    if (!mounted.current || revision !== loadRevision.current) return;
    setOrderRows([...orderRows, ...rows]);
    setMoreOrders(rows.length === 50);
  }

  async function readInvoice(sale: string) {
    if (!client) return;
    const revision = loadRevision.current;
    setOrder(undefined);
    const status = await client.status(sale);
    if (!mounted.current || revision !== loadRevision.current) return;
    if (approved?.order.id === sale && ['paid', 'closed'].includes(status.state))
      await keepPayment();
    if (!mounted.current || revision !== loadRevision.current) return;
    setOrder(status);
    setInvoiceDetail(true);
    setShowSubscriptions(false);
  }

  async function keepTicket(value?: {
    order: string;
    key: string;
    code?: string;
  }) {
    if (!client || !journals) throw new Error('CLIENT_REQUIRED');
    const revision = loadRevision.current;
    await journals.write('qr', value && {
      ...value, branch: client.profile.branchRef, subject: client.profile.subjectRef,
    });
    if (mounted.current && revision === loadRevision.current) setTicket(value);
  }

  async function checkTicket(
    current: { order: string; key: string; code?: string },
    initiate: boolean,
  ) {
    if (!client) throw new Error('CLIENT_REQUIRED');
    const revision = loadRevision.current;
    const claim = await client.claim(current.order);
    if (!mounted.current || revision !== loadRevision.current) return;
    if (claim) {
      const result = await client.status(current.order);
      if (!mounted.current || revision !== loadRevision.current) return;
      if (result.escrow || ['paid', 'closed'].includes(result.state)) {
        // Nothing is left to connect; the order itself is in the list.
        await keepTicket();
        return;
      }
      setOrder(result);
      setMessage('매장 단말에서 이 휴대전화의 연결을 확인했습니다.');
      // The review button below still works if this attempt does not.
      if (policy && paysThroughEscrow(policy) && canReplaceReview(result.id) && ['preview', 'issued'].includes(result.state))
        await preparePayment(result).catch(() => {});
      return;
    }
    if (!initiate) {
      setMessage(
        '주문 연결을 기다리고 있습니다. 매장 단말에 같은 확인 번호가 있는지 대조하세요.',
      );
      return;
    }
    await keepTicket(current);
    const found = await client.requestComparison(current.order, current.key);
    if (!mounted.current || revision !== loadRevision.current) return;
    await keepTicket({ ...current, code: found.code });
    setMessage(
      '이 확인 번호를 매장 단말의 번호와 대조하고 같은 번호를 확인하세요.',
    );
  }

  // The store confirms the number on its own screen, so the phone asks for the result by itself.
  const awaited = useRef<() => void>(undefined);
  awaited.current = () => { if (ticket) work(() => checkTicket(ticket, false), '연결 결과가 아직 확인되지 않았습니다.'); };
  const awaiting = phase === 'active' && !!ticket?.code && !order && !approved && !shopEntry && !pickupMode;
  useEffect(() => {
    if (!awaiting) return;
    let left = 40; // the kiosk shows its number for a minute
    const timer = setInterval(() => { if (left-- > 0) awaited.current?.(); else clearInterval(timer); }, 3000);
    return () => clearInterval(timer);
  }, [awaiting]);

  async function fromQr(raw: string) {
    if (!client) throw new Error('CLIENT_REQUIRED');
    const located = parseOrderLocator(raw, now());
    if (located.branchRef !== client.profile.branchRef)
      throw new Error('WRONG_BRANCH');
    if (link) {
      const revision = loadRevision.current;
      const direct = await client.status(located.orderRef).catch(() => undefined);
      if (!mounted.current || revision !== loadRevision.current) return;
      if (direct?.cafeReview) {
        setOrder(direct);
        setLocator('');
        return;
      }
      await nearbyCheckout({ order: located.orderRef, key: await walletId() });
      return;
    }
    if (ticket && ticket.order !== located.orderRef)
      throw new Error('PENDING_QR');
    const current = ticket ?? {
      order: located.orderRef,
      key: await walletId(),
    };
    await checkTicket(current, true);
    setLocator('');
  }

  const nearbyPending = pendingNearby?.order === order ? pendingNearby : undefined;
  const nearbyUnpaid = !!unpaidNearby && unpaidNearby === order;
  const nearbyClosed = approved?.route === 'nearby' && order?.id === approved.order.id && order.state === 'closed';
  const pendingReadOnly = nearbyClosed || (!!pendingNearby && pendingNearby.order.id === (order?.id ?? lookupOrder));
  const stoppedReview = approved && paymentReviewStop(approved, Math.max(reviewClock, now()), reviewWaiting);
  const reviewExpired = stoppedReview === 'expired';
  useEffect(() => { if (reviewExpired) setMessage(''); }, [reviewExpired]);
  const visibleReview = approved && (!order || approved.order.id === order.id) &&
    (!travelPayment || approved.route === 'direct' && matchesTravelOrder(approved.order) && !!order);
  const verifiedReview = useMemo(() => {
    if (!approved || !policy || (approved.reviewStop !== undefined)) return undefined;
    try {
      const verificationTime = approved.authorization ? approved.quote.expiresAt - 1 : Math.max(reviewClock, now());
      return reviewedPayment(approved.quote, approved.order, policy, verificationTime, approved.scope);
    } catch {
      return undefined;
    }
  }, [approved, policy, reviewClock]);
  const gasReview = useMemo(() => {
    if (!approved || !policy || !needsMerchantSponsorship(approved) || stoppedReview || !verifiedReview) return undefined;
    try {
      return {proof: verifyPresentedGasSponsor(approved.quote, approved.order, policy, Math.max(reviewClock, now()), approved.scope)};
    } catch (error) {
      return {problem: error instanceof Error ? error.message : ''};
    }
  }, [approved, policy, reviewClock, stoppedReview, verifiedReview]);
  const rentalEscrowNeeded = !!approved?.order.cafeReview && verifiedReview?.payerRole === 'renter';
  let rentalEscrowVerified = false;
  if (rentalEscrowNeeded && approved && policy && !stoppedReview) {
    try {
      verifyPresentedRentalEscrow(approved.quote, approved.order, policy, Math.max(reviewClock, now()), approved.scope);
      rentalEscrowVerified = true;
    } catch {}
  }
  const canStartBoard = !!verifiedReview && (!travelPayment || order?.state === 'issued') &&
    (!approved || !needsMerchantSponsorship(approved) || !!gasReview?.proof) &&
    !rentalEscrowNeeded;
  const rentalReviewOpen = !rentalEscrowNeeded || !order || order.state === 'issued';
  const rentalBoardVerified = rentalEscrowNeeded && rentalEscrowVerified && rentalReviewOpen && !!link &&
    rentalBoardReview?.terminal === link && rentalBoardReview.checkpoint === approved?.request &&
    rentalBoardReview.verified && rentalBoardReview.expiresAt > now();
  const showSponsorshipDetails = approved && (
    approved.route === 'nearby' || !!approved.authorization || rentalBoardVerified ||
    (reviewWaiting && boardVerified === approved.request)
  );
  if (phase==='active' && showApprovalMaintenance && approvalDevice?.request===approved?.request && approvalDevice && approved) {
    const stillPending = reviewWaiting || approved.boardStarted && !approved.authorization && approved.reviewStop!=='refused';
    const live = reviewWaiting && approvalProgress?.request===approved.request && approvalProgress.expires>BigInt(now());
    return <ScrollView contentContainerStyle={s.scroll} keyboardShouldPersistTaps="handled">
      <WalletMaintenance language={menuLocale} scope={approvalDevice.scope} active signing={false} unsettled={false}
        firmwareOnly approvalHold={stillPending?(live?'board':'unknown'):undefined} networkBusy={false}
        devices={[{address:approvalDevice.handle,name:'원승인 기기'}]} scan={()=>{}}
        read={()=>readApprovalDevice(approvalDevice,session=>session.maintenanceStatus())}
        campaign={(_:string,command:string,expected:OwnerStatus)=>readApprovalDevice(approvalDevice,session=>session.firmwareProof(command,expected))}
        back={()=>setShowApprovalMaintenance(false)} backLabel="원승인으로 돌아가기" backup={()=>{}} orders={()=>{}} />
    </ScrollView>;
  }
  if (phase === 'active' && formerOrder) return <ScrollView contentContainerStyle={s.scroll}>
    <FormerOrderDetail order={formerOrder} amount={orderMoney(formerOrder.amount, formerOrder.asset, policy, 'charge')} back={back} backLabel="매장 화면으로" />
  </ScrollView>;
  const orderView = phase === 'active' && client && !showSubscriptions ? orderPaymentView() : undefined;
  const protectionNotice = <View style={s.scroll}>
    <Text style={s.text} accessibilityLiveRegion="polite">{protection.failed ? '결제 화면 보호를 시작하지 못했습니다.' : '결제 화면을 준비하고 있습니다.'}</Text>
    {protection.failed && <Button label="다시 시도" run={protection.retry} />}
  </View>;
  if (orderView && !protection.on) return protectionNotice;
  if (orderView) {
    // Each step of the payment starts at its top, whatever was scrolled before.
    return <ScrollView key={orderView.kind} contentContainerStyle={s.scroll} keyboardShouldPersistTaps="handled">
      <OrderPayment view={orderView} busy={busy} storePage={!!storeAccess} message={message || (storeProblem ? accessProblemText(storeProblem, !!approved) : '')}
        t={text => menuLocale === 'en' ? walletEnglish[text] ?? text : text}
        actions={{
          findBoard: () => work(findOrderBoard, '보드를 찾지 못했습니다. 보드 전원과 블루투스 권한을 확인하세요.'),
          checkBoard: handle => work(() => checkOrderBoard(handle), '보드 상태를 확인하지 못했습니다. 다시 시도하세요.'),
          approve: () => work(() => acceptBoard(orderBoard!.handle!), '보드 승인 결과를 확인하지 못했습니다. 같은 보드에서 다시 확인하세요.'),
          cancel: cancelOrder,
          again: () => work(() => preparePayment(orderBlocked?.sale ?? approved!.order),
            (approved?.quote.expiresAt ?? orderBlocked?.until ?? 0) <= now()
              ? '이 주문은 결제 기한이 끝나 다시 받을 수 없습니다. 키오스크에서 새로 주문하세요.'
              : '결제 내용을 다시 받지 못했습니다. 키오스크에 다시 연결한 뒤 시도하세요.'),
          openBoard: () => openBoard?.(),
          check: () => work(reconcilePayment, '아직 결과를 받지 못했습니다. 다시 결제하지 말고 잠시 뒤 확인하세요.'),
          resend: () => work(() => submitLock(approved!), '아직 보내지 못했습니다. 키오스크에 다시 연결하고 시도하세요.'),
          close: () => work(closeOrder, '결제 기록을 정리하지 못했습니다.'),
          startNew: () => work(() => leaveOrder(true), leaveFailed),
        }} />
    </ScrollView>;
  }
  if (phase === 'active' && travelPayment && visibleReview && !stoppedReview && approved &&
      verifiedReview && policy && recipientQuote === approved.quote.quoteDigest) {
    return (
      <ScrollView contentContainerStyle={s.scroll}>
        <Text style={s.title} accessibilityRole="header">수취 주소 확인</Text>
        <Text style={s.text} selectable>서명 견적 수령 주소 {policy.beneficiary}</Text>
        <Text style={s.text}>이번 지급 {orderMoney(approved.order.amount, approved.order.asset, policy, 'charge')}</Text>
        <Button label="돌아가기" run={() => setRecipientQuote(undefined)} />
      </ScrollView>
    );
  }


  const recordedDetail = openOrders.find(row => row.sale === (detailSale || order?.id));
  const otherDetail = !recordedDetail && order?.escrow && !travelBranch && !pickupMode ? order : undefined;
  if (phase === 'active' && client && !travelBranch && !showSubscriptions && (recordedDetail && (detailSale || order) || otherDetail)) {
    if (!protection.on) return protectionNotice;
    detailShown.current = true;
    const shown: OpenOrder = recordedDetail ?? {...otherDetail!.escrow!, sale: otherDetail!.id, branch: client.profile.branchRef, customer: client.profile.subjectRef,
      generation: '', asset: otherDetail!.asset, amount: otherDetail!.amount, sentAt: Math.floor(Date.parse(otherDetail!.payment?.includedAt ?? otherDetail!.expiresAt) / 1000),
      pickupNo: otherDetail!.pickupNo, pickup: otherDetail!.payment?.pickupStage};
    const entrance = configuredEntrance();
    // Once the store records the handover the order reads as collected, and the detail comes back by itself.
    if (recordedDetail && pickupSale === shown.sale && shown.stage === 'locked' && shown.pickup === 'ready')
      return <ScrollView contentContainerStyle={s.scroll}>
        <PickupQrScreen key={shown.sale} order={shown} issue={entrance && (() => issuePickupQr(entrance, ownerPickupBoard, shown))}
          back={() => setPickupSale('')} />
      </ScrollView>;
    return <ScrollView contentContainerStyle={s.scroll}>
      <OrderDetail order={shown} amount={orderMoney(shown.amount, shown.asset, policy, 'charge')} money={units => orderMoney(units, shown.asset, policy, 'charge')}
        token={policy?.asset === shown.asset ? paymentToken(policy) : undefined} fresh={order?.id === shown.sale} mine={!!recordedDetail} former={isFormerBoard(shown, boardAccount)}
        busy={busy} message={message || (storeProblem ? accessProblemText(storeProblem) : '')} now={now()} actions={{
          answer: action => work(() => answerOrder(action), '답을 보내지 못했습니다. 상태를 다시 확인하세요.'),
          refresh: () => work(() => openOrderDetail(shown.sale), orderOffline),
          back: closeDetail,
          ...(entrance ? {pickupQr: () => setPickupSale(shown.sale)} : {}),
        }} />
    </ScrollView>;
  }
  const menuMoney = (units: string, asset: string) => orderMoney(units, asset, policy, 'charge');
  // What the store's server said of this very basket when it did not hold it; a changed basket is another one.
  const serverShort = shortHold && shortHold.basket === cart ? shortHold : undefined;
  // Known only for a store page's order, once the balance was read: the basket costs more than the board account holds.
  const cartShort = !!storeAccess && (!!serverShort || (typeof boardBalance === 'bigint' &&
    commerceCartTotals(cart).some(row => row.asset === policy?.asset && BigInt(row.amount) > boardBalance)));
  const oneOptionOnly = menuLocale === 'ko'
    ? '이 매장에서는 품목당 옵션을 하나씩 담을 수 있습니다. 옵션을 바꾸려면 담은 옵션을 먼저 빼세요.'
    : 'This store accepts one option per item. Remove the selected option to choose another.';
  const choiceRows = choosing ? catalog.filter(row => row.sku === choosing && row.choices) : [];
  if (phase === 'active' && client && !deniedLink && !showSubscriptions && !point && !menuReadOnly && (shop || pickupMode) && choiceRows.length) {
    return <MenuChoices key={choosing} rows={choiceRows} cart={cart} locale={menuLocale} money={menuMoney} disabled={busy} photos={photos}
      blocked={row => row.skuAvailable === undefined && commerceHasSiblingOption(row, cart) ? oneOptionOnly : undefined}
      add={(row, quantity) => {
        setCart(old => quantity <= commerceLineCapacity(row, old) ? {...old, [commerceLineKey(row)]: {row, quantity}} : old);
        setChoosing('');
      }}
      back={() => setChoosing('')} />;
  }
  const pairView = pair && (
            <>
              {pair.code && <Text style={s.text}>확인 번호 {pair.code}</Text>}
              <Button
                label="단말 연결 상태 확인"
                disabled={busy}
                run={() =>
                  work(
                    () => nearbyCheckout(pair),
                    '단말의 공개 주문과 연결할 수 없습니다.',
                  )
                }
              />
            </>
          );
  const qrView = <View style={s.scroll}>
          {pairView}
          {ShopQr && openPayments && <Button label="키오스크 QR 스캔" disabled={busy}
            run={() => work(scanOrderQr, '카메라를 사용할 수 없습니다. QR 내용을 입력하거나 다시 시도하세요.')} />}
          <TextInput placeholderTextColor={inputPlaceholder}
            accessibilityLabel="매장 주문 QR 내용"
            placeholder="카메라를 사용할 수 없으면 QR 내용 입력"
            autoCapitalize="none"
            maxLength={132}
            style={s.input}
            value={locator}
            onChangeText={setLocator}
          />
          <Button
            label="QR 주문 연결"
            variant={locator ? 'primary' : 'secondary'}
            disabled={busy || !locator}
            run={() =>
              work(
                () => fromQr(locator),
                ticket ? '먼저 연결한 주문이 남아 있습니다. 그 주문을 확인하거나 연결 화면을 닫은 뒤 다시 시도하세요.'
                  : 'QR이 만료되었거나 등록 지점과 다릅니다. 키오스크에 새로 나온 QR로 다시 시도하세요.',
              )
            }
          />
          {ticket && (
            <>
              {ticket.code && (
                <Text style={s.text}>매장 확인 번호 {ticket.code}</Text>
              )}
              <Button
                label="같은 주문 연결 확인"
                disabled={busy}
                run={() =>
                  work(
                    () => checkTicket(ticket, false),
                    '연결 결과가 아직 확인되지 않았습니다.',
                  )
                }
              />
              {!approved && (
                <Button
                  label="연결 화면 닫기"
                  disabled={busy}
                  run={() =>
                    work(async () => {
                      await keepTicket();
                      setOrder(undefined);
                    }, '저장된 상태를 닫을 수 없습니다.')
                  }
                />
              )}
            </>
          )}
  </View>;
  const title = invoiceDetail && !showSubscriptions ? '구독 회차 청구서' : travelPayment ? '원예약 지급 주문'
    : travelBranch || initialLocator || shopEntry ? '내 매장 주문' : '내 결제와 미결 청구';
  return (
    <ScrollView
      ref={page}
      key={showSubscriptions ? 'subscriptions' : invoiceDetail ? 'invoice' : 'orders'}
      contentContainerStyle={s.scroll}
      keyboardShouldPersistTaps="handled"
      keyboardDismissMode="on-drag"
    >
      {tabbed ? <TabHeader title={title} />
        : <Text style={s.title} accessibilityRole="header">{title}</Text>}
      {boardWaiting && <BoardStepPanel wait={{flow: 'sign', step: 'display'}}
        t={text => menuLocale === 'en' ? walletEnglish[text] ?? text : text} />}
      {!!message && (
        <Text style={s.text} accessibilityLiveRegion="polite">
          {message}
        </Text>
      )}
      {phase === 'check' && <Text style={s.text}>{storeBranch ? '매장 메뉴를 여는 중' : '등록 상태 확인 중'}</Text>}
      {(phase === 'locked' || phase === 'setup') && !storeRoute && !!stranded.length && <Card>
        <Text accessibilityRole="header" style={s.cardTitle}>{`진행 중인 주문이 ${stranded.length}건 있습니다`}</Text>
        {stranded.map(row => <Text key={row.sale} style={s.text}>
          {`${row.store ?? '매장 주문'}${row.pickupNo === undefined ? '' : ` · 주문번호 ${row.pickupNo}`} · ${openOrderLabel(row)}`}</Text>)}
        <Text style={s.text}>상태를 보거나 받았다고 답하려면 인터넷 주문 등록이 필요합니다. 매장 담당자에게 이 휴대전화의 주문 등록 공개키를 보여 주고 새 등록서를 받아 이 화면에 입력하세요.</Text>
      </Card>}
      {/* A build that orders without a registration: orders made from a store page open with that store, and the
          way to a new order is the store finder. No registration is asked for and no form for one is shown. */}
      {storeRoute && !storeBranch && (phase === 'locked' || phase === 'setup') && (() => {
        const made = stranded.filter(madeAtStorePage), others = stranded.filter(row => !madeAtStorePage(row));
        return <>
          {made.length > 0 ? <View style={s.inProgress}>
            <Text accessibilityRole="header" style={s.cardTitle}>진행 중</Text>
            {made.map(row => <OpenOrderCard key={row.sale} order={row} former={isFormerBoard(row, boardAccount)}
              open={() => storeRoute.open({branch: row.branch, name: row.store ?? '매장', merchantId: row.merchantId}, row.sale)} />)}
          </View> : !others.length && <Text style={s.text}>진행 중인 주문이 없습니다.</Text>}
          {others.length > 0 && <Card>
            <Text accessibilityRole="header" style={s.cardTitle}>{`진행 중인 주문이 ${others.length}건 있습니다`}</Text>
            {others.map(row => <Text key={row.sale} style={s.text}>
              {`${row.store ?? '매장 주문'}${row.pickupNo === undefined ? '' : ` · 주문번호 ${row.pickupNo}`} · ${openOrderLabel(row)}`}</Text>)}
            <Text style={s.text}>이 앱에서는 이 주문의 상태를 볼 수 없습니다. 매장에 문의하세요.</Text>
          </Card>}
          <Text style={s.text}>메뉴는 누구나 볼 수 있습니다. 주문할 때 보드로 본인을 확인합니다.</Text>
          <Button label="매장 찾기" run={storeRoute.find} />
        </>;
      })()}
      {phase === 'locked' && !storeBranch && !storeRoute && (
        <Text style={s.text}>
          보호된 주문 등록을 열 수 없습니다. 발급 담당자에게 문의하세요.
        </Text>
      )}
      {!!storeBranch && phase === 'locked' && !storeProblem && <Text style={s.text}>매장 주문을 열지 못했습니다. 다시 시도하세요.</Text>}
      {storeProblem && <View style={s.line}>
        <Text accessibilityRole="alert" style={s.text}>{accessProblemText(storeProblem)}</Text>
        {(storeProblem.kind === 'no-account' || storeProblem.kind === 'board-changed') && openBoard &&
          <Button label="내 보드 열기" variant="secondary" run={openBoard} />}
      </View>}
      {!!storeBranch && phase === 'locked' && <Button label="매장 화면으로" variant="secondary" run={back} />}
      {phase === 'setup' && !storeRoute && (
        <>
          {/* Before registration the payments tab explains the three steps instead of opening on a bare form. */}
          <Card>
            <Text accessibilityRole="header" style={s.cardTitle}>매장 결제를 시작하려면</Text>
            {['매장 담당자에게 이 휴대전화의 주문 등록 공개키를 보여 주세요.',
              '받은 주문 등록서와 두 지문을 아래에 입력하세요.',
              '등록을 확인하면 이 휴대전화의 주문과 결제가 여기에 모입니다.'].map((step, index) =>
              <Text key={step} style={s.text}>{`${index + 1}. ${step}`}</Text>)}
          </Card>
          <Text accessibilityRole="header" style={s.cardTitle}>이 휴대전화의 공개키</Text>
          <Button
            label="주문 등록 공개키 보기"
            variant="row"
            detail="매장 담당자에게 보여 줄 주문용 키"
            disabled={busy}
            run={() =>
              work(
                async () => setPub(await Http!.publicIdentity('customer')),
                '이 휴대전화에서 공개키를 만들 수 없습니다.',
              )
            }
          />
          {Nearby && (
            <Button
              label="근거리 등록 공개키 보기"
              variant="row"
              detail="매장 단말과 가까이서 주문할 때 쓰는 키"
              disabled={busy}
              run={() =>
                work(
                  async () => setNearbyKey(await Nearby!.publicIdentity()),
                  '근거리 공개키를 읽을 수 없습니다.',
                )
              }
            />
          )}
          {!!nearbyKey && (
            <Text selectable style={s.text}>
              {nearbyKey}
            </Text>
          )}
          {!!pub && (
            <Text selectable style={s.text}>
              {pub}
            </Text>
          )}
          <Text accessibilityRole="header" style={s.cardTitle}>등록 정보</Text>
          <TextInput placeholderTextColor={inputPlaceholder}
            testID="customer-commerce-registration"
            accessibilityLabel="고객 주문 등록서"
            placeholder="고객 주문 등록서"
            style={[s.input, s.registrationInput]}
            editable={!busy}
            multiline
            maxLength={16384}
            autoCapitalize="none"
            autoCorrect={false}
            spellCheck={false}
            returnKeyType="next"
            submitBehavior="submit"
            onSubmitEditing={() => serverPinInput.current?.focus()}
            value={profile}
            onChangeText={setProfile}
          />
          <TextInput placeholderTextColor={inputPlaceholder}
            ref={serverPinInput}
            testID="customer-commerce-tls-pin"
            accessibilityLabel="독립 확인한 주문 서버 지문"
            placeholder="독립 확인한 주문 서버 지문"
            style={s.input}
            editable={!busy}
            maxLength={64}
            autoCapitalize="none"
            autoCorrect={false}
            spellCheck={false}
            returnKeyType="next"
            submitBehavior="submit"
            onSubmitEditing={() => presentationPinInput.current?.focus()}
            value={pin}
            onChangeText={setPin}
          />
          <TextInput placeholderTextColor={inputPlaceholder}
            ref={presentationPinInput}
            testID="customer-commerce-presentation-pin"
            accessibilityLabel="별도 발급한 비공개 주문 서명 지문"
            placeholder="별도 발급한 비공개 주문 서명 지문"
            style={s.input}
            editable={!busy}
            maxLength={64}
            autoCapitalize="none"
            autoCorrect={false}
            spellCheck={false}
            returnKeyType="done"
            onSubmitEditing={submitEnrollment}
            value={proofPin}
            onChangeText={setProofPin}
          />
          <Button
            label="주문 등록 확인"
            disabled={busy}
            run={submitEnrollment}
          />
        </>
      )}
      {phase === 'active' && client && !deniedLink && !showSubscriptions && !pickupMode && !travelPayment && !invoiceDetail && !initialLocator && !shopEntry && qrView}
      {phase === 'active' && client && !pickupMode && !travelPayment && !shopEntry && (
        <Button
          label={showSubscriptions ? '매장 주문으로' : invoiceDetail ? '구독 일정' : '구독 관리'}
          variant="secondary"
          disabled={busy}
          run={() => {
            if (showSubscriptions && invoiceDetail) {
              setInvoiceDetail(false);
              setOrder(undefined);
            }
            setShowSubscriptions(!showSubscriptions);
          }}
        />
      )}
      {phase === 'active' && client && showSubscriptions && (
        <SubscriptionScreen
          key={client.profile.branchRef + '/' + client.profile.subjectRef}
          client={client}
          policy={policy}
          keyFor={walletId}
          alertRef={subscriptionLink}
          backToPayments={openPayments ? () => openPayments() : undefined}
          onInvoice={sale => {
            work(() => readInvoice(sale), '회차의 현재 상태를 확인할 수 없습니다.');
          }}
        />
      )}
      {phase === 'active' && deniedLink && (
        <>
          <Text style={s.text}>이 휴대전화에 아직 연결하지 않았거나 이 계정에서 열 수 없는 주문입니다. 매장 단말의 QR이 맞으면 대조해 연결하세요.</Text>
          {openPayments && <Button variant="secondary" label="내 결제로" run={() => openPayments()} />}
          {openPayments && counterQrAllowed && (
            <Button label="매장 QR 대조하기" run={() => openPayments(initialLocator)} />
          )}
        </>
      )}
      {phase === 'active' && !travelBranch && !initialLocator && !showSubscriptions && !invoiceDetail && !shopEntry && (
        <View style={s.scroll}>
          {(openOrders.some(isOpenOrder) || !!approved?.confirmer) && <View style={s.inProgress}>
            <Text accessibilityRole="header" style={s.cardTitle}>진행 중</Text>
            {approved?.confirmer && !openOrders.some(row => row.sale === approved.order.id) &&
              <Button label="진행 중인 결제 이어서 보기" variant="secondary" run={() => { setOrder(undefined); setOrderFocus(true); }} />}
            {openOrders.filter(isOpenOrder).map(row => <OpenOrderCard key={row.sale} order={row} former={isFormerBoard(row, boardAccount)} open={() => {
              if (approved?.order.id === row.sale) { setOrder(undefined); setOrderFocus(true); }
              else work(() => openOrderDetail(row.sale), orderOffline);
            }} />)}
          </View>}
          {!!orderRows.length && <Section title="매장 주문" aside={`${orderRows.length}건`}>
            {orderRows.map(row => (
              <Button key={row.id} testID={`매장 주문 ${row.id}`} disabled={busy} variant="row"
                label={`${openOrders.find(item => item.sale === row.id)?.store ?? '매장 주문'} · ${orderMoney(row.amount, row.asset, policy, 'charge')}`}
                detail={orderRowDetail(row, openOrders.find(item => item.sale === row.id))}
                run={() => work(() => openOrders.some(item => item.sale === row.id) ? openOrderDetail(row.id) : readHistoryOrder(row.id), '이 계정에서 열 수 없는 주문입니다.')} />
            ))}
          </Section>}
          {moreOrders && <Button label="매장 주문 더 보기" disabled={busy}
            run={() => work(nextOrderPage, '매장 주문을 더 불러올 수 없습니다.')} />}
          {!!invoiceRows.length && <Section title="구독 청구" aside={`${invoiceRows.length}건`}>
            {invoiceRows.map(row => (
              <Button key={row.id} variant="row" label={`구독 회차 청구서 ${row.cycle} · ${orderMoney(row.amount, row.asset, policy, 'charge')} · ${orderStateLabel[row.state]}`} disabled={busy}
                run={() => work(() => readInvoice(row.saleRef), '이 계정에서 열 수 없는 청구서입니다.')} />
            ))}
          </Section>}
          {openTravel && client && <Button label="여행 예약" disabled={busy} variant="secondary"
            run={() => openTravel({branch: client.profile.branchRef, customer: client.profile.subjectRef, tenant: client.profile.tenantRef})} />}
          {historyState === 'unavailable' && client && (
            <Button label="결제 목록 다시 조회" disabled={busy}
              run={() => work(() => loadHistory(client, loadRevision.current), '결제 목록을 확인하지 못했습니다.')} />
          )}
          {historyState === 'ready' && !orderRows.length && !invoiceRows.length && (
            <Text style={s.text}>현재 등록의 매장 주문과 구독 청구서가 없습니다.</Text>
          )}
        </View>
      )}
      {phase === 'active' && !deniedLink && !showSubscriptions && (
        <View collapsable={false} style={s.scroll}>
          {travelPayment && <BookingPaymentTerms payment={travelPayment} policy={policy} />}
          {!travelPayment && !invoiceDetail && !initialLocator && <>
          {pickupMode && <>
            <Text style={s.text}>{pickupChoice ? `선택한 픽업 시간 · ${pickupChoice.label}` : '저장한 픽업 주문'}</Text>
            <Text style={s.text}>좌석은 예약하지 않습니다.</Text>
            {!pickupChoice && !point?.cart.pickup && !travelSale && <Text style={s.text}>이 기기에 저장한 픽업 주문이 없습니다.</Text>}
          </>}
          {!pickupMode && !shop && (
            <ShopDiscovery initialQr={entrancePending ? initialShopQr : undefined} offline={shopOffline} permissionDenied={shopPermissionDenied}
              find={findNearby} select={connectNearby} online={checkOnlineShop}
              readQr={scanShopQr} open={openConfirmedShop} reset={resetShop} wallet={back} page={openStorePage}
              recover={approved?.order.id || point?.order || ticket?.order ? recoverOriginal : undefined} />
          )}
          {!pickupMode && shop && (
            <>
              <Text accessibilityRole="header" style={s.title}>{`${shop.identity.branchLabel} 메뉴`}</Text>
              {menuReadOnly ? <>
                <Text accessibilityRole="header" style={s.text}>{menuLocale === 'ko'
                  ? '최근 메뉴를 보고 있습니다' : 'Viewing the last synced menu'}</Text>
                <Text style={s.text}>{menuLocale === 'ko' ? '마지막 조회 · ' : 'Last checked · '}
                  {menuCheckedAt.current !== undefined && new Date(menuCheckedAt.current * 1000).toLocaleTimeString(
                    menuLocale === 'ko' ? 'ko-KR' : 'en-US', {hour: '2-digit', minute: '2-digit'})}</Text>
                <Text style={s.text}>{menuLocale === 'ko' ? '신규 견적 발급 일시 중지' : 'New quotes paused'}</Text>
                <Button label={menuLocale === 'ko' ? '매장 상태 다시 확인' : 'Check store connection'}
                  disabled={busy} run={() => work(refreshShopMenu, '매장과 현재 메뉴를 다시 확인하세요.')} />
              </> : <Text style={s.text}>{storeAccess ? '메뉴는 누구나 볼 수 있습니다. 주문할 때 보드로 본인을 확인합니다.'
                : shop.route === 'relay' ? '온라인 단말 중계' : '휴대전화 온라인 연결'}</Text>}
              {storeAccess ? <Button label="매장 화면으로" variant="secondary" disabled={busy} run={back} />
                : <Button label="매장 다시 선택" disabled={busy} run={resetShop} />}
              {/* The payment tab is where an order's state is read: an order paid at this store is on its page too. */}
              {storeAccess && !point && openOrders.some(row => isOpenOrder(row) && madeAtStorePage(row)) && <View style={s.inProgress}>
                <Text accessibilityRole="header" style={s.cardTitle}>진행 중</Text>
                {openOrders.filter(row => isOpenOrder(row) && madeAtStorePage(row)).map(row => <OpenOrderCard key={row.sale} order={row}
                  former={isFormerBoard(row, boardAccount)} open={() => work(() => openOrderDetail(row.sale), orderOffline)} />)}
              </View>}
            </>
          )}
          {(shopEntry || pickupMode) && pairView}
          {!point && pickupMode && pickupChoice && (
            <Button
              label="지점 메뉴 보기"
              disabled={busy}
              run={() =>
                work(
                  getCatalog,
                  '서버 연결 대기 중입니다. 주문은 아직 접수되지 않았습니다.',
                )
              }
            />
          )}
          {!point && (shop || pickupMode) && (
            <StoreNotice state={storeState.state} refused={storeState.refused} checkedAt={storeStopped(storeState.state) ? storeState.checkedAt : undefined} />
          )}
          {/* A store page's menu that was read and has no row: nothing is on sale now. The store page says why. */}
          {!point && storeAccess && shop && !catalog.length && !!storeState.checkedAt && !storeStopped(storeState.state) && !message &&
            <Text style={s.text}>지금 판매 중인 메뉴가 없습니다. 주문 받는 시간은 매장 화면에서 확인하세요.</Text>}
          {!point && (shop || pickupMode) && storeStopped(storeState.state) && (
            <Button label="다시 받는지 확인" disabled={busy}
              run={() => work(async () => { setCatalog(await readCatalog()); }, '매장 상태를 확인하지 못했습니다. 다시 시도하세요.')} />
          )}
          {!point && (shop || pickupMode) && !!catalog.length && (
            <Button
              label={
                menuLocale === 'ko'
                  ? '메뉴 언어 English'
                  : 'Menu language 한국어'
              }
              run={() =>
                setMenuLocale(locale => (locale === 'ko' ? 'en' : 'ko'))
              }
            />
          )}
          {!point && (shop || pickupMode) && <ShopMenuCategories
            categories={categories} selected={selectedCategory} select={setMenuCategory} locale={menuLocale} />}
          {!point && !menuReadOnly && (shop || pickupMode) && catalog.some(row => row.skuAvailable === undefined && commerceHasSiblingOption(row, cart)) && (
            <Text style={s.text}>{oneOptionOnly}</Text>
          )}
          {!point && (shop || pickupMode) &&
            catalog.map(row => {
              if (selectedCategory && row.category?.key !== selectedCategory) return null;
              // Combinations are picked on their own page; the ones in the basket stay in the list, after the product.
              const card = row.choices && catalog.find(item => item.sku === row.sku) === row ? <ChoiceCard key={row.sku}
                rows={catalog.filter(item => item.sku === row.sku)} cart={cart} locale={menuLocale} money={menuMoney}
                pick={menuReadOnly ? undefined : {run: () => setChoosing(row.sku), disabled: busy}} /> : null;
              if (row.choices && !cart[commerceLineKey(row)]) return card;
              const names = commerceMenuLabels(row, menuLocale);
              const key = commerceLineKey(row);
              const quantity = cart[key]?.quantity ?? 0;
              const available = commerceLineCapacity(row, cart);
              const needsRemoval = row.skuAvailable === undefined && commerceHasSiblingOption(row, cart);
              return (
                <Fragment key={row.sku + '/' + row.optionKey}>{card}<View style={s.line}>
                  <Text style={s.text}>
                    {names.name} ·{' '}
                    {orderMoney(row.unitPrice, row.asset, policy, 'charge')}
                  </Text>
                  <Text style={s.text}>
                    {commerceMenuDisclosure(row, menuLocale)}
                  </Text>
                  {/* A menu read without a proof tells sold out or on sale, never how many are left (row.uncounted). */}
                  {!menuReadOnly && <Text style={s.text}>
                    {available > 0
                      ? row.uncounted ? `선택 ${quantity}` : `선택 ${quantity} · 가능 ${available}`
                      : menuLocale === 'ko'
                      ? '품절'
                      : 'Sold out'}
                  </Text>}
                  {!menuReadOnly && <Button
                    label={`${names.name === names.product ? names.product : `${names.product} ${names.option}`} ${
                      menuLocale === 'ko' ? '추가' : 'Add'
                    }`}
                    disabled={busy || needsRemoval || quantity >= available}
                    run={() =>
                      setCart(old => {
                        const next = (old[key]?.quantity ?? 0) + 1;
                        const compatible = row.skuAvailable !== undefined || !commerceHasSiblingOption(row, old);
                        return compatible && next <= commerceLineCapacity(row, old)
                          ? { ...old, [key]: { row, quantity: next } }
                          : old;
                      })
                    }
                  />}
                  {!menuReadOnly && !!quantity && (
                    <Button
                      label={`${names.name === names.product ? names.product : `${names.product} ${names.option}`} ${
                        menuLocale === 'ko' ? '빼기' : 'Remove'
                      }`}
                      disabled={busy}
                      run={() =>
                        setCart(old => {
                          const next = { ...old };
                          const remaining = (old[key]?.quantity ?? 0) - 1;
                          if (remaining < 1) delete next[key];
                          else next[key] = { row, quantity: remaining };
                          return next;
                        })
                      }
                    />
                  )}
                </View></Fragment>
              );
            })}
          {nearbySoldOut && point?.route === 'nearby' && shop &&
            <Text accessibilityRole="header" style={s.text}>선택한 상품이나 옵션의 재고가 부족합니다</Text>}
          {!point && pickupChoice && cartReview && cartNeedsReview(cartReview) && (
            <>
              <Text accessibilityRole="header" style={s.text}>재고 또는 가격이 변경됐어요</Text>
              <Text style={s.text}>선택한 옵션과 새 합계를 다시 확인하세요.</Text>
              {cartReview.pickupChanges?.map(({previous, current}) => {
                const before = commerceMenuLabels(previous, menuLocale);
                const after = commerceMenuLabels(current, menuLocale);
                return <Text key={commerceLineKey(current)} style={s.text}>
                  {`${after.product} · ${before.option} → ${after.option}`}
                </Text>;
              })}
            </>
          )}
          {!point &&
            cartReview?.shortages.map(({ row, quantity }) => {
              const names = commerceMenuLabels(row, menuLocale);
              return (
                <Text key={commerceLineKey(row)} style={s.text}>
                  {`확보 못한 품목 · ${names.name} ×${quantity}`}
                </Text>
              );
            })}
          {!point && !!cartReview?.priceChanges.length && (
            <>
              <Text style={s.text}>
                가격이 바뀌었습니다. 변경된 옵션과 합계를 확인하세요.
              </Text>
              {cartReview.priceChanges.map(({ previous, current }) => {
                const names = commerceMenuLabels(current, menuLocale);
                return (
                  <Text key={commerceLineKey(current)} style={s.text}>{`${names.name} · ${cartMoney(
                    previous.unitPrice,
                    previous.asset,
                    policy,
                    'charge',
                  )} → ${cartMoney(
                    current.unitPrice,
                    current.asset,
                    policy,
                    'charge',
                  )}`}</Text>
                );
              })}
              {cartReview.previousTotals.map(({ asset, amount }) => (
                <Text key={asset} style={s.text}>
                  {`기존 합계 · ${cartMoney(amount, asset, policy, 'charge')}`}
                </Text>
              ))}
            </>
          )}
          {!point &&
            commerceCartTotals(cart).map(({ asset, amount }) => (
              <Text key={asset} style={s.text}>
                {`${
                  Boolean(cartReview?.priceChanges.length) ? '새 합계' : '합계'
                } · ${cartMoney(amount, asset, policy, 'charge')}`}
              </Text>
            ))}
          {!point && (() => {
            // Shown only when the store said the tax of every line in the basket.
            const taxed = commerceCartTax(cart, policy);
            const asset = Object.values(cart)[0]?.row.asset;
            // The two rows add up to the total above them: the tax is cut off and the supply value is the rest.
            const [shown] = taxed && policy ? receiptLines([taxed], paymentToken(policy)) : [];
            return shown && asset ? <>
              <Text style={s.text}>{`공급가액 · ${cartMoney(shown.supply.toString(), asset, policy, 'charge')}`}</Text>
              <Text style={s.text}>{`세금(가격에 포함) · ${cartMoney(shown.tax.toString(), asset, policy, 'tax')}`}</Text>
              <Text style={s.text}>공급가액은 품목마다 원 단위에서 반올림하고, 남은 금액이 세금입니다.</Text>
            </> : null;
          })()}
          {!point && storeAccess && Object.keys(cart).length > 0 && (() => {
            // The board is never asked to approve what its account cannot pay: a short balance is said at the basket.
            const total = commerceCartTotals(cart).find(row => row.asset === policy?.asset);
            if (!total || !policy) return null;
            if (!serverShort && (boardBalance === undefined || boardBalance === null)) return <Text style={s.text}>{boardBalance === null
              ? '보드 잔액을 확인하지 못했습니다. 결제 전에 보드에서 다시 확인합니다.' : '보드 잔액을 확인하고 있습니다.'}</Text>;
            // The numbers are the server's when it refused this basket for its balance, and the phone's own reading otherwise.
            const held = (serverShort?.balance ?? boardBalance) as bigint, needed = serverShort?.required ?? BigInt(total.amount);
            return <>
              <Text style={s.text}>{`보드 잔액 · ${cartMoney(shownBalance(held, paymentToken(policy)), policy.asset, policy, 'balance')}`}</Text>
              {cartShort && <>
                <Text style={s.text}>{`모자란 금액 · ${cartMoney(missingUnits(needed, held, paymentToken(policy)), policy.asset, policy, 'shortfall')}`}</Text>
                <Text accessibilityRole="alert" style={s.text}>보드 잔액이 합계보다 적어 주문할 수 없습니다. 지갑에서 충전한 뒤 다시 확인하세요.</Text>
                {openBoard && <Button label="충전하러 가기" variant="secondary" run={openBoard} />}
                <Button label="잔액 다시 확인" variant="secondary" disabled={busy}
                  run={() => work(async () => { setShortHold(undefined); setBoardBalance(await storeAccess.balance() ?? null); }, '보드 잔액을 확인하지 못했습니다.')} />
              </>}
            </>;
          })()}
          {!point && !menuReadOnly && (shop || pickupMode) && Object.keys(cart).length > 0 && (
            <Button
              label={
                pickupChoice
                  ? (cartReview && cartNeedsReview(cartReview)
                    ? '변경된 옵션과 금액 확인 후 주문 접수'
                    : '픽업 시간과 결제 금액 확인 후 주문 접수')
                  : cartReview
                    ? '바뀐 장바구니 확인 후 주문 접수'
                    : '주문 접수 요청'
              }
              disabled={busy || storeStopped(storeState.state) || cartShort}
              run={() =>
                work(
                  reserve,
                  '접수 여부를 확인하지 못했습니다. 같은 주문을 다시 조회하세요.',
                )
              }
            />
          )}
          {nearbySoldOut && point?.route === 'nearby' && shop && (
            <>
              {!nearbySoldOut.length && <Text style={s.text}>품절된 옵션의 원래 기록을 확인할 수 없습니다.</Text>}
              {nearbySoldOut.map(({sku, option_key, quantity}) => {
                const key = commerceLineKey({sku, optionKey: option_key});
                const row = point.selection?.[key]?.row;
                const names = row ? commerceMenuLabels(row, menuLocale) : {product: sku, option: option_key, name: option_key === 'standard' ? sku : `${sku} · ${option_key}`};
                return <Text key={key} style={s.text}>
                  {`확보 못한 품목 · ${names.name} ×${quantity}`}
                </Text>;
              })}
              {Object.values(point.selection ?? {}).filter(({row}) => !nearbySoldOut.some(
                line => line.sku === row.sku && line.option_key === row.optionKey)).map(({row, quantity}) => {
                const names = commerceMenuLabels(row, menuLocale);
                return <Text key={commerceLineKey(row)} style={s.text}>
                  {`선택 유지 · ${names.name} ×${quantity}`}
                </Text>;
              })}
              <Button label="수량·옵션 수정" disabled={busy} run={() =>
                work(async () => {
                  const revision = loadRevision.current;
                  const current = () => mounted.current && revision === loadRevision.current;
                  await showRejectedCart(point, current);
                  if (current()) setNearbySoldOut(undefined);
                }, '장바구니를 다시 열 수 없습니다.')} />
            </>
          )}
          {point?.route === 'nearby' && !nearbySoldOut && !order &&
            (!approved || approved.order.id !== point.order) && (
            <View accessibilityLiveRegion="polite" style={s.line}>
              <Text accessibilityRole="header" style={s.text}>{busy
                ? '결제 견적을 확인하고 있습니다'
                : '같은 견적을 조회하고 있습니다'}</Text>
              <Text style={s.text} selectable>{point.order ? `개인 주문 · ${point.order}` : `원래 요청 · ${point.key}`}</Text>
              {shop && <Text style={s.text}>{shop.identity.branchLabel} · 매장 단말 BLE 중계</Text>}
              {!busy && <Text style={s.text}>발급 여부 미확인</Text>}
            </View>
          )}
          {!!point && !nearbySoldOut && (pickupMode ? point.cart.pickup : shop || (point.route === 'direct' && point.order)) && (
            <Button
              label={point.route === 'nearby' ? (busy ? '이 요청 다시 확인' : '원래 요청 재조회') : '저장한 주문 다시 확인'}
              disabled={busy}
              run={() =>
                work(
                  () => reconcile(point),
                  '연결 대기 중입니다. 같은 주문 번호와 요청을 유지합니다.',
                )
              }
            />
          )}
          {!nearbySoldOut && canStartNew() && <Button label="새로 주문하기" disabled={busy}
            run={() => work(() => leaveOrder(true), leaveFailed)} />}
          {/* An order made from a store page holds its stock, and its account's one unpaid cart, until it is taken back. */}
          {!nearbySoldOut && storeAccess && canStartNew() && <Button label="주문 그만두기" variant="secondary" disabled={busy}
            run={() => work(() => leaveOrder(false), leaveFailed)} />}
          </>}
          {order && (
            <View style={s.line}>
              {nearbyPending && <>
                <Text accessibilityRole="header" style={s.text}>{`${order.id} 지급 결과 확인 중`}</Text>
                <Text style={s.text}>{`지점 ${nearbyPending.label ?? nearbyPending.branch}`}</Text>
                <Text style={s.text}>이 원주문의 결합 견적과 네트워크 결과를 조회합니다. 중복 제출은 차단됩니다.</Text>
              </>}
              {nearbyUnpaid && <>
                <Text accessibilityRole="header" style={s.text}>{`${order.id} 미지급 확인`}</Text>
                <Text style={s.text}>원 견적 미소비와 유효 범위를 확인했습니다.</Text>
                <Text style={s.text}>지급 0건</Text>
                {openPayments && <Button variant="secondary" label="내 결제" disabled={busy} run={() => openPayments()} />}
              </>}
              <Text style={s.text}>
                {invoiceDetail ? invoiceStatus(order) : order.escrow ? openOrderLabel({stage: order.escrow.stage, pickupNo: order.pickupNo, pickup: order.payment?.pickupStage}) : order.payment ? settlementLabel[order.payment.stage] : order.state === 'paid'
                  ? '서버 지급 확인'
                  : order.state === 'closed'
                  ? '주문 종료'
                  : order.state === 'checking'
                  ? '지급 결과 확인 중'
                  : travelPayment ? order.state === 'preview' ? '원주문 연결 · 견적 요청 전' : '견적 발행 · 지급 미확정'
                  : '주문이 연결되었습니다. 보드 승인 대기 중입니다.'}
              </Text>
              <AmountLine amount={orderMoney(order.amount, order.asset, policy, 'charge')} />
              {travelPayment && travelOrderObservedAt !== undefined &&
                <Text style={s.text}>마지막 조회 {new Date(travelOrderObservedAt).toLocaleString()}</Text>}
              {invoiceDetail && order.subscription && (
                <>
                  <Text style={s.text}>회차 {order.subscription.cycle} · 약관 {order.subscription.termsVersion}</Text>
                  <Text style={s.text} selectable>청구서 {order.subscription.invoiceRef}</Text>
                  <Text style={s.text}>납기 {new Date(order.subscription.dueAt).toLocaleString()}</Text>
                  {order.subscription.quoteExpiresAt && (
                    <Text style={s.text}>서명 견적 기한 {new Date(order.subscription.quoteExpiresAt).toLocaleString()}</Text>
                  )}
                  {order.subscription.quoteRef && (
                    <Text style={s.text} selectable>원 견적 {order.subscription.quoteRef}</Text>
                  )}
                </>
              )}
              {client && (pickupMode || point?.cart.pickup) && (
                <TravelPickupDetail key={order.id} branch={client.profile.branchRef} sale={order.id}
                  policy={policy} orderRevision={order.revision} />
              )}
              {!pickupMode && !point?.cart.pickup && order.state === 'paid' &&
                order.receiptRef && order.receiptKind === 'pickup-awaiting-acceptance' && (
                <Text accessibilityLiveRegion="polite" style={s.text}>지급 확정 · 매장 접수 대기</Text>
              )}
              {!pickupMode && !point?.cart.pickup && order.state === 'paid' && order.pickupNo !== undefined && (
                <PickupTicket number={order.pickupNo} />
              )}
              {order.payment?.finalizedAt && (
                <Text style={s.text}>지급 확정 시각 {new Date(order.payment.finalizedAt).toLocaleString()}</Text>
              )}
              {order.payment?.pickupReadyAt && (
                <Text style={s.text}>
                  {order.payment.pickupStage === 'collected' ? '픽업 준비 완료 시각' : '픽업 가능 시각'}{' '}
                  {new Date(order.payment.pickupReadyAt).toLocaleString()}
                </Text>
              )}
              {order.payment && <>
                <Text style={s.text}>네트워크 StableNet 테스트넷 · {order.payment.chainId}</Text>
                {!order.escrow && order.payment.relayTxHash && order.payment.relayTxHash !== order.payment.txHash && (
                  <Text style={s.text} selectable>원 릴레이 거래 {order.payment.relayTxHash}</Text>
                )}
                {!order.escrow && order.payment.txHash && <Text style={s.text} selectable>확정 거래 {order.payment.txHash}</Text>}
                {order.payment.includedAt && <Text style={s.text}>
                  지급 포함 시각 {new Date(order.payment.includedAt).toLocaleString()}
                </Text>}
                {order.payment.pickupStage && (
                  <Text accessibilityLiveRegion="polite" style={s.text}>{fulfillmentLabel[order.payment.pickupStage]}</Text>
                )}
                {order.payment.stage === 'finalized' && !order.payment.pickupStage &&
                  !pickupMode && !point?.cart.pickup && order.pickupNo !== undefined && (
                  <Text style={s.text}>같은 주문을 다시 조회해 매장의 픽업 준비 상태를 확인하세요.</Text>
                )}
                {order.payment.pickupStage === 'ready' && !pickupMode && !point?.cart.pickup && (
                  <Text style={s.text}>매장 픽업대에서 같은 주문번호를 보여주세요.</Text>
                )}
              </>}
              {order.state === 'paid' && order.receiptRef && (
                <Text selectable style={s.text}>
                  영수증 번호 {order.receiptRef}
                </Text>
              )}
              {order.state === 'paid' && order.payment?.pickupReadyAt &&
                order.payment.pickupStage === 'ready' && openPayments && (
                <Button variant="secondary" label="결제 목록" disabled={busy} run={() => openPayments()} />
              )}
              {nearbyPending && <>
                <Button label="같은 주문의 지급 결과 확인" disabled={busy}
                  run={() => work(() => readHistoryOrder(order.id),
                    '네트워크 응답을 기다립니다. 같은 원주문을 조회하세요.')} />
                {openPayments && <Button variant="secondary" label="내 결제" disabled={busy} run={() => openPayments()} />}
              </>}
              {canReplaceReview(order.id) &&
                policy &&
                (invoiceDetail ? invoiceCanApprove(order) : ['preview', 'issued'].includes(order.state)) && (
                  <Button
                    label={stoppedReview && visibleReview ? '새 견적 확인' : invoiceDetail ? '보드 요약 검토' : travelPayment ? '이 원주문의 서명 견적 요청' : '이 주문의 결제 내용 검토'}
                    disabled={busy}
                    run={() =>
                      work(
                        () => preparePayment(order),
                        // No answer from the store's server is a lost connection, and the order stays as it is.
                        error => storeAccess && isMenuNetworkOffline(error) ? storeUnreached
                          : '주문, 자산, 보드 주소 또는 독립 복구 상태를 확인할 수 없습니다.',
                      )
                    }
                  />
                )}
              {/* The order's reference is no word for a customer: an order from a store page is known by its store, its
                  amount and its number. The earlier route keeps it, where staff may ask for it. */}
              {!order.escrow && !storeAccess && <Text style={s.text} selectable>
                {order.id}
              </Text>}
              {invoiceDetail && approved?.order.id !== order.id && (
                <Button label="기존 회차 재조회" disabled={busy}
                  run={() => work(() => readInvoice(order.id), '회차의 현재 상태를 확인할 수 없습니다.')} />
              )}
            </View>
          )}
          {!nearbyPending && !invoiceDetail && (!approved || pendingReadOnly || travelPayment && !visibleReview) && (order || lookupOrder) && (
            <View style={s.line}>
              {!order && <Text style={s.text} selectable>원주문 {lookupOrder}</Text>}
              <Button label="같은 주문의 지급 결과 확인" disabled={busy}
                run={() => work(() => readHistoryOrder(order?.id ?? lookupOrder!),
                  '네트워크 응답을 기다립니다. 같은 원주문을 조회하세요.')} />
            </View>
          )}
          {approved && !approved.confirmer && visibleReview && !pendingReadOnly && (
            <View style={s.line}>
              <Text style={s.text}>
                검토한 주문 {approved.order.id} · 금액{' '}
                {orderMoney(
                  approved.order.amount,
                  approved.order.asset,
                  policy,
                  'charge',
                )}
              </Text>
              {!stoppedReview && verifiedReview && policy && needsMerchantSponsorship(approved) && rentalReviewOpen && (
                <>
                  <Text style={s.text} selectable>인증한 가맹점 견적 · {approved.order.cafeReview?.branchLabel ?? policy.merchantId}</Text>
                  {showSponsorshipDetails &&
                    <Text style={s.text} selectable>{rentalBoardVerified ? '결제 원천 · 대여 지갑 예치금' : verifiedReview.payerRole === 'owner' ?
                      '개인 지급 계정' : '검증한 지급 계정'}{' '}{verifiedReview.payer}</Text>}
                  {rentalEscrowNeeded && !rentalBoardVerified && (
                    <Text accessibilityLiveRegion="polite" style={s.text}>{rentalEscrowVerified
                      ? '승인하려면 이 원견적을 확인한 대여 보드의 증거가 필요합니다.'
                      : '현재 원견적의 대여 예치금을 확인할 수 없습니다.'}</Text>
                  )}
                  {gasReview?.proof ? (
                    showSponsorshipDetails && (
                      <>
                        <Text style={s.text}>가스 비용 · 매장 부담</Text>
                        <Text style={s.text} selectable>등록된 릴레이 {gasReview.proof.account}</Text>
                      </>
                    )
                  ) : (
                    <>
                      <Text style={s.text}>{gasReview?.problem === 'GAS_SPONSOR_EXPIRED'
                        ? '가스 부담 근거가 만료되었습니다.'
                        : '가스 부담 근거를 받지 못했습니다.'}</Text>
                    </>
                  )}
                  {!approved.authorization && (!gasReview?.proof || rentalEscrowNeeded && !rentalEscrowVerified) && (
                    <Button label={rentalEscrowNeeded ? "같은 견적의 후원과 예치금 다시 확인" : "같은 견적의 가스 부담 다시 확인"}
                      disabled={busy || reviewWaiting} run={() => work(async () => keepPayment(await freshSponsorship(approved)),
                        '같은 원견적의 결제 근거를 확인할 수 없습니다.')} />
                  )}
                  {rentalEscrowNeeded && !approved.authorization && !rentalBoardVerified && (
                    link ? <Button label="대여 보드 원견적 검토 확인" disabled={busy}
                      run={() => work(reviewRentalBoard, '같은 원견적의 대여 보드 검증 결과를 확인할 수 없습니다.')} /> :
                      <Text style={s.text}>대여 보드 검토를 위해 매장 단말을 연결하세요.</Text>
                  )}
                  {!approved.authorization && rentalBoardVerified && (
                    <Button label="대여 보드 결정 확인" disabled={busy}
                      run={() => work(confirmRentalDecision, '같은 원견적의 대여 보드 결정을 확인할 수 없습니다.')} />
                  )}
                  {!approved.authorization && (
                    <Text accessibilityLiveRegion="polite" style={s.text}>
                      {rentalBoardVerified || reviewWaiting && boardVerified === approved.request
                        ? '보드가 원견적을 검증했습니다'
                        : '보드의 원견적 검증 대기'}
                    </Text>
                  )}
                </>
              )}
              {verifiedReview && point?.route === 'nearby' && point.order === approved.order.id &&
                Object.values(point.selection ?? {}).map(({row, quantity}) => {
                  const names = commerceMenuLabels(row, menuLocale);
                  return <Text key={commerceLineKey(row)} style={s.text}>
                    {`${names.name} ×${quantity} · ${cartMoney(row.unitPrice, row.asset, policy, 'charge')}`}
                  </Text>;
                })}
              {approved.authorization && (
                <Text style={s.text}>
                  {approved.forwardAccepted ? '매장이 지급 요청을 받았습니다.' :
                    approved.forwardAttempted ? '네트워크 응답을 기다립니다.' : '보드 서명 확인'}
                </Text>
              )}
              {approved.authorization && !approved.forwardAccepted && approved.quote.expiresAt > now() &&
                (!order || order.state === 'issued') && (!approved.forwardAttempted || order?.state === 'issued') && (
                <Button label={approved.forwardAttempted ? '같은 지급 요청 다시 제출' : '지급 요청 제출'}
                  disabled={busy} run={() => work(forwardPayment,
                    '이 주문의 견적과 현재 지급 상태를 다시 확인하세요.')} />
              )}
              {stoppedReview && (
                <Text accessibilityLiveRegion="polite" style={s.text}>
                  승인 대기가 끝났습니다. {reviewStopLabel[stoppedReview]}
                </Text>
              )}
              {!stoppedReview && verifiedReview && policy && (
                <>
                  {!travelPayment && <Button label={reviewDetails ? '견적 확인으로' : '원견적 상세'}
                    run={() => setReviewDetails(!reviewDetails)} />}
                  {(reviewDetails || travelPayment) && <>
                    <Text style={s.text} selectable>원주문 {approved.order.id}</Text>
                    {approved.order.subscription && <>
                      <Text style={s.text} selectable>청구서 {approved.order.subscription.invoiceRef}</Text>
                      <Text style={s.text}>회차 {approved.order.subscription.cycle} · 약관 {approved.order.subscription.termsVersion}</Text>
                      <Text style={s.text} selectable>견적 해시 {approved.quote.quoteDigest}</Text>
                    </>}
                    <Text style={s.text} selectable>체인 주문 키 {approved.quote.orderKey}</Text>
                    <Text style={s.text} selectable>가맹점 식별자 {policy.merchantId}</Text>
                    {travelPayment
                      ? <Button label="주소 전체 보기" run={() => setRecipientQuote(approved.quote.quoteDigest)} />
                      : <Text style={s.text} selectable>수령 주소 {policy.beneficiary}</Text>}
                    <Text style={s.text}>네트워크 StableNet 테스트넷 · {policy.chain}</Text>
                    <Text style={s.text} selectable>수취 자산 {policy.asset}</Text>
                    <Text style={s.text} selectable>결제 원천 {verifiedReview.payer}</Text>
                    <Text style={s.text}>제출 대행 · 매장 릴레이</Text>
                    {travelPayment && <>
                      <Text style={s.text}>네트워크 수수료 · 매장 릴레이 부담</Text>
                      <Text style={s.text}>독립 표시기에서 수취인, 전체 주소, 금액, 네트워크와 수수료 부담을 확인하세요.</Text>
                    </>}
                    <Text style={s.text}>견적 기한 {new Date(approved.quote.expiresAt * 1000).toLocaleString()}</Text>
                  </>}
                </>
              )}
              {approvalDevice?.request===approved.request && (
                <Button label="원승인 기기 상태" run={()=>setShowApprovalMaintenance(true)} />
              )}
              {!approved.authorization && !stoppedReview && rentalReviewOpen && (
                <Button label="승인 종료" run={cancelApproval} />
              )}
              {!approved.authorization && !stoppedReview && !reviewWaiting && canStartBoard && (!invoiceDetail || (order && invoiceCanApprove(order))) && (
                <Button
                  label="검증한 보드 찾기"
                  disabled={busy || now() >= approved.quote.expiresAt}
                  run={() =>
                    work(
                      async () => { await findBoard(); },
                      '기기 등록 또는 견적 만료 상태를 확인하세요.',
                    )
                  }
                />
              )}
              {!stoppedReview && !approved.authorization && !reviewWaiting && canStartBoard && (!invoiceDetail || (order && invoiceCanApprove(order))) && boards.map(candidate => (
                <View key={candidate.handle}>
                  <Button label={`${candidate.name || '기기'}에서 결제 검토`} disabled={busy}
                    run={()=>work(()=>acceptBoard(candidate.handle),
                      '보드 승인 결과를 확인하지 못했습니다. 동일한 요청으로 다시 조회하세요.')} />
                  <Button label={`${candidate.name || '기기'}에서 결제 검토 및 기기 상태 확인`} disabled={busy}
                    run={()=>work(()=>acceptBoard(candidate.handle,true),
                      '보드 승인 결과를 확인하지 못했습니다. 동일한 요청으로 다시 조회하세요.')} />
                </View>
              ))}
              <Button
                label={invoiceDetail ? '같은 회차의 지급 결과 확인' : '같은 주문의 지급 결과 확인'}
                disabled={busy}
                run={() =>
                  work(
                    reconcilePayment,
                    '네트워크 응답을 기다립니다. 같은 원주문을 조회하세요.',
                  )
                }
              />
            </View>
          )}
          {!invoiceDetail && point &&
            order &&
            point.order === order.id &&
            ['paid', 'closed'].includes(order.state) && (
              <Button
                label="완료한 주문 화면 닫기"
                disabled={busy}
                run={() =>
                  work(async () => {
                    await save();
                    setOrder(undefined);
                    setCart({});
                  }, '완료 기록을 정리할 수 없습니다.')
                }
              />
            )}
        </View>
      )}
      {!tabbed && <Button label="지갑으로 돌아가기" run={back} disabled={busy} variant="secondary" />}
    </ScrollView>
  );
}

const s = StyleSheet.create({
  inProgress: { gap: 10 },
  scroll: { gap: 12, paddingBottom: 24 },
  title: { fontFamily: font, fontSize: 28, fontWeight: '800', letterSpacing: -0.8, color: color.ink },
  text: { fontFamily: font, fontSize: 16, lineHeight: 24, color: color.ink },
  cardTitle: { fontFamily: font, fontSize: 18, fontWeight: '800', color: color.ink },
  registrationInput: {
    height: 120,
    textAlignVertical: 'top',
  },
  input: {
    fontFamily: font,
    borderWidth: 1,
    borderColor: '#8fa497',
    borderRadius: radius.m,
    padding: 12,
    minHeight: 50,
    color: color.ink,
    backgroundColor: color.sheet,
  },
  // A detail block reads as one card, as in the design book's order and receipt pages.
  line: {
    backgroundColor: color.sheet,
    borderWidth: 1,
    borderColor: color.line,
    borderRadius: radius.l,
    padding: 16,
    gap: 8,
  },
});
