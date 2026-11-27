#include "nu54_vault.h"

#include <assert.h>
#include <string.h>

struct disk {
	uint8_t data[1 + NU54_VAULT_ENROLLMENT_RECORD][128];
	size_t length[1 + NU54_VAULT_ENROLLMENT_RECORD];
	uint8_t keys[2][20];
	unsigned fail_write;
};

static int read_record(void *context, unsigned record, void *data, size_t length)
{
	struct disk *disk = context;
	if (disk->length[record] == 0) return NU54_VAULT_ABSENT;
	if (disk->length[record] != length) return NU54_VAULT_IO;
	memcpy(data, disk->data[record], length);
	return 0;
}

static int write_record(void *context, unsigned record, const void *data, size_t length)
{
	struct disk *disk = context;
	if (disk->fail_write == 1) return NU54_VAULT_IO;
	assert(length <= sizeof(disk->data[record]));
	memcpy(disk->data[record], data, length);
	disk->length[record] = length;
	return disk->fail_write == 2 ? NU54_VAULT_IO : 0;
}

static int key(void *context, bool personal, bool create, uint8_t address[20])
{
	struct disk *disk = context;
	if (!disk->keys[personal][0]) {
		if (!create) return NU54_VAULT_ABSENT;
		disk->keys[personal][0] = personal ? 42 : 43;
	}
	memcpy(address, disk->keys[personal], 20);
	return 0;
}

