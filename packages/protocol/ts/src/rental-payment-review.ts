import { p256 } from "@noble/curves/p256";
import { sha256 } from "@noble/hashes/sha2";
import { bytesToBig, bytesToHex, concat, equalBytes, hexToBytes, utf8Encode } from "./bytes.ts";
import { verifyCommerceApproval, type VerifiedCommercePayment } from "./commerce-payment.ts";
import { quoteV2Layout as quote } from "./quote-v2.ts";

const prepareMagic = utf8Encode("W10Q");
const receiptMagic = utf8Encode("W10R");
const domain = utf8Encode("NU54 W10 rental payment review v1\0");
const bodySize = 232;
export const rentalReviewPrepareSize = 36 + quote.unsignedSize + quote.signatureSize;
export const rentalReviewReceiptSize = bodySize + 64;

export type RentalReviewExpectation = {
  /** Pins resolved independently of the terminal's receipt. */
  boardPin: string;
  terminalPin: string;
  /** Canonical uncompressed P-256 point of the protected installation ID. */
  signer: string;
  request: string;
  challenge: string;
  now: number;
};

export function rentalReviewPrepare(challenge: Uint8Array, signedQuote: Uint8Array): Uint8Array {
  if (challenge.length !== 32 || !challenge.some(Boolean) || signedQuote.length !== rentalReviewPrepareSize - 36)
    throw new Error("RENTAL_REVIEW_SCOPE");
  return concat(prepareMagic, challenge, signedQuote);
}

export function isRentalReviewPrepare(body: Uint8Array): boolean {
  return body.length === rentalReviewPrepareSize && equalBytes(body.subarray(0, 4), prepareMagic) &&
    body.subarray(4, 36).some(Boolean);
}

function exactHex(value: string, size: number, prefixed = false): Uint8Array {
  const digits = prefixed ? value.slice(2) : value;
  if (prefixed && !value.startsWith("0x") || digits.length !== size * 2 ||
      !/^[0-9a-f]+$/.test(digits) || /^0+$/.test(digits))
    throw new Error("RENTAL_REVIEW_SCOPE");
  return hexToBytes(digits);
}

/** Verify only against an independently authorized original payment and current handover. */
export function verifyRentalPaymentReview(
  receipt: Uint8Array,
  payment: VerifiedCommercePayment,
  expected: RentalReviewExpectation,
): { epoch: Uint8Array; expires: number } {
  const epoch = receipt.slice(68, 84);
  if (receipt.length !== rentalReviewReceiptSize || !equalBytes(receipt.subarray(0, 4), receiptMagic) ||
      !epoch.some(Boolean) || payment.payerRole !== "renter" || payment.generation <= 0n ||
      payment.bytes.length !== rentalReviewPrepareSize - 36 || payment.bytes[quote.modeOffset + 31] !== 1 ||
      !Number.isSafeInteger(expected.now) || expected.now <= 0 || expected.now >= payment.deadline ||
      !Number.isSafeInteger(payment.deadline) || payment.deadline - expected.now > 120 ||
      !equalBytes(receipt.subarray(4, 36), exactHex(expected.boardPin, 32)) ||
      !equalBytes(receipt.subarray(36, 68), exactHex(expected.terminalPin, 32)) ||
      !equalBytes(receipt.subarray(84, 100), exactHex(expected.request, 16)) ||
      !equalBytes(receipt.subarray(100, 132), exactHex(expected.challenge, 32)) ||
      !equalBytes(receipt.subarray(132, 164), payment.bytes.subarray(quote.orderIdOffset, quote.orderIdOffset + 32)) ||
      !equalBytes(receipt.subarray(164, 196), sha256(payment.bytes)) ||
      !equalBytes(receipt.subarray(196, 216), exactHex(payment.payer, 20, true)) ||
      bytesToBig(receipt.subarray(216, 224)) !== payment.generation ||
      bytesToBig(receipt.subarray(224, 232)) !== BigInt(payment.deadline))
    throw new Error("RENTAL_REVIEW_SCOPE");
  const signer = exactHex(expected.signer, 65);
  const spki = concat(hexToBytes("3059301306072a8648ce3d020106082a8648ce3d030107034200"), signer);
  if (signer[0] !== 4 || bytesToHex(sha256(spki), false) !== expected.boardPin ||
      !p256.verify(receipt.subarray(bodySize), sha256(concat(domain, receipt.subarray(0, bodySize))), signer,
        { lowS: true, prehash: false, format: "compact" }))
    throw new Error("RENTAL_REVIEW_SIGNATURE");
  return {epoch, expires: payment.deadline};
}


const decisionDomain = utf8Encode("NU54 W10 rental payment decision v1\0");
const decisionMagic = utf8Encode("W10D");
export const rentalDecisionSize = 166;
export type RentalPaymentDecision = {decision:"approved";signature:string} | {decision:"refused"};

export function verifyRentalPaymentDecision(
  bytes:Uint8Array,
  review:Uint8Array,
  payment:VerifiedCommercePayment,
  expected:RentalReviewExpectation,
): RentalPaymentDecision {
  verifyRentalPaymentReview(review,payment,expected);
  const signature=bytes.subarray(37,102);
  const status=bytes[36];
  if(bytes.length!==rentalDecisionSize || !equalBytes(bytes.subarray(0,4),decisionMagic) ||
      !equalBytes(bytes.subarray(4,36),sha256(review)) ||
      (status!==1 && status!==2) || (status===2 && signature.some(Boolean)) ||
      !p256.verify(bytes.subarray(102),sha256(concat(decisionDomain,bytes.subarray(0,102))),
        exactHex(expected.signer,65),{lowS:true,prehash:false,format:"compact"}))
    throw new Error("RENTAL_DECISION_EVIDENCE");
  if (status===2) return {decision:"refused"};
  return {decision:"approved",signature:verifyCommerceApproval(payment,signature,expected.now)};
}
