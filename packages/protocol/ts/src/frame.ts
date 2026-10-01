// BLE framing (payment-protocol.md 4): envelope and fragments, plus a receiver.
//
//   envelope = length(u16 BE, whole envelope) | digest(8 = SHA-256(body)[0..8]) | CBOR body
//   fragment = sequence(u8) | index(u8) | up to ATT_MTU - 5 envelope bytes
//
// The receiver follows the rules the shared fragment vectors encode: index starts at 0 and
// grows by one, the sequence stays the same within a message (it is not compared between
// messages), every fragment carries data, the length is 10..2058, and the digest is checked
// once the envelope is complete.

import { sha256 } from "@noble/hashes/sha2";
import { concat, equalBytes, ProtocolError } from "./bytes.ts";

export const ENVELOPE_HEADER_LEN = 10;
export const MAX_BODY_LEN = 2048;

export function envelope(body: Uint8Array): Uint8Array {
  if (body.length > MAX_BODY_LEN) throw new ProtocolError("BAD_FRAME", "body above 2048 bytes");
  const total = ENVELOPE_HEADER_LEN + body.length;
  return concat(Uint8Array.of(total >> 8, total & 0xff), sha256(body).subarray(0, 8), body);
}

/** Payload bytes per fragment for a negotiated ATT_MTU. */
export function fragmentPayload(attMtu: number): number {
  if (attMtu <= 5) throw new Error("ATT_MTU too small");
  return attMtu - 5;
}

export function fragments(env: Uint8Array, sequence: number, attMtu: number): Uint8Array[] {
  const size = fragmentPayload(attMtu);
  const out: Uint8Array[] = [];
  for (let i = 0, idx = 0; i < env.length; i += size, idx++) {
    if (idx > 255) throw new Error("more than 256 fragments");
    out.push(concat(Uint8Array.of(sequence & 0xff, idx), env.subarray(i, i + size)));
  }
  return out;
}

/** Sender side: numbers messages per direction and splits them. */
export class FrameWriter {
  private sequence = 0;
  private attMtu: number;
  constructor(attMtu: number) {
    this.attMtu = attMtu;
  }
  setMtu(attMtu: number): void {
    this.attMtu = attMtu;
  }
  write(body: Uint8Array): Uint8Array[] {
    const out = fragments(envelope(body), this.sequence, this.attMtu);
    this.sequence = (this.sequence + 1) & 0xff;
    return out;
  }
}

export type FeedResult = { status: "more" } | { status: "done"; body: Uint8Array };

/** Receiver side. Throws ProtocolError BAD_FRAME and resets on any rule violation. */
export class Reassembler {
  private buf: Uint8Array[] = [];
  private have = 0;
  private want = 0;
  private sequence = -1;
  private nextIndex = 0;
  private active = false;

  reset(): void {
    this.buf = [];
    this.have = 0;
    this.want = 0;
    this.nextIndex = 0;
    this.active = false;
  }

  private bad(detail: string): never {
    this.reset();
    throw new ProtocolError("BAD_FRAME", detail);
  }

  feed(fragment: Uint8Array): FeedResult {
    if (fragment.length <= 2) this.bad("fragment without data");
    const [seq, idx] = fragment;
    if (!this.active) {
      if (idx !== 0) this.bad("message does not start at index 0");
      this.active = true;
      this.sequence = seq;
    } else if (seq !== this.sequence || idx !== this.nextIndex) {
      this.bad("fragment out of order");
    }
    const data = fragment.subarray(2);
    if (this.have + data.length > ENVELOPE_HEADER_LEN + MAX_BODY_LEN) this.bad("envelope too long");
    this.buf.push(data);
    this.have += data.length;
    this.nextIndex++;
    const env = this.buf.length === 1 ? data : concat(...this.buf);
    if (this.want === 0 && this.have >= 2) {
      this.want = (env[0] << 8) | env[1];
      if (this.want < ENVELOPE_HEADER_LEN || this.want > ENVELOPE_HEADER_LEN + MAX_BODY_LEN) this.bad("length out of range");
    }
    if (this.want !== 0 && this.have > this.want) this.bad("bytes beyond the length");
    if (this.want !== 0 && this.have === this.want) {
      const body = env.slice(ENVELOPE_HEADER_LEN);
      if (!equalBytes(sha256(body).subarray(0, 8), env.subarray(2, ENVELOPE_HEADER_LEN))) this.bad("digest mismatch");
      this.reset();
      return { status: "done", body };
    }
    return { status: "more" };
  }
}
