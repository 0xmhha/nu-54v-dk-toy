#include "device_rental_journal_ps.h"

#ifdef NDEBUG
#undef NDEBUG
#endif
#include <assert.h>
#include <string.h>

#include <psa/protected_storage.h>
#include <secp256k1.h>

#include "nu54_sig.h"
#include "nu54_keccak.h"

typedef struct {
	uint8_t bytes[sizeof(nu54_rental_journal_image_t)];
	size_t length;
	uint32_t flags;
	unsigned writes;
} asset_t;

static asset_t journal_asset, marker_asset, fence_asset, handoff_asset;
static unsigned reads;
static bool qualified, retirement_qualified, short_read, bad_info;
static uint8_t actual_measurement[32] = {4};
static asset_t *fail_with_commit, *fail_without_commit;
static const device_rental_handoff_t *verified_fixture;

bool device_rental_handoff_verify(const device_rental_handoff_t *handoff, uint64_t now)
{
	/* Independent trust-provider seam. Only the externally held immutable fixture passes. */
	return verified_fixture && now >= 1100 && !memcmp(verified_fixture, handoff, sizeof(*handoff));
}

bool device_rental_journal_platform_qualified(void) { return qualified; }
bool device_rental_retirement_platform_qualified(void) { return retirement_qualified; }

bool device_rental_journal_measurement(uint8_t measurement[32])
{
	if (!qualified) return false;
	memcpy(measurement, actual_measurement, 32);
	return true;
}

static asset_t *asset(psa_storage_uid_t uid)
{
	if (uid == UINT64_C(0x4e35344c00000005)) return &journal_asset;
	if (uid == UINT64_C(0x4e35344c00000006)) return &marker_asset;
	if (uid == UINT64_C(0x4e35344c00000007)) return &fence_asset;
	assert(uid == UINT64_C(0x4e35344c00000008));
	return &handoff_asset;
}

psa_status_t psa_ps_get_info(psa_storage_uid_t uid, struct psa_storage_info_t *info)
{
	asset_t *data = asset(uid);
	reads++;
	if (bad_info) return -42;
	if (!data->length) return PSA_ERROR_DOES_NOT_EXIST;
	*info = (struct psa_storage_info_t){.size = data->length, .flags = data->flags};
	return PSA_SUCCESS;
}

psa_status_t psa_ps_get(psa_storage_uid_t uid, size_t offset, size_t len, void *out, size_t *received)
{
	asset_t *data = asset(uid);
	assert(offset == 0 && len == data->length);
	memcpy(out, data->bytes, len);
	*received = short_read ? len - 1 : len;
	return PSA_SUCCESS;
}

psa_status_t psa_ps_set(psa_storage_uid_t uid, size_t len, const void *value, uint32_t flags)
{
	asset_t *data = asset(uid);
	assert(len <= sizeof(data->bytes));
	assert(flags == (data == &fence_asset ? PSA_STORAGE_FLAG_WRITE_ONCE : PSA_STORAGE_FLAG_NONE));
	if ((data->length && data->flags == PSA_STORAGE_FLAG_WRITE_ONCE) || data == fail_without_commit)
		return -42;
	memcpy(data->bytes, value, len);
	data->flags = flags;
	data->length = len;
	data->writes++;
	return fail_with_commit == data ? -42 : PSA_SUCCESS;
}

