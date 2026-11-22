import { readFileSync } from "node:fs";
import assert from "node:assert/strict";
import { ENCODE_TYPE } from "@nu54/protocol";
import { quoteDigest, verifyQuote, V2Link, V2Journal, v2PeerRole, bindServiceRequestV2, validateServiceReplyV2 } from "@nu54/protocol/v2";
if (!ENCODE_TYPE.PaymentAuthorization.startsWith("PaymentAuthorization(")) throw new Error("v1 SDK mismatch");
const specs = new URL("./node_modules/@nu54/protocol/spec-v2/", import.meta.url);
const cases = readFileSync(new URL("testdata/quote.vectors", specs), "utf8").split("\n")
  .filter((line) => line && !line.startsWith("#")).map((line) => line.split("|"));
assert(cases.length >= 31);
const quote = cases[0];
const bytes = (value: string) => Buffer.from(value, "hex");
for (const [name, signed, expected, unsigned, signer, now, consumed, digest, status] of cases) {
  if (digest) assert.equal(Buffer.from(quoteDigest(bytes(unsigned))).toString("hex"), digest, name);
  else assert.throws(() => quoteDigest(bytes(unsigned)));
  const policy = {expected: bytes(expected), merchantSigner: bytes(signer), now: BigInt(now), consumed: consumed === "true"};
  assert.equal(verifyQuote(bytes(signed), policy), Number(status), name);
}
assert.equal(v2PeerRole("payment", "terminal"), "board");
const service = JSON.parse(readFileSync(new URL("service-vectors.json", specs), "utf8")).find((item: {id: string, reply?: object}) => item.id === "C7/evidence.read" && item.reply);
const binding = bindServiceRequestV2(service.request, service.scope, service.at);
if (binding.requestDigest !== service.requestDigest) throw new Error("service binding mismatch");
validateServiceReplyV2(service.request, binding, service.reply, service.at);
assert.throws(() => bindServiceRequestV2(service.request, {...service.scope, grants: []}, service.at));
const journal = new V2Journal(2);
const owner = new Uint8Array(32).fill(1), board = new Uint8Array(32).fill(2);
const request = {command: "pay.prepare" as const, id: new Uint8Array(16).fill(7), expires: 100n, payload: bytes(quote[1])};
function submit(epochByte: number, now: bigint, body: typeof request) {
    const epoch = new Uint8Array(16).fill(epochByte);
    const send = new V2Link("management", "owner", epoch, board, new V2Journal(1));
    const receive = new V2Link("management", "board", epoch, owner, journal);
    return send.send(body).map((frame) => receive.receive(frame, now)).at(-1);
}
assert.equal(submit(1, 1n, request)?.disposition, "new");
assert.equal(submit(2, 2n, request)?.disposition, "resume");
assert.equal(submit(3, 400n, {...request, id: new Uint8Array(16).fill(8), expires: 500n})?.disposition, "new");
assert.throws(() => submit(4, 90n, request), /clock/);
