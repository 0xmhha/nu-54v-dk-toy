// What the confirmation screen shows (P02 design 2, P02-FR-03, FR-04).
//
// Every string comes from the confirm.show the device sent and nothing else: the screen never
// mixes in values from the kiosk or from input. Addresses are EIP-55 checksummed; the amount is
// in token units truncated (never rounded) to two decimals with integer division, so the screen
// never shows more than the device signs [N31]. A token missing from the table is shown as its
// base-unit integer and address.

import { keccak_256 } from "@noble/hashes/sha3";
import { bytesToHex, utf8Encode } from "@nu54/protocol";

export interface TokenInfo {
  symbol: string;
  decimals: number;
}

/** Tokens the app knows, by lower-case address. This cycle has the test USDC only. */
export const TOKENS: Record<string, TokenInfo> = {
  "0x500ef69da230e42bff34487b578ea0cbefbda2ba": { symbol: "tUSDC", decimals: 6 }, // deployments/8283.json
};

/** EIP-55 mixed-case checksum of a 20-byte hex address. */
export function checksumAddress(address: string): string {
  const hex = address.toLowerCase().replace(/^0x/, "");
  if (!/^[0-9a-f]{40}$/.test(hex)) throw new Error(`not an address: ${address}`);
  const hash = bytesToHex(keccak_256(utf8Encode(hex)), false);
  let out = "0x";
  for (let i = 0; i < 40; i++) out += parseInt(hash[i], 16) >= 8 ? hex[i].toUpperCase() : hex[i];
  return out;
}

/** Base units -> "5.00" for a token with `decimals` places: truncated, integer arithmetic only. */
export function truncatedAmount(baseUnits: bigint, decimals: number): string {
  if (decimals < 2) return baseUnits.toString();
  const cents = baseUnits / 10n ** BigInt(decimals - 2); // drops the third decimal and below
  return `${cents / 100n}.${(cents % 100n).toString().padStart(2, "0")}`;
}

export interface ConfirmView {
  merchantName: string;
  /** "4.50 tUSDC", or the base-unit integer for an unknown token. */
  amount: string;
  /** The token symbol, or the checksummed token address when the table does not know it. */
  token: string;
  known: boolean;
  /** Full checksummed payout: the renter compares it (P02-NFR-02). */
  payout: string;
  /** Order id shortened for the screen [N16]. */
  orderShort: string;
}

export interface LimitView {
  /** "20.00 tUSDC", or "상한 그대로" for 0 (payment-protocol.md 2: 0 means the cap). */
  perPayment: string;
  daily: string;
  /** Expiry as unix seconds; the device refuses it later than authorizationExpiry. */
  expiry: number;
}

/** Builds the limit change screen from confirm.limit; limits are in the test token's units. */
export function limitView(show: Record<string, unknown>, token: TokenInfo = Object.values(TOKENS)[0]): LimitView {
  const text = (v: unknown) => {
    const n = BigInt(String(v));
    return n === 0n ? "상한 그대로" : `${truncatedAmount(n, token.decimals)} ${token.symbol}`;
  };
  return { perPayment: text(show.perPaymentLimit), daily: text(show.dailyLimit), expiry: Number(show.expiry) };
}

/** Builds the screen strings from confirm.show fields (the decoded JSON form). */
export function confirmView(show: Record<string, unknown>, tokens: Record<string, TokenInfo> = TOKENS): ConfirmView {
  const token = String(show.token).toLowerCase();
  const info = tokens[token];
  const amount = BigInt(String(show.amount));
  const orderId = String(show.orderId);
  return {
    merchantName: String(show.merchantName),
    amount: info ? `${truncatedAmount(amount, info.decimals)} ${info.symbol}` : amount.toString(),
    token: info ? info.symbol : checksumAddress(token),
    known: !!info,
    payout: checksumAddress(String(show.payout)),
    orderShort: `${orderId.slice(0, 8)}…${orderId.slice(-4)}`,
  };
}
