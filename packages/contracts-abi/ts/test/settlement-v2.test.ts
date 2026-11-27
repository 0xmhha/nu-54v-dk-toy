import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { paymentSettlementAbi, settlementV2Abi } from "../src/index.ts";

test("public v1 and v2 ABI consumers keep distinct events, types and calls", () => {
  const schema = JSON.parse(readFileSync(new URL("../../../protocol/v2/schema.json", import.meta.url), "utf8"));
  const fixture = JSON.parse(readFileSync(new URL("../../abi/vectors/settlement-v2.json", import.meta.url), "utf8"));
  assert.ok(paymentSettlementAbi.some((item) => item.type === "function" && item.name === "cashOut"));
  assert.ok(paymentSettlementAbi.some((item) => item.type === "event" && item.name === "PaymentSettled"));
  const settle = settlementV2Abi.find((item) => item.type === "function" && item.name === "settle");
  assert.deepEqual(settle?.inputs[0].components.map(({ name, type }) => [name, type]), schema.quote.fields);
  const paid = settlementV2Abi.find((item) => item.type === "event" && item.name === "Paid");
  const refunded = settlementV2Abi.find((item) => item.type === "event" && item.name === "Refunded");
  assert.deepEqual(paid?.inputs.filter((input) => input.indexed).map((input) => input.name),
    ["merchantId", "orderId", "payer"]);
  assert.deepEqual(refunded?.inputs.filter((input) => input.indexed).map((input) => input.name),
    ["merchantId", "orderId", "refundId"]);
  assert.equal(fixture.chainId, 8283);
  const cases = fixture.cases.map(({ status, settlementLogs }) => [
    status, settlementLogs.length, settlementLogs.length ? BigInt(`0x${settlementLogs[0].data.slice(130, 194)}`) : null,
  ]);
  assert.deepEqual(cases, [[0, 0, null], [1, 1, 30n], [1, 1, 10n], [1, 1, 20n], [0, 0, null]]);
});
