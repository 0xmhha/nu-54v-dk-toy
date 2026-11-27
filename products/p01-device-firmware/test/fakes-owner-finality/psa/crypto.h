#ifndef TEST_FINALITY_CRYPTO_H
#define TEST_FINALITY_CRYPTO_H
#include <stddef.h>
#include <stdint.h>
#include <psa/protected_storage.h>
typedef uint32_t psa_key_id_t;
typedef struct {int unused;} psa_key_attributes_t;
#define PSA_KEY_ATTRIBUTES_INIT {0}
#define PSA_ALG_SHA_256 1
#define PSA_ALG_ECDSA(x) (x)
#define PSA_ECC_FAMILY_SECP_R1 1
#define PSA_KEY_TYPE_ECC_PUBLIC_KEY(x) (x)
#define PSA_KEY_USAGE_VERIFY_HASH 1
static inline void psa_set_key_type(psa_key_attributes_t *a,int x){(void)a;(void)x;}
static inline void psa_set_key_bits(psa_key_attributes_t *a,int x){(void)a;(void)x;}
static inline void psa_set_key_usage_flags(psa_key_attributes_t *a,int x){(void)a;(void)x;}
static inline void psa_set_key_algorithm(psa_key_attributes_t *a,int x){(void)a;(void)x;}
static inline void psa_reset_key_attributes(psa_key_attributes_t *a){(void)a;}
psa_status_t psa_hash_compute(int,const uint8_t *,size_t,uint8_t *,size_t,size_t *);
psa_status_t psa_import_key(const psa_key_attributes_t *,const uint8_t *,size_t,psa_key_id_t *);
psa_status_t psa_verify_hash(psa_key_id_t,int,const uint8_t *,size_t,const uint8_t *,size_t);
psa_status_t psa_destroy_key(psa_key_id_t);
#endif
psa_status_t psa_generate_random(uint8_t *,size_t);
