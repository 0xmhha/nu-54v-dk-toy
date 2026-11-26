import assert from "node:assert/strict";
import test from "node:test";
import { commerceCartTax, commerceLineKey, commerceLineTax, assertCommerceCatalogPage, type CommercePaymentProfile, type CommercePrice } from "../src/index.ts";

const asset = "0x" + "5".repeat(40);
const profile = (token: { name: string; symbol: string; label: string; decimals: number }) =>
  ({ asset, token } as unknown as CommercePaymentProfile);
const won = profile({ name: "NU54 Test KRW", symbol: "tKRW", label: "tKRW", decimals: 2 });
const coin = profile({ name: "WKRC", symbol: "WKRC", label: "KRWC", decimals: 18 });
const row = (unitPrice: string, tax?: "vat10" | "exempt", refund?: true): CommercePrice => ({
  version: 1, expiresAt: "2030-01-01T00:00:00Z", sku: "s" + unitPrice, optionKey: "standard", label: "x", asset, unitPrice, available: 5,
  ...(tax ? { metadata: { tax, ...(refund ? { taxRefund: refund } : {}) } } : {}),
});

test("a price includes its tax, cut to the whole won on each line", () => {
  assert.deepEqual(commerceLineTax(row("600000", "vat10"), 1, won), { total: 600000n, tax: 54500n, supply: 545500n });
  assert.deepEqual(commerceLineTax(row("650000", "vat10"), 1, won), { total: 650000n, tax: 59000n, supply: 591000n });
  assert.equal(commerceLineTax(row("650000", "vat10"), 3, won)!.tax, 177200n); // 19,500원 → 1,772원, not 3 × 590원
  assert.equal(commerceLineTax(row(6000n * 10n ** 18n + "", "vat10"), 1, coin)!.tax, 545n * 10n ** 18n);
  assert.deepEqual(commerceLineTax(row("650000", "exempt"), 2, won), { total: 1300000n, tax: 0n, supply: 1300000n });
  assert.equal(commerceLineTax(row("650000"), 1, won), undefined);
  assert.equal(commerceLineTax(row("650000", "vat10"), 1, undefined), undefined);
});

test("a dollar price keeps its tax to the cent", () => {
  const usd = profile({ name: "NU54 Test USD", symbol: "tUSDC", label: "TestUSDC", decimals: 6 });
  assert.equal(commerceLineTax(row("6500000", "vat10"), 1, usd)!.tax, 590000n); // 0.590909 → 0.59
});

test("a basket adds its lines, and says nothing while one line has no tax said", () => {
  const latte = row("600000", "vat10"), cake = row("650000", "vat10");
  const cart = { [commerceLineKey(latte)]: { row: latte, quantity: 1 }, [commerceLineKey(cake)]: { row: cake, quantity: 1 } };
  assert.deepEqual(commerceCartTax(cart, won), { total: 1250000n, tax: 113500n, supply: 1136500n });
  const plain = row("300000");
  assert.equal(commerceCartTax({ ...cart, [commerceLineKey(plain)]: { row: plain, quantity: 1 } }, won), undefined);
  assert.equal(commerceCartTax({}, won), undefined);
});

test("the menu carries the kind of tax and the refund mark, and refuses anything else", () => {
  assertCommerceCatalogPage([row("600000", "vat10", true), row("650000", "exempt")], 0, 4);
  for (const metadata of [{ tax: "vat20" }, { taxRefund: true }, { tax: "vat10", taxRefund: false }])
    assert.throws(() => assertCommerceCatalogPage([{ ...row("1"), metadata } as CommercePrice], 0, 4), /BAD_RESPONSE/);
});
