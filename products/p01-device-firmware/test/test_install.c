#include "nu54_install.h"
#include "nu54_sig.h"
#include "nu54_hd.h"

#include <assert.h>
#include <openssl/sha.h>
#include <openssl/ecdsa.h>
#include <openssl/evp.h>
#include <openssl/hmac.h>
#include <openssl/pem.h>
#include <openssl/rand.h>
#include <secp256k1.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

static int hash(const uint8_t *data, size_t size, uint8_t digest[32])
{
	return SHA256(data, size, digest) ? 0 : -1;
}

static EVP_PKEY *authority, *display, *review_identity;

static int verify(EVP_PKEY *key, const uint8_t *data, size_t size, const uint8_t signature[64])
{
	ECDSA_SIG *value = ECDSA_SIG_new();
	assert(value && ECDSA_SIG_set0(value, BN_bin2bn(signature, 32, NULL), BN_bin2bn(signature + 32, 32, NULL)));
	uint8_t der[80], *cursor = der;
	int length = i2d_ECDSA_SIG(value, &cursor);
	ECDSA_SIG_free(value);
	EVP_MD_CTX *ctx = EVP_MD_CTX_new();
	bool valid = ctx && length > 0 && EVP_DigestVerifyInit(ctx, NULL, EVP_sha256(), NULL, key) == 1 &&
		     EVP_DigestVerify(ctx, der, (size_t)length, data, size) == 1;
	EVP_MD_CTX_free(ctx);
	return valid ? 0 : -1;
}

static void sign_p256(EVP_PKEY *key, const uint8_t *data, size_t size, uint8_t signature[64])
{
	uint8_t der[80];
	size_t length = sizeof(der);
	EVP_MD_CTX *ctx = EVP_MD_CTX_new();
	assert(ctx && EVP_DigestSignInit(ctx, NULL, EVP_sha256(), NULL, key) == 1);
	assert(EVP_DigestSign(ctx, der, &length, data, size) == 1);
	EVP_MD_CTX_free(ctx);
	const uint8_t *cursor = der;
	ECDSA_SIG *value = d2i_ECDSA_SIG(NULL, &cursor, (long)length);
	const BIGNUM *r, *s;
	assert(value);
	ECDSA_SIG_get0(value, &r, &s);
	assert(BN_bn2binpad(r, signature, 32) == 32 && BN_bn2binpad(s, signature + 32, 32) == 32);
	ECDSA_SIG_free(value);
}

static int test_authority(const uint8_t *data, size_t length, const uint8_t signature[64])
{
	return verify(authority, data, length, signature);
}

static int test_display(const uint8_t *data, size_t length, const uint8_t signature[64])
{
	return verify(display, data, length, signature);
}

/* The simulator display provides only its public key and its proof of the exact staged bytes. */
static void require_external_display(const nu54_local_approval *approval, const uint8_t quote[NU54_V2_SIGNED_SIZE])
{
	const char *proof_path = getenv("NU54_INSTALL_DISPLAY_PROOF");
	const char *key_path = getenv("NU54_INSTALL_DISPLAY_KEY");
	const char *challenge_path = getenv("NU54_INSTALL_DISPLAY_CHALLENGE");
	if (!proof_path && !key_path && !challenge_path) return;
	assert(proof_path && key_path && challenge_path);
	FILE *proof_file = fopen(proof_path, "rb");
	uint8_t proof[64], challenge[32];
	assert(proof_file && fread(proof, sizeof(proof), 1, proof_file) == 1);
	assert(fgetc(proof_file) == EOF && !fclose(proof_file));
	FILE *challenge_file = fopen(challenge_path, "rb");
	assert(challenge_file && fread(challenge, sizeof(challenge), 1, challenge_file) == 1);
	assert(fgetc(challenge_file) == EOF && !fclose(challenge_file));
	FILE *key_file = fopen(key_path, "rb");
	assert(key_file);
	EVP_PKEY *registered = PEM_read_PUBKEY(key_file, NULL, NULL, NULL);
	assert(registered && !fclose(key_file));
	assert(EVP_PKEY_is_a(registered, "EC") && EVP_PKEY_get_bits(registered) == 256);
	EVP_PKEY *internal = display;
	display = registered;
	assert(!nu54_pilot_display(approval, quote, challenge, proof, test_display));
	display = internal;
	EVP_PKEY_free(registered);
}

static void seal(uint8_t *data, size_t length)
{
	sign_p256(authority, data, length - 64, data + length - 64);
}

static void bootstrap_tests(void)
{
	uint8_t ticket[43 + 3 + 64] = "NU54IT01";
	uint8_t challenge[NU54_INSTALL_BOOTSTRAP_PUBLIC_SIZE] = {1, 9};
	memcpy(ticket + 8, challenge, sizeof(challenge));
	ticket[42] = 3;
	ticket[43] = 0x30;
	seal(ticket, sizeof(ticket));
	const uint8_t *certificate = NULL;
	size_t length = 0;
	assert(!nu54_install_bootstrap_ticket(ticket, sizeof(ticket), challenge, test_authority, &certificate, &length));
	assert(certificate == ticket + 43 && length == 3);
	assert(nu54_install_bootstrap_ticket(ticket, sizeof(ticket), challenge, test_display, &certificate, &length));
	assert(nu54_install_bootstrap_ticket(ticket, sizeof(ticket) - 1, challenge, test_authority, &certificate, &length));
	challenge[1]++;
	assert(nu54_install_bootstrap_ticket(ticket, sizeof(ticket), challenge, test_authority, &certificate, &length));
	challenge[1]--;
	ticket[0] ^= 1;
	assert(nu54_install_bootstrap_ticket(ticket, sizeof(ticket), challenge, test_authority, &certificate, &length));
	uint8_t exported[32] = {1}, offer[NU54_INSTALL_OFFER_SIZE] = "N54E", first[32], later[32];
	assert(!nu54_install_bootstrap_offer_digest(offer, exported, hash, first));
	exported[0]++;
	assert(!nu54_install_bootstrap_offer_digest(offer, exported, hash, later) && memcmp(first, later, sizeof(first)));
}

