/*
 * The payment link's session runner: one work queue owns the session (core/nu54_session), the
 * link router (core/nu54_links) and the reassemblers. Bluetooth threads only queue events
 * (ble_links.c handlers), the main loop only queues the button and the PIN, so nothing here is
 * shared between threads except public prompt IDs and the transient local PIN mailbox.
 */
#include "pay_link.h"

#include <errno.h>
#include <string.h>

#include <app_version.h>
#include <psa/crypto.h>
#include <zephyr/kernel.h>
#include <zephyr/logging/log.h>
#include <zephyr/sys/atomic.h>

#include "ble_links.h"
#include "board_io.h"
#include "device_setup.h"
#include "device_pilot.h"
#include "nu54_pin_entry.h"
#include "nu54_cbor.h"
#include "nu54_frame.h"
#include "nu54_links.h"
#include "nu54_pin_entry.h"
#include "nu54_session.h"
#include "status_led.h"

LOG_MODULE_REGISTER(pay_link, LOG_LEVEL_INF);

BUILD_ASSERT(CONFIG_BT_MAX_CONN <= NU54_LINK_MAX, "the link router tracks NU54_LINK_MAX centrals");

/* ---------------------------------------------------------------- state (work queue only) */

static nu54_device_t device;
#if defined(CONFIG_NU54_EXPERIMENTAL_RENTAL_JOURNAL_PS)
_Static_assert(BLE_RENTAL_PEER_SIZE == NU54_RENTAL_PEER_SIZE, "collector identity layout");
#endif

static nu54_links_t router;
static nu54_reassembler_t rx[NU54_LINK_MAX];
static uint64_t link_epochs[NU54_LINK_MAX];
static uint64_t reply_epoch;

/* ---------------------------------------------------------------- events from other threads */

typedef enum { EV_CONNECTED, EV_DISCONNECTED, EV_FRAGMENT, EV_BUTTON, EV_PIN, EV_WALLET_CHECK_TIMEOUT } event_type_t;

typedef struct {
	uint8_t type;
	int8_t link;
	uint16_t len; /* fragment length or button decision */
	uint32_t prompt;
	uint64_t epoch;
	uint8_t data[BLE_LINKS_FRAGMENT_MAX];
} event_t;

/* Connection events must not be lost, so the queue leaves room beyond a burst of fragments. */
K_MSGQ_DEFINE(events, sizeof(event_t), 24, 4);

static K_THREAD_STACK_DEFINE(work_stack, 12288);
static struct k_work_q work_q;
static void event_work_handler(struct k_work *w);
static K_WORK_DEFINE(event_work, event_work_handler);
static atomic_t local_prompt;
static atomic_t local_pin_prompt;
static uint32_t prompt_counter;
static void pin_deadline(struct k_work *work);
static K_WORK_DELAYABLE_DEFINE(pin_timer, pin_deadline);
static struct k_spinlock pin_lock;
static uint32_t pin_mailbox_prompt;
static char pin_mailbox[NU54_PIN_LEN];
static size_t pin_mailbox_length;

static void clear_pin_mailbox(void)
{
	memset(pin_mailbox, 0, sizeof(pin_mailbox));
	pin_mailbox_prompt = 0;
	pin_mailbox_length = 0;
}

static void refresh_prompt(nu54_pending_t before, const uint8_t session[8])
{
	if (before == device.pending && memcmp(session, device.session_id, 8) == 0) return;
	k_spinlock_key_t key = k_spin_lock(&pin_lock);
	clear_pin_mailbox();
	if (device.pending == NU54_PENDING_NONE || prompt_counter == NU54_PILOT_PROMPT_MASK - 1) {
		atomic_set(&local_prompt, 0);
	} else {
		atomic_set(&local_prompt, (atomic_val_t)++prompt_counter);
	}
	bool pin_required = device.pending == NU54_PENDING_PIN || device.pending == NU54_PENDING_LIMIT_PIN;
	atomic_set(&local_pin_prompt, pin_required ? atomic_get(&local_prompt) : 0);
	board_io_set_prompt((uint32_t)atomic_get(&local_prompt));
	k_spin_unlock(&pin_lock, key);
	k_work_cancel_delayable(&pin_timer);
	if (pin_required)
		k_work_reschedule_for_queue(&work_q, &pin_timer, K_MSEC(NU54_PIN_ENTRY_MS));
}

