/*
 * LEDs and buttons of the NU-54V-DK, reached through devicetree aliases
 * (led0..led3, sw0..sw3) so the pin numbers stay in the board package.
 */
#ifndef BOARD_IO_H
#define BOARD_IO_H

#include <stdbool.h>
#include <stdint.h>

#include "nu54_button.h"

#define BOARD_IO_COUNT 4

typedef struct {
	uint8_t button;             /* 0..3 = SW1..SW4 */
	nu54_button_event_t event;  /* CLICK or LONG */
} board_button_event_t;

int board_io_init(void);
void board_led_set(uint8_t led, bool on);
void board_led_toggle(uint8_t led);

/* Wait for the next button gesture. Returns 0, or -EAGAIN on timeout. */
int board_button_wait(board_button_event_t *out, int32_t timeout_ms);

#endif /* BOARD_IO_H */
