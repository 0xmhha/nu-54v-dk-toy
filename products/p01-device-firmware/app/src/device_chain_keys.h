#ifndef DEVICE_CHAIN_KEYS_H
#define DEVICE_CHAIN_KEYS_H

#include <stddef.h>
#include <stdint.h>

enum { NU54_CHAIN_BTC = 1, NU54_CHAIN_SOL = 2, NU54_CHAIN_XRPL = 3 };

typedef struct { uint8_t chain, network; uint32_t account; } device_chain_account;

/* Available only to the owner management dispatcher. Signing callers must
 * enforce the independent display, local PIN and physical approval. */
int device_chain_public(const device_chain_account *, uint32_t, uint32_t, uint8_t [33]);
int device_chain_secp_sign(const device_chain_account *, uint32_t, uint32_t,
			    const uint8_t [33], const uint8_t [32], uint8_t [64]);
int device_chain_sol_sign(const device_chain_account *, const uint8_t [32],
			   const uint8_t *, size_t, uint8_t [64]);
int device_chain_btc_root(uint8_t [33]);
int device_chain_btc_account(const device_chain_account *, uint8_t [33], uint8_t [32], uint8_t [33]);

int device_chain_bip48_pub(const device_chain_account *, uint8_t [33], uint8_t [32]);
int device_chain_bip48_parent(const device_chain_account *, uint8_t [33]);
int device_chain_bip48_sign(const device_chain_account *, uint32_t, uint32_t,
			     const uint8_t [33], const uint8_t [32], uint8_t [64]);
int device_chain_public_hmac(const uint8_t *, size_t, const uint8_t *, size_t, uint8_t [64]);

#endif
