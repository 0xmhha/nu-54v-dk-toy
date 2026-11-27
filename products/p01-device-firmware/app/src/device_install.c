#include "device_install.h"
#if defined(CONFIG_NU54_SPENDING_PROFILE)
#include "nu54_order.h"
#endif
#include "nu54_sig.h"
#include "device_vault.h"

#include <psa/crypto.h>
#include <psa/internal_trusted_storage.h>
#include <string.h>
#include <zephyr/kernel.h>
#include <zephyr/sys/util.h>
#include <zephyr/sys/byteorder.h>
#include <zephyr/storage/flash_map.h>
#include <zephyr/dfu/mcuboot.h>

#define INSTALL_ID ((psa_key_id_t)0x54b0)
#define INSTALL_PAYER ((psa_key_id_t)0x54b1)
#define INSTALL_FLOOR ((psa_storage_uid_t)0x5453)
#define INSTALL_REPLAY_FLOOR ((psa_storage_uid_t)0x5454)
#define INSTALL_STORAGE_MARKER ((psa_storage_uid_t)0x5455)
#define INSTALL_KEYS_WITNESS ((psa_storage_uid_t)0x5456)
#define INSTALL_POLICY_WITNESS ((psa_storage_uid_t)0x5457)
/* The grant itself, kept through a restart (spending profile): its length, the time the board
 * saves, and the grant in parts of the size one storage item takes in every profile. */
#define INSTALL_GRANT ((psa_storage_uid_t)0x5458)
#define INSTALL_CLOCK ((psa_storage_uid_t)0x5459)
#define INSTALL_GRANT_PART ((psa_storage_uid_t)0x5460)
#define GRANT_PART_SIZE 128
#define GRANT_PARTS ((NU54_INSTALL_BUNDLE_MAX + GRANT_PART_SIZE - 1) / GRANT_PART_SIZE)
#define CLOCK_SAVE_SECONDS 600
_Static_assert(INSTALL_GRANT_PART + GRANT_PARTS <= 0x5470, "the grant parts run into the next record");
#define ID_ALG PSA_ALG_ECDSA(PSA_ALG_SHA_256)
#define PAY_ALG PSA_ALG_DETERMINISTIC_ECDSA(PSA_ALG_SHA_256)
#define PUBLIC_TYPE PSA_KEY_TYPE_ECC_PUBLIC_KEY(PSA_ECC_FAMILY_SECP_R1)
#define OFFER_TTL_MS 60000
#if defined(CONFIG_NU54_SPENDING_PROFILE)
_Static_assert(OFFER_TTL_MS / 1000 < NU54_PILOT_BRIEF_SECONDS, "a slow grant outlasts a short request");
#endif

_Static_assert(sizeof(nu54_install_floor) <= 128, "P01 enrollment floor exceeds ITS limit");

static const char pin[] = CONFIG_NU54_INSTALL_AUTHORITY_HEX;
static const uint8_t p256_spki_header[] = {
	0x30, 0x59, 0x30, 0x13, 0x06, 0x07, 0x2a, 0x86, 0x48, 0xce, 0x3d, 0x02, 0x01,
	0x06, 0x08, 0x2a, 0x86, 0x48, 0xce, 0x3d, 0x03, 0x01, 0x07, 0x03, 0x42, 0x00,
};
static uint8_t challenge[32], identity[65], payer[20], offer[NU54_INSTALL_OFFER_SIZE];
static psa_key_id_t authority;
static nu54_install_policy policy;
static nu54_install_floor floor;
static int64_t challenge_birth, installed;
static uint64_t max_offset, replay_floor;
static bool offered, active;
/* The trusted lower bound of the time at `installed`: the grant's own time, or after a restart
 * the time the board had saved. */
static uint64_t clock_base;
static nu54_install_clock clock_saved; /* what storage holds; zero when nothing */
static bool clock_sealed;              /* the end of the grant is in storage: nothing more to save */

static int hash(const uint8_t *data, size_t length, uint8_t output[32])
{
	size_t used;
	return psa_hash_compute(PSA_ALG_SHA_256, data, length, output, 32, &used) == PSA_SUCCESS && used == 32 ? 0 : -1;
}

