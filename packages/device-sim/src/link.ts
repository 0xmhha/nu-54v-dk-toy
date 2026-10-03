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
  private readonly foreign: () => boolean;
  /** `foreign`: this central does not hold the device's open session (see BodyOptions). */
  constructor(device: SoftwareDevice, attMtu: number, foreign: () => boolean = () => false) {
    this.device = device;
    this.tx = new FrameWriter(attMtu);
    this.foreign = foreign;
  }

  receive(fragment: Uint8Array): Uint8Array[] {
    const body = this.reassemble(fragment);
    if (body === null) return [];
    return this.frame(Array.isArray(body) ? body : this.device.handleBody(body, { foreign: this.foreign() }));
  }

  /**
   * Like receive, for a device whose buttons are real (asynchronous) presses. Fragments of
   * messages sent before the step ends (the setup.operator ack, before the PIN) go to `emit`.
   */
  async receiveAsync(fragment: Uint8Array, emit?: (fragments: Uint8Array[]) => void): Promise<Uint8Array[]> {
    const body = this.reassemble(fragment);
    if (body === null) return [];
    if (Array.isArray(body)) return this.frame(body);
    return this.frame(await this.device.handleBodyAsync(body, emit && ((early) => emit(this.frame([early]))), { foreign: this.foreign() }));
  }

  /** A complete body, null while fragments are missing, or the replies to a bad frame. */
  private reassemble(fragment: Uint8Array): Uint8Array | Uint8Array[] | null {
    try {
      const r = this.rx.feed(fragment);
      return r.status === "more" ? null : r.body;
    } catch (e) {
      if (!(e instanceof ProtocolError)) throw e;
      // An empty body is not a message: the device answers error{BAD_FRAME} and closes the
      // session, under the session's channel when it has one (4, 4.1).
      return this.device.handleBody(new Uint8Array(0), { foreign: this.foreign() });
    }
  }

  private frame(bodies: Uint8Array[]): Uint8Array[] {
    return bodies.flatMap((b) => this.tx.write(b));
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
