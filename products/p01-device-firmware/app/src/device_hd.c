#include "nu54_commissioned_chain.h"
#include "device_hd.h"
#include "device_chain_keys.h"
#include "nu54_solana.h"

#include <zephyr/devicetree.h>

#if DT_NODE_HAS_STATUS(DT_ALIAS(nu54_recovery_uart), okay)
#include "device_recovery_pin.h"
#include "device_install.h"
#include "device_vault.h"
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
#include "device_owner_limits_ps.h"
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
#include "device_owner_finality_ps.h"
#endif
#endif
#include "status_led.h"
#include "board_io.h"
#include <zephyr/sys/atomic.h>

#include <psa/crypto.h>
#include <psa/internal_trusted_storage.h>
#include <cracen_psa_key_ids.h>
#include <cracen_psa_kmu.h>
#include <string.h>
#include <zephyr/sys/util.h>
#include <zephyr/kernel.h>
#include <zephyr/drivers/uart.h>
#include <zephyr/sys/ring_buffer.h>

#define HD_PROMPT UINT32_C(0xc0000000)
#define HD_METADATA ((psa_storage_uid_t)0x5470)
#define HD_SECRET ((psa_storage_uid_t)0x5471)
#define HD_WITNESS PSA_KEY_ID_FROM_CRACEN_KMU_SLOT(CRACEN_KMU_KEY_USAGE_SCHEME_PROTECTED, CONFIG_NU54_HD_KMU_SLOT)

#if defined(CONFIG_BUILD_WITH_TFM)
/* Nordic nRF54L TF-M ITS uses CRACEN MKEK-derived authenticated encryption.
 * The legacy Zephyr secure-storage Kconfig symbol does not exist in NS builds. */
BUILD_ASSERT(IS_ENABLED(CONFIG_TRUSTED_EXECUTION_NONSECURE) && IS_ENABLED(CONFIG_SOC_SERIES_NRF54L) &&
             IS_ENABLED(CONFIG_TFM_PARTITION_INTERNAL_TRUSTED_STORAGE) && IS_ENABLED(CONFIG_TFM_ITS_ENCRYPTED),
             "TF-M recovery storage requires encrypted nRF54L ITS");
#if DT_SAME_NODE(DT_ALIAS(nu54_recovery_uart), DT_NODELABEL(uart00))
BUILD_ASSERT(!IS_ENABLED(CONFIG_NRF_UARTE00_SECURE) && !IS_ENABLED(CONFIG_NRF_GPIO2_SECURE) &&
	     !DT_NODE_HAS_STATUS(DT_NODELABEL(spi00), okay),
	     "The experimental NS recovery UART must not share a secured or enabled peripheral");
#endif
#else
BUILD_ASSERT(CONFIG_SECURE_STORAGE_ITS_TRANSFORM_AEAD_KEY_PROVIDER_HUK_LIBRARY,
             "Recovery storage must be sealed to this board's HUK");
#endif

static void erase(void *data, size_t length)
{
	volatile uint8_t *bytes = data;
	while (length--) *bytes++ = 0;
}

static int stat_record(psa_storage_uid_t id, size_t size)
{
	struct psa_storage_info_t info;
	psa_status_t result = psa_its_get_info(id, &info);
	if (result == PSA_ERROR_DOES_NOT_EXIST) return 1;
	return result == PSA_SUCCESS && info.size == size ? 0 : -1;
}

static int read(psa_storage_uid_t id, void *output, size_t length)
{
	int state = stat_record(id, length);
	if (state) return state;
	size_t used;
	return psa_its_get(id, 0, length, output, &used) == PSA_SUCCESS && used == length ? 0 : -1;
}

static int witness_present(void)
{
	psa_key_attributes_t attributes = PSA_KEY_ATTRIBUTES_INIT;
	psa_status_t status = psa_get_key_attributes(HD_WITNESS, &attributes);
	if (status == PSA_ERROR_DOES_NOT_EXIST || status == PSA_ERROR_INVALID_HANDLE) return 1;
	if (status != PSA_SUCCESS) return -1;
	psa_key_lifetime_t lifetime = psa_get_key_lifetime(&attributes);
	bool correct = PSA_KEY_LIFETIME_GET_LOCATION(lifetime) == PSA_KEY_LOCATION_CRACEN_KMU &&
		       PSA_KEY_LIFETIME_GET_PERSISTENCE(lifetime) == CRACEN_KEY_PERSISTENCE_READ_ONLY &&
		       psa_get_key_type(&attributes) == PSA_KEY_TYPE_AES &&
		       psa_get_key_algorithm(&attributes) == PSA_ALG_CMAC &&
		       (psa_get_key_usage_flags(&attributes) & PSA_KEY_USAGE_VERIFY_MESSAGE) &&
		       !(psa_get_key_usage_flags(&attributes) &
			 ~(PSA_KEY_USAGE_VERIFY_MESSAGE | PSA_KEY_USAGE_VERIFY_HASH));
	psa_reset_key_attributes(&attributes);
	return correct ? 0 : -1;
}

