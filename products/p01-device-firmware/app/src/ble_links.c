#include "ble_links.h"

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

#include "nu54_frame.h"
#include "nu54_protocol.h"
#include "status_led.h"

LOG_MODULE_REGISTER(ble_links, LOG_LEVEL_INF);

#define FRAGMENT_MAX BLE_LINKS_FRAGMENT_MAX
#define LINK_MAX CONFIG_BT_MAX_CONN

/* One connected central. Changed on Bluetooth threads under the lock; read with link_conn(). */
typedef struct {
	struct bt_conn *conn;
	uint8_t tx_sequence; /* send side only (session work queue) */
} link_t;

static link_t links[LINK_MAX];
static struct k_spinlock links_lock;
static const ble_links_handlers_t *handlers;

static int link_of(const struct bt_conn *conn)
{
	k_spinlock_key_t key = k_spin_lock(&links_lock);
	int found = -1;
	for (int i = 0; i < LINK_MAX && found < 0; i++) {
		found = links[i].conn == conn ? i : -1;
	}
	k_spin_unlock(&links_lock, key);
	return found;
}

/* A reference to link i's connection, or NULL; the caller unrefs it. */
static struct bt_conn *link_conn(int i)
{
	if (i < 0 || i >= LINK_MAX) {
		return NULL;
	}
	k_spinlock_key_t key = k_spin_lock(&links_lock);
	struct bt_conn *c = links[i].conn ? bt_conn_ref(links[i].conn) : NULL;
	k_spin_unlock(&links_lock, key);
	return c;
}

/* ---------------------------------------------------------------- GATT */

/* One more expansion step, so the generated UUID parts become five macro arguments. */
#define UUID128(...) BT_UUID_128_ENCODE(__VA_ARGS__)

static const struct bt_uuid_128 svc_uuid = BT_UUID_INIT_128(UUID128(NU54_GATT_SERVICE_UUID_PARTS));
static const struct bt_uuid_128 rx_uuid = BT_UUID_INIT_128(UUID128(NU54_GATT_RX_UUID_PARTS));
static const struct bt_uuid_128 tx_uuid = BT_UUID_INIT_128(UUID128(NU54_GATT_TX_UUID_PARTS));

