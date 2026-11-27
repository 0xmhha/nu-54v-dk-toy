#include "device_pilot.h"
#if defined(CONFIG_NU54_HD_RECOVERY)
#include "device_hd.h"
#include "nu54_hd.h"
#include "device_evm.h"
static device_evm direct;
#endif
#if defined(CONFIG_NU54_WALLET_CHAINS)
#include "device_chains.h"
static device_chains chains;
#if defined(CONFIG_NU54_HD_RECOVERY)
_Static_assert(NU54_EVM_SIWE_OP != NU54_CHAIN_XPUB_OP, "Each management request must have a unique operation");
#endif
#endif

#include "board_io.h"
#include "ble_links.h"
#include "device_install.h"
#if defined(CONFIG_NU54_SPENDING_PROFILE)
#include "device_order.h"
#include "device_spending.h"
#include <zephyr/sys/reboot.h>
#endif
#include "device_firmware_power.h"
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
#include "device_owner_limits_ps.h"
#include "device_owner_finality_ps.h"
#include "nu54_owner_limits_wire.h"
#endif
#include "device_recovery_pin.h"
#include <zephyr/sys/byteorder.h>
#include "device_tls_v2.h"
#include "device_vault.h"
#include "nu54_pilot.h"
#include "nu54_owner.h"
#include "nu54_owner_idle.h"
#include <mbedtls/pk.h>
#include <mbedtls/platform_util.h>
#include "nu54_keccak.h"
#include "nu54_sig.h"
#include "pay_link.h"
#include "status_led.h"

#include <psa/crypto.h>
#include <psa/internal_trusted_storage.h>
#include <string.h>
#include <zephyr/bluetooth/bluetooth.h>
#include <zephyr/bluetooth/conn.h>
#include <zephyr/bluetooth/gatt.h>
#include <zephyr/bluetooth/hci.h>
#include <zephyr/kernel.h>
#include <zephyr/sys/atomic.h>
#include <zephyr/sys/ring_buffer.h>

#if defined(CONFIG_NU54_SPENDING_PROFILE)
BUILD_ASSERT(IS_ENABLED(CONFIG_REBOOT), "The spending profile restarts after a reset");
#endif

#define PILOT_REFUSAL_UID ((psa_storage_uid_t)0x5530)
#define PILOT_RECEIPT_UID ((psa_storage_uid_t)0x5490)
#define PILOT_TIMEOUT_MS 120000

#define PILOT_UUID128(...) BT_UUID_128_ENCODE(__VA_ARGS__)
static struct bt_uuid_128 pilot_uuid = BT_UUID_INIT_128(PILOT_UUID128(NU54_PILOT_SERVICE_UUID_PARTS));
static struct bt_uuid_128 input_uuid = BT_UUID_INIT_128(BT_UUID_128_ENCODE(0x41ba05a0, 0xe0dd, 0x4fd7, 0x901c, 0x4c25d5010001));
static struct bt_uuid_128 output_uuid = BT_UUID_INIT_128(BT_UUID_128_ENCODE(0x41ba05a0, 0xe0dd, 0x4fd7, 0x901c, 0x4c25d5010002));

static struct bt_uuid_128 offer_uuid = BT_UUID_INIT_128(BT_UUID_128_ENCODE(0x41ba05a0, 0xe0dd, 0x4fd7, 0x901c, 0x4c25d5010003));
static struct bt_uuid_128 provision_uuid = BT_UUID_INIT_128(BT_UUID_128_ENCODE(0x41ba05a0, 0xe0dd, 0x4fd7, 0x901c, 0x4c25d5010004));
static struct bt_uuid_128 display_uuid = BT_UUID_INIT_128(BT_UUID_128_ENCODE(0x41ba05a0, 0xe0dd, 0x4fd7, 0x901c, 0x4c25d5010005));

static ssize_t read_install_nonce(struct bt_conn *, const struct bt_gatt_attr *, void *, uint16_t, uint16_t);
static ssize_t provision(struct bt_conn *, const struct bt_gatt_attr *, const void *, uint16_t, uint16_t, uint8_t);
static ssize_t read_display(struct bt_conn *, const struct bt_gatt_attr *, void *, uint16_t, uint16_t);
static ssize_t confirm_display(struct bt_conn *, const struct bt_gatt_attr *, const void *, uint16_t, uint16_t, uint8_t);
static ssize_t receive_bytes(struct bt_conn *, const struct bt_gatt_attr *, const void *, uint16_t, uint16_t, uint8_t);
BT_GATT_SERVICE_DEFINE(pilot_service, BT_GATT_PRIMARY_SERVICE(&pilot_uuid),
	BT_GATT_CHARACTERISTIC(&input_uuid.uuid, BT_GATT_CHRC_WRITE, BT_GATT_PERM_WRITE, NULL, receive_bytes, NULL),
	BT_GATT_CHARACTERISTIC(&output_uuid.uuid, BT_GATT_CHRC_NOTIFY, BT_GATT_PERM_NONE, NULL, NULL, NULL),
	BT_GATT_CCC(NULL, BT_GATT_PERM_READ | BT_GATT_PERM_WRITE),
	BT_GATT_CHARACTERISTIC(&offer_uuid.uuid, BT_GATT_CHRC_READ, BT_GATT_PERM_READ, read_install_nonce, NULL, NULL),
	BT_GATT_CHARACTERISTIC(&provision_uuid.uuid, BT_GATT_CHRC_WRITE, BT_GATT_PERM_WRITE, NULL, provision, NULL),
	BT_GATT_CHARACTERISTIC(&display_uuid.uuid, BT_GATT_CHRC_READ | BT_GATT_CHRC_WRITE,
			       BT_GATT_PERM_READ | BT_GATT_PERM_WRITE, read_display, confirm_display, NULL));

K_MUTEX_DEFINE(pilot_lock);
RING_BUF_DECLARE(pilot_rx, 8192);
static struct bt_conn *connection;
static struct k_work_delayable worker;
static mbedtls_ssl_context tls;
struct provisioned_tls {
	mbedtls_x509_crt ca, board;
	mbedtls_pk_context key;
	device_tls_v2_profile profiles[2];
};
static struct provisioned_tls tls_banks[2];
static unsigned tls_bank;
static nu54_v2_profile profile;
static nu54_v2_link link;
static nu54_local_approval approval;
static nu54_pilot_review_binding reviewed_quote;
static unsigned refusal_slot;
static nu54_owner_enrollment enrollment;
static nu54_v2_journal_entry journal_slots[128];
static int hash(const uint8_t *data, size_t length, uint8_t digest[32])
{
	size_t used;
	return psa_hash_compute(PSA_ALG_SHA_256, data, length, digest, 32, &used) == PSA_SUCCESS && used == 32 ? 0 : -1;
}
static int owner_random(uint8_t *data, size_t length)
{
	return psa_generate_random(data, length) == PSA_SUCCESS ? 0 : -1;
}

static int owner_verify(const uint8_t *der, size_t length, const uint8_t digest[32],
                        const uint8_t *signature, size_t signature_length)
{
	/* Canonical DER SPKI for id-ecPublicKey with prime256v1 and an uncompressed point. */
	static const uint8_t p256_spki[] = {
		0x30, 0x59, 0x30, 0x13, 0x06, 0x07, 0x2a, 0x86, 0x48, 0xce, 0x3d, 0x02, 0x01,
		0x06, 0x08, 0x2a, 0x86, 0x48, 0xce, 0x3d, 0x03, 0x01, 0x07, 0x03, 0x42, 0x00, 0x04,
	};
	if (length != sizeof(p256_spki) + 64 || memcmp(der, p256_spki, sizeof(p256_spki))) return -1;
	mbedtls_pk_context key;
	mbedtls_pk_init(&key);
	int rc = mbedtls_pk_parse_public_key(&key, der, length);
	if (!rc) rc = mbedtls_pk_verify(&key, MBEDTLS_MD_SHA256, digest, 32, signature, signature_length);
	mbedtls_pk_free(&key);
	return rc;
}

static const nu54_owner_crypto owner_crypto = {owner_random, hash, owner_verify};
static nu54_v2_journal journal = {journal_slots, ARRAY_SIZE(journal_slots), hash, 0};
static bool initialized, ready, bound, recovering;
static int64_t deadline, enrollment_window, recovery_window;
static nu54_owner_idle owner_idle;
static uint8_t display_challenge[32], active_quote[NU54_V2_SIGNED_SIZE];
static bool display_confirmed;
#if defined(CONFIG_NU54_SPENDING_PROFILE)
static unsigned order_slot;
static uint64_t order_amount, order_expires;
static nu54_spend_action spend_action;
/* One at a time, in RAM, kept across sessions so that the operator has time to answer. */
#define RESET_NONCE_MS (15 * 60 * 1000)
static uint8_t reset_nonce[32];
static uint64_t reset_nonce_generation;
static int64_t reset_nonce_until; /* 0: none */
#endif
static uint8_t rental_review_hash[32];
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
#define OWNER_LIMITS_READ_OP 23
#define OWNER_LIMITS_CHANGE_OP 24
#define OWNER_LIMITS_ACTIVATE_OP 25
#define OWNER_LIMITS_INITIALIZE_OP 26
#define OWNER_FINALITY_RECONCILE_OP 27
#define OWNER_FINALITY_ENROLL_OP 28
#define OWNER_FINALITY_RESUME_OP 29
#define OWNER_FINALITY_REPLACEMENT_OP 31
static nu54_local_approval limits_approval;
static nu54_limit_change limits_change;
static nu54_limit_recipient_change limits_set_change;
static uint8_t limits_statement[NU54_LIMIT_SET_STATEMENT_MAX], limits_proof[64], limits_operation;
static size_t limits_statement_length;
static bool limits_displayed;
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
static nu54_local_approval finality_approval;
static nu54_finality_enrollment finality_enrollment;
static uint8_t finality_statement[NU54_FINALITY_ENROLL_BODY_SIZE],finality_issuer_proof[64],finality_display_proof[64];
static bool finality_displayed;
#endif
static int present_limits(void *context,const uint8_t *preimage,size_t length)
{
    (void)context;
    return length == NU54_LOCAL_PREIMAGE_SIZE && preimage[0] == 0x19 && preimage[1] == 1 ? 0 : -1;
}
#endif
enum install_phase { INSTALL_IDLE, INSTALL_HANDSHAKE, INSTALL_SEND_OFFER, INSTALL_RECEIVE_GRANT, INSTALL_ACK };
static enum install_phase install_phase;
static uint8_t install_bundle[2 + NU54_INSTALL_BUNDLE_MAX];
static uint8_t install_offer[NU54_INSTALL_BOUND_OFFER_SIZE];
static uint8_t install_public[NU54_INSTALL_BOOTSTRAP_PUBLIC_SIZE];
static size_t install_used, install_total;
static int64_t install_deadline;
static mbedtls_ssl_config install_tls;
static mbedtls_x509_crt install_station;
static struct bt_conn *ticket_peer, *display_connection;
static int apply_install(const uint8_t *, size_t);
static uint8_t display_signature[64];
static size_t display_used;
static unsigned receipt_slot;
static uint8_t wire[NU54_V2_FRAME_MAX + 2];
static size_t wire_length;
static uint32_t next_prompt;
static atomic_t pending_prompt;
_Static_assert(sizeof(nu54_pilot_receipt) <= 128, "P01 pilot receipt exceeds ITS limit");
_Static_assert(sizeof(nu54_pilot_refusal) <= 128, "P01 pilot refusal exceeds ITS limit");

static uint64_t trusted_now(void)
{
	return device_install_now();
}

static void reset_display_input(void)
{
	if (display_connection) bt_conn_unref(display_connection);
	display_connection = NULL;
	display_used = 0;
}

static void reset_install_input(void)
{
	if (ticket_peer) bt_conn_unref(ticket_peer);
	ticket_peer = NULL;
	mbedtls_platform_zeroize(install_bundle, sizeof(install_bundle));
	install_used = 0;
	install_total = 0;
}

static void reset_link(void)
{
	if (install_phase != INSTALL_IDLE) {
		mbedtls_ssl_free(&tls);
		mbedtls_ssl_init(&tls);
		mbedtls_ssl_config_free(&install_tls);
		mbedtls_x509_crt_free(&install_station);
		mbedtls_platform_zeroize(install_offer, sizeof(install_offer));
		reset_install_input();
		ring_buf_reset(&pilot_rx);
		bt_conn_unref(connection);
		connection = NULL;
		install_phase = INSTALL_IDLE;
		return;
	}
	nu54_owner_idle_clear(&owner_idle);
	reset_display_input();
	memset(display_signature, 0, sizeof(display_signature));
	receipt_slot = 0;
	refusal_slot = 0;
	memset(&reviewed_quote, 0, sizeof(reviewed_quote));
	memset(rental_review_hash, 0, sizeof(rental_review_hash));
	if (atomic_get(&pending_prompt) && !pay_link_waiting()) {
		board_io_set_prompt(0);
		status_led_waiting(false);
	}
	atomic_set(&pending_prompt, 0);
	display_confirmed = false;
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	order_slot = 0;
	order_amount = 0;
	order_expires = 0;
	memset(&spend_action, 0, sizeof(spend_action));
#endif
	memset(active_quote, 0, sizeof(active_quote));
	memset(display_challenge, 0, sizeof(display_challenge));
	nu54_local_approval_cancel(&approval);
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
	nu54_local_approval_cancel(&limits_approval);
	memset(&limits_change,0,sizeof(limits_change)); memset(limits_statement,0,sizeof(limits_statement));
	memset(&limits_set_change,0,sizeof(limits_set_change)); limits_operation=0; limits_statement_length=0;
	memset(limits_proof,0,sizeof(limits_proof)); limits_displayed=false;
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
	nu54_local_approval_cancel(&finality_approval);memset(&finality_enrollment,0,sizeof(finality_enrollment));
	memset(finality_statement,0,sizeof(finality_statement));memset(finality_issuer_proof,0,sizeof(finality_issuer_proof));
	memset(finality_display_proof,0,sizeof(finality_display_proof));finality_displayed=false;
#endif
#endif
	nu54_owner_cancel(&enrollment);
#if defined(CONFIG_NU54_WALLET_CHAINS)
	device_chains_cancel(&chains);
#endif
#if defined(CONFIG_NU54_HD_RECOVERY)
	device_evm_cancel(&direct);
	if (profile == NU54_V2_MANAGEMENT) device_hd_disconnected();
#endif
	nu54_v2_link_close(&link);
	mbedtls_ssl_free(&tls);
	mbedtls_ssl_init(&tls);
	ring_buf_reset(&pilot_rx);
	bound = false;
	recovering = false;
	wire_length = 0;
	if (connection) bt_conn_unref(connection);
	connection = NULL;
}

