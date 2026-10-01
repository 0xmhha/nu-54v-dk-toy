#include "device_key.h"

#include <psa/crypto.h>
#include <zephyr/logging/log.h>

#include "nu54_sig.h"

/* One persistent key per device; reset (DeviceReset) destroys it. */
#define DEVICE_KEY_ID ((psa_key_id_t)0x00005401)
#define SIGN_ALG PSA_ALG_DETERMINISTIC_ECDSA(PSA_ALG_SHA_256)

LOG_MODULE_REGISTER(device_key, LOG_LEVEL_INF);

int device_key_init(uint8_t address[20], int *created)
{
	psa_status_t st = psa_crypto_init();
	psa_key_attributes_t attr = PSA_KEY_ATTRIBUTES_INIT;
	uint8_t pub[65];
	size_t pub_len;

	if (st != PSA_SUCCESS) {
		return st;
	}
	*created = 0;
	st = psa_get_key_attributes(DEVICE_KEY_ID, &attr);
	psa_reset_key_attributes(&attr);
	LOG_DBG("get attributes: %d", st);
	if (st == PSA_ERROR_INVALID_HANDLE || st == PSA_ERROR_DOES_NOT_EXIST) {
		psa_key_id_t id;
		psa_set_key_id(&attr, DEVICE_KEY_ID);
		psa_set_key_lifetime(&attr, PSA_KEY_LIFETIME_PERSISTENT);
		psa_set_key_type(&attr, PSA_KEY_TYPE_ECC_KEY_PAIR(PSA_ECC_FAMILY_SECP_K1));
		psa_set_key_bits(&attr, 256);
		psa_set_key_usage_flags(&attr, PSA_KEY_USAGE_SIGN_HASH); /* not exportable */
		psa_set_key_algorithm(&attr, SIGN_ALG);
		st = psa_generate_key(&attr, &id);
		psa_reset_key_attributes(&attr);
		LOG_INF("generate: %d", st);
		if (st != PSA_SUCCESS) {
			return st;
		}
		*created = 1;
	} else if (st != PSA_SUCCESS) {
		return st;
	}
	st = psa_export_public_key(DEVICE_KEY_ID, pub, sizeof(pub), &pub_len);
	LOG_DBG("export public key: %d (%u bytes)", st, (unsigned)pub_len);
	if (st != PSA_SUCCESS || pub_len != 65) {
		return st != PSA_SUCCESS ? st : PSA_ERROR_GENERIC_ERROR;
	}
	nu54_address_of_pubkey(pub, address);
	return 0;
}

int device_key_sign(const uint8_t digest[32], uint8_t rs[64])
{
	size_t len;
	psa_status_t st = psa_sign_hash(DEVICE_KEY_ID, SIGN_ALG, digest, 32, rs, 64, &len);
	return st == PSA_SUCCESS && len == 64 ? 0 : (st != PSA_SUCCESS ? st : PSA_ERROR_GENERIC_ERROR);
}
