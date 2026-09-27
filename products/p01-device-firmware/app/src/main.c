/*
 * P01 payment signer firmware skeleton (WBS2-P01-01).
 *
 * Prints the protocol version it was built against and blinks led0 as a
 * heartbeat. Key handling, BLE and signing are added by later WBS tasks.
 */
#include <zephyr/kernel.h>
#include <zephyr/drivers/gpio.h>
#include <zephyr/logging/log.h>

#include "nu54_protocol.h"

LOG_MODULE_REGISTER(nu54_signer, LOG_LEVEL_INF);

#define HEARTBEAT_MS 500

static const struct gpio_dt_spec heartbeat = GPIO_DT_SPEC_GET(DT_ALIAS(led0), gpios);

int main(void)
{
	LOG_INF("nu54 signer skeleton, protocol v%d, %d reason codes", NU54_PROTOCOL_VERSION, NU54_REASON_COUNT);

	if (!gpio_is_ready_dt(&heartbeat) || gpio_pin_configure_dt(&heartbeat, GPIO_OUTPUT_INACTIVE) != 0) {
		LOG_ERR("heartbeat LED not ready");
		return 0;
	}

	while (true) {
		gpio_pin_toggle_dt(&heartbeat);
		k_msleep(HEARTBEAT_MS);
	}
	return 0;
}
