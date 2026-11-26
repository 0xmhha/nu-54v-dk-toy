/* What the lamp shows for each button answer of the spending profile. The button code is extracted
 * from device_pilot.c at build time and runs against the real status_led.c. */
#include "board_io.h"
#include "device_order.h"
#include "device_recovery_pin.h"
#include "device_spending.h"
#include "nu54_install.h"
#include "nu54_keccak.h"
#include "nu54_owner.h"
#include "nu54_sig.h"
#include "status_led.h"
#include "../core/include/nu54_owner_idle.h"

#include <assert.h>
#include <stdio.h>
#include <string.h>
#include <zephyr/kernel.h>

#define NOW UINT64_C(1800000000)

/* ---- time and lamp; `trace` is the on and off times since the last start() ---- */
static struct k_work_delayable *pending_work;
static int64_t uptime = 1, next_tick;
static bool lamp;
static unsigned trace[256], steps;

int64_t k_uptime_get(void) { return uptime; }
void k_work_reschedule(struct k_work_delayable *work, int delay)
{
	pending_work = work;
	next_tick = uptime + delay;
}
void board_led_set(uint8_t index, bool on) { lamp = on; (void)index; }

static void advance(int milliseconds)
{
	for (int i = 0; i < milliseconds; ++i) {
		while (pending_work && next_tick <= uptime) {
			struct k_work_delayable *task = pending_work;
			pending_work = NULL;
			task->handler(NULL);
		}
		if (lamp != !!(steps % 2)) ++steps; /* odd: lit */
		assert(steps < 256);
		++trace[steps];
		++uptime;
	}
}

static void start(void)
{
	status_led_clear();
	advance(10);
	memset(trace, 0, sizeof(trace));
	steps = 0;
}

/* The lit times from `from` on, and that the lamp then stays dark. The light of the pattern before
 * may run into the first flash. */
static bool flashes(unsigned from, const unsigned *lit, unsigned count)
{
	for (unsigned i = 0; i < count; ++i)
		if (from + 2 * i > steps || (i ? trace[from + 2 * i] != lit[i] : trace[from] < lit[0])) return false;
	return steps == from + 2 * count - 1 && !lamp;
}
static const unsigned FAILED[] = {100, 100, 100}, LOCKED[] = {60, 60, 60, 60, 60}, DONE[] = {1500, 1500};

/* ---- the rest of device_pilot.c ---- */
typedef int32_t atomic_val_t;
typedef int32_t atomic_t;
struct bt_conn {int unused;};
#define NU54_PILOT_PROMPT_MASK UINT32_C(0x80000000)
#define PROMPT (NU54_PILOT_PROMPT_MASK | 1)
#define K_FOREVER 0
#define k_mutex_lock(lock, timeout) ((void)(lock))
#define k_mutex_unlock(lock) ((void)(lock))
static int pilot_lock;
static atomic_t pending_prompt;
static int32_t atomic_get(const atomic_t *value) { return *value; }
static void atomic_set(atomic_t *value, int32_t next) { *value = next; }
static nu54_owner_enrollment enrollment;
static nu54_owner_idle owner_idle;
static nu54_local_approval approval;
static nu54_spend_action spend_action;
static nu54_v2_link link;
static nu54_install_policy policy;
static struct bt_conn phone, *connection = &phone;
static const nu54_owner_crypto owner_crypto;
static bool recovering, display_confirmed = true;
static int64_t recovery_window;
static unsigned order_slot = 1;
static uint64_t order_amount = 1, order_expires = NOW + 100;
static const uint8_t wallet[20] = {1};

static nu54_vault vault;
static int32_t pin_wait;
static bool order_open, right_pin, erased, send_fails, sign_fails, confirm_fails;
static int64_t pin_at;
static unsigned stored;
static board_button_event_t early[2];
static unsigned early_count;

