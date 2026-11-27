/* The operator's grant kept through a restart. The storage and clock code is extracted from
 * device_install.c at build time and runs over a fake flash that loses power at every write. The
 * certificates of the grant are not looked at here (adopt is a stand-in). */
#include "install_store_fixture.h"

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

/* The audit's second finding, for a grant: one that took long to arrive leaves the latest
 * possible time that far above the lower bound. A fresh grant is the authority's word about the
 * time, so it sets both ends: what an earlier grant left, in this start or through a restart, does
 * not stay with it. */
static void test_a_fresh_grant_sets_both_ends(void)
{
	uint8_t slow[GRANT_SIZE], quick[GRANT_SIZE];
	wipe(3, 0);
	make_grant(slow, 1, 3, ISSUED, ISSUED + 31 * DAY);
	assert(!install_after(slow, 58000) && device_install_now() - device_install_earliest() == 58);
	/* The slow grant is kept, and its width with it. */
	run(700);
	assert(restart() == 0 && device_install_earliest() == ISSUED + 600 && device_install_now() == ISSUED + 658);
	/* A quick grant ten seconds on by the board's time: two seconds wide. */
	make_grant(quick, 2, 3, ISSUED + 610, ISSUED + 31 * DAY);
	assert(!install(quick) && device_install_earliest() == ISSUED + 610 && device_install_now() == ISSUED + 612);
	assert(adopted_at == ISSUED + 612);
	/* That is what a restart goes on from. */
	assert(restart() == 0 && device_install_now() - device_install_earliest() == 2);

	/* In one start as well, with the slow one still in use. */
	wipe(3, 0);
	assert(!install_after(slow, 58000));
	uptime += 5000;
	make_grant(quick, 2, 3, ISSUED + 6, ISSUED + 31 * DAY);
	assert(!install(quick) && device_install_now() - device_install_earliest() == 2);
	/* The lower bound still never goes back: a grant from before the time the board reached is
	 * refused, however fresh it is. */
	uptime += 5000;
	make_grant(slow, 3, 3, ISSUED + 10, ISSUED + 31 * DAY);
	assert(install(slow) && device_install_earliest() == ISSUED + 11);
	/* A grant that took the whole minute is not taken, and one a moment quicker is. */
	make_grant(slow, 3, 3, ISSUED + 20, ISSUED + 31 * DAY);
	assert(install_after(slow, 60000) && !install_after(slow, 59999));
	assert(device_install_now() - device_install_earliest() == 60);
}

int main(void)
{
	test_a_fresh_grant_sets_both_ends();
	test_kept_through_a_restart();
	test_the_end_is_kept();
	test_bound_to_the_generation();
	test_reset_removes_it();
	test_power_loss_while_installing();
	test_storage_put_back();
	puts("install store ok");
	return 0;
}
