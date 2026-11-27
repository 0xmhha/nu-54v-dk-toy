#ifndef NU54_HD_H
#define NU54_HD_H

#include <stddef.h>
#include <stdint.h>

#define NU54_HD_SEED_SIZE 32
#define NU54_HD_PUBLIC_SIZE 65
#define NU54_HD_FORMAT 2
#define NU54_HD_PROFILE 1
#define NU54_HD_BIP32_MAX_DEPTH 6

typedef int (*nu54_hd_hmac)(const uint8_t *key, size_t key_length,
			    const uint8_t *message, size_t message_length, uint8_t digest[64]);

/* Recovery boundary only. Local variables carrying private material are scrubbed before return. */
int nu54_hd_first_evm(const uint8_t seed[NU54_HD_SEED_SIZE], const uint8_t blinding[32], nu54_hd_hmac hmac,
		      uint8_t public_key[NU54_HD_PUBLIC_SIZE], uint8_t address[20]);

/* Sign only with the exact backed-up first EVM account. The scalar stays in the derivation function. */
int nu54_hd_sign_first_evm(const uint8_t seed[NU54_HD_SEED_SIZE], const uint8_t blinding[32], nu54_hd_hmac hmac,
			   const uint8_t address[20], const uint8_t digest[32], uint8_t signature[64]);

/* A distinct hardware account for the isolated StableNet learning market. */
int nu54_hd_market_evm(const uint8_t [32], const uint8_t [32], nu54_hd_hmac, uint8_t [20]);
int nu54_hd_sign_market_evm(const uint8_t [32], const uint8_t [32], nu54_hd_hmac,
                           const uint8_t [20], const uint8_t [32], uint8_t [64]);

/* Trusted device code selects a fixed profile and derives the exact path.
 * Path elements are uint32 BIP32 indices, with bit 31 for hardened levels. */
int nu54_hd_bip32_public(const uint8_t [NU54_HD_SEED_SIZE], const uint8_t [32], nu54_hd_hmac,
			const uint32_t *, size_t, uint8_t [33], uint8_t *);
int nu54_hd_bip32_sign(const uint8_t [NU54_HD_SEED_SIZE], const uint8_t [32], nu54_hd_hmac,
		      const uint32_t *, size_t, const uint8_t [33], const uint8_t [32], uint8_t [64]);

#endif
