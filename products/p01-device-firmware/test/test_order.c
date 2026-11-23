/* Order payment signing against the vector the real contracts accepted
 * (products/p06-stablenet-contracts/test/OrderLockVector.t.sol). */
#include "nu54_keccak.h"
#include "nu54_order.h"
#include "nu54_sig.h"

#include <secp256k1.h>
#include <secp256k1_recovery.h>

#include <assert.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define NOW UINT64_C(1800000000)
#define GENERATION 1
#define EARLIEST (NOW - 2)
static uint64_t earliest = EARLIEST;

static uint8_t unsigned_quote[NU54_V2_UNSIGNED_SIZE], merchant_signature[65], merchant_signer[20], confirmer[20];
static uint8_t expected_preimage[NU54_LOCAL_PREIMAGE_SIZE], payer_signature[65];
static uint8_t quote[NU54_V2_SIGNED_SIZE];
static nu54_pilot_policy allowlist;
static nu54_v2_quote_policy order;
static uint8_t payer[20];

static void load(FILE *file, const char *key, uint8_t *out, size_t length)
{
	static char line[2 * NU54_V2_UNSIGNED_SIZE + 64];
	rewind(file);
	while (fgets(line, sizeof(line), file)) {
		size_t size = strlen(key);
		if (strncmp(line, key, size) || line[size] != '=') continue;
		const char *hex = line + size + 3; /* =0x */
		assert(strcspn(hex, "\n") == length * 2);
		for (size_t i = 0; i < length; ++i) {
			unsigned byte;
			assert(sscanf(hex + 2 * i, "%2x", &byte) == 1);
			out[i] = (uint8_t)byte;
		}
		return;
	}
	assert(!"vector key missing");
}

static int shown;
static int present(void *context, const uint8_t *preimage, size_t length)
{
	(void)context;
	assert(length == NU54_LOCAL_PREIMAGE_SIZE && !memcmp(preimage, expected_preimage, length));
	++shown;
	return 0;
}

static nu54_v2_link channel(void)
{
	nu54_v2_link link = {.profile = NU54_V2_MANAGEMENT, .local = NU54_V2_BOARD, .peer = NU54_V2_OWNER,
			     .pending_expiry = NOW + 60};
	memset(link.pending_id, 5, sizeof(link.pending_id));
	memset(link.epoch, 6, sizeof(link.epoch));
	return link;
}

static nu54_v2_request request(void)
{
	nu54_v2_request value = {.command = NU54_V2_PAY_PREPARE, .expires = NOW + 60};
	memset(value.id, 5, sizeof(value.id));
	return value;
}

static int stage(nu54_v2_link link, nu54_v2_request req, const uint8_t *phone, const uint8_t *signed_quote,
		 nu54_pilot_policy policy, const uint8_t *account, uint64_t generation)
{
	nu54_local_approval approval = {0};
	return nu54_order_stage(&approval, &link, &req, phone, signed_quote, &order, &policy, account, generation, earliest, 9,
				present, NULL);
}

static void test_signature_matches_the_contract(void)
{
	uint8_t preimage[NU54_LOCAL_PREIMAGE_SIZE], digest[32], address[20];
	assert(nu54_order_preimage(quote, confirmer, preimage));
	assert(!memcmp(preimage, expected_preimage, sizeof(preimage)));
	nu54_keccak256(preimage, sizeof(preimage), digest);
	assert(nu54_sig_recover(digest, payer_signature, 65, address) == NU54_SIG_OK);
	assert(!memcmp(address, payer, 20));

	uint8_t other[NU54_LOCAL_PREIMAGE_SIZE], legacy[NU54_LOCAL_PREIMAGE_SIZE], phone[20];
	memcpy(phone, confirmer, 20);
	phone[19] ^= 1;
	assert(nu54_order_preimage(quote, phone, other) && memcmp(other, preimage, sizeof(other)));
	assert(nu54_pilot_approval_preimage(quote, legacy) && memcmp(legacy, preimage, sizeof(legacy)));
	/* Every quote field changes what is signed. */
	for (size_t word = 0; word < NU54_V2_FIELD_COUNT; ++word) {
		uint8_t changed[NU54_V2_UNSIGNED_SIZE];
		memcpy(changed, quote, sizeof(changed));
		if (word * 32 == NU54_V2_MODE_OFFSET) changed[word * 32 + 31] = 2;
		else if (word * 32 == NU54_V2_PAYER_ROLE_OFFSET) changed[word * 32 + 31] = 2;
		else changed[word * 32 + 31] ^= 0x40;
		if (!nu54_order_preimage(changed, confirmer, other)) continue; /* not a valid quote at all */
		assert(memcmp(other, preimage, sizeof(other)));
	}
}

