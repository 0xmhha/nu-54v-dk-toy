import React, {useCallback, useEffect, useRef, useState} from 'react';
import {Linking, Platform, StyleSheet, Text, TextInput, View} from 'react-native';
import {paymentToken, readEntranceQr} from '@nu54/protocol';
import {Card, KitButton, Row, Section, TabHeader} from '../ui/kit.tsx';
import {color, font, inputPlaceholder, radius, type} from '../ui/theme.ts';
import {EntranceError} from './customerEntrance.ts';
import {orderPaymentPin} from './orderPaymentTrust.ts';
import {hoursRows, recentStores, searchStores, storeNow, type RecentStore, type StoreDirectory, type StorePage, type StoreSummary} from './storeDirectory.ts';

// Finding a store and its store page (traveller-services.md chapter 9 items 6 and 7). The customer picks a store
// by name from the list or opens one with the store's shortcut QR; both lead to the store page, which leads to the
// menu. The customer's location is never read. Nothing here needs a board or a registration at the store.

type Recent = ReturnType<typeof recentStores>;
/**
 * `next`: the list goes on after this store. `more`: the page after it is being read, could not be read, or was not
 * asked for because the finder read as many pages as it reads (`pages`).
 */
type Listing = {kind: 'loading'} | {kind: 'problem'; code: 'busy' | 'unavailable' | 'no-entrance'}
  | {kind: 'stores'; stores: StoreSummary[]; next?: string; pages: number; more?: 'loading' | 'busy' | 'unavailable'};
/**
 * How many pages of the list the finder reads (200 stores each). A search by name reads them all, one after another,
 * so that a store on a later page is found; past this many it says that it searched only a part.
 */
export const listPages = 10;
type Opened = {kind: 'loading'; branch: string; name?: string} | {kind: 'page'; page: StorePage} | {kind: 'missing'} | {kind: 'problem'; branch: string; busy: boolean};
type View_ = {kind: 'finder'} | {kind: 'bad-qr'} | {kind: 'store'; opened: Opened};

/** Opens another app with one address: the phone's dialer, or the map app the customer chose. */
export type Outside = {openURL(url: string): Promise<unknown>};

/** The map link holds the store's address only: no coordinates of the phone and no name of the customer. */
export const mapLink = (address: string, os = Platform.OS) =>
  os === 'ios' ? `https://maps.apple.com/?q=${encodeURIComponent(address)}` : `geo:0,0?q=${encodeURIComponent(address)}`;
/**
 * How the token a board pays in is named on a store page: the label of the build's order contract token, never a
 * word of a server. Undefined in a build without its order contract, where the phone's store registration says it.
 */
export function boardPaymentLabel(): string | undefined {
  const pin = orderPaymentPin();
  return pin && paymentToken(pin).label;
}
export const phoneLink = (phone: string) => `tel:${phone.replace(/[^0-9+]/g, '')}`;

const problemOf = (error: unknown) => error instanceof EntranceError && error.code === 'BUSY';

/** What a row of the list says after the store's kind and address: only what keeps the customer from ordering now. */
const listNote = (store: StoreSummary) => storeNow(store).note;

