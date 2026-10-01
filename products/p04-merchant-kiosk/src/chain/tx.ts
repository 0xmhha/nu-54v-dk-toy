// EIP-1559 (type 2) transactions: RLP encoding and signing with the kiosk gas key.

import { keccak_256 } from "@noble/hashes/sha3";
import { bigToBytes, bytesToHex, concat, hexToBytes } from "@nu54/protocol";
import type { Hex } from "./rpc.ts";

/** The kiosk gas key. Android Keystore backs it in the app ([N32]); tests use a raw key. */
export interface Signer {
  address: Hex;
  /** 65-byte r || s || v signature (v = 27/28, low-s) of a 32-byte digest. */
  sign(digest32: Uint8Array): Uint8Array;
}

type RlpItem = Uint8Array | RlpItem[];

function rlpLength(len: number, offset: number): Uint8Array {
  if (len < 56) return Uint8Array.of(offset + len);
  const l = bigToBytes(BigInt(len));
  return concat(Uint8Array.of(offset + 55 + l.length), l);
}

export function rlp(item: RlpItem): Uint8Array {
  if (item instanceof Uint8Array) {
    if (item.length === 1 && item[0] < 0x80) return item;
    return concat(rlpLength(item.length, 0x80), item);
  }
  const body = concat(...item.map(rlp));
  return concat(rlpLength(body.length, 0xc0), body);
}

const int = (n: bigint) => bigToBytes(n); // minimal big-endian, zero is empty

export interface Eip1559Tx {
  chainId: bigint;
  nonce: bigint;
  maxPriorityFeePerGas: bigint;
  maxFeePerGas: bigint;
  gas: bigint;
  to: Hex;
  data: Hex;
}

/** Signs a type-2 transaction and returns the raw bytes for eth_sendRawTransaction. */
export function signTransaction(tx: Eip1559Tx, signer: Signer): Hex {
  const fields: RlpItem[] = [
    int(tx.chainId),
    int(tx.nonce),
    int(tx.maxPriorityFeePerGas),
    int(tx.maxFeePerGas),
    int(tx.gas),
    hexToBytes(tx.to),
    new Uint8Array(0), // value
    hexToBytes(tx.data),
    [], // access list
  ];
  const digest = keccak_256(concat(Uint8Array.of(0x02), rlp(fields)));
  const sig = signer.sign(digest);
  const yParity = BigInt(sig[64] - 27);
  const r = BigInt(bytesToHex(sig.subarray(0, 32)));
  const s = BigInt(bytesToHex(sig.subarray(32, 64)));
  return bytesToHex(concat(Uint8Array.of(0x02), rlp([...fields, int(yParity), int(r), int(s)]))) as Hex;
}

/** keccak256 of the raw transaction: its hash on the chain. */
export function transactionHash(raw: Hex): Hex {
  return bytesToHex(keccak_256(hexToBytes(raw))) as Hex;
}
