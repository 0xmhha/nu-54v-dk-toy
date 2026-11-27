import { parsePickupQr } from "@nu54/protocol";

// The pickup QR check of one ready order (traveller-services.md chapter 9 item 8): staff open the order, scan the
// customer's QR, and opsd records the handover only when the QR belongs to that order of this store.

/** One line of the order as the scan answers it. */
export type HandedLine = { label: string; option?: string; quantity: number };
export type Handed = { saleRef: string; pickupNo: number; items: HandedLine[] };

/**
 * Why a scan was refused. opsd answers EXPIRED alike for a QR that timed out, was used, or belongs to another store,
 * so `unusable` stands for all three; `other-order` and `not-ready` are told apart.
 */
export type Refusal = "unusable" | "other-order" | "not-ready" | "denied" | "unanswered";
export type ScanOutcome = { kind: "handed"; handed: Handed } | { kind: "refused"; reason: Refusal };

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

function handed(body: unknown, sale: string): Handed | undefined {
  const reply = body as Partial<Handed> | null;
  if (!reply || typeof reply !== "object" || reply.saleRef !== sale || !reference.test(sale) ||
      !Number.isSafeInteger(reply.pickupNo) || (reply.pickupNo ?? 0) < 1 || !Array.isArray(reply.items)) return undefined;
  const items: HandedLine[] = [];
  for (const line of reply.items) {
    if (!line || typeof line.label !== "string" || !line.label || !Number.isSafeInteger(line.quantity) || line.quantity < 1 ||
        (line.option != null && typeof line.option !== "string")) return undefined;
    items.push({ label: line.label, ...(line.option && line.option !== "standard" ? { option: line.option } : {}), quantity: line.quantity });
  }
  return { saleRef: sale, pickupNo: reply.pickupNo!, items };
}

/** What opsd's answer means for the counter. Anything not understood is `unanswered`: the order is read again. */
export function scanOutcome(status: number, body: unknown, sale: string): ScanOutcome {
  const code = (body as { error?: unknown } | null)?.error;
  if (status === 200) {
    const done = handed(body, sale);
    return done ? { kind: "handed", handed: done } : { kind: "refused", reason: "unanswered" };
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
