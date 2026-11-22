#include "nu54_session_int.h"

static void limit_refused(const nu54_device_t *d, nu54_out_t *out, const char *reason)
{
	out->event = NU54_EVENT_REFUSED;
	nu54_cbor_entry_t e[] = {{"v", NU54_V_UINT, 0, 0, 1}, ss_text("type", "limit.result"), ss_bytes("sessionId", ss_reply_sid(d), 8),
				 ss_text("outcome", "refused"), ss_text("reason", reason)};
	ss_emit(out, 0, e, 5);
}

/* limit.change (payment-protocol.md 5): checks, confirm.limit to the phone, then the PIN. */
void ss_limit_change(nu54_device_t *d, const nu54_msg_t *m, uint64_t now, nu54_out_t *out)
{
	uint64_t t;
	if (!ss_in_session(d, m, 0)) {
		ss_error(out, ss_field(m, 0, "sessionId")->ptr, "NOT_PERMITTED");
		return;
	}
	if (d->state == NU54_STATE_PIN_LOCKED) {
		limit_refused(d, out, "PIN_LOCKED");
		return;
	}
	if (!ss_device_time(d, now, &t) || d->state != NU54_STATE_READY) {
		limit_refused(d, out, "TIME_ANCHOR_MISSING");
		return;
	}
	if (!ss_rental_can_prompt(d)) {
		limit_refused(d, out, "NOT_PERMITTED");
		return;
	}
	int c = nu54_msg_find(m, 0, "change");
	nu54_limit_change_t *l = &d->pending_limit;
	memset(l, 0, sizeof(*l));
	ss_u256(ss_field(m, c, "chainId"), l->chain_id);
	memcpy(l->contract, ss_field(m, c, "contract")->ptr, 20);
	ss_u256(ss_field(m, c, "perPaymentLimit"), l->per_payment_limit);
	ss_u256(ss_field(m, c, "dailyLimit"), l->daily_limit);
	const nu54_cbor_item_t *expiry = ss_field(m, c, "expiry");
	l->expiry = ss_u64(expiry);
	if (memcmp(l->chain_id, d->chain_id, 32) != 0 || memcmp(l->contract, d->contract, 20) != 0 || !d->cfg.platform.check_pin) {
		limit_refused(d, out, "NOT_PERMITTED");
		return;
	}
	if (expiry->type != NU54_CBOR_UINT || l->expiry < t || l->expiry > t + d->cfg.authorization_expiry) {
		limit_refused(d, out, "ATTESTATION_EXPIRED");
		return;
	}
	if (d->cfg.require_phone && !d->call.phone_present) {
		limit_refused(d, out, "NOT_PERMITTED");
		return;
	}
	nu54_cbor_entry_t e[] = {
		{"v", NU54_V_UINT, 0, 0, 1},
		ss_text("type", "confirm.limit"),
		ss_bytes("sessionId", d->session_id, 8),
		{"perPaymentLimit", NU54_V_UINT256, l->per_payment_limit, 32, 0},
		{"dailyLimit", NU54_V_UINT256, l->daily_limit, 32, 0},
		{"expiry", NU54_V_UINT, 0, 0, l->expiry},
	};
	ss_emit(out, 1, e, 6);
	d->pending = NU54_PENDING_LIMIT_PIN;
}

void ss_limit_pin(nu54_device_t *d, const char *pin, size_t len, nu54_out_t *out)
{
	d->pending = NU54_PENDING_NONE;
	if (!pin) {
		limit_refused(d, out, "TIMEOUT");
		return;
	}
	int r = d->cfg.platform.check_pin(d->cfg.platform.ctx, pin, len);
	if (r == NU54_PIN_LOCKED) {
		d->state = NU54_STATE_PIN_LOCKED;
		limit_refused(d, out, "PIN_LOCKED");
	} else if (r != NU54_PIN_OK) {
		limit_refused(d, out, "NOT_PERMITTED");
	} else {
		d->pending = NU54_PENDING_LIMIT_CONFIRM;
	}
}

void ss_limit_confirmed(nu54_device_t *d, int approve, nu54_out_t *out)
{
	d->pending = NU54_PENDING_NONE;
	if (!approve) {
		limit_refused(d, out, "USER_REJECTED");
		return;
	}
	nu54_limit_change_t l = d->pending_limit;
	uint8_t domain[32], h[32], digest[32], sig[65];
	nu54_rental_journal_request_t request = {0};
	memset(l.nonce, 0, sizeof(l.nonce));
	nu54_hash_limit_change(&l, request.key);
	if (ss_rental_nonce(d, NU54_RENTAL_LIMIT, request.key, l.nonce) != 0) {
		limit_refused(d, out, "NOT_PERMITTED");
		return;
	}
	nu54_eip712_domain_separator(d->chain_id, d->contract, domain);
	nu54_hash_limit_change(&l, h);
	nu54_eip712_digest(domain, h, digest);
	if (ss_rental_sign(d, NU54_RENTAL_LIMIT, &request, l.expiry, digest, l.nonce, sig) != 0) {
		limit_refused(d, out, "NOT_PERMITTED");
		return;
	}
	nu54_cbor_entry_t e[] = {
		{"v", NU54_V_UINT, 0, 0, 1},
		ss_text("type", "limit.result"),
		ss_bytes("sessionId", ss_reply_sid(d), 8),
		ss_text("outcome", "approved"),
		ss_bytes("signature", sig, 65),
		{"nonce", NU54_V_UINT256, l.nonce, 32, 0},
	};
	out->event = NU54_EVENT_SIGNED;
	ss_emit(out, 0, e, 6);
}
