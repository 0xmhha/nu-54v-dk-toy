#ifndef NU54_SPENDING_FAKE_KERNEL_H
#define NU54_SPENDING_FAKE_KERNEL_H
#include <stdint.h>
int64_t k_uptime_get(void);
static inline uint32_t k_uptime_get_32(void) { return (uint32_t)k_uptime_get(); }
#endif
