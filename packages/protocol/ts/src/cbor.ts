// Deterministic CBOR for BLE message bodies (payment-protocol.md 4.2).
//
// Messages use the JSON form of the schema and vectors: byte fields as hex strings
// (sessionId without 0x), uints as decimal strings (bigint and number are accepted when
// encoding), text, booleans and the integer `v`. Field kinds come from the generated
// MESSAGE_FIELDS, so encoding and decoding follow the schema exactly.

import { bigToBytes, bytesToBig, bytesToHex, compareBytes, concat, hexToBytes, ProtocolError, utf8Decode, utf8Encode } from "./bytes.ts";
import { MESSAGE_FIELDS, type FieldKind, type MessageType, type ObjectKind } from "./generated.ts";

export type JsonValue = string | number | boolean | bigint | { [k: string]: JsonValue };
export type Message = { type: MessageType; [k: string]: JsonValue };

const BYTE_LENGTHS: Record<string, number> = { hex20: 20, hex32: 32, signature: 65, sessionId: 8 };
const UINT64_LIMIT = 1n << 64n;
const enc = { encode: utf8Encode };
const dec = { decode: utf8Decode };

function head(major: number, n: bigint | number): Uint8Array {
  const v = BigInt(n);
  if (v < 24n) return Uint8Array.of((major << 5) | Number(v));
  for (const [info, size] of [[24, 1], [25, 2], [26, 4], [27, 8]] as const) {
    if (v < 1n << BigInt(8 * size)) return concat(Uint8Array.of((major << 5) | info), bigToBytes(v, size));
  }
  throw new Error("CBOR argument too large");
}

function encodeUint(v: bigint): Uint8Array {
  if (v < 0n) throw new Error("negative uint");
  if (v < UINT64_LIMIT) return head(0, v);
  const raw = bigToBytes(v); // tag 2 bignum, no leading zero bytes
  return concat(head(6, 2), head(2, raw.length), raw);
}

function encodeText(s: string): Uint8Array {
  const b = enc.encode(s);
  return concat(head(3, b.length), b);
}

function encodeValue(kind: FieldKind, value: JsonValue, path: string): Uint8Array {
  if (typeof kind === "object") {
    if (typeof value !== "object" || value === null || typeof value === "bigint") throw new Error(`${path}: object expected`);
    return encodeObject(kind, value as Record<string, JsonValue>, path);
  }
  if (kind in BYTE_LENGTHS) {
    const raw = hexToBytes(String(value));
    if (raw.length !== BYTE_LENGTHS[kind]) throw new Error(`${path}: ${kind} must be ${BYTE_LENGTHS[kind]} bytes`);
    return concat(head(2, raw.length), raw);
  }
  switch (kind) {
    case "uint":
      return encodeUint(BigInt(value as string | number | bigint));
    case "int":
      if (typeof value !== "number" || !Number.isInteger(value) || value < 0) throw new Error(`${path}: integer expected`);
      return head(0, value);
    case "bool":
      if (typeof value !== "boolean") throw new Error(`${path}: boolean expected`);
      return Uint8Array.of(value ? 0xf5 : 0xf4);
    case "text":
      if (typeof value !== "string") throw new Error(`${path}: text expected`);
      return encodeText(value);
  }
  throw new Error(`${path}: unknown kind ${String(kind)}`);
}

function encodeObject(kind: ObjectKind, value: Record<string, JsonValue>, path: string): Uint8Array {
  const keys = Object.keys(value);
  const unknown = keys.filter((k) => !(k in kind.fields));
  const missing = kind.required.filter((k) => !(k in value));
  if (unknown.length || missing.length) throw new Error(`${path}: unknown ${unknown}, missing ${missing}`);
  const items = keys
    .map((k) => [encodeText(k), encodeValue(kind.fields[k], value[k], `${path}.${k}`)] as const)
    .sort((a, b) => compareBytes(a[0], b[0])); // RFC 8949 4.2.1: by encoded key bytes
  return concat(head(5, items.length), ...items.flatMap(([k, v]) => [k, v]));
}

/** Encodes a message in the JSON form to its deterministic CBOR body. */
export function encodeMessage(message: Message): Uint8Array {
  const kind = MESSAGE_FIELDS[message.type];
  if (!kind) throw new ProtocolError("UNSUPPORTED_TYPE", `unknown message type ${String(message.type)}`);
  return encodeObject(kind, message, message.type);
}

// ---------------------------------------------------------------- decoding

type Item =
  | { major: 0; value: bigint }
  | { major: 2; value: Uint8Array }
  | { major: 3; value: string }
  | { major: 5; entries: [Uint8Array, Item][] }
  | { major: 6; value: bigint } // tag 2 bignum
  | { major: 7; value: boolean };

class Reader {
  i = 0;
  readonly b: Uint8Array;
  constructor(b: Uint8Array) {
    this.b = b;
  }
  take(n: number): Uint8Array {
    if (this.i + n > this.b.length) throw new ProtocolError("BAD_FRAME", "truncated CBOR");
    const out = this.b.subarray(this.i, this.i + n);
    this.i += n;
    return out;
  }
}

