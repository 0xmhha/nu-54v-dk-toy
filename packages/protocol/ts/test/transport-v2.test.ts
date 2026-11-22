import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import { hexToBytes } from "../src/bytes.ts";
import { encodePaymentReview, readPaymentReview, V2Link, V2Journal, type V2Request } from "../src/transport-v2.ts";

const epoch = Uint8Array.from({ length: 16 }, (_, i) => i + 1);
const nextEpoch = Uint8Array.from(epoch, (i) => i + 16);
const owner = new Uint8Array(32).fill(1);
const board = new Uint8Array(32).fill(2);
const terminal = new Uint8Array(32).fill(3);
const request: V2Request = {
  command: "pay.prepare", id: new Uint8Array(16).fill(7), expires: 100n, payload: new Uint8Array(737).fill(9),
};

test("payment review evidence is bound to the exact request ID and signed quote bytes", () => {
  const vectors = readFileSync(new URL("../../v2/testdata/payment-review.vectors", import.meta.url), "utf8")
    .trim().split("\n").map(row => row.split("|")[1]);
  const body = encodePaymentReview(request.id, request.payload);
  assert.equal(body.length, 754);
  assert.equal(body[0], 0x75);
  const alternateRequest = body.slice();
  const alternateSignature = body.slice();
  alternateRequest[1] = 4;
  alternateSignature[body.length - 1] = 8;
  for (const [i, reviewed] of [body, alternateRequest, alternateSignature].entries()) {
    const reply = hexToBytes("75" + vectors[i] + "01");
    assert.equal(readPaymentReview(reviewed, reply), "ready");
    reply[33] = 0;
    assert.equal(readPaymentReview(reviewed, reply), "refused");
    if (i > 0) assert.throws(() => readPaymentReview(body, reply), /PAYMENT_BOARD_EVIDENCE/);
    reply[33] = 2;
    assert.throws(() => readPaymentReview(reviewed, reply), /PAYMENT_BOARD_EVIDENCE/);
    assert.throws(() => readPaymentReview(reviewed, reply.subarray(0, 33)), /PAYMENT_BOARD_EVIDENCE/);
  }
  assert.throws(() => encodePaymentReview(new Uint8Array(16), request.payload));
  assert.throws(() => encodePaymentReview(request.id, request.payload.subarray(1)));
});

test("role routes carry identical payment bytes and resume on a fresh epoch", () => {
  const journal = new V2Journal(8);
  const send = new V2Link("management", "owner", epoch, board, new V2Journal(1));
  const receive = new V2Link("management", "board", epoch, owner, journal);
  const fragments = send.send(request);
  assert(fragments.length > 1);
  for (const fragment of fragments.slice(0, -1)) assert.equal(receive.receive(fragment, 1n), undefined);
  const first = receive.receive(fragments.at(-1)!, 1n)!;
  assert.equal(first.disposition, "new");
  assert.deepEqual(first.request.payload, request.payload);
  const reconnect = new V2Link("management", "board", nextEpoch, owner, journal);
  const retry = new V2Link("management", "owner", nextEpoch, board, new V2Journal(1));
  assert.equal(retry.send(request).map((f) => reconnect.receive(f, 2n)).at(-1)?.disposition, "resume");
  const direct = new V2Link("payment", "terminal", epoch, board, new V2Journal(1));
  const pay = new V2Link("payment", "board", epoch, terminal, journal);
  assert.deepEqual(direct.send(request).map((f) => pay.receive(f, 2n)).at(-1)?.request.payload, first.request.payload);
});

test("rejects old epoch, reordered fragments, expiry and nearby role confusion", () => {
  const send = new V2Link("management", "owner", epoch, board, new V2Journal(1));
  const frames = send.send(request);
  assert.throws(() => new V2Link("management", "board", nextEpoch, owner, new V2Journal(1)).receive(frames[0], 1n));
  assert.throws(() => new V2Link("management", "board", epoch, owner, new V2Journal(1)).receive(frames[1], 1n));
  const slow = new V2Link("management", "board", epoch, owner, new V2Journal(1));
  assert.equal(slow.receive(frames[0], 1n), undefined);
  assert.throws(() => slow.receive(frames[1], 6n), /fragment timeout/);
  const expired = new V2Link("management", "board", epoch, owner, new V2Journal(1));
  assert.throws(() => frames.forEach((f) => expired.receive(f, 100n)), /expiry/);
  const nearby = new V2Link("nearby", "customer", epoch, terminal, new V2Journal(1));
  assert.throws(() => nearby.send(request), /role/);
});

test("reusing an ID for another body is terminal and a complete replay is rejected", () => {
  const sender = new V2Link("management", "owner", epoch, board, new V2Journal(1));
  const receiver = new V2Link("management", "board", epoch, owner, new V2Journal(8));
  const frames = sender.send(request);
  frames.forEach((f) => receiver.receive(f, 1n));
  receiver.send({ ...request, command: "reply", payload: new Uint8Array() }).forEach((f) => sender.receive(f, 1n));
  assert.throws(() => sender.send({ ...request, payload: new Uint8Array([1]) }).forEach((f) => receiver.receive(f, 1n)), /conflict/);
  assert.throws(() => receiver.receive(frames[0], 1n), /closed/);
  const fresh = new V2Link("management", "board", epoch, owner, new V2Journal(8));
  frames.forEach((f) => fresh.receive(f, 1n));
  assert.throws(() => fresh.receive(frames[0], 1n), /sequence/);
});