export function StoreFinder({directory, recent = recentStores(), scan, back, order, payQr, initialBranch, outside = Linking, payment = boardPaymentLabel()}: {
  /** Undefined while this build has no entrance address. */
  directory?: StoreDirectory;
  recent?: Recent;
  /** Reads one QR with the camera. Undefined when the phone has no scanner. */
  scan?: () => Promise<string>;
  back: () => void;
  /** Opens the store's menu to order. Without it the page shows no order button. */
  order?: (store: {branch: string; name: string; merchantId?: string}) => void;
  /** A kiosk's payment QR read by the shortcut scanner goes to the payment screen. */
  payQr?: (raw: string) => void;
  /** A store to open first, from a shortcut QR read outside the app. */
  initialBranch?: string;
  outside?: Outside;
  /** The label of the token a board pays in at any store of this build. */
  payment?: string;
}) {
  const [view, setView] = useState<View_>({kind: 'finder'});
  const [listing, setListing] = useState<Listing>(directory ? {kind: 'loading'} : {kind: 'problem', code: 'no-entrance'});
  const [recents, setRecents] = useState<RecentStore[]>([]);
  const [query, setQuery] = useState('');
  const [note, setNote] = useState('');
  const live = useRef(true), turn = useRef(0);

  const load = useCallback(async () => {
    if (!directory) return;
    setListing({kind: 'loading'});
    try {
      const page = await directory.list();
      if (live.current) setListing({kind: 'stores', stores: page.stores, next: page.next, pages: 1});
    } catch (error) {
      if (live.current) setListing({kind: 'problem', code: problemOf(error) ? 'busy' : 'unavailable'});
    }
  }, [directory]);

  /** Reads the page after the last one. One page at a time; a page that could not be read is asked for by hand. */
  const loadMore = useCallback(async (from: Extract<Listing, {kind: 'stores'}>) => {
    if (!directory || !from.next || from.more === 'loading' || from.pages >= listPages) return;
    setListing({...from, more: 'loading'});
    try {
      const page = await directory.list(from.next);
      if (!live.current) return;
      const known = new Set(from.stores.map(store => store.branch));
      setListing({kind: 'stores', stores: [...from.stores, ...page.stores.filter(store => !known.has(store.branch))], next: page.next, pages: from.pages + 1});
    } catch (error) {
      if (live.current) setListing({...from, more: problemOf(error) ? 'busy' : 'unavailable'});
    }
  }, [directory]);

  // A name is looked for in the whole list, not only in the pages read so far: while something is typed, the pages
  // that follow are read one after another, up to `listPages`. A page that failed stops it until the customer asks.
  const searching = query.trim() !== '';
  useEffect(() => {
    if (searching && listing.kind === 'stores' && listing.next && !listing.more && listing.pages < listPages) void loadMore(listing);
  }, [searching, listing, loadMore]);

  const open = useCallback(async (branch: string, name?: string) => {
    setNote('');
    if (!directory) { setView({kind: 'store', opened: {kind: 'problem', branch, busy: false}}); return; }
    const mine = ++turn.current;
    const current = () => live.current && mine === turn.current;
    setView({kind: 'store', opened: {kind: 'loading', branch, name}});
    try {
      const page = await directory.page(branch);
      if (!current()) return;
      setView({kind: 'store', opened: {kind: 'page', page}});
      await recent.remember({branch: page.branch, name: page.name, kind: page.kind});
      const rows = await recent.list();
      if (live.current) setRecents(rows);
    } catch (error) {
      if (!current()) return;
      const refused = error instanceof EntranceError && error.code === 'REFUSED';
      setView({kind: 'store', opened: refused ? {kind: 'missing'} : {kind: 'problem', branch, busy: problemOf(error)}});
    }
  }, [directory, recent]);

  useEffect(() => {
    live.current = true;
    recent.list().then(rows => { if (live.current) setRecents(rows); }).catch(() => {});
    if (initialBranch) void open(initialBranch); else void load();
    return () => { live.current = false; };
    // The parent remounts this screen for each incoming shortcut.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const toFinder = () => {
    turn.current++;
    setView({kind: 'finder'});
    setNote('');
    if (listing.kind !== 'stores') void load();
  };

  /** The shortcut QR holds only the store's reference. A kiosk's payment QR goes to the payment screen. */
  const scanShortcut = async () => {
    setNote('');
    let raw: string;
    try {
      if (!scan) throw new Error('SHOP_CAMERA');
      raw = await scan();
    } catch {
      if (live.current) setView({kind: 'bad-qr'});
      return;
    }
    if (!live.current || !raw) return;
    if (raw.startsWith('nu54://order') && payQr) { payQr(raw.slice(0, 133)); return; }
    try {
      await open(readEntranceQr(raw));
    } catch {
      if (live.current) setView({kind: 'bad-qr'});
    }
  };

  const leave = (run: () => Promise<unknown>, failed: string) => { run().catch(() => { if (live.current) setNote(failed); }); };

  if (view.kind === 'bad-qr') return <View style={styles.page}>
    <Text accessibilityRole="header" style={type.title}>이 QR로는 매장을 열 수 없습니다</Text>
    <View style={styles.warn} accessibilityRole="alert"><Text style={styles.warnText}>
      <Text style={styles.strong}>매장 바로가기 QR이 아닙니다</Text>{'\n'}결제 QR이거나, 다른 앱의 QR이거나, 카메라를 쓸 수 없는 경우입니다.</Text></View>
    <KitButton label="다시 찍기" run={() => { void scanShortcut(); }} />
    <KitButton label="이름으로 매장 찾기" run={toFinder} variant="secondary" />
    <KitButton label="홈으로" run={back} variant="secondary" />
  </View>;

  if (view.kind === 'store') {
    const opened = view.opened;
    if (opened.kind === 'page') return <StorePageView page={opened.page} note={note} back={toFinder} payment={payment}
      order={order && (() => order({branch: opened.page.branch, name: opened.page.name, ...(opened.page.merchantId ? {merchantId: opened.page.merchantId} : {})}))}
      call={phone => leave(() => outside.openURL(phoneLink(phone)), '전화 앱을 열지 못했습니다.')}
      map={address => leave(() => outside.openURL(mapLink(address)), '지도 앱을 열지 못했습니다. 주소를 길게 눌러 복사하세요.')} />;
    return <View style={styles.page}>
      <Text accessibilityRole="header" style={type.title}>{opened.kind === 'loading' && opened.name ? opened.name : '매장 화면'}</Text>
      {opened.kind === 'loading' ? <Text accessibilityLiveRegion="polite" style={type.body}>매장 화면을 불러오고 있습니다.</Text>
        : opened.kind === 'missing' ? <View style={styles.warn} accessibilityRole="alert"><Text style={styles.warnText}>
            <Text style={styles.strong}>이 매장을 찾지 못했습니다</Text>{'\n'}매장이 문을 닫았거나, 이 앱에서 주문을 받지 않는 매장입니다.</Text></View>
        : <>
          <View style={styles.warn} accessibilityRole="alert"><Text style={styles.warnText}>
            {!directory ? '이 앱에는 매장 정보를 불러올 매장 서버 주소가 아직 없습니다.'
              : opened.busy ? '요청이 많습니다. 잠시 뒤 다시 시도하세요.' : '매장 화면을 불러오지 못했습니다. 연결을 확인하고 다시 시도하세요.'}</Text></View>
          {directory && <KitButton label="다시 불러오기" run={() => { void open(opened.branch); }} />}
        </>}
      <KitButton label="매장 찾기로" run={toFinder} variant="secondary" />
    </View>;
  }

  const stores = listing.kind === 'stores' ? listing.stores : [];
  const found = searchStores(stores, query);
  // More stores follow the ones read: being read now, not read yet, or past what the finder reads.
  const rest = listing.kind === 'stores' && listing.next ? listing : undefined;
  const restAll = rest && rest.pages >= listPages;
  const known = new Map(stores.map(store => [store.branch, store]));
  const shownRecents = searchStores(recents, query);
  return <View style={styles.page}>
    <TabHeader title="매장 찾기" />
    <TextInput accessibilityLabel="매장 이름" placeholder="매장 이름" placeholderTextColor={inputPlaceholder} style={styles.input}
      value={query} onChangeText={setQuery} maxLength={120} autoCorrect={false} returnKeyType="search" />
    {shownRecents.length > 0 && <Section title="최근 매장">
      {shownRecents.map(store => {
        const live_ = known.get(store.branch);
        const detail = [live_?.kind ?? store.kind, live_ && storeNow(live_).line].filter(Boolean).join(' · ');
        return <KitButton key={store.branch} variant="row" label={live_?.name ?? store.name} detail={detail || undefined}
          testID={`recent-${store.branch}`} run={() => { void open(store.branch, store.name); }} />;
      })}
    </Section>}
    {/* The list is kept on this phone only; the customer can empty it. */}
    {recents.length > 0 && <KitButton label="최근 매장 지우기" variant="secondary"
      run={() => { recent.forget().then(() => recent.list()).then(rows => { if (live.current) setRecents(rows); }).catch(() => {}); }} />}
    <Section title="모든 매장">
      {listing.kind === 'loading' && <View style={styles.cell}><Text accessibilityLiveRegion="polite" style={type.body}>매장 목록을 불러오고 있습니다.</Text></View>}
      {listing.kind === 'problem' && <View style={styles.cell}><Text accessibilityRole="alert" style={type.body}>
        {listing.code === 'no-entrance' ? '이 앱에는 매장 목록을 불러올 매장 서버 주소가 아직 없습니다. 매장의 바로가기 QR도 주소가 생긴 뒤에 열립니다.'
          : listing.code === 'busy' ? '요청이 많습니다. 잠시 뒤 다시 시도하세요.' : '매장 목록을 불러오지 못했습니다. 연결을 확인하고 다시 시도하세요.'}</Text></View>}
      {listing.kind === 'stores' && !stores.length && <View style={styles.cell}><Text style={type.body}>아직 목록에 올린 매장이 없습니다.</Text></View>}
      {listing.kind === 'stores' && !!stores.length && !found.length && !(rest && !restAll) &&
        <View style={styles.cell}><Text style={type.body}>{restAll ? '앞쪽 매장에서는 이 이름의 매장을 찾지 못했습니다.' : '이 이름의 매장을 찾지 못했습니다.'}</Text></View>}
      {found.map(store => {
        const now = storeNow(store);
        return <View key={store.branch} style={styles.storeRow}>
          <View style={styles.storeBody}>
            <KitButton variant="row" label={store.name} testID={`store-${store.branch}`}
              detail={[store.kind, store.address, listNote(store)].filter(Boolean).join(' · ') || undefined}
              run={() => { void open(store.branch, store.name); }} />
          </View>
          {now.dayOff && <View style={styles.badge}><Text style={styles.badgeText}>휴무</Text></View>}
        </View>;
      })}
      {/* The list is read a page at a time. What is shown is never called the whole list while more follows. */}
      {rest && <View style={styles.cell}><Text accessibilityLiveRegion="polite" style={type.body}>
        {rest.more === 'loading' ? (searching ? '나머지 매장에서도 찾고 있습니다.' : '매장을 더 불러오고 있습니다.')
          : rest.more === 'busy' ? '요청이 많아 나머지 매장을 불러오지 못했습니다. 잠시 뒤 다시 시도하세요.'
          : rest.more === 'unavailable' ? '나머지 매장을 불러오지 못했습니다. 연결을 확인하고 다시 시도하세요.'
          : restAll ? `매장이 많아 앞쪽 ${stores.length}곳만 불러왔습니다. 찾는 매장이 없으면 매장 바로가기 QR로 여세요.`
          : `매장 ${stores.length}곳을 불러왔습니다. 더 있습니다.`}</Text></View>}
      {rest && !restAll && rest.more !== 'loading' && <KitButton variant="row" label="매장 더 보기" run={() => { void loadMore({...rest, more: undefined}); }} />}
    </Section>
    {listing.kind === 'problem' && listing.code !== 'no-entrance' && <KitButton label="다시 불러오기" run={() => { void load(); }} variant="secondary" />}
    <KitButton label="매장 바로가기 QR 찍기" run={() => { void scanShortcut(); }} variant="secondary" />
    <Text style={type.small}>위치는 쓰지 않습니다. 매장을 누르면 매장 화면이 열리고, 가는 길에 미리 주문할 수 있습니다.</Text>
    <Text style={type.small}>매장 바로가기 QR에는 매장 번호만 들어 있습니다. 내 정보는 매장에 가지 않습니다.</Text>
    <KitButton label="홈으로" run={back} variant="secondary" />
  </View>;
}

/** One store's page: what the store wrote, and the way to its menu. A field the store left empty is not drawn. */
export function StorePageView({page, note, back, order, call, map, reserve, payment}: {
  page: StorePage; note?: string; back: () => void;
  /** The label of the build's token (boardPaymentLabel). */
  payment?: string;
  order?: () => void; call: (phone: string) => void; map: (address: string) => void;
  /** Reservations belong to a later phase; the button needs both a page that takes them and a way to make one. */
  reserve?: () => void;
}) {
  const now = storeNow(page);
  // Why no order can be placed now is the server's reason, said as it is. The menu can still be read when the store
  // only stopped taking orders; a store with no menu on sale now is not said to have one, and no button leads to it.
  const menu = order && page.status.reason !== 'no_menu' && page.status.reason !== 'outside_hours' ? order : undefined;
  const facts = [page.amenities && ['편의', page.amenities], page.parking && ['주차', page.parking],
    ['결제', payment ? `보드 결제 · ${payment}` : '보드 결제']].filter(Boolean) as [string, string][];
  return <View style={styles.page}>
    <Text accessibilityRole="header" style={type.title}>{page.name}</Text>
    <Text style={[type.body, now.shut && styles.shut]} accessibilityLabel={`지금 ${now.line}`}>{[page.kind, now.line].filter(Boolean).join(' · ')}</Text>
    {!!page.intro && <Text style={type.body}>{page.intro}</Text>}
    {menu && <KitButton label={now.shut ? '메뉴만 보기' : '메뉴 보고 주문'} run={menu} />}
    {now.shut && <Text accessibilityRole="alert" style={type.small}>{`${now.why ?? '지금은 주문을 받지 않습니다.'}${menu ? ' 메뉴는 볼 수 있습니다.' : ''}`}</Text>}
    {page.reservations && reserve && <KitButton label="예약" run={reserve} variant="secondary" />}
    {!!page.phone && <KitButton label="전화" detail={page.phone} run={() => call(page.phone!)} variant="secondary" />}
    {(!!page.address || !!page.directions) && <Section title="위치">
      <View style={styles.cell}>
        {!!page.address && <Text selectable accessibilityLabel={`주소 ${page.address}`} style={type.body}>{page.address}</Text>}
        {!!page.directions && <Text style={type.small}>{page.directions}</Text>}
        {!!page.address && <Text style={type.small}>주소를 길게 누르면 복사할 수 있습니다.</Text>}
      </View>
      {!!page.address && <KitButton variant="row" label="지도 앱으로 열기" detail="매장 주소가 지도 앱으로 갑니다. 내 위치는 보내지 않습니다."
        run={() => map(page.address!)} />}
    </Section>}
    {(!!page.hours || !!page.closedNote) && <Section title="영업시간">
      <View style={styles.cell}>
        {page.hours && hoursRows(page.hours).map(row => <Row key={row.days} label={row.days} value={row.time} />)}
        {!!page.closedNote && <Row label="쉬는 날" value={page.closedNote} />}
      </View>
    </Section>}
    <Section title="매장 정보">
      <View style={styles.cell}>{facts.map(([label, value]) => <Row key={label} label={label} value={value} />)}</View>
    </Section>
    <Card>
      <Text style={type.label}>결제·환불 문의</Text>
      {!!page.operatorContact && <Text selectable style={type.body}>운영자 {page.operatorContact}</Text>}
      <Text style={type.small}>음식 준비는 매장에, 결제와 환불은 운영자에게 묻습니다.</Text>
    </Card>
    {!!note && <Text accessibilityRole="alert" style={type.small}>{note}</Text>}
    <KitButton label="매장 찾기로" run={back} variant="secondary" />
  </View>;
}

const styles = StyleSheet.create({
  page: {gap: 14, paddingBottom: 24},
  input: {minHeight: 50, borderRadius: radius.m, borderWidth: 1, borderColor: color.line, backgroundColor: color.sheet,
    paddingHorizontal: 14, fontFamily: font, fontSize: 16, color: color.ink},
  cell: {backgroundColor: color.sheet, padding: 14, gap: 4},
  storeRow: {flexDirection: 'row', alignItems: 'center', backgroundColor: color.sheet},
  storeBody: {flex: 1},
  badge: {marginRight: 12, borderRadius: radius.pill, backgroundColor: color.line, paddingHorizontal: 10, paddingVertical: 4},
  badgeText: {fontFamily: font, fontSize: 13, fontWeight: '700', color: color.ink},
  shut: {color: color.amber},
  warn: {padding: 14, borderRadius: radius.l, backgroundColor: color.amberBg},
  warnText: {fontFamily: font, fontSize: 15, lineHeight: 22, color: '#5a3b05'},
  strong: {fontSize: 17, fontWeight: '700'},
});
