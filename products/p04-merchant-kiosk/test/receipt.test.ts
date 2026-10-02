// Receipt lookup against the P07 API shapes (P07 design 4); the indexer is never in the paid path.
import { fetchReceipt } from "../src/kiosk/receipt.ts";

const M = "0xF92a32CEB9d940057be7b76BF4e2Eb76409F61A4";
const O = "0xea4ff5e9d9680b14c5a0209810062e6612c0814522ab2e8d84caa97e70e7eaa0";
const RECEIPT = { merchant: M.toLowerCase(), orderId: O, device: "0xbc3152c1fd512552b3962e86c7b7c248ea17f4d8", amount: "1000000",
  blockNumber: 21181324, blockTime: 1790854023, txHashShort: "0xaf8adb…0b00", duplicate: false };

function server(answers: { status: number; body: unknown }[]) {
  const urls: string[] = [];
  const f = (async (url: string) => {
    urls.push(url);
    const a = answers[Math.min(urls.length - 1, answers.length - 1)];
    return { status: a.status, json: async () => a.body } as Response;
  }) as unknown as typeof fetch;
  return { f, urls };
}
const noSleep = async () => {};

test("found after the indexer catches up", async () => {
  const s = server([{ status: 404, body: { error: "NOT_INDEXED" } }, { status: 200, body: RECEIPT }]);
  const r = await fetchReceipt("http://indexer:8080/", M, O, { fetch: s.f, sleep: noSleep });
  expect(r).toEqual({ status: "found", receipt: RECEIPT });
  expect(s.urls[0]).toBe(`http://indexer:8080/receipts/${M.toLowerCase()}/${O}`);
  expect(s.urls).toHaveLength(2);
});

test("still not indexed after polling, or the indexer is behind", async () => {
  const missing = server([{ status: 404, body: { error: "NOT_INDEXED" } }]);
  expect(await fetchReceipt("http://i", M, O, { fetch: missing.f, sleep: noSleep, tries: 3 })).toEqual({ status: "notIndexed" });
  expect(missing.urls).toHaveLength(3);
  const behind = server([{ status: 503, body: { error: "RPC_STALE", cursor: 40 } }]);
  expect(await fetchReceipt("http://i", M, O, { fetch: behind.f, sleep: noSleep, tries: 2 })).toEqual({ status: "stale", cursor: 40 });
});

test("an unreachable indexer is reported, not thrown", async () => {
  const down = (async () => { throw new Error("Network request failed"); }) as unknown as typeof fetch;
  expect(await fetchReceipt("http://i", M, O, { fetch: down, sleep: noSleep, tries: 2 })).toEqual({ status: "unavailable", reason: "Network request failed" });
  const bad = server([{ status: 400, body: { error: "BAD_REQUEST" } }]);
  expect(await fetchReceipt("http://i", M, O, { fetch: bad.f, sleep: noSleep })).toEqual({ status: "unavailable", reason: "HTTP 400" });
});