static void byte_at_u64(uint8_t *data, uint8_t byte)
{
	memset(data, 0, 8);
	data[7] = byte;
}

static void integer_at_u64(uint8_t *data, uint64_t value)
{
	for (size_t i = 8; i > 0; --i) {
		data[i - 1] = (uint8_t)value;
		value >>= 8;
	}
}

static void storage_boot_tests(void)
{
	assert(nu54_install_boot(0) == NU54_INSTALL_BOOT_CREATE);
	unsigned marker = NU54_INSTALL_FOUND_MARKER;
	unsigned keys = NU54_INSTALL_FOUND_ID | NU54_INSTALL_FOUND_PAYER;
	unsigned prepared = marker | NU54_INSTALL_FOUND_REPLAY;
	unsigned ready = prepared | keys | NU54_INSTALL_FOUND_KEYS;
	unsigned staged = ready | NU54_INSTALL_FOUND_FLOOR;
	unsigned bound = staged | NU54_INSTALL_FOUND_COMMIT;
	assert(nu54_install_boot(marker) == NU54_INSTALL_BOOT_CREATE_REPLAY);
	assert(nu54_install_boot(prepared | NU54_INSTALL_FOUND_ID) == NU54_INSTALL_BOOT_CREATE_KEYS);
	assert(nu54_install_boot(prepared | keys) == NU54_INSTALL_BOOT_CREATE_KEYS);
	assert(nu54_install_boot(ready) == NU54_INSTALL_BOOT_READY);
	assert(nu54_install_boot(staged) == NU54_INSTALL_BOOT_FINISH);
	assert(nu54_install_boot(bound) == NU54_INSTALL_BOOT_READY);
	assert(nu54_install_boot(bound & ~NU54_INSTALL_FOUND_FLOOR) == NU54_INSTALL_BOOT_DENY);
	assert(nu54_install_boot(bound & ~NU54_INSTALL_FOUND_REPLAY) == NU54_INSTALL_BOOT_DENY);
	assert(nu54_install_boot(bound & ~NU54_INSTALL_FOUND_MARKER) == NU54_INSTALL_BOOT_DENY);
	assert(nu54_install_boot(ready & ~NU54_INSTALL_FOUND_ID) == NU54_INSTALL_BOOT_DENY);
	assert(nu54_install_boot(keys) == NU54_INSTALL_BOOT_DENY);
}

static void policy_tests(void)
{
	uint8_t record[NU54_INSTALL_HEADER_SIZE + 2 + 64] = {0};
	uint8_t challenge[32] = {1}, other_challenge[32] = {2}, identity[65] = {4,5}, payer[20] = {6};
	memcpy(record, "N54P", 4);
	memcpy(record + 4, challenge, 32);
	memcpy(record + 36, identity, 65);
	memcpy(record + 101, payer, 20);
	byte_at_u64(record + 121, 100);
	uint8_t *raw = record + 129;
	byte_at_u64(raw, 1);
	byte_at_u64(raw + 8, 3);
	byte_at_u64(raw + 16, 200);
	byte_at_u64(raw + 24, 1);
	byte_at_u64(raw + 32, 1);
	byte_at_u64(raw + 40, 50);
	raw[48] = NU54_V2_OWNER_ROLE;
	memset(raw + 49, 1, NU54_INSTALL_POLICY_SIZE - 49);
	raw[49 + 80 + 64] = 2; /* Different terminal pin. */
	raw[49 + 80 + 96] = 4;
	raw[49 + 80 + 96 + 65] = 4;
	raw[49 + 80 + 96 + 66] = 3; /* Different recovery key. */
	raw[NU54_INSTALL_POLICY_SIZE + 1] = 1;
	raw[NU54_INSTALL_POLICY_SIZE + 3] = 1;
	seal(record, sizeof(record));

	nu54_install_floor empty = {0}, floor, next;
	nu54_install_policy policy;
	size_t ca, board;
#define ACCEPT(bound_challenge, minimum, old, out) nu54_install_accept(record, sizeof(record), bound_challenge, \
	identity, payer, minimum, old, hash, test_authority, out, &policy, &ca, &board)
	assert(!ACCEPT(challenge, 99, &empty, &floor));
	assert(ca == 1 && board == 1 && floor.generation == 3 && policy.not_after == 200);
	assert(ACCEPT(other_challenge, 99, &empty, &next));
	assert(ACCEPT(challenge, 101, &empty, &next));
	record[sizeof(record) - 64] ^= 1;
	assert(ACCEPT(challenge, 99, &empty, &next));
	seal(record, sizeof(record));
	byte_at_u64(raw + 40, 51);
	seal(record, sizeof(record));
	assert(ACCEPT(challenge, 99, &floor, &next));
	byte_at_u64(raw, 2);
	byte_at_u64(raw + 8, 2);
	seal(record, sizeof(record));
	assert(ACCEPT(challenge, 99, &floor, &next));
	byte_at_u64(raw + 8, 4);
	seal(record, sizeof(record));
	assert(!ACCEPT(challenge, 99, &floor, &next));
	raw[49 + 80 + 96 + 66] ^= 1;
	seal(record, sizeof(record));
	assert(ACCEPT(challenge, 99, &floor, &next));
	raw[49 + 80 + 96 + 66] ^= 1;
	integer_at_u64(raw + 40, UINT64_C(30000000));
	seal(record, sizeof(record));
	assert(ACCEPT(challenge, 99, &floor, &next)); /* N54P remains low-value. */
	memcpy(record, "N54Q", 4);
	assert(ACCEPT(challenge, 99, &floor, &next)); /* Marker is signed. */
	seal(record, sizeof(record));
	assert(!ACCEPT(challenge, 99, &floor, &next));
	assert(policy.allowlist.commercial_testnet && policy.allowlist.max_amount == UINT64_C(30000000));
	integer_at_u64(raw + 40, UINT64_C(30000001));
	seal(record, sizeof(record));
	assert(ACCEPT(challenge, 99, &floor, &next));
	integer_at_u64(raw + 40, 50);
	seal(record, sizeof(record));
	assert(!ACCEPT(challenge, 99, &floor, &next));
	memcpy(record, "N54P", 4);
	seal(record, sizeof(record));
	assert(ACCEPT(challenge, 99, &next, &floor)); /* No cross-domain replay at same revision. */
