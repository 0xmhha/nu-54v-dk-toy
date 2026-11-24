import { keccak_256 } from "@noble/hashes/sha3";
import { concat, hexToBytes, utf8Encode } from "./bytes.ts";
import { readServer } from "./commerce-payment.ts";
import { recoverSigner } from "./signature.ts";

/** Addresses of one network. The list names a network that the app already knows; it carries no chain anchor. */
export type NetworkRow = {
  /** An EVM network. Exactly one of chainId and id is present. */
  chainId?: number;
  /** A network that is not EVM, by the app's own name for it, such as "bitcoin-signet". */
  id?: string;
  name: string;
  rpc: string;
  explorer?: string;
  gasCoinUrl?: string;
  /** EVM networks only. */
  tokens?: NetworkToken[];
};
export type NetworkToken = { address: string; symbol: string; name: string; decimals: number };
export type NetworkList = { issuedAt: number; expiresAt: number; sequence: number; networks: NetworkRow[] };
/** The operator key that also signs store registrations, as a lower- or mixed-case 0x address. */
export type NetworkListTrust = { operator: string };

export const networkListLimit = 24_576;
const longestValidity = 400 * 24 * 3600;
// The operator's clock may run ahead of the phone's by this much.
const clockAllowance = 300;

/** EIP-712 digest of the exact payload text, in a domain of its own. */
export function networkListDigest(payload: string): Uint8Array {
  const text = (value: string) => keccak_256(utf8Encode(value));
  const domain = keccak_256(concat(text("EIP712Domain(string name,string version)"), text("NU54 Network List"), text("1")));
  return keccak_256(concat(Uint8Array.of(0x19, 0x01), domain,
    keccak_256(concat(text("NetworkList(string payload)"), text(payload)))));
}

function fail(reason: string): never {
  throw new Error(reason);
}

function keys(value: unknown, required: readonly string[], optional: readonly string[] = []): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value) &&
    required.every(key => Object.hasOwn(value, key)) &&
    Object.keys(value).every(key => required.includes(key) || optional.includes(key));
}

function label(value: unknown): boolean {
  return typeof value === "string" && value === value.trim() && value.length > 0 && value.length <= 40 &&
    !/[\u0000-\u001f\u007f-\u009f\u200b-\u200f\u2028-\u202e\u2066-\u2069\ufeff]/.test(value);
}

function time(value: unknown): value is number {
  return Number.isSafeInteger(value) && (value as number) > 0;
}

function tokens(value: unknown): boolean {
  if (!Array.isArray(value) || value.length > 8) return false;
  return value.every(token => keys(token, ["address", "symbol", "name", "decimals"]) &&
      typeof token.address === "string" && /^0x[0-9a-f]{40}$/.test(token.address) && !/^0x0+$/.test(token.address) &&
      typeof token.symbol === "string" && /^[A-Za-z0-9.-]{1,12}$/.test(token.symbol) && label(token.name) &&
      Number.isInteger(token.decimals) && (token.decimals as number) >= 0 && (token.decimals as number) <= 24) &&
    new Set(value.map(token => token.address)).size === value.length;
}

function row(value: unknown): boolean {
  if (!keys(value, ["name", "rpc"], ["chainId", "id", "explorer", "gasCoinUrl", "tokens"])) return false;
  const evm = Object.hasOwn(value, "chainId");
  return evm !== Object.hasOwn(value, "id") &&
    (evm ? time(value.chainId) : typeof value.id === "string" && /^[a-z][a-z0-9-]{1,31}$/.test(value.id)) &&
    label(value.name) && readServer(value.rpc) &&
    (value.explorer === undefined || readServer(value.explorer)) &&
    (value.gasCoinUrl === undefined || readServer(value.gasCoinUrl)) &&
    (value.tokens === undefined || (evm && tokens(value.tokens)));
}

/**
 * Reads the operator's network list, as `CommerceClient.networkList` returns it. Throws NETWORK_LIST_SHAPE,
 * NETWORK_LIST_SIGNATURE, NETWORK_LIST_TIME or NETWORK_LIST_ROLLBACK. `lastSequence` is the newest sequence the app
 * ever accepted (0 for none); the same one may be read again. The caller still checks each server against the chain
 * anchor built into the app before using it.
 */
export function readNetworkList(raw: string, trust: NetworkListTrust, now: number, lastSequence: number): NetworkList {
  let payload: string;
  let signature: string;
  let body: Record<string, unknown>;
  if (typeof raw !== "string" || raw.length > networkListLimit || !/^0x[0-9a-fA-F]{40}$/.test(trust?.operator ?? "") ||
      !time(now) || !Number.isSafeInteger(lastSequence) || lastSequence < 0) fail("NETWORK_LIST_SHAPE");
  try {
    const envelope: unknown = JSON.parse(raw);
    if (!keys(envelope, ["payload", "signature"]) || typeof envelope.payload !== "string" ||
        typeof envelope.signature !== "string" || !/^0x[0-9a-f]{128}(1b|1c)$/.test(envelope.signature)) fail("NETWORK_LIST_SHAPE");
    ({ payload, signature } = envelope as { payload: string; signature: string });
  } catch {
    fail("NETWORK_LIST_SHAPE");
  }
  let signer: string;
  try {
    signer = recoverSigner(networkListDigest(payload), hexToBytes(signature));
  } catch {
    fail("NETWORK_LIST_SIGNATURE");
  }
  if (signer.toLowerCase() !== trust.operator.toLowerCase()) fail("NETWORK_LIST_SIGNATURE");
  try {
    body = JSON.parse(payload);
  } catch {
    fail("NETWORK_LIST_SHAPE");
  }
  // One text form, so a duplicate key cannot be read two ways.
  if (JSON.stringify(body) !== payload || !keys(body, ["v", "issuedAt", "expiresAt", "sequence", "networks"]) || body.v !== 1 ||
      !time(body.issuedAt) || !time(body.expiresAt) || !time(body.sequence) ||
      !Array.isArray(body.networks) || body.networks.length < 1 || body.networks.length > 32 || !body.networks.every(row))
    fail("NETWORK_LIST_SHAPE");
  const list = body as unknown as NetworkList;
  if (new Set(list.networks.map(item => String(item.chainId ?? item.id))).size !== list.networks.length) fail("NETWORK_LIST_SHAPE");
  if (list.issuedAt > now + clockAllowance || list.expiresAt <= now || list.expiresAt <= list.issuedAt ||
      list.expiresAt - list.issuedAt > longestValidity) fail("NETWORK_LIST_TIME");
  if (list.sequence < lastSequence) fail("NETWORK_LIST_ROLLBACK");
  return { issuedAt: list.issuedAt, expiresAt: list.expiresAt, sequence: list.sequence, networks: list.networks };
}