static nu54_vault *device_vault_owner(void) { return &vault; }
static int pay_link_waiting(void) { return 0; }
uint64_t fake_earliest(void) { return NOW; }
#define device_install_earliest fake_earliest
static const nu54_install_policy *device_install_policy(void) { return &policy; }
#define device_recovery_pin_guarded(virgin, guard, context) ((void)(context), assert(false), -1)
#define device_order_open(earliest, now) (order_open)
#define device_order_charge(earliest, amount) ((amount) ? 0 : -1)
#define device_order_store(slot, record) (stored = (record)->outcome, 0)
#define device_spending_address() (wallet)
#define device_spending_return_address(out) (memset(out, 2, 20) != NULL)
#define device_spending_set_return(address) (0)
#define device_spending_sign(digest, raw) (memset(raw, 1, 64) == NULL)
#define nu54_sig_finish(digest, raw, address, out) (memset(out, 1, 65), sign_fails ? NU54_SIG_BAD_V : NU54_SIG_OK)
#define nu54_owner_confirm(e, v, l, now, prompt, ok) ((void)(v), (e)->proven = false, confirm_fails || !(ok) ? -1 : 0)
#define nu54_owner_status(v, c, e, out) ((void)(c), memset(out, 0, NU54_OWNER_STATUS_SIZE) == NULL)
/* A function, not an expression: the board code calls it as a statement and ignores the result. */
static int link_send_stub(const void *reply) { (void)reply; return send_fails ? -1 : 0; }
#define nu54_v2_link_send(l, reply, write, context) link_send_stub(reply)
#define send_receipt(reply, signature, length) ((void)(reply), (void)(signature), send_fails ? -1 : 0)
#define payment_generation_current(p) ((p) != NULL)
#define payment_address(p, out) (memset(out, 1, 20) != NULL)
#define nu54_local_approval_cancel(a) ((a)->pending = false)
#define nu54_pilot_complete(a, l, g, now, prompt, button, payer, sign, context, out) \
	((a)->pending = false, memset(out, 1, 65), sign_fails ? -1 : 0)
#define trusted_now() (NOW)
#define clear_legacy_bonds() ((void)0)
#define report_legacy_bonds(status) ((void)(status))
#define board_io_set_prompt(prompt) ((void)(prompt))
static int hash(const uint8_t *data, size_t length, uint8_t out[32])
{
	nu54_keccak256(data, length, out);
	return 0;
}
static bool session_live(int64_t now) { return now > 0; }
static void reset_link(void)
{
	if (pending_prompt) status_led_waiting(false); /* as reset_link does */
	pending_prompt = 0;
	memset(&spend_action, 0, sizeof(spend_action));
	memset(&approval, 0, sizeof(approval));
}
static void spending_erase_and_reboot(void)
{
	erased = true;
	status_led_clear();
	status_led_reset_done();
	advance(4000);
}

int board_button_wait(board_button_event_t *out, int32_t timeout)
{
	if (early_count) {
		*out = early[--early_count];
		advance(1);
		return 0;
	}
	if (timeout == 100) { /* the armed reset waits for SW1 */
		*out = (board_button_event_t){.button = 0, .event = NU54_BTN_LONG};
		advance(5);
		return 0;
	}
	advance(timeout);
	return -1;
}

int device_recovery_pin(bool virgin)
{
	(void)virgin;
	if (pin_wait) {
		status_led_locked(); /* as device_recovery_pin_guarded does */
		return -1;
	}
	assert(!early_count); /* an early press is no digit */
	pin_at = uptime;
	status_led_pin(STATUS_PIN_START);
	advance(900);
	status_led_pin(STATUS_PIN_TAP);
	advance(200);
	status_led_pin_end();
	return right_pin ? 0 : -1;
}

void device_pilot_button_reset(void);
void device_pilot_button(uint32_t prompt, bool approved);
#include "board_buttons.h"

/* ---- cases ---- */
enum ask {ORDER, RETURN, SET_RETURN, REGISTER};

static void ask(enum ask what)
{
	start();
	vault = (nu54_vault){.enabled = true, .record.generation = 1};
	policy = (nu54_install_policy){.allowlist.commercial_testnet = true, .not_after = NOW + 100};
	link = (nu54_v2_link){.profile = NU54_V2_MANAGEMENT, .local = NU54_V2_BOARD, .peer = NU54_V2_OWNER, .pending_expiry = NOW + 60};
	memset(&approval, 0, sizeof(approval));
	memset(&spend_action, 0, sizeof(spend_action));
	memset(&enrollment, 0, sizeof(enrollment));
	uint8_t body[49] = {what == RETURN ? NU54_SPEND_RETURN_OP : NU54_SPEND_SET_RETURN_OP};
	if (what == RETURN) {
		body[8] = 5;
		for (unsigned i = 0; i < 8; ++i) body[9 + i] = (uint8_t)((NOW + 50) >> (56 - 8 * i));
		body[17] = 1;
	} else body[1] = 3;
	nu54_v2_request request = {.command = NU54_V2_MANAGE, .expires = NOW + 60, .payload = body,
				   .payload_length = what == RETURN ? 49 : 21};
	if (what == ORDER) approval = (nu54_local_approval){.pending = true, .intent.expires = NOW + 60};
	else if (what == REGISTER) enrollment.proven = true;
	else assert(!nu54_spend_stage(&spend_action, &link, &request, NOW, NOW, 1, PROMPT));
	pending_prompt = (atomic_t)PROMPT;
	status_led_waiting(true);
	advance(50); /* lit by the waiting pattern */
	stored = 0;
	pin_at = 0;
}

