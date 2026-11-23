#include "device_recovery_pin.h"
#include "board_io.h"
#include "nu54_pin_attempt.h"
#include "nu54_pin_entry.h"
#include "status_led.h"

#include <psa/crypto.h>
#include <psa/internal_trusted_storage.h>
#include <string.h>
#include <zephyr/kernel.h>

#define PIN_UID ((psa_storage_uid_t)0x5472)
#define PIN_KEY ((psa_key_id_t)0x54a0)
#define PIN_ALG PSA_ALG_HMAC(PSA_ALG_SHA_256)

struct pin_record {uint8_t expected[32], count;};

static void erase(void *data, size_t size)
{
	volatile uint8_t *bytes = data;
	while (size--) *bytes++ = 0;
}

static int entry(char pin[NU54_PIN_LEN + 1], device_pin_guard guard, void *context)
{
	nu54_pin_entry_t local;
	nu54_pin_entry_start(&local, k_uptime_get_32());
	status_led_pin(STATUS_PIN_START);
	int rc = -1;
	while ((!guard || guard(context, false, 0)) && nu54_pin_entry_poll(&local, k_uptime_get_32()) != NU54_PIN_TIMEOUT) {
		board_button_event_t button;
		if (board_button_wait(&button, 100) || button.event != NU54_BTN_CLICK || button.button > 2) continue;
		if (guard && !guard(context, true, button.prompt)) break;
		static const nu54_pin_event_t events[] = {NU54_PIN_TAP, NU54_PIN_CLEAR, NU54_PIN_NEXT};
		if (nu54_pin_entry_event(&local, events[button.button], k_uptime_get_32(), pin) == NU54_PIN_DONE) {
			rc = 0;
			break;
		}
		status_led_pin(button.button == 0 ? STATUS_PIN_TAP : STATUS_PIN_KEEP);
	}
	nu54_pin_entry_stop(&local);
	status_led_pin_end();
	return rc;
}

static int read_count(void *context, uint8_t *count)
{
	*count = ((struct pin_record *)context)->count;
	return 0;
}

static int write_count(void *context, uint8_t count)
{
	struct pin_record next = *(struct pin_record *)context;
	next.count = count;
	if (psa_its_set(PIN_UID, sizeof(next), &next, PSA_STORAGE_FLAG_NONE) != PSA_SUCCESS) return -1;
	*(struct pin_record *)context = next;
	return 0;
}

static bool verify(void *context, const char *pin, size_t length)
{
	const struct pin_record *record = context;
	return psa_mac_verify(PIN_KEY, PIN_ALG, (const uint8_t *)pin, length,
			      record->expected, sizeof(record->expected)) == PSA_SUCCESS;
}

static int enroll(const char *first, struct pin_record *record, device_pin_guard guard, void *context)
{
	char confirmation[NU54_PIN_LEN + 1];
	int result = entry(confirmation, guard, context);
	if (guard && !guard(context, false, 0)) result = -1;
	if (!result && memcmp(first, confirmation, NU54_PIN_LEN)) result = -1;
	erase(confirmation, sizeof(confirmation));
	if (result) return result;
	psa_key_attributes_t attrs = PSA_KEY_ATTRIBUTES_INIT;
	psa_set_key_id(&attrs, PIN_KEY);
	psa_set_key_lifetime(&attrs, PSA_KEY_LIFETIME_PERSISTENT);
	psa_set_key_type(&attrs, PSA_KEY_TYPE_HMAC);
	psa_set_key_bits(&attrs, 256);
	psa_set_key_algorithm(&attrs, PIN_ALG);
	psa_set_key_usage_flags(&attrs, PSA_KEY_USAGE_SIGN_MESSAGE | PSA_KEY_USAGE_VERIFY_MESSAGE);
	psa_key_id_t handle;
	psa_status_t state = psa_generate_key(&attrs, &handle);
	psa_reset_key_attributes(&attrs);
	if (state == PSA_ERROR_ALREADY_EXISTS) {
		handle = PIN_KEY;
		if (psa_get_key_attributes(handle, &attrs) != PSA_SUCCESS) return -1;
		bool matches = psa_get_key_type(&attrs) == PSA_KEY_TYPE_HMAC && psa_get_key_bits(&attrs) == 256 &&
			       psa_get_key_algorithm(&attrs) == PIN_ALG &&
			       psa_get_key_usage_flags(&attrs) == (PSA_KEY_USAGE_SIGN_MESSAGE | PSA_KEY_USAGE_VERIFY_MESSAGE);
		psa_reset_key_attributes(&attrs);
		if (!matches) return -1;
	} else if (state != PSA_SUCCESS) return -1;
	size_t used;
	if (psa_mac_compute(handle, PIN_ALG, (const uint8_t *)first, NU54_PIN_LEN, record->expected,
			    sizeof(record->expected), &used) != PSA_SUCCESS || used != sizeof(record->expected)) return -1;
	return write_count(record, 0);
}

