#include "nu54_local_approval.h"

#include <string.h>

void nu54_local_approval_cancel(nu54_local_approval *approval)
{
	memset(approval, 0, sizeof(*approval));
}

int nu54_local_approval_stage(nu54_local_approval *approval, const nu54_local_intent *intent, uint64_t now,
			       int (*present)(void *, const uint8_t *, size_t), void *context)
{
	if (approval->pending || !intent->generation || !intent->physical_prompt || intent->expires <= now || !present) return -1;
	approval->intent = *intent;
	if (present(context, approval->intent.preimage, sizeof(approval->intent.preimage)) != 0) {
		nu54_local_approval_cancel(approval);
		return -1;
	}
	approval->prepared_at = now;
	approval->pending = true;
	return 0;
}

int nu54_local_approval_consume(nu54_local_approval *approval, const nu54_local_intent *current, uint64_t now,
				 uint32_t physical_prompt, nu54_local_button button, uint8_t signable[NU54_LOCAL_PREIMAGE_SIZE])
{
	const nu54_local_intent *shown = &approval->intent;
	if (!approval->pending || physical_prompt != shown->physical_prompt) return -1;
	bool matched = button == NU54_LOCAL_APPROVE && now >= approval->prepared_at && now < shown->expires &&
		       shown->expires == current->expires && shown->generation == current->generation &&
		       memcmp(shown->session, current->session, sizeof(shown->session)) == 0 &&
		       memcmp(shown->request, current->request, sizeof(shown->request)) == 0 &&
		       memcmp(shown->preimage, current->preimage, sizeof(shown->preimage)) == 0;
	if (matched) memcpy(signable, shown->preimage, sizeof(shown->preimage));
	nu54_local_approval_cancel(approval);
	return matched ? 0 : -1;
}
