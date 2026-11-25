import { keccak_256 } from "@noble/hashes/sha3";
import {
  bytesToBig,
  bytesToHex,
  concat,
  hexToBytes,
  utf8Encode,
} from "./bytes.ts";
import { quoteDigest, quoteV2Layout as q, verifyQuote } from "./quote-v2.ts";
import { orderTokenName, validTokenName } from "./order-escrow.ts";
import { recoverSigner } from "./signature.ts";
import type { CommerceOrder, CommerceQuote } from "./commerce-v2.ts";

/** How a store's token signs and is shown. `name` is in its signing domain, `symbol` on chain, `label` on screens. */
export type PaymentToken = { name: string; symbol: string; label: string; decimals: number };
export const dollarToken: PaymentToken = { name: orderTokenName, symbol: "tUSDC", label: "TestUSDC", decimals: 6 };

export type CommercePaymentProfile = {
  chain: 8283;
  contract: string;
  asset: string;
  beneficiary: string;
  merchantId: string;
  signer: string;
  keyVersion: number;
  beneficiaryVersion: number;
  maxBaseUnits: string;
  presentationKey?: string;
  /** The store's contract, as P05 names it in its payment profile. Absent means SettlementV2. */
  abiSha256?: string;
  /** The store's payment token. Present only with `namedTokenAbiSha256`; absent means the dollar token. */
  token?: PaymentToken;
  /** Where the app reads this chain unless its owner chose another server. The app checks the genesis block before use. */
  rpc?: string;
};
// packages/contracts-abi/abi/manifest.json; test/order-escrow.test.ts keeps these in sync.
export const settlementAbiSha256 = "06f5bad7c462de2835bb3054583d7d962b179424bd67e411360313d47da02814";
export const orderEscrowAbiSha256 = "e326762a55880c673cec09249bab8a76725183e812f3b3709dec2b92bd936c29";

/**
 * The same order contract at a store that names its own token. Apps from before this field would show such a store's
 * prices as dollars, so it has a value they refuse: the SHA-256 of "NU54 named payment token\n" and the contract's hash.
 */
export const namedTokenAbiSha256 = "6627cf1b4142565d31cb1fc786a43c3aca301a107eb19a97eb3093e5a93bcc92";

/** A store uses one contract; the signed quote alone cannot tell the two apart. */
export function paysThroughEscrow(profile: Pick<CommercePaymentProfile, "abiSha256">): boolean {
  if (profile.abiSha256 === undefined || profile.abiSha256 === settlementAbiSha256) return false;
  if (profile.abiSha256 === orderEscrowAbiSha256 || profile.abiSha256 === namedTokenAbiSha256) return true;
  throw new Error("PAYMENT_PROFILE");
}
export type VerifiedCommercePayment = {
  bytes: Uint8Array;
  payer: string;
  payerRole?: "owner" | "renter";
  generation: bigint;
  digest: string;
  approvalDigest: string;
  deadline: number;
};
const hash = (value: string) => keccak_256(utf8Encode(value));

const tokenWord = /^[A-Za-z][A-Za-z0-9]{0,15}$/;
export function validPaymentToken(value: unknown): value is PaymentToken {
  const token = value as PaymentToken;
  return !!token && typeof token === "object" && Object.keys(token).sort().join() === "decimals,label,name,symbol" &&
    validTokenName(token.name) && tokenWord.test(token.symbol) && tokenWord.test(token.label) &&
    Number.isInteger(token.decimals) && token.decimals >= 1 && token.decimals <= 18;
}

/** The token a store's orders are paid in. */
export const paymentToken = (profile?: Pick<CommercePaymentProfile, "token">): PaymentToken => profile?.token ?? dollarToken;

/** Dollars and cents of an amount. The rest is cut off, never rounded, so no screen shows more than there is (N31). */
export function dollarText(baseUnits: string | bigint, decimals: number): string {
  const units = BigInt(baseUnits);
  const absolute = units < 0n ? -units : units;
  const scale = 10n ** BigInt(decimals);
  const cents = (absolute * 100n) / scale; // integer division drops the rest
  const sign = units < 0n && cents ? "-" : "";
  return `${sign}${cents / 100n}.${(cents % 100n).toString().padStart(2, "0")}`;
}

/**
 * An amount as the screens show it. Won is exact: "12,500원" in Korean and "₩12,500" in English.
 * Any other store token is a dollar token and keeps two decimals.
 */
export function tokenAmount(baseUnits: string | bigint, token: PaymentToken, language: "ko" | "en" = "ko"): string {
  if (token.symbol !== wonSymbol) return `${dollarText(baseUnits, token.decimals)} ${token.label}`;
  const units = BigInt(baseUnits);
  const absolute = units < 0n ? -units : units;
  const scale = 10n ** BigInt(token.decimals);
  const fraction = (absolute % scale).toString().padStart(token.decimals, "0").replace(/0+$/, "");
  const sign = units < 0n ? "-" : "";
  const amount = (absolute / scale).toString().replace(/\B(?=(\d{3})+$)/g, ",") + (fraction ? `.${fraction.padEnd(2, "0")}` : "");
  return language === "ko" ? `${sign}${amount}원` : `${sign}₩${amount}`;
}
const wonSymbol = "tKRW";

/** An HTTPS address without credentials, query or fragment, as the app accepts one from its owner. */
export function readServer(value: unknown): value is string {
  return typeof value === "string" && value.length <= 512 &&
    /^https:\/\/[A-Za-z0-9.[\]:_-]+(?:\/[A-Za-z0-9/_.-]*)?$/.test(value);
}

