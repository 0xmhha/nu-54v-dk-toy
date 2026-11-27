#include "device_rental_journal_ps.h"
#include "nu54_keccak.h"

#include <psa/protected_storage.h>
#include <string.h>

#if !defined(CONFIG_TRUSTED_EXECUTION_NONSECURE) || !defined(CONFIG_TFM_PARTITION_PROTECTED_STORAGE) || !defined(CONFIG_TFM_PS_ROLLBACK_PROTECTION)
#error "Rental journal requires the rollback-protected TF-M PS API"
#endif

#define RENTAL_JOURNAL_UID ((psa_storage_uid_t)0x4e35344c00000005ULL)
#define RENTAL_ACTIVATION_UID ((psa_storage_uid_t)0x4e35344c00000006ULL)
#define RENTAL_RETIREMENT_UID ((psa_storage_uid_t)0x4e35344c00000007ULL)
#define RENTAL_HANDOFF_UID ((psa_storage_uid_t)0x4e35344c00000008ULL)

static const uint8_t activation_magic[] = {'R', 'A', '0', '1'};
enum activation_phase { PREPARED = 1, ACTIVE };
typedef struct {
	uint8_t magic[4];
	uint8_t phase;
	nu54_rental_binding_t binding;
	uint8_t first[32];
} rental_activation_t;

_Static_assert(sizeof(rental_activation_t) == 245, "RA01 activation layout must remain stable");
#if !defined(CONFIG_TFM_PS_MAX_ASSET_SIZE)
#error "A measured TF-M PS asset size is required for the rental journal"
#else
_Static_assert(sizeof(nu54_rental_journal_image_t) <= CONFIG_TFM_PS_MAX_ASSET_SIZE,
	       "rental journal exceeds the configured TF-M PS asset size");
#endif

__attribute__((weak)) bool device_rental_journal_platform_qualified(void)
{
	return false;
}

__attribute__((weak)) bool device_rental_journal_measurement(uint8_t measurement[32])
{
	(void)measurement;
	return false;
}

__attribute__((weak)) bool device_rental_collector_peer_allowed(const nu54_rental_binding_t *binding, const uint8_t peer[7])
{
	(void)binding;
	(void)peer;
	return false;
}

__attribute__((weak)) bool device_rental_retirement_platform_qualified(void)
{
	return false;
}

__attribute__((weak)) int device_rental_cleanup_station_allowed(void *context, const nu54_rental_binding_t *binding,
							      const uint8_t peer[7])
{
	(void)context;
	(void)binding;
	(void)peer;
	return 0;
}

__attribute__((weak)) int device_rental_cleanup_begin(void *context)
{
	(void)context;
	return -1;
}

__attribute__((weak)) void device_rental_cleanup_end(void *context)
{
	(void)context;
}

__attribute__((weak)) int device_rental_cleanup_time(void *context, uint64_t *now)
{
	(void)context;
	(void)now;
	return -1;
}

__attribute__((weak)) int device_rental_cleanup_witness(void *context, const nu54_rental_binding_t *binding,
						      const nu54_rental_cleanup_intent_t *intent,
						      nu54_rental_cleanup_witness_t *witness)
{
	(void)context;
	(void)binding;
	(void)intent;
	(void)witness;
	return -1;
}

__attribute__((weak)) bool device_rental_handoff_verify(const device_rental_handoff_t *handoff, uint64_t now)
{
	(void)handoff;
	(void)now;
	return false;
}

static bool running_measurement_matches(const nu54_rental_binding_t *binding)
{
	uint8_t actual[32];
	return device_rental_journal_platform_qualified() && device_rental_journal_measurement(actual) &&
	       !memcmp(actual, binding->measurement, sizeof(actual));
}

static int read_asset_flags(psa_storage_uid_t uid, void *image, size_t size, uint32_t flags)
{
	if (!device_rental_journal_platform_qualified()) return -1;
	struct psa_storage_info_t info;
	psa_status_t status = psa_ps_get_info(uid, &info);
	if (status == PSA_ERROR_DOES_NOT_EXIST) return 1;
	if (status != PSA_SUCCESS || info.size != size || info.flags != flags) return -1;

	size_t received;
	return psa_ps_get(uid, 0, size, image, &received) == PSA_SUCCESS && received == size ? 0 : -1;
}

