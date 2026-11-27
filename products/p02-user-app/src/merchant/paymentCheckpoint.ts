import {
  preparePresentedPayment,
  type PresentationScope,
  orderLockFor,
  paymentToken,
  quoteV2Layout,
  verifyCommerceApproval,
  verifyOrderLockFor,
  hexToBytes,
  type CommerceOrder,
  type CommercePaymentProfile,
  type CommerceQuote,
} from '@nu54/protocol';

import {isPinnedPolicy, verifyPinnedQuote} from './orderPaymentTrust.ts';

/**
 * The quote under review, checked against what this phone trusts. A phone without a store registration trusts the
 * order contract of its build and that contract's merchant record (orderPaymentTrust.ts); a registered phone trusts
 * its registration's payment profile and the presentation signed for it.
 */
export function reviewedPayment(quote: CommerceQuote, order: CommerceOrder, profile: CommercePaymentProfile, now: number, scope: PresentationScope) {
  return isPinnedPolicy(profile) ? verifyPinnedQuote(quote, order, profile, now, scope) : preparePresentedPayment(quote, order, profile, now, scope);
}

const stops = ['cancelled', 'expired', 'interrupted', 'changed', 'refused'] as const;
export type PaymentReviewStop = typeof stops[number];

export type PaymentCheckpoint = {
  version: 2;
  branch: string;
  customer: string;
  order: CommerceOrder;
  quote: CommerceQuote;
  scope: PresentationScope;
  request: string;
  submitKey: string;
  route: 'direct' | 'nearby';
  authorization?: string;
  boardStarted?: true;
  reviewStop?: PaymentReviewStop;
  forwardAttempted?: true;
  forwardAccepted?: true;
  relayKey?: string;
  quoteKey?: string;
  authoritative?: true;
  /** Set only for an order paid through OrderEscrow: the phone key the board signature commits to. */
  confirmer?: string;
  store?: string;
  /** The board signed this order, so it counts in today's total even when nothing was sent. */
  boardSigned?: true;
};

export function paymentCheckpoint(
  raw: string,
  branch: string,
  customer: string,
): PaymentCheckpoint | undefined {
  if (!raw) return undefined;
  const item = JSON.parse(raw) as PaymentCheckpoint;
  const id =
    /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
  if (
    !item ||
    item.version !== 2 ||
    item.branch !== branch ||
    item.customer !== customer ||
    !id.test(item.order?.id) ||
    item.scope?.sale !== item.order.id ||
    item.scope?.branch !== branch ||
    item.scope?.customer !== customer ||
    !id.test(item.submitKey) ||
    !/^[0-9a-f]{32}$/.test(item.request) ||
    !Number.isSafeInteger(item.quote?.expiresAt) ||
    !['direct', 'nearby'].includes(item.route) ||
    (item.route === 'nearby' && !id.test(item.relayKey ?? '')) ||
    (item.quoteKey !== undefined && (item.route !== 'direct' || !id.test(item.quoteKey))) ||
    (item.authoritative !== undefined && (item.authoritative !== true || item.route !== 'nearby')) ||
    (item.boardStarted !== undefined && item.boardStarted !== true) ||
    (item.forwardAttempted !== undefined && (item.forwardAttempted !== true || item.authorization === undefined)) ||
    (item.forwardAccepted !== undefined && (item.forwardAccepted !== true || !item.forwardAttempted)) ||
    (item.confirmer !== undefined && (!/^0x[0-9a-f]{40}$/.test(item.confirmer) || /^0x0+$/.test(item.confirmer))) ||
    (item.boardSigned !== undefined && (item.boardSigned !== true || item.confirmer === undefined)) ||
    (item.store !== undefined && (item.confirmer === undefined || typeof item.store !== 'string' || !item.store.length || item.store.length > 240)) ||
    (item.reviewStop !== undefined && (!stops.includes(item.reviewStop) || item.authorization !== undefined)) ||
    (item.authorization !== undefined &&
      !/^[0-9a-f]{128}(1b|1c)$/.test(item.authorization))
  )
    throw new Error('PAYMENT_JOURNAL');
  return item;
}

export function paymentReviewStop(
  item: PaymentCheckpoint,
  now: number,
  waiting = false,
): PaymentReviewStop | undefined {
  // An order signature that never left this phone cannot pay once its quote ends.
  if (item.authorization)
    return item.confirmer && !item.forwardAttempted && item.quote.expiresAt <= now ? 'expired' : undefined;
  if (item.reviewStop) return item.reviewStop;
  if (item.quote.expiresAt <= now) return 'expired';
  // The board keeps the answer to an order, so a lost connection asks the same board again.
  if (item.boardStarted && !waiting && !item.confirmer) return 'interrupted';
}

/** An order payment that has ended without its signature ever leaving the phone. */
export const neverSent = (item: PaymentCheckpoint) => !!item.confirmer && !item.forwardAttempted && !item.forwardAccepted &&
  (item.reviewStop !== undefined && item.reviewStop !== 'interrupted' || item.quote.expiresAt <= Math.floor(Date.now() / 1000));

export function needsMerchantSponsorship(item: PaymentCheckpoint) {
  // An order signature can only be used by the escrow; the store's server pays for sending it.
  return !item.confirmer && (item.route === 'nearby' || item.order.cafeReview !== undefined);
}

export function matchesReviewedOrder(item: PaymentCheckpoint, live: CommerceOrder) {
  const reviewed = item.order;
  return live.state === 'issued' && live.id === reviewed.id &&
    live.revision === reviewed.revision && live.asset === reviewed.asset &&
    live.amount === reviewed.amount && live.quoteDigest === reviewed.quoteDigest &&
    live.orderKey === reviewed.orderKey &&
    (item.route === 'nearby' || ((live.cafeReview !== undefined) === (reviewed.cafeReview !== undefined) &&
      live.cafeReview?.payerBound === reviewed.cafeReview?.payerBound)) &&
    live.subscription?.invoiceRef === reviewed.subscription?.invoiceRef &&
    live.subscription?.cycle === reviewed.subscription?.cycle &&
    live.subscription?.termsVersion === reviewed.subscription?.termsVersion;
}

export function approvalForForward(
  entry: PaymentCheckpoint,
  profile: CommercePaymentProfile,
  now: number,
) {
  const payment = reviewedPayment(
    entry.quote,
    entry.order,
    profile,
    now,
    entry.scope,
  );
  if (!entry.authorization) throw new Error('PAYMENT_APPROVAL_PENDING');
  return {
    payment,
    approval: verifyCommerceApproval(
      payment,
      hexToBytes(entry.authorization),
      now,
    ),
  };
}

/** The escrow counterpart of approvalForForward: what the store's server needs to lock the order. */
export function lockForSubmit(
  entry: PaymentCheckpoint,
  profile: CommercePaymentProfile,
  now: number,
) {
  const payment = reviewedPayment(entry.quote, entry.order, profile, now, entry.scope);
  if (!entry.authorization || !entry.confirmer) throw new Error('PAYMENT_APPROVAL_PENDING');
  const tokenName = paymentToken(profile).name;
  const signature = verifyOrderLockFor(tokenName, payment, entry.confirmer, hexToBytes(entry.authorization), now);
  const lockDigest = orderLockFor(tokenName, payment.bytes.subarray(0, quoteV2Layout.unsignedSize), entry.confirmer).digest;
  return {payment, confirmer: entry.confirmer, lockDigest, signature};
}
