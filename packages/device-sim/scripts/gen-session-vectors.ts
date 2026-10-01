// Generates the shared session vectors from the software device (payment-protocol.md 5, 6).
//
// Each scenario is a sequence of steps: the CBOR body a central sends, the CBOR bodies the
// device answers, and the confirm.show bodies it sends to the phone app. Every implementation
// of the device (the firmware first) must answer each step with exactly these bytes. Inputs are
// fixed so the bytes are reproducible: public test keys, chain 8283, placeholder contract,
// token and payout, a clock set per step, and a random source whose k-th request (k = 0, 1, ...)
// returns bytes all equal to 0x5a + k. Device signatures are RFC 6979, so they are fixed too.
//
// Usage:
//   node --experimental-strip-types scripts/gen-session-vectors.ts           write the file
//   node --experimental-strip-types scripts/gen-session-vectors.ts --check   fail if it is stale

import { readFileSync, writeFileSync } from "node:fs";
import { bytesToHex, digest, encodeMessage, hexToBytes, signDigest, addressOfPrivateKey, type Message } from "@nu54/protocol";
import { SoftwareDevice } from "../src/device.ts";

const OUT = new URL("../../../docs/content/specifications/protocol/session-vectors.json", import.meta.url);

// Foundry public test mnemonic: 0 device, 1 operator, 2 merchant; and a key with no role.
const KEY = {
  device: "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80",
  operator: "0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d",
  merchant: "0x5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a",
  stranger: "0x7c852118294e51e653712a81e05800f419141751be58f605c371e15141b007a6",
};
const k = (name: keyof typeof KEY) => hexToBytes(KEY[name]);
const CONTRACT = "0xc0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0";
const TOKEN = "0xd1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1";
const PAYOUT = "0xb1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1";
const CHAIN_ID = 8283;
const T0 = 1_790_000_000;
const NONCE_START = 256n * 7n;
const SID = "0102030405060708";
const domain = { chainId: CHAIN_ID, verifyingContract: CONTRACT };
const sign = (key: keyof typeof KEY, type: Parameters<typeof digest>[1], v: Record<string, string | bigint | number>) =>
  bytesToHex(signDigest(digest(domain, type, v), k(key)));
const DEVICE = addressOfPrivateKey(k("device"));
const MERCHANT = addressOfPrivateKey(k("merchant"));

type Step =
  | { at: number; send: Message }
  | { at: number; powerCycle: true };

const open = (mode: "setup" | "payment"): Message => ({ v: 1, type: "session.open", sessionId: SID, mode, kioskNonce: "0x" + "a5".repeat(32) } as Message);
const confirm = (deviceNonce: string): Message => ({ v: 1, type: "session.confirm", sessionId: SID, deviceNonce } as Message);
const anchorMsg = (ts: number, key: keyof typeof KEY = "operator"): Message =>
  ({ v: 1, type: "setup.timeAnchor", sessionId: SID, device: DEVICE, timestamp: String(ts), operatorSignature: sign(key, "TimeAnchor", { device: DEVICE, timestamp: BigInt(ts) }) } as Message);
function attestation(over: Record<string, string> = {}, key: keyof typeof KEY = "operator") {
  const a = { merchant: MERCHANT, payout: PAYOUT, name: "Cafe Test 01", validFrom: String(T0), validUntil: String(T0 + 86400), ...over };
  return { ...a, operatorSignature: sign(key, "MerchantAttestation", a) };
}
const identify = (att = attestation()): Message => ({ v: 1, type: "payment.identify", sessionId: SID, attestation: att } as Message);
function prepare(over: Record<string, string> = {}, key: keyof typeof KEY = "merchant"): Message {
  const auth = { chainId: String(CHAIN_ID), contract: CONTRACT, merchant: MERCHANT, payout: PAYOUT, token: TOKEN, amount: "4500000", orderId: "0x" + "01".repeat(32), expiry: String(T0 + 60), ...over };
  const { orderId, token, amount, payout, expiry } = auth;
  return { v: 1, type: "payment.prepare", sessionId: SID, authorization: auth, merchantSignature: sign(key, "MerchantOrder", { orderId, token, amount, payout, expiry }) } as Message;
}
// The k-th deviceNonce the device hands out is 32 bytes of 0x5a + k.
const deviceNonce = (k: number) => "0x" + (0x5a + k).toString(16).repeat(32);

/** setup session + accepted anchor at T0: the device answers two messages, nonce 0 used. */
const anchored = (): Step[] => [
  { at: T0, send: open("setup") },
  { at: T0, send: anchorMsg(T0) },
];
/** A payment session after `opens` earlier session.open calls. */
const paySession = (opens: number, at: number, ...rest: Message[]): Step[] => [
  { at, send: open("payment") },
  { at, send: confirm(deviceNonce(opens)) },
  ...rest.map((send) => ({ at, send })),
];

