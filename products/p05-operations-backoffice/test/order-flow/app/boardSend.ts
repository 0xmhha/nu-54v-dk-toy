// What the firmware does not know yet, played between the phone and the board process: the status says the
// board can send, and a transfer to another address (0x80) is answered here with the board's own key.
// The real firmware core answers everything else. Remove this when the firmware signs sends.
import {bytesToBig, bytesToHex, concat, encodeV2StreamFrame, fromBoard, recoverSigner, type PaymentToken, V2StreamDecoder, walletReturnDigestFor} from '@nu54/protocol';

const statusOperation = 0x7f, sendOperation = 0x80, replyCommand = 5, frameHead = 22, bodyHead = 25;
type Transfer = {token: string; to: string; amount: bigint; validBefore: bigint; nonce: string};
type Frame = {epoch: Uint8Array; sequence: number; head: Uint8Array; payload: Uint8Array};

/** A frame that carries a whole request or reply. Longer ones are none of the operations played here. */
function whole(frame: Uint8Array): Frame | undefined {
  if (frame[20] !== 0 || frame[21] !== 1 || frame.length < frameHead + bodyHead) return undefined;
  return {epoch: frame.slice(0, 16), sequence: new DataView(frame.buffer, frame.byteOffset).getUint32(16), head: frame.slice(frameHead, frameHead + bodyHead),
    payload: frame.slice(frameHead + bodyHead)};
}
function build({epoch, sequence, head, payload}: Frame) {
  const number = new Uint8Array(4);
  new DataView(number.buffer).setUint32(0, sequence);
  return encodeV2StreamFrame(concat(epoch, number, Uint8Array.of(0, 1), head, payload));
}

export class SendStandIn {
  constructor(private readonly token: PaymentToken) {}
  /** Whether the board says it can send. */
  able = true;
  private fromPhone = new V2StreamDecoder();
  private fromBoard = new V2StreamDecoder();
  private routed = false;
  private asked = 0;
  private replies = 0;
  private limits = {perPayment: 0n, left: 0n};
  private spent = 0n;
  private waiting?: {frame: Frame; transfer: Transfer};

  reset() {
    this.fromPhone = new V2StreamDecoder();
    this.fromBoard = new V2StreamDecoder();
    this.routed = false;
    this.asked = this.replies = 0;
    this.waiting = undefined;
  }
  forget() { this.spent = 0n; }

  /** Bytes of the phone. Returns what the board process gets, and whether a button is now awaited. */
  phone(bytes: Uint8Array): {forward: Uint8Array; prompt: boolean} {
    const out: Uint8Array[] = [];
    let prompt = false, rest = bytes;
    if (!this.routed && bytes.length) { this.routed = true; out.push(bytes.slice(0, 1)); rest = bytes.subarray(1); }
    this.fromPhone.feed(rest, frame => {
      const found = whole(frame);
      this.asked = found?.payload[0] ?? 0;
      if (found && this.able && this.asked === sendOperation && found.payload.length === 89) {
        const body = found.payload;
        const transfer = {token: bytesToHex(body.subarray(1, 21), true), to: bytesToHex(body.subarray(21, 41), true), amount: bytesToBig(body.subarray(41, 49)),
          validBefore: bytesToBig(body.subarray(49, 57)), nonce: bytesToHex(body.subarray(57), false)};
        if (transfer.amount > this.limits.perPayment || transfer.amount > this.limits.left) return; // as the firmware will: no answer
        this.waiting = {frame: found, transfer};
        prompt = true;
      } else out.push(encodeV2StreamFrame(frame));
    });
    return {forward: concat(...out), prompt};
  }

  /** Bytes of the board process, as the phone gets them. */
  board(bytes: Uint8Array): Uint8Array {
    const out: Uint8Array[] = [];
    this.fromBoard.feed(bytes, frame => {
      this.replies++;
      const found = whole(frame);
      if (!found || !this.able || found.head[0] !== replyCommand || this.asked !== statusOperation || found.payload.length !== 71) { out.push(encodeV2StreamFrame(frame)); return; }
      const payload = concat(found.payload, Uint8Array.of(1)), left = bytesToBig(payload.subarray(63, 71));
      this.limits = {perPayment: bytesToBig(payload.subarray(42, 50)), left: left > this.spent ? left - this.spent : 0n};
      new DataView(payload.buffer).setBigUint64(63, this.limits.left);
      out.push(build({...found, payload}));
    });
    return concat(...out);
  }

  get awaits() { return !!this.waiting; }
  /** SW1 and the PIN, or anything else. `sign` is the board's key over a digest (64 bytes, low s). */
  async answer(approved: boolean, account: string, sign: (digest: Uint8Array) => Promise<Uint8Array>): Promise<Uint8Array | undefined> {
    const held = this.waiting;
    this.waiting = undefined;
    if (!held) return undefined;
    const {frame, transfer} = held;
    const reply = (payload: Uint8Array) => build({epoch: frame.epoch, sequence: this.replies++, head: concat(Uint8Array.of(replyCommand), frame.head.subarray(1)), payload});
    if (!approved) return reply(Uint8Array.of(0));
    const digest = walletReturnDigestFor(this.token.name, {...transfer, amount: fromBoard(transfer.amount, this.token), from: account});
    const raw = await sign(digest);
    const signature = [27, 28].map(v => concat(raw, Uint8Array.of(v))).find(item => recoverSigner(digest, item) === account);
    if (!signature) throw new Error('BOARD_SIGNATURE');
    this.spent += transfer.amount;
    return reply(concat(Uint8Array.of(sendOperation), Buffer.from(transfer.to.slice(2), 'hex'), signature));
  }
}