static int post(const event_t *ev)
{
	int err = k_msgq_put(&events, ev, K_NO_WAIT);
	if (err) {
		LOG_ERR("event %d for link %d dropped: queue full", ev->type, ev->link);
		return err;
	}
	k_work_submit_to_queue(&work_q, &event_work);
	return 0;
}

static void on_connected(int link, uint64_t epoch)
{
	post(&(event_t){.type = EV_CONNECTED, .link = (int8_t)link, .epoch = epoch});
}

static void on_disconnected(int link, uint64_t epoch)
{
	post(&(event_t){.type = EV_DISCONNECTED, .link = (int8_t)link, .epoch = epoch});
}

static int on_fragment(int link, uint64_t epoch, const uint8_t *data, uint16_t len)
{
	static event_t ev; /* Bluetooth RX thread only */
	ev = (event_t){.type = EV_FRAGMENT, .link = (int8_t)link, .len = len, .epoch = epoch};
	memcpy(ev.data, data, len);
	return post(&ev);
}

static const ble_links_handlers_t handlers = {
	.connected = on_connected,
	.disconnected = on_disconnected,
	.fragment = on_fragment,
};

/* A wallet.check waits this long for the button (the phone app shows the same countdown). */
#define WALLET_CHECK_MS 60000

static void wallet_check_timeout(struct k_work *w)
{
	(void)w;
	post(&(event_t){.type = EV_WALLET_CHECK_TIMEOUT});
}
static K_WORK_DELAYABLE_DEFINE(wallet_check_timer, wallet_check_timeout);

/* ---------------------------------------------------------------- delivery */

/* The board's facts about every link, as the router needs them now. */
static void refresh_links(void)
{
	bool need_passkey = device.state != NU54_STATE_UNPROVISIONED;
	for (int i = 0; i < NU54_LINK_MAX; i++) {
		nu54_links_facts(&router, i, ble_links_bonded(i, need_passkey), ble_links_listening(i));
	}
}

/* Shows the kiosk's payment.outcome (as forwarded to the phone app) on the LEDs, and a signed
 * wallet.check as approved (a refused one is a REFUSED event already). */
static void show_outcome(const uint8_t *body, size_t len)
{
	static nu54_msg_t m;
	if (nu54_msg_decode(body, len, &m) != NU54_MSG_OK) {
		return;
	}
	if (strcmp(m.message->type, "wallet.check.result") == 0) {
		if (m.items[nu54_msg_find(&m, 0, "accepted")].value) {
			status_led_outcome(true);
		}
		LOG_INF("wallet check answered");
		return;
	}
	if (strcmp(m.message->type, "payment.outcome") != 0) {
		return;
	}
	int o = nu54_msg_find(&m, 0, "outcome");
	bool approved = m.items[o].len == 8 && memcmp(m.items[o].ptr, "approved", 8) == 0;
	status_led_outcome(approved);
	LOG_INF("payment outcome: %.*s", (int)m.items[o].len, m.items[o].ptr);
}

/* Replies to the central they answer; the phone body to every phone app link; then the LEDs. */
static void deliver(const nu54_out_t *out, int reply_link, uint64_t epoch)
{
	for (int i = 0; i < out->kiosk_count; i++) {
		if (epoch) ble_links_send_epoch(reply_link, epoch, out->kiosk[i], out->kiosk_len[i]);
	}
	if (out->phone_count) {
		int sent = 0;
		show_outcome(out->phone, out->phone_len);
		refresh_links();
		for (int i = 0; i < NU54_LINK_MAX; i++) {
			if (nu54_links_is_phone(&router, i)) {
				sent += ble_links_send(i, out->phone, out->phone_len) == 0;
			}
		}
		if (!sent) {
			/* Development builds go on without a phone app (N30); release builds refused earlier. */
			LOG_INF("no phone app listening; SW1 approves, SW2 rejects");
		}
	}
	status_led_event(out->event);
	/* The buttons decide; while the PIN is asked for, main.c's PIN entry shows the digits. */
	status_led_waiting(device.pending == NU54_PENDING_PAYMENT || device.pending == NU54_PENDING_SETUP_CONFIRM ||
			   device.pending == NU54_PENDING_LIMIT_CONFIRM || device.pending == NU54_PENDING_WALLET_CHECK);
	if (device.pending != NU54_PENDING_WALLET_CHECK) {
		k_work_cancel_delayable(&wallet_check_timer);
	}
}

