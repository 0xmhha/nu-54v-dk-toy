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
  // setup.operator outside a setup session is NOT_PERMITTED.
  const operator = link.send({ v: 1, type: "setup.operator", sessionId: SID, operator: OPERATOR, contract: CONTRACT, chainId: "8283", passkey: "123456" } as Message);
  assert.equal(operator[0].reason, "NOT_PERMITTED");
});

// ------------------------------------------------------------------ rental setup (protocol 5)

function blank(opts: { confirm?: boolean; pin?: string | null } = {}) {
  let now = T0;
  const device = new SoftwareDevice({
    now: () => now, random: (n) => globalThis.crypto.getRandomValues(new Uint8Array(n)),
    confirmSetup: () => opts.confirm ?? true, enterPin: () => (opts.pin === undefined ? "2580" : opts.pin),
  });
  return { device, link: connect(device, 23), advance: (s: number) => (now += s) };
}
const openSetup = (link: ReturnType<typeof connect>) =>
  link.send({ v: 1, type: "session.open", sessionId: SID, mode: "setup", kioskNonce: "0x" + "11".repeat(32) } as Message)[0];
const operatorMsg = (passkey = "042195") =>
  ({ v: 1, type: "setup.operator", sessionId: SID, operator: OPERATOR, contract: CONTRACT, chainId: "8283", passkey } as Message);
const resetMsg = (device: string, key = KEY.operator, nonce = 1n) =>
  ({ v: 1, type: "device.reset", sessionId: SID, device, nonce: String(nonce), operatorSignature: sign(key, "DeviceReset", { device, nonce }) } as Message);

test("rental setup: operator values, key and PIN, anchor, then a payment from the new key", () => {
  const { device, link } = blank();
  const opened = openSetup(link);
  assert.equal(opened.state, "UNPROVISIONED");
  assert.equal(opened.device, undefined, "no key yet");
  const [opAck, keygen] = link.send(operatorMsg());
  assert.deepEqual([opAck.step, opAck.accepted], ["setup.operator", true]);
  assert.deepEqual([keygen.step, keygen.accepted], ["keygen", true]);
  assert.equal(keygen.device, device.address);
  assert.equal(device.state, "PROVISIONED_NO_ANCHOR");
  const ack = link.send({
    v: 1, type: "setup.timeAnchor", sessionId: SID, device: device.address, timestamp: String(T0),
    operatorSignature: sign(KEY.operator, "TimeAnchor", { device: device.address, timestamp: BigInt(T0) }),
  } as Message)[0];
  assert.equal(ack.accepted, true);
  const result = pay(link, attestation(), order());
  assert.equal(result.outcome, "approved", JSON.stringify(result));
  assert.equal(BigInt(String(result.nonce)) % 256n, 0n, "the first nonce is the start, a multiple of 256");
});

test("rental setup refusals store nothing", () => {
  const rejected = blank({ confirm: false });
  openSetup(rejected.link);
  const r = rejected.link.send(operatorMsg());
  assert.deepEqual([r.length, r[0].accepted, r[0].reason], [1, false, "USER_REJECTED"]);
  assert.equal(rejected.device.state, "UNPROVISIONED");

  const slow = blank({ pin: null });
  openSetup(slow.link);
  const [op, keygen] = slow.link.send(operatorMsg());
  assert.equal(op.accepted, true);
  assert.deepEqual([keygen.step, keygen.accepted, keygen.reason], ["keygen", false, "TIMEOUT"]);
  assert.equal(slow.device.state, "UNPROVISIONED");
  assert.equal(openSetup(slow.link).device, undefined);

  const big = blank();
  openSetup(big.link);
  assert.equal(big.link.send(operatorMsg("1000000"))[0].reason, "NOT_PERMITTED", "passkey has six digits");

  const twice = blank();
  openSetup(twice.link);
  twice.link.send(operatorMsg());
  openSetup(twice.link); // PROVISIONED_NO_ANCHOR still opens a setup session
  const again = twice.link.send(operatorMsg());
  assert.deepEqual([again[0].step, again[0].accepted, again[0].reason], ["setup.operator", false, "NOT_PERMITTED"]);
});

