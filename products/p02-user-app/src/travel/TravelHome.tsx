import {sha256} from '@noble/hashes/sha256';
import {useCallback, useEffect, useRef, useState} from 'react';
import {AppState, Keyboard, Pressable, ScrollView, StyleSheet, Text, TextInput, View} from 'react-native';
import {commerceAmount, commerceProfile, commerceRequestId, fromBase64, formatPickupTime, paymentProfile, readPickupCustomer, toBase64, utf8Encode, type CommercePaymentProfile, type PickupCustomerOrder} from '@nu54/protocol';

import Http from '../specs/NativeCommerceHttp.ts';
import {CustomerOrders} from '../merchant/CustomerOrders.tsx';
import {customerDigest} from '../merchant/customerJournal.ts';
import {holdJournal, matchesHold, readHold, type Hold, type HoldCheckpoint, type HoldRejection} from './bookingHold.ts';
import {BookingPaymentTerms, BookingPolicy, type TravelBooking as Booking, type TravelBookingPayment, type TravelBookingScope, type TravelLegKind, type TravelTerms as Terms} from './BookingPaymentTerms.tsx';
import {TravelMap} from './TravelMap.tsx';
import {pickupStateLabel} from './TravelPickupDetail.tsx';
import {TravelPrivate, type PrivateTravelScreen} from './TravelPrivate.tsx';
import {approvedTravelOrigin} from './publicDeployment.ts';
import {actionBranch, defaultPlaceFilters, filterPlaces, geographicPoint, publicOrigin, publicSearchURL, publishedOpening, readPublicCatalog, type PlaceFilters, type Catalog, type MapInfo, type Place, type Region} from './catalog.ts';

export type {TravelBookingScope} from './BookingPaymentTerms.tsx';

type Slot = Terms & {id: string; startsAt: string; remaining: number};
type Leg = {kind: TravelLegKind; saleRef: string; state: string; amount: string; receiptRef?: string; refunded: string;
  lastObservedAt?: string; refundExpected?: string; refundState?: 'not-requested' | 'waiting-payment' | 'not-due' | 'settled' | 'waiting-refund'};
type PickupWindow = {id: string; startsAt: string; endsAt: string; timezone: string};
type CancellationPreview = {canCancel: boolean; cancellationDeadline: string;
  legs: {kind: TravelLegKind; saleRef: string; paymentState: string; refundExpected?: string}[]};
type Reply = {error?: unknown; pickupWindows?: PickupWindow[]; slots?: Slot[]; bookings?: Booking[]; booking?: Booking; saleRef?: string; legs?: Leg[];
  cancellationPreview?: CancellationPreview};

const pickupRef = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

function pickupLabel(window: PickupWindow) {
  if (!pickupRef.test(window.id) || !Number.isFinite(Date.parse(window.startsAt)) ||
      !(Date.parse(window.endsAt) > Date.parse(window.startsAt))) throw new Error('Invalid pickup window');
  return formatPickupTime(window.startsAt, window.timezone);
}

const bookingStates: Record<string, string> = {
  held: '예약 보류', expired: '보류 만료', pending_payment: '예약 지급',
  cancelling: '취소 처리 중', cancelled: '취소 완료',
};

async function publicJSON(url: string, revalidate = false, signal?: AbortSignal): Promise<unknown> {
  const headers = {Accept: 'application/json', ...(revalidate ? {'Cache-Control': 'no-cache'} : {})};
  const response = await fetch(url, {credentials: 'omit', headers, ...(signal ? {signal} : {})});
  if (!response.ok || (response.url && response.url !== url)) throw new Error('Public catalog unavailable');
  return response.json();
}

function catalogDigest(catalog: Catalog) {
  const stable = JSON.stringify(catalog, (_, value) =>
    value && typeof value === 'object' && !Array.isArray(value)
      ? Object.fromEntries(Object.keys(value).sort().map(key => [key, value[key]]))
      : value);
  return toBase64(sha256(utf8Encode(stable)));
}

function openingLabel(place: Place, clock: number) {
  const status = publishedOpening(place, new Date(clock));
  return status === 'open' ? '게시 시간 기준 영업 중' : status === 'outside' ? '게시 영업시간 외' : '영업시간 미확인';
}

function Action({label, onPress, disabled, selected}: {label: string; onPress: () => void; disabled?: boolean; selected?: boolean}) {
  return <Pressable accessibilityRole="button" accessibilityLabel={label} accessibilityState={{selected, disabled}} disabled={disabled} style={[styles.action, selected && styles.selected, disabled && styles.disabled]} onPress={onPress}><Text>{label}</Text></Pressable>;
}

async function customerEnrollment(branch: string) {
  if (!Http) throw new Error('Customer service unavailable');
  const raw = await Http.loadProfile('customer');
  const profile = commerceProfile(raw, 'customer');
  if (profile.branchRef !== branch || profile.expiresAt <= Math.floor(Date.now() / 1000)) {
    throw new Error('Customer enrollment required for this branch');
  }
  return {raw, scope: profile, payment: paymentProfile(raw)};
}

async function branchProfile(branch: string) {
  return (await customerEnrollment(branch)).payment;
}

async function exchange(branch: string, operation: string, values?: Hold | {booking: string}, journal?: string,
  expected?: TravelBookingScope): Promise<Reply> {
  const enrollment = await customerEnrollment(branch);
  if (expected && (enrollment.scope.subjectRef !== expected.customer || enrollment.scope.tenantRef !== expected.tenant))
    throw new Error('Booking enrollment changed');
  if (journal && holdJournal(enrollment.scope) !== journal) throw new Error('Booking enrollment changed');
  const wire = JSON.stringify({travel: {...values, operation, branch}});
  const reply = expected && typeof Http!.exchangeScoped === 'function'
    ? await Http!.exchangeScoped('customer', customerDigest(enrollment.raw), wire)
    : await Http!.exchange('customer', wire);
  if (expected && await Http!.loadProfile('customer') !== enrollment.raw)
    throw new Error('Booking enrollment changed');
  const response = JSON.parse(reply) as Reply;
  if (!response || typeof response !== 'object') throw new Error('Booking response unavailable');
  const rejectedHold = operation === 'hold' && (response.error === 'sold_out' || response.error === 'terms_changed');
  if (response.error !== undefined && !rejectedHold) {
    throw new Error('Booking response unavailable');
  }
  return response;
}

