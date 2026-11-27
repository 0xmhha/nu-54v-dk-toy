#include "nu54_rental_retirement.h"
/*
 * The firmware session against the shared session vectors: every scenario is replayed and each
 * reply must equal the vector byte for byte (signatures included: the host signs with RFC 6979
 * like the simulator, then nu54_sig_finish makes the protocol form as on the device). Secure
 * scenarios (payment-protocol.md 4.1) use OpenSSL for the platform's HKDF and AES-GCM.
 */
#ifdef NDEBUG
#undef NDEBUG
#endif
#include <assert.h>
#include <errno.h>
#include <pthread.h>
#include <stdbool.h>
#include <stdio.h>
#include <string.h>
#include <time.h>

#include <openssl/evp.h>
#include <openssl/kdf.h>

#include "nu54_keccak.h"
#include "nu54_cbor.h"
#include "../core/src/nu54_session_int.h"
#include "nu54_rental_freeze.h"
#include "nu54_sig.h"
#include "secp256k1.h"
#include "vectors.h"
#include "rental_evidence_vector.h"

static secp256k1_context *ctx;
static int draws;
static uint8_t key[32];     /* the device key: SV_KEY, or the one setup made */
static uint8_t new_key[32]; /* made by generate_key, kept only when setup commits */
static int commits, wipes, sign_calls;
static char stored_pin[16];  /* what setup stored; a provisioned vector device has 2580 */
static int pin_failures;     /* kept in secure storage on the board */
#define PIN_MAX_RETRIES 5

static int host_sign(void *c, const uint8_t digest[32], uint8_t rs[64])
{
	(void)c;
	sign_calls++;
	secp256k1_ecdsa_signature sig;
	if (!secp256k1_ecdsa_sign(ctx, &sig, digest, key, NULL, NULL)) {
		return -1;
	}
	secp256k1_ecdsa_signature_serialize_compact(ctx, rs, &sig);
	return 0;
}

/* The vectors' random source: the k-th request returns bytes all equal to 0x5a + k. */
static void vector_random(void *c, uint8_t *out, size_t n)
{
	(void)c;
	memset(out, 0x5a + draws++, n);
}

static int vector_challenge_random(void *c, uint8_t out[32])
{
	vector_random(c, out, 32);
	return 0;
}

static int partial_challenge_random(void *c, uint8_t out[32])
{
	(void)c;
	out[0] = 1;
	return -1;
}

/* Key generation draws its 32 bytes from the vectors' random source, like a TRNG would. */
static int host_generate_key(void *c, uint8_t address[20])
{
	vector_random(c, new_key, 32);
	secp256k1_pubkey pub;
	uint8_t ser[65], h[32];
	size_t n = sizeof(ser);
	if (!secp256k1_ec_pubkey_create(ctx, &pub, new_key)) {
		return -1;
	}
	secp256k1_ec_pubkey_serialize(ctx, ser, &n, &pub, SECP256K1_EC_UNCOMPRESSED);
	nu54_keccak256(ser + 1, 64, h);
	memcpy(address, h + 12, 20);
	return 0;
}

static int host_commit(void *c, const nu54_setup_record_t *rec)
{
	(void)c;
	if (!rec->pin || rec->pin_len == 0) {
		return -1;
	}
	memcpy(key, new_key, 32);
	snprintf(stored_pin, sizeof(stored_pin), "%.*s", (int)rec->pin_len, rec->pin);
	pin_failures = 0;
	commits++;
	return 0;
}

static int host_check_pin(void *c, const char *pin, size_t len)
{
	(void)c;
	if (pin_failures >= PIN_MAX_RETRIES) {
		return NU54_PIN_LOCKED;
	}
	if (len == strlen(stored_pin) && memcmp(pin, stored_pin, len) == 0) {
		pin_failures = 0;
		return NU54_PIN_OK;
	}
	return ++pin_failures >= PIN_MAX_RETRIES ? NU54_PIN_LOCKED : NU54_PIN_WRONG;
}

static int host_wipe(void *c)
{
	(void)c;
	wipes++;
	return 0;
}

static int host_hkdf(void *c, const uint8_t ikm[32], const uint8_t salt[64], const char *info, uint8_t key[16])
{
	(void)c;
	EVP_PKEY_CTX *k = EVP_PKEY_CTX_new_id(EVP_PKEY_HKDF, NULL);
	size_t n = 16;
	int ok = k && EVP_PKEY_derive_init(k) > 0 && EVP_PKEY_CTX_set_hkdf_md(k, EVP_sha256()) > 0 &&
		 EVP_PKEY_CTX_set1_hkdf_salt(k, salt, 64) > 0 && EVP_PKEY_CTX_set1_hkdf_key(k, ikm, 32) > 0 &&
		 EVP_PKEY_CTX_add1_hkdf_info(k, (const unsigned char *)info, (int)strlen(info)) > 0 && EVP_PKEY_derive(k, key, &n) > 0 && n == 16;
	EVP_PKEY_CTX_free(k);
	return ok ? 0 : -1;
}

static int host_gcm(int seal, const uint8_t key[16], const uint8_t iv[12], const uint8_t *in, size_t len, uint8_t *out)
{
	EVP_CIPHER_CTX *x = EVP_CIPHER_CTX_new();
	size_t body = seal ? len : len - 16;
	int n = 0, m = 0;
	int ok = x && (seal ? EVP_EncryptInit_ex(x, EVP_aes_128_gcm(), NULL, key, iv) : EVP_DecryptInit_ex(x, EVP_aes_128_gcm(), NULL, key, iv)) > 0;
	if (ok && seal) {
		ok = EVP_EncryptUpdate(x, out, &n, in, (int)body) > 0 && EVP_EncryptFinal_ex(x, out + n, &m) > 0 &&
		     EVP_CIPHER_CTX_ctrl(x, EVP_CTRL_GCM_GET_TAG, 16, out + body) > 0;
	} else if (ok) {
		ok = EVP_DecryptUpdate(x, out, &n, in, (int)body) > 0 &&
		     EVP_CIPHER_CTX_ctrl(x, EVP_CTRL_GCM_SET_TAG, 16, (void *)(in + body)) > 0 && EVP_DecryptFinal_ex(x, out + n, &m) > 0;
	}
	EVP_CIPHER_CTX_free(x);
	return ok ? 0 : -1;
}

static int host_seal(void *c, const uint8_t key[16], const uint8_t iv[12], const uint8_t *in, size_t len, uint8_t *out)
{
	(void)c;
	return host_gcm(1, key, iv, in, len, out);
}

static int host_open(void *c, const uint8_t key[16], const uint8_t iv[12], const uint8_t *in, size_t len, uint8_t *out)
{
	(void)c;
	return len < 16 ? -1 : host_gcm(0, key, iv, in, len, out);
}

static int mode_calls;
static int host_payment_mode(void *c, int on, uint32_t seconds)
{
	(void)c;
	(void)on;
	(void)seconds;
	mode_calls++;
	return 0;
}

/* The host platform and the vectors' parameters; scenarios add their build options. */
static nu54_config_t host_config(void)
{
	return (nu54_config_t){
		.firmware = SV_FIRMWARE,
		.anchor_clock_skew = SV_ANCHOR_SKEW,
		.authorization_expiry = SV_AUTH_EXPIRY,
		.platform = {.sign = host_sign,
			     .random = vector_random,
			     .rental_challenge_random = vector_challenge_random,
			     .generate_key = host_generate_key,
			     .commit_setup = host_commit,
			     .wipe = host_wipe,
			     .check_pin = host_check_pin,
			     .hkdf = host_hkdf,
			     .aead_seal = host_seal,
			     .aead_open = host_open,
			     .payment_mode = host_payment_mode},
	};
}

static void dump(const char *what, const uint8_t *b, size_t n)
{
	printf("  %s (%zu): ", what, n);
	for (size_t i = 0; i < n; i++) {
		printf("%02x", b[i]);
	}
	printf("\n");
}

typedef struct {
	nu54_rental_journal_image_t image;
	unsigned writes;
	unsigned refuse_write;
} session_journal_store_t;

static int read_session_journal(void *context, void *image, size_t size)
{
	session_journal_store_t *store = context;
	if (!store->writes) return 1;
	memcpy(image, &store->image, size);
	return 0;
}

static int persist_session_journal(void *context, const void *image, size_t size)
{
	session_journal_store_t *store = context;
	memcpy(&store->image, image, size);
	return ++store->writes == store->refuse_write ? -1 : 0;
}

static int forbid_legacy_nonce(void *context, const uint8_t nonce[32])
{
	(void)context;
	(void)nonce;
	assert(!"journaled authorization reached legacy nonce storage");
	return -1;
}

static uint64_t journal_test_time(void *context)
{
	return *(uint64_t *)context;
}

static void setup_journal_device(nu54_device_t *d, nu54_rental_journal_t *journal, uint64_t *clock)
{
	memset(d, 0, sizeof(*d));
	d->cfg = host_config();
	d->cfg.platform.monotonic_seconds = journal_test_time;
	d->cfg.platform.ctx = clock;
	d->cfg.platform.persist_nonce = forbid_legacy_nonce;
	memcpy(d->address, SV_ADDRESS, 20);
	memcpy(d->operator_address, SV_OPERATOR, 20);
	memcpy(d->contract, SV_CONTRACT, 20);
	memcpy(d->chain_id, SV_CHAIN_ID, 32);
	nu54_device_init(d, SV_NONCE_START);
	assert(nu54_device_bind_rental_journal(d, journal) != 0);
	nu54_device_init_rental_candidate(d, SV_NONCE_START);
	if (journal) assert(nu54_device_bind_rental_journal(d, journal) == 0);
	memcpy(key, SV_KEY, 32);
	strcpy(stored_pin, "2580");
	pin_failures = 0;
	draws = 0;
}

static const session_scenario_t *find_scenario(const char *id)
{
	for (size_t i = 0; i < SESSION_SCENARIO_COUNT; ++i) {
		if (!strcmp(SESSION_SCENARIOS[i].id, id)) return &SESSION_SCENARIOS[i];
	}
	return NULL;
}

static int allow_protected_setup(void *context)
{
	(void)context;
	return 1;
}

