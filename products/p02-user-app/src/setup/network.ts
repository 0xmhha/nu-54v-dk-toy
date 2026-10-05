// The deployment the renter app sets devices up for (payment-protocol.md 5, setup.operator).
// Values are the StableNet testnet deployment record
// (products/p06-stablenet-contracts/deployments/8283.json); test/setup.test.ts checks they match.

export const NETWORK = {
  chainId: 8283n,
  /** Operator role: the key that signs merchant attestations and TimeAnchors for this device. */
  operator: "0x246aE7E5b14f096342B96A65E375524Da80c101B",
  /** PaymentSettlement: the EIP-712 verifying contract the device signs for. */
  contract: "0xDe7596556D35Fa62F238F074A0f0E59cF730caA4",
} as const;
