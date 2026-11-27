/* What a board without an owner advertises and which centrals it keeps. The advertising and
 * connection code is extracted from ble_links.c at build time and runs against a fake controller
 * and a fake work queue. */
#include "nu54_adv_schedule.h"

#include <assert.h>
#include <errno.h>
#include <stdio.h>
#include <string.h>

#define CONFIG_NU54_OWNER_WAIT_ADV 1
#define CONFIG_NU54_OWNER_WAIT_IDLE_SECONDS 30
#define CONFIG_NU54_OWNER_WAIT_FAST_SECONDS 30
#define CONFIG_NU54_OWNER_WAIT_TOTAL_SECONDS 10800
#define CONFIG_NU54_OWNER_WAIT_FAST_INTERVAL_MIN 160
#define CONFIG_NU54_OWNER_WAIT_FAST_INTERVAL_MAX 240
#define CONFIG_NU54_OWNER_WAIT_SLOW_INTERVAL_MIN 1600
#define CONFIG_NU54_OWNER_WAIT_SLOW_INTERVAL_MAX 1920
#define IS_ENABLED(option) 1 /* only asked for the commercial TLS service */
#define LINK_MAX 2
#define ARRAY_SIZE(array) (sizeof(array) / sizeof((array)[0]))
#define LOG_INF(...) ((void)0)
#define LOG_WRN(...) ((void)0)
#define LOG_ERR(...) ((void)0)
#define BT_HCI_ERR_REMOTE_USER_TERM_CONN 0x13

/* ---- clock and work queue ---- */
struct k_work;
struct k_work_delayable {void (*handler)(struct k_work *); int64_t due; bool pending;};
static int64_t uptime = 1000;
static int64_t k_uptime_get(void) { return uptime; }
#define K_MSEC(ms) (ms)
#define K_SECONDS(s) ((int64_t)(s) * 1000)
#define K_NO_WAIT 0
static void k_work_reschedule(struct k_work_delayable *work, int64_t delay)
{
	work->pending = true;
	work->due = uptime + delay;
}
/* Leaves a work item that is already waiting at its time. */
static void k_work_schedule(struct k_work_delayable *work, int64_t delay)
{
	if (!work->pending) k_work_reschedule(work, delay);
}
static void k_work_cancel_delayable(struct k_work_delayable *work) { work->pending = false; }

static void adv_retry(struct k_work *w);
static void owner_wait_turn(struct k_work *w);
static void idle_guard(struct k_work *w);
static void mode_end(struct k_work *w);
static struct k_work_delayable adv_retry_work = {.handler = adv_retry}, owner_wait_timer = {.handler = owner_wait_turn},
			       idle_guard_work = {.handler = idle_guard}, mode_timer = {.handler = mode_end};
static struct k_work_delayable *const works[] = {&adv_retry_work, &owner_wait_timer, &idle_guard_work, &mode_timer};

static void advance(int64_t milliseconds)
{
	for (const int64_t end = uptime + milliseconds; uptime < end; ++uptime) {
		for (size_t i = 0; i < ARRAY_SIZE(works); ++i) {
			if (!works[i]->pending || works[i]->due > uptime) continue;
			works[i]->pending = false;
			works[i]->handler(NULL);
		}
	}
}

/* ---- locks: one thread here ---- */
typedef int k_spinlock_key_t;
static int links_lock, adv_lock;
#define K_FOREVER 0
#define k_spin_lock(lock) ((void)(lock), 0)
#define k_spin_unlock(lock, key) ((void)(lock), (void)(key))
#define k_mutex_lock(lock, timeout) ((void)(lock))
#define k_mutex_unlock(lock) ((void)(lock))

/* ---- controller: what is on air, and starts that fail ---- */
struct bt_conn {bool engaged, dropped;};
enum air {AIR_NONE, AIR_WINDOW, AIR_OWNER};
static enum air on_air;
static unsigned air_interval, start_failures, starts;
struct adv_param {unsigned min, max;};
static const struct adv_param window_param = {0x30, 0x60};
static const int ad_pilot[2], ad_pairing[3], sd_pairing[2], ad_payment[2];
#define BT_LE_ADV_OPT_CONN 0
#define BT_LE_ADV_CONN_FAST_1 (&window_param)
#define BT_LE_ADV_PARAM(options, minimum, maximum, peer) (&(const struct adv_param){.min = (minimum), .max = (maximum)})
static int adv_start(const struct adv_param *param, const int *ad)
{
	++starts;
	assert(ad == ad_pilot); /* a commercial board only ever shows the owner service */
	if (start_failures) {
		--start_failures;
		return -EAGAIN;
	}
	if (on_air != AIR_NONE) return -EALREADY;
	on_air = param == &window_param ? AIR_WINDOW : AIR_OWNER;
	air_interval = param->min;
	return 0;
}
#define bt_le_adv_start(param, ad, ad_count, sd, sd_count) ((void)(sd), adv_start(param, (const int *)(ad)))
static int bt_le_adv_stop(void)
{
	on_air = AIR_NONE;
	return 0;
}
#define bt_conn_ref(conn) (conn)
#define bt_conn_unref(conn) ((void)(conn))
static int bt_conn_disconnect(struct bt_conn *conn, int reason)
{
	(void)reason;
	conn->dropped = true;
	return 0;
}