export function TravelHome({back, backLabel, permission, bookingScope}: {
  back: () => void;
  backLabel?: string;
  permission: () => Promise<boolean>;
  bookingScope?: TravelBookingScope;
}) {
  const [endpoint, setEndpoint] = useState('');
  const [privateTravelOpen, setPrivateTravelOpen] = useState<PrivateTravelScreen>();
  const [myTrip, setMyTrip] = useState(false);
  const [myTripBooking, setMyTripBooking] = useState<TravelBookingScope>();
  const [myTripBookingError, setMyTripBookingError] = useState(false);
  const [policy, setPolicy] = useState<CommercePaymentProfile>();
  const [regions, setRegions] = useState<Region[]>([]);
  const [region, setRegion] = useState<Region>();
  const [regionText, setRegionText] = useState('');
  const [placeText, setPlaceText] = useState('');
  const [filters, setFilters] = useState<PlaceFilters>(defaultPlaceFilters);
  const [draft, setDraft] = useState<PlaceFilters>();
  const [page, setPage] = useState(0);
  const [clock, setClock] = useState(Date.now);
  const [mapAttempt, setMapAttempt] = useState(0);
  const [retryRegion,setRetryRegion] = useState<Region>();
  const [mapInfo, setMapInfo] = useState<MapInfo>();
  const [loading, setLoading] = useState(true);
  const [catalog, setCatalog] = useState<Catalog>();
  const [mode, setMode] = useState<'list' | 'map'>('list');
  const [place, setPlace] = useState<Place>();
  const [slots, setSlots] = useState<Slot[]>();
  const [pickupWindows, setPickupWindows] = useState<PickupWindow[]>();
  const pickupHistory = useRef({generation: 0}).current;
  const [pickupOrders,setPickupOrders] = useState<PickupCustomerOrder[]>();
  const [party, setParty] = useState('1');
  const [bookings, setBookings] = useState<Booking[]>();
  const [bookingLoading, setBookingLoading] = useState(false);
  const bookingRevision = useRef({generation: 0}).current;
  const [pending, setPending] = useState<HoldCheckpoint>();
  const [holdRejected, setHoldRejected] = useState<HoldRejection>();
  const loadedHoldScope = useRef<string>(undefined);
  const [cancelling, setCancelling] = useState<{booking: string; scope: TravelBookingScope; preview: CancellationPreview}>();
  const cancellationRevision = useRef({generation: 0}).current;
  const [legs, setLegs] = useState<Record<string, Leg[]>>({});
  const [paymentReview, setPaymentReview] = useState<TravelBookingPayment>();
  const [checkout, setCheckout] = useState<{branch: string; sale?: string; bookingPayment?: TravelBookingPayment;
    pickup?: true | {window: string; startsAt: string; label: string}}>();
  const [message, setMessage] = useState('');
  const [commerceBusy, setBusy] = useState(false);
  const [searching, setSearching] = useState(false);
  const busy = commerceBusy || searching;
  const running = useRef(false);
  const searchRequest = useRef<AbortController>(undefined);
  const mapRequest = useRef<AbortController>(undefined);
  const publication = useRef<{version: number; digests: Map<string, string>}>(undefined);
  const branch = bookingScope?.branch ?? (place && actionBranch(place, 'booking'));
  const pickup = place && actionBranch(place, 'pickup');
  useEffect(() => {
    setCancelling(undefined);
    return () => { cancellationRevision.generation++; };
  }, [branch, bookingScope, cancellationRevision]);
  useEffect(() => {
    if (!cancelling?.preview.canCancel) return;
    const cutoff = Date.parse(cancelling.preview.cancellationDeadline);
    let timer: ReturnType<typeof setTimeout>;
    function awaitCutoff() {
      const remaining = cutoff - Date.now();
      if (remaining <= 0 && cancelling) {
        setCancelling(current => current === cancelling ?
          {...cancelling,preview:{...cancelling.preview,canCancel:false}} : current);
      } else {
        timer = setTimeout(awaitCutoff, Math.min(remaining, 2_147_483_647));
      }
    }
    awaitCutoff();
    return () => clearTimeout(timer);
  }, [cancelling]);
  const matchingRegions = regions.filter(item => [item.name, item.code].some(value => value.toLowerCase().includes(regionText.trim().toLowerCase())));
  const matchingPlaces = filterPlaces(catalog?.places ?? [], placeText, filters, new Date(clock));
  const pageSize = 24;
  const pageCount = Math.max(1, Math.ceil(matchingPlaces.length / pageSize));
  const visiblePage = Math.min(page, pageCount - 1);
  const pagePlaces = matchingPlaces.slice(visiblePage * pageSize, (visiblePage + 1) * pageSize);
  const filtersActive = filters.category !== 'all' || filters.openNow || filters.booking;
  function applyFilters(next: PlaceFilters) {
    setFilters(next);
    setDraft(undefined);
    setPage(0);
  }
  const amount = (raw: string, asset: string) => policy?.asset === asset ? commerceAmount(raw, asset, policy) : '미등록 자산';
  const eligibleForPayment = (booking: Booking) => ['held', 'pending_payment'].includes(booking.state) && policy?.asset === booking.asset &&
    (booking.chainId === undefined || booking.chainId === policy.chain);

  const count = /^(?:[1-9]|1[0-9]|20)$/.test(party) ? Number(party) : 0;

  async function run(action: () => Promise<void>, error: string) {
    if (running.current) return;
    running.current = true;
    setBusy(true);
    setMessage('');
    try {
      await action();
    } catch {
      setMessage(error);
    } finally {
      setBusy(false);
      running.current = false;
    }
  }

  async function workspace() {
    const origin = publicOrigin(approvedTravelOrigin);
    const options = await publicJSON(`${origin}/v1/region-options`) as Region[];
    if (!Array.isArray(options) || !options.every(item => item && typeof item.code === 'string' && typeof item.name === 'string')) {
      throw new Error('Invalid published regions');
    }
    setEndpoint(origin);
    setRegions(options);
  }

  const loadPaymentBookings = useCallback(async () => {
    if (!bookingScope) return;
    const revision = ++bookingRevision.generation;
    setBookingLoading(true);
    setPaymentReview(undefined);
    setCancelling(undefined);
    setBookings(undefined);
    setLegs({});
    setPolicy(undefined);
    setMessage('');
    try {
      const {scope, payment} = await customerEnrollment(bookingScope.branch);
      if (scope.subjectRef !== bookingScope.customer || scope.tenantRef !== bookingScope.tenant)
        throw new Error('Booking enrollment changed');
      if (revision !== bookingRevision.generation) return;
      const result = await exchange(bookingScope.branch, 'my-bookings', undefined, undefined, bookingScope);
      if (revision !== bookingRevision.generation) return;
      setPolicy(payment);
      setBookings(result.bookings ?? []);
    } catch {
      if (revision === bookingRevision.generation) {
        setCheckout(undefined);
        setMessage('이 계정에서 예약을 열 수 없습니다.');
      }
    } finally {
      if (revision === bookingRevision.generation) setBookingLoading(false);
    }
  }, [bookingRevision, bookingScope]);

  useEffect(() => {
    if (bookingScope) return;
    workspace().catch(() => setMessage('승인된 여행 서비스 설정을 확인한 후 다시 시도하세요.'))
      .finally(() => setLoading(false));
  }, [bookingScope]);

  useEffect(() => {
    if (!bookingScope) return;
    loadPaymentBookings();
    const subscription = AppState.addEventListener('change', state => {
      if (state === 'active') {
        loadPaymentBookings();
      } else {
        bookingRevision.generation++;
        setBookings(undefined);
        setLegs({});
        setCancelling(undefined);
        setPaymentReview(undefined);
      }
    });
    return () => {
      bookingRevision.generation++;
      subscription.remove();
    };
  }, [bookingRevision, bookingScope, loadPaymentBookings]);

  useEffect(() => {
    const timer = setInterval(() => setClock(Date.now()), 30_000);
    return () => clearInterval(timer);
  }, []);

  useEffect(() => {
    const subscription = AppState.addEventListener('change', state => {
      if (state !== 'active') {
        pickupHistory.generation++;
        setPickupOrders(undefined);
      }
    });
    return () => {
      pickupHistory.generation++;
      subscription.remove();
    };
  }, [pickupHistory]);

  useEffect(() => () => {
    searchRequest.current?.abort();
    mapRequest.current?.abort();
  }, []);

  async function search(next: Region, revalidate = false) {
    if (running.current && !mapRequest.current) return;
    searchRequest.current?.abort();
    mapRequest.current?.abort();
    const request = new AbortController();
    searchRequest.current = request;
    setSearching(true);
    setMessage('');
    setRetryRegion(next);
    try {
      const payload = await publicJSON(publicSearchURL(endpoint, next.code), revalidate, request.signal);
      if (request.signal.aborted) return;
      const found = readPublicCatalog(payload, next.code);
      const last = publication.current;
      if (last && (found.version === undefined || found.version < last.version)) throw new Error('Older public catalog');
      if (found.version !== undefined) {
        const digest = catalogDigest(found);
        const digests = last?.version === found.version ? last.digests : new Map<string, string>();
        if (digests.has(next.code) && digests.get(next.code) !== digest) throw new Error('Publication changed at the same version');
        digests.set(next.code, digest);
        publication.current = {version: found.version, digests};
      }
      if (region?.code !== next.code) setPlaceText('');
      setRegion(next);
      setPage(0);
      setDraft(undefined);
      setPlace(undefined);
      setSlots(undefined);
      setHoldRejected(undefined);
      loadedHoldScope.current = undefined;
      setPickupWindows(undefined);
      pickupHistory.generation++;
      setPickupOrders(undefined);
      setBookings(undefined);
      setLegs({});
      setCatalog(found);
      setRetryRegion(undefined);
    } catch {
      if (!request.signal.aborted) setMessage(catalog
        ? '공개 정보를 갱신하지 못했습니다. 마지막으로 확인한 결과를 표시합니다.'
        : '지역의 공개 장소를 가져오지 못했습니다. 다시 시도하세요.');
    } finally {
      if (!request.signal.aborted) setSearching(false);
    }
  }

  function select(next: Place) {
    setPlace(next);
    setPolicy(undefined);
    setSlots(undefined);
    setHoldRejected(undefined);
    loadedHoldScope.current = undefined;
    setPickupWindows(undefined);
    pickupHistory.generation++;
    setPickupOrders(undefined);
    setBookings(undefined);
    setLegs({});
    setMessage('');
  }

  async function openMap() {
    const request = new AbortController();
    mapRequest.current = request;
    try {
      const info = await publicJSON(`${endpoint}/v1/map`, false, request.signal) as MapInfo;
      if (request.signal.aborted) return;
      if (!Number.isInteger(info.minZoom) || !Number.isInteger(info.maxZoom) ||
          info.minZoom < 0 || info.maxZoom > 19 || info.minZoom > info.maxZoom ||
          typeof info.attribution !== 'string' || !info.attribution) throw new Error('Invalid public map');
      setMapInfo(info);
      setMapAttempt(current => current + 1);
      setMode('map');
    } catch (error) {
      if (!request.signal.aborted) throw error;
    } finally {
      if (mapRequest.current === request) mapRequest.current = undefined;
    }
  }

  async function loadBookingOptions() {
    if (!branch || !place || !Http) return;
    const enrollment = await customerEnrollment(branch);
    const journal = holdJournal(enrollment.scope);
    const checkpoint = readHold(await Http.loadJournal(journal), branch);
    setPolicy(enrollment.payment);
    if (checkpoint && checkpoint.place !== place.id) {
      setSlots(undefined);
      setMessage('같은 지점의 다른 장소에서 확인 중인 예약이 있습니다. 처음 요청한 장소에서 확인해 주세요.');
      return;
    }
    loadedHoldScope.current = journal;
    setPending(checkpoint);
    setHoldRejected(undefined);
    if (checkpoint) {
      setSlots(undefined);
      setParty(String(checkpoint.intent.partySize));
      setMessage('이 장소에서 보낸 예약 결과를 확인하고 있습니다. 같은 요청을 다시 시도해 주세요.');
      return;
    }
    const reply = await exchange(branch, 'options', undefined, journal);
    if (!Array.isArray(reply.slots)) throw new Error('Missing booking options');
    setSlots(reply.slots);
  }

  async function hold(slot?: Slot) {
    if (!branch || !place || !Http || (!pending && (!slot || slot.asset !== policy?.asset || count < 1 || count > slot.remaining))) return;
    const enrollment = await customerEnrollment(branch);
    const journal = holdJournal(enrollment.scope);
    if (journal !== loadedHoldScope.current) {
      setSlots(undefined);
      setPending(undefined);
      throw new Error('Booking enrollment changed');
    }
    const stored = readHold(await Http.loadJournal(journal), branch);
    if (stored && stored.place !== place.id) throw new Error('Booking place changed');
    if (pending && stored?.intent.requestKey !== pending.intent.requestKey) throw new Error('Booking recovery changed');
    if (!pending && stored) {
      setSlots(undefined);
      setPending(stored);
      setParty(String(stored.intent.partySize));
      setMessage('먼저 보낸 예약 결과를 확인해 주세요.');
      return;
    }
    const intent = stored?.intent ?? {
      branch, slot: slot!.id, requestKey: commerceRequestId(fromBase64(await Http.randomBytes(16))),
      partySize: count, termsVersion: slot!.termsVersion, asset: slot!.asset,
      deposit: (BigInt(slot!.deposit) * BigInt(count)).toString(),
      balance: (BigInt(slot!.balance) * BigInt(count)).toString(),
    };
    const checkpoint = stored ?? {place: place.id, startsAt: slot!.startsAt, intent};
    if (!stored) await Http.saveJournal(journal, JSON.stringify(checkpoint));
    setPending(checkpoint);
    const reply = await exchange(intent.branch, 'hold', intent, journal);
    // P05 checks the original request key before capacity or terms.
    if (reply.error === 'sold_out' || reply.error === 'terms_changed') {
      await Http.saveJournal(journal, '');
      setSlots(undefined);
      setPending(undefined);
      setHoldRejected(reply.error);
      return;
    }
    if (!reply.booking || !matchesHold(reply.booking, intent)) throw new Error('Missing original booking');
    await Http.saveJournal(journal, '');
    setBookings([reply.booking]);
    setSlots(undefined);
    setPending(undefined);
    await refreshLegs(reply.booking);
  }

  async function reviewLeg(booking: Booking, kind: TravelLegKind) {
    if (!branch) return;
    const revision = bookingRevision.generation;
    const {scope} = await customerEnrollment(branch);
    const expected = bookingScope ?? {branch, customer: scope.subjectRef, tenant: scope.tenantRef};
    const reply = await exchange(branch, 'reopen', {booking: booking.id}, undefined, expected);
    if (revision !== bookingRevision.generation) return;
    if (reply.booking?.id !== booking.id) throw new Error('Missing original booking');
    const current = reply.booking;
    const original = await refreshLegs(current, expected);
    if (revision !== bookingRevision.generation) return;
    const leg = original?.find(entry => entry.kind === kind);
    const payment = {booking: current, kind, scope: expected};
    if (leg && leg.state === 'checking') {
      setCheckout({branch, sale: leg.saleRef, bookingPayment: payment});
      return;
    }
    if (!eligibleForPayment(current) || !original || leg && ['paid', 'closed'].includes(leg.state) ||
        kind === 'balance' && !original.some(entry => entry.kind === 'deposit' && entry.state === 'paid' &&
          entry.amount === current.deposit && entry.refunded === '0' && !!entry.receiptRef))
      throw new Error('Booking payment unavailable');
    setPaymentReview(payment);
  }

  async function startLeg(payment: TravelBookingPayment) {
    const {scope, booking, kind} = payment;
    const revision = bookingRevision.generation;
    const reply = await exchange(scope.branch, `start-${kind}`, {booking: booking.id}, undefined, scope);
    if (revision !== bookingRevision.generation) return;
    if (!reply.saleRef || !pickupRef.test(reply.saleRef)) throw new Error('Missing booking order');
    setCheckout({branch: scope.branch, sale: reply.saleRef, bookingPayment: payment});
    setPaymentReview(undefined);
  }

  async function refreshLegs(booking: Booking, expected = bookingScope) {
    if (!branch) return;
    const revision = bookingRevision.generation;
    const reply = await exchange(branch, 'legs', {booking: booking.id}, undefined, expected);
    if (revision !== bookingRevision.generation) return;
    if (!Array.isArray(reply.legs)) throw new Error('Missing original booking payments');
    const checkedAt = new Date().toISOString();
    const entries = reply.legs.map(leg => ({...leg, lastObservedAt: checkedAt}));
    setLegs(current => ({...current, [booking.id]: entries}));
    return entries;
  }

  async function cancel(booking: Booking, scope: TravelBookingScope) {
    if (!branch || branch !== scope.branch) return;
    const revision = bookingRevision.generation;
    await exchange(branch, 'cancel', {booking: booking.id}, undefined, scope);
    if (revision !== bookingRevision.generation) return;
    await refreshLegs(booking, scope);
    const result = await exchange(branch, 'my-bookings', undefined, undefined, scope);
    if (revision === bookingRevision.generation) setBookings(result.bookings ?? []);
  }

  async function previewCancellation(booking: Booking) {
    if (!branch) return;
    const revision = bookingRevision.generation;
    const request = ++cancellationRevision.generation;
    setCancelling(undefined);
    const {scope: enrolled} = await customerEnrollment(branch);
    const scope = bookingScope ?? {branch, customer: enrolled.subjectRef, tenant: enrolled.tenantRef};
    if (revision !== bookingRevision.generation || request !== cancellationRevision.generation) return;
    const reply = await exchange(branch, 'cancel-preview', {booking: booking.id}, undefined, scope);
    if (revision !== bookingRevision.generation || request !== cancellationRevision.generation) return;
    const preview = reply.cancellationPreview;
    if (!preview || typeof preview.canCancel !== 'boolean' || !Number.isFinite(Date.parse(preview.cancellationDeadline)) ||
      (booking.cancellationDeadline !== undefined &&
        Date.parse(booking.cancellationDeadline) !== Date.parse(preview.cancellationDeadline)) ||
      !Array.isArray(preview.legs) || preview.legs.length > 2 || preview.legs.some(leg =>
        !leg || !['deposit', 'balance'].includes(leg.kind) || !pickupRef.test(leg.saleRef) ||
        typeof leg.paymentState !== 'string' || (leg.refundExpected !== undefined &&
          (leg.paymentState !== 'paid' || !/^(0|[1-9][0-9]*)$/.test(leg.refundExpected))))) {
      throw new Error('Invalid cancellation preview');
    }
    setCancelling({booking: booking.id, scope, preview: {
      ...preview,
      canCancel: preview.canCancel && Date.parse(preview.cancellationDeadline) > Date.now(),
    }});
  }

  async function loadPickupHistory(pickupBranch: string) {
    const revision = ++pickupHistory.generation;
    setPickupOrders(undefined);
    try {
      const enrollment = await customerEnrollment(pickupBranch);
      if (revision !== pickupHistory.generation) return;
      const wire = JSON.stringify({travel: {operation: 'pickup-orders', branch: pickupBranch}});
      const raw = typeof Http!.exchangeScoped === 'function'
        ? await Http!.exchangeScoped('customer', customerDigest(enrollment.raw), wire)
        : await Http!.exchange('customer', wire);
      if (revision !== pickupHistory.generation) return;
      const current = await customerEnrollment(pickupBranch);
      if (revision !== pickupHistory.generation) return;
      if (current.raw !== enrollment.raw) throw new Error('Pickup enrollment changed');
      setPickupOrders(readPickupCustomer(raw));
    } catch (error) {
      if (revision === pickupHistory.generation) throw error;
    }
  }

  async function openMyTripBooking() {
    setMyTripBookingError(false);
    try {
      if (!Http) throw new Error('Customer service unavailable');
      const enrolled = commerceProfile(await Http.loadProfile('customer'), 'customer');
      if (enrolled.expiresAt <= Math.floor(Date.now() / 1000)) throw new Error('Customer enrollment expired');
      setMyTripBooking({branch: enrolled.branchRef, customer: enrolled.subjectRef, tenant: enrolled.tenantRef});
    } catch {
      setMyTripBookingError(true);
    }
  }

  if (myTripBooking) return <TravelHome back={() => setMyTripBooking(undefined)} backLabel="내 여행으로 돌아가기"
    bookingScope={myTripBooking} permission={permission} />;
  if (privateTravelOpen) return <TravelPrivate back={() => setPrivateTravelOpen(undefined)}
    backLabel={myTrip ? '내 여행으로 돌아가기' : undefined} initialScreen={privateTravelOpen}
    place={myTrip && privateTravelOpen === 'review' ? undefined : place}
    catalogVersion={catalog?.version} places={catalog?.places ?? []} />;

  if (myTrip) return <ScrollView keyboardShouldPersistTaps="handled">
    <Text accessibilityRole="header" style={styles.title}>나의 여행</Text>
    <Text>선택형 기록과 추천 동의 없이도 장소 탐색은 가능합니다.</Text>
    <Text>예약은 예약 상세에서 확인하세요.</Text>
    <Text>여행 기록은 선택 사항입니다.</Text>
    <Action label="예약" disabled={busy} onPress={() => {run(openMyTripBooking, '예약 상태를 확인할 수 없습니다.');}} />
    {myTripBookingError && <Text accessibilityRole="alert">예약을 열려면 승인된 고객 등록을 확인해 주세요.</Text>}
    <Action label="후기" disabled={busy} onPress={() => setPrivateTravelOpen('review')} />
    <Action label="스탬프" disabled={busy} onPress={() => setPrivateTravelOpen('stamps')} />
    <Action label="기록과 동의" disabled={busy} onPress={() => setPrivateTravelOpen('records')} />
    <Action label="AI 코스" disabled={busy} onPress={() => setPrivateTravelOpen('itinerary')} />
    <Action label="챌린지" disabled={busy} onPress={() => setPrivateTravelOpen('challenge')} />
    <Action label="탐색" disabled={busy} onPress={() => setMyTrip(false)} />
  </ScrollView>;
  if (checkout) return <CustomerOrders back={() => {
    setCheckout(undefined);
    if (checkout.pickup) setPickupWindows(undefined);
    const booking = checkout.bookingPayment?.booking;
    if (booking) {
      run(async () => { await refreshLegs(booking, checkout.bookingPayment?.scope); }, '예약 지급 상태를 확인할 수 없습니다.');
    }
  }} initialLocator="" permission={permission} travelCheckout={checkout} />;

  if (paymentReview) return <ScrollView keyboardShouldPersistTaps="handled">
    <Action label="원예약으로" onPress={() => setPaymentReview(undefined)} disabled={busy} />
    <BookingPaymentTerms payment={paymentReview} policy={policy} />
    {!!message && <Text accessibilityRole="alert">{message}</Text>}
    <Action label="이 조건으로 지급 주문 열기" disabled={busy}
      onPress={() => {run(() => startLeg(paymentReview), '응답을 확인하지 못했습니다. 같은 예약의 지급 주문을 다시 여세요.');}} />
  </ScrollView>;

  const bookingDetails = bookings && (bookings.length ? bookings.map(item => {
    const entries = legs[item.id];
    const paid = entries?.filter(leg => leg.state === 'paid') ?? [];
    const paidAmount = paid.reduce((total, leg) => total + BigInt(leg.amount), 0n);
    const fullAmount = BigInt(item.deposit) + BigInt(item.balance);
    const inProgress = entries?.some(leg => leg.refundState === 'waiting-refund' || leg.refundState === 'waiting-payment');
    const hasRefund = paid.some(leg => leg.refunded !== '0');
    const cancellation = cancelling?.booking === item.id && cancelling.scope.branch === branch ? cancelling : undefined;
    const cancellationAvailable = cancellation?.preview.canCancel && Date.parse(cancellation.preview.cancellationDeadline) > Date.now();
    return <View key={item.id} style={styles.item}>
      <Text selectable>{`예약 참조 ${item.id}`}</Text>
      <Text>{bookingStates[item.state] ?? '예약 상태 확인 중'} · {item.partySize}명</Text>
      <Text>{new Date(item.expiresAt).toLocaleString()}</Text>
      <Text>{`총액 ${amount(fullAmount.toString(), item.asset)}`}</Text>
      <Text>예약금 {amount(item.deposit, item.asset)} · 잔금 {amount(item.balance, item.asset)}</Text>
      {paidAmount === fullAmount && <Text>{item.balance === '0' ? '예약금 지급이 확인됐어요' : '예약금과 잔금이 확인됐어요'}</Text>}
      {inProgress && <Text>{hasRefund ? '부분 환불' : '환불 확인 중'}</Text>}
      <BookingPolicy terms={cancellation && item.cancellationDeadline === undefined ?
        {...item,cancellationDeadline:cancellation.preview.cancellationDeadline} : item} />
      {['held', 'pending_payment'].includes(item.state) && (cancellation ? <View>
        {cancellationAvailable ? <>
          <Text>이 예약을 취소할 때의 예상 환불액입니다.</Text>
          {cancellation.preview.legs.map(leg => <View key={leg.saleRef}>
            <Text selectable>원주문 {leg.saleRef}</Text>
            <Text>{leg.kind === 'deposit' ? '예약금' : '잔금'} {leg.refundExpected === undefined ?
              '지급이 확정되지 않아 예상 환불액을 표시할 수 없습니다.' : `예상 환불 ${amount(leg.refundExpected, item.asset)}`}</Text>
          </View>)}
          <Action label="예약 취소 요청 확인" disabled={busy}
            onPress={() => {run(async () => {
              if (Date.parse(cancellation.preview.cancellationDeadline) <= Date.now()) {
                setCancelling(undefined);
                throw new Error('The cancellation deadline has passed');
              }
              await cancel(item, cancellation.scope);
              setCancelling(undefined);
            }, '취소 가능 시간과 예약의 현재 상태를 확인해 주세요.');}} />
        </> : <Text>이 예약은 현재 취소할 수 없습니다.</Text>}
        <Action label="예약 유지" disabled={busy} onPress={() => setCancelling(undefined)} />
      </View> : <Action label="예약 취소 검토" disabled={busy}
        onPress={() => {run(() => previewCancellation(item), '이 예약의 예상 환불액을 확인할 수 없습니다.');}} />)}
      <Action label="예약 지급 및 영수증 조회" disabled={busy}
        onPress={() => {run(async () => { await refreshLegs(item); }, '예약 지급 상태를 확인할 수 없습니다.');}} />
      {entries?.map(leg => <View key={leg.saleRef} style={styles.item}>
        <Text>{leg.kind === 'deposit' ? '예약금' : '잔금'} {amount(leg.amount, item.asset)}</Text>
        <Text selectable>원주문 {leg.saleRef}</Text>
        <Text>{leg.state === 'paid' ? '지급 확정' : leg.state === 'closed' ? '지급 종료' :
          leg.state === 'preview' ? '견적 요청 전' : leg.state === 'issued' ? '견적 발행 · 지급 미확정' : '지급 확인 중'}</Text>
        {!!leg.lastObservedAt && <Text>마지막 조회 {new Date(leg.lastObservedAt).toLocaleString()}</Text>}
        {!!leg.receiptRef && <Text selectable>원 지급 영수증 {leg.receiptRef}</Text>}
        {leg.refunded !== '0' && <Text>{`확정 환불 ${amount(leg.refunded, item.asset)}`}</Text>}
        {leg.refundState === 'waiting-payment' && <Text>원 지급 확인 후 환불 여부 조회</Text>}
        {leg.refundState === 'not-due' && leg.state === 'paid' && <Text>해당 지급의 조건상 환불 없음</Text>}
        {leg.refundState === 'waiting-refund' && <Text>{leg.kind === 'deposit' ? '예약금 환불 확인 중' : '잔금 환불 확인 중'}</Text>}
        {leg.refundExpected !== undefined && leg.refundExpected !== '0' &&
          <Text>조건에 따른 환불액 {amount(leg.refundExpected, item.asset)}</Text>}
      </View>)}
      {eligibleForPayment(item) && entries && !entries.some(leg => leg.kind === 'deposit' && ['paid', 'closed'].includes(leg.state)) &&
        <Action label={entries.some(leg => leg.kind === 'deposit' && leg.state === 'checking') ? '같은 예약금 주문 조회' : '예약금 결제 검토'} disabled={busy}
          onPress={() => {run(() => reviewLeg(item, 'deposit'), '예약금 지급을 열 수 없습니다.');}} />}
      {eligibleForPayment(item) && item.balance !== '0' && entries?.some(leg => leg.kind === 'deposit' && leg.state === 'paid' && !!leg.receiptRef && leg.refunded === '0') &&
        !entries.some(leg => leg.kind === 'balance' && ['paid', 'closed'].includes(leg.state)) &&
        <Action label={entries.some(leg => leg.kind === 'balance' && leg.state === 'checking') ? '같은 잔금 주문 조회' : '잔금 결제 검토'} disabled={busy}
          onPress={() => {run(() => reviewLeg(item, 'balance'), '잔금 지급을 열 수 없습니다.');}} />}

    </View>;
  }) : <Text>확인할 예약이 없습니다.</Text>);

  if (bookingScope) return <ScrollView keyboardShouldPersistTaps="handled">
    <Action label={backLabel ?? "내 결제로"} onPress={back} />
    <Text style={styles.title}>내 여행 예약</Text>
    {bookingLoading && <Text>예약 상태 확인 중</Text>}
    {!!message && <Text accessibilityRole="alert">{message}</Text>}
    <Action label="예약 다시 조회" onPress={loadPaymentBookings} disabled={bookingLoading || commerceBusy} />
    {bookingDetails}
  </ScrollView>;

  return <ScrollView keyboardShouldPersistTaps="handled">
    <Action label={backLabel ?? "지갑으로 돌아가기"} disabled={commerceBusy} onPress={back} />
    <Text style={styles.title}>여행지 찾기</Text>
    <Action label="내 여행" disabled={busy} onPress={() => {setMyTripBookingError(false); setMyTrip(true);}} />
    <Action label="여행 기록과 동의" onPress={()=>setPrivateTravelOpen('records')} />
    {loading ? <Text>공개 지역을 불러오는 중입니다.</Text> : !endpoint ?
      <Action label="여행 서비스 다시 연결" disabled={busy} onPress={() => {run(workspace, '승인된 여행 서비스 설정을 확인해 주세요.');}} />
      : <>
        <TextInput accessibilityLabel="지역 이름" placeholder="지역 이름 예시 서울" editable={!busy && !pending}
          value={regionText} onChangeText={setRegionText} style={styles.input} />
        {matchingRegions.map(option => <Action key={option.code} label={option.name} disabled={(commerceBusy && !mapRequest.current) || !!pending}
          onPress={() => {search(option);}} />)}
        {!matchingRegions.length && <Text>일치하는 지역이 없습니다.</Text>}
      </>}
    {retryRegion && !busy && <Action label={`${retryRegion.name} 검색 다시 시도`} disabled={!!pending}
      onPress={() => {search(retryRegion, true);}} />}
    {catalog && <>
      {catalog.environment === 'synthetic' && <Text>시험용 장소 데이터</Text>}
      <Text style={styles.heading}>{region?.name}</Text>
      <TextInput accessibilityLabel="장소 이름" placeholder="장소 이름 또는 공개 주소" value={placeText}
        editable={!busy && !pending} onChangeText={value => {setPlaceText(value); setPage(0);}} style={styles.input} />
      <View style={styles.row}>
        <Action label="전체" onPress={() => applyFilters({...filters, category: 'all'})} />
        <Action label="맛집" onPress={() => applyFilters({...filters, category: 'restaurant'})} />
        <Action label="명소" onPress={() => applyFilters({...filters, category: 'attraction'})} />
        <Action label="필터" onPress={() => setDraft({...filters})} />
        {filtersActive && <Action label="필터 해제" onPress={() => applyFilters(defaultPlaceFilters)} />}
      </View>
      {draft && <View style={styles.item}>
        <Text style={styles.heading}>찾고 싶은 장소를 선택하세요</Text>
        <View style={styles.row}>
          {([['all', '유형 전체'], ['restaurant', '맛집 유형'], ['attraction', '명소 유형']] as const).map(([value, label]) =>
            <Action key={value} label={label} selected={draft.category === value}
              onPress={() => setDraft({...draft, category: value})} />)}
        </View>
        <Action label="영업 중" selected={draft.openNow} onPress={() => setDraft({...draft, openNow: !draft.openNow})} />
        <Text>현재 시각을 장소의 게시 영업시간과 비교합니다.</Text>
        <Action label="예약 가능" selected={draft.booking} onPress={() => setDraft({...draft, booking: !draft.booking})} />
        <Text>예약 기능이 있는 장소를 찾습니다. 좌석은 날짜와 인원을 선택한 후 확인합니다.</Text>
        <View style={styles.row}>
          <Action label="필터 적용" onPress={() => {setClock(Date.now()); applyFilters(draft);}} />
          <Action label="취소" onPress={() => setDraft(undefined)} />
        </View>
      </View>}
      {filtersActive && <Text>적용한 필터 · {[
        filters.category === 'restaurant' ? '맛집' : filters.category === 'attraction' ? '명소' : '',
        filters.openNow ? '게시 시간 기준 영업 중' : '', filters.booking ? '예약 기능 있음' : '',
      ].filter(Boolean).join(' · ')}</Text>}
      <View style={styles.row}>
        <Action label="목록" selected={mode === 'list'} onPress={() => {mapRequest.current?.abort(); setMode('list');}} />
        <Action label="지도" selected={mode === 'map'} disabled={busy} onPress={() => {run(openMap, '지도를 불러오지 못했습니다. 목록에서 같은 장소를 볼 수 있습니다.');}} />
        {region && <Action label="공개 장소 정보 다시 조회" disabled={busy || !!pending}
          onPress={() => {search(region, true);}} />}
      </View>
      {mode === 'map' && mapInfo && pagePlaces.length > 0 && <>
        <TravelMap key={`${region?.code}/${mapAttempt}/${pagePlaces.find(geographicPoint)?.id ?? ''}`}
          origin={endpoint} info={mapInfo} places={pagePlaces} select={select} fallback={() => setMode('list')} disabled={busy || !!pending} />
        {pagePlaces.some(item => !geographicPoint(item)) && <Text>좌표가 공개되지 않은 장소는 목록에서 확인할 수 있습니다.</Text>}
      </>}
      {mode === 'list' && pagePlaces.map(result => <View key={result.id} style={styles.item}>
        <Action label={result.name} disabled={busy || !!pending} onPress={() => select(result)} />
        <Text>{result.address}</Text>
        <Text>{result.source} · {result.updatedAt}</Text>
        <Text>{openingLabel(result, clock)}</Text>
        {!!actionBranch(result, 'booking') && <Text>예약 기능 있음 · 좌석 미확인</Text>}
      </View>)}
      {pageCount > 1 && <View style={styles.row}>
        <Text>공개 장소 {matchingPlaces.length}곳 · {visiblePage + 1} / {pageCount}쪽</Text>
        <Action label="이전 결과" disabled={visiblePage === 0} onPress={() => setPage(visiblePage - 1)} />
        <Action label="다음 결과" disabled={visiblePage + 1 === pageCount} onPress={() => setPage(visiblePage + 1)} />
      </View>}
      {!matchingPlaces.length && <Text>조건에 맞는 공개 장소가 없습니다.</Text>}
    </>}
    {place && <View style={styles.item}>
      <Text style={styles.heading}>{place.name}</Text>
      <Text>{place.address}</Text>
      <Text>{place.source} · {place.updatedAt}</Text>
      <Text>{openingLabel(place, clock)}</Text>
      {branch && !slots && <Text>예약 기능 있음 · 좌석 미확인</Text>}
      {place.opening?.map(({weekday, opens, closes, timezone}, index) => <Text key={index}>{['일', '월', '화', '수', '목', '금', '토'][weekday]} {opens}–{closes} ({timezone})</Text>)}
      <Action label="이 장소의 후기와 이용 자격" onPress={()=>setPrivateTravelOpen('review')} />
      {pickup && <>
        <Action label="픽업 선주문" disabled={busy || !!pending} onPress={() => {
          run(async () => {
            const options = (await exchange(pickup, 'pickup-options')).pickupWindows;
            if (!Array.isArray(options)) throw new Error('Pickup windows unavailable');
            options.forEach(pickupLabel);
            setPickupWindows(options);
          }, '선택할 수 있는 픽업 시간을 확인하지 못했습니다.');
        }} />
        {pickupWindows && <>
          <Text>픽업 시간을 선택하세요. 좌석 예약은 만들지 않습니다.</Text>
          {pickupWindows.length === 0 && <Text>지금 선택할 수 있는 픽업 시간이 없습니다.</Text>}
          {pickupWindows.map(window => <Action key={window.id} label={`픽업 ${pickupLabel(window)} 선택`}
            disabled={busy} onPress={() => setCheckout({branch: pickup, pickup: {
              window: window.id, startsAt: window.startsAt, label: pickupLabel(window),
            }})} />)}
        </>}
        <Action label="내 픽업 주문" disabled={busy} onPress={() => {
          run(() => loadPickupHistory(pickup), '등록 지점의 픽업 주문을 확인하지 못했습니다.');
        }} />
        {pickupOrders?.length === 0 && <Text>이 지점의 픽업 주문이 없습니다.</Text>}
        {pickupOrders?.map(order => <Action key={order.saleRef} label={`픽업 ${formatPickupTime(order.startsAt,order.timezone)} · ${pickupStateLabel[order.state]}`}
          disabled={busy} onPress={() => setCheckout({branch: pickup, sale: order.saleRef, pickup: true})} />)}
        <Action label="저장한 픽업 주문 열기" disabled={busy} onPress={() => setCheckout({branch: pickup, pickup: true})} />
      </>}
      {branch && <>
        <Action label="예약 가능한 시간" disabled={busy || !!pending} onPress={() => {run(loadBookingOptions, '등록된 고객 자격과 현재 예약 가능 여부를 확인해 주세요.');}} />
        {holdRejected && <>
          <Text accessibilityRole="alert">{holdRejected === 'sold_out' ? '선택한 시간이 마감됐어요. 다른 시간을 선택해 주세요.' :
            '예약 조건이 바뀌었어요. 최신 가격과 취소 조건을 확인해 주세요.'}</Text>
          <Action label={holdRejected === 'sold_out' ? '다른 시간 선택' : '최신 조건 확인'} disabled={busy}
            onPress={() => {run(loadBookingOptions, '예약 시간과 최신 조건을 불러오지 못했습니다.');}} />
        </>}
        <Action label="내 예약 다시 열기" disabled={busy} onPress={() => {run(async () => {setPolicy(await branchProfile(branch)); setBookings((await exchange(branch, 'my-bookings')).bookings ?? []);}, '예약 상태를 확인할 수 없습니다.');}} />
        {slots && <>
          <View style={styles.row}>
            <TextInput accessibilityLabel="인원" keyboardType="number-pad" editable={!busy && !pending} value={party}
              maxLength={2} onChangeText={setParty} onSubmitEditing={Keyboard.dismiss} style={styles.input} />
            <Action label="인원 입력 완료" disabled={busy || !!pending || count === 0} onPress={Keyboard.dismiss} />
          </View>
          {slots.length === 0 && <Text>예약 가능한 시간이 없습니다.</Text>}
          {slots.map(slot => <View key={slot.id} style={styles.item}>
            <Text>{new Date(slot.startsAt).toLocaleString()} · 잔여 {slot.remaining}석</Text>
            <Text>1인 예약금 {amount(slot.deposit, slot.asset)} · 잔금 {amount(slot.balance, slot.asset)}</Text>
            {count > 0 && <Text>{count}명 합계 {amount(((BigInt(slot.deposit) + BigInt(slot.balance)) * BigInt(count)).toString(), slot.asset)}</Text>}
            <BookingPolicy terms={slot} />
            <Action label="이 조건으로 예약 보류" disabled={busy || !!pending || !count || count > slot.remaining || slot.asset !== policy?.asset}
              onPress={() => {run(() => hold(slot), '보류 결과를 확인하지 못했습니다. 같은 요청을 재시도하거나 내 예약을 확인하세요.');}} />
          </View>)}
        </>}
        {pending && <>
          <Text>{new Date(pending.startsAt).toLocaleString()} · {pending.intent.partySize}명 · 조건 버전 {pending.intent.termsVersion}</Text>
          <Text>예약금 {amount(pending.intent.deposit, pending.intent.asset)} · 잔금 {amount(pending.intent.balance, pending.intent.asset)}</Text>
          <Action label="같은 예약 요청 다시 시도" disabled={busy} onPress={() => {run(() => hold(), '예약 결과를 아직 확인하지 못했습니다. 같은 요청을 다시 시도해 주세요.');}} />
        </>}
        {bookingDetails}
      </>}
    </View>}
    {!!message && <Text accessibilityRole="alert">{message}</Text>}
  </ScrollView>;
}

const styles = StyleSheet.create({
  title: {fontSize: 26, fontWeight: '600', marginVertical: 12},
  heading: {fontSize: 19, fontWeight: '600'},
  input: {borderColor: '#9daab0', borderBottomWidth: 1, padding: 12, marginVertical: 8},
  action: {alignItems: 'center', borderRadius: 5, backgroundColor: '#e5ede9', marginVertical: 5, padding: 12},
  disabled: {opacity: 0.5},
  row: {flexDirection: 'row', flexWrap: 'wrap', gap: 8},
  selected: {backgroundColor: '#bcdbcc'},
  item: {paddingVertical: 12, borderBottomWidth: 1, borderBottomColor: '#d3dbd8'},
});
