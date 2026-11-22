import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { p256 } from "@noble/curves/p256";
import { sha256 } from "@noble/hashes/sha2";
import { bigToBytes, bytesToBig, bytesToHex, concat, hexToBytes, utf8Encode } from "../src/bytes.ts";
import { quoteV2Layout as q } from "../src/quote-v2.ts";
import { rentalReviewPrepare, verifyRentalPaymentReview, verifyRentalPaymentDecision } from "../src/rental-payment-review.ts";
import { V2Journal, V2Link } from "../src/transport-v2.ts";

const [, line] = readFileSync(new URL("../../v2/testdata/quote.vectors", import.meta.url), "utf8").split("\n");
const [,,,,merchant,clock] = line.split("|");
const quote = hexToBytes(line.split("|")[1]);
const word = (offset: number) => quote.subarray(offset, offset + 32);
const payer = word(q.payerOffset).subarray(12);
const payment = {
  bytes: quote, payer: bytesToHex(payer), payerRole: "renter" as const,
  generation: bytesToBig(word(q.payerGenerationOffset)), deadline: Number(bytesToBig(word(q.expiresAtOffset))),
  digest: "", approvalDigest: "",
};
const domain = utf8Encode("NU54 W10 rental payment review v1\0");
const key = new Uint8Array(32).fill(7);
const pub = p256.getPublicKey(key, false);
const spki = concat(hexToBytes("3059301306072a8648ce3d020106082a8648ce3d030107034200"), pub);
const boardPin = sha256(spki);
function seeded(size: number, value: number) {
  const bytes = new Uint8Array(size);
  bytes[0] = value;
  return bytes;
}
const terminalPin = seeded(32, 2), epoch = seeded(16, 1), request = seeded(16, 4), challenge = seeded(32, 3);
const expected = {
  boardPin: bytesToHex(boardPin,false), terminalPin: bytesToHex(terminalPin,false), signer: bytesToHex(pub,false),
  request: bytesToHex(request,false), challenge: bytesToHex(challenge,false), now: Number(clock),
};
const signed = concat(utf8Encode("W10R"), boardPin, terminalPin, epoch, request, challenge,
  word(q.orderIdOffset), sha256(quote), payer, bigToBytes(payment.generation, 8), bigToBytes(BigInt(payment.deadline), 8));
const digest = sha256(concat(domain, signed));
const compact = p256.sign(digest, key, {lowS: true, prehash: false}).toCompactRawBytes();
const receipt = concat(signed, compact);

test("W10 checks the independent P-256 key and every original scope binding", () => {
  assert.deepEqual(verifyRentalPaymentReview(receipt, payment, expected), {epoch,expires: payment.deadline});
  for (const offset of [0,4,36,68,84,100,132,164,196,216,224,232]) {
    const bad = receipt.slice();
    bad[offset] ^= 0x80;
    assert.throws(() => verifyRentalPaymentReview(bad, payment, expected));
  }
  assert.throws(() => verifyRentalPaymentReview(receipt, payment, {...expected, now: payment.deadline}));
  assert.throws(() => verifyRentalPaymentReview(receipt, {...payment, payerRole: "owner"}, expected));
  assert.throws(() => verifyRentalPaymentReview(receipt, payment, {...expected, signer: bytesToHex(p256.getPublicKey(new Uint8Array(32).fill(8),false),false)}));
  const highS = bigToBytes(p256.CURVE.n - bytesToBig(compact.subarray(32)), 32);
  assert.throws(() => verifyRentalPaymentReview(concat(signed,compact.subarray(0,32),highS), payment, expected));
});

test("a C2 review leaves the original outstanding, with no duplicate or cross-profile progress", () => {
  const board = new V2Link("payment","board",epoch.slice(),terminalPin,new V2Journal(2));
  const kiosk = new V2Link("payment","terminal",epoch.slice(),boardPin,new V2Journal(2));
  const prepare = {command:"pay.prepare" as const,id:request,expires:BigInt(payment.deadline),payload:rentalReviewPrepare(challenge, quote)};
  const review = {...prepare,command:"rental.review" as const,payload:receipt};
  const send = (origin:V2Link, dest:V2Link, message:typeof prepare|typeof review) =>
    origin.send(message).map(frame=>dest.receive(frame,BigInt(clock))).at(-1);
  assert.throws(() => board.send(review));
  assert.throws(() => kiosk.send(review));
  assert.equal(send(kiosk,board,prepare)?.disposition,"new");
  assert.equal(send(board,kiosk,review)?.request.command,"rental.review");
  assert.throws(()=>board.send(review));
  assert.throws(()=>kiosk.send(prepare)); // Still outstanding after the receipt.
  const final = {...prepare,command:"reply" as const,payload:Uint8Array.of(0)};
  for (const frame of board.send(final)) kiosk.receive(frame,BigInt(clock));
  assert.throws(()=>board.send(review));
  const owner = new V2Link("management","board",epoch.slice(),boardPin,new V2Journal(2));
  assert.throws(()=>owner.send(review));
});

if (process.env.NU54_W10_REVIEW_C) {
  test("W10 verifies the C-generated receipt over the same exact quote", async () => {
    const { execFileSync } = await import("node:child_process");
    const { writeFileSync } = await import("node:fs");
    const input = process.env.NU54_W10_REVIEW_INPUT!;
    writeFileSync(input, concat(quote,hexToBytes(merchant),new Uint8Array(32)));
    const result = new Uint8Array(execFileSync(process.env.NU54_W10_REVIEW_C!,["--review-rental",input]));
    const signer = result.subarray(0,65), raw = result.subarray(65);
    assert.equal(raw.length,296);
    assert.deepEqual(verifyRentalPaymentReview(raw,payment,{...expected,
      signer: bytesToHex(signer,false),
      boardPin: bytesToHex(sha256(concat(spki.subarray(0,26),signer)),false),
    }),{epoch,expires:payment.deadline});
  });
}


test("a signed final decision strictly separates refusal, invalid states and payer approval", () => {
  function final(status:number, signature:Uint8Array, forReview=receipt) {
    const body=concat(utf8Encode("W10D"),sha256(forReview),Uint8Array.of(status),signature);
    const digest=sha256(concat(utf8Encode("NU54 W10 rental payment decision v1\0"),body));
    return concat(body,p256.sign(digest,key,{lowS:true,prehash:false}).toCompactRawBytes());
  }
  const blank=new Uint8Array(65);
  const refusal=final(2,blank);
  assert.deepEqual(verifyRentalPaymentDecision(refusal,receipt,payment,expected),{decision:"refused"});
  const nonzero=blank.slice(); nonzero[0]=1;
  for(const bad of [final(1,blank),final(2,nonzero),final(0,blank),final(3,blank),
    final(2,blank,receipt.slice(1)),refusal.slice(0,-1),Uint8Array.of(0)])
    assert.throws(()=>verifyRentalPaymentDecision(bad,receipt,payment,expected));
});