static int sign(void *context, const uint8_t digest[32], uint8_t signature[64])
{
	(void)digest;
	memcpy(signature, context, 64);
	return 0;
}

static void test_physical_approval_signs_exactly_once(void)
{
	nu54_local_approval approval = {0};
	nu54_v2_link link = channel();
	nu54_v2_request req = request();
	uint8_t signature[65];
	shown = 0;
	assert(!nu54_order_stage(&approval, &link, &req, confirmer, quote, &order, &allowlist, payer, GENERATION, EARLIEST, 9,
				 present, NULL));
	assert(shown == 1 && approval.pending);
	assert(nu54_pilot_complete(&approval, &link, GENERATION, NOW + 1, 8, NU54_LOCAL_APPROVE, payer, sign,
				   payer_signature, signature)); /* a press meant for another prompt */
	assert(!nu54_pilot_complete(&approval, &link, GENERATION, NOW + 1, 9, NU54_LOCAL_APPROVE, payer, sign,
				    payer_signature, signature));
	assert(!memcmp(signature, payer_signature, 65));
	assert(nu54_pilot_complete(&approval, &link, GENERATION, NOW + 1, 9, NU54_LOCAL_APPROVE, payer, sign,
				   payer_signature, signature));
	/* The reject button never signs. */
	assert(!nu54_order_stage(&approval, &link, &req, confirmer, quote, &order, &allowlist, payer, GENERATION, EARLIEST, 9,
				 present, NULL));
	assert(nu54_pilot_complete(&approval, &link, GENERATION, NOW + 1, 9, NU54_LOCAL_REJECT, payer, sign,
				   payer_signature, signature));
}

static void test_refuses_everything_outside_the_grant(void)
{
	assert(!stage(channel(), request(), confirmer, quote, allowlist, payer, GENERATION));

	nu54_v2_link link = channel();
	link.peer = NU54_V2_TERMINAL;
	link.profile = NU54_V2_PAYMENT; /* the kiosk is not the owner's phone */
	assert(stage(link, request(), confirmer, quote, allowlist, payer, GENERATION));
	link = channel();
	link.peer = NU54_V2_TERMINAL;
	assert(stage(link, request(), confirmer, quote, allowlist, payer, GENERATION));
	link = channel();
	link.profile = NU54_V2_PAYMENT;
	assert(stage(link, request(), confirmer, quote, allowlist, payer, GENERATION));
	link = channel();
	link.local = NU54_V2_OWNER;
	assert(stage(link, request(), confirmer, quote, allowlist, payer, GENERATION));
	link = channel();
	link.closed = true;
	assert(stage(link, request(), confirmer, quote, allowlist, payer, GENERATION));
	nu54_v2_request req = request();
	req.id[0] ^= 1;
	assert(stage(channel(), req, confirmer, quote, allowlist, payer, GENERATION));
	req = request();
	req.expires += 1; /* not the request the link is waiting on */
	assert(stage(channel(), req, confirmer, quote, allowlist, payer, GENERATION));
	req = request();
	req.command = NU54_V2_MANAGE;
	assert(stage(channel(), req, confirmer, quote, allowlist, payer, GENERATION));

	nu54_pilot_policy policy = allowlist;
	policy = allowlist;
	policy.max_amount = 6499999; /* one unit under the price */
	assert(stage(channel(), request(), confirmer, quote, policy, payer, GENERATION));
	policy.max_amount = 6500000;
	assert(!stage(channel(), request(), confirmer, quote, policy, payer, GENERATION));
	policy = allowlist;
	policy.contract[19] ^= 1;
	assert(stage(channel(), request(), confirmer, quote, policy, payer, GENERATION));
	policy = allowlist;
	policy.token[19] ^= 1;
	assert(stage(channel(), request(), confirmer, quote, policy, payer, GENERATION));
	policy = allowlist;
	policy.beneficiary[19] ^= 1;
	assert(stage(channel(), request(), confirmer, quote, policy, payer, GENERATION));

	uint8_t account[20];
	memcpy(account, payer, 20);
	account[0] ^= 1;
	assert(stage(channel(), request(), confirmer, quote, allowlist, account, GENERATION));
	assert(stage(channel(), request(), confirmer, quote, allowlist, payer, GENERATION + 1));

	const uint8_t none[20] = {0};
	assert(stage(channel(), request(), none, quote, allowlist, payer, GENERATION));
	assert(stage(channel(), request(), merchant_signer, quote, allowlist, payer, GENERATION));
	assert(stage(channel(), request(), allowlist.beneficiary, quote, allowlist, payer, GENERATION));

	uint8_t forged[NU54_V2_SIGNED_SIZE];
	memcpy(forged, quote, sizeof(forged));
	forged[NU54_V2_UNSIGNED_SIZE] ^= 1; /* the store's signature */
	assert(stage(channel(), request(), confirmer, forged, allowlist, payer, GENERATION));
	nu54_v2_quote_policy saved = order;
	order.merchant_signer[0] ^= 1; /* a store that is not enrolled */
	assert(stage(channel(), request(), confirmer, quote, allowlist, payer, GENERATION));
	order = saved;
	order.now = NOW + 110;
	assert(stage(channel(), request(), confirmer, quote, allowlist, payer, GENERATION));
	order = saved;

	/* The vector expires at NOW + 110. A longer-lived authorization could outlast a reset. */
	earliest = NOW + 110 - NU54_ORDER_MAX_SECONDS;
	assert(!stage(channel(), request(), confirmer, quote, allowlist, payer, GENERATION));
	earliest -= 1;
	assert(stage(channel(), request(), confirmer, quote, allowlist, payer, GENERATION));
	earliest = 0;
	assert(stage(channel(), request(), confirmer, quote, allowlist, payer, GENERATION));
	earliest = NOW + 1; /* a lower bound above the upper one */
	assert(stage(channel(), request(), confirmer, quote, allowlist, payer, GENERATION));
	earliest = EARLIEST;
}

