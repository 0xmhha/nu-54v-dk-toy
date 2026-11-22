#ifndef NU54_RENTAL_FREEZE_H
#define NU54_RENTAL_FREEZE_H

#include "nu54_session.h"

#define NU54_RENTAL_FREEZE_LIFETIME 120

typedef struct nu54_rental_freeze_request {
	uint8_t challenge[32];
	uint8_t channel_nonce[32];
	uint64_t expires_at;
} nu54_rental_freeze_request_t;

typedef struct nu54_rental_freeze_receipt {
	uint8_t binding[32];
	uint8_t challenge[32];
	uint8_t projection_root[32];
	uint8_t first_nonce[32];
	uint8_t next_nonce[32];
	uint64_t entry_count;
	uint64_t frozen_at;
	uint64_t expires_at;
} nu54_rental_freeze_receipt_t;

void nu54_rental_freeze_request_digest(const nu54_rental_binding_t *binding, const nu54_rental_freeze_request_t *request,
				      uint8_t digest[32]);

void nu54_rental_freeze_receipt_digest(const nu54_rental_binding_t *binding, const nu54_rental_freeze_receipt_t *receipt,
				      uint8_t digest[32]);

/* Accept a fresh request signed by the operator authorized in this rental setup. Record the
 * permanent fence before signing with the independently provisioned board identity key. The
 * caller supplies the trusted board identity address and delivers this on the rental work queue. */
int nu54_device_attest_rental_freeze(nu54_device_t *d, const nu54_rental_freeze_request_t *request,
				     const uint8_t operator_signature[65], const uint8_t board_identity[20],
				     nu54_rental_freeze_receipt_t *receipt, uint8_t board_signature[65]);

#endif
