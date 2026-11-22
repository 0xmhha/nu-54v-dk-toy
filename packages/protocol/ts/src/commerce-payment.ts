import { keccak_256 } from "@noble/hashes/sha3";
import {
  bytesToBig,
  bytesToHex,
  concat,
  hexToBytes,
  utf8Encode,
} from "./bytes.ts";
import { quoteDigest, quoteV2Layout as q, verifyQuote } from "./quote-v2.ts";
import { recoverSigner } from "./signature.ts";
import type { CommerceOrder, CommerceQuote } from "./commerce-v2.ts";

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
};
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

/** StableNet pilot displays its pinned TestUSDC asset with exact six-decimal conversion. */
export function commerceAmount(
  baseUnits: string,
  asset: string,
  profile?: CommercePaymentProfile
): string {
  if (!profile || asset !== profile.asset || !/^(0|[1-9][0-9]*)$/.test(baseUnits)) {
    return `${baseUnits} 기준 단위`;
  }
  const units = BigInt(baseUnits);
  const decimals = (units % 1_000_000n).toString().padStart(6, "0").replace(/0+$/, "").padEnd(2, "0");
  return `${units / 1_000_000n}.${decimals} TestUSDC`;
}
