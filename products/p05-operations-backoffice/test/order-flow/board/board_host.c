/* The spending wallet board on the host. The session, order, return and reset code comes from the
 * firmware (see CMakeLists.txt). Played here: flash, keys and hashes (PSA), Bluetooth and TLS
 * (stdin and stdout), the PIN, the buttons, the lamp, and the grant the operator installs per boot.
 *
 * Commands, one per line:
 *   policy ESCROW TOKEN BENEFICIARY SIGNER MERCHANT PHONE   hex; what the grant pins
 *   grant SECONDS AHEAD | nogrant  a grant for this boot, or none; AHEAD is how far the board's latest
 *                                possible time leads, which is how long the operator took to sign
 *   authority POINT              the pinned installation authority (P-256, uncompressed)
 *   connect EPOCH PEER           a TLS session of the phone with this SPKI SHA-256
 *   data HEX                     bytes the phone wrote; the first one of a session is the route
 *   close                        the Bluetooth link ended
 *   press | refuse | wrongpin    SW1 and the PIN, SW2, SW1 and a wrong PIN
 *   digest HEX                   the wallet key's signature; only for requests the firmware does not
 *                                know yet, which ../app/board.ts plays ("signature HEX")
 *   boot                         power cycle
 * Answers: "out HEX" bytes to the phone, "prompt" a button is awaited, "lamp N" an event,
 * "ended" the board left the session, "erased", "booted DEVICE ACCOUNT GENERATION", "ok". */
#include "device_firmware_power.h"
#include "device_key.h"
#include "device_order.h"
#include "device_recovery_pin.h"
#include "device_spending.h"
#include "device_vault.h"
#include "nu54_install.h"
#include "nu54_keccak.h"
#include "nu54_owner.h"
#include "nu54_owner_idle.h"
#include "nu54_running_firmware.h"
#include "nu54_sig.h"
#include "board_io.h"
#include "status_led.h"

#include <assert.h>
#include <openssl/core_names.h>
#include <openssl/ecdsa.h>
#include <openssl/evp.h>
#include <openssl/rand.h>
#include <openssl/sha.h>
#include <openssl/x509.h>
#include <poll.h>
#include <psa/internal_trusted_storage.h>
#include <secp256k1.h>
#include <setjmp.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>
#include <zephyr/kernel.h>

static void say(const char *format, ...)
{
	va_list arguments;
	va_start(arguments, format);
	vprintf(format, arguments);
	va_end(arguments);
	putchar('\n');
	fflush(stdout);
}

static void say_bytes(const char *word, const uint8_t *data, size_t length)
{
	printf("%s ", word);
	for (size_t i = 0; i < length; ++i) printf("%02x", data[i]);
	putchar('\n');
	fflush(stdout);
}

/* ---- time ---- */
static int64_t monotonic(void)
{
	struct timespec now;
	clock_gettime(CLOCK_MONOTONIC, &now);
	return (int64_t)now.tv_sec * 1000 + now.tv_nsec / 1000000;
}
static int64_t boot_at;
int64_t k_uptime_get(void) { return monotonic() - boot_at + 1; }
static void k_msleep(int64_t milliseconds) { (void)milliseconds; }

/* ---- flash ---- */
static struct {uint64_t uid; size_t size; uint8_t data[160];} flash[96];
static int find(uint64_t uid)
{
	for (int i = 0; i < 96; ++i)
		if (flash[i].uid == uid) return i;
	return -1;
}
psa_status_t psa_its_get_info(psa_storage_uid_t uid, struct psa_storage_info_t *info)
{
	int at = find(uid);
	if (at < 0) return PSA_ERROR_DOES_NOT_EXIST;
	info->size = flash[at].size;
	return PSA_SUCCESS;
}
psa_status_t psa_its_get(psa_storage_uid_t uid, size_t offset, size_t size, void *data, size_t *used)
{
	int at = find(uid);
	assert(!offset);
	if (at < 0) return PSA_ERROR_DOES_NOT_EXIST;
	*used = size < flash[at].size ? size : flash[at].size;
	memcpy(data, flash[at].data, *used);
	return PSA_SUCCESS;
}
psa_status_t psa_its_set(psa_storage_uid_t uid, size_t size, const void *data, uint32_t flags)
{
	int at = find(uid);
	if (at < 0) at = find(0);
	assert(!flags && uid && at >= 0 && size <= sizeof(flash[0].data));
	flash[at].uid = uid;
	flash[at].size = size;
	memcpy(flash[at].data, data, size);
	return PSA_SUCCESS;
}
psa_status_t psa_its_remove(psa_storage_uid_t uid)
{
	int at = find(uid);
	if (at < 0) return PSA_ERROR_DOES_NOT_EXIST;
	memset(&flash[at], 0, sizeof(flash[at]));
	return PSA_SUCCESS;
}

