import { ed25519 } from "@noble/curves/ed25519";
import { toBase64, fromBase64 } from "../src/base64.ts";
import { utf8Encode } from "../src/bytes.ts";
import {
  preparePresentedPayment,
  presentationMessage,
  invoicePresentationMessage,
  gasSponsorMessage,
  verifyPresentedGasSponsor,
  rentalEscrowMessage,
  verifyPresentedRentalEscrow,
  packNearbyPresentation,
  unpackNearbyPresentation,
} from "../src/private-presentation.ts";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { bytesToBig, bytesToHex, hexToBytes } from "../src/bytes.ts";
import {
  prepareCommercePayment,
  verifyCommerceApproval,
} from "../src/commerce-payment.ts";
import { quoteDigest, quoteV2Layout as q } from "../src/quote-v2.ts";
import { addressOfPrivateKey, signDigest } from "../src/signature.ts";
import { concat } from "../src/bytes.ts";
import { V2Journal, V2Link } from "../src/transport-v2.ts";
import { quoteOrderRevision, type CommerceOrder, type CommerceQuote } from "../src/commerce-v2.ts";

const [, signed, , unsigned, signer, clock, , digest] = readFileSync(
  new URL("../../v2/testdata/quote.vectors", import.meta.url),
  "utf8"
)
  .split("\n")[1]
  .split("|");
const raw = hexToBytes(unsigned);
const address = (offset: number) =>
  bytesToHex(raw.subarray(offset + 12, offset + 32));
const value = (offset: number) => bytesToBig(raw.subarray(offset, offset + 32));
const profile = {
  chain: 8283 as const,
  contract: address(q.verifyingContractOffset),
  asset: address(q.assetOffset),
  beneficiary: address(q.beneficiaryOffset),
  merchantId: bytesToHex(
    raw.subarray(q.merchantIdOffset, q.merchantIdOffset + 32),
    false
  ),
  signer: "0x" + signer,
  keyVersion: Number(value(q.merchantKeyVersionOffset)),
  beneficiaryVersion: Number(value(q.beneficiaryVersionOffset)),
  maxBaseUnits: "1000000",
};
const sale = "f0551c76-7592-4850-a414-54464d24dafe";
const branch = "0fbc54ef-2827-4ab5-bfcd-3a27611c39ea";
const orderKey = bytesToHex(
  raw.subarray(q.orderIdOffset, q.orderIdOffset + 32),
  false
);
const quote = {
  saleRef: sale,
  branchRef: branch,
  orderKey,
  signedQuote: signed,
  quoteDigest: digest,
  expiresAt: Number(value(q.expiresAtOffset)),
};
const order: CommerceOrder = {
  id: sale,
  state: "issued",
  revision: 1,
  orderKey,
  quoteDigest: digest,
  asset: profile.asset,
  amount: value(q.amountOffset).toString(),
  expiresAt: new Date(quote.expiresAt * 1000).toISOString(),
};

test("payment uses exact canonical quote and rejects another payout, cart, cap, expiry, or forged approval", () => {
  const payment = prepareCommercePayment(
    quote,
    order,
    profile,
    Number(clock),
    branch
  );
  assert.equal(bytesToHex(payment.bytes, false), signed);
  assert.equal(payment.generation, value(q.payerGenerationOffset));
  assert.throws(() =>
    prepareCommercePayment(
      quote,
      { ...order, amount: "1" },
      profile,
      Number(clock),
      branch
    )
  );
  assert.throws(() =>
    prepareCommercePayment(
      quote,
      order,
      { ...profile, beneficiary: "0x" + "1".repeat(40) },
      Number(clock),
      branch
    )
  );
  assert.throws(() =>
    prepareCommercePayment(quote, order, profile, quote.expiresAt, branch)
  );
  assert.throws(() =>
    verifyCommerceApproval(payment, new Uint8Array(65), Number(clock))
  );
});

