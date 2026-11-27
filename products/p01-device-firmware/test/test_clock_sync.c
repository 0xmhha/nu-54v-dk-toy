/* The registered owner's phone sets the clock of a board that was without power (decision of
 * 2026-11-27). The board's clock and storage code (device_install.c), its message handling
 * (device_pilot.c: bind_session, frame, clock_request, order_request) and its order records
 * (device_order.c) run here as the board builds them, with the real transport, the real order
 * checks and an order a store signed (packages/protocol/v2/testdata/order-lock.vectors). The TLS
 * handshake is a stand-in that judges the certificate's period with the board's own rule. */
#include "install_store_fixture.h"

#include "device_order.h"
#include "nu54_owner.h"
#include "nu54_spending.h"

#include <stdlib.h>

/* ---- the rest of device_pilot.c ---- */
typedef struct {
	uint64_t from, until;
} device_tls_v2_validity;
typedef int32_t atomic_t;
typedef int32_t atomic_val_t;
#define NU54_PILOT_PROMPT_MASK UINT32_C(0x80000000)
#define K_NO_WAIT 0
static nu54_v2_profile profile = NU54_V2_MANAGEMENT;
static nu54_v2_link link, phone;
static nu54_v2_journal_entry journal_slots[8], phone_slots[8];
static nu54_v2_journal journal = {journal_slots, 8, hash, 0}, phone_journal = {phone_slots, 8, hash, 0};
static nu54_owner_enrollment enrollment;
static nu54_local_approval approval;
static nu54_spend_action spend_action;
static bool recovering, clock_only, display_confirmed;
static device_tls_v2_validity session_validity;
static int64_t session_began;
static uint32_t next_prompt;
static atomic_t pending_prompt;
static unsigned order_slot, managed, clock_turns;
static uint64_t order_amount, order_expires;
static int tls, clock_worker;
static struct {
	nu54_install_clock_told told;
	int64_t until;
	bool pending;
} clock_wait;
static int radio, *connection; /* the Bluetooth link of the session, as the board holds it */
static unsigned prompts, lamp_waits;
static uint32_t shown_prompt;

/* The phone's clock, which is the calendar unless a case says the phone is wrong. The board does
 * not see it. */
static uint64_t real;

static uint8_t quote[NU54_V2_SIGNED_SIZE], merchant_signer[20], confirmer[20];
static const uint8_t phone_key[32] = {7, 7}, other_key[32] = {8, 8};

/* What the phone's certificate says, and whether its key is the one the handshake proved. */
static struct {
	uint64_t from, until;
	uint8_t key[32];
} presented;
static uint8_t epoch[16];

static uint64_t trusted_now(void) { return device_install_now(); }
static int pay_link_waiting(void) { return 0; }
static void atomic_set(atomic_t *value, int32_t next) { *value = next; }
static void board_io_set_prompt(uint32_t prompt) { shown_prompt = prompt; prompts += prompt != 0; }
static void status_led_waiting(bool on) { lamp_waits += on; }
static void k_work_reschedule(int *work, int delay) { (void)work; (void)delay; ++clock_turns; }
static uint64_t sys_get_be64(const uint8_t *in)
{
	uint64_t value = 0;
	for (int i = 0; i < 8; ++i) value = (value << 8) | in[i];
	return value;
}
static int present(void *context, const uint8_t *preimage, size_t length)
{
	(void)context; (void)preimage;
	return length == NU54_LOCAL_PREIMAGE_SIZE ? 0 : -1;
}
static bool payment_generation_current(const nu54_install_policy *current) { return current != NULL; }
static bool payment_address(const nu54_install_policy *current, uint8_t address[20])
{
	(void)current;
	memcpy(address, quote + NU54_V2_PAYER_OFFSET + 12, 20);
	return true;
}

/* As device_tls_v2_bind: the pinned key, the certificate's period by the board's rule, and a
 * certificate that is not valid yet only where the caller asked for the period. */
static int device_tls_v2_bind(int *session, nu54_v2_link *out, nu54_v2_profile route, const uint8_t *expected_peer,
			      nu54_v2_journal *records, uint64_t now, device_tls_v2_validity *validity)
{
	(void)session;
	const int window = nu54_install_certificate_window(presented.from, presented.until, now);
	if (!expected_peer || memcmp(expected_peer, presented.key, 32) || window == NU54_CERTIFICATE_REFUSED ||
	    (validity && route != NU54_V2_MANAGEMENT) || (window == NU54_CERTIFICATE_EARLY && !validity)) return -1;
	if (validity) *validity = (device_tls_v2_validity){presented.from, presented.until};
	return nu54_v2_link_init(out, route, NU54_V2_BOARD, epoch, presented.key, records);
}

/* ---- the phone's end of the session ---- */
static uint8_t answer[64];
static size_t answer_length;
static bool answered;

/* As reset_link: what a session leaves is gone with it. */
static void end_session(void)
{
	nu54_v2_link_close(&link);
	nu54_local_approval_cancel(&approval);
	memset(&spend_action, 0, sizeof(spend_action));
	memset(&clock_wait, 0, sizeof(clock_wait));
	memset(&enrollment, 0, sizeof(enrollment));
	connection = NULL;
	shown_prompt = 0;
	order_slot = 0;
	order_amount = order_expires = 0;
	pending_prompt = 0;
	display_confirmed = recovering = clock_only = false;
	session_validity = (device_tls_v2_validity){0};
}

static int send_frame(void *context, const uint8_t *bytes, size_t length)
{
	(void)context;
	nu54_v2_request reply;
	if (nu54_v2_link_receive(&phone, bytes, length, real, &reply) != NU54_V2_NEW) return -1;
	assert(reply.payload_length <= sizeof(answer));
	if (reply.payload_length) memcpy(answer, reply.payload, reply.payload_length);
	answer_length = reply.payload_length;
	answered = reply.command == NU54_V2_REPLY;
	return 0;
}

static int send_receipt(nu54_v2_request *request, const uint8_t *signature, size_t length)
{
	request->command = NU54_V2_REPLY;
	request->payload = signature;
	request->payload_length = length;
	return nu54_v2_link_send(&link, request, send_frame, NULL);
}

/* Every management request but the time; the board's own answers are not looked at here. */
static int manage(nu54_v2_request *request, uint64_t now, const nu54_install_policy *current)
{
	(void)now; (void)current;
	++managed;
	return send_receipt(request, NULL, 0);
}

static int receipt_status(nu54_v2_request *request, const nu54_install_policy *current)
{
	(void)current;
	++managed;
	return send_receipt(request, NULL, 0);
}

/* What apply_install keeps of a grant besides the board's time: the TLS configuration, which is
 * not looked at here. */
struct provisioned_tls {
	mbedtls_x509_crt ca, board;
	mbedtls_pk_context key;
	device_tls_v2_profile profiles[2];
};
static struct provisioned_tls tls_banks[2];
static unsigned tls_bank;
static bool ready;
static void free_tls_bank(struct provisioned_tls *bank) { (void)bank; }

static int bind_session(const nu54_vault *, const uint8_t *, uint64_t);
static int frame(const uint8_t *, size_t);
#include "board_clock.h"

#define FREE ((uint64_t)CONFIG_NU54_CLOCK_FREE_SECONDS)
static const nu54_install_clock_limits build_limits = {.session_seconds = CONFIG_NU54_CLOCK_SESSION_SECONDS,
							.free_seconds = CONFIG_NU54_CLOCK_FREE_SECONDS};

/* The rule with the limits of this build, for a message that came `age` seconds into its session. */
static int rule(const nu54_install_clock *current, uint64_t end, uint64_t time, uint64_t expires, uint64_t age,
		nu54_install_clock *next)
{
	const nu54_install_clock_word word = {.time = time, .expires = expires, .session_seconds = age};
	return nu54_install_clock_set(current, end, &word, &build_limits, next);
}

/* The same for a phone that has moved the clock by `moved` seconds since the board's start, with
 * SW1 pressed for this message or not. */
static int rule_after(const nu54_install_clock *current, uint64_t end, uint64_t time, uint64_t moved, bool button,
		      nu54_install_clock *next)
{
	const nu54_install_clock_word word = {.time = time, .expires = time + 90, .moved_seconds = moved, .button = button};
	return nu54_install_clock_set(current, end, &word, &build_limits, next);
}

/* The board's side of a button while a time waits for it, as device_pilot_button: SW1 or SW2.
 * Returns what clock_button returns. */
static int button(bool approved)
{
	assert(clock_wait.pending && pending_prompt && connection);
	const int took = clock_button(approved);
	if (took) end_session(); /* failed: reset_link */
	else pending_prompt = 0, shown_prompt = 0; /* answered */
	return took;
}

/* The TLS work item, which looks at the session every 25 ms (poll_tls). */
static void board_polls(void)
{
	if (connection && (!device_install_policy() || clock_wait_over(uptime))) end_session();
}

/* The operator's grant arrives `took` milliseconds after the board's offer, as on the board: the
 * storage and the clock (device_install_load) and what the session code does with a new grant. */
static int operator_installs(const uint8_t record[GRANT_SIZE], int64_t took)
{
	offered = true;
	challenge_birth = uptime - took;
	return apply_install(record, GRANT_SIZE);
}

/* What the phone sends is on its way first and reaches the board when the phone has sent it all. */
static struct {
	uint8_t bytes[NU54_V2_FRAGMENT_MAX][NU54_V2_FRAME_MAX];
	size_t length[NU54_V2_FRAGMENT_MAX], count;
} air;

static int to_air(void *context, const uint8_t *bytes, size_t length)
{
	(void)context;
	assert(air.count < NU54_V2_FRAGMENT_MAX);
	memcpy(air.bytes[air.count], bytes, length);
	air.length[air.count++] = length;
	return 0;
}

static int board_reads(const uint8_t *bytes, size_t length)
{
	const int took = frame(bytes, length);
	if (took) end_session(); /* as poll_tls does */
	return took;
}

/* The phone opens a TLS session with a certificate for `key`, valid from `from` until `until`.
 * As poll_tls: the pin is the vault's owner, or the grant's phone while there is none. */
static int connect_as(const uint8_t key[32], uint64_t from, uint64_t until)
{
	const nu54_install_policy *current = device_install_policy();
	const uint8_t *pin = NULL;
	end_session();
	if (!current) return -1;
	if (profile == NU54_V2_PAYMENT) pin = current->terminal;
	else if (!nu54_owner_session_pin(&vault, current->owner, &pin)) return -1;
	++epoch[0];
	memcpy(presented.key, key, 32);
	presented.from = from;
	presented.until = until;
	session_began = uptime;
	if (trusted_now() >= current->not_after || bind_session(&vault, pin, trusted_now())) return -1;
	connection = &radio;
	memset(journal_slots, 0, sizeof(journal_slots)); /* journal_new_session */
	/* `real` is this phone's clock, and some cases set it wrong. Its own records start anew. */
	memset(phone_slots, 0, sizeof(phone_slots));
	phone_journal.trusted_time = 0;
	assert(!nu54_v2_link_init(&phone, profile, profile == NU54_V2_PAYMENT ? NU54_V2_TERMINAL : NU54_V2_OWNER, epoch,
				  key, &phone_journal));
	return 0;
}

/* A certificate issued long ago that lasts beyond every test: the usual case. */
static int connect_phone(void)
{
	return connect_as(phone_key, 1, UINT64_MAX);
}

