// The digital receipt (payment-protocol.md 6, receipt): what the kiosk shows after an approved
// payment and hands to the phone app inside payment.outcome. The settlement itself is on the
// chain; the receipt is the merchant's statement of what was bought, with the transaction that
// paid for it. The phone app opens the explorer for the transaction it got as `txHash`, never a
// URL from the kiosk.

import { ProtocolError } from "./bytes.ts";

export interface ReceiptItem {
  name: string;
  qty: number;
  /** Price of one, in the token's base units. */
  unitPrice: string;
}

export interface DigitalReceipt {
  v: 1;
  /** The merchant's name as the operator attested it. */
  store: string;
  /** What the merchant entered at the kiosk (not checked by the operator). */
  representative?: string;
  businessNumber?: string;
  address?: string;
  phone?: string;
  /** The number the customer sees, e.g. "A-0007". */
  orderNumber: string;
  orderId: string;
  /** Unix seconds, when the payment was approved. */
  time: number;
  items: ReceiptItem[];
  /** Total in base units; the sum of the items. */
  total: string;
  token: { symbol: string; decimals: number };
  chainId: number;
  /** The device account that paid. */
  payer: string;
}

/** Longest receipt text payment.outcome carries (schema `receipt.maxLength`). */
export const RECEIPT_MAX_LEN = 1600;

/** The receipt as payment.outcome carries it; throws when it would not fit. */
export function receiptText(r: DigitalReceipt): string {
  const total = r.items.reduce((s, i) => s + BigInt(i.unitPrice) * BigInt(i.qty), 0n);
  if (total.toString() !== r.total) throw new Error("receipt total is not the sum of its items");
  const text = JSON.stringify(r);
  if (text.length > RECEIPT_MAX_LEN) throw new Error(`receipt is ${text.length} characters, over ${RECEIPT_MAX_LEN}`);
  return text;
}

/** Reads a receipt text; throws ProtocolError when it is not one. */
export function parseReceipt(text: string): DigitalReceipt {
  let r: DigitalReceipt;
  try {
    r = JSON.parse(text) as DigitalReceipt;
  } catch {
    throw new ProtocolError("BAD_FRAME", "receipt is not JSON");
  }
  const ok =
    r?.v === 1 && typeof r.store === "string" && typeof r.orderNumber === "string" && typeof r.time === "number" &&
    Array.isArray(r.items) && r.items.every((i) => typeof i.name === "string" && Number.isInteger(i.qty) && /^\d+$/.test(i.unitPrice)) &&
    /^\d+$/.test(r.total) && typeof r.token?.symbol === "string" && Number.isInteger(r.token?.decimals);
  if (!ok) throw new ProtocolError("BAD_FRAME", "receipt fields are missing or malformed");
  return r;
}

/** Explorers the apps open, by chain id. */
const EXPLORERS: Record<number, string> = {
  8283: "https://explorer.stablenet.network",
};

/** The explorer page of a transaction, or null for an unknown chain or a malformed hash. */
export function explorerTxUrl(chainId: number, txHash: string): string | null {
  const base = EXPLORERS[chainId];
  return base && /^0x[0-9a-fA-F]{64}$/.test(txHash) ? `${base}/tx/${txHash}` : null;
}

/** An amount in base units as the screens show it: truncated to two decimals (N31). */
export function receiptAmount(baseUnits: string | bigint, decimals: number): string {
  const v = BigInt(baseUnits);
  const unit = 10n ** BigInt(decimals);
  const cents = ((v % unit) * 100n) / unit;
  return `${(v / unit).toLocaleString("en-US")}.${cents.toString().padStart(2, "0")}`;
}
