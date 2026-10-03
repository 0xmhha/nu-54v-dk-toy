// The kiosk's secure payment session (payment-protocol.md 4.1) against the software device over
// the full wire: the one-time key exchange, AES-GCM bodies, and the cases the kiosk must refuse.
import { addressOfPrivateKey, bytesToHex, decodeMessage, digest, FrameWriter, hexToBytes, Reassembler, signDigest, type Message } from "@nu54/protocol";
import { DeviceEndpoint, SoftwareDevice, type DeviceConfig } from "@nu54/device-sim";
import { FramedLink, type FragmentTransport, type MessageLink } from "../src/ble/framing.ts";
import { runPayment, type PaymentRequest } from "../src/payment/session.ts";
import { runLimitChange } from "../src/payment/limits.ts";
import { signKioskKey, signMerchantOrder } from "../src/payment/signing.ts";

// Foundry public test mnemonic: 0 device, 1 operator, 2 merchant; 3 has no role (test-only keys).
const KEY = {
  device: hexToBytes("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"),
  operator: hexToBytes("0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"),
  merchant: hexToBytes("0x5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a"),
  stranger: hexToBytes("0x7c852118294e51e653712a81e05800f419141751be58f605c371e15141b007a6"),
};
const CONTRACT = "0xc0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0";
const domain = { chainId: 8283, verifyingContract: CONTRACT };
const T0 = 1_790_000_000;
const DEVICE = addressOfPrivateKey(KEY.device);
const sign = (key: Uint8Array, type: Parameters<typeof digest>[1], v: Record<string, string | bigint | number>) =>
  bytesToHex(signDigest(digest(domain, type, v), key));
let draws = 1;
const random = (n: number) => new Uint8Array(n).fill(draws++ % 250 + 1);

/**
 * The device behind an asynchronous fragment transport; `bodies` collects what it carried.
 * `tamperReply` rewrites a device body and frames it again (valid envelope digest), as an
 * attacker on the air could.
 */
function wire(cfg: Partial<DeviceConfig> = {}, tamperReply?: (body: Uint8Array) => Uint8Array) {
  const device = new SoftwareDevice({ key: KEY.device, operator: addressOfPrivateKey(KEY.operator), contract: CONTRACT, chainId: 8283,
    nonceStart: 256n * 3n, now: () => T0, ...cfg });
  const endpoint = new DeviceEndpoint(device, 185);
  const toDevice = new Reassembler(), fromDevice = new Reassembler(), reframe = new Reassembler();
  const writer = new FrameWriter(185);
  const bodies: { dir: "kiosk" | "device"; body: Uint8Array }[] = [];
  let handler: ((f: Uint8Array) => void) | null = null;
  const tap = (r: Reassembler, dir: "kiosk" | "device", f: Uint8Array) => {
    const got = r.feed(f);
    if (got.status === "done") bodies.push({ dir, body: got.body });
  };
  const transport: FragmentTransport = {
    mtu: 185,
    async write(fragment) {
      const copy = fragment.slice();
      tap(toDevice, "kiosk", copy);
      setTimeout(async () => {
        for (const back of await endpoint.receiveAsync(copy)) {
          let out = [back];
          if (tamperReply) {
            const r = reframe.feed(back);
            out = r.status === "done" ? writer.write(tamperReply(r.body)) : [];
          }
          for (const f of out) {
            tap(fromDevice, "device", f);
            handler?.(f);
          }
        }
      }, 1);
    },
    onFragment(h) {
      handler = h;
      return () => (handler = null);
    },
    async close() {},
  };
  return { device, link: new FramedLink(transport), bodies };
}

function attestation() {
  const a = { merchant: addressOfPrivateKey(KEY.merchant), payout: "0x" + "b1".repeat(20), name: "Cafe Test 01", validFrom: String(T0), validUntil: String(T0 + 86400) };
  return { ...a, operatorSignature: sign(KEY.operator, "MerchantAttestation", a) };
}
const anchor = { device: DEVICE, timestamp: String(T0), operatorSignature: sign(KEY.operator, "TimeAnchor", { device: DEVICE, timestamp: BigInt(T0) }) };

