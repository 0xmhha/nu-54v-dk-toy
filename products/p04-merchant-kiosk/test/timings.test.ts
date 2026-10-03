// W12-04 timing rows from the kiosk's orders, and reading them from Android SharedPreferences.
import type { OrderRecord } from "../src/kiosk/orders.ts";
import { ordersFromSharedPrefs, timingCsv, timingRows } from "../src/kiosk/timings.ts";

const order = (i: number, over: Partial<OrderRecord> = {}): OrderRecord => ({
  orderId: "0x" + i.toString(16).padStart(64, "0"), amount: "1", state: "approved", createdAt: i, updatedAt: i,
  signed: { auth: {} as never, signature: "0x", device: "0xd", requestedAt: 1000 * i }, finishedAt: 1000 * i + 4200 + i, ...over,
});

test("the last 20 signed payments with a final result, request -> result in ms", () => {
  const orders = [
    ...Array.from({ length: 25 }, (_, i) => order(i + 1)),
    order(30, { state: "cancelled", signed: undefined, finishedAt: undefined }), // no signature: not a timed payment
    order(31, { state: "Checking", finishedAt: undefined }), // not final yet
  ];
  const rows = timingRows(orders);
  expect(rows).toHaveLength(20);
  expect(rows[0]).toEqual({ run: 1, orderId: "0x000000…0006", ms: 4206, outcome: "approved" });
  expect(timingCsv(rows).split("\n")[0]).toBe("run,orderId,ms,outcome");
});

test("orders come out of the SharedPreferences XML with its escaping undone", () => {
  const json = JSON.stringify([order(1)]).replace(/&/g, "&amp;").replace(/"/g, "&quot;");
  const xml = `<?xml version='1.0' encoding='utf-8' standalone='yes' ?>\n<map>\n    <string name="config">{}</string>\n    <string name="orders">${json}</string>\n</map>\n`;
  expect(ordersFromSharedPrefs(xml)).toEqual([order(1)]);
  expect(ordersFromSharedPrefs("<map></map>")).toEqual([]);
});
