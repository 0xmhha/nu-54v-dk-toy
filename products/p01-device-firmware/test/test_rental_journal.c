#include "nu54_rental_journal.h"
#include "nu54_rental_retirement.h"
#include "nu54_keccak.h"

#ifdef NDEBUG
#undef NDEBUG
#endif
#include <assert.h>
#include <stdio.h>
#include <string.h>

#include "rental_evidence_vector.h"

typedef struct {
	nu54_rental_journal_image_t image;
	int exists;
	int fail;
	int commit_on_failure;
} storage_t;

static int read_image(void *context, void *image, size_t bytes)
{
	storage_t *s = context;
	assert(bytes == sizeof(s->image));
	if (!s->exists) return 1;
	memcpy(image, &s->image, bytes);
	return 0;
}

static int replace_image(void *context, const void *image, size_t bytes)
{
	storage_t *s = context;
	assert(bytes == sizeof(s->image));
	if (!s->fail || s->commit_on_failure) {
		memcpy(&s->image, image, bytes);
		s->exists = 1;
	}
	return s->fail ? -1 : 0;
}

static void verify_shared_projection(const char *path)
{
	FILE *file = fopen(path, "r");
	assert(file);
	char wire_hex[sizeof(nu54_rental_evidence_t) * 2 + 1], root_hex[65], extra;
	assert(fscanf(file, "%2570s %64s %c", wire_hex, root_hex, &extra) == 2);
	fclose(file);
	size_t bytes = strlen(wire_hex) / 2;
	nu54_rental_evidence_t expected = {0}, observed;
	assert(bytes >= offsetof(nu54_rental_evidence_t, entries) && bytes <= sizeof(expected));
	rental_vector_decode_hex(wire_hex, (uint8_t *)&expected, bytes);
	assert(!memcmp(expected.magic, "RJP1", 4) && expected.count <= NU54_RENTAL_JOURNAL_CAPACITY);
	assert(bytes == offsetof(nu54_rental_evidence_t, entries) + expected.count * sizeof(expected.entries[0]));

	storage_t persisted = {0};
	nu54_rental_journal_store_t store = {&persisted, read_image, replace_image};
	nu54_rental_journal_t journal = {0};
	assert(nu54_rental_journal_open(&journal, &store, &expected.binding, expected.first, 1) == 0);
	for (unsigned i = 0; i < expected.count; ++i) {
		const nu54_rental_evidence_entry_t *entry = &expected.entries[i];
		nu54_rental_journal_request_t request = {.key = {(uint8_t)(i + 1)}};
		if (entry->kind == NU54_RENTAL_PAYMENT)
			nu54_rental_journal_payment_request(entry->merchant, entry->order, &request);
		uint64_t expiry = 0;
		for (unsigned b = 0; b < 8; ++b) expiry = (expiry << 8) | entry->expiry[b];
		uint8_t nonce[32], replay[65], local_test_signature[65] = {0x42};
		assert(nu54_rental_journal_nonce(&journal, entry->kind, request.key, nonce) == 0);
		assert(!memcmp(nonce, entry->nonce, sizeof(nonce)));
		assert(nu54_rental_journal_reserve(&journal, entry->kind, &request, expiry, entry->digest, nonce, replay) == 0);
		assert(entry->state == NU54_RENTAL_EVIDENCE_ISSUED || entry->state == NU54_RENTAL_EVIDENCE_BURNED);
		assert(nu54_rental_journal_finish(&journal, nonce,
			entry->state == NU54_RENTAL_EVIDENCE_ISSUED ? local_test_signature : NULL) == 0);
	}
	uint8_t root[32], expected_root[32];
	rental_vector_decode_hex(root_hex, expected_root, sizeof(expected_root));
	assert(nu54_rental_journal_freeze(&journal, 100, root) == 0);
	assert(nu54_rental_journal_project(&journal, &observed, root) == 0);
	assert(!memcmp(root, expected_root, sizeof(root)));
	assert(!memcmp(&observed, &expected, bytes));
}

typedef struct {
	nu54_rental_retirement_fence_t fence;
	int exists, fail_read, fail_create, commit_on_failure;
	unsigned creates;
} retirement_storage_t;

static int read_retirement(void *context, nu54_rental_retirement_fence_t *fence)
{
	retirement_storage_t *s = context;
	if (s->fail_read) return -1;
	if (!s->exists) return 1;
	*fence = s->fence;
	return 0;
}

