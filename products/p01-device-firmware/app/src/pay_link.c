#include "pay_link.h"

#include <errno.h>
#include <string.h>

#include <app_version.h>
#include <psa/crypto.h>
#include <zephyr/bluetooth/bluetooth.h>
#include <zephyr/bluetooth/conn.h>
#include <zephyr/bluetooth/gatt.h>
#include <zephyr/bluetooth/uuid.h>
#include <zephyr/kernel.h>
#include <zephyr/logging/log.h>
#include <zephyr/settings/settings.h>

#include "board_io.h"
#include "device_setup.h"
#include "nu54_cbor.h"
#include "nu54_frame.h"
#include "nu54_pin_entry.h"
#include "nu54_protocol.h"
#include "nu54_session.h"

LOG_MODULE_REGISTER(pay_link, LOG_LEVEL_INF);

#define FRAGMENT_MAX 247
#define LINK_MAX CONFIG_BT_MAX_CONN

/* ---------------------------------------------------------------- session */

static nu54_device_t device;

/*
 * One connected central (payment-protocol.md 3). The kiosk writes without pairing; the phone app
 * and the operator tool are bonded. A bonded central that subscribed and never wrote is taken as
 * the renter's phone app and gets confirm.show, confirm.limit and the forwarded payment.outcome;
 * a central that writes is a session peer.
 */
typedef struct {
	struct bt_conn *conn;
	nu54_reassembler_t rx;
	uint8_t tx_sequence;
	bool wrote;
} link_t;

static link_t links[LINK_MAX];
static struct k_spinlock links_lock;
static int session_link = -1; /* the link whose messages the session answers (work queue only) */

static int link_of(const struct bt_conn *conn)
{
	for (int i = 0; i < LINK_MAX; i++) {
		if (links[i].conn == conn) {
			return i;
		}
	}
	return -1;
}

/* A reference to link i's connection, or NULL; the caller unrefs it. */
static struct bt_conn *link_conn(int i)
{
	k_spinlock_key_t key = k_spin_lock(&links_lock);
	struct bt_conn *c = i >= 0 && links[i].conn ? bt_conn_ref(links[i].conn) : NULL;
	k_spin_unlock(&links_lock, key);
	return c;
}

/* Bonded with LE Secure Connections: passkey-authenticated once the device has its label passkey;
 * the first setup of an UNPROVISIONED device bonds with Just Works (payment-protocol.md 3). */
static bool link_bonded(int i)
{
	struct bt_conn *c = link_conn(i);
	struct bt_conn_info info;
	bool ok = false;

	if (c && bt_conn_get_info(c, &info) == 0) {
		bt_security_t need = device.state == NU54_STATE_UNPROVISIONED ? BT_SECURITY_L2 : BT_SECURITY_L4;
		ok = info.security.level >= need && (info.security.flags & BT_SECURITY_FLAG_SC);
	}
	if (c) {
		bt_conn_unref(c);
	}
	return ok;
}

static bool listening(int i);

static bool is_phone(int i, int except)
{
	return i != except && links[i].conn && !links[i].wrote && listening(i) && link_bonded(i);
}

static bool phone_present(int except)
{
	for (int i = 0; i < LINK_MAX; i++) {
		if (is_phone(i, except)) {
			return true;
		}
	}
	return false;
}

/* ---------------------------------------------------------------- BLE */

/* One more expansion step, so the generated UUID parts become five macro arguments. */
#define UUID128(...) BT_UUID_128_ENCODE(__VA_ARGS__)

static const struct bt_uuid_128 svc_uuid = BT_UUID_INIT_128(UUID128(NU54_GATT_SERVICE_UUID_PARTS));
static const struct bt_uuid_128 rx_uuid = BT_UUID_INIT_128(UUID128(NU54_GATT_RX_UUID_PARTS));
static const struct bt_uuid_128 tx_uuid = BT_UUID_INIT_128(UUID128(NU54_GATT_TX_UUID_PARTS));

static bool payment_mode;
static bool pairing_mode;

typedef struct {
	uint8_t link;
	uint16_t len;
	uint8_t data[FRAGMENT_MAX];
} fragment_t;

K_MSGQ_DEFINE(rx_queue, sizeof(fragment_t), 16, 4);

