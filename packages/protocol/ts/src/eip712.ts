// EIP-712 hashing for the protocol structs ([N04][N21]). Types and the domain name/version
// come from the generated schema bindings; chainId and verifyingContract from the deployment.

import { keccak_256 } from "@noble/hashes/sha3";
import { bigToBytes, concat, hexToBytes, utf8Encode } from "./bytes.ts";
import { EIP712_DOMAIN, EIP712_TYPES, ENCODE_TYPE } from "./generated.ts";

export type StructName = keyof typeof EIP712_TYPES;
export type StructValue = Record<string, string | number | bigint>;
export interface Domain {
  chainId: bigint | number | string;
  verifyingContract: string;
}

const enc = { encode: utf8Encode };
const DOMAIN_TYPEHASH = keccak_256(enc.encode("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"));

function word(type: string, value: string | number | bigint): Uint8Array {
  switch (type) {
    case "uint256":
    case "uint64":
      return bigToBytes(BigInt(value), 32);
    case "address": {
      const raw = hexToBytes(String(value));
      if (raw.length !== 20) throw new Error("address must be 20 bytes");
      return concat(new Uint8Array(12), raw);
    }
    case "bytes32": {
      const raw = hexToBytes(String(value));
      if (raw.length !== 32) throw new Error("bytes32 must be 32 bytes");
      return raw;
    }
    case "string":
      return keccak_256(enc.encode(String(value)));
  }
  throw new Error(`unsupported EIP-712 type ${type}`);
}

export function typeHash(name: StructName): Uint8Array {
  return keccak_256(enc.encode(ENCODE_TYPE[name]));
}

export function hashStruct(name: StructName, value: StructValue): Uint8Array {
  const parts = EIP712_TYPES[name].map((f) => {
    if (!(f.name in value)) throw new Error(`${name}.${f.name} missing`);
    return word(f.type, value[f.name]);
  });
  return keccak_256(concat(typeHash(name), ...parts));
}

export function domainSeparator(d: Domain): Uint8Array {
  return keccak_256(
    concat(
      DOMAIN_TYPEHASH,
      keccak_256(enc.encode(EIP712_DOMAIN.name)),
      keccak_256(enc.encode(EIP712_DOMAIN.version)),
      word("uint256", BigInt(d.chainId)),
      word("address", d.verifyingContract),
    ),
  );
}

/** keccak256(0x1901 || domainSeparator || hashStruct): the value that is signed. */
export function digest(d: Domain, name: StructName, value: StructValue): Uint8Array {
  return keccak_256(concat(Uint8Array.of(0x19, 0x01), domainSeparator(d), hashStruct(name, value)));
}
