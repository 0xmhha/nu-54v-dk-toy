import { ed25519 } from "@noble/curves/ed25519";
import { sha256 } from "@noble/hashes/sha256";
import {
  bigToBytes,
  bytesToHex,
  concat,
  hexToBytes,
  utf8Encode,
} from "./bytes.ts";
import {
  prepareCommercePayment,
  type CommercePaymentProfile,
} from "./commerce-payment.ts";
import type { CommerceOrder, CommerceQuote } from "./commerce-v2.ts";
import { quoteDigest, quoteV2Layout } from "./quote-v2.ts";

export type PrivatePresentation = {
  customer: string;
  origin: string;
  claim: string;
  signature: string;
  gasSponsor?: MerchantGasSponsor;
  rentalEscrow?: { expiresAt: number; signature: string };
  rentalReview?: RentalReviewAuthority;
  invoice?: { cycle: string; termsVersion: string; signature: string };
};
export type RentalReviewChallenge = { request: string; challenge: string };
export type RentalReviewAuthority = RentalReviewChallenge & {
  boardPin: string;
  terminalPin: string;
  signer: string;
  expiresAt: number;
  signature: string;
};

export function validRentalReviewChallenge(value: RentalReviewChallenge) {
  return /^[0-9a-f]{32}$/.test(value?.request) && !/^0+$/.test(value.request) &&
    /^[0-9a-f]{64}$/.test(value?.challenge) && !/^0+$/.test(value.challenge);
}

export type MerchantGasSponsor = {
  account: string;
  expiresAt: number;
  grant: string;
  signature: string;
};
export type PresentationScope = {
  branch: string;
  sale: string;
  customer: string;
  origin?: string;
  claim?: string;
};
const uuid =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const hash = /^[0-9a-f]{64}$/;

export function presentationMessage(quote: CommerceQuote, generation: bigint) {
  const proof = quote.presentation;
  if (
    !proof ||
    ![quote.branchRef, quote.saleRef, proof.customer, proof.origin].every(
      (value) => uuid.test(value)
    ) ||
    ![proof.claim, quote.orderKey, quote.quoteDigest].every((value) =>
      hash.test(value)
    )
  )
    throw new Error("PRESENTATION_SHAPE");
  return concat(
    utf8Encode("NU54-private-presentation-v1\0"),
    ...[quote.branchRef, quote.saleRef, proof.customer, proof.origin].map(
      (value) => hexToBytes(value.replace(/-/g, ""))
    ),
    ...[proof.claim, quote.orderKey, quote.quoteDigest].map(hexToBytes),
    bigToBytes(generation, 8),
    bigToBytes(BigInt(quote.expiresAt), 8)
  );
}

export function invoicePresentationMessage(quote: CommerceQuote, generation: bigint) {
  const invoice = quote.presentation?.invoice;
  if (!invoice || typeof invoice.cycle !== "string" ||
      !/^\d{4}-\d{2}-\d{2}$/.test(invoice.cycle) ||
      typeof invoice.termsVersion !== "string") {
    throw new Error("PRESENTATION_SHAPE");
  }
  const terms = utf8Encode(invoice.termsVersion);
  if (terms.length === 0 || terms.length > 160) throw new Error("PRESENTATION_SHAPE");
  return concat(
    presentationMessage(quote, generation),
    Uint8Array.of(1),
    utf8Encode(invoice.cycle),
    bigToBytes(BigInt(terms.length), 2),
    terms,
    sha256(hexToBytes(quote.signedQuote))
  );
}

export function presentationFingerprint(
  profile: CommercePaymentProfile
): string {
  if (!hash.test(profile.presentationKey ?? ""))
    throw new Error("PRESENTATION_TRUST");
  return bytesToHex(sha256(hexToBytes(profile.presentationKey!)), false);
}

/** No display or board command should consume a relay projection before this verifies. */
export function preparePresentedPayment(
  quote: CommerceQuote,
  order: CommerceOrder,
  profile: CommercePaymentProfile,
  now: number,
  expected: PresentationScope
) {
  const proof = quote.presentation;
  if (
    !proof ||
    quote.saleRef !== expected.sale ||
    order.id !== expected.sale ||
    quote.branchRef !== expected.branch ||
    proof.customer !== expected.customer ||
    (expected.origin !== undefined && proof.origin !== expected.origin) ||
    (order.subscription !== undefined
      ? proof.origin !== order.subscription.invoiceRef ||
        proof.invoice?.cycle !== order.subscription.cycle ||
        proof.invoice?.termsVersion !== order.subscription.termsVersion
      : proof.invoice !== undefined) ||
    (expected.claim !== undefined && proof.claim !== expected.claim) ||
    !/^[0-9a-f]{128}$/.test(proof.signature) ||
    !hash.test(profile.presentationKey ?? "")
  )
    throw new Error("PRESENTATION_SCOPE");
  const payment = prepareCommercePayment(
    quote,
    order,
    profile,
    now,
    expected.branch
  );
  if (
    !ed25519.verify(
      hexToBytes(proof.signature),
      presentationMessage(quote, payment.generation),
      hexToBytes(profile.presentationKey!),
      { zip215: false }
    )
  )
    throw new Error("PRESENTATION_SIGNATURE");
  if (proof.invoice && (
    !/^[0-9a-f]{128}$/.test(proof.invoice.signature) ||
    !ed25519.verify(
      hexToBytes(proof.invoice.signature),
      invoicePresentationMessage(quote, payment.generation),
      hexToBytes(profile.presentationKey!),
      { zip215: false }
    )
  )) throw new Error("PRESENTATION_SIGNATURE");
  return payment;
}