/* A price an ordinary grant could pay, so only the testnet flag is left to refuse it. */
static void test_needs_a_testnet_grant(void)
{
	uint8_t key[32] = {0}, digest[32], small[NU54_V2_SIGNED_SIZE], raw[65];
	key[29] = 0x0a; key[30] = 0x11; key[31] = 0xce;
	memcpy(small, quote, sizeof(small));
	memset(small + NU54_V2_AMOUNT_OFFSET, 0, 32);
	small[NU54_V2_AMOUNT_OFFSET + 31] = 100;
	static const nu54_v2_quote_crypto hasher = {nu54_keccak256, NULL};
	assert(nu54_v2_quote_digest(small, NU54_V2_UNSIGNED_SIZE, &hasher, digest));
	secp256k1_context *context = secp256k1_context_create(SECP256K1_CONTEXT_NONE);
	secp256k1_ecdsa_recoverable_signature signature;
	int id;
	assert(secp256k1_ecdsa_sign_recoverable(context, &signature, digest, key, NULL, NULL));
	secp256k1_ecdsa_recoverable_signature_serialize_compact(context, raw, &id, &signature);
	raw[64] = (uint8_t)(27 + id);
	secp256k1_context_destroy(context);
	memcpy(small + NU54_V2_UNSIGNED_SIZE, raw, 65);

	nu54_v2_quote_policy saved = order;
	uint8_t preimage[NU54_LOCAL_PREIMAGE_SIZE];
	memcpy(order.expected, small, sizeof(order.expected));
	memcpy(preimage, expected_preimage, sizeof(preimage));
	assert(nu54_order_preimage(small, confirmer, expected_preimage));
	nu54_pilot_policy policy = allowlist;
	policy.max_amount = NU54_PILOT_LEGACY_CAP;
	assert(!stage(channel(), request(), confirmer, small, policy, payer, GENERATION));
	policy.commercial_testnet = false;
	assert(stage(channel(), request(), confirmer, small, policy, payer, GENERATION));
	order = saved;
	memcpy(expected_preimage, preimage, sizeof(preimage));
}

