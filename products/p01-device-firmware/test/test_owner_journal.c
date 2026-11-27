/* The owner session's request records and time bounds, with the real transport. The journal and its
 * per-session clearing are extracted from device_pilot.c at build time. */
#include "nu54_pilot.h"
#include "nu54_transport_v2.h"

#include <assert.h>
#include <string.h>

#define ARRAY_SIZE(array) (sizeof(array) / sizeof((array)[0]))
#define NOW UINT64_C(1800000000)
#define TTL 90        /* reads and other requests without a button */
#define ACTION_TTL 150 /* requests that wait for SW1 and the PIN */
#define UNSURE 60     /* OFFER_TTL_MS */

static int hash(const uint8_t *data, size_t length, uint8_t out[32])
{
	memset(out, 0, 32);
	for (size_t i = 0; i < length; ++i) out[i % 32] = (uint8_t)(out[i % 32] * 31 + data[i] + 1);
	return 0;
}

#include "board_journal.h"

static nu54_v2_journal journal = {journal_slots, ARRAY_SIZE(journal_slots), hash, 0};
static nu54_v2_journal_entry phone_slots[4];
static nu54_v2_journal phone_journal = {phone_slots, 4, hash, 0};
static nu54_v2_link board, phone;
static const uint8_t owner[32] = {1}, board_id[32] = {2};
static uint8_t frame[NU54_V2_FRAME_MAX];
static size_t frame_length;

static void session(uint8_t epoch)
{
	uint8_t bytes[16] = {epoch};
	assert(!nu54_v2_link_init(&board, NU54_V2_MANAGEMENT, NU54_V2_BOARD, bytes, owner, &journal));
	assert(!nu54_v2_link_init(&phone, NU54_V2_MANAGEMENT, NU54_V2_OWNER, bytes, board_id, &phone_journal));
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	journal_new_session();
#endif
}

static int keep(void *context, const uint8_t *data, size_t length)
{
	(void)context;
	assert(length <= sizeof(frame));
	memcpy(frame, data, length);
	frame_length = length;
	return 0;
}

/* One request of the phone and the board's answer to it. */
static nu54_v2_result request(uint32_t number, uint8_t body, uint64_t expires, uint64_t now)
{
	nu54_v2_request out, ask = {.command = NU54_V2_MANAGE, .expires = expires, .payload = &body, .payload_length = 1};
	ask.id[0] = 1;
	memcpy(ask.id + 1, &number, sizeof(number));
	assert(!nu54_v2_link_send(&phone, &ask, keep, NULL));
	nu54_v2_result result = nu54_v2_link_receive(&board, frame, frame_length, now, &out);
	if (result == NU54_V2_NEW || result == NU54_V2_RESUME) {
		uint8_t sent[NU54_V2_FRAME_MAX];
		size_t length = frame_length;
		memcpy(sent, frame, length);
		ask.command = NU54_V2_REPLY;
		assert(!nu54_v2_link_send(&board, &ask, keep, NULL));
		assert(nu54_v2_link_receive(&phone, frame, frame_length, now, &out) == NU54_V2_NEW);
		memcpy(frame, sent, length); /* the request, for a replay */
		frame_length = length;
	}
	return result;
}

/* Registration (14) and payments (6 each), one right after the other, then more than the slots hold. */
static void test_many_sessions(void)
{
	uint32_t number = 0;
	uint8_t epoch = 0;
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	const unsigned payments = 100;
#else
	const unsigned payments = 19;
#endif
	session(++epoch);
	for (int i = 0; i < 14; ++i) assert(request(++number, 1, NOW + ACTION_TTL, NOW) == NU54_V2_NEW);
	for (unsigned payment = 0; payment < payments; ++payment) {
		session(++epoch);
		for (int i = 0; i < 6; ++i) assert(request(++number, 1, NOW + ACTION_TTL, NOW) == NU54_V2_NEW);
	}
	assert(number > 4 * 32 - 4);
}