static int witness_seed(const uint8_t seed[NU54_HD_SEED_SIZE], bool provision)
{
	static const uint8_t binding[] = "NU54 recovery seed commitment v1";
	uint8_t digest[32], tag[16];
	size_t used;
	int result = -1;
	if (psa_hash_compute(PSA_ALG_SHA_256, seed, NU54_HD_SEED_SIZE, digest, sizeof(digest), &used) != PSA_SUCCESS ||
	    used != sizeof(digest)) goto done;
	int existing = witness_present();
	psa_key_attributes_t attributes = PSA_KEY_ATTRIBUTES_INIT;
	psa_set_key_type(&attributes, PSA_KEY_TYPE_AES);
	psa_set_key_bits(&attributes, 128);
	psa_set_key_algorithm(&attributes, PSA_ALG_CMAC);
	psa_key_id_t handle;
	if (existing == 1 && provision) {
		psa_set_key_id(&attributes, HD_WITNESS);
		psa_set_key_lifetime(&attributes, PSA_KEY_LIFETIME_FROM_PERSISTENCE_AND_LOCATION(
			CRACEN_KEY_PERSISTENCE_READ_ONLY, PSA_KEY_LOCATION_CRACEN_KMU));
		psa_set_key_usage_flags(&attributes, PSA_KEY_USAGE_VERIFY_MESSAGE);
		if (psa_import_key(&attributes, digest, 16, &handle) == PSA_SUCCESS) existing = witness_present();
		psa_reset_key_attributes(&attributes);
		psa_set_key_type(&attributes, PSA_KEY_TYPE_AES);
		psa_set_key_bits(&attributes, 128);
		psa_set_key_algorithm(&attributes, PSA_ALG_CMAC);
	}
	if (existing == 0) {
		psa_set_key_usage_flags(&attributes, PSA_KEY_USAGE_SIGN_MESSAGE);
		if (psa_import_key(&attributes, digest, 16, &handle) == PSA_SUCCESS) {
			psa_status_t rc = psa_mac_compute(handle, PSA_ALG_CMAC, binding, sizeof(binding), tag, sizeof(tag), &used);
			int destroyed = psa_destroy_key(handle);
			if (rc == PSA_SUCCESS && used == sizeof(tag) && destroyed == PSA_SUCCESS &&
			    psa_mac_verify(HD_WITNESS, PSA_ALG_CMAC, binding, sizeof(binding), tag, sizeof(tag)) == PSA_SUCCESS)
				result = 0;
		}
	}
	psa_reset_key_attributes(&attributes);
done:
	erase(digest, sizeof(digest));
	erase(tag, sizeof(tag));
	return result;
}

static int read_record(void *context, nu54_hd_record *record)
{
	(void)context;
	int state = read(HD_METADATA, record, sizeof(*record));
	if (!state && (record->phase == NU54_HD_READY || record->phase == NU54_HD_BACKUP_REQUIRED) &&
	    witness_present()) return -1;
	return state;
}

static int save_record(void *context, const nu54_hd_record *record)
{
	(void)context;
	return psa_its_set(HD_METADATA, sizeof(*record), record, PSA_STORAGE_FLAG_NONE) == PSA_SUCCESS ? 0 : -1;
}

static int exists(void *context)
{
	(void)context;
	int state = stat_record(HD_SECRET, NU54_HD_SEED_SIZE);
	return state == 1 && witness_present() != 1 ? -1 : state;
}

static int seed_import(void *context, const uint8_t *seed, size_t size)
{
	if (size != NU54_HD_SEED_SIZE || exists(context) != 1) return -1;
	if (psa_its_set(HD_SECRET, size, seed, PSA_STORAGE_FLAG_WRITE_ONCE) != PSA_SUCCESS) return -1;
	return witness_seed(seed, true);
}

static int seed_generate(void *context)
{
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
	/* Only a genuinely empty HD slot may acquire the fresh-key marker. Imported
	 * or previously generated keys never enter this path. */
	const nu54_vault *vault=device_vault_owner();
	uint8_t board[32]; size_t used=0;
	if(exists(context)!=1 || !vault || !vault->enabled || !vault->record.generation ||
	   (vault->record.flags & NU54_VAULT_REVOKED) ||
	   psa_hash_compute(PSA_ALG_SHA_256,vault->record.identity,sizeof(vault->record.identity),board,sizeof(board),&used)!=PSA_SUCCESS ||
	   used!=sizeof(board) || device_owner_limits_mark_fresh_ps(board,vault->record.generation))return -1;
#endif
	uint8_t seed[NU54_HD_SEED_SIZE];
	int outcome = psa_generate_random(seed, sizeof(seed)) == PSA_SUCCESS ? seed_import(context, seed, sizeof(seed)) : -1;
	erase(seed, sizeof(seed));
	return outcome;
}

static int hmac_sha512(const uint8_t *key, size_t length, const uint8_t *data, size_t size, uint8_t digest[64])
{
	psa_key_attributes_t attr = PSA_KEY_ATTRIBUTES_INIT;
	psa_key_id_t handle;
	psa_set_key_type(&attr, PSA_KEY_TYPE_HMAC);
	psa_set_key_algorithm(&attr, PSA_ALG_HMAC(PSA_ALG_SHA_512));
	psa_set_key_usage_flags(&attr, PSA_KEY_USAGE_SIGN_MESSAGE);
	psa_status_t result = psa_import_key(&attr, key, length, &handle);
	psa_reset_key_attributes(&attr);
	if (result != PSA_SUCCESS) return -1;
	size_t used;
	result = psa_mac_compute(handle, PSA_ALG_HMAC(PSA_ALG_SHA_512), data, size, digest, 64, &used);
	int destroyed = psa_destroy_key(handle);
	return result == PSA_SUCCESS && used == 64 && destroyed == PSA_SUCCESS ? 0 : -1;
}

