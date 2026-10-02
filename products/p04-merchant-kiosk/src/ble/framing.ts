// Messages over a fragment transport (payment-protocol.md 4): CBOR body -> envelope -> fragments
// for the negotiated ATT_MTU on the way out, reassembly and decoding on the way in.
//
// The transport is the BLE central (NusBle on Android) or, in tests, an in-process device.

import { decodeMessage, encodeMessage, FrameWriter, ProtocolError, Reassembler, type Message } from "@nu54/protocol";

const ZERO_SESSION = "0000000000000000";

/** A link that moves raw fragments: write to the device's RX, fragments arrive from its TX. */
export interface FragmentTransport {
  /** Negotiated ATT_MTU; fragments carry at most ATT_MTU - 5 data bytes. */
  readonly mtu: number;
  write(fragment: Uint8Array): Promise<void>;
  /** Registers the handler for TX notifications; returns a function that removes it. */
  onFragment(handler: (fragment: Uint8Array) => void): () => void;
  close(): Promise<void>;
}

/** A link that moves whole protocol messages. */
export interface MessageLink {
  /**
   * Sends one message and resolves with the replies that arrived within waitMs; it stops
   * early once `want` replies are in. Only replies of the message's session count (a frame
   * error carries the zero session id); replies that arrived while nothing waited, such as a
   * press after the kiosk gave up, are dropped and never answer a later request.
   */
  send(m: Message, want: number, waitMs: number): Promise<Message[]>;
  close(): Promise<void>;
}

export class FramedLink implements MessageLink {
  private readonly writer: FrameWriter;
  private readonly rx = new Reassembler();
  private readonly inbox: Message[] = [];
  private wake: (() => void) | null = null;
  /** Session id of the message being answered. */
  private session: string | null = null;
  private readonly detach: () => void;
  /** Set when the device sent a fragment that breaks the receiver rules (BAD_FRAME). */
  badFrame: string | null = null;

  private readonly transport: FragmentTransport;

  constructor(transport: FragmentTransport) {
    this.transport = transport;
    this.writer = new FrameWriter(transport.mtu);
    this.detach = transport.onFragment((f) => this.receive(f));
  }

  private receive(fragment: Uint8Array): void {
    try {
      const r = this.rx.feed(fragment);
      if (r.status === "done") {
        const m = decodeMessage(r.body);
        if (m.sessionId === this.session || m.sessionId === ZERO_SESSION) this.inbox.push(m);
      }
    } catch (e) {
      if (!(e instanceof ProtocolError)) throw e;
      this.badFrame = e.message;
    }
    this.wake?.();
  }

  async send(m: Message, want: number, waitMs: number): Promise<Message[]> {
    this.inbox.length = 0;
    this.session = String(m.sessionId);
    for (const f of this.writer.write(encodeMessage(m))) await this.transport.write(f);
    const deadline = Date.now() + waitMs;
    while (this.inbox.length < want && this.badFrame === null && Date.now() < deadline) {
      await new Promise<void>((resolve) => {
        const t = setTimeout(resolve, Math.max(0, deadline - Date.now()));
        this.wake = () => {
          clearTimeout(t);
          resolve();
        };
      });
      this.wake = null;
    }
    return this.inbox.splice(0);
  }

  async close(): Promise<void> {
    this.detach();
    await this.transport.close();
  }
}
