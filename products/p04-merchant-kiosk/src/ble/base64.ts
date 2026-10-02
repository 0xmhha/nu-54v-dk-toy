// Base64 (RFC 4648, with padding) for bytes crossing the native module boundary.
/* eslint-disable no-bitwise -- base64 is bit packing */

const ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
const INDEX = new Map([...ALPHABET].map((c, i) => [c, i] as const));

export function toBase64(b: Uint8Array): string {
  let out = "";
  for (let i = 0; i < b.length; i += 3) {
    const n = (b[i] << 16) | ((b[i + 1] ?? 0) << 8) | (b[i + 2] ?? 0);
    out += ALPHABET[(n >> 18) & 63] + ALPHABET[(n >> 12) & 63];
    out += i + 1 < b.length ? ALPHABET[(n >> 6) & 63] : "=";
    out += i + 2 < b.length ? ALPHABET[n & 63] : "=";
  }
  return out;
}

export function fromBase64(s: string): Uint8Array {
  const clean = s.replace(/[=]+$/, "");
  const out = new Uint8Array(Math.floor((clean.length * 3) / 4));
  let bits = 0;
  let acc = 0;
  let j = 0;
  for (const c of clean) {
    const v = INDEX.get(c);
    if (v === undefined) throw new Error(`not base64: ${c}`);
    acc = (acc << 6) | v;
    bits += 6;
    if (bits >= 8) {
      bits -= 8;
      out[j++] = (acc >> bits) & 0xff;
    }
  }
  return out;
}
