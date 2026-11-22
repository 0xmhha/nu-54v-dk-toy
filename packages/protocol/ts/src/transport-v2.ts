import { sha256 } from "@noble/hashes/sha2";
import { bytesToHex, concat, equalBytes } from "./bytes.ts";

import { isRentalReviewPrepare, rentalReviewReceiptSize } from "./rental-payment-review.ts";
import { quoteV2Layout } from "./quote-v2.ts";

export const V2_ALPN = {
  management: "nu54-v2-management",
  payment: "nu54-v2-payment",
  nearby: "nu54-v2-nearby",
} as const;
export const V2_EXPORTER_LABEL = "EXPORTER-NU54-wire2";

export type V2Profile = keyof typeof V2_ALPN;
export type V2Role = "owner" | "board" | "terminal" | "customer";
const ROLES = {
  management: ["owner", "board"],
  payment: ["terminal", "board"],
  nearby: ["customer", "terminal"],
} as const;
const COMMANDS = ["manage", "pay.prepare", "status", "nearby.submit", "reply", "approval.progress", "rental.review"] as const;
export type V2Command = typeof COMMANDS[number];
export interface V2Request {
  command: V2Command;
  id: Uint8Array;
  expires: bigint;
  payload: Uint8Array;
}
export type V2Result = { disposition: "new" | "resume"; request: V2Request };

const EPOCH = 16;
const ID = 16;
const IDENTITY = 32;
export const V2_PAYMENT_REVIEW_OP = 0x75;
const SIGNED_QUOTE = quoteV2Layout.unsignedSize + quoteV2Layout.signatureSize;

export const V2_APPROVAL_PROGRESS_OP = 0x76;
const APPROVAL_PROGRESS_SIZE = 9;

export function encodeApprovalProgressOffer(generation: bigint): Uint8Array {
  if (typeof generation !== "bigint" || generation <= 0n || generation > MAX_TIME)
    throw new Error("invalid approval progress generation");
  const body = new Uint8Array(APPROVAL_PROGRESS_SIZE);
  body[0] = V2_APPROVAL_PROGRESS_OP;
  view(body).setBigUint64(1, generation);
  return body;
}
export function encodePaymentReview(request: Uint8Array, quote: Uint8Array): Uint8Array {
  if (request.length !== ID || !request.some(Boolean) || quote.length !== SIGNED_QUOTE)
    throw new Error("PAYMENT_BOARD_BINDING");
  return concat(Uint8Array.of(V2_PAYMENT_REVIEW_OP), request, quote);
}

export function readPaymentReview(request: Uint8Array, reply: Uint8Array): "ready" | "refused" {
  if (request.length !== 1 + ID + SIGNED_QUOTE || request[0] !== V2_PAYMENT_REVIEW_OP ||
      !request.subarray(1, 1 + ID).some(Boolean) ||
      reply.length !== 34 || reply[0] !== V2_PAYMENT_REVIEW_OP ||
      !equalBytes(reply.subarray(1, 33), sha256(request)) || ![0, 1].includes(reply[33]))
    throw new Error("PAYMENT_BOARD_EVIDENCE");
  return reply[33] === 1 ? "ready" : "refused";
}

const FRAME = 22;
const HEADER = 25;
const CHUNK = 192;
const MAX_BODY = 2048;
const MAX_SEQUENCE = 0xffffffff;
const MAX_TTL = 300n;
const FRAGMENT_TIMEOUT = 5n;
const MAX_TIME = 0xffffffffffffffffn;

export function v2PeerRole(profile: V2Profile, local: V2Role): V2Role {
  const [initiator, receiver] = ROLES[profile];
  if (local === initiator) return receiver;
  if (local === receiver) return initiator;
  throw new Error("role does not belong to this TLS profile");
}

