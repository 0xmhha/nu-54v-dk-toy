#include "nu54_session_int.h"

void ss_identify(nu54_device_t *d, const nu54_msg_t *m, uint64_t now, nu54_out_t *out)
{
	uint64_t t;
	if (!ss_in_session(d, m, 0)) {
		ss_error(out, ss_field(m, 0, "sessionId")->ptr, "NOT_PERMITTED");
		return;
	}
	if (!ss_device_time(d, now, &t)) {
		ss_refused(d, out, "TIME_ANCHOR_MISSING");
		return;
	}
	nu54_merchant_attestation_t att;
	/* A secure session serves only the merchant its session.open proved. */
	if (!ss_operator_attestation(d, m, nu54_msg_find(m, 0, "attestation"), &att) || (d->secure && memcmp(att.merchant, d->secure_merchant, 20) != 0)) {
		ss_refused(d, out, "MERCHANT_FORGED");
		return;
	}
	uint64_t skew = d->cfg.anchor_clock_skew;
	if (t + skew < att.valid_from || (t > skew && t - skew > att.valid_until) || att.name_len >= sizeof(d->att_name)) {
		ss_refused(d, out, "ATTESTATION_EXPIRED");
		return;
	}
	memcpy(d->att_merchant, att.merchant, 20);
	memcpy(d->att_payout, att.payout, 20);
	memcpy(d->att_name, att.name, att.name_len);
	d->att_name_len = att.name_len;
	d->att_name[att.name_len] = 0;
	d->attested = 1; /* accepted: no reply, the device answers payment.prepare */
}

void ss_prepare(nu54_device_t *d, const nu54_msg_t *m, uint64_t now, nu54_out_t *out)
{
	uint64_t t;
	if (!ss_in_session(d, m, 0)) {
		ss_error(out, ss_field(m, 0, "sessionId")->ptr, "NOT_PERMITTED");
		return;
	}
	if (d->state == NU54_STATE_PIN_LOCKED) {
		ss_refused(d, out, "PIN_LOCKED"); /* every signature is refused */
		return;
	}
	if (!ss_device_time(d, now, &t) || d->state != NU54_STATE_READY) {
		ss_refused(d, out, "TIME_ANCHOR_MISSING");
		return;
	}
	if (!d->attested) {
		ss_refused(d, out, "MERCHANT_FORGED");
		return;
	}
	int a = nu54_msg_find(m, 0, "authorization");
	nu54_payment_authorization_t p;
	ss_u256(ss_field(m, a, "chainId"), p.chain_id);
	memcpy(p.contract, ss_field(m, a, "contract")->ptr, 20);
	memcpy(p.merchant, ss_field(m, a, "merchant")->ptr, 20);
	memcpy(p.payout, ss_field(m, a, "payout")->ptr, 20);
	memcpy(p.token, ss_field(m, a, "token")->ptr, 20);
	ss_u256(ss_field(m, a, "amount"), p.amount);
	memcpy(p.order_id, ss_field(m, a, "orderId")->ptr, 32);
	const nu54_cbor_item_t *expiry = ss_field(m, a, "expiry");
	p.expiry = ss_u64(expiry);

	nu54_merchant_order_t o;
	memcpy(o.order_id, p.order_id, 32);
	memcpy(o.token, p.token, 20);
	memcpy(o.amount, p.amount, 32);
	memcpy(o.payout, p.payout, 20);
	o.expiry = p.expiry;
	uint8_t domain[32], h[32], digest[32], signer[20];
	nu54_eip712_domain_separator(d->chain_id, d->contract, domain);
	nu54_hash_merchant_order(&o, h);
	nu54_eip712_digest(domain, h, digest);
	const nu54_cbor_item_t *msig = ss_field(m, 0, "merchantSignature");
	if (expiry->type != NU54_CBOR_UINT || nu54_sig_recover(digest, msig->ptr, msig->len, signer) != NU54_SIG_OK ||
	    memcmp(signer, d->att_merchant, 20) != 0 || memcmp(p.merchant, d->att_merchant, 20) != 0 ||
	    memcmp(p.payout, d->att_payout, 20) != 0 || memcmp(p.chain_id, d->chain_id, 32) != 0 ||
	    memcmp(p.contract, d->contract, 20) != 0) {
		ss_refused(d, out, "MERCHANT_FORGED");
		return;
	}
	if (p.expiry < t || p.expiry > t + d->cfg.authorization_expiry) {
		ss_refused(d, out, "ATTESTATION_EXPIRED");
		return;
	}
	/* confirm.show to the phone app, then wait for the button; a release device with no phone to
	 * show it on refuses (payment-protocol.md 3). */
	if (d->cfg.require_phone && !d->call.phone_present) {
		ss_refused(d, out, "NOT_PERMITTED");
		return;
	}
	nu54_cbor_entry_t e[] = {
		{"v", NU54_V_UINT, 0, 0, 1},
		ss_text("type", "confirm.show"),
		ss_bytes("sessionId", d->session_id, 8),
		{"merchantName", NU54_V_TEXT, (const uint8_t *)d->att_name, d->att_name_len, 0},
		ss_bytes("orderId", p.order_id, 32),
		ss_bytes("token", p.token, 20),
		ss_bytes("payout", p.payout, 20),
		{"amount", NU54_V_UINT256, p.amount, 32, 0},
	};
	ss_emit(out, 1, e, 8);
	d->pending_auth = p;
	d->pending = NU54_PENDING_PAYMENT;
}

