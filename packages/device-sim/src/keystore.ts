// Opens a Web3 Secret Storage (v3) keystore, as `cast wallet new` writes it, with a password
// from a secretRef: keychain:<service> (macOS Keychain, account nu54) or file:<path>.
// Node only: the simulator runs on a development machine, never on a device.

import { execFileSync } from "node:child_process";
import { createDecipheriv, scryptSync } from "node:crypto";
import { readFileSync } from "node:fs";
import { keccak_256 } from "@noble/hashes/sha3";
import { bytesToHex, concat, hexToBytes } from "@nu54/protocol";

export function resolveSecret(ref: string): string {
  if (ref.startsWith("keychain:")) {
    return execFileSync("security", ["find-generic-password", "-a", "nu54", "-s", ref.slice(9), "-w"], { encoding: "utf8" }).replace(/\n$/, "");
  }
  if (ref.startsWith("file:")) return readFileSync(ref.slice(5), "utf8").replace(/\n$/, "");
  throw new Error(`secretRef ${ref}: use keychain:<service> or file:<path>`);
}

export function openKeystore(path: string, passwordRef: string): Uint8Array {
  const ks = JSON.parse(readFileSync(path, "utf8"));
  const c = ks.crypto ?? ks.Crypto;
  if (c.kdf !== "scrypt" || c.cipher !== "aes-128-ctr") throw new Error(`unsupported keystore (${c.kdf}, ${c.cipher})`);
  const p = c.kdfparams;
  const key = scryptSync(resolveSecret(passwordRef), Buffer.from(hexToBytes(p.salt)), p.dklen, {
    N: p.n,
    r: p.r,
    p: p.p,
    maxmem: 256 * p.n * p.r + 1024 * 1024,
  });
  const cipher = hexToBytes(c.ciphertext);
  const mac = keccak_256(concat(new Uint8Array(key.subarray(16, 32)), cipher));
  if (bytesToHex(mac, false) !== c.mac.toLowerCase()) throw new Error("wrong keystore password");
  const d = createDecipheriv("aes-128-ctr", key.subarray(0, 16), Buffer.from(hexToBytes(c.cipherparams.iv)));
  return new Uint8Array(Buffer.concat([d.update(Buffer.from(cipher)), d.final()]));
}
