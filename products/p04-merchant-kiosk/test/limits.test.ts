// Limit change relay (payment-protocol.md 5, WBS2-P04-03): setLimits calldata equals cast,
// the session against the software device, and the submission's judgment on a fake chain.
import { addressOfPrivateKey, bytesToHex, digest, hexToBytes, signDigest, type Message } from "@nu54/protocol";
import { DeviceEndpoint, SoftwareDevice, type DeviceConfig } from "@nu54/device-sim";
import { FramedLink } from "../src/ble/framing.ts";
import { encodeSetLimits, ERRORS, LIMITS_CHANGED_TOPIC } from "../src/chain/settlement.ts";
import { RpcError, type Chain, type Hex, type Receipt } from "../src/chain/rpc.ts";
import { transactionHash, type Signer } from "../src/chain/tx.ts";
import { runLimitChange, submitLimits } from "../src/payment/limits.ts";
import type { SubmitContext } from "../src/payment/submit.ts";

// cast calldata "setLimits((uint256,address,uint256,uint256,uint256,uint64),bytes)"
//   "(8283,0xDe7596556D35Fa62F238F074A0f0E59cF730caA4,20000000,0,1792,1790000060)" 0xabab...ab1b
const CAST_SET_LIMITS = "0xde26c6f2000000000000000000000000000000000000000000000000000000000000205b000000000000000000000000de7596556d35fa62f238f074a0f0e59cf730caa40000000000000000000000000000000000000000000000000000000001312d0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000700000000000000000000000000000000000000000000000000000000006ab13bbc00000000000000000000000000000000000000000000000000000000000000e00000000000000000000000000000000000000000000000000000000000000041abababababababababababababababababababababababababababababababababababababababababababababababababababababababababababababababab1b00000000000000000000000000000000000000000000000000000000000000";

test("setLimits calldata and the LimitsChanged topic match cast", () => {
  const change = { chainId: "8283", contract: "0xDe7596556D35Fa62F238F074A0f0E59cF730caA4", perPaymentLimit: "20000000", dailyLimit: "0", nonce: "1792", expiry: "1790000060" };
  expect(encodeSetLimits(change, "0x" + "ab".repeat(64) + "1b")).toBe(CAST_SET_LIMITS);
  expect(LIMITS_CHANGED_TOPIC).toBe("0x40673dd94c74259f5630bc8b77c27dce7b9dad44aa1a9e15acbac2dcf8643a08");
});

// ---------------------------------------------------------------- the session

const KEY = {
  device: hexToBytes("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"),
  operator: hexToBytes("0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"),
};
const CONTRACT = "0xc0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0";
const domain = { chainId: 8283, verifyingContract: CONTRACT };
const T0 = 1_790_000_000;
const DEVICE = addressOfPrivateKey(KEY.device);
let draws = 1;
const random = (n: number) => new Uint8Array(n).fill(draws++ % 256);

function wire(cfg: Partial<DeviceConfig> = {}) {
  const device = new SoftwareDevice({ key: KEY.device, operator: addressOfPrivateKey(KEY.operator), contract: CONTRACT, chainId: 8283, nonceStart: 256n * 4n, now: () => T0, ...cfg });
  const ep = new DeviceEndpoint(device, 185);
  // anchor the device directly (the setup session is not under test here)
  device.handle({ v: 1, type: "session.open", sessionId: "0909090909090909", mode: "setup", kioskNonce: "0x" + "11".repeat(32) } as Message);
  device.handle({ v: 1, type: "setup.timeAnchor", sessionId: "0909090909090909", device: DEVICE, timestamp: String(T0),
    operatorSignature: bytesToHex(signDigest(digest(domain, "TimeAnchor", { device: DEVICE, timestamp: BigInt(T0) }), KEY.operator)) } as Message);
  let h: ((f: Uint8Array) => void) | null = null;
  const link = new FramedLink({
    mtu: 185,
    async write(f) {
      const c = f.slice();
      setTimeout(async () => (await ep.receiveAsync(c)).forEach((b) => h?.(b)), 1);
    },
    onFragment(fn) {
      h = fn;
      return () => (h = null);
    },
    async close() {},
  });
  return { device, link };
}

