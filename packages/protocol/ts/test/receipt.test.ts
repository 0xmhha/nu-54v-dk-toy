// The digital receipt format: sums, size limit, parsing and the explorer link.
import { test } from "node:test";
import assert from "node:assert/strict";
import { explorerTxUrl, parseReceipt, receiptAmount, receiptText, RECEIPT_MAX_LEN, type DigitalReceipt } from "../src/index.ts";

const receipt: DigitalReceipt = {
  v: 1, store: "NU54 Test Cafe", representative: "Test Owner", businessNumber: "000-00-00000", address: "Test address", phone: "000-0000-0000",
  orderNumber: "A-0001", orderId: "0x" + "01".repeat(32), time: 1_790_000_000,
  items: [{ name: "Americano", qty: 2, unitPrice: "1500000" }, { name: "Cookie", qty: 1, unitPrice: "1000000" }],
  total: "4000000", token: { symbol: "tUSDC", decimals: 6 }, chainId: 8283, payer: "0x" + "bc".repeat(20),
};

test("a receipt round-trips and its total must be the sum of its items", () => {
  assert.deepEqual(parseReceipt(receiptText(receipt)), receipt);
  assert.throws(() => receiptText({ ...receipt, total: "1" }), /sum/);
});

test("a receipt over RECEIPT_MAX_LEN is refused", () => {
  const items = Array.from({ length: 60 }, (_, k) => ({ name: `Item number ${k} with a long name`, qty: 1, unitPrice: "1" }));
  assert.throws(() => receiptText({ ...receipt, items, total: "60" }), new RegExp(String(RECEIPT_MAX_LEN)));
});

test("malformed receipts are BAD_FRAME", () => {
  assert.throws(() => parseReceipt("not json"), /BAD_FRAME/);
  assert.throws(() => parseReceipt(JSON.stringify({ ...receipt, items: [{ name: "x", qty: 1.5, unitPrice: "1" }] })), /BAD_FRAME/);
});

test("explorer links only for known chains and well-formed hashes", () => {
  const h = "0x" + "ab".repeat(32);
  assert.equal(explorerTxUrl(8283, h), `https://explorer.stablenet.network/tx/${h}`);
  assert.equal(explorerTxUrl(1, h), null);
  assert.equal(explorerTxUrl(8283, "https://evil.example/" + h), null);
});

test("amounts are truncated to two decimals with thousands separators", () => {
  assert.equal(receiptAmount("1234567890", 6), "1,234.56");
  assert.equal(receiptAmount(999n, 6), "0.00");
});
