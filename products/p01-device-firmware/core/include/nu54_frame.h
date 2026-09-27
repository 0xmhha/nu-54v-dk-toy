/*
 * BLE message framing from payment-protocol.md §4 ([N09]).
 *
 * envelope = length(u16 BE) | digest(8 bytes, SHA-256 prefix of body) | CBOR body
 * fragment = sequence(u8) | index(u8) | up to ATT_MTU - 5 envelope bytes
 *
 * `length` is the whole envelope length including the 10-byte header, so it
 * ranges from 10 to 2058 (body limit 2048).
 *
 * Board-independent: no Zephyr headers, so it builds and tests on the host.
 * The SHA-256 digest is checked by the caller with the platform crypto.
 */
#ifndef NU54_FRAME_H
#define NU54_FRAME_H

#include <stddef.h>
#include <stdint.h>

#define NU54_ENVELOPE_HEADER_LEN 10u
#define NU54_FRAGMENT_HEADER_LEN 2u
#define NU54_MAX_BODY_LEN 2048u
#define NU54_MAX_ENVELOPE_LEN (NU54_ENVELOPE_HEADER_LEN + NU54_MAX_BODY_LEN)

typedef enum {
	NU54_FRAME_NEED_MORE = 0, /* fragment accepted, message not complete */
	NU54_FRAME_DONE,          /* whole envelope received */
	NU54_FRAME_BAD,           /* order, length or size violation -> error{BAD_FRAME} */
} nu54_frame_result_t;

typedef struct {
	uint8_t buf[NU54_MAX_ENVELOPE_LEN];
	size_t have;         /* envelope bytes collected */
	size_t want;         /* total envelope length, 0 until known */
	uint8_t sequence;    /* sequence of the message being collected */
	uint8_t next_index;  /* expected fragment index */
	uint8_t active;      /* 1 while a message is being collected */
} nu54_reassembler_t;

/* Payload bytes per fragment for a negotiated ATT MTU (ATT_MTU - 5). Returns 0 if the MTU is too small. */
size_t nu54_fragment_payload(uint16_t att_mtu);

/* Write the 10-byte envelope header. Returns 0 on success, -1 if body_len is too large. */
int nu54_envelope_header(uint8_t out[NU54_ENVELOPE_HEADER_LEN], size_t body_len, const uint8_t digest8[8]);

void nu54_reassembler_reset(nu54_reassembler_t *r);

/* Feed one fragment as received from the GATT characteristic. */
nu54_frame_result_t nu54_reassembler_feed(nu54_reassembler_t *r, const uint8_t *frag, size_t len);

/* After NU54_FRAME_DONE: the CBOR body and the 8-byte digest carried in the envelope. */
const uint8_t *nu54_reassembler_body(const nu54_reassembler_t *r, size_t *body_len);
const uint8_t *nu54_reassembler_digest(const nu54_reassembler_t *r);

#endif /* NU54_FRAME_H */
