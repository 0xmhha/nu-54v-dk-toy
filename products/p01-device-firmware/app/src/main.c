/*
 * P01 payment signer firmware skeleton (WBS2-P01-01).
 *
 * Current behavior, used to check the board I/O:
 * - click SWn     -> toggle LEDn
 * - long-press SWn -> blink all four LEDs three times, then restore them
 * Every gesture is logged on the VCOM console. Key handling, BLE and signing
 * are added by later WBS tasks.
 */
#include <zephyr/kernel.h>
#include <zephyr/logging/log.h>

#include "board_io.h"
#include "nu54_protocol.h"

LOG_MODULE_REGISTER(nu54_signer, LOG_LEVEL_INF);

static bool led_on[BOARD_IO_COUNT];

static void blink_all(int times)
{
	for (int n = 0; n < times; n++) {
		for (uint8_t i = 0; i < BOARD_IO_COUNT; i++) {
			board_led_set(i, true);
		}
		k_msleep(150);
		for (uint8_t i = 0; i < BOARD_IO_COUNT; i++) {
			board_led_set(i, false);
		}
		k_msleep(150);
	}
	for (uint8_t i = 0; i < BOARD_IO_COUNT; i++) {
		board_led_set(i, led_on[i]);
	}
}

int main(void)
{
	LOG_INF("nu54 signer skeleton, protocol v%d, %d reason codes", NU54_PROTOCOL_VERSION, NU54_REASON_COUNT);

	if (board_io_init() != 0) {
		return 0;
	}
	LOG_INF("ready: click SWn toggles LEDn, long-press blinks all LEDs");

	while (true) {
		board_button_event_t ev;

		if (board_button_wait(&ev, -1) != 0) {
			continue;
		}
		if (ev.event == NU54_BTN_CLICK) {
			led_on[ev.button] = !led_on[ev.button];
			board_led_set(ev.button, led_on[ev.button]);
			LOG_INF("SW%u click -> LED%u %s", ev.button + 1, ev.button + 1, led_on[ev.button] ? "on" : "off");
		} else {
			LOG_INF("SW%u long press -> blink all", ev.button + 1);
			blink_all(3);
		}
	}
	return 0;
}
