/* The real storage, PIN, order and vault code over a fake flash that loses power at every step. */
#include "board_io.h"
#include "device_order.h"
#include "device_recovery_pin.h"
#include "device_spending.h"
#include "device_vault.h"
#include "nu54_pin_entry.h"
#include "status_led.h"

#include <psa/crypto.h>
#include <psa/internal_trusted_storage.h>

#include <assert.h>
#include <setjmp.h>
#include <stdio.h>
#include <string.h>

/* ---- flash and keys; a write either happened or did not when the power goes ---- */
struct item {uint64_t uid; size_t size; uint8_t data[160];};
struct key {psa_key_attributes_t attrs; uint8_t seed;};
struct flash {struct item items[64]; struct key keys[8];};
static struct flash flash;
static uint8_t next_seed = 1;
static int writes_left = -1; /* -1: power stays on */
static bool power_after;     /* the write lands and then the power goes */
static jmp_buf power;
static bool read_fails;
static bool key_stays; /* the key store says "destroyed" and keeps the key */

static bool cut(void)
{
	if (writes_left < 0) return false;
	if (writes_left-- > 0) return false;
	writes_left = -1;
	return true;
}

static struct item *item(uint64_t uid)
{
	for (size_t i = 0; i < 64; ++i)
		if (flash.items[i].uid == uid) return &flash.items[i];
	return NULL;
}

