// The order on the kiosk screen: menu items and quantities, the order number the customer sees,
// and the digital receipt after an approved payment (payment-protocol.md 6, receipt).

import { receiptText, type DigitalReceipt, type ReceiptItem } from "@nu54/protocol";
import type { KioskConfig, Vault } from "./config.ts";

export interface MenuItem {
  name: string;
  /** Price of one, in the token's base units. */
  price: string;
}

/** Quantities by menu index. */
export type Cart = Record<number, number>;

export function cartItems(menu: MenuItem[], cart: Cart): ReceiptItem[] {
  return menu.flatMap((m, i) => ((cart[i] ?? 0) > 0 ? [{ name: m.name, qty: cart[i], unitPrice: m.price }] : []));
}

export function cartTotal(items: ReceiptItem[]): bigint {
  return items.reduce((s, i) => s + BigInt(i.unitPrice) * BigInt(i.qty), 0n);
}

/**
 * The next order number, "A-0001" upward, starting again each day (tablet's local date). Kept in
 * the settings so a restart does not reuse one.
 */
export async function nextOrderNumber(vault: Pick<Vault, "getSetting" | "putSetting">, now = new Date()): Promise<string> {
  const day = `${now.getFullYear()}-${now.getMonth() + 1}-${now.getDate()}`;
  const kept = JSON.parse((await vault.getSetting("orderNumber")) || "{}") as { day?: string; n?: number };
  const n = kept.day === day ? (kept.n ?? 0) + 1 : 1;
  await vault.putSetting("orderNumber", JSON.stringify({ day, n }));
  return `A-${String(n).padStart(4, "0")}`;
}

/** What the kiosk knows about an order when it is approved. */
export interface ReceiptInput {
  orderNumber: string;
  orderId: string;
  items: ReceiptItem[];
  total: bigint;
  payer: string;
  time?: number;
}

/** The receipt for an approved payment: the attested merchant name and the merchant's own details. */
export function buildReceipt(config: KioskConfig, o: ReceiptInput): DigitalReceipt {
  const p = config.merchantProfile ?? {};
  return {
    v: 1,
    store: config.attestation.name,
    ...(p.representative ? { representative: p.representative } : {}),
    ...(p.businessNumber ? { businessNumber: p.businessNumber } : {}),
    ...(p.address ? { address: p.address } : {}),
    ...(p.phone ? { phone: p.phone } : {}),
    orderNumber: o.orderNumber,
    orderId: o.orderId,
    time: o.time ?? Math.floor(Date.now() / 1000),
    items: o.items,
    total: o.total.toString(),
    token: { symbol: config.tokenSymbol, decimals: config.tokenDecimals },
    chainId: config.chainId,
    payer: o.payer,
  };
}

/** The receipt text for payment.outcome, or undefined when it does not fit (the phone then shows the outcome only). */
export function receiptForPhone(r: DigitalReceipt): string | undefined {
  try {
    return receiptText(r);
  } catch {
    return undefined;
  }
}