function allowed(profile: V2Profile, role: V2Role, command: V2Command): boolean {
  const [initiator, receiver] = ROLES[profile];
  if (role === receiver) return command === "reply" ||
    (profile === "management" && command === "approval.progress") ||
    (profile === "payment" && command === "rental.review");
  if (role !== initiator) return false;
  if (command === "status") return true;
  if (profile === "management") return command === "manage" || command === "pay.prepare";
  if (profile === "payment") return command === "pay.prepare";
  return command === "nearby.submit";
}

function view(bytes: Uint8Array): DataView {
  return new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
}

/** Retain this journal across authenticated reconnects. Durable order effects belong to the application. */
export class V2Journal {
  private readonly records = new Map<string, { digest: Uint8Array; retainUntil: bigint }>();
  private readonly capacity: number;
  private trustedTime = 0n;

  constructor(capacity: number) {
    if (!Number.isSafeInteger(capacity) || capacity <= 0) throw new Error("invalid journal capacity");
    this.capacity = capacity;
  }

  observeTime(now: bigint): void {
    if (typeof now !== "bigint" || now < this.trustedTime || now > MAX_TIME) {
      throw new Error("invalid or backwards trusted clock");
    }
    this.trustedTime = now;
  }

  record(profile: V2Profile, peer: Uint8Array, request: V2Request, body: Uint8Array, now: bigint): V2Result["disposition"] {
    this.observeTime(now);
    for (const [key, record] of this.records) if (record.retainUntil <= now) this.records.delete(key);
    const key = `${profile}/${bytesToHex(peer)}/${bytesToHex(request.id)}`;
    const digest = sha256(body);
    const prior = this.records.get(key);
    if (prior) {
      if (!equalBytes(digest, prior.digest)) throw new Error("request ID conflict");
      return "resume";
    }
    if (this.records.size >= this.capacity) throw new Error("request journal full");
    this.records.set(key, { digest, retainUntil: request.expires + MAX_TTL });
    return "new";
  }
}

/** Frames are application data inside the verified TLS stream. Epoch is its TLS exporter. */
export class V2Link {
  private readonly profile: V2Profile;
  private readonly local: V2Role;
  private readonly peer: V2Role;
  private readonly epoch: Uint8Array;
  private readonly identity: Uint8Array;
  private readonly journal: V2Journal;
  private closed = false;
  private sent = 0;
  private received = 0;
  private part = 0;
  private count = 0;
  private size = 0;
  private started = 0n;
  private pending?: { id: Uint8Array; expires: bigint; command: V2Command; fresh: boolean; reported: boolean; rentalReview?: {challenge: Uint8Array; quoteHash: Uint8Array} };
  private approvalGeneration = 0n;
  private readonly body = new Uint8Array(MAX_BODY);

  constructor(profile: V2Profile, local: V2Role, epoch: Uint8Array, identity: Uint8Array, journal: V2Journal) {
    if (epoch.length !== EPOCH || identity.length !== IDENTITY) throw new Error("invalid TLS peer binding");
    this.profile = profile;
    this.local = local;
    this.peer = v2PeerRole(profile, local);
    this.epoch = epoch.slice();
    this.identity = identity.slice();
    this.journal = journal;
  }

  /** Call only after opt-in and owner generation are confirmed on this authenticated link. */
  enableApprovalProgress(generation: bigint): void {
    this.ensureOpen();
    if (this.profile !== "management" || this.pending || this.approvalGeneration ||
        typeof generation !== "bigint" || generation <= 0n || generation > MAX_TIME) {
      throw new Error("invalid approval progress negotiation");
    }
    this.approvalGeneration = generation;
  }

  acceptApprovalProgressOffer(reply: Uint8Array, generation: bigint): void {
    if (!equalBytes(reply, encodeApprovalProgressOffer(generation))) {
      throw new Error("unverified approval progress negotiation");
    }
    this.enableApprovalProgress(generation);
  }

