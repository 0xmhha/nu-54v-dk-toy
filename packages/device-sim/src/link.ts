// The wire between a central (kiosk, phone app, operator tool) and the simulated device.
// Every message goes through the same layers as on BLE: deterministic CBOR, envelope and
// fragments for the negotiated ATT_MTU, reassembly and decoding on the other side.

import { decodeMessage, encodeMessage, FrameWriter, ProtocolError, Reassembler, type Message } from "@nu54/protocol";
import type { SoftwareDevice } from "./device.ts";

/** Device side: fragments in, fragments out. */
export class DeviceEndpoint {
  private readonly rx = new Reassembler();
  private readonly tx: FrameWriter;
  readonly device: SoftwareDevice;
  constructor(device: SoftwareDevice, attMtu: number) {
    this.device = device;
    this.tx = new FrameWriter(attMtu);
  }

  receive(fragment: Uint8Array): Uint8Array[] {
    const m = this.decode(fragment);
    if (m === null) return [];
    return this.frame(Array.isArray(m) ? m : this.device.handle(m));
  }

  /** Like receive, for a device whose button is a real (asynchronous) press. */
  async receiveAsync(fragment: Uint8Array): Promise<Uint8Array[]> {
    const m = this.decode(fragment);
    if (m === null) return [];
    return this.frame(Array.isArray(m) ? m : await this.device.handleAsync(m));
  }

  /** A complete message, null while fragments are missing, or the error replies of a bad frame. */
  private decode(fragment: Uint8Array): Message | Message[] | null {
    try {
      const r = this.rx.feed(fragment);
      return r.status === "more" ? null : decodeMessage(r.body);
    } catch (e) {
      if (!(e instanceof ProtocolError)) throw e;
      // The receiver drops the message, reports the reason and closes the session (4, 4.2).
      this.device.handle({ v: 1, type: "session.cancel", sessionId: "0000000000000000" } as Message);
      return [{ v: 1, type: "error", sessionId: "0000000000000000", reason: e.reason } as Message];
    }
  }

  private frame(replies: Message[]): Uint8Array[] {
    return replies.flatMap((m) => this.tx.write(encodeMessage(m)));
  }
}

/** Central side: sends messages and collects decoded replies. */
export class CentralLink {
  private readonly rx = new Reassembler();
  private readonly tx: FrameWriter;
  private readonly transmit: (fragment: Uint8Array) => Uint8Array[];
  constructor(transmit: (fragment: Uint8Array) => Uint8Array[], attMtu: number) {
    this.transmit = transmit;
    this.tx = new FrameWriter(attMtu);
  }

  /** Sends one message and returns every complete message that came back. */
  send(m: Message): Message[] {
    return this.sendFragments(this.tx.write(encodeMessage(m)));
  }

  /** Sends raw fragments (for corrupted-frame tests). */
  sendFragments(fragments: Uint8Array[]): Message[] {
    const out: Message[] = [];
    for (const f of fragments) {
      for (const back of this.transmit(f)) {
        const r = this.rx.feed(back);
        if (r.status === "done") out.push(decodeMessage(r.body));
      }
    }
    return out;
  }
}

/** An in-process pair: a central talking to a simulated device. */
export function connect(device: SoftwareDevice, attMtu = 185): CentralLink {
  const endpoint = new DeviceEndpoint(device, attMtu);
  return new CentralLink((f) => endpoint.receive(f), attMtu);
}
