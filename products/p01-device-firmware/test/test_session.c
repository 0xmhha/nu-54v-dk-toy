/*
 * The firmware session against the shared session vectors: every scenario is replayed and each
 * reply must equal the vector byte for byte (signatures included: the host signs with RFC 6979
 * like the simulator, then nu54_sig_finish makes the protocol form as on the device).
 */
#include <stdio.h>
#include <string.h>

#include "nu54_session.h"
#include "secp256k1.h"
#include "vectors.h"

static secp256k1_context *ctx;
static int draws;

static int host_sign(void *c, const uint8_t digest[32], uint8_t rs[64])
{
	(void)c;
	secp256k1_ecdsa_signature sig;
	if (!secp256k1_ecdsa_sign(ctx, &sig, digest, SV_KEY, NULL, NULL)) {
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
		memcpy(d.address, SV_ADDRESS, 20);
		memcpy(d.operator_address, SV_OPERATOR, 20);
		memcpy(d.contract, SV_CONTRACT, 20);
		memcpy(d.chain_id, SV_CHAIN_ID, 32);
		d.anchor_clock_skew = SV_ANCHOR_SKEW;
		d.authorization_expiry = SV_AUTH_EXPIRY;
		d.firmware = SV_FIRMWARE;
		d.platform = (nu54_platform_t){host_sign, vector_random, NULL, NULL};
		nu54_device_init(&d, SV_NONCE_START);
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
			if (out.phone_count) {
				nu54_session_button(&d, sc->approve, &out); /* the renter's button after confirm.show */
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
	printf("%s: %zu session scenarios, %d failures\n", failures ? "FAIL" : "ok", (size_t)SESSION_SCENARIO_COUNT, failures);
	return failures ? 1 : 0;
}
