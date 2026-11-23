import type { CommerceOrder } from '@nu54/protocol';

/** What the public screen may say about the order it shows. Times are Unix seconds. */
export type OrderProgress =
  | 'open' // no quote yet
  | 'approving' // the phone has the quote
  | 'late' // the quote ran out and nothing proves whether it was paid
  | 'submitting' // a signature is on its way to the chain
  | 'unknown' // submitted, and the result did not arrive in time
  | 'paid'
  | 'unpaid' // proven that nothing was taken
  | 'elsewhere' // finished, but this screen cannot name the result
  ;

export type ShownOrder = {
  sale?: CommerceOrder;
  /** The relay handed the phone's signature to the server. */
  submitting?: true;
  /** The screen stays until this time while a phone is working on the order. */
  holdUntil?: number;
};

/** Quotes live at most this long; used only when this terminal does not hold the quote itself. */
export const longestQuote = 120;
export const paidScreenMs = 10_000;
export const idleMs = 60_000;

export function orderProgress({ sale, submitting, holdUntil }: ShownOrder, now: number): OrderProgress {
  if (!sale || sale.state === 'preview') return 'open';
  if (sale.state === 'paid') return sale.pickupNo === undefined ? 'elsewhere' : 'paid';
  if (sale.escrow?.stage === 'failed' || sale.payment?.stage === 'unpaid' ||
      sale.menuCheckout?.finalOutcome === 'expired-unpaid') return 'unpaid';
  if (sale.state === 'closed') return 'elsewhere';
  const late = holdUntil !== undefined && now >= holdUntil;
  if (sale.state === 'checking' || submitting) return late ? 'unknown' : 'submitting';
  return late ? 'late' : 'approving';
}

export function orderFinished(progress: OrderProgress) {
  return progress === 'paid' || progress === 'unpaid' || progress === 'elsewhere';
}

/** A held order ends with its quote, so a phone that walks away cannot keep the terminal. */
export function holdOrder(sale: CommerceOrder, now: number, quoteUntil?: number, previous?: number) {
  return quoteUntil ?? previous ?? Math.min(now + longestQuote, Math.floor(Date.parse(sale.expiresAt) / 1000));
}

/** How long the screen may stay without a touch. */
export function screenMs(progress: OrderProgress, holdUntil: number | undefined, nowMs: number) {
  if (progress === 'paid') return paidScreenMs;
  if (orderFinished(progress) || holdUntil === undefined) return idleMs;
  return Math.max(0, holdUntil * 1000 - nowMs) + idleMs;
}

/** Bounded backoff for reading one order while somebody waits for it. */
export function pollDelayMs(reads: number, failures: number) {
  return failures ? Math.min(2_000 * 2 ** failures, 15_000) : reads < 15 ? 2_000 : 5_000;
}