/* ---- the one wallet key: secp256k1, made by the board ---- */
static secp256k1_context *curve;
static psa_key_attributes_t wallet_attributes;
static uint8_t wallet_key[32];
psa_status_t psa_get_key_attributes(psa_key_id_t id, psa_key_attributes_t *out)
{
	if (!id || wallet_attributes.id != id) return PSA_ERROR_INVALID_HANDLE;
	*out = wallet_attributes;
	return PSA_SUCCESS;
}
psa_status_t psa_generate_key(const psa_key_attributes_t *attributes, psa_key_id_t *id)
{
	if (wallet_attributes.id) return PSA_ERROR_ALREADY_EXISTS;
	do assert(RAND_bytes(wallet_key, sizeof(wallet_key)) == 1);
	while (!secp256k1_ec_seckey_verify(curve, wallet_key));
	wallet_attributes = *attributes;
	*id = attributes->id;
	return PSA_SUCCESS;
}
psa_status_t psa_destroy_key(psa_key_id_t id)
{
	if (!id || wallet_attributes.id != id) return PSA_ERROR_INVALID_HANDLE;
	memset(wallet_key, 0, sizeof(wallet_key));
	memset(&wallet_attributes, 0, sizeof(wallet_attributes));
	return PSA_SUCCESS;
}
psa_status_t psa_export_public_key(psa_key_id_t id, uint8_t *out, size_t size, size_t *used)
{
	secp256k1_pubkey key;
	if (!id || wallet_attributes.id != id) return PSA_ERROR_INVALID_HANDLE;
	assert(secp256k1_ec_pubkey_create(curve, &key, wallet_key));
	*used = size;
	return secp256k1_ec_pubkey_serialize(curve, out, used, &key, SECP256K1_EC_UNCOMPRESSED) ? PSA_SUCCESS : PSA_ERROR_GENERIC_ERROR;
}
psa_status_t psa_sign_hash(psa_key_id_t id, psa_algorithm_t algorithm, const uint8_t *digest, size_t length, uint8_t *out,
			   size_t size, size_t *used)
{
	secp256k1_ecdsa_signature signature;
	if (!id || wallet_attributes.id != id) return PSA_ERROR_INVALID_HANDLE;
	assert(length == 32 && size == 64 && algorithm == wallet_attributes.algorithm);
	assert(secp256k1_ecdsa_sign(curve, &signature, digest, wallet_key, NULL, NULL));
	assert(secp256k1_ecdsa_signature_serialize_compact(curve, out, &signature));
	*used = 64;
	return PSA_SUCCESS;
}