static int generate(psa_key_id_t id, psa_ecc_family_t family, psa_algorithm_t algorithm, uint8_t public_key[65])
{
	psa_key_attributes_t attrs = PSA_KEY_ATTRIBUTES_INIT;
	psa_key_type_t type = PSA_KEY_TYPE_ECC_KEY_PAIR(family);
	psa_status_t status = psa_get_key_attributes(id, &attrs);
	if (status == PSA_ERROR_DOES_NOT_EXIST || status == PSA_ERROR_INVALID_HANDLE) {
		psa_reset_key_attributes(&attrs);
		psa_set_key_lifetime(&attrs, PSA_KEY_LIFETIME_PERSISTENT);
		psa_set_key_id(&attrs, id);
		psa_set_key_type(&attrs, type);
		psa_set_key_bits(&attrs, 256);
		psa_set_key_usage_flags(&attrs, PSA_KEY_USAGE_SIGN_HASH);
		psa_set_key_algorithm(&attrs, algorithm);
		psa_key_id_t created;
		status = psa_generate_key(&attrs, &created);
	} else if (status == PSA_SUCCESS &&
		   (psa_get_key_lifetime(&attrs) != PSA_KEY_LIFETIME_PERSISTENT || psa_get_key_type(&attrs) != type ||
		    psa_get_key_bits(&attrs) != 256 || psa_get_key_algorithm(&attrs) != algorithm ||
		    (psa_get_key_usage_flags(&attrs) & (PSA_KEY_USAGE_EXPORT | PSA_KEY_USAGE_COPY)))) {
		status = PSA_ERROR_NOT_PERMITTED;
	}
	psa_reset_key_attributes(&attrs);
	size_t used;
	return status == PSA_SUCCESS && psa_export_public_key(id, public_key, 65, &used) == PSA_SUCCESS && used == 65
	       && public_key[0] == 4 ? 0 : -1;
}

static int public_fingerprint(const uint8_t key[65], uint8_t fingerprint[32])
{
	uint8_t spki[sizeof(p256_spki_header) + 65];
	memcpy(spki, p256_spki_header, sizeof(p256_spki_header));
	memcpy(spki + sizeof(p256_spki_header), key, 65);
	return hash(spki, sizeof(spki), fingerprint);
}

static bool separate(const uint8_t key[65], const uint8_t owner[32], const uint8_t terminal[32])
{
	uint8_t fingerprint[32], authority_point[65];
	const nu54_vault *vault = device_vault_owner();
	return hex2bin(pin, sizeof(pin) - 1, authority_point, sizeof(authority_point)) == sizeof(authority_point) &&
	       memcmp(key, authority_point, 65) && memcmp(key, identity, 65) &&
	       !public_fingerprint(key, fingerprint) && memcmp(fingerprint, owner, 32) &&
	       memcmp(fingerprint, terminal, 32) && memcmp(fingerprint, vault->record.owner, 32);
}

int device_install_independent_finality_key(const uint8_t key[65])
{
 if(!key || key[0]!=4 || !separate(key,policy.owner,policy.terminal) ||
    !memcmp(key,policy.display,65) || !memcmp(key,floor.recovery,65))return 0;
 return 1;
}

bool device_install_independent_peer(const uint8_t peer[32])
{
	uint8_t display[32], recovery[32];
	return !public_fingerprint(policy.display, display) && !public_fingerprint(floor.recovery, recovery) &&
	       memcmp(peer, display, 32) && memcmp(peer, recovery, 32);
}

static int import_public(const uint8_t key[65], psa_key_id_t *id)
{
	psa_key_attributes_t attrs = PSA_KEY_ATTRIBUTES_INIT;
	psa_set_key_type(&attrs, PUBLIC_TYPE);
	psa_set_key_bits(&attrs, 256);
	psa_set_key_usage_flags(&attrs, PSA_KEY_USAGE_VERIFY_HASH);
	psa_set_key_algorithm(&attrs, ID_ALG);
	return psa_import_key(&attrs, key, 65, id) == PSA_SUCCESS ? 0 : -1;
}

static int verify(psa_key_id_t key, const uint8_t *data, size_t length, const uint8_t signature[64])
{
	uint8_t digest[32];
	return hash(data, length, digest) ||
	       psa_verify_hash(key, ID_ALG, digest, sizeof(digest), signature, 64) != PSA_SUCCESS ? -1 : 0;
}

int device_install_public_authority(uint8_t output[65])
{
	size_t length;
	return authority && psa_export_public_key(authority, output, 65, &length) == PSA_SUCCESS && length == 65 ? 0 : -1;
}

int device_install_verify_authority(const uint8_t *data, size_t length, const uint8_t signature[64])
{
	return verify(authority, data, length, signature);
}

bool device_install_required(void) { return sizeof(pin) > 1; }

static int storage_found(psa_storage_uid_t uid, void *data, size_t size, unsigned flag, unsigned *found)
{
	struct psa_storage_info_t info;
	psa_status_t status = psa_its_get_info(uid, &info);
	if (status == PSA_ERROR_DOES_NOT_EXIST) return 0;
	if (status != PSA_SUCCESS || info.size != size) return -1;
	size_t used;
	if (psa_its_get(uid, 0, size, data, &used) != PSA_SUCCESS || used != size) return -1;
	*found |= flag;
	return 0;
}

static int witness_found(psa_storage_uid_t uid, unsigned flag, unsigned *found)
{
	uint32_t value = 0;
	if (storage_found(uid, &value, sizeof(value), flag, found)) return -1;
	return (*found & flag) && value != (uint32_t)uid ? -1 : 0;
}

static int put_witness(psa_storage_uid_t uid)
{
	uint32_t value = (uint32_t)uid;
	return psa_its_set(uid, sizeof(value), &value, PSA_STORAGE_FLAG_WRITE_ONCE) == PSA_SUCCESS ? 0 : -1;
}