/* All callers hold pilot_lock except the PIN guard, which acquires it. */
static bool session_live(int64_t now)
{
	if (!connection) return false;
	if (!owner_idle.generation) return now < deadline;

	const nu54_vault *vault = device_vault_owner();
	return bound && !recovering && profile == NU54_V2_MANAGEMENT && link.peer == NU54_V2_OWNER &&
	       vault->enabled && !(vault->record.flags & NU54_VAULT_REVOKED) &&
	       !memcmp(link.peer_identity, vault->record.owner, sizeof(vault->record.owner)) &&
	       nu54_owner_idle_live(&owner_idle, vault->record.generation, now);
}

static bool owner_pin_live(void *context, bool local_input, uint32_t input_prompt)
{
	uint32_t prompt = *(const uint32_t *)context;
	k_mutex_lock(&pilot_lock, K_FOREVER);
	int64_t now = k_uptime_get();
	uint64_t current = trusted_now();
	bool live = owner_idle.generation && session_live(now) && device_install_policy() &&
	            prompt == (uint32_t)atomic_get(&pending_prompt) &&
	            current && current < link.pending_expiry;
	if (live && local_input)
		live = input_prompt == prompt && nu54_owner_idle_activity(&owner_idle, owner_idle.generation, now);
	k_mutex_unlock(&pilot_lock);
	return live;
}

/* Called with the mutex held, and returns with it held. */
static int pilot_pin(bool virgin, uint32_t prompt)
{
	bool guarded = owner_idle.generation != 0;
	k_mutex_unlock(&pilot_lock);
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	/* Show the taken press first. A press in between belongs to no digit. */
	const int64_t shown = k_uptime_get() + STATUS_LED_PRESSED_MS;
	for (int64_t left = STATUS_LED_PRESSED_MS; left > 0; left = shown - k_uptime_get()) {
		board_button_event_t early;
		(void)board_button_wait(&early, (int32_t)left);
	}
#endif
	int result = guarded ? device_recovery_pin_guarded(virgin, owner_pin_live, &prompt) : device_recovery_pin(virgin);
	k_mutex_lock(&pilot_lock, K_FOREVER);
	return guarded && !session_live(k_uptime_get()) ? -1 : result;
}

static void disconnected(struct bt_conn *conn, uint8_t reason)
{
	(void)reason;
	k_mutex_lock(&pilot_lock, K_FOREVER);
	if (connection == conn) reset_link();
	if (display_connection == conn) {
		reset_display_input();
		memset(display_signature, 0, sizeof(display_signature));
	}
	if (ticket_peer == conn) reset_install_input();
	k_mutex_unlock(&pilot_lock);
}

BT_CONN_CB_DEFINE(pilot_connection_events) = {.disconnected = disconnected};

static int pull(void *context, unsigned char *data, size_t length)
{
	(void)context;
	size_t found = ring_buf_get(&pilot_rx, data, length);
	return found ? (int)found : MBEDTLS_ERR_SSL_WANT_READ;
}

static int push(void *context, const unsigned char *data, size_t length)
{
	(void)context;
	size_t capacity = bt_gatt_get_mtu(connection) - 3u;
	if (length > capacity) length = capacity;
	int result = bt_gatt_notify(connection, &pilot_service.attrs[4], data, length);
	return result == 0 ? (int)length : (result == -ENOMEM ? MBEDTLS_ERR_SSL_WANT_WRITE : MBEDTLS_ERR_SSL_INTERNAL_ERROR);
}

static int send_frame(void *context, const uint8_t *frame, size_t length)
{
	(void)context;
	uint8_t packet[NU54_V2_FRAME_MAX + 2] = {(uint8_t)(length >> 8), (uint8_t)length};
	memcpy(packet + 2, frame, length);
	return mbedtls_ssl_write(&tls, packet, length + 2) == (int)(length + 2) ? 0 : -1;
}

static int present(void *context, const uint8_t *preimage, size_t length)
{
	(void)context;
	/* Staging only captures the intent. The physical prompt waits for the signed display confirmation. */
	return length == NU54_LOCAL_PREIMAGE_SIZE && preimage[0] == 0x19 && preimage[1] == 1 &&
	       !owner_random(display_challenge, sizeof(display_challenge)) ? 0 : -1;
}

static int reclaim_receipt(nu54_v2_request *, const nu54_vault *);

#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA) && !defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
static int manage_limit_set(nu54_v2_request *request, const nu54_vault *vault)
{
	uint8_t board[32], reply[NU54_LIMIT_SET_REPLY_SIZE] = {0};
	const uint8_t *wire = request->payload;
	if (request->payload_length < 2 || wire[1] != NU54_LIMIT_SET_WIRE_VERSION ||
	    recovering || enrollment.issued || !vault->enabled || !vault->record.generation ||
	    !(vault->record.flags & NU54_VAULT_PERSONAL) || (vault->record.flags & NU54_VAULT_REVOKED) ||
	    hash(vault->record.identity, sizeof(vault->record.identity), board)) return -1;
	uint64_t generation = vault->record.generation;
	uint64_t lower = device_install_earliest(), upper = device_install_now();
	size_t length = 3;
	if (wire[0] == NU54_LIMIT_SET_ACTIVATE_OP) {
		if (request->payload_length != 26 ||
		    device_owner_limits_activate_set_ps(board, generation, nu54_limits_get64(wire + 2),
							wire + 10, lower, upper)) return -1;
		reply[0] = wire[0]; reply[1] = NU54_LIMIT_SET_WIRE_VERSION; reply[2] = 1;
	} else {
		nu54_owner_limits state;
		nu54_limit_recipients active, pending;
		enum nu54_limit_pending_kind kind;
		bool available = !device_owner_limits_read_set_ps(board, generation, &state, &active, &pending, &kind);
		if (wire[0] == NU54_LIMIT_SET_READ_OP) {
			if (request->payload_length != 2) return -1;
			reply[0] = wire[0]; reply[1] = NU54_LIMIT_SET_WIRE_VERSION;
			if (available) {
				nu54_limits_encode_set(&state, &active, &pending, (uint8_t)kind, reply);
				length = sizeof(reply);
			}
		} else {
			memset(&limits_set_change, 0, sizeof(limits_set_change));
			if (!available || nu54_limits_parse_set(wire, request->payload_length, &active, &limits_set_change) ||
			    request->expires > UINT64_MAX - NU54_LIMIT_CHANGE_DELAY || next_prompt == (NU54_PILOT_PROMPT_MASK >> 1)-1 ||
			    owner_random(limits_set_change.challenge, sizeof(limits_set_change.challenge))) return -1;
			limits_set_change.generation = generation;
			limits_set_change.chain = state.chain;
			limits_set_change.expires = request->expires;
			limits_set_change.not_before = request->expires + NU54_LIMIT_CHANGE_DELAY;
			memcpy(limits_set_change.board, board, sizeof(board));
			memcpy(limits_set_change.asset, state.asset, sizeof(state.asset));
			memcpy(limits_set_change.session, link.epoch, sizeof(link.epoch));
			memcpy(limits_set_change.request, request->id, sizeof(request->id));
			uint32_t prompt = NU54_PILOT_PROMPT_MASK | ++next_prompt;
			if (device_owner_limits_stage_recipients_ps(&limits_set_change, lower, upper, prompt,
			    &limits_approval, hash, present_limits, NULL) ||
			    nu54_limit_set_statement(&state, &active, &limits_set_change, limits_statement,
						     &limits_statement_length)) {
				nu54_local_approval_cancel(&limits_approval);
				return -1;
			}
			limits_operation = wire[0];
			limits_displayed = false;
			return 0;
		}
	}
	request->command = NU54_V2_REPLY;
	request->payload = reply;
	request->payload_length = length;
	return nu54_v2_link_send(&link, request, send_frame, NULL);
}
#endif

static bool payment_generation_current(const nu54_install_policy *policy);
static bool payment_address(const nu54_install_policy *policy, uint8_t address[20]);

static int load_refusal(void *context, unsigned slot, nu54_pilot_refusal *out)
{
	(void)context;
	size_t used;
	psa_status_t read = psa_its_get(PILOT_REFUSAL_UID + slot, 0, sizeof(*out), out, &used);
	if (read == PSA_ERROR_DOES_NOT_EXIST) return 1;
	return read == PSA_SUCCESS && used == sizeof(*out) ? 0 : -1;
}

/* Commercial owner phones never bond: the owner is the C1 TLS generation and the owner service needs
 * no link encryption. Every bond in the legacy store therefore predates commercial mode, so the
 * board removes all of them at boot and at each owner change, and the owner status reports whether
 * the store is empty. A failed removal is retried at the next owner change or boot. */
static void clear_legacy_bonds(void)
{
	(void)ble_links_unpair_all();
}

static void report_legacy_bonds(uint8_t status[NU54_OWNER_STATUS_SIZE])
{
	nu54_owner_status_bonds(status, ble_links_bond_count() == 0);
}

#if defined(CONFIG_NU54_SPENDING_PROFILE)
static int reset_nonce_issue(uint64_t generation)
{
	reset_nonce_until = 0;
	if (owner_random(reset_nonce, sizeof(reset_nonce))) return -1;
	reset_nonce_generation = generation;
	reset_nonce_until = k_uptime_get() + RESET_NONCE_MS;
	return 0;
}

/* A new TLS session ends the old one's request records, because nothing of the old one can come
 * back: each frame carries the epoch that only its own session has. A request id was never proof
 * of a new request either, as the phone picks it. What must not happen twice is kept elsewhere:
 * answered orders by request id, returns by their token nonce, the return address and the reset
 * nonce once, registration by its random challenge. The time watermark stays. */
static void journal_new_session(void)
{
	memset(journal_slots, 0, sizeof(journal_slots));
}

static int spending_request(const nu54_v2_request *request, uint64_t now, const nu54_vault *vault)
{
	uint8_t board[32], statement[NU54_SPEND_OPERATOR_STATEMENT_SIZE], known[20];
	const uint8_t op = request->payload[0];
	const bool operator_reset = op == NU54_SPEND_OPERATOR_RESET_OP;
	/* Only the operator's signed reset works for a phone that is not the enrolled owner. */
	if (enrollment.issued || !vault->enabled || !vault->record.generation ||
	    (vault->record.flags & NU54_VAULT_REVOKED) || !device_spending_address() ||
	    (!operator_reset && (recovering ||
	      memcmp(vault->record.owner, link.peer_identity, sizeof(vault->record.owner)))) ||
	    next_prompt == (NU54_PILOT_PROMPT_MASK >> 1) - 1) return -1;
	if (op != NU54_SPEND_SET_RETURN_OP && device_order_open(device_install_earliest(), k_uptime_get())) return -1;
	if (op == NU54_SPEND_RETURN_OP && !device_spending_return_address(known)) return -1;
	if (op == NU54_SPEND_SET_RETURN_OP && device_spending_return_address(known)) return -1;
	if (operator_reset) {
		bool live = k_uptime_get() < reset_nonce_until && reset_nonce_generation == vault->record.generation;
		reset_nonce_until = 0; /* one try per nonce */
		if (!live || request->payload_length != 97 || memcmp(request->payload + 1, reset_nonce, 32) ||
		    hash(vault->record.identity, sizeof(vault->record.identity), board)) return -1;
		nu54_spend_operator_statement(board, vault->record.generation, reset_nonce, statement);
		if (device_install_verify_authority(statement, sizeof(statement), request->payload + 33)) return -1;
	}
	uint32_t prompt = NU54_PILOT_PROMPT_MASK | ++next_prompt;
	if (nu54_spend_stage(&spend_action, &link, request, now, device_install_earliest(), vault->record.generation, prompt))
		return -1;
	atomic_set(&pending_prompt, (atomic_val_t)prompt);
	board_io_set_prompt(prompt);
	status_led_waiting(true);
	return 0;
}

/* Runs with the pilot lock held and does not return. */
static void spending_erase_and_reboot(void)
{
	/* An unfinished erase leaves its mark, and the restart continues it. */
	const bool erased = device_spending_erase() == 0;
	status_led_clear();
	if (erased) status_led_reset_done();
	else status_led_event(NU54_EVENT_REFUSED);
	k_msleep(4000);
	sys_reboot(SYS_REBOOT_COLD);
}

static int spending_button(uint32_t prompt, bool approved)
{
	nu54_spend_action action;
	nu54_v2_request reply = {.command = NU54_V2_REPLY, .expires = link.pending_expiry};
	const nu54_vault *vault = device_vault_owner();
	uint8_t answer[1 + 20 + 65] = {spend_action.op};
	memcpy(reply.id, link.pending_id, sizeof(reply.id));
	reply.payload = answer;
	if (!approved) {
		memset(&spend_action, 0, sizeof(spend_action));
		answer[0] = 0;
		reply.payload_length = 1;
		nu54_v2_link_send(&link, &reply, send_frame, NULL);
		return -1;
	}
	/* The operator resets a board whose PIN is forgotten, so only that action skips the PIN. */
	int pin = spend_action.op == NU54_SPEND_OPERATOR_RESET_OP ? 0 : pilot_pin(false, prompt);
	if (pin || prompt != (uint32_t)atomic_get(&pending_prompt) || !connection || pay_link_waiting() ||
	    !device_install_policy() || !vault->enabled || (vault->record.flags & NU54_VAULT_REVOKED) ||
	    (recovering && k_uptime_get() >= recovery_window) ||
	    !nu54_spend_consume(&spend_action, &link, trusted_now(), vault->record.generation, prompt, &action)) return -1;
	if (action.op == NU54_SPEND_SET_RETURN_OP) {
		if (device_spending_set_return(action.address)) return -1;
		memcpy(answer + 1, action.address, 20);
		reply.payload_length = 21;
		return nu54_v2_link_send(&link, &reply, send_frame, NULL);
	}
	if (device_order_open(device_install_earliest(), k_uptime_get())) return -1;
	if (action.op == NU54_SPEND_RETURN_OP) {
		const nu54_install_policy *policy = device_install_policy();
		uint8_t preimage[NU54_LOCAL_PREIMAGE_SIZE], digest[32], raw[64];
		const uint8_t *account = device_spending_address();
		if (!policy || !policy->allowlist.commercial_testnet || !account ||
		    !device_spending_return_address(answer + 1)) return -1;
		nu54_spend_return_preimage(policy->allowlist.token, account, answer + 1, action.amount,
					   action.valid_before, action.nonce, preimage);
		nu54_keccak256(preimage, sizeof(preimage), digest);
		if (device_spending_sign(digest, raw) ||
		    nu54_sig_finish(digest, raw, account, answer + 21) != NU54_SIG_OK) return -1;
		reply.payload_length = sizeof(answer);
		if (nu54_v2_link_send(&link, &reply, send_frame, NULL)) return -1;
		status_led_event(NU54_EVENT_SIGNED);
		return 0;
	}
	reply.payload_length = 1; /* the phone learns that the reset starts; the board then restarts */
	nu54_v2_link_send(&link, &reply, send_frame, NULL);
	reset_link();
	spending_erase_and_reboot();
	return 0;
}

