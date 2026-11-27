#include "nu54_pin_attempt.h"
#include "nu54_pin_entry.h"

bool nu54_pin_is_local_format(const char *pin, size_t length)
{
	if (!pin || length != NU54_PIN_LEN) return false;
	for (size_t i = 0; i < length; ++i) {
		if (pin[i] < '0' || pin[i] > '9') return false;
	}
	return true;
}

int nu54_pin_attempt(const nu54_pin_attempt_storage *storage, const char *pin, size_t length)
{
	if (!nu54_pin_is_local_format(pin, length)) return NU54_PIN_WRONG;
	uint8_t count;
	if (storage->read_count(storage->context, &count) != 0 || count >= NU54_PIN_ATTEMPT_LIMIT) return NU54_PIN_LOCKED;
	/* The attempt is durable before a comparison can yield success or a power loss can erase it. */
	if (storage->write_count(storage->context, ++count) != 0) return NU54_PIN_LOCKED;
	if (!storage->verify(storage->context, pin, length)) {
		return count >= NU54_PIN_ATTEMPT_LIMIT ? NU54_PIN_LOCKED : NU54_PIN_WRONG;
	}
	return storage->write_count(storage->context, 0) == 0 ? NU54_PIN_OK : NU54_PIN_LOCKED;
}

uint32_t nu54_pin_wait_seconds(uint8_t failures)
{
	static const uint32_t waits[] = {60, 300, 900};
	if (failures < NU54_PIN_ATTEMPT_LIMIT) return 0;
	unsigned step = failures - NU54_PIN_ATTEMPT_LIMIT;
	return step < sizeof(waits) / sizeof(waits[0]) ? waits[step] : 3600;
}

int nu54_pin_attempt_waited(const nu54_pin_attempt_storage *storage, const char *pin, size_t length)
{
	if (!nu54_pin_is_local_format(pin, length)) return NU54_PIN_WRONG;
	uint8_t count;
	if (storage->read_count(storage->context, &count) != 0) return NU54_PIN_LOCKED;
	if (count < UINT8_MAX) ++count;
	if (storage->write_count(storage->context, count) != 0) return NU54_PIN_LOCKED;
	if (!storage->verify(storage->context, pin, length)) return NU54_PIN_WRONG;
	return storage->write_count(storage->context, 0) == 0 ? NU54_PIN_OK : NU54_PIN_LOCKED;
}
