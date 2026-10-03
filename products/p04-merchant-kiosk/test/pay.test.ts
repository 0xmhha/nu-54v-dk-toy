// The kiosk payment from the screen's point of view (P04 design 3, 4): gas check, device
// session against the software device, submission, payment.outcome to the device, provisioning.
import { addressOfPrivateKey, bytesToHex, digest, hexToBytes, signDigest, type Message } from "@nu54/protocol";
import { DeviceEndpoint, SoftwareDevice } from "@nu54/device-sim";
import { FramedLink, type MessageLink } from "../src/ble/framing.ts";
import type { Chain, Hex } from "../src/chain/rpc.ts";
import { loadKiosk, takeAnchor, type Loaded, type Provision, type Vault } from "../src/kiosk/config.ts";
import { changeLimits, pay, resumeOrders, type Phase } from "../src/kiosk/pay.ts";
import { OrderStore } from "../src/kiosk/orders.ts";
import type { Outcome } from "../src/payment/submit.ts";

// Foundry public test mnemonic: 0 device, 1 operator, 2 merchant (test-only keys).
const KEY = {
  device: "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80",
  operator: "0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d",
  merchant: "0x5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a",
};
const SETTLEMENT = "0xc0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0" as Hex;
const TOKEN = "0xd1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1d1" as Hex;
const PAYOUT = "0xb1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1b1";
const T0 = 1_790_000_000;
const domain = { chainId: 8283, verifyingContract: SETTLEMENT };
const sign = (key: string, type: Parameters<typeof digest>[1], v: Record<string, string | bigint>) =>
  bytesToHex(signDigest(digest(domain, type, v), hexToBytes(key)));
const DEVICE = addressOfPrivateKey(hexToBytes(KEY.device));
const MERCHANT = addressOfPrivateKey(hexToBytes(KEY.merchant));

function provision(): Provision {
  const a = { merchant: MERCHANT, payout: PAYOUT, name: "Cafe Test 01", validFrom: String(T0), validUntil: String(T0 + 86400) };
  return {
    config: {
      rpc: "http://unused", chainId: 8283, settlement: SETTLEMENT, token: TOKEN, tokenSymbol: "tUSDC", tokenDecimals: 6,
      fromBlock: "1", minGasBalanceWei: (13n * 10n ** 18n).toString(),
      attestation: { ...a, operatorSignature: sign(KEY.operator, "MerchantAttestation", a) },
    },
    keys: { gas: KEY.merchant, merchant: KEY.merchant },
  };
}

class MemoryVault implements Vault {
  files = new Map<string, string>();
  keys = new Map<string, string>();
  settings = new Map<string, string>();
  async wrapKey(alias: string, b64: string) { this.keys.set(alias, b64); }
  async unwrapKey(alias: string) { return this.keys.get(alias) ?? null; }
  async putSetting(n: string, v: string) { this.settings.set(n, v); }
  async getSetting(n: string) { return this.settings.get(n) ?? null; }
  async takeFile(n: string) { const t = this.files.get(n) ?? null; this.files.delete(n); return t; }
}

const chain = (gasWei: bigint) =>
  ({ balance: async () => gasWei, block: async () => ({ number: 1n, timestamp: BigInt(T0), baseFee: 1n }) }) as unknown as Chain;

/** The software device behind an asynchronous fragment transport; records what the device got. */
function deviceLink(approve = true) {
  const device = new SoftwareDevice({
    key: hexToBytes(KEY.device), operator: addressOfPrivateKey(hexToBytes(KEY.operator)), contract: SETTLEMENT, chainId: 8283,
    nonceStart: 256n, now: () => T0, approve: () => approve,
  });
  const received: Message[] = [];
  const handle = device.handle.bind(device);
  device.handle = (m) => {
    received.push(m);
    return handle(m);
  };
  const endpoint = new DeviceEndpoint(device, 185);
  let closed = false;
  // The device ends the session with the link, after the writes already in flight; a new
  // connection means the old one is gone.
  let dropping: ReturnType<typeof setTimeout> | null = null;
  const drop = () => {
    if (dropping) clearTimeout(dropping);
    dropping = null;
    device.linkClosed();
  };
  const connect = async (): Promise<MessageLink> => {
    if (dropping) drop();
    let h: ((f: Uint8Array) => void) | null = null;
    const link = new FramedLink({
      mtu: 185,
      async write(f) { const c = f.slice(); setTimeout(() => endpoint.receive(c).forEach((b) => h?.(b)), 1); },
      onFragment(fn) { h = fn; return () => (h = null); },
      async close() { closed = true; dropping = setTimeout(drop, 5); },
    });
    return link;
  };
  return { device, received, connect, closed: () => closed };
}

