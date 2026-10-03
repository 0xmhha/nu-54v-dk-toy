// Opening a payment-mode session, in plaintext or over the secure channel (payment-protocol.md 4.1).
//
// Secure: the kiosk makes a one-time key, sends its x coordinate with the merchant attestation
// and the merchant key's KioskKey signature, and turns the channel on once session.open.ok brings
// the device's one-time key. A device that answers without one is refused: the kiosk never falls
// back to plaintext once it asked for the channel.

import { bytesToHex, ephemeralKey, hexToBytes, SecureChannel, sessionKey, type Message } from "@nu54/protocol";
import type { MessageLink } from "../ble/framing.ts";

/** MerchantAttestation as opsctl issues it, with the operator's signature. */
export interface Attestation {
  merchant: string;
  payout: string;
  name: string;
  validFrom: string;
  validUntil: string;
  operatorSignature: string;
}

/** What a secure session needs from the merchant: the attestation and a KioskKey signer. */
export interface KioskKeySigner {
  attestation: Attestation;
  sign: (value: { merchant: string; kioskEphemeral: string; kioskNonce: string }) => string;
}

export type Opened = { ok: Message } | { refused: string; detail: string };

const REPLY_MS = 3_000;

export async function openPaymentSession(link: MessageLink, sessionId: string, random: (n: number) => Uint8Array, secure?: KioskKeySigner): Promise<Opened> {
  await link.endSession?.(); // session.open is always plaintext
  link.secure?.(null);
  const kioskNonce = bytesToHex(random(32));
  let fields: Record<string, unknown> = { mode: "payment", kioskNonce };
  let eph: ReturnType<typeof ephemeralKey> | undefined;
  if (secure) {
    if (!link.secure) throw new Error("this link cannot carry the secure channel");
    eph = ephemeralKey(random);
    const kioskEphemeral = bytesToHex(eph.x);
    const kioskKeySignature = secure.sign({ merchant: secure.attestation.merchant, kioskEphemeral, kioskNonce });
    fields = { ...fields, kioskEphemeral, attestation: secure.attestation, kioskKeySignature };
  }
  const ok = (await link.send({ v: 1, type: "session.open", sessionId, ...fields } as Message, 1, REPLY_MS))[0];
  if (ok?.type !== "session.open.ok") {
    eph?.privateKey.fill(0);
    return { refused: String(ok?.reason ?? "NO_REPLY"), detail: ok ? `payment session: ${ok.type}` : "payment session: no reply" };
  }
  if (eph) {
    try {
      if (!ok.deviceEphemeral) throw new Error("no deviceEphemeral");
      const key = sessionKey(eph.privateKey, hexToBytes(String(ok.deviceEphemeral)), hexToBytes(kioskNonce), hexToBytes(String(ok.deviceNonce)));
      link.secure!(new SecureChannel(key, "kiosk"));
    } catch {
      return { refused: "NOT_PERMITTED", detail: "payment session: the device did not open the secure channel" };
    } finally {
      eph.privateKey.fill(0);
    }
  }
  return { ok };
}
