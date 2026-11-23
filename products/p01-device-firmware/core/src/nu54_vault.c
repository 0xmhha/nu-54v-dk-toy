#include "nu54_vault.h"

#include <string.h>

#define VAULT_VERSION UINT32_C(0x4e563201)
#define ENROLLMENT_WITNESS UINT32_C(0x4e564231)

struct request_record {
	uint64_t generation;
	uint8_t id[16], intent[32], result[65];
	uint8_t committed;
};

_Static_assert(sizeof(nu54_vault_record) <= 128, "owner record exceeds ITS limit");
_Static_assert(sizeof(struct request_record) <= 128, "request record exceeds ITS limit");

static bool nonzero(const uint8_t *bytes, size_t length)
{
	for (size_t i = 0; i < length; ++i) {
		if (bytes[i]) return true;
	}
	return false;
}

static int save(nu54_vault *vault, const nu54_vault_record *next)
{
	if (vault->storage.write(vault->storage.context, 0, next, sizeof(*next)) != 0) {
		/* Write acknowledgement can be lost after commit. Only a new read can prove which epoch is active. */
		vault->enabled = false;
		vault->pending_record = 0;
		return NU54_VAULT_IO;
	}
	vault->record = *next;
	return 0;
}

int nu54_vault_open(nu54_vault *vault, const nu54_vault_storage *storage)
{
	nu54_vault_storage io = *storage;
	memset(vault, 0, sizeof(*vault));
	vault->storage = io;
	nu54_vault_record record = {0};
	int read = io.read(io.context, 0, &record, sizeof(record));
	uint32_t witness;
	int witness_read = io.read(io.context, NU54_VAULT_ENROLLMENT_RECORD, &witness, sizeof(witness));
	if (witness_read != NU54_VAULT_ABSENT && (witness_read != 0 || witness != ENROLLMENT_WITNESS)) return NU54_VAULT_IO;
	uint8_t actual[20];
	if (read == NU54_VAULT_ABSENT) {
		/* An interrupted first boot may leave an identity key. A personal key without its
		 * owner record is recovery state, so a fresh binding cannot adopt it. */
		if (witness_read != NU54_VAULT_ABSENT || io.key(io.context, true, false, actual) != NU54_VAULT_ABSENT)
			return NU54_VAULT_IO;
		for (unsigned index = 1; index <= NU54_VAULT_REQUESTS; ++index) {
			struct request_record prior;
			if (io.read(io.context, index, &prior, sizeof(prior)) != NU54_VAULT_ABSENT) return NU54_VAULT_IO;
		}
		record.version = VAULT_VERSION;
		if (io.key(io.context, false, true, record.identity) != 0 || save(vault, &record) != 0) return NU54_VAULT_IO;
	} else if (read != 0 || record.version != VAULT_VERSION || (record.flags & ~(NU54_VAULT_PERSONAL | NU54_VAULT_BACKUP | NU54_VAULT_REVOKED | NU54_VAULT_RELEASED)) ||
		   ((record.flags & NU54_VAULT_RELEASED) &&
		    (!(record.flags & NU54_VAULT_REVOKED) || (record.flags & NU54_VAULT_PERSONAL))) ||
		   ((record.flags & NU54_VAULT_BACKUP) && !(record.flags & NU54_VAULT_PERSONAL)) ||
		   ((record.flags & NU54_VAULT_PERSONAL) && (record.root_format != NU54_VAULT_ROOT_PSA_SECP256K1 ||
		    record.root_profile_version != NU54_VAULT_ROOT_PROFILE_V1)) ||
		   (!(record.flags & NU54_VAULT_PERSONAL) && (record.root_format || record.root_profile_version)) ||
		   (record.generation == 0 && record.flags) ||
		   (record.generation != 0 && (witness_read != 0 ||
		    (!!(record.flags & NU54_VAULT_REVOKED) == nonzero(record.owner, sizeof(record.owner))))) ||
		   io.key(io.context, false, false, actual) != 0 || memcmp(record.identity, actual, sizeof(actual)) ||
		   ((record.flags & NU54_VAULT_PERSONAL) && (io.key(io.context, true, false, actual) != 0 ||
		    memcmp(record.personal, actual, sizeof(actual))))) {
		return NU54_VAULT_IO;
	}
	vault->record = record;
	vault->enabled = true;
	return 0;
}

int nu54_vault_rebind(nu54_vault *vault, uint64_t expected_generation, const uint8_t owner[32])
{
	if (!vault->enabled || vault->record.generation != expected_generation ||
	    expected_generation == UINT64_MAX || !nonzero(owner, 32)) return NU54_VAULT_DENIED;
	nu54_vault_record next = vault->record;
	if (!expected_generation) {
		uint32_t witness = ENROLLMENT_WITNESS;
		if (vault->storage.write(vault->storage.context, NU54_VAULT_ENROLLMENT_RECORD, &witness, sizeof(witness)) != 0)
			return NU54_VAULT_IO;
	}
	next.generation++;
	next.flags &= ~(NU54_VAULT_REVOKED | NU54_VAULT_RELEASED);
	memcpy(next.owner, owner, sizeof(next.owner));
	vault->pending_record = 0;
	return save(vault, &next);
}