static int key_found(psa_key_id_t id, unsigned flag, unsigned *found)
{
	psa_key_attributes_t attributes = PSA_KEY_ATTRIBUTES_INIT;
	psa_status_t status = psa_get_key_attributes(id, &attributes);
	psa_reset_key_attributes(&attributes);
	if (status == PSA_ERROR_DOES_NOT_EXIST || status == PSA_ERROR_INVALID_HANDLE) return 0;
	if (status != PSA_SUCCESS) return -1;
	*found |= flag;
	return 0;
}

static bool policy_witnessed;

static bool removed(psa_status_t status)
{
	return status == PSA_SUCCESS || status == PSA_ERROR_DOES_NOT_EXIST;
}

/* The saved time is a floor like the others: a record that cannot be read is not absence. */
static int clock_load(void)
{
	unsigned found = 0;
	clock_saved = (nu54_install_clock){0};
	clock_sealed = false;
	return storage_found(INSTALL_CLOCK, &clock_saved, sizeof(clock_saved), 1, &found);
}

#if defined(CONFIG_NU54_SPENDING_PROFILE)
static int clock_store(const nu54_install_clock *now)
{
	if (psa_its_set(INSTALL_CLOCK, sizeof(*now), now, PSA_STORAGE_FLAG_NONE) != PSA_SUCCESS) return -1;
	clock_saved = *now;
	return 0;
}

/* The length first, then the parts. The floor is written after them and is what commits the
 * grant: until then the parts are a grant the board does not know. */
static int grant_store(const uint8_t *bundle, size_t length)
{
	const uint32_t size = (uint32_t)length;
	if (psa_its_set(INSTALL_GRANT, sizeof(size), &size, PSA_STORAGE_FLAG_NONE) != PSA_SUCCESS) return -1;
	for (size_t part = 0, done = 0; done < length; ++part, done += GRANT_PART_SIZE) {
		if (psa_its_set(INSTALL_GRANT_PART + part, MIN(GRANT_PART_SIZE, length - done), bundle + done,
				PSA_STORAGE_FLAG_NONE) != PSA_SUCCESS) return -1;
	}
	return 0;
}

/* 0 read, 1 nothing usable. A grant that cannot be read back gives nothing, so it counts as none. */
static int grant_read(uint8_t *bundle, size_t capacity, size_t *length)
{
	uint32_t size = 0;
	size_t used;
	if (psa_its_get(INSTALL_GRANT, 0, sizeof(size), &size, &used) != PSA_SUCCESS || used != sizeof(size) ||
	    !size || size > capacity || size > NU54_INSTALL_BUNDLE_MAX) return 1;
	for (size_t part = 0, done = 0; done < size; ++part, done += GRANT_PART_SIZE) {
		const size_t wanted = MIN(GRANT_PART_SIZE, size - done);
		if (psa_its_get(INSTALL_GRANT_PART + part, 0, wanted, bundle + done, &used) != PSA_SUCCESS ||
		    used != wanted) return 1;
	}
	*length = size;
	return 0;
}
#endif

int device_install_forget(void)
{
	active = false;
	offered = false;
	clock_sealed = false;
	/* The length goes first: without it the parts are no grant, wherever the power goes. */
	bool done = removed(psa_its_remove(INSTALL_GRANT));
	for (size_t part = 0; part < GRANT_PARTS; ++part) done &= removed(psa_its_remove(INSTALL_GRANT_PART + part));
	done &= removed(psa_its_remove(INSTALL_CLOCK));
	if (done) clock_saved = (nu54_install_clock){0};
	return done ? 0 : -1;
}

