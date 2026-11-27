#ifndef NU54_RENTAL_REGISTRATION_H
#define NU54_RENTAL_REGISTRATION_H

#include "nu54_rental_journal.h"

#define NU54_RENTAL_REGISTRATION_LIFETIME 120

typedef struct {
	nu54_rental_binding_t binding;
	uint8_t first[32];
	uint8_t challenge[32];
	uint64_t expires_at;
} nu54_rental_registration_t;

/* The expected values come from the board plus its authenticated commissioning context.
 * In particular the board reads its own running measurement and derives the chain, settlement
 * contract, operator, account and starting nonce from the new, signing-disabled setup. */
typedef struct {
	nu54_rental_binding_t binding;
	uint8_t first[32];
	uint8_t challenge[32];
	uint8_t operator_address[20];
	uint64_t now;
} nu54_rental_registration_trust_t;

void nu54_rental_registration_digest(const nu54_rental_registration_t *registration, uint8_t digest[32]);
int nu54_rental_registration_verify(const nu54_rental_registration_t *registration,
				   const nu54_rental_registration_trust_t *trust, const uint8_t signature[65]);

#endif
