// The TypeScript protocol core against the shared vectors: EIP-712 digests and signers,
// deterministic CBOR (both directions plus the rejection rules) and BLE fragments.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import {
  addressOfPrivateKey,
  bytesToHex,
  decodeMessage,
  digest,
  encodeMessage,
  fragments,
  FrameWriter,
  hexToBytes,
  ProtocolError,
  Reassembler,
  recoverSigner,
  signDigest,
  type Message,
  type StructName,
} from "../src/index.ts";

const dir = new URL("../../../../docs/content/specifications/protocol/", import.meta.url);
const load = (name: string) => JSON.parse(readFileSync(new URL(name, dir), "utf8"));
const eip = load("eip712-vectors.json");
const cbor = load("cbor-vectors.json");
const frame = load("frame-vectors.json");

// Foundry's public test mnemonic, indexes 0..2: test-only keys, never used on a real network.
const ROLE_KEYS: Record<string, string> = {
  device: "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80",
  operator: "0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d",
  merchant: "0x5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a",
};

const lower = (v: unknown): unknown =>
  typeof v === "string" ? v.toLowerCase() : v && typeof v === "object" ? Object.fromEntries(Object.entries(v).map(([k, x]) => [k, lower(x)])) : v;

test("EIP-712 digests, signers and deterministic signatures match the vectors", () => {
  const domain = { chainId: eip.domain.chainId, verifyingContract: eip.domain.verifyingContract };
  for (const v of eip.vectors) {
    const d = digest(domain, v.primaryType as StructName, v.message);
    assert.equal(bytesToHex(d), v.digest, `${v.id} digest`);
    const sig = hexToBytes(v.signature);
    assert.equal(recoverSigner(d, sig), v.signer.toLowerCase(), `${v.id} signer`);
    const key = hexToBytes(ROLE_KEYS[v.signerRole]);
    assert.equal(addressOfPrivateKey(key), v.signer.toLowerCase(), `${v.id} role key`);
    assert.equal(bytesToHex(signDigest(d, key)), v.signature.toLowerCase(), `${v.id} signature bytes`);
  }
});

test("signatures outside the accepted form are refused", () => {
  const v = eip.vectors[0];
  const d = hexToBytes(v.digest);
  const sig = hexToBytes(v.signature);
  assert.throws(() => recoverSigner(d, sig.subarray(0, 64)));
  const badV = sig.slice();
  badV[64] = 1;
  assert.throws(() => recoverSigner(d, badV));
  const n = 0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141n;
  const s = BigInt(bytesToHex(sig.subarray(32, 64)));
  const high = sig.slice();
  high.set(hexToBytes((n - s).toString(16).padStart(64, "0")), 32);
  high[64] = sig[64] === 27 ? 28 : 27;
  assert.throws(() => recoverSigner(d, high), /high-s/);
});

test("CBOR encodes every vector to its bytes and decodes it back", () => {
  for (const v of cbor.vectors) {
    assert.equal(bytesToHex(encodeMessage(v.message as Message), false), v.cborHex, `${v.id} encode`);
    assert.deepEqual(lower(decodeMessage(hexToBytes(v.cborHex))), lower(v.message), `${v.id} decode`);
  }
});