static void journal_candidate_through_setup_pin(void)
{
	const session_scenario_t *scenario = find_scenario("SV-13");
	assert(scenario && scenario->count >= 2);
	const nu54_link_t link = {.bonded = scenario->link_bonded, .phone_present = scenario->phone_present};
	nu54_device_t d = {.cfg = host_config()};
	nu54_out_t out;
	nu54_device_init_unprovisioned(&d);
	draws = 0;
	const session_step_t *opening = &scenario->steps[0];
	nu54_session_handle(&d, &link, opening->send, opening->send_len, opening->at, &out);
	const nu54_cbor_entry_t requested[] = {
		{"v", NU54_V_UINT, NULL, 0, 1}, {"type", NU54_V_TEXT, (const uint8_t *)"setup.operator", 14, 0},
		{"sessionId", NU54_V_BYTES, d.session_id, 8, 0}, {"operator", NU54_V_BYTES, SV_OPERATOR, 20, 0},
		{"contract", NU54_V_BYTES, SV_CONTRACT, 20, 0}, {"chainId", NU54_V_UINT256, SV_CHAIN_ID, 32, 0},
		{"passkey", NU54_V_UINT, NULL, 0, 123456}, {"rentalJournal", NU54_V_BOOL, NULL, 0, 1},
	};
	uint8_t wire[256];
	nu54_cbor_writer_t writer;
	nu54_cbor_writer_init(&writer, wire, sizeof(wire));
	assert(nu54_cbor_write_map(&writer, requested, sizeof(requested) / sizeof(requested[0])) == 0);
	nu54_session_handle(&d, &link, wire, writer.len, opening->at, &out);
	assert(out.event == NU54_EVENT_REFUSED && d.pending == NU54_PENDING_NONE);
	d.cfg.platform.can_commission_rental_journal = allow_protected_setup;
	nu54_session_handle(&d, &link, wire, writer.len, opening->at, &out);
	assert(d.pending == NU54_PENDING_SETUP_CONFIRM && d.pending_setup.journal_requested);
	nu54_session_button(&d, 1, &out);
	assert(d.pending == NU54_PENDING_PIN);
	const session_step_t *legacy = &scenario->steps[1];
	nu54_session_handle(&d, &link, legacy->send, legacy->send_len, legacy->at, &out);
	assert(out.event == NU54_EVENT_REFUSED && d.pending == NU54_PENDING_PIN && d.pending_setup.journal_requested);
	memset(&out, 0, sizeof(out));
	int previous_commits = commits;
	nu54_session_pin(&d, scenario->pin, strlen(scenario->pin), &out);
	assert(commits == previous_commits && d.state == NU54_STATE_UNPROVISIONED);
	assert(out.event == NU54_EVENT_REFUSED && d.pending == NU54_PENDING_PIN);

	/* This flag is set by authenticated board commissioning, after the local setup approval. */
	d.pending_setup.journal_required = 1;
	uint8_t first[32];
	memcpy(first, d.pending_setup.nonce_start, sizeof(first));
	int candidate_commit = commits;
	memset(&out, 0, sizeof(out));
	nu54_session_pin(&d, scenario->pin, strlen(scenario->pin), &out);
	assert(commits == candidate_commit + 1 && d.state == NU54_STATE_PROVISIONED_NO_ANCHOR);
	assert(d.rental_journal_required && !d.rental_journal && !memcmp(first, d.next_nonce, sizeof(first)));
	assert(d.session_open && d.pending == NU54_PENDING_NONE);
	nu54_msg_t ack;
	assert(out.kiosk_count > 0 && nu54_msg_decode(out.kiosk[out.kiosk_count - 1],
	       out.kiosk_len[out.kiosk_count - 1], &ack) == NU54_MSG_OK);
	int accepted = nu54_msg_find(&ack, 0, "accepted");
	assert(accepted >= 0 && ack.items[accepted].value == 1);
	puts("ok: real setup PIN entry commits a signing-disabled rental candidate");
}

static void journal_candidate_refuses(const char *id, size_t last_step)
{
	const session_scenario_t *scenario = find_scenario(id);
	assert(scenario && last_step < scenario->count);
	const nu54_link_t link = {.bonded = scenario->link_bonded, .phone_present = scenario->phone_present};
	nu54_device_t d;
	nu54_out_t out;
	uint64_t clock;
	setup_journal_device(&d, NULL, &clock);
	const int before_signs = sign_calls, before_wipes = wipes;
	for (size_t i = 0; i <= last_step; ++i) {
		const session_step_t *step = &scenario->steps[i];
		clock = step->at;
		nu54_session_handle(&d, &link, step->send, step->send_len, clock, &out);
	}
	assert(d.state == NU54_STATE_READY && d.pending == NU54_PENDING_NONE);
	assert(out.event == NU54_EVENT_REFUSED && out.phone_count == 0);
	assert(sign_calls == before_signs && wipes == before_wipes);
}

static void journal_limit_and_payment(const session_scenario_t *scenario)
{
	/* SV-18 authorizes a limit via PIN/button, then a payment via button. A lost issued write
	 * acknowledgment must refuse the first reply and recover the exact signatures on reboot. */
	session_journal_store_t persisted = {.refuse_write = 3};
	nu54_rental_journal_store_t store = {&persisted, read_session_journal, persist_session_journal};
	nu54_rental_binding_t binding = {
		.generation = {1}, .branch = {2}, .device = {3}, .registration = {4}, .measurement = {5},
		.deployment = {6},
	};
	memcpy(binding.account, SV_ADDRESS, 20);
	memcpy(binding.contract, SV_CONTRACT, 20);
	memcpy(binding.chain, SV_CHAIN_ID, 32);
	nu54_rental_journal_t journal = {0};
	nu54_device_t d;
	nu54_out_t out;
	uint64_t clock = 0;
	assert(nu54_rental_journal_open(&journal, &store, &binding, SV_NONCE_START, 1) == 0);
	const nu54_link_t link = {.bonded = scenario->link_bonded, .phone_present = scenario->phone_present};
	/* A protected candidate can accept the signed time anchor but cannot show or sign limit
	 * approval until its own verified journal is available. Reboot keeps the candidate state. */
	setup_journal_device(&d, NULL, &clock);
	const int before_stage = sign_calls;
	for (unsigned i = 0; i <= 4; ++i) {
		const session_step_t *step = &scenario->steps[i];
		clock = step->at;
		nu54_session_handle(&d, &link, step->send, step->send_len, clock, &out);
	}
	assert(d.state == NU54_STATE_READY && d.pending == NU54_PENDING_NONE);
	assert(out.event == NU54_EVENT_REFUSED && out.phone_count == 0 && before_stage == sign_calls);
	assert(d.rental_journal_required);
	nu54_device_power_cycle(&d);
	assert(d.rental_journal_required && !d.rental_journal);
	d.chain_id[0] ^= 0x80;
	assert(nu54_device_bind_rental_journal(&d, &journal) != 0);
	d.chain_id[0] ^= 0x80;
	assert(nu54_device_bind_rental_journal(&d, &journal) == 0);
	const int old_signs = sign_calls;
	for (unsigned pass = 0; pass < 5; ++pass) {
		if (pass > 0) assert(nu54_rental_journal_reload(&journal) == 0);
		setup_journal_device(&d, &journal, &clock);
		for (size_t i = 0; i < scenario->count; ++i) {
			const session_step_t *step = &scenario->steps[i];
			clock = step->at;
			nu54_session_handle(&d, &link, step->send, step->send_len, clock, &out);
			if (d.pending == NU54_PENDING_LIMIT_PIN) {
				if (pass == 4) {
					uint8_t root[32];
					assert(nu54_device_freeze_rental_journal(&d, root) == 0);
					assert(d.pending == NU54_PENDING_NONE);
					nu54_session_pin(&d, scenario->pin, strlen(scenario->pin), &out);
					nu54_session_button(&d, 1, &out);
					assert(out.kiosk_count == 0 && sign_calls == old_signs + 2);
					nu54_session_handle(&d, &link, step->send, step->send_len, clock, &out);
					assert(out.event == NU54_EVENT_REFUSED && out.phone_count == 0);
					break;
				}
				nu54_session_pin(&d, scenario->pin, strlen(scenario->pin), &out);
				if (pass == 3) {
					clock = d.anchor_at + (d.pending_limit.expiry - d.anchor_timestamp) + 1;
					nu54_session_button(&d, 1, &out);
					assert(out.event == NU54_EVENT_REFUSED && sign_calls == old_signs + 2);
					break;
				}
			}
			if (d.pending == NU54_PENDING_PAYMENT || d.pending == NU54_PENDING_LIMIT_CONFIRM)
				nu54_session_button(&d, 1, &out);
			if (pass == 0 && i == 4) {
				assert(out.event == NU54_EVENT_REFUSED && out.kiosk_count == 1 && !journal.loaded);
				assert(sign_calls == old_signs + 1);
				break;
			}
			assert(out.kiosk_count == step->expect_count && (int)out.event == step->event);
			for (int r = 0; r < out.kiosk_count; ++r) {
				assert(out.kiosk_len[r] == step->expect_len[r]);
				assert(memcmp(out.kiosk[r], step->expect[r], out.kiosk_len[r]) == 0);
			}
		}
		if (pass > 0) {
			assert(sign_calls == old_signs + 2);
			assert(journal.image.count == 2);
			assert(journal.image.entries[0].kind == NU54_RENTAL_LIMIT);
			assert(journal.image.entries[1].kind == NU54_RENTAL_PAYMENT);
			assert(journal.image.next[31] == 2);
		}
	}
	assert(journal.image.frozen);
	puts("ok: journaled SV-18 issue, lost acknowledgment, exact replay, expiry and pending freeze");
}

static void fixture_address(const uint8_t private_key[32], uint8_t address[20])
{
	secp256k1_pubkey pub;
	uint8_t encoded[65];
	size_t length = sizeof(encoded);
	assert(secp256k1_ec_pubkey_create(ctx, &pub, private_key));
	assert(secp256k1_ec_pubkey_serialize(ctx, encoded, &length, &pub, SECP256K1_EC_UNCOMPRESSED));
	nu54_address_of_pubkey(encoded, address);
}

static void fixture_signature(const uint8_t private_key[32], const uint8_t address[20], const uint8_t digest[32], uint8_t output[65])
{
	secp256k1_ecdsa_signature signature;
	uint8_t raw[64];
	assert(secp256k1_ecdsa_sign(ctx, &signature, digest, private_key, NULL, NULL));
	assert(secp256k1_ecdsa_signature_serialize_compact(ctx, raw, &signature));
	assert(nu54_sig_finish(digest, raw, address, output) == NU54_SIG_OK);
}

static size_t fixture_map(uint8_t *out, size_t capacity, const nu54_cbor_entry_t *entries, size_t count)
{
	nu54_cbor_writer_t writer;
	nu54_cbor_writer_init(&writer, out, capacity);
	assert(nu54_cbor_write_map(&writer, entries, count) == 0);
	return writer.len;
}

