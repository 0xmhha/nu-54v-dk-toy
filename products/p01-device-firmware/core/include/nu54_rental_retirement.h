#ifndef NU54_RENTAL_RETIREMENT_H
#define NU54_RENTAL_RETIREMENT_H

#include "nu54_rental_journal.h"

/* Internal sidecar for a pending retirement only, separate from the RJ03 journal. */
typedef struct {
	uint8_t magic[4];
	uint8_t binding[32];
	uint8_t root[32];
	uint8_t intent[32];
	uint8_t disposition[32];
	uint8_t request[32];
} nu54_rental_retirement_fence_t;

typedef struct nu54_rental_retirement_authority {
	uint8_t binding[32], root[32], intent[32], disposition[32], request[32];
	uint64_t issued_at, expires_at;
	unsigned pending_payments, pending_limits;
	int disposition_final;
} nu54_rental_retirement_authority_t;

typedef struct {
	void *context;
	/* Dedicated record for this slot; absent=1, present=0, error<0. */
	int (*read)(void *context, nu54_rental_retirement_fence_t *fence);
	/* Durable create-if-absent. Existing record=1, committed=0, uncertain/error<0. */
	int (*create)(void *context, const nu54_rental_retirement_fence_t *fence);
} nu54_rental_retirement_store_t;

/* The caller resolves authenticated intent/settlement evidence and trusted time
 * independently, and holds the same per-slot guard as the protected journal.
 * The result identifies a pending fence only; no result authorizes erasure or signing. */
int nu54_rental_retirement_fence(const nu54_rental_journal_t *journal,
			       const nu54_rental_retirement_store_t *store,
			       const nu54_rental_retirement_authority_t *authority, uint64_t now);

#define NU54_RENTAL_CLEANUP_INTENT_SIZE 181

typedef struct nu54_rental_cleanup_intent {
	uint8_t binding[32], disposition[32], request[32], digest[32], envelope[32];
	uint64_t issued_at, expires_at;
} nu54_rental_cleanup_intent_t;

typedef struct nu54_rental_cleanup_witness {
	uint8_t binding[32], request[32], intent[32], envelope[32], root[32];
	uint8_t freeze[32], close[32], anchor[32], withdrawal[32];
	uint64_t checked_at, valid_until;
	unsigned pending_payments, pending_limits;
	uint8_t kind;
} nu54_rental_cleanup_witness_t;

/* Production codec; the SHA/Keccak identifiers in witness come from a local,
 * authenticated resolver, not a claimant-provided envelope. */
void nu54_rental_cleanup_intent_digest(const uint8_t chain[32], const uint8_t settlement[20],
				      const uint8_t raw[NU54_RENTAL_CLEANUP_INTENT_SIZE], uint8_t digest[32]);
int nu54_rental_cleanup_intent_verify(const nu54_rental_binding_t *binding, const uint8_t operator_address[20],
				      const uint8_t *raw, size_t length, nu54_rental_cleanup_intent_t *intent);
int nu54_rental_cleanup_authorize(const nu54_rental_cleanup_intent_t *intent, const nu54_rental_cleanup_witness_t *witness,
				   uint64_t now, nu54_rental_retirement_authority_t *authority);

#endif