/* ---------------------------------------------------------------- the work queue */

static void handle_body(int link, uint64_t epoch, const uint8_t *body, size_t len)
{
	if (!ble_links_connection_matches(link, epoch)) return;
	if (IS_ENABLED(CONFIG_NU54_PILOT_TLS) && device_pilot_active()) return;
	static nu54_out_t out;
	uint8_t sid[8];
	int was_open = device.session_open;

	nu54_pending_t asked = device.pending;

	memcpy(sid, device.session_id, 8);
	refresh_links();
	nu54_link_t call = nu54_links_call(&router, link, device.session_open);
#if defined(CONFIG_NU54_EXPERIMENTAL_RENTAL_JOURNAL_PS)
	if (call.bonded && ble_links_rental_peer(link, epoch, call.peer))
		call.connection_epoch = epoch;
#endif
	nu54_session_handle(&device, &call, body, len, k_uptime_get() / 1000, &out);
	/* The kiosk gave up on the renter's step (session.cancel after its wait, or a new session):
	 * the step ends without a reply, but the lamp shows it as cancelled. */
	if (asked != NU54_PENDING_NONE && device.pending == NU54_PENDING_NONE && out.event == NU54_EVENT_NONE) {
		out.event = NU54_EVENT_REFUSED;
	}
	int opened = device.session_open && (!was_open || memcmp(sid, device.session_id, 8) != 0);
	nu54_links_after(&router, link, opened);
	if (opened) {
		reply_epoch = epoch;
		status_led_new_session();
	}
	LOG_INF("message on link %d: %u-byte body, %d replies", link, (unsigned)len, out.kiosk_count);
	if (device.pending == NU54_PENDING_WALLET_CHECK && asked != NU54_PENDING_WALLET_CHECK) {
		LOG_INF("wallet check: SW1 signs, SW2 refuses");
		k_work_reschedule_for_queue(&work_q, &wallet_check_timer, K_MSEC(WALLET_CHECK_MS));
	}
	deliver(&out, link, epoch);
}

static void on_fragment_event(const event_t *ev)
{
	if ((unsigned)ev->link >= NU54_LINK_MAX || link_epochs[ev->link] != ev->epoch ||
	    !ble_links_connection_matches(ev->link, ev->epoch)) return;
	nu54_reassembler_t *r = &rx[ev->link];
	nu54_frame_result_t res = nu54_reassembler_feed(r, ev->data, ev->len);
	const uint8_t *body;
	size_t blen;
	uint8_t digest[32];
	size_t dlen;

	if (res == NU54_FRAME_NEED_MORE) {
		return;
	}
	if (res == NU54_FRAME_DONE) {
		body = nu54_reassembler_body(r, &blen);
		if (psa_hash_compute(PSA_ALG_SHA_256, body, blen, digest, sizeof(digest), &dlen) == PSA_SUCCESS &&
		    memcmp(digest, nu54_reassembler_digest(r), 8) == 0) {
			handle_body(ev->link, ev->epoch, body, blen);
			return;
		}
		LOG_WRN("BAD_FRAME: envelope digest mismatch (%u-byte body)", (unsigned)blen);
	} else {
		LOG_WRN("BAD_FRAME: fragment order or length (%u bytes, seq %u idx %u)", ev->len, ev->data[0], ev->data[1]);
	}
	/* Order, length or digest violation: an empty body is not a valid message, so the session
	 * answers error{BAD_FRAME} and closes, as for any malformed message. */
	nu54_reassembler_reset(r);
	handle_body(ev->link, ev->epoch, NULL, 0);
}

/*
 * The central that held the session left: the payment (or setup) is over however it ended -
 * approved, refused, timed out and cancelled by the kiosk, or an error. The device goes back to
 * the start: the session ends, payment mode ends (the next payment starts with SW4 or the phone
 * app), and the lamp goes dark once the result it is showing has played.
 */
