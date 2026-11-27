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

/* Time from `since` to `now`; a clock that went back counts as no time. */
static int64_t span_ms(int64_t since, int64_t now)
{
	if (now <= since) return 0;
	return since < 0 && now > INT64_MAX + since ? INT64_MAX : now - since;
}

static int64_t elapsed_ms(const nu54_adv_schedule *schedule, int64_t now_ms)
{
	return span_ms(schedule->started_ms, now_ms);
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

/* Sums and products that stop at UINT64_MAX. */
static uint64_t add(uint64_t a, uint64_t b)
{
	return a > UINT64_MAX - b ? UINT64_MAX : a + b;
}

static uint64_t multiply(uint64_t a, uint64_t b)
{
	return a && b > UINT64_MAX / a ? UINT64_MAX : a * b;
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
	uint64_t nanocoulombs = add(multiply(count, event_nc), (uint64_t)seconds * idle_na);
	return nanocoulombs > UINT64_MAX - 3599 ? UINT64_MAX : (nanocoulombs + 3599) / 3600;
}

nu54_adv_set nu54_adv_select(const nu54_adv_state *state)
{
	if (!state || state->connected >= state->capacity) return NU54_ADV_SET_NONE;
	if (state->pairing) return NU54_ADV_SET_PAIRING;
	if (state->payment) return NU54_ADV_SET_PAYMENT;
	return state->owner_wait == NU54_ADV_FAST ? NU54_ADV_SET_OWNER_FAST :
	       state->owner_wait == NU54_ADV_SLOW ? NU54_ADV_SET_OWNER_SLOW : NU54_ADV_SET_NONE;
}

int32_t nu54_adv_retry_delay_ms(unsigned failures)
{
	if (!failures || failures > NU54_ADV_RETRY_LIMIT) return -1;
	return (int32_t)(100u << (failures - 1));
}

int64_t nu54_owner_wait_idle_left_ms(int64_t idle_since_ms, int64_t now_ms, uint32_t limit_seconds)
{
	if (!limit_seconds) return -1;
	const int64_t limit = (int64_t)limit_seconds * 1000, idle = span_ms(idle_since_ms, now_ms);
	return idle >= limit ? 0 : limit - idle;
}
