/* How a session of the spending profile is put together on the board: the first write that opens
 * it (receive_bytes), the TLS work item that binds it, feeds it and ends it (poll_tls), and what
 * its end leaves behind (reset_link). Before that, what the board does at its start and in which
 * order (device_pilot_init). These functions are extracted from device_pilot.c at build
 * time. Bluetooth, the ring buffer and TLS are stand-ins: TLS hands the written bytes on as they
 * are, and the handshake ends when the test says so. What a message does once it is read is not
 * looked at here (frame is a stand-in; test_clock_sync.c runs the real one). */
#include "nu54_install.h"
#include "nu54_owner.h"
#include "nu54_owner_idle.h"
#include "nu54_spending.h"

#include <assert.h>
#include <stdio.h>
#include <string.h>
#include <sys/types.h>

/* ---- Zephyr, Bluetooth and mbed TLS ---- */
typedef int32_t atomic_t;
typedef int32_t atomic_val_t;
static int32_t atomic_get(const atomic_t *value) { return *value; }
static void atomic_set(atomic_t *value, int32_t next) { *value = next; }
#define K_FOREVER 0
#define K_NO_WAIT 0
#define K_MSEC(milliseconds) (milliseconds)
#define k_mutex_lock(lock, timeout) ((void)++*(lock))
#define k_mutex_unlock(lock) ((void)--*(lock))
static int pilot_lock;
static int64_t uptime = 1000, worker = -1; /* the uptime at which the TLS work item runs next */
static int64_t k_uptime_get(void) { return uptime; }
static void k_work_reschedule(int64_t *work, int64_t delay) { *work = uptime + delay; }

struct bt_conn {int references;};
struct bt_gatt_attr;
#define BT_ATT_ERR_WRITE_NOT_PERMITTED 3
#define BT_GATT_ERR(error) (-(error))
static struct bt_conn *bt_conn_ref(struct bt_conn *conn) { ++conn->references; return conn; }
static void bt_conn_unref(struct bt_conn *conn) { --conn->references; }

static struct {uint8_t bytes[8192]; size_t used;} pilot_rx;
#define ring_buf_reset(ring) ((ring)->used = 0)
#define ring_buf_space_get(ring) (sizeof((ring)->bytes) - (ring)->used)
#define ring_buf_put(ring, data, count) (memcpy((ring)->bytes + (ring)->used, data, count), (ring)->used += (count))

#define MBEDTLS_ERR_SSL_WANT_READ (-0x6900)
#define MBEDTLS_ERR_SSL_WANT_WRITE (-0x6880)
typedef struct {int unused;} mbedtls_ssl_config, mbedtls_x509_crt, mbedtls_pk_context;
#define k_work_init_delayable(work, handler) ((void)(work))
static int tls, handshake = MBEDTLS_ERR_SSL_WANT_READ; /* what the handshake answers now */
static unsigned setups, ended_tls;
static const mbedtls_ssl_config *set_up;
#define mbedtls_ssl_init(context) ((void)(context))
#define mbedtls_ssl_free(context) ((void)(context), (void)++ended_tls)
#define mbedtls_ssl_config_free(config) ((void)(config))
#define mbedtls_x509_crt_free(certificate) ((void)(certificate))
#define mbedtls_ssl_set_bio(context, context_bio, send, receive, timeout) ((void)(context))
#define mbedtls_platform_zeroize(data, size) memset(data, 0, size)
static int mbedtls_ssl_setup(int *context, const mbedtls_ssl_config *config)
{
	(void)context;
	++setups;
	set_up = config;
	return 0;
}
static int mbedtls_ssl_handshake(int *context) { (void)context; return handshake; }
/* The bytes the phone wrote, as they are, 100 at a time. */
static int mbedtls_ssl_read(int *context, uint8_t *out, size_t size)
{
	(void)context;
	size_t count = pilot_rx.used < 100 ? pilot_rx.used : 100;
	if (count > size) count = size;
	if (!count) return MBEDTLS_ERR_SSL_WANT_READ;
	memcpy(out, pilot_rx.bytes, count);
	memmove(pilot_rx.bytes, pilot_rx.bytes + count, pilot_rx.used - count);
	pilot_rx.used -= count;
	return (int)count;
}

