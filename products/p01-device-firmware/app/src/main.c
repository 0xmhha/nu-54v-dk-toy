/*
 * P01 payment signer firmware (week-7 development build).
 *
 * Boot: load the stored setup and start the payment link (device_setup.c), then run the key
 * self-test when the device holds a key.
 * Buttons: long-press SW4 enters payment mode (advertising), long-press SW3 pairing mode (a new
 * bond for the phone app or operator tool), SW1 approves and SW2 rejects a payment waiting on
 * the device. LEDs: LED1 payment mode, LED2 waiting for the button, LED3
 * approved, LED4 refused or failed. Every step is logged on the VCOM console.
 */
#include <zephyr/kernel.h>
#include <zephyr/logging/log.h>

#include <string.h>

#include "board_io.h"
#include "device_key.h"
#include "pay_link.h"
#include "nu54_keccak.h"
#include "nu54_protocol.h"
#include "nu54_sig.h"

LOG_MODULE_REGISTER(nu54_signer, LOG_LEVEL_INF);

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

	LOG_INF("nu54 signer, protocol v%d, %d reason codes", NU54_PROTOCOL_VERSION, NU54_REASON_COUNT);
	if (board_io_init() != 0) {
		return 0;
	}
	int st = pay_link_init();
	if (st != 0) {
		LOG_ERR("payment link unavailable (%d)", st);
		return 0;
	}
	if (pay_link_address(address)) {
		log_hex("device address", address, 20);
		key_self_test(address);
	}
	/* Week-7 development mapping (design 3): SW4 long press enters payment mode, SW1 approves,
	 * SW2 rejects. The final layout comes with the PIN LED guidance design. */
	LOG_INF("ready: long-press SW4 for payment mode, SW3 for pairing; SW1 approves, SW2 rejects");

	while (true) {
		board_button_event_t ev;

		if (board_button_wait(&ev, -1) != 0) {
			continue;
		}
		LOG_INF("SW%u %s", ev.button + 1, ev.event == NU54_BTN_CLICK ? "click" : "long press");
		if (ev.button == 3 && ev.event == NU54_BTN_LONG) {
			for (uint8_t i = 0; i < BOARD_IO_COUNT; i++) {
				board_led_set(i, false);
			}
			pay_link_payment_mode(120);
		} else if (ev.button == 2 && ev.event == NU54_BTN_LONG) {
			pay_link_pairing_mode(60);
		} else if (ev.button == 0 && ev.event == NU54_BTN_CLICK) {
			pay_link_button(1);
		} else if (ev.button == 1 && ev.event == NU54_BTN_CLICK) {
			pay_link_button(0);
		} else if (ev.button <= 1) {
			LOG_WRN("SW%u was held: approve and reject need a short press", ev.button + 1);
		}
	}
	return 0;
}