static void fixture_payment(nu54_device_t *d, const uint8_t operator_key[32], const uint8_t merchant_key[32],
			    uint64_t now, uint8_t amount, nu54_out_t *out)
{
	const nu54_link_t link = {.bonded = 1, .phone_present = 1};
	uint8_t domain[32], data[32], digest[32], sig[65], nested[384], wire[512];
	nu54_merchant_order_t order = {
		.order_id = {0x42}, .token = {0x23}, .payout = {0x82}, .amount = {[31] = amount}, .expiry = now + 60,
	};
	nu54_merchant_attestation_t merchant = {
		.name = (const uint8_t *)"Paid fixture", .name_len = sizeof("Paid fixture") - 1,
		.valid_from = now - 1, .valid_until = now + 120,
	};
	fixture_address(merchant_key, merchant.merchant);
	memcpy(merchant.payout, order.payout, sizeof(merchant.payout));
	nu54_eip712_domain_separator(d->chain_id, d->contract, domain);
	nu54_hash_merchant_attestation(&merchant, data);
	nu54_eip712_digest(domain, data, digest);
	fixture_signature(operator_key, d->operator_address, digest, sig);
	const nu54_cbor_entry_t attested[] = {
		{"merchant", NU54_V_BYTES, merchant.merchant, 20, 0}, {"payout", NU54_V_BYTES, merchant.payout, 20, 0},
		{"name", NU54_V_TEXT, merchant.name, merchant.name_len, 0},
		{"validFrom", NU54_V_UINT, NULL, 0, merchant.valid_from}, {"validUntil", NU54_V_UINT, NULL, 0, merchant.valid_until},
		{"operatorSignature", NU54_V_BYTES, sig, 65, 0},
	};
	size_t length = fixture_map(nested, sizeof(nested), attested, sizeof(attested) / sizeof(attested[0]));
	const nu54_cbor_entry_t identify[] = {
		{"v", NU54_V_UINT, NULL, 0, 1}, {"type", NU54_V_TEXT, (const uint8_t *)"payment.identify", 16, 0},
		{"sessionId", NU54_V_BYTES, d->session_id, 8, 0}, {"attestation", NU54_V_RAW, nested, length, 0},
	};
	length = fixture_map(wire, sizeof(wire), identify, sizeof(identify) / sizeof(identify[0]));
	nu54_session_handle(d, &link, wire, length, now, out);
	assert(d->attested && out->event == NU54_EVENT_NONE);

	nu54_hash_merchant_order(&order, data);
	nu54_eip712_digest(domain, data, digest);
	fixture_signature(merchant_key, merchant.merchant, digest, sig);
	const nu54_cbor_entry_t authorization[] = {
		{"chainId", NU54_V_UINT256, d->chain_id, 32, 0}, {"contract", NU54_V_BYTES, d->contract, 20, 0},
		{"merchant", NU54_V_BYTES, merchant.merchant, 20, 0}, {"payout", NU54_V_BYTES, order.payout, 20, 0},
		{"token", NU54_V_BYTES, order.token, 20, 0}, {"amount", NU54_V_UINT256, order.amount, 32, 0},
		{"orderId", NU54_V_BYTES, order.order_id, 32, 0}, {"expiry", NU54_V_UINT, NULL, 0, order.expiry},
	};
	length = fixture_map(nested, sizeof(nested), authorization, sizeof(authorization) / sizeof(authorization[0]));
	const nu54_cbor_entry_t prepare[] = {
		{"v", NU54_V_UINT, NULL, 0, 1}, {"type", NU54_V_TEXT, (const uint8_t *)"payment.prepare", 15, 0},
		{"sessionId", NU54_V_BYTES, d->session_id, 8, 0}, {"authorization", NU54_V_RAW, nested, length, 0},
		{"merchantSignature", NU54_V_BYTES, sig, 65, 0},
	};
	length = fixture_map(wire, sizeof(wire), prepare, sizeof(prepare) / sizeof(prepare[0]));
	nu54_session_handle(d, &link, wire, length, now, out);
	assert(d->pending == NU54_PENDING_PAYMENT && out->phone_count == 1);
	nu54_session_button(d, 1, out);
}

static const uint8_t board_fixture_key[32] = {[31] = 13};
static int board_attestation_fail;

static int fixture_board_attest(void *context, const uint8_t digest[32], uint8_t raw[64])
{
	(void)context;
	if (board_attestation_fail) return -1;
	secp256k1_ecdsa_signature signature;
	if (!secp256k1_ecdsa_sign(ctx, &signature, digest, board_fixture_key, NULL, NULL)) return -1;
	return secp256k1_ecdsa_signature_serialize_compact(ctx, raw, &signature) ? 0 : -1;
}

static int capture_fixture_rental_freeze(void *context, nu54_device_t *d, const nu54_rental_freeze_request_t *request,
					 const uint8_t operator_signature[65], nu54_rental_freeze_receipt_t *receipt,
					 uint8_t board_signature[65])
{
	(void)context;
	uint8_t identity[20];
	fixture_address(board_fixture_key, identity);
	return nu54_device_attest_rental_freeze(d, request, operator_signature, identity, receipt, board_signature);
}

static nu54_link_t fixture_collector_link(bool bonded, uint8_t peer, uint64_t epoch)
{
	return (nu54_link_t){.bonded = bonded, .foreign = 1, .connection_epoch = epoch, .peer = {peer}};
}

static int fixture_collector_allowed(void *context, const nu54_rental_binding_t *binding,
				    const uint8_t peer[NU54_RENTAL_PEER_SIZE])
{
	(void)context;
	return binding->generation[0] == 1 && peer[0] == 1 && peer[1] == 0;
}

static void fixture_collector_challenge(nu54_device_t *d, uint64_t now, nu54_link_t link, uint8_t nonce[32],
					 int permitted, bool for_cleanup)
{
	uint8_t wire[96], zero[8] = {0};
	const char *type = for_cleanup ? "rental.cleanup.challenge" : "rental.freeze.challenge";
	const nu54_cbor_entry_t fields[] = {
		{"v", NU54_V_UINT, NULL, 0, 1}, {"type", NU54_V_TEXT, (const uint8_t *)type, strlen(type), 0},
		{"sessionId", NU54_V_BYTES, zero, 8, 0},
	};
	size_t length = fixture_map(wire, sizeof(wire), fields, sizeof(fields) / sizeof(fields[0]));
	nu54_out_t out;
	nu54_session_handle(d, &link, wire, length, now, &out);
	assert(out.kiosk_count == 1);
	if (!permitted) {
		assert(out.event == NU54_EVENT_REFUSED);
		return;
	}
	nu54_msg_t reply;
	assert(out.event == NU54_EVENT_NONE && nu54_msg_decode(out.kiosk[0], out.kiosk_len[0], &reply) == NU54_MSG_OK);
	assert(!strcmp(reply.message->type, for_cleanup ? "rental.cleanup.challenge.result" : "rental.freeze.challenge.result"));
	int value = nu54_msg_find(&reply, 0, "channelNonce");
	assert(value >= 0 && reply.items[value].len == 32);
	memcpy(nonce, reply.items[value].ptr, 32);
}

static void freeze_fixture_challenge(nu54_device_t *d, uint64_t now, nu54_link_t link, uint8_t nonce[32], int permitted)
{
	fixture_collector_challenge(d, now, link, nonce, permitted, false);
}

static void freeze_fixture_wire(nu54_device_t *d, uint64_t now, nu54_link_t link, int index,
				const nu54_rental_freeze_request_t *request, const uint8_t signature[65], nu54_out_t *out)
{
	uint8_t wire[256], zero[8] = {0};
	const char type[] = "rental.freeze.v2";
	const nu54_cbor_entry_t fields[] = {
		{"v", NU54_V_UINT, NULL, 0, 1}, {"type", NU54_V_TEXT, (const uint8_t *)type, sizeof(type) - 1, 0},
		{"sessionId", NU54_V_BYTES, zero, 8, 0}, {"challenge", NU54_V_BYTES, request->challenge, 32, 0},
		{"channelNonce", NU54_V_BYTES, request->channel_nonce, 32, 0},
		{"expiresAt", NU54_V_UINT, NULL, 0, request->expires_at},
		{"operatorSignature", NU54_V_BYTES, signature, 65, 0},
		{"projectionIndex", NU54_V_UINT, NULL, 0, (uint64_t)index},
	};
	size_t length = fixture_map(wire, sizeof(wire), fields, sizeof(fields) / sizeof(fields[0]) - (index < 0 ? 1 : 0));
	nu54_session_handle(d, &link, wire, length, now, out);
}

static void fixture_legacy_freeze_digest(const nu54_rental_binding_t *binding, const nu54_rental_freeze_request_t *request,
					 uint8_t digest[32])
{
	static const char type[] = "RentalFreezeRequest(bytes32 binding,bytes32 challenge,uint64 expiresAt)";
	uint8_t words[4][32] = {{0}}, data[32], domain[32];
	nu54_keccak256((const uint8_t *)type, sizeof(type) - 1, words[0]);
	nu54_keccak256((const uint8_t *)binding, sizeof(*binding), words[1]);
	memcpy(words[2], request->challenge, 32);
	for (unsigned i = 0; i < 8; ++i) words[3][31 - i] = (uint8_t)(request->expires_at >> (8 * i));
	nu54_keccak256((const uint8_t *)words, sizeof(words), data);
	nu54_eip712_domain_separator(binding->chain, binding->contract, domain);
	nu54_eip712_digest(domain, data, digest);
}

static struct {
	nu54_rental_journal_t *journal;
	nu54_rental_cleanup_witness_t witness;
	nu54_rental_retirement_fence_t fence;
	uint64_t utc, witness_advance;
	int held, writes, store_failure, witness_failure, guard_failure;
} cleanup;

static int cleanup_guard(void *context)
{
	(void)context;
	assert(!cleanup.held);
	if (cleanup.guard_failure) return -1;
	cleanup.held = 1;
	return 0;
}

static void cleanup_unlock(void *context)
{
	(void)context;
	assert(cleanup.held);
	cleanup.held = 0;
}

static int cleanup_clock(void *context, uint64_t *utc)
{
	(void)context;
	assert(cleanup.held);
	*utc = cleanup.utc;
	return cleanup.utc ? 0 : -1;
}

static int cleanup_binding(void *context, nu54_rental_binding_t *binding)
{
	(void)context;
	assert(cleanup.held);
	*binding = cleanup.journal->expected_binding;
	return 0;
}