/* ---- hashes and the P-256 keys of the phone and the installation authority, by OpenSSL ---- */
static int hash(const uint8_t *data, size_t length, uint8_t digest[32]) { return SHA256(data, length, digest) ? 0 : -1; }
static int owner_random(uint8_t *data, size_t length) { return RAND_bytes(data, (int)length) == 1 ? 0 : -1; }
static bool verify_der(EVP_PKEY *key, const uint8_t digest[32], const uint8_t *signature, size_t length)
{
	EVP_PKEY_CTX *context = EVP_PKEY_CTX_new(key, NULL);
	bool valid = context && EVP_PKEY_verify_init(context) == 1 && EVP_PKEY_verify(context, signature, length, digest, 32) == 1;
	EVP_PKEY_CTX_free(context);
	return valid;
}
static int owner_verify(const uint8_t *der, size_t length, const uint8_t digest[32], const uint8_t *signature, size_t signature_length)
{
	EVP_PKEY *key = length == 91 ? d2i_PUBKEY(NULL, &der, (long)length) : NULL;
	bool valid = key && verify_der(key, digest, signature, signature_length);
	EVP_PKEY_free(key);
	return valid ? 0 : -1;
}
static const nu54_owner_crypto owner_crypto = {owner_random, hash, owner_verify};
#define PSA_ALG_ECDSA(hash) (64u | (hash))
#define PSA_ECC_FAMILY_SECP_R1 3u
#define PSA_KEY_TYPE_ECC_PUBLIC_KEY(family) (128u | (family))
#define PSA_KEY_USAGE_VERIFY_HASH 32u
#define ID_ALG PSA_ALG_ECDSA(PSA_ALG_SHA_256)
#define PUBLIC_TYPE PSA_KEY_TYPE_ECC_PUBLIC_KEY(PSA_ECC_FAMILY_SECP_R1)
static EVP_PKEY *authority_key;
static psa_key_id_t authority;
static psa_status_t psa_import_key(const psa_key_attributes_t *attributes, const uint8_t *key, size_t length, psa_key_id_t *id)
{
	char group[] = "prime256v1";
	OSSL_PARAM params[] = {OSSL_PARAM_construct_utf8_string(OSSL_PKEY_PARAM_GROUP_NAME, group, 0),
			       OSSL_PARAM_construct_octet_string(OSSL_PKEY_PARAM_PUB_KEY, (void *)(uintptr_t)key, length),
			       OSSL_PARAM_construct_end()};
	EVP_PKEY_CTX *context = EVP_PKEY_CTX_new_from_name(NULL, "EC", NULL);
	assert(attributes->type == PUBLIC_TYPE && attributes->algorithm == ID_ALG);
	EVP_PKEY_free(authority_key);
	authority_key = NULL;
	bool valid = context && EVP_PKEY_fromdata_init(context) == 1 &&
		     EVP_PKEY_fromdata(context, &authority_key, EVP_PKEY_PUBLIC_KEY, params) == 1;
	EVP_PKEY_CTX_free(context);
	*id = 1;
	return valid ? PSA_SUCCESS : PSA_ERROR_GENERIC_ERROR;
}
static psa_status_t psa_verify_hash(psa_key_id_t id, unsigned algorithm, const uint8_t *digest, size_t digest_length,
				    const uint8_t *signature, size_t length)
{
	assert(algorithm == ID_ALG && digest_length == 32 && length == 64);
	if (!id || !authority_key) return PSA_ERROR_GENERIC_ERROR;
	uint8_t der[80], *cursor = der;
	ECDSA_SIG *value = ECDSA_SIG_new();
	assert(value && ECDSA_SIG_set0(value, BN_bin2bn(signature, 32, NULL), BN_bin2bn(signature + 32, 32, NULL)));
	int size = i2d_ECDSA_SIG(value, &cursor);
	ECDSA_SIG_free(value);
	return size > 0 && verify_der(authority_key, digest, der, (size_t)size) ? PSA_SUCCESS : PSA_ERROR_GENERIC_ERROR;
}

/* ---- parts of the board that no flow here depends on ---- */
int device_key_slot_open(device_key_slot slot, bool create, uint8_t address[20])
{
	(void)create;
	memset(address, 0x5d, 20);
	return slot == DEVICE_KEY_IDENTITY ? 0 : PSA_ERROR_DOES_NOT_EXIST;
}
enum nu54_firmware_vin device_firmware_vin(void) { return NU54_FIRMWARE_VIN_UNKNOWN; }
static int device_install_running_firmware(uint64_t generation, uint8_t *out) { (void)generation; (void)out; return -1; }
static int device_install_update_ack(const uint8_t *in, uint8_t *out) { (void)in; (void)out; return -1; }
static int device_install_fence_wallet_expiry(uint64_t expiry) { (void)expiry; return -1; }
static int device_install_fence_receipt_expiry(uint64_t expiry) { (void)expiry; return -1; }
static uint64_t device_install_wallet_replay_floor(void) { return 0; }
static bool device_install_independent_peer(const uint8_t peer[32]) { (void)peer; return true; }
static const uint8_t *device_install_payer(void) { static const uint8_t none[20] = {0}; return none; }
static int device_install_sign(const uint8_t *digest, uint8_t *signature) { (void)digest; (void)signature; return -1; }
static int device_install_account_statement(const uint8_t *statement, size_t length, uint8_t signature[64])
{
	(void)statement;
	memset(signature, 0, 64); /* signed by the board identity key, which the app does not check */
	return length == NU54_ACCOUNT_STATEMENT_SIZE ? 0 : -1;
}
static int ble_links_unpair_all(void) { return 0; }
static int ble_links_bond_count(void) { return 0; }
static int pay_link_waiting(void) { return 0; }

