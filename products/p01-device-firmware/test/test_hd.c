#include "nu54_accessory.h"
#include "nu54_sig.h"

#include <assert.h>
#include <openssl/evp.h>
#include <openssl/hmac.h>
#include <string.h>

struct store {
	bool metadata, sealed;
	nu54_hd_record record;
	uint8_t seed[NU54_HD_SEED_SIZE];
	unsigned generations;
	unsigned fail_write;
};

static int hmac(const uint8_t *key, size_t key_size, const uint8_t *message, size_t size, uint8_t out[64])
{
	unsigned used;
	return HMAC(EVP_sha512(), key, (int)key_size, message, size, out, &used) && used == 64 ? 0 : -1;
}

static int load(void *context, nu54_hd_record *out)
{
	struct store *store = context;
	if (!store->metadata) return 1;
	*out = store->record;
	return 0;
}

static int save(void *context, const nu54_hd_record *record)
{
	struct store *store = context;
	if (store->fail_write == 1) return -1;
	store->metadata = true;
	store->record = *record;
	return store->fail_write == 2 ? -1 : 0;
}

static int present(void *context)
{
	return ((struct store *)context)->sealed ? 0 : 1;
}

static int import(void *context, const uint8_t *secret, size_t length)
{
	struct store *store = context;
	if (store->sealed || length != sizeof(store->seed)) return -1;
	memcpy(store->seed, secret, length);
	store->sealed = true;
	return 0;
}

static int generate(void *context)
{
	struct store *store = context;
	uint8_t vector[32];
	for (size_t i = 0; i < sizeof(vector); ++i) vector[i] = (uint8_t)i;
	store->generations++;
	return import(store, vector, sizeof(vector));
}

static int preview(void *context, const uint8_t *seed, size_t length, uint8_t key[65], uint8_t address[20])
{
	(void)context;
	uint8_t blinding[32] = {1};
	return length == 32 ? nu54_hd_first_evm(seed, blinding, hmac, key, address) : -1;
}

static int derive(void *context, uint8_t key[65], uint8_t address[20])
{
	struct store *store = context;
	return store->sealed ? preview(context, store->seed, sizeof(store->seed), key, address) : -1;
}

static nu54_hd_backend backend(struct store *store)
{
	return (nu54_hd_backend){load, save, present, generate, preview, import, derive, store};
}

struct signer_set {EVP_PKEY *key[3];};

static int sign(void *context, enum nu54_recovery_role role, const uint8_t *data, size_t size, uint8_t *sig, size_t *used)
{
	struct signer_set *keys = context;
	EVP_MD_CTX *md = EVP_MD_CTX_new();
	bool ok = md && EVP_DigestSignInit(md, NULL, EVP_sha256(), NULL, keys->key[role]) == 1 &&
		  EVP_DigestSign(md, sig, used, data, size) == 1;
	EVP_MD_CTX_free(md);
	return ok ? 0 : -1;
}

static int verify(void *context, enum nu54_recovery_role role, const uint8_t *data, size_t size, const uint8_t *sig, size_t used)
{
	struct signer_set *keys = context;
	EVP_MD_CTX *md = EVP_MD_CTX_new();
	bool ok = md && EVP_DigestVerifyInit(md, NULL, EVP_sha256(), NULL, keys->key[role]) == 1 &&
		  EVP_DigestVerify(md, sig, used, data, size) == 1;
	EVP_MD_CTX_free(md);
	return ok ? 0 : -1;
}

