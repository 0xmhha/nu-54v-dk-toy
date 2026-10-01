/*
 * The firmware CBOR codec against the shared CBOR vectors: every body decodes and validates
 * against the schema tables and re-encodes to the same bytes; payment.result is built through
 * the encoding API (integer and bignum nonce); bodies that break the rules are refused.
 */
#include <stdio.h>
#include <string.h>

#include "nu54_cbor.h"
#include "vectors.h"

static int failures;
#define CHECK(cond, ...)                                           \
	do {                                                       \
		if (!(cond)) {                                     \
			failures++;                                \
			printf("FAIL %s:%d ", __FILE__, __LINE__); \
			printf(__VA_ARGS__);                       \
			printf("\n");                              \
		}                                                  \
	} while (0)

/* Re-encodes the map at `map` generically from the decoded items. */
static int reencode(const nu54_msg_t *m, int map, nu54_cbor_writer_t *w)
{
	nu54_cbor_entry_t e[24];
	char keys[24][32];
	uint8_t nested[4][512];
	size_t n = 0, nn = 0;
	int i = map + 1;
	for (uint64_t k = 0; k < m->items[map].value; k++) {
		const nu54_cbor_item_t *kt = &m->items[i];
		const nu54_cbor_item_t *v = &m->items[kt->next];
		memcpy(keys[n], kt->ptr, kt->len);
		keys[n][kt->len] = 0;
		e[n].key = keys[n];
		switch (v->type) {
		case NU54_CBOR_UINT:
			e[n].kind = NU54_V_UINT;
			e[n].value = v->value;
			break;
		case NU54_CBOR_BYTES:
		case NU54_CBOR_TEXT:
			e[n].kind = v->type == NU54_CBOR_BYTES ? NU54_V_BYTES : NU54_V_TEXT;
			e[n].ptr = v->ptr;
			e[n].len = v->len;
			break;
		case NU54_CBOR_BOOL:
			e[n].kind = NU54_V_BOOL;
			e[n].value = v->value;
			break;
		case NU54_CBOR_BIGNUM: {
			static uint8_t wide[4][32];
			memset(wide[nn], 0, 32);
			memcpy(&wide[nn][32 - v->len], v->ptr, v->len);
			e[n].kind = NU54_V_UINT256;
			e[n].ptr = wide[nn++];
			break;
		}
		case NU54_CBOR_MAP: {
			nu54_cbor_writer_t sub;
			nu54_cbor_writer_init(&sub, nested[nn], sizeof(nested[nn]));
			if (reencode(m, kt->next, &sub) != 0) {
				return -1;
			}
			e[n].kind = NU54_V_RAW;
			e[n].ptr = nested[nn++];
			e[n].len = sub.len;
			break;
		}
		}
		n++;
		i = v->next;
	}
	return nu54_cbor_write_map(w, e, n);
}

static nu54_msg_result_t decode_hex(const char *hex, nu54_msg_t *m)
{
	static uint8_t buf[256];
	size_t n = strlen(hex) / 2;
	for (size_t i = 0; i < n; i++) {
		unsigned x;
		sscanf(&hex[2 * i], "%2x", &x);
		buf[i] = (uint8_t)x;
	}
	return nu54_msg_decode(buf, n, m);
}

