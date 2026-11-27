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
#include "device_vault.h"
#if defined(CONFIG_NU54_EXPERIMENTAL_RENTAL_JOURNAL_PS)
#include <zephyr/kernel.h>
#include "device_rental_journal_ps.h"
#endif

LOG_MODULE_REGISTER(device_setup, LOG_LEVEL_INF);

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

enum rental_record_kind {
	RENTAL_ABSENT = 0,
	RENTAL_LEGACY,
	RENTAL_ACTIVE = 0x52454e31,
	RENTAL_FENCED = 0x52454e32,
	RENTAL_RETIRED = 0x52454e33,
	RENTAL_JOURNAL_REQUIRED = 0x52454e34,
};

struct rental_record {
	struct stored_record fields;
	uint32_t kind;
	nu54_rental_binding_t binding;
	uint8_t first[32];
};

#define RENTAL_V1_RECORD_SIZE offsetof(struct rental_record, binding)
_Static_assert(RENTAL_V1_RECORD_SIZE == 80, "legacy rental records must remain readable");

/* Read size before selecting an on-device format. An unreadable record is not an absent one. */
static int read_rental_record(struct rental_record *record)
{
	struct psa_storage_info_t info;
	psa_status_t status = psa_its_get_info(UID_RECORD, &info);
	memset(record, 0, sizeof(*record));
	if (status == PSA_ERROR_DOES_NOT_EXIST) return RENTAL_ABSENT;
	if (status != PSA_SUCCESS || (info.size != sizeof(record->fields) && info.size != RENTAL_V1_RECORD_SIZE &&
				      info.size != sizeof(*record))) return -EIO;
	size_t received;
	status = psa_its_get(UID_RECORD, 0, info.size, record, &received);
	if (status != PSA_SUCCESS || received != info.size) return -EIO;
	if (info.size == sizeof(record->fields)) return RENTAL_LEGACY;
	if (info.size == sizeof(*record))
		return record->kind == RENTAL_JOURNAL_REQUIRED ? RENTAL_JOURNAL_REQUIRED : -EIO;
	if (record->kind != RENTAL_ACTIVE && record->kind != RENTAL_FENCED && record->kind != RENTAL_RETIRED) return -EIO;
	return (int)record->kind;
}

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

#if defined(CONFIG_NU54_EXPERIMENTAL_RENTAL_JOURNAL_PS)
int device_setup_prepare_rental_generation(nu54_device_t *d, const nu54_rental_registration_t *registration,
					   const nu54_rental_registration_trust_t *trust, const uint8_t signature[65])
{
	if (!d || !registration || !trust || !signature || d->state != NU54_STATE_UNPROVISIONED ||
	    !d->session_open || !d->session_setup || !d->session_bonded || d->pending != NU54_PENDING_PIN ||
	    !device_rental_journal_platform_qualified()) return -EACCES;

	nu54_setup_record_t *setup = &d->pending_setup;
	uint8_t measurement[32];
	if (!setup->journal_requested || !device_rental_journal_measurement(measurement) ||
	    memcmp(measurement, trust->binding.measurement, sizeof(measurement)) ||
	    memcmp(d->pending_address, trust->binding.account, sizeof(d->pending_address)) ||
	    memcmp(setup->chain_id, trust->binding.chain, sizeof(setup->chain_id)) ||
	    memcmp(setup->contract, trust->binding.contract, sizeof(setup->contract)) ||
	    memcmp(setup->operator_address, trust->operator_address, sizeof(setup->operator_address)) ||
	    memcmp(setup->nonce_start, trust->first, sizeof(setup->nonce_start)) ||
	    (setup->journal_required && memcmp(&setup->rental_binding, &trust->binding, sizeof(trust->binding))) ||
	    nu54_rental_registration_verify(registration, trust, signature) != 0) return -EACCES;

	setup->rental_binding = trust->binding;
	setup->journal_required = 1;
	return 0;
}

static int can_commission_rental_journal(void *ctx)
{
	(void)ctx;
	uint8_t measurement[32];
	return device_rental_journal_platform_qualified() && device_rental_journal_measurement(measurement);
}