static K_THREAD_STACK_DEFINE(work_stack, 12288);
static struct k_work_q work_q;
static void rx_work_handler(struct k_work *w);
static K_WORK_DEFINE(rx_work, rx_work_handler);
static void close_work_handler(struct k_work *w);
static K_WORK_DEFINE(close_work, close_work_handler);

static ssize_t rx_write(struct bt_conn *conn, const struct bt_gatt_attr *attr, const void *buf, uint16_t len, uint16_t offset,
			uint8_t flags)
{
	fragment_t f;
	int i = link_of(conn);
	(void)attr;
	(void)flags;
	if (i < 0 || offset != 0 || len > FRAGMENT_MAX) {
		return BT_GATT_ERR(BT_ATT_ERR_INVALID_ATTRIBUTE_LEN);
	}
	links[i].wrote = true;
	f.link = (uint8_t)i;
	f.len = len;
	memcpy(f.data, buf, len);
	if (k_msgq_put(&rx_queue, &f, K_NO_WAIT) != 0) {
		return BT_GATT_ERR(BT_ATT_ERR_INSUFFICIENT_RESOURCES);
	}
	k_work_submit_to_queue(&work_q, &rx_work);
	return len;
}

/* The CCC callback does not say which central; listening() asks per connection when it matters. */
static void ccc_changed(const struct bt_gatt_attr *attr, uint16_t value)
{
	(void)attr;
	LOG_DBG("notifications %s", value == BT_GATT_CCC_NOTIFY ? "on" : "off");
}

BT_GATT_SERVICE_DEFINE(pay_svc, BT_GATT_PRIMARY_SERVICE(&svc_uuid),
		       BT_GATT_CHARACTERISTIC(&rx_uuid.uuid, BT_GATT_CHRC_WRITE | BT_GATT_CHRC_WRITE_WITHOUT_RESP, BT_GATT_PERM_WRITE,
					      NULL, rx_write, NULL),
		       BT_GATT_CHARACTERISTIC(&tx_uuid.uuid, BT_GATT_CHRC_NOTIFY, BT_GATT_PERM_NONE, NULL, NULL, NULL),
		       BT_GATT_CCC(ccc_changed, BT_GATT_PERM_READ | BT_GATT_PERM_WRITE));

/* Link i subscribed to TX notifications. */
static bool listening(int i)
{
	struct bt_conn *c = link_conn(i);
	bool on = c && bt_gatt_is_subscribed(c, &pay_svc.attrs[4], BT_GATT_CCC_NOTIFY);
	if (c) {
		bt_conn_unref(c);
	}
	return on;
}

static const struct bt_data ad[] = {
	BT_DATA_BYTES(BT_DATA_FLAGS, (BT_LE_AD_GENERAL | BT_LE_AD_NO_BREDR)),
	BT_DATA_BYTES(BT_DATA_UUID128_ALL, UUID128(NU54_GATT_SERVICE_UUID_PARTS)),
};
static const struct bt_data sd[] = {
	BT_DATA(BT_DATA_NAME_COMPLETE, CONFIG_BT_DEVICE_NAME, sizeof(CONFIG_BT_DEVICE_NAME) - 1),
};

static void mode_end(struct k_work *w);
static K_WORK_DELAYABLE_DEFINE(mode_timer, mode_end);
static void pairing_end(struct k_work *w);
static K_WORK_DELAYABLE_DEFINE(pairing_timer, pairing_end);

static bool link_free(void)
{
	return link_of(NULL) >= 0;
}

/* Advertises while payment or pairing mode is on and a connection slot is free. */
static int advertise(void)
{
	if (!(payment_mode || pairing_mode) || !link_free()) {
		return 0;
	}
	int err = bt_le_adv_start(BT_LE_ADV_CONN_FAST_1, ad, ARRAY_SIZE(ad), sd, ARRAY_SIZE(sd));
	return err == -EALREADY ? 0 : err;
}

static bool any_link(void)
{
	for (int i = 0; i < LINK_MAX; i++) {
		if (links[i].conn) {
			return true;
		}
	}
	return false;
}

static void mode_end(struct k_work *w)
{
	(void)w;
	payment_mode = false;
	if (!pairing_mode) {
		bt_le_adv_stop();
	}
	if (!any_link()) {
		board_led_set(PAY_LED_MODE, false);
	}
	LOG_INF("payment mode ended");
}

static void pairing_end(struct k_work *w)
{
	(void)w;
	pairing_mode = false;
	if (!payment_mode) {
		bt_le_adv_stop();
	}
	LOG_INF("pairing mode ended");
}

