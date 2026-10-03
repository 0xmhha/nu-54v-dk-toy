// Messages over a fragment transport (payment-protocol.md 4): CBOR body -> envelope -> fragments
// for the negotiated ATT_MTU on the way out, reassembly and decoding on the way in. In a secure
// payment session (4.1) the bodies in both directions are AES-GCM under the session's channel.
//
// The transport is the BLE central (NusBle on Android) or, in tests, an in-process device.

import { decodeMessage, encodeMessage, FrameWriter, ProtocolError, Reassembler, type Message, type SecureChannel } from "@nu54/protocol";

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
  /**
   * Sets the secure channel of the current session (4.1), or null to go back to plaintext for
   * the next session.open. Links without it carry plaintext sessions only.
   */
  secure?(channel: SecureChannel | null): void;
  /**
   * Ends a secure session still open on this link with session.cancel under its channel, then
   * drops the channel: a plaintext session.open would otherwise fail the device's tag check (4.1).
   */
  endSession?(): Promise<void>;
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
  private channel: SecureChannel | null = null;

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
        const m = this.open(r.body);
        if (m.sessionId === this.session || m.sessionId === ZERO_SESSION) this.inbox.push(m);
      }
    } catch (e) {
      if (!(e instanceof ProtocolError)) throw e;
      this.badFrame = e.message;
    }
    this.wake?.();
  }

  /**
   * A body from the device. Under a channel every body is opened, even one nobody waits for, so
   * the message numbers stay in step. The one plaintext body a secure session can meet is the
   * device's error after it already dropped the channel (4.1, step 4); anything else that does
   * not open is BAD_FRAME.
   */
  private open(body: Uint8Array): Message {
    if (!this.channel) return decodeMessage(body);
    try {
      return decodeMessage(this.channel.open(body));
    } catch (e) {
      if (!(e instanceof ProtocolError)) throw e;
      let plain: Message;
      try {
        plain = decodeMessage(body);
      } catch {
        throw e;
      }
      if (plain.type !== "error") throw e;
      this.channel = null;
      return plain;
    }
  }

  secure(channel: SecureChannel | null): void {
    this.channel = channel;
  }

  async endSession(): Promise<void> {
    if (this.channel && this.session) await this.send({ v: 1, type: "session.cancel", sessionId: this.session } as Message, 0, 0);
    this.channel = null;
  }

  async send(m: Message, want: number, waitMs: number): Promise<Message[]> {
    this.inbox.length = 0;
    this.session = String(m.sessionId);
    const body = encodeMessage(m);
    for (const f of this.writer.write(this.channel ? this.channel.seal(body) : body)) await this.transport.write(f);
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