static int preview(void *context, const uint8_t *seed, size_t length, uint8_t key[65], uint8_t address[20])
{
	(void)context;
	uint8_t blind[32];
	int result = -1;
	if (length == NU54_HD_SEED_SIZE && psa_generate_random(blind, sizeof(blind)) == PSA_SUCCESS)
		result = nu54_hd_first_evm(seed, blind, hmac_sha512, key, address);
	erase(blind, sizeof(blind));
	return result;
}

static int derive(void *context, uint8_t key[65], uint8_t address[20])
{
	uint8_t seed[NU54_HD_SEED_SIZE];
	int result = read(HD_SECRET, seed, sizeof(seed));
	if (result == 0) result = witness_seed(seed, true);
	if (result == 0) result = preview(context, seed, sizeof(seed), key, address);
	erase(seed, sizeof(seed));
	return result;
}

static int device_hd_open(nu54_hd_slot *slot)
{
	static const nu54_hd_backend backend = {read_record, save_record, exists, seed_generate, preview, seed_import, derive, NULL};
	return nu54_hd_open(slot, &backend);
}


#define ACCESSORY_UART DT_ALIAS(nu54_recovery_uart)
BUILD_ASSERT(DT_NODE_HAS_STATUS(ACCESSORY_UART, okay), "Independent accessory UART alias is required");
BUILD_ASSERT(!DT_SAME_NODE(ACCESSORY_UART, DT_CHOSEN(zephyr_console)) &&
	     !DT_SAME_NODE(ACCESSORY_UART, DT_CHOSEN(zephyr_shell_uart)) &&
	     !DT_SAME_NODE(ACCESSORY_UART, DT_CHOSEN(zephyr_uart_mcumgr)) &&
	     !DT_SAME_NODE(ACCESSORY_UART, DT_NODELABEL(uart30)),
	     "Recovery must use separate physical port from diagnostic and management links");

static const struct device *const uart = DEVICE_DT_GET(ACCESSORY_UART);
static mbedtls_x509_crt accessory_ca;
static mbedtls_pk_context source_pin, target_pin, accessory_pin;
static uint8_t accessory_peer_pin[32];
static mbedtls_pk_context *local_key;
static device_tls_v2_profile private_profile;
static nu54_hd_slot hd;
static uint64_t (*trusted_clock)(void);
static bool enabled, running;
static enum nu54_recovery_role local_role;
static uint8_t operation;
static nu54_recovery_context context;
static uint8_t receipt[NU54_RECOVERY_ENVELOPE_MAX];
static size_t receipt_length;
static atomic_t overrun;
static uint32_t context_revision;
K_MUTEX_DEFINE(recovery_lock);
RING_BUF_DECLARE(input, 8192);

static int hash32(const uint8_t *bytes, size_t length, uint8_t out[32])
{
	size_t used;
	return psa_hash_compute(PSA_ALG_SHA_256, bytes, length, out, 32, &used) == PSA_SUCCESS && used == 32 ? 0 : -1;
}

static int proof_sign(void *unused, enum nu54_recovery_role role, const uint8_t *data, size_t length, uint8_t *out, size_t *used)
{
	(void)unused;
	uint8_t digest[32];
	if (role != local_role || hash32(data, length, digest)) return -1;
	return mbedtls_pk_sign(local_key, MBEDTLS_MD_SHA256, digest, sizeof(digest), out, NU54_RECOVERY_DER_MAX, used);
}

static int proof_verify(void *unused, enum nu54_recovery_role role, const uint8_t *data, size_t length,
			const uint8_t *signature, size_t used)
{
	(void)unused;
	uint8_t digest[32];
	mbedtls_pk_context *pin = role == NU54_RECOVERY_SOURCE ? &source_pin :
				  role == NU54_RECOVERY_TARGET ? &target_pin : &accessory_pin;
	return hash32(data, length, digest) ||
	       mbedtls_pk_verify(pin, MBEDTLS_MD_SHA256, digest, sizeof(digest), signature, used) ? -1 : 0;
}

static const nu54_recovery_signers signers = {proof_sign, proof_verify, NULL};

static void received(const struct device *dev, void *ignored)
{
	(void)ignored;
	uint8_t bytes[64];
	while (uart_irq_update(dev) && uart_irq_rx_ready(dev)) {
		int count = uart_fifo_read(dev, bytes, sizeof(bytes));
		if (count <= 0) break;
		if (ring_buf_put(&input, bytes, count) != (uint32_t)count) atomic_set(&overrun, 1);
	}
}

static int write_uart(void *unused, const unsigned char *data, size_t length)
{
	(void)unused;
	for (size_t i = 0; i < length; ++i) uart_poll_out(uart, data[i]);
	return (int)length;
}

static int read_uart(void *unused, unsigned char *data, size_t length)
{
	(void)unused;
	if (atomic_get(&overrun)) return MBEDTLS_ERR_SSL_INTERNAL_ERROR;
	size_t count = ring_buf_get(&input, data, length);
	return count ? (int)count : MBEDTLS_ERR_SSL_WANT_READ;
}

static int transfer(mbedtls_ssl_context *tls, uint8_t *data, size_t length, bool write, int64_t end)
{
	while (length && k_uptime_get() < end) {
		int result = write ? mbedtls_ssl_write(tls, data, length) : mbedtls_ssl_read(tls, data, length);
		if (result == MBEDTLS_ERR_SSL_WANT_READ || result == MBEDTLS_ERR_SSL_WANT_WRITE) {
			k_msleep(5);
			continue;
		}
		if (result <= 0) return -1;
		data += result;
		length -= (size_t)result;
	}
	return length ? -1 : 0;
}

