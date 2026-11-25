import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { sha256 } from "@noble/hashes/sha256";
import { bytesToHex, utf8Encode } from "../src/bytes.ts";
import {
  commerceAmount, dollarToken, namedTokenAbiSha256, orderEscrowAbiSha256, paymentProfile, paymentToken, paysThroughEscrow,
  settlementAbiSha256, tokenAmount, type CommercePaymentProfile,
} from "../src/commerce-payment.ts";

const won = { name: "NU54 Test KRW", symbol: "tKRW", label: "tKRW", decimals: 2 };
const profile: CommercePaymentProfile = {
  chain: 8283, contract: "0x" + "1".repeat(40), asset: "0x" + "2".repeat(40), beneficiary: "0x" + "3".repeat(40),
  signer: "0x" + "4".repeat(40), merchantId: "5".repeat(64), keyVersion: 1, beneficiaryVersion: 1, maxBaseUnits: "30000000",
  abiSha256: namedTokenAbiSha256, token: won,
};
const read = (payment: object) => paymentProfile(JSON.stringify({ payment }));

test("a store names its token with a mark that earlier apps refuse", () => {
  assert.deepEqual(paymentToken(read(profile)), won);
  assert.ok(paysThroughEscrow(profile));
  assert.equal(namedTokenAbiSha256, bytesToHex(sha256(utf8Encode("NU54 named payment token\n" + orderEscrowAbiSha256)), false));
  // What an app from before this field did: any contract it does not know ends the registration.
  const earlier = (abi?: string) => {
    if (abi === undefined || abi === settlementAbiSha256) return false;
    if (abi === orderEscrowAbiSha256) return true;
    throw new Error("PAYMENT_PROFILE");
  };
  assert.throws(() => earlier(profile.abiSha256), /PAYMENT_PROFILE/);
  const { token: _token, ...unnamed } = profile;
  for (const bad of [
    { ...profile, abiSha256: orderEscrowAbiSha256 }, { ...profile, abiSha256: settlementAbiSha256 }, { ...profile, abiSha256: undefined },
    unnamed, { ...profile, token: null }, { ...profile, token: {} }, { ...profile, token: { ...won, name: "" } },
    { ...profile, token: { ...won, label: "t KRW" } }, { ...profile, token: { ...won, decimals: 19 } },
    { ...profile, token: { ...won, decimals: 1.5 } }, { ...profile, token: { ...won, extra: 1 } },
  ]) assert.throws(() => read(bad), /PAYMENT_PROFILE/, JSON.stringify(bad));
});

test("a dollar store reads as before", () => {
  const { token: _token, abiSha256: _abi, ...plain } = profile;
  for (const abiSha256 of [undefined, settlementAbiSha256, orderEscrowAbiSha256])
    assert.deepEqual(paymentToken(read({ ...plain, abiSha256 })), dollarToken);
  const dollar = { ...plain, abiSha256: orderEscrowAbiSha256 };
  assert.equal(commerceAmount("6500000", plain.asset, dollar), "6.50 TestUSDC");
  assert.equal(commerceAmount("6500001", plain.asset, dollar), "6.500001 TestUSDC");
  assert.equal(commerceAmount("0", plain.asset, dollar), "0.00 TestUSDC");
});

test("won amounts are exact", () => {
  const show = (units: string) => commerceAmount(units, profile.asset, profile);
  assert.equal(show("6500000"), "65,000원");
  assert.equal(show("1250000"), "12,500원");
  assert.equal(show("30000000"), "300,000원");
  assert.equal(show("123456789012"), "1,234,567,890.12원");
  assert.equal(show("5"), "0.05원");
  assert.equal(show("50"), "0.50원");
  assert.equal(show("0"), "0원");
  assert.equal(commerceAmount("1250000", profile.asset, profile, "en"), "₩12,500");
  assert.equal(tokenAmount(-1250050n, won), "-12,500.50원");
  assert.equal(tokenAmount(-500000n, dollarToken), "-0.50 TestUSDC");
  assert.equal(commerceAmount("1250000", "0x" + "9".repeat(40), profile), "1250000 기준 단위");
});

test("the server and the apps agree on the default token", () => {
  const go = readFileSync(new URL("../../../../products/p05-operations-backoffice/internal/commerce/token.go", import.meta.url), "utf8");
  assert.match(go, /Symbol: "tUSDC", Label: "TestUSDC", Decimals: 6/);
});