static bool authorized(const nu54_vault *vault, uint64_t generation, const uint8_t owner[32])
{
	return vault->enabled && !(vault->record.flags & NU54_VAULT_REVOKED) && generation && generation == vault->record.generation &&
	       memcmp(vault->record.owner, owner, sizeof(vault->record.owner)) == 0;
}

int nu54_vault_revoke(nu54_vault *vault, uint64_t generation, const uint8_t owner[32])
{
	if (!authorized(vault, generation, owner) || generation == UINT64_MAX) return NU54_VAULT_DENIED;
	nu54_vault_record next = vault->record;
	next.generation++;
	next.flags |= NU54_VAULT_REVOKED;
	memset(next.owner, 0, sizeof(next.owner));
	vault->pending_record = 0;
	return save(vault, &next);
}

int nu54_vault_release(nu54_vault *vault)
{
	if (!vault->enabled || (vault->record.flags & NU54_VAULT_PERSONAL) || vault->record.generation == UINT64_MAX)
		return NU54_VAULT_DENIED;
	if (!vault->record.generation || (vault->record.flags & NU54_VAULT_RELEASED)) return 0;
	nu54_vault_record next = vault->record;
	/* The generation only grows, so a grant or a phone of the previous owner stays outdated. */
	next.generation++;
	next.flags |= NU54_VAULT_REVOKED | NU54_VAULT_RELEASED;
	memset(next.owner, 0, sizeof(next.owner));
	vault->pending_record = 0;
	return save(vault, &next);
}

int nu54_vault_initialize_personal(nu54_vault *vault, uint64_t generation, const uint8_t owner[32])
{
	if (!authorized(vault, generation, owner)) return NU54_VAULT_DENIED;
	if (vault->record.flags & NU54_VAULT_PERSONAL) return 0;
	nu54_vault_record next = vault->record;
	if (vault->storage.key(vault->storage.context, true, true, next.personal) != 0) return NU54_VAULT_IO;
	next.flags |= NU54_VAULT_PERSONAL;
	next.root_format = NU54_VAULT_ROOT_PSA_SECP256K1;
	next.root_profile_version = NU54_VAULT_ROOT_PROFILE_V1;
	return save(vault, &next);
}

int nu54_vault_personal_address(const nu54_vault *vault, uint64_t generation, const uint8_t owner[32], uint8_t address[20])
{
	if (!authorized(vault, generation, owner) || !(vault->record.flags & NU54_VAULT_BACKUP)) return NU54_VAULT_DENIED;
	memcpy(address, vault->record.personal, sizeof(vault->record.personal));
	return 0;
}

int nu54_vault_reserve(nu54_vault *vault, uint64_t generation, const uint8_t owner[32], const uint8_t id[16],
		      const uint8_t intent[32], uint8_t cached[65])
{
	if (!authorized(vault, generation, owner) || !(vault->record.flags & NU54_VAULT_BACKUP) ||
	    !nonzero(id, 16) || !nonzero(intent, 32)) return NU54_VAULT_DENIED;
	unsigned free_record = 0;
	for (unsigned index = 1; index <= NU54_VAULT_REQUESTS; ++index) {
		struct request_record existing;
		int read = vault->storage.read(vault->storage.context, index, &existing, sizeof(existing));
		if (read == NU54_VAULT_ABSENT || (read == 0 && existing.generation < generation)) {
			if (!free_record) free_record = index;
			continue;
		}
		if (read != 0 || existing.generation > generation || existing.committed > 1) return NU54_VAULT_IO;
		if (memcmp(id, existing.id, sizeof(existing.id))) continue;
		if (memcmp(intent, existing.intent, sizeof(existing.intent))) return NU54_VAULT_CONFLICT;
		if (!existing.committed) return NU54_VAULT_UNKNOWN_RESULT;
		memcpy(cached, existing.result, sizeof(existing.result));
		return NU54_VAULT_RECORDED;
	}
	if (vault->pending_record || !free_record) return NU54_VAULT_DENIED;
	struct request_record next = {.generation = generation};
	memcpy(next.id, id, sizeof(next.id));
	memcpy(next.intent, intent, sizeof(next.intent));
	if (vault->storage.write(vault->storage.context, free_record, &next, sizeof(next)) != 0) return NU54_VAULT_IO;
	vault->pending_record = free_record;
	return NU54_VAULT_NEW;
}

int nu54_vault_commit_result(nu54_vault *vault, uint64_t generation, const uint8_t owner[32], const uint8_t id[16],
			    const uint8_t intent[32], const uint8_t result[65])
{
	if (!authorized(vault, generation, owner) || !vault->pending_record) return NU54_VAULT_DENIED;
	unsigned index = vault->pending_record;
	struct request_record record;
	if (vault->storage.read(vault->storage.context, index, &record, sizeof(record)) != 0 ||
	    record.generation != generation || record.committed ||
	    memcmp(id, record.id, sizeof(record.id)) || memcmp(intent, record.intent, sizeof(record.intent))) return NU54_VAULT_DENIED;
	record.committed = 1;
	memcpy(record.result, result, sizeof(record.result));
	vault->pending_record = 0;
	return vault->storage.write(vault->storage.context, index, &record, sizeof(record)) == 0 ? 0 : NU54_VAULT_IO;
}

void nu54_vault_cancel_pending(nu54_vault *vault)
{
	vault->pending_record = 0;
}