static int packet(mbedtls_ssl_context *tls, uint8_t *data, size_t *length, size_t max, bool write, int64_t end)
{
	uint8_t size[2] = {(uint8_t)(*length >> 8), (uint8_t)*length};
	if (transfer(tls, size, sizeof(size), write, end)) return -1;
	if (!write) *length = ((size_t)size[0] << 8) | size[1];
	return *length && *length <= max ? transfer(tls, data, *length, write, end) : -1;
}

static int unlock_tls(mbedtls_ssl_context *tls, int64_t end)
{
	if (mbedtls_ssl_setup(tls, &private_profile.config)) return -1;
	mbedtls_ssl_set_bio(tls, NULL, write_uart, read_uart, NULL);
	while (k_uptime_get() < end) {
		int rc = mbedtls_ssl_handshake(tls);
		if (!rc) return device_tls_recovery_pin(tls, accessory_peer_pin, trusted_clock());
		if (rc != MBEDTLS_ERR_SSL_WANT_READ && rc != MBEDTLS_ERR_SSL_WANT_WRITE) return -1;
		k_msleep(5);
	}
	return -1;
}

void device_hd_clear(void)
{
	k_mutex_lock(&recovery_lock, K_FOREVER);
	if (!running) {
		enabled = false;
		operation = 0;
		receipt_length = 0;
		mbedtls_ssl_config_free(&private_profile.config);
		mbedtls_pk_free(&source_pin);
		mbedtls_pk_free(&target_pin);
		mbedtls_pk_free(&accessory_pin);
		mbedtls_x509_crt_free(&accessory_ca);
		local_key = NULL;
	}
	k_mutex_unlock(&recovery_lock);
}

int device_hd_configure(mbedtls_x509_crt *board, mbedtls_pk_context *board_key, uint64_t (*clock)(void),
			const uint8_t *trust, size_t length)
{
	const size_t spki_size = 91, fixed = 3 * spki_size + 2;
	if (length <= fixed || !board || board->pk_raw.len != spki_size || !device_is_ready(uart) ||
	    uart_irq_callback_user_data_set(uart, received, NULL)) return -1;
	const uint8_t *source = trust, *target = trust + spki_size, *accessory = trust + 2 * spki_size;
	size_t ca_size = ((size_t)trust[fixed - 2] << 8) | trust[fixed - 1];
	if (length != fixed + ca_size || !memcmp(source, target, spki_size) ||
	    !memcmp(source, accessory, spki_size) || !memcmp(target, accessory, spki_size)) return -1;
	enum nu54_recovery_role role;
	if (!memcmp(board->pk_raw.p, source, spki_size)) role = NU54_RECOVERY_SOURCE;
	else if (!memcmp(board->pk_raw.p, target, spki_size)) role = NU54_RECOVERY_TARGET;
	else return -1;

	k_mutex_lock(&recovery_lock, K_FOREVER);
	int rc = -1;
	if (running || operation || enabled || device_hd_open(&hd)) goto done;
	if (mbedtls_x509_crt_parse_der(&accessory_ca, trust + fixed, ca_size) ||
	    !device_tls_v2_certificate_current(&accessory_ca, clock()) ||
	    mbedtls_pk_parse_public_key(&source_pin, source, spki_size) ||
	    mbedtls_pk_parse_public_key(&target_pin, target, spki_size) ||
	    mbedtls_pk_parse_public_key(&accessory_pin, accessory, spki_size) ||
	    hash32(accessory, spki_size, accessory_peer_pin) || !device_install_independent_peer(accessory_peer_pin) ||
	    device_tls_recovery_configure(&private_profile, &accessory_ca, board, board_key)) goto done;
	local_role = role;
	local_key = board_key;
	trusted_clock = clock;
	enabled = true;
	rc = 0;
done:
	k_mutex_unlock(&recovery_lock);
	if (rc && !enabled) device_hd_clear();
	return rc;
}

static bool owner_current(const nu54_v2_link *link)
{
	const nu54_vault *vault = device_vault_owner();
	return enabled && vault->enabled && vault->record.generation &&
	       !(vault->record.flags & (NU54_VAULT_REVOKED | NU54_VAULT_PERSONAL)) &&
	       link->profile == NU54_V2_MANAGEMENT && link->peer == NU54_V2_OWNER &&
	       !memcmp(vault->record.owner, link->peer_identity, 32);
}

