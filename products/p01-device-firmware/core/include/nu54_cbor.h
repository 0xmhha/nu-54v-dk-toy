/*
 * Deterministic CBOR for BLE message bodies (payment-protocol.md 4.2).
 *
 * Decoding parses a body into a flat array of items (no allocation), refuses anything outside
 * the rules (non-shortest heads, indefinite lengths, floats, tags other than 2, bignums below
 * 2^64 or with a leading zero, keys out of order or repeated, invalid UTF-8, trailing bytes),
 * then checks the message against the generated field tables in nu54_protocol.h: known type,
 * required fields present, no unknown keys, the right kind and byte length for every field.
 * Encoding writes the map keys in the deterministic order whatever order the caller gives.
 * Board-independent: builds and tests on the host.
 */
#ifndef NU54_CBOR_H
#define NU54_CBOR_H

#include <stddef.h>
#include <stdint.h>

#include "nu54_protocol.h"

#define NU54_CBOR_MAX_ITEMS 64

typedef enum {
	NU54_CBOR_UINT,   /* major 0 */
	NU54_CBOR_BYTES,  /* major 2 */
	NU54_CBOR_TEXT,   /* major 3 */
	NU54_CBOR_MAP,    /* major 5; followed by count key/value pairs */
	NU54_CBOR_BIGNUM, /* tag 2: ptr/len are the magnitude bytes (>= 2^64) */
	NU54_CBOR_BOOL,
} nu54_cbor_type_t;

typedef struct {
	nu54_cbor_type_t type;
	const uint8_t *ptr; /* bytes, text or bignum magnitude */
	uint32_t len;
	uint64_t value; /* uint value, map pair count, or bool (0/1) */
	uint16_t next;  /* index of the item after this one and everything it contains */
} nu54_cbor_item_t;

typedef enum {
	NU54_MSG_OK = 0,
	NU54_MSG_BAD_FRAME,        /* body breaks the encoding rules or the schema */
	NU54_MSG_UNSUPPORTED_TYPE, /* well-formed, but the type is not in the schema */
} nu54_msg_result_t;

typedef struct {
	nu54_cbor_item_t items[NU54_CBOR_MAX_ITEMS];
	uint16_t count;
	const nu54_message_t *message; /* schema entry of the decoded type */
} nu54_msg_t;

/* Parses and validates one message body. */
nu54_msg_result_t nu54_msg_decode(const uint8_t *body, size_t len, nu54_msg_t *out);

/* Value item of `key` in the map item at index `map` (0 = the message itself), or -1. */
int nu54_msg_find(const nu54_msg_t *m, int map, const char *key);

/* ---- encoding ---- */

typedef struct {
	uint8_t *buf;
	size_t cap;
	size_t len;
	int overflow;
} nu54_cbor_writer_t;

typedef enum { NU54_V_BYTES, NU54_V_TEXT, NU54_V_UINT, NU54_V_UINT256, NU54_V_BOOL, NU54_V_RAW } nu54_value_kind_t;

/* One map entry to write. UINT256 is a 32-byte big-endian value written as an integer when it
 * fits 64 bits and as a tag-2 bignum otherwise. RAW is an already encoded CBOR item (a nested map). */
typedef struct {
	const char *key;
	nu54_value_kind_t kind;
	const uint8_t *ptr;
	size_t len;
	uint64_t value;
} nu54_cbor_entry_t;

void nu54_cbor_writer_init(nu54_cbor_writer_t *w, uint8_t *buf, size_t cap);
/* Writes a map of n entries in deterministic key order. Returns 0, or -1 on overflow. */
int nu54_cbor_write_map(nu54_cbor_writer_t *w, const nu54_cbor_entry_t *entries, size_t n);

#endif /* NU54_CBOR_H */