int main(void)
{
	struct disk disk = {0};
	nu54_vault vault;
	nu54_vault_storage io = {&disk, read_record, write_record, key};
	uint8_t owner[32] = {1}, replacement[32] = {2}, id[16] = {3}, intent[32] = {4};
	uint8_t address[20], personal[20], result[65] = {5}, cached[65];

	assert(nu54_vault_open(&vault, &io) == 0);
	assert(vault.record.identity[0] == 43 && vault.record.generation == 0);
	assert(nu54_vault_rebind(&vault, 0, owner) == 0);
	disk.fail_write = 1;
	assert(nu54_vault_initialize_personal(&vault, 1, owner) == NU54_VAULT_IO);
	assert(nu54_vault_personal_address(&vault, 1, owner, address) == NU54_VAULT_DENIED);
	disk.fail_write = 0;
	assert(nu54_vault_open(&vault, &io) == 0);
	assert(nu54_vault_initialize_personal(&vault, 1, owner) == 0);
	assert(vault.record.root_format == NU54_VAULT_ROOT_PSA_SECP256K1);
	assert(vault.record.root_profile_version == NU54_VAULT_ROOT_PROFILE_V1);
	assert(nu54_vault_personal_address(&vault, 1, owner, address) == NU54_VAULT_DENIED);
	memcpy(personal, disk.keys[true], 20);
	/* An injected verified record exercises request journaling independently of the
	 * single-key application profile, which has no API to assert a backup. */
	vault.record.flags |= NU54_VAULT_BACKUP;
	assert(write_record(&disk, 0, &vault.record, sizeof(vault.record)) == 0);
	assert(nu54_vault_personal_address(&vault, 1, owner, address) == 0);
	assert(memcmp(address, personal, 20) == 0);
	assert(nu54_vault_reserve(&vault, 1, owner, id, intent, cached) == NU54_VAULT_NEW);
	assert(nu54_vault_open(&vault, &io) == 0);
	assert(nu54_vault_reserve(&vault, 1, owner, id, intent, cached) == NU54_VAULT_UNKNOWN_RESULT);
	assert(nu54_vault_commit_result(&vault, 1, owner, id, intent, result) == NU54_VAULT_DENIED);
	uint8_t other_id[16] = {9};
	assert(nu54_vault_reserve(&vault, 1, owner, other_id, intent, cached) == NU54_VAULT_NEW);
	assert(nu54_vault_commit_result(&vault, 1, owner, other_id, intent, result) == 0);
	assert(nu54_vault_reserve(&vault, 1, owner, other_id, intent, cached) == NU54_VAULT_RECORDED);
	assert(memcmp(cached, result, sizeof(result)) == 0);
	intent[0]++;
	assert(nu54_vault_reserve(&vault, 1, owner, other_id, intent, cached) == NU54_VAULT_CONFLICT);

	disk.fail_write = 1;
	assert(nu54_vault_rebind(&vault, 1, replacement) == NU54_VAULT_IO);
	disk.fail_write = 0;
	assert(nu54_vault_open(&vault, &io) == 0);
	assert(nu54_vault_personal_address(&vault, 1, owner, address) == 0);
	disk.fail_write = 2;
	assert(nu54_vault_rebind(&vault, 1, replacement) == NU54_VAULT_IO);
	assert(nu54_vault_personal_address(&vault, 1, owner, address) == NU54_VAULT_DENIED);
	disk.fail_write = 0;
	assert(nu54_vault_open(&vault, &io) == 0);
	assert(nu54_vault_personal_address(&vault, 1, owner, address) == NU54_VAULT_DENIED);
	assert(nu54_vault_personal_address(&vault, 2, replacement, address) == 0);
	assert(memcmp(address, personal, 20) == 0);
	assert(nu54_vault_reserve(&vault, 2, replacement, id, intent, cached) == NU54_VAULT_NEW);
	nu54_vault_cancel_pending(&vault);
	assert(nu54_vault_reserve(&vault, 2, replacement, id, intent, cached) == NU54_VAULT_UNKNOWN_RESULT);
	assert(nu54_vault_commit_result(&vault, 2, replacement, id, intent, result) == NU54_VAULT_DENIED);
	assert(nu54_vault_rebind(&vault, 2, replacement) == 0);
	assert(nu54_vault_personal_address(&vault, 2, replacement, address) == NU54_VAULT_DENIED);
	assert(nu54_vault_personal_address(&vault, 3, replacement, address) == 0);
	assert(memcmp(address, personal, 20) == 0);
	assert(nu54_vault_release(&vault) == NU54_VAULT_DENIED); /* never drops a personal key's owner */
	assert(vault.record.generation == 3 && !(vault.record.flags & NU54_VAULT_RELEASED));
	nu54_vault_record kept = vault.record, odd = vault.record;
	odd.flags = NU54_VAULT_RELEASED; /* released without being ownerless */
	memset(odd.personal, 0, sizeof(odd.personal));
	odd.root_format = odd.root_profile_version = 0;
	assert(write_record(&disk, 0, &odd, sizeof(odd)) == 0 && nu54_vault_open(&vault, &io) == NU54_VAULT_IO);
	odd = kept;
	odd.flags |= NU54_VAULT_RELEASED | NU54_VAULT_REVOKED; /* released but personal */
	memset(odd.owner, 0, sizeof(odd.owner));
	assert(write_record(&disk, 0, &odd, sizeof(odd)) == 0 && nu54_vault_open(&vault, &io) == NU54_VAULT_IO);
	odd.flags = NU54_VAULT_RELEASED | NU54_VAULT_REVOKED;
	memset(odd.personal, 0, sizeof(odd.personal));
	odd.root_format = odd.root_profile_version = 0;
	assert(write_record(&disk, 0, &odd, sizeof(odd)) == 0 && nu54_vault_open(&vault, &io) == 0);
	assert(write_record(&disk, 0, &kept, sizeof(kept)) == 0 && nu54_vault_open(&vault, &io) == 0);
	disk.length[0] = 0;
	assert(nu54_vault_open(&vault, &io) == NU54_VAULT_IO);
	assert(nu54_vault_rebind(&vault, 0, owner) == NU54_VAULT_DENIED);
	assert(memcmp(disk.keys[true], personal, 20) == 0 && disk.keys[false][0] == 43);
	memset(disk.keys[true], 0, 20); /* previous enrollment evidence survives absent keys */
	assert(nu54_vault_open(&vault, &io) == NU54_VAULT_IO);
	return 0;
}
