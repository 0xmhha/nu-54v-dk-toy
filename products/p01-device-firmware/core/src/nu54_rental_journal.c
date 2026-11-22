#include "nu54_rental_journal.h"

#include <string.h>

#include "nu54_keccak.h"

static const uint8_t magic[] = {'R', 'J', '0', '3'};

enum entry_state {
	ENTRY_RESERVED = 1,
	ENTRY_ISSUED,
	ENTRY_BURNED,
};

_Static_assert(sizeof(nu54_rental_journal_image_t) == 4 + 208 + 64 + 2 + 8 + 191 * NU54_RENTAL_JOURNAL_CAPACITY,
	       "RJ03 storage layout must remain stable");
_Static_assert(offsetof(nu54_rental_evidence_t, entries) == 277 && sizeof(nu54_rental_evidence_entry_t) == 126,
	       "RJP1 reconciliation layout must remain stable");

static int nonzero(const uint8_t *bytes, size_t len)
{
	while (len--) {
		if (*bytes++) return 1;
	}
	return 0;
}

static int increment(uint8_t nonce[32])
{
	for (unsigned i = 32; i > 0; --i) {
		if (++nonce[i - 1] != 0) return 0;
	}
	return -1;
}

static void entry_nonce(const nu54_rental_journal_image_t *image, unsigned index, uint8_t nonce[32])
{
	memcpy(nonce, image->first, 32);
	for (unsigned i = 0; i < index; ++i) increment(nonce);
}

static int valid_kind(unsigned kind)
{
	return kind == NU54_RENTAL_PAYMENT || kind == NU54_RENTAL_LIMIT;
}

int nu54_rental_journal_binding_valid(const nu54_rental_binding_t *binding)
{
	return nonzero(binding->generation, sizeof(binding->generation)) &&
	       nonzero(binding->branch, sizeof(binding->branch)) &&
	       nonzero(binding->device, sizeof(binding->device)) &&
	       nonzero(binding->registration, sizeof(binding->registration)) &&
	       nonzero(binding->measurement, sizeof(binding->measurement)) &&
	       nonzero(binding->deployment, sizeof(binding->deployment)) &&
	       nonzero(binding->chain, sizeof(binding->chain)) &&
	       nonzero(binding->contract, sizeof(binding->contract)) &&
	       nonzero(binding->account, sizeof(binding->account));
}

void nu54_rental_journal_payment_request(const uint8_t merchant[20], const uint8_t order[32],
					 nu54_rental_journal_request_t *request)
{
	static const char domain[] = "RJ02/payment";
	uint8_t data[sizeof(domain) - 1 + 20 + 32];
	memcpy(data, domain, sizeof(domain) - 1);
	memcpy(data + sizeof(domain) - 1, merchant, 20);
	memcpy(data + sizeof(domain) - 1 + 20, order, 32);
	nu54_keccak256(data, sizeof(data), request->key);
	memcpy(request->merchant, merchant, 20);
	memcpy(request->order, order, 32);
}

static int valid_request(unsigned kind, const nu54_rental_journal_request_t *request)
{
	if (!nonzero(request->key, sizeof(request->key))) return 0;
	if (kind == NU54_RENTAL_LIMIT)
		return !nonzero(request->merchant, sizeof(request->merchant)) && !nonzero(request->order, sizeof(request->order));

	nu54_rental_journal_request_t expected;
	if (kind != NU54_RENTAL_PAYMENT || !nonzero(request->merchant, sizeof(request->merchant))) return 0;
	nu54_rental_journal_payment_request(request->merchant, request->order, &expected);
	return !memcmp(expected.key, request->key, sizeof(expected.key));
}

static int find_entry(const nu54_rental_journal_image_t *image, unsigned kind, const uint8_t request[32])
{
	for (unsigned i = 0; i < image->count; ++i) {
		const nu54_rental_journal_entry_t *entry = &image->entries[i];
		if (entry->kind == kind && memcmp(entry->request.key, request, 32) == 0) return (int)i;
	}
	return -1;
}

static int valid_image(const nu54_rental_journal_image_t *image)
{
	if (memcmp(image->magic, magic, sizeof(magic)) != 0 || !nu54_rental_journal_binding_valid(&image->binding) ||
	    image->count > NU54_RENTAL_JOURNAL_CAPACITY || image->frozen > 1 || image->first[31] != 0 ||
	    nonzero(image->frozen_at, sizeof(image->frozen_at)) != image->frozen) return 0;

	uint8_t end[32];
	memcpy(end, image->first, 32);
	for (unsigned i = 0; i < image->count; ++i) {
		const nu54_rental_journal_entry_t *entry = &image->entries[i];
		if (increment(end) != 0 || !valid_kind(entry->kind) ||
		    entry->state < ENTRY_RESERVED || entry->state > ENTRY_BURNED ||
		    (entry->state == ENTRY_RESERVED && (i + 1 != image->count || image->frozen)) ||
		    !nonzero(entry->expiry, sizeof(entry->expiry)) ||
		    !valid_request(entry->kind, &entry->request) ||
		    !nonzero(entry->digest, sizeof(entry->digest)) ||
		    nonzero(entry->signature, sizeof(entry->signature)) != (entry->state == ENTRY_ISSUED) ||
		    find_entry(image, entry->kind, entry->request.key) != (int)i) return 0;
	}
	return memcmp(end, image->next, 32) == 0 &&
	       !nonzero((const uint8_t *)&image->entries[image->count],
			sizeof(image->entries[0]) * (NU54_RENTAL_JOURNAL_CAPACITY - image->count));
}

