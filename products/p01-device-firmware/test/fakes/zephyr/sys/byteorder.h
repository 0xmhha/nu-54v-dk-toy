#ifndef TEST_SYS_BYTEORDER_H
#define TEST_SYS_BYTEORDER_H
#include <stdint.h>
static inline uint16_t sys_get_be16(const uint8_t *input)
{
	return (uint16_t)((input[0] << 8) | input[1]);
}
static inline uint64_t sys_get_be64(const uint8_t *input)
{
	uint64_t value = 0;
	for (unsigned i = 0; i < 8; i++) value = (value << 8) | input[i];
	return value;
}
static inline void sys_put_be64(uint64_t value, uint8_t *output)
{
	for (unsigned i = 8; i; i--) { output[i - 1] = value & 255; value >>= 8; }
}
#endif
