// One payment from the kiosk screen: gas check, device session, submission, outcome to the
// device (P04 design 3 and 4).
//
//   gas below kioskMinGasBalance -> busy (no order taken, P04-FR-08)
//   find and connect the device -> payment session (session.ts) -> approved signature
//   -> submit and judge (submit.ts) -> payment.outcome to the device (P04-FR-16) -> disconnect

import { addressOfPrivateKey, REASONS, signDigest, type Message } from "@nu54/protocol";
import type { MessageLink } from "../ble/framing.ts";
import type { Chain, Hex } from "../chain/rpc.ts";
import { runPayment, type SessionStep, type TimeAnchor } from "../payment/session.ts";
import { signKioskKey, signMerchantOrder } from "../payment/signing.ts";
import { gasReady, resume, submit, type Outcome, type Signed, type SubmitContext } from "../payment/submit.ts";
import { runLimitChange, submitLimits, type LimitOutcome } from "../payment/limits.ts";
import type { Loaded } from "./config.ts";
import type { OrderRecord, OrderStore } from "./orders.ts";

export type Phase = "checkingGas" | "connecting" | SessionStep | "submitting";

export type PayResult =
  | Exclude<Outcome, { status: "Checking" }>
  /** Submitted but not judged within 10 s: no new signature, re-check with recheck(signed). */
  | (Extract<Outcome, { status: "Checking" }> & { signed: Signed })
  | { status: "busy" }
  | { status: "cancelled" }
  | { status: "noDevice"; reason: string };

export interface PayDeps {
  kiosk: Loaded;
  chain: Chain;
  /** Finds the device and opens a message link to it. */
  connect: () => Promise<MessageLink>;
  /** The TimeAnchor waiting for the device, if one was pushed (week-7 development setup). */
  anchor: () => Promise<TimeAnchor | undefined>;
  /** The device took the anchor (or it is too old): it is not offered again. */
  anchorUsed?: () => Promise<void>;
  random: (n: number) => Uint8Array;
  /** Submission; tests replace it. */
  submit?: typeof submit;
  /** Where orders survive a restart (P04-NFR-05); optional in tests. */
  orders?: OrderStore;
}

export function submitContext(deps: PayDeps): SubmitContext {
  const { config, gasKey } = deps.kiosk;
  return {
    chain: deps.chain,
    signer: { address: addressOfPrivateKey(gasKey) as Hex, sign: (d) => signDigest(d, gasKey) },
    settlement: config.settlement,
    chainId: BigInt(config.chainId),
    minGasBalance: BigInt(config.minGasBalanceWei),
    fromBlock: BigInt(config.fromBlock),
  };
}