static uint64_t rental_monotonic_seconds(void *ctx)
{
	(void)ctx;
	return (uint64_t)k_uptime_get() / 1000;
}

static int sign_rental_attestation(void *ctx, const uint8_t digest[32], uint8_t rs[64])
{
	(void)ctx;
	if (!device_rental_journal_platform_qualified()) return -EIO;
	return device_key_slot_sign(DEVICE_KEY_IDENTITY, digest, rs);
}

static int allow_rental_collector(void *ctx, const nu54_rental_binding_t *binding,
				  const uint8_t peer[NU54_RENTAL_PEER_SIZE])
{
	(void)ctx;
	return device_rental_journal_platform_qualified() && device_rental_collector_peer_allowed(binding, peer);
}

static int device_setup_rental_freeze(nu54_device_t *d, const nu54_rental_freeze_request_t *request,
			       const uint8_t operator_signature[65], nu54_rental_freeze_receipt_t *receipt,
			       uint8_t board_signature[65])
{
	uint8_t identity[20], first[32];
	nu54_rental_binding_t binding;
	bool active;
	if (!d || !d->call.bonded || !d->rental_journal ||
	    device_rental_journal_marker_ps(&binding, first, &active) != 0 || !active ||
	    memcmp(&binding, &d->rental_journal->expected_binding, sizeof(binding)) ||
	    memcmp(first, d->rental_journal->expected_first, sizeof(first)) ||
	    device_key_slot_open(DEVICE_KEY_IDENTITY, false, identity) != 0) return -EIO;
	return nu54_device_attest_rental_freeze(d, request, operator_signature, identity, receipt, board_signature);
}
static int begin_rental_cleanup(void *ctx)
{
	return device_rental_retirement_platform_qualified() ? device_rental_cleanup_begin(ctx) : -EACCES;
}

static int rental_cleanup_binding(void *ctx, nu54_rental_binding_t *binding)
{
	(void)ctx;
	uint8_t first[32];
	bool active = false;
	return device_rental_journal_marker_ps(binding, first, &active) == 0 && active ? 0 : -EACCES;
}

static int fence_rental_cleanup(void *ctx, const nu54_rental_retirement_authority_t *authority, uint64_t now)
{
	(void)ctx;
	return device_rental_journal_retirement_fence_ps(authority, now);
}

static int freeze_rental(void *ctx, nu54_device_t *d, const nu54_rental_freeze_request_t *request,
			 const uint8_t operator_signature[65], nu54_rental_freeze_receipt_t *receipt,
			 uint8_t board_signature[65])
{
	(void)ctx;
	return device_setup_rental_freeze(d, request, operator_signature, receipt, board_signature);
}

#endif

#if defined(CONFIG_NU54_EXPERIMENTAL_RENTAL_JOURNAL_PS)
static int rental_challenge_random(void *ctx, uint8_t out[32])
{
	(void)ctx;
	return psa_generate_random(out, 32) == PSA_SUCCESS ? 0 : -EIO;
}
#endif

static void random_bytes(void *ctx, uint8_t *out, size_t n)
{
	(void)ctx;
	(void)psa_generate_random(out, n);
}

/* Stage only the dedicated rental key. The durable setup record enables legacy-wire signing. */
static int generate_key(void *ctx, uint8_t address[20])
{
	struct rental_record record;
	int kind = read_rental_record(&record);
	(void)ctx;
	if (kind != RENTAL_ABSENT && kind != RENTAL_RETIRED) return -EIO;
	return device_key_slot_open(DEVICE_KEY_RENTER, true, address) == 0 ? 0 : -EIO;
}