/* ---- the PIN, buttons and lamp ---- */
static bool pin_is_right = true;
int device_recovery_pin(bool virgin) { (void)virgin; return pin_is_right ? 0 : -1; }
int device_recovery_pin_guarded(bool virgin, device_pin_guard guard, void *context)
{
	return guard(context, false, 0) ? device_recovery_pin(virgin) : -1;
}
int32_t device_recovery_pin_wait(void) { return 0; }
int device_recovery_pin_erase(void) { return 0; }
int board_button_wait(board_button_event_t *out, int32_t timeout) { (void)out; (void)timeout; return -1; }
static uint32_t shown_prompt;
void board_io_set_prompt(uint32_t prompt) { shown_prompt = prompt; if (prompt) say("prompt"); }
void status_led_waiting(bool on) { (void)on; }
void status_led_event(nu54_event_t event) { say("lamp %d", (int)event); }
void status_led_pressed(void) {}
void status_led_clear(void) {}
void status_led_reset_done(void) { say("erased"); }
bool status_led_locate(bool start) { (void)start; return false; }

/* ---- the grant of this boot (device_install.c): trusted time and the pinned store ---- */
static nu54_install_policy grant;
static bool granted;
static uint64_t granted_time;
static int64_t granted_uptime;
static uint64_t device_install_earliest(void) { return granted ? granted_time + (uint64_t)(k_uptime_get() - granted_uptime) / 1000u : 0; }
/* The grant's time is a lower bound and lies 2 seconds back here. */
#define GRANT_LAG 2
static uint64_t max_offset;
static uint64_t device_install_now(void) { return granted ? device_install_earliest() + max_offset : 0; }
static const nu54_install_policy *device_install_policy(void)
{
	return granted && device_install_now() < grant.not_after ? &grant : NULL;
}

/* ---- the state of device_pilot.c, and what its functions use of Zephyr and mbed TLS ---- */
#define PILOT_REFUSAL_UID ((psa_storage_uid_t)0x5530)
#define PILOT_RECEIPT_UID ((psa_storage_uid_t)0x5490)
#define PILOT_TIMEOUT_MS 120000
#define NU54_PILOT_PROMPT_MASK UINT32_C(0x80000000)
#define ARRAY_SIZE(a) (sizeof(a) / sizeof((a)[0]))
#define ARG_UNUSED(x) ((void)(x))
#define MAX(a, b) ((a) > (b) ? (a) : (b))
#define IS_ENABLED(x) 1
#define K_FOREVER 0
#define k_mutex_lock(lock, timeout) ((void)(lock))
#define k_mutex_unlock(lock) ((void)(lock))
typedef int32_t atomic_val_t;
typedef int32_t atomic_t;
static int32_t atomic_get(const atomic_t *value) { return *value; }
static void atomic_set(atomic_t *value, int32_t next) { *value = next; }
static uint64_t sys_get_be64(const uint8_t *in)
{
	uint64_t value = 0;
	for (int i = 0; i < 8; ++i) value = value << 8 | in[i];
	return value;
}
static void sys_put_be64(uint64_t value, uint8_t *out) { for (int i = 7; i >= 0; --i, value >>= 8) out[i] = (uint8_t)value; }
static void sys_put_be32(uint32_t value, uint8_t *out) { for (int i = 3; i >= 0; --i, value >>= 8) out[i] = (uint8_t)value; }
struct bt_conn {int unused;};
#define bt_conn_unref(conn) ((void)(conn))
#define ring_buf_reset(buffer) ((void)0)
#define mbedtls_ssl_init(context) ((void)0)
#define mbedtls_ssl_free(context) ((void)0)
#define mbedtls_ssl_config_free(config) ((void)0)
#define mbedtls_x509_crt_free(certificate) ((void)0)
#define mbedtls_platform_zeroize(data, size) memset(data, 0, size)
static int mbedtls_ssl_write(const int *context, const uint8_t *data, size_t length)
{
	(void)context;
	say_bytes("out", data, length);
	return (int)length;
}
static jmp_buf restart;
#define SYS_REBOOT_COLD 0
static void sys_reboot(int kind) { (void)kind; longjmp(restart, 1); }