test("quote retry uses the original order revision, including initial revision zero", () => {
  assert.equal(
    quoteOrderRevision({ ...order, state: "preview", revision: 0 }),
    0
  );
  assert.equal(
    quoteOrderRevision({ ...order, state: "issued", revision: 1 }),
    0
  );
  assert.throws(() =>
    quoteOrderRevision({ ...order, state: "checking", revision: 2 })
  );
});

test("another genuine same-merchant same-payer same-amount quote cannot enter this order approval", () => {
  const key = new Uint8Array(32).fill(9);
  const second = raw.slice();
  second[q.quoteIdOffset] ^= 1;
  second[q.orderIdOffset] ^= 1;
  const authenticatedProfile = { ...profile, signer: addressOfPrivateKey(key) };
  const asQuote = (unsigned: Uint8Array) => {
    const hash = quoteDigest(unsigned);
    return {
      ...quote,
      quoteDigest: bytesToHex(hash, false),
      orderKey: bytesToHex(
        unsigned.subarray(q.orderIdOffset, q.orderIdOffset + 32),
        false
      ),
      signedQuote: bytesToHex(concat(unsigned, signDigest(hash, key)), false),
    };
  };
  const firstQuote = asQuote(raw);
  const secondQuote = asQuote(second);
  const firstOrder = {
    ...order,
    quoteDigest: firstQuote.quoteDigest,
    orderKey: firstQuote.orderKey,
  };
  const secondOrder = {
    ...order,
    quoteDigest: secondQuote.quoteDigest,
    orderKey: secondQuote.orderKey,
  };
  prepareCommercePayment(
    firstQuote,
    firstOrder,
    authenticatedProfile,
    Number(clock),
    branch
  );
  prepareCommercePayment(
    secondQuote,
    secondOrder,
    authenticatedProfile,
    Number(clock),
    branch
  );
  assert.throws(
    () =>
      prepareCommercePayment(
        secondQuote,
        firstOrder,
        authenticatedProfile,
        Number(clock),
        branch
      ),
    "mixed order must reject before board write"
  );
});

