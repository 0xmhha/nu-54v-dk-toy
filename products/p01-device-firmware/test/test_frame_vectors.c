/*
 * The firmware reassembler against the shared fragment vectors: every valid stream rebuilds
 * its CBOR body, and every invalid stream ends in BAD_FRAME (in the reassembler, or in the
 * caller's digest check once the envelope is complete).
 */
#include <stdio.h>
#include <string.h>

#include "nu54_frame.h"
#include "vectors.h"

int main(void)
{
	int failures = 0;
	static nu54_reassembler_t r;

	for (size_t i = 0; i < FRAME_VECTOR_COUNT; i++) {
		const frame_vector_t *v = &FRAME_VECTORS[i];
		nu54_frame_result_t res = NU54_FRAME_NEED_MORE;
		int bad = 0, done = 0;
		nu54_reassembler_reset(&r);
		for (size_t k = 0; k < v->count && !bad; k++) {
			res = nu54_reassembler_feed(&r, v->frags[k], v->lens[k]);
			bad = res == NU54_FRAME_BAD;
			if (res == NU54_FRAME_DONE) {
				done = 1;
				/* The caller's digest check: carried digest vs SHA-256 of the body. */
				bad = memcmp(nu54_reassembler_digest(&r), v->sha8, 8) != 0 || k != v->count - 1;
			}
		}
		if (v->valid) {
			size_t len = 0;
			const uint8_t *body = done ? nu54_reassembler_body(&r, &len) : NULL;
			if (bad || !done || len != v->body_len || memcmp(body, v->body, len) != 0) {
				printf("FAIL %s: valid stream not rebuilt\n", v->id);
				failures++;
			}
		} else if (!bad) {
			printf("FAIL %s: invalid stream accepted\n", v->id);
			failures++;
		}
	}
	printf("%s: %zu fragment vectors, %d failures\n", failures ? "FAIL" : "ok", (size_t)FRAME_VECTOR_COUNT, failures);
	return failures ? 1 : 0;
}
