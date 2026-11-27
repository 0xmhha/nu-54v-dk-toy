import React, {useEffect, useState} from 'react';
import {AppState, Pressable, StyleSheet, Text, View} from 'react-native';
import type {OrderBuyerAction} from '@nu54/protocol';
import Http from '../specs/NativeCommerceHttp.ts';
import {Card, KitButton} from '../ui/kit.tsx';
import {color, font, radius, type} from '../ui/theme.ts';
import {onReturnChanged, returnRecord, settleReturn} from '../wallet/boardReturn.ts';
import {clockMargin, customerClient, offeredAnswers, openOrderLabel, refreshOpenOrders} from './customerClient.ts';
import {awaitsStore, isOpenOrder, onOrdersChanged, orderRecords, type OpenOrder} from './orderRecords.ts';
import {AmountLine, PickupTicket} from './paymentParts.tsx';
import {madeAtStorePage, openStoreAccess} from './storeAccess.ts';

export type OrderPayer = {address: string; generation: number} | null;
/** Paid by a board account this phone no longer uses. `null` means no account; undefined, not known yet. */
export const isFormerBoard = (order: OpenOrder, account?: OrderPayer) =>
  account !== undefined && (!account || order.payer !== account.address || order.generation !== String(account.generation));

/** One order in progress, as on the home and payments pages. */
export function OpenOrderCard({order, open, former = false}: {order: OpenOrder; open: () => void; former?: boolean}) {
  const state = (former ? '이전 보드 · ' : '') + openOrderLabel(order), store = order.store ?? '매장 주문';
  const ready = order.stage === 'locked' && order.pickup === 'ready';
  return <Pressable accessibilityRole="button" onPress={open} style={({pressed}) => [styles.card, pressed && styles.pressed]}
    accessibilityLabel={`${store} ${order.pickupNo === undefined ? '' : `주문번호 ${order.pickupNo} `}${state}`}>
    <View style={[styles.number, ready && styles.numberReady]}>
      <Text style={[styles.numberLabel, ready && styles.numberReadyText]}>주문번호</Text>
      <Text style={[styles.numberText, ready && styles.numberReadyText]}>{order.pickupNo ?? '–'}</Text>
    </View>
    <View style={styles.words}><Text style={styles.store}>{store}</Text><Text style={styles.state}>{state}</Text></View>
  </Pressable>;
}

/**
 * Orders in progress, for the whole time the app is in front. The status is asked every 20 seconds while the
 * store still has to act, then every 2 minutes. A closed app asks again when it is opened; there is no push.
 */
export function useOpenOrders(): OpenOrder[] {
  const [orders, setOrders] = useState<OpenOrder[]>([]);
  useEffect(() => {
    const storage = Http;
    if (!storage) return;
    let live = true, asking = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const show = async () => {
      const rows = (await orderRecords(storage).list()).filter(isOpenOrder);
      if (live) setOrders(old => JSON.stringify(old) === JSON.stringify(rows) ? old : rows);
      return rows;
    };
    const returning = async () => (await returnRecord(storage).load().catch(() => undefined))?.stage === 'sent';
    const plan = async (rows: OpenOrder[]) => {
      const sent = await returning();
      clearTimeout(timer);
      timer = undefined;
      if (live && (rows.length || sent) && AppState.currentState !== 'background')
        timer = setTimeout(() => { ask().catch(() => {}); }, sent || rows.some(awaitsStore) ? 20_000 : 120_000);
    };
    const ask = async () => {
      if (asking) return;
      asking = true;
      try {
        const raw = await storage.loadProfile('customer');
        if (live && raw && await returning()) await settleReturn(storage, customerClient(raw), () => Math.floor(Date.now() / 1000)).catch(() => {});
        if (live && raw && (await show()).length) await refreshOpenOrders(customerClient(raw));
        // Orders made from a store page are read with the session each got from the board that paid it. One
        // without a session keeps its last state until its page asks the board.
        const entrance = live ? (await show()).filter(madeAtStorePage) : [];
        for (const branch of new Set(entrance.map(row => row.branch))) {
          const {store, merchantId} = entrance.find(row => row.branch === branch)!;
          const access = await openStoreAccess({branch, name: store ?? '매장', merchantId});
          if (live && access) await refreshOpenOrders(access.client);
        }
      } catch { /* The last known state stays on screen. */ } finally {
        asking = false;
        if (live) await plan(await show());
      }
    };
    const replan = () => { show().then(rows => timer === undefined && !asking ? plan(rows) : undefined).catch(() => {}); };
    const changed = onOrdersChanged(replan), moved = onReturnChanged(replan);
    const state = AppState.addEventListener('change', next => { if (next === 'active') ask().catch(() => {}); else { clearTimeout(timer); timer = undefined; } });
    show().then(async rows => rows.length || await returning() ? ask() : undefined).catch(() => {});
    return () => { live = false; clearTimeout(timer); changed(); moved(); state.remove(); };
  }, []);
  return orders;
}