/* ---- the state of device_pilot.c ---- */
#define PILOT_TIMEOUT_MS 120000
typedef struct {
	uint64_t from, until;
} device_tls_v2_validity;
typedef struct {
	mbedtls_ssl_config config;
} device_tls_v2_profile;
struct provisioned_tls {
	mbedtls_x509_crt ca, board;
	mbedtls_pk_context key;
	device_tls_v2_profile profiles[2];
};
static struct provisioned_tls tls_banks[2];
static unsigned tls_bank = 1;
static struct bt_conn *connection, *ticket_peer, *display_connection;
static nu54_v2_profile profile;
static nu54_v2_link link;
static nu54_local_approval approval;
static nu54_pilot_review_binding reviewed_quote;
static unsigned refusal_slot, receipt_slot;
static nu54_owner_enrollment enrollment;
static nu54_v2_journal_entry journal_slots[8];
static int hash(const uint8_t *data, size_t length, uint8_t out[32])
{
	memset(out, 0, 32);
	for (size_t i = 0; i < length; ++i) out[i % 32] ^= data[i];
	return 0;
}
static nu54_v2_journal journal = {journal_slots, 8, hash, 0};
static bool initialized, ready, bound, recovering, clock_only, display_confirmed;
static int64_t clock_worker = -1;
static int64_t deadline, recovery_window, session_began;
static device_tls_v2_validity session_validity;
static nu54_owner_idle owner_idle;
static uint8_t display_challenge[32], active_quote[NU54_V2_SIGNED_SIZE], display_signature[64], rental_review_hash[32];
static size_t display_used;
static unsigned order_slot;
static uint64_t order_amount, order_expires, order_life;
static nu54_spend_action spend_action;
static struct {
	nu54_install_clock_told told;
	int64_t until;
	bool pending;
} clock_wait;
enum install_phase {INSTALL_IDLE, INSTALL_HANDSHAKE};
static enum install_phase install_phase;
static uint8_t install_bundle[4], install_offer[4];
static size_t install_used, install_total;
static mbedtls_ssl_config install_tls;
static mbedtls_x509_crt install_station;
static uint8_t wire[NU54_V2_FRAME_MAX + 2];
static size_t wire_length;
static atomic_t pending_prompt;

/* ---- the rest of the board ---- */
static nu54_vault vault = {.enabled = true, .record.generation = 1, .record.owner = {7, 7}};
static nu54_install_policy policy = {.not_after = 2000000000, .owner = {5, 5}, .terminal = {6, 6}, .payer_generation = 1};
static bool granted = true;
static uint64_t board_time = 1800000000; /* the board's latest possible time */
static uint32_t shown_prompt = 99;
static bool lamp_waits;
static unsigned installs_polled;

static nu54_vault *device_vault_owner(void) { return &vault; }
static const nu54_install_policy *device_install_policy(void) { return granted && board_time < policy.not_after ? &policy : NULL; }
static uint64_t trusted_now(void) { return board_time; }
static int pay_link_waiting(void) { return 0; }
static void board_io_set_prompt(uint32_t prompt) { shown_prompt = prompt; }
static void status_led_waiting(bool on) { lamp_waits = on; }
static int poll_install(int64_t now) { (void)now; ++installs_polled; return 0; }

