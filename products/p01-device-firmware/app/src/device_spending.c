#include "device_spending.h"

#include <psa/crypto.h>
#include <psa/internal_trusted_storage.h>
#include <string.h>

#include "device_order.h"
#include "device_recovery_pin.h"
#include "device_vault.h"
#include "nu54_sig.h"

#define SPEND_KEY ((psa_key_id_t)0x54b2)
#define SPEND_ALG PSA_ALG_DETERMINISTIC_ECDSA(PSA_ALG_SHA_256)
#define SPEND_RESET_UID ((psa_storage_uid_t)0x5570)
#define SPEND_ADDRESS_UID ((psa_storage_uid_t)0x5571)
#define SPEND_RETURN_UID ((psa_storage_uid_t)0x5572)
#define SPEND_RESET_MARK UINT32_C(0x4e355257)

static uint8_t address[20];
static bool ready;  /* the stored state was read and is consistent */
static bool wallet; /* the key exists and `address` is its address */

static bool key_missing(psa_status_t status)
{
	return status == PSA_ERROR_DOES_NOT_EXIST || status == PSA_ERROR_INVALID_HANDLE;
}

static bool gone(psa_status_t status)
{
	return status == PSA_SUCCESS || status == PSA_ERROR_DOES_NOT_EXIST || status == PSA_ERROR_INVALID_HANDLE;
}

/* 1 absent, 0 read, -1 failure. A failure is never treated as absence. */
static int load(psa_storage_uid_t uid, void *data, size_t size)
{
	size_t used;
	psa_status_t status = psa_its_get(uid, 0, size, data, &used);
	if (status == PSA_ERROR_DOES_NOT_EXIST) return 1;
	return status == PSA_SUCCESS && used == size ? 0 : -1;
}

static int store(psa_storage_uid_t uid, const void *data, size_t size)
{
	return psa_its_set(uid, size, data, PSA_STORAGE_FLAG_NONE) == PSA_SUCCESS ? 0 : -1;
}

/* 1 absent, 0 opened, -1 failure or a key with another policy. */
static int open_key(uint8_t out[20])
{
	psa_key_attributes_t attrs = PSA_KEY_ATTRIBUTES_INIT;
	psa_status_t status = psa_get_key_attributes(SPEND_KEY, &attrs);
	bool expected = psa_get_key_lifetime(&attrs) == PSA_KEY_LIFETIME_PERSISTENT &&
			psa_get_key_type(&attrs) == PSA_KEY_TYPE_ECC_KEY_PAIR(PSA_ECC_FAMILY_SECP_K1) &&
			psa_get_key_bits(&attrs) == 256 && psa_get_key_algorithm(&attrs) == SPEND_ALG &&
			!(psa_get_key_usage_flags(&attrs) & (PSA_KEY_USAGE_EXPORT | PSA_KEY_USAGE_COPY));
	psa_reset_key_attributes(&attrs);
	if (key_missing(status)) return 1;
	uint8_t point[65];
	size_t used;
	if (status != PSA_SUCCESS || !expected ||
	    psa_export_public_key(SPEND_KEY, point, sizeof(point), &used) != PSA_SUCCESS || used != sizeof(point) ||
	    point[0] != 4) return -1;
	nu54_address_of_pubkey(point, out);
	return 0;
}

static int create_key(void)
{
	psa_key_attributes_t attrs = PSA_KEY_ATTRIBUTES_INIT;
	psa_key_id_t created;
	psa_set_key_id(&attrs, SPEND_KEY);
	psa_set_key_lifetime(&attrs, PSA_KEY_LIFETIME_PERSISTENT);
	psa_set_key_type(&attrs, PSA_KEY_TYPE_ECC_KEY_PAIR(PSA_ECC_FAMILY_SECP_K1));
	psa_set_key_bits(&attrs, 256);
	psa_set_key_usage_flags(&attrs, PSA_KEY_USAGE_SIGN_HASH); /* never exported */
	psa_set_key_algorithm(&attrs, SPEND_ALG);
	psa_status_t status = psa_generate_key(&attrs, &created);
	psa_reset_key_attributes(&attrs);
	return status == PSA_SUCCESS ? 0 : -1;
}