static int read_asset(psa_storage_uid_t uid, void *image, size_t size)
{
	return read_asset_flags(uid, image, size, PSA_STORAGE_FLAG_NONE);
}

static int retirement_read(void *context, nu54_rental_retirement_fence_t *fence)
{
	(void)context;
	return read_asset_flags(RENTAL_RETIREMENT_UID, fence, sizeof(*fence), PSA_STORAGE_FLAG_WRITE_ONCE);
}

static int retirement_create(void *context, const nu54_rental_retirement_fence_t *fence)
{
	(void)context;
	if (!device_rental_retirement_platform_qualified()) return -1;
	nu54_rental_retirement_fence_t observed;
	int result = retirement_read(NULL, &observed);
	if (result != 1) return result == 0 ? 1 : -1;
	/* The caller serializes rental operations. WRITE_ONCE also forbids a replacement at PS. */
	return psa_ps_set(RENTAL_RETIREMENT_UID, sizeof(*fence), fence, PSA_STORAGE_FLAG_WRITE_ONCE) == PSA_SUCCESS ? 0 : -1;
}

typedef struct {
	uint8_t magic[4];
	device_rental_handoff_t handoff;
} rental_handoff_image_t;

_Static_assert(sizeof(rental_handoff_image_t) == 593, "RH01 layout must remain stable");
_Static_assert(sizeof(rental_handoff_image_t) <= CONFIG_TFM_PS_MAX_ASSET_SIZE,
	       "rental handoff exceeds the configured TF-M PS asset size");

static const uint8_t handoff_magic[4] = {'R', 'H', '0', '1'};

static int read_handoff(rental_handoff_image_t *image)
{
	int result = read_asset(RENTAL_HANDOFF_UID, image, sizeof(*image));
	if (result == 0 && memcmp(image->magic, handoff_magic, sizeof(image->magic))) return -1;
	return result;
}

static int handoff_state(void)
{
	rental_handoff_image_t image;
	return read_handoff(&image);
}

static int retirement_absent(void)
{
	nu54_rental_retirement_fence_t fence;
	return retirement_read(NULL, &fence) == 1;
}

static int write_asset(psa_storage_uid_t uid, const void *image, size_t size)
{
	if (!device_rental_journal_platform_qualified()) return -1;
	return psa_ps_set(uid, size, image, PSA_STORAGE_FLAG_NONE) == PSA_SUCCESS ? 0 : -1;
}

static int load(void *context, void *image, size_t size)
{
	(void)context;
	return read_asset(RENTAL_JOURNAL_UID, image, size);
}

static int replace(void *context, const void *image, size_t size)
{
	(void)context;
	return write_asset(RENTAL_JOURNAL_UID, image, size);
}

static int read_marker(rental_activation_t *marker)
{
	int result = read_asset(RENTAL_ACTIVATION_UID, marker, sizeof(*marker));
	if (result != 0) return result;
	if (memcmp(marker->magic, activation_magic, sizeof(activation_magic)) ||
	    (marker->phase != PREPARED && marker->phase != ACTIVE) ||
	    !nu54_rental_journal_binding_valid(&marker->binding) || marker->first[31] != 0) return -1;
	return 0;
}

static int marker_matches(const rental_activation_t *marker, const nu54_rental_binding_t *binding, const uint8_t first[32])
{
	return !memcmp(&marker->binding, binding, sizeof(*binding)) && !memcmp(marker->first, first, 32);
}

static const nu54_rental_journal_store_t store = {.read = load, .replace = replace};

int device_rental_journal_stage_ps(const nu54_rental_binding_t *binding, const uint8_t first[32])
{
	if (!nu54_rental_journal_binding_valid(binding) || first[31] != 0 || !running_measurement_matches(binding)) return -1;
	rental_activation_t marker;
	int result = read_marker(&marker);
	if (result == 0)
		return marker_matches(&marker, binding, first) ? 0 : -1;
	if (result != 1 || !retirement_absent() || handoff_state() != 1) return -1;
	/* A leftover or inaccessible journal cannot be delegated to a new generation. */
	struct psa_storage_info_t info;
	if (psa_ps_get_info(RENTAL_JOURNAL_UID, &info) != PSA_ERROR_DOES_NOT_EXIST) return -1;
	marker = (rental_activation_t){.phase = PREPARED, .binding = *binding};
	memcpy(marker.magic, activation_magic, sizeof(activation_magic));
	memcpy(marker.first, first, sizeof(marker.first));
	return write_asset(RENTAL_ACTIVATION_UID, &marker, sizeof(marker));
}

