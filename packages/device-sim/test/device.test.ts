// The simulated device through the full wire (CBOR, envelope, fragments): the payment flow of
// payment-protocol.md 6 and each refusal of its step-4 checks.
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  addressOfPrivateKey,
  bytesToHex,
  digest,
  hexToBytes,
  recoverSigner,
  signDigest,
  fragments,
  envelope,
  encodeMessage,
  type Message,
} from "@nu54/protocol";
import { connect, SoftwareDevice } from "../src/index.ts";

// Foundry public test mnemonic: 0 device, 1 operator, 2 merchant (test-only keys).
const KEY = {
  device: hexToBytes("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"),
  operator: hexToBytes("0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"),
  merchant: hexToBytes("0x5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a"),
  stranger: hexToBytes("0x7c852118294e51e653712a81e05800f419141751be58f605c371e15141b007a6"),
};
const OPERATOR = addressOfPrivateKey(KEY.operator);
const MERCHANT = addressOfPrivateKey(KEY.merchant);
const CONTRACT = "0xc0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0";
const TOKEN = "0xd1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1";
const PAYOUT = "0xb1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1";
const domain = { chainId: 8283, verifyingContract: CONTRACT };
const SID = "0102030405060708";
const T0 = 1_790_000_000;

function setup(opts: { approve?: boolean; nonceStart?: bigint } = {}) {
  let now = T0;
  const device = new SoftwareDevice({
    key: KEY.device,
    operator: OPERATOR,
    contract: CONTRACT,
    chainId: 8283,
    nonceStart: opts.nonceStart ?? 256n * 7n,
    now: () => now,
    approve: () => opts.approve ?? true,
  });
  const link = connect(device, 23); // smallest MTU: every message is split into many fragments
  return { device, link, advance: (s: number) => (now += s) };
}

const sign = (key: Uint8Array, type: Parameters<typeof digest>[1], v: Record<string, string | bigint | number>) =>
  bytesToHex(signDigest(digest(domain, type, v), key));

function anchor(link: ReturnType<typeof setup>["link"], device: SoftwareDevice, ts: number, key = KEY.operator) {
  link.send({ v: 1, type: "session.open", sessionId: SID, mode: "setup", kioskNonce: "0x" + "11".repeat(32) } as Message);
  return link.send({
    v: 1, type: "setup.timeAnchor", sessionId: SID, device: device.address, timestamp: String(ts),
    operatorSignature: sign(key, "TimeAnchor", { device: device.address, timestamp: BigInt(ts) }),
  } as Message)[0];
}

function attestation(over: Partial<Record<string, string>> = {}, key = KEY.operator) {
  const a = { merchant: MERCHANT, payout: PAYOUT, name: "Cafe Test 01", validFrom: String(T0), validUntil: String(T0 + 86400), ...over };
  return { ...a, operatorSignature: sign(key, "MerchantAttestation", a) };
}

function order(over: Partial<Record<string, string>> = {}) {
  return {
    chainId: "8283", contract: CONTRACT, merchant: MERCHANT, payout: PAYOUT, token: TOKEN,
    amount: "4500000", orderId: "0x" + "01".repeat(32), expiry: String(T0 + 60), ...over,
  };
}

function pay(link: ReturnType<typeof setup>["link"], att: object, auth: Record<string, string>, merchantKey = KEY.merchant) {
  const ok = link.send({ v: 1, type: "session.open", sessionId: SID, mode: "payment", kioskNonce: "0x" + "a5".repeat(32) } as Message)[0];
  link.send({ v: 1, type: "session.confirm", sessionId: SID, deviceNonce: ok.deviceNonce } as Message);
  const identify = link.send({ v: 1, type: "payment.identify", sessionId: SID, attestation: att } as Message);
  if (identify.length) return identify[0];
  const { orderId, token, amount, payout, expiry } = auth;
  const merchantSignature = sign(merchantKey, "MerchantOrder", { orderId, token, amount, payout, expiry });
  return link.send({ v: 1, type: "payment.prepare", sessionId: SID, authorization: auth, merchantSignature } as Message)[0];
}

test("payment flow: the signature recovers to the device and the nonce is sequential", () => {
  const { device, link } = setup();
  const ack = anchor(link, device, T0);
  assert.equal(ack.accepted, true);
  assert.equal(device.state, "READY");
  for (const n of [1792n, 1793n]) {
    const auth = order({ orderId: "0x" + n.toString(16).padStart(64, "0") });
    const result = pay(link, attestation(), auth);
    assert.equal(result.outcome, "approved", JSON.stringify(result));
    assert.equal(result.nonce, n.toString());
    const signer = recoverSigner(digest(domain, "PaymentAuthorization", { ...auth, nonce: n }), hexToBytes(String(result.signature)));
    assert.equal(signer, device.address);
  }
  assert.equal(device.phone.length, 2);
  assert.equal(device.phone[0].merchantName, "Cafe Test 01");
});

