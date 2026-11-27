#ifndef NU54_LOCAL_APPROVAL_H
#define NU54_LOCAL_APPROVAL_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define NU54_LOCAL_PREIMAGE_SIZE 66

typedef struct {
	uint64_t generation, expires;
	uint8_t session[16], request[16];
	uint8_t preimage[NU54_LOCAL_PREIMAGE_SIZE];
	uint32_t physical_prompt;
} nu54_local_intent;

typedef struct {
	nu54_local_intent intent;
	uint64_t prepared_at;
	bool pending;
} nu54_local_approval;

typedef enum {
	NU54_LOCAL_REJECT,
	NU54_LOCAL_APPROVE,
} nu54_local_button;

/* Only a trusted board renderer (or the bounded synthetic policy) can stage a prompt. */
int nu54_local_approval_stage(nu54_local_approval *approval, const nu54_local_intent *intent, uint64_t now,
			       int (*present)(void *context, const uint8_t *bytes, size_t length), void *context);
/* The prompt ID is captured at the physical GPIO press edge and never accepted from a wire body. */
int nu54_local_approval_consume(nu54_local_approval *approval, const nu54_local_intent *current, uint64_t now,
				 uint32_t physical_prompt, nu54_local_button button, uint8_t signable[NU54_LOCAL_PREIMAGE_SIZE]);
void nu54_local_approval_cancel(nu54_local_approval *approval);

#endif
