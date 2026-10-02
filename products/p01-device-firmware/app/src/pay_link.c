#include "pay_link.h"

#include <errno.h>
#include <string.h>

#include <psa/crypto.h>
#include <zephyr/bluetooth/bluetooth.h>
#include <zephyr/bluetooth/conn.h>
#include <zephyr/bluetooth/gatt.h>
#include <zephyr/bluetooth/uuid.h>
#include <zephyr/kernel.h>
#include <zephyr/logging/log.h>
#include <zephyr/settings/settings.h>

#include "board_io.h"
#include "dev_setup.h"
#include "device_key.h"
#include "nu54_cbor.h"
#include "nu54_frame.h"
#include "nu54_protocol.h"
#include "nu54_session.h"

LOG_MODULE_REGISTER(pay_link, LOG_LEVEL_INF);

#define FIRMWARE_VERSION "0.2.0"
#define FRAGMENT_MAX 247

/* ---------------------------------------------------------------- session and platform */

static nu54_device_t device;
static nu54_reassembler_t rx;
static uint8_t tx_sequence;

static int platform_sign(void *ctx, const uint8_t digest[32], uint8_t rs[64])
{
	(void)ctx;
	return device_key_sign(digest, rs);
}

static void platform_random(void *ctx, uint8_t *out, size_t n)
{
	(void)ctx;
	(void)psa_generate_random(out, n);
}

static int platform_persist_nonce(void *ctx, const uint8_t next[32])
{
	(void)ctx;
	return settings_save_one("nu54/nonce", next, 32);
}

static int nonce_loaded;

static int settings_set(const char *name, size_t len, settings_read_cb read_cb, void *cb_arg)
{
	if (strcmp(name, "nonce") == 0 && len == 32) {
		nonce_loaded = read_cb(cb_arg, device.next_nonce, 32) == 32;
	}
	return 0;
}

SETTINGS_STATIC_HANDLER_DEFINE(nu54, "nu54", NULL, settings_set, NULL, NULL);

/* ---------------------------------------------------------------- BLE */

/* One more expansion step, so the generated UUID parts become five macro arguments. */
#define UUID128(...) BT_UUID_128_ENCODE(__VA_ARGS__)

static const struct bt_uuid_128 svc_uuid = BT_UUID_INIT_128(UUID128(NU54_GATT_SERVICE_UUID_PARTS));
static const struct bt_uuid_128 rx_uuid = BT_UUID_INIT_128(UUID128(NU54_GATT_RX_UUID_PARTS));
static const struct bt_uuid_128 tx_uuid = BT_UUID_INIT_128(UUID128(NU54_GATT_TX_UUID_PARTS));

static struct bt_conn *central;
static bool notify_on;
static bool payment_mode;

typedef struct {
	uint16_t len;
	uint8_t data[FRAGMENT_MAX];
} fragment_t;

K_MSGQ_DEFINE(rx_queue, sizeof(fragment_t), 16, 4);

static K_THREAD_STACK_DEFINE(work_stack, 12288);
static struct k_work_q work_q;
static void rx_work_handler(struct k_work *w);
static K_WORK_DEFINE(rx_work, rx_work_handler);

static ssize_t rx_write(struct bt_conn *conn, const struct bt_gatt_attr *attr, const void *buf, uint16_t len,
			uint16_t offset, uint8_t flags)
{
	fragment_t f;
	(void)attr;
	(void)flags;
	if (conn != central || offset != 0 || len > FRAGMENT_MAX) {
		return BT_GATT_ERR(BT_ATT_ERR_INVALID_ATTRIBUTE_LEN);
	}
	f.len = len;
	memcpy(f.data, buf, len);
	if (k_msgq_put(&rx_queue, &f, K_NO_WAIT) != 0) {
		return BT_GATT_ERR(BT_ATT_ERR_INSUFFICIENT_RESOURCES);
	}
	k_work_submit_to_queue(&work_q, &rx_work);
	return len;
}

static void ccc_changed(const struct bt_gatt_attr *attr, uint16_t value)
{
	(void)attr;
	notify_on = value == BT_GATT_CCC_NOTIFY;
}