int device_hd_command(const nu54_v2_link *link, const nu54_v2_request *request, uint64_t now,
		      uint8_t output[1 + NU54_RECOVERY_ENVELOPE_MAX], size_t *length)
{
	if (request->payload_length != 1) return -1;
	uint8_t op = request->payload[0];
	output[0] = op;
	*length = 1;
	k_mutex_lock(&recovery_lock, K_FOREVER);
	if (!owner_current(link) || running || !hd.enabled) goto done;
	if (operation && now >= context.expires) {
		operation = 0;
		board_io_set_prompt(0);
		status_led_waiting(false);
	}
	if (op == 4) {
		if (receipt_length && now < context.expires && !memcmp(link->epoch, context.epoch, sizeof(context.epoch))) {
			memcpy(output + 1, receipt, receipt_length);
			*length += receipt_length;
		}
		goto done;
	}
	if (operation || context_revision == HD_PROMPT - 1 || request->expires <= now ||
	    (op == 5 && local_role != NU54_RECOVERY_TARGET) ||
	    (op == 5 && hd.record.phase != NU54_HD_EMPTY && hd.record.phase != NU54_HD_RESTORING &&
	     hd.record.phase != NU54_HD_READY) ||
	    (op == 6 && !nu54_hd_backup_eligible(&hd, local_role))) goto done;
	const nu54_vault_record *record = &device_vault_owner()->record;
	memset(&context, 0, sizeof(context));
	if (hash32(record->identity, sizeof(record->identity), context.target) ||
	    psa_generate_random(context.challenge, sizeof(context.challenge)) != PSA_SUCCESS) goto done;
	memcpy(context.request, request->id, sizeof(context.request));
	memcpy(context.owner, link->peer_identity, sizeof(context.owner));
	memcpy(context.epoch, link->epoch, sizeof(context.epoch));
	context.generation = record->generation;
	context.expires = request->expires;
	context_revision++;
	operation = op;
	board_io_set_prompt(HD_PROMPT | context_revision);
	status_led_waiting(true);
	receipt_length = 0;
	nu54_recovery_open_message(&context, output);
	output[0] = op;
	*length = NU54_RECOVERY_OPEN_SIZE;
done:
	k_mutex_unlock(&recovery_lock);
	return 0;
}

void device_hd_disconnected(void)
{
	k_mutex_lock(&recovery_lock, K_FOREVER);
	if (operation) {
		board_io_set_prompt(0);
		status_led_waiting(false);
	}
	operation = 0;
	receipt_length = 0;
	memset(&context, 0, sizeof(context));
	k_mutex_unlock(&recovery_lock);
}

bool device_hd_pending(void)
{
	k_mutex_lock(&recovery_lock, K_FOREVER);
	bool pending = operation && trusted_clock() < context.expires;
	if (!pending && operation) {
		operation = 0;
		board_io_set_prompt(0);
		status_led_waiting(false);
	}
	pending |= running;
	k_mutex_unlock(&recovery_lock);
	return pending;
}

static uint64_t parse64(const uint8_t *data)
{
	uint64_t value = 0;
	for (size_t i = 0; i < 8; i++) value = (value << 8) | data[i];
	return value;
}

static int previous_restore(const uint8_t *proof, size_t length, uint32_t revision)
{
	if (length <= NU54_RECOVERY_BODY_SIZE) return -1;
	nu54_recovery_context expected = {.generation = parse64(proof + 293), .expires = parse64(proof + 310)};
	memcpy(expected.challenge, proof + 25, 32);
	memcpy(expected.request, proof + 57, 16);
	memcpy(expected.target, proof + 107, 32);
	memcpy(expected.owner, proof + 237, 32);
	memcpy(expected.epoch, proof + 269, 16);
	uint64_t issued = parse64(proof + 302);
	k_mutex_lock(&recovery_lock, K_FOREVER);
	int result = !operation || revision != context_revision || issued > trusted_clock() ? -1 :
		     nu54_hd_mark_backup(&hd, proof, length, &expected, issued, &signers);
	k_mutex_unlock(&recovery_lock);
	return result;
}

static int source_transfer(mbedtls_ssl_context *tls, const nu54_recovery_context *active, int64_t end, uint32_t revision)
{
	uint8_t instructions[NU54_RECOVERY_OPEN_SIZE], ready[1];
	nu54_recovery_open_message(active, instructions);
	instructions[0] = 6;
	size_t length = sizeof(instructions), readiness = 0;
	if (packet(tls, instructions, &length, sizeof(instructions), true, end) ||
	    packet(tls, ready, &readiness, sizeof(ready), false, end) || ready[0] != 10)
		return -1;
	uint8_t capsule[NU54_ACCESSORY_CAPSULE_MAX] = {3};
	k_mutex_lock(&recovery_lock, K_FOREVER);
	int rc = !operation || revision != context_revision ? -1 :
		 hd.record.phase == NU54_HD_BACKUP_REQUIRED ? 0 : nu54_hd_create(&hd, active->target);
	k_mutex_unlock(&recovery_lock);
	if (!rc) {
		nu54_recovery_claim(capsule + 1, active->target, active->generation, hd.record.public_key, hd.record.address);
		size_t size = NU54_RECOVERY_DER_MAX;
		rc = proof_sign(NULL, NU54_RECOVERY_SOURCE, capsule + 1, NU54_RECOVERY_CLAIM_SIZE,
				capsule + 2 + NU54_RECOVERY_CLAIM_SIZE, &size);
		if (!rc && size && size <= NU54_RECOVERY_DER_MAX) {
			capsule[1 + NU54_RECOVERY_CLAIM_SIZE] = (uint8_t)size;
			length = 2 + NU54_RECOVERY_CLAIM_SIZE + size + NU54_HD_SEED_SIZE;
			k_mutex_lock(&recovery_lock, K_FOREVER);
			rc = !operation || revision != context_revision || trusted_clock() >= active->expires ? -1 :
			     read(HD_SECRET, capsule + length - NU54_HD_SEED_SIZE, NU54_HD_SEED_SIZE);
			if (!rc) rc = packet(tls, capsule, &length, sizeof(capsule), true, end);
			k_mutex_unlock(&recovery_lock);
		}
	}
	erase(capsule, sizeof(capsule));
	if (!rc) {
		uint8_t result[1 + NU54_RECOVERY_ENVELOPE_MAX];
		size_t size = 0;
		rc = packet(tls, result, &size, sizeof(result), false, end);
		if (!rc && (result[0] != 7 || size != 1))
			rc = result[0] == 8 ? previous_restore(result + 1, size - 1, revision) : -1;
	}
	return rc;
}

