// Orders the kiosk keeps across restarts (P04 design 3 and 6, P04-NFR-05).
//
// A payment's record is written at every step that matters for recovery, and the signature is
// written before it is submitted: if the app dies after the device signed, the same signature
// can still be re-simulated and resent (P04-FR-14), and no new signature is asked for the order.
//
//   waitingDevice -> signed -> submitted -> approved | refused | failed | Checking
//   waitingDevice -> refused | cancelled           (device refusal, no press in time)
//   Checking      -> Checking | approved | refused | failed   (re-checks, same signature only)

import type { Outcome, Signed } from "../payment/submit.ts";

export type OrderState = "waitingDevice" | "signed" | "submitted" | "Checking" | "approved" | "refused" | "failed" | "cancelled";

export interface OrderRecord {
  orderId: string;
  amount: string;
  state: OrderState;
  createdAt: number;
  updatedAt: number;
  /** The device's signature and the authorization it signed (with its nonce). */
  signed?: Signed;
  txHash?: string;
  reason?: string;
  /** When the result became final (tablet clock, ms); with signed.requestedAt it times W12-04. */
  finishedAt?: number;
}

export type OrderEvent =
  | { type: "signed"; signed: Signed }
  | { type: "submitted"; txHash: string }
  | { type: "outcome"; outcome: Outcome }
  | { type: "refused"; reason: string } // by the device, before any signature
  | { type: "cancelled" };

const FINAL: OrderState[] = ["approved", "refused", "failed", "cancelled"];
export const isFinal = (s: OrderState) => FINAL.includes(s);

/** The allowed transitions of design 3; anything else is a bug and throws. */
export function transition(o: OrderRecord, e: OrderEvent, now: number): OrderRecord {
  const to = (state: OrderState, extra: Partial<OrderRecord> = {}): OrderRecord => ({ ...o, ...extra, state, updatedAt: now });
  const bad = () => new Error(`order ${o.orderId.slice(0, 10)}: ${e.type} in ${o.state}`);
  switch (e.type) {
    case "signed":
      if (o.state !== "waitingDevice") throw bad();
      return to("signed", { signed: e.signed });
    case "submitted":
      if (o.state !== "signed" && o.state !== "Checking") throw bad();
      return to("submitted", { txHash: e.txHash });
    case "refused":
    case "cancelled":
      if (o.state !== "waitingDevice") throw bad(); // a signed order is never cancelled
      return e.type === "refused" ? to("refused", { reason: e.reason }) : to("cancelled");
    case "outcome": {
      if (!o.signed || isFinal(o.state)) throw bad();
      const out = e.outcome;
      const tx = "txHash" in out && out.txHash ? { txHash: out.txHash } : {};
      if (out.status === "Checking") return to("Checking", tx);
      const done = { ...tx, finishedAt: now };
      if (out.status === "approved") return to("approved", done);
      return to(out.status, { ...done, reason: out.reason });
    }
  }
}

/** Key-value storage under the app's control (KioskVault settings on the tablet). */
export interface KeyValue {
  getSetting(name: string): Promise<string | null>;
  putSetting(name: string, value: string): Promise<void>;
}

const KEY = "orders";
const KEEP_FINAL = 100;

/** Orders by orderId, saved as one JSON value after every change. Open orders are never pruned. */
export class OrderStore {
  private orders = new Map<string, OrderRecord>();
  private readonly kv: KeyValue;
  private readonly clock: () => number;
  private constructor(kv: KeyValue, clock: () => number) {
    this.kv = kv;
    this.clock = clock;
  }

  static async open(kv: KeyValue, clock: () => number = Date.now): Promise<OrderStore> {
    const s = new OrderStore(kv, clock);
    const raw = await kv.getSetting(KEY);
    for (const o of raw ? (JSON.parse(raw) as OrderRecord[]) : []) s.orders.set(o.orderId, o);
    return s;
  }

  get(orderId: string): OrderRecord | undefined {
    return this.orders.get(orderId);
  }

  /** Orders that are not final: signed, submitted or Checking (and waitingDevice after a crash). */
  open(): OrderRecord[] {
    return [...this.orders.values()].filter((o) => !isFinal(o.state)).sort((a, b) => a.createdAt - b.createdAt);
  }

  async create(orderId: string, amount: bigint): Promise<OrderRecord> {
    const now = this.clock();
    const o: OrderRecord = { orderId, amount: amount.toString(), state: "waitingDevice", createdAt: now, updatedAt: now };
    this.orders.set(orderId, o);
    await this.save();
    return o;
  }

  async apply(orderId: string, e: OrderEvent): Promise<OrderRecord> {
    const o = this.orders.get(orderId);
    if (!o) throw new Error(`unknown order ${orderId}`);
    const next = transition(o, e, this.clock());
    this.orders.set(orderId, next);
    await this.save();
    return next;
  }

  private async save(): Promise<void> {
    const all = [...this.orders.values()];
    const finals = all.filter((o) => isFinal(o.state)).sort((a, b) => b.updatedAt - a.updatedAt);
    for (const o of finals.slice(KEEP_FINAL)) this.orders.delete(o.orderId);
    await this.kv.putSetting(KEY, JSON.stringify([...this.orders.values()]));
  }
}