static uint8_t request_count;
static int phone_asks(nu54_v2_command command, const uint8_t *payload, size_t length, uint64_t expires)
{
	nu54_v2_request request = {.command = command, .expires = expires, .payload = payload, .payload_length = length};
	request.id[0] = ++request_count;
	request.id[1] = 1;
	answered = false;
	air.count = 0;
	if (nu54_v2_link_send(&phone, &request, to_air, NULL)) return -1;
	int took = -1;
	for (size_t i = 0; i < air.count; ++i) took = board_reads(air.bytes[i], air.length[i]);
	return took;
}

/* The phone tells the time it reads now and gives the request 90 seconds, as its short requests. */
static int tell(uint64_t time, uint64_t expires)
{
	uint8_t body[NU54_CLOCK_SET_REQUEST_SIZE] = {NU54_CLOCK_SET_OP};
	put64(body + 1, time);
	return phone_asks(NU54_V2_CLOCK, body, sizeof(body), expires);
}
static int tell_time(uint64_t time) { return tell(time, time + 90); }

struct told {
	int result;
	uint64_t earliest, latest, end;
	uint32_t days;
};
static struct told told(void)
{
	assert(answered && answer_length == NU54_CLOCK_SET_REPLY_SIZE && answer[0] == NU54_CLOCK_SET_OP);
	return (struct told){answer[1], sys_get_be64(answer + 2), sys_get_be64(answer + 10), sys_get_be64(answer + 18),
			     (uint32_t)(sys_get_be64(answer + 22) & UINT32_MAX)};
}

static int pay(void)
{
	uint8_t body[NU54_ORDER_REQUEST_SIZE];
	memcpy(body, "N54O", 4);
	memcpy(body + 4, confirmer, 20);
	memcpy(body + 24, quote, sizeof(quote));
	return phone_asks(NU54_V2_PAY_PREPARE, body, sizeof(body), real + 60);
}

static int status(void)
{
	static const uint8_t body[1] = {3};
	return phone_asks(NU54_V2_MANAGE, body, sizeof(body), real + 90);
}

/* ---- the store's order and the board's time line ---- */
static uint64_t quote_issued, quote_end, calendar;

static void load_vector(const char *path, const char *key, uint8_t *out, size_t length)
{
	static char line[2 * NU54_V2_UNSIGNED_SIZE + 64];
	FILE *file = fopen(path, "r");
	assert(file);
	while (fgets(line, sizeof(line), file)) {
		const size_t size = strlen(key);
		if (strncmp(line, key, size) || line[size] != '=') continue;
		const char *hex = line + size + 3; /* =0x */
		assert(strcspn(hex, "\n") == length * 2);
		for (size_t i = 0; i < length; ++i) {
			unsigned byte;
			assert(sscanf(hex + 2 * i, "%2x", &byte) == 1);
			out[i] = (uint8_t)byte;
		}
		fclose(file);
		return;
	}
	assert(!"vector key missing");
}

/* The grant of the fixture names no real store. What the grant fixes about an order is taken from
 * the store's signed order, as the operator's grant for that store would have it. */
static void grant_names_the_store(void)
{
	memcpy(policy.allowlist.contract, quote + NU54_V2_VERIFYING_CONTRACT_OFFSET + 12, 20);
	memcpy(policy.allowlist.token, quote + NU54_V2_ASSET_OFFSET + 12, 20);
	memcpy(policy.allowlist.beneficiary, quote + NU54_V2_BENEFICIARY_OFFSET + 12, 20);
	policy.allowlist.max_amount = NU54_PILOT_TESTUSDC2_CAP;
	memcpy(policy.merchant_signer, merchant_signer, 20);
	memcpy(policy.merchant, quote + NU54_V2_MERCHANT_ID_OFFSET, 32);
	policy.merchant_key_version = sys_get_be64(quote + NU54_V2_MERCHANT_KEY_VERSION_OFFSET + 24);
	policy.beneficiary_version = sys_get_be64(quote + NU54_V2_BENEFICIARY_VERSION_OFFSET + 24);
	assert(policy.payer_role == NU54_V2_OWNER_ROLE && policy.allowlist.commercial_testnet);
	assert(policy.payer_generation == sys_get_be64(quote + NU54_V2_PAYER_GENERATION_OFFSET + 24));
}

/* The board restarts: memory is gone, storage is read, the stored grant is taken up again. */
static int board_restart(void)
{
	end_session();
	memset(journal_slots, 0, sizeof(journal_slots));
	journal.trusted_time = 0;
	device_order_start(); /* device_spending_init */
	const int restored = restart();
	if (!restored) grant_names_the_store();
	resume_order_hold(); /* device_pilot_init, whether a grant was found or not */
	return restored;
}

/* Time passes with the board switched on, and with it switched off. */
static void on_for(uint64_t seconds)
{
	run(seconds);
	real += seconds;
}

static uint64_t board_end;

/* A board with a registered owner and a grant. It saved its time, was without power, and is
 * switched on again at `calendar` with its lower bound of the time `late` seconds behind. Its
 * latest possible time is two seconds above that (the grant took two seconds to arrive). */
static void late_board(uint64_t late)
{
	uint8_t record[GRANT_SIZE];
	const uint64_t granted = calendar - late - 600;
	wipe(1, 0);
	memcpy(vault.record.owner, phone_key, 32);
	end_session();
	real = granted;
	board_end = granted + 31 * DAY;
	make_grant(record, 1, 1, granted, board_end);
	assert(!install(record));
	on_for(600); /* the time is saved here */
	real += late;
	assert(board_restart() == 0 && real == calendar);
	assert(device_install_earliest() == calendar - late && device_install_now() == calendar - late + 2);
}

/* ---- cases ---- */

/* The rule itself (core/src/nu54_install.c). */
static void test_the_rule(void)
{
	const uint64_t now = UINT64_C(1800000000), end = now + 10 * DAY;
	const nu54_install_clock clock = {.earliest = now, .offset = 2, .phone_days = 4};
	nu54_install_clock next = {0};

	/* Forward only: the time the board has, or an earlier one, moves nothing. */
	assert(rule(&clock, end, now, now + 90, 0, &next) == NU54_CLOCK_NOT_FORWARD);
	assert(rule(&clock, end, now - 1, now + 89, 0, &next) == NU54_CLOCK_NOT_FORWARD);
	assert(rule(&clock, end, 1, 91, 0, &next) == NU54_CLOCK_NOT_FORWARD && !next.earliest);
	/* Less than a step on, and inside the board's interval: nothing moves either. */
	for (uint64_t ahead = 1; ahead < NU54_CLOCK_STEP_SECONDS; ++ahead)
		assert(rule(&clock, end, now + ahead, now + ahead + 90, 0, &next) == NU54_CLOCK_NOT_FORWARD);
	assert(!next.earliest);
	/* Past the board's latest possible time the board is late, by however little: that moves. */
	const nu54_install_clock tight = {.earliest = now, .offset = 1};
	assert(rule(&tight, end, now + 1, now + 91, 0, &next) == NU54_CLOCK_NOT_FORWARD);
	assert(rule(&tight, end, now + 2, now + 92, 0, &next) == NU54_CLOCK_MOVED && next.earliest == now + 2);
	/* One step on, with what the session vouches for as the distance to the latest time. */
	assert(rule(&clock, end, now + NU54_CLOCK_STEP_SECONDS, now + 91, 0, &next) == NU54_CLOCK_MOVED);
	assert(next.earliest == now + NU54_CLOCK_STEP_SECONDS && next.offset == NU54_CLOCK_PHONE_SECONDS && next.phone_days == 4);
	assert(rule(&clock, end, now + 1000, now + 1090, 7, &next) == NU54_CLOCK_MOVED);
	assert(next.earliest == now + 1000 && next.offset == 7 + NU54_CLOCK_PHONE_SECONDS);
	/* The interval becomes the phone's: the latest possible time comes down with it. (Before the
	 * audit of 2026-11-27 it never moved back, and this case kept 50 seconds.) */
	const nu54_install_clock slow = {.earliest = now, .offset = 60, .phone_days = 4, .hold = now + 77};
	assert(rule(&slow, end, now + 10, now + 100, 0, &next) == NU54_CLOCK_MOVED);
	assert(next.earliest == now + 10 && next.offset == NU54_CLOCK_PHONE_SECONDS);
	assert(rule(&slow, end, now + 56, now + 100, 0, &next) == NU54_CLOCK_MOVED && next.offset == 5);
	assert(rule(&slow, end, now + 10, now + 100, 9, &next) == NU54_CLOCK_MOVED && next.offset == 9 + NU54_CLOCK_PHONE_SECONDS);
	/* A time that does not move the lower bound brings the latest possible time down to what the
	 * phone vouches for: its time, the session's age and its own allowance. */
	next = (nu54_install_clock){0};
	assert(rule(&slow, end, now, now + 90, 0, &next) == NU54_CLOCK_MOVED);
	assert(next.earliest == now && next.offset == NU54_CLOCK_PHONE_SECONDS && next.phone_days == 4 && next.hold == now + 77);
	assert(rule(&slow, end, now + 2, now + 92, 4, &next) == NU54_CLOCK_MOVED);
	assert(next.earliest == now && next.offset == 2 + 4 + NU54_CLOCK_PHONE_SECONDS);
	/* A phone that is behind the board within its allowance vouches for no less than the board's
	 * own lower bound. One that is further behind contradicts the board and brings nothing down. */
	assert(rule(&slow, end, now - NU54_CLOCK_PHONE_SECONDS, now + 80, 0, &next) == NU54_CLOCK_MOVED);
	assert(next.earliest == now && next.offset == NU54_CLOCK_PHONE_SECONDS);
	next = (nu54_install_clock){0};
	assert(rule(&slow, end, now - NU54_CLOCK_PHONE_SECONDS - 1, now + 80, 0, &next) == NU54_CLOCK_NOT_FORWARD);
	/* Down by a step or more, or not at all; and never up. */
	const nu54_install_clock narrow = {.earliest = now, .offset = NU54_CLOCK_PHONE_SECONDS + NU54_CLOCK_STEP_SECONDS};
	assert(rule(&narrow, end, now, now + 90, 1, &next) == NU54_CLOCK_NOT_FORWARD);
	assert(rule(&narrow, end, now + 1, now + 90, 0, &next) == NU54_CLOCK_NOT_FORWARD);
	assert(rule(&narrow, end, now, now + 90, 15, &next) == NU54_CLOCK_NOT_FORWARD && !next.earliest);
	assert(rule(&narrow, end, now, now + 90, 0, &next) == NU54_CLOCK_MOVED && next.offset == NU54_CLOCK_PHONE_SECONDS);

	/* A message that came later in its session than the limit says too little about the time now.
	 * It is answered and changes nothing, whatever it carries. */
	next = (nu54_install_clock){0};
	const uint64_t limit = CONFIG_NU54_CLOCK_SESSION_SECONDS;
	assert(rule(&clock, end, now + 1000, now + 1090, limit, &next) == NU54_CLOCK_MOVED);
	assert(next.offset == limit + NU54_CLOCK_PHONE_SECONDS);
	next = (nu54_install_clock){0};
	assert(rule(&clock, end, now + 1000, now + 1090, limit + 1, &next) == NU54_CLOCK_LATE);
	assert(rule(&clock, end, now + 1000, now + 1090, 119, &next) == NU54_CLOCK_LATE);
	assert(rule(&clock, end, now + 1000, now + 1090, UINT64_MAX, &next) == NU54_CLOCK_LATE);
	assert(rule(&slow, end, now, now + 90, limit + 1, &next) == NU54_CLOCK_LATE);
	assert(rule(&clock, end, now - 50, now + 40, limit + 1, &next) == NU54_CLOCK_LATE);
	assert(rule(&clock, end, end + DAY, end + DAY + 90, limit + 1, &next) == NU54_CLOCK_LATE && !next.earliest);
	/* A message that is not well formed is not answered, late or not. */
	assert(rule(&clock, end, now + 1000, now + 1000, limit + 1, &next) == -1);

	/* Never to the grant's end or past it. (The board is within a day of the end here: a move of
	 * ten days is more than the phone makes by itself, see test_the_phone_alone_moves_a_day.) */
	const nu54_install_clock last_day = {.earliest = end - 3600, .offset = 2, .phone_days = 4};
	assert(rule(&last_day, end, end - 1, end + 89, 0, &next) == NU54_CLOCK_MOVED && next.earliest == end - 1);
	assert(rule(&clock, end, end - 1, end + 89, 0, &next) == NU54_CLOCK_BUTTON);
	next = (nu54_install_clock){0};
	assert(rule(&clock, end, end, end + 90, 0, &next) == NU54_CLOCK_PAST_END);
	assert(rule(&clock, end, end + 40 * DAY, end + 40 * DAY + 90, 0, &next) == NU54_CLOCK_PAST_END);
	assert(rule(&clock, end, UINT64_MAX - 1, UINT64_MAX, 0, &next) == NU54_CLOCK_PAST_END);
	assert(!next.earliest);

	/* The message must end shortly after the time it carries, by its own word. */
	assert(rule(&clock, end, now + 500, now + 500 + NU54_PILOT_BRIEF_SECONDS, 0, &next) == NU54_CLOCK_MOVED);
	assert(rule(&clock, end, now + 500, now + 501 + NU54_PILOT_BRIEF_SECONDS, 0, &next) == -1);
	assert(rule(&clock, end, now + 500, now + 500, 0, &next) == -1);
	assert(rule(&clock, end, now + 500, now + 499, 0, &next) == -1);
	assert(rule(&clock, end, now + 500, UINT64_MAX, 0, &next) == -1);
	assert(rule(&clock, end, 0, 90, 0, &next) == -1);
	/* A board that does not know the time is not told it this way: that takes a grant. */
	const nu54_install_clock none = {0};
	assert(rule(&none, end, now, now + 90, 0, &next) == -1);
	assert(rule(NULL, end, now, now + 90, 0, &next) == -1);

	/* A later day opens once per move, however many days the move covers. */
	const uint64_t midnight = (now / DAY + 1) * DAY;
	assert(rule(&clock, end, midnight - 1, midnight + 80, 0, &next) == NU54_CLOCK_MOVED && next.phone_days == 4);
	assert(rule(&clock, end, midnight, midnight + 90, 0, &next) == NU54_CLOCK_MOVED && next.phone_days == 5);
	assert(rule(&clock, end, midnight + 3 * DAY, midnight + 3 * DAY + 90, 0, &next) == NU54_CLOCK_BUTTON);
	assert(rule_after(&clock, end, midnight + 3 * DAY, 0, true, &next) == NU54_CLOCK_MOVED); /* with SW1 */
	assert(next.phone_days == 5);

	uint8_t reply[NU54_CLOCK_SET_REPLY_SIZE];
	const nu54_install_clock shown = {.earliest = 0x0102030405060708, .offset = 0x10, .phone_days = 0x0a0b0c0d};
	static const uint8_t expected[NU54_CLOCK_SET_REPLY_SIZE] = {
		0x81, 1, 1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 0x18, 0, 0, 0, 0, 0, 0, 0x30, 0x39, 0x0a, 0x0b, 0x0c, 0x0d,
	};
	nu54_install_clock_reply(NU54_CLOCK_MOVED, &shown, 12345, reply);
	assert(!memcmp(reply, expected, sizeof(reply)));

	/* The certificate's period, and what a session that was opened early may do. */
	assert(nu54_install_certificate_window(100, 200, 100) == NU54_CERTIFICATE_CURRENT);
	assert(nu54_install_certificate_window(100, 200, 199) == NU54_CERTIFICATE_CURRENT);
	assert(nu54_install_certificate_window(100, 200, 99) == NU54_CERTIFICATE_EARLY);
	assert(nu54_install_certificate_window(100, 200, 200) == NU54_CERTIFICATE_REFUSED);
	assert(nu54_install_certificate_window(100, 200, 0) == NU54_CERTIFICATE_REFUSED);
	assert(nu54_install_certificate_window(200, 200, 150) == NU54_CERTIFICATE_REFUSED);
	for (nu54_v2_command command = NU54_V2_MANAGE; command <= NU54_V2_CLOCK; ++command) {
		assert(nu54_install_session_admits(false, command));
		assert(nu54_install_session_admits(true, command) == (command == NU54_V2_CLOCK));
	}
}