int main(void)
{
	struct store first = {0}, second = {0};
	const nu54_hd_backend source = backend(&first), target = backend(&second);
	nu54_hd_slot original, restored;
	assert(nu54_hd_open(&original, &source) == 0 && nu54_hd_open(&restored, &target) == 0);
	assert(nu54_hd_backup_eligible(&original, NU54_RECOVERY_SOURCE));
	assert(!nu54_hd_backup_eligible(&original, NU54_RECOVERY_TARGET));
	original.enabled = false;
	assert(!nu54_hd_backup_eligible(&original, NU54_RECOVERY_SOURCE));
	original.enabled = true;
	const uint8_t original_id[32] = {9};
	uint8_t expected[20] = {0x91, 0x95, 0x38, 0x11, 0x6b, 0x4f, 0x25, 0xf1, 0xce, 0x01,
				0x42, 0x9f, 0xd9, 0xed, 0x79, 0x64, 0x55, 0x6b, 0xf5, 0x65}, address[20];
	first.fail_write = 2;
	assert(nu54_hd_create(&original, original_id) < 0);
	assert(!first.sealed && first.record.phase == NU54_HD_CREATING);
	first.fail_write = 0;
	assert(nu54_hd_open(&original, &source) == 0 && nu54_hd_create(&original, original_id) == 0);
	assert(nu54_hd_backup_eligible(&original, NU54_RECOVERY_SOURCE));
	assert(first.generations == 1 && nu54_hd_address(&original, address) < 0);
	assert(memcmp(original.record.address, expected, sizeof(expected)) == 0);

	nu54_recovery_context request = {.challenge = {1}, .request = {2}, .target = {3}, .owner = {4},
					  .epoch = {5}, .generation = 4, .expires = 100};
	uint8_t claim[NU54_RECOVERY_CLAIM_SIZE], receipt[NU54_RECOVERY_ENVELOPE_MAX];
	nu54_recovery_claim(claim, original_id, 1, original.record.public_key, original.record.address);
	assert(nu54_recovery_prepare(receipt, claim, &request, 90) == 0);
	struct signer_set keys;
	for (size_t i = 0; i < 3; ++i) {
		keys.key[i] = EVP_PKEY_Q_keygen(NULL, NULL, "EC", "prime256v1");
		assert(keys.key[i]);
	}
	const nu54_recovery_signers pins = {sign, verify, &keys};
	uint8_t capsule[NU54_ACCESSORY_CAPSULE_MAX] = {3}, partial[NU54_RECOVERY_ENVELOPE_MAX];
	memcpy(capsule + 1, claim, sizeof(claim));
	size_t source_size = NU54_RECOVERY_DER_MAX;
	assert(sign(&keys, NU54_RECOVERY_SOURCE, claim, sizeof(claim), capsule + 2 + sizeof(claim), &source_size) == 0);
	capsule[1 + sizeof(claim)] = (uint8_t)source_size;
	memcpy(capsule + 2 + sizeof(claim) + source_size, first.seed, sizeof(first.seed));
	size_t capsule_size = 2 + sizeof(claim) + source_size + sizeof(first.seed);
	size_t partial_length, length;
	assert(nu54_accessory_target_proof(&restored, capsule, capsule_size, &request, 90, &pins, partial, &partial_length) == 0);
	request.challenge[0]++;
	assert(nu54_accessory_complete(capsule, capsule_size, partial, partial_length, &request, 91, &pins, receipt, &length) < 0);
	request.challenge[0]--;
	assert(nu54_accessory_complete(capsule, capsule_size, partial, partial_length, &request, 91, &pins, receipt, &length) == 0);
	EVP_PKEY *key = keys.key[NU54_RECOVERY_TARGET];
	keys.key[NU54_RECOVERY_TARGET] = keys.key[NU54_RECOVERY_ACCESSORY];
	assert(nu54_recovery_verify(receipt, length, &request, 91, &pins) < 0);
	keys.key[NU54_RECOVERY_TARGET] = key;
	uint8_t mismatched[32] = {3}, mismatched_key[NU54_HD_PUBLIC_SIZE], mismatched_address[20];
	assert(target.preview(target.context, mismatched, sizeof(mismatched), mismatched_key, mismatched_address) == 0);
	assert(memcmp(mismatched_address, expected, sizeof(expected)) != 0);
	const struct store source_before_mismatch = first;
	assert(nu54_hd_restore(&restored, mismatched, sizeof(mismatched), receipt, length, &request, 91, &pins) < 0);
	assert(!second.sealed && !second.metadata && second.generations == 0);
	assert(nu54_hd_open(&original, &source) == 0 && nu54_hd_open(&restored, &target) == 0);
	assert(original.record.phase == NU54_HD_BACKUP_REQUIRED && restored.record.phase == NU54_HD_EMPTY);
	assert(nu54_hd_address(&original, address) < 0 && nu54_hd_address(&restored, address) < 0);
	assert(first.sealed == source_before_mismatch.sealed && first.generations == source_before_mismatch.generations &&
	       memcmp(first.seed, source_before_mismatch.seed, sizeof(first.seed)) == 0 &&
	       memcmp(original.record.address, expected, sizeof(expected)) == 0);
	assert(nu54_hd_restore(&restored, first.seed, sizeof(first.seed), receipt, length - 1, &request, 91, &pins) < 0);
	request.epoch[0]++;
	assert(nu54_hd_restore(&restored, first.seed, sizeof(first.seed), receipt, length, &request, 91, &pins) < 0);
	request.epoch[0]--;
	assert(nu54_recovery_verify(receipt, length, &request, 100, &pins) < 0);
	second.fail_write = 2;
	assert(nu54_hd_restore(&restored, first.seed, sizeof(first.seed), receipt, length, &request, 91, &pins) < 0);
	assert(second.record.phase == NU54_HD_RESTORING && !second.sealed);
	second.fail_write = 0;
	assert(nu54_hd_open(&restored, &target) == 0);
	assert(nu54_hd_restore(&restored, first.seed, sizeof(first.seed), receipt, length, &request, 91, &pins) == 0);
	assert(nu54_hd_mark_backup(&original, receipt, length, &request, 91, &pins) == 0);
	assert(!nu54_hd_backup_eligible(&original, NU54_RECOVERY_SOURCE));
	assert(nu54_hd_address(&original, address) == 0 && memcmp(address, expected, 20) == 0);
	assert(nu54_hd_address(&restored, address) == 0 && memcmp(address, expected, 20) == 0);
	const uint8_t payment_hash[32] = {7}, blinding[32] = {9};
	uint8_t source_rs[64], restored_rs[64], eth[65];
	assert(!nu54_hd_sign_first_evm(first.seed, blinding, hmac, address, payment_hash, source_rs));
	assert(!nu54_hd_sign_first_evm(second.seed, blinding, hmac, address, payment_hash, restored_rs));
	assert(!memcmp(source_rs, restored_rs, sizeof(source_rs)));
	assert(nu54_sig_finish(payment_hash, restored_rs, address, eth) == NU54_SIG_OK);
	address[0] ^= 1;
	assert(nu54_hd_sign_first_evm(second.seed, blinding, hmac, address, payment_hash, restored_rs));
	address[0] ^= 1;

	/* A valid source signature over a different root still cannot replace an occupied slot. */
	uint8_t substitute[NU54_ACCESSORY_CAPSULE_MAX] = {3}, other_key[65], other_address[20];
	assert(!preview(NULL, mismatched, sizeof(mismatched), other_key, other_address));
	nu54_recovery_claim(substitute + 1, original_id, 1, other_key, other_address);
	size_t other_size = NU54_RECOVERY_DER_MAX;
	assert(!sign(&keys, NU54_RECOVERY_SOURCE, substitute + 1, NU54_RECOVERY_CLAIM_SIZE,
		     substitute + 2 + NU54_RECOVERY_CLAIM_SIZE, &other_size));
	substitute[1 + NU54_RECOVERY_CLAIM_SIZE] = (uint8_t)other_size;
	memcpy(substitute + 2 + NU54_RECOVERY_CLAIM_SIZE + other_size, mismatched, sizeof(mismatched));
	assert(nu54_accessory_target_proof(&restored, substitute,
		2 + NU54_RECOVERY_CLAIM_SIZE + other_size + sizeof(mismatched), &request, 91, &pins, partial, &partial_length) < 0);
	assert(nu54_hd_address(&restored, address) == 0 && memcmp(address, expected, 20) == 0);

	assert(nu54_hd_create(&restored, request.target) < 0);
	request.epoch[0]++;
	assert(nu54_accessory_target_proof(&restored, capsule, capsule_size, &request, 92, &pins, partial, &partial_length) == 0);
	assert(nu54_accessory_complete(capsule, capsule_size, partial, partial_length, &request, 93, &pins, receipt, &length) == 0);
	second.fail_write = 1;
	assert(nu54_hd_restore(&restored, first.seed, sizeof(first.seed), receipt, length, &request, 93, &pins) == 0);
	assert(nu54_hd_address(&restored, address) == 0 && memcmp(address, expected, 20) == 0);
	second.metadata = false;
	assert(nu54_hd_open(&restored, &target) < 0 && nu54_hd_address(&restored, address) < 0);
	for (size_t i = 0; i < 3; ++i) EVP_PKEY_free(keys.key[i]);
	return 0;
}
