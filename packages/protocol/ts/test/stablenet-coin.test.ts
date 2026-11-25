import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { keccak_256 } from "@noble/hashes/sha3";
import { bytesToHex, hexToBytes } from "../src/bytes.ts";
import { orderLockFor, walletReturnDigestFor } from "../src/order-escrow.ts";
import { recoverSigner } from "../src/signature.ts";
import {
  boardUnit, commerceAmount, dollarToken, fromBoard, isWon, namedTokenAbiSha256, paymentProfile, stableNetCoin, toBoard, tokenAmount, validPaymentToken,
  type CommercePaymentProfile,
} from "../src/commerce-payment.ts";

const won = { name: "NU54 Test KRW", symbol: "tKRW", label: "tKRW", decimals: 2 };
const coin = { name: "WKRC", symbol: "WKRC", label: "KRWC", decimals: 18, boardDecimals: 2 };
const wei = 10n ** 18n;
const profile: CommercePaymentProfile = {
  chain: 8283, contract: "0x" + "1".repeat(40), asset: stableNetCoin.asset, beneficiary: "0x" + "3".repeat(40),
  signer: "0x" + "4".repeat(40), merchantId: "5".repeat(64), keyVersion: 1, beneficiaryVersion: 1, maxBaseUnits: (300_000n * wei).toString(),
  abiSha256: namedTokenAbiSha256, token: coin,
};
const read = (payment: object) => paymentProfile(JSON.stringify({ payment }));

test("the coin is accepted only at its own address", () => {
  assert.equal(stableNetCoin.asset, "0x0000000000000000000000000000000000001000");
  assert.deepEqual(read(profile)?.token, coin);
  const { token: _token, abiSha256: _abi, ...plain } = profile;
  for (const bad of [
    { ...profile, asset: "0x" + "2".repeat(40) }, // another token that calls itself the coin
    { ...profile, token: won, maxBaseUnits: "30000000" }, { ...plain, maxBaseUnits: "30000000" }, // the coin as another token
    { ...profile, maxBaseUnits: (300_000n * wei + 1n).toString() }, { ...profile, maxBaseUnits: "0" }, { ...profile, maxBaseUnits: "9".repeat(33) },
    { ...profile, asset: "0x" + "2".repeat(40), token: won, maxBaseUnits: "30000001" },
  ]) assert.throws(() => read(bad), /PAYMENT_PROFILE/, JSON.stringify(bad));
});

test("a token with many decimals names the decimals its board counts in, and no other does", () => {
  assert.ok(validPaymentToken(coin) && validPaymentToken({ ...coin, boardDecimals: 8 }) && validPaymentToken({ ...won, decimals: 8 }));
  const { boardDecimals: _board, ...forgotten } = coin;
  for (const bad of [
    forgotten, { ...coin, boardDecimals: undefined }, { ...coin, boardDecimals: 1 }, { ...coin, boardDecimals: 9 }, { ...coin, boardDecimals: 2.5 },
    { ...coin, boardDecimals: "2" }, { ...won, boardDecimals: 2 }, { ...won, boardDecimals: undefined }, { ...won, decimals: 9 },
  ]) assert.ok(!validPaymentToken(bad), JSON.stringify(bad));
});

test("board amounts are converted exactly or refused", () => {
  assert.equal(boardUnit(coin), 10n ** 16n);
  assert.equal(fromBoard(30_000_000n, coin), 300_000n * wei);
  assert.equal(toBoard(6_500n * wei, coin), 650_000n);
  assert.equal(toBoard(0n, coin), 0n);
  for (const token of [won, dollarToken]) {
    assert.equal(boardUnit(token), 1n);
    assert.equal(toBoard(6_500_001n, token), 6_500_001n);
  }
  const most = (1n << 64n) - 1n;
  assert.equal(toBoard(fromBoard(most, coin), coin), most);
  assert.equal(toBoard(most, won), most);
  for (const [amount, token] of [[1n, coin], [10n ** 16n - 1n, coin], [10n ** 16n + 1n, coin], [-(10n ** 16n), coin], [(most + 1n) * 10n ** 16n, coin], [most + 1n, won], [-1n, won]] as const)
    assert.throws(() => toBoard(amount, token), /BOARD_AMOUNT/);
});

test("the coin is shown as won and never rounded up", () => {
  assert.ok(isWon(coin) && isWon(won) && !isWon(dollarToken));
  const show = (units: bigint) => commerceAmount(units.toString(), profile.asset, profile);
  assert.equal(show(6_500n * wei), "6,500원");
  assert.equal(show(6_500n * wei + 5n * 10n ** 17n), "6,500.50원");
  assert.equal(show(10n ** 16n), "0.01원");
  for (const rest of [1n, 10n ** 15n, 10n ** 16n - 1n]) assert.equal(show(6_500n * wei + rest), "6,500원");
  assert.equal(show(7_000n * wei - 1n), "6,999.99원");
  assert.equal(tokenAmount(-1n, coin), "0원");
  assert.equal(tokenAmount(-(6_500n * wei), coin), "-6,500원");
  assert.equal(commerceAmount((6_500n * wei).toString(), profile.asset, profile, "en"), "₩6,500");
});

test("the coin's signatures match the contract", () => {
  const vectors = (name: string) => Object.fromEntries(
    readFileSync(new URL(`../../v2/testdata/${name}`, import.meta.url), "utf8").trim().split("\n").map((l) => l.split("=")),
  ) as Record<string, string>;
  const lock = vectors("order-lock-coin.vectors"), built = orderLockFor(coin.name, hexToBytes(lock.unsigned), lock.confirmer);
  assert.equal(bytesToHex(built.preimage), lock.preimage);
  assert.equal(recoverSigner(hexToBytes(built.digest), hexToBytes(lock.payerSignature)), "0x" + lock.unsigned.slice(2 + 288 * 2 + 24, 2 + 320 * 2));
  const v = vectors("wallet-return-coin.vectors");
  const digest = walletReturnDigestFor(coin.name, { token: v.token, from: v.from, to: v.to, amount: fromBoard(3_420_000n, coin), validBefore: 1_800_000_500n, nonce: v.nonce.slice(2) });
  assert.equal(bytesToHex(digest), bytesToHex(keccak_256(hexToBytes(v.preimage))));
});
