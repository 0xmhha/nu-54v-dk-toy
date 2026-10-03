// A software stand-in for the payment device (payment-protocol.md 5 and 6).
//
// Setup (5): setup.operator with the renter's confirmation, key generation, PIN, TimeAnchor and
// device.reset. Payments (6): the payment session with its step-4 checks, in plaintext or over
// the secure channel of 4.1 (handleBody opens and seals the kiosk link's bodies).
//
// It answers the same messages the firmware does, with the same checks and refusal reasons,
// and signs with a software key instead of the TF-M secure partition. The kiosk uses it to
// run the whole payment flow before the firmware is ready; the firmware team uses it as the
// reference for what each message should produce. It is a test tool: it never runs on a
// device and its key is a test key.

import {
  addressOfPrivateKey,
  bytesToHex,
  decodeMessage,
  digest,
  encodeMessage,
  ephemeralKey,
  hexToBytes,
  isPointX,
  ProtocolError,
  recoverSigner,
  SecureChannel,
  sessionKey,
  signDigest,
  type Domain,
  type Message,
} from "@nu54/protocol";

export type DeviceState = "UNPROVISIONED" | "PROVISIONED_NO_ANCHOR" | "READY" | "PIN_LOCKED";

export interface DeviceConfig {
  /**
   * A provisioned device: its key (test key) and the values setup.operator recorded (operator
   * address, settlement contract, chain id). Without them the device starts UNPROVISIONED and
   * gets them through a setup session.
   */
  key?: Uint8Array;
  operator?: string;
  contract?: string;
  chainId?: bigint | number;
  /** Register parameters (seconds; pinMaxRetries a count). */
  anchorClockSkew?: number;
  authorizationExpiry?: number;
  pinMaxRetries?: number;
  /** The PIN a provisioned device was set up with (setup records it otherwise). */
  pin?: string;
  /** First nonce; a multiple of 256 chosen at setup (payment-protocol.md 2). */
  nonceStart?: bigint;
  /** Clock in seconds since the epoch; the device's time is the anchor plus the elapsed time. */
  now?: () => number;
  /**
   * The renter's button: approve or reject a payment shown on the phone. A promise models a
   * real press (for example from a terminal); only handleAsync accepts it.
   */
  approve?: (show: Message) => boolean | Promise<boolean>;
  /** The renter's button for setup.operator: confirm or refuse the operator values. */
  confirmSetup?: (values: SetupValues) => boolean | Promise<boolean>;
  /** The PIN the renter enters on the buttons (at setup and for a limit change); null when it was not finished in time. */
  enterPin?: () => string | null | Promise<string | null>;
  firmware?: string;
  /** Random bytes (deviceNonce, one-time key, new key, nonce start); injectable so session vectors are deterministic. */
  random?: (n: number) => Uint8Array;
  /** Release build after both sides have the secure channel: plaintext payment sessions are refused (4.1). */
  requireSecureSession?: boolean;
}

export interface SetupValues {
  operator: string;
  contract: string;
  chainId: bigint;
  passkey: number;
}

/** What a step waits for: a button or the PIN. A promise is a real press (handleAsync only). */
type Wait = boolean | string | null | Promise<boolean | string | null>;
/** A step yields early messages ({send}) or waits ({wait}) and returns the remaining replies. */
type Step = Generator<{ send: Message } | { wait: Wait }, Message[], boolean | string | null>;

const SECP256K1_N = 0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141n;

const FIRMWARE = "sim-0.1.0";

/** Web Crypto of Node; typed here so the package also typechecks under the React Native lib. */
const webCrypto = () => (globalThis as unknown as { crypto: { getRandomValues<T extends Uint8Array>(a: T): T } }).crypto;

export class SoftwareDevice {
  state: DeviceState;
  /** Messages the device sent to the phone app (confirm.show, the forwarded payment.outcome). */
  readonly phone: Message[] = [];
  /** Called for each message to the phone app as it is sent (the BLE peripheral routes it). */
  onPhone: ((m: Message) => void) | null = null;

