// The kiosk payment session (payment-protocol.md 5, 6; P04 design 3) against the software
// device through the full wire: CBOR, envelope, fragments for a small ATT_MTU.
import { addressOfPrivateKey, bytesToHex, digest, hexToBytes, signDigest, type Message } from "@nu54/protocol";
import { DeviceEndpoint, SoftwareDevice, type DeviceConfig } from "@nu54/device-sim";
import { FramedLink, type FragmentTransport } from "../src/ble/framing.ts";
import { runPayment, type PaymentRequest } from "../src/payment/session.ts";
import { signMerchantOrder } from "../src/payment/signing.ts";

// Foundry public test mnemonic: 0 device, 1 operator, 2 merchant (test-only keys).
const KEY = {
  device: hexToBytes("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"),
  operator: hexToBytes("0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"),
  merchant: hexToBytes("0x5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a"),
};
const CONTRACT = "0xc0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0";
const TOKEN = "0xd1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1";
const PAYOUT = "0xb1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1";
const domain = { chainId: 8283, verifyingContract: CONTRACT };
const T0 = 1_790_000_000;
const sign = (key: Uint8Array, type: Parameters<typeof digest>[1], v: Record<string, string | bigint | number>) =>
  bytesToHex(signDigest(digest(domain, type, v), key));

/** The device behind a transport that delivers fragments asynchronously, like BLE. */
function wire(approve: DeviceConfig["approve"], mtu = 23) {
  const device = new SoftwareDevice({
    key: KEY.device, operator: addressOfPrivateKey(KEY.operator), contract: CONTRACT, chainId: 8283,
    nonceStart: 256n * 9n, now: () => T0, approve,
  });
  const endpoint = new DeviceEndpoint(device, mtu);
  let handler: ((f: Uint8Array) => void) | null = null;
  const transport: FragmentTransport = {
    mtu,
    async write(fragment) {
      const copy = fragment.slice();
      setTimeout(async () => {
        for (const back of await endpoint.receiveAsync(copy)) handler?.(back);
      }, 1);
    },
    onFragment(h) {
      handler = h;
      return () => (handler = null);
    },
    async close() {},
  };
  return { device, link: new FramedLink(transport) };
}

const DEVICE = addressOfPrivateKey(KEY.device);
// The k-th draw is n bytes of k: distinct session ids, nonces and order ids within a run.
let draws = 1;
const random = (n: number) => new Uint8Array(n).fill(draws++ % 256);
function attestation(key = KEY.operator) {
  const a = { merchant: addressOfPrivateKey(KEY.merchant), payout: PAYOUT, name: "Cafe Test 01", validFrom: String(T0), validUntil: String(T0 + 86400) };
  return { ...a, operatorSignature: sign(key, "MerchantAttestation", a) };
}
const anchor = { device: DEVICE, timestamp: String(T0), operatorSignature: sign(KEY.operator, "TimeAnchor", { device: DEVICE, timestamp: BigInt(T0) }) };

function request(over: Partial<PaymentRequest> = {}): PaymentRequest {
  return {
    domain, attestation: attestation(), token: TOKEN, amount: 4_500_000n, expiry: BigInt(T0 + 60),
    signOrder: (o) => signMerchantOrder(domain, o, KEY.merchant), anchor, random, ...over,
  };
}

test("approved: the device signs after the press and the kiosk checks the signer", async () => {
  const { link } = wire(() => true);
  const steps: string[] = [];
  const r = await runPayment(link, request({ onStep: (s) => steps.push(s) }));
  expect(r.status).toBe("approved");
  if (r.status !== "approved") return;
  expect(r.device).toBe(DEVICE);
  expect(r.auth.nonce).toBe(String(256 * 9));
  expect(r.auth.amount).toBe("4500000");
  expect(steps).toEqual(["anchor", "opening", "identifying", "waitingDevice"]);
});

test("a device already READY keeps its anchor and the next payment uses the next nonce", async () => {
  const { link } = wire(() => true);
  await runPayment(link, request());
  const r = await runPayment(link, request());
  expect(r.status === "approved" && r.auth.nonce).toBe(String(256 * 9 + 1));
});

