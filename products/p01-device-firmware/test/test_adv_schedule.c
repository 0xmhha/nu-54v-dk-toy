/* The advertising schedule of a board that waits for its next owner (core/nu54_adv_schedule). */
#include <stdio.h>

#include "nu54_adv_schedule.h"

static int failures;

static void check(int ok, const char *what)
{
	if (!ok) {
		failures++;
		printf("FAIL %s\n", what);
	}
}

/* The defaults of app/Kconfig: 30 s at 100 to 150 ms, then 1 to 1.2 s until three hours. */
static const nu54_adv_plan defaults = {
	.fast_seconds = 30, .total_seconds = 10800, .fast_min = 160, .fast_max = 240, .slow_min = 1600, .slow_max = 1920,
};

static void phases(void)
{
	nu54_adv_schedule s = {0};
	uint16_t min = 0, max = 0;
	const int64_t t0 = 5000;

	check(nu54_adv_schedule_phase(&s, t0) == NU54_ADV_OFF, "a schedule that never started is off");
	check(!nu54_adv_schedule_interval(&s, t0, &min, &max) && nu54_adv_schedule_wait_ms(&s, t0) == -1,
	      "off has no interval and no next change");

	check(nu54_adv_schedule_start(&s, &defaults, t0), "the default plan starts");
	check(nu54_adv_schedule_phase(&s, t0) == NU54_ADV_FAST, "it starts fast");
	check(nu54_adv_schedule_interval(&s, t0, &min, &max) && min == 160 && max == 240, "fast interval 100 to 150 ms");
	check(nu54_adv_schedule_wait_ms(&s, t0) == 30000, "the fast phase lasts 30 s");
	check(nu54_adv_schedule_wait_ms(&s, t0 + 29999) == 1, "one millisecond of the fast phase left");

	check(nu54_adv_schedule_phase(&s, t0 + 30000) == NU54_ADV_SLOW, "slow from 30 s on");
	check(nu54_adv_schedule_interval(&s, t0 + 30000, &min, &max) && min == 1600 && max == 1920, "slow interval 1 to 1.2 s");
	check(nu54_adv_schedule_wait_ms(&s, t0 + 30000) == 10770000, "the slow phase runs to three hours");
	check(nu54_adv_schedule_phase(&s, t0 + 10799999) == NU54_ADV_SLOW, "still slow just before three hours");

	check(nu54_adv_schedule_phase(&s, t0 + 10800000) == NU54_ADV_OFF, "off at three hours");
	check(!nu54_adv_schedule_interval(&s, t0 + 10800000, &min, &max), "no interval after the end");
	check(nu54_adv_schedule_wait_ms(&s, t0 + 10800000) == -1, "nothing follows the end");
	check(nu54_adv_schedule_phase(&s, INT64_MAX) == NU54_ADV_OFF, "off far in the future");

	/* A button starts it again from the fast phase. */
	check(nu54_adv_schedule_start(&s, &defaults, t0 + 20000000) && nu54_adv_schedule_phase(&s, t0 + 20000000) == NU54_ADV_FAST,
	      "starting again begins with the fast phase");
	check(nu54_adv_schedule_phase(&s, t0) == NU54_ADV_FAST && nu54_adv_schedule_wait_ms(&s, t0) == 30000,
	      "a clock that went back counts as no time");

	nu54_adv_schedule_stop(&s);
	check(nu54_adv_schedule_phase(&s, t0 + 20000001) == NU54_ADV_OFF, "stopped is off");
}

