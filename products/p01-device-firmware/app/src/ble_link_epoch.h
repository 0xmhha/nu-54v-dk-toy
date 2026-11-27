#ifndef BLE_LINK_EPOCH_H
#define BLE_LINK_EPOCH_H

#include <stdbool.h>
#include <stdint.h>

/* The caller holds the slot lock while checking a captured connection epoch. */
static inline bool ble_link_epoch_matches(uint64_t current, uint64_t expected)
{
	return current != 0 && current == expected;
}

#endif
