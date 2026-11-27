#ifndef NU54_SIWE_H
#define NU54_SIWE_H

#include <stddef.h>
#include <stdint.h>

#define NU54_SIWE_MAX 600

/* Narrow EIP-4361 profile: HTTPS, ASCII, no optional statement/resources, bound
 * domain/address/chain, and a required short expiration. */
int nu54_siwe_digest(const uint8_t *, size_t, const uint8_t *, size_t, const uint8_t [20],
		      uint64_t chain, uint64_t now, uint64_t expiry, uint8_t [32]);

#endif
