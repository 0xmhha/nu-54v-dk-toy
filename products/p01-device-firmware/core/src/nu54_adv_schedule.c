#include "nu54_adv_schedule.h"

#include <string.h>

static bool interval_ok(uint16_t min, uint16_t max)
{
	return min >= NU54_ADV_INTERVAL_MIN && max <= NU54_ADV_INTERVAL_MAX && min <= max;
}

bool nu54_adv_plan_valid(const nu54_adv_plan *plan)
{
	return plan && interval_ok(plan->fast_min, plan->fast_max) && interval_ok(plan->slow_min, plan->slow_max) &&
	       plan->fast_max <= plan->slow_min && (!plan->total_seconds || plan->fast_seconds <= plan->total_seconds);
}

bool nu54_adv_schedule_start(nu54_adv_schedule *schedule, const nu54_adv_plan *plan, int64_t now_ms)
{
	memset(schedule, 0, sizeof(*schedule));
	if (!nu54_adv_plan_valid(plan)) return false;
	schedule->plan = *plan;
	schedule->started_ms = now_ms;
	schedule->running = true;
	return true;
}

void nu54_adv_schedule_stop(nu54_adv_schedule *schedule)
{
	schedule->running = false;
}

/* Time since the start; a clock that went back counts as no time. */
static int64_t elapsed_ms(const nu54_adv_schedule *schedule, int64_t now_ms)
{
	return now_ms > schedule->started_ms ? now_ms - schedule->started_ms : 0;
}

nu54_adv_phase nu54_adv_schedule_phase(const nu54_adv_schedule *schedule, int64_t now_ms)
{
	if (!schedule->running) return NU54_ADV_OFF;
	const int64_t elapsed = elapsed_ms(schedule, now_ms);
	const nu54_adv_plan *plan = &schedule->plan;
	if (plan->total_seconds && elapsed >= (int64_t)plan->total_seconds * 1000) return NU54_ADV_OFF;
	return elapsed < (int64_t)plan->fast_seconds * 1000 ? NU54_ADV_FAST : NU54_ADV_SLOW;
}

bool nu54_adv_schedule_interval(const nu54_adv_schedule *schedule, int64_t now_ms, uint16_t *min, uint16_t *max)
{
	const nu54_adv_phase phase = nu54_adv_schedule_phase(schedule, now_ms);
	if (phase == NU54_ADV_OFF) return false;
	*min = phase == NU54_ADV_FAST ? schedule->plan.fast_min : schedule->plan.slow_min;
	*max = phase == NU54_ADV_FAST ? schedule->plan.fast_max : schedule->plan.slow_max;
	return true;
}

int64_t nu54_adv_schedule_wait_ms(const nu54_adv_schedule *schedule, int64_t now_ms)
{
	const nu54_adv_phase phase = nu54_adv_schedule_phase(schedule, now_ms);
	const nu54_adv_plan *plan = &schedule->plan;
	if (phase == NU54_ADV_OFF || (phase == NU54_ADV_SLOW && !plan->total_seconds)) return -1;
	const uint32_t end = phase == NU54_ADV_FAST ? plan->fast_seconds : plan->total_seconds;
	return (int64_t)end * 1000 - elapsed_ms(schedule, now_ms);
}

/* Events in `seconds` at an interval of `units` (0.625 ms each), rounded up. */
static uint64_t events(uint64_t seconds, uint16_t units)
{
	return (seconds * 1600 + units - 1) / units;
}

uint64_t nu54_adv_plan_charge_nah(const nu54_adv_plan *plan, uint32_t seconds, uint32_t event_nc, uint32_t idle_na)
{
	if (!nu54_adv_plan_valid(plan)) return 0;
	uint64_t on = plan->total_seconds && seconds > plan->total_seconds ? plan->total_seconds : seconds;
	uint64_t fast = on < plan->fast_seconds ? on : plan->fast_seconds;
	uint64_t count = events(fast, plan->fast_min) + events(on - fast, plan->slow_min);
	/* The idle current flows for the whole time, also after advertising stopped. */
	uint64_t nanocoulombs = count * event_nc + (uint64_t)seconds * idle_na;
	return (nanocoulombs + 3599) / 3600;
}