/* The audit's finding first: a restarted board refuses an honest order as soon as its latest
 * possible time is behind the time the store signed the order at. Then the same order after the
 * phone told the time. */
static void test_orders_work_again(void)
{
	static const uint64_t late[] = {0, 5, 6, 60, 600, 3 * 3600, FREE, FREE + 1, 3 * DAY, 31 * DAY - 3600 - 600};
	for (size_t i = 0; i < sizeof(late) / sizeof(late[0]); ++i) {
		/* The store signed the order three seconds ago. The board's latest possible time is
		 * `late - 2` behind the calendar, so the order is refused once the board is more than
		 * 2 + 3 seconds late. */
		late_board(late[i]);
		assert(!connect_phone());
		assert(pay() == (late[i] <= 5 ? 0 : -1));
		assert(approval.pending == (late[i] <= 5));
		/* The usual requests of the phone: 90 seconds against the transport's 300. */
		assert(!connect_phone());
		assert(status() == (late[i] <= 212 ? 0 : -1));

		late_board(late[i]);
		assert(!connect_phone());
		clock_turns = 0;
		assert(!tell_time(real));
		struct told now = told();
		if (late[i] > FREE) {
			/* Off for more than a day: the phone alone does not move the board that far (decision of
			 * 2026-11-27). The answer says so, nothing has changed, and the board waits for SW1.
			 * Its owner presses it, and the time is taken. */
			assert(now.result == NU54_CLOCK_BUTTON && !clock_turns && pending_prompt && clock_wait.pending);
			assert(now.earliest == real - late[i] && now.latest == now.earliest + 2 && now.end == board_end && !now.days);
			assert(!button(true) && !pending_prompt);
			now = (struct told){NU54_CLOCK_MOVED, device_install_earliest(), device_install_now(), board_end,
					    (uint32_t)clock_saved.phone_days};
		}
		if (late[i]) {
			assert(now.result == NU54_CLOCK_MOVED && clock_turns == 1);
			assert(now.earliest == real && now.latest == real + NU54_CLOCK_PHONE_SECONDS);
		} else {
			assert(now.result == NU54_CLOCK_NOT_FORWARD && !clock_turns);
			assert(now.earliest == real && now.latest == real + 2);
		}
		assert(now.end == board_end && now.days == (late[i] >= DAY));
		assert(device_install_earliest() == now.earliest && device_install_now() == now.latest);
		/* The same session goes on, and the order is taken. */
		assert(!status() && managed);
		assert(!pay() && approval.pending && pending_prompt && order_expires == quote_end);
		/* The time is in storage: a restart right now goes on from it. */
		assert(board_restart() == 0 && device_install_earliest() == real);
		assert(device_install_now() == real + (late[i] ? NU54_CLOCK_PHONE_SECONDS : 2));
		assert(!connect_phone() && !pay() && approval.pending);
	}
}

/* What the session vouches for: the phone read its clock after the session began, so the true
 * time is at most the session's age, and the phone's own error, past the time it sent. */
static void test_session_age(void)
{
	late_board(3 * 3600);
	assert(!connect_phone());
	uptime += 6400; /* the phone took its time after the handshake */
	real += 6;
	assert(!tell_time(real) && told().result == NU54_CLOCK_MOVED);
	assert(told().earliest == real && told().latest == real + 7 + NU54_CLOCK_PHONE_SECONDS);
	assert(device_install_earliest() == real && device_install_now() == real + 7 + NU54_CLOCK_PHONE_SECONDS);
	/* The board's time runs on from the moment of the move. */
	uptime += 2000;
	assert(device_install_earliest() == real + 2);
	/* A later session starts its own count, and what the earlier one left above it comes down.
	 * (Before the audit of 2026-11-27 the seven seconds stayed: this said `+ 7 - 1`.) */
	real += 2;
	writes = 0;
	assert(!connect_phone() && !tell_time(real + 1) && told().result == NU54_CLOCK_MOVED && writes == 1);
	assert(told().earliest == real && told().latest == real + 1 + NU54_CLOCK_PHONE_SECONDS);
	assert(device_install_earliest() == real && device_install_now() == real + 1 + NU54_CLOCK_PHONE_SECONDS);
	assert(!connect_phone() && !tell_time(real + 30) && told().latest == real + 30 + NU54_CLOCK_PHONE_SECONDS);
}

/* Forward only, never past the grant's end, and bound to the message's own end. */
static void test_what_is_refused(void)
{
	late_board(3 * DAY);
	const uint64_t before = device_install_earliest(), latest = device_install_now();
	const struct item saved = *item(INSTALL_CLOCK);

	/* A phone whose clock is behind the board's. The time the board has, or an earlier one:
	 * answered, nothing moves, the session goes on. */
	real = before;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_NOT_FORWARD);
	real = before - 50;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_NOT_FORWARD);
	assert(told().earliest == before && told().latest == latest && device_install_earliest() == before);
	assert(!memcmp(item(INSTALL_CLOCK), &saved, sizeof(saved)));
	assert(!status()); /* the session goes on */
	/* A time so far back that its message has ended by the board's time is not even read. */
	real = before - 100;
	assert(!connect_phone() && tell_time(real) == -1 && !answered && device_install_earliest() == before);

	/* A phone whose clock is past the grant's end, by a second or by weeks: answered with the
	 * end, nothing moves. */
	real = board_end;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_PAST_END);
	assert(told().end == board_end && told().earliest == before);
	real = board_end + 40 * DAY;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_PAST_END);
	assert(device_install_earliest() == before && !memcmp(item(INSTALL_CLOCK), &saved, sizeof(saved)));
	/* The last seconds of the grant are taken, and the grant has then ended by the board's time:
	 * its latest possible time is past the end. Nothing more is written or moved for it. */
	real = board_end - 3;
	/* That is four weeks ahead of the board, so SW1 is asked for (decision of 2026-11-27). */
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_BUTTON && told().earliest == before);
	assert(!button(true) && device_install_earliest() == real);
	assert(device_install_now() == real + NU54_CLOCK_PHONE_SECONDS && device_install_now() > board_end);
	assert(!device_install_policy() && device_install_keep_time() == -1 && connect_phone() == -1);
	uint8_t reply[NU54_CLOCK_SET_REPLY_SIZE];
	const nu54_install_clock_told again = {.time = board_end - 2, .expires = board_end + 80, .received = uptime};
	assert(device_install_set_time(&again, true, 0, reply) == -1 && device_install_earliest() == board_end - 3);
	assert(board_restart() == 1 && !device_install_policy());

	/* A message that claims a time and lives on long after it: refused whole. */
	late_board(3 * DAY);
	assert(!connect_phone() && tell(real, real + NU54_PILOT_BRIEF_SECONDS + 1) == -1 && !answered);
	assert(!connect_phone() && tell(real, real + 40 * DAY) == -1 && !answered);
	assert(!connect_phone() && tell(real + 2 * DAY, real + 2 * DAY + 400) == -1 && !answered);
	assert(device_install_earliest() == calendar - 3 * DAY);

	/* Not the message of the time command: another operation, another length. */
	uint8_t body[NU54_CLOCK_SET_REQUEST_SIZE + 1] = {NU54_CLOCK_SET_OP};
	put64(body + 1, real);
	assert(!connect_phone() && phone_asks(NU54_V2_CLOCK, body, sizeof(body), real + 90) == -1 && !answered);
	assert(!connect_phone() && phone_asks(NU54_V2_CLOCK, body, sizeof(body) - 2, real + 90) == -1 && !answered);
	body[0] = 3;
	assert(!connect_phone() && phone_asks(NU54_V2_CLOCK, body, NU54_CLOCK_SET_REQUEST_SIZE, real + 90) == -1 && !answered);
	assert(device_install_earliest() == calendar - 3 * DAY);
}

