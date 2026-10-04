/*
 * The payment link's session runner: one work queue owns the session (core/nu54_session), the
 * link router (core/nu54_links) and the reassemblers. Bluetooth threads only queue events
 * (ble_links.c handlers), the main loop only queues the button and the PIN, so nothing here is
 * shared between threads except the event queue.
 */
#include "pay_link.h"

#include <errno.h>
#include <string.h>

#include <app_version.h>
#include <psa/crypto.h>
#include <zephyr/kernel.h>
#include <zephyr/logging/log.h>

#include "ble_links.h"
#include "device_setup.h"
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
static nu54_links_t router;
static nu54_reassembler_t rx[NU54_LINK_MAX];

/* ---------------------------------------------------------------- events from other threads */

typedef enum { EV_CONNECTED, EV_DISCONNECTED, EV_FRAGMENT, EV_BUTTON, EV_PIN } event_type_t;

typedef struct {
	uint8_t type;
	int8_t link;
	uint16_t len; /* fragment length; PIN: digits, 0 for a timeout */
	uint8_t data[BLE_LINKS_FRAGMENT_MAX];
} event_t;

/* Connection events must not be lost, so the queue leaves room beyond a burst of fragments. */
K_MSGQ_DEFINE(events, sizeof(event_t), 24, 4);

static K_THREAD_STACK_DEFINE(work_stack, 12288);
static struct k_work_q work_q;
static void event_work_handler(struct k_work *w);
static K_WORK_DEFINE(event_work, event_work_handler);

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

static void on_connected(int link)
{
	post(&(event_t){.type = EV_CONNECTED, .link = (int8_t)link});
}

static void on_disconnected(int link)
{
	post(&(event_t){.type = EV_DISCONNECTED, .link = (int8_t)link});
}

static int on_fragment(int link, const uint8_t *data, uint16_t len)
{
	static event_t ev; /* Bluetooth RX thread only */
	ev = (event_t){.type = EV_FRAGMENT, .link = (int8_t)link, .len = len};
	memcpy(ev.data, data, len);
	return post(&ev);
}

static const ble_links_handlers_t handlers = {
	.connected = on_connected,
	.disconnected = on_disconnected,
	.fragment = on_fragment,
};

/* ---------------------------------------------------------------- delivery */

/* The board's facts about every link, as the router needs them now. */
static void refresh_links(void)
{
	bool need_passkey = device.state != NU54_STATE_UNPROVISIONED;
	for (int i = 0; i < NU54_LINK_MAX; i++) {
		nu54_links_facts(&router, i, ble_links_bonded(i, need_passkey), ble_links_listening(i));
	}
}

/* Shows the kiosk's payment.outcome (as forwarded to the phone app) on the LEDs. */
static void show_outcome(const uint8_t *body, size_t len)
{
	static nu54_msg_t m;
	if (nu54_msg_decode(body, len, &m) != NU54_MSG_OK || strcmp(m.message->type, "payment.outcome") != 0) {
		return;
	}
	int o = nu54_msg_find(&m, 0, "outcome");
	bool approved = m.items[o].len == 8 && memcmp(m.items[o].ptr, "approved", 8) == 0;
	status_led_outcome(approved);
	LOG_INF("payment outcome: %.*s", (int)m.items[o].len, m.items[o].ptr);
}

/* Replies to the central they answer; the phone body to every phone app link; then the LEDs. */
static void deliver(const nu54_out_t *out, int reply_link)
{
	for (int i = 0; i < out->kiosk_count; i++) {
		ble_links_send(reply_link, out->kiosk[i], out->kiosk_len[i]);
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
			   device.pending == NU54_PENDING_LIMIT_CONFIRM);
}

/* ---------------------------------------------------------------- the work queue */

static void handle_body(int link, const uint8_t *body, size_t len)
{
	static nu54_out_t out;
	uint8_t sid[8];
	int was_open = device.session_open;

	memcpy(sid, device.session_id, 8);
	refresh_links();
	const nu54_link_t call = nu54_links_call(&router, link, device.session_open);
	nu54_session_handle(&device, &call, body, len, k_uptime_get() / 1000, &out);
	int opened = device.session_open && (!was_open || memcmp(sid, device.session_id, 8) != 0);
	nu54_links_after(&router, link, opened);
	if (opened) {
		status_led_new_session();
	}
	LOG_INF("message on link %d: %u-byte body, %d replies", link, (unsigned)len, out.kiosk_count);
	deliver(&out, link);
}

