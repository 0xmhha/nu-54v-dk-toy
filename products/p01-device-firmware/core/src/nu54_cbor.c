#include "nu54_cbor.h"

#include <string.h>

/* ---------------------------------------------------------------- decoding */

typedef struct {
	const uint8_t *b;
	size_t len;
	size_t i;
	nu54_msg_t *m;
} parser_t;

static int head(parser_t *p, int *major, uint64_t *arg)
{
	if (p->i >= p->len) {
		return -1;
	}
	uint8_t ib = p->b[p->i++];
	unsigned info = ib & 0x1fu;
	*major = ib >> 5;
	if (info < 24) {
		*arg = info;
		return 0;
	}
	unsigned size = info == 24 ? 1 : info == 25 ? 2 : info == 26 ? 4 : info == 27 ? 8 : 0;
	if (size == 0 || p->len - p->i < size) {
		return -1; /* indefinite length, reserved value or truncated */
	}
	uint64_t v = 0;
	for (unsigned k = 0; k < size; k++) {
		v = (v << 8) | p->b[p->i++];
	}
	/* Shortest form: the value must not fit the next smaller encoding. */
	uint64_t min = size == 1 ? 24 : (uint64_t)1 << (4 * size);
	if (v < min) {
		return -1;
	}
	*arg = v;
	return 0;
}

static int utf8_ok(const uint8_t *s, size_t n)
{
	for (size_t i = 0; i < n;) {
		uint8_t x = s[i];
		int k = x < 0x80 ? 0 : (x >= 0xc2 && x <= 0xdf) ? 1 : (x >= 0xe0 && x <= 0xef) ? 2 : (x >= 0xf0 && x <= 0xf4) ? 3 : -1;
		if (k < 0 || i + (size_t)k >= n) { /* bad lead byte, or truncated (k = 0 never is) */
			return 0;
		}
		uint32_t c = k == 0 ? x : (uint32_t)(x & (0x3f >> k));
		for (int j = 1; j <= k; j++) {
			if ((s[i + j] & 0xc0) != 0x80) {
				return 0;
			}
			c = (c << 6) | (s[i + j] & 0x3f);
		}
		if ((k == 2 && (c < 0x800 || (c >= 0xd800 && c <= 0xdfff))) || (k == 3 && (c < 0x10000 || c > 0x10ffff))) {
			return 0;
		}
		i += (size_t)k + 1;
	}
	return 1;
}

/* Compares two encoded keys (length-first, then bytes, as RFC 8949 4.2.1 orders them). */
static int key_cmp(const nu54_cbor_item_t *a, const nu54_cbor_item_t *b)
{
	if (a->len != b->len) {
		return a->len < b->len ? -1 : 1; /* text keys here are short: header size follows length */
	}
	return memcmp(a->ptr, b->ptr, a->len);
}

static int item(parser_t *p, int depth)
{
	if (depth > 3 || p->m->count >= NU54_CBOR_MAX_ITEMS) {
		return -1;
	}
	uint16_t at = p->m->count++;
	nu54_cbor_item_t *it = &p->m->items[at];
	int major;
	uint64_t arg;
	size_t start = p->i;
	if (head(p, &major, &arg) != 0) {
		return -1;
	}
	switch (major) {
	case 0:
		it->type = NU54_CBOR_UINT;
		it->value = arg;
		break;
	case 2:
	case 3:
		if (arg > p->len - p->i) {
			return -1;
		}
		it->type = major == 2 ? NU54_CBOR_BYTES : NU54_CBOR_TEXT;
		it->ptr = &p->b[p->i];
		it->len = (uint32_t)arg;
		p->i += (size_t)arg;
		if (major == 3 && !utf8_ok(it->ptr, it->len)) {
			return -1;
		}
		break;
	case 5: {
		it->type = NU54_CBOR_MAP;
		it->value = arg;
		int prev = -1;
		for (uint64_t k = 0; k < arg; k++) {
			int key = p->m->count;
			if (item(p, depth + 1) != 0 || p->m->items[key].type != NU54_CBOR_TEXT) {
				return -1;
			}
			if (prev >= 0 && key_cmp(&p->m->items[prev], &p->m->items[key]) >= 0) {
				return -1; /* out of order or repeated */
			}
			prev = key;
			if (item(p, depth + 1) != 0) {
				return -1;
			}
		}
		break;
	}
	case 6: {
		if (arg != 2) {
			return -1;
		}
		int major2;
		uint64_t n;
		if (head(p, &major2, &n) != 0 || major2 != 2 || n == 0 || n > 32 || n > p->len - p->i) {
			return -1;
		}
		const uint8_t *mag = &p->b[p->i];
		if (mag[0] == 0 || n <= 8) {
			return -1; /* leading zero, or a value below 2^64 that must be a plain integer */
		}
		it->type = NU54_CBOR_BIGNUM;
		it->ptr = mag;
		it->len = (uint32_t)n;
		p->i += (size_t)n;
		break;
	}
	case 7:
		if (p->b[start] != 0xf4 && p->b[start] != 0xf5) {
			return -1; /* floats and other simple values */
		}
		it->type = NU54_CBOR_BOOL;
		it->value = p->b[start] == 0xf5;
		break;
	default:
		return -1;
	}
	it->next = p->m->count;
	return 0;
}