static ssize_t rx_write(struct bt_conn *conn, const struct bt_gatt_attr *attr, const void *buf, uint16_t len, uint16_t offset,
			uint8_t flags)
{
	int i = link_of(conn);
	(void)attr;
	(void)flags;
	if (i < 0 || offset != 0 || len > FRAGMENT_MAX) {
		return BT_GATT_ERR(BT_ATT_ERR_INVALID_ATTRIBUTE_LEN);
	}
	if (handlers->fragment(i, buf, len) != 0) {
		return BT_GATT_ERR(BT_ATT_ERR_INSUFFICIENT_RESOURCES);
	}
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

bool ble_links_listening(int i)
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

bool ble_links_bonded(int i, bool need_passkey)
{
	struct bt_conn *c = link_conn(i);
	struct bt_conn_info info;
	bool ok = false;

	if (c && bt_conn_get_info(c, &info) == 0) {
		ok = info.security.level >= (need_passkey ? BT_SECURITY_L4 : BT_SECURITY_L2) && (info.security.flags & BT_SECURITY_FLAG_SC);
	}
	if (c) {
		bt_conn_unref(c);
	}
	return ok;
}

/* ---------------------------------------------------------------- advertising and modes */

static bool payment_mode;
static bool pairing_mode;
static uint32_t pairing_passkey;

static void mode_end(struct k_work *w);
static K_WORK_DELAYABLE_DEFINE(mode_timer, mode_end);
static void pairing_end(struct k_work *w);
static K_WORK_DELAYABLE_DEFINE(pairing_timer, pairing_end);

/* Advertises while payment or pairing mode is on and a connection slot is free. */
static int advertise(void)
{
	bool free_slot = false;
	for (int i = 0; i < LINK_MAX; i++) {
		struct bt_conn *c = link_conn(i);
		free_slot |= c == NULL;
		if (c) {
			bt_conn_unref(c);
		}
	}
	if (!(payment_mode || pairing_mode) || !free_slot) {
		return 0;
	}
	int err = bt_le_adv_start(BT_LE_ADV_CONN_FAST_1, ad, ARRAY_SIZE(ad), sd, ARRAY_SIZE(sd));
	return err == -EALREADY ? 0 : err;
}

static void stop_advertising_if_idle(void)
{
	if (!payment_mode && !pairing_mode) {
		bt_le_adv_stop();
	}
}

static void mode_end(struct k_work *w)
{
	(void)w;
	payment_mode = false;
	stop_advertising_if_idle();
	status_led_mode(false);
	LOG_INF("payment mode ended");
}

static void pairing_end(struct k_work *w)
{
	(void)w;
	pairing_mode = false;
	stop_advertising_if_idle();
	status_led_pairing(false);
	LOG_INF("pairing mode ended");
}

int ble_links_payment_mode(uint32_t seconds)
{
	if (seconds == 0) {
		k_work_cancel_delayable(&mode_timer);
		mode_end(NULL);
		return 0;
	}
	payment_mode = true;
	int err = advertise();
	if (err) {
		payment_mode = false;
		LOG_ERR("advertising failed (%d)", err);
		return err;
	}
	status_led_mode(true);
	k_work_reschedule(&mode_timer, K_SECONDS(seconds));
	LOG_INF("payment mode for %u s", seconds);
	return 0;
}

/* ---------------------------------------------------------------- connections */

static void connected(struct bt_conn *conn, uint8_t err)
{
	int i = link_of(NULL);
	if (err || i < 0) {
		return;
	}
	k_spinlock_key_t key = k_spin_lock(&links_lock);
	links[i] = (link_t){.conn = bt_conn_ref(conn)};
	k_spin_unlock(&links_lock, key);
	LOG_INF("central connected (link %d)", i);
	handlers->connected(i);
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
	handlers->disconnected(i);
	advertise();
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
	return pairing_passkey;
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
	status_led_paired(bonded);
}

static void pairing_failed(struct bt_conn *conn, enum bt_security_err reason)
{
	LOG_WRN("pairing link %d failed (%d)", link_of(conn), (int)reason);
	status_led_paired(false);
}

static struct bt_conn_auth_info_cb auth_info = {
	.pairing_complete = pairing_complete,
	.pairing_failed = pairing_failed,
};

static void auth_select(bool just_works)
{
	bt_conn_auth_cb_register(NULL);
	bt_conn_auth_cb_register(just_works ? &auth_just_works : &auth_passkey);
}

int ble_links_pairing_mode(uint32_t seconds, uint32_t passkey, bool just_works)
{
	pairing_passkey = passkey;
	auth_select(just_works);
	pairing_mode = true;
	int err = advertise();
	if (err) {
		pairing_mode = false;
		LOG_ERR("advertising failed (%d)", err);
		return err;
	}
	status_led_pairing(true);
	k_work_reschedule(&pairing_timer, K_SECONDS(seconds));
	LOG_INF("pairing mode for %u s (%s)", seconds, just_works ? "Just Works" : "label passkey");
	return 0;
}

int ble_links_unpair_all(void)
{
	return bt_is_ready() ? bt_unpair(BT_ID_DEFAULT, BT_ADDR_LE_ANY) : 0;
}

/* ---------------------------------------------------------------- framing out */

int ble_links_send(int i, const uint8_t *body, size_t len)
{
	static uint8_t env[NU54_MAX_ENVELOPE_LEN];
	uint8_t digest[32], frag[FRAGMENT_MAX];
	size_t dlen;
	int rc = -ENOTCONN;
	struct bt_conn *c = link_conn(i);

	if (!c || !bt_gatt_is_subscribed(c, &pay_svc.attrs[4], BT_GATT_CCC_NOTIFY)) {
		LOG_WRN("reply dropped: link %d is not listening", i);
		goto out;
	}
	rc = -EINVAL;
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
		while ((rc = bt_gatt_notify(c, &pay_svc.attrs[4], frag, n + 2)) == -ENOMEM) {
			k_sleep(K_MSEC(5)); /* wait for a TX buffer */
		}
		if (rc) {
			LOG_WRN("notify failed (%d)", rc);
			goto out;
		}
	}
	links[i].tx_sequence++;
out:
	if (c) {
		bt_conn_unref(c);
	}
	return rc;
}

/* ---------------------------------------------------------------- init */

int ble_links_init(const ble_links_handlers_t *h, uint32_t passkey, bool just_works)
{
	int err;

	handlers = h;
	err = bt_enable(NULL);
	if (!err) {
		err = settings_load_subtree("bt"); /* bonds (CONFIG_BT_SETTINGS) */
	}
	if (!err) {
		err = bt_conn_auth_info_cb_register(&auth_info);
	}
	if (!err) {
		pairing_passkey = passkey;
		auth_select(just_works); /* pairing_accept refuses outside pairing mode */
	}
	return err;
}
