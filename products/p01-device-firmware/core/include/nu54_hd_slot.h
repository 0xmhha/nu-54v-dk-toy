#ifndef NU54_HD_SLOT_H
#define NU54_HD_SLOT_H

#include "nu54_recovery.h"

#define NU54_HD_RECORD_VERSION UINT32_C(0x4e484401)

enum nu54_hd_phase {
	NU54_HD_EMPTY, NU54_HD_CREATING, NU54_HD_BACKUP_REQUIRED, NU54_HD_RESTORING, NU54_HD_READY
};

typedef struct {
	uint32_t version;
	uint8_t phase, reserved[3], origin[32], public_key[NU54_HD_PUBLIC_SIZE], address[20];
} nu54_hd_record;

typedef struct {
	/* load/present return 1 for missing, 0 for success, -1 for unreadable. */
	int (*load)(void *, nu54_hd_record *);
	int (*save)(void *, const nu54_hd_record *);
	int (*present)(void *);
	int (*generate)(void *);
	/* Import consumes only the capsule already verified by the private provider. */
	int (*preview)(void *, const uint8_t *, size_t, uint8_t [NU54_HD_PUBLIC_SIZE], uint8_t [20]);
	int (*import)(void *, const uint8_t *, size_t);
	int (*derive)(void *, uint8_t [NU54_HD_PUBLIC_SIZE], uint8_t [20]);
	void *context;
} nu54_hd_backend;

typedef struct {
	nu54_hd_backend backend;
	nu54_hd_record record;
	bool enabled;
} nu54_hd_slot;

int nu54_hd_open(nu54_hd_slot *, const nu54_hd_backend *);
bool nu54_hd_backup_eligible(const nu54_hd_slot *, enum nu54_recovery_role recovery_role);
int nu54_hd_create(nu54_hd_slot *, const uint8_t identity[32]);
int nu54_hd_restore(nu54_hd_slot *, const uint8_t *capsule, size_t capsule_length,
		    const uint8_t *receipt, size_t receipt_length, const nu54_recovery_context *,
		    uint64_t now, const nu54_recovery_signers *);
int nu54_hd_mark_backup(nu54_hd_slot *, const uint8_t *receipt, size_t receipt_length,
			const nu54_recovery_context *, uint64_t now, const nu54_recovery_signers *);
int nu54_hd_address(const nu54_hd_slot *, uint8_t [20]);

#endif
