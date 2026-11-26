import assert from "node:assert/strict";
import { includedTax } from "../src/taxes.ts";

const won = { symbol: "KRW", decimals: 2, token: { name: "NU54 Test KRW", symbol: "tKRW", label: "KRW", decimals: 2 } };
const coin = { symbol: "KRWC", decimals: 18, token: { name: "WKRC", symbol: "WKRC", label: "KRWC", decimals: 18 } };
const usd = { symbol: "TestUSDC", decimals: 6 };
// The same counts as the apps: 1/11 of the price, cut to the whole won or the cent.
assert.equal(includedTax(600000n, "vat10", won), 54500n);
assert.equal(includedTax(650000n, "vat10", won), 59000n);
assert.equal(includedTax(6000n * 10n ** 18n, "vat10", coin), 545n * 10n ** 18n);
assert.equal(includedTax(6500000n, "vat10", usd), 590000n);
assert.equal(includedTax(650000n, "exempt", won), 0n);
assert.equal(includedTax(650000n, undefined, won), 0n);