test("CBOR decoding refuses bodies that break the encoding rules", () => {
  const good = hexToBytes(cbor.vectors.find((v: { id: string }) => v.id === "CB-08").cborHex);
  const bad = (hex: string, why: RegExp) =>
    assert.throws(() => decodeMessage(hexToBytes(hex)), (e: unknown) => e instanceof ProtocolError && e.reason === "BAD_FRAME" && why.test(e.message), String(why));
  // CB-08 is {sessionId, type, v, reason} in deterministic order; break it in specific ways.
  const msg = decodeMessage(good);
  assert.equal(msg.type, "error");
  bad(bytesToHex(good, false) + "00", /trailing bytes/);
  bad("a1" + "6474797065" + "6565727266", /truncated/); // 5-byte text with 4 bytes
  bad("a1" + "6474797065" + "62c328", /invalid UTF-8/);
  bad("a2" + "6176" + "1801" + "6474797065" + "656572726f72", /non-shortest/);
  bad("a2" + "6474797065" + "656572726f72" + "6176" + "01", /deterministic order/);
  bad("bf" + "6176" + "01" + "ff", /indefinite/);
  bad("a1" + "6176" + "f93c00", /float/);
  bad("a1" + "6176" + "c34100", /tag other than 2/);
  // A type outside the schema is UNSUPPORTED_TYPE, not BAD_FRAME ({type: "nope"}).
  assert.throws(() => decodeMessage(hexToBytes("a1" + "6474797065" + "646e6f7065")), (e: unknown) => e instanceof ProtocolError && e.reason === "UNSUPPORTED_TYPE");
  // An unknown key or a wrong byte length is refused in both directions.
  bad(bytesToHex(encodeMessage({ v: 1, type: "error", sessionId: "0102030405060708", reason: "BAD_FRAME" } as Message), false).replace(/^a4/, "a5") + "6b" + "7a".repeat(11) + "01", /unknown key/);
  assert.equal(bytesToHex(encodeMessage(msg), false), bytesToHex(good, false));
  assert.throws(() => encodeMessage({ v: 1, type: "error", sessionId: "01020304", reason: "BAD_FRAME" } as Message));
  assert.throws(() => encodeMessage({ v: 1, type: "error", sessionId: "0102030405060708", reason: "BAD_FRAME", extra: "x" } as Message));
});

test("bignum rule: below 2^64 must be a plain integer", () => {
  const v = cbor.vectors.find((x: { id: string }) => x.id === "CB-07");
  const hex: string = v.cborHex;
  // The nonce of CB-07 is a tag-2 bignum; replace it by a bignum of value 1 (below 2^64).
  const i = hex.indexOf("c258");
  assert.ok(i > 0);
  const len = parseInt(hex.slice(i + 4, i + 6), 16);
  const patched = hex.slice(0, i) + "c24101" + hex.slice(i + 6 + 2 * len);
  assert.throws(() => decodeMessage(hexToBytes(patched)), /bignum below 2\^64/);
});

test("fragments match the vectors and reassemble to the CBOR body", () => {
  const bodies = Object.fromEntries(cbor.vectors.map((v: { id: string; cborHex: string }) => [v.id, v.cborHex]));
  const envs = Object.fromEntries(cbor.vectors.map((v: { id: string; envelopeHex: string }) => [v.id, v.envelopeHex]));
  for (const v of frame.valid) {
    const ours = fragments(hexToBytes(envs[v.message]), v.sequence, v.attMtu).map((f) => bytesToHex(f, false));
    assert.deepEqual(ours, v.fragmentsHex, `${v.id} split`);
    const r = new Reassembler();
    let body: Uint8Array | undefined;
    v.fragmentsHex.forEach((h: string, i: number) => {
      const res = r.feed(hexToBytes(h));
      if (i < v.fragmentsHex.length - 1) assert.equal(res.status, "more", `${v.id} fragment ${i}`);
      else if (res.status === "done") body = res.body;
    });
    assert.equal(body && bytesToHex(body, false), bodies[v.message], `${v.id} body`);
  }
});

test("the receiver refuses every invalid fragment stream with BAD_FRAME", () => {
  for (const v of frame.invalid) {
    const r = new Reassembler();
    assert.throws(
      () => {
        for (const h of v.fragmentsHex) r.feed(hexToBytes(h));
        throw new Error("accepted");
      },
      (e: unknown) => e instanceof ProtocolError && e.reason === "BAD_FRAME",
      v.id,
    );
  }
});

test("the writer numbers messages and wraps the sequence after 255", () => {
  const w = new FrameWriter(23);
  const body = hexToBytes(cbor.vectors[7].cborHex);
  for (let i = 0; i < 256; i++) assert.equal(w.write(body)[0][0], i);
  assert.equal(w.write(body)[0][0], 0);
});

test("UTF-8 helpers round-trip and refuse malformed input", async () => {
  const { utf8Encode, utf8Decode } = await import("../src/bytes.ts");
  for (const s of ["", "Cafe Test 01", "카페 테스트", "€", "😀"]) {
    assert.deepEqual(utf8Encode(s), new Uint8Array(Buffer.from(s, "utf8")), s);
    assert.equal(utf8Decode(utf8Encode(s)), s);
  }
  for (const hex of ["c328", "c0af", "eda080", "f4908080", "e282", "80", "ff"]) {
    assert.throws(() => utf8Decode(hexToBytes(hex)), /invalid UTF-8/, hex);
  }
});
