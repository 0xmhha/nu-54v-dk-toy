/*
 * The firmware session against the shared session vectors: every scenario is replayed and each
 * reply must equal the vector byte for byte (signatures included: the host signs with RFC 6979
 * like the simulator, then nu54_sig_finish makes the protocol form as on the device).
 */
#include <stdio.h>
#include <string.h>

#include "nu54_keccak.h"
#include "nu54_session.h"
#include "secp256k1.h"
#include "vectors.h"

static secp256k1_context *ctx;
static int draws;
static uint8_t key[32];     /* the device key: SV_KEY, or the one setup made */
static uint8_t new_key[32]; /* made by generate_key, kept only when setup commits */
static int commits, wipes;
static char stored_pin[16];  /* what setup stored; a provisioned vector device has 2580 */
static int pin_failures;     /* kept in secure storage on the board */
#define PIN_MAX_RETRIES 5

static int host_sign(void *c, const uint8_t digest[32], uint8_t rs[64])
{
	(void)c;
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

static void dump(const char *what, const uint8_t *b, size_t n)
{
	printf("  %s (%zu): ", what, n);
	for (size_t i = 0; i < n; i++) {
		printf("%02x", b[i]);
	}
	printf("\n");
}

int main(void)
{
	int failures = 0;
	static nu54_device_t d;
	static nu54_out_t out;
	ctx = secp256k1_context_create(SECP256K1_CONTEXT_NONE);

	for (size_t s = 0; s < SESSION_SCENARIO_COUNT; s++) {
		const session_scenario_t *sc = &SESSION_SCENARIOS[s];
		memset(&d, 0, sizeof(d));
		d.anchor_clock_skew = SV_ANCHOR_SKEW;
		d.authorization_expiry = SV_AUTH_EXPIRY;
		d.firmware = SV_FIRMWARE;
		d.platform = (nu54_platform_t){host_sign, vector_random, NULL, NULL, host_generate_key, host_commit, host_wipe, host_check_pin};
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
			nu54_session_handle(&d, st->send, st->send_len, st->at, &out);
			int phone_ok = out.phone_count == (st->phone ? 1 : 0) &&
				       (!st->phone || (out.phone_len == st->phone_len && memcmp(out.phone, st->phone, st->phone_len) == 0));
			/* The renter answers whatever the device waits for: the button (payment, setup values,
			 * limit change after its PIN) or the PIN (setup, limit change). */
			for (int guard = 0; d.pending != NU54_PENDING_NONE && guard < 4; guard++) {
				if (d.pending == NU54_PENDING_PIN || d.pending == NU54_PENDING_LIMIT_PIN) {
					nu54_session_pin(&d, sc->pin, sc->pin ? strlen(sc->pin) : 0, &out);
				} else {
					nu54_session_button(&d, sc->approve, &out);
				}
			}
			int ok = phone_ok && out.kiosk_count == st->expect_count;
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
	secp256k1_context_destroy(ctx);
	/* Setup stores once per finished setup (SV-13) and wipes on the PIN timeout and the reset. */
	if (commits != 1 || wipes != 2) {
		failures++;
		printf("FAIL setup storage: %d commits (want 1), %d wipes (want 2)\n", commits, wipes);
	}
	printf("%s: %zu session scenarios, %d failures\n", failures ? "FAIL" : "ok", (size_t)SESSION_SCENARIO_COUNT, failures);
	return failures ? 1 : 0;
}