void device_pilot_button_reset(void)
{
	k_mutex_lock(&pilot_lock, K_FOREVER);
	const nu54_vault *vault = device_vault_owner();
	/* A revoked owner's board can still be reset by whoever knows its PIN; nothing else could free it. */
	bool allowed = vault->enabled && vault->record.generation && !(vault->record.flags & NU54_VAULT_RELEASED) &&
		       !approval.pending && !enrollment.issued && !spend_action.pending && !pay_link_waiting() &&
		       !device_order_open(device_install_earliest(), k_uptime_get());
	if (allowed) reset_link();
	k_mutex_unlock(&pilot_lock);
	if (!allowed || device_recovery_pin(false)) {
		status_led_event(NU54_EVENT_REFUSED);
		return;
	}
	status_led_reset_armed(true);
	const int64_t until = k_uptime_get() + 10000;
	bool confirmed = false;
	while (!confirmed && k_uptime_get() < until) {
		board_button_event_t event;
		if (board_button_wait(&event, 100)) continue;
		if (event.button != 0 || event.event != NU54_BTN_LONG) break; /* anything else cancels */
		confirmed = true;
	}
	status_led_reset_armed(false);
	if (!confirmed) {
		status_led_event(NU54_EVENT_REFUSED);
		return;
	}
	k_mutex_lock(&pilot_lock, K_FOREVER);
	/* A phone may have connected while the PIN was entered. */
	if (!approval.pending && !spend_action.pending && !device_order_open(device_install_earliest(), k_uptime_get())) {
		reset_link();
		spending_erase_and_reboot();
	} else {
		status_led_event(NU54_EVENT_REFUSED);
	}
	k_mutex_unlock(&pilot_lock);
}
#endif