int nu54_msg_find(const nu54_msg_t *m, int map, const char *key)
{
	size_t n = strlen(key);
	if (map < 0 || map >= m->count || m->items[map].type != NU54_CBOR_MAP) {
		return -1;
	}
	int i = map + 1;
	for (uint64_t k = 0; k < m->items[map].value; k++) {
		const nu54_cbor_item_t *kt = &m->items[i];
		int v = kt->next;
		if (kt->len == n && memcmp(kt->ptr, key, n) == 0) {
			return v;
		}
		i = m->items[v].next;
	}
	return -1;
}

static const unsigned BYTE_LEN[] = {[NU54_K_HEX20] = 20, [NU54_K_HEX32] = 32, [NU54_K_SIGNATURE] = 65, [NU54_K_SESSION_ID] = 8};

static int check_object(const nu54_msg_t *m, int map, const nu54_object_t *obj)
{
	if (m->items[map].type != NU54_CBOR_MAP) {
		return -1;
	}
	/* Every key known, with the right kind. */
	int i = map + 1;
	for (uint64_t k = 0; k < m->items[map].value; k++) {
		const nu54_cbor_item_t *kt = &m->items[i];
		int v = kt->next;
		const nu54_field_t *f = NULL;
		for (unsigned j = 0; j < obj->count; j++) {
			if (strlen(obj->fields[j].name) == kt->len && memcmp(obj->fields[j].name, kt->ptr, kt->len) == 0) {
				f = &obj->fields[j];
			}
		}
		if (!f) {
			return -1;
		}
		const nu54_cbor_item_t *vt = &m->items[v];
		switch (f->kind) {
		case NU54_K_HEX20:
		case NU54_K_HEX32:
		case NU54_K_SIGNATURE:
		case NU54_K_SESSION_ID:
			if (vt->type != NU54_CBOR_BYTES || vt->len != BYTE_LEN[f->kind]) {
				return -1;
			}
			break;
		case NU54_K_UINT:
			if (vt->type != NU54_CBOR_UINT && vt->type != NU54_CBOR_BIGNUM) {
				return -1;
			}
			break;
		case NU54_K_INT:
			if (vt->type != NU54_CBOR_UINT) {
				return -1;
			}
			break;
		case NU54_K_TEXT:
			if (vt->type != NU54_CBOR_TEXT) {
				return -1;
			}
			break;
		case NU54_K_BOOL:
			if (vt->type != NU54_CBOR_BOOL) {
				return -1;
			}
			break;
		case NU54_K_OBJECT:
			if (check_object(m, v, f->object) != 0) {
				return -1;
			}
			break;
		}
		i = vt->next;
	}
	/* Every required field present. */
	for (unsigned j = 0; j < obj->count; j++) {
		if (obj->fields[j].required && nu54_msg_find(m, map, obj->fields[j].name) < 0) {
			return -1;
		}
	}
	return 0;
}