#undef ACCEPT
}

static int read_receipt(void *context, unsigned slot, nu54_pilot_receipt *result)
{
	const nu54_pilot_receipt *disk = context;
	if (!disk[slot - 1].complete) return 1;
	*result = disk[slot - 1];
	return 0;
}

static void receipt_tests(void)
{
	nu54_pilot_receipt disk[NU54_PILOT_RECEIPTS] = {0}, result;
	const uint8_t id[16] = {1}, second[16] = {2};
	const uint8_t digest[32] = {3}, other[32] = {4};
	unsigned slot;
	assert(nu54_pilot_receipt_find(read_receipt, disk, 7, id, digest, &result, &slot) == NU54_PILOT_RECEIPT_NEW);
	assert(slot == 1);
	disk[slot - 1] = (nu54_pilot_receipt){.generation = 7, .request = {1}, .digest = {3}, .signature = {9}, .complete = 1};
	assert(nu54_pilot_receipt_find(read_receipt, disk, 7, second, other, &result, &slot) == NU54_PILOT_RECEIPT_NEW);
	assert(slot == 2);
	disk[slot - 1] = (nu54_pilot_receipt){.generation = 7, .request = {2}, .digest = {4}, .signature = {8}, .complete = 1};
	/* A reopened journal has only durable storage, with no pending request state. */
	memset(&result, 0, sizeof(result));
	slot = 0;
	assert(nu54_pilot_receipt_find(read_receipt, disk, 7, id, digest, &result, &slot) == NU54_PILOT_RECEIPT_RECORDED);
	assert(result.signature[0] == 9);
	assert(nu54_pilot_receipt_find(read_receipt, disk, 7, second, other, &result, &slot) == NU54_PILOT_RECEIPT_RECORDED);
	assert(result.signature[0] == 8);
	assert(nu54_pilot_receipt_find(read_receipt, disk, 7, id, other, &result, &slot) == NU54_PILOT_RECEIPT_ERROR);
	assert(nu54_pilot_receipt_find(read_receipt, disk, 8, id, digest, &result, &slot) == NU54_PILOT_RECEIPT_ERROR);
	assert(nu54_pilot_receipt_find(read_receipt, disk, 7, second, digest, &result, &slot) == NU54_PILOT_RECEIPT_ERROR);
}

struct payment {
	uint8_t quote[NU54_V2_SIGNED_SIZE], merchant_signer[20], secret[32];
};
struct frames {
	nu54_v2_link *board;
	nu54_v2_request request;
	uint64_t now;
	int received;
};

static int wire(void *opaque, const uint8_t *frame, size_t length)
{
	struct frames *state = opaque;
	state->received = nu54_v2_link_receive(state->board, frame, length, state->now, &state->request);
	return state->received == NU54_V2_ERROR ? -1 : 0;
}

static int present(void *opaque, const uint8_t *preimage, size_t length)
{
	return length == NU54_LOCAL_PREIMAGE_SIZE && !memcmp(opaque, preimage, length) ? 0 : -1;
}

static int sign(void *opaque, const uint8_t digest[32], uint8_t out[64])
{
	secp256k1_ecdsa_signature sig;
	secp256k1_context *context = secp256k1_context_create(SECP256K1_CONTEXT_NONE);
	int signed_ok = secp256k1_ecdsa_sign(context, &sig, digest, opaque, NULL, NULL) &&
			secp256k1_ecdsa_signature_serialize_compact(context, out, &sig);
	secp256k1_context_destroy(context);
	return signed_ok ? 0 : -1;
}

static int fixture_hmac(const uint8_t *key, size_t key_length, const uint8_t *message, size_t message_length,
			uint8_t digest[64])
{
	unsigned used;
	return HMAC(EVP_sha512(), key, (int)key_length, message, message_length, digest, &used) && used == 64 ? 0 : -1;
}

static int sign_hd_fixture(void *opaque, const uint8_t digest[32], uint8_t out[64])
{
	const struct payment *input = opaque;
	const uint8_t blind[32] = {1};
	return nu54_hd_sign_first_evm(input->secret, blind, fixture_hmac, input->quote + NU54_V2_PAYER_OFFSET + 12, digest, out);
}

static uint64_t quote_word(const uint8_t *quote, size_t offset)
{
	uint64_t value = 0;
	for (size_t i = 24; i < 32; ++i) value = (value << 8) | quote[offset + i];
	return value;
}

