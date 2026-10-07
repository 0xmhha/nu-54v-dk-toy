#include "device_setup.h"

#include <errno.h>
#include <string.h>

#include <psa/crypto.h>
#include <psa/internal_trusted_storage.h>
#include <zephyr/logging/log.h>
#include <zephyr/settings/settings.h>

#include "ble_links.h"
#include "dev_setup.h"
#include "device_key.h"

LOG_MODULE_REGISTER(device_setup, LOG_LEVEL_INF);

/* pinMaxRetries (session-vectors.json parameters; design register bound 3..10). */
#define PIN_MAX_RETRIES 5
#define PIN_KEY_ID ((psa_key_id_t)0x00005402)
#define PIN_ALG PSA_ALG_HMAC(PSA_ALG_SHA_256)

/* Secure storage entries. The record is written last at setup and removed first at a wipe, so
 * its presence means a finished setup and anything else without it is a leftover to wipe. */
#define UID_RECORD ((psa_storage_uid_t)0x5410)
#define UID_PIN ((psa_storage_uid_t)0x5411)
#define UID_PIN_FAILURES ((psa_storage_uid_t)0x5412)

struct stored_record {
	uint8_t operator_address[20];
	uint8_t contract[20];
	uint8_t chain_id[32];
	uint32_t passkey;
};

/* ---------------------------------------------------------------- nonce counter (settings) */

static uint8_t stored_nonce[32];
static int nonce_loaded;

static int settings_set(const char *name, size_t len, settings_read_cb read_cb, void *cb_arg)
{
	if (strcmp(name, "nonce") == 0 && len == 32) {
		nonce_loaded = read_cb(cb_arg, stored_nonce, 32) == 32;
	}
	return 0;
}

SETTINGS_STATIC_HANDLER_DEFINE(nu54, "nu54", NULL, settings_set, NULL, NULL);

static int persist_nonce(void *ctx, const uint8_t next[32])
{
	(void)ctx;
	return settings_save_one("nu54/nonce", next, 32);
}

/* ---------------------------------------------------------------- secure storage helpers */

static int gone(psa_status_t st)
{
	return st == PSA_SUCCESS || st == PSA_ERROR_DOES_NOT_EXIST || st == PSA_ERROR_INVALID_HANDLE;
}

static int its_read(psa_storage_uid_t uid, void *buf, size_t len)
{
	size_t n;
	return psa_its_get(uid, 0, len, buf, &n) == PSA_SUCCESS && n == len ? 0 : -ENOENT;
}

static int its_write(psa_storage_uid_t uid, const void *buf, size_t len)
{
	return psa_its_set(uid, len, buf, PSA_STORAGE_FLAG_NONE) == PSA_SUCCESS ? 0 : -EIO;
}

/* The PIN key: a random HMAC key that never leaves PSA, so the stored PIN HMAC cannot be
 * brute-forced off the device. */
static int pin_key_generate(void)
{
	psa_key_attributes_t attr = PSA_KEY_ATTRIBUTES_INIT;
	psa_key_id_t id;
	psa_status_t st = psa_destroy_key(PIN_KEY_ID);

	if (!gone(st)) {
		return -EIO;
	}
	psa_set_key_id(&attr, PIN_KEY_ID);
	psa_set_key_lifetime(&attr, PSA_KEY_LIFETIME_PERSISTENT);
	psa_set_key_type(&attr, PSA_KEY_TYPE_HMAC);
	psa_set_key_bits(&attr, 256);
	psa_set_key_usage_flags(&attr, PSA_KEY_USAGE_SIGN_MESSAGE | PSA_KEY_USAGE_VERIFY_MESSAGE);
	psa_set_key_algorithm(&attr, PIN_ALG);
	st = psa_generate_key(&attr, &id);
	psa_reset_key_attributes(&attr);
	return st == PSA_SUCCESS ? 0 : -EIO;
}

/* ---------------------------------------------------------------- platform callbacks */

static int sign(void *ctx, const uint8_t digest[32], uint8_t rs[64])
{
	(void)ctx;
	return device_key_sign(digest, rs);
}

static void random_bytes(void *ctx, uint8_t *out, size_t n)
{
	(void)ctx;
	(void)psa_generate_random(out, n);
}

/* The new key replaces nothing that matters: setup only runs UNPROVISIONED. It is not a setup
 * until commit_setup writes the record; a reset before that wipes it at boot. */
static int generate_key(void *ctx, uint8_t address[20])
{
	(void)ctx;
	return device_key_generate(address) == 0 ? 0 : -EIO;
}