nu54_msg_result_t nu54_msg_decode(const uint8_t *body, size_t len, nu54_msg_t *out)
{
	parser_t p = {body, len, 0, out};
	out->count = 0;
	out->message = NULL;
	if (item(&p, 0) != 0 || p.i != len || out->items[0].type != NU54_CBOR_MAP) {
		return NU54_MSG_BAD_FRAME;
	}
	int t = nu54_msg_find(out, 0, "type");
	if (t < 0 || out->items[t].type != NU54_CBOR_TEXT) {
		return NU54_MSG_BAD_FRAME;
	}
	for (unsigned k = 0; k < NU54_MESSAGE_COUNT; k++) {
		const char *name = nu54_messages[k].type;
		if (strlen(name) == out->items[t].len && memcmp(name, out->items[t].ptr, out->items[t].len) == 0) {
			out->message = &nu54_messages[k];
		}
	}
	if (!out->message) {
		return NU54_MSG_UNSUPPORTED_TYPE;
	}
	return check_object(out, 0, out->message->object) == 0 ? NU54_MSG_OK : NU54_MSG_BAD_FRAME;
}

/* ---------------------------------------------------------------- encoding */

void nu54_cbor_writer_init(nu54_cbor_writer_t *w, uint8_t *buf, size_t cap)
{
	w->buf = buf;
	w->cap = cap;
	w->len = 0;
	w->overflow = 0;
}

static void put(nu54_cbor_writer_t *w, const uint8_t *b, size_t n)
{
	if (w->overflow || w->cap - w->len < n) {
		w->overflow = 1;
		return;
	}
	memcpy(&w->buf[w->len], b, n);
	w->len += n;
}

static void put_head(nu54_cbor_writer_t *w, unsigned major, uint64_t v)
{
	uint8_t h[9];
	size_t n;
	if (v < 24) {
		h[0] = (uint8_t)(major << 5 | v);
		n = 1;
	} else {
		unsigned size = v <= 0xff ? 1 : v <= 0xffff ? 2 : v <= 0xffffffffu ? 4 : 8;
		h[0] = (uint8_t)(major << 5 | (size == 1 ? 24 : size == 2 ? 25 : size == 4 ? 26 : 27));
		for (unsigned k = 0; k < size; k++) {
			h[1 + k] = (uint8_t)(v >> (8 * (size - 1 - k)));
		}
		n = 1 + size;
	}
	put(w, h, n);
}

static void put_uint256(nu54_cbor_writer_t *w, const uint8_t v[32])
{
	size_t z = 0;
	while (z < 32 && v[z] == 0) {
		z++;
	}
	if (32 - z <= 8) {
		uint64_t x = 0;
		for (size_t k = z; k < 32; k++) {
			x = (x << 8) | v[k];
		}
		put_head(w, 0, x);
		return;
	}
	put_head(w, 6, 2); /* tag 2 bignum, no leading zero bytes */
	put_head(w, 2, 32 - z);
	put(w, &v[z], 32 - z);
}

/* Encoded-key order: shorter keys first (one-byte text heads), then bytewise. */
static int entry_cmp(const nu54_cbor_entry_t *a, const nu54_cbor_entry_t *b)
{
	size_t la = strlen(a->key), lb = strlen(b->key);
	if (la != lb) {
		return la < lb ? -1 : 1;
	}
	return memcmp(a->key, b->key, la);
}

int nu54_cbor_write_map(nu54_cbor_writer_t *w, const nu54_cbor_entry_t *entries, size_t n)
{
	const nu54_cbor_entry_t *order[24];
	if (n > 24) {
		return -1;
	}
	for (size_t i = 0; i < n; i++) {
		size_t j = i;
		while (j > 0 && entry_cmp(order[j - 1], &entries[i]) > 0) {
			order[j] = order[j - 1];
			j--;
		}
		order[j] = &entries[i];
	}
	put_head(w, 5, n);
	for (size_t i = 0; i < n; i++) {
		const nu54_cbor_entry_t *e = order[i];
		put_head(w, 3, strlen(e->key));
		put(w, (const uint8_t *)e->key, strlen(e->key));
		switch (e->kind) {
		case NU54_V_BYTES:
			put_head(w, 2, e->len);
			put(w, e->ptr, e->len);
			break;
		case NU54_V_TEXT:
			put_head(w, 3, e->len);
			put(w, e->ptr, e->len);
			break;
		case NU54_V_UINT:
			put_head(w, 0, e->value);
			break;
		case NU54_V_UINT256:
			put_uint256(w, e->ptr);
			break;
		case NU54_V_BOOL: {
			uint8_t b = e->value ? 0xf5 : 0xf4;
			put(w, &b, 1);
			break;
		}
		case NU54_V_RAW:
			put(w, e->ptr, e->len);
			break;
		}
	}
	return w->overflow ? -1 : 0;
}
