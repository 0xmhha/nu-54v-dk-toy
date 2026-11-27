import assert from "node:assert/strict";
import test from "node:test";
import { encodePickupQr, parsePickupQr } from "../src/index.ts";

const token = "0123456789abcdef".repeat(4);

test("a pickup QR is the fixed prefix and the token, and reads back to the token", () => {
  assert.equal(encodePickupQr(token), `nu54://pickup/${token}`);
  assert.equal(parsePickupQr(encodePickupQr(token)), token);
});

test("anything other than the exact form is refused", () => {
  for (const text of [
    "",
    token,
    `nu54://pickup/${token.toUpperCase()}`,
    `NU54://pickup/${token}`,
    `nu54://pickup/${token.slice(1)}`,
    `nu54://pickup/${token}0`,
    `nu54://pickup/${token}\n`,
    ` nu54://pickup/${token}`,
    `nu54://pickup/${token} `,
    `nu54://pickup/${token}?order=1`,
    `nu54://pickup/${token}#x`,
    `nu54://pickup//${token}`,
    `nu54://pickup/${token.slice(0, 63)}g`,
    `nu54://order/${token}`,
    `https://pickup/${token}`,
    `nu54://pickup/${"0".repeat(64)}`,
    `nu54://pickup/${token}\u0000`,
  ]) assert.throws(() => parsePickupQr(text), /PICKUP_QR_INVALID/, JSON.stringify(text));
  assert.throws(() => parsePickupQr(undefined as unknown as string), /PICKUP_QR_INVALID/);
});

test("the encoder refuses a token the reader would refuse", () => {
  for (const bad of [token.toUpperCase(), token.slice(2), "0".repeat(64), `${token}/x`, "0x" + token.slice(2)])
    assert.throws(() => encodePickupQr(bad), /PICKUP_QR_INVALID/);
});