const steps = ['접수', '준비 중', '준비 완료', '전달됨', '완료'];
function stepOf(order: OpenOrder) {
  if (order.stage === 'paid') return 4;
  if (order.stage === 'delivered' || order.stage === 'disputed' || order.pickup === 'collected') return 3;
  return order.pickup === 'ready' ? 2 : order.pickup === 'preparing' ? 1 : 0;
}

const payouts = {receipt: '받았다고 확인해서 매장에 지급되었습니다.', claim: '확인 기간이 끝나 매장에 지급되었습니다.',
  arbiter: '운영자 판정으로 매장에 지급되었습니다.', 'dispute-withdrawn': '이의를 취소해서 매장에 지급되었습니다.'};
const refunds = {store: '매장이 주문을 취소해서', undelivered: '기한 안에 전달되지 않아서', arbiter: '운영자 판정으로', 'dispute-expired': '이의 처리 기한이 지나서'};
const day = (seconds: number) => new Date(seconds * 1000).toLocaleString('ko-KR', {month: 'long', day: 'numeric', hour: '2-digit', minute: '2-digit'});
export function remaining(seconds: number) {
  const minutes = Math.ceil(seconds / 60);
  const [d, h, m] = [Math.floor(minutes / 1440), Math.floor(minutes / 60) % 24, minutes % 60];
  return [d && `${d}일`, h && `${h}시간`, !d && m && `${m}분`].filter(Boolean).join(' ');
}

function story(order: OpenOrder, now: number, pickupQr = false): string {
  switch (order.stage) {
    case 'locked':
      if (order.pickupNo === undefined) return '결제가 확정되었습니다. 주문번호를 기다리고 있습니다.';
      if (order.pickup === 'unfulfillable') return '매장이 이 주문을 준비할 수 없어 환불을 처리하고 있습니다.';
      if (order.pickup === 'collected') return '매장이 전달을 기록했습니다. 확정되면 받았는지 여기에서 묻습니다.';
      if (order.pickup === 'ready') return pickupQr ? '준비되었습니다. 카운터에서 수령 QR을 보여 주세요.' : '준비가 끝났습니다. 매장에서 주문번호를 보여 주고 받으세요.';
      return order.pickup === 'preparing' ? '매장이 준비하고 있습니다.' : '매장이 주문을 받았습니다.';
    case 'delivered':
      if (Math.abs(now - (order.claimableAt ?? 0)) <= clockMargin) return '매장이 전달했다고 알렸습니다. 확인 기간이 거의 끝났거나 막 끝났습니다. 받지 못했다면 지금 알려 주세요. 기간이 지났으면 서버가 받지 않습니다.';
      return now < (order.claimableAt ?? 0)
        ? `매장이 전달했다고 알렸습니다. 확인 기간이 ${remaining(order.claimableAt! - now)} 남았습니다. 받지 못했다면 그 안에 알려 주세요.`
        : '확인 기간이 끝났습니다. 매장이 대금을 청구할 수 있습니다.';
    case 'disputed':
      return `받지 못했다는 이의를 접수했습니다. 운영자가 ${order.resolveBy ? day(order.resolveBy) + '까지 ' : ''}정하고, 기한이 지나면 보드 계정으로 환불됩니다.`;
    case 'paid': return payouts[order.payout ?? 'receipt'];
    case 'refunded': return `${refunds[order.refund ?? 'store']} 결제 금액이 보드 계정으로 돌아왔습니다.`;
    case 'failed': return '결제가 실행되지 않았습니다. 빠져나간 돈이 없습니다.';
    default: return '결제를 확인하고 있습니다.';
  }
}