static nu54_install_policy commercial_policy(const struct payment *input, uint64_t cap)
{
	const uint8_t *quote = input->quote;
	uint8_t challenge[32] = {1}, identity[65] = {4, 5};
	const uint8_t *payer = quote + NU54_V2_PAYER_OFFSET + 12;
	uint8_t record[NU54_INSTALL_HEADER_SIZE + 2 + 64] = {0};
	memcpy(record, "N54Q", 4);
	memcpy(record + 4, challenge, sizeof(challenge));
	memcpy(record + 36, identity, sizeof(identity));
	memcpy(record + 101, payer, 20);
	integer_at_u64(record + 121, quote_word(quote, NU54_V2_ISSUED_AT_OFFSET));
	uint8_t *raw = record + 129;
	byte_at_u64(raw, 1);
	integer_at_u64(raw + 8, quote_word(quote, NU54_V2_PAYER_GENERATION_OFFSET));
	integer_at_u64(raw + 16, quote_word(quote, NU54_V2_EXPIRES_AT_OFFSET) + 60);
	integer_at_u64(raw + 24, quote_word(quote, NU54_V2_MERCHANT_KEY_VERSION_OFFSET));
	integer_at_u64(raw + 32, quote_word(quote, NU54_V2_BENEFICIARY_VERSION_OFFSET));
	integer_at_u64(raw + 40, cap);
	raw[48] = quote[NU54_V2_PAYER_ROLE_OFFSET + 31];
	memcpy(raw + 49, quote + NU54_V2_VERIFYING_CONTRACT_OFFSET + 12, 20);
	memcpy(raw + 69, quote + NU54_V2_ASSET_OFFSET + 12, 20);
	memcpy(raw + 89, quote + NU54_V2_BENEFICIARY_OFFSET + 12, 20);
	memcpy(raw + 109, input->merchant_signer, 20);
	memcpy(raw + 129, quote + NU54_V2_MERCHANT_ID_OFFSET, 32);
	raw[161] = 1;
	raw[193] = 2;
	raw[225] = 4; raw[226] = 1;
	raw[290] = 4; raw[291] = 2;
	raw[NU54_INSTALL_POLICY_SIZE + 1] = 1;
	raw[NU54_INSTALL_POLICY_SIZE + 3] = 1;
	seal(record, sizeof(record));
	nu54_install_floor current = {0}, resulting;
	nu54_install_policy policy;
	size_t ca, board;
	assert(!nu54_install_accept(record, sizeof(record), challenge, identity, payer, 0, &current,
		hash, test_authority, &resulting, &policy, &ca, &board));
	return policy;
}

static int read_refusal(void *context, unsigned slot, nu54_pilot_refusal *out)
{
	FILE *storage = context;
	if (fseek(storage, (long)((slot - 1) * sizeof(*out)), SEEK_SET)) return -1;
	if (!fread(out, sizeof(*out), 1, storage)) return feof(storage) ? 1 : -1;
	return 0;
}

static void refuse_original(nu54_local_approval *approval, nu54_v2_link *board,
			    const uint8_t *quote, uint64_t now)
{
	FILE *storage = tmpfile();
	assert(storage);
	nu54_pilot_refusal record;
	unsigned slot;
	assert(!nu54_pilot_refuse(approval, board, quote, now, 1, hash, &record));
	assert(!approval->pending);
	assert(nu54_pilot_refusal_find(read_refusal, storage, now, record.request, record.digest, &slot)
	       == NU54_PILOT_RECEIPT_NEW && slot == 1);
	assert(!fseek(storage, 0, SEEK_SET));
	assert(fwrite(&record, sizeof(record), 1, storage) == 1 && !fflush(storage));
	/* The resolver has only protected storage after reboot, with no session state. */
	memset(&record, 0, sizeof(record));
	assert(!read_refusal(storage, 1, &record));
	assert(nu54_pilot_refusal_find(read_refusal, storage, now + 1, record.request, record.digest, &slot)
	       == NU54_PILOT_RECEIPT_RECORDED);
	uint8_t other_id[16], other_digest[32];
	memcpy(other_id, record.request, sizeof(other_id)); other_id[0] ^= 1;
	memcpy(other_digest, record.digest, sizeof(other_digest)); other_digest[0] ^= 1;
	assert(nu54_pilot_refusal_find(read_refusal, storage, now + 1, other_id, record.digest, &slot)
	       == NU54_PILOT_RECEIPT_ERROR);
	assert(nu54_pilot_refusal_find(read_refusal, storage, now + 1, record.request, other_digest, &slot)
	       == NU54_PILOT_RECEIPT_ERROR);
	assert(nu54_pilot_refusal_find(read_refusal, storage, now + 1, other_id, other_digest, &slot)
	       == NU54_PILOT_RECEIPT_NEW && slot == 2);
	assert(nu54_pilot_refusal_find(read_refusal, storage, record.expires, other_id, other_digest, &slot)
	       == NU54_PILOT_RECEIPT_NEW && slot == 1);
	assert(!fclose(storage));
}