static void on_disconnected_event(int link, uint64_t epoch)
{
	nu54_device_rental_peer_closed(&device, epoch);
	if ((unsigned)link >= NU54_LINK_MAX || link_epochs[link] != epoch) return;
	link_epochs[link] = 0;
	bool waited = device.pending != NU54_PENDING_NONE;
	if (!nu54_links_disconnected(&router, link)) {
		/* The phone app that asked for a wallet check left: nobody can read the answer. */
		refresh_links();
		bool phone_left = true;
		for (int i = 0; i < NU54_LINK_MAX; i++) {
			phone_left &= !nu54_links_is_phone(&router, i);
		}
		if (device.pending == NU54_PENDING_WALLET_CHECK && phone_left) {
			static nu54_out_t out;
			memset(&out, 0, sizeof(out));
			nu54_session_wallet_check_end(&device, &out);
			status_led_event(NU54_EVENT_REFUSED);
			status_led_waiting(false);
			k_work_cancel_delayable(&wallet_check_timer);
			LOG_INF("wallet check ended: the phone app left");
		}
		return;
	}
	reply_epoch = 0;
	nu54_session_link_closed(&device);
	if (waited) {
		status_led_event(NU54_EVENT_REFUSED); /* the renter's step was cancelled */
	}
	ble_links_payment_mode(0);
	status_led_idle();
	LOG_INF("session over: back to idle");
}

/* The renter's button or PIN: replies go to the central that holds the session. */
static void on_renter_event(const event_t *ev)
{
	static nu54_out_t out;
	if (!ev->prompt || ev->prompt != (uint32_t)atomic_get(&local_prompt)) return;
	bool unprovisioned = device.state == NU54_STATE_UNPROVISIONED;
	memset(&out, 0, sizeof(out));
	if (ev->type == EV_BUTTON) {
		nu54_session_button(&device, ev->len, &out);
	} else {
		char pin[NU54_PIN_LEN];
		k_spinlock_key_t key = k_spin_lock(&pin_lock);
		size_t length = pin_mailbox_prompt == ev->prompt ? pin_mailbox_length : 0;
		memcpy(pin, pin_mailbox, sizeof(pin));
		clear_pin_mailbox();
		k_spin_unlock(&pin_lock, key);
		nu54_session_pin(&device, length ? pin : NULL, length, &out);
		memset(pin, 0, sizeof(pin));
	}
	deliver(&out, nu54_links_session(&router), reply_epoch);
	/* Setup just stored the key and passkey: the phone app's Just Works bond is not enough any
	 * more (passkey bonds only, payment-protocol.md 3), so pairing mode opens with the new passkey
	 * for the app to bond again without another long press. */
	if (unprovisioned && device.state != NU54_STATE_UNPROVISIONED) {
		(void)ble_links_pairing_mode(120, device.passkey, false);
	}
}

static void pin_deadline(struct k_work *work)
{
	(void)work;
	uint32_t prompt = (uint32_t)atomic_get(&local_pin_prompt);
	if (!prompt) return;
	nu54_pending_t before = device.pending;
	uint8_t session[8];
	memcpy(session, device.session_id, sizeof(session));
	on_renter_event(&(event_t){.type = EV_PIN, .prompt = prompt});
	refresh_prompt(before, session);
}

static void event_work_handler(struct k_work *w)
{
	static event_t ev;
	(void)w;
	while (k_msgq_get(&events, &ev, K_NO_WAIT) == 0) {
		nu54_pending_t before = device.pending;
		uint8_t session[8];
		memcpy(session, device.session_id, sizeof(session));
		switch (ev.type) {
		case EV_CONNECTED:
			if (!ble_links_connection_matches(ev.link, ev.epoch)) break;
			if (link_epochs[ev.link] && link_epochs[ev.link] != ev.epoch)
				on_disconnected_event(ev.link, link_epochs[ev.link]);
			link_epochs[ev.link] = ev.epoch;
			nu54_links_connected(&router, ev.link);
			nu54_reassembler_reset(&rx[ev.link]);
			break;
		case EV_DISCONNECTED:
			on_disconnected_event(ev.link, ev.epoch);
			break;
		case EV_FRAGMENT:
			on_fragment_event(&ev);
			break;
		case EV_BUTTON:
			on_renter_event(&ev);
			break;
		case EV_PIN:
			break;
		case EV_WALLET_CHECK_TIMEOUT: {
			static nu54_out_t out;
			memset(&out, 0, sizeof(out));
			if (device.pending == NU54_PENDING_WALLET_CHECK) {
				LOG_INF("wallet check timed out");
				nu54_session_wallet_check_end(&device, &out);
				deliver(&out, -1, 0);
			}
			break;
		}
		}
		refresh_prompt(before, session);
	}
}

