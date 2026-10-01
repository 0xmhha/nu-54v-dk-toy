// Small byte helpers shared by the codec, framing and EIP-712 modules.

export class ProtocolError extends Error {
  readonly reason: "BAD_FRAME" | "UNSUPPORTED_TYPE";
  constructor(reason: "BAD_FRAME" | "UNSUPPORTED_TYPE", detail: string) {
    super(`${reason}: ${detail}`);
    this.reason = reason;
  }
}

export function hexToBytes(hex: string): Uint8Array {
  const h = hex.startsWith("0x") || hex.startsWith("0X") ? hex.slice(2) : hex;
  if (h.length % 2 !== 0 || !/^[0-9a-fA-F]*$/.test(h)) throw new Error(`not hex: ${hex}`);
  const out = new Uint8Array(h.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(h.slice(2 * i, 2 * i + 2), 16);
  return out;
}

export function bytesToHex(b: Uint8Array, prefix = true): string {
  let s = "";
  for (const x of b) s += x.toString(16).padStart(2, "0");
  return prefix ? `0x${s}` : s;
}

export function concat(...parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let o = 0;
  for (const p of parts) {
    out.set(p, o);
    o += p.length;
  }
  return out;
}

export function compareBytes(a: Uint8Array, b: Uint8Array): number {
  for (let i = 0; i < Math.min(a.length, b.length); i++) if (a[i] !== b[i]) return a[i] - b[i];
  return a.length - b.length;
}

export function equalBytes(a: Uint8Array, b: Uint8Array): boolean {
  return compareBytes(a, b) === 0;
}

/** Big-endian bytes of a non-negative integer, padded to `size` bytes (0 = shortest). */
export function bigToBytes(n: bigint, size = 0): Uint8Array {
  if (n < 0n) throw new Error("negative");
  let h = n.toString(16);
  if (h.length % 2) h = "0" + h;
  if (n === 0n) h = "";
  const raw = hexToBytes(h);
  if (size === 0) return raw;
  if (raw.length > size) throw new Error(`value does not fit ${size} bytes`);
  return concat(new Uint8Array(size - raw.length), raw);
}

export function bytesToBig(b: Uint8Array): bigint {
  return b.length === 0 ? 0n : BigInt(bytesToHex(b));
}

// UTF-8 without TextEncoder/TextDecoder, which React Native runtimes do not all provide.

export function utf8Encode(s: string): Uint8Array {
  const out: number[] = [];
  for (const ch of s) {
    const c = ch.codePointAt(0)!;
    if (c < 0x80) out.push(c);
    else if (c < 0x800) out.push(0xc0 | (c >> 6), 0x80 | (c & 0x3f));
    else if (c < 0x10000) out.push(0xe0 | (c >> 12), 0x80 | ((c >> 6) & 0x3f), 0x80 | (c & 0x3f));
    else out.push(0xf0 | (c >> 18), 0x80 | ((c >> 12) & 0x3f), 0x80 | ((c >> 6) & 0x3f), 0x80 | (c & 0x3f));
  }
  return Uint8Array.from(out);
}

/** Strict UTF-8 decoding: overlong forms, surrogates and truncated sequences throw. */
export function utf8Decode(b: Uint8Array): string {
  let s = "";
  for (let i = 0; i < b.length; ) {
    const x = b[i];
    const n = x < 0x80 ? 0 : x >= 0xc2 && x <= 0xdf ? 1 : x >= 0xe0 && x <= 0xef ? 2 : x >= 0xf0 && x <= 0xf4 ? 3 : -1;
    if (n < 0 || i + n >= b.length) throw new Error("invalid UTF-8"); // bad lead byte or truncated
    let c = n === 0 ? x : x & (0x3f >> n);
    for (let k = 1; k <= n; k++) {
      const y = b[i + k];
      if (y === undefined || (y & 0xc0) !== 0x80) throw new Error("invalid UTF-8");
      c = (c << 6) | (y & 0x3f);
    }
    if ((n === 2 && (c < 0x800 || (c >= 0xd800 && c <= 0xdfff))) || (n === 3 && (c < 0x10000 || c > 0x10ffff))) {
      throw new Error("invalid UTF-8");
    }
    s += String.fromCodePoint(c);
    i += n + 1;
  }
  return s;
}