static int target_transfer(mbedtls_ssl_context *tls, const nu54_recovery_context *active, int64_t end, uint32_t revision)
{
	uint8_t instructions[NU54_RECOVERY_OPEN_SIZE];
	nu54_recovery_open_message(active, instructions);
	size_t length = sizeof(instructions);
	uint8_t capsule[NU54_ACCESSORY_CAPSULE_MAX], proof[NU54_RECOVERY_ENVELOPE_MAX];
	size_t capsule_length = 0, proof_length = 0;
	int rc = -1;
	if (packet(tls, instructions, &length, sizeof(instructions), true, end) ||
	    packet(tls, capsule, &capsule_length, sizeof(capsule), false, end) ||
	    nu54_accessory_target_proof(&hd, capsule, capsule_length, active, trusted_clock(), &signers, proof, &proof_length) ||
	    packet(tls, proof, &proof_length, sizeof(proof), true, end) ||
	    packet(tls, proof, &proof_length, sizeof(proof), false, end)) goto done;
	k_mutex_lock(&recovery_lock, K_FOREVER);
	if (operation && revision == context_revision &&
	    !nu54_hd_restore(&hd, capsule + capsule_length - NU54_HD_SEED_SIZE, NU54_HD_SEED_SIZE,
			     proof, proof_length, active, trusted_clock(), &signers)) {
		memcpy(receipt, proof, proof_length);
		receipt_length = proof_length;
		rc = 0;
	}
	k_mutex_unlock(&recovery_lock);
	if (!rc) {
		uint8_t committed[33] = {9};
		size_t length = sizeof(committed);
		if (hash32(proof, proof_length, committed + 1) || packet(tls, committed, &length, sizeof(committed), true, end))
			rc = -1;
	}
done:
	erase(capsule, sizeof(capsule));
	return rc;
}

bool device_hd_physical(uint32_t captured)
{
	k_mutex_lock(&recovery_lock, K_FOREVER);
	uint8_t op = operation;
	uint32_t revision = context_revision;
	nu54_recovery_context active = context;
	if (!op || running || captured != (HD_PROMPT | revision) || trusted_clock() >= active.expires) {
		k_mutex_unlock(&recovery_lock);
		return false;
	}
	running = true;
	k_mutex_unlock(&recovery_lock);
	if (device_recovery_pin(hd.record.phase == NU54_HD_EMPTY && exists(NULL) == 1) ||
	    trusted_clock() >= active.expires) goto done;
	k_mutex_lock(&recovery_lock, K_FOREVER);
	bool current = operation && revision == context_revision;
	k_mutex_unlock(&recovery_lock);
	if (!current) goto done;
	mbedtls_ssl_context tls;
	mbedtls_ssl_init(&tls);
	ring_buf_reset(&input);
	atomic_set(&overrun, 0);
	uart_irq_rx_enable(uart);
	int64_t end = k_uptime_get() + 60000;
	int rc = unlock_tls(&tls, end);
	if (!rc) rc = op == 5 ? target_transfer(&tls, &active, end, revision) : source_transfer(&tls, &active, end, revision);
	mbedtls_ssl_free(&tls);
	uart_irq_rx_disable(uart);
	status_led_event(rc ? NU54_EVENT_REFUSED : NU54_EVENT_SIGNED);
done:
	k_mutex_lock(&recovery_lock, K_FOREVER);
	if (revision == context_revision && operation) {
		operation = 0;
		board_io_set_prompt(0);
		status_led_waiting(false);
	}
	running = false;
	k_mutex_unlock(&recovery_lock);
	return true;
}

#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
static bool personal_policy_ready(void)
{
	const nu54_vault *vault = device_vault_owner();
	uint8_t board[32];
	if (!vault || !vault->enabled || !vault->record.generation || (vault->record.flags & NU54_VAULT_REVOKED) ||
	    hash32(vault->record.identity, sizeof(vault->record.identity), board)) return false;
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
	return device_owner_finality_ready_ps(board, vault->record.generation, hd.record.address);
#else
	return device_owner_limits_ready_ps(board, vault->record.generation);
#endif
}
#endif

int device_hd_sign(const uint8_t address[20], const uint8_t digest[32], uint8_t signature[64])
{
	uint8_t seed[NU54_HD_SEED_SIZE], blind[32];
	k_mutex_lock(&recovery_lock, K_FOREVER);
	int rc = -1;
	if (enabled && hd.enabled && hd.record.phase == NU54_HD_READY && !running && !operation &&
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
	    personal_policy_ready() &&
#endif
	    !memcmp(address, hd.record.address, 20) &&
	    psa_generate_random(blind, sizeof(blind)) == PSA_SUCCESS &&
	    !read(HD_SECRET, seed, sizeof(seed)) && !witness_seed(seed, true))
		rc = nu54_hd_sign_first_evm(seed, blind, hmac_sha512, address, digest, signature);
	erase(seed, sizeof(seed));
	erase(blind, sizeof(blind));
	k_mutex_unlock(&recovery_lock);
	return rc;
}