static int cleanup_witness(void *context, const nu54_rental_binding_t *binding,
			    const nu54_rental_cleanup_intent_t *intent, nu54_rental_cleanup_witness_t *out)
{
	(void)context;
	assert(cleanup.held);
	if (cleanup.witness_failure || memcmp(binding, &cleanup.journal->expected_binding, sizeof(*binding)) ||
	    memcmp(intent->envelope, cleanup.witness.envelope, 32)) return -1;
	*out = cleanup.witness;
	if (cleanup.witness_advance) cleanup.utc = cleanup.witness_advance;
	return 0;
}

static int read_cleanup_fence(void *context, nu54_rental_retirement_fence_t *fence)
{
	(void)context;
	assert(cleanup.held);
	if (!cleanup.writes) return 1;
	*fence = cleanup.fence;
	return 0;
}

static int persist_cleanup_fence(void *context, const nu54_rental_retirement_fence_t *fence)
{
	(void)context;
	assert(cleanup.held && !cleanup.writes);
	if (cleanup.store_failure) return -1;
	cleanup.fence = *fence;
	++cleanup.writes;
	return 0;
}

static int pin_cleanup(void *context, const nu54_rental_retirement_authority_t *authority, uint64_t now)
{
	(void)context;
	assert(cleanup.held);
	const nu54_rental_retirement_store_t store = {.read = read_cleanup_fence, .create = persist_cleanup_fence};
	return nu54_rental_retirement_fence(cleanup.journal, &store, authority, now);
}

static void cleanup_wire_test(nu54_device_t *d, const nu54_link_t *link, uint64_t clock,
			      const uint8_t nonce[32], const uint8_t raw[NU54_RENTAL_CLEANUP_INTENT_SIZE], bool permit)
{
	static const char digits[] = "0123456789abcdef";
	static const char type[] = "rental.cleanup.admit";
	uint8_t wire[512], zero[8] = {0}, hex[2 * NU54_RENTAL_CLEANUP_INTENT_SIZE];
	for (size_t i = 0; i < NU54_RENTAL_CLEANUP_INTENT_SIZE; ++i) {
		hex[i * 2] = digits[raw[i] >> 4];
		hex[i * 2 + 1] = digits[raw[i] & 15];
	}
	const nu54_cbor_entry_t fields[] = {
		{"v", NU54_V_UINT, NULL, 0, 1}, {"type", NU54_V_TEXT, (const uint8_t *)type, sizeof(type) - 1, 0},
		{"sessionId", NU54_V_BYTES, zero, 8, 0}, {"channelNonce", NU54_V_BYTES, nonce, 32, 0},
		{"intent", NU54_V_TEXT, hex, sizeof(hex), 0},
	};
	size_t length = fixture_map(wire, sizeof(wire), fields, sizeof(fields) / sizeof(fields[0]));
	nu54_out_t out;
	nu54_session_handle(d, link, wire, length, clock, &out);
	assert(!cleanup.held && out.kiosk_count == 1 && out.phone_count == 0);
	if (!permit) {
		assert(out.event == NU54_EVENT_REFUSED);
		return;
	}
	nu54_msg_t reply;
	assert(out.event == NU54_EVENT_NONE && nu54_msg_decode(out.kiosk[0], out.kiosk_len[0], &reply) == NU54_MSG_OK);
	assert(!strcmp(reply.message->type, "rental.cleanup.status"));
	int status = nu54_msg_find(&reply, 0, "status");
	assert(status >= 0 && reply.items[status].len == 7 && !memcmp(reply.items[status].ptr, "pending", 7));
	assert(nu54_msg_find(&reply, 0, "signature") < 0 && d->pending == NU54_PENDING_NONE);
}

static void signed_cleanup_test(nu54_device_t *d, nu54_rental_journal_t *journal, uint64_t clock,
				const uint8_t operator_key[32], const uint8_t root[32])
{
	const nu54_rental_binding_t *binding = &journal->expected_binding;
	static const char domain[] = "NU54-rental-cleanup-disposition-v1";
	uint8_t raw[NU54_RENTAL_CLEANUP_INTENT_SIZE] = "RCI1";
	uint8_t preimage[sizeof(domain) + 161] = {0};
	uint8_t *fields = preimage + sizeof(domain);
	memcpy(preimage, domain, sizeof(domain));
	nu54_keccak256((const uint8_t *)binding, sizeof(*binding), raw + 4);
	memcpy(fields, raw + 4, 32);
	fields[32] = 2;
	fields[64] = 3;
	fields[96] = 4;
	fields[128] = 2;
	fields[129] = 5;
	nu54_keccak256(preimage, sizeof(preimage), raw + 36);
	raw[68] = 6;
	for (unsigned i = 0; i < 8; ++i) {
		raw[107 - i] = (uint8_t)((clock - 1) >> (i * 8));
		raw[115 - i] = (uint8_t)((clock + 8) >> (i * 8));
	}
	cleanup.journal = journal;
	cleanup.utc = clock;
	cleanup.witness.checked_at = clock;
	cleanup.witness.valid_until = clock + 20;
	cleanup.witness.kind = 2;
	memcpy(cleanup.witness.binding, raw + 4, 32);
	memcpy(cleanup.witness.request, raw + 68, 32);
	memcpy(cleanup.witness.root, root, 32);
	cleanup.witness.freeze[0] = 2;
	cleanup.witness.close[0] = 3;
	cleanup.witness.anchor[0] = 4;
	cleanup.witness.withdrawal[0] = 5;
	nu54_rental_cleanup_intent_digest(binding->chain, binding->contract, raw, cleanup.witness.intent);
	fixture_signature(operator_key, d->operator_address, cleanup.witness.intent, raw + 116);
	nu54_keccak256(raw, sizeof(raw), cleanup.witness.envelope);

	const nu54_link_t collector = fixture_collector_link(true, 1, 23);
	const nu54_link_t other = fixture_collector_link(true, 2, 24);
	uint8_t nonce[32], stale[32] = {9};
	fixture_collector_challenge(d, clock, collector, nonce, 0, true); /* absent provider */
	freeze_fixture_challenge(d, clock, collector, nonce, 1);
	cleanup_wire_test(d, &collector, clock, nonce, raw, false); /* freeze nonce cannot authorize cleanup */
	d->cfg.platform.begin_rental_cleanup = cleanup_guard;
	d->cfg.platform.end_rental_cleanup = cleanup_unlock;
	d->cfg.platform.rental_cleanup_time = cleanup_clock;
	d->cfg.platform.rental_cleanup_binding = cleanup_binding;
	d->cfg.platform.rental_cleanup_witness = cleanup_witness;
	d->cfg.platform.fence_rental_cleanup = pin_cleanup;
	fixture_collector_challenge(d, clock, collector, nonce, 0, true); /* ordinary freeze permission alone */
	d->cfg.platform.allow_rental_cleanup_station = fixture_collector_allowed;
	const int sign_count = sign_calls;
	const nu54_rental_journal_image_t original = journal->image;
	cleanup.utc = clock - 119;
	fixture_collector_challenge(d, clock, other, nonce, 0, true);
	fixture_collector_challenge(d, clock, collector, nonce, 1, true);
	cleanup.utc = clock;
	cleanup.witness_advance = clock + 1;
	cleanup_wire_test(d, &collector, clock, nonce, raw, false); /* collector lease expired during witness read */
	assert(!cleanup.writes);
	cleanup.witness_advance = 0;
	fixture_collector_challenge(d, clock, collector, nonce, 1, true);
	cleanup_wire_test(d, &other, clock, nonce, raw, false);
	cleanup_wire_test(d, &collector, clock, stale, raw, false);
	d->rental_access.cleanup_binding[0] ^= 1;
	cleanup_wire_test(d, &collector, clock, nonce, raw, false);
	d->rental_access.cleanup_binding[0] ^= 1;
	raw[4] ^= 1;
	cleanup_wire_test(d, &collector, clock, nonce, raw, false);
	raw[4] ^= 1;
	raw[116] ^= 1;
	cleanup_wire_test(d, &collector, clock, nonce, raw, false);
	raw[116] ^= 1;
	cleanup.witness_failure = 1;
	cleanup_wire_test(d, &collector, clock, nonce, raw, false);
	cleanup.witness_failure = 0;
	cleanup.utc = 0;
	cleanup_wire_test(d, &collector, clock, nonce, raw, false);
	cleanup.utc = clock;
	cleanup.witness.pending_payments = 1;
	cleanup_wire_test(d, &collector, clock, nonce, raw, false);
	cleanup.witness.pending_payments = 0;
	cleanup.witness.pending_limits = 1;
	cleanup_wire_test(d, &collector, clock, nonce, raw, false);
	cleanup.witness.pending_limits = 0;
	cleanup.witness.root[0] ^= 1;
	cleanup_wire_test(d, &collector, clock, nonce, raw, false);
	cleanup.witness.root[0] ^= 1;
	cleanup.witness.envelope[0] ^= 1;
	cleanup_wire_test(d, &collector, clock, nonce, raw, false);
	cleanup.witness.envelope[0] ^= 1;
	cleanup.guard_failure = 1;
	cleanup_wire_test(d, &collector, clock, nonce, raw, false);
	cleanup.guard_failure = 0;
	cleanup.utc = clock + 8;
	cleanup_wire_test(d, &collector, clock, nonce, raw, false); /* no first admission when expired */
	cleanup.utc = clock;
	cleanup.store_failure = 1;
	cleanup_wire_test(d, &collector, clock, nonce, raw, false);
	assert(!cleanup.writes);
	cleanup.store_failure = 0;
	cleanup_wire_test(d, &collector, clock, nonce, raw, true);
	cleanup.utc = clock + 8;
	nu54_device_rental_peer_closed(d, collector.connection_epoch);
	cleanup_wire_test(d, &collector, clock, nonce, raw, false);
	d->rental_journal = NULL; /* separate protected reader can recover after signing-disabled reboot */
	d->anchored = 0;
	freeze_fixture_challenge(d, clock, collector, nonce, 0);
	fixture_collector_challenge(d, clock, collector, nonce, 1, true);
	cleanup_wire_test(d, &collector, clock, nonce, raw, true);
	assert(!d->rental_journal);
	d->rental_journal = journal;
	cleanup.utc = cleanup.witness.valid_until;
	cleanup_wire_test(d, &collector, clock, nonce, raw, false);
	assert(cleanup.writes == 1 && sign_calls == sign_count && !memcmp(&original, &journal->image, sizeof(original)));
	puts("ok: signed RCI1 authenticated collector command pins and reopens only its pending fence");
}