/* Who may tell the time: the phone the vault holds as the owner, on a board that has one, under a
 * grant of that owner's generation, and not while a registration or a recovery is under way. */
static void test_who_may_tell(void)
{
	uint8_t record[GRANT_SIZE];
	const uint64_t before = calendar - 3 * DAY;

	/* The session's peer is not the owner (a recovery session is bound without a pin). */
	late_board(3 * DAY);
	assert(!connect_phone());
	link.peer_identity[5] ^= 1;
	assert(tell_time(real) == -1 && !answered && device_install_earliest() == before);
	/* Another phone with a certificate of the same CA does not get a session at all. */
	assert(connect_as(other_key, 1, UINT64_MAX) == -1);
	/* A terminal: neither the route nor the transport lets the command through. */
	profile = NU54_V2_PAYMENT;
	assert(!connect_as(policy.terminal, 1, UINT64_MAX));
	assert(tell_time(real) == -1 && !answered && device_install_earliest() == before);
	profile = NU54_V2_MANAGEMENT;

	/* During a recovery window, and while a registration is under way. */
	assert(!connect_phone());
	recovering = true;
	assert(tell_time(real) == -1 && !answered && device_install_earliest() == before);
	assert(!connect_phone());
	enrollment.issued = true;
	assert(tell_time(real) == -1 && !answered && device_install_earliest() == before);
	/* While the board waits for a button: an order, a return or reset, a registration. */
	assert(!connect_phone());
	approval.pending = true;
	assert(tell_time(real) == -1 && !answered && device_install_earliest() == before);
	assert(!connect_phone());
	spend_action.pending = true;
	assert(tell_time(real) == -1 && !answered && device_install_earliest() == before);
	assert(!connect_phone());
	enrollment.proven = true;
	assert(tell_time(real) == -1 && !answered && device_install_earliest() == before);

	/* The vault stopped after a failed write, or the owner was revoked. */
	assert(!connect_phone());
	vault.enabled = false;
	assert(tell_time(real) == -1 && !answered && device_install_earliest() == before);
	vault.enabled = true;
	assert(!connect_phone());
	vault.record.flags = NU54_VAULT_REVOKED;
	assert(tell_time(real) == -1 && !answered && device_install_earliest() == before);
	vault.record.flags = 0;
	/* A vault without a generation has no owner, whatever else its record and the grant say. */
	assert(!connect_phone());
	vault.record.generation = policy.payer_generation = 0;
	assert(tell_time(real) == -1 && !answered && device_install_earliest() == before);
	vault.record.generation = policy.payer_generation = 1;
	/* The grant in use is not one of the owner's generation. */
	assert(!connect_phone());
	policy.payer_generation = 2;
	assert(tell_time(real) == -1 && !answered && device_install_earliest() == before);
	policy.payer_generation = 1;
	/* No grant in use: the board has no time to move. */
	assert(!connect_phone());
	active = false;
	assert(tell_time(real) == -1 && !answered);
	active = true;
	assert(device_install_earliest() == before);
	/* The handler itself asks for the owner's route and the owner's end of it, whatever let the
	 * message through to it. */
	uint8_t body[NU54_CLOCK_SET_REQUEST_SIZE] = {NU54_CLOCK_SET_OP};
	nu54_v2_request direct = {.command = NU54_V2_CLOCK, .id = {3}, .expires = real + 90, .payload = body,
				  .payload_length = sizeof(body)};
	put64(body + 1, real);
	assert(!connect_phone());
	profile = NU54_V2_PAYMENT;
	assert(clock_request(&direct, device_install_policy()) == -1 && device_install_earliest() == before);
	profile = NU54_V2_MANAGEMENT;
	link.peer = NU54_V2_TERMINAL;
	assert(clock_request(&direct, device_install_policy()) == -1 && device_install_earliest() == before);
	/* The same phone, with none of that in the way. The board is three days late, which is more
	 * than the phone moves by itself: it is answered, and SW1 takes the time. */
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_BUTTON && device_install_earliest() == before);
	assert(!button(true) && device_install_earliest() == real);

	/* A board without an owner: new, or released by a factory reset. The grant's phone gets a
	 * session to register, and that session does not tell the time. */
	for (int released = 0; released < 2; ++released) {
		wipe(released ? 3 : 0, released ? NU54_VAULT_REVOKED | NU54_VAULT_RELEASED : 0);
		end_session();
		real = calendar - 3 * DAY;
		make_grant(record, 1, released ? 4 : 1, real, real + 31 * DAY);
		assert(!install(record));
		on_for(600);
		real = calendar;
		end_session();
		assert(restart() == 0);
		const uint64_t reached = device_install_earliest();
		assert(connect_phone() == -1); /* this phone is not the one the grant names */
		assert(!connect_as(policy.owner, 1, UINT64_MAX) && !clock_only);
		assert(tell_time(real) == -1 && !answered && device_install_earliest() == reached);
		/* And with a certificate that is not valid yet by the board's clock it gets no session. */
		assert(connect_as(policy.owner, real - 3600, UINT64_MAX) == -1);
	}
}

/* A message that was captured can only ever set the time it carries. */
static void test_replay(void)
{
	late_board(3 * 3600);
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED);
	const uint64_t set = device_install_earliest();
	assert(set == real);

	/* The same request again in its own session, 50 seconds later: it has not ended yet, the
	 * board has passed the time it carries, and nothing moves. The session is too old by then for
	 * any time to be taken, and the answer says so. */
	on_for(50);
	assert(!tell(set, set + 90) && told().result == NU54_CLOCK_LATE);
	assert(told().earliest == set + 50 && device_install_earliest() == set + 50);
	/* In a session of its own, where its time is looked at, it is behind the board. */
	assert(!connect_phone() && !tell(set, set + 90) && told().result == NU54_CLOCK_NOT_FORWARD);
	assert(told().earliest == set + 50 && device_install_earliest() == set + 50);
	/* After its end the board does not read it any more. */
	on_for(41);
	assert(tell(set, set + 90) == -1 && !answered && device_install_earliest() == set + 91);
	/* Nor in a later session, where it would also need that session's keys. */
	assert(!connect_phone() && tell(set, set + 90) == -1 && !answered && device_install_earliest() == set + 91);

	/* The frame itself, in the session it came from and in a later one: the transport refuses
	 * it by its counter and by its epoch. */
	late_board(3 * 3600);
	assert(!connect_phone());
	uint8_t body[NU54_CLOCK_SET_REQUEST_SIZE] = {NU54_CLOCK_SET_OP};
	nu54_v2_request request = {.command = NU54_V2_CLOCK, .id = {9, 9}, .expires = real + 90, .payload = body,
				   .payload_length = sizeof(body)};
	put64(body + 1, real);
	air.count = 0;
	answered = false;
	assert(!nu54_v2_link_send(&phone, &request, to_air, NULL) && air.count == 1);
	const uint8_t *const kept = air.bytes[0];
	const size_t kept_length = air.length[0];
	assert(!board_reads(kept, kept_length) && told().result == NU54_CLOCK_MOVED && device_install_earliest() == real);
	const uint64_t once = device_install_earliest();
	on_for(10);
	assert(board_reads(kept, kept_length) == -1 && device_install_earliest() == once + 10);
	assert(!connect_phone() && board_reads(kept, kept_length) == -1 && device_install_earliest() == once + 10);

	/* A board that lost power right after the move still holds the moved time, so the captured
	 * message stays behind it. Only storage put back to an older state could bring the board's
	 * time under the message's, and then the message sets a time that has really passed. */
	assert(board_restart() == 0 && device_install_earliest() == once);
	assert(!connect_phone() && !tell(once, once + 90) && told().result == NU54_CLOCK_NOT_FORWARD);
}

/* The owner's certificate was issued while the board was without power. */
static void test_certificate_issued_while_off(void)
{
	const uint64_t issued = calendar - 3600, expires = calendar + 20 * DAY;

	/* The board is three days behind: by its clock the certificate is not valid yet. Without the
	 * rule for the time command this phone would have no session. */
	late_board(3 * DAY);
	assert(nu54_install_certificate_window(issued, expires, device_install_now()) == NU54_CERTIFICATE_EARLY);
	assert(!connect_as(phone_key, issued, expires) && clock_only);
	/* The session tells the time and does nothing else: no order, no status, no other command. */
	managed = 0;
	assert(pay() == -1 && !approval.pending && !pending_prompt);
	assert(!connect_as(phone_key, issued, expires) && clock_only);
	assert(status() == -1 && !managed && !answered);
	for (nu54_v2_command command = NU54_V2_MANAGE; command <= NU54_V2_STATUS; ++command) {
		static const uint8_t body[2] = {NU54_SPEND_STATUS_OP, 0};
		assert(!connect_as(phone_key, issued, expires) && clock_only);
		assert(phone_asks(command, body, 1, device_install_now() + 60) == -1 && !managed && !answered);
	}
	assert(device_install_earliest() == calendar - 3 * DAY);
	/* It tells the time. Three days are more than the phone moves by itself, so the board asks
	 * for SW1, and the session stays what it was while it waits: it may tell the time and nothing
	 * else. With SW1 the time is taken, the certificate is valid at it, and the session goes on
	 * as any. */
	assert(!connect_as(phone_key, issued, expires) && clock_only);
	assert(!tell_time(real) && told().result == NU54_CLOCK_BUTTON && clock_only && !link.closed);
	assert(!button(true) && !clock_only && device_install_earliest() == real);
	assert(!status() && managed == 1 && !pay() && approval.pending);
	/* A board that is late by less than a day needs no button for the same. */
	late_board(20 * 3600);
	assert(!connect_as(phone_key, issued, expires) && clock_only);
	assert(!tell_time(real) && told().result == NU54_CLOCK_MOVED && !clock_only);
	managed = 0;
	assert(!status() && managed == 1 && !pay() && approval.pending);

	/* A time that does not reach the certificate's start: the clock moves, as it is forward and
	 * the owner's, and the session ends after the answer without having done anything else. */
	late_board(3 * DAY);
	real = issued - 600; /* this phone's clock is behind its own certificate */
	assert(!connect_as(phone_key, issued, expires) && clock_only);
	assert(!tell_time(real) && told().result == NU54_CLOCK_BUTTON);
	assert(button(true) == 1 && link.closed); /* the clock moved, and the session ends with it */
	assert(device_install_earliest() == issued - 600);
	/* The board's time and "not forward": answered, and the early session still ends. */
	assert(!connect_as(phone_key, issued, expires) && clock_only);
	assert(tell_time(real) == -1 && told().result == NU54_CLOCK_NOT_FORWARD && link.closed);
	managed = 0;
	assert(!connect_as(phone_key, issued, expires) && clock_only && status() == -1 && !managed);
	real = calendar;

	/* A certificate that has ended by the board's clock is refused as before, also the owner's. */
	late_board(3 * DAY);
	assert(connect_as(phone_key, 1, device_install_now()) == -1);
	assert(!connect_as(phone_key, 1, device_install_now() + 1) && !clock_only);
	/* One that ended by the calendar while the board's clock stood still: the session opens by
	 * the board's clock, the phone tells the time, and the session ends with the answer. */
	assert(!connect_as(phone_key, 1, calendar - DAY) && !clock_only);
	assert(!tell_time(real) && told().result == NU54_CLOCK_BUTTON);
	assert(button(true) == 1 && link.closed && device_install_earliest() == real);
	assert(connect_as(phone_key, 1, calendar - DAY) == -1);
	/* Without the button: the board is 20 hours late and the certificate ended an hour ago. */
	late_board(20 * 3600);
	assert(!connect_as(phone_key, 1, calendar - 3600) && !clock_only);
	assert(tell_time(real) == -1 && told().result == NU54_CLOCK_MOVED && link.closed);
	assert(connect_as(phone_key, 1, calendar - 3600) == -1);

	/* Early or not, the key must be the owner's. */
	late_board(3 * DAY);
	assert(connect_as(other_key, issued, expires) == -1);
	/* The payment route never takes a certificate that is not valid yet. */
	profile = NU54_V2_PAYMENT;
	assert(!connect_as(policy.terminal, 1, UINT64_MAX));
	assert(connect_as(policy.terminal, issued, expires) == -1);
	profile = NU54_V2_MANAGEMENT;
}