static int manage(nu54_v2_request *request, uint64_t now, const nu54_install_policy *policy)
{
	nu54_vault *vault = device_vault_owner();
	uint8_t status[NU54_OWNER_STATUS_SIZE + 20 + 48];
	const uint8_t *response = status;
	size_t response_length = NU54_OWNER_STATUS_SIZE;
	if (!request->payload_length || profile != NU54_V2_MANAGEMENT || approval.pending) return -1;
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
	if (limits_approval.pending) return -1;
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
	if (finality_approval.pending) return -1;
#endif
#endif
	if (recovering && (k_uptime_get() >= recovery_window ||
	    (request->payload[0] != 1 && request->payload[0] != 2 && request->payload[0] != 3 &&
	     request->payload[0] != 7
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	     && request->payload[0] != NU54_SPEND_RESET_NONCE_OP && request->payload[0] != NU54_SPEND_OPERATOR_RESET_OP
#endif
	     ))) return -1;
	switch (request->payload[0]) {
	case NU54_OWNER_IDLE_OP: {
		if (!owner_idle.generation || request->payload_length != 1 || !session_live(k_uptime_get())) return -1;
		uint8_t reply[] = {NU54_OWNER_IDLE_OP, 1, NU54_OWNER_IDLE_FLAGS, 0, NU54_OWNER_IDLE_SECONDS};
		request->command = NU54_V2_REPLY;
		request->payload = reply;
		request->payload_length = sizeof(reply);
		return nu54_v2_link_send(&link, request, send_frame, NULL);
	}

#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
#if !defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
	case NU54_LIMIT_SET_READ_OP:
	case NU54_LIMIT_SET_CHANGE_OP:
	case NU54_LIMIT_SET_CAPS_OP:
	case NU54_LIMIT_SET_ACTIVATE_OP:
		return manage_limit_set(request, vault);
#endif
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
	case OWNER_FINALITY_ENROLL_OP: {
		uint8_t board[32],address[20];
		if(recovering || enrollment.issued || !vault->enabled || !vault->record.generation ||
		   !(vault->record.flags & NU54_VAULT_PERSONAL) || (vault->record.flags & NU54_VAULT_REVOKED) ||
		   request->payload_length!=1+NU54_FINALITY_GRANT_SIZE+64 || !device_owner_finality_platform_qualified() ||
		   hash(vault->record.identity,sizeof(vault->record.identity),board) ||
		   !device_owner_limits_is_fresh_ps(board,vault->record.generation) || !device_hd_public_address(address) ||
		   nu54_finality_grant_parse(request->payload+1,NU54_FINALITY_GRANT_SIZE,&finality_enrollment) ||
		   memcmp(finality_enrollment.board,board,32) || finality_enrollment.generation!=vault->record.generation ||
		   memcmp(finality_enrollment.signer,address,20) || request->expires>finality_enrollment.expires ||
		   next_prompt==(NU54_PILOT_PROMPT_MASK >> 1)-1 || owner_random(finality_enrollment.challenge,32))return -1;
		memcpy(finality_enrollment.session,link.epoch,16);memcpy(finality_enrollment.request,request->id,16);
		memcpy(finality_issuer_proof,request->payload+1+NU54_FINALITY_GRANT_SIZE,64);
		uint32_t prompt=NU54_PILOT_PROMPT_MASK | ++next_prompt;
		if(nu54_finality_enrollment_stage(&finality_enrollment,device_install_earliest(),device_install_now(),prompt,&finality_approval,
		   hash,device_install_verify_authority,finality_issuer_proof,device_install_independent_finality_key,present_limits,NULL) ||
		   nu54_finality_enrollment_body(&finality_enrollment,finality_statement)) {nu54_local_approval_cancel(&finality_approval);return -1;}
		finality_displayed=false;return 0;
	}
	case OWNER_FINALITY_RESUME_OP: {
		uint8_t board[32],address[20],reply[2]={OWNER_FINALITY_RESUME_OP,0};
		if(request->payload_length!=1 || recovering || !vault->enabled || !vault->record.generation ||
		   !(vault->record.flags & NU54_VAULT_PERSONAL) || (vault->record.flags & NU54_VAULT_REVOKED) ||
		   hash(vault->record.identity,sizeof(vault->record.identity),board) || !device_hd_public_address(address))return -1;
		int result=device_owner_finality_resume_ps(board,vault->record.generation,address);
		if(result!=0 && result!=NU54_FINALITY_ALREADY)return -1;
		reply[1]=(uint8_t)(result==NU54_FINALITY_ALREADY?2:1);request->command=NU54_V2_REPLY;
		request->payload=reply;request->payload_length=2;return nu54_v2_link_send(&link,request,send_frame,NULL);
	}
#endif
	case OWNER_FINALITY_REPLACEMENT_OP: {
		uint8_t board[32],reply[2]={OWNER_FINALITY_REPLACEMENT_OP,0};nu54_finality_claim claim;
		const size_t header=1+NU54_FINALITY_REPLACEMENT_BODY_SIZE+64+4;
		if(recovering || enrollment.issued || !vault->enabled || !vault->record.generation ||
		   !(vault->record.flags & NU54_VAULT_PERSONAL) || (vault->record.flags & NU54_VAULT_REVOKED) ||
		   request->payload_length<header || hash(vault->record.identity,sizeof(vault->record.identity),board) ||
		   nu54_finality_replacement_parse(request->payload+1,NU54_FINALITY_REPLACEMENT_BODY_SIZE,&claim) ||
		   memcmp(claim.board,board,32) || claim.generation!=vault->record.generation)return -1;
		size_t original_len=sys_get_be16(request->payload+header-4),replacement_len=sys_get_be16(request->payload+header-2);
		if(!original_len || original_len>NU54_EVM_TX_MAX || !replacement_len || replacement_len>NU54_EVM_TX_MAX ||
		   request->payload_length!=header+original_len+replacement_len)return -1;
		int result=device_owner_finality_reconcile_replacement_ps(&claim,request->payload+header,original_len,
		  request->payload+header+original_len,replacement_len,request->payload+1+NU54_FINALITY_REPLACEMENT_BODY_SIZE,
		  device_install_earliest(),device_install_now());
		if(result!=0 && result!=NU54_FINALITY_ALREADY)return -1;
		reply[1]=(uint8_t)(result==NU54_FINALITY_ALREADY?2:1);request->command=NU54_V2_REPLY;
		request->payload=reply;request->payload_length=2;return nu54_v2_link_send(&link,request,send_frame,NULL);
	}
	case OWNER_FINALITY_RECONCILE_OP: {
		/* The signed finality claim is checked against the active owner on this
		 * authenticated management channel.  The v2 PS adapter remains
		 * unqualified and unprovisioned by default. */
		uint8_t board[32],reply[2]={OWNER_FINALITY_RECONCILE_OP,0};nu54_finality_claim claim;
		const size_t header=1+NU54_FINALITY_BODY_SIZE+64+2;
		if(recovering || enrollment.issued || !vault->enabled || !vault->record.generation ||
		   !(vault->record.flags & NU54_VAULT_PERSONAL) || (vault->record.flags & NU54_VAULT_REVOKED) ||
		   request->payload_length<header || hash(vault->record.identity,sizeof(vault->record.identity),board) ||
		   nu54_finality_parse(request->payload+1,NU54_FINALITY_BODY_SIZE,&claim) ||
		   memcmp(claim.board,board,32) || claim.generation!=vault->record.generation)return -1;
		size_t raw_length=sys_get_be16(request->payload+header-2);
		if(!raw_length || raw_length>NU54_EVM_TX_MAX || request->payload_length!=header+raw_length)return -1;
		int result=device_owner_finality_reconcile_ps(&claim,request->payload+header,raw_length,
		  request->payload+1+NU54_FINALITY_BODY_SIZE,device_install_earliest(),device_install_now());
		if(result!=0 && result!=NU54_FINALITY_ALREADY)return -1;
		reply[1]=(uint8_t)(result==NU54_FINALITY_ALREADY ? 2 : 1);request->command=NU54_V2_REPLY;
		request->payload=reply;request->payload_length=sizeof(reply);return nu54_v2_link_send(&link,request,send_frame,NULL);
	}
	case OWNER_LIMITS_READ_OP:
	case OWNER_LIMITS_CHANGE_OP:
	case OWNER_LIMITS_ACTIVATE_OP:
	case OWNER_LIMITS_INITIALIZE_OP: {
		uint8_t board[32], reply[176]={0};
		if (recovering || enrollment.issued || !vault->enabled || !vault->record.generation ||
		    !(vault->record.flags & NU54_VAULT_PERSONAL) || (vault->record.flags & NU54_VAULT_REVOKED) ||
		    hash(vault->record.identity,sizeof(vault->record.identity),board)) return -1;
		nu54_owner_limits stored;
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
		nu54_finality_state finality_state;
		int available=device_owner_finality_read_ps(board,vault->record.generation,&finality_state)==0;
		if(available)stored=finality_state.policy;
		if(request->payload[0]!=OWNER_LIMITS_READ_OP)return -1; /* V1 policy writes do not operate on V2. */
#else
		int available=device_owner_limits_read_ps(board,vault->record.generation,&stored)==0;
#endif
		if (request->payload[0] == OWNER_LIMITS_READ_OP) {
			if(request->payload_length!=1)return -1;
			reply[0]=OWNER_LIMITS_READ_OP;
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
			reply[1]=available ? 1 : (device_owner_finality_pending_ps(board,vault->record.generation) ? 4 :
              (device_owner_finality_platform_qualified() && device_owner_limits_is_fresh_ps(board,vault->record.generation) ? 3 : 0));
#else
			reply[1]=available ? 1 : (device_owner_limits_is_fresh_ps(board,vault->record.generation) ? 2 : 0);
#endif
			if (!available) {request->command=NU54_V2_REPLY; request->payload=reply; request->payload_length=2; return nu54_v2_link_send(&link,request,send_frame,NULL);}
			size_t off=2;
			memcpy(reply+off,board,32); off+=32;
			sys_put_be64(stored.generation,reply+off);off+=8; sys_put_be64(stored.revision,reply+off);off+=8;
			sys_put_be64(stored.chain,reply+off);off+=8; memcpy(reply+off,stored.asset,20);off+=20;
			memcpy(reply+off,stored.destination,20);off+=20; sys_put_be64(stored.per_transfer,reply+off);off+=8;
			sys_put_be64(stored.daily,reply+off);off+=8; sys_put_be64(stored.day_spent,reply+off);off+=8;
			reply[off++]=stored.change.pending;
			if(stored.change.pending) {sys_put_be64(stored.change.per_transfer,reply+off);off+=8;
			 sys_put_be64(stored.change.daily,reply+off);off+=8; sys_put_be64(stored.change.not_before,reply+off);off+=8;
			 memcpy(reply+off,stored.change.destination,20);off+=20;}
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
			reply[off++]=2; /* Explicit V2 capability, never interpreted as V1. */
#endif
			request->command=NU54_V2_REPLY;request->payload=reply;request->payload_length=off;
			return nu54_v2_link_send(&link,request,send_frame,NULL);
		}
		if (request->payload[0] == OWNER_LIMITS_INITIALIZE_OP) {
			if(available || nu54_limits_parse_initialize(request->payload,request->payload_length,&limits_change) || next_prompt==(NU54_PILOT_PROMPT_MASK >> 1)-1 ||
			   owner_random(limits_change.challenge,32))return -1;
#if defined(CONFIG_NU54_HD_RECOVERY)
			uint8_t fresh_address[20]; if(!device_hd_public_address(fresh_address))return -1;
#else
			return -1;
#endif
			limits_change.initialize=1;limits_change.generation=vault->record.generation;limits_change.revision=1;
			limits_change.expires=request->expires;limits_change.not_before=0;
			memcpy(limits_change.board,board,32);memcpy(limits_change.session,link.epoch,16);memcpy(limits_change.request,request->id,16);
			nu54_owner_limits baseline={.generation=limits_change.generation,.revision=1,.chain=limits_change.chain,
			 .per_transfer=limits_change.per_transfer,.daily=limits_change.daily};
			memcpy(baseline.board,board,32);memcpy(baseline.asset,limits_change.asset,20);memcpy(baseline.destination,limits_change.destination,20);
			uint32_t prompt=NU54_PILOT_PROMPT_MASK | ++next_prompt;
			if(device_owner_limits_initialize_stage_ps(&limits_change,device_install_earliest(),device_install_now(),prompt,
			 &limits_approval,hash,present_limits,NULL) || nu54_limit_change_statement(&baseline,&limits_change,limits_statement)) {
			 nu54_local_approval_cancel(&limits_approval);return -1;}
			limits_operation=OWNER_LIMITS_INITIALIZE_OP;limits_statement_length=NU54_LIMIT_CHANGE_STATEMENT_SIZE;
			limits_displayed=false;return 0;
		}
		if (!available) return -1;
		if (request->payload[0] == OWNER_LIMITS_ACTIVATE_OP) {
			if(request->payload_length!=1 || device_owner_limits_activate_ps(device_install_earliest(),device_install_now()))return -1;
			reply[0]=OWNER_LIMITS_ACTIVATE_OP; reply[1]=1; request->command=NU54_V2_REPLY;
			request->payload=reply;request->payload_length=2;return nu54_v2_link_send(&link,request,send_frame,NULL);
		}
		if(nu54_limits_parse_change(request->payload,request->payload_length,&limits_change) || next_prompt==(NU54_PILOT_PROMPT_MASK >> 1)-1 ||
		   request->expires>UINT64_MAX-NU54_LIMIT_CHANGE_DELAY || owner_random(limits_change.challenge,32))return -1;
		limits_change.initialize=0;limits_change.generation=stored.generation; limits_change.revision=stored.revision; limits_change.chain=stored.chain;
		limits_change.expires=request->expires; limits_change.not_before=request->expires+NU54_LIMIT_CHANGE_DELAY;
		memcpy(limits_change.board,board,32);memcpy(limits_change.asset,stored.asset,20);
		memcpy(limits_change.session,link.epoch,16);
		memcpy(limits_change.request,request->id,16);
		uint32_t prompt=NU54_PILOT_PROMPT_MASK | ++next_prompt;
		if(device_owner_limits_stage_ps(&limits_change,device_install_earliest(),device_install_now(),prompt,
		   &limits_approval,hash,present_limits,NULL) ||
		   nu54_limit_change_statement(&stored,&limits_change,limits_statement)) {nu54_local_approval_cancel(&limits_approval);return -1;}
		limits_operation=OWNER_LIMITS_CHANGE_OP;limits_statement_length=NU54_LIMIT_CHANGE_STATEMENT_SIZE;
		limits_displayed=false;
		return 0;
	}
#endif
#if defined(CONFIG_NU54_WALLET_CHAINS)
	case NU54_CHAIN_PUBLIC_OP:
	case NU54_CHAIN_BIP48_STATUS:
	case NU54_CHAIN_BIP48_REGISTER:
	case NU54_CHAIN_BIP48_SIGN:
	case NU54_CHAIN_TRANSFER_OP:
	case NU54_CHAIN_XPUB_OP: {
		uint8_t board[32], reply[NU54_CHAIN_INDEXED_REPLY_SIZE];
		if (recovering || enrollment.issued ||
		    !vault->enabled || !vault->record.generation ||
		    (vault->record.flags & NU54_VAULT_REVOKED)) return -1;
		if ((request->payload[0] == NU54_CHAIN_PUBLIC_OP || request->payload[0] == NU54_CHAIN_BIP48_STATUS)) {
			size_t used;
			if (device_chains_public(request, reply, &used)) return -1;
			request->command = NU54_V2_REPLY;
			request->payload = reply;
			request->payload_length = used;
			return nu54_v2_link_send(&link, request, send_frame, NULL);
		}
		if (next_prompt == (NU54_PILOT_PROMPT_MASK >> 1) - 1 ||
		    hash(vault->record.identity, sizeof(vault->record.identity), board)) return -1;
		uint32_t prompt = NU54_PILOT_PROMPT_MASK | ++next_prompt;
		return device_chains_stage(&chains, request, &link, MAX(now, device_install_wallet_replay_floor()), vault->record.generation, prompt, board);
	}
#endif
#if defined(CONFIG_NU54_HD_RECOVERY)
	case 12:
		if (request->payload_length != 1 || enrollment.issued || !vault->record.generation ||
		    device_install_public_authority(status + 1)) return -1;
		status[0] = 12;
		sys_put_be64(policy->revision, status + 66);
		request->command = NU54_V2_REPLY;
		request->payload = status;
		request->payload_length = 74;
		return nu54_v2_link_send(&link, request, send_frame, NULL);
	case NU54_EVM_MARKET_ADDRESS_OP:
		if (request->payload_length != 1 || recovering || enrollment.issued || !device_hd_market_address(status + 1)) return -1;
		status[0] = NU54_EVM_MARKET_ADDRESS_OP;
		request->command = NU54_V2_REPLY;
		request->payload = status;
		request->payload_length = 21;
		return nu54_v2_link_send(&link, request, send_frame, NULL);
	case NU54_EVM_MARKET_OP:
	case NU54_EVM_TRANSFER_OP:
	case NU54_EVM_REVOKE_OP:
	case NU54_EVM_EXTERNAL_OP:
	case NU54_EVM_DAPP_ALLOWANCE_OP:
	case NU54_EVM_SIWE_OP: {
		uint8_t address[20], board[32];
		bool market_scope = request->payload[0] == NU54_EVM_MARKET_OP;
		if (recovering || enrollment.issued || !(market_scope ? device_hd_market_address(address) : device_hd_public_address(address)) ||
		    next_prompt == (NU54_PILOT_PROMPT_MASK >> 1) - 1 ||
		    hash(vault->record.identity, sizeof(vault->record.identity), board)) return -1;
		uint32_t prompt = NU54_PILOT_PROMPT_MASK | ++next_prompt;
		if (market_scope)
			return device_evm_stage_market(&direct, request, &link, MAX(now, device_install_wallet_replay_floor()),
						       vault->record.generation, prompt, board, address);
		if (request->payload[0] == NU54_EVM_SIWE_OP)
			return device_evm_stage_login(&direct, request, &link, now, vault->record.generation, prompt, board, address);
		return device_evm_stage(&direct, request, &link, MAX(now, device_install_wallet_replay_floor()), vault->record.generation, prompt, board, address);
	}
#endif
#if defined(CONFIG_NU54_HD_RECOVERY)
	case 4:
	case 5:
	case 6: {
		uint8_t proof[1 + NU54_RECOVERY_ENVELOPE_MAX];
		size_t length;
		if (enrollment.issued || device_hd_command(&link, request, now, proof, &length)) return -1;
		request->command = NU54_V2_REPLY;
		request->payload = proof;
		request->payload_length = length;
		return nu54_v2_link_send(&link, request, send_frame, NULL);
	}
#endif
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	case NU54_SPEND_STATUS_OP: {
		uint8_t answer[NU54_SPEND_STATUS_SIZE] = {NU54_SPEND_STATUS_OP};
		const uint8_t *account = device_spending_address();
		int32_t wait = device_recovery_pin_wait();
		if (request->payload_length != 1 || !account || wait < 0) return -1;
		answer[1] = device_spending_return_address(answer + 22);
		memcpy(answer + 2, account, 20);
		sys_put_be64(policy->allowlist.max_amount, answer + 42);
		sys_put_be64(CONFIG_NU54_SPENDING_DAILY_CAP, answer + 50);
		sys_put_be32((uint32_t)wait, answer + 58);
		answer[62] = device_order_open(device_install_earliest(), k_uptime_get());
		sys_put_be64(device_order_daily_left(device_install_earliest()), answer + 63);
		request->command = NU54_V2_REPLY;
		request->payload = answer;
		request->payload_length = sizeof(answer);
		return nu54_v2_link_send(&link, request, send_frame, NULL);
	}
	case NU54_SPEND_RESET_NONCE_OP: {
		uint8_t answer[33] = {NU54_SPEND_RESET_NONCE_OP};
		if (request->payload_length != 1 || enrollment.issued || !vault->record.generation ||
		    reset_nonce_issue(vault->record.generation)) return -1;
		memcpy(answer + 1, reset_nonce, 32);
		request->command = NU54_V2_REPLY;
		request->payload = answer;
		request->payload_length = sizeof(answer);
		return nu54_v2_link_send(&link, request, send_frame, NULL);
	}
	case NU54_SPEND_SET_RETURN_OP:
	case NU54_SPEND_RETURN_OP:
	case NU54_SPEND_RESET_OP:
	case NU54_SPEND_OPERATOR_RESET_OP:
		return spending_request(request, now, vault);
	case NU54_ACCOUNT_PROOF_OP: {
		uint8_t proof[NU54_ACCOUNT_PROOF_REPLY_SIZE] = {NU54_ACCOUNT_PROOF_OP};
		uint8_t preimage[NU54_LOCAL_PREIMAGE_SIZE], statement[NU54_ACCOUNT_STATEMENT_SIZE], digest[32], raw[64];
		const uint8_t *account = device_spending_address(), *challenge = request->payload + 1;
		if (!account || request->payload_length != NU54_ACCOUNT_PROOF_REQUEST_SIZE || recovering || enrollment.issued ||
		    !vault->enabled || !vault->record.generation || (vault->record.flags & NU54_VAULT_REVOKED) ||
		    link.peer != NU54_V2_OWNER ||
		    memcmp(vault->record.owner, link.peer_identity, sizeof(vault->record.owner)) ||
		    !policy->allowlist.commercial_testnet || policy->payer_role != NU54_V2_OWNER_ROLE ||
		    request->expires <= now || request->expires - now > NU54_PILOT_BRIEF_SECONDS ||
		    !nu54_account_preimage(account, challenge, preimage)) return -1;
		nu54_keccak256(preimage, sizeof(preimage), digest);
		if (device_spending_sign(digest, raw) ||
		    nu54_sig_finish(digest, raw, account, proof + 21) != NU54_SIG_OK) return -1;
		memcpy(proof + 1, account, 20);
		nu54_account_statement(account, challenge, proof + 21, statement);
		if (device_install_account_statement(statement, sizeof(statement), proof + 86)) return -1;
		request->command = NU54_V2_REPLY;
		request->payload = proof;
		request->payload_length = sizeof(proof);
		return nu54_v2_link_send(&link, request, send_frame, NULL);
	}
#endif
	case NU54_RUNNING_FIRMWARE_OP: {
		uint8_t running[NU54_RUNNING_FIRMWARE_REPLY_SIZE] = {NU54_RUNNING_FIRMWARE_OP};
		uint64_t generation = vault->record.generation;
		if (recovering || enrollment.issued || !vault->enabled ||
		    (vault->record.flags & NU54_VAULT_REVOKED) || link.peer != NU54_V2_OWNER ||
		    memcmp(vault->record.owner, link.peer_identity, sizeof(vault->record.owner)) ||
		    request->expires <= now || request->expires - now > NU54_PILOT_BRIEF_SECONDS ||
		    !nu54_running_firmware_request(request->payload, request->payload_length, generation)) return -1;
		size_t size = device_install_running_firmware(generation, running) ? 1 : sizeof(running);
		if (trusted_now() >= request->expires || !vault->enabled || vault->record.generation != generation ||
		    (vault->record.flags & NU54_VAULT_REVOKED) ||
		    memcmp(vault->record.owner, link.peer_identity, sizeof(vault->record.owner))) return -1;
		request->command = NU54_V2_REPLY;
		request->payload = running;
		request->payload_length = size;
		return nu54_v2_link_send(&link, request, send_frame, NULL);
	}
	case NU54_FIRMWARE_POWER_OP: {
		uint8_t reply[NU54_FIRMWARE_POWER_REPLY_SIZE], board[32];
		uint64_t generation = vault->record.generation;
		if (recovering || enrollment.issued || !vault->enabled ||
		    (vault->record.flags & NU54_VAULT_REVOKED) || link.peer != NU54_V2_OWNER ||
		    memcmp(vault->record.owner, link.peer_identity, sizeof(vault->record.owner)) ||
		    request->expires <= now || request->expires - now > NU54_PILOT_BRIEF_SECONDS ||
		    !nu54_firmware_power_request(request->payload, request->payload_length, generation)) return -1;
		enum nu54_firmware_vin vin = device_firmware_vin();
		if (trusted_now() >= request->expires || !vault->enabled || vault->record.generation != generation ||
		    (vault->record.flags & NU54_VAULT_REVOKED) ||
		    memcmp(vault->record.owner, link.peer_identity, sizeof(vault->record.owner)) ||
		    hash(vault->record.identity, sizeof(vault->record.identity), board) ||
		    nu54_firmware_power_reply(vin, generation, board, reply)) return -1;
		request->command = NU54_V2_REPLY;
		request->payload = reply;
		request->payload_length = sizeof(reply);
		return nu54_v2_link_send(&link, request, send_frame, NULL);
	}
    case 16: {
        uint8_t proof[181];
        if (request->payload_length != 49 || recovering || enrollment.issued ||
            (vault->record.flags & NU54_VAULT_REVOKED) ||
            device_install_update_ack(request->payload + 1, proof + 1)) return -1;
        proof[0] = 16;
        request->command = NU54_V2_REPLY;
        request->payload = proof;
        request->payload_length = sizeof(proof);
        return nu54_v2_link_send(&link, request, send_frame, NULL);
    }
#if defined(CONFIG_NU54_EXPERIMENTAL_RENTAL_JOURNAL_PS)
	case NU54_RENTAL_INSTALL_OP: {
		uint8_t proof[1 + NU54_RENTAL_INSTALL_WIRE_SIZE];
		if (request->payload_length != sizeof(proof) || recovering || enrollment.issued ||
		    link.peer != NU54_V2_OWNER || memcmp(link.peer_identity, vault->record.owner, sizeof(vault->record.owner)) ||
		    device_install_rental_ack(request->payload + 1, proof + 1)) return -1;
		proof[0] = NU54_RENTAL_INSTALL_OP;
		request->command = NU54_V2_REPLY;
		request->payload = proof;
		request->payload_length = sizeof(proof);
		return nu54_v2_link_send(&link, request, send_frame, NULL);
	}
#endif

	case 0x70: /* Owner device tool capabilities */
	case 0x71: /* Request a bounded LED indication */
	case 0x72: { /* Stop only the locate indication */
		const uint8_t op = request->payload[0];
		if (request->payload_length != 1 || request->expires <= now || request->expires - now > NU54_PILOT_BRIEF_SECONDS ||
		    recovering || enrollment.issued || !vault->enabled ||
		    !vault->record.generation || link.peer != NU54_V2_OWNER ||
		    (vault->record.flags & NU54_VAULT_REVOKED) || memcmp(vault->record.owner, link.peer_identity, 32))
			return -1;
		if (!(vault->record.flags & NU54_VAULT_PERSONAL)) {
#if defined(CONFIG_NU54_HD_RECOVERY)
			/* The personal HD root is recorded outside the legacy vault. */
			memset(status, 0, NU54_OWNER_STATUS_SIZE);
			device_hd_public_status(status);
			if (status[2] != NU54_HD_FORMAT || status[3] != NU54_HD_PROFILE) return -1;
#else
			return -1;
#endif
		}
		/* This board has LEDs and no user audio transport. */
		uint8_t reply[2] = {op, op == 0x70 || status_led_locate(op == 0x71)};
		request->command = NU54_V2_REPLY;
		request->payload = reply;
		request->payload_length = sizeof(reply);
		return nu54_v2_link_send(&link, request, send_frame, NULL);
	}
	case NU54_V2_APPROVAL_PROGRESS_OP: {
		uint64_t generation = vault->record.generation;
		if (recovering || enrollment.issued || !vault->enabled ||
		    (vault->record.flags & NU54_VAULT_REVOKED) ||
		    !payment_generation_current(policy) || policy->payer_role != NU54_V2_OWNER_ROLE ||
		    link.peer != NU54_V2_OWNER || memcmp(vault->record.owner, link.peer_identity, 32) ||
		    link.approval_generation || request->expires <= now || request->expires - now > NU54_PILOT_BRIEF_SECONDS ||
		    !nu54_v2_approval_progress_offer(request->payload, request->payload_length, generation))
			return -1;
		request->command = NU54_V2_REPLY;
		if (nu54_v2_link_send(&link, request, send_frame, NULL)) return -1;
		return nu54_v2_link_enable_approval_progress(&link, generation);
	}
	case NU54_V2_PAYMENT_REVIEW_OP: {
		uint8_t payer[20], evidence[NU54_V2_PAYMENT_REVIEW_REPLY_SIZE];
		uint8_t preimage[NU54_LOCAL_PREIMAGE_SIZE], digest[32];
		nu54_v2_quote_policy order;
		unsigned slot;
		memset(&reviewed_quote, 0, sizeof(reviewed_quote));
		const uint8_t *original, *quote;
		if (recovering || enrollment.issued || !vault->record.generation ||
		    (vault->record.flags & NU54_VAULT_REVOKED) ||
		    memcmp(vault->record.owner, link.peer_identity, sizeof(vault->record.owner)) ||
		    !payment_generation_current(policy) ||
		    !payment_address(policy, payer) ||
		    nu54_v2_payment_review_decode(request->payload, request->payload_length, &original, &quote)) return -1;
		if (nu54_install_quote(policy, quote, NU54_V2_SIGNED_SIZE, now, &order) ||
		    nu54_pilot_verify_quote(quote, NU54_V2_SIGNED_SIZE, &order, &policy->allowlist,
					   payer, policy->payer_generation) ||
		    !nu54_pilot_approval_preimage(quote, preimage) || hash(preimage, sizeof(preimage), digest)) return -1;
		int refusal = nu54_pilot_refusal_find(load_refusal, NULL, device_install_earliest(), original, digest, &slot);
		if (refusal == NU54_PILOT_RECEIPT_NEW) {
			if (nu54_pilot_review_bind(&reviewed_quote, &link, original, quote, hash)) return -1;
		} else if (refusal != NU54_PILOT_RECEIPT_RECORDED) return -1;
		if (nu54_v2_payment_review_reply(request->payload, request->payload_length,
					       refusal == NU54_PILOT_RECEIPT_NEW, hash, evidence)) return -1;
		request->command = NU54_V2_REPLY;
		request->payload = evidence;
		request->payload_length = sizeof(evidence);
		return nu54_v2_link_send(&link, request, send_frame, NULL);
	}
	case 1:
#if defined(CONFIG_NU54_HD_RECOVERY)
		if (device_hd_pending()) return -1;
#endif
		if (request->payload_length != 34 || !device_install_independent_peer(request->payload + 2)) return -1;
		if (recovering ? nu54_owner_begin_lost(&enrollment, vault, &owner_crypto, &link, request, now) :
		    nu54_owner_begin(&enrollment, vault, &owner_crypto, &link, request, now, policy->owner))
			return -1;
		response = enrollment.record;
		response_length = sizeof(enrollment.record);
		break;
	case 2: {
		if (next_prompt == (NU54_PILOT_PROMPT_MASK >> 1) - 1) return -1;
		uint32_t prompt = NU54_PILOT_PROMPT_MASK | ++next_prompt;
		if (nu54_owner_prove(&enrollment, vault, &owner_crypto, &link, request, now, prompt)) return -1;
		if (!recovering) {
			atomic_set(&pending_prompt, (atomic_val_t)prompt);
			board_io_set_prompt(prompt);
			status_led_waiting(true);
		}
		return 0; /* Reply follows the independent proof (recovery only), PIN and physical outcome. */
	}
	case 3:
		if (request->payload_length != 1) return -1;
		break;
	case 8:
		if (enrollment.issued || !vault->record.generation) return -1;
		return reclaim_receipt(request, vault);
	case 7:
		if (!vault->record.generation) return -1;
		response_length += 20;
		if (request->payload_length == 41) {
			const uint64_t expiry = sys_get_be64(request->payload + 33);
			if (recovering || enrollment.issued || (vault->record.flags & NU54_VAULT_REVOKED) ||
			    device_install_fence_wallet_expiry(expiry)) return -1;
			memcpy(status + response_length, request->payload + 1, 40);
			sys_put_be64(MAX(device_install_earliest(), device_install_wallet_replay_floor()), status + response_length + 40);
			response_length += 48;
		} else if (request->payload_length != 1) {
			return -1;
		}
#if defined(CONFIG_NU54_SPENDING_PROFILE)
		if (!device_spending_address()) return -1;
		memcpy(status + NU54_OWNER_STATUS_SIZE, device_spending_address(), 20);
		break;
#endif
		if (vault->record.flags & NU54_VAULT_PERSONAL)
			memcpy(status + NU54_OWNER_STATUS_SIZE, vault->record.personal, 20);
#if defined(CONFIG_NU54_HD_RECOVERY)
		else if (device_hd_public_address(status + NU54_OWNER_STATUS_SIZE)) {}
#endif
		else memcpy(status + NU54_OWNER_STATUS_SIZE, device_install_payer(), 20);
		break;
	default:
		return -1;
	}
	if (response == status) {
		if (nu54_owner_status(vault, &owner_crypto, &enrollment, status)) return -1;
#if defined(CONFIG_NU54_HD_RECOVERY)
		device_hd_public_status(status);
#endif
		report_legacy_bonds(status); /* after the HD status, which rewrites byte 4 */
	}
	request->command = NU54_V2_REPLY;
	request->payload = response;
	request->payload_length = response_length;
	return nu54_v2_link_send(&link, request, send_frame, NULL);
}