int device_rental_journal_marker_ps(nu54_rental_binding_t *binding, uint8_t first[32], bool *active)
{
	rental_activation_t marker;
	int result = read_marker(&marker);
	if (result == 1) {
		struct psa_storage_info_t info;
		return psa_ps_get_info(RENTAL_JOURNAL_UID, &info) == PSA_ERROR_DOES_NOT_EXIST && retirement_absent() && handoff_state() == 1 ? 1 : -1;
	}
	if (result != 0) return result;
	if (!running_measurement_matches(&marker.binding)) return -1;
	*binding = marker.binding;
	memcpy(first, marker.first, 32);
	*active = marker.phase == ACTIVE;
	return 0;
}

int device_rental_journal_activate_ps(nu54_rental_journal_t *journal, const nu54_rental_registration_t *registration,
				     const nu54_rental_registration_trust_t *trust, const uint8_t signature[65])
{
	rental_activation_t marker;
	if (!device_rental_journal_platform_qualified() || nu54_rental_registration_verify(registration, trust, signature) != 0 ||
	    read_marker(&marker) != 0 || !marker_matches(&marker, &trust->binding, trust->first) ||
	    !running_measurement_matches(&marker.binding) || !retirement_absent() || handoff_state() != 1)
		return NU54_RENTAL_JOURNAL_ERROR;
	if (nu54_rental_journal_open(journal, &store, &marker.binding, marker.first, marker.phase == PREPARED) != 0)
		return NU54_RENTAL_JOURNAL_ERROR;
	if (marker.phase == ACTIVE) return 0;
	marker.phase = ACTIVE;
	if (write_asset(RENTAL_ACTIVATION_UID, &marker, sizeof(marker)) != 0) {
		/* Resume must independently establish that both durable writes took effect. */
		journal->bound = journal->loaded = 0;
		return NU54_RENTAL_JOURNAL_ERROR;
	}
	return 0;
}

int device_rental_journal_resume_ps(nu54_rental_journal_t *journal, const nu54_rental_binding_t *binding,
				   const uint8_t first[32])
{
	rental_activation_t marker;
	if (read_marker(&marker) != 0 || marker.phase != ACTIVE || !marker_matches(&marker, binding, first) ||
	    !running_measurement_matches(&marker.binding))
		return NU54_RENTAL_JOURNAL_ERROR;
	nu54_rental_retirement_fence_t fence;
	int state = handoff_state();
	if (state == 1) state = retirement_read(NULL, &fence);
	if (state != 1) {
		/* A valid or unreadable retirement sidecar both prevent signing reactivation. */
		journal->bound = journal->loaded = 0;
		return state == 0 ? NU54_RENTAL_JOURNAL_FROZEN : NU54_RENTAL_JOURNAL_ERROR;
	}
	return nu54_rental_journal_open(journal, &store, &marker.binding, marker.first, 0);
}


static int deny_journal_write(void *context, const void *image, size_t size)
{
	(void)context;
	(void)image;
	(void)size;
	return -1;
}

int device_rental_journal_retirement_fence_ps(const nu54_rental_retirement_authority_t *authority,
					     uint64_t now)
{
	rental_activation_t marker;
	if (!device_rental_retirement_platform_qualified() || read_marker(&marker) != 0 ||
	    marker.phase != ACTIVE || !running_measurement_matches(&marker.binding))
		return NU54_RENTAL_JOURNAL_ERROR;
	int staged = handoff_state();
	if (staged != 1) {
		nu54_rental_retirement_fence_t historical;
		if (staged != 0 || retirement_read(NULL, &historical) != 0) return NU54_RENTAL_JOURNAL_ERROR;
	}

	/* Read a frozen journal independently of the boot/signing entry. Never burn a reservation here. */
	const nu54_rental_journal_store_t frozen_store = {.read = load, .replace = deny_journal_write};
	nu54_rental_journal_t journal = {0};
	if (nu54_rental_journal_open(&journal, &frozen_store, &marker.binding, marker.first, 0) != 0)
		return NU54_RENTAL_JOURNAL_ERROR;
	const nu54_rental_retirement_store_t retirement = {.read = retirement_read, .create = retirement_create};
	return nu54_rental_retirement_fence(&journal, &retirement, authority, now);
}


