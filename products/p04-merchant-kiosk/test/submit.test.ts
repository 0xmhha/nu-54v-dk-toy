// Settlement calls and the submission judgment of payment-protocol.md 7 (P04-FR-06, 08-15).
import { addressOfPrivateKey, bytesToHex, hexToBytes, signDigest } from "@nu54/protocol";
import { encodeSettle, ERRORS, PAYMENT_SETTLED_TOPIC, SETTLE_SELECTOR, decodeError, topicOfAddress } from "../src/chain/settlement";
import { signTransaction, transactionHash, rlp, type Signer } from "../src/chain/tx";
import { RpcError, type Chain, type Hex, type Log, type Receipt } from "../src/chain/rpc";
import { gasReady, recheck, submit, type Signed, type SubmitContext } from "../src/payment/submit";

// The week-6 gate settle on chain 8283 (deployments/8283.json): its calldata is the reference.
const GATE_INPUT = "0x7e717651000000000000000000000000000000000000000000000000000000000000205b000000000000000000000000de7596556d35fa62f238f074a0f0e59cf730caa4000000000000000000000000f92a32ceb9d940057be7b76bf4e2eb76409f61a4000000000000000000000000246ae7e5b14f096342b96a65e375524da80c101b000000000000000000000000500ef69da230e42bff34487b578ea0cbefbda2ba000000000000000000000000000000000000000000000000000000000044aa20209f73ded70ab766b39a1505163019f2e1323e6abeb1824dee46a51c855740e8cb79d0c2d61f53305c896156598e534b8c2ba798250bc0c93b09a739f7cebd00000000000000000000000000000000000000000000000000000000006abd7820000000000000000000000000000000000000000000000000000000000000014000000000000000000000000000000000000000000000000000000000000000411590e632218ef25afcace90a7eca8ad99ac9a8fd66c0f50ffb83fbe8d24fb9386967983ce5e665a69b4b0053968d260291b087a2d494fe083ea9763f24f8502c1b00000000000000000000000000000000000000000000000000000000000000";
const GATE_AUTH = {
  chainId: "8283", contract: "0xDe7596556D35Fa62F238F074A0f0E59cF730caA4", merchant: "0xF92a32CEB9d940057be7b76BF4e2Eb76409F61A4",
  payout: "0x246aE7E5b14f096342B96A65E375524Da80c101B", token: "0x500ef69da230E42bFF34487B578eA0cBefbdA2BA", amount: "4500000",
  orderId: "0x209f73ded70ab766b39a1505163019f2e1323e6abeb1824dee46a51c855740e8",
  nonce: "92034737573260763964974316053185194381222955312256459714721409574940173057280", expiry: "1790801952",
};
const GATE_SIG = "0x1590e632218ef25afcace90a7eca8ad99ac9a8fd66c0f50ffb83fbe8d24fb9386967983ce5e665a69b4b0053968d260291b087a2d494fe083ea9763f24f8502c1b";

// cast mktx --private-key <test key 0> --chain 8283 --nonce 7 --gas-limit 150000
//   --priority-gas-price 27600 gwei --gas-price 67600 gwei <settlement> 0x1234
const CAST_TX = "0x02f87382205b0786191a20322000863d7b59fca000830249f094de7596556d35fa62f238f074a0f0e59cf730caa480821234c001a015f9f985203590c9c7651472603e02a80987599300959db0bb0ea98be5342cd0a0729b5b45cddbc2ebc28753490d9dfeb03d02d36cb64e68dbaa916a6ee71e8ae5";

const TEST_KEY = hexToBytes("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80");
const signer: Signer = { address: addressOfPrivateKey(TEST_KEY) as Hex, sign: (d) => signDigest(d, TEST_KEY) };