#if defined(CONFIG_NU54_SPENDING_PROFILE)
/* Counted from the last wrong PIN or from boot, so switching the power off does not shorten it. */
static int64_t wait_from;

static int32_t wait_left(uint8_t failures)
{
	uint32_t seconds = nu54_pin_wait_seconds(failures);
	if (!seconds) return 0;
	int64_t left = wait_from + (int64_t)seconds * 1000 - k_uptime_get();
	return left > 0 ? (int32_t)((left + 999) / 1000) : 0;
}

int32_t device_recovery_pin_wait(void)
{
	struct pin_record record;
	size_t length;
	psa_status_t state = psa_its_get(PIN_UID, 0, sizeof(record), &record, &length);
	if (state == PSA_ERROR_DOES_NOT_EXIST) return 0;
	return state == PSA_SUCCESS && length == sizeof(record) ? wait_left(record.count) : -1;
}

int device_recovery_pin_erase(void)
{
	psa_status_t key = psa_destroy_key(PIN_KEY), record = psa_its_remove(PIN_UID);
	wait_from = 0;
	return (key == PSA_SUCCESS || key == PSA_ERROR_DOES_NOT_EXIST || key == PSA_ERROR_INVALID_HANDLE) &&
	       (record == PSA_SUCCESS || record == PSA_ERROR_DOES_NOT_EXIST) ? 0 : -1;
}
#endif

int device_recovery_pin_guarded(bool virgin, device_pin_guard guard, void *context)
{
	struct pin_record record = {0};
	struct psa_storage_info_t info;
	psa_status_t state = psa_its_get_info(PIN_UID, &info);
	if (state == PSA_ERROR_DOES_NOT_EXIST ? !virgin : state != PSA_SUCCESS || info.size != sizeof(record))
		return -1;
	if (state == PSA_SUCCESS) {
		size_t length;
		if (psa_its_get(PIN_UID, 0, sizeof(record), &record, &length) != PSA_SUCCESS || length != sizeof(record) ||
#if defined(CONFIG_NU54_SPENDING_PROFILE)
		    false) return -1;
		if (wait_left(record.count)) {
			status_led_locked();
			return -1;
		}
#else
		    record.count >= NU54_PIN_ATTEMPT_LIMIT) return -1;
#endif
	}
	char pin[NU54_PIN_LEN + 1];
	int result = -1;
	if (!entry(pin, guard, context) && (!guard || guard(context, false, 0)) && nu54_pin_is_local_format(pin, NU54_PIN_LEN)) {
		const nu54_pin_attempt_storage storage = {&record, read_count, write_count, verify};
#if defined(CONFIG_NU54_SPENDING_PROFILE)
		if (state == PSA_ERROR_DOES_NOT_EXIST) {
			result = enroll(pin, &record, guard, context);
		} else {
			result = nu54_pin_attempt_waited(&storage, pin, NU54_PIN_LEN) == NU54_PIN_OK ? 0 : -1;
			if (result) wait_from = k_uptime_get();
		}
#else
		result = state == PSA_ERROR_DOES_NOT_EXIST ? enroll(pin, &record, guard, context) :
			 nu54_pin_attempt(&storage, pin, NU54_PIN_LEN) == NU54_PIN_OK ? 0 : -1;
#endif
	}
	erase(pin, sizeof(pin));
	return result;
}

int device_recovery_pin(bool virgin)
{
	return device_recovery_pin_guarded(virgin, NULL, NULL);
}
