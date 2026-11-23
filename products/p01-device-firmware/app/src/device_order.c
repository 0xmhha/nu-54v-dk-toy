#include "device_order.h"
#include "nu54_spending.h"

#include <psa/internal_trusted_storage.h>

#define ORDER_RECORD_UID ((psa_storage_uid_t)0x5550)
#define ORDER_SPENDING_UID ((psa_storage_uid_t)0x5561)
_Static_assert(sizeof(nu54_order_record) <= 128, "P01 order record exceeds ITS limit");

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
	return psa_its_set(ORDER_RECORD_UID + slot, sizeof(*record), record, PSA_STORAGE_FLAG_NONE) == PSA_SUCCESS ? 0 : -1;
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

bool device_order_open(uint64_t earliest, int64_t uptime_ms)
{
	return nu54_spend_order_open(load, NULL, earliest, uptime_ms);
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
