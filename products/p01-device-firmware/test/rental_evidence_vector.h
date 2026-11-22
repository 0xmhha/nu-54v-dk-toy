#ifndef RENTAL_EVIDENCE_VECTOR_H
#define RENTAL_EVIDENCE_VECTOR_H

#include <assert.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>

static inline void rental_vector_decode_hex(const char *text, uint8_t *bytes, size_t size)
{
	assert(strlen(text) == 2 * size);
	for (size_t i = 0; i < size; ++i) {
		unsigned value;
		assert(sscanf(text + i * 2, "%2x", &value) == 1 && value <= 255);
		bytes[i] = (uint8_t)value;
	}
}

#endif