static void verify_retirement(nu54_rental_journal_t *journal, const nu54_rental_registration_trust_t *trust)
{
	nu54_rental_retirement_authority_t authority = {
		.intent = {1}, .disposition = {2}, .request = {3}, .issued_at = 940, .expires_at = 1000,
		.disposition_final = 1,
	};
	nu54_keccak256((const uint8_t *)&trust->binding, sizeof(trust->binding), authority.binding);
	assert(device_rental_journal_retirement_fence_ps(&authority, 950) < 0 && !fence_asset.writes);
	retirement_qualified = true;
	assert(device_rental_journal_retirement_fence_ps(&authority, 950) < 0 && !fence_asset.writes);
	assert(nu54_rental_journal_freeze(journal, 930, authority.root) == 0);
	const asset_t original_marker = marker_asset, original_journal = journal_asset;
	fail_without_commit = &fence_asset;
	assert(device_rental_journal_retirement_fence_ps(&authority, 950) < 0 && !fence_asset.writes);
	assert(device_rental_journal_resume_ps(journal, &trust->binding, trust->first) == 0);
	fail_without_commit = NULL;
	fail_with_commit = &fence_asset;
	assert(device_rental_journal_retirement_fence_ps(&authority, 951) < 0 && fence_asset.writes == 1);
	fail_with_commit = NULL;
	assert(device_rental_journal_resume_ps(journal, &trust->binding, trust->first) == NU54_RENTAL_JOURNAL_FROZEN);
	assert(!journal->loaded && !journal->bound);
	assert(device_rental_journal_retirement_fence_ps(&authority, 1000) == NU54_RENTAL_JOURNAL_REPLAY);
	nu54_rental_retirement_authority_t other = authority;
	other.request[0] ^= 1;
	assert(device_rental_journal_retirement_fence_ps(&other, 960) == NU54_RENTAL_JOURNAL_CONFLICT);
	other = authority;
	other.binding[0] ^= 1;
	assert(device_rental_journal_retirement_fence_ps(&other, 960) == NU54_RENTAL_JOURNAL_CONFLICT);
	assert(!memcmp(&original_marker, &marker_asset, sizeof(marker_asset)));
	assert(!memcmp(&original_journal, &journal_asset, sizeof(journal_asset)));
	assert(fence_asset.writes == 1 && fence_asset.flags == PSA_STORAGE_FLAG_WRITE_ONCE);
	fence_asset.flags = PSA_STORAGE_FLAG_NONE;
	assert(device_rental_journal_resume_ps(journal, &trust->binding, trust->first) == NU54_RENTAL_JOURNAL_ERROR);
	assert(device_rental_journal_retirement_fence_ps(&authority, 1001) < 0);
	fence_asset.flags = PSA_STORAGE_FLAG_WRITE_ONCE;
}

static void verify_handoff(nu54_rental_journal_t *journal, const nu54_rental_registration_trust_t *trust)
{
	const asset_t history = fence_asset;
	device_rental_handoff_t initial = {.prior = trust->binding, .successor = trust->binding, .server_ack = {2}};
	initial.successor.generation[7]++;
	initial.successor.registration[0]++;
	initial.successor.account[0]++;
	memcpy(initial.receipt, "RCR1", 4);
	initial.receipt[4] = 1; /* Exact old intent pinned by the legacy fence fixture. */
	assert(device_rental_journal_stage_successor_ps(&initial, 1100) < 0 && !handoff_asset.writes);
	verified_fixture = &initial;
	device_rental_handoff_t altered = initial;
	altered.server_ack[0]++;
	assert(device_rental_journal_stage_successor_ps(&altered, 1100) < 0 && !handoff_asset.writes);
	altered = initial;
	altered.receipt[4]++;
	verified_fixture = &altered;
	assert(device_rental_journal_stage_successor_ps(&altered, 1100) < 0 && !handoff_asset.writes);
	verified_fixture = &initial;
	fail_without_commit = &handoff_asset;
	assert(device_rental_journal_stage_successor_ps(&initial, 1100) < 0 && !handoff_asset.writes);
	fail_without_commit = NULL;
	fail_with_commit = &handoff_asset;
	assert(device_rental_journal_stage_successor_ps(&initial, 1100) < 0 && handoff_asset.writes == 1);
	fail_with_commit = NULL;
	assert(device_rental_journal_stage_successor_ps(&initial, 1200) == NU54_RENTAL_JOURNAL_REPLAY);
	assert(device_rental_journal_resume_ps(journal, &initial.prior, trust->first) == NU54_RENTAL_JOURNAL_FROZEN);
	assert(device_rental_journal_resume_ps(journal, &initial.successor, trust->first) < 0);
	assert(device_rental_journal_stage_ps(&initial.successor, trust->first) < 0);
	fence_asset.length = 0; /* RH01 alone also blocks signing; a missing old asset is never rewritten. */
	assert(device_rental_journal_resume_ps(journal, &initial.prior, trust->first) == NU54_RENTAL_JOURNAL_FROZEN);
	nu54_rental_retirement_fence_t historical;
	memcpy(&historical, history.bytes, sizeof(historical));
	nu54_rental_retirement_authority_t prior_authority = {
		.issued_at = 1150, .expires_at = 1250, .disposition_final = 1,
	};
	memcpy(prior_authority.binding, historical.binding, 32);
	memcpy(prior_authority.root, historical.root, 32);
	memcpy(prior_authority.intent, historical.intent, 32);
	memcpy(prior_authority.disposition, historical.disposition, 32);
	memcpy(prior_authority.request, historical.request, 32);
	assert(device_rental_journal_retirement_fence_ps(&prior_authority, 1200) < 0 && !fence_asset.length);
	fence_asset = history;

	device_rental_handoff_t second = initial;
	second.prior = initial.successor;
	second.successor = second.prior;
	second.successor.generation[7]++;
	second.successor.registration[0]++;
	second.successor.account[0]++;
	second.receipt[4]++;
	second.server_ack[0]++;
	verified_fixture = &second;
	assert(device_rental_journal_stage_successor_ps(&second, 1200) == NU54_RENTAL_JOURNAL_CONFLICT);
	nu54_keccak256(initial.receipt, sizeof(initial.receipt), second.reclaim_receipt);
	fail_with_commit = &handoff_asset;
	assert(device_rental_journal_stage_successor_ps(&second, 1200) < 0 && handoff_asset.writes == 2);
	fail_with_commit = NULL;
	assert(device_rental_journal_stage_successor_ps(&second, 1300) == NU54_RENTAL_JOURNAL_REPLAY);
	verified_fixture = &initial;
	assert(device_rental_journal_stage_successor_ps(&initial, 1300) == NU54_RENTAL_JOURNAL_CONFLICT);
	assert(handoff_asset.writes == 2 && !memcmp(&history, &fence_asset, sizeof(history)));
	verified_fixture = NULL;
}