/* On an ambiguous storage failure no caller may use this RAM state. Reopening reads the durable
 * image and resolves whether the write committed. A reserved entry never escapes as a signature. */
static int replace(nu54_rental_journal_t *journal)
{
	if (journal->store.replace(journal->store.context, &journal->image, sizeof(journal->image)) == 0) return 0;
	journal->loaded = 0;
	return NU54_RENTAL_JOURNAL_ERROR;
}

int nu54_rental_journal_open(nu54_rental_journal_t *journal, const nu54_rental_journal_store_t *store,
			    const nu54_rental_binding_t *binding, const uint8_t first[32], int create)
{
	journal->loaded = 0;
	if (!store || !store->read || !store->replace || !nu54_rental_journal_binding_valid(binding) || first[31] != 0)
		return NU54_RENTAL_JOURNAL_ERROR;

	journal->store = *store;
	nu54_rental_journal_image_t *image = &journal->image;
	int read = store->read(store->context, image, sizeof(*image));
	if (read == 1 && create) {
		memset(image, 0, sizeof(*image));
		memcpy(image->magic, magic, sizeof(magic));
		image->binding = *binding;
		memcpy(image->first, first, 32);
		memcpy(image->next, first, 32);
		if (replace(journal) != 0) return NU54_RENTAL_JOURNAL_ERROR;
	} else if (read != 0 || !valid_image(image)) {
		return NU54_RENTAL_JOURNAL_ERROR;
	} else if (memcmp(&image->binding, binding, sizeof(*binding)) != 0 || memcmp(image->first, first, 32) != 0) {
		return NU54_RENTAL_JOURNAL_CONFLICT;
	}
	if (image->count && image->entries[image->count - 1].state == ENTRY_RESERVED) {
		image->entries[image->count - 1].state = ENTRY_BURNED;
		if (replace(journal) != 0) return NU54_RENTAL_JOURNAL_ERROR;
	}
	journal->expected_binding = *binding;
	memmove(journal->expected_first, first, 32);
	journal->bound = 1;
	journal->loaded = 1;
	return 0;
}

int nu54_rental_journal_reload(nu54_rental_journal_t *journal)
{
	if (!journal->bound) return NU54_RENTAL_JOURNAL_ERROR;
	return nu54_rental_journal_open(journal, &journal->store, &journal->expected_binding, journal->expected_first, 0);
}

int nu54_rental_journal_observe_open(const nu54_rental_journal_t *journal)
{
	if (!journal || !journal->bound || !journal->loaded || journal->image.frozen || !journal->store.read)
		return NU54_RENTAL_JOURNAL_ERROR;

	nu54_rental_journal_image_t persisted;
	if (journal->store.read(journal->store.context, &persisted, sizeof(persisted)) != 0 ||
	    !valid_image(&persisted) ||
	    memcmp(&persisted.binding, &journal->expected_binding, sizeof(persisted.binding)) ||
	    memcmp(persisted.first, journal->expected_first, sizeof(persisted.first)) ||
	    memcmp(&persisted, &journal->image, sizeof(persisted)) ||
	    (persisted.count && persisted.entries[persisted.count - 1].state == ENTRY_RESERVED))
		return NU54_RENTAL_JOURNAL_ERROR;
	return 0;
}

int nu54_rental_journal_nonce(const nu54_rental_journal_t *journal, nu54_rental_journal_kind_t kind,
			     const uint8_t request[32], uint8_t nonce[32])
{
	if (!journal->loaded || !valid_kind(kind) || !nonzero(request, 32)) return NU54_RENTAL_JOURNAL_ERROR;
	const nu54_rental_journal_image_t *image = &journal->image;
	if (image->frozen) return NU54_RENTAL_JOURNAL_FROZEN;

	int index = find_entry(image, kind, request);
	if (index >= 0) {
		entry_nonce(image, (unsigned)index, nonce);
		return 0;
	}
	if (image->count == NU54_RENTAL_JOURNAL_CAPACITY) return NU54_RENTAL_JOURNAL_FULL;
	if (image->count && image->entries[image->count - 1].state == ENTRY_RESERVED) return NU54_RENTAL_JOURNAL_ERROR;
	memcpy(nonce, image->next, 32);
	return 0;
}

