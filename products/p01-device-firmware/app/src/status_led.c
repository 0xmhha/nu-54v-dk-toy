#include "status_led.h"

#include <zephyr/kernel.h>

#include "board_io.h"

/* A pattern is pairs of on and off times in milliseconds. */
typedef struct {
	const uint16_t *steps;
	uint8_t count; /* number of on/off pairs */
	bool repeat;
} pattern_t;

#define PATTERN(name, rep, ...)                                                                                        \
	static const uint16_t name##_steps[] = {__VA_ARGS__};                                                          \
	static const pattern_t name = {name##_steps, ARRAY_SIZE(name##_steps) / 2, rep}

PATTERN(P_MODE, true, 100, 1900);
PATTERN(P_PAIRING, true, 100, 150, 100, 1650);
PATTERN(P_BONDED, false, 1000, 0);
PATTERN(P_WAITING, true, 200, 200);
PATTERN(P_SIGNED, true, 500, 500);
PATTERN(P_APPROVED, false, 2000, 0);
PATTERN(P_FAILED, false, 100, 150, 100, 150, 100, 0);
PATTERN(P_PIN_START, false, 300, 200, 300, 0);
PATTERN(P_PIN_TAP, false, 80, 0);
PATTERN(P_PIN_KEEP, false, 500, 0);

#define SIGNED_MS 30000

static bool mode, pairing, waiting, signed_wait, pin_active;
static int64_t signed_until;
static const pattern_t *oneshot; /* plays over the ongoing pattern, then ends */
static const pattern_t *playing;
static uint8_t step; /* index of the half-step (on, off, on, off, ...) */

static void tick(struct k_work *w);
static K_WORK_DELAYABLE_DEFINE(tick_work, tick);
static struct k_spinlock lock; /* callers are the session work queue and the main loop */

static void lamp(bool on)
{
	for (uint8_t i = 0; i < BOARD_IO_COUNT; i++) {
		board_led_set(i, on);
	}
}

/* The ongoing indication, by priority; NULL is dark. */
static const pattern_t *ongoing(void)
{
	if (pin_active) {
		return NULL;
	}
	if (waiting) {
		return &P_WAITING;
	}
	if (signed_wait && k_uptime_get() < signed_until) {
		return &P_SIGNED;
	}
	signed_wait = false;
	if (pairing) {
		return &P_PAIRING;
	}
	return mode ? &P_MODE : NULL;
}

/* Starts the pattern that should show now, from its first step. */
static void restart(void)
{
	playing = oneshot ? oneshot : ongoing();
	step = 0;
	k_work_reschedule(&tick_work, K_NO_WAIT);
}

static void tick(struct k_work *w)
{
	(void)w;
	k_spinlock_key_t key = k_spin_lock(&lock);
	if (playing && step >= 2 * playing->count) {
		if (playing->repeat) {
			step = 0;
		} else {
			oneshot = NULL; /* a one-off ends: hand back to the ongoing pattern */
			playing = ongoing();
			step = 0;
		}
	}
	if (!playing) {
		lamp(false);
		k_spin_unlock(&lock, key);
		return;
	}
	bool on = (step % 2) == 0;
	uint16_t ms = playing->steps[step];
	step++;
	lamp(on && ms > 0);
	k_spin_unlock(&lock, key);
	k_work_reschedule(&tick_work, K_MSEC(ms > 0 ? ms : 1));
}

static void set(bool *flag, bool value)
{
	k_spinlock_key_t key = k_spin_lock(&lock);
	if (*flag != value) {
		*flag = value;
		if (!oneshot) {
			restart();
		}
	}
	k_spin_unlock(&lock, key);
}

static void play(const pattern_t *p)
{
	k_spinlock_key_t key = k_spin_lock(&lock);
	oneshot = p;
	restart();
	k_spin_unlock(&lock, key);
}

void status_led_mode(bool on)
{
	set(&mode, on);
}

void status_led_pairing(bool on)
{
	set(&pairing, on);
}

void status_led_paired(bool ok)
{
	play(ok ? &P_BONDED : &P_FAILED);
}

void status_led_waiting(bool on)
{
	set(&waiting, on);
}

void status_led_event(nu54_event_t event)
{
	if (event == NU54_EVENT_SIGNED) {
		k_spinlock_key_t key = k_spin_lock(&lock);
		signed_wait = true;
		signed_until = k_uptime_get() + SIGNED_MS;
		k_spin_unlock(&lock, key);
		set(&waiting, false);
		play(NULL); /* show the slow blinking now */
	} else if (event == NU54_EVENT_REFUSED) {
		k_spinlock_key_t key = k_spin_lock(&lock);
		signed_wait = false;
		k_spin_unlock(&lock, key);
		play(&P_FAILED);
	}
}

void status_led_outcome(bool approved)
{
	k_spinlock_key_t key = k_spin_lock(&lock);
	signed_wait = false;
	k_spin_unlock(&lock, key);
	play(approved ? &P_APPROVED : &P_FAILED);
}

void status_led_new_session(void)
{
	k_spinlock_key_t key = k_spin_lock(&lock);
	signed_wait = false;
	oneshot = NULL;
	restart();
	k_spin_unlock(&lock, key);
}

void status_led_clear(void)
{
	k_spinlock_key_t key = k_spin_lock(&lock);
	mode = pairing = waiting = signed_wait = pin_active = false;
	oneshot = NULL;
	restart();
	k_spin_unlock(&lock, key);
}

void status_led_idle(void)
{
	k_spinlock_key_t key = k_spin_lock(&lock);
	mode = waiting = signed_wait = false;
	if (!oneshot) {
		restart();
	}
	k_spin_unlock(&lock, key);
}

void status_led_pin(status_pin_t what)
{
	k_spinlock_key_t key = k_spin_lock(&lock);
	pin_active = true;
	k_spin_unlock(&lock, key);
	play(what == STATUS_PIN_START ? &P_PIN_START : what == STATUS_PIN_TAP ? &P_PIN_TAP : &P_PIN_KEEP);
}

void status_led_pin_end(void)
{
	set(&pin_active, false);
}
