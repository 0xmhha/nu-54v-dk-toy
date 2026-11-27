import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { encodeQuote, quoteDigest, quoteV2Layout, verifyQuote } from "../src/quote-v2.ts";

for (const line of readFileSync(new URL("../../v2/testdata/quote.vectors", import.meta.url), "utf8").split("\n")) {
  if (!line || line.startsWith("#")) continue;
  const [name, raw, expected, unsigned, signer, clock, consumed, digest, status] = line.split("|");
  const bytes = (hex: string) => Buffer.from(hex, "hex");
  test(`merchant quote v2 / ${name}`, () => {
    if (digest) {
      const raw = bytes(unsigned);
      const fields = quoteV2Layout.widths.map((width, i) => raw.subarray((i + 1) * 32 - width, (i + 1) * 32));
      assert.deepEqual(Buffer.from(encodeQuote(fields)), raw);
      assert.equal(Buffer.from(quoteDigest(raw)).toString("hex"), digest);
    } else {
      assert.throws(() => quoteDigest(bytes(unsigned)));
    }
    assert.equal(verifyQuote(bytes(raw), {
      expected: bytes(expected), merchantSigner: bytes(signer), now: BigInt(clock), consumed: consumed === "true",
    }), Number(status));
  });
}

test("merchant quote v2 rejects malformed runtime policy before authorization", () => {
  const [, raw, expected, , signer, clock] = readFileSync(
    new URL("../../v2/testdata/quote.vectors", import.meta.url), "utf8",
  ).split("\n")[1].split("|");
  const signed = Buffer.from(raw, "hex");
  const good = {
    expected: Buffer.from(expected, "hex"), merchantSigner: Buffer.from(signer, "hex"),
    now: BigInt(clock), consumed: false,
  };
  const zeroAmount = Buffer.from(good.expected);
  zeroAmount.fill(0, quoteV2Layout.amountOffset, quoteV2Layout.amountOffset + 32);
  for (const policy of [
    null, undefined, { ...good, now: undefined }, { ...good, now: null }, { ...good, now: NaN },
    { ...good, now: Number(clock) }, { ...good, now: clock }, { ...good, now: -1n },
    { ...good, now: 1n << 64n }, { ...good, consumed: undefined }, { ...good, consumed: null },
    { ...good, consumed: 0 }, { ...good, consumed: "false" }, { ...good, expected: zeroAmount },
    { ...good, expected: new Uint8Array(0) }, { ...good, merchantSigner: new Uint8Array(20) },
  ]) {
    assert.equal(verifyQuote(signed, policy as typeof good), quoteV2Layout.statusMalformed);
  }
});
