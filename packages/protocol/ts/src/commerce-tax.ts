// The tax inside a price, line by line. A price includes its tax; each line's supply value (10/11 of it) is rounded to the
// whole won (or the cent), half up as the tax office recommends, and the tax is the rest, so it is 10% of the supply value.
import type { CommerceSelection } from "./commerce-cart-review.ts";
import { isWon, paymentToken, type CommercePaymentProfile } from "./commerce-payment.ts";
import type { CommercePrice } from "./commerce-v2.ts";

export type CommerceTax = { total: bigint; tax: bigint; supply: bigint };

/** The smallest amount a screen shows: one won, or one cent of a dollar token. */
function shownUnit(profile: CommercePaymentProfile) {
  const token = paymentToken(profile);
  return 10n ** BigInt(isWon(token) ? token.decimals : Math.max(0, token.decimals - 2));
}

/** One line of an order. Undefined when the store has not said what tax its price includes. */
export function commerceLineTax(row: CommercePrice, quantity: number, profile?: CommercePaymentProfile): CommerceTax | undefined {
  const kind = row.metadata?.tax;
  if (!kind || !profile || row.asset !== profile.asset || !Number.isSafeInteger(quantity) || quantity < 1) return undefined;
  const total = BigInt(row.unitPrice) * BigInt(quantity);
  const unit = shownUnit(profile);
  // 10% on top of the supply value: the supply value is 10/11 of the price that includes it.
  // Half up: (2·10·total + 11·unit) / (2·11·unit), in whole units.
  const supply = kind === "vat10" ? (total * 20n + 11n * unit) / (22n * unit) * unit : total;
  return { total, tax: total - supply, supply };
}

/** The least a visitor buys at once from a tax-free shop to claim the tax back, in won (the decree's rule). */
export const commerceRefundMinimumWon = 15_000n;

/**
 * The goods in a basket a foreign visitor may claim the tax back on, while the store is a designated tax-free shop.
 * Only for won prices; reaches says whether they come to the minimum. The refund itself is the store's counter's.
 */
export function commerceRefundGoods(selection: CommerceSelection, profile?: CommercePaymentProfile, taxFreeShop?: string) {
  if (!taxFreeShop || !profile || !isWon(paymentToken(profile))) return undefined;
  const lines = Object.values(selection).filter(({ row }) => row.metadata?.taxRefund && row.metadata.tax === "vat10" && row.metadata.takeAway);
  if (!lines.length) return undefined;
  const total = lines.reduce((sum, { row, quantity }) => sum + BigInt(row.unitPrice) * BigInt(quantity), 0n);
  const unit = 10n ** BigInt(paymentToken(profile).decimals);
  return { total, reaches: total >= commerceRefundMinimumWon * unit, lines: lines.map(({ row }) => row) };
}

/** A basket: the sum of its lines, or undefined while any line has no tax said. */
export function commerceCartTax(selection: CommerceSelection, profile?: CommercePaymentProfile): CommerceTax | undefined {
  const lines = Object.values(selection).map(({ row, quantity }) => commerceLineTax(row, quantity, profile));
  if (!lines.length || lines.some((line) => !line)) return undefined;
  const sum: CommerceTax = { total: 0n, tax: 0n, supply: 0n };
  for (const line of lines as CommerceTax[]) {
    sum.total += line.total;
    sum.tax += line.tax;
    sum.supply += line.supply;
  }
  return sum;
}
