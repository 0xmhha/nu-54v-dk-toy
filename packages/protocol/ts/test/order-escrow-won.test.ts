import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { keccak_256 } from "@noble/hashes/sha3";
import { bytesToHex, hexToBytes } from "../src/bytes.ts";
import { recoverSigner } from "../src/signature.ts";
import {
  orderLockFor, orderTokenName, validTokenName, verifyOrderLock, verifyOrderLockFor, walletReturnDigestFor,
} from "../src/order-escrow.ts";

const won = "NU54 Test KRW";
const vectors = (name: string) => Object.fromEntries(
  readFileSync(new URL(`../../v2/testdata/${name}`, import.meta.url), "utf8").trim().split("\n").map((l) => l.split("=")),
) as Record<string, string>;
const lock = vectors("order-lock-won.vectors");
const unsigned = hexToBytes(lock.unsigned);
const payment = {
  bytes: hexToBytes(lock.unsigned + lock.merchantSignature.slice(2)),
  payer: "0x" + lock.unsigned.slice(2 + 288 * 2 + 24, 2 + 320 * 2),
  deadline: 1_800_000_110,
};

test("won order lock matches the contract and no other name", () => {
  const built = orderLockFor(won, unsigned, lock.confirmer);
  assert.equal(bytesToHex(built.preimage), lock.preimage);
  const signature = hexToBytes(lock.payerSignature);
  assert.equal(recoverSigner(hexToBytes(built.digest), signature), payment.payer);
  assert.equal(verifyOrderLockFor(won, payment, lock.confirmer, signature, 1_800_000_000), lock.payerSignature.slice(2));
  // The quote and the token address are the same; only the name differs.
  for (const name of [orderTokenName, "NU54 Test KRWC"])
    assert.throws(() => verifyOrderLockFor(name, payment, lock.confirmer, signature, 1_800_000_000), /ORDER_LOCK_MISMATCH/);
  assert.throws(() => verifyOrderLock(payment, lock.confirmer, signature, 1_800_000_000), /ORDER_LOCK_MISMATCH/);
});

test("won return matches the contract", () => {
  const v = vectors("wallet-return-won.vectors");
  const digest = walletReturnDigestFor(won, {
    token: v.token, from: v.from, to: v.to, amount: 3_420_000n, validBefore: 1_800_000_500n, nonce: v.nonce.slice(2),
  });
  assert.equal(bytesToHex(digest), bytesToHex(keccak_256(hexToBytes(v.preimage))));
});

test("the token name is checked", () => {
  for (const name of ["a", won, "a".repeat(64), "a!~z"]) assert.ok(validTokenName(name), name);
  for (const name of ["", " NU54", "NU54 ", "a\nb", "\uc6d0", "a".repeat(65), "a\x7f", "\x7fa", "a\x1fb"]) {
    assert.ok(!validTokenName(name), JSON.stringify(name));
    assert.throws(() => orderLockFor(name, unsigned, lock.confirmer), /ORDER_TOKEN_NAME/);
    assert.throws(() => walletReturnDigestFor(name, { token: lock.confirmer, from: payment.payer, to: lock.confirmer, amount: 1n, validBefore: 1n, nonce: "1".repeat(64) }),
      /ORDER_TOKEN_NAME/);
  }
});
