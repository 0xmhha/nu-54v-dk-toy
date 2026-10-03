// The secure channel against Node's own crypto (OpenSSL): ECDH x coordinate, HKDF-SHA256 and
// AES-128-GCM with the 4.1 IV layout, so the TypeScript side is not checked only by itself.
import { test } from "node:test";
import assert from "node:assert/strict";
import { createCipheriv, createECDH, hkdfSync } from "node:crypto";
import { ephemeralKey, isPointX, ProtocolError, SecureChannel, sessionKey } from "../src/index.ts";

const fill = (b: number) => (n: number) => new Uint8Array(n).fill(b);
const kioskNonce = new Uint8Array(32).fill(0xa5);
const deviceNonce = new Uint8Array(32).fill(0x5a);

test("both ends derive the key OpenSSL derives", () => {
  const kiosk = ephemeralKey(fill(0xe1));
  const device = ephemeralKey(fill(0x5b));
  const a = sessionKey(kiosk.privateKey, device.x, kioskNonce, deviceNonce);
  const b = sessionKey(device.privateKey, kiosk.x, kioskNonce, deviceNonce);
  assert.deepEqual(a, b);

  const ecdh = createECDH("secp256k1");
  ecdh.setPrivateKey(Buffer.from(kiosk.privateKey));
  const shared = ecdh.computeSecret(Buffer.concat([Buffer.from([0x02]), Buffer.from(device.x)]));
  const want = new Uint8Array(hkdfSync("sha256", shared, Buffer.concat([kioskNonce, deviceNonce]), "nu54 session v1", 16));
  assert.deepEqual(a, want);
});

test("bodies are AES-128-GCM with direction and message number in the IV", () => {
  const key = new Uint8Array(16).fill(7);
  const kiosk = new SecureChannel(key, "kiosk");
  const device = new SecureChannel(key, "device");
  const body = new TextEncoder().encode("first");
  kiosk.seal(new Uint8Array(1)); // message 0
  const sealed = kiosk.seal(body); // message 1
  const iv = Buffer.alloc(12);
  iv[0] = 0x01;
  iv[11] = 1;
  const c = createCipheriv("aes-128-gcm", key, iv);
  const want = Buffer.concat([c.update(body), c.final(), c.getAuthTag()]);
  assert.deepEqual(sealed, new Uint8Array(want));
  assert.deepEqual(device.open(new SecureChannel(key, "kiosk").seal(body)), body); // a fresh kiosk end starts at message 0
});

test("a body out of order, altered or from the wrong direction is BAD_FRAME", () => {
  const key = new Uint8Array(16).fill(9);
  const kiosk = new SecureChannel(key, "kiosk");
  const device = new SecureChannel(key, "device");
  const m0 = kiosk.seal(Uint8Array.of(1, 2, 3));
  const m1 = kiosk.seal(Uint8Array.of(4));
  assert.throws(() => device.open(m1), (e) => e instanceof ProtocolError && e.reason === "BAD_FRAME");
  assert.deepEqual(device.open(m0), Uint8Array.of(1, 2, 3));
  const altered = m1.slice();
  altered[0] ^= 1;
  assert.throws(() => device.open(altered), ProtocolError);
  assert.deepEqual(device.open(m1), Uint8Array.of(4));
  assert.throws(() => device.open(device.seal(Uint8Array.of(5))), ProtocolError); // its own direction
});

test("ephemeral keys skip draws that are not scalars; x off the curve is refused", () => {
  let draws = 0;
  const k = ephemeralKey((n) => new Uint8Array(n).fill(draws++ === 0 ? 0xff : 0x11));
  assert.equal(draws, 2);
  assert.deepEqual(k.privateKey, new Uint8Array(32).fill(0x11));
  assert.ok(isPointX(k.x));
  const bad = new Uint8Array(32).fill(0); // x = 0: 0^3 + 7 has no square root mod p
  assert.equal(isPointX(bad), false);
  assert.throws(() => sessionKey(k.privateKey, bad, kioskNonce, deviceNonce));
});
