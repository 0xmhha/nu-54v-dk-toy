// Receipt lookup from the P07 indexer (P07 design 4). The receipt is a convenience for the
// screen: "paid" was already decided on the chain ([N08]), so a slow or missing indexer never
// changes the payment result.

export interface Receipt {
  merchant: string;
  orderId: string;
  device: string;
  amount: string;
  blockNumber: number;
  blockTime: number;
  txHashShort: string;
  duplicate: boolean;
}

export type ReceiptResult =
  | { status: "found"; receipt: Receipt }
  | { status: "notIndexed" }
  | { status: "stale"; cursor: number }
  | { status: "unavailable"; reason: string };

export interface ReceiptOptions {
  fetch?: typeof fetch;
  sleep?: (ms: number) => Promise<void>;
  /** The indexer catches up within about 10 s of finalization (P07-NFR-01): poll that long. */
  tries?: number;
  intervalMs?: number;
}

export async function fetchReceipt(indexer: string, merchant: string, orderId: string, o: ReceiptOptions = {}): Promise<ReceiptResult> {
  const get = o.fetch ?? fetch;
  const sleep = o.sleep ?? ((ms: number) => new Promise<void>((r) => setTimeout(r, ms)));
  const tries = o.tries ?? 6;
  const url = `${indexer.replace(/\/+$/, "")}/receipts/${merchant.toLowerCase()}/${orderId.toLowerCase()}`;
  let last: ReceiptResult = { status: "notIndexed" };
  for (let i = 0; i < tries; i++) {
    if (i > 0) await sleep(o.intervalMs ?? 2000);
    try {
      const res = await get(url);
      const body = (await res.json()) as Record<string, unknown>;
      if (res.status === 200) return { status: "found", receipt: body as unknown as Receipt };
      if (res.status === 503 && body.error === "RPC_STALE") last = { status: "stale", cursor: Number(body.cursor) };
      else if (res.status === 404) last = { status: "notIndexed" };
      else return { status: "unavailable", reason: `HTTP ${res.status}` };
    } catch (e) {
      last = { status: "unavailable", reason: e instanceof Error ? e.message : String(e) };
    }
  }
  return last;
}

/** "2026-10-01 20:27:03" in the tablet's local time. */
export function blockTimeText(unixSeconds: number): string {
  const d = new Date(unixSeconds * 1000);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}
