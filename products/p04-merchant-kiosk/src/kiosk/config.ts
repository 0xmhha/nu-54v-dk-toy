// Kiosk settings and keys (N32).
//
// Development provisioning (scripts/provision-dev.ts on the Mac) writes provision.json into the
// app's private files directory with adb run-as; the app takes it once, wraps the two keys with
// the Keystore key and deletes the file. anchor.json (an operator-signed TimeAnchor from opsctl)
// arrives the same way right before a week-7 run: the device clock follows the anchor, so it
// must be fresh (payment-protocol.md 5).

import { hexToBytes } from "@nu54/protocol";
import type { Hex } from "../chain/rpc.ts";
import type { Attestation, TimeAnchor } from "../payment/session.ts";
import { fromBase64, toBase64 } from "../ble/base64.ts";

export interface KioskConfig {
  rpc: string;
  chainId: number;
  settlement: Hex;
  token: Hex;
  tokenSymbol: string;
  tokenDecimals: number;
  /** Deployment block of the settlement contract (event search start). */
  fromBlock: string;
  /** kioskMinGasBalance in wei (register). */
  minGasBalanceWei: string;
  attestation: Attestation;
}

export interface Provision {
  config: KioskConfig;
  /** Raw secp256k1 keys, hex. */
  keys: { gas: string; merchant: string };
}

/** The native vault, as the app sees it; tests pass a fake. */
export interface Vault {
  wrapKey(alias: string, base64: string): Promise<void>;
  unwrapKey(alias: string): Promise<string | null>;
  putSetting(name: string, value: string): Promise<void>;
  getSetting(name: string): Promise<string | null>;
  takeFile(name: string): Promise<string | null>;
}

export interface Loaded {
  config: KioskConfig;
  gasKey: Uint8Array;
  merchantKey: Uint8Array;
}

function checkProvision(p: Provision): void {
  const c = p.config;
  for (const k of ["rpc", "settlement", "token", "fromBlock", "minGasBalanceWei"] as const) {
    if (!c?.[k]) throw new Error(`provision.json: config.${k} is missing`);
  }
  if (!c.attestation?.operatorSignature) throw new Error("provision.json: config.attestation is missing");
  for (const k of ["gas", "merchant"] as const) {
    if (hexToBytes(p.keys?.[k] ?? "").length !== 32) throw new Error(`provision.json: keys.${k} is not a 32-byte key`);
  }
}

/** Imports a pending provision.json, then loads settings and keys. Null when not provisioned. */
export async function loadKiosk(vault: Vault): Promise<Loaded | null> {
  const pending = await vault.takeFile("provision.json");
  if (pending) {
    const p = JSON.parse(pending) as Provision;
    checkProvision(p);
    await vault.wrapKey("gas", toBase64(hexToBytes(p.keys.gas)));
    await vault.wrapKey("merchant", toBase64(hexToBytes(p.keys.merchant)));
    await vault.putSetting("config", JSON.stringify(p.config));
  }
  const config = await vault.getSetting("config");
  const gas = await vault.unwrapKey("gas");
  const merchant = await vault.unwrapKey("merchant");
  if (!config || !gas || !merchant) return null;
  return { config: JSON.parse(config) as KioskConfig, gasKey: fromBase64(gas), merchantKey: fromBase64(merchant) };
}

/** A fresh anchor pushed for the next run (week-7 development setup), taken once. */
export async function takeAnchor(vault: Vault): Promise<TimeAnchor | undefined> {
  const text = await vault.takeFile("anchor.json");
  return text ? (JSON.parse(text) as TimeAnchor) : undefined;
}