/* A moved time is in storage before the board uses it. */
static void test_power_loss(void)
{
	for (int after = 0; after < 2; ++after) {
		late_board(3 * 3600);
		const uint64_t before = device_install_earliest();
		assert(!connect_phone());
		power_after = after;
		writes_left = 0;
		volatile bool finished = false;
		if (!setjmp(power)) {
			(void)tell_time(real);
			finished = true;
		}
		writes_left = -1;
		assert(!finished);
		/* The old time or the new one, and the grant is there either way. */
		assert(board_restart() == 0 && device_install_earliest() == (after ? real : before));
		assert(!connect_phone() && !tell_time(real));
		assert(told().result == (after ? NU54_CLOCK_NOT_FORWARD : NU54_CLOCK_MOVED) && device_install_earliest() == real);
	}
	/* One write per move, and none for a time that moves nothing. */
	late_board(3 * 3600);
	assert(!connect_phone());
	writes = 0;
	assert(!tell_time(real) && writes == 1);
	assert(!tell(real, real + 90) && told().result == NU54_CLOCK_NOT_FORWARD && writes == 1);
	real = board_end + 1;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_PAST_END && writes == 1);
}

/* The saved time cannot be read: the grant goes, the board starts and takes a new grant. */
static void test_unreadable_saved_time(void)
{
	uint8_t record[GRANT_SIZE];
	for (int owner = 0; owner < 2; ++owner) {
		wipe(owner ? 1 : 3, owner ? 0 : NU54_VAULT_REVOKED | NU54_VAULT_RELEASED);
		if (owner) memcpy(vault.record.owner, phone_key, 32);
		const uint64_t generation = owner ? 1 : 4;
		real = calendar - DAY;
		make_grant(record, 1, generation, real, real + 31 * DAY);
		assert(!install(record));
		on_for(1200);
		assert(restart() == 0);
		item(INSTALL_CLOCK)->size = 16; /* a record of another size, as an older build wrote it */
		assert(!clock_load() && !clock_saved.earliest);
		assert(restart() == 1 && !device_install_policy() && !device_install_earliest());
		assert(!item(INSTALL_GRANT) && !item(INSTALL_CLOCK) && item(INSTALL_FLOOR));
		for (size_t part = 0; part < GRANT_PARTS; ++part) assert(!item(INSTALL_GRANT_PART + part));
		/* The board had reached `real`. Without the saved time its floor is the time of the
		 * grant it committed, 1200 seconds earlier: nothing before that is taken, that is. */
		make_grant(record, 2, generation, real - 1201, real + 20 * DAY);
		assert(install(record));
		make_grant(record, 2, generation, real - 1200, real + 20 * DAY);
		assert(!install(record) && device_install_policy() && device_install_earliest() == real - 1200);
		assert(restart() == 0);
	}
}

/* The accepted behaviour: a move into a later day opens that day's total, once per move, and the
 * board counts how often the phone did that under the grant. Since the decision of 2026-11-27 the
 * phone alone moves the clock by a day per start at the most, so it opens one later day by itself;
 * every further one takes SW1 on the board, or a restart. */
static void test_a_later_day_opens(void)
{
	uint8_t record[GRANT_SIZE];
	const uint64_t cap = CONFIG_NU54_SPENDING_DAILY_CAP, midnight = (calendar / DAY + 1) * DAY;
	assert(midnight - calendar > 100 && midnight - calendar < DAY - 100); /* what the cases below count on */
	late_board(0);
	assert(!device_order_erase());
	assert(!connect_phone() && !tell_time(real) && told().days == 0);
	/* The whole day's total is spent. */
	assert(!device_order_charge(device_install_earliest(), cap) && !device_order_daily_left(device_install_earliest()));
	assert(!connect_phone() && pay() == -1 && !approval.pending);
	/* Later the same day, a step before midnight: still spent. */
	real = midnight - NU54_CLOCK_STEP_SECONDS;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED && told().days == 0);
	assert(!device_order_daily_left(device_install_earliest()));
	/* Into the next day: the total is open again, and the count says the phone opened it. */
	real = midnight;
	assert(!tell_time(real) && told().result == NU54_CLOCK_MOVED && told().days == 1);
	assert(device_order_daily_left(device_install_earliest()) == cap);
	assert(!device_order_charge(device_install_earliest(), cap));
	/* That was the phone's day. The day after it is out of its reach in this start: the board
	 * asks for SW1, nothing has moved, and the total stays spent. */
	real = midnight + DAY;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_BUTTON && told().days == 1);
	assert(device_install_earliest() == midnight && !device_order_daily_left(device_install_earliest()));
	/* However the phone cuts the way into pieces: an hour at a time, each in a session of its own. */
	unsigned taken = 0;
	for (uint64_t at = midnight + 3600; at <= midnight + DAY; at += 3600) {
		real = at;
		assert(!connect_phone() && !tell_time(at));
		assert(told().result == NU54_CLOCK_MOVED || told().result == NU54_CLOCK_BUTTON);
		taken += told().result == NU54_CLOCK_MOVED;
	}
	assert(taken && told().result == NU54_CLOCK_BUTTON && told().days == 1);
	assert(device_install_earliest() < midnight + DAY && device_install_earliest() / DAY == midnight / DAY);
	assert(!device_order_daily_left(device_install_earliest()));
	/* A restart takes a hand on the board as the button does, and the count of what the phone
	 * moved begins anew: one more day, to the second. */
	const uint64_t reached = device_install_earliest();
	assert(board_restart() == 0 && device_install_earliest() == reached);
	real = reached + FREE;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED && told().days == 2);
	assert(device_order_daily_left(device_install_earliest()) == cap && !device_order_charge(device_install_earliest(), cap));
	/* Three days at once take SW1, and open one total, not three. */
	real = (real / DAY + 1) * DAY + 3 * DAY + 5;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_BUTTON && told().days == 2);
	assert(!device_order_daily_left(device_install_earliest()));
	assert(!button(true) && device_install_earliest() == real && clock_saved.phone_days == 3);
	assert(device_order_daily_left(device_install_earliest()) == cap && !device_order_charge(device_install_earliest(), cap));
	assert(device_order_charge(device_install_earliest(), 1));
	/* The count is kept with the time: through the periodic save and a restart. */
	on_for(1300);
	assert(board_restart() == 0 && clock_saved.phone_days == 3);
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED && told().days == 3);
	/* A day the board reached by running opens the total without the count. */
	on_for(DAY);
	assert(device_order_daily_left(device_install_earliest()) == cap);
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_NOT_FORWARD && told().days == 3);
	/* The period bounds it: every move goes forward and stops before the grant's end, so a later
	 * day opens at most once per day the grant still has. Of all those days the phone opens one
	 * by itself in this start. The others take SW1 each. */
	unsigned moves = 0, by_itself = 0;
	for (uint64_t day = real / DAY + 1; day * DAY < board_end && device_install_policy(); ++day) {
		real = day * DAY;
		assert(!connect_phone() && !tell_time(real));
		if (told().result == NU54_CLOCK_MOVED) ++by_itself;
		else assert(told().result == NU54_CLOCK_BUTTON && !button(true));
		assert(device_install_earliest() == real);
		++moves;
	}
	assert(moves > 15 && moves <= 31 && by_itself == 1 && clock_saved.phone_days == 3 + moves);
	/* A new grant starts the count of the days again. */
	late_board(0);
	real = midnight;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED && told().days == 1);
	make_grant(record, 2, 1, real + 10, real + 10 * DAY);
	assert(!install(record) && !clock_saved.phone_days);
	grant_names_the_store();
	real += 20;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED && told().days == 0);
}

/* What the rule costs when the phone's clock is ahead of the calendar, by mistake or on purpose.
 * It follows from "forward only" and is recorded as it is. Since the decision of 2026-11-27 the
 * phone alone takes the board a day ahead per start at the most; further takes SW1. */
static void test_a_phone_that_is_ahead(void)
{
	uint8_t record[GRANT_SIZE];

	/* A phone that is five days ahead, and nobody at the board: the board asks for SW1, and until
	 * then nothing has moved. A phone and a store whose clocks are right go on as before. */
	late_board(0);
	real = calendar + 5 * DAY;
	writes = 0;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_BUTTON && !writes);
	assert(device_install_earliest() == calendar && device_install_now() == calendar + 2);
	real = calendar;
	assert(!connect_phone() && !status() && !pay() && approval.pending);
	/* A day ahead is what such a phone still does by itself, in one message or in many, and a
	 * board that stays on does not come back from it: nothing moves a clock back. */
	late_board(0);
	real = calendar + FREE;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED && device_install_earliest() == real);
	real = calendar;
	assert(!connect_phone() && pay() == -1 && !approval.pending);
	assert(!connect_phone() && status() == -1);
	on_for(3600);
	assert(device_install_earliest() == real + FREE && !connect_phone() && status() == -1);
	/* It comes back when the calendar has caught up: here after a day without power. */
	assert(board_restart() == 0);
	real += FREE;
	assert(!connect_phone() && !status() && !tell_time(real) && told().result == NU54_CLOCK_NOT_FORWARD);

	/* With SW1, by whoever has the phone and stands at the board, five days as before. */
	for (int fenced = 0; fenced < 2; ++fenced) {
		late_board(3 * 3600);
		real = calendar + 5 * DAY;
		assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_BUTTON);
		assert(!button(true));
		const uint64_t reached = device_install_earliest();
		assert(reached == calendar + 5 * DAY && device_install_policy());
		/* A phone and a store whose clocks are right: their requests have ended for the board. */
		real = calendar;
		assert(!connect_phone() && pay() == -1 && !approval.pending);
		assert(!connect_phone() && status() == -1);
		/* Nothing moves the board back, the right time included. */
		assert(!connect_phone() && tell_time(real) == -1 && device_install_earliest() == reached);
		/* The operator's new grant carries the calendar's time and is refused. One that carries
		 * the time the board has reached would be taken. */
		make_grant(record, 2, 1, calendar + 100, calendar + 20 * DAY);
		assert(install(record) && device_install_earliest() == reached);
		if (fenced) assert(!device_install_fence_wallet_expiry(reached - 10)); /* owner status with an expiry (op 7) */
		/* A factory reset removes the saved time, and with it this floor. A fenced expiry stays
		 * through the reset, so then the board waits for the calendar: at most to the end of the
		 * grant the phone moved the clock under. */
		assert(!device_install_forget());
		assert(restart() == 1 && !device_install_policy());
		assert(install(record) == (fenced ? -1 : 0));
		if (fenced) {
			make_grant(record, 2, 1, reached - 11, reached + 20 * DAY);
			assert(install(record));
			make_grant(record, 2, 1, reached - 10, reached + 20 * DAY);
			assert(!install(record));
		}
	}
}

