#include "device_key.h"
#include <psa/crypto.h>

#include <assert.h>
#include <string.h>

static bool exists[0x5406];
static psa_key_id_t removed, signed_by;

psa_status_t psa_export_public_key(psa_key_id_t id, uint8_t *key, size_t size, size_t *length)
{
	assert(id < sizeof(exists) / sizeof(exists[0]));
	if (!exists[id]) return PSA_ERROR_DOES_NOT_EXIST;
	assert(size == 65);
	memset(key, (int)(id & 0xff), size);
	key[0] = 4;
	*length = size;
	return 0;
}

psa_status_t psa_generate_key(const psa_key_attributes_t *attributes, psa_key_id_t *id)
{
	*id = attributes->id;
	assert(*id < sizeof(exists) / sizeof(exists[0]));
	assert(!exists[*id]);
	exists[*id] = true;
	return 0;
}

psa_status_t psa_destroy_key(psa_key_id_t id)
{
	assert(id == 0x5403);
	removed = id;
	if (!exists[id]) return PSA_ERROR_DOES_NOT_EXIST;
	exists[id] = false;
	return 0;
}

psa_status_t psa_sign_hash(psa_key_id_t id, int algorithm, const uint8_t *digest, size_t digest_size,
			  uint8_t *signature, size_t signature_size, size_t *length)
{
	assert(algorithm == PSA_ALG_SHA_256 && digest && digest_size == 32 && signature && signature_size == 64);
	assert(id < sizeof(exists) / sizeof(exists[0]) && exists[id]);
	signed_by = id;
	*length = 64;
	return 0;
}

int main(void)
{
	uint8_t legacy[20], rental[20], personal[20], identity[20], after[20], digest[32] = {0}, signature[64];
	exists[0x5401] = true; /* deployed key, owner unknown */
	assert(device_key_activate_rental(DEVICE_KEY_LEGACY_RENTER, false, legacy) == 0);
	assert(device_key_sign(digest, signature) == 0 && signed_by == 0x5401);
	device_key_deactivate_rental();
	assert(device_key_sign(digest, signature) == PSA_ERROR_NOT_PERMITTED);
	assert(device_key_slot_open(DEVICE_KEY_PERSONAL, true, personal) == 0);
	assert(device_key_slot_open(DEVICE_KEY_IDENTITY, true, identity) == 0);
	assert(device_key_slot_open(DEVICE_KEY_RENTER, true, rental) == 0);
	assert(device_key_sign(digest, signature) == PSA_ERROR_NOT_PERMITTED);
	assert(memcmp(legacy, rental, 20) && memcmp(personal, rental, 20) && memcmp(identity, personal, 20));
	assert(device_key_activate_rental(DEVICE_KEY_PERSONAL, false, after) == PSA_ERROR_NOT_PERMITTED);
	assert(device_key_activate_rental(DEVICE_KEY_RENTER, false, after) == 0);
	assert(device_key_sign(digest, signature) == 0 && signed_by == 0x5403);
	assert(device_key_retire_new_rental() == 0 && removed == 0x5403);
	assert(device_key_sign(digest, signature) == PSA_ERROR_NOT_PERMITTED);
	assert(device_key_slot_open(DEVICE_KEY_PERSONAL, true, after) == 0 && !memcmp(personal, after, 20));
	assert(device_key_slot_open(DEVICE_KEY_IDENTITY, true, after) == 0 && !memcmp(identity, after, 20));
	assert(device_key_slot_open(DEVICE_KEY_LEGACY_RENTER, false, after) == 0 && !memcmp(legacy, after, 20));
	return 0;
}