static struct key *key(psa_key_id_t id)
{
	for (size_t i = 0; i < 8; ++i)
		if (flash.keys[i].attrs.id == id) return &flash.keys[i];
	return NULL;
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
	if (read_fails) return PSA_ERROR_GENERIC_ERROR;
	if (!found) return PSA_ERROR_DOES_NOT_EXIST;
	info->size = found->size;
	return PSA_SUCCESS;
}
psa_status_t psa_its_get(psa_storage_uid_t uid, size_t offset, size_t size, void *data, size_t *used)
{
	struct item *found = item(uid);
	assert(!offset);
	if (read_fails) return PSA_ERROR_GENERIC_ERROR;
	if (!found) return PSA_ERROR_DOES_NOT_EXIST;
	*used = size < found->size ? size : found->size;
	memcpy(data, found->data, *used);
	return PSA_SUCCESS;
}
psa_status_t psa_its_set(psa_storage_uid_t uid, size_t size, const void *data, uint32_t flags)
{
	assert(!flags && uid && size <= 160);
	struct item *slot = item(uid);
	for (size_t i = 0; !slot && i < 64; ++i)
		if (!flash.items[i].uid) slot = &flash.items[i];
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
psa_status_t psa_get_key_attributes(psa_key_id_t id, psa_key_attributes_t *attrs)
{
	if (!key(id)) return PSA_ERROR_INVALID_HANDLE;
	*attrs = key(id)->attrs;
	return PSA_SUCCESS;
}
psa_status_t psa_generate_key(const psa_key_attributes_t *attrs, psa_key_id_t *id)
{
	if (key(attrs->id)) return PSA_ERROR_ALREADY_EXISTS;
	struct key *slot = NULL;
	for (size_t i = 0; !slot && i < 8; ++i)
		if (!flash.keys[i].attrs.id) slot = &flash.keys[i];
	assert(slot);
	WRITE(slot->attrs = *attrs; slot->seed = next_seed++);
	*id = attrs->id;
	return PSA_SUCCESS;
}
psa_status_t psa_destroy_key(psa_key_id_t id)
{
	if (!key(id)) return PSA_ERROR_INVALID_HANDLE;
	if (key_stays && id == 0x54b2) return PSA_SUCCESS;
	WRITE(memset(key(id), 0, sizeof(struct key)));
	return PSA_SUCCESS;
}
psa_status_t psa_export_public_key(psa_key_id_t id, uint8_t *out, size_t size, size_t *used)
{
	if (!key(id)) return PSA_ERROR_INVALID_HANDLE;
	assert(size >= 65);
	memset(out, key(id)->seed, 65);
	out[0] = 4;
	*used = 65;
	return PSA_SUCCESS;
}
psa_status_t psa_sign_hash(psa_key_id_t id, psa_algorithm_t alg, const uint8_t *digest, size_t length, uint8_t *out,
			   size_t size, size_t *used)
{
	(void)digest;
	if (!key(id)) return PSA_ERROR_INVALID_HANDLE;
	assert(length == 32 && size == 64 && alg == key(id)->attrs.algorithm);
	memset(out, key(id)->seed, 64);
	*used = 64;
	return PSA_SUCCESS;
}
psa_status_t psa_mac_compute(psa_key_id_t id, psa_algorithm_t alg, const uint8_t *data, size_t length, uint8_t *out,
			     size_t size, size_t *used)
{
	(void)alg;
	if (!key(id)) return PSA_ERROR_INVALID_HANDLE;
	memset(out, 0, size);
	memcpy(out, data, length);
	out[31] = key(id)->seed;
	*used = size;
	return PSA_SUCCESS;
}
psa_status_t psa_mac_verify(psa_key_id_t id, psa_algorithm_t alg, const uint8_t *data, size_t length,
			    const uint8_t *mac, size_t size)
{
	uint8_t expected[32];
	size_t used;
	assert(size == 32);
	return psa_mac_compute(id, alg, data, length, expected, 32, &used) == PSA_SUCCESS && !memcmp(expected, mac, 32) ?
	       PSA_SUCCESS : PSA_ERROR_GENERIC_ERROR;
}

/* Included so a simulated restart can clear its memory. */
#include "../app/src/device_recovery_pin.c"

/* ---- vault over the same flash ---- */
static nu54_vault vault;
static int vault_read(void *context, unsigned record, void *data, size_t size)
{
	(void)context;
	struct item *found = item(0x5420 + record);
	if (!found) return NU54_VAULT_ABSENT;
	if (found->size != size) return NU54_VAULT_IO;
	memcpy(data, found->data, size);
	return 0;
}
static int vault_write(void *context, unsigned record, const void *data, size_t size)
{
	(void)context;
	return psa_its_set(0x5420 + record, size, data, 0);
}
static int vault_key(void *context, bool personal, bool create, uint8_t address[20])
{
	(void)context;
	(void)create;
	memset(address, 7, 20);
	return personal ? NU54_VAULT_ABSENT : 0;
}
nu54_vault *device_vault_owner(void) { return &vault; }

/* ---- buttons, lamp, clock ---- */
static int64_t uptime = 1000;
static const char *taps;
static unsigned digit, tapped, locked_shown, started;
int64_t k_uptime_get(void) { return uptime; }
void status_led_pin(status_pin_t what) { if (what == STATUS_PIN_START) ++started; }
void status_led_pin_end(void) {}
void status_led_locked(void) { ++locked_shown; }
int board_button_wait(board_button_event_t *out, int32_t timeout_ms)
{
	(void)timeout_ms;
	uptime += 20;
	*out = (board_button_event_t){.event = NU54_BTN_CLICK};
	if (tapped < (unsigned)(taps[digit] - '0')) {
		++tapped;
		out->button = 0;
	} else {
		out->button = 2;
		tapped = 0;
		digit = (digit + 1) % 4;
	}
	return 0;
}
static int enter(const char *pin, bool virgin)
{
	taps = pin;
	digit = tapped = 0;
	return device_recovery_pin(virgin);
}

static int boot(void)
{
	const nu54_vault_storage io = {NULL, vault_read, vault_write, vault_key};
	uptime = 1000;
	wait_from = 0;
	if (nu54_vault_open(&vault, &io)) return -1;
	return device_spending_init();
}

/* A board without an owner: nothing of a wallet, in memory or on flash. */
static void assert_no_wallet(void)
{
	uint8_t digest[32] = {0}, signature[64], home[20];
	assert(!device_spending_address() && !key(0x54b2) && !item(0x5571));
	assert(device_spending_sign(digest, signature) && !device_spending_return_address(home));
}

/* What the board does when the first owner registers: the key, then the owner. */
static void register_owner(const uint8_t owner[32], uint8_t address[20])
{
	assert_no_wallet();
	assert(!device_spending_enroll());
	memcpy(address, device_spending_address(), 20);
	assert(!nu54_vault_rebind(&vault, vault.record.generation, owner));
}

static void use_wallet(uint8_t address[20])
{
	const uint8_t owner[32] = {1}, home[20] = {9};
	nu54_order_record record = {.expires = 500, .outcome = NU54_ORDER_APPROVED};
	assert(!boot());
	register_owner(owner, address);
	assert(!enter("1234", true));
	assert(!device_spending_set_return(home));
	assert(!device_order_store(3, &record) && !device_order_charge(86400 * 5, 70));
}

static void assert_fresh(uint64_t generation)
{
	uint8_t home[20];
	assert_no_wallet(); /* the reset leaves no key, and the restart makes none */
	assert(!device_spending_return_address(home));
	assert(!item(0x5472) && !key(0x54a0) && !item(0x5553) && !item(0x5561) && !item(0x5570));
	assert(vault.record.generation == generation && (vault.record.flags & NU54_VAULT_RELEASED));
	const uint8_t none[32] = {0};
	assert(!memcmp(vault.record.owner, none, 32));
	assert(device_recovery_pin_wait() == 0);
}

static void test_reset_survives_power_loss_everywhere(void)
{
	unsigned cuts = 0;
	for (int after = 0; after < 2; ++after) {
		for (int step = 0;; ++step) {
			uint8_t before[20];
			memset(&flash, 0, sizeof(flash));
			use_wallet(before);
			uint64_t generation = vault.record.generation;
			power_after = after;
			writes_left = step;
			volatile bool finished = false;
			if (!setjmp(power)) {
				assert(!device_spending_erase());
				finished = true;
			}
			writes_left = -1;
			/* Power comes back; a second loss during the repair is repaired as well. */
			writes_left = 1;
			if (!setjmp(power)) (void)boot();
			writes_left = -1;
			assert(!boot());
			if (device_spending_address()) {
				/* Nothing was erased: only possible when even the first mark was not written. */
				assert(!after && step == 0 && !(vault.record.flags & NU54_VAULT_RELEASED));
				assert(!memcmp(device_spending_address(), before, 20));
			} else {
				assert_fresh(generation + 1);
			}
			++cuts;
			if (finished) break;
			assert(step < 40);
		}
	}
	printf("power cut at %u points\n", cuts);
}

static void test_first_boot_survives_power_loss(void)
{
	for (int after = 0; after < 2; ++after) {
		for (int step = 0; step < 8; ++step) {
			memset(&flash, 0, sizeof(flash));
			power_after = after;
			writes_left = step;
			if (!setjmp(power)) (void)boot();
			writes_left = -1;
			assert(!boot());
			assert_no_wallet(); /* a new board makes no key by itself */
		}
	}
}

/* The key is made right before the owner is stored. Wherever the power goes, the next start finds
 * either no owner and no key, or the owner with the key that was made for it. */
static void test_registration_survives_power_loss(void)
{
	const uint8_t owner[32] = {3};
	unsigned cuts = 0;
	for (int released = 0; released < 2; ++released) {
		for (int after = 0; after < 2; ++after) {
			for (int step = 0;; ++step) {
				uint8_t made[20] = {0};
				memset(&flash, 0, sizeof(flash));
				if (released) {
					use_wallet(made);
					assert(!device_spending_erase());
				}
				assert(!boot());
				const uint64_t generation = vault.record.generation;
				power_after = after;
				writes_left = step;
				volatile bool finished = false;
				if (!setjmp(power)) {
					register_owner(owner, made);
					finished = true;
				}
				writes_left = 1; /* a second loss while the start repairs */
				if (!setjmp(power)) (void)boot();
				writes_left = -1;
				assert(!boot());
				if (vault.record.generation == generation) {
					assert_no_wallet();
					assert(!finished);
				} else {
					assert(vault.record.generation == generation + 1 && !vault.record.flags);
					assert(device_spending_address() && key(0x54b2) && item(0x5571));
					assert(!memcmp(item(0x5571)->data, device_spending_address(), 20));
				}
				++cuts;
				if (finished) break;
				assert(step < 20);
			}
		}
	}
	printf("registration cut at %u points\n", cuts);
}

static void test_key_only_for_the_first_owner(void)
{
	uint8_t first[20], other[20];
	const uint8_t owner[32] = {4};
	memset(&flash, 0, sizeof(flash));
	assert(!boot());
	register_owner(owner, first);
	assert(device_spending_enroll()); /* an owner exists: the wallet is never replaced */
	assert(!memcmp(device_spending_address(), first, 20));
	assert(!boot() && !memcmp(device_spending_address(), first, 20));
	assert(device_spending_enroll() && !memcmp(device_spending_address(), first, 20));

	/* The vault did not take the owner: the key that was made for it goes at once. */
	memset(&flash, 0, sizeof(flash));
	assert(!boot() && !device_spending_enroll() && key(0x54b2));
	device_spending_withdraw();
	assert_no_wallet();
	assert(!device_spending_enroll()); /* and the next try makes another one */
	memcpy(other, device_spending_address(), 20);
	assert(memcmp(other, first, 20));

	/* A vault that stopped after a failed write may hold the owner: the key stays for the restart. */
	vault.enabled = false;
	device_spending_withdraw();
	assert(!device_spending_address() && key(0x54b2) && device_spending_enroll());
	assert(!boot());
	assert_no_wallet(); /* flash holds no owner, so the start removes the key */

	/* A board of firmware that made the key at start: no owner, a key and its recorded address. */
	memset(&flash, 0, sizeof(flash));
	assert(!boot() && !device_spending_enroll());
	assert(!boot());
	assert_no_wallet();
}

/* What the board does for a first owner, in its order: the PIN, the key, the owner. A step that
 * fails takes the key and the PIN back (device_pilot_button). */
static int register_with_pin(const uint8_t owner[32], const char *pin, bool vault_takes)
{
	if (enter(pin, true)) {
		device_spending_withdraw();
		return -1;
	}
	if (device_spending_enroll() || !vault_takes || nu54_vault_rebind(&vault, vault.record.generation, owner)) {
		device_spending_withdraw();
		return -1;
	}
	return 0;
}

static void assert_no_pin(void)
{
	assert(!item(0x5472) && !key(0x54a0) && device_recovery_pin_wait() == 0);
}

/* A board without an owner keeps no PIN: the next person could not know it and could never
 * register. Before this rule a first registration that failed after the PIN entry left the PIN. */
static void test_unowned_board_keeps_no_pin(void)
{
	const uint8_t first[32] = {5}, second[32] = {6};
	for (int released = 0; released < 2; ++released) {
		uint8_t made[20];
		memset(&flash, 0, sizeof(flash));
		if (released) {
			use_wallet(made);
			assert(!device_spending_erase());
		}
		assert(!boot());
		/* The PIN is set, the key is made, and the vault does not take the owner. */
		assert(register_with_pin(first, "1234", false));
		assert_no_wallet();
		assert_no_pin();
		assert(!boot());
		assert_no_wallet();
		assert_no_pin();
		/* Another person sets another PIN and registers. */
		assert(!register_with_pin(second, "5678", true));
		assert(!enter("5678", false) && enter("1234", false));
		assert(!boot() && device_spending_address() && !enter("5678", false)); /* with an owner the PIN stays */
	}

	/* A PIN found at start on a board without an owner: a power cut after the PIN entry, or
	 * firmware from before this rule. */
	memset(&flash, 0, sizeof(flash));
	assert(!boot() && !enter("1234", true) && item(0x5472) && key(0x54a0));
	assert(!boot());
	assert_no_pin();
	assert(!enter("5678", true) && !enter("5678", false));

	/* Only the PIN key was made before the power went. */
	memset(&flash, 0, sizeof(flash));
	assert(!boot() && !enter("1234", true));
	memset(item(0x5472), 0, sizeof(struct item));
	assert(!boot());
	assert_no_pin();

	/* A vault that stopped after a failed write may hold the owner: the PIN stays for the restart,
	 * which finds no owner on flash and removes it. */
	memset(&flash, 0, sizeof(flash));
	assert(!boot() && !enter("1234", true) && !device_spending_enroll());
	vault.enabled = false;
	device_spending_withdraw();
	assert(item(0x5472) && key(0x54a0) && key(0x54b2));
	assert(!boot());
	assert_no_wallet();
	assert_no_pin();

	/* The start does not go on while a PIN without an owner cannot be removed. */
	memset(&flash, 0, sizeof(flash));
	assert(!boot() && !enter("1234", true));
	read_fails = true;
	assert(boot());
	read_fails = false;
	assert(!boot());
	assert_no_pin();
}

/* The same through a power cut at every write of a first registration, completed or taken back:
 * the next start finds the owner with the PIN that was entered, or no owner and no PIN. */
static void test_first_registration_with_pin_survives_power_loss(void)
{
	const uint8_t owner[32] = {7};
	unsigned cuts = 0;
	for (int takes = 0; takes < 2; ++takes) {
		for (int after = 0; after < 2; ++after) {
			for (int step = 0;; ++step) {
				memset(&flash, 0, sizeof(flash));
				assert(!boot());
				const uint64_t generation = vault.record.generation;
				power_after = after;
				writes_left = step;
				volatile bool finished = false;
				if (!setjmp(power)) {
					(void)register_with_pin(owner, "1234", takes);
					finished = true;
				}
				writes_left = 1; /* a second loss while the start repairs */
				if (!setjmp(power)) (void)boot();
				writes_left = -1;
				assert(!boot());
				if (vault.record.generation == generation) {
					assert_no_wallet();
					assert_no_pin();
					assert(!enter("5678", true)); /* the next person's PIN is taken */
				} else {
					assert(takes && device_spending_address() && !enter("1234", false));
				}
				++cuts;
				if (finished) break;
				assert(step < 30);
			}
		}
	}
	printf("registration with a PIN cut at %u points\n", cuts);
}

/* A reset is done only when the key store itself says the key is gone. */
static void test_reset_fails_while_the_key_is_there(void)
{
	uint8_t before[20];
	memset(&flash, 0, sizeof(flash));
	use_wallet(before);
	uint64_t generation = vault.record.generation;
	key_stays = true;
	assert(device_spending_erase() && !device_spending_address());
	assert(item(0x5570) && key(0x54b2)); /* the mark stays, so the next start goes on */
	assert(boot() && !device_spending_address() && device_spending_enroll());
	assert(item(0x5570));
	key_stays = false;
	assert(!boot());
	assert_fresh(generation + 1);

	/* The same check when the start finds a key without an owner. */
	assert(!device_spending_enroll());
	key_stays = true;
	assert(boot() && !device_spending_address() && device_spending_enroll());
	device_spending_withdraw(); /* a failed removal stops the wallet until the restart */
	key_stays = false;
	assert(!boot());
	assert_no_wallet();
}

static void test_reset_then_new_owner(void)
{
	uint8_t first[20], second[20];
	const uint8_t owner[32] = {2};
	memset(&flash, 0, sizeof(flash));
	use_wallet(first);
	uint64_t generation = vault.record.generation;
	assert(!device_spending_erase());
	assert_no_wallet(); /* gone at once, before the restart */
	assert(device_spending_enroll()); /* and nothing is made until the restart */
	assert(!boot());
	assert_fresh(generation + 1);
	assert(!device_spending_erase() && !boot()); /* resetting an empty wallet is harmless */
	assert(vault.record.generation == generation + 1);
	assert_fresh(generation + 1);
	register_owner(owner, second);
	assert(memcmp(first, second, 20)); /* the next owner gets another key */
	assert(vault.record.generation == generation + 2 && !vault.record.flags);
	assert(!enter("5678", true) && enter("1234", false) && !enter("5678", false));
	assert(!boot() && !memcmp(device_spending_address(), second, 20)); /* stays after a restart */
}

static void test_lost_key_is_not_replaced(void)
{
	uint8_t before[20];
	memset(&flash, 0, sizeof(flash));
	use_wallet(before);
	memset(key(0x54b2), 0, sizeof(struct key));
	assert(boot() && !device_spending_address() && !key(0x54b2));

	memset(&flash, 0, sizeof(flash));
	use_wallet(before);
	key(0x54b2)->attrs.usage |= PSA_KEY_USAGE_EXPORT; /* a key that could leave the board */
	assert(boot() && !device_spending_address());

	memset(&flash, 0, sizeof(flash));
	use_wallet(before);
	key(0x54b2)->seed ^= 0x40; /* another key under the same name */
	assert(boot() && !device_spending_address());

	memset(&flash, 0, sizeof(flash));
	use_wallet(before);
	read_fails = true;
	assert(device_spending_init() && !device_spending_address());
	/* Unreadable records hold a reset back for one order life, not forever. */
	assert(device_order_open(1, NU54_SPEND_ORDER_LIFE_MS - 1) && !device_order_open(1, NU54_SPEND_ORDER_LIFE_MS));
	read_fails = false;
}

static void test_return_address_is_set_once(void)
{
	uint8_t before[20], home[20];
	const uint8_t other[20] = {8};
	memset(&flash, 0, sizeof(flash));
	assert(!boot());
	assert(device_spending_set_return(other)); /* no wallet before the first owner */
	assert(!device_spending_enroll());
	assert(device_spending_set_return(device_spending_address())); /* not to itself */
	use_wallet(before);
	assert(device_spending_return_address(home) && home[0] == 9);
	assert(device_spending_set_return(other));
	assert(device_spending_return_address(home) && home[0] == 9);
}

static void test_wrong_pins_wait_and_never_erase(void)
{
	uint8_t before[20];
	memset(&flash, 0, sizeof(flash));
	use_wallet(before);
	const int32_t waits[] = {60, 300, 900, 3600, 3600};
	for (int i = 0; i < 4; ++i) assert(enter("0000", false) && device_recovery_pin_wait() == 0);
	for (int i = 0; i < 5; ++i) {
		assert(enter("0000", false));
		/* Rounded up to whole seconds: never shorter than the table. */
		assert(device_recovery_pin_wait() >= waits[i] && device_recovery_pin_wait() <= waits[i] + 1);
		unsigned shown = locked_shown, begun = started;
		assert(enter("1234", false)); /* even the right PIN waits */
		assert(locked_shown == shown + 1 && started == begun);
		uptime += (int64_t)waits[i] * 1000 / 2;
		assert(device_recovery_pin_wait() >= waits[i] / 2 && device_recovery_pin_wait() <= waits[i] / 2 + 1);
		/* Switching the power off starts the same wait again, never a shorter one. */
		assert(!boot());
		uptime = (int64_t)waits[i] * 1000 - 1; /* counted from boot */
		assert(device_recovery_pin_wait() == 1 && enter("1234", false));
		uptime = (int64_t)waits[i] * 1000;
		assert(device_recovery_pin_wait() == 0);
	}
	assert(!memcmp(device_spending_address(), before, 20) && key(0x54b2)); /* the wallet is still there */
	assert(!enter("1234", false) && device_recovery_pin_wait() == 0);
	assert(enter("0000", false) && device_recovery_pin_wait() == 0); /* the count started over */

	struct item *record = item(0x5472);
	assert(record->size == 33);
	record->data[32] = 255;
	uptime += 3601 * 1000;
	assert(enter("0000", false) && record->data[32] == 255); /* the count never wraps */
	assert(device_recovery_pin_wait() >= 3600);
}

static void test_open_orders_hold_a_reset_back(void)
{
	nu54_order_record record = {.expires = 500, .outcome = NU54_ORDER_APPROVED};
	memset(&flash, 0, sizeof(flash));
	assert(!device_order_open(400, 0) && !device_order_open(0, 0));
	assert(!device_order_store(1, &record));
	assert(device_order_open(499, 1 << 30) && !device_order_open(500, 0));
	assert(device_order_open(0, NU54_SPEND_ORDER_LIFE_MS - 1) && !device_order_open(0, NU54_SPEND_ORDER_LIFE_MS));
	record.outcome = NU54_ORDER_REFUSED;
	assert(!device_order_store(1, &record) && !device_order_open(499, 0));
}

int main(void)
{
	assert(device_spending_enroll() && !device_spending_address()); /* nothing before the stored state was read */
	test_first_boot_survives_power_loss();
	test_registration_survives_power_loss();
	test_key_only_for_the_first_owner();
	test_unowned_board_keeps_no_pin();
	test_first_registration_with_pin_survives_power_loss();
	test_reset_fails_while_the_key_is_there();
	test_reset_then_new_owner();
	test_lost_key_is_not_replaced();
	test_return_address_is_set_once();
	test_wrong_pins_wait_and_never_erase();
	test_open_orders_hold_a_reset_back();
	test_reset_survives_power_loss_everywhere();
	puts("spending store ok");
	return 0;
}
