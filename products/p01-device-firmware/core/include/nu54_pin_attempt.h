#ifndef NU54_PIN_ATTEMPT_H
#define NU54_PIN_ATTEMPT_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define NU54_PIN_ATTEMPT_LIMIT 5

enum { NU54_PIN_OK, NU54_PIN_WRONG, NU54_PIN_LOCKED };

typedef struct {
	void *context;
	int (*read_count)(void *context, uint8_t *count);
	int (*write_count)(void *context, uint8_t count);
	bool (*verify)(void *context, const char *pin, size_t length);
} nu54_pin_attempt_storage;

bool nu54_pin_is_local_format(const char *pin, size_t length);
int nu54_pin_attempt(const nu54_pin_attempt_storage *storage, const char *pin, size_t length);

#endif
