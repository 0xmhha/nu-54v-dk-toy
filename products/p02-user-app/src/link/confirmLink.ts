// The phone app's side of the bonded link (P02 design 1 and 3, P02-FR-02, FR-03, FR-07).
//
// The device sends confirm.show before it waits for the button, and forwards the kiosk's
// payment.outcome afterwards. Only a bonded link is trusted: anything from an unbonded link is
// dropped. The screen shows confirm.show values only; there is no approve button (N26).

import { decodeMessage, Reassembler, type Message } from "@nu54/protocol";
import { confirmView, TOKENS, type ConfirmView, type TokenInfo } from "../confirm/display.ts";

/** Fragments from the device's TX characteristic, with the link's bonding state. */
export interface PhoneTransport {
  /** True when the link is encrypted with a stored LE Secure Connections bond. */
  bonded(): boolean;
  onFragment(handler: (fragment: Uint8Array) => void): () => void;
}

export type Screen =
  | { kind: "waiting" }
  | { kind: "confirming"; view: ConfirmView; orderId: string }
  | { kind: "result"; outcome: string; reason?: string; view?: ConfirmView };

/** A confirm.show with no outcome after this long goes back to waiting (the device may have refused). */
export const CONFIRM_TIMEOUT_MS = 120_000;

export class ConfirmLink {
  private readonly rx = new Reassembler();
  private screen: Screen = { kind: "waiting" };
  private timer: ReturnType<typeof setTimeout> | null = null;
  private readonly detach: () => void;
  /** Messages dropped because the link was not bonded or they were not for the phone. */
  dropped = 0;

  constructor(
    private readonly transport: PhoneTransport,
    private readonly onScreen: (s: Screen) => void,
    private readonly tokens: Record<string, TokenInfo> = TOKENS,
  ) {
    this.detach = transport.onFragment((f) => this.receive(f));
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
    this.timer = s.kind === "confirming" ? setTimeout(() => this.show({ kind: "waiting" }), CONFIRM_TIMEOUT_MS) : null;
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
    } else if (m.type === "payment.outcome") {
      const s = this.screen;
      // The outcome of the order on screen; an outcome for another order is shown without details.
      const view = s.kind === "confirming" && s.orderId === String(m.orderId).toLowerCase() ? s.view : undefined;
      this.show({ kind: "result", outcome: String(m.outcome), ...(m.reason ? { reason: String(m.reason) } : {}), view });
    } else {
      this.dropped++;
    }
  }
}