test("device.reset: only the recorded operator, only for this device; it wipes everything", () => {
  const { device, link } = setup();
  anchor(link, device, T0);
  link.send({ v: 1, type: "session.open", sessionId: SID, mode: "payment", kioskNonce: "0x" + "a5".repeat(32) } as Message);
  const stranger = link.send(resetMsg(device.address, KEY.stranger))[0];
  assert.deepEqual([stranger.step, stranger.accepted, stranger.reason], ["device.reset", false, "NOT_PERMITTED"]);
  const other = link.send(resetMsg(OPERATOR))[0];
  assert.equal(other.accepted, false, "signed for another device");
  const ok = link.send(resetMsg(device.address))[0];
  assert.deepEqual([ok.step, ok.accepted], ["device.reset", true]);
  assert.equal(device.state, "UNPROVISIONED");
  assert.throws(() => device.address, /UNPROVISIONED/);
  const identify = link.send({ v: 1, type: "payment.identify", sessionId: SID, attestation: attestation() } as Message)[0];
  assert.equal(identify.reason, "NOT_PERMITTED", "the session ended with the reset");
  const opened = openSetup(link);
  assert.deepEqual([opened.state, opened.device], ["UNPROVISIONED", undefined]);
  assert.equal(link.send(resetMsg(OPERATOR))[0].accepted, false, "nothing to reset");
});