#define link board_link /* not the one of unistd.h */
static int pilot_lock, tls;
static struct bt_conn phone, *connection, *ticket_peer, *display_connection;
static nu54_v2_profile profile;
static nu54_v2_link link;
static nu54_local_approval approval;
static nu54_pilot_review_binding reviewed_quote;
static unsigned refusal_slot, receipt_slot;
static nu54_owner_enrollment enrollment;
static nu54_v2_journal_entry journal_slots[128];
static nu54_v2_journal journal = {journal_slots, ARRAY_SIZE(journal_slots), hash, 0};
static bool bound, recovering;
static int64_t deadline, recovery_window;
static nu54_owner_idle owner_idle;
static uint8_t display_challenge[32], active_quote[NU54_V2_SIGNED_SIZE], display_signature[64], rental_review_hash[32];
static size_t display_used;
static bool display_confirmed;
static unsigned order_slot;
static uint64_t order_amount, order_expires;
static nu54_spend_action spend_action;
static uint8_t reset_nonce[32];
static uint64_t reset_nonce_generation;
static int64_t reset_nonce_until;
enum install_phase {INSTALL_IDLE, INSTALL_HANDSHAKE};
static enum install_phase install_phase;
static uint8_t install_bundle[4], install_offer[4];
static size_t install_used, install_total;
static uint8_t wire[NU54_V2_FRAME_MAX + 2];
static size_t wire_length;
static uint32_t next_prompt;
static atomic_t pending_prompt;
static uint64_t trusted_now(void) { return device_install_now(); }
static int reclaim_receipt(nu54_v2_request *, const nu54_vault *);
static bool payment_generation_current(const nu54_install_policy *policy);
static bool payment_address(const nu54_install_policy *policy, uint8_t address[20]);
static int sign(void *context, const uint8_t digest[32], uint8_t signature[64]);
void device_pilot_button(uint32_t prompt, bool approved);
int device_install_verify_authority(const uint8_t *data, size_t length, const uint8_t signature[64]);
#ifdef NU54_TOKEN_UNIT
#include "token_unit.h"
#endif
#include "board_pilot.h"

/* ---- commands ---- */
static size_t unhex(const char *text, uint8_t *out, size_t capacity)
{
	size_t length = strlen(text) / 2;
	assert(strlen(text) % 2 == 0 && length <= capacity);
	for (size_t i = 0; i < length; ++i) {
		unsigned byte;
		assert(sscanf(text + 2 * i, "%2x", &byte) == 1);
		out[i] = (uint8_t)byte;
	}
	return length;
}

static void exact(const char *text, uint8_t *out, size_t length) { assert(text && unhex(text, out, length) == length); }

static void boot(void)
{
	uint8_t device[32];
	const nu54_vault *vault;
	reset_link();
	memset(journal_slots, 0, sizeof(journal_slots));
	journal.trusted_time = 0;
	reset_nonce_until = 0;
	granted = false;
	boot_at = monotonic();
	assert(!device_vault_load() && !device_spending_init());
	vault = device_vault_owner();
	assert(!hash(vault->record.identity, sizeof(vault->record.identity), device));
	printf("booted ");
	for (int i = 0; i < 32; ++i) printf("%02x", device[i]);
	printf(" ");
	for (int i = 0; i < 20; ++i) printf("%02x", device_spending_address()[i]);
	say(" %llu", (unsigned long long)vault->record.generation);
}

static void ended(void)
{
	if (!connection) say("ended");
}

static void receive(uint8_t *data, size_t length)
{
	const uint8_t *bytes = data;
	if (!connection || !length) return; /* the phone is not told; its request runs out */
	if (!wire_length && !link.received && !bound) {
		if (bytes[0] != NU54_V2_MANAGEMENT) { reset_link(); ended(); return; }
		bound = true;
		++bytes;
		--length;
	}
	if (stream(bytes, length)) { reset_link(); ended(); }
}

static void start(const char *epoch_text, const char *peer_text)
{
	uint8_t epoch[16], peer[32];
	const uint8_t *pin;
	exact(epoch_text, epoch, sizeof(epoch));
	exact(peer_text, peer, sizeof(peer));
	if (connection) reset_link();
	const nu54_install_policy *policy = device_install_policy();
	/* device_tls_v2.c: without a grant there is no TLS, and the peer is the pinned phone. */
	if (!policy || !nu54_owner_session_pin(device_vault_owner(), policy->owner, &pin) || memcmp(pin, peer, sizeof(peer)) ||
	    nu54_v2_link_init(&link, NU54_V2_MANAGEMENT, NU54_V2_BOARD, epoch, peer, &journal)) {
		say("denied");
		return;
	}
	connection = &phone;
	profile = NU54_V2_MANAGEMENT;
	bound = false;
	journal_new_session();
	deadline = k_uptime_get() + PILOT_TIMEOUT_MS;
	say("open");
}