/* SW1 with the PIN: the lamp shows the press for 0.6 s, and only then the PIN starts. */
static void approve(void)
{
	const int64_t pressed = uptime;
	const unsigned from = steps;
	device_pilot_button(PROMPT, true);
	if (pin_wait) return;
	assert(pin_at - pressed == STATUS_LED_PRESSED_MS);
	assert(trace[from] >= 50 + STATUS_LED_PRESSED_MS); /* the waiting light runs into the press */
}

/* Slow blinking for 30 s, then dark. */
static void signed_then_dark(void)
{
	unsigned from = steps + 1;
	advance(5000);
	for (unsigned i = from + 1; i < from + 8; ++i) assert(trace[i] == 500);
	advance(24000);
	assert(pending_work);
	advance(3000);
	assert(!lamp && !pending_work);
}

static void failed_then_dark(void)
{
	while (lamp) advance(1);
	advance(2000);
	assert(steps >= 5 && flashes(steps - 5, FAILED, 3) && !pending_work);
}

static void test_answers(void)
{
	for (enum ask what = ORDER; what <= REGISTER; ++what) {
		right_pin = true;
		ask(what);
		approve();
		if (what == ORDER || what == RETURN) signed_then_dark();
		else {
			advance(3000); /* the request ends; nothing was signed and nothing failed */
			assert(!lamp && !pending_work && trace[steps - 1] == 80);
		}
		if (what == ORDER) assert(stored == NU54_ORDER_APPROVED);

		right_pin = false;
		ask(what);
		approve();
		failed_then_dark();
		assert(!stored);

		ask(what);
		device_pilot_button(PROMPT, false);
		assert(!pin_at);
		failed_then_dark();
		if (what == ORDER) assert(stored == NU54_ORDER_REFUSED);

		pin_wait = 60;
		ask(what);
		approve();
		advance(2000);
		assert(flashes(steps - 9, LOCKED, 5)); /* not the failure pattern */
		pin_wait = 0;
	}
	right_pin = true;
	confirm_fails = true;
	ask(REGISTER);
	approve();
	failed_then_dark();
	confirm_fails = false;
	sign_fails = true;
	for (enum ask what = ORDER; what <= RETURN; ++what) {
		ask(what);
		approve();
		failed_then_dark();
	}
	sign_fails = false;
	send_fails = true;
	ask(ORDER); /* the stored signature can still be read back */
	approve();
	signed_then_dark();
	ask(RETURN); /* this one is lost */
	approve();
	failed_then_dark();
	send_fails = false;
}

static void test_early_presses(void)
{
	right_pin = true;
	ask(ORDER);
	early[0] = early[1] = (board_button_event_t){.button = 0, .event = NU54_BTN_CLICK, .prompt = PROMPT};
	early_count = 2;
	approve();
	signed_then_dark();
}

/* A second request within the 30 s: when it ends unanswered, the first result does not come back. */
static void test_next_request(void)
{
	right_pin = true;
	ask(ORDER);
	approve();
	advance(2000);
	status_led_waiting(true);
	advance(1000);
	status_led_waiting(false);
	advance(1000);
	assert(!lamp && !pending_work);
}

static void test_button_reset(void)
{
	ask(ORDER);
	reset_link();
	right_pin = false;
	start();
	device_pilot_button_reset();
	failed_then_dark();
	order_open = true;
	start();
	device_pilot_button_reset();
	failed_then_dark();
	pin_wait = 60;
	start();
	device_pilot_button_reset(); /* an open order is the reason, the PIN was not asked for */
	failed_then_dark();
	order_open = false;
	start();
	device_pilot_button_reset();
	advance(2000);
	assert(flashes(steps - 9, LOCKED, 5));
	pin_wait = 0;
	assert(!erased);
	right_pin = true;
	start();
	device_pilot_button_reset();
	assert(erased && flashes(steps - 3, DONE, 2));
}

int main(void)
{
	test_answers();
	test_early_presses();
	test_next_request();
	test_button_reset();
	return 0;
}
