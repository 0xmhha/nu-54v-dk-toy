// A software stand-in for the payment device (payment-protocol.md 5 and 6).
//
// It answers the same messages the firmware does, with the same checks and refusal reasons,
// and signs with a software key instead of the TF-M secure partition. The kiosk uses it to
// run the whole payment flow before the firmware is ready; the firmware team uses it as the
// reference for what each message should produce. It is a test tool: it never runs on a
// device and its key is a test key.

import {
  addressOfPrivateKey,
  bytesToHex,
  digest,
  hexToBytes,
  recoverSigner,
  signDigest,
  type Domain,
  type Message,
} from "@nu54/protocol";

export type DeviceState = "UNPROVISIONED" | "PROVISIONED_NO_ANCHOR" | "READY" | "PIN_LOCKED";

export interface DeviceConfig {
  /** Device key (test key). */
  key: Uint8Array;
  /** Setup values recorded by setup.operator: operator address, settlement contract, chain id. */
  operator: string;
  contract: string;
  chainId: bigint | number;
  /** Register parameters (seconds). */
  anchorClockSkew?: number;
  authorizationExpiry?: number;
  /** First nonce; a multiple of 256 chosen at setup (payment-protocol.md 2). */
  nonceStart?: bigint;
  /** Clock in seconds since the epoch; the device's time is the anchor plus the elapsed time. */
  now?: () => number;
  /** The renter's button: approve or reject a payment shown on the phone. */
  approve?: (show: Message) => boolean;
  firmware?: string;
  /** Random bytes (deviceNonce); injectable so session vectors are deterministic. */
  random?: (n: number) => Uint8Array;
}

const FIRMWARE = "sim-0.1.0";

export class SoftwareDevice {
  readonly address: string;
  state: DeviceState = "PROVISIONED_NO_ANCHOR";
  /** Messages the device sent to the phone app (confirm.show). */
  readonly phone: Message[] = [];
  private readonly cfg: Required<Omit<DeviceConfig, "nonceStart">> & { nonceStart: bigint };
  private anchor: { timestamp: number; at: number } | null = null;
  private lastAnchor = 0;
  private nextNonce: bigint;
  private session: { id: string; mode: string; deviceNonce: string; confirmed: boolean } | null = null;
  private attestation: { merchant: string; payout: string; name: string } | null = null;

  constructor(cfg: DeviceConfig) {
    this.cfg = {
      anchorClockSkew: 60,
      authorizationExpiry: 120,
      now: () => Math.floor(Date.now() / 1000),
      approve: () => true,
      firmware: FIRMWARE,
      random: (n: number) => globalThis.crypto.getRandomValues(new Uint8Array(n)),
      nonceStart: 0n,
      ...cfg,
    } as SoftwareDevice["cfg"];
    if (this.cfg.nonceStart % 256n !== 0n) throw new Error("nonceStart must be a multiple of 256");
    this.nextNonce = this.cfg.nonceStart;
    this.address = addressOfPrivateKey(cfg.key);
  }

  private get domain(): Domain {
    return { chainId: this.cfg.chainId, verifyingContract: this.cfg.contract };
  }

  /** Device time: last anchor plus the elapsed time since it was accepted. */
  private time(): number | null {
    return this.anchor ? this.anchor.timestamp + (this.cfg.now() - this.anchor.at) : null;
  }

  /** A RAM-clearing reset: the anchor is lost, key and deposit stay (payment-protocol.md 5). */
  powerCycle(): void {
    this.anchor = null;
    this.session = null;
    if (this.state === "READY") this.state = "PROVISIONED_NO_ANCHOR";
  }

  private reply(type: string, fields: Record<string, unknown>): Message {
    return { v: 1, type, sessionId: this.session?.id ?? "0000000000000000", ...fields } as Message;
  }

  private error(reason: string, sessionId?: string): Message {
    return { v: 1, type: "error", sessionId: sessionId ?? this.session?.id ?? "0000000000000000", reason } as Message;
  }

  private refused(reason: string): Message {
    return this.reply("payment.result", { outcome: "refused", reason });
  }

  /** Handles one message from a central and returns the messages the device sends back. */
  handle(m: Message): Message[] {
    switch (m.type) {
      case "session.open":
        return [this.open(m)];
      case "session.confirm":
        return this.confirm(m);
      case "session.cancel":
        this.session = null;
        return [];
      case "setup.timeAnchor":
        return [this.timeAnchor(m)];
      case "payment.identify":
        return [this.identify(m)].filter((x): x is Message => x !== null);
      case "payment.prepare":
        return this.prepare(m);
      case "payment.outcome":
        return [];
      case "setup.operator":
      case "device.reset":
      case "limit.change":
        return [this.error("NOT_PERMITTED", String(m.sessionId))]; // not simulated yet
      default:
        return [this.error("UNSUPPORTED_TYPE", String(m.sessionId))];
    }
  }

  private open(m: Message): Message {
    const mode = String(m.mode);
    if (mode === "setup" && this.state !== "UNPROVISIONED" && this.state !== "PROVISIONED_NO_ANCHOR") {
      return this.error("NOT_PERMITTED", String(m.sessionId));
    }
    const deviceNonce = bytesToHex(this.cfg.random(32));
    this.session = { id: String(m.sessionId), mode, deviceNonce, confirmed: false };
    this.attestation = null;
    return this.reply("session.open.ok", {
      device: this.address,
      deviceNonce,
      anchorValid: this.anchor !== null,
      firmware: this.cfg.firmware,
      state: this.state,
      lastAnchor: String(this.lastAnchor),
    });
  }