/* ---- the rest of the board ---- */
typedef struct {
	struct bt_conn *conn;
	uint64_t epoch;
	uint8_t tx_sequence;
	int64_t idle_since;
} link_t;
static link_t links[LINK_MAX];
static uint64_t next_link_epoch;
static void link_event(int link, uint64_t epoch) { (void)link; (void)epoch; }
static const struct {void (*connected)(int, uint64_t);} link_handlers = {link_event}, *handlers = &link_handlers;
static struct bt_conn *link_conn(int i) { return links[i].conn; }
static bool payment_mode, pairing_mode, lamp_mode;
static nu54_adv_set advertising;
static unsigned adv_failures;
static nu54_adv_schedule owner_wait;
static bool owner_absent;
static bool device_pilot_active(void) { return true; }
static bool device_pilot_engaged(const struct bt_conn *conn) { return conn->engaged; }
static void status_led_mode(bool on) { lamp_mode = on; }

int ble_links_owner_wait(bool on);
int ble_links_payment_mode(uint32_t seconds);
#include "board_links.h"

/* ---- cases ---- */
static void restart(bool without_owner)
{
	memset(links, 0, sizeof(links));
	for (size_t i = 0; i < ARRAY_SIZE(works); ++i) works[i]->pending = false;
	payment_mode = pairing_mode = lamp_mode = owner_absent = false;
	advertising = NU54_ADV_SET_NONE;
	on_air = AIR_NONE;
	adv_failures = start_failures = starts = 0;
	memset(&owner_wait, 0, sizeof(owner_wait));
	if (without_owner) assert(!ble_links_owner_wait(true));
	advance(1);
}

/* The controller ends connectable advertising when a central connects. */
static void connect(struct bt_conn *conn)
{
	*conn = (struct bt_conn){0};
	on_air = AIR_NONE;
	connected(conn, 0);
}

/* As disconnected() in ble_links.c: free the slot, then advertise what is wanted now. */
static void leave(struct bt_conn *conn)
{
	for (int i = 0; i < LINK_MAX; ++i)
		if (links[i].conn == conn) links[i] = (link_t){0};
	(void)advertise();
}

static void test_owner_wait_by_itself(void)
{
	restart(true);
	assert(on_air == AIR_OWNER && air_interval == 160);
	advance(30000);
	assert(on_air == AIR_OWNER && air_interval == 1600);
	advance(10800000 - 30000);
	assert(on_air == AIR_NONE);

	/* The first owner registered: the board is silent again and keeps its centrals. */
	struct bt_conn phone;
	restart(true);
	connect(&phone);
	assert(on_air == AIR_OWNER); /* one central does not hide the board */
	assert(!ble_links_owner_wait(false));
	advance(1);
	assert(on_air == AIR_NONE);
	advance(120000);
	assert(!phone.dropped && on_air == AIR_NONE && !idle_guard_work.pending);

	restart(false); /* a board with an owner never advertises by itself */
	advance(60000);
	assert(on_air == AIR_NONE && !starts);
}

static void test_idle_centrals(void)
{
	struct bt_conn idle, busy;
	restart(true);
	connect(&idle);
	advance(29000);
	assert(!idle.dropped);
	advance(2000);
	assert(idle.dropped);
	leave(&idle);

	/* In an installation or a session it stays, and gets the whole time again afterwards. */
	connect(&busy);
	busy.engaged = true;
	advance(90000);
	assert(!busy.dropped);
	busy.engaged = false;
	advance(29000);
	assert(!busy.dropped);
	advance(2000);
	assert(busy.dropped);

	/* With an owner the same check leaves every central alone, however long it stays. */
	restart(false);
	connect(&idle);
	advance(600000);
	idle_guard(NULL);
	assert(!idle.dropped);
}

/* Centrals that connect more often than once a second must not put the check off. */
static void test_busy_air_does_not_stop_the_check(void)
{
	struct bt_conn idle, passing;
	const int64_t from = uptime;
	restart(true);
	connect(&idle);
	for (int i = 0; i < 80 && !idle.dropped; ++i) {
		connect(&passing);
		advance(250);
		leave(&passing);
		advance(250);
	}
	assert(idle.dropped && uptime - from < 32000);
}

/* SW4 right when the controller cannot start: the retry serves the window. */
static void test_window_survives_a_failed_start(void)
{
	restart(false);
	start_failures = 2;
	assert(!ble_links_payment_mode(120));
	assert(payment_mode && lamp_mode && on_air == AIR_NONE);
	advance(100);
	assert(on_air == AIR_NONE); /* the first retry failed as well */
	advance(250);
	assert(on_air == AIR_WINDOW);
	advance(120000);
	assert(!payment_mode && !lamp_mode && on_air == AIR_NONE);

	/* The same beside the owner wait: the window comes on top, and the wait is back afterwards. */
	restart(true);
	advance(40000);
	assert(on_air == AIR_OWNER && air_interval == 1600);
	start_failures = 1;
	assert(!ble_links_payment_mode(120));
	advance(150);
	assert(on_air == AIR_WINDOW);
	advance(120000);
	assert(on_air == AIR_OWNER && air_interval == 1600);

	/* Retries stop after a few tries and the next event starts them again. */
	struct bt_conn phone;
	restart(false);
	start_failures = 1000;
	assert(!ble_links_payment_mode(120));
	advance(10000);
	assert(on_air == AIR_NONE && starts == 1 + NU54_ADV_RETRY_LIMIT && !adv_retry_work.pending);
	start_failures = 0;
	connect(&phone);
	assert(on_air == AIR_WINDOW);
}

int main(void)
{
	test_owner_wait_by_itself();
	test_idle_centrals();
	test_busy_air_does_not_stop_the_check();
	test_window_survives_a_failed_start();
	puts("owner wait links ok");
	return 0;
}
