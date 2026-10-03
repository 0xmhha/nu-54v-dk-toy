/*
 * P01 payment signer firmware (week-7 development build).
 *
 * Boot: load the stored setup and start the payment link (device_setup.c), then run the key
 * self-test when the device holds a key.
 * Buttons: long-press SW4 enters payment mode (advertising), long-press SW3 pairing mode (a new
 * bond for the phone app or operator tool), SW1 approves and SW2 rejects a payment waiting on
 * the device. While the session asks for the PIN the buttons enter it instead
 * (core/nu54_pin_entry.h: SW1 taps a digit, SW3 keeps it, SW2 starts again; LEDs show the kept
 * digits). LEDs: LED1 payment mode, LED2 waiting for the button, LED3
 * approved, LED4 refused or failed. Every step is logged on the VCOM console.
 */
#include <zephyr/kernel.h>
#include <zephyr/logging/log.h>

#include <string.h>

#include "board_io.h"
#include "device_key.h"
#include "pay_link.h"
#include "nu54_keccak.h"
#include "nu54_pin_entry.h"
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

static void leds(uint8_t mask)
{
	for (uint8_t i = 0; i < BOARD_IO_COUNT; i++) {
		board_led_set(i, mask & (1u << i));
	}
}

/* The digit being entered flashes on each tap; the kept ones stay lit. */
static void pin_feedback(const nu54_pin_entry_t *e, bool tap)
{
	uint8_t mask = nu54_pin_entry_leds(e);
	if (tap && e->kept < BOARD_IO_COUNT) {
		leds(mask | (1u << e->kept));
		k_msleep(80);
	}
	leds(mask);
}

/* Set when an entry was handed to the session, until the session stops asking: the session works
 * on its own queue, so the next loop may still see the PIN asked for. */
static bool handed_over;

static void hand_over(const char *pin)
{
	leds(0);
	pay_link_pin(pin, pin ? NU54_PIN_LEN : 0);
	handed_over = true;
}

/* A button while the PIN is asked for. Never logs the digits. */
static void pin_button(nu54_pin_entry_t *e, const board_button_event_t *ev)
{
	char pin[NU54_PIN_LEN + 1];
	nu54_pin_event_t pe;

	if (ev->event != NU54_BTN_CLICK || ev->button == 3) {
		return;
	}
	pe = ev->button == 0 ? NU54_PIN_TAP : ev->button == 2 ? NU54_PIN_NEXT : NU54_PIN_CLEAR;
	switch (nu54_pin_entry_event(e, pe, k_uptime_get_32(), pin)) {
	case NU54_PIN_DONE:
		hand_over(pin);
		memset(pin, 0, sizeof(pin));
		break;
	case NU54_PIN_TIMEOUT:
		hand_over(NULL);
		break;
	default:
		pin_feedback(e, pe == NU54_PIN_TAP);
	}
}

int main(void)
{
	static nu54_pin_entry_t pin_entry;
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
		bool wanted = pay_link_pin_wanted();

		/* The session asks for the PIN (setup or a limit change): start, or end a stale entry. */
		if (!wanted) {
			handed_over = false;
		}
		if (wanted && !pin_entry.active && !handed_over) {
			nu54_pin_entry_start(&pin_entry, k_uptime_get_32());
			LOG_INF("enter the PIN: SW1 taps a digit, SW3 keeps it, SW2 starts again");
			pin_feedback(&pin_entry, false);
		} else if (!wanted && pin_entry.active) {
			nu54_pin_entry_stop(&pin_entry);
			leds(0);
		}
		if (pin_entry.active && nu54_pin_entry_poll(&pin_entry, k_uptime_get_32()) == NU54_PIN_TIMEOUT) {
			hand_over(NULL);
			continue;
		}
		/* Poll while the PIN is asked for, so a timeout or a closed session is noticed. */
		if (board_button_wait(&ev, wanted || pin_entry.active ? 250 : -1) != 0) {
			continue;
		}
		if (pin_entry.active) {
			pin_button(&pin_entry, &ev);
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
