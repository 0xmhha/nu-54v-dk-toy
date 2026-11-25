import React, {useCallback, useEffect, useRef, useState} from 'react';
import {Linking, Pressable, StyleSheet, Text, TextInput, View} from 'react-native';
import Http from '../specs/NativeCommerceHttp.ts';
import {openOrderLabel} from '../merchant/customerClient.ts';
import {onOrdersChanged, orderRecords, type OpenOrder} from '../merchant/orderRecords.ts';
import {Card, Chip, KitButton, Row, Section} from '../ui/kit.tsx';
import {useInnerBack, usePageTop} from '../ui/innerBack.ts';
import {color, font, inputPlaceholder, radius, type} from '../ui/theme.ts';
import type {WalletSend} from '@nu54/protocol';
import {credential} from './BoardLife.tsx';
import {FaucetPage} from './FaucetPage.tsx';
import {boardNetwork, networkRows, shownNetwork, type Exchange, type NetworkAdvice, type NetworkRow} from './networks.ts';
import {testedBoard, type WalletStorage} from './profile.ts';
import {defaultReadState, type ReadProfileState} from './readProfiles.ts';
import {addToken, assetMoney, isEvmRow, readToken, removeToken, reviewToken, tokenFailure, tokensOf, type ListedToken, type Store, type TokenAmount, type TokenReview, type WalletToken} from './tokens.ts';

type Say = (ko: string) => string;
/** What others told this phone: servers, tokens the board signs (the store's), tokens the operator lists. */
export type WalletAdvice = NetworkAdvice & {/** The store of this phone's registration. */ registered?: Store; signed?: ListedToken[]; listed?: Partial<Record<string, ListedToken[]>>};
type Props = {
  t: Say; language?: 'ko' | 'en'; stored: WalletStorage; advice?: WalletAdvice; exchange?: Exchange; active: boolean; busy?: boolean;
  hide: () => void; receive: () => void; send: () => void; giveBack: () => void; networks: () => void; board: () => void; top?: () => void;
  save: (change: (state: ReadProfileState) => ReadProfileState) => Promise<unknown>;
  openOrder: (sale: string) => void;
};

/** Payments of this account, newest first, from the phone's own records. */
export function useActivity(account?: string) {
  const [orders, setOrders] = useState<OpenOrder[]>([]);
  useEffect(() => {
    const storage = Http;
    if (!storage || !account) { setOrders([]); return; }
    let live = true;
    const read = () => orderRecords(storage).list()
      .then(rows => { if (live) setOrders(rows.filter(row => row.payer === account).sort((a, b) => b.sentAt - a.sentAt)); }).catch(() => {});
    read();
    const stop = onOrdersChanged(() => { read(); });
    return () => { live = false; stop(); };
  }, [account]);
  return orders;
}

const sendStates = {queued: '보내는 중', submitted: '보내는 중', final: '확정됨', failed: '보내지 않음'};
/** Transfers the store relayed for this account. Nothing is shown when the store has none or cannot be asked. */
function useSends(active: boolean, account?: string) {
  const [sends, setSends] = useState<WalletSend[]>([]);
  const open = sends.some(item => item.state === 'queued' || item.state === 'submitted');
  useEffect(() => {
    if (!active || !account) return;
    let live = true;
    const ask = () => credential().then(({client}) => client.walletSends())
      .then(value => { if (live) setSends(value.sends.filter(item => item.payer === account)); }).catch(() => {});
    ask();
    const timer = setInterval(ask, open ? 10_000 : 60_000);
    return () => { live = false; clearInterval(timer); };
  }, [active, account, open]);
  return sends;
}

/** Balances of the shown network. `null` is a read that failed. */
function useAmounts(row: NetworkRow, tokens: WalletToken[], account: string | undefined, exchange: Exchange | undefined, active: boolean, round: unknown) {
  const scope = `${row.key} ${row.server} ${account ?? ''}`;
  const [amounts, setAmounts] = useState<{scope: string; values: Record<string, TokenAmount | null>}>({scope, values: {}});
  const latest = useRef({row, tokens});
  latest.current = {row, tokens};
  const read = useCallback(async () => {
    if (!account || !exchange || !isEvmRow(latest.current.row)) return;
    const {row: from, tokens: list} = latest.current;
    const at = `${from.key} ${from.server} ${account}`;
    await Promise.all(list.map(async token => {
      const value = await readToken(from, token, account, exchange).catch(() => null);
      setAmounts(known => {
        // A later failure keeps the last known amount of the same server off the screen: it may be stale.
        const values = known.scope === at ? known.values : {};
        return {scope: at, values: {...values, [token.key]: value}};
      });
    }));
  }, [account, exchange]);
  const keys = tokens.map(token => token.key).join();
  useEffect(() => {
    if (!active) return;
    read().catch(() => {});
    const timer = setInterval(() => { read().catch(() => {}); }, 30_000);
    return () => clearInterval(timer);
  }, [active, read, scope, keys, round]);
  return {amounts: amounts.scope === scope ? amounts.values : {}, read};
}

