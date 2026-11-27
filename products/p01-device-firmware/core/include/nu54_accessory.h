#ifndef NU54_ACCESSORY_H
#define NU54_ACCESSORY_H

#include "nu54_hd_slot.h"

/* Private frames inside the separately pinned accessory TLS stream only. */
#define NU54_ACCESSORY_CAPSULE_MAX (1 + NU54_RECOVERY_CLAIM_SIZE + 1 + NU54_RECOVERY_DER_MAX + NU54_HD_SEED_SIZE)
#define NU54_RECOVERY_OPEN_SIZE 145

void nu54_recovery_open_message(const nu54_recovery_context *, uint8_t [NU54_RECOVERY_OPEN_SIZE]);
int nu54_recovery_parse_open(const uint8_t *, size_t, nu54_recovery_context *);
int nu54_accessory_target_proof(const nu54_hd_slot *, const uint8_t *capsule, size_t capsule_length,
				const nu54_recovery_context *, uint64_t now, const nu54_recovery_signers *,
				uint8_t envelope[NU54_RECOVERY_ENVELOPE_MAX], size_t *length);
int nu54_accessory_complete(const uint8_t *capsule, size_t capsule_length, const uint8_t *partial, size_t partial_length,
			    const nu54_recovery_context *, uint64_t now, const nu54_recovery_signers *,
			    uint8_t envelope[NU54_RECOVERY_ENVELOPE_MAX], size_t *length);

#endif