/* A limit that stays (the design document's table of what can be put back): the whole storage put
 * back to an earlier state revives the grant of that state without the operator, also after a
 * newer grant replaced it or after it ended by the board's clock. Writing the storage takes the
 * debug port. This only records what the board does then. */
static void test_whole_storage_put_back(void)
{
	uint8_t first[GRANT_SIZE], second[GRANT_SIZE];
	static struct item earlier[64];
	wipe(1, 0);
	make_grant(first, 1, 1, ISSUED, ISSUED + 2 * DAY);
	make_grant(second, 2, 1, ISSUED + DAY, ISSUED + 3 * DAY);
	assert(!install(first));
	run(3600);
	memcpy(earlier, flash, sizeof(flash));
	assert(!install(second));
	run(DAY + 3600);
	assert(restart() == 0 && policy.revision == 2);
	memcpy(flash, earlier, sizeof(flash));
	assert(restart() == 0 && policy.revision == 1 && device_install_earliest() == ISSUED + 3600);
	run(2 * DAY); /* the first grant ends by the board's clock, and the end is saved */
	assert(!device_install_policy() && restart() == 1);
	memcpy(flash, earlier, sizeof(flash));
	assert(restart() == 0 && policy.revision == 1 && device_install_policy());
}

/* An approved order holds a reset and a return back while it could execute. The phone's time must
 * not end that early, and a clock that is late must not stretch it. */
static void test_open_orders(void)
{
	nu54_order_record record = {.outcome = NU54_ORDER_APPROVED};

	/* Signed in this start: the board's time ran with it. Then the phone moves the clock past
	 * the order's end. The order stays open for the longest an order runs, in running time. */
	late_board(3 * 3600);
	assert(!device_order_erase());
	record.expires = device_install_earliest() + 100;
	assert(!device_order_store(1, &record) && device_order_open(device_install_earliest(), uptime));
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED);
	assert(device_install_earliest() > record.expires && device_order_open(device_install_earliest(), uptime));
	uptime += NU54_ORDER_MAX_SECONDS * 1000 - 1;
	assert(device_order_open(device_install_earliest(), uptime));
	uptime += 1;
	assert(!device_order_open(device_install_earliest(), uptime));
	/* A time that moves nothing holds nothing. */
	real = device_install_earliest();
	record.expires = real + 100;
	assert(!device_order_store(2, &record));
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_NOT_FORWARD);
	uptime += 100 * 1000 - 1;
	assert(device_order_open(device_install_earliest(), uptime));
	uptime += 1;
	assert(!device_order_open(device_install_earliest(), uptime));

	/* Signed before a restart, under a grant that was kept. The board lost ten minutes at the
	 * restart, so by its clock the order still has 700 seconds: it is open, but only for one
	 * order life of running time. */
	late_board(3 * 3600);
	assert(!device_order_erase());
	on_for(600);
	on_for(599);
	record.expires = device_install_earliest() + 101;
	assert(!device_order_store(1, &record));
	for (int i = 0; i < 3; ++i) { /* also when the board restarts again and again */
		assert(board_restart() == 0 && record.expires - device_install_earliest() == 700);
		assert(device_order_open(device_install_earliest(), uptime));
		uptime = NU54_SPEND_ORDER_LIFE_MS - 1;
		assert(device_order_open(device_install_earliest(), uptime));
	}
	uptime = NU54_SPEND_ORDER_LIFE_MS;
	assert(record.expires > device_install_earliest() && !device_order_open(device_install_earliest(), uptime));
	/* The phone's time does not end it before that either. */
	assert(board_restart() == 0);
	uptime = 20000;
	real += 4 * 3600;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED);
	assert(device_install_earliest() > record.expires);
	uptime = NU54_SPEND_ORDER_LIFE_MS - 1;
	assert(device_order_open(device_install_earliest(), uptime));
	uptime = NU54_SPEND_ORDER_LIFE_MS;
	assert(!device_order_open(device_install_earliest(), uptime));
}

/* The independent audit's first finding: a moved clock and a restart together must not close an
 * approved order before the token contract would. The order was approved with 117 seconds to run.
 * Two seconds later a phone whose clock is 200 seconds ahead tells the time, and the power goes. */
static void test_open_orders_through_restarts(void)
{
	nu54_order_record record = {.outcome = NU54_ORDER_APPROVED};

	late_board(3 * 3600);
	assert(!device_order_erase());
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED);
	record.expires = device_install_earliest() + 117;
	assert(!device_order_store(1, &record) && device_order_open(device_install_earliest(), uptime));
	on_for(2);
	real += 200;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED);
	assert(device_install_earliest() > record.expires && device_order_open(device_install_earliest(), uptime));
	real -= 200; /* the calendar: the order has 115 seconds left */
	const struct item moved = *item(INSTALL_CLOCK);
	for (int i = 0; i < 3; ++i) { /* however often the board restarts */
		assert(board_restart() == 0 && device_install_earliest() > record.expires);
		const int64_t started = uptime;
		assert(device_order_open(device_install_earliest(), uptime));
		uptime = 115 * 1000;
		assert(device_order_open(device_install_earliest(), uptime));
		/* It ends with the running time an order can have at the most. */
		uptime = started + NU54_ORDER_MAX_SECONDS * 1000 - 1;
		assert(device_order_open(device_install_earliest(), uptime));
		uptime += 1;
		assert(!device_order_open(device_install_earliest(), uptime));
		assert(!memcmp(item(INSTALL_CLOCK), &moved, sizeof(moved))); /* nothing was saved since the move */
	}
	/* The hold runs out with the board's time, also in storage: once the board has saved a time
	 * past it, a restart holds nothing more. */
	on_for(600);
	assert(board_restart() == 0 && !device_install_order_hold() && !device_order_open(device_install_earliest(), uptime));

	/* The same when the order is from an earlier start: restart, the phone's time, restart. */
	late_board(3 * 3600);
	assert(!device_order_erase());
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED);
	record.expires = device_install_earliest() + 117;
	assert(!device_order_store(1, &record));
	assert(board_restart() == 0);
	on_for(2);
	assert(device_order_open(device_install_earliest(), uptime));
	real += 200;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED);
	assert(device_install_earliest() > record.expires && device_order_open(device_install_earliest(), uptime));
	/* The second start had run for some seconds of its order life when the clock moved. The third
	 * holds the order for what was left of that life. */
	const int64_t left = NU54_SPEND_ORDER_LIFE_MS - uptime;
	assert(left % 1000 == 0 && left > NU54_ORDER_MAX_SECONDS * 1000);
	assert(board_restart() == 0 && device_install_order_hold() == (uint64_t)left / 1000);
	const int64_t started = uptime;
	assert(device_order_open(device_install_earliest(), uptime));
	uptime = started + left - 1;
	assert(device_order_open(device_install_earliest(), uptime));
	uptime += 1;
	assert(!device_order_open(device_install_earliest(), uptime));

	/* The move ended the grant by the board's time (its latest possible time passed the end), so
	 * the restart finds no grant to take up. The order is held all the same. */
	late_board(3 * 3600);
	assert(!device_order_erase());
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED);
	record.expires = device_install_earliest() + 117;
	assert(!device_order_store(1, &record));
	real = board_end - 2;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_BUTTON); /* weeks ahead: SW1 */
	assert(!button(true) && !device_install_policy());
	assert(board_restart() == 1 && device_install_order_hold() == NU54_ORDER_MAX_SECONDS);
	assert(device_order_open(device_install_earliest(), uptime));
	/* A hold is never longer than one order life of running time, whatever storage says. */
	nu54_install_clock stored;
	memcpy(&stored, item(INSTALL_CLOCK)->data, sizeof(stored));
	stored.hold += 40 * DAY;
	memcpy(item(INSTALL_CLOCK)->data, &stored, sizeof(stored));
	assert(board_restart() == 1 && device_install_order_hold() > 40 * DAY);
	assert(!device_order_erase()); /* no record: only the hold speaks */
	uptime = NU54_SPEND_ORDER_LIFE_MS - 1;
	assert(device_order_open(device_install_earliest(), uptime));
	uptime = NU54_SPEND_ORDER_LIFE_MS;
	assert(!device_order_open(device_install_earliest(), uptime));

	/* The power goes while the moved time is written. The hold is in the same write: the board
	 * comes back with the old time, by which the order is open, or with the new time and the hold. */
	for (int after = 0; after < 2; ++after) {
		late_board(3 * 3600);
		assert(!device_order_erase());
		assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED);
		record.expires = device_install_earliest() + 117;
		assert(!device_order_store(1, &record));
		real += 200;
		assert(!connect_phone());
		power_after = after;
		writes_left = 0;
		volatile bool finished = false;
		if (!setjmp(power)) {
			(void)tell_time(real);
			finished = true;
		}
		writes_left = -1;
		assert(!finished && board_restart() == 0);
		assert((device_install_earliest() > record.expires) == after);
		assert(device_install_order_hold() == (after ? NU54_ORDER_MAX_SECONDS : 0));
		assert(device_order_open(device_install_earliest(), uptime));
	}

	/* A new grant brings the authority's time, which is not ahead of the calendar: the records are
	 * judged by it again, and it leaves no hold in storage. */
	uint8_t grant[GRANT_SIZE];
	late_board(3 * 3600);
	assert(!device_order_erase());
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED);
	record.expires = device_install_earliest() + 117;
	assert(!device_order_store(1, &record));
	real += 200;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED && device_install_order_hold());
	make_grant(grant, 2, 1, real + 1, real + 10 * DAY);
	assert(!install(grant) && !device_install_order_hold());
	assert(device_order_open(device_install_earliest(), uptime)); /* this start still holds it */
	assert(board_restart() == 0 && !device_order_open(device_install_earliest(), uptime));
}

/* The audit's third finding: eight messages, each a second ahead of the one before, cost eight
 * writes. A difference below the step moves nothing and writes nothing. */
static void test_small_steps_write_nothing(void)
{
	late_board(3 * 3600);
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED);
	const uint64_t set = device_install_earliest();
	writes = 0;
	unsigned moves = 0;
	for (int i = 1; i <= 8; ++i) {
		const uint64_t before = device_install_earliest();
		assert(!connect_phone() && !tell_time(set + (uint64_t)i));
		const bool moved = told().result == NU54_CLOCK_MOVED;
		assert(moved || told().result == NU54_CLOCK_NOT_FORWARD);
		/* A message that is taken moves the clock by a whole step at the least. */
		assert(device_install_earliest() == (moved ? set + (uint64_t)i : before));
		assert(!moved || device_install_earliest() - before >= NU54_CLOCK_STEP_SECONDS);
		assert(told().earliest == device_install_earliest());
		moves += moved;
	}
	assert(moves == 8 / NU54_CLOCK_STEP_SECONDS && writes == moves);
	/* The time the phone told stays inside the board's interval when nothing moves. */
	assert(device_install_now() >= set + 8);

	/* A step exactly, and one second less. */
	late_board(3 * 3600);
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED);
	writes = 0;
	assert(!tell_time(real + NU54_CLOCK_STEP_SECONDS - 1) && told().result == NU54_CLOCK_NOT_FORWARD && !writes);
	assert(told().earliest == real && device_install_earliest() == real);
	assert(!tell_time(real + NU54_CLOCK_STEP_SECONDS) && told().result == NU54_CLOCK_MOVED && writes == 1);
	assert(device_install_earliest() == real + NU54_CLOCK_STEP_SECONDS);
}

