#include "nu54_rental_retirement.h"
#include "nu54_keccak.h"
#include "nu54_eip712.h"
#include "nu54_sig.h"

#include <string.h>

static int present(const uint8_t bytes[32])
{
	uint8_t all = 0;
	for (unsigned i = 0; i < 32; ++i) all |= bytes[i];
	return all != 0;
}

int nu54_rental_retirement_fence(const nu54_rental_journal_t *journal,
			       const nu54_rental_retirement_store_t *store,
			       const nu54_rental_retirement_authority_t *authority, uint64_t now)
{
	if (!journal || !journal->bound || !journal->loaded || !journal->store.read ||
	    !store || !store->read || !store->create || !authority || !now ||
	    !authority->issued_at || authority->expires_at <= authority->issued_at ||
	    authority->expires_at - authority->issued_at > 120 || now < authority->issued_at ||
	    authority->disposition_final != 1 || authority->pending_payments || authority->pending_limits ||
	    !present(authority->request) || !present(authority->disposition) || !present(authority->intent))
		return NU54_RENTAL_JOURNAL_ERROR;

	nu54_rental_journal_image_t durable;
	if (journal->store.read(journal->store.context, &durable, sizeof(durable)) != 0 ||
	    memcmp(&durable, &journal->image, sizeof(durable)) ||
	    memcmp(&durable.binding, &journal->expected_binding, sizeof(durable.binding)))
		return NU54_RENTAL_JOURNAL_ERROR;

	nu54_rental_evidence_t projection;
	nu54_rental_retirement_fence_t expected = {.magic = {'R', 'C', 'F', '1'}};
	if (nu54_rental_journal_project(journal, &projection, expected.root) != 0)
		return NU54_RENTAL_JOURNAL_ERROR;
	nu54_keccak256((const uint8_t *)&durable.binding, sizeof(durable.binding), expected.binding);
	if (memcmp(expected.binding, authority->binding, 32) || memcmp(expected.root, authority->root, 32))
		return NU54_RENTAL_JOURNAL_CONFLICT;
	memcpy(expected.intent, authority->intent, 32);
	memcpy(expected.disposition, authority->disposition, 32);
	memcpy(expected.request, authority->request, 32);

	nu54_rental_retirement_fence_t existing;
	int status = store->read(store->context, &existing);
	if (status == 1) {
		if (now >= authority->expires_at) return NU54_RENTAL_JOURNAL_ERROR;
		status = store->create(store->context, &expected);
		if (status == 0) return NU54_RENTAL_JOURNAL_RESERVED;
		if (status != 1) return NU54_RENTAL_JOURNAL_ERROR;
		status = store->read(store->context, &existing);
	}
	if (status != 0) return NU54_RENTAL_JOURNAL_ERROR;
	if (memcmp(existing.magic, expected.magic, sizeof(expected.magic)))
		return NU54_RENTAL_JOURNAL_ERROR;
	return memcmp(&existing, &expected, sizeof(expected)) ?
		NU54_RENTAL_JOURNAL_CONFLICT : NU54_RENTAL_JOURNAL_REPLAY;
}

static uint64_t load_u64(const uint8_t *bytes)
{
	uint64_t value = 0;
	for (unsigned i = 0; i < 8; ++i) value = (value << 8) | bytes[i];
	return value;
}

void nu54_rental_cleanup_intent_digest(const uint8_t chain[32], const uint8_t settlement[20],
				      const uint8_t raw[NU54_RENTAL_CLEANUP_INTENT_SIZE], uint8_t digest[32])
{
	static const char type[] = "RentalCleanupIntentV1(bytes32 binding,bytes32 disposition,bytes32 request,uint64 issuedAt,uint64 expiresAt)";
	uint8_t words[6][32] = {{0}}, typed[32], domain[32];
	nu54_keccak256((const uint8_t *)type, sizeof(type) - 1, words[0]);
	for (unsigned i = 0; i < 3; ++i) memcpy(words[1 + i], raw + 4 + i * 32, 32);
	memcpy(words[4] + 24, raw + 100, 8);
	memcpy(words[5] + 24, raw + 108, 8);
	nu54_keccak256(words[0], sizeof(words), typed);
	nu54_eip712_domain_separator(chain, settlement, domain);
	nu54_eip712_digest(domain, typed, digest);
}

