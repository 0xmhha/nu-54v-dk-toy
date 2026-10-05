/*
 * EIP-712 hashing for the protocol structs (payment-protocol.md 2, [N04][N21]).
 *
 * Integers wider than 64 bits are 32-byte big-endian values exactly as they appear in the
 * EIP-712 encoding; uint64 fields are plain integers. The encodeType strings and the domain
 * name/version come from the generated nu54_protocol.h, so they cannot drift from the schema.
 * Board-independent: builds and tests on the host.
 */
#ifndef NU54_EIP712_H
#define NU54_EIP712_H

#include <stddef.h>
#include <stdint.h>

typedef struct {
	uint8_t chain_id[32];
	uint8_t contract[20];
	uint8_t merchant[20];
	uint8_t payout[20];
	uint8_t token[20];
	uint8_t amount[32];
	uint8_t order_id[32];
	uint8_t nonce[32];
	uint64_t expiry;
} nu54_payment_authorization_t;

typedef struct {
	uint8_t chain_id[32];
	uint8_t contract[20];
	uint8_t per_payment_limit[32];
	uint8_t daily_limit[32];
	uint8_t nonce[32];
	uint64_t expiry;
} nu54_limit_change_t;

typedef struct {
	uint8_t merchant[20];
	uint8_t payout[20];
	const uint8_t *name; /* UTF-8, not NUL-terminated */
	size_t name_len;
	uint64_t valid_from;
	uint64_t valid_until;
} nu54_merchant_attestation_t;

typedef struct {
	uint8_t order_id[32];
	uint8_t token[20];
	uint8_t amount[32];
	uint8_t payout[20];
	uint64_t expiry;
} nu54_merchant_order_t;

typedef struct {
	uint8_t device[20];
	uint64_t timestamp;
} nu54_time_anchor_t;

typedef struct {
	uint8_t device[20];
	uint8_t nonce[32];
} nu54_device_reset_t;

/* The merchant's signature over a kiosk's one-time key (payment-protocol.md 4.1). */
typedef struct {
	uint8_t merchant[20];
	uint8_t kiosk_ephemeral[32];
	uint8_t kiosk_nonce[32];
} nu54_kiosk_key_t;

/* The bonded phone app's check of a new wallet (payment-protocol.md 2, 5): not a payment type,
 * the settlement contract has no WalletCheck. */
typedef struct {
	uint8_t device[20];
	uint8_t challenge[32];
} nu54_wallet_check_t;

/* keccak256(EIP712Domain typehash, name, version, chainId, verifyingContract). */
void nu54_eip712_domain_separator(const uint8_t chain_id[32], const uint8_t verifying_contract[20], uint8_t out[32]);

void nu54_hash_payment_authorization(const nu54_payment_authorization_t *v, uint8_t out[32]);
void nu54_hash_limit_change(const nu54_limit_change_t *v, uint8_t out[32]);
void nu54_hash_merchant_attestation(const nu54_merchant_attestation_t *v, uint8_t out[32]);
void nu54_hash_merchant_order(const nu54_merchant_order_t *v, uint8_t out[32]);
void nu54_hash_time_anchor(const nu54_time_anchor_t *v, uint8_t out[32]);
void nu54_hash_device_reset(const nu54_device_reset_t *v, uint8_t out[32]);
void nu54_hash_kiosk_key(const nu54_kiosk_key_t *v, uint8_t out[32]);
void nu54_hash_wallet_check(const nu54_wallet_check_t *v, uint8_t out[32]);

/* keccak256(0x19 0x01 || domain separator || struct hash): the value that is signed. */
void nu54_eip712_digest(const uint8_t domain_separator[32], const uint8_t struct_hash[32], uint8_t out[32]);

#endif /* NU54_EIP712_H */
