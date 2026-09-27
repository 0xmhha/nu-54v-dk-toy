/*
 * Button gesture detection: click and long press.
 *
 * Board-independent: the caller feeds the debounced level and the current time,
 * so the same logic runs on the device and in host tests.
 *
 *   press ... release before NU54_BUTTON_LONG_MS  -> NU54_BTN_CLICK (on release)
 *   press ... held for NU54_BUTTON_LONG_MS        -> NU54_BTN_LONG  (once, while held)
 *   release after a long press                    -> nothing
 */
#ifndef NU54_BUTTON_H
#define NU54_BUTTON_H

#include <stdbool.h>
#include <stdint.h>

#define NU54_BUTTON_LONG_MS 1000u

typedef enum {
	NU54_BTN_NONE = 0,
	NU54_BTN_CLICK,
	NU54_BTN_LONG,
} nu54_button_event_t;

typedef struct {
	bool pressed;
	bool long_sent;
	uint32_t pressed_at;
} nu54_button_t;

void nu54_button_reset(nu54_button_t *b);

/* Feed a debounced level change. Returns CLICK on a short release, otherwise NONE. */
nu54_button_event_t nu54_button_update(nu54_button_t *b, bool pressed, uint32_t now_ms);

/* Call periodically while the button is held. Returns LONG once when the threshold passes. */
nu54_button_event_t nu54_button_tick(nu54_button_t *b, uint32_t now_ms);

#endif /* NU54_BUTTON_H */