const req = (over = {}) => ({ domain, perPaymentLimit: 20_000_000n, dailyLimit: 0n, expiry: BigInt(T0 + 60), random, ...over });

test("approved: the kiosk gets a LimitChange signed by the session's device", async () => {
  const { device, link } = wire();
  const r = await runLimitChange(link, req());
  expect(r).toMatchObject({ status: "approved", device: DEVICE, change: { nonce: String(256 * 4), perPaymentLimit: "20000000" } });
  expect(device.phone.at(-1)?.type).toBe("confirm.limit");
});

test("wrong PIN, rejection and no answer in time", async () => {
  expect(await runLimitChange(wire({ enterPin: () => "0000" }).link, req())).toEqual({ status: "refused", reason: "NOT_PERMITTED" });
  expect(await runLimitChange(wire({ approve: () => false }).link, req())).toEqual({ status: "refused", reason: "USER_REJECTED" });
  const slow = wire({ enterPin: () => new Promise<string>(() => {}) });
  expect(await runLimitChange(slow.link, req({ waitMs: 50 }))).toEqual({ status: "cancelled" });
});

// ---------------------------------------------------------------- submission

const GAS_KEY = hexToBytes("0x5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a");
const signer: Signer = { address: addressOfPrivateKey(GAS_KEY) as Hex, sign: (d) => signDigest(d, GAS_KEY) };
const selectorOf = (name: string) => Object.entries(ERRORS).find(([, n]) => n === name)![0] as Hex;

function chain(revert?: string): Chain & { sent: Hex[] } {
  const sent: Hex[] = [];
  return {
    sent,
    async call() {
      if (revert) throw new RpcError(3, "execution reverted", selectorOf(revert));
      return "0x" as Hex;
    },
    async estimateGas() { return 60_000n; },
    async balance() { return 10n ** 20n; },
    async pendingNonce() { return 1n; },
    async priorityFee() { return 1n; },
    async block() { return { number: 10n, timestamp: BigInt(T0), baseFee: 1n }; },
    async sendRaw(raw: Hex) { sent.push(raw); return transactionHash(raw); },
    async receipt(hash: Hex): Promise<Receipt> {
      return { status: "0x1", blockNumber: "0x9", transactionHash: hash, logs: [{ address: CONTRACT as Hex, topics: [LIMITS_CHANGED_TOPIC], data: "0x", blockNumber: "0x9", transactionHash: hash }] };
    },
    async logs() { return []; },
  };
}
const ctx = (c: Chain): SubmitContext => ({ chain: c, signer, settlement: CONTRACT as Hex, chainId: 8283n, minGasBalance: 0n, fromBlock: 1n, sleep: async () => {} });
const CHANGE = { chainId: "8283", contract: CONTRACT, perPaymentLimit: "1", dailyLimit: "1", nonce: "1024", expiry: String(T0 + 60) };

test("setLimits: sent and final -> approved; resubmission -> NONCE_REPLAYED; above the cap -> OVER_CAP", async () => {
  const ok = chain();
  expect(await submitLimits(ctx(ok), CHANGE, "0x" + "ab".repeat(65))).toMatchObject({ status: "approved" });
  expect(ok.sent).toHaveLength(1);
  const replay = chain("NonceReplayed");
  expect(await submitLimits(ctx(replay), CHANGE, "0x" + "ab".repeat(65))).toEqual({ status: "refused", reason: "NONCE_REPLAYED" });
  expect(replay.sent).toHaveLength(0);
  expect(await submitLimits(ctx(chain("OverCap")), CHANGE, "0x" + "ab".repeat(65))).toEqual({ status: "refused", reason: "OVER_CAP" });
});
