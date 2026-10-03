// The settlement contract calls the kiosk makes, built from the shared ABI package so the
// selectors and event topics cannot drift from the contract.

import { keccak_256 } from "@noble/hashes/sha3";
import { paymentSettlementAbi, paymentSettlementExtensionsAbi } from "@nu54/contracts-abi";
import { bigToBytes, bytesToHex, concat, hexToBytes, utf8Encode } from "@nu54/protocol";
import type { Hex, Log } from "./rpc.ts";

type AbiParam = { type: string; components?: readonly AbiParam[] };
const canonical = (p: AbiParam): string =>
  p.type.startsWith("tuple") ? `(${(p.components ?? []).map(canonical).join(",")})${p.type.slice(5)}` : p.type;
const signature = (name: string, inputs: readonly AbiParam[]) => `${name}(${inputs.map(canonical).join(",")})`;
const hash4 = (s: string) => bytesToHex(keccak_256(utf8Encode(s)).subarray(0, 4));

const settleAbi = paymentSettlementAbi.find((x) => x.type === "function" && x.name === "settle")!;
const setLimitsAbi = paymentSettlementExtensionsAbi.find((x) => x.type === "function" && x.name === "setLimits")!;
const limitsChangedAbi = paymentSettlementExtensionsAbi.find((x) => x.type === "event" && x.name === "LimitsChanged")!;
export const SETTLE_SELECTOR = hash4(signature("settle", settleAbi.inputs as readonly AbiParam[]));
const settledAbi = paymentSettlementAbi.find((x) => x.type === "event" && x.name === "PaymentSettled")!;
export const PAYMENT_SETTLED_TOPIC = bytesToHex(
  keccak_256(utf8Encode(signature("PaymentSettled", settledAbi.inputs as readonly AbiParam[]))),
) as Hex;

export const SET_LIMITS_SELECTOR = hash4(signature("setLimits", setLimitsAbi.inputs as readonly AbiParam[]));
export const LIMITS_CHANGED_TOPIC = bytesToHex(
  keccak_256(utf8Encode(signature("LimitsChanged", limitsChangedAbi.inputs as readonly AbiParam[]))),
) as Hex;

/** custom error selector -> name, from both settlement interfaces. */
export const ERRORS: Record<string, string> = Object.fromEntries(
  [...paymentSettlementAbi, ...paymentSettlementExtensionsAbi]
    .filter((x) => x.type === "error")
    .map((x) => [hash4(signature(x.name, x.inputs as readonly AbiParam[])), x.name]),
);

export interface Authorization {
  chainId: string;
  contract: string;
  merchant: string;
  payout: string;
  token: string;
  amount: string;
  orderId: string;
  nonce: string;
  expiry: string;
}

/** A device-signed LimitChange (payment-protocol.md 2): limits in base units, 0 = the cap. */
export interface LimitChange {
  chainId: string;
  contract: string;
  perPaymentLimit: string;
  dailyLimit: string;
  nonce: string;
  expiry: string;
}

const word = (v: bigint) => bigToBytes(v, 32);
const addr = (a: string) => concat(new Uint8Array(12), hexToBytes(a));

/** ABI-encoded settle(auth, sig): the tuple is static (9 words) and the signature is dynamic. */
export function encodeSettle(a: Authorization, signature: string): Hex {
  const sig = hexToBytes(signature);
  const padded = concat(sig, new Uint8Array((32 - (sig.length % 32)) % 32));
  return bytesToHex(
    concat(
      hexToBytes(SETTLE_SELECTOR),
      word(BigInt(a.chainId)), addr(a.contract), addr(a.merchant), addr(a.payout), addr(a.token),
      word(BigInt(a.amount)), hexToBytes(a.orderId), word(BigInt(a.nonce)), word(BigInt(a.expiry)),
      word(32n * 10n), // offset of the signature after the 9 tuple words and this offset word
      word(BigInt(sig.length)),
      padded,
    ),
  ) as Hex;
}

/** ABI-encoded setLimits(change, sig): a static 6-word tuple and a dynamic signature. */
export function encodeSetLimits(c: LimitChange, sigHex: string): Hex {
  const sig = hexToBytes(sigHex);
  const padded = concat(sig, new Uint8Array((32 - (sig.length % 32)) % 32));
  return bytesToHex(
    concat(
      hexToBytes(SET_LIMITS_SELECTOR),
      word(BigInt(c.chainId)), addr(c.contract), word(BigInt(c.perPaymentLimit)), word(BigInt(c.dailyLimit)),
      word(BigInt(c.nonce)), word(BigInt(c.expiry)),
      word(32n * 7n), // offset of the signature after the 6 tuple words and this offset word
      word(BigInt(sig.length)),
      padded,
    ),
  ) as Hex;
}

/** Name of the custom error in revert data, or undefined. */
export function decodeError(data: Hex | undefined): string | undefined {
  return data && data.length >= 10 ? ERRORS[data.slice(0, 10).toLowerCase()] : undefined;
}

export interface Settled {
  merchant: string;
  orderId: string;
  device: string;
  amount: bigint;
  nonce: bigint;
  txHash: Hex;
  block: bigint;
}

export function decodeSettled(log: Log): Settled {
  const data = hexToBytes(log.data);
  return {
    merchant: "0x" + log.topics[1].slice(26),
    orderId: log.topics[2],
    device: "0x" + log.topics[3].slice(26),
    amount: BigInt(bytesToHex(data.subarray(0, 32))),
    nonce: BigInt(bytesToHex(data.subarray(32, 64))),
    txHash: log.transactionHash,
    block: BigInt(log.blockNumber),
  };
}

export const topicOfAddress = (a: string) => ("0x" + "0".repeat(24) + a.slice(2).toLowerCase()) as Hex;