function readHead(r: Reader): { major: number; info: number; arg: bigint } {
  const ib = r.take(1)[0];
  const major = ib >> 5;
  const info = ib & 0x1f;
  if (info < 24) return { major, info, arg: BigInt(info) };
  const size = { 24: 1, 25: 2, 26: 4, 27: 8 }[info];
  if (!size) throw new ProtocolError("BAD_FRAME", "indefinite length or reserved value");
  const arg = bytesToBig(r.take(size));
  // Shortest form: the argument must not fit a smaller encoding.
  const min = size === 1 ? 24n : 1n << BigInt(8 * (size / 2));
  if (arg < min) throw new ProtocolError("BAD_FRAME", "non-shortest integer or length");
  return { major, info, arg };
}

function readItem(r: Reader, depth = 0): Item {
  if (depth > 4) throw new ProtocolError("BAD_FRAME", "nesting too deep");
  const start = r.i;
  const h = readHead(r);
  switch (h.major) {
    case 0:
      return { major: 0, value: h.arg };
    case 2:
      return { major: 2, value: r.take(Number(h.arg)) };
    case 3: {
      const raw = r.take(Number(h.arg)); // a short body is "truncated", not a UTF-8 error
      try {
        return { major: 3, value: dec.decode(raw) };
      } catch {
        throw new ProtocolError("BAD_FRAME", "invalid UTF-8");
      }
    }
    case 5: {
      const entries: [Uint8Array, Item][] = [];
      let prev: Uint8Array | null = null;
      for (let k = 0n; k < h.arg; k++) {
        const ks = r.i;
        const key = readItem(r, depth + 1);
        if (key.major !== 3) throw new ProtocolError("BAD_FRAME", "map key is not text");
        const kb = r.b.subarray(ks, r.i);
        if (prev && compareBytes(prev, kb) >= 0) throw new ProtocolError("BAD_FRAME", "map keys not in deterministic order or duplicated");
        prev = kb;
        entries.push([kb, readItem(r, depth + 1)]);
      }
      return { major: 5, entries };
    }
    case 6: {
      if (h.arg !== 2n) throw new ProtocolError("BAD_FRAME", "tag other than 2");
      const raw = readItem(r, depth + 1);
      if (raw.major !== 2) throw new ProtocolError("BAD_FRAME", "bignum is not a byte string");
      if (raw.value.length === 0 || raw.value[0] === 0) throw new ProtocolError("BAD_FRAME", "bignum with leading zero");
      const v = bytesToBig(raw.value);
      if (v < UINT64_LIMIT) throw new ProtocolError("BAD_FRAME", "bignum below 2^64");
      return { major: 6, value: v };
    }
    case 7:
      if (r.b[start] === 0xf4) return { major: 7, value: false };
      if (r.b[start] === 0xf5) return { major: 7, value: true };
      throw new ProtocolError("BAD_FRAME", "float or simple value");
  }
  throw new ProtocolError("BAD_FRAME", `unsupported major type ${h.major}`);
}

function toJson(kind: FieldKind, item: Item, path: string): JsonValue {
  const bad = (what: string) => new ProtocolError("BAD_FRAME", `${path}: ${what}`);
  if (typeof kind === "object") {
    if (item.major !== 5) throw bad("map expected");
    const out: Record<string, JsonValue> = {};
    for (const [kb, v] of item.entries) {
      const key = dec.decode(kb.subarray(headLength(kb)));
      if (!(key in kind.fields)) throw bad(`unknown key ${key}`);
      out[key] = toJson(kind.fields[key], v, `${path}.${key}`);
    }
    const missing = kind.required.filter((k) => !(k in out));
    if (missing.length) throw bad(`missing ${missing}`);
    return out;
  }
  if (kind in BYTE_LENGTHS) {
    if (item.major !== 2 || item.value.length !== BYTE_LENGTHS[kind]) throw bad(`${kind} must be ${BYTE_LENGTHS[kind]} bytes`);
    return bytesToHex(item.value, kind !== "sessionId");
  }
  switch (kind) {
    case "uint":
      if (item.major !== 0 && item.major !== 6) throw bad("uint expected");
      return item.value.toString();
    case "int":
      if (item.major !== 0 || item.value > BigInt(Number.MAX_SAFE_INTEGER)) throw bad("integer expected");
      return Number(item.value);
    case "bool":
      if (item.major !== 7) throw bad("boolean expected");
      return item.value;
    case "text":
      if (item.major !== 3) throw bad("text expected");
      return item.value;
  }
  throw bad("unknown kind");
}

function headLength(encodedKey: Uint8Array): number {
  const info = encodedKey[0] & 0x1f;
  return 1 + ({ 24: 1, 25: 2, 26: 4, 27: 8 }[info] ?? 0);
}

/** Decodes and validates a CBOR body; any rule violation is ProtocolError BAD_FRAME. */
export function decodeMessage(body: Uint8Array): Message {
  const r = new Reader(body);
  const item = readItem(r);
  if (r.i !== body.length) throw new ProtocolError("BAD_FRAME", "trailing bytes after the CBOR item");
  if (item.major !== 5) throw new ProtocolError("BAD_FRAME", "body is not a map");
  const typeEntry = item.entries.find(([kb]) => dec.decode(kb.subarray(headLength(kb))) === "type");
  if (!typeEntry || typeEntry[1].major !== 3) throw new ProtocolError("BAD_FRAME", "missing type");
  const type = typeEntry[1].value as MessageType;
  const kind = MESSAGE_FIELDS[type];
  if (!kind) throw new ProtocolError("UNSUPPORTED_TYPE", `unknown message type ${type}`);
  return toJson(kind, item, type) as Message;
}
