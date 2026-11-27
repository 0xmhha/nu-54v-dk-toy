import { parsePickupQr } from "@nu54/protocol";

// The pickup QR check of one ready order (traveller-services.md chapter 9 item 8): staff open the order, scan the
// customer's QR, and opsd records the handover only when the QR belongs to that order of this store.

/** One of today's orders, as the store finds it by its number. */
export type CheckedOrder = { sale: string; number: number; stage: string; items?: { label: string; option?: string; quantity: number }[] };
/** What reading an order gave: the order, no such order today (or no right to see it), or no usable answer. */
export type OrderRead = { kind: "found"; order: CheckedOrder } | { kind: "missing" } | { kind: "failed" };

export const orderUrl = (branch: string, number: string) =>
  `/v2/operations/state/${encodeURIComponent(branch)}/order?number=${encodeURIComponent(number)}`;

/** The order in an answer, or nothing when the answer is not an order. */
export function checkedOrder(body: unknown): CheckedOrder | undefined {
  const order = body as Partial<CheckedOrder> | null;
  if (!order || typeof order !== "object" || typeof order.sale !== "string" || !order.sale || !Number.isSafeInteger(order.number) ||
      (order.number ?? 0) < 1 || typeof order.stage !== "string" || !order.stage || (order.items != null && !Array.isArray(order.items))) return undefined;
  for (const line of order.items ?? [])
    if (!line || typeof line.label !== "string" || !Number.isSafeInteger(line.quantity) || (line.option != null && typeof line.option !== "string")) return undefined;
  return order as CheckedOrder;
}

/**
 * Reads today's order by its number. An answer that cannot be read as an order (a page of a proxy, a body cut short,
 * another shape) is `failed`, like no answer at all: the caller always has something to tell the staff.
 */
export async function readOrder(branch: string, number: string, get: typeof fetch = fetch): Promise<OrderRead> {
  let reply: Response;
  try {
    reply = await get(orderUrl(branch, number), { credentials: "same-origin" });
  } catch {
    return { kind: "failed" };
  }
  if (reply.status === 404) return { kind: "missing" };
  if (!reply.ok) return { kind: "failed" };
  const order = checkedOrder(await reply.json().catch(() => null));
  return order ? { kind: "found", order } : { kind: "failed" };
}

/** One line of the order as the scan answers it. `option` is the option's name when the menu gave one, else its key. */
export type HandedLine = { label: string; option?: string; quantity: number };
/** A recorded handover. `recordedAt` is the server's time of it; `alreadyRecorded` marks a repeated scan of the same QR. */
export type Handed = { saleRef: string; pickupNo: number; items: HandedLine[]; recordedAt?: Date; alreadyRecorded?: true };

/**
 * Why a scan was refused. opsd names five reasons: the QR timed out (`expired`), was used before (`already-used`,
 * with the time of that handover), belongs to another store or to another order, or its order is not ready. A server
 * that names none answers a timed-out, used and foreign QR alike, and `unusable` stands for those three.
 */
export type Refusal = "expired" | "already-used" | "other-store" | "other-order" | "not-ready" | "unusable" | "denied" | "unanswered";
export type ScanOutcome = { kind: "handed"; handed: Handed } | { kind: "refused"; reason: Refusal; recordedAt?: Date };

/** The token in what a camera, a handheld scanner or a paste gave. Only the outer spaces a field may keep are dropped. */
export function scannedToken(text: string): string | undefined {
  try {
    return parsePickupQr(text.trim());
  } catch {
    return undefined;
  }
}

export const pickupScanUrl = (branch: string, sale: string) =>
  `/v2/operations/pickup-scan/${encodeURIComponent(branch)}/${encodeURIComponent(sale)}`;

const reference = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

/** A Unix time in seconds from the server, when it is one this century. */
const moment = (value: unknown) =>
  Number.isSafeInteger(value) && (value as number) > 946_684_800 && (value as number) < 4_102_444_800 ? new Date((value as number) * 1000) : undefined;

function handed(body: unknown, sale: string): Handed | undefined {
  const reply = body as (Partial<Omit<Handed, "recordedAt" | "alreadyRecorded">> & { recordedAt?: unknown; alreadyRecorded?: unknown }) | null;
  if (!reply || typeof reply !== "object" || reply.saleRef !== sale || !reference.test(sale) ||
      !Number.isSafeInteger(reply.pickupNo) || (reply.pickupNo ?? 0) < 1 || !Array.isArray(reply.items)) return undefined;
  const items: HandedLine[] = [];
  for (const line of reply.items as (HandedLine & { optionLabel?: unknown })[]) {
    if (!line || typeof line.label !== "string" || !line.label || !Number.isSafeInteger(line.quantity) || line.quantity < 1 ||
        (line.option != null && typeof line.option !== "string") || (line.optionLabel != null && typeof line.optionLabel !== "string")) return undefined;
    const option = (line.optionLabel as string | undefined) || (line.option !== "standard" ? line.option : undefined);
    items.push({ label: line.label, ...(option ? { option } : {}), quantity: line.quantity });
  }
  const recordedAt = moment(reply.recordedAt);
  return { saleRef: sale, pickupNo: reply.pickupNo!, items, ...(recordedAt ? { recordedAt } : {}), ...(reply.alreadyRecorded === true ? { alreadyRecorded: true as const } : {}) };
}

const named: Record<string, [number, Refusal]> = {
  expired: [400, "expired"], already_used: [409, "already-used"], other_store: [400, "other-store"],
  other_order: [400, "other-order"], not_ready: [409, "not-ready"],
};

/** What opsd's answer means for the counter. Anything not understood is `unanswered`: the order is read again. */
export function scanOutcome(status: number, body: unknown, sale: string): ScanOutcome {
  const reply = body as { error?: unknown; reason?: unknown; recordedAt?: unknown } | null;
  const code = reply?.error;
  if (status === 200) {
    const done = handed(body, sale);
    return done ? { kind: "handed", handed: done } : { kind: "refused", reason: "unanswered" };
  }
  if (typeof reply?.reason === "string" && Object.hasOwn(named, reply.reason)) {
    const [expected, reason] = named[reply.reason];
    if (status !== expected) return { kind: "refused", reason: "unanswered" };
    const recordedAt = reason === "already-used" ? moment(reply.recordedAt) : undefined;
    return { kind: "refused", reason, ...(recordedAt ? { recordedAt } : {}) };
  }
  if (status === 400 && code === "EXPIRED") return { kind: "refused", reason: "unusable" };
  if (status === 400 && code === "MISMATCHED_EVIDENCE") return { kind: "refused", reason: "other-order" };
  if (status === 409 && code === "CHECKOUT_CONFLICT") return { kind: "refused", reason: "not-ready" };
  if (status === 404) return { kind: "refused", reason: "denied" };
  return { kind: "refused", reason: "unanswered" };
}

/** Sends the token with the branch and the order the staff opened. */
export async function sendPickupScan(branch: string, sale: string, token: string, post: typeof fetch = fetch): Promise<ScanOutcome> {
  let reply: Response;
  try {
    reply = await post(pickupScanUrl(branch, sale), {
      method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ token }),
    });
  } catch {
    return { kind: "refused", reason: "unanswered" };
  }
  const body = await reply.json().catch(() => null);
  return scanOutcome(reply.status, body, sale);
}

/** "카페라떼 뜨겁게 크게 샷 추가" and "1개", as the counter reads them. */
export const lineName = (line: HandedLine) => (line.option ? `${line.label} ${line.option}` : line.label);
export const lineCount = (line: HandedLine) => `${line.quantity}개`;