int device_install_init(void)
{
	uint8_t key[65], paying_public[65];
	if (sizeof(pin) != 131 || hex2bin(pin, sizeof(pin) - 1, key, sizeof(key)) != sizeof(key) ||
	    key[0] != 4 || import_public(key, &authority)) return -1;
	unsigned found = 0;
	if (witness_found(INSTALL_STORAGE_MARKER, NU54_INSTALL_FOUND_MARKER, &found) ||
	    witness_found(INSTALL_KEYS_WITNESS, NU54_INSTALL_FOUND_KEYS, &found) ||
	    witness_found(INSTALL_POLICY_WITNESS, NU54_INSTALL_FOUND_COMMIT, &found) ||
	    storage_found(INSTALL_FLOOR, &floor, sizeof(floor), NU54_INSTALL_FOUND_FLOOR, &found) ||
	    storage_found(INSTALL_REPLAY_FLOOR, &replay_floor, sizeof(replay_floor), NU54_INSTALL_FOUND_REPLAY, &found) ||
	    key_found(INSTALL_ID, NU54_INSTALL_FOUND_ID, &found) ||
	    key_found(INSTALL_PAYER, NU54_INSTALL_FOUND_PAYER, &found) ||
	    clock_load() ||
	    ((found & NU54_INSTALL_FOUND_FLOOR) && (!floor.revision || !floor.time || !floor.generation || floor.recovery[0] != 4)))
		return -1;
	enum nu54_install_boot_action action = nu54_install_boot(found);
	if (action == NU54_INSTALL_BOOT_DENY ||
	    (action == NU54_INSTALL_BOOT_CREATE && put_witness(INSTALL_STORAGE_MARKER))) return -1;
	if (action == NU54_INSTALL_BOOT_CREATE || action == NU54_INSTALL_BOOT_CREATE_REPLAY) {
		if (psa_its_set(INSTALL_REPLAY_FLOOR, sizeof(replay_floor), &replay_floor, PSA_STORAGE_FLAG_NONE) != PSA_SUCCESS)
			return -1;
	}
	if (generate(INSTALL_ID, PSA_ECC_FAMILY_SECP_R1, ID_ALG, identity) ||
	    generate(INSTALL_PAYER, PSA_ECC_FAMILY_SECP_K1, PAY_ALG, paying_public)) return -1;
	if (!(found & NU54_INSTALL_FOUND_KEYS) && put_witness(INSTALL_KEYS_WITNESS)) return -1;
	if (action == NU54_INSTALL_BOOT_FINISH && put_witness(INSTALL_POLICY_WITNESS)) return -1;
	policy_witnessed = (found & NU54_INSTALL_FOUND_COMMIT) || action == NU54_INSTALL_BOOT_FINISH;
	nu54_address_of_pubkey(paying_public, payer);
	return 0;
}

int device_install_bound_offer(const uint8_t exporter[32], uint8_t output[NU54_INSTALL_BOUND_OFFER_SIZE])
{
	if (!authority) return -1;
	if (!offered || k_uptime_get() - challenge_birth >= OFFER_TTL_MS) {
		if (psa_generate_random(challenge, sizeof(challenge)) != PSA_SUCCESS) return -1;
		memcpy(offer, "N54E", 4);
		memcpy(offer + 4, challenge, 32);
		memcpy(offer + 36, identity, 65);
		memcpy(offer + 101, payer, 20);
		uint8_t digest[32];
		size_t used;
		const nu54_vault *vault = device_vault_owner();
		if (!vault->enabled || hash(vault->record.identity, sizeof(vault->record.identity), offer + 121) ||
		    hash(offer, 153, digest) ||
		    psa_sign_hash(INSTALL_ID, ID_ALG, digest, sizeof(digest), offer + 153, 64, &used) != PSA_SUCCESS ||
		    used != 64) return -1;
		challenge_birth = k_uptime_get();
		offered = true;
	}
	uint8_t digest[32];
	size_t used;
	if (nu54_install_bootstrap_offer_digest(offer, exporter, hash, digest) ||
	    psa_sign_hash(INSTALL_ID, ID_ALG, digest, sizeof(digest), output + sizeof(offer),
			  NU54_INSTALL_SIGNATURE_SIZE, &used) != PSA_SUCCESS || used != NU54_INSTALL_SIGNATURE_SIZE) return -1;
	memcpy(output, offer, sizeof(offer));
	return 0;
}

int device_install_public_pins(uint8_t output[64])
{
	const nu54_vault *vault = device_vault_owner();
	return !authority || !vault || !vault->enabled || public_fingerprint(identity, output) ||
	       hash(vault->record.identity, sizeof(vault->record.identity), output + 32) ? -1 : 0;
}

bool device_install_distinct_station(const mbedtls_x509_crt *station)
{
	uint8_t authority_point[65];
	const mbedtls_x509_buf *spki = &station->pk_raw;
	if (spki->len != sizeof(p256_spki_header) + sizeof(identity) ||
	    memcmp(spki->p, p256_spki_header, sizeof(p256_spki_header)) ||
	    device_install_public_authority(authority_point)) return false;
	const uint8_t *point = spki->p + sizeof(p256_spki_header);
	return memcmp(point, authority_point, sizeof(authority_point)) && memcmp(point, identity, sizeof(identity));
}

uint64_t device_install_earliest(void)
{
	if (!active) return 0;
	return clock_base + (uint64_t)(k_uptime_get() - installed) / 1000u;
}

int device_install_fence_wallet_expiry(uint64_t expiry)
{
	if (!active || !expiry) return -1;
	if (expiry <= replay_floor) return 0;
	if (expiry > device_install_earliest()) return -1;
	/* Commit the trusted lower bound before acknowledging an expired operation. */
	if (psa_its_set(INSTALL_REPLAY_FLOOR, sizeof(expiry), &expiry, PSA_STORAGE_FLAG_NONE) != PSA_SUCCESS)
		return -1;
	replay_floor = expiry;
	return 0;
}

int device_install_fence_receipt_expiry(uint64_t expiry)
{
	if (expiry > UINT64_MAX - NU54_PILOT_RECLAIM_HOLD_SECONDS) return -1;
	return device_install_fence_wallet_expiry(expiry + NU54_PILOT_RECLAIM_HOLD_SECONDS);
}

uint64_t device_install_wallet_replay_floor(void)
{
	return replay_floor;
}


