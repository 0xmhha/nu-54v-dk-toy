/* The board side of enroll_board.py for the spending profile: the authority check of
 * device_install.c and the staging of device_pilot.c are extracted from those files at build time.
 * PSA is played by OpenSSL. test_operator_reset.py drives it. */
#include "nu54_install.h"
#include "nu54_owner.h"
#include "nu54_pilot.h"
#include "nu54_spending.h"

#include <assert.h>
#include <openssl/core_names.h>
#include <openssl/ecdsa.h>
#include <openssl/evp.h>
#include <openssl/sha.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/* ---- PSA ---- */
typedef int psa_status_t;
typedef uint32_t psa_key_id_t;
typedef struct {unsigned type, bits, usage, algorithm;} psa_key_attributes_t;
enum {PSA_SUCCESS, PSA_ERROR = -1};
enum {PSA_ALG_SHA_256 = 1, PSA_ECC_FAMILY_SECP_R1 = 2, PSA_KEY_USAGE_VERIFY_HASH = 4};
#define PSA_KEY_ATTRIBUTES_INIT {0}
#define PSA_ALG_ECDSA(hash) (16u | (hash))
#define PSA_KEY_TYPE_ECC_PUBLIC_KEY(family) (32u | (family))
#define ID_ALG PSA_ALG_ECDSA(PSA_ALG_SHA_256)
#define PUBLIC_TYPE PSA_KEY_TYPE_ECC_PUBLIC_KEY(PSA_ECC_FAMILY_SECP_R1)
#define psa_set_key_type(a, v) ((a)->type = (v))
#define psa_set_key_bits(a, v) ((a)->bits = (v))
#define psa_set_key_usage_flags(a, v) ((a)->usage = (v))
#define psa_set_key_algorithm(a, v) ((a)->algorithm = (v))

static EVP_PKEY *imported;

static psa_status_t psa_hash_compute(unsigned algorithm, const uint8_t *data, size_t length, uint8_t *out, size_t size,
				     size_t *used)
{
	assert(algorithm == PSA_ALG_SHA_256 && size == 32);
	*used = 32;
	return SHA256(data, length, out) ? PSA_SUCCESS : PSA_ERROR;
}

static psa_status_t psa_import_key(const psa_key_attributes_t *attrs, const uint8_t *key, size_t length, psa_key_id_t *id)
{
	assert(attrs->type == PUBLIC_TYPE && attrs->bits == 256 && attrs->usage == PSA_KEY_USAGE_VERIFY_HASH &&
	       attrs->algorithm == ID_ALG);
	char group[] = "prime256v1";
	OSSL_PARAM params[] = {OSSL_PARAM_construct_utf8_string(OSSL_PKEY_PARAM_GROUP_NAME, group, 0),
			       OSSL_PARAM_construct_octet_string(OSSL_PKEY_PARAM_PUB_KEY, (void *)(uintptr_t)key, length),
			       OSSL_PARAM_construct_end()};
	EVP_PKEY_CTX *ctx = EVP_PKEY_CTX_new_from_name(NULL, "EC", NULL);
	bool ok = ctx && EVP_PKEY_fromdata_init(ctx) == 1 && EVP_PKEY_fromdata(ctx, &imported, EVP_PKEY_PUBLIC_KEY, params) == 1;
	EVP_PKEY_CTX_free(ctx);
	*id = 1;
	return ok ? PSA_SUCCESS : PSA_ERROR;
}

static psa_status_t psa_verify_hash(psa_key_id_t id, unsigned algorithm, const uint8_t *digest, size_t digest_length,
				    const uint8_t *signature, size_t length)
{
	assert(id == 1 && algorithm == ID_ALG && digest_length == 32 && length == 64);
	ECDSA_SIG *value = ECDSA_SIG_new();
	assert(value && ECDSA_SIG_set0(value, BN_bin2bn(signature, 32, NULL), BN_bin2bn(signature + 32, 32, NULL)));
	uint8_t der[80], *cursor = der;
	int size = i2d_ECDSA_SIG(value, &cursor);
	EVP_PKEY_CTX *ctx = EVP_PKEY_CTX_new(imported, NULL);
	bool ok = ctx && EVP_PKEY_verify_init(ctx) == 1 && EVP_PKEY_verify(ctx, der, (size_t)size, digest, 32) == 1;
	EVP_PKEY_CTX_free(ctx);
	ECDSA_SIG_free(value);
	return ok ? PSA_SUCCESS : PSA_ERROR;
}

