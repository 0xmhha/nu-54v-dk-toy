#ifndef NU54_QUOTE_V2_H
#define NU54_QUOTE_V2_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "nu54_quote_v2_generated.h"

#define NU54_V2_SIGNED_SIZE (NU54_V2_UNSIGNED_SIZE + NU54_V2_SIGNATURE_SIZE)

typedef struct {
	void (*keccak256)(const uint8_t *raw, size_t length, uint8_t out[32]);
	bool (*recover_address)(const uint8_t digest[32], const uint8_t sig[65], uint8_t address[20]);
} nu54_v2_quote_crypto;

typedef struct {
	uint8_t expected[NU54_V2_UNSIGNED_SIZE];
	uint8_t merchant_signer[20];
	uint64_t now;
	bool consumed;
} nu54_v2_quote_policy;

typedef struct {
	const uint8_t *bytes;
	size_t length;
} nu54_v2_quote_field;

/* Input fields are ordered and unpadded per the generated v2 schema. */
bool nu54_v2_quote_encode(const nu54_v2_quote_field fields[NU54_V2_FIELD_COUNT],
	size_t field_count, uint8_t out[NU54_V2_UNSIGNED_SIZE]);

bool nu54_v2_quote_digest(const uint8_t *raw, size_t length,
	const nu54_v2_quote_crypto *crypto, uint8_t out[32]);

/* Policy must be supplied by the authenticated order and merchant registry. */
int nu54_v2_quote_verify(const uint8_t *signed_quote, size_t length,
	const nu54_v2_quote_policy *policy, const nu54_v2_quote_crypto *crypto);

#endif
