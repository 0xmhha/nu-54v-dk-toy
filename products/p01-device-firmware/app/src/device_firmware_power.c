#include "device_firmware_power.h"

#include <zephyr/devicetree.h>

#if defined(CONFIG_NU54_EXPERIMENTAL_PMIC_VIN_READ) && DT_NODE_HAS_STATUS_OKAY(DT_NODELABEL(i2c21))
#include <zephyr/drivers/gpio.h>
#include <zephyr/drivers/i2c.h>

#define BQ25186_ADDRESS 0x6a

static const struct device *const pmic = DEVICE_DT_GET(DT_NODELABEL(i2c21));
static const struct gpio_dt_spec pg = GPIO_DT_SPEC_GET(DT_PATH(zephyr_user), pmic_pg_gpios);

static int read_register(void *context, uint8_t reg, uint8_t *value)
{
	(void)context;
	return i2c_reg_read_byte(pmic, BQ25186_ADDRESS, reg, value);
}

static int read_pg(void *context)
{
	(void)context;
	return gpio_pin_get_dt(&pg);
}

enum nu54_firmware_vin device_firmware_vin(void)
{
	if (!device_is_ready(pmic) || !gpio_is_ready_dt(&pg) || gpio_pin_configure_dt(&pg, GPIO_INPUT))
		return NU54_FIRMWARE_VIN_UNKNOWN;
	const nu54_firmware_power_source source = {
		.read_register = read_register,
		.read_pg_asserted = read_pg,
		.qualified = true,
	};
	return nu54_firmware_observe_vin(&source);
}
#else
enum nu54_firmware_vin device_firmware_vin(void)
{
	return NU54_FIRMWARE_VIN_UNKNOWN;
}
#endif
