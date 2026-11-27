/*
 * Low-power advertising schedule for a board that waits for its next owner.
 *
 * A board without an owner (new, or after a factory reset) advertises by itself: a short fast
 * phase so a phone that is already scanning finds it at once, then a slow phase that costs little,
 * and after the total time nothing until a button starts it again.
 *
 *   start ── fast_seconds ──> slow ── until total_seconds ──> off
 *
 * Board-independent: the caller passes the uptime, so the same logic runs on the device and in
 * host tests. Intervals are in 0.625 ms units, as the Bluetooth controller takes them.
 */
#ifndef NU54_ADV_SCHEDULE_H
#define NU54_ADV_SCHEDULE_H

#include <stdbool.h>
#include <stdint.h>

/* Advertising interval limits of the Bluetooth Core Specification: 20 ms and 10.24 s. */
#define NU54_ADV_INTERVAL_MIN 0x0020u
#define NU54_ADV_INTERVAL_MAX 0x4000u

typedef struct {
	uint32_t fast_seconds;  /* length of the fast phase; 0 starts slow */
	uint32_t total_seconds; /* fast and slow together; 0 never stops */
	uint16_t fast_min, fast_max;
	uint16_t slow_min, slow_max;
} nu54_adv_plan;

typedef enum {
	NU54_ADV_OFF = 0,
	NU54_ADV_FAST,
	NU54_ADV_SLOW,
} nu54_adv_phase;

typedef struct {
	nu54_adv_plan plan;
	int64_t started_ms;
	bool running;
} nu54_adv_schedule;

/* Intervals inside the specification, min not above max, the fast phase not slower than the slow
 * one, and a fast phase that fits in the total time. */
bool nu54_adv_plan_valid(const nu54_adv_plan *plan);

/* Starts (or starts again) at now_ms. Returns false and stays off for a plan that is not valid. */
bool nu54_adv_schedule_start(nu54_adv_schedule *schedule, const nu54_adv_plan *plan, int64_t now_ms);
void nu54_adv_schedule_stop(nu54_adv_schedule *schedule);

nu54_adv_phase nu54_adv_schedule_phase(const nu54_adv_schedule *schedule, int64_t now_ms);

/* The interval range of the current phase. Returns false when the schedule is off. */
bool nu54_adv_schedule_interval(const nu54_adv_schedule *schedule, int64_t now_ms, uint16_t *min, uint16_t *max);

/* Milliseconds until the phase changes, or -1 when it never does (off, or slow without an end). */
int64_t nu54_adv_schedule_wait_ms(const nu54_adv_schedule *schedule, int64_t now_ms);

/*
 * Upper estimate of the charge the plan draws in its first `seconds`, in nanoampere-hours.
 * event_nc is the charge of one advertising event in nanocoulombs, idle_na the current between
 * events in nanoamperes. Every event is counted at the shortest interval of its phase, and the
 * random delay the controller adds to each event is left out, so the real charge is lower.
 * A result too large for 64 bits is UINT64_MAX, never a wrapped small number.
 */
uint64_t nu54_adv_plan_charge_nah(const nu54_adv_plan *plan, uint32_t seconds, uint32_t event_nc, uint32_t idle_na);

/* ---- which set the board advertises ---- */

typedef enum {
	NU54_ADV_SET_NONE = 0,
	NU54_ADV_SET_PAYMENT,    /* SW4 window */
	NU54_ADV_SET_PAIRING,    /* SW3 window; carries the payment service too */
	NU54_ADV_SET_OWNER_FAST, /* owner wait, by itself */
	NU54_ADV_SET_OWNER_SLOW,
} nu54_adv_set;

typedef struct {
	unsigned connected, capacity; /* centrals connected now, and how many the board takes */
	bool pairing, payment;        /* the button windows */
	nu54_adv_phase owner_wait;    /* the owner-wait schedule at this moment */
} nu54_adv_state;

/* Nothing without a free connection slot, since advertising is connectable. Pairing wins over
 * payment, and a button window wins over the owner wait. The owner wait goes on while a slot is
 * free, also beside a connected central: one central that stays must not hide the board. */
nu54_adv_set nu54_adv_select(const nu54_adv_state *state);

/* A failed start is tried again a few times, each wait twice the one before, and then left until
 * the next event (a connection, a button, a phase change) asks again. `failures` counts the failed
 * starts in a row, from 1. Returns the wait in milliseconds, or -1 to give up. */
#define NU54_ADV_RETRY_LIMIT 5
int32_t nu54_adv_retry_delay_ms(unsigned failures);

/*
 * A board without an owner takes a central only for the installation or the owner's TLS session.
 * One that does neither is disconnected, so that centrals which merely stay connected cannot use
 * up the connection slots. idle_since_ms is when the central connected or was last seen in such
 * an exchange. Returns the milliseconds it may still stay, 0 to disconnect it now, or -1 when
 * limit_seconds is 0 (no limit).
 */
int64_t nu54_owner_wait_idle_left_ms(int64_t idle_since_ms, int64_t now_ms, uint32_t limit_seconds);

#endif /* NU54_ADV_SCHEDULE_H */