static void connected(struct bt_conn *conn, uint8_t err)
{
	int i = link_of(NULL);
	if (err || i < 0) {
		return;
	}
	k_spinlock_key_t key = k_spin_lock(&links_lock);
	links[i] = (link_t){.conn = bt_conn_ref(conn)};
	nu54_reassembler_reset(&links[i].rx);
	k_spin_unlock(&links_lock, key);
	LOG_INF("central connected (link %d)", i);
	advertise(); /* room for the other central */
}

static void disconnected(struct bt_conn *conn, uint8_t reason)
{
	int i = link_of(conn);
	if (i < 0) {
		return;
	}
	k_spinlock_key_t key = k_spin_lock(&links_lock);
	struct bt_conn *c = links[i].conn;
	links[i] = (link_t){0};
	k_spin_unlock(&links_lock, key);
	bt_conn_unref(c);
	LOG_INF("central disconnected (link %d, 0x%02x)", i, reason);
	k_work_submit_to_queue(&work_q, &close_work); /* the session may end with the link */
	advertise();
	if (!payment_mode && !any_link()) {
		board_led_set(PAY_LED_MODE, false);
	}
}

static void security_changed(struct bt_conn *conn, bt_security_t level, enum bt_security_err err)
{
	LOG_INF("link %d security level %d (%d)", link_of(conn), (int)level, (int)err);
}

BT_CONN_CB_DEFINE(conn_cb) = {
	.connected = connected,
	.disconnected = disconnected,
	.security_changed = security_changed,
};

/* ---------------------------------------------------------------- pairing (payment-protocol.md 3) */

/* New bonds only in pairing mode; the renter opens it on the device. */
static enum bt_security_err pairing_accept(struct bt_conn *conn, const struct bt_conn_pairing_feat *const feat)
{
	(void)conn;
	(void)feat;
	return pairing_mode ? BT_SECURITY_ERR_SUCCESS : BT_SECURITY_ERR_PAIR_NOT_ALLOWED;
}

/* The label passkey recorded at setup; the device has no screen, so it "displays" it on the label. */
static uint32_t app_passkey(struct bt_conn *conn)
{
	(void)conn;
	return device.passkey;
}

static void passkey_display(struct bt_conn *conn, unsigned int passkey)
{
	(void)passkey; /* never logged: it is the label secret */
	LOG_INF("pairing link %d: the central enters the label passkey", link_of(conn));
}

static void auth_cancel(struct bt_conn *conn)
{
	LOG_INF("pairing link %d cancelled", link_of(conn));
}

static struct bt_conn_auth_cb auth_passkey = {
	.pairing_accept = pairing_accept,
	.passkey_display = passkey_display,
	.app_passkey = app_passkey,
	.cancel = auth_cancel,
};

/* UNPROVISIONED: no passkey yet; the first setup bonds with Just Works and the button confirms. */
static struct bt_conn_auth_cb auth_just_works = {
	.pairing_accept = pairing_accept,
	.cancel = auth_cancel,
};

static void pairing_complete(struct bt_conn *conn, bool bonded)
{
	LOG_INF("pairing link %d complete (%s)", link_of(conn), bonded ? "bonded" : "not bonded");
}

static void pairing_failed(struct bt_conn *conn, enum bt_security_err reason)
{
	LOG_WRN("pairing link %d failed (%d)", link_of(conn), (int)reason);
}

static struct bt_conn_auth_info_cb auth_info = {
	.pairing_complete = pairing_complete,
	.pairing_failed = pairing_failed,
};

/* Passkey Entry once the device has its passkey, Just Works before. */
static void auth_select(void)
{
	bt_conn_auth_cb_register(NULL);
	bt_conn_auth_cb_register(device.state == NU54_STATE_UNPROVISIONED ? &auth_just_works : &auth_passkey);
}

/* ---------------------------------------------------------------- framing out */