static void payment_test(const struct payment *input, nu54_v2_profile route, const uint8_t *different,
			 nu54_pilot_receipt disk[NU54_PILOT_RECEIPTS], uint8_t *exported_signature, bool hd, bool commercial, bool refuse_review)
{
	const uint8_t *quote = input->quote;
	uint64_t now = quote_word(quote, NU54_V2_ISSUED_AT_OFFSET) + 1;
	uint64_t expiry = quote_word(quote, NU54_V2_EXPIRES_AT_OFFSET);
	nu54_install_policy policy = {.not_after = expiry + 60, .payer_generation = 1, .merchant_key_version = 1,
		.beneficiary_version = 1, .payer_role = quote[NU54_V2_PAYER_ROLE_OFFSET + 31], .allowlist.max_amount = 100};
	memcpy(policy.merchant, quote + NU54_V2_MERCHANT_ID_OFFSET, 32);
	memcpy(policy.merchant_signer, input->merchant_signer, 20);
	memcpy(policy.allowlist.contract, quote + NU54_V2_VERIFYING_CONTRACT_OFFSET + 12, 20);
	memcpy(policy.allowlist.token, quote + NU54_V2_ASSET_OFFSET + 12, 20);
	memcpy(policy.allowlist.beneficiary, quote + NU54_V2_BENEFICIARY_OFFSET + 12, 20);
	if (commercial) policy = commercial_policy(input, NU54_PILOT_TESTUSDC2_CAP);
	const uint8_t *payer = quote + NU54_V2_PAYER_OFFSET + 12;
	nu54_v2_link board, origin;
	nu54_v2_journal_entry entries[2][4] = {0};
	nu54_v2_journal board_journal = {entries[0], 4, hash, 0}, client_journal = {entries[1], 4, hash, 0};
	uint8_t epoch[16] = {1}, peer[32] = {2};
	assert(!nu54_v2_link_init(&board, route, NU54_V2_BOARD, epoch, peer, &board_journal));
	assert(!nu54_v2_link_init(&origin, route, route == NU54_V2_MANAGEMENT ? NU54_V2_OWNER : NU54_V2_TERMINAL,
		epoch, peer, &client_journal));
	struct frames work = {.board = &board, .now = now};
	nu54_v2_request request = {.command = NU54_V2_PAY_PREPARE, .expires = expiry,
		.payload = quote, .payload_length = NU54_V2_SIGNED_SIZE};
	memcpy(request.id, quote + NU54_V2_ORDER_ID_OFFSET, sizeof(request.id));
	assert(!nu54_v2_link_send(&origin, &request, wire, &work) && work.received == NU54_V2_NEW);
	nu54_v2_quote_policy order;
	assert(!nu54_install_quote(&policy, work.request.payload, work.request.payload_length, now, &order));

	nu54_local_approval approval = {0};
	uint8_t preimage[NU54_LOCAL_PREIMAGE_SIZE];
	assert(nu54_pilot_approval_preimage(different, preimage));
	assert(nu54_pilot_stage(&approval, &board, &work.request, &order, &policy.allowlist, payer,
		policy.payer_generation, 1, present, preimage));
	assert(nu54_pilot_approval_preimage(quote, preimage));
	if (commercial) {
		uint64_t amount = quote_word(quote, NU54_V2_AMOUNT_OFFSET);
		assert(amount > NU54_PILOT_LEGACY_CAP);
		nu54_install_policy lower = commercial_policy(input, amount - 1);
		assert(nu54_pilot_stage(&approval, &board, &work.request, &order, &lower.allowlist, payer,
			policy.payer_generation, 1, present, preimage));
		nu54_pilot_policy unsigned_list = policy.allowlist;
		unsigned_list.commercial_testnet = false;
		assert(nu54_pilot_stage(&approval, &board, &work.request, &order, &unsigned_list, payer,
			policy.payer_generation, 1, present, preimage));
		nu54_pilot_policy wrong_token = policy.allowlist;
		wrong_token.token[0] ^= 1;
		assert(nu54_pilot_stage(&approval, &board, &work.request, &order, &wrong_token, payer,
			policy.payer_generation, 1, present, preimage));
	}
	assert(!nu54_pilot_stage(&approval, &board, &work.request, &order, &policy.allowlist, payer,
		policy.payer_generation, 1, present, preimage));
	uint8_t challenge[32] = {5}, proof[64], statement[4 + 32 + NU54_V2_SIGNED_SIZE];
	memcpy(statement, "N54D", 4);
	memcpy(statement + 4, challenge, sizeof(challenge));
	memcpy(statement + 36, different, NU54_V2_SIGNED_SIZE);
	sign_p256(display, statement, sizeof(statement), proof);
	assert(nu54_pilot_display(&approval, quote, challenge, proof, test_display));
	assert(nu54_pilot_display(&approval, different, challenge, proof, test_display));
	memcpy(statement + 36, quote, NU54_V2_SIGNED_SIZE);
	sign_p256(authority, statement, sizeof(statement), proof);
	assert(nu54_pilot_display(&approval, quote, challenge, proof, test_display));
	sign_p256(display, statement, sizeof(statement), proof);
	assert(!nu54_pilot_display(&approval, quote, challenge, proof, test_display));
	challenge[0] ^= 1;
	assert(nu54_pilot_display(&approval, quote, challenge, proof, test_display));
	if (!exported_signature && route == NU54_V2_MANAGEMENT) {
		assert(!nu54_pilot_verify_quote(quote, NU54_V2_SIGNED_SIZE, &order, &policy.allowlist,
					      payer, policy.payer_generation));
		assert(nu54_pilot_verify_quote(different, NU54_V2_SIGNED_SIZE, &order, &policy.allowlist,
					     payer, policy.payer_generation));
		nu54_pilot_review_binding reviewed = {0};
		nu54_v2_request changed = request;
		changed.payload = different;
		assert(nu54_pilot_review_consume(&reviewed, &board, &request, hash));
		assert(!nu54_pilot_review_bind(&reviewed, &board, request.id, quote, hash));
		assert(nu54_pilot_review_consume(&reviewed, &board, &changed, hash));
		assert(nu54_pilot_review_consume(&reviewed, &board, &request, hash));
		assert(!nu54_pilot_review_bind(&reviewed, &board, request.id, quote, hash));
		nu54_v2_link reconnected = board;
		reconnected.epoch[0] ^= 1;
		assert(nu54_pilot_review_consume(&reviewed, &reconnected, &request, hash));
		assert(!nu54_pilot_review_bind(&reviewed, &board, request.id, quote, hash));
		assert(!nu54_pilot_review_consume(&reviewed, &board, &request, hash));
		assert(nu54_pilot_review_consume(&reviewed, &board, &request, hash));
		nu54_local_approval rejected = approval;
		refuse_original(&rejected, &board, quote, now);
		if (refuse_review) return;
	}
	if (exported_signature) require_external_display(&approval, quote);
	if (exported_signature && getenv("NU54_INSTALL_TEST_REFUSE")) {
		assert(getenv("NU54_INSTALL_DISPLAY_PROOF"));
		assert(nu54_pilot_complete(&approval, &board, policy.payer_generation, now, 1,
			NU54_LOCAL_REJECT, payer, sign, (void *)input->secret, exported_signature));
		assert(!approval.pending);
		return;
	}
	nu54_pilot_receipt receipt = {.generation = policy.payer_generation, .complete = 1};
	memcpy(receipt.request, request.id, sizeof(receipt.request));
	assert(!hash(approval.intent.preimage, sizeof(approval.intent.preimage), receipt.digest));
	nu54_pilot_receipt previous;
	unsigned slot;
	assert(nu54_pilot_receipt_find(read_receipt, disk, receipt.generation, receipt.request, receipt.digest, &previous, &slot)
	       == NU54_PILOT_RECEIPT_NEW);
	assert(!nu54_pilot_complete(&approval, &board, policy.payer_generation, now, 1, NU54_LOCAL_APPROVE, payer,
		hd ? sign_hd_fixture : sign, hd ? (void *)input : (void *)input->secret, receipt.signature));
	assert(receipt.signature[64] == 27 || receipt.signature[64] == 28);
	assert(nu54_pilot_complete(&approval, &board, policy.payer_generation, now, 1, NU54_LOCAL_APPROVE, payer,
		hd ? sign_hd_fixture : sign, hd ? (void *)input : (void *)input->secret, previous.signature));
	disk[slot - 1] = receipt;
	if (exported_signature) memcpy(exported_signature, receipt.signature, sizeof(receipt.signature));
}