/* The certificates and keys of a grant whose signature and fields are already accepted: the TLS
 * configuration of both services. `latest` is the latest possible time now. */
static int adopt(const uint8_t *bundle, size_t ca_length, size_t board_length, const nu54_install_policy *decoded,
		 uint64_t latest, mbedtls_x509_crt *ca, mbedtls_x509_crt *board, mbedtls_pk_context *key,
		 device_tls_v2_profile profiles[2])
{
	const uint8_t *certs = bundle + NU54_INSTALL_HEADER_SIZE;
	int32_t tls_result = mbedtls_x509_crt_parse_der(ca, certs, ca_length);
	if (!tls_result) tls_result = mbedtls_x509_crt_parse_der(board, certs + ca_length, board_length);
	uint32_t verification_flags = 0;
	uint8_t authority_point[65];
	if (tls_result || hex2bin(pin, sizeof(pin) - 1, authority_point, sizeof(authority_point)) != sizeof(authority_point) ||
	    (ca->pk_raw.len == sizeof(p256_spki_header) + sizeof(authority_point) &&
	     !memcmp(ca->pk_raw.p, p256_spki_header, sizeof(p256_spki_header)) &&
	     !memcmp(ca->pk_raw.p + sizeof(p256_spki_header), authority_point, sizeof(authority_point))) || !device_tls_v2_certificate_current(ca, latest) ||
	    !device_tls_v2_certificate_current(board, latest) ||
	    !device_tls_v2_certificate_current(ca, decoded->not_after - 1) ||
	    !device_tls_v2_certificate_current(board, decoded->not_after - 1) ||
	    mbedtls_x509_crt_verify(board, ca, NULL, NULL, &verification_flags, NULL, NULL) ||
	    board->pk_raw.len != sizeof(p256_spki_header) + sizeof(identity) ||
	    memcmp(board->pk_raw.p, p256_spki_header, sizeof(p256_spki_header)) ||
	    memcmp(board->pk_raw.p + sizeof(p256_spki_header), identity, sizeof(identity)) ||
	    mbedtls_pk_wrap_psa(key, INSTALL_ID) ||
	    device_tls_v2_configure(&profiles[0], NU54_V2_MANAGEMENT, ca, board, key) ||
	    device_tls_v2_configure(&profiles[1], NU54_V2_PAYMENT, ca, board, key)) return -1;
	psa_key_id_t display_key;
	if (import_public(decoded->display, &display_key)) return -1;
	if (psa_destroy_key(display_key) != PSA_SUCCESS) return -1;
	if (import_public(decoded->recovery, &display_key)) return -1;
	if (psa_destroy_key(display_key) != PSA_SUCCESS ||
	    !separate(decoded->display, decoded->owner, decoded->terminal) ||
	    !separate(decoded->recovery, decoded->owner, decoded->terminal)) return -1;
	return 0;
}

int device_install_load(const uint8_t *bundle, size_t length, mbedtls_x509_crt *ca,
			mbedtls_x509_crt *board, mbedtls_pk_context *key, device_tls_v2_profile profiles[2])
{
	const int64_t age = k_uptime_get() - challenge_birth;
	nu54_install_floor next;
	nu54_install_policy decoded;
	size_t ca_length, board_length;
	/* The time never goes back: not behind the running clock, a fenced expiry or the saved time. */
	const uint64_t minimum = nu54_install_resume(&clock_saved, device_install_earliest(), replay_floor);
	if (!offered || age < 0 || age >= OFFER_TTL_MS ||
	    nu54_install_accept(bundle, length, challenge, identity, payer, minimum,
				&floor, hash, device_install_verify_authority, &next, &decoded, &ca_length, &board_length))
		return -1;
	uint64_t offset = ((uint64_t)age + 999u) / 1000u;
	uint64_t previous_upper = device_install_now();
	if (previous_upper > next.time && previous_upper - next.time > offset)
		offset = previous_upper - next.time;
	if (decoded.not_after - next.time <= offset ||
	    adopt(bundle, ca_length, board_length, &decoded, next.time + offset, ca, board, key, profiles)) return -1;
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	/* Kept for the next start. A failure here commits nothing; the grant in use stays in use. */
	const nu54_install_clock start = {next.time, offset};
	if (grant_store(bundle, length) || clock_store(&start)) return -1;
#endif
	if (psa_its_set(INSTALL_FLOOR, sizeof(next), &next, PSA_STORAGE_FLAG_NONE) != PSA_SUCCESS ||
	    (!policy_witnessed && put_witness(INSTALL_POLICY_WITNESS))) {
		/* A write can fail after committing. Stop until reboot re-reads the durable floor. */
		active = false;
		offered = false;
		psa_destroy_key(authority);
		authority = 0;
		return -1;
	}
	policy_witnessed = true;
	floor = next;
	policy = decoded;
	clock_base = next.time;
	installed = k_uptime_get();
	max_offset = offset;
	clock_sealed = false;
	offered = false;
	active = true;
	return 0;
}