/* The audit's second finding: the latest possible time went up with a message that came late in
 * its session and never came down again, so the board refused every request that had less than
 * that long to run. */
static void test_the_latest_time_comes_down(void)
{
	uint8_t record[GRANT_SIZE];
	const uint64_t limit = CONFIG_NU54_CLOCK_SESSION_SECONDS;

	/* A message at the very end of its session is answered and moves nothing: taken, it would
	 * have put the latest possible time 124 seconds above the lower bound. */
	late_board(3 * 3600);
	const uint64_t before = device_install_earliest(), latest = device_install_now();
	assert(!connect_phone());
	uptime += 119 * 1000;
	real += 119;
	writes = 0;
	clock_turns = 0;
	assert(!tell_time(real) && told().result == NU54_CLOCK_LATE && !writes && !clock_turns);
	assert(told().earliest == before + 119 && told().latest == latest + 119 && told().end == board_end);
	assert(device_install_earliest() == before + 119 && device_install_now() == latest + 119);
	/* The phone connects again and tells the time first. Its requests with 90 seconds to run, the
	 * next time command among them, are taken from then on: now, after an hour, after a restart. */
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED);
	assert(told().latest == real + NU54_CLOCK_PHONE_SECONDS && !status());
	on_for(3600);
	assert(!connect_phone() && !status() && !tell_time(real) && told().result == NU54_CLOCK_NOT_FORWARD);
	assert(board_restart() == 0 && device_install_now() - device_install_earliest() == NU54_CLOCK_PHONE_SECONDS);

	/* The limit exactly, and a second over it. The board counts whole seconds, rounded up. */
	late_board(3 * 3600);
	assert(!connect_phone());
	uptime += (int64_t)limit * 1000;
	real += limit;
	assert(!tell_time(real) && told().result == NU54_CLOCK_MOVED && told().latest == real + limit + NU54_CLOCK_PHONE_SECONDS);
	late_board(3 * 3600);
	assert(!connect_phone());
	uptime += (int64_t)limit * 1000 + 1;
	real += limit;
	assert(!tell_time(real) && told().result == NU54_CLOCK_LATE && device_install_earliest() == calendar - 3 * 3600 + limit);
	/* The widest interval the phone can leave is narrower than the life of its short requests. */
	assert(limit + NU54_CLOCK_PHONE_SECONDS < 90);
	/* A session that may only tell the time, and tells it too late, ends with the answer. */
	late_board(3 * DAY);
	assert(!connect_as(phone_key, calendar - 3600, UINT64_MAX) && clock_only);
	uptime += (int64_t)(limit + 1) * 1000;
	assert(tell_time(real) == -1 && told().result == NU54_CLOCK_LATE && link.closed);
	assert(device_install_earliest() == calendar - 3 * DAY + limit + 1);

	/* A grant that took 58 seconds to arrive leaves the board's interval that wide, and the board
	 * keeps the grant through restarts. The store's order has 50 seconds left; by the board's
	 * latest possible time it has ended. */
	const uint64_t granted = quote_end - 55;
	wipe(1, 0);
	memcpy(vault.record.owner, phone_key, 32);
	end_session();
	device_order_start();
	make_grant(record, 1, 1, granted, granted + 31 * DAY);
	assert(!operator_installs(record, 58000) && ready);
	grant_names_the_store();
	assert(device_install_earliest() == granted && device_install_now() == granted + 58);
	real = granted + 5; /* where in those 58 seconds the calendar is */
	assert(quote_end - real == 50 && device_install_now() >= quote_end);
	assert(!connect_phone() && pay() == -1 && !approval.pending);
	assert(board_restart() == 0 && device_install_now() == granted + 58);
	assert(!connect_phone() && pay() == -1 && !approval.pending);
	/* The owner's phone tells the time, and the interval is the phone's: five seconds wide. The
	 * same session goes on with a time that came down, and the order is taken. */
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED);
	assert(told().earliest == real && told().latest == real + NU54_CLOCK_PHONE_SECONDS);
	assert(!status() && !pay() && approval.pending);
	assert(board_restart() == 0 && device_install_now() - device_install_earliest() == NU54_CLOCK_PHONE_SECONDS);

	/* The same where the phone's time is no step ahead of the board's lower bound: the lower bound
	 * stays, the latest possible time comes down, and that is one write. */
	wipe(1, 0);
	memcpy(vault.record.owner, phone_key, 32);
	end_session();
	device_order_start();
	make_grant(record, 1, 1, quote_end - 51, quote_end - 51 + 31 * DAY);
	assert(!operator_installs(record, 58000));
	grant_names_the_store();
	real = quote_end - 50;
	const uint64_t lower = device_install_earliest();
	assert(!connect_phone() && pay() == -1 && !approval.pending);
	assert(!connect_phone());
	writes = 0;
	clock_turns = 0;
	assert(!tell_time(real) && told().result == NU54_CLOCK_MOVED && writes == 1 && clock_turns == 1);
	assert(told().earliest == lower && told().latest == real + NU54_CLOCK_PHONE_SECONDS);
	assert(device_install_earliest() == lower && device_install_now() == real + NU54_CLOCK_PHONE_SECONDS);
	/* Nothing moved forward, so nothing was held open, and no later day was counted. */
	assert(!device_install_order_hold() && !told().days);
	assert(!pay() && approval.pending);
	/* A second message finds nothing more to bring down. */
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_NOT_FORWARD && writes == 1);
	assert(board_restart() == 0 && device_install_earliest() == lower && device_install_now() == real + NU54_CLOCK_PHONE_SECONDS);

	/* A fresh grant sets both ends, also in a start in which a session saw the wide interval:
	 * the session after it is taken, with a time that is earlier than the one before. */
	wipe(1, 0);
	memcpy(vault.record.owner, phone_key, 32);
	end_session();
	make_grant(record, 1, 1, granted, granted + 31 * DAY);
	assert(!operator_installs(record, 58000));
	grant_names_the_store();
	real = granted + 5;
	assert(!connect_phone() && !status());
	const uint64_t seen = journal.trusted_time;
	assert(seen == granted + 58);
	make_grant(record, 2, 1, real - 2, granted + 31 * DAY);
	assert(!operator_installs(record, 2000) && device_install_earliest() == real - 2 && device_install_now() == real);
	grant_names_the_store();
	assert(device_install_now() < seen);
	assert(!connect_phone() && !status() && !pay() && approval.pending);
	/* A grant that is refused changes nothing of this. */
	const uint64_t kept = journal.trusted_time;
	make_grant(record, 3, 1, real - 100, granted + 31 * DAY);
	assert(operator_installs(record, 2000) && journal.trusted_time == kept && device_install_now() == real);
}

/* The decision of 2026-11-27 on how far the phone alone moves the clock: by a day (the build's
 * CONFIG_NU54_CLOCK_FREE_SECONDS) since the board's last start, all its messages together. A move
 * beyond that takes SW1 on the board. */
