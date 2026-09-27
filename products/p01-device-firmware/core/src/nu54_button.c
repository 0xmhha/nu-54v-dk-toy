#include "nu54_button.h"

void nu54_button_reset(nu54_button_t *b)
{
	b->pressed = false;
	b->long_sent = false;
	b->pressed_at = 0;
}

nu54_button_event_t nu54_button_update(nu54_button_t *b, bool pressed, uint32_t now_ms)
{
	if (pressed == b->pressed) {
		return NU54_BTN_NONE; /* no edge: chatter that settled back */
	}
	b->pressed = pressed;
	if (pressed) {
		b->pressed_at = now_ms;
		b->long_sent = false;
		return NU54_BTN_NONE;
	}
	if (b->long_sent) {
		return NU54_BTN_NONE; /* the long press was already reported */
	}
	/* Unsigned subtraction handles the 32-bit millisecond counter wrapping. */
	return (uint32_t)(now_ms - b->pressed_at) < NU54_BUTTON_LONG_MS ? NU54_BTN_CLICK : NU54_BTN_LONG;
}

nu54_button_event_t nu54_button_tick(nu54_button_t *b, uint32_t now_ms)
{
	if (!b->pressed || b->long_sent || (uint32_t)(now_ms - b->pressed_at) < NU54_BUTTON_LONG_MS) {
		return NU54_BTN_NONE;
	}
	b->long_sent = true;
	return NU54_BTN_LONG;
}
