#include "status_led.h"

#include <zephyr/kernel.h>

#include "board_io.h"

#define BLINK_MS 250
#define RESULT_MS 5000
#define SIGNED_MS 30000

typedef enum { RESULT_NONE, RESULT_SIGNED, RESULT_OK, RESULT_FAIL } result_t;

static bool mode, waiting, blink_on;
static bool pin_shown;
static uint8_t pin_mask;
static result_t result;
static int64_t result_until;

static void tick(struct k_work *w);
static K_WORK_DELAYABLE_DEFINE(tick_work, tick);

static void redraw(void)
{
	if (pin_shown) {
		for (uint8_t i = 0; i < BOARD_IO_COUNT; i++) {
			board_led_set(i, pin_mask & (1u << i));
		}
		return;
	}
	board_led_set(0, mode);
	board_led_set(1, waiting);
	board_led_set(2, result == RESULT_OK || (result == RESULT_SIGNED && blink_on));
	board_led_set(3, result == RESULT_FAIL);
}

/* Blinks LED3 while signed and ends a result when its time is up. */
static void tick(struct k_work *w)
{
	(void)w;
	if (result != RESULT_NONE && k_uptime_get() >= result_until) {
		result = RESULT_NONE;
	}
	blink_on = !blink_on;
	redraw();
	if (result != RESULT_NONE) {
		k_work_reschedule(&tick_work, K_MSEC(result == RESULT_SIGNED ? BLINK_MS : result_until - k_uptime_get()));
	}
}

static void show(result_t r, int64_t ms)
{
	result = r;
	result_until = k_uptime_get() + ms;
	blink_on = true;
	redraw();
	k_work_reschedule(&tick_work, K_MSEC(r == RESULT_SIGNED ? BLINK_MS : ms));
}

void status_led_mode(bool on)
{
	mode = on;
	redraw();
}

void status_led_waiting(bool on)
{
	waiting = on;
	redraw();
}

void status_led_event(nu54_event_t event)
{
	if (event == NU54_EVENT_SIGNED) {
		show(RESULT_SIGNED, SIGNED_MS);
	} else if (event == NU54_EVENT_REFUSED) {
		show(RESULT_FAIL, RESULT_MS);
	}
}

void status_led_outcome(bool approved)
{
	show(approved ? RESULT_OK : RESULT_FAIL, RESULT_MS);
}

void status_led_new_session(void)
{
	result = RESULT_NONE;
	k_work_cancel_delayable(&tick_work);
	redraw();
}

void status_led_clear(void)
{
	mode = false;
	waiting = false;
	status_led_new_session();
}

void status_led_pin(uint8_t mask)
{
	pin_shown = true;
	pin_mask = mask;
	redraw();
}

void status_led_pin_end(void)
{
	pin_shown = false;
	redraw();
}