/* ---- what the extracted code uses from its own files ---- */
typedef int32_t atomic_val_t;
#define PROMPT_MASK UINT32_C(0x80000000)
#define NU54_PILOT_PROMPT_MASK PROMPT_MASK
#define NOW UINT64_C(1800000000)
static psa_key_id_t authority;
static nu54_owner_enrollment enrollment;
static nu54_v2_link link;
static nu54_spend_action spend_action;
static uint8_t reset_nonce[32], next_nonce[32];
static uint64_t reset_nonce_generation;
static int64_t reset_nonce_until, uptime = 1;
static bool recovering, random_fails;
static uint32_t next_prompt, shown;
static atomic_val_t pending_prompt;
static bool waiting, order_open;
static const uint8_t account[20] = {1};

static const uint8_t *device_spending_address(void) { return account; }
static bool device_spending_return_address(uint8_t out[20]) { return memset(out, 0, 20) == NULL; }
static bool device_order_open(uint64_t earliest, int64_t at) { return earliest && at && order_open; }
static uint64_t device_install_earliest(void) { return NOW; }
static int64_t k_uptime_get(void) { return uptime; }
static int owner_random(uint8_t *out, size_t length)
{
	memcpy(out, next_nonce, length);
	return random_fails ? -1 : 0;
}
static void atomic_set(atomic_val_t *target, atomic_val_t value) { *target = value; }
static void board_io_set_prompt(uint32_t prompt) { shown = prompt; }
static void status_led_waiting(bool on) { waiting = on; }

#include "board_authority_check.h"

/* ---- commands ---- */
static void unhex(const char *text, uint8_t *out, size_t length)
{
	assert(strlen(text) == 2 * length);
	for (size_t i = 0; i < length; ++i) {
		unsigned byte;
		assert(sscanf(text + 2 * i, "%2x", &byte) == 1);
		out[i] = (uint8_t)byte;
	}
}

static void pin_authority(const char *point)
{
	uint8_t key[65];
	unhex(point, key, sizeof(key));
	assert(!import_public(key, &authority));
}

static nu54_vault board(const char *identity, const char *generation)
{
	nu54_vault vault = {.enabled = true, .record.generation = strtoull(generation, NULL, 10)};
	unhex(identity, vault.record.identity, sizeof(vault.record.identity));
	memset(vault.record.owner, 7, sizeof(vault.record.owner));
	return vault;
}

/* What the phone reads with the owner status request (op 3). */
static int status(char **args)
{
	nu54_vault vault = board(args[0], args[1]);
	const nu54_owner_crypto crypto = {.hash = hash};
	uint8_t out[NU54_OWNER_STATUS_SIZE];
	assert(!nu54_owner_status(&vault, &crypto, &enrollment, out));
	for (size_t i = 0; i < sizeof(out); ++i) printf("%02x", out[i]);
	return 0;
}

static nu54_v2_request prepare(const uint8_t *payload, size_t length)
{
	nu54_v2_request request = {.command = NU54_V2_MANAGE, .expires = NOW + 60, .payload = payload, .payload_length = length};
	memset(request.id, 5, sizeof(request.id));
	link = (nu54_v2_link){.profile = NU54_V2_MANAGEMENT, .local = NU54_V2_BOARD, .peer = NU54_V2_OWNER, .pending_expiry = NOW + 60};
	memcpy(link.pending_id, request.id, sizeof(request.id));
	return request;
}

static int attempt(const nu54_v2_request *request, const nu54_vault *vault)
{
	int result = spending_request(request, NOW, vault);
	memset(&spend_action, 0, sizeof(spend_action));
	return result;
}

/* With an answer the board accepts: how long, how often and for which board state it does. */
static void reset_nonce_rules(const nu54_v2_request *request, nu54_vault *vault)
{
	const uint64_t generation = vault->record.generation;
	assert(RESET_NONCE_MS == 900000);
	assert(!reset_nonce_issue(generation));
	uptime += RESET_NONCE_MS - 1;
	link.epoch[0] ^= 1; /* another session */
	assert(!attempt(request, vault));
	assert(!reset_nonce_issue(generation));
	uptime += RESET_NONCE_MS;
	assert(attempt(request, vault));

	uint8_t saved[32];
	memcpy(saved, next_nonce, sizeof(saved));
	assert(!reset_nonce_issue(generation));
	next_nonce[0] ^= 1;
	assert(!reset_nonce_issue(generation)); /* a new request replaces it */
	assert(attempt(request, vault));
	memcpy(next_nonce, saved, sizeof(saved));
	assert(!reset_nonce_issue(generation));
	random_fails = true;
	assert(reset_nonce_issue(generation)); /* and leaves none when it fails */
	random_fails = false;
	assert(attempt(request, vault));

	assert(!reset_nonce_issue(generation - 1)); /* asked for under another generation */
	assert(attempt(request, vault));
	assert(!reset_nonce_issue(generation));
	order_open = true;
	assert(attempt(request, vault)); /* an order can still execute; the nonce stays for a later try */
	order_open = false;
	assert(!attempt(request, vault));
	assert(!reset_nonce_issue(generation));
	vault->record.flags = NU54_VAULT_REVOKED;
	assert(attempt(request, vault));
	vault->record.flags = 0;
}

