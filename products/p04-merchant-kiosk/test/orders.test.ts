// Orders across restarts (P04 design 3 and 6, P04-NFR-05, P04-FR-14/15).
import { OrderStore, transition, type KeyValue, type OrderRecord } from "../src/kiosk/orders.ts";
import type { Signed } from "../src/payment/submit.ts";

class Memory implements KeyValue {
  values = new Map<string, string>();
  async getSetting(n: string) { return this.values.get(n) ?? null; }
  async putSetting(n: string, v: string) { this.values.set(n, v); }
}

const SIGNED: Signed = {
  auth: { chainId: "8283", contract: "0xc0", merchant: "0xm", payout: "0xp", token: "0xt", amount: "1", orderId: "0x01", nonce: "7", expiry: "100" },
  signature: "0xsig", device: "0xd", requestedAt: 0,
};
const base: OrderRecord = { orderId: "0x01", amount: "1", state: "waitingDevice", createdAt: 0, updatedAt: 0 };

test("allowed transitions and the ones that must never happen", () => {
  const signed = transition(base, { type: "signed", signed: SIGNED }, 1);
  expect(signed.state).toBe("signed");
  expect(transition(signed, { type: "outcome", outcome: { status: "Checking", txHash: "0xaa" } }, 2)).toMatchObject({ state: "Checking", txHash: "0xaa" });
  expect(transition(signed, { type: "outcome", outcome: { status: "refused", reason: "OVER_CAP" } }, 2)).toMatchObject({ state: "refused", reason: "OVER_CAP" });
  expect(transition(base, { type: "cancelled" }, 1).state).toBe("cancelled");
  // A signed order is never cancelled (it may still settle), and a final order does not move.
  expect(() => transition(signed, { type: "cancelled" }, 2)).toThrow();
  const done = transition(signed, { type: "outcome", outcome: { status: "failed", reason: "EXPIRED" } }, 3);
  expect(() => transition(done, { type: "outcome", outcome: { status: "approved", event: {} as never } }, 4)).toThrow();
  expect(() => transition(base, { type: "outcome", outcome: { status: "Checking" } }, 1)).toThrow();
});

test("the store survives a restart and keeps open orders while pruning old final ones", async () => {
  const kv = new Memory();
  let t = 0;
  const s = await OrderStore.open(kv, () => ++t);
  await s.create("0xopen", 5n);
  await s.apply("0xopen", { type: "signed", signed: SIGNED });
  await s.apply("0xopen", { type: "outcome", outcome: { status: "Checking", txHash: "0xaa" } });
  for (let i = 0; i < 120; i++) {
    await s.create(`0x${i}`, 1n);
    await s.apply(`0x${i}`, { type: "cancelled" });
  }
  const again = await OrderStore.open(kv);
  expect(again.open().map((o) => [o.orderId, o.state, o.signed?.signature, o.txHash])).toEqual([["0xopen", "Checking", "0xsig", "0xaa"]]);
  const all = JSON.parse(kv.values.get("orders")!) as OrderRecord[];
  expect(all).toHaveLength(101); // 100 final + the open one
  expect(again.get("0x0")).toBeUndefined(); // the oldest final ones went first
});
