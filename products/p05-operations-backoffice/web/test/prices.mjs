import assert from "node:assert/strict";
import { humanAmount, priceProblem, rowTotal, shownTotal } from "../src/prices.ts";

const usd = { symbol: "TestUSDC", decimals: 6 };
const won = { symbol: "KRW", decimals: 2, token: { name: "NU54 Test KRW", symbol: "tKRW", label: "KRW", decimals: 2 } };
const row = (price, tax = "", discount = "") => ({ price, tax, discount });

// Input fields keep every digit, so saving a row again never changes a price.
assert.equal(humanAmount("6500001", 6), "6.500001");
assert.equal(humanAmount("6500000", 6), "6.50");
assert.equal(humanAmount("650055", 2), "6500.55");
for (const units of ["1", "6500001", "999999999"]) {
  assert.equal(rowTotal(row(humanAmount(units, 6)), usd), BigInt(units));
  assert.equal(rowTotal(row(humanAmount(units, 2)), won), BigInt(units));
}
assert.equal(rowTotal(row("6500", "100", "50"), won), 655000n);
assert.equal(rowTotal(row("6.5000001"), usd), undefined);
assert.equal(rowTotal(row("6500.555"), won), undefined);
// The screen follows the token's symbol, not the label.
assert.equal(shownTotal(row("6500"), won), "6,500원");
assert.equal(shownTotal(row("6.509999"), usd), "6.50 TestUSDC");
assert.equal(shownTotal(row("abc"), usd), "abc");
for (const bad of [row("6500.55"), row("6500", "0.50"), row("6500", "", "0.01"), row("6500", "0.50", "0.50")])
  assert.match(priceProblem([row("1000"), bad], won), /원 단위/);
assert.match(priceProblem([row("0")], won), /0보다/);
assert.match(priceProblem([row("1", "", "1")], usd), /둘째 자리/);
for (const bad of [row("6.500001"), row("6.5", "0.001"), row("6.5", "", "0.001"), row("0.005")])
  assert.match(priceProblem([row("1.00"), bad], usd), /둘째 자리/);
assert.equal(priceProblem([row("6500", "0", "500")], won), undefined);
assert.equal(priceProblem([row("6.50", "0.01", "0.02")], usd), undefined);
