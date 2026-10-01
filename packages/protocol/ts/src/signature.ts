// secp256k1 signatures as the contract and the device accept them:
// 65 bytes r || s || v, low-s only, v = 27 or 28.

import { secp256k1 } from "@noble/curves/secp256k1";
import { keccak_256 } from "@noble/hashes/sha3";
import { bigToBytes, bytesToBig, bytesToHex, concat } from "./bytes.ts";

const HALF_ORDER = secp256k1.CURVE.n >> 1n;

export class SignatureError extends Error {}

export function addressOfPublicKey(uncompressed: Uint8Array): string {
  return bytesToHex(keccak_256(uncompressed.subarray(1)).subarray(12));
}

export function addressOfPrivateKey(privateKey: Uint8Array): string {
  return addressOfPublicKey(secp256k1.getPublicKey(privateKey, false));
}

/** Recovers the signer address (lower-case hex) or throws SignatureError. */
export function recoverSigner(digest32: Uint8Array, signature: Uint8Array): string {
  if (signature.length !== 65) throw new SignatureError("signature must be 65 bytes");
  const r = bytesToBig(signature.subarray(0, 32));
  const s = bytesToBig(signature.subarray(32, 64));
  const v = signature[64];
  if (v !== 27 && v !== 28) throw new SignatureError("v must be 27 or 28");
  if (s > HALF_ORDER) throw new SignatureError("high-s signature");
  const sig = new secp256k1.Signature(r, s).addRecoveryBit(v - 27);
  return addressOfPublicKey(sig.recoverPublicKey(digest32).toRawBytes(false));
}

/** Deterministic (RFC 6979) low-s signature in the protocol's 65-byte form. */
export function signDigest(digest32: Uint8Array, privateKey: Uint8Array): Uint8Array {
  const sig = secp256k1.sign(digest32, privateKey, { lowS: true });
  return concat(bigToBytes(sig.r, 32), bigToBytes(sig.s, 32), Uint8Array.of(27 + (sig.recovery ?? 0)));
}
