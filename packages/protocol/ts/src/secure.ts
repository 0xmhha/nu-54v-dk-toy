// The payment session's secure channel (payment-protocol.md 4.1): one-time secp256k1 keys on
// both sides, an ECDH shared x coordinate, HKDF-SHA256 to a 128-bit session key, and AES-GCM
// over every body after session.open.ok. Public keys travel as their x coordinate only; the
// receiver lifts the point with an even y, which gives the same shared x either way.

import { gcm } from "@noble/ciphers/aes";
import { secp256k1 } from "@noble/curves/secp256k1";
import { hkdf } from "@noble/hashes/hkdf";
import { sha256 } from "@noble/hashes/sha2";
import { bytesToBig, concat, ProtocolError, utf8Encode } from "./bytes.ts";

export const SESSION_KEY_INFO = "nu54 session v1";
export const SESSION_KEY_LEN = 16;
export const GCM_TAG_LEN = 16;

/** Who sends a body: the first IV byte (payment-protocol.md 4.1, step 4). */
export type ChannelSide = "kiosk" | "device";
const DIRECTION: Record<ChannelSide, number> = { kiosk: 0x01, device: 0x02 };

export interface EphemeralKey {
  privateKey: Uint8Array;
  /** x coordinate of the public key, the value sent as kioskEphemeral or deviceEphemeral. */
  x: Uint8Array;
}

/**
 * A one-time key from a random source. A 32-byte draw that is not a valid scalar (zero or not
 * below the group order) is drawn again, so implementations that share a random source agree.
 */
export function ephemeralKey(random: (n: number) => Uint8Array): EphemeralKey {
  for (;;) {
    const k = random(32);
    const n = bytesToBig(k);
    if (n > 0n && n < secp256k1.CURVE.n) return { privateKey: k, x: secp256k1.getPublicKey(k, true).subarray(1) };
  }
}

/** True when `x` is the x coordinate of a curve point. */
export function isPointX(x: Uint8Array): boolean {
  if (x.length !== 32) return false;
  try {
    secp256k1.ProjectivePoint.fromHex(concat(Uint8Array.of(0x02), x)).assertValidity();
    return true;
  } catch {
    return false;
  }
}

/**
 * The session key: HKDF-SHA256 over the ECDH shared x coordinate, salt kioskNonce || deviceNonce,
 * info "nu54 session v1", 16 bytes. Throws when the peer's x is not on the curve.
 */
export function sessionKey(privateKey: Uint8Array, peerX: Uint8Array, kioskNonce: Uint8Array, deviceNonce: Uint8Array): Uint8Array {
  if (!isPointX(peerX)) throw new Error("the peer's one-time key is not a curve point");
  const shared = secp256k1.getSharedSecret(privateKey, concat(Uint8Array.of(0x02), peerX), true).subarray(1);
  return hkdf(sha256, shared, concat(kioskNonce, deviceNonce), utf8Encode(SESSION_KEY_INFO), SESSION_KEY_LEN);
}

/** IV: the sender's direction byte, then its message number as 11 bytes big-endian. */
function iv(side: ChannelSide, counter: bigint): Uint8Array {
  const out = new Uint8Array(12);
  out[0] = DIRECTION[side];
  for (let i = 11, c = counter; i >= 1; i--, c >>= 8n) out[i] = Number(c & 0xffn);
  return out;
}

/** One end of the channel: seals what it sends and opens what the other end sent. */
export class SecureChannel {
  private readonly key: Uint8Array;
  private readonly self: ChannelSide;
  private readonly peer: ChannelSide;
  private sent = 0n;
  private received = 0n;

  constructor(key: Uint8Array, self: ChannelSide) {
    if (key.length !== SESSION_KEY_LEN) throw new Error("session key must be 16 bytes");
    this.key = key;
    this.self = self;
    this.peer = self === "kiosk" ? "device" : "kiosk";
  }

  /** CBOR body -> ciphertext || tag. */
  seal(body: Uint8Array): Uint8Array {
    return gcm(this.key, iv(this.self, this.sent++)).encrypt(body);
  }

  /** ciphertext || tag -> CBOR body; a failed tag is BAD_FRAME (the session closes). */
  open(sealed: Uint8Array): Uint8Array {
    try {
      const body = gcm(this.key, iv(this.peer, this.received)).decrypt(sealed);
      this.received++;
      return body;
    } catch {
      throw new ProtocolError("BAD_FRAME", "secure channel tag check failed");
    }
  }
}