static int load_receipt(void *context, unsigned slot, nu54_pilot_receipt *out)
{
	(void)context;
	size_t used;
	psa_status_t read = psa_its_get(PILOT_RECEIPT_UID + slot, 0, sizeof(*out), out, &used);
	if (read == PSA_ERROR_DOES_NOT_EXIST) return 1;
	return read == PSA_SUCCESS && used == sizeof(*out) && out->complete == 1 ? 0 : -1;
}

static int send_receipt(nu54_v2_request *request, const uint8_t *signature, size_t length)
{
	request->command = NU54_V2_REPLY;
	request->payload = signature;
	request->payload_length = length;
	return nu54_v2_link_send(&link, request, send_frame, NULL);
}

#if !defined(CONFIG_NU54_SPENDING_PROFILE)
/* A W10 decision is emitted only by the physical handler after durable commitment. */
static int send_payment_decision(nu54_v2_request *request, uint64_t generation, const uint8_t *signature)
{
	if (!link.pending_rental_review) {
		const uint8_t refused = 0;
		return signature ? send_receipt(request, signature, 65) : send_receipt(request, &refused, 1);
	}
	uint8_t decision[NU54_W10_DECISION_SIZE];
	if (device_install_rental_decision(&link, generation, rental_review_hash, signature, decision))
		return -1;
	memset(rental_review_hash, 0, sizeof(rental_review_hash));
	return send_receipt(request, decision, sizeof(decision));
}
#endif


/* Reclaim only after an independent signed disposition for the exact archived approval. */
static int reclaim_receipt(nu54_v2_request *request, const nu54_vault *vault)
{
	const size_t fields = 1 + 16 + NU54_V2_UNSIGNED_SIZE;
	if (request->payload_length != fields + 1 + 64) return -1;
	const uint8_t *original = request->payload + 1;
	const uint8_t *quote = original + 16;
	uint8_t disposition = request->payload[fields];
	uint8_t board[32], statement[NU54_PILOT_RECLAIM_BODY_SIZE];
	if (hash(vault->record.identity, sizeof(vault->record.identity), board)) return -1;
	for (unsigned slot = 1; slot <= NU54_PILOT_RECEIPTS; ++slot) {
		nu54_pilot_receipt saved;
		int read = load_receipt(NULL, slot, &saved);
		if (read < 0) return -1;
		if (read || memcmp(saved.request, original, sizeof(saved.request))) continue;
		if (nu54_pilot_reclaim_body(board, &saved, quote, device_install_earliest(), disposition, hash, statement) ||
		    device_install_verify_authority(statement, sizeof(statement), request->payload + fields + 1) ||
		    device_install_fence_receipt_expiry(sys_get_be64(quote + NU54_V2_EXPIRES_AT_OFFSET + 24)) ||
		    psa_its_remove(PILOT_RECEIPT_UID + slot) != PSA_SUCCESS) return -1;
		const uint8_t reclaimed = 1;
		return send_receipt(request, &reclaimed, sizeof(reclaimed));
	}
	const uint8_t absent = 0;
	return send_receipt(request, &absent, sizeof(absent));
}

static int receipt_status(nu54_v2_request *request, const nu54_install_policy *policy)
{
	if (request->payload_length != 49 || request->payload[0] != 1 || approval.pending || enrollment.issued)
		return -1;
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	ARG_UNUSED(policy);
	nu54_order_record answer;
	unsigned slot;
	int known = device_order_find(device_install_earliest(), request->payload + 1, request->payload + 17, &answer, &slot);
	if (known == NU54_PILOT_RECEIPT_ERROR) return -1;
	if (known == NU54_PILOT_RECEIPT_RECORDED && answer.outcome == NU54_ORDER_APPROVED)
		return send_receipt(request, answer.signature, sizeof(answer.signature));
	const uint8_t none = 0;
	return send_receipt(request, &none, sizeof(none));
#else
	nu54_pilot_receipt saved;
	unsigned free_slot;
	int found = nu54_pilot_receipt_find(load_receipt, NULL, policy->payer_generation,
					   request->payload + 1, request->payload + 17, &saved, &free_slot);
	if (found == NU54_PILOT_RECEIPT_ERROR) return -1;
	if (found == NU54_PILOT_RECEIPT_RECORDED)
		return send_receipt(request, saved.signature, sizeof(saved.signature));
	const uint8_t unknown = 0;
	return send_receipt(request, &unknown, sizeof(unknown));
#endif
}

static bool payment_generation_current(const nu54_install_policy *policy)
{
	if (policy->payer_role != NU54_V2_OWNER_ROLE) return true;
	const nu54_vault *vault = device_vault_owner();
	return vault->enabled && !(vault->record.flags & NU54_VAULT_REVOKED) &&
	       vault->record.generation == policy->payer_generation;
}

static bool payment_address(const nu54_install_policy *policy, uint8_t address[20])
{
#if !defined(CONFIG_NU54_SPENDING_PROFILE)
	if (policy->payer_role == NU54_V2_RENTER_ROLE) {
		memcpy(address, device_install_payer(), 20);
		return true;
	}
#else
	if (policy->payer_role != NU54_V2_OWNER_ROLE) return false;
#endif
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	if (!device_spending_address()) return false;
	memcpy(address, device_spending_address(), 20);
	return true;
#elif defined(CONFIG_NU54_HD_RECOVERY)
	return device_hd_public_address(address);
#else
	return false;
#endif
}

static int sign(void *context, const uint8_t digest[32], uint8_t signature[64]);

#if defined(CONFIG_NU54_SPENDING_PROFILE)
static int order_request(nu54_v2_request *request, int state, uint64_t now, const nu54_install_policy *policy,
			 const uint8_t payer[20])
{
	const nu54_vault *vault = device_vault_owner();
	const uint8_t *confirmer, *quote;
	nu54_v2_quote_policy order;
	nu54_order_record saved;
	uint8_t digest[32];
	if (request->command != NU54_V2_PAY_PREPARE || policy->payer_role != NU54_V2_OWNER_ROLE ||
	    !vault->enabled || !vault->record.generation || (vault->record.flags & NU54_VAULT_REVOKED) ||
	    memcmp(vault->record.owner, link.peer_identity, sizeof(vault->record.owner)) ||
	    !nu54_order_decode(request->payload, request->payload_length, &confirmer, &quote) ||
	    nu54_install_quote(policy, quote, NU54_V2_SIGNED_SIZE, now, &order)) return -1;
	uint32_t prompt = NU54_PILOT_PROMPT_MASK | ++next_prompt;
	if (nu54_order_stage(&approval, &link, request, confirmer, quote, &order, &policy->allowlist, payer,
			     policy->payer_generation, device_install_earliest(), prompt, present, NULL)) return -1;
	if (hash(approval.intent.preimage, sizeof(approval.intent.preimage), digest)) return -1;
	int found = device_order_find(device_install_earliest(), request->id, digest, &saved, &order_slot);
	if (found == NU54_PILOT_RECEIPT_RECORDED) {
		const uint8_t refused = 0;
		nu54_local_approval_cancel(&approval);
		return saved.outcome == NU54_ORDER_APPROVED ? send_receipt(request, saved.signature, sizeof(saved.signature)) :
							      send_receipt(request, &refused, 1);
	}
	if (found != NU54_PILOT_RECEIPT_NEW) return -1;
	order_amount = sys_get_be64(quote + NU54_V2_AMOUNT_OFFSET + 24);
	order_expires = sys_get_be64(quote + NU54_V2_EXPIRES_AT_OFFSET + 24);
	if (!device_order_within_daily(device_install_earliest(), order_amount)) return -1;
	if (state == NU54_V2_NEW && link.approval_generation &&
	    link.approval_generation == vault->record.generation && trusted_now() < request->expires) {
		uint8_t progress[NU54_V2_APPROVAL_PROGRESS_SIZE];
		if (nu54_v2_approval_progress_body(1, link.approval_generation, progress)) return -1;
		request->command = NU54_V2_APPROVAL_PROGRESS;
		request->payload = progress;
		request->payload_length = sizeof(progress);
		if (nu54_v2_link_send(&link, request, send_frame, NULL)) return -1;
	}
	/* The phone shows the order in this profile, so the physical prompt opens without the
	 * independent display. SW1 and the PIN still decide. */
	display_confirmed = true;
	atomic_set(&pending_prompt, (atomic_val_t)prompt);
	board_io_set_prompt(prompt);
	status_led_waiting(true);
	return 0;
}

