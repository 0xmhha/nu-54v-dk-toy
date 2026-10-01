/*
 * The device side of the payment and setup sessions (payment-protocol.md 5, 6).
 *
 * Messages arrive as CBOR bodies (after framing); replies are CBOR bodies for the central and,
 * for confirm.show, for the phone app. The device key never appears here: the platform signs
 * a digest and returns a raw (r, s), which nu54_sig_finish turns into the protocol form. The
 * button is asynchronous: payment.prepare that passes every check sends confirm.show and waits;
 * nu54_session_button() then answers payment.result. The behavior must match the shared
 * session vectors byte for byte (docs/content/specifications/protocol/session-vectors.json).
 * Board-independent: builds and tests on the host.
 */
#ifndef NU54_SESSION_H
#define NU54_SESSION_H

#include <stddef.h>
#include <stdint.h>

#include "nu54_eip712.h"

#define NU54_OUT_MAX 512

typedef enum {
	NU54_STATE_UNPROVISIONED,
	NU54_STATE_PROVISIONED_NO_ANCHOR,
	NU54_STATE_READY,
	NU54_STATE_PIN_LOCKED,
} nu54_state_t;

typedef struct {
	/* Signs a 32-byte digest with the device key; writes raw r || s. Returns 0 on success. */
	int (*sign)(void *ctx, const uint8_t digest[32], uint8_t rs[64]);
	/* Fills `out` with random bytes (TRNG on the device). */
	void (*random)(void *ctx, uint8_t *out, size_t n);
	void *ctx;
} nu54_platform_t;

/* Replies of one call: up to two bodies for the central, one for the phone app. */
typedef struct {
	uint8_t kiosk[2][NU54_OUT_MAX];
	size_t kiosk_len[2];
	int kiosk_count;
	uint8_t phone[NU54_OUT_MAX];
	size_t phone_len;
	int phone_count;
} nu54_out_t;

typedef struct {
	/* Setup values (setup.operator and key generation). */
	uint8_t address[20];
	uint8_t operator_address[20];
	uint8_t contract[20];
	uint8_t chain_id[32];
	uint32_t anchor_clock_skew;
	uint32_t authorization_expiry;
	const char *firmware;
	nu54_platform_t platform;

	/* State. */
	nu54_state_t state;
	int anchored;
	uint64_t anchor_timestamp; /* accepted TimeAnchor */
	uint64_t anchor_at;        /* local clock when it was accepted */
	uint64_t last_anchor;
	uint8_t next_nonce[32]; /* sequential nonce, 256-bit big-endian */

	int session_open;
	uint8_t session_id[8];
	int session_setup; /* 1 setup, 0 payment */
	int confirmed;
	uint8_t device_nonce[32];

	int attested;
	uint8_t att_merchant[20];
	uint8_t att_payout[20];
	char att_name[128];
	size_t att_name_len;

	int pending; /* waiting for the button */
	nu54_payment_authorization_t pending_auth;
} nu54_device_t;

/* `nonce_start` is the 32-byte big-endian first nonce (a multiple of 256). */
void nu54_device_init(nu54_device_t *d, const uint8_t nonce_start[32]);

/* A RAM-clearing reset: the anchor and the session are lost; key and deposit stay. */
void nu54_device_power_cycle(nu54_device_t *d);

/* Handles one CBOR body from a central at local time `now` (seconds). */
void nu54_session_handle(nu54_device_t *d, const uint8_t *body, size_t len, uint64_t now, nu54_out_t *out);

/* The renter's button after confirm.show: approve (1) signs, reject (0) refuses. */
void nu54_session_button(nu54_device_t *d, int approve, nu54_out_t *out);

#endif /* NU54_SESSION_H */