  send(request: V2Request): Uint8Array[] {
    this.ensureOpen();
    if (!allowed(this.profile, this.local, request.command)) throw new Error("command outside authenticated role");
    if (request.id.length !== ID || !request.id.some(Boolean) || request.expires <= 0n ||
        request.expires > MAX_TIME || request.payload.length > MAX_BODY - HEADER) {
      throw new Error("invalid request bounds");
    }
    this.assertOutstanding(request);
    if (this.sent === MAX_SEQUENCE) return this.fail("send sequence exhausted");
    const body = new Uint8Array(HEADER + request.payload.length);
    body[0] = COMMANDS.indexOf(request.command) + 1;
    body.set(request.id, 1);
    view(body).setBigUint64(17, request.expires);
    body.set(request.payload, HEADER);
    const count = Math.ceil(body.length / CHUNK);
    const fragments: Uint8Array[] = [];
    for (let index = 0; index < count; index++) {
      const part = body.subarray(index * CHUNK, (index + 1) * CHUNK);
      const frame = new Uint8Array(FRAME + part.length);
      frame.set(this.epoch);
      view(frame).setUint32(EPOCH, this.sent);
      frame[20] = index;
      frame[21] = count;
      frame.set(part, FRAME);
      fragments.push(frame);
    }
    this.sent++;
    this.updateOutstanding(request, true);
    return fragments;
  }

  receive(frame: Uint8Array, now: bigint): V2Result | undefined {
    this.ensureOpen();
    const length = frame.length - FRAME;
    if (length <= 0 || length > CHUNK || !equalBytes(frame.subarray(0, EPOCH), this.epoch)) return this.fail("invalid frame or TLS epoch");
    if (this.received === MAX_SEQUENCE || view(frame).getUint32(EPOCH) !== this.received) return this.fail("invalid frame sequence");
    const [part, count] = [frame[20], frame[21]];
    if (count < 1 || count > Math.ceil(MAX_BODY / CHUNK) || part >= count || part !== this.part ||
        (part > 0 && count !== this.count) || (part + 1 < count && length !== CHUNK) ||
        this.size + length > MAX_BODY) return this.fail("invalid fragment order or bounds");
    try {
      this.journal.observeTime(now);
    } catch (error) {
      return this.fail((error as Error).message);
    }
    if (this.part && now - this.started >= FRAGMENT_TIMEOUT) return this.fail("fragment timeout");
    this.count = count;
    if (this.part === 0) this.started = now;
    this.part++;
    this.body.set(frame.subarray(FRAME), this.size);
    this.size += length;
    if (this.part < count) return;

    const body = this.body.subarray(0, this.size);
    const command = COMMANDS[body[0] - 1];
    const id = body.slice(1, 17);
    if (body.length < HEADER || !id.some(Boolean) || !command || !allowed(this.profile, this.peer, command)) {
      return this.fail("invalid request or authenticated role");
    }
    const expires = view(body).getBigUint64(17);
    if (now < 0n || expires <= now || expires - now > MAX_TTL) return this.fail("invalid request expiry");
    const request = { command, id, expires, payload: body.slice(HEADER) };
    let disposition: V2Result["disposition"];
    try {
      this.assertOutstanding(request);
      disposition = request.command === "reply" || request.command === "approval.progress" || request.command === "rental.review"
        ? "new" : this.journal.record(this.profile, this.identity, request, body, now);
    } catch (error) {
      return this.fail((error as Error).message);
    }
    this.body.fill(0);
    this.count = this.part = this.size = 0;
    this.received++;
    this.updateOutstanding(request, disposition === "new");
    return { disposition, request };
  }

  close(): void {
    this.body.fill(0);
    this.epoch.fill(0);
    this.pending = undefined;
    this.approvalGeneration = 0n;
    this.closed = true;
  }

  private ensureOpen(): void {
    if (this.closed) throw new Error("TLS application link is closed");
  }

