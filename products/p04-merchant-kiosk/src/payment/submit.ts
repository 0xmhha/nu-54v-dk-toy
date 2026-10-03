// Submission and judgment of one signed payment (payment-protocol.md 7, P04-FR-06, 08-15).
//
//   simulate settle (eth_call) -> custom error? -> refused, or OrderAlreadyPaid -> compare the
//   finalized PaymentSettled with our signature -> send the transaction -> PaymentSettled in a
//   finalized block -> approved. status 0 -> simulate once more. No result within 10 s of the
//   request -> Checking (no new signature, same order blocked) until the event shows up, or the
//   signature expires -> failed.

import type { Chain, Hex } from "../chain/rpc.ts";
import { RpcError } from "../chain/rpc.ts";
import { PAYMENT_SETTLED_TOPIC, decodeError, decodeSettled, encodeSettle, topicOfAddress, type Authorization, type Settled } from "../chain/settlement.ts";
import { signTransaction, transactionHash, type Signer } from "../chain/tx.ts";

export type Outcome =
  | { status: "approved"; txHash?: Hex; event: Settled }
  | { status: "refused"; reason: string }
  | { status: "failed"; reason: string; txHash?: Hex }
  | { status: "Checking"; txHash?: Hex };

/** Contract custom errors -> refusal codes (payment-protocol.md 8). */
export const REASONS: Record<string, string> = {
  WrongDomain: "WRONG_DOMAIN",
  Expired: "EXPIRED",
  AccountInactive: "ACCOUNT_INACTIVE",
  MerchantRevoked: "MERCHANT_REVOKED",
  MerchantForged: "MERCHANT_FORGED",
  NonceReplayed: "NONCE_REPLAYED",
  OverCap: "OVER_CAP",
  InsufficientBalance: "INSUFFICIENT_BALANCE",
};

export interface SubmitContext {
  chain: Chain;
  signer: Signer;
  settlement: Hex;
  chainId: bigint;
  /** kioskMinGasBalance in wei (register value). */
  minGasBalance: bigint;
  /** Clock in ms and a sleep, injectable for tests. */
  now?: () => number;
  sleep?: (ms: number) => Promise<void>;
  /** Block to search events from (the deployment block). */
  fromBlock: bigint;
}

export interface Signed {
  auth: Authorization; // with the device's nonce
  signature: string;
  device: string;
  /** Time the payment.prepare request finished (ms); the 10 s budget starts here. */
  requestedAt: number;
}

const BUDGET_MS = 10_000;
const shortHash = (h: string) => `${h.slice(0, 8)}…${h.slice(-4)}`;

/** P04-FR-08: no new order below kioskMinGasBalance. */
export async function gasReady(ctx: SubmitContext): Promise<boolean> {
  return (await ctx.chain.balance(ctx.signer.address)) >= ctx.minGasBalance;
}

/** The finalized PaymentSettled of (merchant, orderId), if any. */
export async function findSettled(ctx: SubmitContext, s: Signed): Promise<Settled | undefined> {
  const logs = await ctx.chain.logs({
    address: ctx.settlement,
    topics: [PAYMENT_SETTLED_TOPIC, topicOfAddress(s.auth.merchant), s.auth.orderId as Hex],
    fromBlock: ("0x" + ctx.fromBlock.toString(16)) as Hex,
    toBlock: "finalized",
  });
  return logs.length ? decodeSettled(logs[0]) : undefined;
}

const ours = (e: Settled, s: Signed) =>
  e.device.toLowerCase() === s.device.toLowerCase() && e.amount === BigInt(s.auth.amount) && e.nonce === BigInt(s.auth.nonce);

type Simulation = { ok: true } | { ok: false; error?: string };

/** eth_call of `data` against the settlement contract from the kiosk address. */
export async function simulate(ctx: SubmitContext, data: Hex): Promise<Simulation> {
  try {
    await ctx.chain.call({ from: ctx.signer.address, to: ctx.settlement, data }, "latest");
    return { ok: true };
  } catch (e) {
    if (e instanceof RpcError) return { ok: false, error: decodeError(e.data) };
    throw e;
  }
}

/** Applies P04-FR-11 to an OrderAlreadyPaid result. */
async function alreadyPaid(ctx: SubmitContext, s: Signed): Promise<Outcome> {
  const e = await findSettled(ctx, s);
  if (e && ours(e, s)) return { status: "approved", txHash: e.txHash, event: e };
  return { status: "refused", reason: "ORDER_ALREADY_PAID" };
}