static void test_decode(void)
{
	uint8_t body[NU54_ORDER_REQUEST_SIZE + 1] = {'N', '5', '4', 'O'};
	const uint8_t *phone, *inner;
	assert(!nu54_order_decode(body, NU54_ORDER_REQUEST_SIZE, &phone, &inner)); /* no phone key */
	memcpy(body + 4, confirmer, 20);
	memcpy(body + 24, quote, sizeof(quote));
	assert(nu54_order_decode(body, NU54_ORDER_REQUEST_SIZE, &phone, &inner));
	assert(phone == body + 4 && !memcmp(inner, quote, sizeof(quote)));
	assert(!nu54_order_decode(body, NU54_ORDER_REQUEST_SIZE - 1, &phone, &inner));
	assert(!nu54_order_decode(body, NU54_ORDER_REQUEST_SIZE + 1, &phone, &inner));
	assert(!nu54_order_decode(quote, sizeof(quote), &phone, &inner)); /* a plain quote is not an order */
	body[3] = 'Q';
	assert(!nu54_order_decode(body, NU54_ORDER_REQUEST_SIZE, &phone, &inner));
}

static void test_daily_total(void)
{
	const uint64_t day = NOW / 86400, start = day * 86400, cap = 100;
	nu54_order_spending state = {0}, next;
	assert(!nu54_order_charge(&state, start, 60, cap, &next));
	state = next;
	assert(state.day == day && state.spent == 60);
	assert(nu54_order_charge(&state, start + 1, 41, cap, &next));
	assert(!nu54_order_charge(&state, start + 86399, 40, cap, &next) && next.spent == 100);
	state = next;
	assert(nu54_order_charge(&state, start + 86399, 1, cap, &next));
	assert(!nu54_order_charge(&state, start + 86400, 100, cap, &next) && next.day == day + 1 && next.spent == 100);
	/* A clock that goes back must not hand out a fresh allowance. */
	state = next;
	assert(nu54_order_charge(&state, start, 1, cap, &next));
	assert(nu54_order_charge(&state, start + 86400, UINT64_MAX, cap, &next));
	nu54_order_spending corrupt = {.day = day, .spent = cap + 1};
	assert(nu54_order_charge(&corrupt, start, 1, cap, &next));
	nu54_order_spending fresh = {0};
	assert(nu54_order_charge(&fresh, 0, 1, cap, &next)); /* no trusted time */
	assert(nu54_order_charge(&fresh, start, 0, cap, &next));
	assert(nu54_order_charge(&fresh, start, 1, 0, &next));
}

static nu54_order_record records[NU54_ORDER_RECORDS + 1];
static bool present_slots[NU54_ORDER_RECORDS + 1], broken;
static int read_slot(void *context, unsigned slot, nu54_order_record *out)
{
	(void)context;
	assert(slot >= 1 && slot <= NU54_ORDER_RECORDS);
	if (broken) return -1;
	if (!present_slots[slot]) return 1;
	*out = records[slot];
	return 0;
}

static void test_records(void)
{
	uint8_t id[16] = {1}, digest[32] = {2}, other[32] = {3};
	nu54_order_record found;
	unsigned slot;
	assert(nu54_order_find(read_slot, NULL, NOW, id, digest, &found, &slot) == NU54_PILOT_RECEIPT_NEW && slot == 1);
	for (unsigned i = 1; i <= NU54_ORDER_RECORDS; ++i) {
		records[i] = (nu54_order_record){.expires = NOW + 100, .outcome = NU54_ORDER_APPROVED};
		records[i].request[0] = (uint8_t)(i + 10);
		records[i].digest[0] = (uint8_t)(i + 10);
		present_slots[i] = true;
	}
	assert(nu54_order_find(read_slot, NULL, NOW, id, digest, &found, &slot) == NU54_PILOT_RECEIPT_FULL);
	/* An authorization that can no longer execute frees its slot, not a moment earlier. */
	records[7].expires = NOW + 1;
	assert(nu54_order_find(read_slot, NULL, NOW, id, digest, &found, &slot) == NU54_PILOT_RECEIPT_FULL);
	assert(nu54_order_find(read_slot, NULL, NOW + 1, id, digest, &found, &slot) == NU54_PILOT_RECEIPT_NEW && slot == 7);

	memcpy(records[3].request, id, 16);
	memcpy(records[3].digest, digest, 32);
	records[3].outcome = NU54_ORDER_REFUSED;
	assert(nu54_order_find(read_slot, NULL, NOW, id, digest, &found, &slot) == NU54_PILOT_RECEIPT_RECORDED);
	assert(found.outcome == NU54_ORDER_REFUSED);
	assert(nu54_order_find(read_slot, NULL, NOW, id, other, &found, &slot) == NU54_PILOT_RECEIPT_ERROR);
	records[5].outcome = 0;
	assert(nu54_order_find(read_slot, NULL, NOW, id + 1, other, &found, &slot) == NU54_PILOT_RECEIPT_ERROR);
	broken = true;
	assert(nu54_order_find(read_slot, NULL, NOW, id, digest, &found, &slot) == NU54_PILOT_RECEIPT_ERROR);
}