static int market_account(uint8_t address[20], const uint8_t *digest, uint8_t *signature)
{
	uint8_t seed[32], blind[32];
	int rc = -1;
	k_mutex_lock(&recovery_lock, K_FOREVER);
	if (enabled && hd.enabled && hd.record.phase == NU54_HD_READY && !running && !operation &&
	    psa_generate_random(blind, sizeof(blind)) == PSA_SUCCESS &&
	    !read(HD_SECRET, seed, sizeof(seed)) && !witness_seed(seed, true))
		rc = digest ? nu54_hd_sign_market_evm(seed, blind, hmac_sha512, address, digest, signature)
		            : nu54_hd_market_evm(seed, blind, hmac_sha512, address);
	erase(seed,sizeof(seed));
	erase(blind,sizeof(blind));
	k_mutex_unlock(&recovery_lock);
	return rc;
}

bool device_hd_market_address(uint8_t address[20]) { return market_account(address,NULL,NULL) == 0; }

int device_hd_market_sign(const uint8_t address[20], const uint8_t digest[32], uint8_t signature[64])
{
	uint8_t expected[20];
	memcpy(expected,address,sizeof(expected));
	return market_account(expected,digest,signature);
}

#if defined(CONFIG_NU54_WALLET_CHAINS)
#define CHAIN_HARDENED UINT32_C(0x80000000)
#define CHAIN_ACCOUNT_MAX 99

typedef struct {
	const device_chain_account *account;
	uint32_t branch, index;
	const uint8_t *expected, *message;
	size_t length;
	uint8_t *output, *chain_code;
	bool bip48;
} chain_key_operation;

static int chain_ed25519(const uint8_t child[32], void *context)
{
	const chain_key_operation *op = context;
	psa_key_attributes_t attrs = PSA_KEY_ATTRIBUTES_INIT;
	psa_key_id_t id = 0;
	uint8_t public_key[32];
	size_t length;
	int result = -1;
	psa_set_key_type(&attrs, PSA_KEY_TYPE_ECC_KEY_PAIR(PSA_ECC_FAMILY_TWISTED_EDWARDS));
	psa_set_key_bits(&attrs, 255);
	psa_set_key_usage_flags(&attrs, PSA_KEY_USAGE_SIGN_MESSAGE);
	psa_set_key_algorithm(&attrs, PSA_ALG_PURE_EDDSA);
	if (psa_import_key(&attrs, child, 32, &id) != PSA_SUCCESS ||
	    psa_export_public_key(id, public_key, sizeof(public_key), &length) != PSA_SUCCESS ||
	    length != sizeof(public_key)) goto done;
	if (op->expected) {
		if (memcmp(op->expected, public_key, sizeof(public_key)) ||
		    psa_sign_message(id, PSA_ALG_PURE_EDDSA, op->message, op->length, op->output, 64, &length) != PSA_SUCCESS ||
		    length != 64) goto done;
	} else {
		memcpy(op->output, public_key, sizeof(public_key));
		op->output[32] = 0;
	}
	result = 0;
done:
	if (id && psa_destroy_key(id) != PSA_SUCCESS) result = -1;
	psa_reset_key_attributes(&attrs);
	return result;
}

static int chain_key(chain_key_operation *op, size_t depth)
{
	uint8_t seed[NU54_HD_SEED_SIZE], blind[32];
	const device_chain_account *account = op->account;
	int rc = -1;
	uint8_t max_network = 1;
#ifdef CONFIG_NU54_COMMISSIONED_WALLET_CHAINS
	max_network = NU54_CHAIN_COMMISSIONED;
#endif
	if (!account || !op->output || account->network > max_network || account->account > CHAIN_ACCOUNT_MAX ||
	    op->branch > 1 || op->index > 9999 ||
	    account->chain < NU54_CHAIN_BTC || account->chain > NU54_CHAIN_XRPL ||
	    (account->chain != NU54_CHAIN_BTC && (op->branch || op->index))) return -1;
	k_mutex_lock(&recovery_lock, K_FOREVER);
	if (!enabled || !hd.enabled || hd.record.phase != NU54_HD_READY || running || operation ||
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
	#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
	    op->expected ||
#else
	    (op->expected && !personal_policy_ready()) ||
#endif
#endif
	    !device_install_policy() || psa_generate_random(blind, sizeof(blind)) != PSA_SUCCESS ||
	    read(HD_SECRET, seed, sizeof(seed)) || witness_seed(seed, false)) goto done;
	if (account->chain == NU54_CHAIN_SOL) {
		rc = nu54_sol_child(seed, account->account, hmac_sha512, chain_ed25519, op);
	} else {
		uint32_t purpose = op->bip48 ? 48 : account->chain == NU54_CHAIN_BTC ? 84 : 44;
		uint32_t coin = account->chain == NU54_CHAIN_BTC ? (account->network ? 1 : 0) : 144;
		uint32_t path[] = {CHAIN_HARDENED | purpose, CHAIN_HARDENED | coin,
			CHAIN_HARDENED | nu54_chain_derivation_account(account->chain, account->network, account->account),
			op->branch, op->index, 0};
		if (op->bip48) {
			path[3] = CHAIN_HARDENED | 2;
			path[4] = op->branch;
			path[5] = op->index;
		}
		rc = op->expected ?
			nu54_hd_bip32_sign(seed, blind, hmac_sha512, path, depth, op->expected, op->message, op->output) :
			nu54_hd_bip32_public(seed, blind, hmac_sha512, path, depth, op->output, op->chain_code);
	}
done:
	erase(seed, sizeof(seed));
	erase(blind, sizeof(blind));
	k_mutex_unlock(&recovery_lock);
	return rc;
}

int device_chain_public(const device_chain_account *account, uint32_t branch, uint32_t index, uint8_t output[33])
{
	chain_key_operation op = {.account = account, .branch = branch, .index = index, .output = output};
	return chain_key(&op, 5);
}

