/*
 * secp256k1 signatures in the form the contract and the protocol accept:
 * 65 bytes r || s || v, low-s only, v = 27 or 28 (payment-protocol.md 2, design 4).
 *
 * The device key never signs here: the TF-M secure partition signs with CRACEN and passes
 * (r, s) to nu54_sig_finish, which normalizes s and finds v. Recovery (operator and merchant
 * signatures, v) uses libsecp256k1's recovery module from third_party/secp256k1 (MIT).
 */
#ifndef NU54_SIG_H
#define NU54_SIG_H

#include <stddef.h>
#include <stdint.h>

typedef enum {
	NU54_SIG_OK = 0,
	NU54_SIG_BAD_LENGTH, /* not 65 bytes */
	NU54_SIG_BAD_V,      /* v is not 27 or 28 */
	NU54_SIG_HIGH_S,     /* s above n / 2 */
	NU54_SIG_INVALID,    /* r or s out of range, or no point recovers */
} nu54_sig_result_t;

/* Ethereum address of an uncompressed public key (0x04 || X || Y). */
void nu54_address_of_pubkey(const uint8_t pubkey65[65], uint8_t address[20]);

/* Recovers the signer address of a protocol signature over a 32-byte digest. */
nu54_sig_result_t nu54_sig_recover(const uint8_t digest[32], const uint8_t *sig, size_t sig_len, uint8_t address[20]);

/*
 * Turns a raw (r, s) from the secure partition into the protocol form: s is replaced by
 * n - s when it is high, and v is the recovery id (+27) whose public key hashes to `address`
 * (the device's own address). Returns NU54_SIG_INVALID if neither id matches.
 */
nu54_sig_result_t nu54_sig_finish(const uint8_t digest[32], const uint8_t rs[64], const uint8_t address[20], uint8_t sig[65]);

#endif /* NU54_SIG_H */