test("paired valid quote and projection substitution fails against the original private checkout", () => {
  const merchant = new Uint8Array(32).fill(9);
  const presenter = new Uint8Array(32).fill(13);
  const authenticatedProfile = {
    ...profile,
    signer: addressOfPrivateKey(merchant),
    presentationKey: bytesToHex(ed25519.getPublicKey(presenter), false),
  };
  const audience = "1728d4b7-f136-4b87-89cd-26baec26804e";
  const origin = "22572997-f880-4c99-adbb-7dea0db07cfa";
  const secondSale = "0fbc54ef-2827-4ab5-8fcd-3a27611c39ea";
  const secondOrigin = "22572997-f880-4c99-bdbb-7dea0db07cfa";
  const make = (sale: string, cart: string, payload: Uint8Array) => {
    const hash = quoteDigest(payload);
    const key = bytesToHex(
      payload.subarray(q.orderIdOffset, q.orderIdOffset + 32),
      false
    );
    const evidence = {
      ...quote,
      saleRef: sale,
      orderKey: key,
      quoteDigest: bytesToHex(hash, false),
      signedQuote: bytesToHex(
        concat(payload, signDigest(hash, merchant)),
        false
      ),
      presentation: {
        customer: audience,
        origin: cart,
        claim: "b".repeat(64),
        signature: "",
      },
    };
    evidence.presentation.signature = bytesToHex(
      ed25519.sign(
        presentationMessage(evidence, value(q.payerGenerationOffset)),
        presenter
      ),
      false
    );
    const display = {
      ...order,
      id: sale,
      quoteDigest: evidence.quoteDigest,
      orderKey: key,
    };
    return { display, evidence };
  };
  const original = make(sale, origin, raw);
  const other = raw.slice();
  other[q.orderIdOffset] = (other[q.orderIdOffset] + 1) % 256;
  other[q.quoteIdOffset] = (other[q.quoteIdOffset] + 1) % 256;
  const replacement = make(secondSale, secondOrigin, other);
  const expected = { branch, sale, customer: audience, origin };
  preparePresentedPayment(
    original.evidence,
    original.display,
    authenticatedProfile,
    Number(clock),
    expected
  );
  preparePresentedPayment(
    replacement.evidence,
    replacement.display,
    authenticatedProfile,
    Number(clock),
    { ...expected, sale: secondSale, origin: secondOrigin }
  );
  assert.throws(() =>
    preparePresentedPayment(
      replacement.evidence,
      replacement.display,
      authenticatedProfile,
      Number(clock),
      expected
    )
  );
  const altered = {
    ...replacement.evidence,
    saleRef: sale,
    presentation: { ...replacement.evidence.presentation, origin },
  };
  assert.throws(() =>
    preparePresentedPayment(
      altered,
      { ...replacement.display, id: sale },
      authenticatedProfile,
      Number(clock),
      expected
    )
  );
  assert.throws(() =>
    preparePresentedPayment(
      original.evidence,
      original.display,
      authenticatedProfile,
      Number(clock),
      { ...expected, customer: secondOrigin }
    )
  );
  const invoice = { cycle: "2026-11-07", termsVersion: "약관 v2" };
  const renewal = { ...original.evidence, presentation: { ...original.evidence.presentation, invoice: { ...invoice, signature: "" } } };
  renewal.presentation.invoice.signature = bytesToHex(
    ed25519.sign(invoicePresentationMessage(renewal, value(q.payerGenerationOffset)), presenter), false
  );
  const subscription = { ...invoice, invoiceRef: origin, dueAt: "2026-11-07T09:00:00Z", stage: "issued" as const };
  const renewalOrder = { ...original.display, subscription };
  preparePresentedPayment(renewal, renewalOrder, authenticatedProfile, Number(clock), expected);
  assert.throws(() =>
    preparePresentedPayment(original.evidence, renewalOrder, authenticatedProfile, Number(clock), expected),
    /PRESENTATION_SCOPE/
  );
  for (const mutated of [{ ...invoice, cycle: "2026-11-14" }, { ...invoice, termsVersion: "v3" }]) {
    const changedQuote = { ...renewal, presentation: { ...renewal.presentation, invoice: { ...mutated, signature: renewal.presentation.invoice.signature } } };
    const changedOrder = { ...renewalOrder, subscription: { ...subscription, ...mutated } };
    assert.throws(() =>
      preparePresentedPayment(changedQuote, changedOrder, authenticatedProfile, Number(clock), expected),
      /PRESENTATION_SIGNATURE/
    );
  }
  const near = packNearbyPresentation(
    original.display,
    original.evidence,
    toBase64
  );
  assert.ok(
    utf8Encode(JSON.stringify({ status: "ok", value: near })).length < 2023
  );
  const rebuilt = unpackNearbyPresentation(near, branch, fromBase64);
  preparePresentedPayment(
    rebuilt.quote,
    rebuilt.order,
    authenticatedProfile,
    Number(clock),
    expected
  );
});


