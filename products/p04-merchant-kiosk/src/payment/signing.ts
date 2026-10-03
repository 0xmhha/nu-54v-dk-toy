// Kiosk-side signing and checks for one payment (payment-protocol.md 1, 2 and 7).
//
// The kiosk holds the merchant signing key and signs the MerchantOrder; the device returns
// a PaymentAuthorization signature, which the kiosk checks before it submits settle.

import {
  digest,
  hexToBytes,
  recoverSigner,
  signDigest,
  bytesToHex,
  type Domain,
  type MerchantOrder,
  type PaymentAuthorization,
} from "@nu54/protocol";

type Fields<T> = { [K in keyof T]: string | bigint | number };

/** Signs the order with the merchant key (deterministic, low-s, v = 27/28). */
export function signMerchantOrder(domain: Domain, order: Fields<MerchantOrder>, merchantKey: Uint8Array): string {
  return bytesToHex(signDigest(digest(domain, "MerchantOrder", order), merchantKey));
}

/** Signs the session's one-time key with the merchant key (EIP-712 KioskKey, payment-protocol.md 4.1). */
export function signKioskKey(domain: Domain, value: { merchant: string; kioskEphemeral: string; kioskNonce: string }, merchantKey: Uint8Array): string {
  return bytesToHex(signDigest(digest(domain, "KioskKey", value), merchantKey));
}

export type AuthorizationCheck = { ok: true; device: string } | { ok: false; reason: "MERCHANT_FORGED" | "BAD_SIGNATURE" };

/**
 * Checks a device-signed PaymentAuthorization before submission: the signature must be in
 * the accepted form and recover to the device the session opened with, and the signed
 * fields must be the ones the kiosk sent (the device only adds the nonce).
 */
export function checkDeviceAuthorization(
  domain: Domain,
  sent: Omit<Fields<PaymentAuthorization>, "nonce">,
  returned: { signature: string; nonce: string | bigint },
  expectedDevice: string,
): AuthorizationCheck {
  const auth = { ...sent, nonce: BigInt(returned.nonce) };
  let signer: string;
  try {
    signer = recoverSigner(digest(domain, "PaymentAuthorization", auth), hexToBytes(returned.signature));
  } catch {
    return { ok: false, reason: "BAD_SIGNATURE" };
  }
  return signer === expectedDevice.toLowerCase() ? { ok: true, device: signer } : { ok: false, reason: "MERCHANT_FORGED" };
}

/** Device-signed limit change: recovers the signer so the kiosk can match it to the session. */
export function limitChangeSigner(domain: Domain, change: Record<string, string | bigint | number>, signature: string): string {
  return recoverSigner(digest(domain, "LimitChange", change), hexToBytes(signature));
}