function request(over: Partial<PaymentRequest> = {}): PaymentRequest {
  return {
    domain, attestation: attestation(), token: "0x" + "d1".repeat(20), amount: 4_500_000n, expiry: BigInt(T0 + 60),
    signOrder: (o) => signMerchantOrder(domain, o, KEY.merchant), anchor, random,
    signKioskKey: (v) => signKioskKey(domain, v, KEY.merchant), ...over,
  };
}

const decodes = (b: Uint8Array) => {
  try {
    decodeMessage(b);
    return true;
  } catch {
    return false;
  }
};

test("approved over the channel: after session.open.ok no body on the wire is readable CBOR", async () => {
  const w = wire({ approve: () => true });
  const r = await runPayment(w.link, request());
  expect(r).toMatchObject({ status: "approved", device: DEVICE });
  const open = w.bodies.findIndex((b) => b.dir === "device" && decodes(b.body) && decodeMessage(b.body).deviceEphemeral);
  expect(open).toBeGreaterThan(0);
  const after = w.bodies.slice(open + 1);
  expect(after.length).toBeGreaterThanOrEqual(4); // confirm, identify, prepare, result
  expect(after.every((b) => !decodes(b.body))).toBe(true);
  expect(w.device.phone.at(-1)?.type).toBe("confirm.show"); // the phone's copy stays plain CBOR
});

test("a one-time key signed by someone other than the attested merchant: MERCHANT_FORGED, nothing opens", async () => {
  const w = wire({ approve: () => true });
  const r = await runPayment(w.link, request({ signKioskKey: (v) => signKioskKey(domain, v, KEY.stranger) }));
  expect(r).toMatchObject({ status: "refused", reason: "MERCHANT_FORGED" });
});

test("a device that answers without its one-time key is refused; the kiosk does not fall back to plaintext", async () => {
  const sent: Message[] = [];
  const link: MessageLink = {
    async send(m) {
      sent.push(m);
      return m.type === "session.open"
        ? [{ v: 1, type: "session.open.ok", sessionId: m.sessionId, device: DEVICE, deviceNonce: "0x" + "22".repeat(32), anchorValid: true, firmware: "x", state: "READY" } as Message]
        : [];
    },
    secure() {},
    async close() {},
  };
  const r = await runPayment(link, request({ anchor: undefined }));
  expect(r).toMatchObject({ status: "refused", reason: "NOT_PERMITTED" });
  expect(sent.map((m) => m.type)).toEqual(["session.open"]);
});

test("a release device refuses a kiosk that opens in plaintext", async () => {
  const w = wire({ approve: () => true, requireSecureSession: true });
  expect(await runPayment(w.link, request({ signKioskKey: undefined }))).toMatchObject({ status: "refused", reason: "NOT_PERMITTED" });
  expect(await runPayment(w.link, request())).toMatchObject({ status: "approved" });
});

test("a device reply altered on the air fails its tag: the kiosk stops at BAD_FRAME", async () => {
  let flipped = 0;
  // Flip a bit in the first sealed device body (payment.result): CBOR bodies pass untouched.
  const w = wire({ approve: () => true }, (b) => {
    if (decodes(b) || flipped++) return b;
    const c = b.slice();
    c[c.length - 1] = c[c.length - 1] === 0 ? 1 : 0; // any change breaks the tag
    return c;
  });
  const r = await runPayment(w.link, request());
  expect(flipped).toBe(1);
  expect(r.status).not.toBe("approved");
  expect(w.link.badFrame).toMatch(/tag check failed/);
});

test("a limit change runs over the channel too", async () => {
  const w = wire({ approve: () => true, enterPin: () => "2580" });
  await runPayment(w.link, request()); // anchors the device (week-7 development setup)
  const r = await runLimitChange(w.link, { domain, perPaymentLimit: 20_000_000n, dailyLimit: 0n, expiry: BigInt(T0 + 60), random,
    secure: { attestation: attestation(), sign: (v) => signKioskKey(domain, v, KEY.merchant) } });
  expect(r).toMatchObject({ status: "approved", device: DEVICE });
});