BT_GATT_SERVICE_DEFINE(pay_svc, BT_GATT_PRIMARY_SERVICE(&svc_uuid),
		       BT_GATT_CHARACTERISTIC(&rx_uuid.uuid, BT_GATT_CHRC_WRITE | BT_GATT_CHRC_WRITE_WITHOUT_RESP,
					      BT_GATT_PERM_WRITE, NULL, rx_write, NULL),
		       BT_GATT_CHARACTERISTIC(&tx_uuid.uuid, BT_GATT_CHRC_NOTIFY, BT_GATT_PERM_NONE, NULL, NULL, NULL),
		       BT_GATT_CCC(ccc_changed, BT_GATT_PERM_READ | BT_GATT_PERM_WRITE));

static const struct bt_data ad[] = {
	BT_DATA_BYTES(BT_DATA_FLAGS, (BT_LE_AD_GENERAL | BT_LE_AD_NO_BREDR)),
	BT_DATA_BYTES(BT_DATA_UUID128_ALL, UUID128(NU54_GATT_SERVICE_UUID_PARTS)),
};
static const struct bt_data sd[] = {
	BT_DATA(BT_DATA_NAME_COMPLETE, CONFIG_BT_DEVICE_NAME, sizeof(CONFIG_BT_DEVICE_NAME) - 1),
};

static void mode_end(struct k_work *w);
static K_WORK_DELAYABLE_DEFINE(mode_timer, mode_end);

static int advertise(void)
{
	int err = bt_le_adv_start(BT_LE_ADV_CONN_FAST_1, ad, ARRAY_SIZE(ad), sd, ARRAY_SIZE(sd));
	return err == -EALREADY ? 0 : err;
}

static void mode_end(struct k_work *w)
{
	(void)w;
	payment_mode = false;
	bt_le_adv_stop();
	if (!central) {
		board_led_set(PAY_LED_MODE, false);
	}
	LOG_INF("payment mode ended");
}

static void connected(struct bt_conn *conn, uint8_t err)
{
	if (err || central) {
		return;
	}
	central = bt_conn_ref(conn);
	nu54_reassembler_reset(&rx);
	tx_sequence = 0;
	LOG_INF("central connected");
}

static void disconnected(struct bt_conn *conn, uint8_t reason)
{
	if (conn != central) {
		return;
	}
	bt_conn_unref(central);
	central = NULL;
	notify_on = false;
	device.session_open = 0; /* the session ends with the link */
	device.pending = 0;
	board_led_set(PAY_LED_WAITING, false);
	LOG_INF("central disconnected (0x%02x)", reason);
	if (payment_mode) {
		advertise();
	} else {
		board_led_set(PAY_LED_MODE, false);
	}
}

BT_CONN_CB_DEFINE(conn_cb) = {
	.connected = connected,
	.disconnected = disconnected,
};

/* ---------------------------------------------------------------- framing out */

static void send_body(const uint8_t *body, size_t len)
{
	static uint8_t env[NU54_MAX_ENVELOPE_LEN];
	uint8_t digest[32], frag[FRAGMENT_MAX];
	size_t dlen;

	if (!central || !notify_on) {
		LOG_WRN("reply dropped: no central listening");
		return;
	}
	if (psa_hash_compute(PSA_ALG_SHA_256, body, len, digest, sizeof(digest), &dlen) != PSA_SUCCESS ||
	    nu54_envelope_header(env, len, digest) != 0) {
		return;
	}
	memcpy(&env[NU54_ENVELOPE_HEADER_LEN], body, len);
	size_t total = NU54_ENVELOPE_HEADER_LEN + len;
	size_t chunk = nu54_fragment_payload(bt_gatt_get_mtu(central));
	if (chunk > FRAGMENT_MAX - NU54_FRAGMENT_HEADER_LEN) {
		chunk = FRAGMENT_MAX - NU54_FRAGMENT_HEADER_LEN;
	}
	uint8_t index = 0;
	for (size_t off = 0; off < total; off += chunk, index++) {
		size_t n = total - off < chunk ? total - off : chunk;
		frag[0] = tx_sequence;
		frag[1] = index;
		memcpy(&frag[2], &env[off], n);
		int err;
		while ((err = bt_gatt_notify(central, &pay_svc.attrs[4], frag, n + 2)) == -ENOMEM) {
			k_sleep(K_MSEC(5)); /* wait for a TX buffer */
		}
		if (err) {
			LOG_WRN("notify failed (%d)", err);
			return;
		}
	}
	tx_sequence++;
}