static void edges(void)
{
	nu54_adv_schedule s;
	nu54_adv_plan p = defaults;

	p.total_seconds = 0;
	check(nu54_adv_schedule_start(&s, &p, 0), "a plan without an end is valid");
	check(nu54_adv_schedule_wait_ms(&s, 0) == 30000, "its fast phase still ends");
	check(nu54_adv_schedule_phase(&s, INT64_MAX) == NU54_ADV_SLOW && nu54_adv_schedule_wait_ms(&s, 40000) == -1,
	      "then it stays slow for good");

	p = defaults;
	p.fast_seconds = 0;
	check(nu54_adv_schedule_start(&s, &p, 0) && nu54_adv_schedule_phase(&s, 0) == NU54_ADV_SLOW, "no fast phase starts slow");

	p = defaults;
	p.fast_seconds = p.total_seconds;
	check(nu54_adv_schedule_start(&s, &p, 0) && nu54_adv_schedule_phase(&s, 10799999) == NU54_ADV_FAST &&
	      nu54_adv_schedule_phase(&s, 10800000) == NU54_ADV_OFF, "fast for the whole time, then off");
	check(nu54_adv_schedule_wait_ms(&s, 0) == 10800000 && nu54_adv_schedule_wait_ms(&s, 10799999) == 1 &&
	      nu54_adv_schedule_wait_ms(&s, 10800000) == -1, "fast for the whole time: one change, straight to off");
	check(nu54_adv_plan_charge_nah(&p, 10800, 35000, 0) == (10800ull * 10 * 35000 + 3599) / 3600,
	      "fast for the whole time: every event at the fast interval");

	/* No fast phase and no end: slow from the first moment, for good. */
	uint16_t min = 0, max = 0;
	p = defaults;
	p.fast_seconds = 0;
	p.total_seconds = 0;
	check(nu54_adv_plan_valid(&p) && nu54_adv_schedule_start(&s, &p, 7), "neither a fast phase nor an end is valid");
	check(nu54_adv_schedule_phase(&s, 7) == NU54_ADV_SLOW && nu54_adv_schedule_phase(&s, INT64_MAX) == NU54_ADV_SLOW,
	      "it is slow at once and stays slow");
	check(nu54_adv_schedule_interval(&s, 7, &min, &max) && min == 1600 && max == 1920, "at the slow interval");
	check(nu54_adv_schedule_wait_ms(&s, 7) == -1 && nu54_adv_schedule_wait_ms(&s, INT64_MAX) == -1,
	      "with no change to wait for");
	check(nu54_adv_plan_charge_nah(&p, 3600, 35000, 0) == 3600ull * 35000 / 3600, "one event a second, hour after hour");

	/* The longest times the plan can hold do not overflow. */
	p = defaults;
	p.fast_seconds = UINT32_MAX;
	p.total_seconds = UINT32_MAX;
	const int64_t end = (int64_t)UINT32_MAX * 1000;
	check(nu54_adv_plan_valid(&p) && nu54_adv_schedule_start(&s, &p, 0), "the longest plan is valid");
	check(nu54_adv_schedule_wait_ms(&s, 0) == end && nu54_adv_schedule_phase(&s, end - 1) == NU54_ADV_FAST &&
	      nu54_adv_schedule_phase(&s, end) == NU54_ADV_OFF && nu54_adv_schedule_phase(&s, INT64_MAX) == NU54_ADV_OFF,
	      "the longest fast phase ends where it should");
	p.fast_seconds = 30;
	check(nu54_adv_schedule_start(&s, &p, 0) && nu54_adv_schedule_wait_ms(&s, 30000) == end - 30000 &&
	      nu54_adv_schedule_phase(&s, end - 1) == NU54_ADV_SLOW, "the longest slow phase too");
	check(nu54_adv_schedule_start(&s, &p, INT64_MIN) && nu54_adv_schedule_phase(&s, INT64_MAX) == NU54_ADV_OFF &&
	      nu54_adv_schedule_wait_ms(&s, INT64_MAX) == -1, "the widest clock span is past the end, not an overflow");
	/* 30 s fast and the rest slow: 300 + (UINT32_MAX - 30) events. */
	check(nu54_adv_plan_charge_nah(&p, UINT32_MAX, 35000, 5000) ==
	      ((300ull + UINT32_MAX - 30) * 35000 + (uint64_t)UINT32_MAX * 5000 + 3599) / 3600, "136 years add up exactly");
	p.fast_seconds = UINT32_MAX;
	p.fast_min = p.fast_max = NU54_ADV_INTERVAL_MIN;
	check(nu54_adv_plan_charge_nah(&p, UINT32_MAX, UINT32_MAX, UINT32_MAX) == UINT64_MAX,
	      "a charge beyond 64 bits is reported as the largest value, not wrapped");
}