static void signed_freeze_test(nu54_device_t *d, nu54_rental_journal_t *journal, uint64_t *clock,
			       const uint8_t operator_key[32], const uint8_t other_key[32])
{
	uint8_t identity[20], other[20], digest[32], request_signature[65], signature[65], repeated[65];
	nu54_rental_freeze_receipt_t receipt = {0}, replay = {0};
	nu54_rental_freeze_request_t request = {.challenge = {0x72}, .expires_at = *clock + 90};
	fixture_address(board_fixture_key, identity);
	fixture_address(other_key, other);
	d->cfg.platform.sign_rental_attestation = fixture_board_attest;
	d->cfg.platform.freeze_rental = capture_fixture_rental_freeze;
	d->pending = NU54_PENDING_PAYMENT;
	const nu54_link_t collector = fixture_collector_link(true, 1, 10);
	const nu54_link_t paired_phone = fixture_collector_link(true, 2, 11);
	const nu54_link_t reconnected = fixture_collector_link(true, 1, 12);
	freeze_fixture_challenge(d, *clock, collector, request.channel_nonce, 0); /* unqualified default */
	d->cfg.platform.allow_rental_collector = fixture_collector_allowed;
	freeze_fixture_challenge(d, *clock, paired_phone, request.channel_nonce, 0);
	freeze_fixture_challenge(d, *clock, collector, request.channel_nonce, 1);
	d->cfg.platform.rental_challenge_random = partial_challenge_random;
	freeze_fixture_challenge(d, *clock, collector, request.channel_nonce, 0);
	assert(d->rental_access.epoch == 0);
	d->cfg.platform.rental_challenge_random = vector_challenge_random;
	freeze_fixture_challenge(d, *clock, collector, request.channel_nonce, 1);

	nu54_out_t wire_response;
	fixture_legacy_freeze_digest(&journal->expected_binding, &request, digest);
	fixture_signature(operator_key, d->operator_address, digest, request_signature);
	freeze_fixture_wire(d, *clock, collector, -1, &request, request_signature, &wire_response);
	assert(wire_response.event == NU54_EVENT_REFUSED && !journal->image.frozen && d->pending == NU54_PENDING_PAYMENT);
	nu54_rental_freeze_request_digest(&journal->expected_binding, &request, digest);
	fixture_signature(other_key, other, digest, request_signature);
	assert(nu54_device_attest_rental_freeze(d, &request, request_signature, identity, &receipt, signature) != 0);
	assert(!journal->image.frozen && d->pending == NU54_PENDING_PAYMENT && !receipt.frozen_at);

	fixture_signature(operator_key, d->operator_address, digest, request_signature);
	freeze_fixture_wire(d, *clock, fixture_collector_link(false, 1, 10), -1, &request, request_signature, &wire_response);
	assert(wire_response.event == NU54_EVENT_REFUSED && !journal->image.frozen && d->pending == NU54_PENDING_PAYMENT);
	freeze_fixture_wire(d, *clock, collector, 0, &request, request_signature, &wire_response);
	assert(wire_response.event == NU54_EVENT_REFUSED && !journal->image.frozen && d->pending == NU54_PENDING_PAYMENT);
	request.challenge[0] ^= 1;
	assert(nu54_device_attest_rental_freeze(d, &request, request_signature, identity, &receipt, signature) != 0);
	request.challenge[0] ^= 1;
	assert(nu54_device_attest_rental_freeze(d, &request, request_signature, d->address, &receipt, signature) != 0);
	assert(!journal->image.frozen && d->pending == NU54_PENDING_PAYMENT);
	board_attestation_fail = 1;
	assert(nu54_device_attest_rental_freeze(d, &request, request_signature, identity, &receipt, signature) != 0);
	assert(journal->image.frozen && d->pending == NU54_PENDING_NONE && !receipt.frozen_at);

	uint64_t first_fence_time = *clock;
	++*clock;
	board_attestation_fail = 0;
	assert(nu54_rental_journal_reload(journal) == 0);
	assert(nu54_device_attest_rental_freeze(d, &request, request_signature, identity, &receipt, signature) == 0);
	assert(receipt.frozen_at == first_fence_time && receipt.entry_count == 2 && receipt.expires_at == request.expires_at);
	assert(receipt.first_nonce[31] == 0 && receipt.next_nonce[31] == 2 && !memcmp(receipt.challenge, request.challenge, 32));
	nu54_rental_evidence_t public_rows;
	uint8_t public_root[32];
	assert(nu54_rental_journal_project(journal, &public_rows, public_root) == 0);
	assert(!memcmp(receipt.projection_root, public_root, sizeof(public_root)));

	nu54_rental_freeze_receipt_digest(&journal->expected_binding, &receipt, digest);
	uint8_t recovered[20];
	assert(nu54_sig_recover(digest, signature, sizeof(signature), recovered) == NU54_SIG_OK);
	assert(!memcmp(identity, recovered, sizeof(recovered)));

	++*clock;
	assert(nu54_device_attest_rental_freeze(d, &request, request_signature, identity, &replay, repeated) == 0);
	assert(!memcmp(&replay, &receipt, sizeof(receipt)) && !memcmp(signature, repeated, sizeof(signature)));
	freeze_fixture_wire(d, *clock, collector, -1, &request, request_signature, &wire_response);
	assert(wire_response.event == NU54_EVENT_NONE && wire_response.kiosk_count == 1 && d->session_open);
	nu54_msg_t wire_receipt;
	assert(nu54_msg_decode(wire_response.kiosk[0], wire_response.kiosk_len[0], &wire_receipt) == NU54_MSG_OK);
	assert(!strcmp(wire_receipt.message->type, "rental.freeze.result"));
	int proof = nu54_msg_find(&wire_receipt, 0, "boardSignature"), root_id = nu54_msg_find(&wire_receipt, 0, "projectionRoot");
	assert(proof >= 0 && root_id >= 0 && !memcmp(wire_receipt.items[proof].ptr, signature, sizeof(signature)));
	assert(!memcmp(wire_receipt.items[root_id].ptr, receipt.projection_root, sizeof(receipt.projection_root)));
	freeze_fixture_wire(d, *clock, collector, 1, &request, request_signature, &wire_response);
	assert(wire_response.event == NU54_EVENT_NONE && wire_response.kiosk_count == 1);
	assert(nu54_msg_decode(wire_response.kiosk[0], wire_response.kiosk_len[0], &wire_receipt) == NU54_MSG_OK);
	assert(!strcmp(wire_receipt.message->type, "rental.freeze.entry"));
	assert(nu54_msg_find(&wire_receipt, 0, "boardSignature") < 0);
	int approval = nu54_msg_find(&wire_receipt, 0, "approvalDigest");
	assert(approval >= 0 && !memcmp(wire_receipt.items[approval].ptr, public_rows.entries[1].digest, 32));
	d->cfg.platform.allow_rental_collector = NULL;
	freeze_fixture_wire(d, *clock, collector, 1, &request, request_signature, &wire_response);
	assert(wire_response.event == NU54_EVENT_REFUSED);
	d->cfg.platform.allow_rental_collector = fixture_collector_allowed;
	freeze_fixture_wire(d, *clock, paired_phone, 1, &request, request_signature, &wire_response);
	assert(wire_response.event == NU54_EVENT_REFUSED);
	freeze_fixture_wire(d, *clock, reconnected, 1, &request, request_signature, &wire_response);
	assert(wire_response.event == NU54_EVENT_REFUSED);
	nu54_device_rental_peer_closed(d, collector.connection_epoch);
	freeze_fixture_wire(d, *clock, collector, 1, &request, request_signature, &wire_response);
	assert(wire_response.event == NU54_EVENT_REFUSED);
	freeze_fixture_challenge(d, *clock, reconnected, request.channel_nonce, 1);
	freeze_fixture_wire(d, *clock, reconnected, 1, &request, request_signature, &wire_response);
	assert(wire_response.event == NU54_EVENT_REFUSED); /* old operator proof with another nonce */
	nu54_rental_freeze_request_digest(&journal->expected_binding, &request, digest);
	fixture_signature(operator_key, d->operator_address, digest, request_signature);
	freeze_fixture_wire(d, *clock, reconnected, 1, &request, request_signature, &wire_response);
	assert(wire_response.event == NU54_EVENT_NONE && wire_response.kiosk_count == 1);
	uint8_t replacement_nonce[32];
	freeze_fixture_challenge(d, *clock, reconnected, replacement_nonce, 1);
	assert(memcmp(replacement_nonce, request.channel_nonce, sizeof(replacement_nonce)));
	freeze_fixture_wire(d, *clock, reconnected, 1, &request, request_signature, &wire_response);
	assert(wire_response.event == NU54_EVENT_REFUSED);
	freeze_fixture_wire(d, *clock, reconnected, 2, &request, request_signature, &wire_response);
	assert(wire_response.event == NU54_EVENT_REFUSED);
	*clock = request.expires_at + 1;
	assert(nu54_device_attest_rental_freeze(d, &request, request_signature, identity, &replay, repeated) != 0);
	assert(journal->image.frozen);
	signed_cleanup_test(d, journal, *clock, operator_key, public_root);
	puts("ok: operator-authenticated freeze commits before distinct board signature and replays exact registered-collector wire receipt and requires reconnect authentication");
}

static void journal_merchant_order_scope(void)
{
	session_journal_store_t persisted = {0};
	nu54_rental_journal_store_t store = {&persisted, read_session_journal, persist_session_journal};
	nu54_rental_binding_t binding = {
		.generation = {1}, .branch = {2}, .device = {3}, .registration = {4}, .measurement = {5},
		.deployment = {6},
	};
	memcpy(binding.account, SV_ADDRESS, 20);
	memcpy(binding.contract, SV_CONTRACT, 20);
	memcpy(binding.chain, SV_CHAIN_ID, 32);
	nu54_rental_journal_t journal = {0};
	assert(nu54_rental_journal_open(&journal, &store, &binding, SV_NONCE_START, 1) == 0);

	nu54_device_t d;
	nu54_out_t out;
	uint64_t clock = 1000;
	const uint8_t operator_key[32] = {0, [31] = 9};
	const uint8_t first_merchant[32] = {0, [31] = 10};
	const uint8_t second_merchant[32] = {0, [31] = 11};
	setup_journal_device(&d, &journal, &clock);
	fixture_address(operator_key, d.operator_address);
	d.state = NU54_STATE_READY;
	d.anchored = d.session_open = d.confirmed = 1;
	d.anchor_timestamp = d.anchor_at = clock;
	d.session_id[0] = 1;
	int signs = sign_calls;
	fixture_payment(&d, operator_key, first_merchant, clock, 1, &out);
	assert(out.event == NU54_EVENT_SIGNED && journal.image.count == 1);
	uint8_t first_reply[NU54_OUT_MAX];
	size_t first_len = out.kiosk_len[0];
	memcpy(first_reply, out.kiosk[0], first_len);

	fixture_payment(&d, operator_key, second_merchant, clock, 2, &out);
	assert(out.event == NU54_EVENT_SIGNED && journal.image.count == 2);
	fixture_payment(&d, operator_key, first_merchant, clock, 5, &out);
	assert(out.event == NU54_EVENT_REFUSED && journal.image.count == 2);
	fixture_payment(&d, operator_key, first_merchant, clock, 1, &out);
	assert(out.event == NU54_EVENT_SIGNED && journal.image.count == 2 && sign_calls == signs + 2);
	assert(out.kiosk_len[0] == first_len && !memcmp(out.kiosk[0], first_reply, first_len));
	puts("ok: payment journals two merchants sharing order ID and preserves exact first replay");
	signed_freeze_test(&d, &journal, &clock, operator_key, first_merchant);
}


