/* Internal to the session core (core/src/nu54_session*.c); not part of the public API.
 *
 * nu54_session.c        lifecycle, session.open/confirm/cancel, dispatch, the public calls
 * nu54_session_util.c   CBOR reply helpers, field access, time, nonce, setup.ack
 * nu54_session_channel.c the secure channel (payment-protocol.md 4.1) and the merchant proofs
 * nu54_session_setup.c  TimeAnchor, setup.operator, key generation and PIN, device.reset
 * nu54_session_pay.c    payment.identify/prepare, the payment button, device.paymentMode
 * nu54_session_limit.c  limit.change, its PIN and button
 */
#ifndef NU54_SESSION_INT_H
#define NU54_SESSION_INT_H

#include <stdint.h>
#include <string.h>

#include "nu54_cbor.h"
#include "nu54_secure.h"
#include "nu54_session.h"
#include "nu54_sig.h"

extern const uint8_t ss_zero_session[8];

/* ---- replies and fields (nu54_session_util.c) */
nu54_cbor_entry_t ss_text(const char *key, const char *s);
nu54_cbor_entry_t ss_bytes(const char *key, const uint8_t *p, size_t n);
/* Writes one body; `who` 0 = central, 1 = phone. */
void ss_emit(nu54_out_t *out, int who, const nu54_cbor_entry_t *e, size_t n);
void ss_error(nu54_out_t *out, const uint8_t sid[8], const char *reason);
const uint8_t *ss_reply_sid(const nu54_device_t *d);
void ss_refused(const nu54_device_t *d, nu54_out_t *out, const char *reason);
void ss_u256(const nu54_cbor_item_t *it, uint8_t out[32]);
uint64_t ss_u64(const nu54_cbor_item_t *it);
const nu54_cbor_item_t *ss_field(const nu54_msg_t *m, int map, const char *key);
int ss_same_session(const nu54_device_t *d, const nu54_msg_t *m);
int ss_in_session(const nu54_device_t *d, const nu54_msg_t *m, int setup);
int ss_device_time(const nu54_device_t *d, uint64_t now, uint64_t *t);
void ss_increment(uint8_t n[32]);
void ss_abort_pending(nu54_device_t *d);
void ss_last_anchor(const nu54_device_t *d, uint8_t last[32]);
void ss_setup_ack(const nu54_device_t *d, nu54_out_t *out, const char *step, int accepted, const char *reason, const uint8_t *device);
void ss_forget_setup(nu54_device_t *d);

/* ---- secure channel (nu54_session_channel.c) */
enum { DIR_KIOSK = 0x01, DIR_DEVICE = 0x02 };

/* The channel a message came in on: its replies go out under it even if the session ends. */
typedef struct {
	int on;
	int first; /* replies before this index were sealed by an earlier call */
	uint32_t id;
	uint8_t key[16];
	uint64_t tx;
} channel_t;

/* Opens a body from the kiosk under the channel: 0 and `plain` (len - 16 bytes), or -1 on a
 * failed tag (the caller closes the session). */
int ss_channel_open(nu54_device_t *d, const channel_t *c, const uint8_t *body, size_t len, uint8_t *plain, size_t cap);
void ss_drop_channel(nu54_device_t *d);
void ss_channel_begin(const nu54_device_t *d, channel_t *c, const nu54_out_t *out);
void ss_channel_end(nu54_device_t *d, channel_t *c, nu54_out_t *out);
int ss_operator_attestation(const nu54_device_t *d, const nu54_msg_t *m, int a, nu54_merchant_attestation_t *att);
int ss_kiosk_merchant(const nu54_device_t *d, const nu54_msg_t *m, uint8_t merchant[20]);
int ss_start_channel(nu54_device_t *d, const uint8_t kiosk_x[32], const uint8_t kiosk_nonce[32], uint8_t device_x[32]);

/* ---- message handlers */
void ss_time_anchor(nu54_device_t *d, const nu54_msg_t *m, uint64_t now, nu54_out_t *out);
void ss_setup_operator(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out);
void ss_setup_confirmed(nu54_device_t *d, int approve, nu54_out_t *out);
void ss_setup_pin(nu54_device_t *d, const char *pin, size_t len, nu54_out_t *out);
void ss_device_reset(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out);
void ss_identify(nu54_device_t *d, const nu54_msg_t *m, uint64_t now, nu54_out_t *out);
void ss_prepare(nu54_device_t *d, const nu54_msg_t *m, uint64_t now, nu54_out_t *out);
void ss_payment_button(nu54_device_t *d, int approve, nu54_out_t *out);
void ss_payment_mode(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out);
void ss_limit_change(nu54_device_t *d, const nu54_msg_t *m, uint64_t now, nu54_out_t *out);
void ss_limit_pin(nu54_device_t *d, const char *pin, size_t len, nu54_out_t *out);
void ss_limit_confirmed(nu54_device_t *d, int approve, nu54_out_t *out);

#endif /* NU54_SESSION_INT_H */
