import {PermissionsAndroid, Platform} from 'react-native';
import {CommerceClient, commerceProfile, commerceRequestId, fromBase64, orderBuyerActions, type CommerceOrder, type OrderBuyerAction} from '@nu54/protocol';
import Http from '../specs/NativeCommerceHttp.ts';
import {orderAlertsOff} from '../legal/consent.ts';
import Alerts from '../specs/NativeSubscriptionAlerts.ts';
import {customerDigest} from './customerJournal.ts';
import {isOpenOrder, orderRecords, type OpenOrder, type OrderProgress} from './orderRecords.ts';

export const walletId = async () =>
  commerceRequestId(fromBase64(await Http!.randomBytes(16)));

/** The enrolled customer connection. A call fails once the enrollment it was made for is replaced. */
export function customerClient(raw: string) {
  const digest = customerDigest(raw);
  return new CommerceClient(
    commerceProfile(raw, 'customer'),
    async body => {
      if (await Http!.loadProfile('customer') !== raw)
        throw new Error('COMMERCE_PROFILE_CHANGED');
      const reply = typeof Http!.exchangeScoped === 'function'
        ? await Http!.exchangeScoped('customer', digest, body)
        : await Http!.exchange('customer', body);
      if (await Http!.loadProfile('customer') !== raw)
        throw new Error('COMMERCE_PROFILE_CHANGED');
      return reply;
    },
    walletId,
  );
}

/** What an authenticated status says about a recorded order. Undefined when it is not about that order. */
export function orderProgress(order: OpenOrder, view: CommerceOrder): OrderProgress | undefined {
  const escrow = view.escrow;
  if (view.id !== order.sale || view.asset !== order.asset || view.amount !== order.amount || !escrow ||
      escrow.orderKey !== order.orderKey || escrow.confirmer !== order.confirmer || escrow.payer !== order.payer ||
      escrow.escrow !== order.escrow || escrow.merchantId !== order.merchantId) return undefined;
  const {stage, claimableAt, resolveBy, payout, refund, pendingAction} = escrow;
  return {stage, claimableAt, resolveBy, payout, refund, pendingAction,
    pickupNo: view.pickupNo ?? order.pickupNo, pickup: view.payment?.pickupStage ?? order.pickup};
}

/** Writes a status down and announces once that the goods are ready. */
export async function recordOrder(order: OpenOrder, view: CommerceOrder): Promise<OpenOrder> {
  const records = orderRecords(Http!);
  const progress = orderProgress(order, view);
  const next = progress && await records.update(order.sale, progress) || order;
  // The notice names no store, number or amount. A closed app shows it when it is opened again.
  await records.announceReady(order.sale, async () => orderAlertsOff() || await Alerts?.orderReady?.(order.sale).catch(() => false) === true);
  return next;
}

let turn = 0;
/** Reads the recorded orders of this enrollment again; an order that cannot be read keeps its last state. */
export async function refreshOpenOrders(client: CommerceClient): Promise<OpenOrder[]> {
  const mine = (await orderRecords(Http!).list()).filter(order =>
    order.branch === client.profile.branchRef && order.customer === client.profile.subjectRef);
  // Eight per round, starting after the last one asked, so every open order gets its turn.
  const open = mine.filter(isOpenOrder);
  const live = new Set(Array.from({length: Math.min(8, open.length)}, (_, index) => open[(turn + index) % open.length]));
  turn = open.length ? (turn + live.size) % open.length : 0;
  return Promise.all(mine.map(async order => {
    if (!live.has(order)) return order;
    try {
      return await recordOrder(order, await client.status(order.sale));
    } catch {
      return order;
    }
  }));
}

/** The server said no, rather than nothing. COMMERCE_REFUSED is an answer outside the protocol, such as a plain 404. */
function refusedAnswer(error: unknown) {
  const code = (error as {code?: string}).code, reason = error instanceof Error ? error.message : '';
  return code === 'COMMERCE_REFUSED' ||
    ['CHECKOUT_CONFLICT', 'MISMATCHED_EVIDENCE', 'EXPIRED', 'BAD_REQUEST', 'NOT_AUTHORIZED', 'UNSUPPORTED_CAPABILITY'].includes(reason);
}

/** Near the end of the confirmation period only the server decides, so the phone clock hides nothing there. */
export const clockMargin = 300;
export const offeredAnswers = (escrow: Parameters<typeof orderBuyerActions>[0], now: number) => orderBuyerActions(escrow,
  escrow.claimableAt !== undefined && Math.abs(now - escrow.claimableAt) <= clockMargin ? escrow.claimableAt - 1 : now);

/** Sends the buyer's answer. It is written down before it leaves, and a retry sends the identical answer. */
export async function sendOrderAnswer(client: CommerceClient, order: OpenOrder, view: CommerceOrder, action: OrderBuyerAction, now: number) {
  const escrow = view.escrow;
  if (!escrow || !orderProgress(order, view)) throw new Error('ORDER_ANSWER_SCOPE');
  const again = order.answer?.action === action && order.answer.claimableAt === escrow.claimableAt;
  if (!again && !offeredAnswers(escrow, now).includes(action)) throw new Error('ORDER_ANSWER_CLOSED');
  const records = orderRecords(Http!);
  const answer = await records.signAnswer(order.sale, action, escrow.claimableAt!, walletId);
  await records.markAnswerSent(order.sale, now);
  try {
    if (await client.answer(order.sale, action, answer.claimableAt, answer.signature, answer.key) !== order.sale)
      throw new Error('ORDER_ANSWER_UNKNOWN');
  } catch (error) {
    if (refusedAnswer(error)) {
      // The same codes can come from a reply that arrived late, so the server is asked what it holds.
      const held = await client.status(order.sale);
      const next = await recordOrder(order, held);
      if (held.escrow && !held.escrow.pendingAction && next.answer?.key === answer.key) await records.dropAnswer(order.sale);
    }
    throw error;
  }
}

/** Asked once, after the first escrow order, as the subscription reminders do. */
export async function askOrderNotices() {
  if (orderAlertsOff()) return false;
  const allowed = Platform.OS !== 'android' || Number(Platform.Version) < 33 ||
    (await PermissionsAndroid.request(PermissionsAndroid.PERMISSIONS.POST_NOTIFICATIONS)) === PermissionsAndroid.RESULTS.GRANTED;
  return allowed && await Alerts?.permission(true) === true;
}

export function openOrderLabel(order: OrderProgress) {
  if (order.stage === 'locked') {
    if (order.pickupNo === undefined) return '주문번호 기다리는 중';
    return {preparing: '준비 중', ready: '준비 완료', collected: '전달 확인 중', unfulfillable: '매장 환불 처리 중'}[order.pickup ?? 'preparing'];
  }
  if (order.stage === 'delivered') return order.pendingAction ? '답을 확인하는 중' : '전달됨';
  if (order.stage === 'disputed') return order.pendingAction ? '답을 확인하는 중' : '이의 확인 중';
  if (order.stage === 'paid') return '완료';
  if (order.stage === 'refunded') return '환불됨';
  if (order.stage === 'failed') return '결제되지 않음';
  return '결제 확인 중';
}
