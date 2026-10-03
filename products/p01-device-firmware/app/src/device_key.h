/*
 * The device key (payment-protocol.md 2, P01-FR-04). A persistent secp256k1 key made with the
 * TRNG and kept by PSA Crypto (CRACEN); this build keeps it in Zephyr Secure storage, the TF-M
 * build moves it to encrypted ITS. The key never leaves PSA: callers get the address and raw
 * (r, s) signatures of 32-byte digests.
 */
#ifndef DEVICE_KEY_H
#define DEVICE_KEY_H

#include <stdint.h>

/* Week-7 fixed setup: opens the key, creating it on first boot. Returns 0 and the address, or a
 * negative PSA status. */
int device_key_init(uint8_t address[20], int *created);

/* Opens the existing key. Returns 0 and the address, or a PSA status (PSA_ERROR_INVALID_HANDLE
 * when there is no key). */
int device_key_open(uint8_t address[20]);

/* Rental setup: makes a new key in place of any old one. Returns 0 and the address, or a PSA status. */
int device_key_generate(uint8_t address[20]);

/* Destroys the key. Returns 0, also when there was none, or a PSA status. */
int device_key_destroy(void);

/* Deterministic ECDSA (RFC 6979) over a 32-byte digest; raw r || s. Returns 0 or a PSA status. */
int device_key_sign(const uint8_t digest[32], uint8_t rs[64]);

#endif /* DEVICE_KEY_H */
