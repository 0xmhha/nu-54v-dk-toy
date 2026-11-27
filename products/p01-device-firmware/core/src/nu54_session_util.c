#include "nu54_session_int.h"

const uint8_t ss_zero_session[8] = {0};

size_t nu54_pin_entry_body(const char digits[5], int position, uint8_t *out, size_t cap)
{
	nu54_cbor_writer_t w;
	nu54_cbor_entry_t e[] = {{"v", NU54_V_UINT, 0, 0, 1}, ss_text("type", "pin.entry"), ss_bytes("sessionId", ss_zero_session, 8),
				 ss_text("digits", digits), {"position", NU54_V_UINT, 0, 0, (uint64_t)position}};
	nu54_cbor_writer_init(&w, out, cap);
	return nu54_cbor_write_map(&w, e, 5) == 0 ? w.len : 0;
}

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
		nu54_cbor_writer_init(&w, out->phone, sizeof(out->phone));
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

int ss_rental_time(const nu54_device_t *d, uint64_t *now)
{
	if (!d->anchored || !d->cfg.platform.monotonic_seconds) return -1;
	uint64_t local = d->cfg.platform.monotonic_seconds(d->cfg.platform.ctx);
	if (local < d->anchor_at || local - d->anchor_at > UINT64_MAX - d->anchor_timestamp) return -1;
	*now = d->anchor_timestamp + (local - d->anchor_at);
	return *now ? 0 : -1;
}

static int rental_credential_permits(const nu54_device_t *d, uint64_t approval_expiry)
{
	const nu54_rental_credential_store_t *store = d->cfg.platform.rental_credential_store;
	if (!d->cfg.platform.rental_credential_required) return 1;
	if (!store) return 0;
	nu54_rental_credential_t credential;
	uint64_t checked_at;
	return nu54_rental_credential_current(store, &d->rental_journal->expected_binding,
					      d->operator_address, &credential, &checked_at) == 0 &&
	       (!approval_expiry || (checked_at <= approval_expiry && approval_expiry < credential.expires_at));
}

int ss_rental_can_prompt(nu54_device_t *d)
{
	nu54_rental_journal_t *journal = d->rental_journal;
	if (!journal) return !d->rental_journal_required && !d->cfg.platform.rental_credential_required;
	return (journal->loaded || nu54_rental_journal_reload(journal) == 0) && !journal->image.frozen &&
	       rental_credential_permits(d, 0);
}

int ss_rental_nonce(nu54_device_t *d, nu54_rental_journal_kind_t kind, const uint8_t request[32], uint8_t nonce[32])
{
	if (!d->rental_journal) {
		if (d->rental_journal_required || d->cfg.platform.rental_credential_required) return -1;
		memcpy(nonce, d->next_nonce, 32);
		return 0;
	}
	if (!ss_rental_can_prompt(d)) return -1;
	return nu54_rental_journal_nonce(d->rental_journal, kind, request, nonce);
}

int ss_rental_sign(nu54_device_t *d, nu54_rental_journal_kind_t kind, const nu54_rental_journal_request_t *request,
		   uint64_t expiry, const uint8_t digest[32], const uint8_t nonce[32], uint8_t signature[65])
{
	nu54_rental_journal_t *journal = d->rental_journal;
	if (journal) {
		uint64_t now;
		if (ss_rental_time(d, &now) != 0 || now > expiry || !rental_credential_permits(d, expiry)) return -1;
		int state = nu54_rental_journal_reserve(journal, kind, request, expiry, digest, nonce, signature);
		if (state == NU54_RENTAL_JOURNAL_REPLAY) return rental_credential_permits(d, expiry) ? 0 : -1;
		if (state != NU54_RENTAL_JOURNAL_RESERVED) return -1;
		if (!rental_credential_permits(d, expiry)) {
			(void)nu54_rental_journal_finish(journal, nonce, NULL);
			return -1;
		}
	} else {
		if (d->rental_journal_required || d->cfg.platform.rental_credential_required) return -1;
		ss_increment(d->next_nonce);
		if (d->cfg.platform.persist_nonce && d->cfg.platform.persist_nonce(d->cfg.platform.ctx, d->next_nonce) != 0)
			return -1;
	}
	uint8_t rs[64];
	int signed_ok = d->cfg.platform.sign(d->cfg.platform.ctx, digest, rs) == 0 &&
			nu54_sig_finish(digest, rs, d->address, signature) == NU54_SIG_OK;
	if (journal && !rental_credential_permits(d, expiry)) signed_ok = 0;
	if (journal && nu54_rental_journal_finish(journal, nonce, signed_ok ? signature : NULL) != 0) return -1;
	return signed_ok && (!journal || rental_credential_permits(d, expiry)) ? 0 : -1;
}

/* A setup that did not reach the PIN leaves nothing behind: drop the key it made. */
void ss_abort_pending(nu54_device_t *d)
{
	if (d->pending == NU54_PENDING_PIN) {
		int (*discard)(void *) = d->cfg.platform.discard_setup ? d->cfg.platform.discard_setup : d->cfg.platform.wipe;
		if (discard) discard(d->cfg.platform.ctx);
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
	d->rental_journal = NULL;
	d->rental_journal_required = 0;
	memset(&d->rental_access, 0, sizeof(d->rental_access));
	d->passkey = 0;
	d->state = NU54_STATE_UNPROVISIONED;
	d->anchored = 0;
	d->last_anchor = 0;
	d->session_open = 0;
	d->attested = 0;
	d->pending = NU54_PENDING_NONE;
}
