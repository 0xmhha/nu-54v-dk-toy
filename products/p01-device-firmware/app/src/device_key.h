/*
 * The device key (payment-protocol.md 2, P01-FR-04). A persistent secp256k1 key made with the
 * TRNG and kept by PSA Crypto (CRACEN); this build keeps it in Zephyr Secure storage, the TF-M
 * build moves it to encrypted ITS. The key never leaves PSA: callers get the address and raw
 * (r, s) signatures of 32-byte digests.
 */
#ifndef DEVICE_KEY_H
#define DEVICE_KEY_H

#include <stdint.h>
#include <stdbool.h>

typedef enum {
	DEVICE_KEY_LEGACY_RENTER,
	DEVICE_KEY_RENTER,
	DEVICE_KEY_PERSONAL,
	DEVICE_KEY_IDENTITY,
} device_key_slot;

/* Create only if absent. Existing keys are opened without replacement. */
int device_key_slot_open(device_key_slot slot, bool create, uint8_t address[20]);
int device_key_slot_sign(device_key_slot slot, const uint8_t digest[32], uint8_t rs[64]);

/* Legacy wire can activate only a committed rental key. Staging cannot enable signing. */
int device_key_activate_rental(device_key_slot slot, bool create, uint8_t address[20]);
void device_key_deactivate_rental(void);
int device_key_retire_new_rental(void);

/* Deterministic ECDSA (RFC 6979) over a 32-byte digest; raw r || s. Returns 0 or a PSA status. */
int device_key_sign(const uint8_t digest[32], uint8_t rs[64]);

#endif /* DEVICE_KEY_H */