/* What the phone's certificate says, and the key its handshake proved (as device_tls_v2_bind). */
static struct {
	uint64_t from, until;
	uint8_t key[32];
} presented;
static const uint8_t *asked_pin;
static bool asked_period;
static int device_tls_v2_bind(int *session, nu54_v2_link *out, nu54_v2_profile route, const uint8_t *expected_peer,
			      nu54_v2_journal *records, uint64_t now, device_tls_v2_validity *validity)
{
	static const uint8_t epoch[16] = {9};
	(void)session;
	asked_pin = expected_peer;
	asked_period = validity != NULL;
	const int window = nu54_install_certificate_window(presented.from, presented.until, now);
	if ((expected_peer && memcmp(expected_peer, presented.key, 32)) || window == NU54_CERTIFICATE_REFUSED ||
	    (window == NU54_CERTIFICATE_EARLY && !validity)) return -1;
	if (validity) *validity = (device_tls_v2_validity){presented.from, presented.until};
	return nu54_v2_link_init(out, route, NU54_V2_BOARD, epoch, presented.key, records);
}

/* A message that was read whole: its bytes, the session's age then, and what the board answers. */
static struct {
	unsigned count;
	size_t length;
	uint8_t first;
	int64_t session_age;
	int result;
} frames;
static int frame(const uint8_t *data, size_t length)
{
	++frames.count;
	frames.length = length;
	frames.first = data[0];
	frames.session_age = k_uptime_get() - session_began;
	return frames.result;
}

/* The start: what each step answers, and the order in which the board took them. */
static struct {
	int install, reset, restore;
	uint64_t hold;
	char steps[64];
	unsigned freed;
	int64_t resumed_at;
	uint64_t resumed;
} start;
static int step(const char *name, int result)
{
	strcat(start.steps, name);
	return result;
}
static int device_install_init(void) { return step("install,", start.install); }
static int device_spending_init(void) { return step("reset,", start.reset); }
static int device_install_restore(uint8_t *bundle, size_t capacity, mbedtls_x509_crt *ca, mbedtls_x509_crt *board,
				  mbedtls_pk_context *key, device_tls_v2_profile profiles[2])
{
	assert(bundle == install_bundle && capacity == sizeof(install_bundle) && ca == &tls_banks[tls_bank].ca);
	assert(board == &tls_banks[tls_bank].board && key == &tls_banks[tls_bank].key && profiles == tls_banks[tls_bank].profiles);
	return step("restore,", start.restore);
}
static void free_tls_bank(struct provisioned_tls *bank) { assert(bank == &tls_banks[tls_bank]); ++start.freed; }
static uint64_t device_install_order_hold(void) { return (uint64_t)step("hold,", (int)start.hold); }
static void device_order_resume(int64_t at, uint64_t seconds) { start.resumed_at = at; start.resumed = seconds; }
static bool device_pilot_active(void) { return true; }
static void clear_legacy_bonds(void) { (void)step("bonds,", 0); }
void device_pilot_init(void);

struct k_work;
static void reset_link(void);
static void poll_tls(struct k_work *ignored);
static ssize_t receive_bytes(struct bt_conn *conn, const struct bt_gatt_attr *attribute, const void *data, uint16_t length,
			     uint16_t offset, uint8_t flags);
#include "board_session.h"

/* ---- the phone and the passing of time ---- */
static struct bt_conn phone, other;

static ssize_t phone_writes(struct bt_conn *conn, const uint8_t *bytes, size_t length)
{
	return receive_bytes(conn, NULL, bytes, (uint16_t)length, 0, 0);
}

/* Runs the board for `milliseconds`: the TLS work item whenever it is due. */
static void run(int64_t milliseconds)
{
	for (const int64_t end = uptime + milliseconds; uptime < end; ++uptime) {
		if (worker >= 0 && worker <= uptime) {
			worker = -1;
			poll_tls(NULL);
			assert(!pilot_lock);
		}
	}
}

static void fresh(void)
{
	if (connection) reset_link();
	memset(&vault, 0, sizeof(vault));
	vault = (nu54_vault){.enabled = true, .record.generation = 1, .record.owner = {7, 7}};
	memset(&frames, 0, sizeof(frames));
	memset(&presented, 0, sizeof(presented));
	memcpy(presented.key, vault.record.owner, 32);
	presented.from = 1;
	presented.until = UINT64_MAX;
	handshake = MBEDTLS_ERR_SSL_WANT_READ;
	granted = ready = true;
	recovery_window = 0;
	phone.references = other.references = 0;
	worker = -1;
	setups = installs_polled = 0;
	run(1);
}

