#ifndef NU54_RECOVERY_H
#define NU54_RECOVERY_H

#include "nu54_hd.h"

#include <stdbool.h>

#define NU54_RECOVERY_CLAIM_SIZE 161
#define NU54_RECOVERY_BODY_SIZE 318
#define NU54_RECOVERY_DER_MAX 72
#define NU54_RECOVERY_ENVELOPE_MAX (NU54_RECOVERY_BODY_SIZE + 3 * (1 + NU54_RECOVERY_DER_MAX))
#define NU54_HD_ADDRESS_SIZE 20
#define NU54_HD_PATH_FIRST_EVM 1
#define NU54_HD_CHAIN_EVM_PILOT UINT64_C(8283)

enum nu54_recovery_role {NU54_RECOVERY_SOURCE, NU54_RECOVERY_TARGET, NU54_RECOVERY_ACCESSORY};

typedef struct {
	uint8_t challenge[32], request[16], target[32], owner[32], epoch[16];
	uint64_t generation, expires;
} nu54_recovery_context;

typedef struct {
	int (*sign)(void *, enum nu54_recovery_role, const uint8_t *, size_t, uint8_t *, size_t *);
	int (*verify)(void *, enum nu54_recovery_role, const uint8_t *, size_t, const uint8_t *, size_t);
	void *context;
} nu54_recovery_signers;

void nu54_recovery_claim(uint8_t [NU54_RECOVERY_CLAIM_SIZE], const uint8_t source[32], uint64_t generation,
			 const uint8_t key[NU54_HD_PUBLIC_SIZE], const uint8_t address[NU54_HD_ADDRESS_SIZE]);
int nu54_recovery_prepare(uint8_t [NU54_RECOVERY_BODY_SIZE], const uint8_t [NU54_RECOVERY_CLAIM_SIZE],
			 const nu54_recovery_context *, uint64_t now);
int nu54_recovery_verify(const uint8_t *, size_t, const nu54_recovery_context *, uint64_t now,
			 const nu54_recovery_signers *);

#endif