/* Asks the key store itself. The answer of the destroy call alone is not taken as proof. */
static bool key_absent(void)
{
	psa_key_attributes_t attrs = PSA_KEY_ATTRIBUTES_INIT;
	psa_status_t status = psa_get_key_attributes(SPEND_KEY, &attrs);
	psa_reset_key_attributes(&attrs);
	return key_missing(status);
}

static bool owner_absent(const nu54_vault *vault)
{
	return !vault->record.generation || (vault->record.flags & NU54_VAULT_RELEASED);
}

static void forget_wallet(void)
{
	wallet = false;
	memset(address, 0, sizeof(address));
}

/* The key and its recorded address, with the proof that the key is gone. */
static int remove_wallet(void)
{
	forget_wallet();
	return gone(psa_destroy_key(SPEND_KEY)) && key_absent() && gone(psa_its_remove(SPEND_ADDRESS_UID)) ? 0 : -1;
}

int device_spending_erase(void)
{
	const uint32_t mark = SPEND_RESET_MARK;
	ready = false;
	forget_wallet();
	if (store(SPEND_RESET_UID, &mark, sizeof(mark)) || !gone(psa_destroy_key(SPEND_KEY)) ||
	    device_recovery_pin_erase() || device_order_erase() || !gone(psa_its_remove(SPEND_RETURN_UID)) ||
	    !gone(psa_its_remove(SPEND_ADDRESS_UID)) || nu54_vault_release(device_vault_owner())) return -1;
	/* The mark goes only after the key store says there is no key. Until then the reset is not
	 * done: it reports a failure, and the next start comes back here before anything else. */
	if (!key_absent()) return -1;
	return gone(psa_its_remove(SPEND_RESET_UID)) ? 0 : -1;
}

int device_spending_init(void)
{
	uint32_t mark;
	uint8_t known[20];
	const nu54_vault *vault = device_vault_owner();
	ready = false;
	forget_wallet();
	int pending = load(SPEND_RESET_UID, &mark, sizeof(mark));
	if (pending < 0 || (!pending && (mark != SPEND_RESET_MARK || device_spending_erase())) || !vault->enabled) return -1;
	if (owner_absent(vault)) {
		/* No owner, no wallet. A key found here was made for a registration that lost power before
		 * the owner was stored, or by firmware that made the key at start. */
		if (remove_wallet()) return -1;
		ready = true;
		return 0;
	}
	/* With an owner the key and its recorded address must be there. A missing key is a loss and is
	 * never replaced: the owner was given the address. */
	if (load(SPEND_ADDRESS_UID, known, sizeof(known)) || open_key(address) ||
	    memcmp(known, address, sizeof(address))) {
		forget_wallet();
		return -1;
	}
	ready = wallet = true;
	return 0;
}

int device_spending_enroll(void)
{
	const nu54_vault *vault = device_vault_owner();
	if (!ready || wallet || !vault->enabled || !owner_absent(vault)) return -1;
	/* What an attempt that did not complete left behind goes first. */
	if (remove_wallet() || create_key() || open_key(address) || store(SPEND_ADDRESS_UID, address, sizeof(address))) {
		forget_wallet();
		return -1;
	}
	wallet = true;
	return 0;
}

void device_spending_withdraw(void)
{
	const nu54_vault *vault = device_vault_owner();
	/* A vault that stopped after a failed write may or may not hold the owner now. Then the key
	 * stays, nothing is signed until the restart, and the restart decides from what is stored. */
	if (!vault->enabled || !owner_absent(vault)) {
		ready = false;
		forget_wallet();
	} else if (remove_wallet()) {
		ready = false;
	}
}

const uint8_t *device_spending_address(void)
{
	return ready && wallet ? address : NULL;
}

int device_spending_sign(const uint8_t digest[32], uint8_t signature[64])
{
	size_t used;
	return ready && wallet &&
	       psa_sign_hash(SPEND_KEY, SPEND_ALG, digest, 32, signature, 64, &used) == PSA_SUCCESS && used == 64 ? 0 : -1;
}

bool device_spending_return_address(uint8_t out[20])
{
	return ready && wallet && load(SPEND_RETURN_UID, out, 20) == 0;
}

int device_spending_set_return(const uint8_t target[20])
{
	uint8_t existing[20];
	if (!ready || !wallet || !memcmp(target, address, sizeof(address)) ||
	    load(SPEND_RETURN_UID, existing, sizeof(existing)) != 1) return -1;
	return store(SPEND_RETURN_UID, target, 20);
}