static void on_fragment_event(const event_t *ev)
{
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
			handle_body(ev->link, body, blen);
			return;
		}
		LOG_WRN("BAD_FRAME: envelope digest mismatch (%u-byte body)", (unsigned)blen);
	} else {
		LOG_WRN("BAD_FRAME: fragment order or length (%u bytes, seq %u idx %u)", ev->len, ev->data[0], ev->data[1]);
	}
	/* Order, length or digest violation: an empty body is not a valid message, so the session
	 * answers error{BAD_FRAME} and closes, as for any malformed message. */
	nu54_reassembler_reset(r);
	handle_body(ev->link, NULL, 0);
}

static void on_disconnected_event(int link)
{
	bool waited = device.pending != NU54_PENDING_NONE;
	if (!nu54_links_disconnected(&router, link)) {
		return;
	}
	/* The link held the session: it ends with it, and a step the renter was on is cancelled. */
	nu54_session_link_closed(&device);
	status_led_waiting(false);
	if (waited) {
		status_led_event(NU54_EVENT_REFUSED);
	}
}

/* The renter's button or PIN: replies go to the central that holds the session. */
static void on_renter_event(const event_t *ev)
{
	static nu54_out_t out;
	memset(&out, 0, sizeof(out));
	if (ev->type == EV_BUTTON) {
		nu54_session_button(&device, ev->len, &out);
	} else {
		nu54_session_pin(&device, ev->len ? (const char *)ev->data : NULL, ev->len, &out);
	}
	deliver(&out, nu54_links_session(&router));
}

static void event_work_handler(struct k_work *w)
{
	static event_t ev;
	(void)w;
	while (k_msgq_get(&events, &ev, K_NO_WAIT) == 0) {
		switch (ev.type) {
		case EV_CONNECTED:
			nu54_links_connected(&router, ev.link);
			nu54_reassembler_reset(&rx[ev.link]);
			break;
		case EV_DISCONNECTED:
			on_disconnected_event(ev.link);
			break;
		case EV_FRAGMENT:
			on_fragment_event(&ev);
			break;
		case EV_BUTTON:
		case EV_PIN:
			on_renter_event(&ev);
			break;
		}
		memset(ev.data, 0, sizeof(ev.data)); /* a PIN never outlives its event */
	}
}

/* ---------------------------------------------------------------- the main loop's calls */

void pay_link_button(int approve)
{
	/* Read on another thread: a stale value only decides whether to log, the session re-checks. */
	if (device.pending == NU54_PENDING_NONE) {
		LOG_INF("no payment is waiting for the button");
		return;
	}
	LOG_INF("%s", approve ? "approved by the button" : "rejected by the button");
	post(&(event_t){.type = EV_BUTTON, .len = (uint16_t)(approve ? 1 : 0)});
}

void pay_link_pin(const char *pin, size_t len)
{
	event_t ev = {.type = EV_PIN};
	if (pin && len != NU54_PIN_LEN) {
		LOG_WRN("PIN of %u digits ignored", (unsigned)len);
		return;
	}
	if (pin) {
		memcpy(ev.data, pin, len);
		ev.len = (uint16_t)len;
	}
	LOG_INF("%s", pin ? "PIN entered" : "PIN entry timed out");
	post(&ev);
	memset(&ev, 0, sizeof(ev));
}

int pay_link_pin_wanted(void)
{
	return device.pending == NU54_PENDING_PIN || device.pending == NU54_PENDING_LIMIT_PIN;
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
	device.cfg.dev_unpaired_anchor = IS_ENABLED(CONFIG_NU54_DEV_SETUP);
	device.cfg.platform.payment_mode = platform_payment_mode;
	k_work_queue_start(&work_q, work_stack, K_THREAD_STACK_SIZEOF(work_stack), K_PRIO_PREEMPT(7), NULL);
	err = ble_links_init(&handlers, device.passkey, device.state == NU54_STATE_UNPROVISIONED);
	if (err) {
		return err;
	}
	LOG_INF("payment link ready (state %d)", (int)device.state);
	return 0;
}