const label = (token: WalletToken) => token.symbol;
/** The name without the test prefix, as it fits a badge. */
const shortSymbol = (symbol: string) => (symbol.replace(/^(?:Test|t)(?=[A-Z])/, '') || symbol).slice(0, 4);
function shownAmount(t: Say, amount: TokenAmount | null | undefined, hidden: boolean) {
  return hidden ? '••••' : amount ? amount.text : t(amount === undefined ? '확인하는 중' : '확인 안 됨');
}
const note = (t: Say, row: NetworkRow, token: WalletToken) => t(token.sends ? '매장 결제에 씀' : token.contract ? (token.added ? '직접 추가함 · 보기만' : '보기만')
  : row.key === boardNetwork ? '체인 수수료용 코인 · 보기만' : '보기만');

function NetworkPage({t, rows, shown, choose, networks}: {t: Say; rows: NetworkRow[]; shown: NetworkRow; choose: (row: NetworkRow) => void; networks: () => void}) {
  const [search, setSearch] = useState('');
  const words = search.trim().toLowerCase();
  const list = rows.filter(row => !row.hidden && (!words || `${row.label} ${row.chainId ?? ''}`.toLowerCase().includes(words)));
  return <View style={styles.page}>
    <Text accessibilityRole="header" style={type.title}>{t('네트워크 고르기')}</Text>
    <TextInput accessibilityLabel={t('네트워크 찾기')} placeholder={t('이름이나 체인 ID')} placeholderTextColor={inputPlaceholder} value={search} onChangeText={setSearch}
      style={styles.input} autoCapitalize="none" autoCorrect={false} maxLength={40} />
    <View style={styles.list}>{list.map(row => <Pressable key={row.key} accessibilityRole="button" accessibilityState={{selected: row.key === shown.key}}
      accessibilityLabel={`${row.label}, ${t(row.sends ? '받기·보내기' : '보기만')}`} onPress={() => choose(row)} style={({pressed}) => [styles.line, pressed && styles.pressed]}>
      <View style={styles.badge}><Text style={styles.badgeText}>{row.short}</Text></View>
      <View style={styles.words}><Text style={type.label}>{row.label}</Text>
        <Text style={type.small}>{`${row.chainId ? `${t('체인')} ${row.chainId}` : t('보드에 이 네트워크의 계정이 없습니다')}${row.key === shown.key ? ` · ${t('보는 중')}` : ''}`}</Text></View>
      <Chip label={t(row.sends ? '받기·보내기' : '보기만')} tone={row.sends ? 'ok' : 'off'} />
    </Pressable>)}</View>
    <Text style={type.small}>{t('"보기만"은 보드가 아직 이 네트워크에서 보낼 수 없다는 뜻입니다. 받은 돈이 묶이지 않도록 받기 주소를 보여 주지 않습니다.')}</Text>
    <KitButton label={t('네트워크 설정')} run={networks} variant="secondary" />
  </View>;
}

function TokenPage({t, row, token, amount, hidden, busy, receive, send, forget}: {
  t: Say; row: NetworkRow; token: WalletToken; amount?: TokenAmount | null; hidden: boolean; busy?: boolean; receive: () => void; send: () => void; forget?: () => void;
}) {
  return <View style={styles.page}>
    <Text accessibilityRole="header" style={type.title}>{label(token)}</Text>
    <Card tone="green">
      <Text style={styles.onGreen}>{row.label}</Text>
      <Text style={styles.balance}>{shownAmount(t, amount, hidden)} <Text style={styles.unit}>{label(token)}</Text></Text>
    </Card>
    <Card>
      <Row label={t('보드')} value={t(token.sends ? '받기·보내기' : '보기만')} />
      <Row label={t('소수 자리')} value={String(token.decimals)} />
      {token.contract && <><Text style={type.small}>{t('계약 주소')}</Text><Text selectable style={styles.mono}>{token.contract}</Text></>}
    </Card>
    {token.sends ? <View style={styles.actions}>
      <View style={styles.half}><KitButton label={t('받기')} run={receive} variant="secondary" disabled={busy} /></View>
      <View style={styles.half}><KitButton label={t('보내기')} run={send} variant="secondary" disabled={busy} /></View>
    </View> : <Text style={styles.warn}>{t('보드가 아직 이 자산을 보낼 수 없습니다. 이 자산을 보드 계정으로 보내지 마세요. 보드를 초기화하면 잃습니다.')}</Text>}
    {token.contract && <Text style={type.small}>{t('이름이 같은 다른 토큰이 있을 수 있습니다. 앱은 계약 주소로 토큰을 구분합니다.')}</Text>}
    {!!row.explorer && !!token.contract && <KitButton label={t('탐색기에서 보기')} variant="secondary"
      run={() => { Linking.openURL(`${row.explorer!.replace(/\/$/, '')}/address/${token.contract}`).catch(() => {}); }} />}
    {forget && <KitButton label={t('목록에서 지우기')} run={forget} variant="secondary" disabled={busy} />}
  </View>;
}

