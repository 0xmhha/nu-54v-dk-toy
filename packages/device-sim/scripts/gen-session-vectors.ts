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
import {
  addressOfPrivateKey,
  bytesToHex,
  decodeMessage,
  digest,
  encodeMessage,
  ephemeralKey,
  hexToBytes,
  SecureChannel,
  sessionKey,
  signDigest,
  type Message,
} from "@nu54/protocol";
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
  | { at: number; send: Message; tamper?: true } // tamper: one bit of the sent body flipped
  | { at: number; raw: Uint8Array } // a body outside the schema, as a refusal test sends it
  | { at: number; powerCycle: true };

/** Deterministic CBOR for a flat map of text keys to uint, text or bytes (RFC 8949 4.2.1). */
function rawMap(fields: Record<string, number | string | Uint8Array>): Uint8Array {
  const head = (major: number, n: number) => (n < 24 ? [(major << 5) | n] : n < 256 ? [(major << 5) | 24, n] : [(major << 5) | 25, n >> 8, n & 0xff]);
  const item = (v: number | string | Uint8Array): number[] =>
    typeof v === "number" ? head(0, v) : typeof v === "string" ? [...head(3, utf8(v).length), ...utf8(v)] : [...head(2, v.length), ...v];
  const utf8 = (t: string) => [...new TextEncoder().encode(t)];
  const entries = Object.entries(fields).map(([k, v]) => [item(k), item(v)] as const)
    .sort((a, b) => a[0].length - b[0].length || Buffer.compare(Buffer.from(a[0]), Buffer.from(b[0])));
  return Uint8Array.from([...head(5, entries.length), ...entries.flatMap(([k, v]) => [...k, ...v])]);
}
// Requests the device never signs (payment-protocol.md 2): a raw transaction and an ERC-2612 Permit.
const rawTx = () => rawMap({ v: 1, type: "sign.transaction", sessionId: hexToBytes(SID), tx: hexToBytes("0x02f86f82205b0180808094" + "dd".repeat(20) + "8080c0") });
const rawPermit = () => rawMap({ v: 1, type: "sign.permit", sessionId: hexToBytes(SID), spender: hexToBytes("0x" + "ee".repeat(20)), value: 1000000 });

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
// Secure channel (4.1): the kiosk's one-time key is 32 bytes of 0xe1.
const KIOSK_EPHEMERAL = new Uint8Array(32).fill(0xe1);
const KIOSK_NONCE = "0x" + "a5".repeat(32);
const STRANGER = addressOfPrivateKey(k("stranger"));
function secureOpen(o: { mode?: "setup" | "payment"; att?: ReturnType<typeof attestation>; signer?: keyof typeof KEY; x?: string } = {}): Message {
  const x = o.x ?? bytesToHex(ephemeralKey(() => KIOSK_EPHEMERAL).x);
  const att = o.att ?? attestation();
  const kioskKeySignature = sign(o.signer ?? "merchant", "KioskKey", { merchant: att.merchant, kioskEphemeral: x, kioskNonce: KIOSK_NONCE });
  return { ...open(o.mode ?? "payment"), kioskEphemeral: x, attestation: att, kioskKeySignature } as Message;
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

/** A secure payment session whose deviceNonce is draw `draw` (the one-time key is the next draw). */
const securePaySession = (draw: number, at: number, ...rest: Message[]): Step[] => [
  { at, send: secureOpen() },
  { at, send: confirm(deviceNonce(draw)) },
  ...rest.map((send) => ({ at, send })),
];

// Rental setup (payment-protocol.md 5). The device starts UNPROVISIONED; draw 0 is the setup
// session's deviceNonce, draw 1 the new key (32 bytes of 0x5b), draw 2 the nonce start (31 bytes
// of 0x5c and a zero low byte).
const PASSKEY = "042195";
const NEW_KEY = new Uint8Array(32).fill(0x5b);
const NEW_DEVICE = addressOfPrivateKey(NEW_KEY);
const operatorMsg = (passkey = PASSKEY): Message =>
  ({ v: 1, type: "setup.operator", sessionId: SID, operator: addressOfPrivateKey(k("operator")), contract: CONTRACT, chainId: String(CHAIN_ID), passkey } as Message);
const anchorFor = (device: string, ts: number): Message =>
  ({ v: 1, type: "setup.timeAnchor", sessionId: SID, device, timestamp: String(ts), operatorSignature: sign("operator", "TimeAnchor", { device, timestamp: BigInt(ts) }) } as Message);
const resetMsg = (device: string, key: keyof typeof KEY = "operator", nonce = 1n): Message =>
  ({ v: 1, type: "device.reset", sessionId: SID, device, nonce: String(nonce), operatorSignature: sign(key, "DeviceReset", { device, nonce }) } as Message);

// Limit change (payment-protocol.md 5): the provisioned device's PIN is 2580.
const limitMsg = (over: Record<string, string> = {}): Message =>
  ({ v: 1, type: "limit.change", sessionId: SID, change: { chainId: String(CHAIN_ID), contract: CONTRACT, perPaymentLimit: "20000000", dailyLimit: "100000000", expiry: String(T0 + 60), ...over } } as Message);

type Scenario = {
  id: string;
  description: string;
  /** The renter's button for payment.prepare and setup.operator. */
  button: "approve" | "reject";
  steps: Step[];
  /** Setup scenarios: the device starts UNPROVISIONED. */
  initialState?: "UNPROVISIONED";
  /** What the renter enters whenever the device asks for the PIN (null: not in time). Default 2580. */
  pin?: string | null;
  /** A release device that refuses plaintext payment sessions (4.1). */
  requireSecureSession?: true;
  /** A release device that needs the phone app, and no phone app is listening (3). */
  requirePhoneAbsent?: true;
  /** Every message arrives on a link that is not bonded (an unpaired central, 3). */
  unbondedLink?: true;
};

const SCENARIOS: Scenario[] = [
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
  { id: "SV-13", description: "rental setup: setup.operator acked on the press, keygen ack with the new address after the PIN, anchor, then a payment from the new key starting at the nonce start",
    button: "approve", initialState: "UNPROVISIONED", pin: "2580",
    steps: [{ at: T0, send: open("setup") }, { at: T0, send: operatorMsg() }, { at: T0, send: anchorFor(NEW_DEVICE, T0) },
      ...paySession(3, T0 + 5, identify(), prepare())] },
  { id: "SV-14", description: "the renter refuses setup.operator: USER_REJECTED, the device stays UNPROVISIONED",
    button: "reject", initialState: "UNPROVISIONED", pin: "2580",
    steps: [{ at: T0, send: open("setup") }, { at: T0, send: operatorMsg() }, { at: T0, send: open("setup") }] },
  { id: "SV-15", description: "the PIN is not entered in time: keygen TIMEOUT, nothing stored",
    button: "approve", initialState: "UNPROVISIONED", pin: null,
    steps: [{ at: T0, send: open("setup") }, { at: T0, send: operatorMsg() }, { at: T0, send: open("setup") }] },
  { id: "SV-16", description: "setup.operator on a provisioned device or with a passkey over six digits: NOT_PERMITTED",
    button: "approve",
    steps: [{ at: T0, send: open("setup") }, { at: T0, send: operatorMsg() }, { at: T0, send: operatorMsg("1000000") }] },
  { id: "SV-17", description: "device.reset: refused for another signer or device, accepted from the recorded operator in any session; the device ends UNPROVISIONED",
    button: "approve",
    steps: [...anchored(), { at: T0, send: open("payment") }, { at: T0, send: resetMsg(DEVICE, "stranger") }, { at: T0, send: resetMsg(NEW_DEVICE) },
      { at: T0, send: resetMsg(DEVICE) }, { at: T0, send: identify() }, { at: T0, send: open("setup") }, { at: T0, send: resetMsg(DEVICE) }] },
  { id: "SV-18", description: "limit change: confirm.limit to the phone, the PIN, the button, a signature with the next sequential nonce; a payment after it takes the following nonce",
    button: "approve", pin: "2580",
    steps: [...anchored(), ...paySession(1, T0 + 5, limitMsg(), identify(), prepare({ expiry: String(T0 + 65) }))] },
  { id: "SV-19", description: "limit change with a wrong PIN: NOT_PERMITTED, nothing signed",
    button: "approve", pin: "1111",
    steps: [...anchored(), ...paySession(1, T0, limitMsg())] },
  { id: "SV-20", description: "limit change the renter rejects after the PIN: USER_REJECTED",
    button: "reject", pin: "2580",
    steps: [...anchored(), ...paySession(1, T0, limitMsg())] },
  { id: "SV-21", description: "pinMaxRetries wrong PINs lock the device: PIN_LOCKED for limit changes and payments, kept across a RAM-clearing reset",
    button: "approve", pin: "1111",
    steps: [...anchored(), ...paySession(1, T0, limitMsg(), limitMsg(), limitMsg(), limitMsg(), limitMsg(), limitMsg(), identify(), prepare()),
      { at: T0, powerCycle: true }, { at: T0, send: open("payment") }] },
  { id: "SV-22", description: "limit change for another contract: NOT_PERMITTED; expiry beyond authorizationExpiry: ATTESTATION_EXPIRED",
    button: "approve", pin: "2580",
    steps: [...anchored(), ...paySession(1, T0, limitMsg({ contract: "0x" + "c1".repeat(20) }), limitMsg({ expiry: String(T0 + 121) }))] },
  { id: "SV-23", description: "requests outside the schema (a raw transaction, a Permit) in a payment session: UNSUPPORTED_TYPE with the zero session id, and the session is closed",
    button: "approve",
    steps: [...anchored(), { at: T0, send: open("payment") }, { at: T0, send: confirm(deviceNonce(1)) }, { at: T0, raw: rawTx() },
      { at: T0, send: identify() }, { at: T0, send: open("payment") }, { at: T0, send: confirm(deviceNonce(2)) }, { at: T0, raw: rawPermit() }] },
  { id: "SV-24", description: "secure session (4.1): the device proves the merchant's one-time key, answers with its own, and every later body both ways is AES-GCM; confirm.show to the phone stays plaintext",
    button: "approve",
    steps: [...anchored(), ...securePaySession(1, T0 + 5, identify(), prepare(),
      { v: 1, type: "payment.outcome", sessionId: SID, orderId: "0x" + "01".repeat(32), outcome: "approved" } as Message)] },
  { id: "SV-25", description: "secure session.open refused with MERCHANT_FORGED: attestation not the operator's, one-time key not signed by the attested merchant, key not a curve point; no random draw is spent on a refusal",
    button: "approve",
    steps: [...anchored(), { at: T0, send: secureOpen({ att: attestation({}, "stranger") }) }, { at: T0, send: secureOpen({ signer: "stranger" }) },
      { at: T0, send: secureOpen({ x: "0x" + "00".repeat(32) }) }, ...securePaySession(1, T0)] },
  { id: "SV-26", description: "a secure body that fails the tag check: error{BAD_FRAME} under the channel and the session closes; the next sealed body meets no channel and is BAD_FRAME in plaintext",
    button: "approve",
    steps: [...anchored(), ...securePaySession(1, T0), { at: T0, send: identify(), tamper: true }, { at: T0, send: prepare() }] },
  { id: "SV-27", description: "in a secure session payment.identify must attest the merchant session.open proved: another operator-signed merchant is MERCHANT_FORGED",
    button: "approve",
    steps: [...anchored(), ...securePaySession(1, T0, identify(attestation({ merchant: STRANGER })))] },
  { id: "SV-28", description: "release device: a plaintext payment session.open and a secure setup session.open are NOT_PERMITTED; setup stays plaintext and a secure payment session opens",
    button: "approve", requireSecureSession: true,
    steps: [...anchored(), { at: T0, send: open("payment") }, { at: T0, send: secureOpen({ mode: "setup" }) }, ...securePaySession(1, T0)] },
  { id: "SV-29", description: "an UNPROVISIONED device has no operator to check a secure session.open against: NOT_PERMITTED",
    button: "approve", initialState: "UNPROVISIONED",
    steps: [{ at: T0, send: secureOpen() }] },
  { id: "SV-30", description: "release device with no phone app listening: payment.prepare and limit.change pass their checks, then are refused NOT_PERMITTED with nothing sent to a phone",
    button: "approve", requirePhoneAbsent: true,
    steps: [...anchored(), ...paySession(1, T0, identify(), prepare(), limitMsg())] },
  { id: "SV-31", description: "a setup session.open from a central that is not bonded: NOT_PERMITTED; payment sessions need no bond",
    button: "approve", initialState: "UNPROVISIONED", unbondedLink: true,
    steps: [{ at: T0, send: open("setup") }, { at: T0, send: open("payment") }] },
];

function run() {
  return SCENARIOS.map((sc) => {
    let now = 0;
    let draws = 0;
    const provisioned = sc.initialState === "UNPROVISIONED"
      ? {}
      : { key: k("device"), operator: addressOfPrivateKey(k("operator")), contract: CONTRACT, chainId: CHAIN_ID, nonceStart: NONCE_START };
    const device = new SoftwareDevice({
      ...provisioned, now: () => now, approve: () => sc.button === "approve",
      confirmSetup: () => sc.button === "approve", enterPin: () => (sc.pin === undefined ? "2580" : sc.pin), pin: "2580",
      random: (n) => new Uint8Array(n).fill(0x5a + draws++),
      requireSecureSession: sc.requireSecureSession === true,
      requirePhone: sc.requirePhoneAbsent === true, phoneConnected: () => sc.requirePhoneAbsent !== true,
      linkBonded: () => sc.unbondedLink !== true,
    });
    // The kiosk's end of the channel, made from the device's session.open.ok (4.1).
    let kiosk: SecureChannel | null = null;
    const steps = sc.steps.map((st) => {
      now = st.at;
      if ("powerCycle" in st) {
        device.powerCycle();
        return { at: st.at, powerCycle: true };
      }
      const phoneBefore = device.phone.length;
      let plain: Uint8Array;
      let body: Uint8Array;
      let sendType: string;
      if ("raw" in st) {
        plain = body = st.raw;
        sendType = "(outside the schema)";
      } else {
        if (st.send.type === "session.open") kiosk = null; // session.open is always plaintext
        plain = encodeMessage(st.send);
        body = kiosk ? kiosk.seal(plain) : plain.slice();
        if (st.tamper) body[body.length - 1] ^= 1;
        sendType = st.send.type + (st.tamper ? " (one bit flipped)" : "");
      }
      const replies = device.handleBody(body);
      // What the kiosk reads: sealed replies open with its channel; a reply sent after the device
      // dropped the channel is plaintext.
      const opened = replies.map((r) => {
        if (!kiosk) return r;
        try {
          return kiosk.open(r);
        } catch {
          return r;
        }
      });
      if ("send" in st && st.send.type === "session.open") {
        const ok = decodeMessage(opened[0]);
        if (ok.type === "session.open.ok" && ok.deviceEphemeral) {
          kiosk = new SecureChannel(sessionKey(KIOSK_EPHEMERAL, hexToBytes(String(ok.deviceEphemeral)), hexToBytes(KIOSK_NONCE), hexToBytes(String(ok.deviceNonce))), "kiosk");
        }
      }
      const hex = (b: Uint8Array) => bytesToHex(b, false);
      const sealed = hex(body) !== hex(plain) || replies.some((r, i) => hex(r) !== hex(opened[i]));
      return {
        at: st.at,
        sendType,
        send: hex(body),
        expect: replies.map(hex),
        // Secure steps: the CBOR bodies inside the AES-GCM ones, for reading and debugging.
        ...(sealed ? { plain: { send: hex(plain), expect: opened.map(hex) } } : {}),
        phone: device.phone.slice(phoneBefore).map((p) => hex(encodeMessage(p))),
      };
    });
    return { id: sc.id, description: sc.description, button: sc.button, pin: sc.pin === undefined ? "2580" : sc.pin, ...(sc.initialState ? { initialState: sc.initialState } : {}),
      ...(sc.requireSecureSession ? { requireSecureSession: true } : {}),
      ...(sc.requirePhoneAbsent ? { requirePhone: true, phoneConnected: false } : {}),
      ...(sc.unbondedLink ? { linkBonded: false } : {}), steps };
  });
}

const doc = {
  description:
    "Shared session vectors (payment-protocol.md 5, 6), generated by packages/device-sim. A device implementation set up with the values below must answer every step's `send` CBOR body with exactly the `expect` bodies (in order) and send the `phone` bodies to the phone app. `at` is the device clock (unix seconds) for that step. `powerCycle` is a RAM-clearing reset. `button` is the renter's answer to payment.prepare and setup.operator. A scenario with `initialState` UNPROVISIONED starts without key or setup values. A provisioned device was set up with PIN 2580; `pin` is what the renter enters whenever the device asks for the PIN, at setup or for a limit change (null: not entered in time). The device asks for the PIN before the approve button. The k-th random request (k = 0, 1, ...) returns bytes all equal to 0x5a + k: deviceNonce at each session.open (none for a refused one), then in a secure session the device's one-time key (32 bytes), and at setup the new key (32 bytes) and the nonce start (31 bytes, followed by a zero byte). Device signatures are deterministic (RFC 6979). Secure sessions (payment-protocol.md 4.1): the kiosk's one-time private key is 32 bytes of 0xe1 and its kioskNonce 32 bytes of 0xa5; `send` and `expect` are the bodies on the wire (AES-GCM after session.open.ok), and `plain` holds the CBOR bodies inside them. A scenario with `requireSecureSession` is a release device that refuses plaintext payment sessions. `requirePhone` with `phoneConnected` false is a release device with no bonded phone app listening; `linkBonded` false means every message comes from a central that is not bonded (payment-protocol.md 3). Otherwise the phone app is connected and the link is bonded.",
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
    pinMaxRetries: 5,
    pin: "2580",
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
