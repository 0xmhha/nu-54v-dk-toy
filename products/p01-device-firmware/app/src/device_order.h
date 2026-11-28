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
/* True while an approved order could still execute. Any one of three keeps it open:
 * - the record of an order answered since this start, until the board's time passes its end;
 * - the record of one from an earlier start, until its end as well and for at most
 *   NU54_SPEND_ORDER_LIFE_MS (240 s) of running time, also under a grant that was kept through
 *   the restart;
 * - a hold, until the running time it names, whatever the board's time and the records say. A
 *   hold is set when the owner's phone moves the clock (device_order_hold_open), at each approval
 *   (device_order_hold_approved) and from the storage at start (device_order_resume). */
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
/* Call when an order is approved, before its record is stored. The record's end is the store's,
 * by the calendar, and the board's time may be ahead of the calendar since the owner's phone moved
 * it: by that time alone the order would end early for the board by as much as the clock is ahead.
 * So the order stays open for `seconds` of running time from now, whatever the clock says: the
 * time from the order's issue to its end, which the store signed and which is
 * NU54_ORDER_MAX_SECONDS at the most. This only adds to what the record says. The caller has saved
 * the same hold with the board's time (device_install_hold_order), so that a restart keeps it. */
void device_order_hold_approved(int64_t uptime_ms, uint64_t seconds);
/* Call once at start, after device_order_start, with the hold the storage had: what the phone's
 * time must not close stays open for that much running time, and for no more than one order life
 * of it, as every order of an earlier start. */
void device_order_resume(int64_t uptime_ms, uint64_t hold_seconds);
int device_order_erase(void);

#endif
