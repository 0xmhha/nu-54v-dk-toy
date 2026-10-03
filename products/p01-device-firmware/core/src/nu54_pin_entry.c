#include "nu54_pin_entry.h"

#include <string.h>

void nu54_pin_entry_start(nu54_pin_entry_t *e, uint32_t now_ms)
{
	memset(e, 0, sizeof(*e));
	e->started_ms = now_ms;
	e->active = 1;
}

void nu54_pin_entry_stop(nu54_pin_entry_t *e)
{
	memset(e, 0, sizeof(*e));
}

nu54_pin_result_t nu54_pin_entry_poll(nu54_pin_entry_t *e, uint32_t now_ms)
{
	if (e->active && now_ms - e->started_ms > NU54_PIN_ENTRY_MS) {
		nu54_pin_entry_stop(e);
		return NU54_PIN_TIMEOUT;
	}
	return NU54_PIN_ENTERING;
}

nu54_pin_result_t nu54_pin_entry_event(nu54_pin_entry_t *e, nu54_pin_event_t ev, uint32_t now_ms, char out[NU54_PIN_LEN + 1])
{
	if (!e->active || nu54_pin_entry_poll(e, now_ms) == NU54_PIN_TIMEOUT) {
		return NU54_PIN_TIMEOUT;
	}
	switch (ev) {
	case NU54_PIN_TAP:
		e->current = (e->current + 1) % 10;
		break;
	case NU54_PIN_CLEAR:
		memset(e->digits, 0, sizeof(e->digits));
		e->kept = 0;
		e->current = 0;
		break;
	case NU54_PIN_NEXT:
		e->digits[e->kept++] = (uint8_t)e->current;
		e->current = 0;
		if (e->kept == NU54_PIN_LEN) {
			for (int i = 0; i < NU54_PIN_LEN; i++) {
				out[i] = (char)('0' + e->digits[i]);
			}
			out[NU54_PIN_LEN] = 0;
			nu54_pin_entry_stop(e);
			return NU54_PIN_DONE;
		}
		break;
	}
	return NU54_PIN_ENTERING;
}

uint8_t nu54_pin_entry_leds(const nu54_pin_entry_t *e)
{
	return (uint8_t)((1u << e->kept) - 1u);
}
