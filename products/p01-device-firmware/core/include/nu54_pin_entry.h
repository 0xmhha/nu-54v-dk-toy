/*
 * PIN entry on the buttons (payment-protocol.md 5, N28): the device has no screen and the PIN
 * never travels over BLE. The renter enters four digits one at a time:
 *
 *   SW1 click   tap: the digit is the number of taps (0 to 9; a tenth tap wraps to 0)
 *   SW3 click   next: keep this digit and go to the next one
 *   SW2 click   clear: start again from the first digit
 *
 * LED k (1..4) is lit once digit k is kept; the LED of the digit being entered flashes on each
 * tap. The fourth "next" finishes the PIN. Entry that is not finished within
 * NU54_PIN_ENTRY_MS of its start times out, which the session treats as a PIN not entered in
 * time (setup TIMEOUT, limit change TIMEOUT). Board-independent: the caller feeds events and
 * the time, so the same logic runs in host tests.
 */
#ifndef NU54_PIN_ENTRY_H
#define NU54_PIN_ENTRY_H

#include <stdint.h>

#define NU54_PIN_LEN 4
/* Leaves the kiosk's 60 s limit-change wait room for the approve button after the PIN. */
#define NU54_PIN_ENTRY_MS 45000u

typedef enum {
	NU54_PIN_TAP,
	NU54_PIN_NEXT,
	NU54_PIN_CLEAR,
} nu54_pin_event_t;

typedef enum {
	NU54_PIN_ENTERING, /* keep feeding events */
	NU54_PIN_DONE,     /* `out` holds the digits, NUL-terminated */
	NU54_PIN_TIMEOUT,  /* not finished in time */
} nu54_pin_result_t;

typedef struct {
	uint8_t digits[NU54_PIN_LEN];
	int kept;    /* digits kept so far */
	int current; /* taps on the digit being entered */
	uint32_t started_ms;
	int active;
} nu54_pin_entry_t;

void nu54_pin_entry_start(nu54_pin_entry_t *e, uint32_t now_ms);

/* One button event. On DONE `out` gets the PIN and the entry ends (and is wiped). */
nu54_pin_result_t nu54_pin_entry_event(nu54_pin_entry_t *e, nu54_pin_event_t ev, uint32_t now_ms, char out[NU54_PIN_LEN + 1]);

/* TIMEOUT once the entry has run longer than NU54_PIN_ENTRY_MS (the entry ends), else ENTERING. */
nu54_pin_result_t nu54_pin_entry_poll(nu54_pin_entry_t *e, uint32_t now_ms);

/* LEDs to light: bit k-1 for each kept digit k. */
uint8_t nu54_pin_entry_leds(const nu54_pin_entry_t *e);

/* Ends the entry and wipes the digits (the session no longer asks for the PIN). */
void nu54_pin_entry_stop(nu54_pin_entry_t *e);

#endif /* NU54_PIN_ENTRY_H */