test("merchant gas proof is optional on old quotes and bound to the signed offer, terms and audience", () => {
  const presenter = new Uint8Array(32).fill(13);
  const authenticatedProfile = {...profile, presentationKey:bytesToHex(ed25519.getPublicKey(presenter),false)};
  const customer = "1728d4b7-f136-4b87-89cd-26baec26804e";
  const origin = "22572997-f880-4c99-adbb-7dea0db07cfa";
  const scope = {branch, sale, customer, origin};
  const evidence: CommerceQuote = {...quote, presentation:{customer, origin, claim:"b".repeat(64), signature:""}};
  const generation = value(q.payerGenerationOffset);
  evidence.presentation!.signature = bytesToHex(ed25519.sign(presentationMessage(evidence,generation),presenter),false);
  assert(preparePresentedPayment(evidence, order, authenticatedProfile, Number(clock), scope));
  assert.throws(() => verifyPresentedGasSponsor(evidence,order,authenticatedProfile,Number(clock),scope), /GAS_SPONSOR_MISSING/);
  const sponsor = {account:"0x"+"9".repeat(40), expiresAt:Number(clock)+1, grant:"a".repeat(64), signature:""};
  evidence.presentation!.gasSponsor = sponsor;
  sponsor.signature = bytesToHex(ed25519.sign(gasSponsorMessage(evidence,generation),presenter),false);
  assert.deepEqual(verifyPresentedGasSponsor(evidence,order,authenticatedProfile,Number(clock),scope),
    {account:sponsor.account, expiresAt:sponsor.expiresAt, grant:sponsor.grant});
  const packed = packNearbyPresentation(order,evidence,toBase64);
  assert.deepEqual(unpackNearbyPresentation(packed,branch,fromBase64).quote,evidence);
  const epoch = new Uint8Array(16).fill(1), peer = new Uint8Array(32).fill(1), id = new Uint8Array(16).fill(2);
  const customerLink = new V2Link("nearby","customer",epoch,peer,new V2Journal(2));
  const terminalLink = new V2Link("nearby","terminal",epoch,peer,new V2Journal(2));
  const expires = BigInt(Number(clock) + 30);
  for (const frame of customerLink.send({command:"nearby.submit",id,expires,payload:utf8Encode("checkout")}))
    terminalLink.receive(frame,BigInt(clock));
  const frames = terminalLink.send({command:"reply",id,expires,
    payload:utf8Encode(JSON.stringify({status:"ok",value:packed}))});
  const framed = frames.map(frame => customerLink.receive(frame,BigInt(clock))).at(-1);
  assert(framed && frames.length > 1);
  assert.throws(() => verifyPresentedGasSponsor(evidence,order,authenticatedProfile,sponsor.expiresAt,scope), /GAS_SPONSOR_EXPIRED/);
  const wrong = {...evidence,presentation:{...evidence.presentation!,gasSponsor:{...sponsor,account:"0x"+"8".repeat(40)}}};
  assert.throws(() => verifyPresentedGasSponsor(wrong,order,authenticatedProfile,Number(clock),scope), /GAS_SPONSOR_SIGNATURE/);
  assert.throws(() => verifyPresentedGasSponsor(evidence,order,authenticatedProfile,Number(clock),{...scope,customer:sale}), /PRESENTATION_SCOPE/);
});

test("customer cafe rental escrow proof uses the signed renter quote and private scope", () => {
  const at = Number(clock);
  const seed = new Uint8Array(32).fill(19);
  const enrolled = {...profile, presentationKey: bytesToHex(ed25519.getPublicKey(seed), false)};
  const scope = {branch, sale, customer: "0fbc54ef-2827-4ab5-bfcd-3a27611c39eb"};
  const checked = {...order, cafeReview: {branchLabel: "Cafe", payerBound: true}};
  const signedProof: CommerceQuote = {...quote, presentation: {
    customer: scope.customer, origin: sale, claim: "a".repeat(64), signature: "",
    rentalEscrow: {expiresAt: at + 5, signature: ""},
  }};
  const generation = value(q.payerGenerationOffset);
  signedProof.presentation!.signature = bytesToHex(ed25519.sign(presentationMessage(signedProof, generation), seed), false);
  assert.throws(() => verifyPresentedRentalEscrow(signedProof, checked, enrolled, at, scope), /RENTAL_ESCROW_MISSING/);
  signedProof.presentation!.rentalEscrow!.signature =
    bytesToHex(ed25519.sign(rentalEscrowMessage(signedProof, generation), seed), false);
  assert.deepEqual(verifyPresentedRentalEscrow(signedProof, checked, enrolled, at, scope), {expiresAt: at + 5});
  assert.throws(() => verifyPresentedRentalEscrow(signedProof, checked, enrolled, at + 5, scope), /RENTAL_ESCROW_EXPIRED/);
  assert.throws(() => verifyPresentedRentalEscrow(signedProof, order, enrolled, at, scope), /RENTAL_ESCROW_MISSING/);
});
