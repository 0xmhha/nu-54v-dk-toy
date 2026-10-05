/* PIN entry on the buttons (core/nu54_pin_entry): taps, next, clear, wrap and the time limit. */
#include <stdio.h>
#include <string.h>

#include "nu54_pin_entry.h"

static int failures;

static void check(int ok, const char *what)
{
	if (!ok) {
		failures++;
		printf("FAIL %s\n", what);
	}
}

/* Enters `pin` digit by digit (n taps, then next) starting at time t; returns the last result. */
static nu54_pin_result_t enter(nu54_pin_entry_t *e, const char *pin, uint32_t t, char out[5])
{
	nu54_pin_result_t r = NU54_PIN_ENTERING;
	for (const char *p = pin; *p; p++) {
		for (int k = 0; k < *p - '0'; k++) {
			r = nu54_pin_entry_event(e, NU54_PIN_TAP, t, out);
		}
		r = nu54_pin_entry_event(e, NU54_PIN_NEXT, t, out);
	}
	return r;
}

int main(void)
{
	nu54_pin_entry_t e;
	char out[5] = {0};

	nu54_pin_entry_start(&e, 1000);
	check(enter(&e, "2580", 2000, out) == NU54_PIN_DONE && strcmp(out, "2580") == 0, "2580 is entered as taps and next");
	check(!e.active && e.digits[0] == 0 && e.kept == 0, "a finished entry is wiped");

	nu54_pin_entry_start(&e, 0);
	enter(&e, "97", 10, out);
	check(nu54_pin_entry_leds(&e) == 0x3, "two kept digits light LED1 and LED2");
	nu54_pin_entry_event(&e, NU54_PIN_CLEAR, 20, out);
	check(nu54_pin_entry_leds(&e) == 0 && e.kept == 0, "clear starts again");
	check(enter(&e, "1111", 30, out) == NU54_PIN_DONE && strcmp(out, "1111") == 0, "after clear a new PIN is entered");

	nu54_pin_entry_start(&e, 0);
	for (int k = 0; k < 10; k++) {
		nu54_pin_entry_event(&e, NU54_PIN_TAP, 1, out);
	}
	check(e.current == 0, "a tenth tap wraps the digit to 0");

	nu54_pin_entry_start(&e, 100);
	check(nu54_pin_entry_poll(&e, 100 + NU54_PIN_ENTRY_MS) == NU54_PIN_ENTERING, "still entering at the limit");
	check(nu54_pin_entry_poll(&e, 101 + NU54_PIN_ENTRY_MS) == NU54_PIN_TIMEOUT && !e.active, "times out past the limit");
	check(nu54_pin_entry_event(&e, NU54_PIN_NEXT, 200 + NU54_PIN_ENTRY_MS, out) == NU54_PIN_TIMEOUT, "events after a timeout do nothing");

	nu54_pin_entry_start(&e, 0xfffff000u); /* the millisecond clock wraps during entry */
	check(enter(&e, "0000", 0x00000100u, out) == NU54_PIN_DONE && strcmp(out, "0000") == 0, "entry across the clock wrap");

	/* The view the phone app shows: kept digits, the digit being entered, zeros after it. */
	{
		char v[5];
		nu54_pin_entry_start(&e, 0);
		check(nu54_pin_entry_view(&e, v) == 0 && strcmp(v, "0000") == 0, "view starts at 0000");
		for (int k = 0; k < 3; k++) {
			nu54_pin_entry_event(&e, NU54_PIN_TAP, 1, out);
		}
		check(nu54_pin_entry_view(&e, v) == 0 && strcmp(v, "3000") == 0, "taps count up the first digit");
		nu54_pin_entry_event(&e, NU54_PIN_NEXT, 1, out);
		for (int k = 0; k < 12; k++) {
			nu54_pin_entry_event(&e, NU54_PIN_TAP, 1, out);
		}
		check(nu54_pin_entry_view(&e, v) == 1 && strcmp(v, "3200") == 0, "the second digit wraps after 9");
		nu54_pin_entry_event(&e, NU54_PIN_CLEAR, 1, out);
		check(nu54_pin_entry_view(&e, v) == 0 && strcmp(v, "0000") == 0, "clear shows 0000 again");
		nu54_pin_entry_stop(&e);
	}

	printf("%s: PIN entry, %d failures\n", failures ? "FAIL" : "ok", failures);
	return failures ? 1 : 0;
}