/* The route byte and one framed message of `size` bytes, written in pieces. */
static void open_and_send(uint8_t route, size_t size)
{
	uint8_t bytes[1 + 2 + 300] = {route, (uint8_t)(size >> 8), (uint8_t)size, 0x42};
	assert(size >= 23 && size <= 300);
	assert(phone_writes(&phone, bytes, 2) == 2);
	assert(phone_writes(&phone, bytes + 2, 1 + size) == (ssize_t)(1 + size));
}

/* ---- cases ---- */
static void starts_with(int install, int reset, int restore, uint64_t hold)
{
	memset(&start, 0, sizeof(start));
	start.install = install;
	start.reset = reset;
	start.restore = restore;
	start.hold = hold;
	start.resumed_at = -1;
	initialized = ready = false;
	clock_worker = -1;
	uptime += 300;
	device_pilot_init();
}

/* The start of the board. A reset that a power cut interrupted is finished before the stored grant
 * is looked for, and the hold of an order that was open when the phone moved the clock is taken up
 * after that, whether a grant was found or not. */
static void test_the_start(void)
{
	starts_with(0, 0, 0, 170);
	assert(!strcmp(start.steps, "install,reset,restore,hold,bonds,") && initialized && ready);
	assert(clock_worker == uptime && !start.freed && start.resumed == 170 && start.resumed_at == uptime);
	/* No grant to take up: nothing is saved for one, and the hold is asked for all the same. */
	starts_with(0, 0, 1, 240);
	assert(!strcmp(start.steps, "install,reset,restore,hold,bonds,") && initialized && !ready);
	assert(clock_worker < 0 && start.freed == 1 && start.resumed == 240 && start.resumed_at == uptime);
	starts_with(0, 0, 1, 0);
	assert(start.resumed_at == uptime && !start.resumed);
	/* The interrupted reset could not be finished, or the installation state is not readable:
	 * the board does not go on to a grant. */
	starts_with(0, -1, 0, 170);
	assert(!strcmp(start.steps, "install,reset,") && !initialized && !ready && clock_worker < 0 && start.resumed_at < 0);
	starts_with(-1, 0, 0, 170);
	assert(!strcmp(start.steps, "install,") && !initialized && !ready && start.resumed_at < 0);
}

static void test_the_first_write_opens_the_session(void)
{
	static const uint8_t route[1] = {NU54_V2_MANAGEMENT}, more[3] = {1, 2, 3}, unknown[1] = {9};
	/* Nothing is taken without a grant, and nothing but one of the two routes opens a session. */
	fresh();
	ready = false;
	assert(phone_writes(&phone, route, 1) < 0 && !connection && !setups && !phone.references && worker < 0);
	ready = true;
	granted = false;
	assert(phone_writes(&phone, route, 1) < 0 && !connection);
	granted = true;
	assert(phone_writes(&phone, unknown, 1) < 0 && !connection && !setups);
	assert(receive_bytes(&phone, NULL, route, 0, 0, 0) < 0 && receive_bytes(&phone, NULL, route, 1, 1, 0) < 0 && !connection);

	/* The first byte names the route and is not part of the TLS stream. The session's age and its
	 * two minutes count from this write, and the TLS work item is due at once. */
	uptime += 5000;
	assert(phone_writes(&phone, route, 1) == 1);
	assert(connection == &phone && phone.references == 1 && profile == NU54_V2_MANAGEMENT && setups == 1);
	assert(set_up == &tls_banks[tls_bank].profiles[0].config); /* the grant in use, the owner's route */
	assert(session_began == uptime && deadline == uptime + PILOT_TIMEOUT_MS && !pilot_rx.used && worker == uptime);
	const int64_t began = session_began;
	uptime += 700;
	assert(phone_writes(&phone, more, 3) == 3 && pilot_rx.used == 3 && !memcmp(pilot_rx.bytes, more, 3));
	assert(session_began == began && deadline == began + PILOT_TIMEOUT_MS && setups == 1 && phone.references == 1);
	/* Another central does not write into it. */
	assert(phone_writes(&other, more, 3) < 0 && pilot_rx.used == 3 && !other.references && connection == &phone);
	/* More than the board can hold ends the session. */
	static uint8_t flood[8192];
	assert(phone_writes(&phone, flood, sizeof(flood)) < 0 && !connection && !phone.references && !pilot_rx.used);

	/* The payment route takes the other configuration; during the recovery window only the
	 * owner's route opens. */
	static const uint8_t payment[1] = {NU54_V2_PAYMENT};
	fresh();
	assert(phone_writes(&phone, payment, 1) == 1 && profile == NU54_V2_PAYMENT);
	assert(set_up == &tls_banks[tls_bank].profiles[1].config);
	fresh();
	recovery_window = uptime + 1000;
	assert(phone_writes(&phone, payment, 1) < 0 && !connection);
	assert(phone_writes(&phone, route, 1) == 1 && connection == &phone);
}

