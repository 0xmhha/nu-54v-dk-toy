#ifndef TEST_PSA_CRYPTO_H
#define TEST_PSA_CRYPTO_H

#include <stddef.h>
#include <stdint.h>

typedef uint32_t psa_key_id_t;
typedef int psa_status_t;
typedef struct { psa_key_id_t id; } psa_key_attributes_t;

#define PSA_KEY_ATTRIBUTES_INIT {0}
#define PSA_SUCCESS 0
#define PSA_ERROR_INVALID_HANDLE -1
#define PSA_ERROR_DOES_NOT_EXIST -2
#define PSA_ERROR_INVALID_ARGUMENT -3
#define PSA_ERROR_GENERIC_ERROR -4
#define PSA_ERROR_NOT_PERMITTED -5
#define PSA_KEY_LIFETIME_PERSISTENT 1
#define PSA_ECC_FAMILY_SECP_K1 1
#define PSA_KEY_USAGE_SIGN_HASH 1
#define PSA_ALG_SHA_256 1
#define PSA_KEY_TYPE_ECC_KEY_PAIR(family) (family)
#define PSA_ALG_DETERMINISTIC_ECDSA(algorithm) (algorithm)

static inline void psa_set_key_id(psa_key_attributes_t *attributes, psa_key_id_t id) { attributes->id = id; }
static inline void psa_set_key_lifetime(psa_key_attributes_t *a, int x) { (void)a; (void)x; }
static inline void psa_set_key_type(psa_key_attributes_t *a, int x) { (void)a; (void)x; }
static inline void psa_set_key_bits(psa_key_attributes_t *a, int x) { (void)a; (void)x; }
static inline void psa_set_key_usage_flags(psa_key_attributes_t *a, int x) { (void)a; (void)x; }
static inline void psa_set_key_algorithm(psa_key_attributes_t *a, int x) { (void)a; (void)x; }
static inline void psa_reset_key_attributes(psa_key_attributes_t *attributes) { attributes->id = 0; }

psa_status_t psa_export_public_key(psa_key_id_t id, uint8_t *key, size_t size, size_t *length);
psa_status_t psa_generate_random(uint8_t *output, size_t size);
psa_status_t psa_generate_key(const psa_key_attributes_t *attributes, psa_key_id_t *id);
psa_status_t psa_destroy_key(psa_key_id_t id);
psa_status_t psa_sign_hash(psa_key_id_t id, int algorithm, const uint8_t *digest, size_t digest_size,
			  uint8_t *signature, size_t signature_size, size_t *length);

#endif
