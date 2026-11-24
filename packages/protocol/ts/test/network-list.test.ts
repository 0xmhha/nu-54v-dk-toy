import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { bytesToHex, utf8Encode } from "../src/bytes.ts";
import { keccak_256 } from "@noble/hashes/sha3";
import { networkListDigest, networkListLimit, readNetworkList } from "../src/network-list.ts";
import { addressOfPrivateKey, signDigest } from "../src/signature.ts";

const v = Object.fromEntries(readFileSync(new URL("../../v2/testdata/network-list.vectors", import.meta.url), "utf8").trim().split("\n")
  .map(l => [l.slice(0, l.indexOf("=")), l.slice(l.indexOf("=") + 1)])) as Record<string, string>;
const trust = { operator: v.operator };
const now = 1_800_000_000;
// A synthetic key; it signs nothing outside these tests.
const key = keccak_256(utf8Encode("nu54 network list test key"));
const envelope = (payload: string, signature = bytesToHex(signDigest(networkListDigest(payload), key))) =>
  JSON.stringify({ payload, signature });
const read = (payload: string, last = 0, at = now) => readNetworkList(envelope(payload), trust, at, last);
const edit = (from: string, to: string) => {
  const changed = v.payload.replace(from, to);
  assert.notEqual(changed, v.payload, from);
  return changed;
};

test("reads the list that the Go side signs", () => {
  assert.equal(addressOfPrivateKey(key).toLowerCase(), v.operator.toLowerCase());
  assert.equal(bytesToHex(networkListDigest(v.payload)), v.digest);
  assert.equal(envelope(v.payload), envelope(v.payload, v.signature));
  const list = readNetworkList(envelope(v.payload, v.signature), trust, now, 0);
  assert.deepEqual([list.sequence, list.networks.length], [7, 2]);
  assert.equal(list.networks[0].chainId, 8283);
  assert.deepEqual(list.networks[0].tokens, [{ address: "0x" + "0".repeat(38) + "a1", symbol: "tUSDC", name: "NU54 Test USD", decimals: 6 }]);
  assert.equal(list.networks[1].id, "bitcoin-signet");
});

test("refuses another signer and changed text", () => {
  assert.throws(() => readNetworkList(envelope(edit("api.test", "evil.test"), v.signature), trust, now, 0), /NETWORK_LIST_SIGNATURE/);
  assert.throws(() => readNetworkList(envelope(v.payload), { operator: "0x" + "1".repeat(40) }, now, 0), /NETWORK_LIST_SIGNATURE/);
  assert.throws(() => readNetworkList(envelope(v.payload, "0x" + "00".repeat(64) + "1b"), trust, now, 0), /NETWORK_LIST_SIGNATURE/);
  for (const raw of ["", "{}", "null", JSON.stringify({ payload: v.payload }), JSON.stringify({ payload: v.payload, signature: v.signature, extra: 1 }),
    JSON.stringify({ payload: v.payload, signature: v.signature.toUpperCase() }), " ".repeat(networkListLimit + 1)])
    assert.throws(() => readNetworkList(raw, trust, now, 0), /NETWORK_LIST_SHAPE/);
  assert.throws(() => readNetworkList(envelope(v.payload), { operator: "" }, now, 0), /NETWORK_LIST_SHAPE/);
});

test("refuses an expired list, one from the future and an older one", () => {
  assert.throws(() => read(v.payload, 0, 1_802_592_000), /NETWORK_LIST_TIME/);
  assert.throws(() => read(v.payload, 0, 1_799_999_000 - 301), /NETWORK_LIST_TIME/);
  assert.equal(read(v.payload, 0, 1_799_999_000 - 300).sequence, 7);
  assert.throws(() => read(edit('"expiresAt":1802592000', '"expiresAt":2802592000')), /NETWORK_LIST_TIME/);
  assert.throws(() => read(v.payload, 8), /NETWORK_LIST_ROLLBACK/);
  assert.equal(read(v.payload, 7).sequence, 7);
});

test("refuses plain HTTP, chain anchors and ambiguous rows even when signed", () => {
  for (const [from, to] of [
    ['"rpc":"https://', '"rpc":"http://'], ['"explorer":"https://', '"explorer":"javascript:'], ['"gasCoinUrl":"https://', '"gasCoinUrl":"http://'],
    ["https://api", "https://user@api"], ['network/"', 'network/?key=1"'],
    ['"v":1', '"v":1,"genesis":"0x00"'], ['"name":"StableNet Testnet"', '"name":"StableNet Testnet","anchor":"0x00"'],
    ['"v":1', '"v":1,"v":1'], ['"v":1', '"v": 1'], ['"v":1', '"v":2'],
    ['"id":"bitcoin-signet"', '"chainId":1,"id":"bitcoin-signet"'], ['"id":"bitcoin-signet"', '"chainId":8283'], ['"id":"bitcoin-signet",', ""],
    ['"chainId":8283', '"chainId":"8283"'], ["StableNet Testnet", "Stable" + String.fromCharCode(0x202e) + "Net"],
    ['"decimals":6', '"decimals":25'], ['"symbol":"tUSDC"', '"symbol":"t USDC"'],
    ["00a1", "00A1"], ['"sequence":7', '"sequence":0'],
    ['"rpc":"https://signet.example/bitcoin"', '"rpc":"https://signet.example/bitcoin","tokens":[]'],
  ]) assert.throws(() => read(edit(from, to)), /NETWORK_LIST_SHAPE/, to);
});
