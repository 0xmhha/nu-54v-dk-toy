#ifndef NU54_SPENDING_FAKE_CRYPTO_H
#define NU54_SPENDING_FAKE_CRYPTO_H
#include <stddef.h>
#include <stdint.h>
typedef int psa_status_t;
typedef uint32_t psa_key_id_t, psa_algorithm_t, psa_key_type_t, psa_key_usage_t, psa_key_lifetime_t;
typedef struct {
	psa_key_id_t id;
	psa_key_lifetime_t lifetime;
	psa_key_type_t type;
	size_t bits;
	psa_key_usage_t usage;
	psa_algorithm_t algorithm;
} psa_key_attributes_t;
#define PSA_KEY_ATTRIBUTES_INIT {0}
#define PSA_SUCCESS 0
#define PSA_ERROR_GENERIC_ERROR (-132)
#define PSA_ERROR_ALREADY_EXISTS (-139)
#define PSA_ERROR_DOES_NOT_EXIST (-140)
#define PSA_ERROR_INVALID_HANDLE (-136)
#define PSA_ALG_SHA_256 1u
#define PSA_ALG_HMAC(hash) (2u | (hash))
#define PSA_ALG_DETERMINISTIC_ECDSA(hash) (4u | (hash))
#define PSA_ECC_FAMILY_SECP_K1 1u
#define PSA_KEY_TYPE_ECC_KEY_PAIR(family) (16u | (family))
#define PSA_KEY_TYPE_HMAC 32u
#define PSA_KEY_LIFETIME_PERSISTENT 1u
#define PSA_KEY_USAGE_EXPORT 1u
#define PSA_KEY_USAGE_COPY 2u
#define PSA_KEY_USAGE_SIGN_HASH 4u
#define PSA_KEY_USAGE_SIGN_MESSAGE 8u
#define PSA_KEY_USAGE_VERIFY_MESSAGE 16u
#define psa_set_key_id(a, v) ((a)->id = (v))
#define psa_set_key_lifetime(a, v) ((a)->lifetime = (v))
#define psa_set_key_type(a, v) ((a)->type = (v))
#define psa_set_key_bits(a, v) ((a)->bits = (v))
#define psa_set_key_usage_flags(a, v) ((a)->usage = (v))
#define psa_set_key_algorithm(a, v) ((a)->algorithm = (v))
#define psa_get_key_lifetime(a) ((a)->lifetime)
#define psa_get_key_type(a) ((a)->type)
#define psa_get_key_bits(a) ((a)->bits)
#define psa_get_key_usage_flags(a) ((a)->usage)
#define psa_get_key_algorithm(a) ((a)->algorithm)
#define psa_reset_key_attributes(a) ((void)(a))
psa_status_t psa_get_key_attributes(psa_key_id_t, psa_key_attributes_t *);
psa_status_t psa_generate_key(const psa_key_attributes_t *, psa_key_id_t *);
psa_status_t psa_destroy_key(psa_key_id_t);
psa_status_t psa_export_public_key(psa_key_id_t, uint8_t *, size_t, size_t *);
psa_status_t psa_sign_hash(psa_key_id_t, psa_algorithm_t, const uint8_t *, size_t, uint8_t *, size_t, size_t *);
psa_status_t psa_mac_compute(psa_key_id_t, psa_algorithm_t, const uint8_t *, size_t, uint8_t *, size_t, size_t *);
psa_status_t psa_mac_verify(psa_key_id_t, psa_algorithm_t, const uint8_t *, size_t, const uint8_t *, size_t);
#endif