const answers: Record<OrderBuyerAction, {label: string; warn?: string; confirm?: string}> = {
  receipt: {label: '받았어요', warn: '확인하면 매장에 대금이 지급되고 취소할 수 없습니다. 물건을 받았나요?', confirm: '네, 받았어요'},
  dispute: {label: '못 받았어요'},
  withdraw: {label: '이의 취소하고 지급', warn: '이의를 취소하면 매장에 바로 지급되고 되돌릴 수 없습니다. 물건을 받았나요?', confirm: '이의 취소하고 지급하기'},
};

/** One order from payment to its end, with the answers the buyer may give now. */
export function OrderDetail({order, amount, money, fresh, mine = true, former = false, busy = false, message, now, actions}: {
  order: OpenOrder; amount: string;
  /** Formats base units of this order's token, for the lines of the receipt. */
  money?: (units: string) => string;
  /** The state below comes from the server in this visit, so it can be answered. */
  fresh: boolean;
  /** This phone holds the key the order answers to. */
  mine?: boolean;
  /** The paying board account is gone, so a refund cannot be reached. */
  former?: boolean;
  busy?: boolean; message?: string; now: number;
  actions: {answer: (action: OrderBuyerAction) => void; refresh: () => void; back: () => void;
    /** Opens the pickup QR of a ready order; without it the order is collected by its number as before. */
    pickupQr?: () => void};
}) {
  const [asking, setAsking] = useState<OrderBuyerAction>();
  const [clock, setClock] = useState(now);
  useEffect(() => { setClock(now); }, [now]);
  useEffect(() => { setAsking(undefined); }, [order.stage, order.pendingAction, order.answer?.action]);
  useEffect(() => {
    if (order.stage !== 'delivered') return;
    const timer = setInterval(() => setClock(Math.floor(Date.now() / 1000)), 30_000);
    return () => clearInterval(timer);
  }, [order.stage]);
  const ended = order.stage === 'refunded' || order.stage === 'failed' || order.pickup === 'unfulfillable';
  const qr = !!actions.pickupQr && mine && order.stage === 'locked' && order.pickup === 'ready' && order.pickupNo !== undefined;
  const step = stepOf(order);
  const given = order.pendingAction ?? order.answer?.action;
  const offered = !given && fresh && mine && order.stage ? offeredAnswers({
    stage: order.stage, claimableAt: order.claimableAt, resolveBy: order.resolveBy, escrow: order.escrow, merchantId: order.merchantId,
    orderKey: order.orderKey, payer: order.payer, confirmer: order.confirmer,
  }, clock) : [];
  return <View style={styles.detail} testID="order-detail">
    <Text accessibilityRole="header" style={type.title}>{openOrderLabel(order)}</Text>
    {!!message && <Text accessibilityLiveRegion="polite" style={type.body}>{message}</Text>}
    {order.pickupNo !== undefined ? <PickupTicket number={order.pickupNo} store={order.store} /> : <Text style={type.section}>{order.store ?? '매장 주문'}</Text>}
    <Card>
      {!ended && <View style={styles.steps} accessible accessibilityLabel={`주문 단계: ${steps[step]}`}>{steps.map((label, index) =>
        <View key={label} style={styles.step}>
          <View style={[styles.bar, index <= step && styles.barDone, index === step && styles.barNow]} />
          <Text style={[styles.stepText, index === step && styles.stepNow]}>{label}</Text>
        </View>)}</View>}
      <Text style={type.body} accessibilityLiveRegion="polite">{story(order, clock, qr)}</Text>
      <AmountLine amount={amount} label="결제 금액" />
      <Text style={type.small}>{`결제한 때 ${day(order.sentAt)}`}</Text>
    </Card>
    {qr && <>
      <KitButton label="수령 QR 받기" run={actions.pickupQr!} disabled={busy} />
      <Text style={type.small}>{`결제한 보드로 확인한 뒤 QR이 나옵니다. ${amount}은 주문 계약에 보관 중이고, 직원이 QR을 찍어 전달을 기록하면 받았는지 여기에서 묻습니다.`}</Text>
    </>}
    {order.items && money && order.items.reduce((sum, item) => sum + BigInt(item.total), 0n).toString() === order.amount &&
      <Receipt items={order.items} money={money} taxFreeShop={order.taxFreeShop} />}
    {given && (order.pendingAction
      ? <View style={styles.notice}><Text style={styles.noticeText}>{`'${answers[given].label}' 답을 확인하고 있습니다. 확정되면 상태가 바뀝니다.`}</Text></View>
      : <View style={styles.notice}>
        <Text style={styles.noticeText}>{`'${answers[given].label}' 답을 보냈지만 결과를 아직 받지 못했습니다. 같은 답만 다시 보낼 수 있습니다.`}</Text>
        <KitButton label="같은 답 다시 보내기" run={() => actions.answer(given)} disabled={busy || !fresh} variant="secondary" />
      </View>)}
    {asking && offered.includes(asking) ? <View style={styles.notice}>
      <Text style={styles.noticeText}>{answers[asking].warn}</Text>
      <KitButton label={answers[asking].confirm!} run={() => actions.answer(asking)} disabled={busy} />
      <KitButton label="돌아가기" run={() => setAsking(undefined)} disabled={busy} variant="secondary" />
    </View> : offered.map((action, index) => <KitButton key={action} label={answers[action].label} disabled={busy}
      variant={index === 0 && action === 'receipt' ? 'primary' : 'secondary'}
      run={() => answers[action].warn ? setAsking(action) : actions.answer(action)} />)}
    {former && mine && <Text style={type.small}>{madeAtStorePage(order)
      // An order made from a store page is answered with that account's own proof, so the earlier promise does not hold for it.
      ? '초기화했거나 바꾼 이전 보드의 주문입니다. 답을 보내거나 수령 QR을 받으려면 결제한 보드 계정의 확인이 필요합니다. 환불되면 이전 보드 계정으로 가서 되찾을 수 없습니다.'
      : '초기화했거나 바꾼 이전 보드의 주문입니다. 받았다는 답은 보낼 수 있지만, 환불되면 이전 보드 계정으로 가서 되찾을 수 없습니다.'}</Text>}
    {!mine && isOpenOrder(order) && <Text style={type.small}>이 휴대전화로 결제한 주문이 아니어서 상태만 볼 수 있습니다.</Text>}
    {isOpenOrder(order) && <KitButton label="상태 다시 확인" run={actions.refresh} disabled={busy} variant="secondary" />}
    <KitButton label="결제 목록으로" run={actions.back} disabled={busy} variant="secondary" />
  </View>;
}

