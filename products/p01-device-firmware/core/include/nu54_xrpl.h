#ifndef NU54_XRPL_H
#define NU54_XRPL_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define NU54_XRPL_UNSIGNED_MAX 224
#define NU54_XRPL_SIGNED_MAX 298

typedef struct {
	uint8_t account[20], public_key[33];
	uint8_t issuer[20], currency[20];
	uint32_t ledger, max_ledger_delta, sequence, ticket;
	uint64_t max_fee;
	bool allow_issued;
	int (*sha512_half)(const uint8_t *, size_t, uint8_t [32]);
} nu54_xrpl_policy;

typedef struct {
	uint8_t account[20], destination[20], issuer[20], currency[20];
	uint8_t digest[32];
	uint32_t destination_tag, sequence, ticket, last_ledger;
	/* XRP drops when issued=false. Exact coefficient * 10^exponent otherwise. */
	uint64_t coefficient, fee_drops;
	int16_t exponent;
	bool has_tag, issued;
} nu54_xrpl_intent;

typedef int (*nu54_xrpl_sign)(void *, const uint8_t [33], const uint8_t [32], uint8_t [72], size_t *);

int nu54_xrpl_review(const uint8_t *, size_t, const nu54_xrpl_policy *, nu54_xrpl_intent *);
int nu54_xrpl_finalize(const uint8_t *, size_t, const nu54_xrpl_policy *, const uint8_t [32],
		       nu54_xrpl_sign, void *, uint8_t *, size_t, size_t *);

#endif