typedef struct {
	uint8_t record[NU54_RENTAL_CREDENTIAL_SIZE];
	uint64_t now;
	uint64_t next_now;
	unsigned reads_until_advance;
	int writes;
	bool available;
	bool ambiguous_write;
	bool commissioned;
} fixture_credential_store_t;

static int credential_read(void *context, void *record, size_t length)
{
	const fixture_credential_store_t *s = context;
	assert(length == sizeof(s->record));
	if (!s->writes) return 1;
	memcpy(record, s->record, length);
	return 0;
}

static int credential_create(void *context, const void *record, size_t length)
{
	fixture_credential_store_t *s = context;
	assert(length == sizeof(s->record));
	if (s->writes) return -1;
	memcpy(s->record, record, length);
	++s->writes;
	return s->ambiguous_write ? -1 : 0;
}

static int credential_may_issue(void *context, const nu54_rental_binding_t *binding)
{
	const fixture_credential_store_t *s = context;
	return s->commissioned && binding->generation[0] == 1;
}

static int credential_now(void *context, uint64_t *now)
{
	fixture_credential_store_t *s = context;
	if (!s->available) return -1;
	if (s->reads_until_advance && --s->reads_until_advance == 0) s->now = s->next_now;
	*now = s->now;
	return 0;
}

/* Binary stdin only. The ephemeral fixture key never appears in argv or stdout. */
static int credential_fixture_attest(void *context, const uint8_t digest[32], uint8_t raw[64])
{
	secp256k1_ecdsa_signature signature;
	if (!secp256k1_ecdsa_sign(ctx, &signature, digest, context, NULL, NULL)) return -1;
	return secp256k1_ecdsa_signature_serialize_compact(ctx, raw, &signature) ? 0 : -1;
}

static pthread_mutex_t slot_observation_guard = PTHREAD_MUTEX_INITIALIZER;
static nu54_rental_journal_t *attestation_writer_journal;

static int credential_observation_begin(void *context)
{
	(void)context;
	return pthread_mutex_trylock(&slot_observation_guard);
}

static void credential_observation_end(void *context)
{
	(void)context;
	assert(pthread_mutex_unlock(&slot_observation_guard) == 0);
}

static void *credential_observation_writer(void *context)
{
	(void)context;
	assert(pthread_mutex_trylock(&slot_observation_guard) == EBUSY);
	return NULL;
}

static int credential_guarded_signer(void *context, const uint8_t digest[32], uint8_t raw[64])
{
	pthread_t writer;
	assert(pthread_create(&writer, NULL, credential_observation_writer, NULL) == 0);
	assert(pthread_join(writer, NULL) == 0);
	assert(!attestation_writer_journal->image.frozen);
	return fixture_board_attest(context, digest, raw);
}

static fixture_credential_store_t *attestation_fault_store;
static nu54_rental_journal_t *attestation_fault_journal;
static unsigned attestation_fault;
static uint8_t attestation_replacement[NU54_RENTAL_CREDENTIAL_SIZE];

static int credential_attestation_fault(void *context, const uint8_t digest[32], uint8_t raw[64])
{
	switch (attestation_fault) {
	case 1: attestation_fault_journal->image.frozen = 1; break;
	case 2: attestation_fault_journal->expected_binding.generation[7] ^= 1; break;
	case 3: attestation_fault_store->now = 1100; break;
	case 4: attestation_fault_store->now = 1000; break;
	case 5:
		memcpy(attestation_fault_store->record, attestation_replacement, sizeof(attestation_replacement));
		break;
	case 6: attestation_fault_store->available = false; break;
	case 7: {
		uint8_t root[32];
		nu54_rental_journal_t external = *attestation_fault_journal;
		assert(nu54_rental_journal_freeze(&external, attestation_fault_store->now, root) == 0);
		break;
	}
	}
	return fixture_board_attest(context, digest, raw);
}

static int credential_fixture_commissioned(void *context, const nu54_rental_binding_t *binding)
{
	return ((fixture_credential_store_t *)context)->commissioned && nu54_rental_journal_binding_valid(binding);
}

static int credential_fixture_command(bool slot_open)
{
	struct {
		nu54_rental_binding_t binding;
		uint8_t first[32], authority[20], signature[65], board_key[32], challenge[32], issued[8], ends[8], frozen;
	} input;
	if (fread(&input, 1, sizeof(input), stdin) != sizeof(input) || fgetc(stdin) != EOF || input.frozen > 1) return 1;
	uint64_t issued = 0, ends = 0;
	for (size_t i = 0; i < 8; ++i) {
		issued = (issued << 8) | input.issued[i];
		ends = (ends << 8) | input.ends[i];
	}
	time_t wall = time(NULL);
	if (wall <= 0) return 1;
	fixture_credential_store_t retained = {.now = (uint64_t)wall, .available = true, .commissioned = true};
	const nu54_rental_credential_store_t credentials = {
		.context = &retained, .read = credential_read, .create = credential_create,
		.may_issue = credential_fixture_commissioned, .trusted_now = credential_now,
	};
	session_journal_store_t persisted = {0};
	nu54_rental_journal_store_t store = {&persisted, read_session_journal, persist_session_journal};
	nu54_rental_journal_t journal = {0};
	if (nu54_rental_journal_open(&journal, &store, &input.binding, input.first, 1) != 0) return 1;
	nu54_device_t d = {0};
	d.cfg.platform.ctx = input.board_key;
	d.cfg.platform.rental_credential_required = true;
	d.cfg.platform.rental_credential_store = &credentials;
	d.cfg.platform.sign_rental_attestation = credential_fixture_attest;
	memcpy(d.operator_address, input.authority, 20);
	/* A test-only clock and PS provider replace hardware. The credential,
	 * journal, signature validation and receipt use the linked production core. */
	d.rental_journal = &journal;
	memcpy(d.address, input.binding.account, 20);
	nu54_rental_credential_t original = {1, 1, issued, ends};
	if (nu54_device_install_rental_credential(&d, &original, input.signature) != 0) return 1;
	uint8_t root[32];
	if (input.frozen && nu54_rental_journal_freeze(&journal, retained.now, root) != 0) return 1;
	nu54_rental_credential_receipt_t receipt;
	uint8_t signature[65], identity[20], checked[8];
	fixture_address(input.board_key, identity);
	d.cfg.platform.begin_rental_slot_observation = credential_observation_begin;
	d.cfg.platform.end_rental_slot_observation = credential_observation_end;
	int status = slot_open ? nu54_device_attest_rental_slot_open(&d, input.challenge, identity, &receipt, signature)
			       : nu54_device_attest_rental_credential(&d, input.challenge, identity, &receipt, signature);
	if (status != 0) return 2;
	for (size_t i = 0; i < 8; ++i) checked[7 - i] = (uint8_t)(receipt.checked_at >> (8 * i));
	return fwrite(checked, 1, sizeof(checked), stdout) == sizeof(checked) &&
	       fwrite(signature, 1, sizeof(signature), stdout) == sizeof(signature) ? 0 : 1;
}

