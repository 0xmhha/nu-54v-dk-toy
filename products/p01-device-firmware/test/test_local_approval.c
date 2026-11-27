#include "nu54_local_approval.h"

#include <assert.h>
#include <string.h>

static uint8_t shown[NU54_LOCAL_PREIMAGE_SIZE];

static int present(void *context, const uint8_t *bytes, size_t length)
{
	(void)context;
	assert(length == sizeof(shown));
	memcpy(shown, bytes, length);
	return 0;
}

int main(void)
{
	nu54_local_approval approval = {0};
	nu54_local_intent first = {.generation = 1, .expires = 15, .session = {1}, .request = {2}, .preimage = {3}, .physical_prompt = 9};
	nu54_local_intent changed = first;
	uint8_t signable[NU54_LOCAL_PREIMAGE_SIZE];
	assert(nu54_local_approval_stage(&approval, &first, 5, NULL, NULL) != 0);
	assert(nu54_local_approval_stage(&approval, &first, 5, present, NULL) == 0);
	changed.preimage[10] = 99;
	assert(nu54_local_approval_consume(&approval, &changed, 6, 9, NU54_LOCAL_APPROVE, signable) != 0);
	assert(nu54_local_approval_consume(&approval, &first, 6, 9, NU54_LOCAL_APPROVE, signable) != 0);

	assert(nu54_local_approval_stage(&approval, &first, 5, present, NULL) == 0);
	changed = first;
	changed.generation++;
	assert(nu54_local_approval_consume(&approval, &changed, 6, 9, NU54_LOCAL_APPROVE, signable) != 0);
	assert(nu54_local_approval_stage(&approval, &first, 5, present, NULL) == 0);
	assert(nu54_local_approval_consume(&approval, &first, 15, 9, NU54_LOCAL_APPROVE, signable) != 0);
	assert(nu54_local_approval_stage(&approval, &first, 5, present, NULL) == 0);
	assert(nu54_local_approval_consume(&approval, &first, 6, 9, NU54_LOCAL_REJECT, signable) != 0);

	assert(nu54_local_approval_stage(&approval, &first, 5, present, NULL) == 0);
	assert(nu54_local_approval_consume(&approval, &first, 6, 8, NU54_LOCAL_APPROVE, signable) != 0);
	assert(nu54_local_approval_consume(&approval, &first, 6, 9, NU54_LOCAL_APPROVE, signable) == 0);
	assert(memcmp(signable, shown, sizeof(shown)) == 0);
	assert(nu54_local_approval_consume(&approval, &first, 6, 9, NU54_LOCAL_APPROVE, signable) != 0);
	return 0;
}
