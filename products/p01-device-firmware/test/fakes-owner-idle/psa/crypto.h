#ifndef NU54_OWNER_IDLE_FAKE_CRYPTO_H
#define NU54_OWNER_IDLE_FAKE_CRYPTO_H
#include "../../fakes/psa/crypto.h"

#define PSA_ALG_HMAC(alg) (alg)
#define PSA_KEY_TYPE_HMAC 1
#define PSA_KEY_USAGE_SIGN_MESSAGE 1
#define PSA_KEY_USAGE_VERIFY_MESSAGE 2
#define PSA_ERROR_ALREADY_EXISTS -6
int psa_mac_compute(psa_key_id_t, int, const uint8_t *, size_t, uint8_t *, size_t, size_t *);
int psa_mac_verify(psa_key_id_t, int, const uint8_t *, size_t, const uint8_t *, size_t);
int psa_get_key_attributes(psa_key_id_t, psa_key_attributes_t *);
static inline int psa_get_key_type(const psa_key_attributes_t *a) {(void)a; return PSA_KEY_TYPE_HMAC;}
static inline size_t psa_get_key_bits(const psa_key_attributes_t *a) {(void)a; return 256;}
static inline int psa_get_key_algorithm(const psa_key_attributes_t *a) {(void)a; return PSA_ALG_SHA_256;}
static inline int psa_get_key_usage_flags(const psa_key_attributes_t *a) {(void)a; return 3;}
#endif