static void selection(void)
{
	const nu54_adv_phase phases[] = {NU54_ADV_OFF, NU54_ADV_FAST, NU54_ADV_SLOW};
	const nu54_adv_set owner[] = {NU54_ADV_SET_NONE, NU54_ADV_SET_OWNER_FAST, NU54_ADV_SET_OWNER_SLOW};
	check(nu54_adv_select(NULL) == NU54_ADV_SET_NONE, "no state advertises nothing");
	for (unsigned connected = 0; connected <= 3; ++connected) {
		for (unsigned modes = 0; modes < 4; ++modes) {
			for (unsigned phase = 0; phase < 3; ++phase) {
				const nu54_adv_state state = {.connected = connected, .capacity = 2, .pairing = modes & 1,
					.payment = modes & 2, .owner_wait = phases[phase]};
				const nu54_adv_set expected = connected >= 2 ? NU54_ADV_SET_NONE : (modes & 1) ? NU54_ADV_SET_PAIRING :
							      (modes & 2) ? NU54_ADV_SET_PAYMENT : owner[phase];
				check(nu54_adv_select(&state) == expected, "full: nothing; else pairing, payment, owner wait");
			}
		}
	}
	/* The reviewed case: one central that stays connected no longer hides a board without an owner. */
	nu54_adv_state held = {.connected = 1, .capacity = 2, .owner_wait = NU54_ADV_SLOW};
	check(nu54_adv_select(&held) == NU54_ADV_SET_OWNER_SLOW, "the owner wait goes on beside one connected central");
	held.connected = 2;
	held.payment = true;
	check(nu54_adv_select(&held) == NU54_ADV_SET_NONE, "two centrals leave no slot, whatever the button asks");
	held.capacity = 0;
	held.connected = 0;
	check(nu54_adv_select(&held) == NU54_ADV_SET_NONE, "a board that takes no central does not advertise");
}

static void retries(void)
{
	const int32_t waits[] = {100, 200, 400, 800, 1600};
	int32_t total = 0;
	check(nu54_adv_retry_delay_ms(0) == -1, "no failure, no retry");
	for (unsigned failures = 1; failures <= NU54_ADV_RETRY_LIMIT; ++failures) {
		check(nu54_adv_retry_delay_ms(failures) == waits[failures - 1], "each wait is twice the one before");
		total += nu54_adv_retry_delay_ms(failures);
	}
	check(total == 3100, "five tries within about three seconds");
	check(nu54_adv_retry_delay_ms(NU54_ADV_RETRY_LIMIT + 1) == -1 && nu54_adv_retry_delay_ms(UINT32_MAX) == -1,
	      "then it gives up until the next event");
}

/* Two centrals that connect and do nothing must not keep the slots (app/Kconfig default 30 s). */
static void idle_centrals(void)
{
	check(nu54_owner_wait_idle_left_ms(1000, 1000, 30) == 30000, "a central gets the whole limit when it connects");
	check(nu54_owner_wait_idle_left_ms(1000, 30999, 30) == 1, "one millisecond left");
	check(nu54_owner_wait_idle_left_ms(1000, 31000, 30) == 0, "at the limit it goes");
	check(nu54_owner_wait_idle_left_ms(1000, INT64_MAX, 30) == 0, "and long after");
	check(nu54_owner_wait_idle_left_ms(5000, 1000, 30) == 30000, "a clock that went back counts as no time");
	check(nu54_owner_wait_idle_left_ms(INT64_MIN, INT64_MAX, 30) == 0, "the widest clock span does not overflow");
	check(nu54_owner_wait_idle_left_ms(1000, INT64_MAX, 0) == -1, "a limit of 0 never disconnects");
	check(nu54_owner_wait_idle_left_ms(0, 0, UINT32_MAX) == (int64_t)UINT32_MAX * 1000, "the longest limit does not overflow");
	/* Seen in an installation or TLS exchange at 20 s: the count starts again from there. */
	check(nu54_owner_wait_idle_left_ms(20000, 31000, 30) == 19000, "an exchange starts the count again");
}