#if defined(CONFIG_NU54_SPENDING_PROFILE)
static bool no_owner(const nu54_vault *vault)
{
	return !vault->record.generation || (vault->record.flags & NU54_VAULT_REVOKED);
}

int device_install_restore(uint8_t *bundle, size_t capacity, mbedtls_x509_crt *ca, mbedtls_x509_crt *board,
			   mbedtls_pk_context *key, device_tls_v2_profile profiles[2])
{
	const nu54_vault *vault = device_vault_owner();
	nu54_install_policy decoded;
	size_t length, ca_length, board_length;
	/* Without the saved time the board does not know how far the grant has run. */
	if (active || !authority || !vault->enabled || !clock_saved.earliest || grant_read(bundle, capacity, &length) ||
	    nu54_install_restore(bundle, length, identity, payer, &floor, hash, device_install_verify_authority,
				 &decoded, &ca_length, &board_length) ||
	    !nu54_install_generation_current(decoded.payer_generation, vault->record.generation, no_owner(vault)))
		return 1;
	const uint64_t base = nu54_install_resume(&clock_saved, floor.time, replay_floor), offset = clock_saved.offset;
	/* Ended by the board's own clock, which stood still without power and never ran ahead. */
	if (base >= decoded.not_after || decoded.not_after - base <= offset ||
	    adopt(bundle, ca_length, board_length, &decoded, base + offset, ca, board, key, profiles)) return 1;
	policy = decoded;
	clock_base = base;
	installed = k_uptime_get();
	max_offset = offset;
	clock_sealed = false;
	active = true;
	return 0;
}

int64_t device_install_keep_time(void)
{
	if (!active || clock_sealed) return -1;
	const nu54_install_clock now = {device_install_earliest(), max_offset};
	/* A failed write is tried again at the next turn; the time in storage is then older, not wrong. */
	if (now.earliest > clock_saved.earliest && !clock_store(&now) && now.earliest + now.offset >= policy.not_after) {
		clock_sealed = true;
		return -1;
	}
	return nu54_install_clock_wait_ms(now.earliest + now.offset, policy.not_after, CLOCK_SAVE_SECONDS);
}
#endif

uint64_t device_install_now(void)
{
	if (!active) return 0;
	return active ? device_install_earliest() + max_offset : 0;
}

const nu54_install_policy *device_install_policy(void)
{
	return active && device_install_now() < policy.not_after ? &policy : NULL;
}

const uint8_t *device_install_payer(void) { return payer; }

#if defined(CONFIG_BOOTLOADER_MCUBOOT)
static int read_running_image(const void *context, uint32_t offset, uint8_t *out, size_t size)
{
	return flash_area_read(context, offset, out, size);
}
#endif

static int measure_image(uint8_t slot, uint8_t digest[32], nu54_running_firmware *version)
{
#if defined(CONFIG_BOOTLOADER_MCUBOOT)
	const struct flash_area *area;
	if (flash_area_open(slot, &area)) return -1;
	uint8_t bytes[512], header[32];
	psa_hash_operation_t operation = PSA_HASH_OPERATION_INIT;
	size_t output_size;
	int rc = -1;
	uint64_t length;
	if (version) {
		if ((uint32_t)area->fa_size != area->fa_size ||
		    nu54_running_firmware_inspect(read_running_image, area, (uint32_t)area->fa_size, version))
			goto done;
		length = version->measured_size;
	} else {
		if (flash_area_read(area, 0, header, sizeof(header)) || sys_get_le32(header) != 0x96f3b83d)
			goto done;
		const uint16_t header_size = sys_get_le16(header + 8), protected_tlv = sys_get_le16(header + 10);
		const uint32_t image_size = sys_get_le32(header + 12);
		length = (uint64_t)header_size + image_size + protected_tlv;
		if (header_size < sizeof(header) || !image_size || length > area->fa_size) goto done;
	}
	if (psa_hash_setup(&operation, PSA_ALG_SHA_256) != PSA_SUCCESS) goto done;
	for (size_t offset = 0; offset < length;) {
		size_t count = MIN(sizeof(bytes), length - offset);
		if (flash_area_read(area, offset, bytes, count) ||
		    psa_hash_update(&operation, bytes, count) != PSA_SUCCESS) goto done;
		offset += count;
	}
	if (psa_hash_finish(&operation, digest, 32, &output_size) == PSA_SUCCESS && output_size == 32)
		rc = 0;
done:
	psa_hash_abort(&operation);
	flash_area_close(area);
	return rc;
#else
	(void)slot;
	(void)digest;
	(void)version;
	return -1;
#endif
}

static int measure_slot(uint8_t digest[32])
{
#if defined(CONFIG_BOOTLOADER_MCUBOOT)
	return measure_image(FIXED_PARTITION_ID(slot0_partition), digest, NULL);
#else
	return measure_image(0, digest, NULL);
#endif
}

