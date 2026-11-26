import assert from "node:assert/strict";
import test from "node:test";
import { sha256 } from "@noble/hashes/sha256";
import {
  assertCommerceCatalogPage,
  bindServiceRequestV2,
  bytesToHex,
  CommerceClient,
  CommercePhotoCache,
  fromBase64,
  toBase64,
  type CommercePrice,
  type ServiceRequestV2,
} from "../src/index.ts";

const branch = "0fbc54ef-2827-4ab5-bfcd-3a27611c3001";
const now = 1_750_000_000;
const profile = {
  subjectRef: "0fbc54ef-2827-4ab5-bfcd-3a27611c3002", role: "customer", branchRef: branch,
  tenantRef: "0fbc54ef-2827-4ab5-bfcd-3a27611c3003", expiresAt: now + 120,
} as const;
const row = (sku: string): CommercePrice => ({
  version: 1, expiresAt: new Date((now + 60) * 1000).toISOString(), sku, optionKey: "standard", label: sku,
  unitPrice: "450000", asset: "0x" + "5".repeat(40), available: 2,
});
const jpeg = Uint8Array.from({ length: 40_000 }, (_, i) => (i * 7) & 255);
const name = (bytes: Uint8Array) => bytesToHex(sha256(bytes), false);
const photo = { photo: name(jpeg), thumb: name(jpeg.slice(0, 900)) };

function client(answer: (operation: string, body: Record<string, string>) => object, calls: string[] = []) {
  return new CommerceClient(profile, async (raw) => {
    const { command } = JSON.parse(raw) as { command: ServiceRequestV2 };
    calls.push(`${command.operation} ${command.body.offset ?? ""}`.trim());
    const binding = bindServiceRequestV2(command,
      { ...profile, grants: [{ domainRef: branch, objectRef: branch, operations: [`C3/${command.operation}`] }] }, now);
    return JSON.stringify({ ...binding, status: "ok", resultRef: branch, ...answer(command.operation, command.body) });
  }, async () => "0fbc54ef-2827-4ab5-bfcd-3a27611c3004", () => now);
}
// What the server sends: 18,000 bytes a part.
const parts = (bytes: Uint8Array) => (_: string, body: Record<string, string>) => {
  const at = Number(body.offset);
  return { photo: { size: bytes.length, data: toBase64(bytes.slice(at, at + 18_000)) } };
};

test("a menu page names each product's photo, and the rows carry it on", async () => {
  const { prices } = await client(() => ({ catalog: [row("cake"), row("latte")], catalogPhotos: [{ sku: "latte", ...photo }] })).catalogWindow(0);
  assert.deepEqual(prices[1].photo, photo);
  assert.equal(prices[0].photo, undefined);
  assertCommerceCatalogPage(prices, now, 4); // as the kiosk relays them to a phone
  for (const [what, photos] of Object.entries<unknown>({
    "a product not on the page": [{ sku: "mocha", ...photo }], "the same product twice": [{ sku: "latte", ...photo }, { sku: "latte", ...photo }],
    "a name that is not a digest": [{ sku: "latte", ...photo, thumb: "x" }], "more fields": [{ sku: "latte", ...photo, url: "http://x" }],
    "no list": { latte: photo },
  })) await assert.rejects(client(() => ({ catalog: [row("latte")], catalogPhotos: photos })).catalogWindow(0), /BAD_RESPONSE/, what);
  assert.throws(() => assertCommerceCatalogPage([{ ...row("latte"), photo: { ...photo, photo: "../x" } }], now, 4), /BAD_RESPONSE/);
});

test("a photo comes in parts under the reply limit and is checked against its name", async () => {
  const calls: string[] = [];
  const data = await client(parts(jpeg), calls).catalogPhoto(photo.photo);
  assert.deepEqual(fromBase64(data), jpeg);
  assert.deepEqual(calls, ["catalog.photo 0", "catalog.photo 18000", "catalog.photo 36000"]);
  const small = jpeg.slice(0, 900);
  calls.length = 0;
  assert.deepEqual(fromBase64(await client(parts(small), calls).catalogPhoto(photo.thumb)), small);
  assert.equal(calls.length, 1);
  const other = jpeg.map((b) => b ^ 1);
  await assert.rejects(client(parts(other)).catalogPhoto(photo.photo), /BAD_RESPONSE/);
  for (const [what, answer] of Object.entries<() => object>({
    "no photo": () => ({}), "an empty part": () => ({ photo: { size: 10, data: "" } }),
    "too large": () => ({ photo: { size: 200_000, data: toBase64(jpeg.slice(0, 10)) } }),
    "more than it said": () => ({ photo: { size: 5, data: toBase64(jpeg.slice(0, 10)) } }),
  })) await assert.rejects(client(answer).catalogPhoto(photo.photo), /BAD_RESPONSE/, what);
  let size = 40_000;
  await assert.rejects(client((_, body) => ({ photo: { size: size++, data: toBase64(jpeg.slice(Number(body.offset), Number(body.offset) + 18_000)) } })).catalogPhoto(photo.photo), /BAD_RESPONSE/);
  await assert.rejects(client(parts(jpeg)).catalogPhoto("not-a-name"), /BAD_REQUEST/);
});

test("the cache asks once, one photo at a time, keeps a few and retries a failure later", async () => {
  let clock = 0, running = 0, most = 0;
  const asked: string[] = [];
  const cache = new CommercePhotoCache(async (name) => {
    asked.push(name);
    running++;
    most = Math.max(most, running);
    await new Promise((resolve) => setTimeout(resolve, 2));
    running--;
    if (name === "bad") throw new Error("offline");
    return "data:" + name;
  }, 2, () => clock);
  let told = 0;
  const stop = cache.subscribe(() => told++);
  for (const name of ["a", "a", "b", "bad", "c", undefined]) cache.want(name);
  await cache.settled();
  assert.deepEqual(asked, ["a", "b", "bad", "c"]);
  assert.equal(most, 1);
  assert.equal(told, 4);
  assert.equal(cache.get("a"), undefined); // the oldest left
  assert.equal(cache.get("c"), "data:c");
  cache.want("bad");
  await cache.settled();
  assert.equal(asked.length, 4);
  clock = 60_001;
  cache.want("bad");
  await cache.settled();
  assert.equal(asked.length, 5);
  stop();
});