static int commit_setup(void *ctx, const nu54_setup_record_t *rec)
{
	struct stored_record r;
	uint8_t mac[PSA_HASH_LENGTH(PSA_ALG_SHA_256)];
	uint8_t failures = 0;
	size_t mac_len;
	int err;

	(void)ctx;
	if (!rec->pin || rec->pin_len == 0) {
		return -EINVAL;
	}
	err = pin_key_generate();
	if (!err && psa_mac_compute(PIN_KEY_ID, PIN_ALG, (const uint8_t *)rec->pin, rec->pin_len, mac, sizeof(mac), &mac_len) !=
			    PSA_SUCCESS) {
		err = -EIO;
	}
	if (!err) {
		err = its_write(UID_PIN, mac, sizeof(mac));
	}
	if (!err) {
		err = its_write(UID_PIN_FAILURES, &failures, 1);
	}
	if (!err) {
		err = persist_nonce(NULL, rec->nonce_start);
	}
	if (!err) {
		memcpy(r.operator_address, rec->operator_address, 20);
		memcpy(r.contract, rec->contract, 20);
		memcpy(r.chain_id, rec->chain_id, 32);
		r.passkey = rec->passkey;
		err = its_write(UID_RECORD, &r, sizeof(r)); /* last: the setup is now finished */
	}
	if (err) {
		LOG_ERR("setup not stored (%d)", err);
	}
	return err;
}

/* Matches the host test's check: locked at PIN_MAX_RETRIES failures, a right PIN clears them. */
static int check_pin(void *ctx, const char *pin, size_t len)
{
	uint8_t failures, mac[PSA_HASH_LENGTH(PSA_ALG_SHA_256)];

	(void)ctx;
	if (its_read(UID_PIN_FAILURES, &failures, 1) != 0 || its_read(UID_PIN, mac, sizeof(mac)) != 0) {
		return NU54_PIN_WRONG; /* no PIN stored (week-7 fixed setup): nothing can match */
	}
	if (failures >= PIN_MAX_RETRIES) {
		return NU54_PIN_LOCKED;
	}
	/* Count the attempt before comparing, so cutting power after a wrong PIN does not undo it. */
	failures++;
	if (its_write(UID_PIN_FAILURES, &failures, 1) != 0) {
		return NU54_PIN_WRONG;
	}
	if (psa_mac_verify(PIN_KEY_ID, PIN_ALG, (const uint8_t *)pin, len, mac, sizeof(mac)) == PSA_SUCCESS) {
		failures = 0;
		if (its_write(UID_PIN_FAILURES, &failures, 1) != 0) {
			LOG_WRN("PIN failure counter not cleared");
		}
		return NU54_PIN_OK;
	}
	return failures >= PIN_MAX_RETRIES ? NU54_PIN_LOCKED : NU54_PIN_WRONG;
}

/* Secure channel (payment-protocol.md 4.1): HKDF-SHA256 and AES-128-GCM on CRACEN. */
static int hkdf(void *ctx, const uint8_t ikm[32], const uint8_t salt[64], const char *info, uint8_t key[16])
{
	psa_key_derivation_operation_t op = PSA_KEY_DERIVATION_OPERATION_INIT;
	psa_status_t st = psa_key_derivation_setup(&op, PSA_ALG_HKDF(PSA_ALG_SHA_256));

	(void)ctx;
	if (st == PSA_SUCCESS) {
		st = psa_key_derivation_input_bytes(&op, PSA_KEY_DERIVATION_INPUT_SALT, salt, 64);
	}
	if (st == PSA_SUCCESS) {
		st = psa_key_derivation_input_bytes(&op, PSA_KEY_DERIVATION_INPUT_SECRET, ikm, 32);
	}
	if (st == PSA_SUCCESS) {
		st = psa_key_derivation_input_bytes(&op, PSA_KEY_DERIVATION_INPUT_INFO, (const uint8_t *)info, strlen(info));
	}
	if (st == PSA_SUCCESS) {
		st = psa_key_derivation_output_bytes(&op, key, 16);
	}
	psa_key_derivation_abort(&op);
	return st == PSA_SUCCESS ? 0 : -EIO;
}

/* One volatile key per call: the session key stays in the session's RAM, not in PSA slots. */
static int gcm(int seal, const uint8_t key[16], const uint8_t iv[12], const uint8_t *in, size_t len, uint8_t *out)
{
	psa_key_attributes_t attr = PSA_KEY_ATTRIBUTES_INIT;
	psa_key_id_t id;
	size_t n;
	psa_status_t st;

	psa_set_key_type(&attr, PSA_KEY_TYPE_AES);
	psa_set_key_bits(&attr, 128);
	psa_set_key_usage_flags(&attr, PSA_KEY_USAGE_ENCRYPT | PSA_KEY_USAGE_DECRYPT);
	psa_set_key_algorithm(&attr, PSA_ALG_GCM);
	st = psa_import_key(&attr, key, 16, &id);
	psa_reset_key_attributes(&attr);
	if (st != PSA_SUCCESS) {
		return -EIO;
	}
	if (seal) {
		st = psa_aead_encrypt(id, PSA_ALG_GCM, iv, 12, NULL, 0, in, len, out, len + 16, &n);
	} else {
		st = len < 16 ? PSA_ERROR_INVALID_SIGNATURE : psa_aead_decrypt(id, PSA_ALG_GCM, iv, 12, NULL, 0, in, len, out, len - 16, &n);
	}
	psa_destroy_key(id);
	return st == PSA_SUCCESS ? 0 : -EIO;
}

