#include "device_order.h"
#include "nu54_spending.h"

#include <psa/internal_trusted_storage.h>

#define ORDER_RECORD_UID ((psa_storage_uid_t)0x5550)
#define ORDER_SPENDING_UID ((psa_storage_uid_t)0x5561)
_Static_assert(sizeof(nu54_order_record) <= 128, "P01 order record exceeds ITS limit");
_Static_assert(NU54_ORDER_RECORDS < 32, "one bit per order record");

/* Which records were written since this start, and until when (board running time) an order stays
 * open whatever the clock says. */
static uint32_t answered_now;
static int64_t held_open_until;

static int load(void *context, unsigned slot, nu54_order_record *out)
{
	(void)context;
	size_t used;
	psa_status_t read = psa_its_get(ORDER_RECORD_UID + slot, 0, sizeof(*out), out, &used);
	if (read == PSA_ERROR_DOES_NOT_EXIST) return 1;
	return read == PSA_SUCCESS && used == sizeof(*out) ? 0 : -1;
}

int device_order_find(uint64_t earliest, const uint8_t request[16], const uint8_t digest[32],
		      nu54_order_record *result, unsigned *slot)
{
	return earliest ? nu54_order_find(load, NULL, earliest, request, digest, result, slot) : NU54_PILOT_RECEIPT_ERROR;
}

int device_order_store(unsigned slot, const nu54_order_record *record)
{
	if (!slot || slot > NU54_ORDER_RECORDS) return -1;
	/* Marked first: a write that fails may still have reached the storage. */
	answered_now |= UINT32_C(1) << slot;
	return psa_its_set(ORDER_RECORD_UID + slot, sizeof(*record), record, PSA_STORAGE_FLAG_NONE) == PSA_SUCCESS ? 0 : -1;
}

void device_order_start(void)
{
	answered_now = 0;
	held_open_until = 0;
}

static int load_of_this_start(void *context, unsigned slot, nu54_order_record *out)
{
	return answered_now & (UINT32_C(1) << slot) ? load(context, slot, out) : 1;
}

static int load_of_earlier_starts(void *context, unsigned slot, nu54_order_record *out)
{
	return answered_now & (UINT32_C(1) << slot) ? 1 : load(context, slot, out);
}

static int load_total(nu54_order_spending *saved)
{
	size_t used;
	*saved = (nu54_order_spending){0};
	psa_status_t read = psa_its_get(ORDER_SPENDING_UID, 0, sizeof(*saved), saved, &used);
	return read == PSA_ERROR_DOES_NOT_EXIST || (read == PSA_SUCCESS && used == sizeof(*saved)) ? 0 : -1;
}

static int next_total(uint64_t earliest, uint64_t amount, nu54_order_spending *next)
{
	nu54_order_spending saved;
	if (load_total(&saved)) return -1;
	return nu54_order_charge(&saved, earliest, amount, CONFIG_NU54_SPENDING_DAILY_CAP, next);
}

uint64_t device_order_daily_left(uint64_t earliest)
{
	nu54_order_spending saved;
	return load_total(&saved) ? 0 : nu54_order_daily_left(&saved, earliest, CONFIG_NU54_SPENDING_DAILY_CAP);
}

bool device_order_within_daily(uint64_t earliest, uint64_t amount)
{
	nu54_order_spending next;
	return next_total(earliest, amount, &next) == 0;
}

int device_order_charge(uint64_t earliest, uint64_t amount)
{
	nu54_order_spending next;
	if (next_total(earliest, amount, &next)) return -1;
	return psa_its_set(ORDER_SPENDING_UID, sizeof(next), &next, PSA_STORAGE_FLAG_NONE) == PSA_SUCCESS ? 0 : -1;
}

/* An order answered since this start is judged by the board's time, which ran with it. */
static bool open_of_this_start(uint64_t earliest, int64_t uptime_ms)
{
	return nu54_spend_order_open(load_of_this_start, NULL, earliest, uptime_ms);
}

/* One from before this start was signed at least as long ago as the board has run, and it could
 * execute for NU54_ORDER_MAX_SECONDS at most. The board's time does not tell how long ago that
 * was: it stood still without power and lost what passed since its last save. So such an order
 * counts as open only while the board has run for less than one order life. */
static bool open_of_earlier_starts(uint64_t earliest, int64_t uptime_ms)
{
	return uptime_ms < NU54_SPEND_ORDER_LIFE_MS && nu54_spend_order_open(load_of_earlier_starts, NULL, earliest, uptime_ms);
}

/* The running time until which what is open now stays open, whatever the clock says later. */
static int64_t hold_until(uint64_t earliest, int64_t uptime_ms)
{
	int64_t until = 0;
	if (open_of_this_start(earliest, uptime_ms)) until = uptime_ms + NU54_ORDER_MAX_SECONDS * INT64_C(1000);
	else if (open_of_earlier_starts(earliest, uptime_ms)) until = NU54_SPEND_ORDER_LIFE_MS;
	return until > held_open_until ? until : held_open_until;
}

void device_order_hold_open(uint64_t earliest, int64_t uptime_ms)
{
	held_open_until = hold_until(earliest, uptime_ms);
}

uint64_t device_order_hold_seconds(uint64_t earliest, int64_t uptime_ms)
{
	const int64_t until = hold_until(earliest, uptime_ms);
	return until > uptime_ms ? ((uint64_t)(until - uptime_ms) + 999u) / 1000u : 0;
}

void device_order_hold_approved(int64_t uptime_ms, uint64_t seconds)
{
	const int64_t until = uptime_ms + (int64_t)seconds * 1000;
	if (until > held_open_until) held_open_until = until;
}

void device_order_resume(int64_t uptime_ms, uint64_t hold_seconds)
{
	const int64_t life = NU54_SPEND_ORDER_LIFE_MS;
	if (!hold_seconds) return;
	int64_t until = hold_seconds >= (uint64_t)life / 1000u ? life : uptime_ms + (int64_t)hold_seconds * 1000;
	if (until > life) until = life;
	if (until > held_open_until) held_open_until = until;
}

bool device_order_open(uint64_t earliest, int64_t uptime_ms)
{
	return uptime_ms < held_open_until || open_of_this_start(earliest, uptime_ms) ||
	       open_of_earlier_starts(earliest, uptime_ms);
}

int device_order_erase(void)
{
	for (unsigned slot = 1; slot <= NU54_ORDER_RECORDS; ++slot) {
		psa_status_t removed = psa_its_remove(ORDER_RECORD_UID + slot);
		if (removed != PSA_SUCCESS && removed != PSA_ERROR_DOES_NOT_EXIST) return -1;
	}
	psa_status_t removed = psa_its_remove(ORDER_SPENDING_UID);
	return removed == PSA_SUCCESS || removed == PSA_ERROR_DOES_NOT_EXIST ? 0 : -1;
}