static int create_retirement(void *context, const nu54_rental_retirement_fence_t *fence)
{
	retirement_storage_t *s = context;
	++s->creates;
	if (s->exists) return 1;
	if (!s->fail_create || s->commit_on_failure) {
		s->fence = *fence;
		s->exists = 1;
	}
	return s->fail_create ? -1 : 0;
}

static void verify_retirement(nu54_rental_journal_t *journal, storage_t *storage, const uint8_t root[32])
{
	struct {
		retirement_storage_t slot, other_slot;
		uint8_t personal[4];
	} protected_fixture = {.other_slot = {.fence.request = {9}, .exists = 1}, .personal = {1, 2, 3, 4}};
	retirement_storage_t *sidecar = &protected_fixture.slot;
	const nu54_rental_retirement_store_t store = {sidecar, read_retirement, create_retirement};
	const nu54_rental_journal_image_t original = storage->image;
	const retirement_storage_t untouched_slot = protected_fixture.other_slot;
	nu54_rental_retirement_authority_t scope = {
		.intent = {1}, .disposition = {2}, .request = {3},
		.issued_at = 100, .expires_at = 220, .disposition_final = 1,
	};
	memcpy(scope.root, root, 32);
	nu54_keccak256((const uint8_t *)&original.binding, sizeof(original.binding), scope.binding);
	nu54_rental_retirement_authority_t other = scope;
	other.pending_payments = 1;
	assert(nu54_rental_retirement_fence(journal, &store, &other, 100) == NU54_RENTAL_JOURNAL_ERROR);
	other = scope;
	other.pending_limits = 1;
	assert(nu54_rental_retirement_fence(journal, &store, &other, 100) == NU54_RENTAL_JOURNAL_ERROR);
	other = scope;
	other.disposition_final = 0;
	assert(nu54_rental_retirement_fence(journal, &store, &other, 100) == NU54_RENTAL_JOURNAL_ERROR);
	other = scope;
	other.binding[0] ^= 1;
	assert(nu54_rental_retirement_fence(journal, &store, &other, 100) == NU54_RENTAL_JOURNAL_CONFLICT);
	journal->image.frozen = 0;
	assert(nu54_rental_retirement_fence(journal, &store, &scope, 100) == NU54_RENTAL_JOURNAL_ERROR);
	assert(nu54_rental_journal_reload(journal) == 0);
	other = scope;
	other.root[0] ^= 1;
	assert(nu54_rental_retirement_fence(journal, &store, &other, 100) == NU54_RENTAL_JOURNAL_CONFLICT);
	assert(nu54_rental_retirement_fence(journal, &store, &scope, 220) == NU54_RENTAL_JOURNAL_ERROR);
	sidecar->fail_read = 1;
	assert(nu54_rental_retirement_fence(journal, &store, &scope, 100) == NU54_RENTAL_JOURNAL_ERROR);
	sidecar->fail_read = 0;
	assert(!sidecar->exists && !sidecar->creates);
	sidecar->fail_create = 1;
	assert(nu54_rental_retirement_fence(journal, &store, &scope, 100) == NU54_RENTAL_JOURNAL_ERROR);
	assert(!sidecar->exists && sidecar->creates == 1);
	assert(nu54_rental_journal_reload(journal) == 0);
	sidecar->commit_on_failure = 1;
	assert(nu54_rental_retirement_fence(journal, &store, &scope, 101) == NU54_RENTAL_JOURNAL_ERROR);
	assert(sidecar->exists && sidecar->creates == 2);
	const nu54_rental_retirement_fence_t committed = sidecar->fence;
	assert(!memcmp(committed.intent, scope.intent, 32));
	assert(nu54_rental_journal_reload(journal) == 0);
	sidecar->fail_create = 0;
	assert(nu54_rental_retirement_fence(journal, &store, &scope, 220) == NU54_RENTAL_JOURNAL_REPLAY);
	other = scope;
	other.request[1] = 1;
	assert(nu54_rental_retirement_fence(journal, &store, &other, 110) == NU54_RENTAL_JOURNAL_CONFLICT);
	other = scope;
	other.disposition[1] = 1;
	assert(nu54_rental_retirement_fence(journal, &store, &other, 110) == NU54_RENTAL_JOURNAL_CONFLICT);
	other = scope;
	other.intent[1] = 1;
	assert(nu54_rental_retirement_fence(journal, &store, &other, 110) == NU54_RENTAL_JOURNAL_CONFLICT);
	assert(!memcmp(&committed, &sidecar->fence, sizeof(committed)) && sidecar->creates == 2);
	assert(!memcmp(&original, &storage->image, sizeof(original)));
	assert(!memcmp(&protected_fixture.other_slot, &untouched_slot, sizeof(untouched_slot)));
	assert(!memcmp(protected_fixture.personal, (uint8_t[]){1, 2, 3, 4}, sizeof(protected_fixture.personal)));
	*sidecar = (retirement_storage_t){0};
	assert(nu54_rental_retirement_fence(journal, &store, &scope, 110) == NU54_RENTAL_JOURNAL_RESERVED);
	assert(sidecar->creates == 1);
	sidecar->fence.magic[3] = '0';
	assert(nu54_rental_retirement_fence(journal, &store, &scope, 110) == NU54_RENTAL_JOURNAL_ERROR);
}

