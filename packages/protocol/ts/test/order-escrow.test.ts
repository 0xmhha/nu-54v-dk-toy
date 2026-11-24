import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { keccak_256 } from "@noble/hashes/sha3";
import { bytesToHex, hexToBytes } from "../src/bytes.ts";
import { addressOfPrivateKey, recoverSigner } from "../src/signature.ts";
import { orderEscrowAbiSha256, paymentProfile, paysThroughEscrow, settlementAbiSha256 } from "../src/commerce-payment.ts";
import { decodeNearbyRequest } from "../src/nearby-commerce.ts";
import {
  accountProofDigest, payerEnrollChallenge, validPayerEnrollment, validWalletReturn, walletReturnDigest, orderBoardRequest, orderBuyerActions, orderBuyerDigest, orderLock, orderMerchantDigest,
  signOrderBuyerAction, validOrderEscrowStatus, verifyOrderLock, type OrderEscrowStatus,
} from "../src/order-escrow.ts";

const vectors = (name: string) => Object.fromEntries(
  readFileSync(new URL(`../../v2/testdata/${name}`, import.meta.url), "utf8").trim().split("\n").map((l) => l.split("=")),
) as Record<string, string>;
const lock = vectors("order-lock.vectors");
const actions = vectors("order-actions.vectors");
const unsigned = hexToBytes(lock.unsigned);
const signed = hexToBytes(lock.unsigned + lock.merchantSignature.slice(2));
const payer = "0x" + lock.unsigned.slice(2 + 288 * 2 + 24, 2 + 320 * 2);
const order = { escrow: "0x0000000000000000000000000000000000001002", merchantId: "7".padStart(64, "0"), orderKey: "5".padStart(64, "0") };

test("order lock matches the board and the contract", () => {
  const built = orderLock(unsigned, lock.confirmer);
  assert.equal(bytesToHex(built.preimage), lock.preimage);
  const signature = hexToBytes(lock.payerSignature);
  assert.equal(recoverSigner(hexToBytes(built.digest), signature), payer);
  const payment = { bytes: signed, payer, deadline: 1_800_000_110 };
  assert.equal(verifyOrderLock(payment, lock.confirmer, signature, 1_800_000_000), lock.payerSignature.slice(2));
  for (const [confirmer, now] of [["0x0000000000000000000000000000000000001005", 1_800_000_000], [lock.confirmer, 1_800_000_110]] as const)
    assert.throws(() => verifyOrderLock(payment, confirmer, signature, now), /ORDER_LOCK_MISMATCH/);
  assert.throws(() => orderLock(unsigned, "0x" + "0".repeat(40)), /ORDER_ADDRESS/);
  const body = orderBoardRequest(lock.confirmer, signed);
  assert.equal(body.length, 4 + 20 + 737);
  assert.equal(bytesToHex(body.subarray(0, 24), false), "4e35344f" + lock.confirmer.slice(2));
});

test("store and phone actions match the contract", () => {
  for (const action of ["delivered", "cancel"] as const)
    assert.equal(bytesToHex(orderMerchantDigest(action, order)), actions[action]);
  for (const action of ["receipt", "dispute", "withdraw"] as const)
    assert.equal(bytesToHex(orderBuyerDigest(action, { ...order, claimableAt: 1_800_172_800 })), actions[action]);
  assert.throws(() => orderBuyerDigest("receipt", { ...order, claimableAt: 0 }), /ORDER_NOT_DELIVERED/);
  const key = new Uint8Array(32).fill(7);
  const at = { ...order, claimableAt: 1_800_172_800 };
  assert.equal(recoverSigner(orderBuyerDigest("receipt", at), hexToBytes(signOrderBuyerAction("receipt", at, key))), addressOfPrivateKey(key));
});

test("account proof digest matches the board", () => {
  // Pinned against products/p01-device-firmware/test/test_order.c.
  assert.equal(bytesToHex(accountProofDigest(payer, hexToBytes("11".repeat(32))), false), "6ac8be6e20f7c3971e894cfd18e30676ef1032d039b624d7a244cf7742b490c8");
  assert.throws(() => accountProofDigest(payer, new Uint8Array(32)), /ACCOUNT_CHALLENGE/);
});

const locked: OrderEscrowStatus = {
  stage: "locked", ...order, payer, confirmer: lock.confirmer, lockTxHash: "0x" + "1".repeat(64), deliverBy: 1_800_604_800,
};
const delivered: OrderEscrowStatus = { ...locked, stage: "delivered", claimableAt: 1_800_172_800 };
const tx = "0x" + "2".repeat(64);