  private assertOutstanding(request: V2Request): void {
    if (request.command === "reply" || request.command === "approval.progress" || request.command === "rental.review") {
      if (!this.pending || this.pending.expires !== request.expires || !equalBytes(this.pending.id, request.id)) {
        throw new Error("reply does not match the outstanding request");
      }
      if (request.command === "approval.progress" &&
          (!this.approvalGeneration || this.pending.command !== "pay.prepare" ||
           !this.pending.fresh || this.pending.reported || request.payload.length !== APPROVAL_PROGRESS_SIZE || request.payload[0] !== 1 ||
           view(request.payload).getBigUint64(1) !== this.approvalGeneration)) {
        throw new Error("invalid approval progress");
      }
      if (request.command === "rental.review") {
        const body = request.payload;
        const peer = this.local === "board" ? 36 : 4;
        if (!this.pending.rentalReview || !this.pending.fresh || this.pending.reported ||
            body.length !== rentalReviewReceiptSize || !equalBytes(body.subarray(0, 4), Uint8Array.of(87, 49, 48, 82)) ||
            !equalBytes(body.subarray(peer, peer + IDENTITY), this.identity) || !equalBytes(body.subarray(68,84), this.epoch) ||
            !equalBytes(body.subarray(84,100), request.id) ||
            !equalBytes(body.subarray(100,132), this.pending.rentalReview.challenge) ||
            !equalBytes(body.subarray(164,196), this.pending.rentalReview.quoteHash) ||
            view(body).getBigUint64(224) !== request.expires)
          throw new Error("invalid rental review");
      }
    } else if (this.pending) {
      throw new Error("an outstanding request already exists");
    }
  }

  private updateOutstanding(request: V2Request, fresh: boolean): void {
    if (request.command === "approval.progress" || request.command === "rental.review") {
      this.pending!.reported = true;
    } else {
      this.pending = request.command === "reply" ? undefined : {
        id: request.id.slice(), expires: request.expires, command: request.command, fresh, reported: false,
        rentalReview: this.profile === "payment" && request.command === "pay.prepare" && isRentalReviewPrepare(request.payload)
          ? { challenge: request.payload.slice(4,36), quoteHash: sha256(request.payload.subarray(36)) } : undefined,
      };
    }
  }

  private fail(reason: string): never {
    this.close();
    throw new Error(reason);
  }
}

export function encodeV2StreamFrame(frame: Uint8Array): Uint8Array {
  if (frame.length <= FRAME || frame.length > FRAME + CHUNK) throw new Error("invalid v2 frame length");
  const wire = new Uint8Array(2 + frame.length);
  view(wire).setUint16(0, frame.length);
  wire.set(frame, 2);
  return wire;
}

/** Delimits bounded v2 fragments from an ordered TLS application byte stream. */
export class V2StreamDecoder {
  private readonly buffer = new Uint8Array(2 + FRAME + CHUNK);
  private have = 0;
  private wanted = 2;
  private closed = false;

  feed(bytes: Uint8Array, frame: (value: Uint8Array) => void): void {
    if (this.closed) throw new Error("v2 stream is closed");
    for (let cursor = 0; cursor < bytes.length;) {
      const size = Math.min(this.wanted - this.have, bytes.length - cursor);
      this.buffer.set(bytes.subarray(cursor, cursor + size), this.have);
      this.have += size;
      cursor += size;
      if (this.have !== this.wanted) continue;
      if (this.wanted === 2) {
        const length = view(this.buffer).getUint16(0);
        if (length <= FRAME || length > FRAME + CHUNK) {
          this.close();
          throw new Error("invalid v2 frame length");
        }
        this.wanted += length;
      } else {
        const value = this.buffer.slice(2, this.wanted);
        this.have = 0;
        this.wanted = 2;
        try {
          frame(value);
        } catch (error) {
          this.close();
          throw error;
        }
      }
    }
  }

  close(): void {
    this.buffer.fill(0);
    this.closed = true;
  }
}
