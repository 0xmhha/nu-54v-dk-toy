/* Host tests for nu54_frame (payment-protocol.md §4). */
#include <assert.h>
#include <stdio.h>
#include <string.h>

#include "nu54_frame.h"

static const uint8_t DIGEST[8] = {1, 2, 3, 4, 5, 6, 7, 8};

/* Build an envelope for body and split it into fragments of the given MTU. */
static size_t split(const uint8_t *body, size_t body_len, uint16_t mtu, uint8_t seq,
		    uint8_t frags[][256], size_t frag_len[])
{
	uint8_t env[NU54_MAX_ENVELOPE_LEN];
	assert(nu54_envelope_header(env, body_len, DIGEST) == 0);
	memcpy(&env[NU54_ENVELOPE_HEADER_LEN], body, body_len);
	size_t total = NU54_ENVELOPE_HEADER_LEN + body_len, per = nu54_fragment_payload(mtu), count = 0;
	for (size_t off = 0; off < total; off += per, count++) {
		size_t n = total - off < per ? total - off : per;
		frags[count][0] = seq;
		frags[count][1] = (uint8_t)count;
		memcpy(&frags[count][2], &env[off], n);
		frag_len[count] = n + 2;
	}
	return count;
}

static void test_roundtrip_mtu23(void)
{
	uint8_t body[100], frags[16][256];
	size_t lens[16], body_len;
	for (size_t i = 0; i < sizeof(body); i++) {
		body[i] = (uint8_t)i;
	}
	size_t count = split(body, sizeof(body), 23, 7, frags, lens);
	assert(count == 7); /* 110 bytes / 18 per fragment */
	nu54_reassembler_t r;
	nu54_reassembler_reset(&r);
	for (size_t i = 0; i + 1 < count; i++) {
		assert(nu54_reassembler_feed(&r, frags[i], lens[i]) == NU54_FRAME_NEED_MORE);
	}
	assert(nu54_reassembler_feed(&r, frags[count - 1], lens[count - 1]) == NU54_FRAME_DONE);
	const uint8_t *out = nu54_reassembler_body(&r, &body_len);
	assert(body_len == sizeof(body) && memcmp(out, body, body_len) == 0);
	assert(memcmp(nu54_reassembler_digest(&r), DIGEST, 8) == 0);
}

static void test_single_fragment_mtu247(void)
{
	uint8_t body[20] = {0}, frags[2][256];
	size_t lens[2];
	assert(split(body, sizeof(body), 247, 1, frags, lens) == 1);
	nu54_reassembler_t r;
	nu54_reassembler_reset(&r);
	assert(nu54_reassembler_feed(&r, frags[0], lens[0]) == NU54_FRAME_DONE);
}

static void test_out_of_order_is_bad(void)
{
	uint8_t body[60] = {0}, frags[8][256];
	size_t lens[8];
	size_t count = split(body, sizeof(body), 23, 2, frags, lens);
	assert(count >= 3);
	nu54_reassembler_t r;
	nu54_reassembler_reset(&r);
	assert(nu54_reassembler_feed(&r, frags[0], lens[0]) == NU54_FRAME_NEED_MORE);
	assert(nu54_reassembler_feed(&r, frags[2], lens[2]) == NU54_FRAME_BAD);
	/* after BAD the next message must start again at index 0 */
	assert(nu54_reassembler_feed(&r, frags[1], lens[1]) == NU54_FRAME_BAD);
}

static void test_sequence_change_is_bad(void)
{
	uint8_t body[60] = {0}, frags[8][256];
	size_t lens[8];
	split(body, sizeof(body), 23, 3, frags, lens);
	nu54_reassembler_t r;
	nu54_reassembler_reset(&r);
	assert(nu54_reassembler_feed(&r, frags[0], lens[0]) == NU54_FRAME_NEED_MORE);
	frags[1][0] = 4;
	assert(nu54_reassembler_feed(&r, frags[1], lens[1]) == NU54_FRAME_BAD);
}

static void test_oversize_rejected(void)
{
	uint8_t header[NU54_ENVELOPE_HEADER_LEN];
	assert(nu54_envelope_header(header, NU54_MAX_BODY_LEN + 1, DIGEST) == -1);
	uint8_t frag[8] = {0, 0, 0xff, 0xff, 0, 0, 0, 0}; /* declared length 65535 */
	nu54_reassembler_t r;
	nu54_reassembler_reset(&r);
	assert(nu54_reassembler_feed(&r, frag, sizeof(frag)) == NU54_FRAME_BAD);
}

int main(void)
{
	test_roundtrip_mtu23();
	test_single_fragment_mtu247();
	test_out_of_order_is_bad();
	test_sequence_change_is_bad();
	test_oversize_rejected();
	puts("nu54_frame: all tests passed");
	return 0;
}