function AddPage({t, row, account, tokens, exchange, add}: {
  t: Say; row: NetworkRow; account: string; tokens: WalletToken[]; exchange: Exchange; add: (review: TokenReview) => Promise<unknown>;
}) {
  const [address, setAddress] = useState(''), [review, setReview] = useState<TokenReview>();
  const [busy, setBusy] = useState(false), [problem, setProblem] = useState('');
  const run = async (step: () => Promise<unknown>) => {
    if (busy) return;
    setBusy(true);
    setProblem('');
    try { await step(); } catch (error) { setProblem(t(tokenFailure(error))); } finally { setBusy(false); }
  };
  const known = review && tokens.find(token => token.contract === review.contract);
  const twin = review && !known && tokens.some(token => token.symbol.toLowerCase() === review.symbol.toLowerCase());
  return <View style={styles.page}>
    <Text accessibilityRole="header" style={type.title}>{t('토큰 추가')}</Text>
    <Text style={type.small}>{row.label}</Text>
    <Text style={styles.fieldLabel}>{t('토큰 계약 주소')}</Text>
    <TextInput accessibilityLabel={t('토큰 계약 주소')} placeholder="0x" placeholderTextColor={inputPlaceholder} value={address} style={styles.input} maxLength={42}
      onChangeText={text => { setAddress(text); setReview(undefined); setProblem(''); }} autoCapitalize="none" autoCorrect={false} editable={!busy} />
    {!review && <KitButton label={t(busy ? '확인하는 중' : '토큰 정보 확인')} disabled={busy || !address.trim()}
      run={() => { run(async () => setReview(await reviewToken(row, address, account, exchange))); }} />}
    {review && <>
      <Card><Text style={type.label}>{t('체인에서 읽은 정보')}</Text>
        <Row label={t('기호')} value={review.symbol} /><Row label={t('소수 자리')} value={String(review.decimals)} /><Row label={t('내 잔액')} value={review.balance} /></Card>
      {known && <Text style={styles.warn}>{t('이미 목록에 있는 토큰입니다.')}</Text>}
      {twin && <Text style={styles.warn}>{t('기호가 같은 토큰이 이미 있습니다. 주소가 다르면 다른 토큰입니다. 누구나 같은 이름으로 만들 수 있습니다.')}</Text>}
      <Text style={styles.warn}>{t('보드가 보낼 수 있는 토큰이 아니면 잔액만 볼 수 있습니다.')}</Text>
      <KitButton label={t('토큰 추가')} disabled={busy || !!known} run={() => { run(() => add(review)); }} />
    </>}
    {!!problem && <Text accessibilityLiveRegion="polite" style={styles.problem}>{problem}</Text>}
  </View>;
}

