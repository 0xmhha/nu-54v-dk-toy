/* Local actions of the spending wallet: what may be staged, and that one press does one thing. */
#include "nu54_button.h"
#include "nu54_keccak.h"
#include "nu54_pin_attempt.h"
#include "nu54_spending.h"

#include <assert.h>
#include <stdio.h>
#include <string.h>

#define NOW UINT64_C(1800000000)

static void load(FILE *file, const char *key, uint8_t *out, size_t length)
{
	char line[512];
	rewind(file);
	while (fgets(line, sizeof(line), file)) {
		size_t size = strlen(key);
		if (strncmp(line, key, size) || line[size] != '=') continue;
		assert(strcspn(line + size + 3, "\n") == length * 2);
		for (size_t i = 0; i < length; ++i) {
			unsigned byte;
			assert(sscanf(line + size + 3 + 2 * i, "%2x", &byte) == 1);
			out[i] = (uint8_t)byte;
		}
		return;
	}
	assert(!"vector key missing");
}

static void test_return_matches_the_token(const char *path)
{
	uint8_t token[20], from[20], to[20], nonce[32], expected[66], preimage[66];
	FILE *file = fopen(path, "r");
	assert(file);
	load(file, "token", token, 20);
	load(file, "from", from, 20);
	load(file, "to", to, 20);
	load(file, "nonce", nonce, 32);
	load(file, "preimage", expected, 66);
	fclose(file);
	nu54_spend_return_preimage(token, from, to, 9500000, 1800000500, nonce, preimage);
	assert(!memcmp(preimage, expected, 66));
	to[0] ^= 1;
	nu54_spend_return_preimage(token, from, to, 9500000, 1800000500, nonce, preimage);
	assert(memcmp(preimage, expected, 66));
}

static nu54_v2_link link_now(void)
{
	nu54_v2_link link = {.profile = NU54_V2_MANAGEMENT, .local = NU54_V2_BOARD, .peer = NU54_V2_OWNER,
			     .pending_expiry = NOW + 60};
	memset(link.pending_id, 5, sizeof(link.pending_id));
	memset(link.epoch, 6, sizeof(link.epoch));
	return link;
}

static nu54_v2_request request_now(const uint8_t *payload, size_t length)
{
	nu54_v2_request request = {.command = NU54_V2_MANAGE, .expires = NOW + 60, .payload = payload,
				   .payload_length = length};
	memset(request.id, 5, sizeof(request.id));
	return request;
}

static int stage(const uint8_t *payload, size_t length)
{
	nu54_spend_action action = {0};
	nu54_v2_link link = link_now();
	nu54_v2_request request = request_now(payload, length);
	return nu54_spend_stage(&action, &link, &request, NOW, NOW - 2, 3, 9);
}

static void put64(uint8_t *out, uint64_t value)
{
	for (int i = 7; i >= 0; --i, value >>= 8) out[i] = (uint8_t)value;
}

static void transfer(uint8_t body[49], uint64_t amount, uint64_t until)
{
	memset(body, 0, 49);
	body[0] = NU54_SPEND_RETURN_OP;
	put64(body + 1, amount);
	put64(body + 9, until);
	body[48] = 1;
}

