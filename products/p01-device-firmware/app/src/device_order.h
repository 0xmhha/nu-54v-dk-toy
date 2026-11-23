#ifndef DEVICE_ORDER_H
#define DEVICE_ORDER_H

#include "nu54_order.h"

/* Durable answers and the daily total of the spending profile. All callers hold the pilot lock. */
int device_order_find(uint64_t earliest, const uint8_t request[16], const uint8_t digest[32],
		      nu54_order_record *, unsigned *slot);
int device_order_store(unsigned slot, const nu54_order_record *);
/* Lets the board refuse an order over the daily total before it asks for SW1. */
bool device_order_within_daily(uint64_t earliest, uint64_t amount);
/* Commits the amount before the signature exists, so a power cut can only lower what is left. */
int device_order_charge(uint64_t earliest, uint64_t amount);

#endif