  private toPhone(m: Message): void {
    this.phone.push(m);
    this.onPhone?.(m);
  }
  private readonly cfg: Required<Omit<DeviceConfig, "nonceStart" | "key" | "operator" | "contract" | "chainId" | "pin">>;
  /** Values that setup records and device.reset wipes. */
  private setup: { key: Uint8Array; address: string; operator: string; contract: string; chainId: bigint; passkey?: number; pin?: string } | null;
  private anchor: { timestamp: number; at: number } | null = null;
  private lastAnchor = 0;
  /** Wrong PIN entries in a row; kept in secure storage on the board, so a reset of RAM keeps it. */
  private pinFailures = 0;
  private nextNonce: bigint;
  /** The open session; a secure one has its channel and the merchant session.open proved (4.1). */
  private session: { id: string; mode: string; deviceNonce: string; confirmed: boolean; channel?: SecureChannel; merchant?: string } | null = null;
  private attestation: { merchant: string; payout: string; name: string } | null = null;

  constructor(cfg: DeviceConfig) {
    const { key, operator, contract, chainId, nonceStart = 0n, pin = "2580", ...rest } = cfg;
    this.cfg = {
      anchorClockSkew: 60,
      authorizationExpiry: 120,
      pinMaxRetries: 5,
      now: () => Math.floor(Date.now() / 1000),
      approve: () => true,
      confirmSetup: () => true,
      enterPin: () => "2580",
      firmware: FIRMWARE,
      random: (n: number) => webCrypto().getRandomValues(new Uint8Array(n)),
      requireSecureSession: false,
      ...rest,
    };
    if (nonceStart % 256n !== 0n) throw new Error("nonceStart must be a multiple of 256");
    this.nextNonce = nonceStart;
    if (key) {
      if (!operator || !contract || chainId === undefined) throw new Error("a provisioned device needs operator, contract and chainId");
      this.setup = { key, address: addressOfPrivateKey(key), operator, contract, chainId: BigInt(chainId), pin };
      this.state = "PROVISIONED_NO_ANCHOR";
    } else {
      this.setup = null;
      this.state = "UNPROVISIONED";
    }
  }

  /** The device address; a provisioned device only. */
  get address(): string {
    if (!this.setup) throw new Error("the device is UNPROVISIONED: it has no key yet");
    return this.setup.address;
  }

  private get domain(): Domain {
    const s = this.setup!;
    return { chainId: s.chainId, verifyingContract: s.contract };
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
    const step = this.step(m);
    const out: Message[] = [];
    let r = step.next(null);
    while (!r.done) {
      if ("send" in r.value) {
        out.push(r.value.send);
        r = step.next(null);
      } else {
        const v = r.value.wait;
        if (v instanceof Promise) throw new Error("a button callback returned a promise: use handleAsync");
        r = step.next(v);
      }
    }
    return [...out, ...r.value];
  }

  /**
   * Like handle, but waits for button callbacks that return a promise (a real press). Messages
   * the device sends before the end of the step (the setup.operator ack, before the PIN) go to
   * `emit` as soon as they exist; without it they come first in the result.
   */
  async handleAsync(m: Message, emit?: (m: Message) => void): Promise<Message[]> {
    const step = this.step(m);
    const out: Message[] = [];
    let r = step.next(null);
    while (!r.done) {
      if ("send" in r.value) {
        if (emit) emit(r.value.send);
        else out.push(r.value.send);
        r = step.next(null);
      } else {
        r = step.next(await r.value.wait);
      }
    }
    return [...out, ...r.value];
  }

  /**
   * One body from the kiosk link as it arrives over BLE. In a secure session the body is opened
   * with the session's channel and every reply is sealed with it; session.open and its reply are
   * plaintext. A body that does not open or decode is dropped with error{BAD_FRAME} (or
   * UNSUPPORTED_TYPE) and the session closes (4, 4.1, 4.2).
   */
  handleBody(body: Uint8Array): Uint8Array[] {
    const r = this.bodyIn(body);
    return this.bodiesOut(r.channel, "error" in r ? [r.error] : this.handle(r.message));
  }

  /** handleBody with real (asynchronous) presses; early replies go to `emit` as in handleAsync. */
  async handleBodyAsync(body: Uint8Array, emit?: (b: Uint8Array) => void): Promise<Uint8Array[]> {
    const r = this.bodyIn(body);
    if ("error" in r) return this.bodiesOut(r.channel, [r.error]);
    const early = emit && ((m: Message) => emit(this.bodiesOut(r.channel, [m])[0]));
    return this.bodiesOut(r.channel, await this.handleAsync(r.message, early));
  }

