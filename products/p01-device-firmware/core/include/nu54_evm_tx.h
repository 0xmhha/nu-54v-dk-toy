#ifndef NU54_EVM_TX_H
#define NU54_EVM_TX_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define NU54_EVM_TX_MAX 256
#define NU54_EVM_SIGNED_MAX (NU54_EVM_TX_MAX + 68)

typedef struct {
	uint64_t chain, nonce, priority_fee, max_fee, gas_limit, l1_fee_bound, total_fee_bound;
	uint8_t token[20], recipient[20], amount[32], digest[32];
} nu54_evm_intent;

typedef struct {
	uint64_t chain, total_fee_limit, l1_fee_bound;
	bool revoke_allowance;
	uint8_t token[20]; /* Zero for the chain's native asset. */
	/* Both fields must have nonzero values to admit a positive allowance. */
	uint8_t allowance_spender[20], allowance_maximum[32];
} nu54_evm_policy;

/* EIP-1559 canonical unsigned type-2 wire. Native transfer, enrolled ERC-20 transfer, revocation, or bounded allowance. */
int nu54_evm_decode(const uint8_t *, size_t, const nu54_evm_policy *, nu54_evm_intent *);

/* Appends low-s compact RS and recovery parity to the same validated unsigned transaction. */
int nu54_evm_serialize(const uint8_t *, size_t, const nu54_evm_policy *,
			const uint8_t signature[65], uint8_t out[NU54_EVM_SIGNED_MAX], size_t *);

#endif
