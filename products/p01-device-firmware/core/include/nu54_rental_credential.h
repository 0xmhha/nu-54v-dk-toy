#ifndef NU54_RENTAL_CREDENTIAL_H
#define NU54_RENTAL_CREDENTIAL_H

#include "nu54_rental_journal.h"

#define NU54_RENTAL_CREDENTIAL_SIZE 97

typedef struct {
	uint64_t revision;
	uint64_t policy;
	uint64_t issued_at;
	uint64_t expires_at;
} nu54_rental_credential_t;

/* Storage is scoped to the existing protected rental slot. A missing slot must
 * return 1, inaccessible storage must return negative. Create must atomically
 * require absence and commit the complete record without replacing a credential.
 * may_issue returns 1 only for a newly commissioned credential-profile generation.
 * trusted_now must be fresh and rollback-protected across reset. */
typedef struct {
	void *context;
	int (*read)(void *context, void *record, size_t length);
	int (*create)(void *context, const void *record, size_t length);
	int (*may_issue)(void *context, const nu54_rental_binding_t *binding);
	int (*trusted_now)(void *context, uint64_t *now);
} nu54_rental_credential_store_t;

void nu54_rental_credential_digest(const nu54_rental_binding_t *binding,
				   const nu54_rental_credential_t *credential, uint8_t digest[32]);

int nu54_rental_credential_install(const nu54_rental_credential_store_t *store,
				   const nu54_rental_binding_t *binding, const uint8_t operator_address[20],
				   const nu54_rental_credential_t *credential, const uint8_t signature[65]);

/* Returns the verified current expiry only after re-reading protected state. */
int nu54_rental_credential_current(const nu54_rental_credential_store_t *store,
				   const nu54_rental_binding_t *binding, const uint8_t operator_address[20],
				   nu54_rental_credential_t *credential, uint64_t *checked_at);

typedef struct {
	nu54_rental_credential_t credential;
	uint8_t challenge[32];
	uint64_t checked_at;
} nu54_rental_credential_receipt_t;

void nu54_rental_slot_open_receipt_digest(const nu54_rental_binding_t *binding,
					 const nu54_rental_credential_receipt_t *receipt, uint8_t digest[32]);
void nu54_rental_credential_receipt_digest(const nu54_rental_binding_t *binding,
					   const nu54_rental_credential_receipt_t *receipt, uint8_t digest[32]);

#endif
