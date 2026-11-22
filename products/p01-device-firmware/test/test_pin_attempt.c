#include "nu54_pin_attempt.h"

#include <assert.h>
#include <string.h>

struct disk {
	uint8_t failures;
	unsigned compares, fail_write;
	bool read_error, accept;
};

static int read_counter(void *context, uint8_t *failures)
{
	struct disk *disk = context;
	*failures = disk->failures;
	return disk->read_error ? -1 : 0;
}

static int write_counter(void *context, uint8_t failures)
{
	struct disk *disk = context;
	if (disk->fail_write == failures + 1u) return -1;
	disk->failures = failures;
	return 0;
}

static bool verify(void *context, const char *pin, size_t length)
{
	struct disk *disk = context;
	assert(disk->failures > 0 && length == 4 && pin != NULL);
	disk->compares++;
	return disk->accept;
}

int main(void)
{
	struct disk disk = {0};
	nu54_pin_attempt_storage io = {&disk, read_counter, write_counter, verify};
	assert(nu54_pin_attempt(&io, "12a3", 4) == NU54_PIN_WRONG && disk.compares == 0);
	disk.fail_write = 2;
	assert(nu54_pin_attempt(&io, "1234", 4) == NU54_PIN_LOCKED && disk.compares == 0);
	disk.fail_write = 0;
	assert(nu54_pin_attempt(&io, "1234", 4) == NU54_PIN_WRONG && disk.failures == 1);
	disk.accept = true;
	disk.fail_write = 1;
	assert(nu54_pin_attempt(&io, "1234", 4) == NU54_PIN_LOCKED && disk.failures == 2);
	disk.fail_write = 0;
	assert(nu54_pin_attempt(&io, "1234", 4) == NU54_PIN_OK && disk.failures == 0);
	disk.failures = NU54_PIN_ATTEMPT_LIMIT;
	unsigned compares = disk.compares;
	assert(nu54_pin_attempt(&io, "1234", 4) == NU54_PIN_LOCKED && disk.compares == compares);
	disk.read_error = true;
	assert(nu54_pin_attempt(&io, "1234", 4) == NU54_PIN_LOCKED && disk.compares == compares);
	return 0;
}
