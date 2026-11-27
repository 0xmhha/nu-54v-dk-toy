#ifndef DEVICE_ORDER_H
#define DEVICE_ORDER_H

#include "nu54_order.h"

/* Durable answers and the daily total of the spending profile. All callers hold the pilot lock. */
int device_order_find(uint64_t earliest, const uint8_t request[16], const uint8_t digest[32],
		      nu54_order_record *, unsigned *slot);
int device_order_store(unsigned slot, const nu54_order_record *);
/* Lets the board refuse an order over the daily total before it asks for SW1. */
bool device_order_within_daily(uint64_t earliest, uint64_t amount);
uint64_t device_order_daily_left(uint64_t earliest);
/* Commits the amount before the signature exists, so a power cut can only lower what is left. */
int device_order_charge(uint64_t earliest, uint64_t amount);

/* Call once at start, before the first order: no record was written in this start yet. */
void device_order_start(void);
/* True while an approved order could still execute. An order answered since this start is open
 * until the board's time passes its end. One from an earlier start is open for at most
 * NU54_SPEND_ORDER_LIFE_MS of running time, also under a grant that was kept through the restart. */
bool device_order_open(uint64_t earliest, int64_t uptime_ms);
/* Call with the time before the owner's phone moves the clock. The new time must not end an open
 * order for the board earlier than it ends for the token contract. What is open now therefore
 * stays open for as long as it could run at the most, counted in board running time: an order of
 * this start for NU54_ORDER_MAX_SECONDS from now, one of an earlier start until the board has run
 * for NU54_SPEND_ORDER_LIFE_MS. */
void device_order_hold_open(uint64_t earliest, int64_t uptime_ms);
/* What device_order_hold_open would hold, as running time from now in whole seconds, rounded up;
 * zero when nothing is open. The caller saves it with the moved time before it moves the clock,
 * so that a restart cannot lose it. */
uint64_t device_order_hold_seconds(uint64_t earliest, int64_t uptime_ms);
/* Call once at start, after device_order_start, with the hold the storage had: what the phone's
 * time must not close stays open for that much running time, and for no more than one order life
 * of it, as every order of an earlier start. */
void device_order_resume(int64_t uptime_ms, uint64_t hold_seconds);
int device_order_erase(void);

#endif