static void test_account_proof(void)
{
	uint8_t challenge[32] = {1}, preimage[66], other[66], statement[NU54_ACCOUNT_STATEMENT_SIZE];
	const uint8_t none[32] = {0};
	assert(nu54_account_preimage(payer, challenge, preimage));
	assert(!nu54_account_preimage(payer, none, other) && !nu54_account_preimage(none, challenge, other));
	challenge[31] = 9;
	assert(nu54_account_preimage(payer, challenge, other) && memcmp(preimage, other, 66));
	/* Neither the token nor the escrow accepts the domain of the test. */
	assert(memcmp(preimage + 2, expected_preimage + 2, 32));
	nu54_account_statement(payer, challenge, payer_signature, statement);
	assert(!memcmp(statement, "NU54 account address v1", 24) && !memcmp(statement + 24, challenge, 32));
	assert(statement[62] == 0x20 && statement[63] == 0x5b && !memcmp(statement + 64, payer, 20));
	assert(!memcmp(statement + 84, payer_signature, 65));
	/* Known answer computed with Foundry's cast from the EIP-712 definition. */
	static const uint8_t known[32] = {
		0x6a, 0xc8, 0xbe, 0x6e, 0x20, 0xf7, 0xc3, 0x97, 0x1e, 0x89, 0x4c, 0xfd, 0x18, 0xe3, 0x06, 0x76,
		0xef, 0x10, 0x32, 0xd0, 0x39, 0xb6, 0x24, 0xd7, 0xa2, 0x44, 0xcf, 0x77, 0x42, 0xb4, 0x90, 0xc8,
	};
	uint8_t digest[32], fixed[32];
	memset(fixed, 0x11, sizeof(fixed));
	assert(nu54_account_preimage(payer, fixed, preimage));
	nu54_keccak256(preimage, sizeof(preimage), digest);
	assert(!memcmp(digest, known, sizeof(known)));
}

int main(int argc, char **argv)
{
	assert(argc == 2);
	FILE *file = fopen(argv[1], "r");
	assert(file);
	load(file, "unsigned", unsigned_quote, sizeof(unsigned_quote));
	load(file, "merchantSignature", merchant_signature, 65);
	load(file, "merchantSigner", merchant_signer, 20);
	load(file, "confirmer", confirmer, 20);
	load(file, "preimage", expected_preimage, sizeof(expected_preimage));
	load(file, "payerSignature", payer_signature, 65);
	fclose(file);
	memcpy(quote, unsigned_quote, sizeof(unsigned_quote));
	memcpy(quote + NU54_V2_UNSIGNED_SIZE, merchant_signature, 65);
	memcpy(payer, quote + NU54_V2_PAYER_OFFSET + 12, 20);
	memcpy(allowlist.contract, quote + NU54_V2_VERIFYING_CONTRACT_OFFSET + 12, 20);
	memcpy(allowlist.token, quote + NU54_V2_ASSET_OFFSET + 12, 20);
	memcpy(allowlist.beneficiary, quote + NU54_V2_BENEFICIARY_OFFSET + 12, 20);
	allowlist.max_amount = NU54_PILOT_TESTUSDC2_CAP;
	allowlist.commercial_testnet = true;
	order.now = NOW;
	memcpy(order.expected, quote, sizeof(order.expected));
	memcpy(order.merchant_signer, merchant_signer, 20);

	test_signature_matches_the_contract();
	test_physical_approval_signs_exactly_once();
	test_refuses_everything_outside_the_grant();
	test_needs_a_testnet_grant();
	test_decode();
	test_daily_total();
	test_records();
	test_account_proof();
	puts("order payment ok");
	return 0;
}