static void deliver(const nu54_out_t *out)
{
	for (int i = 0; i < out->kiosk_count; i++) {
		send_body(out->kiosk[i], out->kiosk_len[i]);
	}
	if (out->phone_count) {
		/* Week-7 development build: no bonded phone app yet, so the fields are only logged
		 * (payment-protocol.md 3, N30). Release builds refuse with NOT_PERMITTED instead. */
		LOG_INF("confirm.show for the phone app (%u bytes); press SW1 to approve, SW2 to reject",
			(unsigned)out->phone_len);
	}
	board_led_set(PAY_LED_WAITING, device.pending);
}

/* Shows payment.outcome from the kiosk on the LEDs. */
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

/* ---------------------------------------------------------------- work */

static void rx_work_handler(struct k_work *w)
{
	static nu54_out_t out;
	fragment_t f;
	(void)w;
	while (k_msgq_get(&rx_queue, &f, K_NO_WAIT) == 0) {
		nu54_frame_result_t r = nu54_reassembler_feed(&rx, f.data, f.len);
		size_t blen;
		const uint8_t *body;
		uint8_t digest[32];
		size_t dlen;

		if (r == NU54_FRAME_NEED_MORE) {
			continue;
		}
		if (r == NU54_FRAME_BAD) {
			LOG_WRN("BAD_FRAME: fragment order or length (%u bytes, seq %u idx %u)", f.len, f.data[0], f.data[1]);
		}
		if (r == NU54_FRAME_DONE) {
			body = nu54_reassembler_body(&rx, &blen);
			if (psa_hash_compute(PSA_ALG_SHA_256, body, blen, digest, sizeof(digest), &dlen) != PSA_SUCCESS ||
			    memcmp(digest, nu54_reassembler_digest(&rx), 8) != 0) {
				LOG_WRN("BAD_FRAME: envelope digest mismatch (%u-byte body)", (unsigned)blen);
			} else {
				show_outcome(body, blen);
				nu54_session_handle(&device, body, blen, k_uptime_get() / 1000, &out);
				LOG_INF("message: %u-byte body, %d replies", (unsigned)blen, out.kiosk_count);
				deliver(&out);
				continue;
			}
		}
		/* Order, length or digest violation: an empty body is not a valid message, so the
		 * session answers error{BAD_FRAME} and closes, as for any malformed message. */
		nu54_reassembler_reset(&rx);
		nu54_session_handle(&device, NULL, 0, k_uptime_get() / 1000, &out);
		deliver(&out);
	}
}

static int button_choice;
static void button_work_handler(struct k_work *w)
{
	static nu54_out_t out;
	(void)w;
	memset(&out, 0, sizeof(out));
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

int pay_link_payment_mode(uint32_t seconds)
{
	int err = advertise();
	if (err) {
		LOG_ERR("advertising failed (%d)", err);
		return err;
	}
	payment_mode = true;
	board_led_set(PAY_LED_MODE, true);
	k_work_reschedule(&mode_timer, K_SECONDS(seconds));
	LOG_INF("payment mode for %u s", seconds);
	return 0;
}

/* ---------------------------------------------------------------- init */

int pay_link_init(const uint8_t address[20])
{
	int err;

	memcpy(device.address, address, 20);
	memcpy(device.operator_address, DEV_OPERATOR, 20);
	memcpy(device.contract, DEV_CONTRACT, 20);
	memcpy(device.chain_id, DEV_CHAIN_ID, 32);
	device.anchor_clock_skew = 60;
	device.authorization_expiry = 120;
	device.firmware = FIRMWARE_VERSION;
	device.platform = (nu54_platform_t){platform_sign, platform_random, platform_persist_nonce, NULL};

	uint8_t start[32] = {0};
	nu54_device_init(&device, start);
	err = settings_subsys_init();
	if (!err) {
		err = settings_load_subtree("nu54");
	}
	if (err) {
		return err;
	}
	if (!nonce_loaded) {
		/* First boot: a random 256-aligned start (payment-protocol.md 2), stored at once. */
		platform_random(NULL, device.next_nonce, 31);
		device.next_nonce[31] = 0;
		device.next_nonce[0] = 0; /* keep far below 2^256 so the counter never wraps */
		err = platform_persist_nonce(NULL, device.next_nonce);
		if (err) {
			return err;
		}
	}
	nu54_reassembler_reset(&rx);
	k_work_queue_start(&work_q, work_stack, K_THREAD_STACK_SIZEOF(work_stack), K_PRIO_PREEMPT(7), NULL);
	err = bt_enable(NULL);
	if (err) {
		return err;
	}
	LOG_INF("payment link ready: chain %d, %s nonce counter", DEV_CHAIN_ID_NUMBER, nonce_loaded ? "stored" : "new");
	return 0;
}
