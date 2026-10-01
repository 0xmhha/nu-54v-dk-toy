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

#include <string.h>

#include "board_io.h"
#include "device_key.h"
#include "nu54_keccak.h"
#include "nu54_protocol.h"
#include "nu54_sig.h"

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

static void log_hex(const char *what, const uint8_t *b, size_t n)
{
	char s[2 * 65 + 1];
	for (size_t i = 0; i < n && i < 65; i++) {
		static const char d[] = "0123456789abcdef";
		s[2 * i] = d[b[i] >> 4];
		s[2 * i + 1] = d[b[i] & 15];
	}
	s[2 * (n < 65 ? n : 65)] = 0;
	LOG_INF("%s 0x%s", what, s);
}

/* Device key self-test: sign a fixed digest, finish (low-s, v) and recover our own address. */
static void key_self_test(const uint8_t address[20])
{
	static const char probe[] = "nu54 device key self-test";
	uint8_t digest[32], rs[64], sig[65], back[20];
	nu54_keccak256((const uint8_t *)probe, sizeof(probe) - 1, digest);
	int st = device_key_sign(digest, rs);
	if (st != 0) {
		LOG_ERR("key self-test: sign failed (%d)", st);
		return;
	}
	if (nu54_sig_finish(digest, rs, address, sig) != NU54_SIG_OK || nu54_sig_recover(digest, sig, 65, back) != NU54_SIG_OK ||
	    memcmp(back, address, 20) != 0) {
		LOG_ERR("key self-test: signature does not recover to the device address");
		return;
	}
	log_hex("key self-test digest", digest, 32);
	log_hex("key self-test signature", sig, 65);
}

int main(void)
{
	uint8_t address[20];
	int created;

	LOG_INF("nu54 signer, protocol v%d, %d reason codes", NU54_PROTOCOL_VERSION, NU54_REASON_COUNT);
	int st = device_key_init(address, &created);
	if (st != 0) {
		LOG_ERR("device key unavailable (%d)", st);
	} else {
		LOG_INF("device key %s", created ? "created" : "opened");
		log_hex("device address", address, 20);
		key_self_test(address);
	}

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