static void refuse(nu54_adv_plan p, const char *what)
{
	nu54_adv_schedule s;
	check(!nu54_adv_plan_valid(&p), what);
	check(!nu54_adv_schedule_start(&s, &p, 0) && nu54_adv_schedule_phase(&s, 0) == NU54_ADV_OFF, "a refused plan stays off");
	check(nu54_adv_plan_charge_nah(&p, 10800, 35000, 5000) == 0, "a refused plan has no budget");
}

static void validity(void)
{
	nu54_adv_plan p;

	check(nu54_adv_plan_valid(&defaults), "the defaults are valid");
	check(!nu54_adv_plan_valid(NULL), "no plan is not valid");
	p = defaults; p.fast_min = 31; refuse(p, "below 20 ms");
	p = defaults; p.slow_max = 0x4001; refuse(p, "above 10.24 s");
	p = defaults; p.fast_min = 241; refuse(p, "fast min above max");
	p = defaults; p.slow_min = 1921; refuse(p, "slow min above max");
	p = defaults; p.slow_min = 239; refuse(p, "a slow phase faster than the fast one");
	p = defaults; p.fast_seconds = 10801; refuse(p, "a fast phase longer than the total");
	p = defaults; p.fast_min = 0x20; p.fast_max = 0x20; p.slow_min = 0x4000; p.slow_max = 0x4000;
	check(nu54_adv_plan_valid(&p), "the limits themselves are valid");
}

/* The numbers of the three-hour budget in board-secure-key-and-advertising.md, section 5. */
static void budget(void)
{
	/* Linear regulator estimate: 35 uC an event, 5 uA between events.
	 * fast 30 s at 100 ms = 300 events, slow 10770 s at 1 s = 10770 events:
	 * (11070 * 35000 nC + 10800 s * 5000 nA) / 3600 = 122625 nAh. */
	check(nu54_adv_plan_charge_nah(&defaults, 10800, 35000, 5000) == 122625, "three hours on the linear regulator: 0.123 mAh");
	/* DC/DC estimate: 15 uC an event, 3 uA between events: (166050000 + 32400000) / 3600 = 55125 nAh. */
	check(nu54_adv_plan_charge_nah(&defaults, 10800, 15000, 3000) == 55125, "three hours on DC/DC: 0.055 mAh");
	/* Development image that never sleeps deeply: 1.5 mA between events dominates. */
	check(nu54_adv_plan_charge_nah(&defaults, 10800, 35000, 1500000) == 4607625, "three hours without deep sleep: 4.6 mAh");

	check(nu54_adv_plan_charge_nah(&defaults, 30, 35000, 0) == 2917, "the fast phase alone: 300 events");
	check(nu54_adv_plan_charge_nah(&defaults, 0, 35000, 5000) == 0, "no time, no charge");
	/* After the end only the idle current is left: one more hour adds 5 uA * 1 h = 5000 nAh. */
	check(nu54_adv_plan_charge_nah(&defaults, 14400, 35000, 5000) == 127625, "after the end only the idle current flows");

	nu54_adv_plan p = defaults;
	p.total_seconds = 0;
	/* Without an end the slow phase goes on: 300 + 14370 events. */
	check(nu54_adv_plan_charge_nah(&p, 14400, 35000, 5000) == (14670ull * 35000 + 14400ull * 5000 + 3599) / 3600,
	      "without an end the fourth hour still advertises");
	p = defaults;
	p.slow_min = 1601;
	check(nu54_adv_plan_charge_nah(&p, 10800, 35000, 0) == (uint64_t)(300 + 10764) * 35000 / 3600 + 1,
	      "an interval that does not divide the time rounds the events up");
}

int main(void)
{
	phases();
	edges();
	validity();
	budget();
	selection();
	retries();
	idle_centrals();
	if (failures) {
		printf("%d failure(s)\n", failures);
		return 1;
	}
	printf("adv schedule: ok\n");
	return 0;
}
