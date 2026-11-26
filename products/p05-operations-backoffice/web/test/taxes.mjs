import assert from "node:assert/strict";
import { includedTax } from "../src/taxes.ts";

const won = { symbol: "KRW", decimals: 2, token: { name: "NU54 Test KRW", symbol: "tKRW", label: "KRW", decimals: 2 } };
const coin = { symbol: "KRWC", decimals: 18, token: { name: "WKRC", symbol: "WKRC", label: "KRWC", decimals: 18 } };
const usd = { symbol: "TestUSDC", decimals: 6 };
// The same counts as the apps: the supply value (10/11) cut to the whole won or the cent, the tax the rest.
assert.equal(includedTax(600000n, "vat10", won), 54600n);
assert.equal(includedTax(650000n, "vat10", won), 59100n);
assert.equal(includedTax(6000n * 10n ** 18n, "vat10", coin), 546n * 10n ** 18n);
assert.equal(includedTax(6500000n, "vat10", usd), 600000n);
assert.equal(includedTax(650000n, "exempt", won), 0n);
assert.equal(includedTax(650000n, undefined, won), 0n);
