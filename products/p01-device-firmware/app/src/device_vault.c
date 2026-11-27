#include "device_vault.h"

#include <psa/crypto.h>
#include <psa/internal_trusted_storage.h>

#include "device_key.h"

#define VAULT_UID ((psa_storage_uid_t)0x5420)

static nu54_vault device_vault;

static int read_record(void *context, unsigned record, void *data, size_t length)
{
	(void)context;
	if (record > NU54_VAULT_ENROLLMENT_RECORD) return NU54_VAULT_IO;
	struct psa_storage_info_t info;
	psa_status_t status = psa_its_get_info(VAULT_UID + record, &info);
	if (status == PSA_ERROR_DOES_NOT_EXIST) return NU54_VAULT_ABSENT;
	if (status != PSA_SUCCESS || info.size != length) return NU54_VAULT_IO;
	size_t received;
	status = psa_its_get(VAULT_UID + record, 0, length, data, &received);
	return status == PSA_SUCCESS && received == length ? 0 : NU54_VAULT_IO;
}

static int write_record(void *context, unsigned record, const void *data, size_t length)
{
	(void)context;
	if (record > NU54_VAULT_ENROLLMENT_RECORD) return NU54_VAULT_IO;
	return psa_its_set(VAULT_UID + record, length, data, PSA_STORAGE_FLAG_NONE) == PSA_SUCCESS ? 0 : NU54_VAULT_IO;
}

static int slot_key(void *context, bool personal, bool create, uint8_t address[20])
{
	(void)context;
	int status = device_key_slot_open(personal ? DEVICE_KEY_PERSONAL : DEVICE_KEY_IDENTITY, create, address);
	if (status == PSA_ERROR_DOES_NOT_EXIST || status == PSA_ERROR_INVALID_HANDLE) return NU54_VAULT_ABSENT;
	return status;
}

int device_vault_load(void)
{
	const nu54_vault_storage storage = {NULL, read_record, write_record, slot_key};
	return nu54_vault_open(&device_vault, &storage);
}

nu54_vault *device_vault_owner(void)
{
	return &device_vault;
}