/**
 * An order made from a store page and paid with a board account this phone no longer holds. Following it, answering
 * it and its pickup QR each need that account's proof, and a board that was reset holds another account. So nothing
 * is asked of a board or a server here: the page shows what the phone last wrote down and says what cannot be done.
 */
export function FormerOrderDetail({order, amount, back, backLabel}: {order: OpenOrder; amount: string; back: () => void; backLabel: string}) {
  return <View style={styles.detail} testID="former-order">
    <Text accessibilityRole="header" style={type.title}>이전 보드의 주문</Text>
    {order.pickupNo !== undefined ? <PickupTicket number={order.pickupNo} store={order.store} /> : <Text style={type.section}>{order.store ?? '매장 주문'}</Text>}
    <Card>
      <Text style={type.body}>{`마지막으로 확인한 상태 · ${openOrderLabel(order)}`}</Text>
      <AmountLine amount={amount} label="결제 금액" />
      <Text style={type.small}>{`결제한 때 ${day(order.sentAt)}`}</Text>
    </Card>
    <View style={styles.notice} accessibilityRole="alert">
      <Text style={styles.noticeText}>지금 이 휴대전화에 연결된 보드 계정이 아니라, 초기화했거나 바꾼 이전 보드 계정으로 결제한 주문입니다.</Text>
      <Text style={styles.noticeText}>주문 상태 확인, 받았다는 답, 이의, 수령 QR은 결제한 보드 계정의 확인이 필요해서 이 앱에서는 할 수 없습니다. 위 상태는 새로 확인한 것이 아닙니다.</Text>
      <Text style={styles.noticeText}>환불되면 이전 보드 계정으로 가서 되찾을 수 없습니다. 매장이나 운영자에게 문의하세요.</Text>
    </View>
    <KitButton label={backLabel} run={back} variant="secondary" />
  </View>;
}

