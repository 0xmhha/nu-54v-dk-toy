import assert from "node:assert/strict";
import { namedToken, rememberTokens, tokenMoney } from "../src/money.ts";

const won = { name: "NU54 Test KRW", symbol: "tKRW", label: "tKRW", decimals: 2 };
assert.equal(tokenMoney("6500000"), "6.50 TestUSDC");
assert.equal(tokenMoney("-500000"), "-0.50 TestUSDC");
for (const [units, text] of [["6500001", "6.50"], ["6509999", "6.50"], ["6599999", "6.59"], ["999999", "0.99"], ["1", "0.00"], ["-9999", "0.00"], ["-509999", "-0.50"]])
  assert.equal(tokenMoney(units), `${text} TestUSDC`);
assert.equal(tokenMoney("6500000", won), "65,000원");
assert.equal(tokenMoney("-1250050", won), "-12,500.50원");
assert.equal(tokenMoney("0", won), "0원");
assert.equal(namedToken("0xA1"), undefined);
rememberTokens([{ asset: "0xa1", token: won }, { asset: "0xa2" }, {}]);
assert.deepEqual(namedToken("0xA1"), won);
assert.equal(namedToken("0xa2"), undefined);
assert.equal(namedToken(), undefined);