export function paymentProfile(
  raw: string
): CommercePaymentProfile | undefined {
  const { payment } = JSON.parse(raw) as { payment?: CommercePaymentProfile };
  if (!payment) return undefined;
  const address = (v: string) =>
    /^0x[0-9a-f]{40}$/.test(v) && !/^0x0+$/.test(v);
  if (
    payment.chain !== 8283 ||
    ![
      payment.contract,
      payment.asset,
      payment.beneficiary,
      payment.signer,
    ].every(address) ||
    !/^[a-f0-9]{64}$/.test(payment.merchantId) ||
    !Number.isSafeInteger(payment.keyVersion) ||
    payment.keyVersion < 1 ||
    !Number.isSafeInteger(payment.beneficiaryVersion) ||
    payment.beneficiaryVersion < 1 ||
    !/^[1-9][0-9]{0,7}$/.test(payment.maxBaseUnits) ||
    BigInt(payment.maxBaseUnits) > 30_000_000n
  )
    throw new Error("PAYMENT_PROFILE");
  if (payment.rpc !== undefined && !readServer(payment.rpc)) throw new Error("PAYMENT_PROFILE");
  paysThroughEscrow(payment); // refuses a contract this version does not know
  if ((payment.token !== undefined) !== (payment.abiSha256 === namedTokenAbiSha256)) throw new Error("PAYMENT_PROFILE");
  if (payment.token !== undefined && !validPaymentToken(payment.token)) throw new Error("PAYMENT_PROFILE");
  return payment;
}

/** Checks the recorded server quote against a separately enrolled deployment and the private order display. */
export function prepareCommercePayment(
  quote: CommerceQuote,
  order: CommerceOrder,
  profile: CommercePaymentProfile,
  now: number,
  branch: string
): VerifiedCommercePayment {
  const bytes = hexToBytes(quote.signedQuote);
  const raw = bytes.subarray(0, q.unsignedSize);
  const word = (offset: number) => raw.subarray(offset, offset + 32);
  const address = (offset: number) => bytesToHex(word(offset).subarray(12));
  const value = (offset: number) => bytesToBig(word(offset));
  const payer = address(q.payerOffset);
  const generation = value(q.payerGenerationOffset);
  const digest = quoteDigest(raw);
  const amount = value(q.amountOffset);
  const expires = value(q.expiresAtOffset);
  if (
    order.state !== "issued" ||
    quote.saleRef !== order.id ||
    quote.branchRef !== branch ||
    quote.quoteDigest !== order.quoteDigest ||
    quote.orderKey !== order.orderKey ||
    quote.orderKey !== bytesToHex(word(q.orderIdOffset), false)
  )
    throw new Error("PAYMENT_ORDER_STATE");
  if (
    verifyQuote(bytes, {
      expected: raw,
      merchantSigner: hexToBytes(profile.signer),
      now: BigInt(now),
      consumed: false,
    }) !== q.statusOk ||
    bytesToHex(digest, false) !== quote.quoteDigest ||
    expires !== BigInt(quote.expiresAt) ||
    value(q.chainIdOffset) !== BigInt(profile.chain) ||
    address(q.verifyingContractOffset) !== profile.contract ||
    address(q.assetOffset) !== profile.asset ||
    profile.asset !== order.asset ||
    address(q.beneficiaryOffset) !== profile.beneficiary ||
    bytesToHex(word(q.merchantIdOffset), false) !== profile.merchantId ||
    value(q.merchantKeyVersionOffset) !== BigInt(profile.keyVersion) ||
    value(q.beneficiaryVersionOffset) !== BigInt(profile.beneficiaryVersion) ||
    generation < 1n ||
    amount !== BigInt(order.amount) ||
    amount > BigInt(profile.maxBaseUnits)
  )
    throw new Error("PAYMENT_QUOTE_MISMATCH");
  const domain = keccak_256(
    concat(
      hash(q.domainType),
      hash(q.domainName),
      hash(q.domainVersion),
      raw.subarray(0, 64)
    )
  );
  const message = keccak_256(
    concat(hash("PaymentApproval(bytes32 quoteDigest)"), digest)
  );
  const approvalDigest = bytesToHex(
    keccak_256(concat(Uint8Array.of(25, 1), domain, message)),
    false
  );
  return {
    bytes,
    payer,
    payerRole: raw[q.payerRoleOffset + 31] === 1 ? "owner" : "renter",
    generation,
    digest: quote.quoteDigest,
    approvalDigest,
    deadline: quote.expiresAt,
  };
}

export function verifyCommerceApproval(
  payment: VerifiedCommercePayment,
  signature: Uint8Array,
  now: number
): string {
  if (
    now >= payment.deadline ||
    signature.length !== 65 ||
    ![27, 28].includes(signature[64]) ||
    recoverSigner(hexToBytes(payment.approvalDigest), signature) !==
      payment.payer
  )
    throw new Error("PAYMENT_APPROVAL_MISMATCH");
  return bytesToHex(signature, false);
}

/** An amount of the store's enrolled asset, or raw units of any other. */
export function commerceAmount(
  baseUnits: string,
  asset: string,
  profile?: CommercePaymentProfile,
  language: "ko" | "en" = "ko"
): string {
  if (!profile || asset !== profile.asset || !/^(0|[1-9][0-9]*)$/.test(baseUnits)) {
    return `${baseUnits} 기준 단위`;
  }
  return tokenAmount(baseUnits, paymentToken(profile), language);
}