static int commit_setup(void *ctx, const nu54_setup_record_t *rec)
{
	struct rental_record r = {.kind = RENTAL_ACTIVE};
	uint8_t address[20];
	uint8_t mac[PSA_HASH_LENGTH(PSA_ALG_SHA_256)];
	uint8_t failures = 0;
	size_t mac_len;
	int err;

	(void)ctx;
	if (!nu54_pin_is_local_format(rec->pin, rec->pin_len)) {
		return -EINVAL;
	}
	struct rental_record prior;
	int kind = read_rental_record(&prior);
	if ((kind != RENTAL_ABSENT && kind != RENTAL_RETIRED) ||
	    device_key_slot_open(DEVICE_KEY_RENTER, false, address) != 0) return -EIO;
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
	if (!err && !rec->journal_required) err = persist_nonce(NULL, rec->nonce_start);
	if (!err) {
		memcpy(r.fields.operator_address, rec->operator_address, 20);
		memcpy(r.fields.contract, rec->contract, 20);
		memcpy(r.fields.chain_id, rec->chain_id, 32);
		r.fields.passkey = rec->passkey;
		if (rec->journal_required) {
#if defined(CONFIG_NU54_EXPERIMENTAL_RENTAL_JOURNAL_PS)
			r.kind = RENTAL_JOURNAL_REQUIRED;
			r.binding = rec->rental_binding;
			memcpy(r.first, rec->nonce_start, sizeof(r.first));
			err = its_write(UID_RECORD, &r, sizeof(r));
			if (!err) err = device_rental_journal_stage_ps(&r.binding, r.first);
#else
			err = -EACCES;
#endif
		} else {
			err = its_write(UID_RECORD, &r, RENTAL_V1_RECORD_SIZE);
			if (!err) err = device_key_activate_rental(DEVICE_KEY_RENTER, false, address);
		}
	}
	if (err) {
		LOG_ERR("setup not stored (%d)", err);
	}
	return err;
}

static int read_pin_attempts(void *context, uint8_t *count)
{
	(void)context;
	return its_read(UID_PIN_FAILURES, count, 1);
}

static int write_pin_attempts(void *context, uint8_t count)
{
	(void)context;
	return its_write(UID_PIN_FAILURES, &count, 1);
}

static bool verify_local_pin(void *context, const char *pin, size_t length)
{
	(void)context;
	uint8_t mac[PSA_HASH_LENGTH(PSA_ALG_SHA_256)];
	return its_read(UID_PIN, mac, sizeof(mac)) == 0 &&
	       psa_mac_verify(PIN_KEY_ID, PIN_ALG, (const uint8_t *)pin, length, mac, sizeof(mac)) == PSA_SUCCESS;
}

