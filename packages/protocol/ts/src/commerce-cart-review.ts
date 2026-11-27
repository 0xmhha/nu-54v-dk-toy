import {
  commerceAmount,
  type CommercePaymentProfile,
} from "./commerce-payment.ts";
import type { CommercePrice } from "./commerce-v2.ts";

type Line = { row: CommercePrice; quantity: number };
export type CommerceSelection = Record<string, Line>;
export function commerceLineKey(row: Pick<CommercePrice, "sku" | "optionKey">) {
  return row.sku + "/" + row.optionKey;
}

export function commerceLineCapacity(row: CommercePrice, selection: CommerceSelection) {
  const siblingQuantity = Object.values(selection).reduce((sum, line) =>
    sum + (line.row.sku === row.sku && line.row.optionKey !== row.optionKey ? line.quantity : 0), 0);
  return Math.min(1000, row.available, Math.max(0, (row.skuAvailable ?? Infinity) - siblingQuantity));
}

export function commerceHasSiblingOption(row: CommercePrice, selection: CommerceSelection) {
  return Object.values(selection).some(line =>
    line.row.sku === row.sku && line.row.optionKey !== row.optionKey);
}

export function commerceNeedsOptionSupport(selection: CommerceSelection) {
  return Object.values(selection).some(({row}) =>
    row.skuAvailable === undefined && commerceHasSiblingOption(row, selection));
}

export type CommerceCartReview = {
  selection: CommerceSelection;
  previousTotals: { asset: string; amount: string }[];
  shortages: Line[];
  priceChanges: { previous: CommercePrice; current: CommercePrice }[];
};

export function commerceCartTotals(selection: CommerceSelection) {
  const totals = new Map<string, bigint>();
  for (const { row, quantity } of Object.values(selection)) {
    totals.set(
      row.asset,
      (totals.get(row.asset) ?? 0n) + BigInt(row.unitPrice) * BigInt(quantity)
    );
  }
  return Array.from(totals, ([asset, amount]) => ({
    asset,
    amount: amount.toString(),
  }));
}

export function reviewCommerceCart(
  previous: CommerceSelection,
  catalog: CommercePrice[]
): CommerceCartReview {
  const selection: CommerceSelection = {};
  const shortages: Line[] = [];
  const priceChanges: CommerceCartReview["priceChanges"] = [];
  const lines = Object.values(previous).sort((a, b) =>
    a.row.sku < b.row.sku ? -1 : a.row.sku > b.row.sku ? 1 :
      a.row.optionKey < b.row.optionKey ? -1 : a.row.optionKey > b.row.optionKey ? 1 : 0);
  for (const { row, quantity } of lines) {
    const current = catalog.find(
      (candidate) =>
        candidate.sku === row.sku && candidate.optionKey === row.optionKey
    );
    const remaining = current ? Math.min(quantity, commerceLineCapacity(current, selection)) : 0;
    if (remaining < quantity)
      shortages.push({ row, quantity: quantity - remaining });
    if (current && remaining > 0) {
      selection[commerceLineKey(row)] = { row: current, quantity: remaining };
      if (current.unitPrice !== row.unitPrice || current.asset !== row.asset)
        priceChanges.push({ previous: row, current });
    }
  }
  return {
    selection,
    previousTotals: commerceCartTotals(previous),
    shortages,
    priceChanges,
  };
}

// Ambiguous or expired command receipts remain attached to the original order.
export function commerceReservationRejected(error: unknown) {
  return (
    error instanceof Error &&
    (error.message === "OUT_OF_STOCK" || error.message === "CATALOG_CHANGED")
  );
}

export function commerceCartAmount(
  amount: string,
  asset: string,
  profile?: CommercePaymentProfile,
  language: "ko" | "en" = "ko"
) {
  const text = commerceAmount(amount, asset, profile, language);
  return profile?.asset === asset ? text : `${text} · ${asset}`;
}