/* device.paymentMode (payment-protocol.md 3): the bonded phone app turns payment advertising on
 * or off. No session: the reply carries the zero id and an open session is left alone. */
void ss_payment_mode(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	const nu54_cbor_item_t *on = ss_field(m, 0, "on"), *secs = ss_field(m, 0, "seconds");
	int want = on->value != 0;
	uint64_t s = ss_u64(secs);
	int ok = d->call.bonded && d->state != NU54_STATE_UNPROVISIONED && d->cfg.platform.payment_mode && (!want || (s >= 1 && s <= 300)) &&
		 d->cfg.platform.payment_mode(d->cfg.platform.ctx, want, want ? (uint32_t)s : 0) == 0;
	nu54_cbor_entry_t e[] = {{"v", NU54_V_UINT, 0, 0, 1}, ss_text("type", "device.paymentMode.ack"), ss_bytes("sessionId", ss_zero_session, 8),
				 {"accepted", NU54_V_BOOL, 0, 0, (uint64_t)ok}, {"on", NU54_V_BOOL, 0, 0, (uint64_t)want}, ss_text("reason", "NOT_PERMITTED")};
	if (!ok) {
		out->event = NU54_EVENT_REFUSED;
	}
	ss_emit(out, 0, e, ok ? 5 : 6);
}

/* The renter's button on a payment shown on the phone: approve signs with the next nonce. */
void ss_payment_button(nu54_device_t *d, int approve, nu54_out_t *out)
{
	d->pending = NU54_PENDING_NONE;
	if (!approve) {
		ss_refused(d, out, "USER_REJECTED");
		return;
	}
	nu54_payment_authorization_t p = d->pending_auth;
	memcpy(p.nonce, d->next_nonce, 32);
	ss_increment(d->next_nonce); /* sequential: consecutive nonces share one bitmap slot */
	if (d->cfg.platform.persist_nonce && d->cfg.platform.persist_nonce(d->cfg.platform.ctx, d->next_nonce) != 0) {
		ss_refused(d, out, "NOT_PERMITTED"); /* the counter must be stored before signing */
		return;
	}
	uint8_t domain[32], h[32], digest[32], rs[64], sig[65];
	nu54_eip712_domain_separator(d->chain_id, d->contract, domain);
	nu54_hash_payment_authorization(&p, h);
	nu54_eip712_digest(domain, h, digest);
	if (d->cfg.platform.sign(d->cfg.platform.ctx, digest, rs) != 0 || nu54_sig_finish(digest, rs, d->address, sig) != NU54_SIG_OK) {
		ss_refused(d, out, "NOT_PERMITTED");
		return;
	}
	nu54_cbor_entry_t e[] = {
		{"v", NU54_V_UINT, 0, 0, 1},
		ss_text("type", "payment.result"),
		ss_bytes("sessionId", ss_reply_sid(d), 8),
		ss_text("outcome", "approved"),
		ss_bytes("signature", sig, 65),
		{"nonce", NU54_V_UINT256, p.nonce, 32, 0},
	};
	out->event = NU54_EVENT_SIGNED;
	ss_emit(out, 0, e, 6);
}
