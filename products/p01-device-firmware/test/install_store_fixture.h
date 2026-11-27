/* A board that keeps the operator's grant through a restart, for host tests: the storage and
 * clock code extracted from device_install.c at build time (board_install_store.h), a fake flash
 * that can lose power at every write, the operator's grant and the passing of time. The
 * certificates of the grant are not looked at (adopt is a stand-in). */
#ifndef INSTALL_STORE_FIXTURE_H
#define INSTALL_STORE_FIXTURE_H

#include "nu54_install.h"
#include "nu54_vault.h"

#include <psa/crypto.h>
#include <psa/internal_trusted_storage.h>

#include <assert.h>
#include <setjmp.h>
#include <stdio.h>
#include <string.h>

#define MIN(a, b) ((a) < (b) ? (a) : (b))
#define ISSUED UINT64_C(1800000000)
#define DAY UINT64_C(86400)

/* ---- flash; a write either happened or did not when the power goes ---- */
struct item {uint64_t uid; size_t size; uint8_t data[160];};
static struct item flash[64];
static int writes_left = -1; /* -1: power stays on */
static bool power_after;     /* the write lands and then the power goes */
static jmp_buf power;
static unsigned writes;

static struct item *item(uint64_t uid)
{
	for (size_t i = 0; i < 64; ++i)
		if (flash[i].uid == uid) return &flash[i];
	return NULL;
}

static bool cut(void)
{
	++writes;
	if (writes_left < 0) return false;
	if (writes_left-- > 0) return false;
	writes_left = -1;
	return true;
}

#define WRITE(action) do { \
		bool dies = cut(); \
		if (dies && !power_after) longjmp(power, 1); \
		action; \
		if (dies) longjmp(power, 1); \
	} while (0)

psa_status_t psa_its_get_info(psa_storage_uid_t uid, struct psa_storage_info_t *info)
{
	struct item *found = item(uid);
	if (!found) return PSA_ERROR_DOES_NOT_EXIST;
	info->size = found->size;
	return PSA_SUCCESS;
}
psa_status_t psa_its_get(psa_storage_uid_t uid, size_t offset, size_t size, void *data, size_t *used)
{
	struct item *found = item(uid);
	assert(!offset);
	if (!found) return PSA_ERROR_DOES_NOT_EXIST;
	*used = size < found->size ? size : found->size;
	memcpy(data, found->data, *used);
	return PSA_SUCCESS;
}
psa_status_t psa_its_set(psa_storage_uid_t uid, size_t size, const void *data, uint32_t flags)
{
	assert(!flags && uid && size <= 128); /* the item size of the default profile */
	struct item *slot = item(uid);
	for (size_t i = 0; !slot && i < 64; ++i)
		if (!flash[i].uid) slot = &flash[i];
	assert(slot);
	WRITE(slot->uid = uid; slot->size = size; memcpy(slot->data, data, size));
	return PSA_SUCCESS;
}
psa_status_t psa_its_remove(psa_storage_uid_t uid)
{
	struct item *found = item(uid);
	if (!found) return PSA_ERROR_DOES_NOT_EXIST;
	WRITE(memset(found, 0, sizeof(*found)));
	return PSA_SUCCESS;
}
psa_status_t psa_destroy_key(psa_key_id_t id)
{
	(void)id;
	return PSA_SUCCESS;
}

/* ---- the rest of device_install.c ---- */
typedef struct {int unused;} mbedtls_x509_crt, mbedtls_pk_context, device_tls_v2_profile;
static uint8_t challenge[32] = {1}, identity[65] = {4, 5}, payer[20] = {6};
static psa_key_id_t authority = 1;
static nu54_install_policy policy;
static nu54_install_floor floor;
static int64_t challenge_birth, installed, uptime = 5000;
static uint64_t max_offset, replay_floor, clock_base, clock_moved;
static bool offered, active, clock_sealed, policy_witnessed;
static nu54_install_clock clock_saved;
static nu54_vault vault = {.enabled = true};
static uint64_t adopted_at;
static unsigned adoptions;

static int64_t k_uptime_get(void) { return uptime; }
static const nu54_vault *device_vault_owner(void) { return &vault; }
static int put_witness(psa_storage_uid_t uid) { (void)uid; return 0; }
#define INSTALL_POLICY_WITNESS 0x5457

/* Not a cryptographic hash: it only has to change with every byte. */
static int hash(const uint8_t *data, size_t length, uint8_t out[32])
{
	uint64_t state = UINT64_C(1469598103934665603);
	for (size_t i = 0; i < length; ++i) state = (state ^ data[i]) * UINT64_C(1099511628211);
	for (unsigned i = 0; i < 32; ++i) {
		state = (state ^ i) * UINT64_C(1099511628211);
		out[i] = (uint8_t)(state >> 24);
	}
	return 0;
}
static int device_install_verify_authority(const uint8_t *data, size_t length, const uint8_t signature[64])
{
	uint8_t expected[32];
	hash(data, length, expected);
	return memcmp(signature, expected, 32) || memcmp(signature + 32, expected, 32) ? -1 : 0;
}
static int adopt(const uint8_t *bundle, size_t ca_length, size_t board_length, const nu54_install_policy *decoded,
		 uint64_t latest, mbedtls_x509_crt *ca, mbedtls_x509_crt *board, mbedtls_pk_context *key,
		 device_tls_v2_profile profiles[2])
{
	(void)bundle; (void)ca; (void)board; (void)key; (void)profiles;
	assert(ca_length == 1 && board_length == 1 && latest < decoded->not_after);
	adopted_at = latest;
	++adoptions;
	return 0;
}