  private bodyIn(body: Uint8Array): { channel: SecureChannel | null } & ({ message: Message } | { error: Message }) {
    const channel = this.session?.channel ?? null;
    try {
      return { channel, message: decodeMessage(channel ? channel.open(body) : body) };
    } catch (e) {
      if (!(e instanceof ProtocolError)) throw e;
      this.session = null;
      return { channel, error: { v: 1, type: "error", sessionId: "0000000000000000", reason: e.reason } as Message };
    }
  }

  /** Replies go out under the channel of the session the message came in (none for session.open). */
  private bodiesOut(channel: SecureChannel | null, replies: Message[]): Uint8Array[] {
    return replies.map((m) => (channel ? channel.seal(encodeMessage(m)) : encodeMessage(m)));
  }

  /** The steps that wait for the renter run as generators; every other message answers at once. */
  private *step(m: Message): Step {
    if (m.type === "payment.prepare") return yield* this.prepareStep(m);
    if (m.type === "setup.operator") return yield* this.operatorStep(m);
    if (m.type === "limit.change") return yield* this.limitStep(m);
    return this.handleOther(m);
  }

  private handleOther(m: Message): Message[] {
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
      case "payment.outcome":
        // The kiosk's final result: the device forwards it unchanged to the phone app, for the
        // payment session it belongs to (schema payment.outcome; P02-FR-07). No reply to the kiosk.
        if (this.inSession(m, "payment")) this.toPhone(m);
        return [];
      case "device.reset":
        return [this.reset(m)];
      default:
        return [this.error("UNSUPPORTED_TYPE", String(m.sessionId))];
    }
  }

  private open(m: Message): Message {
    const mode = String(m.mode);
    const sid = String(m.sessionId);
    const secure = m.kioskEphemeral !== undefined;
    if (mode === "setup" && this.state !== "UNPROVISIONED" && this.state !== "PROVISIONED_NO_ANCHOR") {
      return this.error("NOT_PERMITTED", sid);
    }
    // The channel is for the unpaired kiosk link only; a release device refuses plaintext payments (4.1).
    if ((secure && (mode !== "payment" || !this.setup)) || (!secure && mode === "payment" && this.cfg.requireSecureSession)) {
      return this.error("NOT_PERMITTED", sid);
    }
    const merchant = secure ? this.kioskMerchant(m) : undefined;
    if (merchant === null) return this.error("MERCHANT_FORGED", sid);
    const deviceNonce = bytesToHex(this.cfg.random(32));
    this.session = { id: sid, mode, deviceNonce, confirmed: false };
    this.attestation = null;
    let deviceEphemeral: string | undefined;
    if (secure) {
      // One-time key: used for this session's key and dropped at once (4.1, step 5).
      const eph = ephemeralKey(this.cfg.random);
      const key = sessionKey(eph.privateKey, hexToBytes(String(m.kioskEphemeral)), hexToBytes(String(m.kioskNonce)), hexToBytes(deviceNonce));
      eph.privateKey.fill(0);
      this.session.channel = new SecureChannel(key, "device");
      this.session.merchant = merchant;
      deviceEphemeral = bytesToHex(eph.x);
    }
    return this.reply("session.open.ok", {
      ...(deviceEphemeral ? { deviceEphemeral } : {}),
      ...(this.setup ? { device: this.setup.address } : {}), // an UNPROVISIONED device has no key yet
      deviceNonce,
      anchorValid: this.anchor !== null,
      firmware: this.cfg.firmware,
      state: this.state,
      lastAnchor: String(this.lastAnchor),
    });
  }

  /**
   * The merchant a secure session.open proves (4.1, step 2): the attestation is the operator's,
   * the one-time key is signed by that merchant and is a curve point. Null: MERCHANT_FORGED.
   * Validity times are checked at payment.identify, once the device has an anchor.
   */
  private kioskMerchant(m: Message): string | null {
    try {
      const { operatorSignature, ...fields } = m.attestation as Record<string, string>;
      if (recoverSigner(digest(this.domain, "MerchantAttestation", fields), hexToBytes(operatorSignature)) !== this.setup!.operator.toLowerCase()) return null;
      const merchant = fields.merchant.toLowerCase();
      const kioskKey = { merchant: fields.merchant, kioskEphemeral: String(m.kioskEphemeral), kioskNonce: String(m.kioskNonce) };
      if (recoverSigner(digest(this.domain, "KioskKey", kioskKey), hexToBytes(String(m.kioskKeySignature))) !== merchant) return null;
      return isPointX(hexToBytes(String(m.kioskEphemeral))) ? merchant : null;
    } catch {
      return null;
    }
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
    if (!this.setup) return ack(false, "NOT_PERMITTED"); // no key and no operator to check against
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
    if (String(m.device).toLowerCase() !== this.setup.address || signer !== this.setup.operator.toLowerCase()) return ack(false, "NOT_PERMITTED");
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
    if (signer !== this.setup!.operator.toLowerCase()) return this.refused("MERCHANT_FORGED");
    // A secure session serves only the merchant its session.open proved.
    if (this.session!.merchant && a.merchant.toLowerCase() !== this.session!.merchant) return this.refused("MERCHANT_FORGED");
    const skew = this.cfg.anchorClockSkew;
    if (t + skew < Number(a.validFrom) || t - skew > Number(a.validUntil)) return this.refused("ATTESTATION_EXPIRED");
    this.attestation = { merchant: a.merchant.toLowerCase(), payout: a.payout.toLowerCase(), name: a.name };
    return null; // accepted: the device answers payment.prepare
  }

  /** Step-4 checks; on success the confirm.show the renter decides on. */
  private prepare(m: Message): Message[] | { show: Message; auth: Record<string, string> } {
    if (!this.inSession(m, "payment")) return [this.error("NOT_PERMITTED", String(m.sessionId))];
    if (this.state === "PIN_LOCKED") return [this.refused("PIN_LOCKED")]; // every signature is refused
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
      BigInt(auth.chainId) !== this.setup!.chainId ||
      !same(auth.contract, this.setup!.contract)
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
    this.toPhone(show);
    return { show, auth };
  }

  private *prepareStep(m: Message): Step {
    const p = this.prepare(m);
    if (Array.isArray(p)) return p;
    const sid = this.session?.id;
    const approved = yield { wait: this.cfg.approve(p.show) };
    // session.cancel (or a new session) while the renter decided: no signature leaves the device.
    if (this.session?.id !== sid) return [];
    if (!approved) return [this.refused("USER_REJECTED")];
    const nonce = this.nextNonce++; // sequential: consecutive nonces share one bitmap slot
    const sig = signDigest(digest(this.domain, "PaymentAuthorization", { ...p.auth, nonce }), this.setup!.key);
    return [this.reply("payment.result", { outcome: "approved", signature: bytesToHex(sig), nonce: nonce.toString() })];
  }

  /**
   * setup.operator (payment-protocol.md 5, steps 1-2): the renter confirms the operator values,
   * the device makes its key and nonce start, the renter sets the PIN, and only then is
   * everything stored at once. A refusal, a PIN timeout or a dropped session stores nothing.
   */
  private *operatorStep(m: Message): Step {
    const ack = (step: string, accepted: boolean, extra: Record<string, unknown> = {}) =>
      this.reply("setup.ack", { step, accepted, ...extra });
    if (!this.inSession(m, "setup")) return [this.error("NOT_PERMITTED", String(m.sessionId))];
    if (this.state !== "UNPROVISIONED") return [ack("setup.operator", false, { reason: "NOT_PERMITTED" })]; // once only [N23]
    const passkey = Number(m.passkey);
    if (!Number.isSafeInteger(passkey) || passkey > 999_999) return [ack("setup.operator", false, { reason: "NOT_PERMITTED" })];
    const values: SetupValues = { operator: String(m.operator), contract: String(m.contract), chainId: BigInt(String(m.chainId)), passkey };
    const sid = this.session!.id;

    const confirmed = yield { wait: this.cfg.confirmSetup(values) };
    if (this.session?.id !== sid) return [];
    if (!confirmed) return [ack("setup.operator", false, { reason: "USER_REJECTED" })];
    yield { send: ack("setup.operator", true) };

    // TRNG key (redrawn in the negligible case it is not a valid scalar), then the nonce start:
    // 31 random bytes and a zero low byte, so a multiple of 256 (payment-protocol.md 2).
    let key: Uint8Array;
    do key = this.cfg.random(32);
    while (BigInt(bytesToHex(key)) === 0n || BigInt(bytesToHex(key)) >= SECP256K1_N);
    const start = new Uint8Array(32);
    start.set(this.cfg.random(31), 0);

    const pin = yield { wait: this.cfg.enterPin() };
    if (this.session?.id !== sid) return [];
    if (typeof pin !== "string") return [ack("keygen", false, { reason: "TIMEOUT" })];
    const address = addressOfPrivateKey(key);
    this.setup = { key, address, operator: values.operator, contract: values.contract, chainId: values.chainId, passkey, pin };
    this.nextNonce = BigInt(bytesToHex(start));
    this.state = "PROVISIONED_NO_ANCHOR";
    return [ack("keygen", true, { device: address })];
  }

  /**
   * limit.change (payment-protocol.md 5): checks, confirm.limit to the phone, the PIN on the
   * buttons, then the approve button; the signature takes the next sequential nonce.
   */
  private *limitStep(m: Message): Step {
    const result = (fields: Record<string, unknown>) => this.reply("limit.result", fields);
    const refused = (reason: string) => [result({ outcome: "refused", reason })];
    if (!this.inSession(m, "payment")) return [this.error("NOT_PERMITTED", String(m.sessionId))];
    if (this.state === "PIN_LOCKED") return refused("PIN_LOCKED");
    const t = this.time();
    if (t === null || this.state !== "READY") return refused("TIME_ANCHOR_MISSING");
    const c = m.change as Record<string, string>;
    const s = this.setup!;
    if (BigInt(c.chainId) !== s.chainId || c.contract.toLowerCase() !== s.contract.toLowerCase()) return refused("NOT_PERMITTED");
    const expiry = Number(c.expiry);
    if (expiry < t || expiry > t + this.cfg.authorizationExpiry) return refused("ATTESTATION_EXPIRED");
    const show = this.reply("confirm.limit", { perPaymentLimit: c.perPaymentLimit, dailyLimit: c.dailyLimit, expiry: c.expiry });
    this.toPhone(show);
    const sid = this.session!.id;

    const pin = yield { wait: this.cfg.enterPin() };
    if (this.session?.id !== sid) return [];
    if (typeof pin !== "string") return refused("TIMEOUT");
    if (pin !== s.pin) {
      this.pinFailures++;
      if (this.pinFailures >= this.cfg.pinMaxRetries) {
        this.state = "PIN_LOCKED";
        return refused("PIN_LOCKED");
      }
      return refused("NOT_PERMITTED");
    }
    this.pinFailures = 0;

    const approved = yield { wait: this.cfg.approve(show) };
    if (this.session?.id !== sid) return [];
    if (!approved) return refused("USER_REJECTED");
    const nonce = this.nextNonce++; // the same sequential counter as payments
    const change = { chainId: c.chainId, contract: c.contract, perPaymentLimit: c.perPaymentLimit, dailyLimit: c.dailyLimit, nonce, expiry: c.expiry };
    const sig = signDigest(digest(this.domain, "LimitChange", change), s.key);
    return [result({ outcome: "approved", signature: bytesToHex(sig), nonce: nonce.toString() })];
  }

  /** device.reset (payment-protocol.md 5): an operator-signed DeviceReset for this device wipes it. */
  private reset(m: Message): Message {
    const ack = (accepted: boolean) =>
      this.reply("setup.ack", { step: "device.reset", accepted, ...(accepted ? {} : { reason: "NOT_PERMITTED" }) });
    if (!this.session || this.session.id !== m.sessionId) return this.error("NOT_PERMITTED", String(m.sessionId));
    if (!this.setup) return ack(false);
    let signer: string;
    try {
      signer = recoverSigner(
        digest(this.domain, "DeviceReset", { device: String(m.device), nonce: BigInt(String(m.nonce)) }),
        hexToBytes(String(m.operatorSignature)),
      );
    } catch {
      return ack(false);
    }
    if (String(m.device).toLowerCase() !== this.setup.address || signer !== this.setup.operator.toLowerCase()) return ack(false);
    const reply = ack(true);
    this.setup = null;
    this.anchor = null;
    this.lastAnchor = 0;
    this.nextNonce = 0n;
    this.pinFailures = 0;
    this.attestation = null;
    this.session = null; // the wiped device keeps no session
    this.state = "UNPROVISIONED";
    return reply;
  }
}