describe("settlement calls", () => {
  test("settle calldata equals the week-6 gate transaction", () => {
    expect(SETTLE_SELECTOR).toBe("0x7e717651");
    expect(encodeSettle(GATE_AUTH, GATE_SIG)).toBe(GATE_INPUT);
  });
  test("event topic and custom error selectors match the contract", () => {
    expect(PAYMENT_SETTLED_TOPIC).toBe("0xeef4300dbd9217414481ce2ade0c4791c3b47c75b0c0d4b6bbe5fbb05260195f");
    expect(decodeError("0x342fa66d")).toBe("OverCap");
    expect(decodeError("0x7f61b868")).toBe("OrderAlreadyPaid");
    expect(Object.values(ERRORS)).toEqual(expect.arrayContaining(["WrongDomain", "Expired", "NonceReplayed", "WithdrawalMatured"]));
  });
  test("type-2 transaction signing equals cast mktx byte for byte", () => {
    const raw = signTransaction(
      { chainId: 8283n, nonce: 7n, maxPriorityFeePerGas: 27_600_000_000_000n, maxFeePerGas: 67_600_000_000_000n, gas: 150_000n, to: "0xDe7596556D35Fa62F238F074A0f0E59cF730caA4", data: "0x1234" },
      signer,
    );
    expect(raw).toBe(CAST_TX);
    expect(transactionHash(raw)).toMatch(/^0x[0-9a-f]{64}$/);
  });
  test("RLP edge cases", () => {
    expect(bytesToHex(rlp(new Uint8Array(0)))).toBe("0x80");
    expect(bytesToHex(rlp(Uint8Array.of(0x7f)))).toBe("0x7f");
    expect(bytesToHex(rlp([]))).toBe("0xc0");
    expect(bytesToHex(rlp(new Uint8Array(56))).slice(0, 6)).toBe("0xb838");
  });
});


// ---------------------------------------------------------------- judgment (fake chain)

const SETTLEMENT = GATE_AUTH.contract as Hex;
const DEVICE = "0xa8edf32009d5a46ab40c30c7eb0fa1145855c3df";
const signed = (over: Partial<typeof GATE_AUTH> = {}): Signed => ({ auth: { ...GATE_AUTH, ...over }, signature: GATE_SIG, device: DEVICE, requestedAt: 0 });

function settledLog(s: Signed, over: { device?: string; amount?: bigint; nonce?: bigint } = {}): Log {
  const word = (v: bigint) => v.toString(16).padStart(64, "0");
  return {
    address: SETTLEMENT,
    topics: [PAYMENT_SETTLED_TOPIC, topicOfAddress(s.auth.merchant), s.auth.orderId as Hex, topicOfAddress(over.device ?? s.device)],
    data: ("0x" + word(over.amount ?? BigInt(s.auth.amount)) + word(over.nonce ?? BigInt(s.auth.nonce))) as Hex,
    blockNumber: "0x64",
    transactionHash: ("0x" + "ab".repeat(32)) as Hex,
  };
}

class FakeChain implements Chain {
  simulate: (n: number) => string | undefined = () => undefined; // custom error name per call
  receiptFor?: (hash: Hex) => Receipt | null;
  events: Log[] = [];
  finalizedTime = 1_790_801_000n;
  gas = 10n ** 21n;
  calls = 0;
  sent: Hex[] = [];
  sendError?: RpcError;
  async call() {
    const name = this.simulate(++this.calls);
    if (!name) return "0x" as Hex;
    const sel = Object.entries(ERRORS).find(([, n]) => n === name)![0];
    throw new RpcError(3, "execution reverted", sel as Hex);
  }
  async estimateGas() { return 120_000n; }
  async balance() { return this.gas; }
  async pendingNonce() { return 3n; }
  async priorityFee() { return 27_600_000_000_000n; }
  async block() { return { number: 100n, timestamp: this.finalizedTime, baseFee: 20_000_000_000_000n }; }
  async sendRaw(raw: Hex) {
    if (this.sendError) throw this.sendError;
    this.sent.push(raw);
    return transactionHash(raw);
  }
  async receipt(hash: Hex) { return this.receiptFor ? this.receiptFor(hash) : null; }
  async logs() { return this.events; }
}

function ctx(chain: FakeChain, clock = { t: 0 }): SubmitContext {
  return {
    chain, signer, settlement: SETTLEMENT, chainId: 8283n, minGasBalance: 13n * 10n ** 18n, fromBlock: 21129130n,
    now: () => clock.t, sleep: async (ms) => { clock.t += ms; },
  };
}

