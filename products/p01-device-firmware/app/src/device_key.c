#include "device_key.h"

#include <psa/crypto.h>
#include <zephyr/logging/log.h>

#include "nu54_sig.h"

#define SIGN_ALG PSA_ALG_DETERMINISTIC_ECDSA(PSA_ALG_SHA_256)

LOG_MODULE_REGISTER(device_key, LOG_LEVEL_INF);

/* 0x5401 predates slot classification. Keep it even after a rental is retired. */
static const psa_key_id_t key_ids[] = {0x5401, 0x5403, 0x5404, 0x5405};
static psa_key_id_t active_rental;

static int missing(psa_status_t st)
{
	return st == PSA_ERROR_INVALID_HANDLE || st == PSA_ERROR_DOES_NOT_EXIST;
}

static psa_key_id_t key_id(device_key_slot slot)
{
	return slot >= DEVICE_KEY_LEGACY_RENTER && slot <= DEVICE_KEY_IDENTITY ? key_ids[slot] : 0;
}

static int open_key(psa_key_id_t id, uint8_t address[20])
{
	uint8_t pub[65];
	size_t pub_len = 0;
	psa_status_t st = psa_export_public_key(id, pub, sizeof(pub), &pub_len);

	LOG_DBG("export public key: %d (%u bytes)", st, (unsigned)pub_len);
	if (st != PSA_SUCCESS || pub_len != 65) {
		return st != PSA_SUCCESS ? st : PSA_ERROR_GENERIC_ERROR;
	}
	nu54_address_of_pubkey(pub, address);
	return 0;
}

int device_key_slot_open(device_key_slot slot, bool create, uint8_t address[20])
{
	psa_key_id_t requested = key_id(slot);
	if (!requested || !address) {
		return PSA_ERROR_INVALID_ARGUMENT;
	}
	int opened = open_key(requested, address);
	if (!create || !missing(opened)) {
		return opened;
	}
	psa_key_attributes_t attr = PSA_KEY_ATTRIBUTES_INIT;
	psa_key_id_t id;
	psa_set_key_id(&attr, requested);
	psa_set_key_lifetime(&attr, PSA_KEY_LIFETIME_PERSISTENT);
	psa_set_key_type(&attr, PSA_KEY_TYPE_ECC_KEY_PAIR(PSA_ECC_FAMILY_SECP_K1));
	psa_set_key_bits(&attr, 256);
	psa_set_key_usage_flags(&attr, PSA_KEY_USAGE_SIGN_HASH); /* not exportable */
	psa_set_key_algorithm(&attr, SIGN_ALG);
	psa_status_t st = psa_generate_key(&attr, &id);
	psa_reset_key_attributes(&attr);
	LOG_INF("generate: %d", st);
	return st != PSA_SUCCESS ? st : open_key(requested, address);
}

int device_key_activate_rental(device_key_slot slot, bool create, uint8_t address[20])
{
	if (slot != DEVICE_KEY_LEGACY_RENTER && slot != DEVICE_KEY_RENTER) {
		return PSA_ERROR_NOT_PERMITTED;
	}
	active_rental = 0;
	int result = device_key_slot_open(slot, create, address);
	if (result == 0) {
		active_rental = key_id(slot);
	}
	return result;
}

void device_key_deactivate_rental(void)
{
	active_rental = 0;
}

int device_key_retire_new_rental(void)
{
	psa_key_id_t rental = key_id(DEVICE_KEY_RENTER);
	if (active_rental == rental) active_rental = 0;
	psa_status_t st = psa_destroy_key(rental);
	return st == PSA_SUCCESS || missing(st) ? 0 : st;
}

static int sign_with(psa_key_id_t id, const uint8_t digest[32], uint8_t rs[64])
{
	if (!id) return PSA_ERROR_NOT_PERMITTED;
	size_t len;
	psa_status_t st = psa_sign_hash(id, SIGN_ALG, digest, 32, rs, 64, &len);
	return st == PSA_SUCCESS && len == 64 ? 0 : (st != PSA_SUCCESS ? st : PSA_ERROR_GENERIC_ERROR);
}

int device_key_sign(const uint8_t digest[32], uint8_t rs[64])
{
	return sign_with(active_rental, digest, rs);
}

int device_key_slot_sign(device_key_slot slot, const uint8_t digest[32], uint8_t rs[64])
{
	if (slot != DEVICE_KEY_PERSONAL && slot != DEVICE_KEY_IDENTITY) return PSA_ERROR_NOT_PERMITTED;
	return sign_with(key_id(slot), digest, rs);
}