/** 지갑 tab of a spending board: one network at a time, its tokens, and what the board can do with them. */
export function WalletTab(props: Props) {
  const {t, stored, advice, exchange, active, busy, save} = props;
  const state = stored.walletRead ?? defaultReadState;
  const rows = networkRows(state, advice), row = shownNetwork(rows, state);
  const board = testedBoard(stored), account = stored.verified?.address;
  const tokens = tokensOf(row, stored.verified, state, advice?.signed, advice?.listed?.[row.key]);
  const orders = useActivity(account);
  const sends = useSends(active && !!board, account);
  const history = [...orders, ...sends].sort((a, b) => ('sale' in b ? b.sentAt : b.createdAt) - ('sale' in a ? a.sentAt : a.createdAt)).slice(0, 10);
  const {amounts, read} = useAmounts(row, tokens, account, exchange, active, `${orders.length} ${sends.map(item => item.state).join()}`);
  const [page, setPage] = useState<'networks' | 'add' | string>();
  const close = () => setPage(undefined);
  useInnerBack(!!page, close);
  usePageTop(page, props.top);
  const hidden = state.selection.hidden || !active;
  const token = tokens.find(item => item.key === page);
  const back = <KitButton label={t('지갑으로 돌아가기')} run={close} variant="secondary" />;
  if (page === 'networks') return <><NetworkPage t={t} rows={rows} shown={row} networks={props.networks}
    choose={next => { save(now => ({...now, shown: next.key})).then(close, () => {}); }} />{back}</>;
  if (page === 'faucet' && account) return <><FaucetPage t={t} account={account} gasCoin={row.gasCoin}
    sends={tokens.filter(item => item.sends && item.contract).map(item => item.contract!)} />{back}</>;
  if (page === 'add' && account && exchange) return <><AddPage t={t} row={row} account={account} tokens={tokens} exchange={exchange}
    add={async review => { await save(now => addToken(now, stored.verified!, row, review)); close(); }} />{back}</>;
  if (token) return <><TokenPage t={t} row={row} token={token} amount={amounts[token.key]} hidden={hidden} busy={busy} receive={props.receive} send={props.send}
    forget={token.added ? () => { save(now => removeToken(now, token.added!)).then(close, () => {}); } : undefined} />{back}</>;
  const failed = tokens.length > 0 && tokens.every(item => amounts[item.key] === null);
  const line = (at: number, state: string, units: string, asset: string) => `${new Date(at * 1000).toLocaleString('ko-KR')} · ${state} · ${hidden ? '••••' : assetMoney(units, asset, advice?.registered, props.language)}`;
  const canAdd = !row.custom && isEvmRow(row) && !!account && !!exchange;
  return <View style={styles.page} testID="wallet-tab">
    <Pressable accessibilityRole="button" accessibilityLabel={`${t('네트워크')} ${row.label}, ${t('바꾸기')}`} onPress={() => setPage('networks')}
      style={({pressed}) => [styles.network, pressed && styles.pressed]}>
      <View style={styles.badge}><Text style={styles.badgeText}>{row.short}</Text></View>
      <View style={styles.words}><Text style={type.label}>{row.label}</Text>
        <Text style={type.small}>{isEvmRow(row) && account ? `${account.slice(0, 6)}…${account.slice(-4)}` : t('보드에 이 네트워크의 계정이 없습니다')}</Text></View>
      <Chip label={t(row.sends ? '받기·보내기' : '보기만')} tone={row.sends ? 'ok' : 'off'} />
      <View style={styles.down} accessibilityElementsHidden importantForAccessibility="no" />
    </Pressable>
    {row.sends && <View style={styles.actions}>
      <View style={styles.half}><KitButton label={t('받기')} run={props.receive} variant="secondary" disabled={busy || !board} /></View>
      <View style={styles.half}><KitButton label={t('보내기')} run={props.send} variant="secondary" disabled={busy || !board} /></View>
      <View style={styles.half}><KitButton label={t('시험 토큰')} run={() => setPage('faucet')} variant="secondary" disabled={busy || !board} /></View>
    </View>}
    {!board && <KitButton label={t('보드 설정 이어서 하기')} run={props.board} variant="secondary" />}
    {isEvmRow(row) ? <>
      <View style={styles.head}><Text accessibilityRole="header" style={type.section}>{t('토큰')}</Text>
        <Pressable accessibilityRole="button" accessibilityLabel={t(state.selection.hidden ? '잔액 표시' : '잔액 숨기기')} onPress={props.hide} disabled={busy} style={styles.small}>
          <Text style={styles.link}>{t(state.selection.hidden ? '잔액 표시' : '잔액 숨기기')}</Text></Pressable></View>
      <View style={styles.list}>{tokens.map(item => <Pressable key={item.key} accessibilityRole="button" onPress={() => setPage(item.key)}
        accessibilityLabel={`${label(item)} ${shownAmount(t, amounts[item.key], hidden)}, ${note(t, row, item)}`} style={({pressed}) => [styles.line, pressed && styles.pressed]}>
        <View style={styles.badge}><Text style={styles.badgeText}>{shortSymbol(item.symbol)}</Text></View>
        <View style={styles.words}><Text style={type.label}>{label(item)}</Text><Text style={type.small}>{note(t, row, item)}</Text></View>
        <Text style={styles.amount}>{shownAmount(t, amounts[item.key], hidden)}</Text>
      </Pressable>)}</View>
      {failed && <View style={styles.warnBox}><Text style={styles.warnText}>{t('서버에서 잔액을 읽지 못했습니다.')}</Text>
        <KitButton label={t('다시 확인')} run={() => { read().catch(() => {}); }} variant="secondary" />
        <KitButton label={t('네트워크 설정')} run={props.networks} variant="secondary" /></View>}
      {canAdd && <KitButton label={t('토큰 추가')} run={() => setPage('add')} variant="secondary" disabled={busy} />}
    </> : <Text style={styles.warn}>{t('보드가 이 네트워크의 계정을 만들지 않습니다. 네트워크 서버만 관리할 수 있습니다.')}</Text>}
    {row.key === boardNetwork && <Section title={t('활동')}>
      {history.length ? history.map(item => 'sale' in item
        ? <KitButton key={item.sale} variant="row" run={() => props.openOrder(item.sale)} label={item.store ?? t('매장 주문')} detail={line(item.sentAt, t(openOrderLabel(item)), item.amount, item.asset)} />
        : <KitButton key={item.nonce} variant="row" run={props.send} label={`${t('보냄')} · ${item.to.slice(0, 8)}…${item.to.slice(-4)}`} detail={line(item.createdAt, t(sendStates[item.state]), item.amount, item.token)} />)
        : <View style={styles.empty}><Text style={type.small}>{t('아직 결제한 기록이 없습니다.')}</Text></View>}
    </Section>}
    {row.sends && <KitButton label={t('반납')} run={props.giveBack} variant="secondary" disabled={busy || !board} />}
  </View>;
}

