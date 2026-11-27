#ifndef NU54_SOLANA_H
#define NU54_SOLANA_H

#include "nu54_hd.h"

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define NU54_SOL_MESSAGE_MAX 1167
#define NU54_SOL_TRANSACTION_MAX 1232

/* These facts must originate in the trusted network context. In particular
 * block height must be refreshed before finalization and the fee must bind
 * to this exact message, recent blockhash and network. */
typedef struct {
	uint8_t payer[32], blockhash[32], message_hash[32];
	uint64_t block_height, last_valid_block_height, fee, max_fee;
	bool allow_spl;
	uint8_t mint[32], decimals;
	int (*sha256)(const uint8_t *, size_t, uint8_t [32]);
} nu54_sol_policy;

typedef struct {
	uint8_t payer[32], source[32], recipient[32], mint[32], blockhash[32];
	uint8_t commitment[32], decimals;
	uint64_t amount, fee, last_valid_block_height;
	bool spl, static_v0;
} nu54_sol_intent;

typedef int (*nu54_sol_sign)(void *, const uint8_t [32], const uint8_t *, size_t, uint8_t [64]);

int nu54_sol_review(const uint8_t *, size_t, const nu54_sol_policy *, nu54_sol_intent *);
int nu54_sol_finalize(const uint8_t *, size_t, const nu54_sol_policy *, const uint8_t [32],
		      nu54_sol_sign, void *, uint8_t *, size_t, size_t *);

/* Trusted crypto uses the ephemeral 32-byte Ed25519 child seed without
 * exporting it. Exact SLIP-0010 path is m/44'/501'/account'/0'. */
typedef int (*nu54_sol_ed25519)(const uint8_t [32], void *);
int nu54_sol_child(const uint8_t [NU54_HD_SEED_SIZE], uint32_t, nu54_hd_hmac,
		   nu54_sol_ed25519, void *);

#endif
