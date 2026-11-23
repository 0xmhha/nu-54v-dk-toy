#ifndef NU54_VAULT_H
#define NU54_VAULT_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define NU54_VAULT_REQUESTS 4
#define NU54_VAULT_ENROLLMENT_RECORD (NU54_VAULT_REQUESTS + 1)
#define NU54_VAULT_ROOT_PSA_SECP256K1 1
#define NU54_VAULT_ROOT_PROFILE_V1 1
#define NU54_VAULT_PERSONAL UINT32_C(1)
#define NU54_VAULT_BACKUP UINT32_C(2)
#define NU54_VAULT_REVOKED UINT32_C(4)
/* A factory reset of a spending wallet: no owner, and a new one may register. */
#define NU54_VAULT_RELEASED UINT32_C(8)

enum {
	NU54_VAULT_IO = -1,
	NU54_VAULT_DENIED = -2,
	NU54_VAULT_CONFLICT = -3,
	NU54_VAULT_ABSENT = 1,
	NU54_VAULT_NEW = 2,
	NU54_VAULT_UNKNOWN_RESULT = 3,
	NU54_VAULT_RECORDED = 4,
};

typedef struct {
	void *context;
	int (*read)(void *context, unsigned record, void *data, size_t length);
	int (*write)(void *context, unsigned record, const void *data, size_t length);
	int (*key)(void *context, bool personal, bool create, uint8_t address[20]);
} nu54_vault_storage;

typedef struct {
	uint64_t generation;
	uint32_t version, flags;
	uint32_t root_format, root_profile_version;
	uint8_t identity[20], personal[20], owner[32];
} nu54_vault_record;

typedef struct {
	nu54_vault_storage storage;
	nu54_vault_record record;
	bool enabled;
	unsigned pending_record;
} nu54_vault;

int nu54_vault_open(nu54_vault *vault, const nu54_vault_storage *storage);
/* The platform calls rebind only after independent recovery/physical authorization. */
int nu54_vault_rebind(nu54_vault *vault, uint64_t expected_generation, const uint8_t owner[32]);
int nu54_vault_revoke(nu54_vault *vault, uint64_t generation, const uint8_t owner[32]);
/* Only after the local reset authorization. Repeating it changes nothing. */
int nu54_vault_release(nu54_vault *vault);
int nu54_vault_initialize_personal(nu54_vault *vault, uint64_t generation, const uint8_t owner[32]);
int nu54_vault_personal_address(const nu54_vault *vault, uint64_t generation, const uint8_t owner[32], uint8_t address[20]);
int nu54_vault_reserve(nu54_vault *vault, uint64_t generation, const uint8_t owner[32], const uint8_t id[16],
		      const uint8_t intent[32], uint8_t cached[65]);
int nu54_vault_commit_result(nu54_vault *vault, uint64_t generation, const uint8_t owner[32], const uint8_t id[16],
			    const uint8_t intent[32], const uint8_t result[65]);
void nu54_vault_cancel_pending(nu54_vault *vault);

#endif