test("authenticated replies must resolve the outstanding request ID and expiry", () => {
  const sender = new V2Link("management", "owner", epoch, board, new V2Journal(1));
  const receiver = new V2Link("management", "board", epoch, owner, new V2Journal(1));
  sender.send(request).forEach((frame) => receiver.receive(frame, 1n));
  const reply = { ...request, command: "reply" as const, payload: new Uint8Array() };
  assert.throws(() => receiver.send({ ...reply, expires: reply.expires + 1n }), /outstanding/);
  assert.throws(() => sender.send({ ...request, id: new Uint8Array(16).fill(1) }), /outstanding/);
  assert.equal(sender.receive(receiver.send(reply)[0], 1n)?.disposition, "new");
  assert.throws(() => receiver.send(reply), /outstanding/);
});

test("expired journal records cannot revive after a clock rollback and reconnect", () => {
  const journal = new V2Journal(2);
  function submit(session: Uint8Array, now: bigint, body: V2Request) {
    const sender = new V2Link("management", "owner", session, board, new V2Journal(1));
    const receiver = new V2Link("management", "board", session, owner, journal);
    return sender.send(body).map((frame) => receiver.receive(frame, now)).at(-1);
  }
  assert.equal(submit(epoch, 1n, request)?.disposition, "new");
  assert.equal(submit(nextEpoch, 400n, { ...request, id: new Uint8Array(16).fill(8), expires: 500n })?.disposition, "new");
  const thirdEpoch = new Uint8Array(16).fill(255);
  assert.throws(() => submit(thirdEpoch, 90n, request), /clock/);
  assert.throws(() => submit(thirdEpoch, 2n ** 64n, request), /clock/);
  assert.throws(() => submit(thirdEpoch, 400 as unknown as bigint, request), /clock/);
});

const progress: V2Request = {
  ...request, command: "approval.progress", payload: hexToBytes("010000000000000005"),
};
const finalReply: V2Request = { ...request, command: "reply", payload: Uint8Array.of(0) };
function progressPair(session: Uint8Array, journal = new V2Journal(2), receiverGeneration = 5n) {
  const ownerLink = new V2Link("management", "owner", session, board, new V2Journal(2));
  const boardLink = new V2Link("management", "board", session, owner, journal);
  ownerLink.enableApprovalProgress(receiverGeneration);
  boardLink.enableApprovalProgress(5n);
  const accepted = ownerLink.send(request).map((fragment) => boardLink.receive(fragment, 2n)).at(-1);
  return { ownerLink, boardLink, accepted };
}

test("negotiated progress observes a fresh original without resolving its one outstanding request", () => {
  const { ownerLink, boardLink } = progressPair(epoch);
  assert.throws(() => boardLink.send({ ...progress, id: new Uint8Array(16).fill(6) }), /outstanding/);
  assert.throws(() => boardLink.send({ ...progress, expires: 99n }), /outstanding/);
  assert.throws(() => boardLink.send({ ...progress, payload: hexToBytes("010000000000000006") }), /progress/);
  const frame = boardLink.send(progress)[0];
  assert.equal(frame[22], 6);
  assert.deepEqual(ownerLink.receive(frame, 2n)?.request, progress);
  assert.throws(() => boardLink.send(progress), /progress/);
  assert.throws(() => ownerLink.send({ ...request, command: "manage", payload: Uint8Array.of(3) }), /outstanding/);
  assert.deepEqual(ownerLink.receive(boardLink.send(finalReply)[0], 3n)?.request, finalReply);
  assert.throws(() => boardLink.send(progress), /outstanding/);
  assert.doesNotThrow(() => ownerLink.send({ ...request, command: "manage", payload: Uint8Array.of(3) }));
});

test("progress fails closed across roles, negotiation, generations, replay, expiry and TLS epochs", () => {
  const journal = new V2Journal(2);
  const { boardLink } = progressPair(epoch, journal);
  const frame = boardLink.send(progress)[0];
  const resumed = progressPair(nextEpoch, journal);
  assert.equal(resumed.accepted?.disposition, "resume");
  assert.throws(() => resumed.boardLink.send(progress), /progress/);
  assert.throws(() => resumed.ownerLink.receive(frame, 2n), /epoch/);
  const mismatched = progressPair(epoch, undefined, 6n);
  assert.throws(() => mismatched.ownerLink.receive(frame, 2n), /progress/);
  assert.throws(() => progressPair(epoch).ownerLink.receive(frame, 100n), /expiry/);
  const duplicate = progressPair(epoch).ownerLink;
  duplicate.receive(frame, 2n);
  const restamped = frame.slice();
  restamped[19] = 1;
  assert.throws(() => duplicate.receive(restamped, 2n), /progress/);
  const unnegotiated = new V2Link("management", "owner", epoch, board, new V2Journal(2));
  unnegotiated.send(request);
  assert.throws(() => unnegotiated.receive(frame, 2n), /progress/);
  const terminalBoard = new V2Link("payment", "board", epoch, terminal, new V2Journal(2));
  assert.throws(() => terminalBoard.enableApprovalProgress(5n), /progress/);
  assert.throws(() => terminalBoard.send(progress), /role/);
});