const anchor = { device: DEVICE, timestamp: String(T0), operatorSignature: sign(KEY.operator, "TimeAnchor", { device: DEVICE, timestamp: BigInt(T0) }) };
let draws = 1;
const random = (n: number) => new Uint8Array(n).fill(draws++ % 256);

async function provisioned(): Promise<Loaded> {
  const vault = new MemoryVault();
  vault.files.set("provision.json", JSON.stringify(provision()));
  const k = await loadKiosk(vault);
  if (!k) throw new Error("not provisioned");
  return k;
}

test("provisioning: provision.json is taken once, keys are wrapped, the file is gone", async () => {
  const vault = new MemoryVault();
  expect(await loadKiosk(vault)).toBeNull();
  vault.files.set("provision.json", JSON.stringify(provision()));
  const k = await loadKiosk(vault);
  expect(k?.config.tokenSymbol).toBe("tUSDC");
  expect(bytesToHex(k!.merchantKey)).toBe(KEY.merchant);
  expect(vault.files.has("provision.json")).toBe(false);
  expect((await loadKiosk(vault))?.config.settlement).toBe(SETTLEMENT); // from storage now
  vault.files.set("anchor.json", JSON.stringify(anchor));
  expect(await takeAnchor(vault)).toEqual(anchor);
  expect(await takeAnchor(vault)).toBeUndefined();
});

test("a provision with a short key is refused", async () => {
  const vault = new MemoryVault();
  const p = provision();
  p.keys.gas = "0x1234";
  vault.files.set("provision.json", JSON.stringify(p));
  await expect(loadKiosk(vault)).rejects.toThrow("keys.gas");
});

test("approved: submits the device signature and tells the device the outcome", async () => {
  const kiosk = await provisioned();
  const d = deviceLink();
  const phases: Phase[] = [];
  let submitted: unknown;
  const approved: Outcome = { status: "approved", event: { device: DEVICE, amount: 4_500_000n, nonce: 256n } as never };
  const r = await pay(
    { kiosk, chain: chain(20n * 10n ** 18n), connect: d.connect, anchor: async () => anchor, random,
      submit: async (_ctx, s) => {
        submitted = s;
        return approved;
      } },
    4_500_000n, (p) => phases.push(p));
  expect(r.status).toBe("approved");
  expect(phases).toEqual(["checkingGas", "connecting", "anchor", "opening", "identifying", "waitingDevice", "submitting"]);
  expect(submitted).toMatchObject({ device: DEVICE, auth: { amount: "4500000", nonce: "256", payout: PAYOUT } });
  await new Promise<void>((done) => setTimeout(done, 20));
  expect(d.received.at(-1)).toMatchObject({ type: "payment.outcome", outcome: "approved" });
  expect(d.closed()).toBe(true);
});

test("gas below kioskMinGasBalance: busy, no device session", async () => {
  const kiosk = await provisioned();
  const d = deviceLink();
  const r = await pay({ kiosk, chain: chain(12n * 10n ** 18n), connect: d.connect, anchor: async () => anchor, random }, 1n);
  expect(r.status).toBe("busy");
  expect(d.received).toEqual([]);
});

test("the renter rejects: refused USER_REJECTED, nothing submitted", async () => {
  const kiosk = await provisioned();
  const d = deviceLink(false);
  const r = await pay(
    { kiosk, chain: chain(20n * 10n ** 18n), connect: d.connect, anchor: async () => anchor, random,
      submit: async () => { throw new Error("must not submit"); } },
    1_000_000n);
  expect(r).toEqual({ status: "refused", reason: "USER_REJECTED" });
});

test("no device in payment mode: noDevice", async () => {
  const kiosk = await provisioned();
  const r = await pay(
    { kiosk, chain: chain(20n * 10n ** 18n), connect: async () => { throw new Error("no device in payment mode nearby"); }, anchor: async () => undefined, random },
    1n);
  expect(r).toEqual({ status: "noDevice", reason: "no device in payment mode nearby" });
});