const SCENARIOS: { id: string; description: string; button: "approve" | "reject"; steps: Step[] }[] = [
  { id: "SV-01", description: "anchor, two approved payments with sequential nonces, payment.outcome", button: "approve",
    steps: [...anchored(), ...paySession(1, T0 + 5, identify(), prepare()),
      ...paySession(2, T0 + 10, identify(), prepare({ orderId: "0x" + "02".repeat(32), expiry: String(T0 + 70) }),
        { v: 1, type: "payment.outcome", sessionId: SID, orderId: "0x" + "02".repeat(32), outcome: "approved" } as Message)] },
  { id: "SV-02", description: "no anchor: the device reports it and refuses with TIME_ANCHOR_MISSING", button: "approve",
    steps: paySession(0, T0, identify()) },
  { id: "SV-03", description: "attestation not signed by the operator: MERCHANT_FORGED", button: "approve",
    steps: [...anchored(), ...paySession(1, T0, identify(attestation({}, "stranger")))] },
  { id: "SV-04", description: "order not signed by the attested merchant: MERCHANT_FORGED", button: "approve",
    steps: [...anchored(), ...paySession(1, T0, identify(), prepare({}, "stranger"))] },
  { id: "SV-05", description: "payout differs from the attestation: MERCHANT_FORGED", button: "approve",
    steps: [...anchored(), ...paySession(1, T0, identify(), prepare({ payout: "0x" + "b2".repeat(20) }))] },
  { id: "SV-06", description: "expiry beyond authorizationExpiry: ATTESTATION_EXPIRED", button: "approve",
    steps: [...anchored(), ...paySession(1, T0, identify(), prepare({ expiry: String(T0 + 121) }))] },
  { id: "SV-07", description: "attestation outside its validity plus anchorClockSkew: ATTESTATION_EXPIRED", button: "approve",
    steps: [...anchored(), ...paySession(1, T0 + 86400 + 61, identify())] },
  { id: "SV-08", description: "the renter presses reject: USER_REJECTED, confirm.show still sent", button: "reject",
    steps: [...anchored(), ...paySession(1, T0, identify(), prepare())] },
  { id: "SV-09", description: "session.confirm with another deviceNonce: NOT_PERMITTED", button: "approve",
    steps: [...anchored(), { at: T0, send: open("payment") }, { at: T0, send: confirm("0x" + "00".repeat(32)) }] },
  { id: "SV-10", description: "setup session while READY: NOT_PERMITTED", button: "approve",
    steps: [...anchored(), { at: T0, send: open("setup") }] },
  { id: "SV-11", description: "anchors not later than the last one or not signed by the operator are refused", button: "approve",
    steps: [...anchored(), { at: T0, powerCycle: true }, { at: T0, send: open("setup") }, { at: T0, send: anchorMsg(T0) },
      { at: T0, send: anchorMsg(T0 + 1, "stranger") }, { at: T0, send: anchorMsg(T0 + 1) }] },
  { id: "SV-12", description: "a device-to-kiosk type sent to the device: UNSUPPORTED_TYPE", button: "approve",
    steps: [...anchored(), { at: T0, send: { v: 1, type: "payment.result", sessionId: SID, outcome: "approved" } as Message }] },
];

function run() {
  return SCENARIOS.map((sc) => {
    let now = 0;
    let draws = 0;
    const device = new SoftwareDevice({
      key: k("device"), operator: addressOfPrivateKey(k("operator")), contract: CONTRACT, chainId: CHAIN_ID,
      nonceStart: NONCE_START, now: () => now, approve: () => sc.button === "approve",
      random: (n) => new Uint8Array(n).fill(0x5a + draws++),
    });
    const steps = sc.steps.map((st) => {
      now = st.at;
      if ("powerCycle" in st) {
        device.powerCycle();
        return { at: st.at, powerCycle: true };
      }
      const phoneBefore = device.phone.length;
      const replies = device.handle(st.send);
      return {
        at: st.at,
        sendType: st.send.type,
        send: bytesToHex(encodeMessage(st.send), false),
        expect: replies.map((r) => bytesToHex(encodeMessage(r), false)),
        phone: device.phone.slice(phoneBefore).map((p) => bytesToHex(encodeMessage(p), false)),
      };
    });
    return { id: sc.id, description: sc.description, button: sc.button, steps };
  });
}

const doc = {
  description:
    "Shared session vectors (payment-protocol.md 5, 6), generated by packages/device-sim. A device implementation set up with the values below must answer every step's `send` CBOR body with exactly the `expect` bodies (in order) and send the `phone` bodies to the phone app. `at` is the device clock (unix seconds) for that step. `powerCycle` is a RAM-clearing reset. The k-th random request (k = 0, 1, ...) returns bytes all equal to 0x5a + k. Device signatures are deterministic (RFC 6979).",
  generator: "packages/device-sim/scripts/gen-session-vectors.ts",
  device: {
    privateKey: KEY.device,
    address: DEVICE,
    operator: addressOfPrivateKey(k("operator")),
    contract: CONTRACT,
    chainId: CHAIN_ID,
    nonceStart: NONCE_START.toString(),
    anchorClockSkew: 60,
    authorizationExpiry: 120,
    firmware: "sim-0.1.0",
    initialState: "PROVISIONED_NO_ANCHOR",
  },
  scenarios: run(),
};
const text = JSON.stringify(doc, null, 2) + "\n";
if (process.argv.includes("--check")) {
  if (readFileSync(OUT, "utf8") !== text) {
    console.error("FAIL session-vectors.json is stale; run scripts/gen-session-vectors.ts");
    process.exit(1);
  }
  console.log(`ok: ${doc.scenarios.length} session scenarios`);
} else {
  writeFileSync(OUT, text);
  console.log(`wrote session-vectors.json: ${doc.scenarios.length} scenarios`);
}