int nu54_rental_cleanup_intent_verify(const nu54_rental_binding_t *binding, const uint8_t operator_address[20],
				      const uint8_t *raw, size_t length, nu54_rental_cleanup_intent_t *intent)
{
	static const uint8_t zero[32] = {0};
	if (!binding || !operator_address || !raw || !intent || length != NU54_RENTAL_CLEANUP_INTENT_SIZE ||
	    memcmp(binding->chain, zero, 24) || !memcmp(binding->chain + 24, zero, 8) ||
	    !memcmp(binding->contract, zero, 20) || !memcmp(operator_address, zero, 20) ||
	    memcmp(raw, "RCI1", 4) || (raw[180] != 27 && raw[180] != 28) ||
	    !present(raw + 36) || !present(raw + 68)) return -1;
	nu54_rental_cleanup_intent_t candidate = {
		.issued_at = load_u64(raw + 100), .expires_at = load_u64(raw + 108),
	};
	if (!candidate.issued_at || candidate.expires_at <= candidate.issued_at ||
	    candidate.expires_at - candidate.issued_at > 120) return -1;
	uint8_t signer[20];
	nu54_keccak256((const uint8_t *)binding, sizeof(*binding), candidate.binding);
	if (memcmp(candidate.binding, raw + 4, sizeof(candidate.binding))) return -1;
	nu54_rental_cleanup_intent_digest(binding->chain, binding->contract, raw, candidate.digest);
	if (nu54_sig_recover(candidate.digest, raw + 116, 65, signer) != NU54_SIG_OK ||
	    memcmp(signer, operator_address, sizeof(signer))) return -1;
	nu54_keccak256(raw, length, candidate.envelope);
	memcpy(candidate.disposition, raw + 36, 32);
	memcpy(candidate.request, raw + 68, 32);
	*intent = candidate;
	return 0;
}

int nu54_rental_cleanup_authorize(const nu54_rental_cleanup_intent_t *intent, const nu54_rental_cleanup_witness_t *witness,
				   uint64_t now, nu54_rental_retirement_authority_t *authority)
{
	static const char domain[] = "NU54-rental-cleanup-disposition-v1";
	if (!intent || !witness || !authority || !now || now < intent->issued_at ||
	    !witness->checked_at || witness->checked_at > now || witness->valid_until <= now ||
	    witness->valid_until - witness->checked_at > 120 ||
	    witness->pending_payments || witness->pending_limits ||
	    (witness->kind != 1 && witness->kind != 2) ||
	    !present(witness->root) || !present(witness->freeze) || !present(witness->close) ||
	    !present(witness->anchor) || !present(witness->withdrawal) ||
	    memcmp(witness->binding, intent->binding, 32) || memcmp(witness->request, intent->request, 32) ||
	    memcmp(witness->intent, intent->digest, 32) || memcmp(witness->envelope, intent->envelope, 32)) return -1;

	uint8_t preimage[sizeof(domain) + 5 * 32 + 1], digest[32];
	memcpy(preimage, domain, sizeof(domain));
	uint8_t *fields = preimage + sizeof(domain);
	memcpy(fields, intent->binding, 32);
	memcpy(fields + 32, witness->freeze, 32);
	memcpy(fields + 64, witness->close, 32);
	memcpy(fields + 96, witness->anchor, 32);
	fields[128] = witness->kind;
	memcpy(fields + 129, witness->withdrawal, 32);
	nu54_keccak256(preimage, sizeof(preimage), digest);
	if (memcmp(digest, intent->disposition, 32)) return -1;

	nu54_rental_retirement_authority_t resolved = {
		.issued_at = intent->issued_at, .expires_at = intent->expires_at, .disposition_final = 1,
	};
	memcpy(resolved.binding, intent->binding, 32);
	memcpy(resolved.root, witness->root, 32);
	memcpy(resolved.intent, intent->digest, 32);
	memcpy(resolved.disposition, intent->disposition, 32);
	memcpy(resolved.request, intent->request, 32);
	*authority = resolved;
	return 0;
}