/* *approval_stored: the phone can read the signature back, so the lamp shows it as signed. */
static int order_button(uint32_t prompt, bool approved, bool *approval_stored)
{
	nu54_order_record record = {.expires = order_expires};
	nu54_v2_request reply = {.expires = link.pending_expiry};
	if (!display_confirmed || !order_slot) return -1;
	memcpy(record.request, approval.intent.request, sizeof(record.request));
	memcpy(reply.id, record.request, sizeof(reply.id));
	if (hash(approval.intent.preimage, sizeof(approval.intent.preimage), record.digest)) return -1;
	if (!approved) {
		const uint8_t refused = 0;
		record.outcome = NU54_ORDER_REFUSED;
		nu54_local_approval_cancel(&approval);
		return device_order_store(order_slot, &record) ? -1 : send_receipt(&reply, &refused, 1);
	}
	/* The PIN consumes its own physical events. Revalidate everything after unlocking. */
	int pin = pilot_pin(false, prompt);
	const nu54_install_policy *policy = device_install_policy();
	const nu54_vault *vault = device_vault_owner();
	uint8_t payer[20];
	if (pin || prompt != (uint32_t)atomic_get(&pending_prompt) || !policy || !approval.pending || !display_confirmed ||
	    !connection || pay_link_waiting() || !order_slot || !policy->allowlist.commercial_testnet ||
	    !vault->enabled || (vault->record.flags & NU54_VAULT_REVOKED) ||
	    !payment_generation_current(policy) || !payment_address(policy, payer) ||
	    trusted_now() >= approval.intent.expires ||
	    device_order_charge(device_install_earliest(), order_amount) ||
	    nu54_pilot_complete(&approval, &link, policy->payer_generation, trusted_now(), prompt, NU54_LOCAL_APPROVE,
				payer, sign, payer, record.signature)) return -1;
	record.outcome = NU54_ORDER_APPROVED;
	if (device_order_store(order_slot, &record)) return -1;
	*approval_stored = true;
	status_led_event(NU54_EVENT_SIGNED);
	return send_receipt(&reply, record.signature, sizeof(record.signature));
}
#endif

static int frame(const uint8_t *data, size_t length)
{
	nu54_v2_request request;
	uint64_t now = trusted_now();
	const nu54_install_policy *policy = device_install_policy();
	int rc = nu54_v2_link_receive(&link, data, length, now, &request);
	if (rc == NU54_V2_MORE) return 0;
	if (rc < 0 || pay_link_waiting() || !policy || enrollment.proven || approval.pending) return -1;
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	if (spend_action.pending) return -1;
#endif
#if defined(CONFIG_NU54_WALLET_CHAINS)
	if (chains.pending) return -1;
#endif
#if defined(CONFIG_NU54_HD_RECOVERY)
	if (direct.pending || (device_hd_pending() &&
	    (request.command != NU54_V2_MANAGE || request.payload_length != 1 || request.payload[0] != 4)))
		return -1; /* Read-only HD status is the only message during an HD physical prompt. */
#endif
	if (request.command == NU54_V2_MANAGE) return manage(&request, now, policy);
	if (recovering || enrollment.issued) return -1;
	uint8_t payer[20];
	if (!payment_generation_current(policy) || !payment_address(policy, payer)) return -1;
	if (request.command == NU54_V2_STATUS) return receipt_status(&request, policy);
	if (next_prompt == (NU54_PILOT_PROMPT_MASK >> 1) - 1) return -1;
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	return order_request(&request, rc, now, policy, payer);
#else
	const uint8_t *renter_challenge = NULL;
	if (nu54_v2_rental_review_prepare(request.payload, request.payload_length)) {
		if (profile != NU54_V2_PAYMENT || request.command != NU54_V2_PAY_PREPARE ||
		    !policy->allowlist.commercial_testnet || policy->payer_role != NU54_V2_RENTER_ROLE ||
		    now >= request.expires || request.expires - now > 120) return -1;
		renter_challenge = request.payload + 4;
		request.payload += 36;
		request.payload_length -= 36;
		if (sys_get_be64(request.payload + NU54_V2_EXPIRES_AT_OFFSET + 24) != request.expires ||
		    request.payload[NU54_V2_MODE_OFFSET + 31] != NU54_V2_ESCROW_MODE) return -1;
	}
	nu54_v2_quote_policy order;
	if (nu54_install_quote(policy, request.payload, request.payload_length, now, &order)) return -1;
	if (profile == NU54_V2_MANAGEMENT && nu54_pilot_review_consume(&reviewed_quote, &link, &request, hash))
		return -1;
	uint32_t prompt = NU54_PILOT_PROMPT_MASK | ++next_prompt;
	if (nu54_pilot_stage(&approval, &link, &request, &order, &policy->allowlist, payer,
			    policy->payer_generation, prompt, present, NULL)) return -1;
	nu54_pilot_receipt saved;
	uint8_t digest[32];
	if (hash(approval.intent.preimage, sizeof(approval.intent.preimage), digest)) return -1;
	int found = nu54_pilot_receipt_find(load_receipt, NULL, policy->payer_generation, request.id,
					   digest, &saved, &receipt_slot);
	if (found == NU54_PILOT_RECEIPT_RECORDED) {
		nu54_local_approval_cancel(&approval);
		return send_receipt(&request, saved.signature, sizeof(saved.signature));
	}
	if (found != NU54_PILOT_RECEIPT_NEW) return -1;
	int refusal = nu54_pilot_refusal_find(load_refusal, NULL, device_install_earliest(), request.id, digest, &refusal_slot);
	if (refusal == NU54_PILOT_RECEIPT_RECORDED) {
		const uint8_t refused = 0;
		nu54_local_approval_cancel(&approval);
		return send_receipt(&request, &refused, 1);
	}
	if (refusal != NU54_PILOT_RECEIPT_NEW) return -1;

	memcpy(active_quote, request.payload, sizeof(active_quote));
	display_confirmed = false;
	if (renter_challenge) {
		uint8_t receipt[NU54_V2_RENTAL_REVIEW_RECEIPT_SIZE];
		if (rc != NU54_V2_NEW || device_install_rental_review(&approval, &link, &request,
								     renter_challenge, receipt)) return -1;
		if (hash(receipt, sizeof(receipt), rental_review_hash)) return -1;
		request.command = NU54_V2_RENTAL_REVIEW;
		request.payload = receipt;
		request.payload_length = sizeof(receipt);
		return nu54_v2_link_send(&link, &request, send_frame, NULL);
	}
	if (rc == NU54_V2_NEW && profile == NU54_V2_MANAGEMENT && link.approval_generation &&
	    policy->payer_role == NU54_V2_OWNER_ROLE &&
	    link.approval_generation == device_vault_owner()->record.generation &&
	    trusted_now() < request.expires) {
		uint8_t progress[NU54_V2_APPROVAL_PROGRESS_SIZE];
		if (nu54_v2_approval_progress_body(1, link.approval_generation, progress)) return -1;
		request.command = NU54_V2_APPROVAL_PROGRESS;
		request.payload = progress;
		request.payload_length = sizeof(progress);
		if (nu54_v2_link_send(&link, &request, send_frame, NULL)) return -1;
	}
	return 0;
#endif
}

static int stream(const uint8_t *data, size_t length)
{
	for (size_t i = 0; i < length; ++i) {
		wire[wire_length++] = data[i];
		if (wire_length < 2) continue;
		size_t wanted = ((size_t)wire[0] << 8) | wire[1];
		if (wanted <= 22 || wanted > NU54_V2_FRAME_MAX) return -1;
		if (wire_length == wanted + 2) {
			if (frame(wire + 2, wanted)) return -1;
			wire_length = 0;
		}
	}
	return 0;
}

static bool install_window_live(int64_t now)
{
	return initialized && now < enrollment_window && now >= recovery_window && !pay_link_waiting();
}

static bool tls_retry(int result)
{
	return result == MBEDTLS_ERR_SSL_WANT_READ || result == MBEDTLS_ERR_SSL_WANT_WRITE;
}

/* No installation data enters this function before a verified TLS handshake. */
static int poll_install(int64_t now)
{
	if (now >= install_deadline || (install_phase != INSTALL_ACK && !install_window_live(now))) return -1;
	if (install_phase == INSTALL_HANDSHAKE) {
		int result = mbedtls_ssl_handshake(&tls);
		if (tls_retry(result)) return 0;
		uint8_t exporter[32];
		if (result || device_tls_bootstrap_bind(&tls, &install_station, install_public + 1, exporter) ||
		    device_install_bound_offer(exporter, install_offer)) return -1;
		install_phase = INSTALL_SEND_OFFER;
	}
	if (install_phase == INSTALL_SEND_OFFER) {
		int result = mbedtls_ssl_write(&tls, install_offer, sizeof(install_offer));
		if (tls_retry(result)) return 0;
		if (result != (int)sizeof(install_offer)) return -1;
		mbedtls_platform_zeroize(install_offer, sizeof(install_offer));
		install_total = 2;
		install_phase = INSTALL_RECEIVE_GRANT;
	}
	for (unsigned reads = 0; reads < 4 && install_phase == INSTALL_RECEIVE_GRANT; ++reads) {
		int result = mbedtls_ssl_read(&tls, install_bundle + install_used, install_total - install_used);
		if (tls_retry(result)) return 0;
		if (result <= 0) return -1;
		install_used += (size_t)result;
		if (install_used < install_total) continue;
		if (install_total == 2) {
			size_t length = sys_get_be16(install_bundle);
			if (!length || length > NU54_INSTALL_BUNDLE_MAX) return -1;
			install_total = length + 2;
			continue;
		}
		if (apply_install(install_bundle + 2, install_total - 2)) return -1;
		enrollment_window = 0;
		mbedtls_platform_zeroize(install_bundle, sizeof(install_bundle));
		install_phase = INSTALL_ACK;
	}
	if (install_phase == INSTALL_ACK) {
		static const uint8_t ack[] = "N54K";
		int result = mbedtls_ssl_write(&tls, ack, sizeof(ack) - 1);
		return tls_retry(result) ? 0 : -1;
	}
	return 0;
}

static void poll_tls(struct k_work *ignored)
{
	(void)ignored;
	k_mutex_lock(&pilot_lock, K_FOREVER);
	if (!connection) goto done;
	if (install_phase != INSTALL_IDLE) {
		if (poll_install(k_uptime_get())) goto failed;
		goto wait;
	}
	const nu54_install_policy *policy = device_install_policy();
	if (!policy || !session_live(k_uptime_get()) || (approval.pending && pay_link_waiting())) goto failed;
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
	if (limits_approval.pending && (pay_link_waiting() || trusted_now() >= limits_approval.intent.expires)) goto failed;
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
	if (finality_approval.pending && (pay_link_waiting() || trusted_now() >= finality_approval.intent.expires || trusted_now() >= link.pending_expiry)) goto failed;
#endif
#endif
#if defined(CONFIG_NU54_HD_RECOVERY)
	if (direct.pending && (pay_link_waiting() || trusted_now() >= direct.expiry)) goto failed;
#endif
#if defined(CONFIG_NU54_WALLET_CHAINS)
	if (chains.pending && (pay_link_waiting() || trusted_now() >= chains.expiry)) goto failed;
#endif
	if (!bound) {
		int result = mbedtls_ssl_handshake(&tls);
		if (result == MBEDTLS_ERR_SSL_WANT_READ || result == MBEDTLS_ERR_SSL_WANT_WRITE) goto wait;
		const nu54_vault *vault = device_vault_owner();
		const uint8_t *pin = policy->terminal;
		if (profile == NU54_V2_MANAGEMENT && !nu54_owner_session_pin(vault, policy->owner, &pin)) goto failed;
		if (result || trusted_now() >= policy->not_after) goto failed;
		if (k_uptime_get() < recovery_window) {
			if (profile != NU54_V2_MANAGEMENT || !vault->record.generation ||
			    (vault->record.flags & NU54_VAULT_REVOKED) ||
			    device_tls_v2_bind(&tls, &link, profile, NULL, &journal, trusted_now()) ||
			    !memcmp(pin, link.peer_identity, 32)) goto failed;
			recovering = true;
		} else if (device_tls_v2_bind(&tls, &link, profile, pin, &journal, trusted_now())) goto failed;
		bound = true;
#if defined(CONFIG_NU54_SPENDING_PROFILE)
		journal_new_session();
#endif
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
		if (profile == NU54_V2_MANAGEMENT && !recovering && vault->record.generation &&
		    (vault->record.flags & NU54_VAULT_PERSONAL) && device_owner_limits_platform_qualified())
			nu54_owner_idle_start(&owner_idle, vault->record.generation, k_uptime_get());
#endif
	}
	for (unsigned reads = 0; reads < 8; ++reads) {
		uint8_t bytes[256];
		int result = mbedtls_ssl_read(&tls, bytes, sizeof(bytes));
		if (result == MBEDTLS_ERR_SSL_WANT_READ || result == MBEDTLS_ERR_SSL_WANT_WRITE) goto wait;
		if (result <= 0 || stream(bytes, (size_t)result)) goto failed;
	}
wait:
	k_work_reschedule(&worker, K_MSEC(25));
	goto done;
failed:
	reset_link();
done:
	k_mutex_unlock(&pilot_lock);
}