int main(void)
{
	nu54_rental_registration_t proposed = {
		.binding = {
			.generation = {[7] = 17}, .branch = {1}, .device = {2}, .registration = {3},
			.measurement = {4}, .deployment = {5}, .chain = {6}, .contract = {7}, .account = {8},
		},
		.first = {42},
		.challenge = {9},
		.expires_at = 1000,
	};
	nu54_rental_registration_trust_t trust = {.binding = proposed.binding, .now = 900};
	memcpy(trust.first, proposed.first, 32);
	memcpy(trust.challenge, proposed.challenge, 32);
	uint8_t private_key[32] = {1}, digest[32], public_key[65], raw[64], signature[65];
	secp256k1_context *context = secp256k1_context_create(SECP256K1_CONTEXT_NONE);
	secp256k1_pubkey key;
	secp256k1_ecdsa_signature signed_registration;
	assert(secp256k1_ec_pubkey_create(context, &key, private_key));
	size_t length = sizeof(public_key);
	assert(secp256k1_ec_pubkey_serialize(context, public_key, &length, &key, SECP256K1_EC_UNCOMPRESSED));
	nu54_address_of_pubkey(public_key, trust.operator_address);
	/* Independently packed EIP-712 vector, using JS noble Keccak. */
	const uint8_t expected[32] = {
		0x47, 0x6e, 0x55, 0x09, 0xcc, 0x5b, 0x9c, 0xb6,
		0x52, 0x9f, 0xa1, 0x78, 0x54, 0x81, 0xa1, 0x98,
		0x23, 0xc5, 0x4b, 0x79, 0x4d, 0x45, 0xf1, 0xa7,
		0xfc, 0x5a, 0xb5, 0x1c, 0xb0, 0x4e, 0x7f, 0xed,
	};
	nu54_rental_registration_digest(&proposed, digest);
	assert(!memcmp(digest, expected, sizeof(expected)));
	assert(secp256k1_ecdsa_sign(context, &signed_registration, digest, private_key, NULL, NULL));
	assert(secp256k1_ecdsa_signature_serialize_compact(context, raw, &signed_registration));
	assert(nu54_sig_finish(digest, raw, trust.operator_address, signature) == NU54_SIG_OK);
	secp256k1_context_destroy(context);
	nu54_rental_journal_t journal = {0};
	assert(device_rental_journal_activate_ps(&journal, &proposed, &trust, signature) < 0);
	assert(reads == 0 && !journal_asset.writes && !marker_asset.writes);
	qualified = true;
	assert(device_rental_journal_resume_ps(&journal, &trust.binding, trust.first) < 0);
	assert(reads == 1 && !journal_asset.writes && !marker_asset.writes);
	trust.now = 1001;
	assert(device_rental_journal_activate_ps(&journal, &proposed, &trust, signature) < 0);
	trust.now = 879;
	assert(device_rental_journal_activate_ps(&journal, &proposed, &trust, signature) < 0);
	trust.now = 900;
	trust.challenge[0] ^= 1;
	assert(device_rental_journal_activate_ps(&journal, &proposed, &trust, signature) < 0);
	trust.challenge[0] ^= 1;
	trust.first[0] ^= 1;
	assert(device_rental_journal_activate_ps(&journal, &proposed, &trust, signature) < 0);
	trust.first[0] ^= 1;
	trust.binding.measurement[0] ^= 1;
	assert(device_rental_journal_activate_ps(&journal, &proposed, &trust, signature) < 0);
	trust.binding.measurement[0] ^= 1;
	trust.operator_address[0] ^= 1;
	assert(device_rental_journal_activate_ps(&journal, &proposed, &trust, signature) < 0);
	trust.operator_address[0] ^= 1;
	assert(reads == 1 && !journal_asset.writes && !marker_asset.writes);

	bad_info = true;
	assert(device_rental_journal_stage_ps(&trust.binding, trust.first) < 0);
	assert(!journal_asset.writes && !marker_asset.writes);
	bad_info = false;
	assert(device_rental_journal_activate_ps(&journal, &proposed, &trust, signature) < 0);

	fail_with_commit = &marker_asset;
	assert(device_rental_journal_stage_ps(&trust.binding, trust.first) < 0);
	assert(!journal_asset.writes && marker_asset.writes == 1);
	fail_with_commit = NULL;
	nu54_rental_binding_t observed_binding;
	uint8_t observed_first[32];
	bool active;
	assert(device_rental_journal_marker_ps(&observed_binding, observed_first, &active) == 0 && !active);
	assert(!memcmp(&observed_binding, &trust.binding, sizeof(trust.binding)));
	assert(!memcmp(observed_first, trust.first, sizeof(trust.first)));
	assert(device_rental_journal_stage_ps(&trust.binding, trust.first) == 0 && marker_asset.writes == 1);
	trust.binding.branch[0] ^= 1;
	assert(device_rental_journal_stage_ps(&trust.binding, trust.first) < 0);
	trust.binding.branch[0] ^= 1;
	assert(device_rental_journal_resume_ps(&journal, &trust.binding, trust.first) < 0);

	fail_with_commit = &journal_asset;
	assert(device_rental_journal_activate_ps(&journal, &proposed, &trust, signature) < 0);
	assert(!journal.loaded && journal_asset.writes == 1 && marker_asset.writes == 1);
	assert(device_rental_journal_resume_ps(&journal, &trust.binding, trust.first) < 0);

	fail_with_commit = &marker_asset;
	assert(device_rental_journal_activate_ps(&journal, &proposed, &trust, signature) < 0);
	assert(!journal.loaded && !journal.bound && journal_asset.writes == 1 && marker_asset.writes == 2);
	fail_with_commit = NULL;
	assert(device_rental_journal_marker_ps(&observed_binding, observed_first, &active) == 0 && active);
	assert(device_rental_journal_resume_ps(&journal, &trust.binding, trust.first) == 0);
	actual_measurement[0] ^= 1;
	assert(device_rental_journal_marker_ps(&observed_binding, observed_first, &active) < 0);
	assert(device_rental_journal_resume_ps(&journal, &trust.binding, trust.first) < 0);
	assert(device_rental_journal_activate_ps(&journal, &proposed, &trust, signature) < 0);
	actual_measurement[0] ^= 1;
	short_read = true;
	assert(device_rental_journal_resume_ps(&journal, &trust.binding, trust.first) < 0);
	short_read = false;
	journal_asset.flags = PSA_STORAGE_FLAG_WRITE_ONCE;
	assert(device_rental_journal_activate_ps(&journal, &proposed, &trust, signature) < 0);
	journal_asset.flags = PSA_STORAGE_FLAG_NONE;
	assert(device_rental_journal_resume_ps(&journal, &trust.binding, trust.first) == 0);
	assert(journal_asset.writes == 1 && marker_asset.writes == 2);
	verify_retirement(&journal, &trust);
	assert(device_rental_journal_activate_ps(&journal, &proposed, &trust, signature) < 0);
	nu54_rental_binding_t successor = trust.binding;
	successor.generation[7]++;
	assert(device_rental_journal_stage_ps(&successor, trust.first) < 0);
	marker_asset.length = 0; /* an orphaned, still-protected journal refuses legacy classification */
	assert(device_rental_journal_marker_ps(&observed_binding, observed_first, &active) < 0);
	assert(device_rental_journal_stage_ps(&trust.binding, trust.first) < 0);
	journal_asset.length = 0; /* the write-once fence alone still bars bootstrap */
	assert(device_rental_journal_marker_ps(&observed_binding, observed_first, &active) < 0);
	assert(device_rental_journal_stage_ps(&successor, trust.first) < 0);
	assert(fence_asset.writes == 1);
	marker_asset.length = 245;
	journal_asset.length = sizeof(nu54_rental_journal_image_t);
	verify_handoff(&journal, &trust);
	return 0;
}