static void protected_rental_credential(void)
{
	session_journal_store_t persisted = {0};
	nu54_rental_journal_store_t store = {&persisted, read_session_journal, persist_session_journal};
	nu54_rental_binding_t binding = {
		.generation = {1}, .branch = {2}, .device = {3}, .registration = {4}, .measurement = {5}, .deployment = {6},
	};
	memcpy(binding.account, SV_ADDRESS, 20);
	memcpy(binding.contract, SV_CONTRACT, 20);
	memcpy(binding.chain, SV_CHAIN_ID, 32);
	nu54_rental_journal_t journal = {0};
	assert(nu54_rental_journal_open(&journal, &store, &binding, SV_NONCE_START, 1) == 0);
	uint64_t clock = 1000;
	fixture_credential_store_t retained = {.now = clock, .available = true, .ambiguous_write = true};
	const nu54_rental_credential_store_t credentials = {
		.context = &retained, .read = credential_read, .create = credential_create,
		.may_issue = credential_may_issue, .trusted_now = credential_now,
	};
	const uint8_t operator_key[32] = {[31] = 9}, merchant_key[32] = {[31] = 10};
	const uint8_t challenge[32] = {0x72};
	uint8_t digest[32], issued[65], signature[65], identity[20], recovered[20], merchant[20], nonce[32];
	nu54_device_t d;
	setup_journal_device(&d, &journal, &clock);
	d.cfg.platform.rental_credential_required = true;
	d.cfg.platform.rental_credential_store = &credentials;
	d.cfg.platform.sign_rental_attestation = fixture_board_attest;
	fixture_address(operator_key, d.operator_address);
	fixture_address(merchant_key, merchant);
	fixture_address(board_fixture_key, identity);

	nu54_rental_journal_request_t limit = {.key = {1}};
	nu54_rental_credential_t grant = {.revision = 1, .policy = 1, .issued_at = 1000, .expires_at = 1100};
	nu54_rental_credential_receipt_t receipt;
	nu54_device_t historical = d;
	historical.rental_journal = NULL;
	historical.rental_journal_required = 0;
	assert(!ss_rental_can_prompt(&historical));
	assert(ss_rental_sign(&historical, NU54_RENTAL_LIMIT, &limit, 1099, digest, nonce, signature) != 0);
	assert(!ss_rental_can_prompt(&d));
	assert(nu54_device_attest_rental_credential(&d, challenge, identity, &receipt, signature) != 0);
	retained.commissioned = true;
	nu54_rental_binding_t other = binding;
	other.generation[7] ^= 1;
	nu54_rental_credential_digest(&other, &grant, digest);
	fixture_signature(operator_key, d.operator_address, digest, issued);
	assert(nu54_device_install_rental_credential(&d, &grant, issued) != 0 && retained.writes == 0);
	nu54_rental_credential_digest(&binding, &grant, digest);
	fixture_signature(merchant_key, merchant, digest, issued);
	assert(nu54_device_install_rental_credential(&d, &grant, issued) != 0 && retained.writes == 0);
	fixture_signature(operator_key, d.operator_address, digest, issued);
	retained.commissioned = false;
	assert(nu54_device_install_rental_credential(&d, &grant, issued) != 0 && retained.writes == 0);
	retained.commissioned = true;
	retained.available = false;
	assert(nu54_device_install_rental_credential(&d, &grant, issued) != 0 && retained.writes == 0);
	retained.available = true;
	assert(nu54_device_install_rental_credential(&d, &grant, issued) != 0 && retained.writes == 1);
	retained.commissioned = false;
	assert(nu54_device_install_rental_credential(&d, &grant, issued) == 0 && retained.writes == 1);
	nu54_rental_credential_t extension = grant;
	++extension.expires_at;
	nu54_rental_credential_digest(&binding, &extension, digest);
	fixture_signature(operator_key, d.operator_address, digest, issued);
	assert(nu54_device_install_rental_credential(&d, &extension, issued) != 0 && retained.writes == 1);
	assert(nu54_device_attest_rental_credential(&d, challenge, identity, &receipt, signature) == 0);
	assert(receipt.checked_at == clock && receipt.credential.expires_at == grant.expires_at);
	nu54_rental_credential_receipt_digest(&binding, &receipt, digest);
	assert(nu54_sig_recover(digest, signature, 65, recovered) == NU54_SIG_OK && !memcmp(recovered, identity, 20));

	/* Legacy signatures cannot verify as an open-slot observation. */
	nu54_rental_slot_open_receipt_digest(&binding, &receipt, digest);
	assert(nu54_sig_recover(digest, signature, 65, recovered) != NU54_SIG_OK || memcmp(recovered, identity, 20));
	assert(nu54_device_attest_rental_slot_open(&d, challenge, identity, &receipt, signature) != 0);
	d.cfg.platform.begin_rental_slot_observation = credential_observation_begin;
	d.cfg.platform.end_rental_slot_observation = credential_observation_end;
	assert(pthread_mutex_lock(&slot_observation_guard) == 0);
	assert(nu54_device_attest_rental_slot_open(&d, challenge, identity, &receipt, signature) != 0);
	assert(pthread_mutex_unlock(&slot_observation_guard) == 0);
	attestation_writer_journal = &journal;
	d.cfg.platform.sign_rental_attestation = credential_guarded_signer;
	assert(nu54_device_attest_rental_slot_open(&d, challenge, identity, &receipt, signature) == 0);
	nu54_rental_slot_open_receipt_digest(&binding, &receipt, digest);
	assert(nu54_sig_recover(digest, signature, 65, recovered) == NU54_SIG_OK && !memcmp(recovered, identity, 20));

	/* Defense in depth against faults inside an adapter callback. No receipt is published. */
	nu54_rental_credential_receipt_t prior_receipt = receipt;
	uint8_t prior_signature[65];
	memcpy(prior_signature, signature, sizeof(signature));
	const session_journal_store_t initial_persisted = persisted;
	const fixture_credential_store_t original_retained = retained;
	/* Make a different valid, authority-signed record in independent storage. */
	retained = (fixture_credential_store_t){.now = clock, .available = true, .commissioned = true};
	assert(nu54_device_install_rental_credential(&d, &extension, issued) == 0);
	memcpy(attestation_replacement, retained.record, sizeof(attestation_replacement));
	retained = original_retained;
	attestation_fault_store = &retained;
	attestation_fault_journal = &journal;
	d.cfg.platform.sign_rental_attestation = credential_attestation_fault;
	for (attestation_fault = 1; attestation_fault <= 7; ++attestation_fault) {
		retained.now = clock + 1;
		assert(nu54_device_attest_rental_slot_open(&d, challenge, identity, &receipt, signature) != 0);
		assert(!memcmp(&receipt, &prior_receipt, sizeof(receipt)));
		assert(!memcmp(signature, prior_signature, sizeof(signature)));
		journal.image.frozen = 0;
		journal.expected_binding = binding;
		persisted = initial_persisted;
		retained = original_retained;
	}
	retained.now = clock;
	d.cfg.platform.sign_rental_attestation = fixture_board_attest;

	d.state = NU54_STATE_READY;
	d.anchored = d.session_open = d.confirmed = 1;
	d.anchor_timestamp = d.anchor_at = clock;
	d.session_id[0] = 1;
	int signs = sign_calls;
	nu54_out_t out;
	fixture_payment(&d, operator_key, merchant_key, clock, 1, &out);
	assert(out.event == NU54_EVENT_SIGNED && journal.image.count == 1 && sign_calls == signs + 1);
	assert(ss_rental_nonce(&d, NU54_RENTAL_LIMIT, limit.key, nonce) == 0);
	assert(ss_rental_sign(&d, NU54_RENTAL_LIMIT, &limit, grant.expires_at, digest, nonce, signature) != 0);
	assert(journal.image.count == 1 && sign_calls == signs + 1);
	assert(ss_rental_sign(&d, NU54_RENTAL_LIMIT, &limit, grant.expires_at - 1, digest, nonce, signature) == 0);
	assert(journal.image.count == 2 && sign_calls == signs + 2);

	const nu54_rental_journal_request_t another_limit = {.key = {2}};
	const uint8_t another_order[32] = {0x43};
	nu54_rental_journal_request_t another_payment;
	nu54_rental_journal_payment_request(merchant, another_order, &another_payment);
	uint8_t next_nonce[32];
	assert(ss_rental_nonce(&d, NU54_RENTAL_LIMIT, another_limit.key, next_nonce) == 0);

	nu54_device_power_cycle(&d);
	/* Restore the firmware profile before its backing protected adapter starts. */
	d.cfg.platform.rental_credential_store = NULL;
	assert(!ss_rental_can_prompt(&d));
	d.anchored = 1;
	assert(ss_rental_sign(&d, NU54_RENTAL_PAYMENT, &another_payment, 1099, digest, next_nonce, signature) != 0);
	assert(ss_rental_sign(&d, NU54_RENTAL_LIMIT, &another_limit, 1099, digest, next_nonce, signature) != 0);
	assert(nu54_device_attest_rental_credential(&d, challenge, identity, &receipt, signature) != 0);
	const nu54_rental_credential_store_t unavailable_clock = {.context = &retained, .read = credential_read};
	d.cfg.platform.rental_credential_store = &unavailable_clock;
	assert(!ss_rental_can_prompt(&d));
	assert(ss_rental_sign(&d, NU54_RENTAL_LIMIT, &another_limit, 1099, digest, next_nonce, signature) != 0);
	d.cfg.platform.rental_credential_store = &credentials;
	retained.available = false;
	assert(!ss_rental_can_prompt(&d));
	d.anchored = 1; /* A stale block anchor cannot substitute for the protected clock. */
	assert(!ss_rental_can_prompt(&d));
	retained.available = true;
	assert(ss_rental_can_prompt(&d));

	retained.now = grant.expires_at - 2;
	assert(ss_rental_can_prompt(&d));
	assert(ss_rental_sign(&d, NU54_RENTAL_LIMIT, &another_limit, grant.expires_at - 3, digest, next_nonce, signature) != 0);
	assert(ss_rental_sign(&d, NU54_RENTAL_PAYMENT, &another_payment, grant.expires_at - 3, digest, next_nonce, signature) != 0);
	retained.now = grant.expires_at;
	assert(!ss_rental_can_prompt(&d));
	assert(ss_rental_sign(&d, NU54_RENTAL_LIMIT, &limit, grant.expires_at - 1, digest, nonce, signature) != 0);
	assert(nu54_device_attest_rental_credential(&d, challenge, identity, &receipt, signature) != 0);
	assert(journal.image.count == 2 && sign_calls == signs + 2);
	/* Advance after signature and journal commit, just before releasing the result. */
	retained.now = 1080;
	retained.next_now = 1091;
	retained.reads_until_advance = 4;
	assert(ss_rental_sign(&d, NU54_RENTAL_LIMIT, &another_limit, 1090, digest, next_nonce, signature) != 0);
	assert(journal.image.count == 3 && sign_calls == signs + 3);
	assert(ss_rental_sign(&d, NU54_RENTAL_LIMIT, &another_limit, 1090, digest, next_nonce, signature) != 0);
	assert(journal.image.count == 3 && sign_calls == signs + 3);

	retained.now = 1080;
	assert(ss_rental_nonce(&d, NU54_RENTAL_PAYMENT, another_payment.key, next_nonce) == 0);
	/* Start the clock transition only at the shared signer, after the preview. */
	retained.reads_until_advance = 2;
	assert(ss_rental_sign(&d, NU54_RENTAL_PAYMENT, &another_payment, 1090, digest, next_nonce, signature) != 0);
	assert(journal.image.count == 4 && sign_calls == signs + 3);
	puts("ok: authenticated protected rental credential commit, board evidence, payment and limit expiry, reset");
}

static uint64_t fixture_u64(const uint8_t bytes[8])
{
	uint64_t result = 0;
	for (unsigned i = 0; i < 8; ++i) result = (result << 8) | bytes[i];
	return result;
}