  private confirm(m: Message): Message[] {
    if (!this.session || m.sessionId !== this.session.id || String(m.deviceNonce).toLowerCase() !== this.session.deviceNonce) {
      this.session = null;
      return [this.error("NOT_PERMITTED", String(m.sessionId))];
    }
    this.session.confirmed = true;
    return [];
  }

  private inSession(m: Message, mode: string): boolean {
    return !!this.session && this.session.id === m.sessionId && this.session.mode === mode && (mode === "setup" || this.session.confirmed);
  }

  private timeAnchor(m: Message): Message {
    const ack = (accepted: boolean, reason?: string) =>
      this.reply("setup.ack", { step: "setup.timeAnchor", accepted, ...(reason ? { reason } : {}), lastAnchor: String(this.lastAnchor) });
    if (!this.inSession(m, "setup")) return this.error("NOT_PERMITTED", String(m.sessionId));
    const ts = Number(m.timestamp);
    let signer: string;
    try {
      signer = recoverSigner(
        digest(this.domain, "TimeAnchor", { device: String(m.device), timestamp: BigInt(ts) }),
        hexToBytes(String(m.operatorSignature)),
      );
    } catch {
      return ack(false, "NOT_PERMITTED");
    }
    // Refused anchors (other device, not the recorded operator, not strictly later) are NOT_PERMITTED.
    if (String(m.device).toLowerCase() !== this.address || signer !== this.cfg.operator.toLowerCase()) return ack(false, "NOT_PERMITTED");
    if (ts <= this.lastAnchor) return ack(false, "NOT_PERMITTED");
    this.anchor = { timestamp: ts, at: this.cfg.now() };
    this.lastAnchor = ts;
    if (this.state === "PROVISIONED_NO_ANCHOR") this.state = "READY";
    return ack(true);
  }

  private identify(m: Message): Message | null {
    if (!this.inSession(m, "payment")) return this.error("NOT_PERMITTED", String(m.sessionId));
    const a = m.attestation as Record<string, string>;
    const t = this.time();
    if (t === null) return this.refused("TIME_ANCHOR_MISSING");
    let signer: string;
    try {
      const { operatorSignature, ...fields } = a;
      signer = recoverSigner(digest(this.domain, "MerchantAttestation", fields), hexToBytes(operatorSignature));
    } catch {
      return this.refused("MERCHANT_FORGED");
    }
    if (signer !== this.cfg.operator.toLowerCase()) return this.refused("MERCHANT_FORGED");
    const skew = this.cfg.anchorClockSkew;
    if (t + skew < Number(a.validFrom) || t - skew > Number(a.validUntil)) return this.refused("ATTESTATION_EXPIRED");
    this.attestation = { merchant: a.merchant.toLowerCase(), payout: a.payout.toLowerCase(), name: a.name };
    return null; // accepted: the device answers payment.prepare
  }

  private prepare(m: Message): Message[] {
    if (!this.inSession(m, "payment")) return [this.error("NOT_PERMITTED", String(m.sessionId))];
    const t = this.time();
    if (t === null || this.state !== "READY") return [this.refused("TIME_ANCHOR_MISSING")];
    const att = this.attestation;
    if (!att) return [this.refused("MERCHANT_FORGED")];
    const auth = m.authorization as Record<string, string>;
    const order = { orderId: auth.orderId, token: auth.token, amount: auth.amount, payout: auth.payout, expiry: auth.expiry };
    let orderSigner: string;
    try {
      orderSigner = recoverSigner(digest(this.domain, "MerchantOrder", order), hexToBytes(String(m.merchantSignature)));
    } catch {
      return [this.refused("MERCHANT_FORGED")];
    }
    const same = (a: string, b: string) => a.toLowerCase() === b.toLowerCase();
    if (
      orderSigner !== att.merchant ||
      !same(auth.merchant, att.merchant) ||
      !same(auth.payout, att.payout) ||
      BigInt(auth.chainId) !== BigInt(this.cfg.chainId) ||
      !same(auth.contract, this.cfg.contract)
    ) {
      return [this.refused("MERCHANT_FORGED")];
    }
    const expiry = Number(auth.expiry);
    if (expiry < t || expiry > t + this.cfg.authorizationExpiry) return [this.refused("ATTESTATION_EXPIRED")];

    const show = this.reply("confirm.show", {
      merchantName: att.name,
      orderId: auth.orderId,
      token: auth.token,
      payout: auth.payout,
      amount: auth.amount,
    });
    this.phone.push(show);
    if (!this.cfg.approve(show)) return [this.refused("USER_REJECTED")];

    const nonce = this.nextNonce++; // sequential: consecutive nonces share one bitmap slot
    const sig = signDigest(digest(this.domain, "PaymentAuthorization", { ...auth, nonce }), this.cfg.key);
    return [this.reply("payment.result", { outcome: "approved", signature: bytesToHex(sig), nonce: nonce.toString() })];
  }
}