int device_install_running_firmware(uint64_t generation, uint8_t output[NU54_RUNNING_FIRMWARE_REPLY_SIZE])
{
#if defined(CONFIG_BOOTLOADER_MCUBOOT) && defined(CONFIG_NU54_COUNTED_UPDATE)
	const nu54_vault *vault = device_vault_owner();
	if (!device_install_policy() || !vault->enabled || !generation ||
	    vault->record.generation != generation || (vault->record.flags & NU54_VAULT_REVOKED) ||
	    !boot_is_img_confirmed()) return -1;
	uint8_t slot = boot_fetch_active_slot(), board[32], measurement[32];
	nu54_running_firmware version;
	if (measure_image(slot, measurement, &version) || boot_fetch_active_slot() != slot ||
	    !boot_is_img_confirmed() || !device_install_policy() || !vault->enabled ||
	    vault->record.generation != generation ||
	    (vault->record.flags & NU54_VAULT_REVOKED) ||
	    hash(vault->record.identity, sizeof(vault->record.identity), board)) return -1;
	return nu54_running_firmware_reply(&version, generation, board, measurement, output);
#else
	(void)generation;
	(void)output;
	return -1;
#endif
}


#if defined(CONFIG_NU54_EXPERIMENTAL_RENTAL_JOURNAL_PS)
#include "device_key.h"
#include "device_rental_journal_ps.h"

int device_install_rental_ack(const uint8_t request[NU54_RENTAL_INSTALL_WIRE_SIZE],
			      uint8_t output[NU54_RENTAL_INSTALL_WIRE_SIZE])
{
#if defined(CONFIG_BOOTLOADER_MCUBOOT) && defined(CONFIG_NU54_COUNTED_UPDATE)
	static const uint8_t empty[32];
	const nu54_vault *vault = device_vault_owner();
	nu54_rental_binding_t binding;
	bool active;
	uint8_t first[32], installed_board[32], freeze_signer[20], measurement[32], digest[32];
	size_t used;
	uint64_t now = device_install_now();
	uint64_t expiry = sys_get_be64(request + 367);
	_Static_assert(sizeof(binding) == 208, "rental installation scope layout");
	if (!now || expiry <= now || expiry - now > 120 ||
	    memcmp(request, "N54G", 4) || !memcmp(request + 4, empty, sizeof(empty)) ||
	    !memcmp(request + 296, empty, sizeof(empty)) ||
	    device_install_verify_authority(request, NU54_RENTAL_INSTALL_BODY_SIZE,
					   request + NU54_RENTAL_INSTALL_BODY_SIZE) ||
	    !device_install_policy() || !vault->enabled || !vault->record.generation ||
	    (vault->record.flags & NU54_VAULT_REVOKED) || !boot_is_img_confirmed() ||
	    hash(vault->record.identity, sizeof(vault->record.identity), installed_board) ||
	    memcmp(request + 36, installed_board, sizeof(installed_board)) ||
	    device_rental_journal_marker_ps(&binding, first, &active) != 0 || !active ||
	    memcmp(request + 68, &binding, sizeof(binding)) ||
	    memcmp(request + 335, first, sizeof(first)) ||
	    measure_slot(measurement) || memcmp(measurement, binding.measurement, sizeof(measurement)) ||
	    device_key_slot_open(DEVICE_KEY_IDENTITY, false, freeze_signer) != 0 ||
	    memcmp(request + 276, freeze_signer, sizeof(freeze_signer)) ||
	    !memcmp(freeze_signer, binding.account, sizeof(freeze_signer)) ||
	    !device_rental_collector_peer_allowed(&binding, request + 328) ||
	    hash(request, NU54_RENTAL_INSTALL_BODY_SIZE, digest)) return -1;
	memcpy(output, request, NU54_RENTAL_INSTALL_BODY_SIZE);
	return psa_sign_hash(INSTALL_ID, ID_ALG, digest, sizeof(digest),
			     output + NU54_RENTAL_INSTALL_BODY_SIZE, 64, &used) == PSA_SUCCESS && used == 64 ? 0 : -1;
#else
	(void)request;
	(void)output;
	return -1;
#endif
}
#endif

static int sign_installation_review(const uint8_t digest[32], uint8_t signature[64])
{
	size_t used;
	return psa_sign_hash(INSTALL_ID, ID_ALG, digest, 32, signature, 64, &used) == PSA_SUCCESS &&
	       used == 64 ? 0 : -1;
}

int device_install_rental_review(const nu54_local_approval *approval, const nu54_v2_link *link,
    const nu54_v2_request *request, const uint8_t challenge[32], uint8_t output[NU54_V2_RENTAL_REVIEW_RECEIPT_SIZE])
{
	const nu54_install_policy *current = device_install_policy();
	uint8_t board_pin[32];
	if (!current || !current->allowlist.commercial_testnet || current->payer_role != NU54_V2_RENTER_ROLE ||
	    approval->intent.generation != current->payer_generation ||
	    memcmp(link->peer_identity, current->terminal, sizeof(current->terminal)) ||
	    request->payload_length != NU54_V2_SIGNED_SIZE ||
	    memcmp(payer, request->payload + NU54_V2_PAYER_OFFSET + 12, sizeof(payer)) ||
	    public_fingerprint(identity, board_pin) ||
	    nu54_pilot_rental_review(approval, link, request, challenge, board_pin, device_install_now(),
				    hash, sign_installation_review, output)) return -1;
	uint64_t now = device_install_now();
	if (!now || now >= request->expires || device_install_policy() != current) {
		memset(output, 0, NU54_V2_RENTAL_REVIEW_RECEIPT_SIZE);
		return -1;
	}
	return 0;
}