uint64_t device_install_now(void);
int device_install_forget(void);
int device_install_load(const uint8_t *, size_t, mbedtls_x509_crt *, mbedtls_x509_crt *, mbedtls_pk_context *,
			device_tls_v2_profile [2]);
int device_install_restore(uint8_t *, size_t, mbedtls_x509_crt *, mbedtls_x509_crt *, mbedtls_pk_context *,
			   device_tls_v2_profile [2]);
int64_t device_install_keep_time(void);
int device_install_set_time(const nu54_install_clock_told *, bool, uint64_t, uint8_t [NU54_CLOCK_SET_REPLY_SIZE]);
uint64_t device_install_order_hold(void);
int device_install_fence_wallet_expiry(uint64_t);
const nu54_install_policy *device_install_policy(void);
uint64_t device_install_earliest(void);
#include "board_install_store.h"

/* ---- the operator, the board's start and the passing of time ---- */
#define GRANT_SIZE (NU54_INSTALL_HEADER_SIZE + 2 + 64)
static void put64(uint8_t *out, uint64_t value)
{
	for (int i = 7; i >= 0; --i, value >>= 8) out[i] = (uint8_t)value;
}

static void make_grant(uint8_t record[GRANT_SIZE], uint64_t revision, uint64_t generation, uint64_t time, uint64_t end)
{
	memset(record, 0, GRANT_SIZE);
	memcpy(record, "N54Q", 4);
	memcpy(record + 4, challenge, 32);
	memcpy(record + 36, identity, 65);
	memcpy(record + 101, payer, 20);
	put64(record + 121, time);
	uint8_t *raw = record + 129;
	put64(raw, revision);
	put64(raw + 8, generation);
	put64(raw + 16, end);
	raw[24 + 7] = raw[32 + 7] = 1;
	raw[40 + 7] = 50;
	raw[48] = NU54_V2_OWNER_ROLE;
	memset(raw + 49, 1, NU54_INSTALL_POLICY_SIZE - 49);
	raw[49 + 80 + 64] = 2;
	raw[49 + 80 + 96] = 4;
	raw[49 + 80 + 96 + 65] = 4;
	raw[49 + 80 + 96 + 66] = 3;
	raw[NU54_INSTALL_POLICY_SIZE + 1] = raw[NU54_INSTALL_POLICY_SIZE + 3] = 1;
	hash(record, GRANT_SIZE - 64, record + GRANT_SIZE - 64);
	memcpy(record + GRANT_SIZE - 32, record + GRANT_SIZE - 64, 32);
}

/* The operator installs a grant `took` milliseconds after the board made the offer it answers. */
static int install_after(const uint8_t record[GRANT_SIZE], int64_t took)
{
	offered = true;
	challenge_birth = uptime - took;
	return device_install_load(record, GRANT_SIZE, NULL, NULL, NULL, NULL);
}

/* The usual grant: two seconds from the offer to the grant. */
static int install(const uint8_t record[GRANT_SIZE])
{
	return install_after(record, 2000);
}

/* A restart: memory is gone, the floors are read as device_install_init reads them, and the
 * stored grant is taken up again. Returns what device_install_restore returns. */
static int restart(void)
{
	uint8_t work[2 + NU54_INSTALL_BUNDLE_MAX];
	unsigned found = 0;
	memset(&policy, 0, sizeof(policy));
	memset(&floor, 0, sizeof(floor));
	installed = 0;
	max_offset = replay_floor = clock_base = clock_moved = 0;
	offered = active = false;
	uptime = 5000;
	assert(!storage_found(INSTALL_FLOOR, &floor, sizeof(floor), 1, &found));
	assert(!storage_found(INSTALL_REPLAY_FLOOR, &replay_floor, sizeof(replay_floor), 2, &found));
	assert(!clock_load());
	return device_install_restore(work, sizeof(work), NULL, NULL, NULL, NULL);
}

/* Runs the board for `seconds`, saving the time whenever the board asks for it. */
static void run(uint64_t seconds)
{
	const int64_t end = uptime + (int64_t)seconds * 1000;
	int64_t wait = device_install_keep_time();
	while (wait >= 0 && uptime + wait <= end) {
		uptime += wait;
		wait = device_install_keep_time();
	}
	uptime = end;
}

static void wipe(uint64_t generation, uint8_t flags)
{
	memset(flash, 0, sizeof(flash));
	vault = (nu54_vault){.enabled = true, .record.generation = generation, .record.flags = flags};
	writes_left = -1;
	(void)restart();
}

#endif
