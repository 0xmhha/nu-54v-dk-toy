// The phone app's side of the bonded link (P02 design 1 and 3, P02-FR-02, FR-03, FR-07).
//
// The device sends confirm.show before it waits for the button, and forwards the kiosk's
// payment.outcome afterwards. Only a bonded link is trusted: anything from an unbonded link is
// dropped. The screen shows confirm.show values only; there is no approve button (N26). The one
// message the phone app sends is device.paymentMode (P02-FR-08): payment advertising on or off.

import { decodeMessage, encodeMessage, FrameWriter, parseReceipt, Reassembler, type DigitalReceipt, type Message } from "@nu54/protocol";
import { confirmView, limitView, TOKENS, type ConfirmView, type LimitView, type TokenInfo } from "../confirm/display.ts";

/** Fragments from the device's TX characteristic, with the link's bonding state. */
export interface PhoneTransport {
  /** True when the link is encrypted with a stored LE Secure Connections bond. */
  bonded(): boolean;
  onFragment(handler: (fragment: Uint8Array) => void): () => void;
  /** Writes one fragment to the device's RX; with the ATT_MTU it needs for device.paymentMode. */
  write?(fragment: Uint8Array): Promise<void>;
  mtu?: number;
}

/** The device's answer to device.paymentMode. */
export interface PaymentMode {
  on: boolean;
  accepted: boolean;
  reason?: string;
}

/** Payment mode lasts this long unless turned off (the device accepts 1 to 300 s). */
export const PAYMENT_MODE_SECONDS = 120;

export type Screen =
  | { kind: "waiting" }
  | { kind: "confirming"; view: ConfirmView; orderId: string }
  | { kind: "limit"; view: LimitView }
  | { kind: "result"; outcome: string; reason?: string; view?: ConfirmView; receipt?: DigitalReceipt; txHash?: string };

/** A confirm.show with no outcome after this long goes back to waiting (the device may have refused). */
export const CONFIRM_TIMEOUT_MS = 120_000;

export class ConfirmLink {
  private readonly rx = new Reassembler();
  private screen: Screen = { kind: "waiting" };
  private timer: ReturnType<typeof setTimeout> | null = null;
  private readonly detach: () => void;
  /** Messages dropped because the link was not bonded or they were not for the phone. */
  dropped = 0;

  private readonly writer: FrameWriter | null;
  /** Called with the device's answer to setPaymentMode. */
  onPaymentMode: ((m: PaymentMode) => void) | null = null;

  constructor(
    private readonly transport: PhoneTransport,
    private readonly onScreen: (s: Screen) => void,
    private readonly tokens: Record<string, TokenInfo> = TOKENS,
  ) {
    this.detach = transport.onFragment((f) => this.receive(f));
    this.writer = transport.write ? new FrameWriter(transport.mtu ?? 23) : null;
  }

  /**
   * Turns payment mode on (the device advertises for kiosks for `seconds`) or off. The device
   * answers with device.paymentMode.ack, reported to onPaymentMode. It belongs to no session.
   */
  async setPaymentMode(on: boolean, seconds = PAYMENT_MODE_SECONDS): Promise<void> {
    if (!this.writer || !this.transport.write) throw new Error("this link cannot write to the device");
    if (!this.transport.bonded()) throw new Error("payment mode needs the bonded link");
    const m = { v: 1, type: "device.paymentMode", sessionId: "0000000000000000", on, seconds: String(on ? seconds : 0) } as Message;
    for (const f of this.writer.write(encodeMessage(m))) await this.transport.write(f);
  }

  current(): Screen {
    return this.screen;
  }

  close(): void {
    this.detach();
    if (this.timer) clearTimeout(this.timer);
  }

  private show(s: Screen): void {
    this.screen = s;
    if (this.timer) clearTimeout(this.timer);
    this.timer = s.kind === "confirming" || s.kind === "limit" ? setTimeout(() => this.show({ kind: "waiting" }), CONFIRM_TIMEOUT_MS) : null;
    this.onScreen(s);
  }

  private receive(fragment: Uint8Array): void {
    let m: Message;
    try {
      const r = this.rx.feed(fragment);
      if (r.status === "more") return;
      m = decodeMessage(r.body);
    } catch {
      return; // a broken frame: the reassembler has reset, wait for the next message
    }
    if (!this.transport.bonded()) {
      this.dropped++; // P02-FR-02: only a bonded link may drive the screen
      return;
    }
    if (m.type === "confirm.show") {
      let view: ConfirmView;
      try {
        view = confirmView(m as Record<string, unknown>, this.tokens);
      } catch {
        this.dropped++;
        return;
      }
      this.show({ kind: "confirming", view, orderId: String(m.orderId).toLowerCase() });
    } else if (m.type === "confirm.limit") {
      this.show({ kind: "limit", view: limitView(m as Record<string, unknown>) });
    } else if (m.type === "device.paymentMode.ack") {
      this.onPaymentMode?.({ on: m.on === true, accepted: m.accepted === true, ...(m.reason ? { reason: String(m.reason) } : {}) });
    } else if (m.type === "payment.outcome") {
      const s = this.screen;
      // The outcome of the order on screen; an outcome for another order is shown without details.
      const view = s.kind === "confirming" && s.orderId === String(m.orderId).toLowerCase() ? s.view : undefined;
      // An approved payment may carry the merchant's digital receipt and its transaction (6). A
      // receipt that does not read is left out; the outcome still shows.
      let receipt: DigitalReceipt | undefined;
      try {
        receipt = m.receipt ? parseReceipt(String(m.receipt)) : undefined;
      } catch {
        receipt = undefined;
      }
      this.show({
        kind: "result", outcome: String(m.outcome), ...(m.reason ? { reason: String(m.reason) } : {}), view,
        ...(receipt ? { receipt } : {}), ...(m.txHash ? { txHash: String(m.txHash) } : {}),
      });
    } else {
      this.dropped++;
    }
  }
}