static void test_what_may_be_staged(void)
{
	uint8_t body[98] = {NU54_SPEND_SET_RETURN_OP};
	assert(stage(body, 21)); /* no address */
	body[20] = 1;
	assert(!stage(body, 21) && stage(body, 20) && stage(body, 22));
	body[0] = NU54_SPEND_RESET_OP;
	assert(!stage(body, 1) && stage(body, 2));
	body[0] = NU54_SPEND_OPERATOR_RESET_OP;
	assert(!stage(body, 97) && stage(body, 96) && stage(body, 98));
	memset(body + 1, 0, 97);
	assert(stage(body, 97)); /* no nonce */
	body[0] = NU54_SPEND_STATUS_OP;
	assert(stage(body, 1));

	uint8_t move[49];
	transfer(move, 5, NOW + 100);
	assert(!stage(move, 49) && stage(move, 48));
	transfer(move, 0, NOW + 100);
	assert(stage(move, 49));
	transfer(move, 5, NOW);
	assert(stage(move, 49)); /* already over */
	transfer(move, 5, NOW - 2 + NU54_SPEND_RETURN_MAX_SECONDS);
	assert(!stage(move, 49));
	transfer(move, 5, NOW - 1 + NU54_SPEND_RETURN_MAX_SECONDS);
	assert(stage(move, 49)); /* an authorization that lives too long */
	transfer(move, 5, NOW + 100);
	move[48] = 0;
	assert(stage(move, 49)); /* no nonce */

	uint8_t reset = NU54_SPEND_RESET_OP;
	nu54_spend_action action = {0};
	nu54_v2_link link = link_now();
	nu54_v2_request request = request_now(&reset, 1);
	assert(nu54_spend_stage(&action, &link, &request, 0, 0, 3, 9));      /* no trusted time */
	assert(nu54_spend_stage(&action, &link, &request, NOW + 60, NOW, 3, 9)); /* expired */
	assert(nu54_spend_stage(&action, &link, &request, NOW, NOW, 0, 9));  /* no owner */
	assert(nu54_spend_stage(&action, &link, &request, NOW, NOW, 3, 0));
	link.peer = NU54_V2_TERMINAL;
	assert(nu54_spend_stage(&action, &link, &request, NOW, NOW, 3, 9));
	link = link_now();
	link.profile = NU54_V2_PAYMENT;
	assert(nu54_spend_stage(&action, &link, &request, NOW, NOW, 3, 9));
	link = link_now();
	link.closed = true;
	assert(nu54_spend_stage(&action, &link, &request, NOW, NOW, 3, 9));
	link = link_now();
	link.pending_id[0] ^= 1;
	assert(nu54_spend_stage(&action, &link, &request, NOW, NOW, 3, 9));
	link = link_now();
	link.pending_expiry -= 1;
	assert(nu54_spend_stage(&action, &link, &request, NOW, NOW, 3, 9));
	link = link_now();
	request.command = NU54_V2_PAY_PREPARE;
	assert(nu54_spend_stage(&action, &link, &request, NOW, NOW, 3, 9));
	request.command = NU54_V2_MANAGE;
	assert(!nu54_spend_stage(&action, &link, &request, NOW, NOW, 3, 9));
	assert(nu54_spend_stage(&action, &link, &request, NOW, NOW, 3, 9)); /* one at a time */
}

static bool consume(uint64_t now, uint64_t generation, uint32_t prompt, nu54_v2_link link, const uint8_t *payload,
		    size_t length)
{
	nu54_spend_action action = {0}, out;
	nu54_v2_link staged = link_now();
	nu54_v2_request request = request_now(payload, length);
	assert(!nu54_spend_stage(&action, &staged, &request, NOW, NOW, 3, 9));
	bool accepted = nu54_spend_consume(&action, &link, now, generation, prompt, &out);
	assert(!action.pending); /* one press, whatever the outcome */
	assert(!nu54_spend_consume(&action, &link, now, generation, prompt, &out));
	return accepted;
}

static void test_one_press_for_one_request(void)
{
	const uint8_t reset = NU54_SPEND_RESET_OP;
	nu54_v2_link link = link_now();
	assert(consume(NOW + 59, 3, 9, link, &reset, 1));
	assert(!consume(NOW + 60, 3, 9, link, &reset, 1));
	assert(!consume(0, 3, 9, link, &reset, 1));
	assert(!consume(NOW, 4, 9, link, &reset, 1));
	assert(!consume(NOW, 3, 8, link, &reset, 1));
	link.epoch[0] ^= 1;
	assert(!consume(NOW, 3, 9, link, &reset, 1));
	link = link_now();
	link.pending_id[3] ^= 1;
	assert(!consume(NOW, 3, 9, link, &reset, 1));
	link = link_now();
	link.closed = true;
	assert(!consume(NOW, 3, 9, link, &reset, 1));
	link = link_now();
	link.peer = NU54_V2_TERMINAL;
	assert(!consume(NOW, 3, 9, link, &reset, 1));
	uint8_t move[49];
	transfer(move, 5, NOW + 20);
	assert(consume(NOW + 19, 3, 9, link_now(), move, 49) && !consume(NOW + 20, 3, 9, link_now(), move, 49));
}

