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

void nu54_button_suppress(nu54_button_t *b)
{
	if (b->pressed) b->long_sent = true;
}

nu54_button_event_t nu54_chord_tick(nu54_chord_t *c, bool both, uint32_t now_ms)
{
	if (!both) {
		c->held = c->sent = false;
		return NU54_BTN_NONE;
	}
	if (!c->held) {
		c->held = true;
		c->sent = false;
		c->since = now_ms;
		return NU54_BTN_NONE;
	}
	if (c->sent || (uint32_t)(now_ms - c->since) < NU54_CHORD_MS) return NU54_BTN_NONE;
	c->sent = true;
	return NU54_BTN_CHORD;
}