static int sign(void *context, const uint8_t digest[32], uint8_t signature[64])
{
	const nu54_install_policy *policy = device_install_policy();
	if (!policy) return -1;
#if defined(CONFIG_NU54_HD_RECOVERY)
	if (policy->payer_role == NU54_V2_OWNER_ROLE) return device_hd_sign(context, digest, signature);
#else
	(void)context;
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	return policy->payer_role == NU54_V2_OWNER_ROLE ? device_spending_sign(digest, signature) : -1;
#else
	if (policy->payer_role == NU54_V2_OWNER_ROLE) return -1;
#endif
#endif
	return device_install_sign(digest, signature);
}

void device_pilot_button(uint32_t prompt, bool approved)
{
	k_mutex_lock(&pilot_lock, K_FOREVER);
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	bool done_well = false; /* the session ends, but nothing was refused */
#endif
	if (prompt != (uint32_t)atomic_get(&pending_prompt)) goto done;
	if (owner_idle.generation) {
		int64_t now = k_uptime_get();
		uint64_t current = trusted_now();
		if (!session_live(now) || !current || current >= link.pending_expiry) goto failed;
		if (approved && !nu54_owner_idle_activity(&owner_idle, owner_idle.generation, now)) goto failed;
	}
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
	if (finality_approval.pending) {
		nu54_v2_request reply={.command=NU54_V2_REPLY,.expires=link.pending_expiry};uint8_t answer[2]={OWNER_FINALITY_ENROLL_OP,0};
		memcpy(reply.id,link.pending_id,sizeof(reply.id));
		if(!approved){reply.payload=answer;reply.payload_length=2;nu54_v2_link_send(&link,&reply,send_frame,NULL);goto failed;}
		int pin = pilot_pin(false, prompt);
		const nu54_vault *vault=device_vault_owner();
		if(pin || prompt!=(uint32_t)atomic_get(&pending_prompt) || !finality_displayed || !connection || !device_install_policy() ||
		   !vault->enabled || (vault->record.flags & NU54_VAULT_REVOKED) || vault->record.generation!=finality_enrollment.generation ||
		   link.closed || link.profile!=NU54_V2_MANAGEMENT || link.peer!=NU54_V2_OWNER ||
		   memcmp(link.epoch,finality_enrollment.session,16) || memcmp(link.pending_id,finality_enrollment.request,16) ||
		   trusted_now()>=link.pending_expiry || pay_link_waiting())goto failed;
		int result=device_owner_finality_enroll_ps(&finality_enrollment,device_install_earliest(),device_install_now(),prompt,NU54_LOCAL_APPROVE,
		 &finality_approval,device_install_verify_authority,finality_issuer_proof,device_install_verify_display,finality_display_proof,device_install_independent_finality_key);
		if(result!=0 && result!=NU54_FINALITY_ALREADY)goto failed;
		answer[1]=(uint8_t)(result==NU54_FINALITY_ALREADY?2:1);reply.payload=answer;reply.payload_length=2;
		nu54_v2_link_send(&link,&reply,send_frame,NULL);goto failed;
	}
#endif
	if (limits_approval.pending) {
		bool set = limits_operation == NU54_LIMIT_SET_CHANGE_OP || limits_operation == NU54_LIMIT_SET_CAPS_OP;
		size_t status = set ? 2 : 1;
		uint8_t answer[11] = {limits_operation, set ? NU54_LIMIT_SET_WIRE_VERSION : 0};
		nu54_v2_request reply = {.command=NU54_V2_REPLY,.expires=link.pending_expiry};
		memcpy(reply.id,link.pending_id,sizeof(reply.id));
		if (!approved) {
			reply.payload=answer; reply.payload_length=status+1;
			nu54_v2_link_send(&link,&reply,send_frame,NULL);
			goto failed;
		}
		int pin=pilot_pin(false,prompt);
		const nu54_vault *vault=device_vault_owner();
		const nu54_local_intent *intent=&limits_approval.intent;
		if (pin || prompt!=(uint32_t)atomic_get(&pending_prompt) || !limits_displayed || !connection ||
		    !device_install_policy() || !vault->enabled || (vault->record.flags & NU54_VAULT_REVOKED) ||
		    vault->record.generation!=intent->generation || link.closed || link.profile!=NU54_V2_MANAGEMENT ||
		    link.peer!=NU54_V2_OWNER || memcmp(link.epoch,intent->session,sizeof(link.epoch)) ||
		    memcmp(link.pending_id,intent->request,sizeof(link.pending_id)) || pay_link_waiting()) goto failed;
		uint64_t lower=device_install_earliest(),upper=device_install_now();
		int result;
		if (set) {
			result=device_owner_limits_confirm_recipients_ps(&limits_set_change,lower,upper,prompt,
				NU54_LOCAL_APPROVE,&limits_approval,limits_proof,hash,device_install_verify_display);
		} else if (limits_change.initialize) {
			result=device_owner_limits_initialize_confirm_ps(&limits_change,lower,upper,prompt,
				NU54_LOCAL_APPROVE,&limits_approval,limits_proof,hash,device_install_verify_display);
		} else {
			result=device_owner_limits_confirm_ps(&limits_change,lower,upper,prompt,
				NU54_LOCAL_APPROVE,&limits_approval,limits_proof,hash,device_install_verify_display);
		}
		if (result) goto failed;
		answer[status]=1;
		sys_put_be64(set ? limits_set_change.not_before : limits_change.not_before,answer+status+1);
		reply.payload=answer;
		reply.payload_length=status+1+(limits_change.initialize && !set ? 0 : 8);
		nu54_v2_link_send(&link,&reply,send_frame,NULL);
		goto failed;
	}
#endif
#if defined(CONFIG_NU54_WALLET_CHAINS)
	if (chains.pending) {
		nu54_v2_request reply = {.expires = link.pending_expiry};
		memcpy(reply.id, link.pending_id, sizeof(reply.id));
		if (!approved) {
			const uint8_t refused = 0;
			send_receipt(&reply, &refused, 1);
			goto failed;
		}
		int pin = pilot_pin(false, prompt);
		if (prompt != (uint32_t)atomic_get(&pending_prompt)) goto done;
		const nu54_vault *vault = device_vault_owner();
		uint8_t tx[NU54_CHAIN_REPLY_MAX];
		size_t length;
		if (pin || !connection || pay_link_waiting() || !vault->enabled ||
		    (vault->record.flags & NU54_VAULT_REVOKED) ||
		    device_chains_complete(&chains, &link, trusted_now(), vault->record.generation,
					   prompt, tx, &length) || trusted_now() >= chains.expiry) goto failed;
		send_receipt(&reply, tx, length);
		goto failed;
	}
#endif
#if defined(CONFIG_NU54_HD_RECOVERY)
	if (direct.pending) {
		if (!approved) {
			const uint8_t refused = 0;
			nu54_v2_request reply = {.expires = link.pending_expiry};
			memcpy(reply.id, link.pending_id, sizeof(reply.id));
			send_receipt(&reply, &refused, 1);
			goto failed;
		}
		int pin = pilot_pin(false, prompt);
		if (prompt != (uint32_t)atomic_get(&pending_prompt)) goto done;
		uint8_t address[20], tx[LAB_TX_LENGTH];
		size_t length;
		if (pin || !connection || pay_link_waiting() ||
		    !(direct.market ? device_hd_market_address(address) : device_hd_public_address(address)) ||
		    device_evm_complete(&direct, &link, trusted_now(), device_vault_owner()->record.generation,
					prompt, address, tx, &length) || trusted_now() >= direct.expiry) goto failed;
		nu54_v2_request reply = {.expires = link.pending_expiry};
		memcpy(reply.id, link.pending_id, sizeof(reply.id));
		send_receipt(&reply, tx, length);
		goto failed;
	}
#endif
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	if (approved) status_led_pressed();
#endif
	if (enrollment.proven) {
		if (recovering && (!enrollment.independent || k_uptime_get() >= recovery_window)) goto failed;
		if (approved) {
			bool virgin = !recovering && (!device_vault_owner()->record.generation ||
						      (device_vault_owner()->record.flags & NU54_VAULT_RELEASED));
			int pin = pilot_pin(virgin, prompt);
#if defined(CONFIG_NU54_SPENDING_PROFILE)
			if (pin) status_led_event(NU54_EVENT_REFUSED);
#endif
			if (prompt != (uint32_t)atomic_get(&pending_prompt)) goto done;
			if (pin || !device_install_policy() || !enrollment.proven ||
			    (recovering && (!enrollment.independent || k_uptime_get() >= recovery_window))) goto failed;
		}
		nu54_vault *vault = device_vault_owner();
		uint64_t now = trusted_now();
		if (link.pending_expiry <= now ||
		    nu54_owner_confirm(&enrollment, vault, &link, now, prompt, approved)) goto failed;
		uint8_t status[NU54_OWNER_STATUS_SIZE];
		/* The owner changed: no bond may outlive the previous owner phone. This runs on the TLS
		 * work item, off the Bluetooth thread, and the phone in session holds no bond. */
		clear_legacy_bonds();
		if (nu54_owner_status(vault, &owner_crypto, &enrollment, status)) goto failed;
		report_legacy_bonds(status);
		nu54_v2_request reply = {.command = NU54_V2_REPLY, .expires = link.pending_expiry,
			.payload = status, .payload_length = sizeof(status)};
		memcpy(reply.id, link.pending_id, sizeof(reply.id));
		nu54_v2_link_send(&link, &reply, send_frame, NULL);
		if (recovering) {
			recovery_window = 0;
		}
#if defined(CONFIG_NU54_SPENDING_PROFILE)
		done_well = true;
#endif
		goto failed; /* A changed generation retires the authenticated session. */
	}
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	if (spend_action.pending) {
		if (spending_button(prompt, approved)) goto failed;
		goto answered;
	}
#endif
	if (!approval.pending) goto done;
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	if (order_button(prompt, approved, &done_well)) goto failed;
	if (!approved) goto failed; /* a refusal ends the session, as on the other payment routes */
	goto answered;
#else
	if (!approved) {
		const uint64_t generation = approval.intent.generation;
		nu54_pilot_refusal refusal;
		if (!display_confirmed || !refusal_slot ||
		    nu54_pilot_refuse(&approval, &link, active_quote, trusted_now(), prompt, hash, &refusal) ||
		    psa_its_set(PILOT_REFUSAL_UID + refusal_slot, sizeof(refusal), &refusal, PSA_STORAGE_FLAG_NONE)
		        != PSA_SUCCESS) goto failed;
		nu54_v2_request reply = {.expires = link.pending_expiry};
		memcpy(reply.id, refusal.request, sizeof(reply.id));
		send_payment_decision(&reply, generation, NULL);
		goto failed;
	}
	if (!display_confirmed || !receipt_slot) goto failed;
	/* Board PIN consumes its own physical events. Revalidate the approval after unlocking. */
	int pin = pilot_pin(false, prompt);
	const nu54_install_policy *policy = device_install_policy();
	if (prompt != (uint32_t)atomic_get(&pending_prompt)) goto done;
	uint8_t payer[20];
	if (pin || !policy || !approval.pending || !display_confirmed || !connection || pay_link_waiting() ||
	    !payment_generation_current(policy) || !payment_address(policy, payer) ||
	    trusted_now() >= approval.intent.expires) goto failed;
	nu54_pilot_receipt receipt = {.generation = policy->payer_generation};
	memcpy(receipt.request, approval.intent.request, sizeof(receipt.request));
	struct psa_storage_info_t info;
	if (psa_its_get_info(PILOT_RECEIPT_UID + receipt_slot, &info) != PSA_ERROR_DOES_NOT_EXIST ||
	    hash(approval.intent.preimage, sizeof(approval.intent.preimage), receipt.digest)) goto failed;
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
	if (device_vault_owner()->record.flags & NU54_VAULT_PERSONAL) {
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
		goto failed; /* Personal commerce quotes are not in the 8283 direct-EVM scope. */
#else
		uint8_t board[32];
		if (hash(device_vault_owner()->record.identity,sizeof(device_vault_owner()->record.identity),board) ||
		    device_owner_limits_authorize_pilot_ps(board,policy->payer_generation,active_quote,
			    receipt.request,receipt.digest,approval.intent.expires,device_install_earliest(),device_install_now())) goto failed;
#endif
	}
#endif
	if (nu54_pilot_complete(&approval, &link, policy->payer_generation, trusted_now(), prompt,
				NU54_LOCAL_APPROVE, payer, sign, payer, receipt.signature)) goto failed;
	receipt.complete = 1;
	if (psa_its_set(PILOT_RECEIPT_UID + receipt_slot, sizeof(receipt), &receipt, PSA_STORAGE_FLAG_NONE) != PSA_SUCCESS) goto failed;
	nu54_v2_request reply = {.expires = link.pending_expiry};
	memcpy(reply.id, receipt.request, sizeof(reply.id));
	if (send_payment_decision(&reply, receipt.generation, receipt.signature)) goto failed;
#endif
#if defined(CONFIG_NU54_SPENDING_PROFILE)
answered:
#endif
	atomic_set(&pending_prompt, 0);
	board_io_set_prompt(0);
	status_led_waiting(false);
	goto done;
failed:
	reset_link();
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	if (!done_well) status_led_event(NU54_EVENT_REFUSED);
#endif
done:
	k_mutex_unlock(&pilot_lock);
}

bool device_pilot_waiting(void)
{
	return atomic_get(&pending_prompt) != 0;
}

bool device_pilot_active(void) { return device_install_required(); }

bool device_pilot_owner_absent(void)
{
	k_mutex_lock(&pilot_lock, K_FOREVER);
	const nu54_vault *vault = device_vault_owner();
	bool absent = initialized && device_pilot_active() &&
		      (!vault->record.generation || (vault->record.flags & NU54_VAULT_RELEASED));
	k_mutex_unlock(&pilot_lock);
	return absent;
}

void device_pilot_enrollment_window(void)
{
	k_mutex_lock(&pilot_lock, K_FOREVER);
	if (install_phase != INSTALL_IDLE) reset_link();
	reset_install_input();
	device_install_consume_offer();
	install_public[0] = NU54_INSTALL_BOOTSTRAP_VERSION;
	enrollment_window = initialized && psa_generate_random(install_public + 1, sizeof(install_public) - 1) == PSA_SUCCESS ?
			    k_uptime_get() + PILOT_TIMEOUT_MS : 0;
	k_mutex_unlock(&pilot_lock);
}

int device_pilot_install_pins(uint8_t output[64])
{
	k_mutex_lock(&pilot_lock, K_FOREVER);
	int result = install_window_live(k_uptime_get()) ? device_install_public_pins(output) : -1;
	k_mutex_unlock(&pilot_lock);
	return result;
}

