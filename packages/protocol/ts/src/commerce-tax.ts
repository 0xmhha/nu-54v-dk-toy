// The tax inside a price, line by line. A price includes its tax; each line's tax is cut to the whole won (or the cent).
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
  // 10% on top of the supply value is 1/11 of the price that includes it.
  const tax = kind === "vat10" ? total / 11n / unit * unit : 0n;
  return { total, tax, supply: total - tax };
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
