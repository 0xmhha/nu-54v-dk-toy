#include "nu54_frame.h"

#include <string.h>

size_t nu54_fragment_payload(uint16_t att_mtu)
{
	return att_mtu > 5u ? (size_t)att_mtu - 5u : 0u;
}

int nu54_envelope_header(uint8_t out[NU54_ENVELOPE_HEADER_LEN], size_t body_len, const uint8_t digest8[8])
{
	if (body_len > NU54_MAX_BODY_LEN) {
		return -1;
	}
	size_t total = NU54_ENVELOPE_HEADER_LEN + body_len;
	out[0] = (uint8_t)(total >> 8);
	out[1] = (uint8_t)(total & 0xffu);
	memcpy(&out[2], digest8, 8);
	return 0;
}

void nu54_reassembler_reset(nu54_reassembler_t *r)
{
	r->have = 0;
	r->want = 0;
	r->next_index = 0;
	r->active = 0;
}

static nu54_frame_result_t bad(nu54_reassembler_t *r)
{
	nu54_reassembler_reset(r);
	return NU54_FRAME_BAD;
}

nu54_frame_result_t nu54_reassembler_feed(nu54_reassembler_t *r, const uint8_t *frag, size_t len)
{
	if (len <= NU54_FRAGMENT_HEADER_LEN) {
		return bad(r);
	}
	uint8_t seq = frag[0];
	uint8_t idx = frag[1];
	const uint8_t *data = &frag[NU54_FRAGMENT_HEADER_LEN];
	size_t n = len - NU54_FRAGMENT_HEADER_LEN;

	if (!r->active) {
		if (idx != 0) {
			return bad(r);
		}
		/* A new message: drop what the previous (completed) message left. */
		r->have = 0;
		r->want = 0;
		r->active = 1;
		r->sequence = seq;
		r->next_index = 0;
	} else if (seq != r->sequence || idx != r->next_index) {
		return bad(r);
	}
	if (r->have + n > NU54_MAX_ENVELOPE_LEN) {
		return bad(r);
	}
	memcpy(&r->buf[r->have], data, n);
	r->have += n;
	r->next_index++;

	if (r->want == 0 && r->have >= 2) {
		r->want = ((size_t)r->buf[0] << 8) | r->buf[1];
		if (r->want < NU54_ENVELOPE_HEADER_LEN || r->want > NU54_MAX_ENVELOPE_LEN) {
			return bad(r);
		}
	}
	if (r->want != 0 && r->have > r->want) {
		return bad(r);
	}
	if (r->want != 0 && r->have == r->want) {
		r->active = 0;
		return NU54_FRAME_DONE;
	}
	return NU54_FRAME_NEED_MORE;
}

const uint8_t *nu54_reassembler_body(const nu54_reassembler_t *r, size_t *body_len)
{
	*body_len = r->want - NU54_ENVELOPE_HEADER_LEN;
	return &r->buf[NU54_ENVELOPE_HEADER_LEN];
}

const uint8_t *nu54_reassembler_digest(const nu54_reassembler_t *r)
{
	return &r->buf[2];
}
