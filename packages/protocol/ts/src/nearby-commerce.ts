import { fromBase64, toBase64 } from "./base64.ts";
import { hexToBytes, utf8Decode, utf8Encode } from "./bytes.ts";
import { encodeCommerceCart, type CommerceCart } from "./commerce-v2.ts";
import {
  encodeV2StreamFrame,
  V2Journal,
  V2Link,
  V2StreamDecoder,
  type V2Request,
} from "./transport-v2.ts";

export type NearbyRequest =
  | { op: "discover"; branch: string }
  | { op: "catalog"; branch: string; offset: number }
  | {
      op: "reserve";
      branch: string;
      key: string;
      cart: CommerceCart;
      asset: string;
      deadline?: number;
    }
  | { op: "status"; branch: string; key: string; order?: string; authoritative?: true }
  | { op: "checkout"; branch: string; key: string; order: string; authoritative?: true }
  | { op: "rental-review-source"; branch: string; key: string; order: string; value: unknown }
  | {
      op: "rental-review";
      branch: string;
      key: string;
      order: string;
      request: string;
      challenge: string;
      quoteDigest: string;
      authority: unknown;
    }
  | {
      op: "rental-decision";
      branch: string;
      key: string;
      order: string;
      request: string;
      challenge: string;
      quoteDigest: string;
      reviewDigest: string;
    }
  | {
      op: "forward";
      branch: string;
      key: string;
      order: string;
      quoteDigest: string;
      approvalDigest: string;
      signature: string;
      authoritative?: true;
    };
type Code = "ok" | "pending" | "denied" | "sold-out" | "catalog-changed" | "expired";
export type NearbyReply = { status: Code; value?: unknown };
export type NearbyReady = {
  kind: "ready";
  link: string;
  epoch: string;
  peer: string;
  principal: string;
  branch: string;
  expiresAt: number;
};

type NativeNearby = {
  close(link: string): Promise<void>;
  write(link: string, data: string): Promise<void>;
};
const reference =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const now = () => Math.floor(Date.now() / 1000);
const hasKeys = (value: object, keys: string[]) =>
  Object.keys(value).sort().join() === keys.sort().join();

