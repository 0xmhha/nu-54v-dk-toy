// The pickup QR of a ready order (traveller-services.md chapter 9 item 8). It carries only the one-time token the
// customer entrance issued after the paying board's account proof: no order, amount or account. Staff scan it on
// that order's page of the operations web, and the server checks the store, the order and the token.

const form = /^nu54:\/\/pickup\/([0-9a-f]{64})$/;
const zero = "0".repeat(64);

/** The 64 lowercase hex characters of the token, or PICKUP_QR_INVALID for any other text, spaces included. */
export function parsePickupQr(text: string): string {
  const match = typeof text === "string" && text.length === 78 ? form.exec(text) : null;
  if (!match || match[1] === zero) throw new Error("PICKUP_QR_INVALID");
  return match[1];
}

/** `nu54://pickup/<token>`; refuses a token the parser would refuse. */
export function encodePickupQr(token: string): string {
  const text = `nu54://pickup/${token}`;
  parsePickupQr(text);
  return text;
}