describe("submission judgment", () => {
  test("P04-FR-08: busy below kioskMinGasBalance", async () => {
    const chain = new FakeChain();
    expect(await gasReady(ctx(chain))).toBe(true);
    chain.gas = 12n * 10n ** 18n;
    expect(await gasReady(ctx(chain))).toBe(false);
  });

  test("P04-FR-09, 10: simulate, send, approve on the finalized PaymentSettled", async () => {
    const chain = new FakeChain();
    const s = signed();
    chain.receiptFor = (hash) => ({ status: "0x1", blockNumber: "0x64", transactionHash: hash, logs: [settledLog(s)] });
    const out = await submit(ctx(chain), s);
    expect(out.status).toBe("approved");
    expect(chain.sent).toHaveLength(1);
    expect(chain.sent[0].startsWith("0x02")).toBe(true);
  });

  test("P04-FR-06: a custom error is refused with its code and nothing is sent", async () => {
    for (const [error, reason] of [["MerchantRevoked", "MERCHANT_REVOKED"], ["OverCap", "OVER_CAP"], ["NonceReplayed", "NONCE_REPLAYED"], ["MerchantForged", "MERCHANT_FORGED"], ["Expired", "EXPIRED"], ["AccountInactive", "ACCOUNT_INACTIVE"], ["InsufficientBalance", "INSUFFICIENT_BALANCE"], ["WrongDomain", "WRONG_DOMAIN"]]) {
      const chain = new FakeChain();
      chain.simulate = () => error;
      expect(await submit(ctx(chain), signed())).toEqual({ status: "refused", reason });
      expect(chain.sent).toHaveLength(0);
    }
  });

  test("P04-FR-11: OrderAlreadyPaid is approved only for our own device, amount and nonce", async () => {
    const chain = new FakeChain();
    const s = signed();
    chain.simulate = () => "OrderAlreadyPaid";
    chain.events = [settledLog(s)];
    expect((await submit(ctx(chain), s)).status).toBe("approved");
    chain.events = [settledLog(s, { device: "0x" + "11".repeat(20) })];
    expect(await submit(ctx(chain), s)).toEqual({ status: "refused", reason: "ORDER_ALREADY_PAID" });
    chain.events = [settledLog(s, { amount: 1n })];
    expect(await submit(ctx(chain), s)).toEqual({ status: "refused", reason: "ORDER_ALREADY_PAID" });
    expect(chain.sent).toHaveLength(0);
  });

  test("P04-FR-12: status 0 is simulated again; OrderAlreadyPaid -> FR-11, otherwise failed with the short hash", async () => {
    const chain = new FakeChain();
    const s = signed();
    chain.receiptFor = (hash) => ({ status: "0x0", blockNumber: "0x64", transactionHash: hash, logs: [] });
    chain.simulate = (n) => (n === 1 ? undefined : "OrderAlreadyPaid");
    chain.events = [settledLog(s)];
    expect((await submit(ctx(chain), s)).status).toBe("approved");
    const chain2 = new FakeChain();
    chain2.receiptFor = chain.receiptFor;
    chain2.simulate = (n) => (n === 1 ? undefined : "OverCap");
    const out = await submit(ctx(chain2), s);
    expect(out.status).toBe("failed");
    expect((out as { reason: string }).reason).toMatch(/^reverted 0x[0-9a-f]{6}…[0-9a-f]{4}$/);
  });

  test("P04-FR-14: no result within 10 s -> Checking, and recheck uses the same signature only", async () => {
    const chain = new FakeChain();
    const s = signed();
    const out = await submit(ctx(chain), s);
    expect(out.status).toBe("Checking");
    expect(chain.sent).toHaveLength(1);
    expect((await recheck(ctx(chain), s)).status).toBe("Checking");
    chain.events = [settledLog(s)];
    expect((await recheck(ctx(chain), s)).status).toBe("approved");
    expect(chain.sent).toHaveLength(1); // recheck never sends or asks for a new signature
  });

  test("P04-FR-15: Checking after the expiry without an event ends as failed", async () => {
    const chain = new FakeChain();
    const s = signed();
    chain.finalizedTime = BigInt(s.auth.expiry) + 1n;
    chain.simulate = () => "Expired";
    expect(await recheck(ctx(chain), s)).toEqual({ status: "failed", reason: "EXPIRED" });
  });

  test("a transaction the node refuses is failed, not thrown", async () => {
    const chain = new FakeChain();
    chain.sendError = new RpcError(-32000, "transaction underpriced");
    expect(await submit(ctx(chain), signed())).toEqual({ status: "failed", reason: "not sent: transaction underpriced" });
  });
});