test("step-4 checks refuse with the protocol reasons", () => {
  const run = (name: string, f: (s: ReturnType<typeof setup>) => Message, reason: string) => {
    const s = setup();
    anchor(s.link, s.device, T0);
    const r = f(s);
    assert.equal(r.reason, reason, `${name}: ${JSON.stringify(r)}`);
  };
  run("attestation not signed by the operator", (s) => pay(s.link, attestation({}, KEY.stranger), order()), "MERCHANT_FORGED");
  run("attestation expired", (s) => (s.advance(86400 + 61), pay(s.link, attestation(), order({ expiry: String(T0 + 86400 + 61 + 30) }))), "ATTESTATION_EXPIRED");
  run("order not signed by the attested merchant", (s) => pay(s.link, attestation(), order(), KEY.stranger), "MERCHANT_FORGED");
  run("payout differs from the attestation", (s) => pay(s.link, attestation(), order({ payout: "0x" + "b2".repeat(20) })), "MERCHANT_FORGED");
  run("other merchant in the authorization", (s) => pay(s.link, attestation(), order({ merchant: OPERATOR })), "MERCHANT_FORGED");
  run("other contract than the setup", (s) => pay(s.link, attestation(), order({ contract: "0x" + "c1".repeat(20) })), "MERCHANT_FORGED");
  run("other chain than the setup", (s) => pay(s.link, attestation(), order({ chainId: "1" })), "MERCHANT_FORGED");
  run("expiry beyond authorizationExpiry", (s) => pay(s.link, attestation(), order({ expiry: String(T0 + 121) })), "ATTESTATION_EXPIRED");
  run("renter presses reject", () => {
    const s = setup({ approve: false });
    anchor(s.link, s.device, T0);
    return pay(s.link, attestation(), order());
  }, "USER_REJECTED");
});

test("without an anchor, and after a power cycle, payments are TIME_ANCHOR_MISSING", () => {
  const { device, link } = setup();
  assert.equal(pay(link, attestation(), order()).reason, "TIME_ANCHOR_MISSING");
  anchor(link, device, T0);
  device.powerCycle();
  assert.equal(device.state, "PROVISIONED_NO_ANCHOR");
  assert.equal(pay(link, attestation(), order()).reason, "TIME_ANCHOR_MISSING");
  assert.equal(anchor(link, device, T0).accepted, false, "re-anchor must be strictly later");
  assert.equal(anchor(link, device, T0 + 1).accepted, true);
  assert.equal(pay(link, attestation(), order()).outcome, "approved");
});

test("a TimeAnchor not signed by the operator is refused", () => {
  const { device, link } = setup();
  assert.equal(anchor(link, device, T0, KEY.stranger).accepted, false);
  assert.equal(device.state, "PROVISIONED_NO_ANCHOR");
});

test("session rules: confirm with the device nonce, setup only before READY, unsupported types", () => {
  const { device, link } = setup();
  anchor(link, device, T0);
  const ok = link.send({ v: 1, type: "session.open", sessionId: SID, mode: "payment", kioskNonce: "0x" + "a5".repeat(32) } as Message)[0];
  assert.equal(ok.state, "READY");
  assert.equal(ok.device, device.address);
  const wrong = link.send({ v: 1, type: "session.confirm", sessionId: SID, deviceNonce: "0x" + "00".repeat(32) } as Message);
  assert.equal(wrong[0].reason, "NOT_PERMITTED");
  const setupAgain = link.send({ v: 1, type: "session.open", sessionId: SID, mode: "setup", kioskNonce: "0x" + "11".repeat(32) } as Message);
  assert.equal(setupAgain[0].reason, "NOT_PERMITTED");
  // A message the device never receives (a device-to-kiosk type) is UNSUPPORTED_TYPE.
  const unsupported = link.send({ v: 1, type: "payment.result", sessionId: SID, outcome: "approved" } as Message);
  assert.equal(unsupported[0].reason, "UNSUPPORTED_TYPE");
  // Setup steps the simulator does not model yet are NOT_PERMITTED.
  const operator = link.send({ v: 1, type: "setup.operator", sessionId: SID, operator: OPERATOR, contract: CONTRACT, chainId: "8283" } as Message);
  assert.equal(operator[0].reason, "NOT_PERMITTED");
});

test("a corrupted frame is answered with BAD_FRAME and closes the session", () => {
  const { device, link } = setup();
  anchor(link, device, T0);
  const body = encodeMessage({ v: 1, type: "session.cancel", sessionId: SID } as Message);
  const env = envelope(body);
  env[2] ^= 0xff; // digest no longer matches
  const replies = link.sendFragments(fragments(env, 9, 23));
  assert.equal(replies[0].type, "error");
  assert.equal(replies[0].reason, "BAD_FRAME");
});