static int check_pin(void *ctx, const char *pin, size_t length)
{
	const nu54_pin_attempt_storage storage = {ctx, read_pin_attempts, write_pin_attempts, verify_local_pin};
	return nu54_pin_attempt(&storage, pin, length);
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

static int clear_rental_material(void)
{
	int ok = device_key_retire_new_rental() == 0;
	ok &= gone(psa_destroy_key(PIN_KEY_ID));
	ok &= gone(psa_its_remove(UID_PIN));
	ok &= gone(psa_its_remove(UID_PIN_FAILURES));
	ok &= settings_delete("nu54/nonce") == 0;
	nonce_loaded = 0;
	return ok ? 0 : -EIO;
}

static int discard_setup(void *ctx)
{
	(void)ctx;
	struct rental_record record;
	int kind = read_rental_record(&record);
	/* The one commit could have succeeded before a lost response. Never discard its key. */
	if (kind != RENTAL_ABSENT && kind != RENTAL_RETIRED) return -EIO;
	return clear_rental_material();
}

static int wipe(void *ctx)
{
	(void)ctx;
	struct rental_record prior;
	int kind = read_rental_record(&prior);
	if (kind < 0) return kind;
	if (kind == RENTAL_JOURNAL_REQUIRED) return -EACCES; /* retain protected evidence */
	device_key_deactivate_rental();

	struct rental_record tombstone = {.kind = RENTAL_FENCED};
	if (kind != RENTAL_FENCED && its_write(UID_RECORD, &tombstone, RENTAL_V1_RECORD_SIZE) != 0) return -EIO;
	if (clear_rental_material() != 0 || ble_links_unpair_all() != 0) return -EIO;
	tombstone.kind = RENTAL_RETIRED;
	return its_write(UID_RECORD, &tombstone, RENTAL_V1_RECORD_SIZE);
}

/* ---------------------------------------------------------------- boot */

static void apply_rental_setup(nu54_device_t *d, const struct stored_record *r)
{
	memcpy(d->operator_address, r->operator_address, 20);
	memcpy(d->contract, r->contract, 20);
	memcpy(d->chain_id, r->chain_id, 32);
	d->passkey = r->passkey;
}

static void apply_rental_pin_state(nu54_device_t *d)
{
	uint8_t failures;
	if (read_pin_attempts(NULL, &failures) != 0 || failures >= NU54_PIN_ATTEMPT_LIMIT)
		d->state = NU54_STATE_PIN_LOCKED; /* only authorized retirement/reset clears it */
	LOG_INF("rental setup loaded%s", d->state == NU54_STATE_PIN_LOCKED ? " (PIN locked)" : "");
}

static int load_rental(nu54_device_t *d, const struct stored_record *r, device_key_slot slot)
{
	if (!nonce_loaded || device_key_activate_rental(slot, false, d->address) != 0)
		return -ENOENT;
	apply_rental_setup(d, r);
	nu54_device_init(d, stored_nonce);
	apply_rental_pin_state(d);
	return 0;
}

#if defined(CONFIG_NU54_EXPERIMENTAL_RENTAL_JOURNAL_PS)
static nu54_rental_journal_t rental_journal;

static int load_journaled_rental(nu54_device_t *d, const struct rental_record *record)
{
	nu54_rental_binding_t binding;
	uint8_t first[32];
	bool active;
	int found = device_rental_journal_marker_ps(&binding, first, &active);
	if (found == 1 && device_rental_journal_stage_ps(&record->binding, record->first) == 0)
		found = device_rental_journal_marker_ps(&binding, first, &active);
	if (found != 0 || memcmp(&record->binding, &binding, sizeof(binding)) ||
	    memcmp(record->first, first, sizeof(first)) ||
	    device_key_slot_open(DEVICE_KEY_RENTER, false, d->address) != 0 ||
	    memcmp(d->address, binding.account, sizeof(d->address)) ||
	    memcmp(record->fields.chain_id, binding.chain, sizeof(record->fields.chain_id)) ||
	    memcmp(record->fields.contract, binding.contract, sizeof(record->fields.contract))) return -EIO;

	apply_rental_setup(d, &record->fields);
	nu54_device_init_rental_candidate(d, first);
	if (active) {
		int result = device_rental_journal_resume_ps(&rental_journal, &binding, first);
		/* A protected fence leaves the rental candidate signing-disabled without stopping the owner link. */
		if (result != NU54_RENTAL_JOURNAL_FROZEN &&
		    (result != 0 || nu54_device_bind_rental_journal(d, &rental_journal) != 0 ||
		     device_key_activate_rental(DEVICE_KEY_RENTER, false, d->address) != 0)) return -EIO;
	}
	apply_rental_pin_state(d);
	return 0;
}

int device_setup_activate_rental_generation(nu54_device_t *d, const nu54_rental_registration_t *registration,
					    const nu54_rental_registration_trust_t *trust, const uint8_t signature[65])
{
	struct rental_record record;
	uint8_t account[20];
	if (!d || !registration || !trust || !signature || !d->rental_journal_required ||
	    !d->session_open || !d->session_setup || !d->session_bonded || d->pending != NU54_PENDING_NONE ||
	    (d->state != NU54_STATE_PROVISIONED_NO_ANCHOR && d->state != NU54_STATE_READY) ||
	    read_rental_record(&record) != RENTAL_JOURNAL_REQUIRED ||
	    memcmp(&record.binding, &trust->binding, sizeof(record.binding)) ||
	    memcmp(record.first, trust->first, sizeof(record.first)) ||
	    memcmp(record.fields.operator_address, trust->operator_address, sizeof(record.fields.operator_address)) ||
	    device_key_slot_open(DEVICE_KEY_RENTER, false, account) != 0 ||
	    memcmp(account, trust->binding.account, sizeof(account))) return -EACCES;
	if (d->rental_journal) {
		nu54_rental_binding_t observed;
		uint8_t first[32];
		bool active;
		return device_rental_journal_marker_ps(&observed, first, &active) == 0 && active &&
		       !memcmp(&observed, &record.binding, sizeof(observed)) &&
		       !memcmp(first, record.first, sizeof(first)) &&
		       nu54_rental_registration_verify(registration, trust, signature) == 0 ? 0 : -EACCES;
	}
	if (device_rental_journal_activate_ps(&rental_journal, registration, trust, signature) != 0 ||
	    nu54_device_bind_rental_journal(d, &rental_journal) != 0) return -EACCES;
	if (device_key_activate_rental(DEVICE_KEY_RENTER, false, account) != 0) {
		d->rental_journal = NULL;
		return -EIO;
	}
	return 0;
}

#endif

static int load_fixed(nu54_device_t *d)
{
	int err = device_key_activate_rental(DEVICE_KEY_LEGACY_RENTER, true, d->address);

	if (err) {
		return err;
	}
	LOG_INF("device key available (week-7 fixed setup)");
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
	struct rental_record r;
	int err;

	d->cfg.platform = (nu54_platform_t){
		.sign = sign,
#if defined(CONFIG_NU54_EXPERIMENTAL_RENTAL_JOURNAL_PS)
		.monotonic_seconds = rental_monotonic_seconds,
		.sign_rental_attestation = sign_rental_attestation,
		.can_commission_rental_journal = can_commission_rental_journal,
		.allow_rental_collector = allow_rental_collector,
		.freeze_rental = freeze_rental,
		.allow_rental_cleanup_station = device_rental_cleanup_station_allowed,
		.begin_rental_cleanup = begin_rental_cleanup,
		.end_rental_cleanup = device_rental_cleanup_end,
		.rental_cleanup_time = device_rental_cleanup_time,
		.rental_cleanup_binding = rental_cleanup_binding,
		.rental_cleanup_witness = device_rental_cleanup_witness,
		.fence_rental_cleanup = fence_rental_cleanup,
		.rental_challenge_random = rental_challenge_random,
#endif
		.random = random_bytes,
		.persist_nonce = persist_nonce,
		.generate_key = generate_key,
		.commit_setup = commit_setup,
		.wipe = wipe,
		.discard_setup = discard_setup,
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
	if (device_vault_load() != 0) {
		/* A personal-vault fault cannot grant or replace any keys. Preserve access to a proven v1 rental. */
		LOG_ERR("owner vault unavailable");
	}
	device_key_deactivate_rental();
	int kind = read_rental_record(&r);
	if (kind < 0) return kind;
#if defined(CONFIG_NU54_EXPERIMENTAL_RENTAL_JOURNAL_PS)
	if (kind == RENTAL_JOURNAL_REQUIRED) return load_journaled_rental(d, &r);
	if (device_rental_journal_platform_qualified()) {
		nu54_rental_binding_t binding;
		uint8_t first[32];
		bool active;
		/* Interrupted commissioning cannot become a legacy rental or discard its staged key. */
		if (device_rental_journal_marker_ps(&binding, first, &active) != 1) return -EIO;
	}
#else
	if (kind == RENTAL_JOURNAL_REQUIRED) return -EACCES;
#endif
	if (kind == RENTAL_LEGACY || kind == RENTAL_ACTIVE)
		return load_rental(d, &r.fields, kind == RENTAL_LEGACY ? DEVICE_KEY_LEGACY_RENTER : DEVICE_KEY_RENTER);
	if (kind == RENTAL_ABSENT && IS_ENABLED(CONFIG_NU54_DEV_SETUP)) return load_fixed(d);
	err = kind == RENTAL_FENCED ? wipe(NULL) : discard_setup(NULL);
	if (err) return err;
	nu54_device_init_unprovisioned(d);
	LOG_INF("unprovisioned: waiting for a setup session");
	return 0;
}