static void button(bool approved, bool right)
{
	pin_is_right = right;
	device_pilot_button((uint32_t)atomic_get(&pending_prompt), approved);
	pin_is_right = true;
	ended();
}

static void give_grant(const char *seconds, const char *ahead)
{
	const nu54_vault *vault = device_vault_owner();
	/* enroll_board.py pins the generation the next registration gets, or the registered one. */
	bool owned = vault->record.generation && !(vault->record.flags & NU54_VAULT_REVOKED);
	grant.payer_generation = vault->record.generation + !owned;
	granted_time = (uint64_t)time(NULL) - GRANT_LAG;
	max_offset = GRANT_LAG + strtoull(ahead, NULL, 10);
	granted_uptime = k_uptime_get();
	grant.not_after = granted_time + strtoull(seconds, NULL, 10);
	granted = true;
}

static void command(char *line)
{
	char *word = strtok(line, " \n"), *arguments[6] = {0};
	for (int i = 0; i < 6; ++i) arguments[i] = strtok(NULL, " \n");
	if (!word) return;
	if (!strcmp(word, "policy")) {
		grant = (nu54_install_policy){.revision = 1, .merchant_key_version = 1, .beneficiary_version = 1,
			.allowlist.max_amount = NU54_PILOT_TESTUSDC2_CAP, .allowlist.commercial_testnet = true, .payer_role = NU54_V2_OWNER_ROLE};
		exact(arguments[0], grant.allowlist.contract, 20);
		exact(arguments[1], grant.allowlist.token, 20);
		exact(arguments[2], grant.allowlist.beneficiary, 20);
		exact(arguments[3], grant.merchant_signer, 20);
		exact(arguments[4], grant.merchant, 32);
		exact(arguments[5], grant.owner, 32);
	} else if (!strcmp(word, "grant")) give_grant(arguments[0], arguments[1]);
	else if (!strcmp(word, "nogrant")) granted = false;
	else if (!strcmp(word, "authority")) {
		uint8_t point[65];
		exact(arguments[0], point, sizeof(point));
		assert(!import_public(point, &authority));
	} else if (!strcmp(word, "connect")) { start(arguments[0], arguments[1]); return; }
	else if (!strcmp(word, "data")) {
		static uint8_t bytes[4096];
		receive(bytes, unhex(arguments[0], bytes, sizeof(bytes)));
		return;
	} else if (!strcmp(word, "close")) reset_link();
	else if (!strcmp(word, "press")) button(true, true);
	else if (!strcmp(word, "wrongpin")) button(true, false);
	else if (!strcmp(word, "refuse")) button(false, true);
	else if (!strcmp(word, "digest")) {
		uint8_t digest[32], signature[64];
		size_t length;
		exact(arguments[0], digest, sizeof(digest));
		assert(psa_sign_hash(wallet_attributes.id, wallet_attributes.algorithm, digest, 32, signature, 64, &length) == PSA_SUCCESS);
		say_bytes("signature", signature, 64);
		return;
	}
	else if (!strcmp(word, "boot")) { boot(); return; }
	else assert(!"unknown command");
	say("ok");
}

int main(void)
{
	static char line[16384];
	static size_t used;
	curve = secp256k1_context_create(SECP256K1_CONTEXT_NONE);
	assert(curve);
	if (setjmp(restart)) used = 0; /* only one command is in flight */
	boot();
	for (;;) {
		struct pollfd input = {.fd = STDIN_FILENO, .events = POLLIN};
		int ready = poll(&input, 1, 100);
		if (ready < 0) return 1;
		/* poll_tls: a session ends after 120 seconds, or when its grant runs out. */
		if (connection && (!device_install_policy() || !session_live(k_uptime_get()))) { reset_link(); ended(); }
		if (!ready) continue;
		ssize_t got = read(STDIN_FILENO, line + used, sizeof(line) - used - 1);
		if (got <= 0) return 0; /* the test ended */
		used += (size_t)got;
		for (;;) {
			char *end = memchr(line, '\n', used);
			if (!end) break;
			static char one[16384];
			size_t length = (size_t)(end - line) + 1;
			memcpy(one, line, length);
			one[length - 1] = 0;
			memmove(line, line + length, used - length);
			used -= length;
			command(one);
		}
		assert(used < sizeof(line) - 1);
	}
}
