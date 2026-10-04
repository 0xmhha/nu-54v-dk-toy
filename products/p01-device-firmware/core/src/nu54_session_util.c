#include "nu54_session_int.h"

const uint8_t ss_zero_session[8] = {0};

nu54_cbor_entry_t ss_text(const char *key, const char *s)
{
	return (nu54_cbor_entry_t){key, NU54_V_TEXT, (const uint8_t *)s, strlen(s), 0};
}

nu54_cbor_entry_t ss_bytes(const char *key, const uint8_t *p, size_t n)
{
	return (nu54_cbor_entry_t){key, NU54_V_BYTES, p, n, 0};
}

/* Writes one body; `who` 0 = central, 1 = phone. */
void ss_emit(nu54_out_t *out, int who, const nu54_cbor_entry_t *e, size_t n)
{
	nu54_cbor_writer_t w;
	if (who == 0) {
		if (out->kiosk_count >= 2) {
			return;
		}
		/* Room for the GCM tag when the reply goes out under the secure channel. */
		nu54_cbor_writer_init(&w, out->kiosk[out->kiosk_count], NU54_OUT_MAX - NU54_GCM_TAG_LEN);
		if (nu54_cbor_write_map(&w, e, n) == 0) {
			out->kiosk_len[out->kiosk_count++] = w.len;
		}
	} else {
		nu54_cbor_writer_init(&w, out->phone, NU54_OUT_MAX);
		if (nu54_cbor_write_map(&w, e, n) == 0) {
			out->phone_len = w.len;
			out->phone_count = 1;
		}
	}
}

void ss_error(nu54_out_t *out, const uint8_t sid[8], const char *reason)
{
	out->event = NU54_EVENT_REFUSED;
	nu54_cbor_entry_t e[] = {{"v", NU54_V_UINT, 0, 0, 1}, ss_text("type", "error"), ss_bytes("sessionId", sid, 8), ss_text("reason", reason)};
	ss_emit(out, 0, e, 4);
}

const uint8_t *ss_reply_sid(const nu54_device_t *d)
{
	return d->session_open ? d->session_id : ss_zero_session;
}

void ss_refused(const nu54_device_t *d, nu54_out_t *out, const char *reason)
{
	out->event = NU54_EVENT_REFUSED;
	nu54_cbor_entry_t e[] = {{"v", NU54_V_UINT, 0, 0, 1}, ss_text("type", "payment.result"), ss_bytes("sessionId", ss_reply_sid(d), 8),
				 ss_text("outcome", "refused"), ss_text("reason", reason)};
	ss_emit(out, 0, e, 5);
}

/* 32-byte big-endian value of a uint item (integer or tag-2 bignum). */
void ss_u256(const nu54_cbor_item_t *it, uint8_t out[32])
{
	memset(out, 0, 32);
	if (it->type == NU54_CBOR_UINT) {
		for (int k = 0; k < 8; k++) {
			out[31 - k] = (uint8_t)(it->value >> (8 * k));
		}
	} else {
		memcpy(&out[32 - it->len], it->ptr, it->len);
	}
}

/* uint item as uint64; a bignum does not fit and reads as UINT64_MAX. */
uint64_t ss_u64(const nu54_cbor_item_t *it)
{
	return it->type == NU54_CBOR_UINT ? it->value : UINT64_MAX;
}

const nu54_cbor_item_t *ss_field(const nu54_msg_t *m, int map, const char *key)
{
	int i = nu54_msg_find(m, map, key);
	return i < 0 ? NULL : &m->items[i];
}

int ss_same_session(const nu54_device_t *d, const nu54_msg_t *m)
{
	const nu54_cbor_item_t *s = ss_field(m, 0, "sessionId");
	return d->session_open && memcmp(s->ptr, d->session_id, 8) == 0;
}

int ss_in_session(const nu54_device_t *d, const nu54_msg_t *m, int setup)
{
	return ss_same_session(d, m) && d->session_setup == setup && (setup || d->confirmed);
}

int ss_device_time(const nu54_device_t *d, uint64_t now, uint64_t *t)
{
	if (!d->anchored) {
		return 0;
	}
	*t = d->anchor_timestamp + (now - d->anchor_at);
	return 1;
}

void ss_increment(uint8_t n[32])
{
	for (int i = 31; i >= 0 && ++n[i] == 0; i--) {
	}
}

/* A setup that did not reach the PIN leaves nothing behind: drop the key it made. */
void ss_abort_pending(nu54_device_t *d)
{
	if (d->pending == NU54_PENDING_PIN && d->cfg.platform.wipe) {
		d->cfg.platform.wipe(d->cfg.platform.ctx);
	}
	d->pending = NU54_PENDING_NONE;
}

void ss_last_anchor(const nu54_device_t *d, uint8_t last[32])
{
	memset(last, 0, 32);
	for (int k = 0; k < 8; k++) {
		last[31 - k] = (uint8_t)(d->last_anchor >> (8 * k));
	}
}

/* setup.ack{step, accepted[, reason][, device]}. */
void ss_setup_ack(const nu54_device_t *d, nu54_out_t *out, const char *step, int accepted, const char *reason, const uint8_t *device)
{
	nu54_cbor_entry_t e[6] = {{"v", NU54_V_UINT, 0, 0, 1}, ss_text("type", "setup.ack"), ss_bytes("sessionId", ss_reply_sid(d), 8),
				  ss_text("step", step), {"accepted", NU54_V_BOOL, 0, 0, (uint64_t)accepted}};
	size_t n = 5;
	if (reason) {
		e[n++] = ss_text("reason", reason);
	}
	if (device) {
		e[n++] = ss_bytes("device", device, 20);
	}
	if (!accepted) {
		out->event = NU54_EVENT_REFUSED;
	}
	ss_emit(out, 0, e, n);
}

/* Clears the key, setup values and anchor in RAM (the platform clears its storage). */
void ss_forget_setup(nu54_device_t *d)
{
	memset(d->address, 0, sizeof(d->address));
	memset(d->operator_address, 0, sizeof(d->operator_address));
	memset(d->contract, 0, sizeof(d->contract));
	memset(d->chain_id, 0, sizeof(d->chain_id));
	memset(d->next_nonce, 0, sizeof(d->next_nonce));
	d->passkey = 0;
	d->state = NU54_STATE_UNPROVISIONED;
	d->anchored = 0;
	d->last_anchor = 0;
	d->session_open = 0;
	d->attested = 0;
	d->pending = NU54_PENDING_NONE;
}
