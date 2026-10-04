// The kiosk side of one payment session with a device (payment-protocol.md 5 and 6).
//
//   [setup session: TimeAnchor]  week-7 development fixed setup only (N30)
//   session.open(payment) [secure channel, 4.1] -> session.confirm -> payment.identify -> payment.prepare
//   -> payment.result approved (signature, nonce) | refused (reason)
//
// The device answers payment.prepare only after the renter presses its button. If no result
// arrives within the wait (10 s from the request, N10) the kiosk sends session.cancel and the
// order is cancelled; it may be paid again.

import { bytesToHex, type Domain, type Message } from "@nu54/protocol";
import type { MessageLink } from "../ble/framing.ts";
import { checkDeviceAuthorization } from "./signing.ts";
import type { Authorization } from "../chain/settlement.ts";
import { openPaymentSession, type Attestation, type KioskKeySigner } from "./open.ts";

export type { Attestation, KioskKeySigner } from "./open.ts";

/** TimeAnchor as opsctl signs it. */
export interface TimeAnchor {
  device: string;
  timestamp: string;
  operatorSignature: string;
}

export interface PaymentRequest {
  domain: Domain;
  attestation: Attestation;
  token: string;
  amount: bigint;
  /** Chain time (s) the order expires; the device refuses more than authorizationExpiry ahead. */
  expiry: bigint;
  /** Signs the MerchantOrder with the merchant key (signMerchantOrder in the app). */
  signOrder: (order: { orderId: string; token: string; amount: string; payout: string; expiry: string }) => string;
  /** Signs the session's one-time key with the merchant key: the session runs over the secure channel (4.1). */
  signKioskKey?: KioskKeySigner["sign"];
  /** Week-7 development setup: the anchor the kiosk hands over before the payment session. */
  anchor?: TimeAnchor;
  /**
   * Chain time (s, the finalized block) when the payment started. With an anchor it lets the kiosk
   * refuse an anchor too old to use and keep the expiry inside the device's clock (see below).
   */
  chainTime?: bigint;
  /** The anchor reached the device (accepted, or the device already had its time): stop offering it. */
  onAnchorUsed?: () => void | Promise<void>;
  /** How long to wait for the press (N10: 10 s). */
  waitMs?: number;
  /** Cryptographic random bytes (session id, kiosk nonce, order id); the app's comes from SecureRandom. */
  random: (n: number) => Uint8Array;
  /** Progress for the screen. */
  onStep?: (step: SessionStep) => void;
  /** Called with the new order id before payment.prepare is sent (the kiosk records the order). */
  onOrder?: (orderId: string) => void | Promise<void>;
}

export type SessionStep = "anchor" | "opening" | "identifying" | "waitingDevice";

export type SessionResult =
  | { status: "approved"; sessionId: string; device: string; auth: Authorization; signature: string; requestedAt: number }
  | { status: "refused"; reason: string; detail?: string }
  | { status: "cancelled"; orderId: string };

const WAIT_MS = 10_000;
const REPLY_MS = 3_000;

/*
 * The device's clock is the anchor's timestamp plus the time since it accepted the anchor, so it
 * runs behind the chain by however long the anchor waited before delivery. The device signs only
 * expiries within authorizationExpiry of its own clock (payment-protocol.md 2), and the contract
 * only those not yet past on the chain. An anchor older than ANCHOR_MAX_AGE_S leaves no room
 * between the two, so the kiosk does not send it (TIME_ANCHOR_STALE: issue a new one); a younger
 * one is sent and the expiry is capped to the device's clock.
 */
export const AUTHORIZATION_EXPIRY_S = 120; // register parameter authorizationExpiry
export const ANCHOR_MAX_AGE_S = 90;
const CLOCK_MARGIN_S = 5;
const MIN_SETTLE_S = 20; // the expiry must leave this long to submit and settle

