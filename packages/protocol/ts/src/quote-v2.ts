import { keccak_256 } from "@noble/hashes/sha3";
import { bytesToBig, bytesToHex, concat, equalBytes, utf8Encode } from "./bytes.ts";
import { recoverSigner } from "./signature.ts";
import * as layout from "./quote-v2-layout.ts";

export const quoteV2Layout = layout;

export interface QuotePolicy {
  expected: Uint8Array;
  merchantSigner: Uint8Array;
  now: bigint;
  consumed: boolean;
}

const hashText = (text: string) => keccak_256(utf8Encode(text));
const domainType = hashText(layout.domainType);
const domainName = hashText(layout.domainName);
const domainVersion = hashText(layout.domainVersion);
const quoteType = hashText(layout.quoteType);
const word = (raw: Uint8Array, offset: number) => raw.subarray(offset, offset + 32);
const value = (raw: Uint8Array, offset: number) => bytesToBig(word(raw, offset));

function valid(raw: Uint8Array): boolean {
  if (raw.length !== layout.unsignedSize) return false;
  if (!layout.widths.every((width, index) => {
    const field = word(raw, index * 32);
    return !field.subarray(0, 32 - width).some(Boolean) && field.subarray(32 - width).some(Boolean);
  })) return false;
  const mode = raw[layout.modeOffset + 31];
  const role = raw[layout.payerRoleOffset + 31];
  return (mode === layout.escrowMode || mode === layout.directMode)
    && (role === layout.ownerRole || role === layout.renterRole)
    && (mode !== layout.directMode || role === layout.ownerRole)
    && value(raw, layout.expiresAtOffset) > value(raw, layout.issuedAtOffset);
}

/** Encode the ordered fixed-width fields from the v2 layout, without ABI padding in the inputs. */
export function encodeQuote(fields: readonly Uint8Array[]): Uint8Array {
  if (fields.length !== layout.widths.length) throw new Error("invalid quote field count");
  const raw = new Uint8Array(layout.unsignedSize);
  for (const [index, width] of layout.widths.entries()) {
    if (fields[index].length !== width) throw new Error("invalid quote field width");
    raw.set(fields[index], (index + 1) * 32 - width);
  }
  if (!valid(raw)) throw new Error("malformed v2 quote");
  return raw;
}

export function quoteDigest(raw: Uint8Array): Uint8Array {
  if (!valid(raw)) throw new Error("malformed v2 quote");
  const domain = keccak_256(concat(domainType, domainName, domainVersion, raw.subarray(0, layout.quoteIdOffset)));
  const quote = keccak_256(concat(quoteType, raw.subarray(layout.quoteIdOffset)));
  return keccak_256(concat(Uint8Array.of(0x19, 0x01), domain, quote));
}

/** Check the signed quote against independent authenticated order and merchant state. */
export function verifyQuote(signed: Uint8Array, policy: QuotePolicy): number {
  if (!(signed instanceof Uint8Array) || signed.length !== layout.unsignedSize + layout.signatureSize
    || !policy || typeof policy !== "object") return layout.statusMalformed;
  const { expected, merchantSigner, now, consumed } = policy;
  if (!(expected instanceof Uint8Array) || !valid(expected)
    || !(merchantSigner instanceof Uint8Array) || merchantSigner.length !== 20 || !merchantSigner.some(Boolean)
    || typeof now !== "bigint" || now < 0n || now >= 1n << 64n
    || typeof consumed !== "boolean") return layout.statusMalformed;
  const raw = signed.subarray(0, layout.unsignedSize);
  if (!valid(raw)) return layout.statusMalformed;
  if (value(raw, layout.chainIdOffset) !== BigInt(layout.pilotChain)
    || raw[layout.modeOffset + 31] !== layout.escrowMode) return layout.statusUnsupportedProfile;
  if (!equalBytes(raw, expected)) return layout.statusWrongBinding;
  if (now < value(raw, layout.issuedAtOffset) || now >= value(raw, layout.expiresAtOffset)) {
    return layout.statusExpired;
  }
  try {
    if (recoverSigner(quoteDigest(raw), signed.subarray(layout.unsignedSize))
      !== bytesToHex(merchantSigner)) return layout.statusUntrustedSigner;
  } catch {
    return layout.statusUntrustedSigner;
  }
  return consumed ? layout.statusConsumed : layout.statusOk;
}
