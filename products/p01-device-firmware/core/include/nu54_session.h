/*
 * The device side of the payment and setup sessions (payment-protocol.md 5, 6).
 *
 * Messages arrive as CBOR bodies (after framing); replies are CBOR bodies for the central and,
 * for confirm.show, for the phone app. The device key never appears here: the platform signs
 * a digest and returns a raw (r, s), which nu54_sig_finish turns into the protocol form. The
 * button is asynchronous: payment.prepare that passes every check sends confirm.show and waits;
 * nu54_session_button() then answers payment.result. Setup works the same way: setup.operator
 * waits for the button, then key generation waits for the PIN (nu54_session_pin()). A payment
 * session opened with a one-time key runs over the secure channel of section 4.1: every body
 * after session.open.ok is AES-GCM in both directions. The behavior must match the shared
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

/* Everything setup stores at once when the PIN is set (payment-protocol.md 5, step 2). */
typedef struct {
	uint8_t operator_address[20];
	uint8_t contract[20];
	uint8_t chain_id[32];
	uint32_t passkey;
	uint8_t nonce_start[32];
	const char *pin;
	size_t pin_len;
} nu54_setup_record_t;

typedef struct {
	/* Signs a 32-byte digest with the device key; writes raw r || s. Returns 0 on success. */
	int (*sign)(void *ctx, const uint8_t digest[32], uint8_t rs[64]);
	/* Fills `out` with random bytes (TRNG on the device). */
	void (*random)(void *ctx, uint8_t *out, size_t n);
	/* Stores the next nonce before a signature is made (payment-protocol.md 2), so a reset can
	 * never reuse one. Returns 0 on success; on failure nothing is signed. May be NULL in tests. */
	int (*persist_nonce)(void *ctx, const uint8_t next_nonce[32]);
	void *ctx;
	/* Setup (may be NULL on a device that is only provisioned in tests). generate_key makes a
	 * new key that is not kept until commit_setup stores it with the record; wipe deletes the
	 * key and every setup value (device.reset, or a setup that did not finish). Return 0 on
	 * success. */
	int (*generate_key)(void *ctx, uint8_t address[20]);
	int (*commit_setup)(void *ctx, const nu54_setup_record_t *record);
	int (*wipe)(void *ctx);
	/* Compares an entered PIN with the stored one and keeps the failure counter in secure
	 * storage (pinMaxRetries, survives RAM-clearing resets). Returns NU54_PIN_OK, NU54_PIN_WRONG,
	 * or NU54_PIN_LOCKED once the failures reach pinMaxRetries. May be NULL: limit changes refuse. */
	int (*check_pin)(void *ctx, const char *pin, size_t len);
	/* Secure channel (payment-protocol.md 4.1); NULL refuses secure sessions (NOT_PERMITTED).
	 * hkdf: HKDF-SHA256 to 16 bytes. aead_seal: AES-128-GCM with a 12-byte IV and no AAD, writes
	 * len + 16 bytes (ciphertext || tag). aead_open: checks the tag of len bytes and writes
	 * len - 16. Return 0 on success. */
	int (*hkdf)(void *ctx, const uint8_t ikm[32], const uint8_t salt[64], const char *info, uint8_t key[16]);
	int (*aead_seal)(void *ctx, const uint8_t key[16], const uint8_t iv[12], const uint8_t *in, size_t len, uint8_t *out);
	int (*aead_open)(void *ctx, const uint8_t key[16], const uint8_t iv[12], const uint8_t *in, size_t len, uint8_t *out);
} nu54_platform_t;

enum { NU54_PIN_OK = 0, NU54_PIN_WRONG = 1, NU54_PIN_LOCKED = 2 };

typedef enum {
	NU54_PENDING_NONE,
	NU54_PENDING_PAYMENT,       /* confirm.show sent; the button decides */
	NU54_PENDING_SETUP_CONFIRM, /* setup.operator received; the button confirms the values */
	NU54_PENDING_PIN,           /* key made; the renter enters the PIN */
	NU54_PENDING_LIMIT_PIN,     /* limit.change checked, confirm.limit sent; the renter enters the PIN */
	NU54_PENDING_LIMIT_CONFIRM, /* PIN right; the button approves or rejects the limit change */
} nu54_pending_t;

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
	uint32_t passkey;
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

	/* Secure channel of the open session (4.1); dropped with the session. */
	int require_secure; /* release build: plaintext payment sessions are refused */
	int secure;
	uint32_t channel_id;
	uint8_t channel_key[16];
	uint64_t channel_rx, channel_tx;
	uint8_t secure_merchant[20]; /* the merchant session.open proved */

	nu54_pending_t pending;
	nu54_payment_authorization_t pending_auth;
	nu54_setup_record_t pending_setup; /* setup values in RAM until the PIN commits them */
	nu54_limit_change_t pending_limit;
	uint8_t pending_address[20];
} nu54_device_t;

/* A provisioned device (address and setup values already set). `nonce_start` is the 32-byte
 * big-endian first nonce (a multiple of 256). */
void nu54_device_init(nu54_device_t *d, const uint8_t nonce_start[32]);

/* A device without key or setup values (UNPROVISIONED), waiting for a setup session. */
void nu54_device_init_unprovisioned(nu54_device_t *d);

/* A RAM-clearing reset: the anchor and the session are lost; key and deposit stay. */
void nu54_device_power_cycle(nu54_device_t *d);

/* The link to the central dropped: the session ends, and a setup waiting for the PIN stores
 * nothing (payment-protocol.md 5, the generated key is wiped). */
void nu54_session_link_closed(nu54_device_t *d);

/* Handles one body from a central at local time `now` (seconds): CBOR, or in a secure session
 * AES-GCM over CBOR. Replies to the central are sealed the same way; bodies for the phone app
 * are plain CBOR (the bonded link protects them). */
void nu54_session_handle(nu54_device_t *d, const uint8_t *body, size_t len, uint64_t now, nu54_out_t *out);

/* The renter's button: after confirm.show approve (1) signs and reject (0) refuses; after
 * setup.operator it confirms or refuses the operator values; after the PIN of a limit change it
 * approves or rejects the change. */
void nu54_session_button(nu54_device_t *d, int approve, nu54_out_t *out);

/* The PIN the renter entered on the buttons (digits), or NULL when it was not entered in time.
 * At setup it stores the setup and answers the keygen ack; for a limit change it is checked
 * against the stored PIN before the button. */
void nu54_session_pin(nu54_device_t *d, const char *pin, size_t len, nu54_out_t *out);

#endif /* NU54_SESSION_H */