const styles = StyleSheet.create({
  refund: {gap: 4, padding: 12, borderRadius: 12, backgroundColor: '#f6e3bf'},
  receiptLine: {gap: 2, paddingVertical: 6, borderTopWidth: 1, borderTopColor: '#d5dfd3'},
  receiptRow: {flexDirection: 'row', justifyContent: 'space-between', gap: 12},
  card: {flexDirection: 'row', alignItems: 'center', gap: 14, padding: 14, borderRadius: radius.l, borderWidth: 1,
    borderColor: color.line, backgroundColor: color.sheet},
  pressed: {opacity: 0.82},
  number: {minWidth: 72, paddingHorizontal: 8, paddingVertical: 8, borderRadius: radius.m, backgroundColor: color.amberBg, alignItems: 'center'},
  numberReady: {backgroundColor: color.mint},
  numberReadyText: {color: color.deep},
  numberLabel: {fontFamily: font, fontSize: 12, fontWeight: '700', color: '#5a3b05'},
  numberText: {fontFamily: font, fontSize: 24, fontWeight: '800', color: '#5a3b05'},
  words: {flex: 1, gap: 2},
  store: {fontFamily: font, fontSize: 16, fontWeight: '700', color: color.ink},
  state: {fontFamily: font, fontSize: 14, color: color.mute},
  detail: {gap: 14, paddingBottom: 24},
  steps: {flexDirection: 'row', gap: 4, marginBottom: 4},
  step: {flex: 1, minWidth: 0, gap: 6},
  bar: {height: 6, borderRadius: 3, backgroundColor: color.line},
  barDone: {backgroundColor: color.green}, barNow: {backgroundColor: color.amber},
  stepText: {fontFamily: font, fontSize: 12, color: color.mute}, stepNow: {color: color.ink, fontWeight: '700'},
  notice: {gap: 10, borderRadius: radius.m, padding: 14, backgroundColor: color.amberBg},
  noticeText: {fontFamily: font, fontSize: 15, lineHeight: 22, color: '#5a3b05'},
});

/** What was bought, line by line: the price, and the supply value and tax inside it when the store said them. */
function Receipt({items, money, taxFreeShop}: {items: NonNullable<OpenOrder['items']>; money: (units: string) => string; taxFreeShop?: string}) {
  const refund = taxFreeShop ? items.filter(item => item.refund) : [];
  const taxed = items.every(item => item.tax !== undefined);
  const sum = (pick: (item: (typeof items)[number]) => bigint) => items.reduce((total, item) => total + pick(item), 0n);
  const row = (label: string, value: string, strong = false) => <View style={styles.receiptRow}>
    <Text style={strong ? type.label : type.small}>{label}</Text><Text style={strong ? type.label : type.small}>{value}</Text></View>;
  return <Card>
    <Text accessibilityRole="header" style={type.section}>영수증</Text>
    {items.map((item, index) => <View key={index} style={styles.receiptLine}>
      {row(`${item.name} ×${item.quantity}`, money(item.total), true)}
      {item.tax !== undefined && <>
        {row(`${item.taxKind === 'exempt' ? '면세' : '부가세 10% 포함'}${item.refund && taxFreeShop ? ' · 외국인 환급 대상 물품' : ''}`, '')}
        {row('공급가액', money((BigInt(item.total) - BigInt(item.tax)).toString()))}
        {row('세금', money(item.tax))}
        {row('뺀 금액', money('0'))}
      </>}
    </View>)}
    {taxed && <>
      {row('공급가액 합계', money(sum(item => BigInt(item.total) - BigInt(item.tax!)).toString()))}
      {row('세금 합계', money(sum(item => BigInt(item.tax!)).toString()))}
    </>}
    {row('낸 금액', money(sum(item => BigInt(item.total)).toString()), true)}
    {refund.length > 0 && <View style={styles.refund}>
      <Text style={type.label}>외국인 세금 환급</Text>
      <Text style={type.small}>{`환급 대상 물품 ${money(refund.reduce((sum, item) => sum + BigInt(item.total), 0n).toString())}. 이 화면과 여권을 매장 계산대에 보여 주면 환급 서류를 받을 수 있습니다(한 번 결제에 15,000원 이상). 구입일부터 3개월 안에 출국하며 물품을 가지고 나가야 환급되고, 환급액은 환급 창구의 수수료를 뺀 금액입니다.`}</Text>
      <Text style={type.small}>{`면세판매장 지정 번호 ${taxFreeShop} (매장이 적은 번호)`}</Text>
      <Text style={type.small}>이 앱은 환급 금액을 계산하거나 돌려주지 않습니다.</Text>
    </View>}
  </Card>;
}