static int aead_seal(void *ctx, const uint8_t key[16], const uint8_t iv[12], const uint8_t *in, size_t len, uint8_t *out)
{
	(void)ctx;
	return gcm(1, key, iv, in, len, out);
}

static int aead_open(void *ctx, const uint8_t key[16], const uint8_t iv[12], const uint8_t *in, size_t len, uint8_t *out)
{
	(void)ctx;
	return gcm(0, key, iv, in, len, out);
}

static int wipe(void *ctx)
{
	int ok;

	struct stored_record r;
	/* A stored rental, not a setup that stopped before storing (refused, PIN timeout, link lost). */
	bool rental = its_read(UID_RECORD, &r, sizeof(r)) == 0;

	(void)ctx;
	ok = gone(psa_its_remove(UID_RECORD)); /* first: without it the rest is a leftover */
	ok &= device_key_destroy() == 0;
	ok &= gone(psa_destroy_key(PIN_KEY_ID));
	ok &= gone(psa_its_remove(UID_PIN));
	ok &= gone(psa_its_remove(UID_PIN_FAILURES));
	ok &= settings_delete("nu54/nonce") == 0;
	/* Bonds go with the rental: the next renter's phone pairs again with the new passkey. A setup
	 * that stopped keeps the phone's link so the phone app gets the answer (its bond goes when it
	 * leaves, ble_links.c). */
	if (rental) {
		ok &= ble_links_unpair_all() == 0;
	}
	nonce_loaded = 0;
	LOG_INF("wipe: %s", ok ? "done" : "incomplete");
	return ok ? 0 : -EIO;
}

/* ---------------------------------------------------------------- boot */

static int load_rental(nu54_device_t *d, const struct stored_record *r)
{
	uint8_t failures;

	if (device_key_open(d->address) != 0 || !nonce_loaded) {
		return -ENOENT;
	}
	memcpy(d->operator_address, r->operator_address, 20);
	memcpy(d->contract, r->contract, 20);
	memcpy(d->chain_id, r->chain_id, 32);
	d->passkey = r->passkey;
	nu54_device_init(d, stored_nonce);
	if (its_read(UID_PIN_FAILURES, &failures, 1) == 0 && failures >= PIN_MAX_RETRIES) {
		d->state = NU54_STATE_PIN_LOCKED; /* only DeviceReset clears it */
	}
	LOG_INF("rental setup loaded%s", d->state == NU54_STATE_PIN_LOCKED ? " (PIN locked)" : "");
	return 0;
}

static int load_fixed(nu54_device_t *d)
{
	int created, err = device_key_init(d->address, &created);

	if (err) {
		return err;
	}
	LOG_INF("device key %s (week-7 fixed setup)", created ? "created" : "opened");
	memcpy(d->operator_address, DEV_OPERATOR, 20);
	memcpy(d->contract, DEV_CONTRACT, 20);
	memcpy(d->chain_id, DEV_CHAIN_ID, 32);
	if (!nonce_loaded) {
		/* First boot: a random 256-aligned start (payment-protocol.md 2), stored at once. */
		random_bytes(NULL, stored_nonce, 31);
		stored_nonce[31] = 0;
		stored_nonce[0] = 0; /* keep far below 2^256 so the counter never wraps */
		err = persist_nonce(NULL, stored_nonce);
		if (err) {
			return err;
		}
	}
	nu54_device_init(d, stored_nonce);
	return 0;
}

int device_setup_load(nu54_device_t *d)
{
	struct stored_record r;
	int err;

	d->cfg.platform = (nu54_platform_t){
		.sign = sign,
		.random = random_bytes,
		.persist_nonce = persist_nonce,
		.generate_key = generate_key,
		.commit_setup = commit_setup,
		.wipe = wipe,
		.check_pin = check_pin,
		.hkdf = hkdf,
		.aead_seal = aead_seal,
		.aead_open = aead_open,
	};
	if (psa_crypto_init() != PSA_SUCCESS) {
		return -EIO;
	}
	err = settings_subsys_init();
	if (!err) {
		err = settings_load_subtree("nu54");
	}
	if (err) {
		return err;
	}
	if (its_read(UID_RECORD, &r, sizeof(r)) == 0) {
		if (load_rental(d, &r) == 0) {
			return 0;
		}
		/* A record without its key or nonce cannot sign safely; come up empty so the operator
		 * can set the device up again (the on-chain account is closed separately). */
		LOG_ERR("stored setup is incomplete; wiping it");
		(void)wipe(NULL);
	} else if (IS_ENABLED(CONFIG_NU54_DEV_SETUP)) {
		return load_fixed(d); /* keeps the fixed-setup key and nonce across boots */
	} else {
		(void)wipe(NULL); /* whatever a setup that never finished left behind */
	}
	nu54_device_init_unprovisioned(d);
	LOG_INF("unprovisioned: waiting for a setup session");
	return 0;
}
