/*
 * GPIO handling for LEDs and buttons.
 *
 * Buttons: press interrupt -> 30 ms debounce work -> while held, a 50 ms poll
 * reads the level, detects the release and the long press through nu54_button
 * (core/) -> event queue read by the application thread. The release is polled
 * rather than taken from a both-edge interrupt: on this SoC the GPIO driver
 * emulates both-edge with SENSE toggling, and in testing the release callback
 * did not arrive reliably.
 * The buttons sit on two GPIO ports (SW4 is P0.04, the others P1.x), so the
 * interrupt handler matches both the port and the pin.
 */
#include "board_io.h"

#include <errno.h>
#include <zephyr/drivers/gpio.h>
#include <zephyr/kernel.h>
#include <zephyr/logging/log.h>
#include <zephyr/sys/atomic.h>

LOG_MODULE_REGISTER(board_io, LOG_LEVEL_INF);

#define DEBOUNCE_MS 30
#define HOLD_POLL_MS 50

static const struct gpio_dt_spec leds[BOARD_IO_COUNT] = {
	GPIO_DT_SPEC_GET(DT_ALIAS(led0), gpios),
	GPIO_DT_SPEC_GET(DT_ALIAS(led1), gpios),
	GPIO_DT_SPEC_GET(DT_ALIAS(led2), gpios),
	GPIO_DT_SPEC_GET(DT_ALIAS(led3), gpios),
};

static const struct gpio_dt_spec buttons[BOARD_IO_COUNT] = {
	GPIO_DT_SPEC_GET(DT_ALIAS(sw0), gpios),
	GPIO_DT_SPEC_GET(DT_ALIAS(sw1), gpios),
	GPIO_DT_SPEC_GET(DT_ALIAS(sw2), gpios),
	GPIO_DT_SPEC_GET(DT_ALIAS(sw3), gpios),
};

static struct gpio_callback button_cb[BOARD_IO_COUNT];
static struct k_work_delayable debounce[BOARD_IO_COUNT];
static struct k_work_delayable hold_poll;
static nu54_button_t state[BOARD_IO_COUNT];
static atomic_t debounce_pending;

K_MSGQ_DEFINE(button_events, sizeof(board_button_event_t), 8, 4);

static void post(uint8_t i, nu54_button_event_t ev)
{
	if (ev == NU54_BTN_NONE) {
		return;
	}
	board_button_event_t msg = {.button = i, .event = ev};
	if (k_msgq_put(&button_events, &msg, K_NO_WAIT) != 0) {
		LOG_WRN("button event dropped (queue full)");
	}
}

static void hold_poll_fn(struct k_work *work)
{
	ARG_UNUSED(work);
	bool any_held = false;
	uint32_t now = k_uptime_get_32();

	for (uint8_t i = 0; i < BOARD_IO_COUNT; i++) {
		if (!state[i].pressed) {
			continue;
		}
		if (gpio_pin_get_dt(&buttons[i]) > 0) {
			post(i, nu54_button_tick(&state[i], now));
			any_held = true;
		} else {
			post(i, nu54_button_update(&state[i], false, now));
		}
	}
	if (any_held) {
		k_work_schedule(&hold_poll, K_MSEC(HOLD_POLL_MS));
	}
}

static void debounce_fn(struct k_work *work)
{
	struct k_work_delayable *dw = k_work_delayable_from_work(work);

	for (uint8_t i = 0; i < BOARD_IO_COUNT; i++) {
		if (dw != &debounce[i]) {
			continue;
		}
		atomic_clear_bit(&debounce_pending, i);
		/* Count the press only if the button is still down after the debounce. */
		if (gpio_pin_get_dt(&buttons[i]) > 0 && !state[i].pressed) {
			nu54_button_update(&state[i], true, k_uptime_get_32());
			k_work_schedule(&hold_poll, K_MSEC(HOLD_POLL_MS));
		}
		return;
	}
}

static void button_isr(const struct device *port, struct gpio_callback *cb, uint32_t pins)
{
	ARG_UNUSED(cb);
	for (uint8_t i = 0; i < BOARD_IO_COUNT; i++) {
		if (buttons[i].port != port || !(pins & BIT(buttons[i].pin))) {
			continue;
		}
		/* One debounce per burst of edges: chatter does not count twice. */
		if (!atomic_test_and_set_bit(&debounce_pending, i)) {
			k_work_schedule(&debounce[i], K_MSEC(DEBOUNCE_MS));
		}
	}
}

int board_io_init(void)
{
	k_work_init_delayable(&hold_poll, hold_poll_fn);

	for (uint8_t i = 0; i < BOARD_IO_COUNT; i++) {
		if (!gpio_is_ready_dt(&leds[i]) || !gpio_is_ready_dt(&buttons[i])) {
			LOG_ERR("GPIO for LED%u or SW%u not ready", i + 1, i + 1);
			return -ENODEV;
		}
		int err = gpio_pin_configure_dt(&leds[i], GPIO_OUTPUT_INACTIVE);
		err = err ? err : gpio_pin_configure_dt(&buttons[i], GPIO_INPUT);
		err = err ? err : gpio_pin_interrupt_configure_dt(&buttons[i], GPIO_INT_EDGE_TO_ACTIVE);
		if (err) {
			LOG_ERR("SW%u/LED%u configure failed (%d)", i + 1, i + 1, err);
			return err;
		}
		nu54_button_reset(&state[i]);
		k_work_init_delayable(&debounce[i], debounce_fn);
		gpio_init_callback(&button_cb[i], button_isr, BIT(buttons[i].pin));
		gpio_add_callback(buttons[i].port, &button_cb[i]);
		LOG_INF("LED%u %s pin %u, SW%u %s pin %u", i + 1, leds[i].port->name, leds[i].pin, i + 1,
			buttons[i].port->name, buttons[i].pin);
	}
	return 0;
}

void board_led_set(uint8_t led, bool on)
{
	if (led < BOARD_IO_COUNT) {
		gpio_pin_set_dt(&leds[led], on);
	}
}

void board_led_toggle(uint8_t led)
{
	if (led < BOARD_IO_COUNT) {
		gpio_pin_toggle_dt(&leds[led]);
	}
}

int board_button_wait(board_button_event_t *out, int32_t timeout_ms)
{
	return k_msgq_get(&button_events, out, timeout_ms < 0 ? K_FOREVER : K_MSEC(timeout_ms));
}
