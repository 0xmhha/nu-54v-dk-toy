#ifndef NU54_RENTAL_JOURNAL_H
#define NU54_RENTAL_JOURNAL_H

#include <stddef.h>
#include <stdint.h>

#define NU54_RENTAL_JOURNAL_CAPACITY 8

typedef enum {
	NU54_RENTAL_JOURNAL_ERROR = -1,
	NU54_RENTAL_JOURNAL_CONFLICT = -2,
	NU54_RENTAL_JOURNAL_FULL = -3,
	NU54_RENTAL_JOURNAL_FROZEN = -4,
	NU54_RENTAL_JOURNAL_RESERVED = 0,
	NU54_RENTAL_JOURNAL_REPLAY = 1,
	NU54_RENTAL_JOURNAL_BURNED = 2,
} nu54_rental_journal_result_t;

typedef enum {
	NU54_RENTAL_PAYMENT = 1,
	NU54_RENTAL_LIMIT = 2,
} nu54_rental_journal_kind_t;

/* Integer fields use unsigned big-endian bytes. The caller must authenticate the registration. */
typedef struct {
	uint8_t generation[8];
	uint8_t branch[16];
	uint8_t device[16];
	uint8_t registration[32];
	uint8_t measurement[32];
	uint8_t deployment[32];
	uint8_t chain[32];
	uint8_t contract[20];
	uint8_t account[20];
} nu54_rental_binding_t;

int nu54_rental_journal_binding_valid(const nu54_rental_binding_t *binding);

typedef struct {
	uint8_t key[32];
	uint8_t merchant[20];
	uint8_t order[32];
} nu54_rental_journal_request_t;

void nu54_rental_journal_payment_request(const uint8_t merchant[20], const uint8_t order[32],
					 nu54_rental_journal_request_t *request);

typedef struct {
	uint8_t kind;
	uint8_t state;
	uint8_t expiry[8];
	nu54_rental_journal_request_t request;
	uint8_t digest[32];
	uint8_t signature[65];
} nu54_rental_journal_entry_t;

typedef struct {
	uint8_t magic[4];
	nu54_rental_binding_t binding;
	uint8_t first[32];
	uint8_t next[32];
	uint8_t count;
	uint8_t frozen;
	uint8_t frozen_at[8];
	nu54_rental_journal_entry_t entries[NU54_RENTAL_JOURNAL_CAPACITY];
} nu54_rental_journal_image_t;

typedef enum {
	NU54_RENTAL_EVIDENCE_ISSUED = 1,
	NU54_RENTAL_EVIDENCE_BURNED = 2,
} nu54_rental_evidence_state_t;

typedef struct {
	uint8_t kind;
	uint8_t state;
	uint8_t nonce[32];
	uint8_t expiry[8];
	uint8_t merchant[20];
	uint8_t order[32];
	uint8_t digest[32];
} nu54_rental_evidence_entry_t;

/* The RJP1 wire data ends immediately after entries[count - 1]. */
typedef struct {
	uint8_t magic[4];
	nu54_rental_binding_t binding;
	uint8_t first[32];
	uint8_t next[32];
	uint8_t count;
	nu54_rental_evidence_entry_t entries[NU54_RENTAL_JOURNAL_CAPACITY];
} nu54_rental_evidence_t;

typedef struct {
	void *context;
	/* Return 0 for one exact complete image, 1 for definite absence, or negative on error.
	 * Replace must commit the entire image atomically, even on interruption. */
	int (*read)(void *context, void *image, size_t bytes);
	int (*replace)(void *context, const void *image, size_t bytes);
} nu54_rental_journal_store_t;

typedef struct {
	nu54_rental_journal_store_t store;
	nu54_rental_journal_image_t image;
	nu54_rental_binding_t expected_binding;
	uint8_t expected_first[32];
	uint8_t bound;
	uint8_t loaded;
} nu54_rental_journal_t;

/* Start at authenticated activation, before business signing, never at legacy upgrade.
 * An existing image is reopened only for the same binding and original starting nonce.
 * A reservation left by a prior boot is burned before the journal opens. */
int nu54_rental_journal_open(nu54_rental_journal_t *journal, const nu54_rental_journal_store_t *store,
			    const nu54_rental_binding_t *binding, const uint8_t first[32], int create);

/* Re-read the last authenticated binding after an ambiguous write. */
int nu54_rental_journal_reload(nu54_rental_journal_t *journal);
/* Read-only check of the current protected image under the adapter's guard. */
int nu54_rental_journal_observe_open(const nu54_rental_journal_t *journal);

/* Preview the assigned nonce without writing it. A matching request uses its original nonce. */
int nu54_rental_journal_nonce(const nu54_rental_journal_t *journal, nu54_rental_journal_kind_t kind,
			     const uint8_t request[32], uint8_t nonce[32]);

/* Accept an exact replay only if kind, identity, expiry, and final digest match.
 * A new reservation and the advancement of the range are one durable replacement. */
int nu54_rental_journal_reserve(nu54_rental_journal_t *journal, nu54_rental_journal_kind_t kind,
			       const nu54_rental_journal_request_t *request, uint64_t expiry,
			       const uint8_t digest[32], const uint8_t nonce[32], uint8_t replay[65]);

/* A signature cannot leave the device unless finish accepts the final bytes. Pass NULL to burn. */
int nu54_rental_journal_finish(nu54_rental_journal_t *journal, const uint8_t nonce[32], const uint8_t signature[65]);

/* Export only the verifiable reconciliation fields of an already frozen journal. */
int nu54_rental_journal_project(const nu54_rental_journal_t *journal, nu54_rental_evidence_t *projection,
			       uint8_t root[32]);

/* Durable fence and the commitment to the public RJP1 projection. */
int nu54_rental_journal_freeze(nu54_rental_journal_t *journal, uint64_t now, uint8_t root[32]);

#endif