/* ---------------------------------------------------------------- the main loop's calls */

void pay_link_button(uint32_t prompt, int approve)
{
	/* Read on another thread: a stale value only decides whether to log, the session re-checks. */
	if (!prompt) {
		LOG_INF("no payment is waiting for the button");
		return;
	}
	LOG_INF("%s", approve ? "approved by the button" : "rejected by the button");
	post(&(event_t){.type = EV_BUTTON, .len = (uint16_t)(approve ? 1 : 0), .prompt = prompt});
}

void pay_link_pin(uint32_t prompt, const char *pin, size_t len)
{
	if (!prompt || (pin && !nu54_pin_is_local_format(pin, len))) return;
	k_spinlock_key_t key = k_spin_lock(&pin_lock);
	if ((uint32_t)atomic_get(&local_prompt) != prompt || pin_mailbox_prompt) {
		k_spin_unlock(&pin_lock, key);
		return;
	}
	pin_mailbox_prompt = prompt;
	if (pin) {
		memcpy(pin_mailbox, pin, len);
		pin_mailbox_length = len;
	}
	k_spin_unlock(&pin_lock, key);
	/* A fixed work-queue deadline handles entered digits and timeout alike. */
}

int pay_link_waiting(void)
{
	return atomic_get(&local_prompt) != 0;
}

uint32_t pay_link_pin_prompt(void)
{
	return (uint32_t)atomic_get(&local_pin_prompt);
}


int pay_link_address(uint8_t address[20])
{
	memcpy(address, device.address, 20);
	return device.state != NU54_STATE_UNPROVISIONED;
}

int pay_link_payment_mode(uint32_t seconds)
{
	return ble_links_payment_mode(seconds);
}

int pay_link_pairing_mode(uint32_t seconds)
{
	bool unprovisioned = device.state == NU54_STATE_UNPROVISIONED;
	return ble_links_pairing_mode(seconds, device.passkey, unprovisioned);
}

/* ---------------------------------------------------------------- init */

/* device.paymentMode from the phone app (payment-protocol.md 3, P02-FR-08); on the work queue. */
static int platform_payment_mode(void *ctx, int on, uint32_t seconds)
{
	(void)ctx;
	return ble_links_payment_mode(on ? seconds : 0);
}

int pay_link_init(void)
{
	int err;

	nu54_links_init(&router);
	err = device_setup_load(&device);
	if (err) {
		return err;
	}
	device.cfg.firmware = APP_VERSION_STRING; /* app/VERSION, also the signed image version */
	device.cfg.anchor_clock_skew = 60;
	device.cfg.authorization_expiry = 120;
	device.cfg.require_secure = IS_ENABLED(CONFIG_NU54_REQUIRE_SECURE_SESSION);
	device.cfg.require_phone = IS_ENABLED(CONFIG_NU54_REQUIRE_PHONE);
	device.cfg.dev_unpaired_anchor = IS_ENABLED(CONFIG_NU54_DEV_UNPAIRED_ANCHOR);
	device.cfg.platform.payment_mode = platform_payment_mode;
	k_work_queue_start(&work_q, work_stack, K_THREAD_STACK_SIZEOF(work_stack), K_PRIO_PREEMPT(7), NULL);
	/* The legacy setup record is independent of the commercial C1 owner vault. */
	const bool legacy_unprovisioned = device.state == NU54_STATE_UNPROVISIONED &&
		!(IS_ENABLED(CONFIG_NU54_PILOT_TLS) && device_pilot_active());
	err = ble_links_init(&handlers, device.passkey, legacy_unprovisioned);
	if (err) {
		return err;
	}
	if (IS_ENABLED(CONFIG_NU54_PILOT_TLS)) device_pilot_init();
	LOG_INF("payment link ready (state %d)", (int)device.state);
	return 0;
}
