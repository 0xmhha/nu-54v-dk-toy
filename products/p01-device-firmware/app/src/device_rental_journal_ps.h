#ifndef DEVICE_RENTAL_JOURNAL_PS_H
#define DEVICE_RENTAL_JOURNAL_PS_H

#include <stdbool.h>

#include "nu54_rental_registration.h"
#include "nu54_rental_retirement.h"

/* Qualification must verify real hardware rollback, standing boot measurement and storage budget.
 * The default implementation refuses. Call only from the serialized rental session. */
bool device_rental_journal_platform_qualified(void);
/* Return the verified executing measurement from the board's trust boundary. */
bool device_rental_journal_measurement(uint8_t measurement[32]);
/* A verified commissioning policy must explicitly authorize this peer for this generation. */
bool device_rental_collector_peer_allowed(const nu54_rental_binding_t *binding, const uint8_t peer[7]);

/* Record the exact new generation before its key is eligible to sign. Only an exclusive,
 * authenticated commissioning path may call this. It cannot replace a different scope. */
int device_rental_journal_stage_ps(const nu54_rental_binding_t *binding, const uint8_t first[32]);

/* Read the protected commissioning marker. Return 1 only for definite absence. */
int device_rental_journal_marker_ps(nu54_rental_binding_t *binding, uint8_t first[32], bool *active);

/* The commissioning caller must hold a new, signing-disabled generation. */
int device_rental_journal_activate_ps(nu54_rental_journal_t *journal, const nu54_rental_registration_t *registration,
				     const nu54_rental_registration_trust_t *trust, const uint8_t signature[65]);

/* Read an already activated generation. No storage is created on boot. The expected binding
 * and first nonce must come from the protected activation record, independently of the journal. */
int device_rental_journal_resume_ps(nu54_rental_journal_t *journal, const nu54_rental_binding_t *binding,
				   const uint8_t first[32]);

/* Qualification of the single-generation retirement fence requires rollback protection
 * and WRITE_ONCE PS semantics. Successor staging has its own verification hook below. */
bool device_rental_retirement_platform_qualified(void);

/* Internal protected tombstone; requires the serialized slot guard and independently verified
 * authority. An exact replay checks the stored fence and never writes it again. No key is erased. */
int device_rental_journal_retirement_fence_ps(const nu54_rental_retirement_authority_t *authority,
					     uint64_t now);

#define DEVICE_RENTAL_RETURN_RECEIPT_BYTES 109

typedef struct {
	nu54_rental_binding_t prior, successor;
	uint8_t receipt[DEVICE_RENTAL_RETURN_RECEIPT_BYTES];
	uint8_t server_ack[32];
	uint8_t reclaim_receipt[32];
} device_rental_handoff_t;

/* The platform trust provider independently verifies the exact RCR1, protected retirement,
 * server acknowledgement authorizing both bindings, and any permitted historical reclaim.
 * False by default; a digest in untrusted input cannot qualify a transition. */
bool device_rental_handoff_verify(const device_rental_handoff_t *handoff, uint64_t now);

/* Requires the shared serialized slot guard. Records a successor candidate only. */
int device_rental_journal_stage_successor_ps(const device_rental_handoff_t *handoff, uint64_t now);

/* Protected dispatch integration. A platform may override the defaults only
 * when the same lock covers enrollment, all journal writers and retirement.
 * Witness data must already be locally authenticated; no callback may use
 * external I/O while the guard is held. */
int device_rental_cleanup_station_allowed(void *context, const nu54_rental_binding_t *binding,
					   const uint8_t peer[7]);
int device_rental_cleanup_begin(void *context);
void device_rental_cleanup_end(void *context);
int device_rental_cleanup_time(void *context, uint64_t *now);
int device_rental_cleanup_witness(void *context, const nu54_rental_binding_t *binding,
				  const nu54_rental_cleanup_intent_t *intent,
				  nu54_rental_cleanup_witness_t *witness);

#endif