bool device_pilot_recovery_window(void)
{
	k_mutex_lock(&pilot_lock, K_FOREVER);
	bool available = ready && device_install_policy() && device_vault_owner()->enabled &&
			 device_vault_owner()->record.generation &&
			 !(device_vault_owner()->record.flags & NU54_VAULT_REVOKED) && !pay_link_waiting();
	struct bt_conn *prior = NULL;
	if (available) {
		if (connection) prior = bt_conn_ref(connection);
		reset_link();
		reset_install_input();
		enrollment_window = 0;
		recovery_window = k_uptime_get() + 120000;
	}
	k_mutex_unlock(&pilot_lock);
	if (prior) {
		bt_conn_disconnect(prior, BT_HCI_ERR_REMOTE_USER_TERM_CONN);
		bt_conn_unref(prior);
	}
	return available;
}

void device_pilot_init(void)
{
	mbedtls_ssl_init(&tls);
	k_work_init_delayable(&worker, poll_tls);
	initialized = device_install_init() == 0;
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	initialized = initialized && device_spending_init() == 0;
#endif
	/* Bluetooth and its bond settings are loaded before this runs (pay_link_init). A commercial
	 * board keeps no legacy bond, including one a power cut left behind after an owner change. */
	if (initialized && device_pilot_active()) clear_legacy_bonds();
}

static ssize_t receive_bytes(struct bt_conn *conn, const struct bt_gatt_attr *attribute,
			     const void *data, uint16_t length, uint16_t offset, uint8_t flags)
{
	(void)attribute;
	(void)flags;
	if (offset || !length) return BT_GATT_ERR(BT_ATT_ERR_WRITE_NOT_PERMITTED);
	const uint8_t *bytes = data;
	k_mutex_lock(&pilot_lock, K_FOREVER);
	ssize_t result = BT_GATT_ERR(BT_ATT_ERR_WRITE_NOT_PERMITTED);
	if (install_phase == INSTALL_IDLE && (!ready || !device_install_policy())) goto done;
	if (!connection) {
		if (bytes[0] != NU54_V2_MANAGEMENT && bytes[0] != NU54_V2_PAYMENT) goto done;
		if (k_uptime_get() < recovery_window && bytes[0] != NU54_V2_MANAGEMENT) goto done;
		profile = (nu54_v2_profile)bytes[0];
		if (mbedtls_ssl_setup(&tls, &tls_banks[tls_bank].profiles[profile - 1].config)) goto done;
		connection = bt_conn_ref(conn);
		deadline = k_uptime_get() + PILOT_TIMEOUT_MS;
		mbedtls_ssl_set_bio(&tls, NULL, push, pull, NULL);
		++bytes;
	} else if (connection != conn) goto done;
	size_t count = length - (size_t)(bytes != data);
	if (ring_buf_space_get(&pilot_rx) < count) {
		reset_link();
		goto done;
	}
	ring_buf_put(&pilot_rx, bytes, count);
	k_work_reschedule(&worker, K_NO_WAIT);
	result = length;
done:
	k_mutex_unlock(&pilot_lock);
	return result;
}

static ssize_t read_install_nonce(struct bt_conn *conn, const struct bt_gatt_attr *attribute,
			  void *data, uint16_t length, uint16_t offset)
{
	uint8_t public_value[NU54_INSTALL_BOOTSTRAP_PUBLIC_SIZE];
	k_mutex_lock(&pilot_lock, K_FOREVER);
	bool live = install_window_live(k_uptime_get());
	memcpy(public_value, install_public, sizeof(public_value));
	k_mutex_unlock(&pilot_lock);
	return live ? bt_gatt_attr_read(conn, attribute, data, length, offset, public_value, sizeof(public_value)) :
	       BT_GATT_ERR(BT_ATT_ERR_READ_NOT_PERMITTED);
}

static void free_tls_bank(struct provisioned_tls *bank)
{
	mbedtls_ssl_config_free(&bank->profiles[0].config);
	mbedtls_ssl_config_free(&bank->profiles[1].config);
	mbedtls_pk_free(&bank->key);
	mbedtls_x509_crt_free(&bank->board);
	mbedtls_x509_crt_free(&bank->ca);
	memset(bank, 0, sizeof(*bank));
}

static int apply_install(const uint8_t *bundle, size_t length)
{
#if defined(CONFIG_NU54_HD_RECOVERY)
	if (device_hd_pending()) return -1;
	if (length >= 4 && !memcmp(bundle, "N54H", 4)) {
		struct provisioned_tls *active = &tls_banks[tls_bank];
		if (!ready || device_install_check_hd(bundle, length) ||
		    device_hd_configure(&active->board, &active->key, trusted_now, bundle + 141, length - 141 - 64)) return -1;
		device_install_consume_offer();
		return 0;
	}
#endif
	unsigned next = tls_bank ^ 1;
	struct provisioned_tls *bank = &tls_banks[next];
	if (device_install_load(bundle, length, &bank->ca, &bank->board, &bank->key, bank->profiles)) {
		free_tls_bank(bank);
		return -1;
	}
#if defined(CONFIG_NU54_HD_RECOVERY)
	device_hd_clear();
#endif
	free_tls_bank(&tls_banks[tls_bank]);
	tls_bank = next;
	ready = true;
	return 0;
}

/* Only the public authority ticket may be assembled before TLS. */
static ssize_t provision(struct bt_conn *conn, const struct bt_gatt_attr *attribute,
			 const void *data, uint16_t length, uint16_t offset, uint8_t flags)
{
	(void)attribute;
	if (offset || flags || length <= 4) return BT_GATT_ERR(BT_ATT_ERR_INVALID_ATTRIBUTE_LEN);
	const uint8_t *bytes = data;
	size_t total = sys_get_be16(bytes), position = sys_get_be16(bytes + 2);
	size_t payload = length - 4u;
	if (total > NU54_INSTALL_BOOTSTRAP_TICKET_MAX || position > total || payload > total - position)
		return BT_GATT_ERR(BT_ATT_ERR_INVALID_ATTRIBUTE_LEN);
	k_mutex_lock(&pilot_lock, K_FOREVER);
	ssize_t result = BT_GATT_ERR(BT_ATT_ERR_WRITE_NOT_PERMITTED);
	int64_t now = k_uptime_get();
	if (!install_window_live(now) || connection ||
	    !bt_gatt_is_subscribed(conn, &pilot_service.attrs[4], BT_GATT_CCC_NOTIFY)) goto done;
#if defined(CONFIG_NU54_HD_RECOVERY)
	if (device_hd_pending()) goto done;
#endif
	if (ticket_peer && now >= install_deadline) reset_install_input();
	if (ticket_peer && ticket_peer != conn) goto done;
	if (position == 0) {
		reset_install_input();
		ticket_peer = bt_conn_ref(conn);
		install_total = total;
		install_deadline = now + 15000;
	}
	if (ticket_peer != conn || install_total != total || install_used != position) goto done;
	memcpy(install_bundle + position, bytes + 4, payload);
	install_used += payload;
	if (install_used == total) {
		const uint8_t *certificate;
		size_t certificate_length;
		int rc = nu54_install_bootstrap_ticket(install_bundle, total, install_public,
					      device_install_verify_authority, &certificate, &certificate_length);
		if (!rc) rc = mbedtls_x509_crt_parse_der(&install_station, certificate, certificate_length);
		if (!rc && !device_install_distinct_station(&install_station)) rc = -1;
		if (!rc) rc = device_tls_bootstrap_setup(&tls, &install_tls, &install_station);
		reset_install_input();
		if (rc) {
			mbedtls_ssl_free(&tls);
			mbedtls_ssl_init(&tls);
			mbedtls_ssl_config_free(&install_tls);
			mbedtls_x509_crt_free(&install_station);
			goto done;
		}
		connection = bt_conn_ref(conn);
		install_phase = INSTALL_HANDSHAKE;
		install_deadline = MIN(enrollment_window, now + 30000);
		mbedtls_ssl_set_bio(&tls, NULL, push, pull, NULL);
		k_work_reschedule(&worker, K_NO_WAIT);
	}
	result = length;
done:
	k_mutex_unlock(&pilot_lock);
	return result;
}

/* The independent display reconstructs the approval separately. The public
 * characteristic can return only a slice of the pending random challenge. */
static ssize_t read_display(struct bt_conn *conn, const struct bt_gatt_attr *attribute,
			    void *data, uint16_t length, uint16_t offset)
{
	k_mutex_lock(&pilot_lock, K_FOREVER);
	ssize_t result = BT_GATT_ERR(BT_ATT_ERR_READ_NOT_PERMITTED);
	if (owner_idle.generation && !session_live(k_uptime_get())) {
		reset_link();
		k_mutex_unlock(&pilot_lock);
		return result;
	}
	const uint8_t (*challenge)[32] = NULL;
	uint64_t now = trusted_now();
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
	if (finality_approval.pending && !finality_displayed && now < link.pending_expiry &&
	    now < finality_approval.intent.expires)
		challenge = &finality_enrollment.challenge;
	else
#endif
	if (limits_approval.pending && !limits_displayed && now < limits_approval.intent.expires)
		challenge = (limits_operation == NU54_LIMIT_SET_CHANGE_OP || limits_operation == NU54_LIMIT_SET_CAPS_OP) ?
			    &limits_set_change.challenge : &limits_change.challenge;
	else
#endif
	if (approval.pending && !display_confirmed && now < approval.intent.expires)
		challenge = &display_challenge;
#if defined(CONFIG_NU54_HD_RECOVERY)
	else if (direct.pending && !direct.displayed && now < direct.expiry)
		challenge = &direct.challenge;
#endif
#if defined(CONFIG_NU54_WALLET_CHAINS)
	else if (chains.pending && !chains.displayed && now < chains.expiry)
		challenge = &chains.challenge;
#endif
	if (challenge)
		result = bt_gatt_attr_read(conn, attribute, data, length, offset, *challenge, sizeof(*challenge));
	k_mutex_unlock(&pilot_lock);
	return result;
}

static ssize_t confirm_display(struct bt_conn *conn, const struct bt_gatt_attr *attribute,
			       const void *data, uint16_t length, uint16_t offset, uint8_t flags)
{
	(void)attribute;
	if (offset || flags || length <= 1 || length > 65) return BT_GATT_ERR(BT_ATT_ERR_INVALID_ATTRIBUTE_LEN);
	k_mutex_lock(&pilot_lock, K_FOREVER);
	ssize_t result = BT_GATT_ERR(BT_ATT_ERR_WRITE_NOT_PERMITTED);
	bool completed = false;
	if (conn == connection || !device_install_policy() ||
	    (owner_idle.generation && !session_live(k_uptime_get()))) goto done;
	bool waiting = approval.pending || enrollment.proven;
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
	waiting |= limits_approval.pending;
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
	waiting |= finality_approval.pending;
#endif
#endif
#if defined(CONFIG_NU54_HD_RECOVERY)
	waiting |= direct.pending;
#endif
#if defined(CONFIG_NU54_WALLET_CHAINS)
	waiting |= chains.pending;
#endif
	if (!waiting) goto done;
	const uint8_t *bytes = data;
	size_t position = bytes[0], count = length - 1u;
	if (position >= sizeof(display_signature) || count > sizeof(display_signature) - position) goto done;
	if (!position && (!display_connection || display_connection == conn)) {
		if (!display_connection) display_connection = bt_conn_ref(conn);
		display_used = 0;
	}
	if (display_connection != conn || position != display_used) goto done;
	memcpy(display_signature + position, bytes + 1, count);
	display_used += count;
	if (display_used < sizeof(display_signature)) {
		result = length;
		goto done;
	}
	completed = true;
	reset_display_input();
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
	if(finality_approval.pending && !finality_displayed && trusted_now()<link.pending_expiry &&
	   trusted_now()<finality_approval.intent.expires){
		if(device_install_verify_display(finality_statement,sizeof(finality_statement),display_signature))goto done;
		memcpy(finality_display_proof,display_signature,sizeof(finality_display_proof));finality_displayed=true;
		atomic_set(&pending_prompt,(atomic_val_t)finality_approval.intent.physical_prompt);
		board_io_set_prompt(finality_approval.intent.physical_prompt);status_led_waiting(true);result=length;goto done;
	}
#endif
	if (limits_approval.pending && !limits_displayed && trusted_now() < limits_approval.intent.expires) {
		if (device_install_verify_display(limits_statement,limits_statement_length,display_signature)) goto done;
		memcpy(limits_proof,display_signature,sizeof(limits_proof)); limits_displayed=true;
		atomic_set(&pending_prompt,(atomic_val_t)limits_approval.intent.physical_prompt);
		board_io_set_prompt(limits_approval.intent.physical_prompt); status_led_waiting(true);
		result=length; goto done;
	}
#endif
#if defined(CONFIG_NU54_WALLET_CHAINS)
	if (chains.pending) {
		if (device_chains_display(&chains, trusted_now(), display_signature)) goto done;
		atomic_set(&pending_prompt, (atomic_val_t)chains.prompt);
		board_io_set_prompt(chains.prompt);
		status_led_waiting(true);
		result = length;
		goto done;
	}
#endif
#if defined(CONFIG_NU54_HD_RECOVERY)
	if (direct.pending) {
		if (device_evm_display(&direct, trusted_now(), display_signature)) goto done;
		atomic_set(&pending_prompt, (atomic_val_t)direct.prompt);
		board_io_set_prompt(direct.prompt);
		status_led_waiting(true);
		result = length;
		goto done;
	}
#endif
	if (recovering && enrollment.proven && !enrollment.independent && k_uptime_get() < recovery_window &&
	    trusted_now() < enrollment.expiry) {
		if (nu54_owner_authorize_lost(&enrollment, device_vault_owner(), &link, trusted_now(),
					     device_install_verify_recovery, display_signature)) goto done;
		atomic_set(&pending_prompt, (atomic_val_t)enrollment.prompt);
		board_io_set_prompt(enrollment.prompt);
		status_led_waiting(true);
		result = length;
		goto done;
	}
	if (!approval.pending || display_confirmed || trusted_now() >= approval.intent.expires) goto done;
	if (nu54_pilot_display(&approval, active_quote, display_challenge, display_signature,
			       device_install_verify_display)) goto done;
	display_confirmed = true;
	atomic_set(&pending_prompt, (atomic_val_t)approval.intent.physical_prompt);
	board_io_set_prompt(approval.intent.physical_prompt);
	status_led_waiting(true);
	result = length;
done:
	if (completed) memset(display_signature, 0, sizeof(display_signature));
	k_mutex_unlock(&pilot_lock);
	return result;
}