export async function pay(deps: PayDeps, amount: bigint, onPhase: (p: Phase) => void = () => {}): Promise<PayResult> {
  const { config, merchantKey } = deps.kiosk;
  const ctx = submitContext(deps);
  onPhase("checkingGas");
  if (!(await gasReady(ctx))) return { status: "busy" };

  onPhase("connecting");
  let link: MessageLink;
  try {
    link = await deps.connect();
  } catch (e) {
    return { status: "noDevice", reason: e instanceof Error ? e.message : String(e) };
  }
  let orderId: string | undefined;
  const record = async (e: Parameters<OrderStore["apply"]>[1]) => {
    if (deps.orders && orderId) await deps.orders.apply(orderId, e);
  };
  try {
    const domain = { chainId: config.chainId, verifyingContract: config.settlement };
    const finalized = await deps.chain.block("finalized");
    const session = await runPayment(link, {
      domain,
      attestation: config.attestation,
      anchor: await deps.anchor(),
      chainTime: finalized.timestamp,
      onAnchorUsed: deps.anchorUsed,
      token: config.token,
      amount,
      expiry: finalized.timestamp + 60n, // capped to the device clock when this session sets it
      signOrder: (o) => signMerchantOrder(domain, o, merchantKey),
      signKioskKey: (v) => signKioskKey(domain, v, merchantKey), // every kiosk session is secure (4.1)
      random: deps.random,
      onStep: onPhase,
      onOrder: async (id) => {
        orderId = id;
        await deps.orders?.create(id, amount);
      },
    });
    if (session.status === "cancelled") {
      await record({ type: "cancelled" });
      return { status: "cancelled" };
    }
    if (session.status === "refused") {
      await record({ type: "refused", reason: session.reason });
      return { status: "refused", reason: session.reason };
    }

    onPhase("submitting");
    const { sessionId, auth, signature, device, requestedAt } = session;
    const signed: Signed = { auth, signature, device, requestedAt };
    await record({ type: "signed", signed }); // before submitting: a crash keeps the signature
    const out = await (deps.submit ?? submit)(ctx, signed);
    await record({ type: "outcome", outcome: out });
    if (out.status === "Checking") return { ...out, signed };
    // P04-FR-16: tell the device the final result; it does not answer. reason is a protocol
    // code; a failure described in words is sent without one.
    const reason = "reason" in out && (REASONS as readonly string[]).includes(out.reason) ? { reason: out.reason } : {};
    const outcome = { v: 1, type: "payment.outcome", sessionId, orderId: auth.orderId, outcome: out.status, ...reason };
    await link.send(outcome as Message, 0, 0).catch(() => undefined);
    return out;
  } finally {
    await link.close().catch(() => undefined);
  }
}

export type LimitPhase = "connecting" | "waitingDevice" | "submitting";
export type LimitResult = LimitOutcome | { status: "cancelled" } | { status: "noDevice"; reason: string };

/**
 * A renter's limit change from the kiosk screen: the device asks for the PIN and the button,
 * the kiosk submits setLimits (payment-protocol.md 5). Limits are base units; 0 = the cap.
 */
export async function changeLimits(
  deps: PayDeps & { submitLimits?: typeof submitLimits },
  perPaymentLimit: bigint,
  dailyLimit: bigint,
  onPhase: (p: LimitPhase) => void = () => {},
): Promise<LimitResult> {
  const { config, merchantKey } = deps.kiosk;
  onPhase("connecting");
  let link: MessageLink;
  try {
    link = await deps.connect();
  } catch (e) {
    return { status: "noDevice", reason: e instanceof Error ? e.message : String(e) };
  }
  try {
    const finalized = await deps.chain.block("finalized");
    onPhase("waitingDevice");
    const domain = { chainId: config.chainId, verifyingContract: config.settlement };
    const s = await runLimitChange(link, {
      domain, perPaymentLimit, dailyLimit, expiry: finalized.timestamp + 60n, random: deps.random,
      secure: { attestation: config.attestation, sign: (v) => signKioskKey(domain, v, merchantKey) },
    });
    if (s.status !== "approved") return s;
    onPhase("submitting");
    return await (deps.submitLimits ?? submitLimits)(submitContext(deps), s.change, s.signature);
  } finally {
    await link.close().catch(() => undefined);
  }
}

/**
 * After a restart: every open order is picked up again (P04 design 6). A signed order is resumed
 * with its own signature (re-simulated, resent if it would still settle); an order that never got
 * a signature is cancelled, so it may be paid again.
 */
export async function resumeOrders(deps: Pick<PayDeps, "kiosk" | "chain" | "orders">, resumeOne: typeof resume = resume): Promise<OrderRecord[]> {
  const store = deps.orders;
  if (!store) return [];
  const ctx = submitContext({ ...deps, connect: async () => { throw new Error("no device needed"); }, anchor: async () => undefined, random: () => new Uint8Array() });
  const out: OrderRecord[] = [];
  for (const o of store.open()) {
    if (!o.signed) {
      out.push(await store.apply(o.orderId, { type: "cancelled" }));
      continue;
    }
    const r = await resumeOne(ctx, o.signed);
    out.push(await store.apply(o.orderId, { type: "outcome", outcome: r }));
  }
  return out;
}