/** This assertion uses the enrolled presentation authority and leaves v2 quote bytes intact. */
export function gasSponsorMessage(quote: CommerceQuote, generation: bigint) {
  const sponsor = quote.presentation?.gasSponsor;
  if (!sponsor || !/^0x[0-9a-f]{40}$/.test(sponsor.account) || /^0x0+$/.test(sponsor.account) ||
      !hash.test(sponsor.grant) || !Number.isSafeInteger(sponsor.expiresAt) ||
      sponsor.expiresAt <= 0 || sponsor.expiresAt > quote.expiresAt)
    throw new Error("GAS_SPONSOR_SHAPE");
  return concat(
    utf8Encode("NU54-merchant-gas-sponsor-v1\0"),
    sha256(presentationMessage(quote, generation)),
    sha256(hexToBytes(quote.signedQuote)),
    hexToBytes(sponsor.account),
    hexToBytes(sponsor.grant),
    bigToBytes(BigInt(sponsor.expiresAt), 8)
  );
}

export function verifyPresentedGasSponsor(
  quote: CommerceQuote, order: CommerceOrder, profile: CommercePaymentProfile,
  now: number, scope: PresentationScope
): Omit<MerchantGasSponsor, "signature"> {
  const payment = preparePresentedPayment(quote, order, profile, now, scope);
  const sponsor = quote.presentation?.gasSponsor;
  if (!sponsor) throw new Error("GAS_SPONSOR_MISSING");
  const body = gasSponsorMessage(quote, payment.generation);
  if (sponsor.expiresAt <= now) throw new Error("GAS_SPONSOR_EXPIRED");
  if (sponsor.expiresAt - now > 30) throw new Error("GAS_SPONSOR_SHAPE");
  if (sponsor.account === payment.payer || !/^[0-9a-f]{128}$/.test(sponsor.signature) ||
      !ed25519.verify(hexToBytes(sponsor.signature), body, hexToBytes(profile.presentationKey!), {zip215:false}))
    throw new Error("GAS_SPONSOR_SIGNATURE");
  return {account:sponsor.account, expiresAt:sponsor.expiresAt, grant:sponsor.grant};
}

export function rentalEscrowMessage(quote: CommerceQuote, generation: bigint) {
  const escrow = quote.presentation?.rentalEscrow;
  if (!escrow || !Number.isSafeInteger(escrow.expiresAt) ||
      escrow.expiresAt <= 0 || escrow.expiresAt > quote.expiresAt)
    throw new Error("RENTAL_ESCROW_SHAPE");
  return concat(
    utf8Encode("NU54-rental-payment-escrow-v1\0"),
    sha256(presentationMessage(quote, generation)),
    sha256(hexToBytes(quote.signedQuote)),
    bigToBytes(BigInt(escrow.expiresAt), 8)
  );
}

export function verifyPresentedRentalEscrow(
  quote: CommerceQuote, order: CommerceOrder, profile: CommercePaymentProfile,
  now: number, scope: PresentationScope
): { expiresAt: number } {
  const payment = preparePresentedPayment(quote, order, profile, now, scope);
  const escrow = quote.presentation?.rentalEscrow;
  if (!order.cafeReview || payment.payerRole !== "renter" ||
      payment.bytes[quoteV2Layout.modeOffset + 31] !== 1 ||
      !escrow || !/^[0-9a-f]{128}$/.test(escrow.signature))
    throw new Error("RENTAL_ESCROW_MISSING");
  const signed = rentalEscrowMessage(quote, payment.generation);
  if (escrow.expiresAt <= now || escrow.expiresAt - now > 15)
    throw new Error("RENTAL_ESCROW_EXPIRED");
  if (!ed25519.verify(hexToBytes(escrow.signature), signed, hexToBytes(profile.presentationKey!), { zip215: false }))
    throw new Error("RENTAL_ESCROW_SIGNATURE");
  return { expiresAt: escrow.expiresAt };
}