/** Simulation verdict, or undefined when the payment may be sent. */
async function judgeSimulation(ctx: SubmitContext, s: Signed, data: Hex): Promise<Outcome | undefined> {
  const sim = await simulate(ctx, data);
  if (sim.ok) return undefined;
  if (sim.error === "OrderAlreadyPaid") return alreadyPaid(ctx, s);
  if (sim.error && REASONS[sim.error]) return { status: "refused", reason: REASONS[sim.error] };
  return { status: "failed", reason: sim.error ?? "SIMULATION_FAILED" };
}

/** Signs and sends a type-2 transaction to the settlement contract with the kiosk's gas key. */
export async function sendToSettlement(ctx: SubmitContext, data: Hex): Promise<{ txHash: Hex } | { error: string }> {
  const [nonce, tip, head, estimate] = await Promise.all([
    ctx.chain.pendingNonce(ctx.signer.address),
    ctx.chain.priorityFee(),
    ctx.chain.block("latest"),
    ctx.chain.estimateGas({ from: ctx.signer.address, to: ctx.settlement, data }),
  ]);
  const raw = signTransaction(
    {
      chainId: ctx.chainId,
      nonce,
      maxPriorityFeePerGas: tip,
      maxFeePerGas: head.baseFee * 2n + tip, // StableNet refuses a cap below base fee + its minimum tip
      gas: (estimate * 6n) / 5n,
      to: ctx.settlement,
      data,
    },
    ctx.signer,
  );
  const txHash = transactionHash(raw);
  try {
    await ctx.chain.sendRaw(raw);
  } catch (e) {
    if (e instanceof RpcError) return { error: `not sent: ${e.message}` };
    throw e;
  }
  return { txHash };
}

/** Submits one signed payment and judges it within the 10 s budget (P04-FR-06, 09-12, 14). */
export async function submit(ctx: SubmitContext, s: Signed): Promise<Outcome> {
  const now = ctx.now ?? Date.now;
  const sleep = ctx.sleep ?? ((ms: number) => new Promise<void>((r) => setTimeout(r, ms)));
  const data = encodeSettle(s.auth, s.signature);

  const verdict = await judgeSimulation(ctx, s, data);
  if (verdict) return verdict;

  const sent = await sendToSettlement(ctx, data);
  if ("error" in sent) return { status: "failed", reason: sent.error };
  const { txHash } = sent;

  while (now() - s.requestedAt < BUDGET_MS) {
    const rc = await ctx.chain.receipt(txHash);
    if (rc) {
      if (BigInt(rc.status) !== 1n) {
        // P04-FR-12: simulate once more; OrderAlreadyPaid -> FR-11, otherwise failed.
        const again = await simulate(ctx, data);
        if (!again.ok && again.error === "OrderAlreadyPaid") return alreadyPaid(ctx, s);
        return { status: "failed", reason: `reverted ${shortHash(txHash)}`, txHash };
      }
      const finalized = await ctx.chain.block("finalized");
      if (BigInt(rc.blockNumber) <= finalized.number) {
        const log = rc.logs.find((l) => l.topics[0] === PAYMENT_SETTLED_TOPIC);
        if (log) {
          const e = decodeSettled(log);
          if (ours(e, s)) return { status: "approved", txHash, event: e };
        }
        return { status: "failed", reason: `no PaymentSettled in ${shortHash(txHash)}`, txHash };
      }
    }
    await sleep(500);
  }
  return { status: "Checking", txHash }; // P04-FR-14: block the order, do not ask for a new signature
}

/**
 * One round of Checking (P04-FR-14, 15): look for the event; when there is none, re-simulate
 * the same signature. Expired after the signature's expiry -> failed, and the order may be paid
 * again. Returns Checking while it is still open.
 */
export async function recheck(ctx: SubmitContext, s: Signed): Promise<Outcome> {
  const e = await findSettled(ctx, s);
  if (e) return ours(e, s) ? { status: "approved", txHash: e.txHash, event: e } : { status: "refused", reason: "ORDER_ALREADY_PAID" };
  const finalized = await ctx.chain.block("finalized");
  if (finalized.timestamp > BigInt(s.auth.expiry)) {
    const sim = await simulate(ctx, encodeSettle(s.auth, s.signature));
    if (!sim.ok && sim.error === "Expired") return { status: "failed", reason: "EXPIRED" };
  }
  return { status: "Checking" };
}