int device_install_rental_decision(const nu54_v2_link *link, uint64_t generation, const uint8_t review_hash[32],
    const uint8_t *payer_signature, uint8_t output[NU54_W10_DECISION_SIZE])
{
	const nu54_install_policy *current = device_install_policy();
	uint64_t now = device_install_now();
	if (!current || !current->allowlist.commercial_testnet || current->payer_role != NU54_V2_RENTER_ROLE ||
	    current->payer_generation != generation || link->profile != NU54_V2_PAYMENT ||
	    link->local != NU54_V2_BOARD || link->peer != NU54_V2_TERMINAL || link->closed ||
	    !link->pending_rental_review || !link->pending_fresh || !link->pending_reported ||
	    memcmp(link->peer_identity, current->terminal, sizeof(current->terminal)) ||
	    !now || now >= link->pending_expiry ||
	    nu54_pilot_rental_decision(review_hash, payer_signature, hash, sign_installation_review, output)) return -1;
	now = device_install_now();
	if (!now || now >= link->pending_expiry || device_install_policy() != current) {
		memset(output, 0, NU54_W10_DECISION_SIZE);
		return -1;
	}
	return 0;
}

int device_install_update_ack(const uint8_t request[48], uint8_t output[180])
{
#if defined(CONFIG_BOOTLOADER_MCUBOOT) && defined(CONFIG_NU54_COUNTED_UPDATE)
    const nu54_vault *vault = device_vault_owner();
    uint8_t digest[32];
    size_t used;
    if (!device_install_policy() || !vault->enabled || !vault->record.generation ||
        !boot_is_img_confirmed()) return -1;
    memcpy(output, "N54F", 4);
    memcpy(output + 4, request, 48);
    if (hash(vault->record.identity, sizeof(vault->record.identity), output + 52) ||
        measure_slot(output + 84) || hash(output, 116, digest) ||
        psa_sign_hash(INSTALL_ID, ID_ALG, digest, sizeof(digest), output + 116, 64, &used) != PSA_SUCCESS ||
        used != 64) return -1;
    return 0;
#else
    (void)request; (void)output;
    return -1;
#endif
}

int device_install_check_hd(const uint8_t *grant, size_t length)
{
	const nu54_install_policy *current = device_install_policy();
	uint8_t measurement[32];
	if (!IS_ENABLED(CONFIG_NU54_COUNTED_UPDATE) || !IS_ENABLED(CONFIG_NRF_APPROTECT_LOCK) ||
	    !IS_ENABLED(CONFIG_NRF_SECURE_APPROTECT_LOCK) || !current || !offered ||
	    k_uptime_get() - challenge_birth >= OFFER_TTL_MS || length < 141 + 64 || memcmp(grant, "N54H", 4) ||
	    memcmp(grant + 4, challenge, 32) || memcmp(grant + 36, identity, 65) ||
	    sys_get_be64(grant + 101) != current->revision ||
	    device_install_verify_authority(grant, length - 64, grant + length - 64) ||
	    measure_slot(measurement) || memcmp(measurement, grant + 109, 32)) return -1;
	return 0;
}

void device_install_consume_offer(void) { offered = false; }

int device_install_sign(const uint8_t digest[32], uint8_t signature[64])
{
	size_t used;
	return device_install_policy() && psa_sign_hash(INSTALL_PAYER, PAY_ALG, digest, 32, signature, 64, &used) ==
	       PSA_SUCCESS && used == 64 ? 0 : -1;
}

#if defined(CONFIG_NU54_SPENDING_PROFILE)
int device_install_account_statement(const uint8_t *statement, size_t length, uint8_t signature[64])
{
	uint8_t digest[32];
	return length == NU54_ACCOUNT_STATEMENT_SIZE && device_install_policy() && !hash(statement, length, digest) ?
	       sign_installation_review(digest, signature) : -1;
}
#endif

static int verify_independent(const uint8_t public_key[65], const uint8_t *data, size_t length, const uint8_t signature[64])
{
	psa_key_id_t key;
	if (!device_install_policy() || import_public(public_key, &key)) return -1;
	int result = verify(key, data, length, signature);
	return psa_destroy_key(key) == PSA_SUCCESS ? result : -1;
}


int device_install_verify_display(const uint8_t *data, size_t length, const uint8_t signature[64])
{
	return verify_independent(policy.display, data, length, signature);
}

int device_install_verify_recovery(const uint8_t *data, size_t length, const uint8_t signature[64])
{
	return verify_independent(floor.recovery, data, length, signature);
}