test("async setup: the operator ack goes out on the press, before the PIN", async () => {
  let pinEntered: (pin: string) => void = () => {};
  const device = new SoftwareDevice({
    now: () => T0, confirmSetup: async () => true, enterPin: () => new Promise<string>((r) => (pinEntered = r)),
  });
  device.handle({ v: 1, type: "session.open", sessionId: SID, mode: "setup", kioskNonce: "0x" + "11".repeat(32) } as Message);
  const early: Message[] = [];
  const done = device.handleAsync(operatorMsg(), (m) => early.push(m));
  await new Promise((r) => setImmediate(r));
  assert.deepEqual(early.map((m) => m.step), ["setup.operator"], "acked before the PIN");
  pinEntered("1357");
  const [keygen] = await done;
  assert.equal(keygen.step, "keygen");
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

test("a real press: handleAsync waits for the button, handle refuses a promise", async () => {
  let press: (v: boolean) => void = () => {};
  let now = T0;
  const device = new SoftwareDevice({
    key: KEY.device, operator: OPERATOR, contract: CONTRACT, chainId: 8283, nonceStart: 256n * 7n,
    now: () => now, approve: () => new Promise<boolean>((r) => (press = r)),
  });
  const sync = connect(device, 23);
  anchor(sync, device, T0);
  const ok = sync.send({ v: 1, type: "session.open", sessionId: SID, mode: "payment", kioskNonce: "0x" + "a5".repeat(32) } as Message)[0];
  sync.send({ v: 1, type: "session.confirm", sessionId: SID, deviceNonce: ok.deviceNonce } as Message);
  sync.send({ v: 1, type: "payment.identify", sessionId: SID, attestation: attestation() } as Message);
  const auth = order();
  const { orderId, token, amount, payout, expiry } = auth;
  const prepare = { v: 1, type: "payment.prepare", sessionId: SID, authorization: auth,
    merchantSignature: sign(KEY.merchant, "MerchantOrder", { orderId, token, amount, payout, expiry }) } as Message;
  assert.throws(() => device.handle(prepare), /handleAsync/);

  const pending = device.handleAsync(prepare);
  let settled = false;
  pending.then(() => (settled = true));
  await new Promise((r) => setImmediate(r));
  assert.equal(settled, false, "the device answers only after the press");
  press(true);
  const [result] = await pending;
  assert.equal(result.outcome, "approved");

  // The kiosk gives up (session.cancel) before the press: nothing is signed.
  const auth2 = order({ orderId: "0x" + "02".repeat(32) });
  const again = device.handleAsync({ ...prepare, authorization: auth2,
    merchantSignature: sign(KEY.merchant, "MerchantOrder", { orderId: auth2.orderId, token, amount, payout, expiry }) } as Message);
  device.handle({ v: 1, type: "session.cancel", sessionId: SID } as Message);
  press(true);
  assert.deepEqual(await again, []);
  now += 1;
});

test("the device forwards the kiosk's payment.outcome to the phone app, for its own session only", () => {
  const { device, link } = setup();
  const seen: Message[] = [];
  device.onPhone = (m) => seen.push(m);
  anchor(link, device, T0);
  const auth = order();
  assert.equal(pay(link, attestation(), auth).outcome, "approved");
  const outcome = { v: 1, type: "payment.outcome", sessionId: SID, orderId: auth.orderId, outcome: "approved" } as Message;
  assert.deepEqual(link.send(outcome), [], "no reply to the kiosk");
  assert.deepEqual(seen.map((m) => m.type), ["confirm.show", "payment.outcome"]);
  assert.deepEqual(seen[1], outcome, "forwarded unchanged");
  link.send({ ...outcome, sessionId: "0909090909090909" } as Message);
  assert.equal(seen.length, 2, "an outcome for another session is not forwarded");
});

// ------------------------------------------------------------------ limit change (protocol 5)

test("limit change: the signature recovers to the device over LimitChange and shares the nonce counter", () => {
  const { device, link } = setup();
  anchor(link, device, T0);
  const ok = link.send({ v: 1, type: "session.open", sessionId: SID, mode: "payment", kioskNonce: "0x" + "a5".repeat(32) } as Message)[0];
  link.send({ v: 1, type: "session.confirm", sessionId: SID, deviceNonce: ok.deviceNonce } as Message);
  const change = { chainId: "8283", contract: CONTRACT, perPaymentLimit: "20000000", dailyLimit: "0", expiry: String(T0 + 60) };
  const [r] = link.send({ v: 1, type: "limit.change", sessionId: SID, change } as Message);
  assert.equal(r.outcome, "approved", JSON.stringify(r));
  const signer = recoverSigner(digest(domain, "LimitChange", { ...change, nonce: BigInt(String(r.nonce)) }), hexToBytes(String(r.signature)));
  assert.equal(signer, device.address);
  assert.equal(device.phone.at(-1)?.type, "confirm.limit");
  assert.equal(String(r.nonce), (256n * 7n).toString());
});

test("limit change refusals and the PIN lock", () => {
  let pin: string | null = "1111";
  let now = T0;
  const device = new SoftwareDevice({ key: KEY.device, operator: OPERATOR, contract: CONTRACT, chainId: 8283, now: () => now, enterPin: () => pin });
  const link = connect(device, 23);
  anchor(link, device, T0);
  const ok = link.send({ v: 1, type: "session.open", sessionId: SID, mode: "payment", kioskNonce: "0x" + "a5".repeat(32) } as Message)[0];
  link.send({ v: 1, type: "session.confirm", sessionId: SID, deviceNonce: ok.deviceNonce } as Message);
  const change = (over: Record<string, string> = {}) =>
    ({ v: 1, type: "limit.change", sessionId: SID, change: { chainId: "8283", contract: CONTRACT, perPaymentLimit: "1", dailyLimit: "1", expiry: String(T0 + 60), ...over } } as Message);
  pin = null;
  assert.equal(link.send(change())[0].reason, "TIMEOUT");
  pin = "2580";
  assert.equal(link.send(change({ chainId: "1" }))[0].reason, "NOT_PERMITTED");
  assert.equal(link.send(change({ expiry: String(T0 + 500) }))[0].reason, "ATTESTATION_EXPIRED");
  pin = "1111";
  const reasons = [1, 2, 3, 4, 5].map(() => link.send(change())[0].reason);
  assert.deepEqual(reasons, ["NOT_PERMITTED", "NOT_PERMITTED", "NOT_PERMITTED", "NOT_PERMITTED", "PIN_LOCKED"]);
  assert.equal(device.state, "PIN_LOCKED");
  pin = "2580";
  assert.equal(link.send(change())[0].reason, "PIN_LOCKED", "the right PIN no longer helps");
  device.powerCycle();
  assert.equal(device.state, "PIN_LOCKED", "a RAM-clearing reset keeps the lock");
  now += 1;
});