int main(int argc, char *argv[])
{
	if (argc == 2) verify_shared_projection(argv[1]);
	storage_t storage = {0};
	nu54_rental_journal_store_t store = {&storage, read_image, replace_image};
	nu54_rental_binding_t binding = {
		.generation = {0, 1}, .branch = {1}, .device = {2}, .registration = {3}, .measurement = {4},
		.deployment = {5}, .chain = {6}, .contract = {7}, .account = {8},
	};
	uint8_t first[32] = {0x10}, merchant[20] = {1}, order[32] = {2}, digest[32] = {3};
	nu54_rental_journal_request_t payment, limit = {.key = {1}};
	nu54_rental_journal_payment_request(merchant, order, &payment);
	uint8_t nonce[32], previous[32], replay[65], sig[65] = {1}, root[32], second_root[32];
	nu54_rental_journal_t journal = {0};
	nu54_rental_evidence_t proof;

	assert(nu54_rental_journal_open(&journal, &store, &binding, first, 0) == NU54_RENTAL_JOURNAL_ERROR);
	assert(!storage.exists);
	assert(nu54_rental_journal_open(&journal, &store, &binding, first, 1) == 0);
	assert(nu54_rental_journal_project(&journal, &proof, root) == NU54_RENTAL_JOURNAL_ERROR);
	assert(nu54_rental_journal_nonce(&journal, NU54_RENTAL_PAYMENT, payment.key, nonce) == 0);
	assert(memcmp(nonce, first, 32) == 0);
	assert(nu54_rental_journal_reserve(&journal, NU54_RENTAL_PAYMENT, &payment, 100, digest, nonce, replay) == 0);
	storage.fail = 1;
	storage.commit_on_failure = 1;
	assert(nu54_rental_journal_finish(&journal, nonce, sig) == NU54_RENTAL_JOURNAL_ERROR);
	assert(nu54_rental_journal_nonce(&journal, NU54_RENTAL_PAYMENT, payment.key, nonce) == NU54_RENTAL_JOURNAL_ERROR);

	storage.fail = 0;
	assert(nu54_rental_journal_reload(&journal) == 0);
	assert(nu54_rental_journal_nonce(&journal, NU54_RENTAL_PAYMENT, payment.key, nonce) == 0);
	assert(memcmp(nonce, first, 32) == 0);
	assert(nu54_rental_journal_reserve(&journal, NU54_RENTAL_PAYMENT, &payment, 100, digest, nonce, replay) ==
	       NU54_RENTAL_JOURNAL_REPLAY);
	assert(memcmp(sig, replay, sizeof(sig)) == 0);
	assert(nu54_rental_journal_reserve(&journal, NU54_RENTAL_PAYMENT, &payment, 101, digest, nonce, replay) ==
	       NU54_RENTAL_JOURNAL_CONFLICT);

	memcpy(previous, nonce, 32);
	assert(nu54_rental_journal_nonce(&journal, NU54_RENTAL_LIMIT, limit.key, nonce) == 0);
	assert(nonce[31] == previous[31] + 1);
	assert(nu54_rental_journal_reserve(&journal, NU54_RENTAL_LIMIT, &limit, 100, digest, nonce, replay) == 0);
	storage.fail = 1;
	storage.commit_on_failure = 0;
	assert(nu54_rental_journal_finish(&journal, nonce, sig) == NU54_RENTAL_JOURNAL_ERROR);
	storage.fail = 0;
	assert(nu54_rental_journal_open(&journal, &store, &binding, first, 0) == 0);
	assert(nu54_rental_journal_reserve(&journal, NU54_RENTAL_LIMIT, &limit, 100, digest, nonce, replay) ==
	       NU54_RENTAL_JOURNAL_BURNED);

	uint8_t *scope_fields[] = {
		binding.generation, binding.device, binding.branch, binding.account, binding.chain,
		binding.contract, binding.deployment, binding.registration, binding.measurement,
	};
	for (unsigned i = 0; i < sizeof(scope_fields) / sizeof(scope_fields[0]); ++i) {
		scope_fields[i][0] ^= 0x80;
		assert(nu54_rental_journal_open(&journal, &store, &binding, first, 1) == NU54_RENTAL_JOURNAL_CONFLICT);
		scope_fields[i][0] ^= 0x80;
	}
	assert(nu54_rental_journal_open(&journal, &store, &binding, first, 0) == 0);
	for (unsigned i = 2; i < NU54_RENTAL_JOURNAL_CAPACITY; ++i) {
		limit.key[0]++;
		assert(nu54_rental_journal_nonce(&journal, NU54_RENTAL_LIMIT, limit.key, nonce) == 0);
		assert(nu54_rental_journal_reserve(&journal, NU54_RENTAL_LIMIT, &limit, 100, digest, nonce, replay) == 0);
		assert(nu54_rental_journal_finish(&journal, nonce, NULL) == 0);
	}
	limit.key[0]++;
	assert(nu54_rental_journal_nonce(&journal, NU54_RENTAL_LIMIT, limit.key, nonce) == NU54_RENTAL_JOURNAL_FULL);
	storage.fail = 1;
	storage.commit_on_failure = 1;
	assert(nu54_rental_journal_freeze(&journal, 100, root) == NU54_RENTAL_JOURNAL_ERROR);
	storage.fail = 0;
	assert(nu54_rental_journal_reload(&journal) == 0);
	assert(nu54_rental_journal_freeze(&journal, 100, root) == 0);
	assert(nu54_rental_journal_open(&journal, &store, &binding, first, 0) == 0);
	assert(nu54_rental_journal_nonce(&journal, NU54_RENTAL_LIMIT, limit.key, nonce) == NU54_RENTAL_JOURNAL_FROZEN);
	assert(nu54_rental_journal_freeze(&journal, 101, second_root) == 0);
	assert(memcmp(root, second_root, sizeof(root)) == 0);
	/* Independently packed RJP1 prefix and rows, with no persistent key or signature fields. */
	const uint8_t expected_root[32] = {
		0x9c, 0xca, 0x7b, 0x28, 0x74, 0x7c, 0x62, 0x35,
		0xdc, 0x44, 0x62, 0xd5, 0xe1, 0x06, 0x76, 0x33,
		0x10, 0x7a, 0x02, 0x94, 0xc1, 0xe4, 0x4f, 0xe3,
		0x67, 0x76, 0x52, 0x99, 0x2e, 0xd2, 0x85, 0xb1,
	};
	assert(!memcmp(root, expected_root, sizeof(root)));
	assert(nu54_rental_journal_project(&journal, &proof, second_root) == 0);
	assert(!memcmp(root, second_root, sizeof(root)) && proof.count == NU54_RENTAL_JOURNAL_CAPACITY);
	assert(proof.entries[0].state == NU54_RENTAL_EVIDENCE_ISSUED && proof.entries[0].kind == NU54_RENTAL_PAYMENT);
	assert(!memcmp(proof.entries[0].merchant, merchant, 20) && !memcmp(proof.entries[0].order, order, 32));
	assert(proof.entries[1].state == NU54_RENTAL_EVIDENCE_BURNED && proof.entries[1].kind == NU54_RENTAL_LIMIT);
	assert(proof.entries[1].nonce[31] == 1 && proof.entries[1].expiry[7] == 100);
	verify_retirement(&journal, &storage, root);
	journal.image.entries[0].signature[1] ^= 1; /* local fault injection, never persisted */
	assert(nu54_rental_journal_project(&journal, &proof, second_root) == 0 && !memcmp(second_root, root, 32));
	journal.image.entries[0].digest[1] ^= 1;
	assert(nu54_rental_journal_project(&journal, &proof, second_root) == 0 && memcmp(second_root, root, 32));

	storage.image.next[31]++;
	assert(nu54_rental_journal_open(&journal, &store, &binding, first, 1) == NU54_RENTAL_JOURNAL_ERROR);
	assert(storage.image.frozen);
	return 0;
}