int device_chain_secp_sign(const device_chain_account *account, uint32_t branch, uint32_t index,
			    const uint8_t expected[33], const uint8_t digest[32], uint8_t output[64])
{
	if (!account || account->chain == NU54_CHAIN_SOL || !expected || !digest) return -1;
	chain_key_operation op = {.account = account, .branch = branch, .index = index, .expected = expected,
		.message = digest, .output = output};
	return chain_key(&op, 5);
}

int device_chain_sol_sign(const device_chain_account *account, const uint8_t expected[32],
			  const uint8_t *message, size_t length, uint8_t output[64])
{
	if (!account || account->chain != NU54_CHAIN_SOL || !expected || !message || !length ||
	    length > NU54_SOL_MESSAGE_MAX) return -1;
	chain_key_operation op = {.account = account, .expected = expected, .message = message,
		.length = length, .output = output};
	return chain_key(&op, 5);
}

int device_chain_btc_root(uint8_t output[33])
{
	const device_chain_account account = {NU54_CHAIN_BTC, 0, 0};
	chain_key_operation op = {.account = &account, .output = output};
	return chain_key(&op, 0);
}

int device_chain_btc_account(const device_chain_account *account, uint8_t public[33], uint8_t code[32],
			     uint8_t parent[33])
{
	if (!account || account->chain != NU54_CHAIN_BTC || !code || !parent) return -1;
	chain_key_operation op = {.account = account, .output = parent};
	if (chain_key(&op, 2)) return -1;
	op.output = public;
	op.chain_code = code;
	return chain_key(&op, 3);
}
int device_chain_bip48_pub(const device_chain_account *account, uint8_t pub[33], uint8_t code[32])
{
	if (!account || account->chain != NU54_CHAIN_BTC || (account->network != 1 && account->network != NU54_CHAIN_COMMISSIONED) || !code) return -1;
	chain_key_operation op = {.account = account, .output = pub, .chain_code = code, .bip48 = true};
	return chain_key(&op, 4);
}

int device_chain_bip48_parent(const device_chain_account *account, uint8_t public_key[33])
{
	if (!account || account->chain != NU54_CHAIN_BTC || (account->network != 1 && account->network != NU54_CHAIN_COMMISSIONED)) return -1;
	chain_key_operation op = {.account = account, .output = public_key, .bip48 = true};
	return chain_key(&op, 3);
}

int device_chain_bip48_sign(const device_chain_account *account, uint32_t branch, uint32_t index,
			     const uint8_t expected[33], const uint8_t digest[32], uint8_t output[64])
{
	if (!account || account->chain != NU54_CHAIN_BTC || (account->network != 1 && account->network != NU54_CHAIN_COMMISSIONED) || !expected || !digest) return -1;
	chain_key_operation op = {.account = account, .branch = branch, .index = index, .expected = expected,
		.message = digest, .output = output, .bip48 = true};
	return chain_key(&op, 6);
}

int device_chain_public_hmac(const uint8_t *key, size_t key_size, const uint8_t *data, size_t length,
			      uint8_t digest[64])
{
	return hmac_sha512(key, key_size, data, length, digest);
}

#endif

bool device_hd_public_address(uint8_t address[20])
{
	k_mutex_lock(&recovery_lock, K_FOREVER);
	bool ready = enabled && hd.enabled && hd.record.phase == NU54_HD_READY;
	if (ready) memcpy(address, hd.record.address, 20);
	k_mutex_unlock(&recovery_lock);
	return ready;
}

void device_hd_public_status(uint8_t status[77])
{
	k_mutex_lock(&recovery_lock, K_FOREVER);
	if (enabled && hd.enabled && hd.record.phase >= NU54_HD_BACKUP_REQUIRED) {
		status[2] = NU54_HD_FORMAT;
		status[3] = NU54_HD_PROFILE;
		status[4] = hd.record.phase == NU54_HD_READY;
	}
	k_mutex_unlock(&recovery_lock);
}

#else

#if defined(CONFIG_NU54_WALLET_CHAINS)
#error "Multichain wallets require the independent recovery UART"
#endif

int device_hd_configure(mbedtls_x509_crt *cert, mbedtls_pk_context *key, uint64_t (*clock)(void),
			const uint8_t *trust, size_t length)
{ (void)cert; (void)key; (void)clock; (void)trust; (void)length; return -1; }
void device_hd_clear(void) {}
void device_hd_disconnected(void) {}
bool device_hd_pending(void) { return false; }
bool device_hd_physical(uint32_t prompt) { (void)prompt; return false; }
int device_hd_sign(const uint8_t address[20], const uint8_t digest[32], uint8_t signature[64])
{ (void)address; (void)digest; (void)signature; return -1; }
bool device_hd_public_address(uint8_t address[20]) { (void)address; return false; }
bool device_hd_market_address(uint8_t address[20]) { (void)address; return false; }
int device_hd_market_sign(const uint8_t address[20], const uint8_t digest[32], uint8_t signature[64])
{ (void)address; (void)digest; (void)signature; return -1; }
void device_hd_public_status(uint8_t status[77]) { (void)status; }
int device_hd_command(const nu54_v2_link *link, const nu54_v2_request *request, uint64_t now,
		      uint8_t output[1 + NU54_RECOVERY_ENVELOPE_MAX], size_t *length)
{
	(void)link;
	(void)now;
	if (request->payload_length != 1) return -1;
	output[0] = request->payload[0];
	*length = 1;
	return 0;
}

#endif