static void receipt_rollover(const uint8_t initial[NU54_V2_UNSIGNED_SIZE])
{
	nu54_pilot_receipt disk[NU54_PILOT_RECEIPTS] = {0}, found;
	uint8_t quote[NU54_V2_UNSIGNED_SIZE], preimage[NU54_LOCAL_PREIMAGE_SIZE], proof[64];
	uint8_t statement[NU54_PILOT_RECLAIM_BODY_SIZE], device[32] = {7};
	memcpy(quote, initial, sizeof(quote));
	uint64_t expiry = quote_word(quote, NU54_V2_EXPIRES_AT_OFFSET);
	const uint64_t settled = expiry + NU54_PILOT_RECLAIM_HOLD_SECONDS;
	for (unsigned index = 1; index <= 100; ++index) {
		nu54_pilot_receipt entry = {.generation = 1, .request = {(uint8_t)index}, .complete = 1};
		quote[NU54_V2_QUOTE_ID_OFFSET] = (uint8_t)index;
		assert(nu54_pilot_approval_preimage(quote, preimage) && !hash(preimage, sizeof(preimage), entry.digest));
		unsigned slot;
		assert(nu54_pilot_receipt_find(read_receipt, disk, 1, entry.request, entry.digest, &found, &slot)
		       == NU54_PILOT_RECEIPT_NEW);
		disk[slot - 1] = entry;
		if (index <= 15) continue; /* Preserve the first 15 unsettled approvals indefinitely. */
		assert(nu54_pilot_receipt_find(read_receipt, disk, 1, entry.request, entry.digest, &found, &slot)
		       == NU54_PILOT_RECEIPT_RECORDED);
		assert(nu54_pilot_reclaim_body(device, &found, quote, settled - 1, 1, hash, statement));
		assert(nu54_pilot_reclaim_body(device, &found, quote, settled, 0, hash, statement));
		quote[NU54_V2_QUOTE_ID_OFFSET] ^= 0x80;
		assert(nu54_pilot_reclaim_body(device, &found, quote, settled, 1, hash, statement));
		quote[NU54_V2_QUOTE_ID_OFFSET] ^= 0x80;
		assert(!nu54_pilot_reclaim_body(device, &found, quote, settled, 1, hash, statement));
		sign_p256(display, statement, sizeof(statement), proof);
		assert(test_authority(statement, sizeof(statement), proof));
		sign_p256(authority, statement, sizeof(statement), proof);
		assert(!test_authority(statement, sizeof(statement), proof));
		statement[4] ^= 1; /* Another board cannot apply the authenticated disposition. */
		assert(test_authority(statement, sizeof(statement), proof));
		const unsigned tail = NU54_PILOT_RECEIPTS - 1;
		assert(!memcmp(disk[tail].request, found.request, sizeof(found.request)));
		memset(&disk[tail], 0, sizeof(disk[tail]));
		assert(nu54_pilot_receipt_find(read_receipt, disk, 1, entry.request, entry.digest, &found, &slot)
		       == NU54_PILOT_RECEIPT_NEW && slot == NU54_PILOT_RECEIPTS);
	}
	for (size_t i = 0; i < 15; ++i)
		assert(disk[i].complete && disk[i].request[0] == i + 1);
}

static int sign_review(const uint8_t digest[32], uint8_t signature[64])
{
	EVP_PKEY_CTX *ctx = EVP_PKEY_CTX_new(review_identity, NULL);
	uint8_t der[80];
	size_t size = sizeof(der);
	assert(ctx && EVP_PKEY_sign_init(ctx) == 1 && EVP_PKEY_CTX_set_signature_md(ctx, EVP_sha256()) == 1);
	assert(EVP_PKEY_sign(ctx, der, &size, digest, 32) == 1);
	EVP_PKEY_CTX_free(ctx);
	const uint8_t *cursor = der;
	ECDSA_SIG *value = d2i_ECDSA_SIG(NULL, &cursor, (long)size);
	const BIGNUM *r, *v;
	assert(value);
	ECDSA_SIG_get0(value, &r, &v);
	assert(BN_bn2binpad(r, signature, 32) == 32 && BN_bn2binpad(v, signature + 32, 32) == 32);
	ECDSA_SIG_free(value);
	return 0;
}