static void test_the_work_item_binds_and_feeds_it(void)
{
	/* While the handshake runs the work item comes back every 25 ms and reads nothing. */
	fresh();
	journal_slots[3].retain_until = 5; /* a record of the session before */
	open_and_send(NU54_V2_MANAGEMENT, 40);
	run(200);
	assert(connection && !bound && !frames.count && worker >= uptime && worker - uptime <= 25);
	/* The handshake is done: the session is bound to the owner the vault holds, with the period
	 * of the certificate asked for, the records of the session before are gone, and the message
	 * that was waiting is read whole and handed on once. */
	handshake = 0;
	run(100);
	assert(bound && !recovering && !clock_only && asked_pin == vault.record.owner && asked_period);
	assert(session_validity.from == 1 && session_validity.until == UINT64_MAX && !journal_slots[3].retain_until);
	assert(link.peer == NU54_V2_OWNER && !link.closed);
	assert(frames.count == 1 && frames.length == 40 && frames.first == 0x42 && !wire_length && connection);
	/* What the handler sees as the session's age is the time since the first write. */
	assert(frames.session_age >= 200 && frames.session_age <= 300);
	/* A second message in the same session, with nothing bound anew. */
	asked_pin = NULL;
	uint8_t next[2 + 30] = {0, 30, 0x43};
	assert(phone_writes(&phone, next, sizeof(next)) == sizeof(next));
	run(100);
	assert(frames.count == 2 && frames.length == 30 && frames.first == 0x43 && !asked_pin);

	/* A board without an owner binds to the phone its grant names, and asks for no period: only a
	 * registered owner comes in with a certificate that is not valid yet. */
	fresh();
	vault.record.generation = 0;
	memcpy(presented.key, policy.owner, 32);
	handshake = 0;
	open_and_send(NU54_V2_MANAGEMENT, 40);
	run(100);
	assert(bound && asked_pin == policy.owner && !asked_period && !session_validity.until && !clock_only);
	fresh();
	vault.record.generation = 0;
	memcpy(presented.key, policy.owner, 32);
	presented.from = board_time + 3600;
	handshake = 0;
	open_and_send(NU54_V2_MANAGEMENT, 40);
	run(100);
	assert(!connection && !frames.count);
	/* The registered owner with such a certificate gets a session that may only tell the time. */
	fresh();
	presented.from = board_time + 3600;
	handshake = 0;
	open_and_send(NU54_V2_MANAGEMENT, 40);
	run(100);
	assert(bound && clock_only && session_validity.from == board_time + 3600 && frames.count == 1);
	/* The payment route binds to the terminal of the grant. */
	fresh();
	memcpy(presented.key, policy.terminal, 32);
	handshake = 0;
	open_and_send(NU54_V2_PAYMENT, 40);
	run(100);
	assert(bound && asked_pin == policy.terminal && !asked_period && link.peer == NU54_V2_TERMINAL);
}