static void test_operator_statement(void)
{
	uint8_t board[32] = {1}, nonce[32] = {2}, first[NU54_SPEND_OPERATOR_STATEMENT_SIZE], other[sizeof(first)];
	nu54_spend_operator_statement(board, 3, nonce, first);
	nu54_spend_operator_statement(board, 4, nonce, other);
	assert(memcmp(first, other, sizeof(first)));
	board[0] ^= 1;
	nu54_spend_operator_statement(board, 3, nonce, other);
	assert(memcmp(first, other, sizeof(first)));
	board[0] ^= 1;
	nonce[0] ^= 1;
	nu54_spend_operator_statement(board, 3, nonce, other);
	assert(memcmp(first, other, sizeof(first)));
}

static void test_pin_wait_table(void)
{
	const uint32_t expected[] = {0, 0, 0, 0, 0, 60, 300, 900, 3600, 3600};
	for (uint8_t i = 0; i < 10; ++i) assert(nu54_pin_wait_seconds(i) == expected[i]);
	assert(nu54_pin_wait_seconds(UINT8_MAX) == 3600);
}

static void test_reset_gesture(void)
{
	nu54_chord_t chord = {0};
	assert(nu54_chord_tick(&chord, true, 100) == NU54_BTN_NONE);
	assert(nu54_chord_tick(&chord, true, 100 + NU54_CHORD_MS - 1) == NU54_BTN_NONE);
	assert(nu54_chord_tick(&chord, true, 100 + NU54_CHORD_MS) == NU54_BTN_CHORD);
	assert(nu54_chord_tick(&chord, true, 100 + 9 * NU54_CHORD_MS) == NU54_BTN_NONE); /* once per hold */
	assert(nu54_chord_tick(&chord, false, 60000) == NU54_BTN_NONE);
	assert(nu54_chord_tick(&chord, true, 60010) == NU54_BTN_NONE);
	assert(nu54_chord_tick(&chord, false, 64000) == NU54_BTN_NONE); /* let go too early */
	assert(nu54_chord_tick(&chord, true, 64010) == NU54_BTN_NONE);
	assert(nu54_chord_tick(&chord, true, 66000) == NU54_BTN_NONE); /* the first hold does not count */
	assert(nu54_chord_tick(&chord, true, 64010 + NU54_CHORD_MS) == NU54_BTN_CHORD); /* a full hold fires again */
	nu54_chord_t wrapped = {0};
	assert(nu54_chord_tick(&wrapped, true, UINT32_MAX - 10) == NU54_BTN_NONE);
	assert(nu54_chord_tick(&wrapped, true, 4989) == NU54_BTN_CHORD);

	nu54_button_t button;
	nu54_button_reset(&button);
	nu54_button_update(&button, true, 0);
	nu54_button_suppress(&button);
	assert(nu54_button_tick(&button, 5000) == NU54_BTN_NONE && nu54_button_update(&button, false, 5100) == NU54_BTN_NONE);
	nu54_button_update(&button, true, 6000);
	assert(nu54_button_update(&button, false, 6100) == NU54_BTN_CLICK); /* the next press is normal */
	nu54_button_suppress(&button);
	nu54_button_update(&button, true, 7000);
	assert(nu54_button_update(&button, false, 7100) == NU54_BTN_CLICK); /* nothing to drop when it was up */
}

/* Both status replies use this rule. Zero means "no owner yet" to the phone, so a board that has
 * an owner and lost its wallet must fail and never answer zero. */
static void test_status_account(void)
{
	const uint8_t account[20] = {1, 2, 3}, zero[20] = {0};
	uint8_t out[20];
	memset(out, 9, sizeof(out));
	assert(!nu54_spend_status_account(account, false, out) && !memcmp(out, account, 20));
	memset(out, 9, sizeof(out));
	assert(!nu54_spend_status_account(NULL, true, out) && !memcmp(out, zero, 20));
	memset(out, 9, sizeof(out));
	assert(nu54_spend_status_account(NULL, false, out) == -1 && out[0] == 9 && out[19] == 9);
	/* A registration that made the key and has not stored the owner yet reports the key. */
	assert(!nu54_spend_status_account(account, true, out) && !memcmp(out, account, 20));
	assert(nu54_spend_status_account(account, false, NULL) == -1);
}

int main(int argc, char **argv)
{
	assert(argc == 2);
	test_status_account();
	test_return_matches_the_token(argv[1]);
	test_what_may_be_staged();
	test_one_press_for_one_request();
	test_operator_statement();
	test_pin_wait_table();
	test_reset_gesture();
	puts("spending actions ok");
	return 0;
}
