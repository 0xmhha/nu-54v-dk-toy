// Messages to and from the device over the bonded link, for setup and the wallet check.
//
// The link carries fragments (payment-protocol.md 4); BodyLink is the same link after framing, so
// tests can plug the software device in directly. Every message from the device is queued until
// a waiter takes it; a waiter that is not answered in time fails with a TimeoutError.

import { decodeMessage, encodeMessage, FrameWriter, Reassembler, type Message } from "@nu54/protocol";

/** One CBOR body each way. */
export interface BodyLink {
  send(body: Uint8Array): Promise<void>;
  onBody(handler: (body: Uint8Array) => void): () => void;
}

/** The BLE link as fragments: what the native module gives. */
export interface FragmentLink {
  write(fragment: Uint8Array): Promise<void>;
  onFragment(handler: (fragment: Uint8Array) => void): () => void;
  mtu: number;
}

/** Frames bodies into fragments and reassembles the device's fragments into bodies. */
export function bodyLink(link: FragmentLink): BodyLink {
  const writer = new FrameWriter(link.mtu);
  return {
    async send(body) {
      for (const f of writer.write(body)) await link.write(f);
    },
    onBody(handler) {
      const rx = new Reassembler();
      return link.onFragment((f) => {
        try {
          const r = rx.feed(f);
          if (r.status !== "more") handler(r.body);
        } catch {
          // a broken frame: the reassembler has reset; the waiter times out
        }
      });
    },
  };
}

export class TimeoutError extends Error {
  constructor(what: string) {
    super(`no answer from the device: ${what}`);
  }
}

type Waiter = { match: (m: Message) => boolean; resolve: (m: Message) => void; reject: (e: Error) => void; timer: ReturnType<typeof setTimeout> };

export class DeviceChannel {
  private readonly queue: Message[] = [];
  private readonly waiters: Waiter[] = [];
  private readonly listeners: { match: (m: Message) => boolean; handler: (m: Message) => void }[] = [];
  private readonly detach: () => void;

  constructor(private readonly link: BodyLink) {
    this.detach = link.onBody((b) => {
      let m: Message;
      try {
        m = decodeMessage(b);
      } catch {
        return;
      }
      const l = this.listeners.find((x) => x.match(m));
      if (l) {
        l.handler(m); // a running indication (pin.entry): not queued for waiters
        return;
      }
      const w = this.waiters.find((x) => x.match(m));
      if (w) {
        this.waiters.splice(this.waiters.indexOf(w), 1);
        clearTimeout(w.timer);
        w.resolve(m);
      } else {
        this.queue.push(m);
      }
    });
  }

  send(m: Message): Promise<void> {
    return this.link.send(encodeMessage(m));
  }

  /** The next message `match` accepts (already queued or still to come), within `ms`. */
  next(match: (m: Message) => boolean, ms: number, what: string): Promise<Message> {
    const i = this.queue.findIndex(match);
    if (i >= 0) return Promise.resolve(this.queue.splice(i, 1)[0]);
    return new Promise((resolve, reject) => {
      const w: Waiter = {
        match, resolve, reject,
        timer: setTimeout(() => {
          this.waiters.splice(this.waiters.indexOf(w), 1);
          reject(new TimeoutError(what));
        }, ms),
      };
      this.waiters.push(w);
    });
  }

  /** Every message `match` accepts goes to `handler` instead of the queue, until the returned call. */
  listen(match: (m: Message) => boolean, handler: (m: Message) => void): () => void {
    const l = { match, handler };
    this.listeners.push(l);
    return () => {
      const i = this.listeners.indexOf(l);
      if (i >= 0) this.listeners.splice(i, 1);
    };
  }

  close(): void {
    this.detach();
    for (const w of this.waiters.splice(0)) {
      clearTimeout(w.timer);
      w.reject(new Error("link closed"));
    }
  }
}

/** A message of `type` (and `step`, for setup.ack). */
export const isType = (type: string, step?: string) => (m: Message) =>
  m.type === type && (step === undefined || m.step === step);