static void verify_shared_freeze_receipt(const char *projection_path, const char *receipt_path, const char *request_path)
{
	FILE *file = fopen(projection_path, "r");
	assert(file);
	char projection_hex[2571];
	assert(fscanf(file, "%2570s", projection_hex) == 1);
	fclose(file);
	uint8_t projection[1285];
	size_t projection_size = strlen(projection_hex) / 2;
	assert(projection_size >= 277 && projection_size <= sizeof(projection));
	rental_vector_decode_hex(projection_hex, projection, projection_size);

	char wire_hex[507], digest_hex[65], identity_hex[41], extra;
	file = fopen(receipt_path, "r");
	assert(file);
	assert(fscanf(file, "%506s %64s %40s %c", wire_hex, digest_hex, identity_hex, &extra) == 3);
	fclose(file);
	uint8_t wire[253], expected_digest[32], identity[20], digest[32], signer[20], root[32];
	rental_vector_decode_hex(wire_hex, wire, sizeof(wire));
	rental_vector_decode_hex(digest_hex, expected_digest, sizeof(expected_digest));
	rental_vector_decode_hex(identity_hex, identity, sizeof(identity));
	assert(!memcmp(wire, "RFR1", 4));
	nu54_rental_binding_t binding;
	memcpy(&binding, projection + 4, sizeof(binding));
	nu54_rental_freeze_receipt_t receipt = {
		.entry_count = fixture_u64(wire + 164), .frozen_at = fixture_u64(wire + 172), .expires_at = fixture_u64(wire + 180),
	};
	memcpy(receipt.binding, wire + 4, 32);
	memcpy(receipt.challenge, wire + 36, 32);
	memcpy(receipt.projection_root, wire + 68, 32);
	memcpy(receipt.first_nonce, wire + 100, 32);
	memcpy(receipt.next_nonce, wire + 132, 32);
	nu54_keccak256(projection, projection_size, root);
	assert(!memcmp(receipt.projection_root, root, sizeof(root)));
	nu54_rental_freeze_receipt_digest(&binding, &receipt, digest);
	assert(!memcmp(expected_digest, digest, sizeof(digest)));
	assert(nu54_sig_recover(digest, wire + 188, 65, signer) == NU54_SIG_OK);
	assert(!memcmp(signer, identity, sizeof(signer)));
	char request_hex[153];
	file = fopen(request_path, "r");
	assert(file);
	assert(fscanf(file, "%152s %64s %c", request_hex, digest_hex, &extra) == 2);
	fclose(file);
	uint8_t request_wire[76];
	rental_vector_decode_hex(request_hex, request_wire, sizeof(request_wire));
	rental_vector_decode_hex(digest_hex, expected_digest, sizeof(expected_digest));
	assert(!memcmp(request_wire, "RFQ2", 4));
	nu54_rental_freeze_request_t request = {.expires_at = fixture_u64(request_wire + 68)};
	memcpy(request.challenge, request_wire + 4, sizeof(request.challenge));
	memcpy(request.channel_nonce, request_wire + 36, sizeof(request.channel_nonce));
	nu54_rental_freeze_request_digest(&binding, &request, digest);
	assert(!memcmp(expected_digest, digest, sizeof(digest)));
	puts("ok: shared RFQ2 request, RFR1 typed receipt and board signer");
}

int main(int argc, char *argv[])
{
	if (argc == 2 && (!strcmp(argv[1], "--w12-credential-fixture") || !strcmp(argv[1], "--w12-slot-open-fixture"))) {
		ctx = secp256k1_context_create(SECP256K1_CONTEXT_NONE);
		int status = credential_fixture_command(!strcmp(argv[1], "--w12-slot-open-fixture"));
		secp256k1_context_destroy(ctx);
		return status;
	}
	if (argc == 4) verify_shared_freeze_receipt(argv[1], argv[2], argv[3]);
	int failures = 0;
	static nu54_device_t d;
	static nu54_out_t out;
	ctx = secp256k1_context_create(SECP256K1_CONTEXT_NONE);

	for (size_t s = 0; s < SESSION_SCENARIO_COUNT; s++) {
		const session_scenario_t *sc = &SESSION_SCENARIOS[s];
		memset(&d, 0, sizeof(d));
		d.cfg = host_config();
		d.cfg.require_secure = sc->require_secure;
		d.cfg.require_phone = sc->require_phone;
		d.cfg.dev_unpaired_anchor = sc->dev_unpaired_anchor;
		const nu54_link_t link = {.bonded = sc->link_bonded, .phone_present = sc->phone_present};
		strcpy(stored_pin, "2580");
		pin_failures = 0;
		if (sc->unprovisioned) {
			nu54_device_init_unprovisioned(&d);
		} else {
			memcpy(d.address, SV_ADDRESS, 20);
			memcpy(d.operator_address, SV_OPERATOR, 20);
			memcpy(d.contract, SV_CONTRACT, 20);
			memcpy(d.chain_id, SV_CHAIN_ID, 32);
			nu54_device_init(&d, SV_NONCE_START);
		}
		memcpy(key, SV_KEY, 32);
		draws = 0;

		for (size_t t = 0; t < sc->count; t++) {
			const session_step_t *st = &sc->steps[t];
			if (st->power_cycle) {
				nu54_device_power_cycle(&d);
				continue;
			}
			nu54_session_handle(&d, &link, st->send, st->send_len, st->at, &out);
			/* The renter answers whatever the device waits for: the button (payment, setup values,
			 * limit change after its PIN) or the PIN (setup, limit change). */
			for (int guard = 0; d.pending != NU54_PENDING_NONE && guard < 4; guard++) {
				if (d.pending == NU54_PENDING_PIN || d.pending == NU54_PENDING_LIMIT_PIN) {
					nu54_session_pin(&d, sc->pin, sc->pin ? strlen(sc->pin) : 0, &out);
				} else {
					nu54_session_button(&d, sc->approve, &out);
				}
			}
			/* After the button: wallet.check answers the phone app then. */
			int phone_ok = out.phone_count == (st->phone ? 1 : 0) &&
				       (!st->phone || (out.phone_len == st->phone_len && memcmp(out.phone, st->phone, st->phone_len) == 0));
			int ok = phone_ok && out.kiosk_count == st->expect_count && (int)out.event == st->event; /* the LED event the replies mean */
			for (int e = 0; ok && e < st->expect_count; e++) {
				ok = out.kiosk_len[e] == st->expect_len[e] && memcmp(out.kiosk[e], st->expect[e], st->expect_len[e]) == 0;
			}
			if (!ok) {
				failures++;
				printf("FAIL %s step %zu\n", sc->id, t);
				for (int e = 0; e < st->expect_count; e++) {
					dump("want", st->expect[e], st->expect_len[e]);
				}
				for (int e = 0; e < out.kiosk_count; e++) {
					dump("got ", out.kiosk[e], out.kiosk_len[e]);
				}
				if (!phone_ok) {
					printf("  phone body differs\n");
				}
			}
		}
	}
	/* The phone app's device.paymentMode arrives on its own link in the middle of SV-24's secure
	 * session: answered in plaintext, and the kiosk's sealed bodies that follow still open. */
	{
		const session_scenario_t *sec = NULL, *mode = NULL;
		for (size_t s = 0; s < SESSION_SCENARIO_COUNT; s++) {
			sec = strcmp(SESSION_SCENARIOS[s].id, "SV-24") == 0 ? &SESSION_SCENARIOS[s] : sec;
			mode = strcmp(SESSION_SCENARIOS[s].id, "SV-32") == 0 ? &SESSION_SCENARIOS[s] : mode;
		}
		memset(&d, 0, sizeof(d));
		d.cfg = host_config();
		const nu54_link_t kiosk = {.bonded = 1, .phone_present = 1}; /* as the vectors assume */
		const nu54_link_t phone = {.bonded = 1, .foreign = 1};
		memcpy(d.address, SV_ADDRESS, 20);
		memcpy(d.operator_address, SV_OPERATOR, 20);
		memcpy(d.contract, SV_CONTRACT, 20);
		memcpy(d.chain_id, SV_CHAIN_ID, 32);
		nu54_device_init(&d, SV_NONCE_START);
		memcpy(key, SV_KEY, 32);
		draws = 0;
		int ok = sec && mode;
		for (size_t t = 0; ok && t < sec->count; t++) {
			const session_step_t *st = &sec->steps[t];
			if (t == 4) { /* after session.confirm, before payment.identify */
				const session_step_t *ms = &mode->steps[3]; /* on for 120 s */
				nu54_session_handle(&d, &phone, ms->send, ms->send_len, st->at, &out);
				ok = out.kiosk_count == 1 && out.kiosk_len[0] == ms->expect_len[0] && memcmp(out.kiosk[0], ms->expect[0], ms->expect_len[0]) == 0;
			}
			nu54_session_handle(&d, &kiosk, st->send, st->send_len, st->at, &out);
			for (int guard = 0; d.pending != NU54_PENDING_NONE && guard < 4; guard++) {
				nu54_session_button(&d, 1, &out);
			}
			ok = ok && out.kiosk_count == st->expect_count;
			for (int e = 0; ok && e < st->expect_count; e++) {
				ok = out.kiosk_len[e] == st->expect_len[e] && memcmp(out.kiosk[e], st->expect[e], st->expect_len[e]) == 0;
			}
		}
		if (!ok) {
			failures++;
			printf("FAIL payment mode from another link during a secure session\n");
		}
	}
	/* Actual signed payment.prepare and legacy device.reset before generation activation. */
	journal_candidate_refuses("SV-01", 5);
	journal_candidate_refuses("SV-17", 5);
	const session_scenario_t *journal_vector = find_scenario("SV-18");
	assert(journal_vector);
	journal_limit_and_payment(journal_vector);
	journal_merchant_order_scope();
	protected_rental_credential();
	secp256k1_context_destroy(ctx);
	/* SV-32: on for 120 s and off reach the platform; refusals do not (plus the call above). */
	if (mode_calls != 3) {
		failures++;
		printf("FAIL payment mode: %d platform calls (want 3)\n", mode_calls);
	}
	/* Setup stores once per finished setup (SV-13, SV-36) and wipes on the PIN timeout (SV-15) and the
	 * resets (SV-17, SV-34). */
	if (commits != 2 || wipes != 3) {
		failures++;
		printf("FAIL setup storage: %d commits (want 2), %d wipes (want 3)\n", commits, wipes);
	}
	/* A link that drops while setup waits for the PIN wipes the key setup made. */
	memset(&d, 0, sizeof(d));
	d.cfg = host_config();
	nu54_device_init_unprovisioned(&d);
	d.session_open = 1;
	d.pending = NU54_PENDING_PIN;
	nu54_session_link_closed(&d);
	if (wipes != 4 || d.pending != NU54_PENDING_NONE || d.session_open) {
		failures++;
		printf("FAIL link closed during the setup PIN: %d wipes (want 4), pending %d\n", wipes, (int)d.pending);
	}
	/* pin.entry is a schema message the phone app can decode. */
	{
		static nu54_msg_t m;
		uint8_t body[NU54_PIN_ENTRY_BODY_MAX]; /* the size the board uses */
		size_t n = nu54_pin_entry_body("3200", 1, body, sizeof(body));
		printf("pin.entry body: %zu bytes\n", n);
		int ok = n > 0 && nu54_msg_decode(body, n, &m) == NU54_MSG_OK && strcmp(m.message->type, "pin.entry") == 0;
		int d = ok ? nu54_msg_find(&m, 0, "digits") : -1;
		if (!ok || d < 0 || m.items[d].len != 4 || memcmp(m.items[d].ptr, "3200", 4) != 0) {
			failures++;
			printf("FAIL pin.entry body\n");
		}
	}
	journal_candidate_through_setup_pin();
	printf("%s: %zu session scenarios, %d failures\n", failures ? "FAIL" : "ok", (size_t)SESSION_SCENARIO_COUNT, failures);
	return failures ? 1 : 0;
}
