/*
 * The firmware session against the shared session vectors: every scenario is replayed and each
 * reply must equal the vector byte for byte (signatures included: the host signs with RFC 6979
 * like the simulator, then nu54_sig_finish makes the protocol form as on the device). Secure
 * scenarios (payment-protocol.md 4.1) use OpenSSL for the platform's HKDF and AES-GCM.
 */
#include <stdio.h>
#include <string.h>

#include <openssl/evp.h>
#include <openssl/kdf.h>

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

int main(void)
{
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
	printf("%s: %zu session scenarios, %d failures\n", failures ? "FAIL" : "ok", (size_t)SESSION_SCENARIO_COUNT, failures);
	return failures ? 1 : 0;
}
