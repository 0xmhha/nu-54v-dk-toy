// The kiosk relays a renter's limit change (payment-protocol.md 5, WBS2-P04-03).
//
//   session.open(payment) [secure channel, 4.1] -> session.confirm -> limit.change{change}
//   device: confirm.limit to the phone, PIN, button -> limit.result approved (signature, nonce)
//   kiosk: check the signer, simulate setLimits, send it, wait for LimitsChanged at finalized
//
// The renter enters the PIN on the device buttons, so the wait is long (60 s). Submitting the
// same signature again is refused by the contract as NonceReplayed (the NONCE_REPLAYED demo).

import { bytesToHex, digest, hexToBytes, recoverSigner, type Domain, type Message } from "@nu54/protocol";
import type { MessageLink } from "../ble/framing.ts";
import type { Hex } from "../chain/rpc.ts";
import { encodeSetLimits, LIMITS_CHANGED_TOPIC, type LimitChange } from "../chain/settlement.ts";
import { REASONS, sendToSettlement, simulate, type SubmitContext } from "./submit.ts";
import { openPaymentSession, type KioskKeySigner } from "./open.ts";

export interface LimitRequest {
  domain: Domain;
  /** New limits in token base units; 0 means "the cap" (payment-protocol.md 2). */
  perPaymentLimit: bigint;
  dailyLimit: bigint;
  /** Chain time (s) the signature expires; within authorizationExpiry of the device clock. */
  expiry: bigint;
  random: (n: number) => Uint8Array;
  /** How long to wait for the PIN and the button. Default 60 s. */
  waitMs?: number;
  /** The merchant attestation and KioskKey signer: the session runs over the secure channel (4.1). */
  secure?: KioskKeySigner;
}

export type LimitSession =
  | { status: "approved"; sessionId: string; device: string; change: LimitChange; signature: string }
  | { status: "refused"; reason: string }
  | { status: "cancelled" };

export async function runLimitChange(link: MessageLink, req: LimitRequest): Promise<LimitSession> {
  const sessionId = bytesToHex(req.random(8), false);
  const msg = (type: string, fields: Record<string, unknown> = {}) => ({ v: 1, type, sessionId, ...fields }) as Message;
  const opened = await openPaymentSession(link, sessionId, req.random, req.secure);
  if ("refused" in opened) return { status: "refused", reason: opened.refused };
  const ok = opened.ok;
  const confirmErr = await link.send(msg("session.confirm", { deviceNonce: ok.deviceNonce }), 1, 300);
  if (confirmErr.length) return { status: "refused", reason: String(confirmErr[0].reason) };

  const change = {
    chainId: String(req.domain.chainId), contract: req.domain.verifyingContract,
    perPaymentLimit: req.perPaymentLimit.toString(), dailyLimit: req.dailyLimit.toString(), expiry: req.expiry.toString(),
  };
  const r = (await link.send(msg("limit.change", { change }), 1, req.waitMs ?? 60_000))[0];
  if (!r) {
    await link.send(msg("session.cancel"), 0, 0);
    return { status: "cancelled" };
  }
  if (r.type !== "limit.result" || r.outcome !== "approved") return { status: "refused", reason: String(r.reason ?? r.type) };
  const signed: LimitChange = { ...change, nonce: String(r.nonce) };
  // The kiosk checks the signer before spending gas: the device the session opened with.
  let signer: string;
  try {
    signer = recoverSigner(digest(req.domain, "LimitChange", { ...signed, nonce: BigInt(signed.nonce) }), hexToBytes(String(r.signature)));
  } catch {
    return { status: "refused", reason: "BAD_SIGNATURE" };
  }
  if (signer !== String(ok.device).toLowerCase()) return { status: "refused", reason: "BAD_SIGNATURE" };
  return { status: "approved", sessionId, device: signer, change: signed, signature: String(r.signature) };
}

export type LimitOutcome =
  | { status: "approved"; txHash: Hex }
  | { status: "refused"; reason: string }
  | { status: "failed"; reason: string; txHash?: Hex };

/** Simulates and sends setLimits, then waits for LimitsChanged in a finalized block. */
export async function submitLimits(ctx: SubmitContext, change: LimitChange, signature: string, waitMs = 30_000): Promise<LimitOutcome> {
  const now = ctx.now ?? Date.now;
  const sleep = ctx.sleep ?? ((ms: number) => new Promise<void>((r) => setTimeout(r, ms)));
  const data = encodeSetLimits(change, signature);
  const sim = await simulate(ctx, data);
  if (!sim.ok) {
    return sim.error && REASONS[sim.error] ? { status: "refused", reason: REASONS[sim.error] } : { status: "failed", reason: sim.error ?? "SIMULATION_FAILED" };
  }
  const sent = await sendToSettlement(ctx, data);
  if ("error" in sent) return { status: "failed", reason: sent.error };
  const start = now();
  while (now() - start < waitMs) {
    const rc = await ctx.chain.receipt(sent.txHash);
    if (rc) {
      if (BigInt(rc.status) !== 1n) return { status: "failed", reason: "reverted", txHash: sent.txHash };
      const finalized = await ctx.chain.block("finalized");
      if (BigInt(rc.blockNumber) <= finalized.number) {
        return rc.logs.some((l) => l.topics[0] === LIMITS_CHANGED_TOPIC)
          ? { status: "approved", txHash: sent.txHash }
          : { status: "failed", reason: "no LimitsChanged", txHash: sent.txHash };
      }
    }
    await sleep(500);
  }
  return { status: "failed", reason: "not final in time", txHash: sent.txHash };
}