static void send_body(int i, const uint8_t *body, size_t len)
{
	static uint8_t env[NU54_MAX_ENVELOPE_LEN];
	uint8_t digest[32], frag[FRAGMENT_MAX];
	size_t dlen;
	struct bt_conn *c = link_conn(i);

	if (!c || !bt_gatt_is_subscribed(c, &pay_svc.attrs[4], BT_GATT_CCC_NOTIFY)) {
		LOG_WRN("reply dropped: link %d is not listening", i);
		goto out;
	}
	if (psa_hash_compute(PSA_ALG_SHA_256, body, len, digest, sizeof(digest), &dlen) != PSA_SUCCESS ||
	    nu54_envelope_header(env, len, digest) != 0) {
		goto out;
	}
	memcpy(&env[NU54_ENVELOPE_HEADER_LEN], body, len);
	size_t total = NU54_ENVELOPE_HEADER_LEN + len;
	size_t chunk = nu54_fragment_payload(bt_gatt_get_mtu(c));
	if (chunk > FRAGMENT_MAX - NU54_FRAGMENT_HEADER_LEN) {
		chunk = FRAGMENT_MAX - NU54_FRAGMENT_HEADER_LEN;
	}
	uint8_t index = 0;
	for (size_t off = 0; off < total; off += chunk, index++) {
		size_t n = total - off < chunk ? total - off : chunk;
		frag[0] = links[i].tx_sequence;
		frag[1] = index;
		memcpy(&frag[2], &env[off], n);
		int err;
		while ((err = bt_gatt_notify(c, &pay_svc.attrs[4], frag, n + 2)) == -ENOMEM) {
			k_sleep(K_MSEC(5)); /* wait for a TX buffer */
		}
		if (err) {
			LOG_WRN("notify failed (%d)", err);
			goto out;
		}
	}
	links[i].tx_sequence++;
out:
	if (c) {
		bt_conn_unref(c);
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
	board_led_set(approved ? PAY_LED_APPROVED : PAY_LED_REFUSED, true);
	LOG_INF("payment outcome: %.*s", (int)m.items[o].len, m.items[o].ptr);
}

/* Replies to the session's central; the phone body to every phone app link. */
static void deliver(const nu54_out_t *out)
{
	for (int i = 0; i < out->kiosk_count; i++) {
		send_body(session_link, out->kiosk[i], out->kiosk_len[i]);
	}
	if (out->phone_count) {
		int sent = 0;
		show_outcome(out->phone, out->phone_len);
		for (int i = 0; i < LINK_MAX; i++) {
			if (is_phone(i, session_link)) {
				send_body(i, out->phone, out->phone_len);
				sent++;
			}
		}
		if (!sent) {
			/* Development builds go on without a phone app (N30); release builds refused earlier. */
			LOG_INF("no phone app listening; SW1 approves, SW2 rejects");
		}
	}
	board_led_set(PAY_LED_WAITING, device.pending);
}

/* The links as the session sees them for a message from link i. */
static void set_links(int i)
{
	device.link_bonded = link_bonded(i);
	device.phone_present = phone_present(i);
}

/* ---------------------------------------------------------------- work */

static void rx_work_handler(struct k_work *w)
{
	static nu54_out_t out;
	fragment_t f;
	(void)w;
	while (k_msgq_get(&rx_queue, &f, K_NO_WAIT) == 0) {
		link_t *l = &links[f.link];
		nu54_frame_result_t r = nu54_reassembler_feed(&l->rx, f.data, f.len);
		size_t blen;
		const uint8_t *body;
		uint8_t digest[32];
		size_t dlen;

		if (r == NU54_FRAME_NEED_MORE) {
			continue;
		}
		session_link = f.link;
		set_links(f.link);
		if (r == NU54_FRAME_BAD) {
			LOG_WRN("BAD_FRAME: fragment order or length (%u bytes, seq %u idx %u)", f.len, f.data[0], f.data[1]);
		}
		if (r == NU54_FRAME_DONE) {
			body = nu54_reassembler_body(&l->rx, &blen);
			if (psa_hash_compute(PSA_ALG_SHA_256, body, blen, digest, sizeof(digest), &dlen) != PSA_SUCCESS ||
			    memcmp(digest, nu54_reassembler_digest(&l->rx), 8) != 0) {
				LOG_WRN("BAD_FRAME: envelope digest mismatch (%u-byte body)", (unsigned)blen);
			} else {
				nu54_session_handle(&device, body, blen, k_uptime_get() / 1000, &out);
				LOG_INF("message on link %d: %u-byte body, %d replies", f.link, (unsigned)blen, out.kiosk_count);
				deliver(&out);
				continue;
			}
		}
		/* Order, length or digest violation: an empty body is not a valid message, so the
		 * session answers error{BAD_FRAME} and closes, as for any malformed message. */
		nu54_reassembler_reset(&l->rx);
		nu54_session_handle(&device, NULL, 0, k_uptime_get() / 1000, &out);
		deliver(&out);
	}
}

/* A link went away: if it carried the session, the session ends with it. */
static void close_work_handler(struct k_work *w)
{
	(void)w;
	if (session_link >= 0 && !links[session_link].conn) {
		nu54_session_link_closed(&device);
		session_link = -1;
		board_led_set(PAY_LED_WAITING, false);
	}
}

static int button_choice;
static void button_work_handler(struct k_work *w)
{
	static nu54_out_t out;
	(void)w;
	memset(&out, 0, sizeof(out));
	set_links(session_link);
	nu54_session_button(&device, button_choice, &out);
	deliver(&out);
}
static K_WORK_DEFINE(button_work, button_work_handler);

void pay_link_button(int approve)
{
	if (!device.pending) {
		LOG_INF("no payment is waiting for the button");
		return;
	}
	LOG_INF("%s", approve ? "approved by the button" : "rejected by the button");
	button_choice = approve;
	k_work_submit_to_queue(&work_q, &button_work);
}

/* The PIN waits in RAM only until the work queue hands it to the session. */
static char pin_entry[NU54_PIN_LEN];
static size_t pin_entry_len;
static bool pin_given;

static void pin_work_handler(struct k_work *w)
{
	static nu54_out_t out;
	(void)w;
	memset(&out, 0, sizeof(out));
	set_links(session_link);
	nu54_session_pin(&device, pin_given ? pin_entry : NULL, pin_entry_len, &out);
	memset(pin_entry, 0, sizeof(pin_entry));
	pin_entry_len = 0;
	deliver(&out);
}
static K_WORK_DEFINE(pin_work, pin_work_handler);

void pay_link_pin(const char *pin, size_t len)
{
	if (device.pending != NU54_PENDING_PIN && device.pending != NU54_PENDING_LIMIT_PIN) {
		LOG_INF("no PIN is asked for");
		return;
	}
	if (pin && len != NU54_PIN_LEN) {
		LOG_WRN("PIN of %u digits ignored", (unsigned)len);
		return;
	}
	pin_given = pin != NULL;
	pin_entry_len = pin ? len : 0;
	if (pin) {
		memcpy(pin_entry, pin, len);
	}
	LOG_INF("%s", pin ? "PIN entered" : "PIN entry timed out");
	k_work_submit_to_queue(&work_q, &pin_work);
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
	payment_mode = true;
	int err = advertise();
	if (err) {
		payment_mode = false;
		LOG_ERR("advertising failed (%d)", err);
		return err;
	}
	board_led_set(PAY_LED_MODE, true);
	k_work_reschedule(&mode_timer, K_SECONDS(seconds));
	LOG_INF("payment mode for %u s", seconds);
	return 0;
}

int pay_link_pairing_mode(uint32_t seconds)
{
	auth_select();
	pairing_mode = true;
	int err = advertise();
	if (err) {
		pairing_mode = false;
		LOG_ERR("advertising failed (%d)", err);
		return err;
	}
	k_work_reschedule(&pairing_timer, K_SECONDS(seconds));
	LOG_INF("pairing mode for %u s (%s)", seconds, device.state == NU54_STATE_UNPROVISIONED ? "Just Works" : "label passkey");
	return 0;
}

/* ---------------------------------------------------------------- init */

int pay_link_init(void)
{
	int err;

	device.anchor_clock_skew = 60;
	device.authorization_expiry = 120;
	device.firmware = APP_VERSION_STRING; /* app/VERSION, also the signed image version */
	device.require_phone = IS_ENABLED(CONFIG_NU54_REQUIRE_PHONE);
	err = device_setup_load(&device);
	if (err) {
		return err;
	}
	k_work_queue_start(&work_q, work_stack, K_THREAD_STACK_SIZEOF(work_stack), K_PRIO_PREEMPT(7), NULL);
	err = bt_enable(NULL);
	if (!err) {
		err = settings_load_subtree("bt"); /* bonds (CONFIG_BT_SETTINGS) */
	}
	if (!err) {
		err = bt_conn_auth_info_cb_register(&auth_info);
	}
	if (err) {
		return err;
	}
	auth_select();
	LOG_INF("payment link ready (state %d)", (int)device.state);
	return 0;
}