const styles = StyleSheet.create({
  page: {gap: 14}, words: {flex: 1, gap: 2},
  network: {flexDirection: 'row', alignItems: 'center', gap: 12, padding: 14, borderRadius: radius.l, borderWidth: 1, borderColor: color.line, backgroundColor: color.sheet},
  pressed: {opacity: 0.6},
  down: {width: 9, height: 9, borderRightWidth: 2, borderBottomWidth: 2, borderColor: color.mute, transform: [{rotate: '45deg'}], marginTop: -4},
  badge: {minWidth: 40, height: 36, borderRadius: 18, paddingHorizontal: 6, backgroundColor: color.mintSoft, alignItems: 'center', justifyContent: 'center'},
  badgeText: {fontFamily: font, fontSize: 11, fontWeight: '800', color: color.deep},
  actions: {flexDirection: 'row', gap: 10}, half: {flex: 1},
  head: {flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between'},
  small: {minHeight: 44, justifyContent: 'center', paddingHorizontal: 8}, link: {fontFamily: font, fontSize: 14, fontWeight: '700', color: color.green},
  list: {borderRadius: radius.l, borderWidth: 1, borderColor: color.line, overflow: 'hidden', gap: 1, backgroundColor: color.line},
  line: {flexDirection: 'row', alignItems: 'center', gap: 12, minHeight: 64, paddingHorizontal: 14, paddingVertical: 10, backgroundColor: color.sheet},
  amount: {fontFamily: font, fontSize: 16, fontWeight: '800', color: color.ink, textAlign: 'right'},
  onGreen: {fontFamily: font, fontSize: 14, color: color.onGreenSoft},
  balance: {fontFamily: font, fontSize: 34, fontWeight: '800', letterSpacing: -1, color: '#ffffff'}, unit: {fontSize: 16},
  mono: {fontFamily: 'monospace', fontSize: 13, lineHeight: 19, color: color.ink},
  fieldLabel: {fontFamily: font, fontSize: 14, fontWeight: '700', color: color.mute},
  input: {fontFamily: font, fontSize: 15, borderWidth: 1, borderColor: '#8fa497', borderRadius: radius.m, padding: 12, minHeight: 50, color: color.ink, backgroundColor: color.sheet},
  warn: {fontFamily: font, fontSize: 14, lineHeight: 21, color: '#5a3b05', backgroundColor: color.amberBg, borderRadius: radius.m, padding: 12},
  warnBox: {gap: 8, backgroundColor: color.amberBg, borderRadius: radius.m, padding: 12}, warnText: {fontFamily: font, fontSize: 14, lineHeight: 21, color: '#5a3b05'},
  problem: {fontFamily: font, fontSize: 15, lineHeight: 22, color: color.danger, backgroundColor: color.dangerBg, borderRadius: radius.m, padding: 12},
  empty: {backgroundColor: color.sheet, padding: 14},
});