export function decodeNearbyRequest(
  payload: Uint8Array,
  branch: string
): NearbyRequest {
  if (!payload.length || payload.length > 1900)
    throw new Error("NEARBY_BOUNDS");
  const req = JSON.parse(utf8Decode(payload)) as NearbyRequest;
  if (!req || req.branch !== branch || !reference.test(branch))
    throw new Error("NEARBY_DOMAIN");
  switch (req.op) {
    case "discover":
      if (!hasKeys(req, ["op", "branch"])) throw new Error("NEARBY_COMMAND");
      break;
    case "catalog":
      if (
        !hasKeys(req, ["op", "branch", "offset"]) ||
        !Number.isSafeInteger(req.offset) ||
        req.offset < 0 ||
        req.offset > 300
      )
        throw new Error("NEARBY_COMMAND");
      break;
    case "reserve":
      if (
        !hasKeys(req, ["op", "branch", "key", "cart", "asset", ...(req.deadline === undefined ? [] : ["deadline"])]) ||
        (req.deadline !== undefined && (!Number.isSafeInteger(req.deadline) || req.deadline < 1)) ||
        !reference.test(req.key) ||
        !/^0x[0-9a-f]{40}$/.test(req.asset) ||
        !req.cart ||
        req.cart.origin !== "menu" ||
        req.cart.nearby !== undefined || req.cart.pickup !== undefined ||
        encodeCommerceCart(req.cart) !== JSON.stringify(req.cart)
      )
        throw new Error("NEARBY_COMMAND");
      break;
    case "status":
      if (
        !hasKeys(
          req,
          ["op", "branch", "key", ...(req.order === undefined ? [] : ["order"]),
            ...(req.authoritative === undefined ? [] : ["authoritative"])]
        ) ||
        (req.authoritative !== undefined && req.authoritative !== true) ||
        !reference.test(req.key) ||
        (req.order !== undefined && !reference.test(req.order))
      )
        throw new Error("NEARBY_COMMAND");
      break;
    case "forward":
      if (
        !hasKeys(req, [
          "op",
          "branch",
          "key",
          "order",
          "quoteDigest",
          "approvalDigest",
          "signature",
          ...(req.authoritative === undefined ? [] : ["authoritative"]),
        ]) ||
        (req.authoritative !== undefined && req.authoritative !== true) ||
        !reference.test(req.key) ||
        !reference.test(req.order) ||
        !/^[a-f0-9]{64}$/.test(req.quoteDigest) ||
        !/^[a-f0-9]{64}$/.test(req.approvalDigest) ||
        !/^[a-f0-9]{128}(1b|1c)$/.test(req.signature)
      )
        throw new Error("NEARBY_COMMAND");
      break;
    case "rental-review-source":
      if (!hasKeys(req, ["op", "branch", "key", "order", "value"]) ||
          !reference.test(req.key) || !reference.test(req.order) ||
          !req.value || typeof req.value !== "object" || Array.isArray(req.value))
        throw new Error("NEARBY_COMMAND");
      break;
    case "rental-decision":
      if (!hasKeys(req, ["op", "branch", "key", "order", "request", "challenge", "quoteDigest", "reviewDigest"]) ||
          !reference.test(req.key) || !reference.test(req.order) ||
          !/^[a-f0-9]{32}$/.test(req.request) || /^0+$/.test(req.request) ||
          !/^[a-f0-9]{64}$/.test(req.challenge) || /^0+$/.test(req.challenge) ||
          !/^[a-f0-9]{64}$/.test(req.quoteDigest) || !/^[a-f0-9]{64}$/.test(req.reviewDigest))
        throw new Error("NEARBY_COMMAND");
      break;
    case "rental-review":
      if (!hasKeys(req, ["op", "branch", "key", "order", "request", "challenge", "quoteDigest", "authority"]) ||
          !reference.test(req.key) || !reference.test(req.order) ||
          !/^[a-f0-9]{32}$/.test(req.request) || /^0+$/.test(req.request) ||
          !/^[a-f0-9]{64}$/.test(req.challenge) || /^0+$/.test(req.challenge) ||
          !/^[a-f0-9]{64}$/.test(req.quoteDigest) ||
          !req.authority || typeof req.authority !== "object" || Array.isArray(req.authority))
        throw new Error("NEARBY_COMMAND");
      break;
    case "checkout":
      if (
        !hasKeys(req, ["op", "branch", "key", "order", ...(req.authoritative === undefined ? [] : ["authoritative"])]) ||
        (req.authoritative !== undefined && req.authoritative !== true) ||
        !reference.test(req.key) ||
        !reference.test(req.order)
      )
        throw new Error("NEARBY_COMMAND");
      break;
    default:
      throw new Error("NEARBY_COMMAND");
  }
  return req;
}

export function readNearbyReady(value: unknown): NearbyReady {
  const ready = value as NearbyReady;
  if (
    !ready ||
    ready.kind !== "ready" ||
    typeof ready.link !== "string" ||
    !ready.link.length ||
    ready.link.length > 80 ||
    typeof ready.epoch !== "string" ||
    !/^[0-9a-f]{64}$/.test(ready.peer) ||
    !reference.test(ready.principal) ||
    !reference.test(ready.branch) ||
    !Number.isSafeInteger(ready.expiresAt) ||
    ready.expiresAt <= now()
  )
    throw new Error("NEARBY_TRUST_REQUIRED");
  const epoch = fromBase64(ready.epoch);
  if (epoch.length !== 16 || !epoch.some(Boolean))
    throw new Error("NEARBY_TRUST_REQUIRED");
  return { ...ready };
}