int main(void)
{
	static nu54_msg_t m;
	uint8_t out[1024];
	nu54_cbor_writer_t w;

	for (size_t i = 0; i < CBOR_VECTOR_COUNT; i++) {
		const cbor_vector_t *v = &CBOR_VECTORS[i];
		CHECK(nu54_msg_decode(v->cbor, v->len, &m) == NU54_MSG_OK, "%s decode", v->id);
		CHECK(m.message && strcmp(m.message->type, v->type) == 0, "%s type", v->id);
		nu54_cbor_writer_init(&w, out, sizeof(out));
		CHECK(reencode(&m, 0, &w) == 0 && w.len == v->len && memcmp(out, v->cbor, v->len) == 0, "%s re-encode", v->id);
	}

	/* The device builds payment.result through the API: CB-06 (small nonce), CB-07 (bignum). */
	for (size_t i = 5; i <= 6; i++) {
		const cbor_vector_t *v = &CBOR_VECTORS[i];
		CHECK(nu54_msg_decode(v->cbor, v->len, &m) == NU54_MSG_OK, "%s decode", v->id);
		int sid = nu54_msg_find(&m, 0, "sessionId"), sig = nu54_msg_find(&m, 0, "signature"), nonce = nu54_msg_find(&m, 0, "nonce");
		uint8_t n256[32] = {0};
		if (m.items[nonce].type == NU54_CBOR_UINT) {
			for (int k = 0; k < 8; k++) {
				n256[31 - k] = (uint8_t)(m.items[nonce].value >> (8 * k));
			}
		} else {
			memcpy(&n256[32 - m.items[nonce].len], m.items[nonce].ptr, m.items[nonce].len);
		}
		nu54_cbor_entry_t e[] = {
			{"nonce", NU54_V_UINT256, n256, 32, 0},
			{"outcome", NU54_V_TEXT, (const uint8_t *)"approved", 8, 0},
			{"signature", NU54_V_BYTES, m.items[sig].ptr, 65, 0},
			{"type", NU54_V_TEXT, (const uint8_t *)"payment.result", 14, 0},
			{"sessionId", NU54_V_BYTES, m.items[sid].ptr, 8, 0},
			{"v", NU54_V_UINT, NULL, 0, 1},
		};
		nu54_cbor_writer_init(&w, out, sizeof(out));
		CHECK(nu54_cbor_write_map(&w, e, 6) == 0 && w.len == v->len && memcmp(out, v->cbor, v->len) == 0, "%s built by the API", v->id);
	}

	/* Refusals (the same cases as the TypeScript core). */
	struct {
		const char *hex;
		nu54_msg_result_t want;
		const char *what;
	} bad[] = {
		{"a1647479706565657272", NU54_MSG_BAD_FRAME, "text shorter than its length"},
		{"a164747970656" "2c328", NU54_MSG_BAD_FRAME, "invalid UTF-8"},
		{"a2617618016474797065656572726f72", NU54_MSG_BAD_FRAME, "non-shortest integer"},
		{"bf617601ff", NU54_MSG_BAD_FRAME, "indefinite-length map"},
		{"a16176f93c00", NU54_MSG_BAD_FRAME, "float"},
		{"a16176c34100", NU54_MSG_BAD_FRAME, "tag other than 2"},
		{"a16474797065646e6f7065", NU54_MSG_UNSUPPORTED_TYPE, "type outside the schema"},
		/* CB-08 is valid; these change only the key order or repeat one key, so nothing else can refuse them. */
		{"a46176016474797065656572726f7266726561736f6e694241445f4652414d456973657373696f6e4964480102030405060708", NU54_MSG_OK, "CB-08 as is"},
		{"a46474797065656572726f7261760166726561736f6e694241445f4652414d456973657373696f6e4964480102030405060708", NU54_MSG_BAD_FRAME, "CB-08 with keys out of order"},
		{"a56176016176016474797065656572726f7266726561736f6e694241445f4652414d456973657373696f6e4964480102030405060708", NU54_MSG_BAD_FRAME, "CB-08 with a repeated key"},
		{"a56176016474797065656572726f7266726561736f6e694241445f4652414d456973657373696f6e4964480102030405060708", NU54_MSG_BAD_FRAME, "CB-08 with a map count above its entries"},
	};
	for (size_t i = 0; i < sizeof(bad) / sizeof(bad[0]); i++) {
		CHECK(decode_hex(bad[i].hex, &m) == bad[i].want, "%s", bad[i].what);
	}
	/* Schema checks inside a well-formed body: CB-08 with a 4-byte sessionId, with an extra key, without reason. */
	uint8_t sid4[4] = {1, 2, 3, 4};
	nu54_cbor_entry_t err[] = {
		{"v", NU54_V_UINT, NULL, 0, 1},
		{"type", NU54_V_TEXT, (const uint8_t *)"error", 5, 0},
		{"sessionId", NU54_V_BYTES, sid4, 4, 0},
		{"reason", NU54_V_TEXT, (const uint8_t *)"BAD_FRAME", 9, 0},
		{"zzzzzzzzzzzz", NU54_V_UINT, NULL, 0, 1},
	};
	nu54_cbor_writer_init(&w, out, sizeof(out));
	nu54_cbor_write_map(&w, err, 4);
	CHECK(nu54_msg_decode(out, w.len, &m) == NU54_MSG_BAD_FRAME, "4-byte sessionId");
	uint8_t sid8[8] = {1, 2, 3, 4, 5, 6, 7, 8};
	err[2].ptr = sid8;
	err[2].len = 8;
	nu54_cbor_writer_init(&w, out, sizeof(out));
	nu54_cbor_write_map(&w, err, 4);
	CHECK(nu54_msg_decode(out, w.len, &m) == NU54_MSG_OK, "valid error message");
	CHECK(w.len == CBOR_VECTORS[7].len && memcmp(out, CBOR_VECTORS[7].cbor, w.len) == 0, "CB-08 built by the API");
	nu54_cbor_writer_init(&w, out, sizeof(out));
	nu54_cbor_write_map(&w, err, 5);
	CHECK(nu54_msg_decode(out, w.len, &m) == NU54_MSG_BAD_FRAME, "unknown key");
	nu54_cbor_writer_init(&w, out, sizeof(out));
	nu54_cbor_write_map(&w, err, 3);
	CHECK(nu54_msg_decode(out, w.len, &m) == NU54_MSG_BAD_FRAME, "missing reason");
	nu54_cbor_writer_init(&w, out, 10);
	CHECK(nu54_cbor_write_map(&w, err, 4) == -1, "writer overflow");

	printf("%s: %zu CBOR vectors, %d failures\n", failures ? "FAIL" : "ok", (size_t)CBOR_VECTOR_COUNT, failures);
	return failures ? 1 : 0;
}