test("base64 round-trips every length and matches RFC 4648 vectors", () => {
  const { toBase64, fromBase64 } = jest.requireActual("@nu54/protocol");
  const vectors: [string, string][] = [["", ""], ["f", "Zg=="], ["fo", "Zm8="], ["foo", "Zm9v"], ["foob", "Zm9vYg=="], ["fooba", "Zm9vYmE="], ["foobar", "Zm9vYmFy"]];
  for (const [plain, enc] of vectors) {
    const bytes = Uint8Array.from([...plain].map((c) => c.charCodeAt(0)));
    expect(toBase64(bytes)).toBe(enc);
    expect(Array.from(fromBase64(enc))).toEqual(Array.from(bytes));
  }
  const all = Uint8Array.from({ length: 256 }, (_, i) => i);
  for (let n = 0; n <= 256; n += 37) expect(Array.from(fromBase64(toBase64(all.slice(0, n))))).toEqual(Array.from(all.slice(0, n)));
});

test("limit change from the kiosk: needs an anchored device, then submits the signed change", async () => {
  const kiosk = await provisioned();
  const d = deviceLink();
  const deps = { kiosk, chain: chain(20n * 10n ** 18n), connect: d.connect, anchor: async () => anchor, random };
  expect(await changeLimits(deps, 1n, 2n)).toEqual({ status: "refused", reason: "TIME_ANCHOR_MISSING" });
  // A payment session hands the device its anchor (week-7 development setup).
  await pay({ ...deps, submit: async () => ({ status: "refused", reason: "x" }) }, 1n);
  let submitted: unknown;
  const r = await changeLimits({ ...deps, submitLimits: async (_c, change) => {
    submitted = change;
    return { status: "approved", txHash: "0x01" as Hex };
  } }, 20_000_000n, 0n);
  expect(r).toEqual({ status: "approved", txHash: "0x01" });
  expect(submitted).toMatchObject({ perPaymentLimit: "20000000", dailyLimit: "0", contract: SETTLEMENT });
  expect(d.device.phone.at(-1)?.type).toBe("confirm.limit");
});

test("the signature is stored before submission, and a restart resumes the open order", async () => {
  const kiosk = await provisioned();
  const kv = new Map<string, string>();
  const backing = { getSetting: async (n: string) => kv.get(n) ?? null, putSetting: async (n: string, v: string) => { kv.set(n, v); } };
  const orders = await OrderStore.open(backing);
  const d = deviceLink();
  // The app dies while submitting: the record must already hold the signature.
  await expect(pay({ kiosk, chain: chain(20n * 10n ** 18n), connect: d.connect, anchor: async () => anchor, random, orders,
    submit: async () => { throw new Error("killed"); } }, 4_500_000n)).rejects.toThrow("killed");
  const reopened = await OrderStore.open(backing);
  const [open] = reopened.open();
  expect(open).toMatchObject({ state: "signed", amount: "4500000", signed: { device: DEVICE } });
  // After the restart the order is resumed with its own signature, never a new one.
  const seen: string[] = [];
  const done = await resumeOrders({ kiosk, chain: chain(0n), orders: reopened }, async (_c, s) => {
    seen.push(s.signature);
    return { status: "Checking", txHash: "0x02" as Hex };
  });
  expect(seen).toEqual([open.signed!.signature]);
  expect(done[0]).toMatchObject({ state: "Checking", txHash: "0x02" });
});

test("a device refusal and a missed press are recorded; an unsigned order is cancelled on restart", async () => {
  const kiosk = await provisioned();
  const kv = new Map<string, string>();
  const orders = await OrderStore.open({ getSetting: async (n) => kv.get(n) ?? null, putSetting: async (n, v) => { kv.set(n, v); } });
  await pay({ kiosk, chain: chain(20n * 10n ** 18n), connect: deviceLink(false).connect, anchor: async () => anchor, random, orders }, 1n);
  expect(orders.open()).toEqual([]);
  await orders.create("0xstale", 1n); // crashed while waiting for the press
  const done = await resumeOrders({ kiosk, chain: chain(0n), orders });
  expect(done).toMatchObject([{ orderId: "0xstale", state: "cancelled" }]);
});