test("an anchor older than ANCHOR_MAX_AGE_S is not sent: TIME_ANCHOR_STALE, and it is dropped", async () => {
  const { link } = wire(() => true);
  let used = 0;
  const r = await runPayment(link, request({ chainTime: BigInt(T0 + 91), onAnchorUsed: () => { used++; } }));
  expect(r).toMatchObject({ status: "refused", reason: "TIME_ANCHOR_STALE" });
  expect(used).toBe(1);
});

test("a late anchor still pays: the expiry is capped to the device clock it set", async () => {
  // The anchor waited 80 s: the device clock runs 80 s behind the chain. The kiosk's usual
  // expiry (chain + 60) would be past the device's window (its clock + 120) and refused.
  const { link } = wire(() => true);
  const r = await runPayment(link, request({ chainTime: BigInt(T0 + 80), expiry: BigInt(T0 + 140) }));
  expect(r.status).toBe("approved");
  // Device clock + authorizationExpiry - 5 s margin, plus the second or two the test itself takes.
  const expiry = r.status === "approved" ? Number(r.auth.expiry) : 0;
  expect(expiry).toBeGreaterThanOrEqual(T0 + 115);
  expect(expiry).toBeLessThanOrEqual(T0 + 117);
});

test("a fresh anchor is fetched for the device the setup session reached", async () => {
  const { link } = wire(() => true);
  const asked: string[] = [];
  const r = await runPayment(link, request({ anchor: undefined, anchorFor: async (device) => { asked.push(device); return anchor; } }));
  expect(r.status).toBe("approved");
  expect(asked).toEqual([DEVICE]);
});

test("a device that already has its time is not sent an anchor", async () => {
  const { link } = wire(() => true);
  await runPayment(link, request()); // anchored now (READY)
  let asked = 0;
  const r = await runPayment(link, request({ anchor: undefined, anchorFor: async () => { asked++; return anchor; } }));
  expect([r.status, asked]).toEqual(["approved", 0]);
});

test("no answer from the anchor server: TIME_ANCHOR_MISSING before any payment", async () => {
  const { link } = wire(() => true);
  const r = await runPayment(link, request({ anchor: undefined, anchorFor: async () => { throw new Error("offline"); } }));
  expect(r).toMatchObject({ status: "refused", reason: "TIME_ANCHOR_MISSING" });
});

test("the renter rejects: refused USER_REJECTED", async () => {
  const { link } = wire(() => false);
  const r = await runPayment(link, request());
  expect(r).toMatchObject({ status: "refused", reason: "USER_REJECTED" });
});

test("attestation not signed by the operator: refused MERCHANT_FORGED at identify", async () => {
  const { link } = wire(() => true);
  const r = await runPayment(link, request({ attestation: attestation(KEY.merchant) }));
  expect(r).toMatchObject({ status: "refused", reason: "MERCHANT_FORGED", detail: "identify: payment.result" });
});

test("no press within the wait: session.cancel, the order is cancelled and nothing is signed", async () => {
  let press: (ok: boolean) => void = () => {};
  const { link } = wire(() => new Promise<boolean>((r) => (press = r)));
  const r = await runPayment(link, request({ waitMs: 50 }));
  expect(r.status).toBe("cancelled");
  await new Promise<void>((done) => setTimeout(done, 20)); // session.cancel reaches the device
  press(true); // too late: the device dropped the session
  const late = await link.send({ v: 1, type: "session.cancel", sessionId: "0000000000000000" } as Message, 1, 50);
  expect(late).toEqual([]);
});

test("no anchor in the request and none on the device: refused TIME_ANCHOR_MISSING", async () => {
  const { link } = wire(() => true);
  const r = await runPayment(link, request({ anchor: undefined }));
  expect(r).toMatchObject({ status: "refused", reason: "TIME_ANCHOR_MISSING" });
});

test("a press that races the cancel is never taken as the next session's answer", async () => {
  let press: (ok: boolean) => void = () => {};
  let real = false;
  const { link } = wire(() => (real ? true : new Promise<boolean>((r) => (press = r))));
  expect((await runPayment(link, request({ waitMs: 50 }))).status).toBe("cancelled");
  press(true); // before session.cancel reached the device: a late payment.result is on its way
  real = true;
  const next = await runPayment(link, request());
  expect(next).toMatchObject({ status: "approved" });
  if (next.status === "approved") expect(next.auth.nonce).not.toBe(String(256 * 9)); // not the stale signature
});
