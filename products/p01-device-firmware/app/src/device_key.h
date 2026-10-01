/*
 * The device key (payment-protocol.md 2, P01-FR-04). A persistent secp256k1 key made once
 * with the TRNG and kept by PSA Crypto (CRACEN); this week-7 build keeps it in Zephyr Secure
 * storage, the TF-M build moves it to encrypted ITS. The key never leaves PSA: callers get
 * the address and raw (r, s) signatures of 32-byte digests.
 */
#ifndef DEVICE_KEY_H
#define DEVICE_KEY_H

#include <stdint.h>

/* Opens the key, creating it on first boot. Returns 0 and the address, or a negative PSA status. */
int device_key_init(uint8_t address[20], int *created);

/* Deterministic ECDSA (RFC 6979) over a 32-byte digest; raw r || s. Returns 0 or a PSA status. */
int device_key_sign(const uint8_t digest[32], uint8_t rs[64]);

#endif /* DEVICE_KEY_H */