int nu54_rental_journal_reserve(nu54_rental_journal_t *journal, nu54_rental_journal_kind_t kind,
			       const nu54_rental_journal_request_t *request, uint64_t expiry,
			       const uint8_t digest[32], const uint8_t nonce[32], uint8_t replay[65])
{
	uint8_t assigned[32], deadline[8];
	int result = nu54_rental_journal_nonce(journal, kind, request->key, assigned);
	if (result != 0) return result;
	if (!expiry || !valid_request(kind, request) || !nonzero(digest, 32) || memcmp(assigned, nonce, 32) != 0)
		return NU54_RENTAL_JOURNAL_CONFLICT;
	for (unsigned i = 0; i < 8; ++i) deadline[i] = (uint8_t)(expiry >> (8 * (7 - i)));

	nu54_rental_journal_image_t *image = &journal->image;
	int index = find_entry(image, kind, request->key);
	if (index >= 0) {
		const nu54_rental_journal_entry_t *entry = &image->entries[index];
		if (memcmp(&entry->request, request, sizeof(*request)) || memcmp(entry->digest, digest, 32) ||
		    memcmp(entry->expiry, deadline, sizeof(deadline))) return NU54_RENTAL_JOURNAL_CONFLICT;
		if (entry->state == ENTRY_BURNED) return NU54_RENTAL_JOURNAL_BURNED;
		if (entry->state != ENTRY_ISSUED) return NU54_RENTAL_JOURNAL_ERROR;
		memcpy(replay, entry->signature, sizeof(entry->signature));
		return NU54_RENTAL_JOURNAL_REPLAY;
	}
	if (increment(assigned) != 0) return NU54_RENTAL_JOURNAL_FULL;
	nu54_rental_journal_entry_t *entry = &image->entries[image->count++];
	entry->kind = (uint8_t)kind;
	entry->state = ENTRY_RESERVED;
	entry->request = *request;
	memcpy(entry->digest, digest, sizeof(entry->digest));
	memcpy(entry->expiry, deadline, sizeof(entry->expiry));
	memcpy(image->next, assigned, sizeof(image->next));
	return replace(journal);
}

int nu54_rental_journal_finish(nu54_rental_journal_t *journal, const uint8_t nonce[32], const uint8_t signature[65])
{
	nu54_rental_journal_image_t *image = &journal->image;
	if (!journal->loaded || image->frozen || !image->count || (signature && !nonzero(signature, 65)))
		return NU54_RENTAL_JOURNAL_ERROR;

	uint8_t assigned[32];
	entry_nonce(image, image->count - 1, assigned);
	nu54_rental_journal_entry_t *entry = &image->entries[image->count - 1];
	if (entry->state != ENTRY_RESERVED || memcmp(assigned, nonce, sizeof(assigned)) != 0)
		return NU54_RENTAL_JOURNAL_CONFLICT;
	entry->state = signature ? ENTRY_ISSUED : ENTRY_BURNED;
	if (signature) memcpy(entry->signature, signature, sizeof(entry->signature));
	return replace(journal);
}

int nu54_rental_journal_project(const nu54_rental_journal_t *journal, nu54_rental_evidence_t *projection, uint8_t root[32])
{
	const nu54_rental_journal_image_t *image = &journal->image;
	if (!journal->loaded || !image->frozen || !valid_image(image)) return NU54_RENTAL_JOURNAL_ERROR;

	memset(projection, 0, sizeof(*projection));
	memcpy(projection->magic, "RJP1", 4);
	projection->binding = image->binding;
	memcpy(projection->first, image->first, 32);
	memcpy(projection->next, image->next, 32);
	projection->count = image->count;
	for (unsigned i = 0; i < projection->count; ++i) {
		const nu54_rental_journal_entry_t *stored = &image->entries[i];
		nu54_rental_evidence_entry_t *entry = &projection->entries[i];
		entry->kind = stored->kind;
		entry->state = stored->state == ENTRY_ISSUED ? NU54_RENTAL_EVIDENCE_ISSUED : NU54_RENTAL_EVIDENCE_BURNED;
		entry_nonce(image, i, entry->nonce);
		memcpy(entry->expiry, stored->expiry, sizeof(entry->expiry));
		memcpy(entry->merchant, stored->request.merchant, sizeof(entry->merchant));
		memcpy(entry->order, stored->request.order, sizeof(entry->order));
		memcpy(entry->digest, stored->digest, sizeof(entry->digest));
	}
	size_t size = offsetof(nu54_rental_evidence_t, entries) + projection->count * sizeof(projection->entries[0]);
	nu54_keccak256((const uint8_t *)projection, size, root);
	return 0;
}

int nu54_rental_journal_freeze(nu54_rental_journal_t *journal, uint64_t now, uint8_t root[32])
{
	if (!journal->loaded || !now) return NU54_RENTAL_JOURNAL_ERROR;
	nu54_rental_journal_image_t *image = &journal->image;
	if (!image->frozen) {
		for (unsigned i = 0; i < 8; ++i) image->frozen_at[7 - i] = (uint8_t)(now >> (i * 8));
		if (image->count && image->entries[image->count - 1].state == ENTRY_RESERVED)
			image->entries[image->count - 1].state = ENTRY_BURNED;
		image->frozen = 1;
		if (replace(journal) != 0) return NU54_RENTAL_JOURNAL_ERROR;
	}
	nu54_rental_evidence_t projection;
	return nu54_rental_journal_project(journal, &projection, root);
}