static void test_what_ends_it(void)
{
	/* A handshake that fails, another key, a certificate that has ended. */
	for (int how = 0; how < 3; ++how) {
		fresh();
		open_and_send(NU54_V2_MANAGEMENT, 40);
		handshake = how == 0 ? -1 : 0;
		if (how == 1) presented.key[0] ^= 1;
		if (how == 2) presented.until = board_time;
		run(100);
		assert(!connection && !phone.references && !bound && !frames.count && link.closed && worker < 0);
	}
	/* A message the board does not take, and a length no message has. */
	fresh();
	handshake = 0;
	frames.result = -1;
	open_and_send(NU54_V2_MANAGEMENT, 40);
	run(100);
	assert(frames.count == 1 && !connection && !phone.references && link.closed && !wire_length);
	fresh();
	handshake = 0;
	static const uint8_t short_one[3] = {NU54_V2_MANAGEMENT, 0, 22};
	assert(phone_writes(&phone, short_one, 3) == 3);
	run(100);
	assert(!frames.count && !connection);
	/* The grant ends, or is gone. */
	fresh();
	handshake = 0;
	open_and_send(NU54_V2_MANAGEMENT, 40);
	run(100);
	assert(connection);
	granted = false;
	run(100);
	assert(!connection && !phone.references);
	/* Two minutes after the first write, whatever went on. */
	fresh();
	handshake = 0;
	open_and_send(NU54_V2_MANAGEMENT, 40);
	run(PILOT_TIMEOUT_MS - 100);
	assert(connection && bound);
	run(200);
	assert(!connection && !phone.references && worker < 0);
	/* The same while the handshake never ends. */
	fresh();
	open_and_send(NU54_V2_MANAGEMENT, 40);
	run(PILOT_TIMEOUT_MS + 100);
	assert(!connection);
}

/* A time from the owner's phone that waits for SW1 (clock_request): the work item drops it and
 * ends the session when the message has ended by its own word, and the end of a session takes
 * the wait, the prompt and the lamp with it. */
static void test_a_time_that_waits_for_the_button(void)
{
	fresh();
	handshake = 0;
	open_and_send(NU54_V2_MANAGEMENT, 40);
	run(100);
	assert(bound);
	clock_wait.told = (nu54_install_clock_told){.time = 5, .expires = 95, .received = uptime};
	clock_wait.until = uptime + 90000;
	clock_wait.pending = true;
	clock_only = true;
	atomic_set(&pending_prompt, 77);
	shown_prompt = 77;
	lamp_waits = true;
	run(89999);
	assert(connection && clock_wait.pending && atomic_get(&pending_prompt) == 77 && lamp_waits);
	run(30);
	assert(!connection && !phone.references && !clock_wait.pending && !clock_wait.until && !clock_wait.told.time);
	assert(!atomic_get(&pending_prompt) && !shown_prompt && !lamp_waits && !clock_only && !bound && link.closed);
	assert(!session_validity.from && !session_validity.until);

	/* Any other end of the session does the same: here the phone goes away. */
	fresh();
	handshake = 0;
	open_and_send(NU54_V2_MANAGEMENT, 40);
	run(100);
	clock_wait.until = uptime + 90000;
	clock_wait.pending = true;
	atomic_set(&pending_prompt, 78);
	shown_prompt = 78;
	lamp_waits = true;
	reset_link(); /* disconnected() */
	assert(!clock_wait.pending && !atomic_get(&pending_prompt) && !shown_prompt && !lamp_waits && !connection);
	/* Without a wait the work item ends nothing on its account. */
	fresh();
	handshake = 0;
	open_and_send(NU54_V2_MANAGEMENT, 40);
	clock_wait.until = uptime - 1; /* left over, not pending */
	run(1000);
	assert(connection && bound);
}

int main(void)
{
	test_the_start();
	test_the_first_write_opens_the_session();
	test_the_work_item_binds_and_feeds_it();
	test_what_ends_it();
	test_a_time_that_waits_for_the_button();
	puts("pilot session ok");
	return 0;
}