static void rental_review_test(const struct payment *input)
{
	const uint8_t *quote = input->quote;
	uint64_t now = quote_word(quote, NU54_V2_ISSUED_AT_OFFSET) + 1;
	uint64_t expiry = quote_word(quote, NU54_V2_EXPIRES_AT_OFFSET);
	nu54_install_policy policy = commercial_policy(input, NU54_PILOT_TESTUSDC2_CAP);
	assert(policy.payer_role == NU54_V2_RENTER_ROLE);
	review_identity = EVP_EC_gen("prime256v1");
	uint8_t spki[91], *cursor = spki, board_pin[32], terminal_pin[32] = {2}, epoch[16] = {1};
	assert(review_identity && i2d_PUBKEY(review_identity, &cursor) == sizeof(spki));
	assert(!hash(spki, sizeof(spki), board_pin));
	nu54_v2_link board, terminal;
	nu54_v2_journal_entry entries[2][4] = {0};
	nu54_v2_journal board_journal = {entries[0], 4, hash, 0}, client_journal = {entries[1], 4, hash, 0};
	assert(!nu54_v2_link_init(&board, NU54_V2_PAYMENT, NU54_V2_BOARD, epoch, terminal_pin, &board_journal));
	assert(!nu54_v2_link_init(&terminal, NU54_V2_PAYMENT, NU54_V2_TERMINAL, epoch, board_pin, &client_journal));
	uint8_t envelope[NU54_V2_RENTAL_REVIEW_PREPARE_SIZE] = {'W','1','0','Q',3};
	memcpy(envelope + 36, quote, NU54_V2_SIGNED_SIZE);
	nu54_v2_request request = {.command = NU54_V2_PAY_PREPARE, .id = {4}, .expires = expiry,
		.payload = envelope, .payload_length = sizeof(envelope)};
	struct frames work = {.board = &board, .now = now};
	assert(!nu54_v2_link_send(&terminal, &request, wire, &work) && work.received == NU54_V2_NEW);
	assert(nu54_v2_rental_review_prepare(work.request.payload, work.request.payload_length));
	const uint8_t *challenge = work.request.payload + 4;
	work.request.payload += 36;
	work.request.payload_length -= 36;
	nu54_v2_quote_policy order;
	assert(!nu54_install_quote(&policy, work.request.payload, work.request.payload_length, now, &order));
	nu54_local_approval approval = {0};
	uint8_t receipt[NU54_V2_RENTAL_REVIEW_RECEIPT_SIZE], preimage[NU54_LOCAL_PREIMAGE_SIZE];
	assert(nu54_pilot_approval_preimage(quote, preimage));
	assert(nu54_pilot_rental_review(&approval, &board, &work.request, challenge, board_pin, now, hash, sign_review, receipt));
	assert(!nu54_pilot_stage(&approval, &board, &work.request, &order, &policy.allowlist,
		quote + NU54_V2_PAYER_OFFSET + 12, policy.payer_generation, 1, present, preimage));
	assert(nu54_pilot_rental_review(&approval, &board, &work.request, challenge, board_pin, expiry, hash, sign_review, receipt));
	assert(!nu54_pilot_rental_review(&approval, &board, &work.request, challenge, board_pin, now, hash, sign_review, receipt));
	nu54_v2_request response = {.command = NU54_V2_RENTAL_REVIEW, .id = {4}, .expires = expiry,
		.payload = receipt, .payload_length = sizeof(receipt)};
	struct frames reply = {.board = &terminal, .now = now};
	assert(nu54_v2_link_send(&terminal, &response, wire, &work)); /* Terminal does not issue board receipts. */
	response.payload_length--;
	assert(nu54_v2_link_send(&board, &response, wire, &reply));
	response.payload_length++;
	assert(!nu54_v2_link_send(&board, &response, wire, &reply) && reply.received == NU54_V2_NEW);
	assert(reply.request.command == NU54_V2_RENTAL_REVIEW && approval.pending);
	assert(nu54_v2_link_send(&board, &response, wire, &reply)); /* One per pending request. */
	/* A disconnect cannot turn the same request into a fresh review or a prompt. */
	epoch[0]++;
	assert(!nu54_v2_link_init(&board, NU54_V2_PAYMENT, NU54_V2_BOARD, epoch, terminal_pin, &board_journal));
	assert(!nu54_v2_link_init(&terminal, NU54_V2_PAYMENT, NU54_V2_TERMINAL, epoch, board_pin, &client_journal));
	assert(!nu54_v2_link_send(&terminal, &request, wire, &work) && work.received == NU54_V2_RESUME);
	assert(nu54_v2_link_send(&board, &response, wire, &reply));
	assert(fwrite(spki + 26, 65, 1, stdout) == 1 && fwrite(receipt, sizeof(receipt), 1, stdout) == 1);
	EVP_PKEY_free(review_identity);
}

static void read_payment(const char *filename, struct payment *result)
{
	FILE *input = fopen(filename, "rb");
	assert(input && fread(result, 1, sizeof(*result), input) == sizeof(*result));
	assert(fgetc(input) == EOF);
	fclose(input);
}

static size_t read_bytes(const char *path, uint8_t *output, size_t capacity)
{
	FILE *file = fopen(path, "rb");
	assert(file);
	size_t used = fread(output, 1, capacity, file);
	assert(fgetc(file) == EOF && !ferror(file));
	fclose(file);
	return used;
}

static int accept_grant(const uint8_t offer[217], const uint8_t *grant, size_t length,
			const nu54_install_floor *previous, nu54_install_floor *current)
{
	nu54_install_policy policy;
	size_t ca_size, board_size;
	return nu54_install_accept(grant, length, offer + 4, offer + 36, offer + 101, 0, previous,
				   hash, test_authority, current, &policy, &ca_size, &board_size);
}

static int real_grants(char **paths)
{
	FILE *root = fopen(paths[0], "rb");
	assert(root);
	authority = PEM_read_PUBKEY(root, NULL, NULL, NULL);
	fclose(root);
	assert(authority);
	uint8_t offer[3][217], grant[3][NU54_INSTALL_BUNDLE_MAX];
	size_t length[3];
	for (size_t i = 0; i < 3; ++i) {
		assert(read_bytes(paths[1 + i * 2], offer[i], sizeof(offer[i])) == sizeof(offer[i]));
		length[i] = read_bytes(paths[2 + i * 2], grant[i], sizeof(grant[i]));
	}
	nu54_install_floor empty = {0}, first, second, again;
	assert(memcmp(offer[0] + 36, offer[1] + 36, 65) && memcmp(offer[0] + 101, offer[1] + 101, 20));
	assert(!accept_grant(offer[0], grant[0], length[0], &empty, &first));
	assert(accept_grant(offer[1], grant[0], length[0], &empty, &second));
	assert(!accept_grant(offer[1], grant[1], length[1], &empty, &second));
	assert(memcmp(offer[0] + 4, offer[2] + 4, 32) && !memcmp(offer[0] + 36, offer[2] + 36, 85));
	assert(accept_grant(offer[2], grant[0], length[0], &first, &again));
	assert(!accept_grant(offer[2], grant[2], length[2], &first, &again));
	assert(again.revision == first.revision && again.generation == first.generation && again.time >= first.time);
	EVP_PKEY_free(authority);
	return 0;
}

