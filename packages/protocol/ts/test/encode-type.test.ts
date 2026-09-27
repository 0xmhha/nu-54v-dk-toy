import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { ENCODE_TYPE } from "../src/index.ts";

// Generated encodeType strings must match the committed EIP-712 vectors.
test("encodeType matches vectors", () => {
  const url = new URL("../../../../docs/content/specifications/protocol/eip712-vectors.json", import.meta.url);
  const doc = JSON.parse(readFileSync(url, "utf8")) as { vectors: { id: string; primaryType: keyof typeof ENCODE_TYPE; encodeType: string }[] };
  for (const v of doc.vectors) {
    assert.equal(ENCODE_TYPE[v.primaryType], v.encodeType, v.id);
  }
});