export async function runPayment(link: MessageLink, req: PaymentRequest): Promise<SessionResult> {
  const random = req.random;
  const hex = (n: number) => bytesToHex(random(n));
  const sessionId = bytesToHex(random(8), false);
  const msg = (type: string, fields: Record<string, unknown> = {}) => ({ v: 1, type, sessionId, ...fields }) as Message;
  const refusedBy = (m: Message | undefined, step: string): SessionResult => ({
    status: "refused",
    reason: String(m?.reason ?? "NO_REPLY"),
    detail: m ? `${step}: ${m.type}` : `${step}: no reply`,
  });

  const started = Date.now();
  const chainNow = () => (req.chainTime ?? 0n) + BigInt(Math.floor((Date.now() - started) / 1000));
  /** Device clock as of now, when this session handed it the anchor. */
  let deviceClock: (() => bigint) | null = null;

  await link.endSession?.();
  link.secure?.(null);
  if (req.anchor) {
    const anchorTime = BigInt(req.anchor.timestamp);
    if (req.chainTime !== undefined && req.chainTime - anchorTime > BigInt(ANCHOR_MAX_AGE_S)) {
      await req.onAnchorUsed?.(); // useless now: a new one has to be issued
      return { status: "refused", reason: "TIME_ANCHOR_STALE", detail: `the TimeAnchor is ${req.chainTime - anchorTime} s old; issue a new one` };
    }
    req.onStep?.("anchor");
    const opened = (await link.send(msg("session.open", { mode: "setup", kioskNonce: hex(32) }), 1, REPLY_MS))[0];
    // NOT_PERMITTED on a setup session means the device is already READY: keep its anchor.
    if (opened?.type === "error" && opened.reason === "NOT_PERMITTED") {
      await req.onAnchorUsed?.();
    } else {
      if (opened?.type !== "session.open.ok") return refusedBy(opened, "setup session");
      const ack = (await link.send(msg("setup.timeAnchor", { ...req.anchor }), 1, REPLY_MS))[0];
      if (!ack?.accepted) return refusedBy(ack, "time anchor");
      await req.onAnchorUsed?.();
      const acceptedAt = Date.now();
      deviceClock = () => anchorTime + BigInt(Math.floor((Date.now() - acceptedAt) / 1000));
    }
  }

  req.onStep?.("opening");
  const secure = req.signKioskKey ? { attestation: req.attestation, sign: req.signKioskKey } : undefined;
  const opened = await openPaymentSession(link, sessionId, random, secure);
  if ("refused" in opened) return { status: "refused", reason: opened.refused, detail: opened.detail };
  const ok = opened.ok;
  const device = String(ok.device);
  // The device answers session.confirm and an accepted payment.identify with nothing.
  const confirmErr = await link.send(msg("session.confirm", { deviceNonce: ok.deviceNonce }), 1, 300);
  if (confirmErr.length) return refusedBy(confirmErr[0], "confirm");
  req.onStep?.("identifying");
  const identifyErr = await link.send(msg("payment.identify", { attestation: req.attestation }), 1, 800);
  if (identifyErr.length) return refusedBy(identifyErr[0], "identify");

  // Inside the device's window when this session set its clock, and still ahead of the chain.
  let until = req.expiry;
  if (deviceClock) {
    const cap = deviceClock() + BigInt(AUTHORIZATION_EXPIRY_S - CLOCK_MARGIN_S);
    if (cap < until) until = cap;
    if (req.chainTime !== undefined && until < chainNow() + BigInt(MIN_SETTLE_S)) {
      return { status: "refused", reason: "TIME_ANCHOR_STALE", detail: "the device clock is too far behind the chain; issue a new TimeAnchor" };
    }
  }

  const auth: Authorization = {
    chainId: String(req.domain.chainId),
    contract: req.domain.verifyingContract,
    merchant: req.attestation.merchant,
    payout: req.attestation.payout,
    token: req.token,
    amount: req.amount.toString(),
    orderId: hex(32),
    nonce: "",
    expiry: until.toString(),
  };
  const { orderId, token, amount, payout, expiry } = auth;
  const merchantSignature = req.signOrder({ orderId, token, amount, payout, expiry });
  const { chainId, contract, merchant } = auth;
  const authorization = { chainId, contract, merchant, payout, token, amount, orderId, expiry }; // the device adds the nonce
  await req.onOrder?.(orderId);
  req.onStep?.("waitingDevice");
  const requestedAt = Date.now();
  const result = (await link.send(msg("payment.prepare", { authorization, merchantSignature }), 1, req.waitMs ?? WAIT_MS))[0];
  if (!result) {
    await link.send(msg("session.cancel"), 0, 0);
    return { status: "cancelled", orderId };
  }
  if (result.type !== "payment.result" || result.outcome !== "approved") return refusedBy(result, "prepare");

  const signed = { signature: String(result.signature), nonce: String(result.nonce) };
  const check = checkDeviceAuthorization(req.domain, authorization, signed, device);
  if (!check.ok) return { status: "refused", reason: check.reason, detail: "device signature check" };
  return { status: "approved", sessionId, device, auth: { ...auth, nonce: signed.nonce }, signature: signed.signature, requestedAt };
}