static void test_one_session(void)
{
	memset(journal_slots, 0, sizeof(journal_slots));
	session(1);
	for (uint32_t i = 1; i <= 100; ++i) assert(request(1000 + i, 1, NOW + TTL, NOW) == NU54_V2_NEW);
	assert(request(1001, 1, NOW + TTL, NOW) == NU54_V2_RESUME);
	assert(request(1001, 2, NOW + TTL, NOW) == NU54_V2_ERROR && board.closed); /* the same id for something else */
	memset(journal_slots, 0, sizeof(journal_slots));
	session(2);
	for (uint32_t i = 1; i <= 128; ++i) assert(request(2000 + i, 1, NOW + TTL, NOW) == NU54_V2_NEW);
	assert(request(2129, 1, NOW + TTL, NOW) == NU54_V2_ERROR && board.closed); /* never an overwrite */
}

static void test_replay(void)
{
	uint8_t old[NU54_V2_FRAME_MAX];
	nu54_v2_request out;
	memset(journal_slots, 0, sizeof(journal_slots));
	session(3);
	assert(request(3001, 1, NOW + TTL, NOW) == NU54_V2_NEW);
	const size_t length = frame_length;
	memcpy(old, frame, length);
	assert(nu54_v2_link_receive(&board, old, length, NOW, &out) == NU54_V2_ERROR && board.closed); /* same session */
	session(4);
	assert(nu54_v2_link_receive(&board, old, length, NOW, &out) == NU54_V2_ERROR && board.closed); /* the next one */
	session(5);
	memcpy(old, board.epoch, sizeof(board.epoch)); /* even re-addressed: the counter already moved */
	assert(request(3002, 1, NOW + TTL, NOW) == NU54_V2_NEW);
	assert(nu54_v2_link_receive(&board, old, length, NOW, &out) == NU54_V2_ERROR && board.closed);
	session(6); /* the watermark outlives the session: time never goes back */
	assert(request(3003, 1, NOW + TTL, NOW - 1) == NU54_V2_ERROR);
	session(7);
	assert(request(3004, 1, NOW, NOW) == NU54_V2_ERROR); /* expired */
	session(8); /* the peer's own retry of an earlier id */
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	assert(request(3001, 1, NOW + TTL, NOW) == NU54_V2_NEW);
#else
	assert(request(3001, 1, NOW + TTL, NOW) == NU54_V2_RESUME);
	assert(request(3001, 2, NOW + TTL, NOW) == NU54_V2_ERROR);
#endif
}

#if defined(CONFIG_NU54_SPENDING_PROFILE)
/* Real time lies between the board's bounds. Any grant delay, any place between them, and a phone
 * clock from 5 s behind to 5 s ahead: a request is live, and within the limit of its kind. */
static void test_time(void)
{
	for (uint64_t unsure = 0; unsure <= UNSURE; ++unsure)
		for (uint64_t lead = 0; lead <= unsure; ++lead) /* the board's latest time minus real time */
			for (int skew = -5; skew <= 5; ++skew) {
				const uint64_t real = NOW, upper = real + lead, clock = real + (uint64_t)(int64_t)skew;
				const uint64_t brief = clock + TTL, action = clock + ACTION_TTL;
				assert(brief > upper && brief - upper <= NU54_PILOT_BRIEF_SECONDS);
				assert(action - upper >= 85 && action - upper <= 300); /* SW1 and a 45 s PIN fit */
			}
	/* What the app used to send, and a clock further ahead. */
	assert(!(NOW + 10 > NOW + UNSURE));
	assert(NOW + 6 + TTL - NOW > NU54_PILOT_BRIEF_SECONDS);
}
#endif

int main(void)
{
	test_many_sessions();
	test_one_session();
	test_replay();
#if defined(CONFIG_NU54_SPENDING_PROFILE)
	test_time();
#endif
	return 0;
}