int main(int argc, char **argv)
{
	if (argc == 9 && !strcmp(argv[1], "--grants")) return real_grants(argv + 2);
	if (argc == 2 && !strcmp(argv[1], "--display-challenge")) {
		uint8_t challenge[32];
		assert(RAND_bytes(challenge, sizeof(challenge)) == 1);
		assert(fwrite(challenge, sizeof(challenge), 1, stdout) == 1);
		return 0;
	}
	authority = EVP_PKEY_Q_keygen(NULL, NULL, "EC", "prime256v1");
	display = EVP_PKEY_Q_keygen(NULL, NULL, "EC", "prime256v1");
	assert(authority && display && !EVP_PKEY_eq(authority, display));
	bootstrap_tests();
	storage_boot_tests();
	policy_tests();
	receipt_tests();
	if (argc == 4 && !strcmp(argv[1], "--refuse-review")) {
		struct payment original, other;
		read_payment(argv[2], &original);
		read_payment(argv[3], &other);
		nu54_pilot_receipt disk[NU54_PILOT_RECEIPTS] = {0};
		payment_test(&original, NU54_V2_MANAGEMENT, other.quote, disk, NULL, true, false, true);
		const nu54_pilot_receipt empty[NU54_PILOT_RECEIPTS] = {0};
		assert(!memcmp(disk, empty, sizeof(disk)));
		return 0;
	}
	if (argc == 3 && !strcmp(argv[1], "--review-commercial-testnet")) {
		struct payment input;
		read_payment(argv[2], &input);
		const uint8_t *quote = input.quote;
		nu54_install_policy policy = commercial_policy(&input, NU54_PILOT_TESTUSDC2_CAP);
		nu54_v2_quote_policy order;
		uint64_t now = (uint64_t)time(NULL);
		return nu54_install_quote(&policy, quote, NU54_V2_SIGNED_SIZE, now, &order) ||
		       nu54_pilot_verify_quote(quote, NU54_V2_SIGNED_SIZE, &order, &policy.allowlist,
					       quote + NU54_V2_PAYER_OFFSET + 12, policy.payer_generation);
	}
	if (argc == 3 && !strcmp(argv[1], "--review-rental")) {
		struct payment input;
		read_payment(argv[2], &input);
		rental_review_test(&input);
	} else if (argc == 4 && (!strcmp(argv[1], "--approve-one") || !strcmp(argv[1], "--approve-commercial-testnet"))) {
		struct payment input;
		read_payment(argv[3], &input);
		assert(!strcmp(argv[2], "management") || !strcmp(argv[2], "payment"));
		nu54_v2_profile profile = !strcmp(argv[2], "management") ? NU54_V2_MANAGEMENT : NU54_V2_PAYMENT;
		uint8_t different[NU54_V2_SIGNED_SIZE], signature[65];
		memcpy(different, input.quote, sizeof(different));
		different[NU54_V2_ORDER_ID_OFFSET + 31] ^= 1;
		nu54_pilot_receipt disk[NU54_PILOT_RECEIPTS] = {0};
		payment_test(&input, profile, different, disk, signature, false, !strcmp(argv[1], "--approve-commercial-testnet"), false);
		if (!getenv("NU54_INSTALL_TEST_REFUSE"))
			assert(fwrite(signature, sizeof(signature), 1, stdout) == 1);
	} else if (argc == 4) {
		struct payment first, second, other_order;
		read_payment(argv[1], &first);
		read_payment(argv[2], &second);
		read_payment(argv[3], &other_order);
		receipt_rollover(first.quote);
		assert(memcmp(first.quote + NU54_V2_TENANT_COMMITMENT_OFFSET,
			second.quote + NU54_V2_TENANT_COMMITMENT_OFFSET, 32));
		assert(memcmp(first.quote + NU54_V2_BRANCH_COMMITMENT_OFFSET,
			second.quote + NU54_V2_BRANCH_COMMITMENT_OFFSET, 32));
		assert(memcmp(first.quote + NU54_V2_PAYER_OFFSET, second.quote + NU54_V2_PAYER_OFFSET, 32));
		assert(!memcmp(first.quote + NU54_V2_PAYER_OFFSET, other_order.quote + NU54_V2_PAYER_OFFSET, 32));
		assert(!memcmp(first.quote + NU54_V2_AMOUNT_OFFSET, other_order.quote + NU54_V2_AMOUNT_OFFSET, 32));
		assert(!memcmp(first.quote + NU54_V2_MERCHANT_ID_OFFSET, other_order.quote + NU54_V2_MERCHANT_ID_OFFSET, 32));
		assert(memcmp(first.quote + NU54_V2_ORDER_ID_OFFSET, other_order.quote + NU54_V2_ORDER_ID_OFFSET, 32));
		nu54_pilot_receipt disks[2][NU54_PILOT_RECEIPTS] = {0};
		payment_test(&first, NU54_V2_MANAGEMENT, other_order.quote, disks[0], NULL, true, false, false);
		payment_test(&second, NU54_V2_PAYMENT, first.quote, disks[1], NULL, false, false, false);
		payment_test(&other_order, NU54_V2_PAYMENT, first.quote, disks[0], NULL, true, false, false);
		for (size_t i = 0; i < 2; ++i) {
			nu54_pilot_receipt saved;
			unsigned slot;
			const nu54_pilot_receipt *expected = &disks[0][i];
			assert(nu54_pilot_receipt_find(read_receipt, disks[0], 1, expected->request, expected->digest,
						      &saved, &slot) == NU54_PILOT_RECEIPT_RECORDED);
			assert(!memcmp(saved.signature, expected->signature, sizeof(saved.signature)));
		}
	} else assert(argc == 1);
	EVP_PKEY_free(display);
	EVP_PKEY_free(authority);
	return 0;
}
