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
	if (failures) {
		printf("%d failure(s)\n", failures);
		return 1;
	}
	printf("adv schedule: ok\n");
	return 0;
}
