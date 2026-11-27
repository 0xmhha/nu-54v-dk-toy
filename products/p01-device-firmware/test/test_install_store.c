/* The operator's grant kept through a restart. The storage and clock code is extracted from
 * device_install.c at build time and runs over a fake flash that loses power at every write. The
 * certificates of the grant are not looked at here (adopt is a stand-in). */
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
static uint64_t max_offset, replay_floor, clock_base;
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

/* The operator installs a grant issued `age` seconds ago by the calendar clock. */
static int install(const uint8_t record[GRANT_SIZE])
{
	offered = true;
	challenge_birth = uptime - 2000;
	return device_install_load(record, GRANT_SIZE, NULL, NULL, NULL, NULL);
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
	max_offset = replay_floor = clock_base = 0;
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

/* ---- cases ---- */
static void test_kept_through_a_restart(void)
{
	uint8_t record[GRANT_SIZE];
	wipe(3, 0);
	assert(restart() == 1 && !device_install_policy() && !device_install_earliest()); /* nothing stored */
	make_grant(record, 1, 3, ISSUED, ISSUED + 31 * DAY);
	assert(!install(record) && device_install_policy() && device_install_earliest() == ISSUED);
	const nu54_install_policy installed_policy = policy;
	assert(item(INSTALL_GRANT) && item(INSTALL_CLOCK) && item(INSTALL_FLOOR));
	for (size_t part = 0; part < (GRANT_SIZE + 127) / 128; ++part) assert(item(INSTALL_GRANT_PART + part));

	/* Power off right away, for however long: the board goes on from the grant's own time. */
	adoptions = 0;
	assert(restart() == 0 && adoptions == 1 && !memcmp(&policy, &installed_policy, sizeof(policy)));
	assert(device_install_policy() && device_install_earliest() == ISSUED && device_install_now() == ISSUED + 2);
	assert(adopted_at == ISSUED + 2);

	/* Ten days on. The time is saved every ten minutes; what passed since the last save is lost. */
	writes = 0;
	run(10 * DAY + 599);
	assert(writes == 10 * 144 && device_install_earliest() == ISSUED + 10 * DAY + 599);
	assert(restart() == 0 && device_install_earliest() == ISSUED + 10 * DAY);
	/* The time without power is not counted, and several restarts in a row neither move the
	 * clock back nor forward. */
	for (int i = 0; i < 5; ++i) {
		run(599);
		assert(restart() == 0 && device_install_earliest() == ISSUED + 10 * DAY);
	}
	run(600);
	assert(restart() == 0 && device_install_earliest() == ISSUED + 10 * DAY + 600);
}

static void test_the_end_is_kept(void)
{
	uint8_t record[GRANT_SIZE];
	wipe(3, 0);
	make_grant(record, 1, 3, ISSUED, ISSUED + 1000);
	assert(!install(record));
	run(990);
	assert(device_install_policy());
	run(10); /* the board saves the time at the end of the grant, not only every ten minutes */
	assert(!device_install_policy() && device_install_keep_time() == -1);
	const unsigned before = writes;
	run(5 * DAY);
	assert(writes == before); /* nothing more is written for a grant that ended */
	assert(restart() == 1 && !device_install_policy()); /* and a restart does not bring it back */

	/* A new grant must not start before the time the board had reached. */
	make_grant(record, 2, 3, ISSUED + 900, ISSUED + 5000);
	assert(install(record));
	make_grant(record, 2, 3, ISSUED + 1000, ISSUED + 5000);
	assert(!install(record) && device_install_earliest() == ISSUED + 1000);

	/* A restart shortly before the end loses at most the time since the last save. */
	run(3990); /* saved at 1600, 2200, ... 4600; now at 4990 */
	assert(device_install_policy() && restart() == 0 && device_install_earliest() == ISSUED + 4600);
	run(397); /* the latest possible time is two seconds above the lower bound */
	assert(device_install_policy());
	run(1);
	assert(!device_install_policy() && restart() == 1);
}

static void test_bound_to_the_generation(void)
{
	uint8_t record[GRANT_SIZE];
	/* Installed for the next owner of a board without one. */
	wipe(3, NU54_VAULT_REVOKED | NU54_VAULT_RELEASED);
	make_grant(record, 1, 4, ISSUED, ISSUED + 31 * DAY);
	assert(!install(record) && restart() == 0);
	/* The owner registered: generation 4, and the grant is the owner's. */
	vault.record.generation = 4;
	vault.record.flags = 0;
	assert(restart() == 0);
	/* A grant left in storage through a factory reset (generation 5, no owner) is ignored: the
	 * phone it names must not register again. The reset also removes it; this is the second bar. */
	vault.record.generation = 5;
	vault.record.flags = NU54_VAULT_REVOKED | NU54_VAULT_RELEASED;
	assert(restart() == 1 && !device_install_policy());
	/* Nor is it the grant of an owner with another generation, or of a vault that stopped. */
	vault.record.flags = 0;
	assert(restart() == 1);
	vault.record.generation = 4;
	vault.enabled = false;
	assert(restart() == 1);
	vault.enabled = true;
	assert(restart() == 0);
}

static void test_reset_removes_it(void)
{
	uint8_t record[GRANT_SIZE];
	for (int after = 0; after < 2; ++after) {
		for (int step = 0;; ++step) {
			wipe(3, 0);
			make_grant(record, 1, 3, ISSUED, ISSUED + 31 * DAY);
			assert(!install(record));
			run(3 * DAY);
			power_after = after;
			writes_left = step;
			volatile bool finished = false;
			if (!setjmp(power)) {
				assert(!device_install_forget());
				finished = true;
			}
			writes_left = -1;
			/* The reset left its mark, so the start repeats the removal before anything else. */
			assert(!clock_load() && !device_install_forget());
			assert(restart() == 1 && !device_install_policy() && !clock_saved.earliest);
			assert(!item(INSTALL_GRANT) && !item(INSTALL_CLOCK));
			for (size_t part = 0; part < GRANT_PARTS; ++part) assert(!item(INSTALL_GRANT_PART + part));
			assert(item(INSTALL_FLOOR) && floor.revision == 1); /* the floors stay through a reset */
			if (finished) break;
			assert(step < 20);
		}
	}
	/* The saved time is gone with the grant; the grant's own time stays as the floor. */
	make_grant(record, 1, 4, ISSUED, ISSUED + DAY);
	vault.record.generation = 3;
	vault.record.flags = NU54_VAULT_REVOKED | NU54_VAULT_RELEASED;
	assert(install(record)); /* the same revision with other content */
	make_grant(record, 2, 4, ISSUED - 1, ISSUED + DAY);
	assert(install(record));
	make_grant(record, 2, 4, ISSUED, ISSUED + DAY);
	assert(!install(record) && restart() == 0);
}

/* What storage may hold when the power goes while a grant replaces another: the new grant with
 * its floor, no usable grant, or the old grant untouched under its own floor. Never one grant
 * under the floor of the other, and never a clock that went back. */
static void test_power_loss_while_installing(void)
{
	uint8_t first[GRANT_SIZE], second[GRANT_SIZE];
	unsigned cuts = 0;
	make_grant(first, 1, 3, ISSUED, ISSUED + 31 * DAY);
	make_grant(second, 2, 3, ISSUED + 5 * DAY, ISSUED + 20 * DAY);
	for (int replacing = 0; replacing < 2; ++replacing) {
		for (int after = 0; after < 2; ++after) {
			for (int step = 0;; ++step) {
				wipe(3, 0);
				uint64_t reached = 0;
				if (replacing) {
					assert(!install(first));
					run(4 * DAY);
					reached = device_install_earliest();
				}
				power_after = after;
				writes_left = step;
				volatile bool finished = false;
				if (!setjmp(power)) {
					assert(!install(replacing ? second : first));
					finished = true;
				}
				writes_left = -1;
				const int restored = restart();
				const uint64_t revision = replacing ? 2 : 1;
				if (restored == 0 && policy.revision == revision) {
					assert(floor.revision == revision);
					assert(device_install_earliest() == (replacing ? ISSUED + 5 * DAY : ISSUED));
				} else if (restored == 0) {
					/* Nothing of the new grant reached storage yet: the one before goes on. */
					assert(replacing && !finished && floor.revision == 1 && policy.revision == 1);
					assert(device_install_earliest() == reached);
				} else {
					assert(!device_install_policy());
				}
				assert(!finished || restored == 0);
				/* The board's time floor never went back. */
				assert(nu54_install_resume(&clock_saved, floor.time, replay_floor) >= reached);
				++cuts;
				if (finished) break;
				assert(step < 20);
			}
		}
	}
	printf("installation cut at %u points\n", cuts);

	/* The same for the periodic save of the time: the old value or the new one. */
	for (int after = 0; after < 2; ++after) {
		wipe(3, 0);
		assert(!install(first));
		run(DAY);
		uptime += 600 * 1000;
		power_after = after;
		writes_left = 0;
		if (!setjmp(power)) (void)device_install_keep_time();
		writes_left = -1;
		assert(restart() == 0 && device_install_earliest() == ISSUED + DAY + (after ? 600 : 0));
	}
}

static void test_storage_put_back(void)
{
	uint8_t first[GRANT_SIZE], second[GRANT_SIZE];
	struct item old_flash[64];
	wipe(3, 0);
	make_grant(first, 1, 3, ISSUED, ISSUED + 2 * DAY);
	make_grant(second, 2, 3, ISSUED + DAY, ISSUED + 3 * DAY);
	assert(!install(first));
	run(DAY);
	memcpy(old_flash, flash, sizeof(flash));
	assert(!install(second));
	run(DAY + 1200);
	const uint64_t reached = device_install_earliest();

	/* The parts and the length of the first grant written back under the floor of the second. */
	for (size_t i = 0; i < 64; ++i) {
		const uint64_t uid = old_flash[i].uid;
		if (uid == INSTALL_GRANT || (uid >= INSTALL_GRANT_PART && uid < INSTALL_GRANT_PART + GRANT_PARTS))
			*item(uid) = old_flash[i];
	}
	assert(restart() == 1 && !device_install_policy());

	/* The saved time written back to what it was a day ago: the second grant's own time and the
	 * fenced expiries still hold it. It is late, never early, and no grant runs longer by the
	 * calendar because of it than its end says in board time. */
	wipe(3, 0);
	assert(!install(first));
	run(DAY);
	const struct item old_clock = *item(INSTALL_CLOCK);
	assert(!install(second));
	run(DAY + 1200);
	*item(INSTALL_CLOCK) = old_clock;
	assert(restart() == 0 && device_install_earliest() == ISSUED + DAY && device_install_earliest() < reached);
	const uint64_t fenced = ISSUED + DAY + 900;
	assert(!psa_its_set(INSTALL_REPLAY_FLOOR, sizeof(fenced), &fenced, 0));
	assert(restart() == 0 && device_install_earliest() == fenced);

	/* A changed byte in a stored part: the authority's signature fails and nothing is used. */
	item(INSTALL_GRANT_PART + 1)->data[5] ^= 1;
	assert(restart() == 1);
	item(INSTALL_GRANT_PART + 1)->data[5] ^= 1;
	assert(restart() == 0);
	/* A part or the saved time missing: nothing is used. */
	const struct item kept = *item(INSTALL_CLOCK);
	memset(item(INSTALL_CLOCK), 0, sizeof(struct item));
	assert(restart() == 1);
	flash[63] = kept;
	assert(restart() == 0);
	memset(item(INSTALL_GRANT_PART + 2), 0, sizeof(struct item));
	assert(restart() == 1);
}

int main(void)
{
	test_kept_through_a_restart();
	test_the_end_is_kept();
	test_bound_to_the_generation();
	test_reset_removes_it();
	test_power_loss_while_installing();
	test_storage_put_back();
	puts("install store ok");
	return 0;
}