/* authority point, board identity, generation, live nonce, then the answer's nonce and signature:
 * one 0x7e request from a phone that is not the owner, in the recovery window. */
static int reset(char **args)
{
	uint8_t payload[97] = {NU54_SPEND_OPERATOR_RESET_OP};
	pin_authority(args[0]);
	nu54_vault vault = board(args[1], args[2]);
	unhex(args[3], next_nonce, sizeof(next_nonce));
	unhex(args[4], payload + 1, 32);
	unhex(args[5], payload + 33, 64);
	nu54_v2_request request = prepare(payload, sizeof(payload));

	recovering = true;
	assert(spending_request(&request, NOW, &vault)); /* no nonce was asked for */
	assert(!reset_nonce_issue(vault.record.generation));
	int result = spending_request(&request, NOW, &vault);
	assert(!result == (waiting && spend_action.pending && shown == (uint32_t)pending_prompt && shown));
	memset(&spend_action, 0, sizeof(spend_action));
	assert(spending_request(&request, NOW, &vault)); /* one try per nonce */
	if (!result) reset_nonce_rules(&request, &vault);
	puts(result ? "refused" : "accepted");
	return 0;
}

static size_t load(const char *path, uint8_t *out, size_t capacity)
{
	FILE *file = fopen(path, "rb");
	assert(file);
	size_t used = fread(out, 1, capacity, file);
	assert(fgetc(file) == EOF && !ferror(file));
	fclose(file);
	return used;
}

/* authority point, then offer and grant for each boot. The floor is what stays on the board. */
static int grants(int count, char **args)
{
	nu54_install_floor floor = {0};
	pin_authority(args[0]);
	for (int i = 1; i + 1 < count; i += 2) {
		uint8_t offer[NU54_INSTALL_OFFER_SIZE], grant[NU54_INSTALL_BUNDLE_MAX];
		size_t ca, leaf, length;
		nu54_install_policy policy;
		assert(load(args[i], offer, sizeof(offer)) == sizeof(offer));
		length = load(args[i + 1], grant, sizeof(grant));
		if (nu54_install_accept(grant, length, offer + 4, offer + 36, offer + 101, 0, &floor, hash,
					device_install_verify_authority, &floor, &policy, &ca, &leaf)) puts("refused");
		else printf("role=%u testnet=%d generation=%llu cap=%llu\n", policy.payer_role, policy.allowlist.commercial_testnet,
			    (unsigned long long)policy.payer_generation, (unsigned long long)policy.allowlist.max_amount);
	}
	return 0;
}

/* ---- the real vault: which generation a registration gets before and after a reset ---- */
struct disk {uint8_t data[1 + NU54_VAULT_ENROLLMENT_RECORD][128]; size_t length[1 + NU54_VAULT_ENROLLMENT_RECORD];};

static int read_record(void *context, unsigned record, void *data, size_t length)
{
	struct disk *disk = context;
	if (!disk->length[record]) return NU54_VAULT_ABSENT;
	assert(disk->length[record] == length);
	memcpy(data, disk->data[record], length);
	return 0;
}

static int write_record(void *context, unsigned record, const void *data, size_t length)
{
	struct disk *disk = context;
	assert(length <= sizeof(disk->data[record]));
	memcpy(disk->data[record], data, length);
	disk->length[record] = length;
	return 0;
}

static int vault_key(void *context, bool personal, bool create, uint8_t address[20])
{
	(void)context;
	(void)create;
	memset(address, 9, 20);
	return personal ? NU54_VAULT_ABSENT : 0;
}

static int generations(void)
{
	struct disk disk = {0};
	const nu54_vault_storage io = {&disk, read_record, write_record, vault_key};
	const uint8_t owner[32] = {1};
	nu54_vault vault = {0};
	assert(!nu54_vault_open(&vault, &io));
	for (int round = 0; round < 2; ++round) {
		assert(!nu54_vault_rebind(&vault, vault.record.generation, owner));
		printf("%llu ", (unsigned long long)vault.record.generation);
		assert(!nu54_vault_release(&vault));
		printf("%llu ", (unsigned long long)vault.record.generation);
	}
	return 0;
}

int main(int argc, char **argv)
{
	if (argc == 4 && !strcmp(argv[1], "status")) return status(argv + 2);
	if (argc == 8 && !strcmp(argv[1], "reset")) return reset(argv + 2);
	if (argc >= 5 && !strcmp(argv[1], "grants")) return grants(argc - 2, argv + 2);
	if (argc == 2 && !strcmp(argv[1], "generations")) return generations();
	return 2;
}
