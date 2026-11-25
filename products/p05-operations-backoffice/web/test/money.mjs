import assert from "node:assert/strict";
import { namedToken, rememberTokens, tokenMoney } from "../src/money.ts";

const won = { name: "NU54 Test KRW", symbol: "tKRW", label: "tKRW", decimals: 2 };
assert.equal(tokenMoney("6500000"), "6.50 TestUSDC");
assert.equal(tokenMoney("-500000"), "-0.50 TestUSDC");
assert.equal(tokenMoney("6500001"), "6.500001 TestUSDC");
assert.equal(tokenMoney("6500000", won), "65,000원");
assert.equal(tokenMoney("-1250050", won), "-12,500.50원");
assert.equal(tokenMoney("0", won), "0원");
assert.equal(namedToken("0xA1"), undefined);
rememberTokens([{ asset: "0xa1", token: won }, { asset: "0xa2" }, {}]);
assert.deepEqual(namedToken("0xA1"), won);
assert.equal(namedToken("0xa2"), undefined);
assert.equal(namedToken(), undefined);