static int any_byte(const uint8_t bytes[32])
{
	uint8_t bits = 0;
	for (unsigned i = 0; i < 32; ++i) bits |= bytes[i];
	return bits != 0;
}

static int handoff_scope_valid(const device_rental_handoff_t *handoff)
{
	const nu54_rental_binding_t *prior = &handoff->prior, *next = &handoff->successor;
	return nu54_rental_journal_binding_valid(prior) && nu54_rental_journal_binding_valid(next) &&
		memcmp(next->generation, prior->generation, sizeof(next->generation)) > 0 &&
		!memcmp(prior->branch, next->branch, sizeof(next->branch)) &&
		!memcmp(prior->device, next->device, sizeof(next->device)) &&
		!memcmp(prior->deployment, next->deployment, sizeof(next->deployment)) &&
		!memcmp(prior->chain, next->chain, sizeof(next->chain)) &&
		!memcmp(prior->contract, next->contract, sizeof(next->contract)) &&
		memcmp(prior->registration, next->registration, sizeof(next->registration)) &&
		memcmp(prior->account, next->account, sizeof(next->account)) &&
		!memcmp(handoff->receipt, "RCR1", 4) && any_byte(handoff->server_ack);
}

static int prior_matches_initial(const device_rental_handoff_t *handoff)
{
	rental_activation_t marker;
	if (any_byte(handoff->reclaim_receipt) || read_marker(&marker) != 0 || marker.phase != ACTIVE ||
	    memcmp(&marker.binding, &handoff->prior, sizeof(marker.binding))) return 0;

	nu54_rental_retirement_fence_t fence;
	int result = retirement_read(NULL, &fence);
	if (result == 1) return 1;
	uint8_t binding[32];
	nu54_keccak256((const uint8_t *)&handoff->prior, sizeof(handoff->prior), binding);
	return result == 0 && !memcmp(fence.magic, "RCF1", 4) &&
		!memcmp(fence.binding, binding, sizeof(binding)) && !memcmp(fence.intent, handoff->receipt + 4, 32);
}

int device_rental_journal_stage_successor_ps(const device_rental_handoff_t *handoff, uint64_t now)
{
	if (!handoff || !now || !handoff_scope_valid(handoff) ||
	    !running_measurement_matches(&handoff->successor) ||
	    !device_rental_handoff_verify(handoff, now)) return NU54_RENTAL_JOURNAL_ERROR;

	rental_handoff_image_t current;
	int status = read_handoff(&current);
	if (status == 0) {
		if (!handoff_scope_valid(&current.handoff)) return NU54_RENTAL_JOURNAL_ERROR;
		if (!memcmp(handoff, &current.handoff, sizeof(*handoff))) return NU54_RENTAL_JOURNAL_REPLAY;
		uint8_t prior_receipt[32];
		nu54_keccak256(current.handoff.receipt, sizeof(current.handoff.receipt), prior_receipt);
		if (memcmp(&handoff->prior, &current.handoff.successor, sizeof(handoff->prior)) ||
		    memcmp(handoff->reclaim_receipt, prior_receipt, sizeof(prior_receipt)))
			return NU54_RENTAL_JOURNAL_CONFLICT;
	} else if (status != 1 || !prior_matches_initial(handoff)) {
		return NU54_RENTAL_JOURNAL_ERROR;
	}
	/* Only this guarded entry may replace the record. PSA set is one atomic object update. */
	rental_handoff_image_t next = {.handoff = *handoff};
	memcpy(next.magic, handoff_magic, sizeof(next.magic));
	return write_asset(RENTAL_HANDOFF_UID, &next, sizeof(next)) == 0 ?
		NU54_RENTAL_JOURNAL_RESERVED : NU54_RENTAL_JOURNAL_ERROR;
}