export function rentalReviewAuthorityMessage(quote: CommerceQuote, generation: bigint) {
  const proof = quote.presentation?.rentalReview;
  if (!proof || !validRentalReviewChallenge(proof) || !hash.test(proof.boardPin) ||
      /^0+$/.test(proof.boardPin) || !hash.test(proof.terminalPin) || /^0+$/.test(proof.terminalPin) ||
      proof.boardPin === proof.terminalPin || !/^04[0-9a-f]{128}$/.test(proof.signer) ||
      !Number.isSafeInteger(proof.expiresAt) || proof.expiresAt <= 0)
    throw new Error("RENTAL_REVIEW_AUTHORITY_SHAPE");
  return concat(
    utf8Encode("NU54-rental-payment-review-authority-v1\0"),
    sha256(presentationMessage(quote, generation)),
    sha256(hexToBytes(quote.signedQuote)),
    hexToBytes(proof.boardPin),
    hexToBytes(proof.terminalPin),
    hexToBytes(proof.signer),
    hexToBytes(proof.request),
    hexToBytes(proof.challenge),
    bigToBytes(BigInt(proof.expiresAt), 8)
  );
}

export function verifyRentalReviewAuthority(
  quote: CommerceQuote, order: CommerceOrder, profile: CommercePaymentProfile, now: number,
  scope: PresentationScope, proof: RentalReviewAuthority, expected: RentalReviewChallenge
): RentalReviewAuthority {
  const payment = preparePresentedPayment(quote, order, profile, now, scope);
  if (payment.payerRole !== "renter" || payment.bytes[quoteV2Layout.modeOffset + 31] !== 1 ||
      !proof || !validRentalReviewChallenge(expected) || proof.request !== expected.request ||
      proof.challenge !== expected.challenge || proof.expiresAt <= now || proof.expiresAt - now > 15 ||
      proof.expiresAt > quote.expiresAt)
    throw new Error("RENTAL_REVIEW_AUTHORITY_SCOPE");
  const body = rentalReviewAuthorityMessage({...quote, presentation: {...quote.presentation!, rentalReview: proof}}, payment.generation);
  if (!/^[0-9a-f]{128}$/.test(proof.signature) ||
      !ed25519.verify(hexToBytes(proof.signature), body, hexToBytes(profile.presentationKey!), {zip215: false}))
    throw new Error("RENTAL_REVIEW_AUTHORITY_SIGNATURE");
  return proof;
}

export function verifyPresentedRentalReviewAuthority(
  quote: CommerceQuote, order: CommerceOrder, profile: CommercePaymentProfile, now: number,
  scope: PresentationScope, expected: RentalReviewChallenge
): RentalReviewAuthority {
  const escrow = verifyPresentedRentalEscrow(quote, order, profile, now, scope);
  const proof = quote.presentation?.rentalReview;
  if (!proof || proof.expiresAt > escrow.expiresAt)
    throw new Error("RENTAL_REVIEW_AUTHORITY_SCOPE");
  return verifyRentalReviewAuthority(quote, order, profile, now, scope, proof, expected);
}

/** Hex expands the quote beyond the C2 frame budget once the proof is attached. */
export function packNearbyPresentation(
  order: CommerceOrder,
  quote: CommerceQuote,
  base64: (bytes: Uint8Array) => string
) {
  return {
    id: order.id,
    revision: order.revision,
    expiresAt: order.expiresAt,
    asset: order.asset,
    amount: order.amount,
    deadline: quote.expiresAt,
    raw: base64(hexToBytes(quote.signedQuote)),
    proof: quote.presentation,
  };
}

export function unpackNearbyPresentation(
  value: unknown,
  branch: string,
  base64: (raw: string) => Uint8Array
) {
  const v = value as ReturnType<typeof packNearbyPresentation>;
  if (
    !v ||
    typeof v.raw !== "string" ||
    v.raw.length > 1000 ||
    !uuid.test(v.id)
  )
    throw new Error("PRESENTATION_SHAPE");
  const bytes = base64(v.raw);
  const orderKey = bytesToHex(
    bytes.subarray(
      quoteV2Layout.orderIdOffset,
      quoteV2Layout.orderIdOffset + 32
    ),
    false
  );
  const digest = bytesToHex(
    quoteDigest(bytes.subarray(0, quoteV2Layout.unsignedSize)),
    false
  );
  const order: CommerceOrder = {
    id: v.id,
    revision: v.revision,
    state: "issued",
    asset: v.asset,
    amount: v.amount,
    expiresAt: v.expiresAt,
    quoteDigest: digest,
    orderKey,
  };
  const quote: CommerceQuote = {
    saleRef: v.id,
    branchRef: branch,
    orderKey,
    quoteDigest: digest,
    signedQuote: bytesToHex(bytes, false),
    expiresAt: v.deadline,
    presentation: v.proof,
  };
  return { order, quote };
}