test("order status accepts only a possible state", () => {
  const { lockTxHash, deliverBy, ...received } = { ...locked, stage: "received" as const };
  for (const good of [
    received, { ...received, stage: "submitted", lockTxHash }, { ...received, stage: "failed" }, locked, delivered,
    { ...delivered, stage: "disputed", resolveBy: 1_801_000_000 },
    { ...delivered, stage: "disputed", resolveBy: 1_801_000_000, pendingAction: "withdraw" },
    { ...delivered, pendingAction: "receipt" },
    { ...delivered, stage: "paid", payout: "claim", settledTxHash: tx },
    { ...delivered, stage: "paid", payout: "arbiter", settledTxHash: tx, resolveBy: 1_801_000_000 },
    { ...locked, stage: "refunded", refund: "undelivered", settledTxHash: tx },
  ]) assert.ok(validOrderEscrowStatus(good), JSON.stringify(good));
  for (const bad of [
    null, {}, { ...received, lockTxHash }, { ...received, stage: "submitted" }, { ...locked, deliverBy: undefined },
    { ...locked, claimableAt: 1 }, { ...delivered, claimableAt: undefined }, { ...delivered, payout: "claim" },
    { ...delivered, stage: "disputed" }, { ...delivered, stage: "paid", payout: "claim" },
    { ...delivered, stage: "paid", payout: "arbitrary", settledTxHash: tx }, { ...locked, pendingAction: "receipt" },
    { ...locked, stage: "refunded", settledTxHash: tx }, { ...locked, extra: 1 }, { ...locked, payer: "0x" + "0".repeat(40) },
    { ...delivered, pendingAction: "withdraw" },
  ]) assert.ok(!validOrderEscrowStatus(bad), JSON.stringify(bad));
});

test("the phone offers only what the contract accepts", () => {
  assert.deepEqual(orderBuyerActions(locked, 1), []);
  assert.deepEqual(orderBuyerActions(delivered, 1_800_172_799), ["receipt", "dispute"]);
  assert.deepEqual(orderBuyerActions(delivered, 1_800_172_800), ["receipt"]);
  assert.deepEqual(orderBuyerActions({ ...delivered, pendingAction: "receipt" }, 1), []);
  assert.deepEqual(orderBuyerActions({ ...delivered, stage: "disputed", resolveBy: 1_801_000_000 }, 1), ["withdraw"]);
});

test("nearby lock carries exactly the signed order", () => {
  const branch = "987e50a6-a874-487c-8d85-1451edacfe99";
  const good = {
    op: "lock", branch, key: "23a6f801-e925-42b9-b4fa-05f5d4f11cee", order: "03f43389-3f6b-4bc4-a1d7-6cb11af65ccb",
    quoteDigest: "a".repeat(64), confirmer: lock.confirmer, lockDigest: "b".repeat(64), signature: "c".repeat(128) + "1b",
  };
  const decode = (value: object) => decodeNearbyRequest(new TextEncoder().encode(JSON.stringify(value)), branch);
  assert.deepEqual(decode(good), good);
  for (const bad of [{ ...good, confirmer: "0x" + "0".repeat(40) }, { ...good, signature: "c".repeat(130) }, { ...good, extra: 1 }, { ...good, lockDigest: "B".repeat(64) }])
    assert.throws(() => decode(bad), /NEARBY_COMMAND/);
});

test("a store profile names a known contract", () => {
  const manifest = JSON.parse(readFileSync(new URL("../../../contracts-abi/abi/manifest.json", import.meta.url), "utf8"));
  assert.equal(manifest.abis.OrderEscrow.sha256, orderEscrowAbiSha256);
  assert.equal(manifest.abis.SettlementV2.sha256, settlementAbiSha256);
  assert.equal(paysThroughEscrow({}), false);
  assert.equal(paysThroughEscrow({ abiSha256: settlementAbiSha256 }), false);
  assert.equal(paysThroughEscrow({ abiSha256: orderEscrowAbiSha256 }), true);
  assert.throws(() => paysThroughEscrow({ abiSha256: "f".repeat(64) }), /PAYMENT_PROFILE/);
});

test("a store profile may name the server the app reads the chain from", () => {
  const address = "0x" + "a".repeat(40);
  const payment = { chain: 8283, contract: address, asset: address, beneficiary: address, signer: address, merchantId: "7".repeat(64),
    keyVersion: 1, beneficiaryVersion: 1, maxBaseUnits: "30000000" };
  const read = (rpc?: unknown) => paymentProfile(JSON.stringify({ payment: { ...payment, rpc } }));
  assert.equal(read()?.rpc, undefined);
  assert.equal(read("https://api.test.stablenet.network/")?.rpc, "https://api.test.stablenet.network/");
  for (const bad of ["http://rpc.example", "https://user@rpc.example", "https://rpc.example/?key=1", "https://rpc.example/#a", "", 7, "https://" + "a".repeat(512)])
    assert.throws(() => read(bad), /PAYMENT_PROFILE/);
});

test("board return and enrollment match the board and the server", () => {
  const v = vectors("wallet-return.vectors");
  const digest = walletReturnDigest({ token: v.token, from: v.from, to: v.to, amount: 9_500_000n, validBefore: 1_800_000_500n, nonce: v.nonce.slice(2) });
  assert.equal(bytesToHex(digest), bytesToHex(keccak_256(hexToBytes(v.preimage))));
  assert.equal(bytesToHex(payerEnrollChallenge({ tenantRef: "t", branchRef: "b", subjectRef: "s",
    payer: "0x3004df73ebac3f1a2d5a0ea01ed39503139b9dcb", generation: 3n, idempotencyKey: "k" }), false), "abdcac8f60735d1edf9ef7ae849eec2a48aa839705e68386170e276043388427");
  assert.ok(validPayerEnrollment({ payer, generation: "1", expiresAt: 1 }));
  assert.ok(!validPayerEnrollment({ payer, generation: "0", expiresAt: 1 }));
  const back = { payer, to: lock.confirmer, amount: "5", validBefore: 9, state: "queued" };
  assert.ok(validWalletReturn(back) && validWalletReturn({ ...back, state: "final", txHash: tx }));
  assert.ok(!validWalletReturn({ ...back, state: "final" }) && !validWalletReturn({ ...back, amount: "0" }));
});