export class NearbyCommerceLink {
  readonly peer: NearbyReady;
  private readonly side: "terminal" | "customer";
  private readonly native: NativeNearby;
  private readonly codec: V2Link;
  private readonly stream = new V2StreamDecoder();
  private readonly dispatch?: (
    input: NearbyRequest,
    peer: NearbyReady
  ) => Promise<NearbyReply>;
  private received = Promise.resolve();
  private done = false;
  private awaiting?: {
    resolve: (response: NearbyReply) => void;
    reject: (error: Error) => void;
  };

  constructor(
    peer: NearbyReady,
    side: "terminal" | "customer",
    native: NativeNearby,
    journal: V2Journal,
    dispatch?: (input: NearbyRequest, peer: NearbyReady) => Promise<NearbyReply>
  ) {
    this.peer = readNearbyReady(peer);
    this.side = side;
    this.native = native;
    this.dispatch = dispatch;
    this.codec = new V2Link(
      "nearby",
      side,
      fromBase64(peer.epoch),
      hexToBytes(peer.peer),
      journal
    );
  }

  receive(frame: string) {
    this.received = this.received
      .then(async () => {
        if (this.done) return;
        const data = fromBase64(frame);
        if (data.length > 3072) throw new Error("NEARBY_BOUNDS");
        const messages: V2Request[] = [];
        this.stream.feed(data, (chunk) => {
          const result = this.codec.receive(chunk, BigInt(now()));
          if (result) messages.push(result.request);
        });
        for (const message of messages) {
          if (this.peer.expiresAt <= now())
            throw new Error("NEARBY_CREDENTIAL_EXPIRED");
          if (
            this.side === "customer" &&
            message.command === "reply" &&
            this.awaiting
          ) {
            const reply = JSON.parse(
              utf8Decode(message.payload)
            ) as NearbyReply;
            if (
              !reply ||
              !["ok", "pending", "denied", "sold-out", "catalog-changed", "expired"].includes(reply.status)
            )
              throw new Error("NEARBY_REPLY");
            const pending = this.awaiting;
            this.awaiting = undefined;
            pending.resolve(reply);
          } else if (
            this.side === "terminal" &&
            message.command === "nearby.submit" &&
            this.dispatch
          ) {
            const request = decodeNearbyRequest(
              message.payload,
              this.peer.branch
            );
            const reply = await this.dispatch(request, this.peer);
            if (
              this.done ||
              this.peer.expiresAt <= now() ||
              message.expires <= BigInt(now())
            )
              throw new Error("NEARBY_RETIRED");
            await this.send({
              ...message,
              command: "reply",
              payload: utf8Encode(JSON.stringify(reply)),
            });
          } else {
            throw new Error("NEARBY_ROLE");
          }
        }
      })
      .catch(() => this.close());
  }

  private async send(request: V2Request) {
    for (const frame of this.codec.send(request))
      await this.native.write(
        this.peer.link,
        toBase64(encodeV2StreamFrame(frame))
      );
  }

  async request(req: NearbyRequest, id: Uint8Array): Promise<NearbyReply> {
    if (this.done || this.side !== "customer" || this.awaiting)
      throw new Error("NEARBY_LINK_STATE");
    const expires = Math.min(now() + 20, this.peer.expiresAt);
    if (expires <= now()) throw new Error("NEARBY_CREDENTIAL_EXPIRED");
    const payload = utf8Encode(JSON.stringify(req));
    decodeNearbyRequest(payload, this.peer.branch);
    const flight = new Promise<NearbyReply>((resolve, reject) => {
      this.awaiting = { resolve, reject };
    });
    const timer = setTimeout(() => this.close(), 8000);
    this.send({
      command: "nearby.submit",
      id,
      expires: BigInt(expires),
      payload,
    }).catch(() => this.close());
    try {
      return await flight;
    } finally {
      clearTimeout(timer);
    }
  }

  close() {
    if (this.done) return;
    this.done = true;
    this.stream.close();
    this.codec.close();
    this.awaiting?.reject(new Error("NEARBY_DISCONNECTED"));
    this.awaiting = undefined;
    this.native.close(this.peer.link).catch(() => {});
  }
}
