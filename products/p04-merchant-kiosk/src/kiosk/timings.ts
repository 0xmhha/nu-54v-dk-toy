// The W12-04 timing record (20 payments in a row, each approved within 10 s of the request being
// delivered) from the orders the kiosk keeps. scripts/pull-timings.ts reads them off the tablet
// and the week-12 evidence tool judges the CSV (products/p10-platform/acceptance/w12.py timings).

import type { OrderRecord } from "./orders.ts";

export interface TimingRow {
  run: number;
  orderId: string; // shortened [N16]
  ms: number; // request delivered -> final result
  outcome: string;
}

/** The last `count` payments that got a signature and a final result, oldest first. */
export function timingRows(orders: OrderRecord[], count = 20): TimingRow[] {
  const done = orders
    .filter((o) => o.signed && o.finishedAt !== undefined && ["approved", "refused", "failed"].includes(o.state))
    .sort((a, b) => a.createdAt - b.createdAt)
    .slice(-count);
  return done.map((o, i) => ({
    run: i + 1,
    orderId: `${o.orderId.slice(0, 8)}…${o.orderId.slice(-4)}`,
    ms: o.finishedAt! - o.signed!.requestedAt,
    outcome: o.state,
  }));
}

export function timingCsv(rows: TimingRow[]): string {
  return ["run,orderId,ms,outcome", ...rows.map((r) => `${r.run},${r.orderId},${r.ms},${r.outcome}`)].join("\n") + "\n";
}

/** The orders JSON in an Android SharedPreferences file (KioskVault settings, key "orders"). */
export function ordersFromSharedPrefs(xml: string): OrderRecord[] {
  const m = /<string name="orders">([\s\S]*?)<\/string>/.exec(xml);
  if (!m) return [];
  const text = m[1]
    .replace(/&quot;/g, '"').replace(/&apos;/g, "'").replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&amp;/g, "&");
  return JSON.parse(text) as OrderRecord[];
}