static void test_the_phone_alone_moves_a_day(void)
{
	const uint64_t now = UINT64_C(1800000000), end = now + 31 * DAY;
	const nu54_install_clock clock = {.earliest = now, .offset = 2}, wide = {.earliest = now, .offset = 60};
	nu54_install_clock next = {0};

	/* ---- the rule ---- */
	/* The allowance exactly, and a second over it. */
	assert(rule_after(&clock, end, now + FREE, 0, false, &next) == NU54_CLOCK_MOVED && next.earliest == now + FREE);
	next = (nu54_install_clock){0};
	assert(rule_after(&clock, end, now + FREE + 1, 0, false, &next) == NU54_CLOCK_BUTTON && !next.earliest);
	/* What the phone moved before in this start counts. */
	assert(rule_after(&clock, end, now + 3600, FREE - 3600, false, &next) == NU54_CLOCK_MOVED);
	next = (nu54_install_clock){0};
	assert(rule_after(&clock, end, now + 3601, FREE - 3600, false, &next) == NU54_CLOCK_BUTTON);
	assert(rule_after(&clock, end, now + NU54_CLOCK_STEP_SECONDS, FREE, false, &next) == NU54_CLOCK_BUTTON);
	assert(rule_after(&clock, end, now + NU54_CLOCK_STEP_SECONDS, FREE + 1, false, &next) == NU54_CLOCK_BUTTON);
	assert(rule_after(&clock, end, now + NU54_CLOCK_STEP_SECONDS, UINT64_MAX, false, &next) == NU54_CLOCK_BUTTON);
	assert(!next.earliest);
	/* With SW1 the same moves are taken, up to the grant's end and never to it. */
	assert(rule_after(&clock, end, now + FREE + 1, 0, true, &next) == NU54_CLOCK_MOVED && next.earliest == now + FREE + 1);
	assert(rule_after(&clock, end, end - 1, FREE, true, &next) == NU54_CLOCK_MOVED && next.earliest == end - 1);
	assert(rule_after(&clock, end, end, FREE, true, &next) == NU54_CLOCK_PAST_END);
	/* A time at the grant's end or past it is answered as that, and no button is asked for. */
	assert(rule_after(&clock, end, end + DAY, 0, false, &next) == NU54_CLOCK_PAST_END);
	/* What does not move the lower bound takes no button, however much the phone moved before. */
	assert(rule_after(&wide, end, now, FREE, false, &next) == NU54_CLOCK_MOVED);
	assert(next.earliest == now && next.offset == NU54_CLOCK_PHONE_SECONDS);
	assert(rule_after(&clock, end, now + 1, FREE, false, &next) == NU54_CLOCK_NOT_FORWARD);
	assert(rule_after(&clock, end, now - 60, FREE, false, &next) == NU54_CLOCK_NOT_FORWARD);
	/* An allowance of zero asks for SW1 for every move. */
	const nu54_install_clock_limits none = {.session_seconds = CONFIG_NU54_CLOCK_SESSION_SECONDS, .free_seconds = 0};
	nu54_install_clock_word word = {.time = now + NU54_CLOCK_STEP_SECONDS, .expires = now + 90};
	assert(nu54_install_clock_set(&clock, end, &word, &none, &next) == NU54_CLOCK_BUTTON);
	word.button = true;
	assert(nu54_install_clock_set(&clock, end, &word, &none, &next) == NU54_CLOCK_MOVED);

	/* ---- on the board ---- */
	/* Off for a day to the second: the phone alone, and nothing is asked of the owner. */
	late_board(FREE);
	prompts = lamp_waits = 0;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED && device_install_earliest() == real);
	assert(!prompts && !lamp_waits && !pending_prompt && !clock_wait.pending && clock_moved == FREE);
	/* That was the allowance of this start: the next step asks for SW1. */
	assert(!connect_phone() && !tell_time(real + NU54_CLOCK_STEP_SECONDS) && told().result == NU54_CLOCK_BUTTON);

	/* Off for a second longer. The answer says "press the button" and carries the board's time as
	 * it is. Nothing is written and nothing has moved, and the board waits with the prompt and the
	 * lamp of every other wait. */
	late_board(FREE + 1);
	const uint64_t before = device_install_earliest();
	const struct item saved = *item(INSTALL_CLOCK);
	assert(!connect_phone());
	uptime += 4000; /* the handshake and the phone took four seconds */
	real += 4;
	prompts = lamp_waits = writes = clock_turns = 0;
	assert(!tell_time(real) && told().result == NU54_CLOCK_BUTTON);
	assert(told().earliest == before + 4 && told().latest == before + 4 + 2 && told().end == board_end && !told().days);
	assert(!writes && !clock_turns && !memcmp(item(INSTALL_CLOCK), &saved, sizeof(saved)) && !clock_moved);
	assert(prompts == 1 && lamp_waits == 1 && pending_prompt && shown_prompt == (uint32_t)pending_prompt);
	assert(((uint32_t)pending_prompt & NU54_PILOT_PROMPT_MASK) && clock_wait.pending && !link.closed);
	/* The owner takes twenty seconds to reach the board. SW1 takes the message as of its arrival:
	 * the board counted the seconds since, so the time is not the stale one the message carries,
	 * and the interval is as wide as on arrival, the session's four seconds and the phone's five. */
	uptime += 20400;
	real += 20;
	board_polls();
	assert(clock_wait.pending && !button(true) && !pending_prompt && !shown_prompt && !clock_wait.pending);
	assert(writes == 1 && clock_turns == 1);
	assert(device_install_earliest() == real && device_install_now() == real + 4 + NU54_CLOCK_PHONE_SECONDS);
	/* The session goes on and the order is taken. */
	assert(!status() && !pay() && approval.pending);
	/* A move with SW1 is not counted against the phone: it still has its day in this start. */
	assert(!clock_moved);
	real += FREE;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED && clock_moved == FREE);
	assert(!connect_phone() && !tell_time(real + NU54_CLOCK_STEP_SECONDS) && told().result == NU54_CLOCK_BUTTON);
	/* The time is in storage as every moved time. */
	assert(board_restart() == 0 && device_install_earliest() == real);

	/* SW2 drops the time: nothing has moved, and the session ends as after every refusal. */
	late_board(2 * DAY);
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_BUTTON);
	writes = 0;
	assert(button(false) == -1 && link.closed && !writes && !clock_wait.pending && !pending_prompt);
	assert(device_install_earliest() == calendar - 2 * DAY);

	/* The board waits for as long as the message has to run by its own word, 90 seconds from the
	 * time it carries. A moment before that SW1 still takes it. */
	late_board(2 * DAY);
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_BUTTON);
	uptime += 89999;
	real += 89;
	board_polls();
	assert(clock_wait.pending && !button(true) && device_install_earliest() == real);
	/* At its end the board drops it by itself and ends the session. */
	late_board(2 * DAY);
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_BUTTON);
	uptime += 90000;
	board_polls();
	assert(!clock_wait.pending && !pending_prompt && link.closed && device_install_earliest() == calendar - 2 * DAY + 90);
	/* A press that comes before the board looked is refused all the same. */
	late_board(2 * DAY);
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_BUTTON);
	uptime += 90000;
	writes = 0;
	assert(button(true) == -1 && link.closed && !writes && device_install_earliest() == calendar - 2 * DAY + 90);
	/* A message that runs for ten seconds waits for ten. */
	late_board(2 * DAY);
	assert(!connect_phone() && !tell(real, real + 10) && told().result == NU54_CLOCK_BUTTON);
	uptime += 9999;
	board_polls();
	assert(clock_wait.pending);
	uptime += 1;
	board_polls();
	assert(!clock_wait.pending && link.closed);

	/* While the board waits, every message ends the session and the wait with it, another time
	 * command included. The board is on time here, so it is not its clock that refuses them. */
	for (int kind = 0; kind < 3; ++kind) {
		late_board(FREE);
		assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED && !status());
		assert(!tell_time(real + 10) && told().result == NU54_CLOCK_BUTTON && clock_wait.pending);
		managed = 0;
		const int took = kind == 0 ? tell_time(real + 10) : kind == 1 ? status() : pay();
		assert(took == -1 && !answered && !managed && link.closed && !clock_wait.pending && !pending_prompt);
		assert(!approval.pending && device_install_earliest() == real);
	}

	/* When SW1 comes, the board must still be what it was when it took the message. */
	for (int state = 0; state < 7; ++state) {
		late_board(2 * DAY);
		assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_BUTTON);
		if (state == 0) vault.record.flags = NU54_VAULT_REVOKED;    /* the owner was revoked */
		else if (state == 1) vault.enabled = false;                 /* the vault stopped */
		else if (state == 2) policy.payer_generation = 2;           /* another generation's grant */
		else if (state == 3) recovering = true;                     /* a recovery */
		else if (state == 4) link.peer_identity[3] ^= 1;            /* not the owner's session */
		else if (state == 5) connection = NULL;                     /* the link is gone */
		else active = false;                                        /* the grant is gone */
		writes = 0;
		assert(clock_button(true) == -1 && !writes && !clock_wait.pending);
		end_session();
		vault.enabled = active = true;
		vault.record.flags = 0;
		policy.payer_generation = 1;
		assert(device_install_earliest() == calendar - 2 * DAY);
	}

	/* An order that is open when SW1 moves the clock is held open as with every move, and through
	 * a restart. */
	nu54_order_record record = {.outcome = NU54_ORDER_APPROVED};
	late_board(2 * DAY);
	assert(!device_order_erase());
	record.expires = device_install_earliest() + 117;
	assert(!device_order_store(1, &record));
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_BUTTON && !device_install_order_hold());
	uptime += 3000;
	real += 3;
	assert(!button(true) && device_install_earliest() > record.expires);
	assert(device_order_open(device_install_earliest(), uptime) && device_install_order_hold() == NU54_ORDER_MAX_SECONDS);
	assert(board_restart() == 0 && device_order_open(device_install_earliest(), uptime));

	/* A message that came too late in its session is answered as that, and no button is asked for. */
	late_board(2 * DAY);
	assert(!connect_phone());
	uptime += (CONFIG_NU54_CLOCK_SESSION_SECONDS + 1) * 1000;
	assert(!tell_time(real) && told().result == NU54_CLOCK_LATE && !clock_wait.pending && !pending_prompt);

	/* Many small moves add up to the allowance, and the one after them asks for SW1. A restart
	 * begins the count anew: it takes a hand on the board as the button does. */
	late_board(3 * 3600);
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED && clock_moved == 3 * 3600);
	unsigned messages = 1;
	while (clock_moved + 3600 <= FREE) {
		real += 3600;
		assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED);
		++messages;
	}
	assert(clock_moved == FREE && messages == 22);
	real += NU54_CLOCK_STEP_SECONDS;
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_BUTTON && clock_moved == FREE);
	assert(device_install_earliest() == real - NU54_CLOCK_STEP_SECONDS);
	assert(board_restart() == 0 && !clock_moved);
	assert(!connect_phone() && !tell_time(real) && told().result == NU54_CLOCK_MOVED && clock_moved == NU54_CLOCK_STEP_SECONDS);
}

/* The audit's fourth finding: a session that was opened while the board had no owner, in which the
 * owner then registered, took the time, moved the clock and ended, because it has no certificate
 * period to judge afterwards. On the board that session does not exist: the button code ends the
 * session the moment a registration completes (device_pilot_button, "a changed generation retires
 * the authenticated session"; test_spending_led.c). This puts the vault into that state by hand.
 * The time is not taken from such a session, and nothing moves before it ends. */
static void test_a_session_from_before_the_owner(void)
{
	uint8_t record[GRANT_SIZE];
	wipe(0, 0);
	end_session();
	real = calendar - 3 * 3600;
	make_grant(record, 1, 1, real, real + 31 * DAY);
	assert(!install(record));
	on_for(600);
	real = calendar;
	assert(board_restart() == 0);
	const uint64_t reached = device_install_earliest();
	assert(!connect_as(policy.owner, 1, UINT64_MAX) && !clock_only && !session_validity.until);
	vault.record.generation = 1; /* what a completed registration leaves in the vault */
	memcpy(vault.record.owner, policy.owner, 32);
	writes = 0;
	assert(tell_time(real) == -1 && !answered && !writes && device_install_earliest() == reached && link.closed);
	/* The owner's next session is bound as an owner's, and it tells the time. */
	assert(!connect_as(policy.owner, 1, UINT64_MAX) && session_validity.until == UINT64_MAX);
	assert(!tell_time(real) && told().result == NU54_CLOCK_MOVED && device_install_earliest() == real && !link.closed);
}

/* The saved time of the build before this one had no hold (24 bytes). It is not read as a time:
 * the grant goes with it, and an approved order counts as open for one order life of running time,
 * as it does on every board that does not know the time. */
static void test_saved_time_of_the_earlier_build(void)
{
	nu54_order_record record = {.outcome = NU54_ORDER_APPROVED};
	uint8_t grant[GRANT_SIZE];
	late_board(3 * 3600);
	assert(!device_order_erase());
	record.expires = device_install_earliest() + 117;
	assert(!device_order_store(1, &record));
	const uint64_t reached = device_install_earliest();
	item(INSTALL_CLOCK)->size = 24;
	assert(board_restart() == 1 && !device_install_policy() && !device_install_earliest());
	assert(!item(INSTALL_GRANT) && !item(INSTALL_CLOCK) && item(INSTALL_FLOOR) && !device_install_order_hold());
	assert(device_order_open(device_install_earliest(), uptime));
	uptime = NU54_SPEND_ORDER_LIFE_MS - 1;
	assert(device_order_open(device_install_earliest(), uptime));
	uptime = NU54_SPEND_ORDER_LIFE_MS;
	assert(!device_order_open(device_install_earliest(), uptime));
	/* The operator's next grant is taken, from the time of the grant the board had committed. */
	make_grant(grant, 2, 1, reached, reached + DAY);
	assert(!install(grant) && device_install_policy() && board_restart() == 0);
}

int main(int argc, char **argv)
{
	uint8_t unsigned_quote[NU54_V2_UNSIGNED_SIZE];
	assert(argc == 2);
	load_vector(argv[1], "unsigned", unsigned_quote, sizeof(unsigned_quote));
	load_vector(argv[1], "merchantSignature", quote + NU54_V2_UNSIGNED_SIZE, 65);
	load_vector(argv[1], "merchantSigner", merchant_signer, 20);
	load_vector(argv[1], "confirmer", confirmer, 20);
	memcpy(quote, unsigned_quote, sizeof(unsigned_quote));
	quote_issued = sys_get_be64(quote + NU54_V2_ISSUED_AT_OFFSET + 24);
	quote_end = sys_get_be64(quote + NU54_V2_EXPIRES_AT_OFFSET + 24);
	calendar = quote_issued + 3; /* the order reaches the board three seconds after the store signed it */
	assert(quote_end - calendar > 60 && quote_end - calendar <= 120);

	test_the_rule();
	test_orders_work_again();
	test_session_age();
	test_what_is_refused();
	test_who_may_tell();
	test_replay();
	test_certificate_issued_while_off();
	test_power_loss();
	test_unreadable_saved_time();
	test_a_later_day_opens();
	test_a_phone_that_is_ahead();
	test_whole_storage_put_back();
	test_open_orders();
	test_open_orders_through_restarts();
	test_small_steps_write_nothing();
	test_the_latest_time_comes_down();
	test_the_phone_alone_moves_a_day();
	test_a_session_from_before_the_owner();
	test_saved_time_of_the_earlier_build();
	puts("clock sync ok");
	return 0;
}
