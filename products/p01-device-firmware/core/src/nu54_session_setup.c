#include "nu54_session_int.h"

void ss_time_anchor(nu54_device_t *d, const nu54_msg_t *m, uint64_t now, nu54_out_t *out)
{
	if (!ss_in_session(d, m, 1)) {
		ss_error(out, ss_field(m, 0, "sessionId")->ptr, "NOT_PERMITTED");
		return;
	}
	if (d->state == NU54_STATE_UNPROVISIONED) {
		uint8_t last[32];
		ss_last_anchor(d, last);
		nu54_cbor_entry_t e[] = {{"v", NU54_V_UINT, 0, 0, 1}, ss_text("type", "setup.ack"), ss_bytes("sessionId", ss_reply_sid(d), 8),
					 ss_text("step", "setup.timeAnchor"), {"accepted", NU54_V_BOOL, 0, 0, 0},
					 {"lastAnchor", NU54_V_UINT256, last, 32, 0}, ss_text("reason", "NOT_PERMITTED")};
		out->event = NU54_EVENT_REFUSED;
		ss_emit(out, 0, e, 7); /* no key and no operator to check against */
		return;
	}
	const nu54_cbor_item_t *dev = ss_field(m, 0, "device"), *ts = ss_field(m, 0, "timestamp"), *sig = ss_field(m, 0, "operatorSignature");
	nu54_time_anchor_t a;
	memcpy(a.device, dev->ptr, 20);
	a.timestamp = ss_u64(ts);
	uint8_t domain[32], h[32], digest[32], signer[20];
	nu54_eip712_domain_separator(d->chain_id, d->contract, domain);
	nu54_hash_time_anchor(&a, h);
	nu54_eip712_digest(domain, h, digest);
	int ok = ts->type == NU54_CBOR_UINT && nu54_sig_recover(digest, sig->ptr, sig->len, signer) == NU54_SIG_OK &&
		 memcmp(dev->ptr, d->address, 20) == 0 && memcmp(signer, d->operator_address, 20) == 0 && a.timestamp > d->last_anchor;
	if (ok) {
		d->anchored = 1;
		d->anchor_timestamp = a.timestamp;
		d->anchor_at = now;
		d->last_anchor = a.timestamp;
		if (d->state == NU54_STATE_PROVISIONED_NO_ANCHOR) {
			d->state = NU54_STATE_READY;
		}
	}
	uint8_t last[32];
	ss_last_anchor(d, last);
	nu54_cbor_entry_t e[] = {
		{"v", NU54_V_UINT, 0, 0, 1},
		ss_text("type", "setup.ack"),
		ss_bytes("sessionId", ss_reply_sid(d), 8),
		ss_text("step", "setup.timeAnchor"),
		{"accepted", NU54_V_BOOL, 0, 0, (uint64_t)ok},
		{"lastAnchor", NU54_V_UINT256, last, 32, 0},
		ss_text("reason", "NOT_PERMITTED"), /* refused anchors only */
	};
	if (!ok) {
		out->event = NU54_EVENT_REFUSED;
	}
	ss_emit(out, 0, e, ok ? 6 : 7);
}

/* setup.operator (payment-protocol.md 5, step 1): keep the values in RAM and wait for the button. */
void ss_setup_operator(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	/* Recording an operator needs a bonded session, whatever let the session open. */
	if (!ss_in_session(d, m, 1) || !d->session_bonded) {
		ss_error(out, ss_field(m, 0, "sessionId")->ptr, "NOT_PERMITTED");
		return;
	}
	const nu54_cbor_item_t *passkey = ss_field(m, 0, "passkey");
	if (d->state != NU54_STATE_UNPROVISIONED || passkey->type != NU54_CBOR_UINT || passkey->value > 999999 || !d->cfg.platform.generate_key) {
		ss_setup_ack(d, out, "setup.operator", 0, "NOT_PERMITTED", NULL); /* once only, six-digit passkey [N23] */
		return;
	}
	nu54_setup_record_t *rec = &d->pending_setup;
	memset(rec, 0, sizeof(*rec));
	memcpy(rec->operator_address, ss_field(m, 0, "operator")->ptr, 20);
	memcpy(rec->contract, ss_field(m, 0, "contract")->ptr, 20);
	ss_u256(ss_field(m, 0, "chainId"), rec->chain_id);
	rec->passkey = (uint32_t)passkey->value;
	d->pending = NU54_PENDING_SETUP_CONFIRM;
}

/* The button confirmed the operator values: ack, make the key and the nonce start, wait for the PIN. */
void ss_setup_confirmed(nu54_device_t *d, int approve, nu54_out_t *out)
{
	d->pending = NU54_PENDING_NONE;
	if (!approve) {
		ss_setup_ack(d, out, "setup.operator", 0, "USER_REJECTED", NULL);
		return;
	}
	ss_setup_ack(d, out, "setup.operator", 1, NULL, NULL);
	if (d->cfg.platform.generate_key(d->cfg.platform.ctx, d->pending_address) != 0) {
		ss_setup_ack(d, out, "keygen", 0, "NOT_PERMITTED", NULL);
		return;
	}
	/* 31 random bytes and a zero low byte: a multiple of 256 (payment-protocol.md 2). */
	d->cfg.platform.random(d->cfg.platform.ctx, d->pending_setup.nonce_start, 31);
	d->pending_setup.nonce_start[31] = 0;
	d->pending = NU54_PENDING_PIN;
}

/* device.reset (payment-protocol.md 5): an operator-signed DeviceReset for this device wipes it. */
void ss_device_reset(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	if (!ss_same_session(d, m)) {
		ss_error(out, ss_field(m, 0, "sessionId")->ptr, "NOT_PERMITTED");
		return;
	}
	const nu54_cbor_item_t *dev = ss_field(m, 0, "device"), *nonce = ss_field(m, 0, "nonce"), *sig = ss_field(m, 0, "operatorSignature");
	int ok = d->state != NU54_STATE_UNPROVISIONED && memcmp(dev->ptr, d->address, 20) == 0;
	if (ok) {
		nu54_device_reset_t r;
		memcpy(r.device, dev->ptr, 20);
		ss_u256(nonce, r.nonce);
		uint8_t domain[32], h[32], digest[32], signer[20];
		nu54_eip712_domain_separator(d->chain_id, d->contract, domain);
		nu54_hash_device_reset(&r, h);
		nu54_eip712_digest(domain, h, digest);
		ok = nu54_sig_recover(digest, sig->ptr, sig->len, signer) == NU54_SIG_OK && memcmp(signer, d->operator_address, 20) == 0 &&
		     (!d->cfg.platform.wipe || d->cfg.platform.wipe(d->cfg.platform.ctx) == 0);
	}
	if (!ok) {
		ss_setup_ack(d, out, "device.reset", 0, "NOT_PERMITTED", NULL);
		return;
	}
	ss_setup_ack(d, out, "device.reset", 1, NULL, NULL); /* answered in the session it came in */
	ss_forget_setup(d);                                    /* which ends with the wipe */
}

/* The PIN at setup (payment-protocol.md 5, step 2): store everything at once and announce the key. */
void ss_setup_pin(nu54_device_t *d, const char *pin, size_t len, nu54_out_t *out)
{
	nu54_setup_record_t *rec = &d->pending_setup;
	rec->pin = pin;
	rec->pin_len = len;
	if (!pin || d->cfg.platform.commit_setup(d->cfg.platform.ctx, rec) != 0) {
		ss_abort_pending(d); /* nothing is stored */
		ss_setup_ack(d, out, "keygen", 0, pin ? "NOT_PERMITTED" : "TIMEOUT", NULL);
		return;
	}
	d->pending = NU54_PENDING_NONE;
	memcpy(d->address, d->pending_address, 20);
	memcpy(d->operator_address, rec->operator_address, 20);
	memcpy(d->contract, rec->contract, 20);
	memcpy(d->chain_id, rec->chain_id, 32);
	d->passkey = rec->passkey;
	memcpy(d->next_nonce, rec->nonce_start, 32);
	rec->pin = NULL; /* the platform keeps the PIN; RAM does not */
	rec->pin_len = 0;
	d->state = NU54_STATE_PROVISIONED_NO_ANCHOR;
	ss_setup_ack(d, out, "keygen", 1, NULL, d->address);
}

/* device.info (payment-protocol.md 3): the bonded phone app asks for the state, to choose between
 * wallet setup and its home screen. No session; a link that is not bonded is NOT_PERMITTED. */
void ss_device_info(nu54_device_t *d, nu54_out_t *out)
{
	if (!d->call.bonded) {
		ss_error(out, ss_zero_session, "NOT_PERMITTED");
		return;
	}
	nu54_cbor_entry_t e[] = {
		{"v", NU54_V_UINT, 0, 0, 1},
		ss_text("type", "device.info.ack"),
		ss_bytes("sessionId", ss_zero_session, 8),
		ss_text("state", ss_state_name[d->state]),
		ss_text("firmware", d->cfg.firmware),
		{"anchorValid", NU54_V_BOOL, 0, 0, (uint64_t)d->anchored},
		ss_bytes("device", d->address, 20), /* an UNPROVISIONED device has no key yet */
	};
	ss_emit(out, 0, e, d->state == NU54_STATE_UNPROVISIONED ? 6 : 7);
}

/* wallet.check.result{accepted[, signature][, reason]} with the zero session id. */
static void wallet_result(nu54_out_t *out, int who, const uint8_t *sig, const char *reason)
{
	nu54_cbor_entry_t e[5] = {{"v", NU54_V_UINT, 0, 0, 1}, ss_text("type", "wallet.check.result"), ss_bytes("sessionId", ss_zero_session, 8),
				  {"accepted", NU54_V_BOOL, 0, 0, (uint64_t)(sig != NULL)}};
	e[4] = sig ? ss_bytes("signature", sig, 65) : ss_text("reason", reason);
	if (!sig) {
		out->event = NU54_EVENT_REFUSED;
	}
	ss_emit(out, who, e, 5);
}

/* wallet.check (payment-protocol.md 5): the bonded phone app checks the new key. The device waits
 * for the approve button, then signs WalletCheck{device, challenge}; the result goes to the phone
 * app. Refusals before the button answer the link that asked. */
void ss_wallet_check(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	/* One button at a time: a payment, setup or limit change that waits keeps it. */
	if (!d->call.bonded || d->state == NU54_STATE_UNPROVISIONED || d->pending != NU54_PENDING_NONE || !d->cfg.platform.sign) {
		wallet_result(out, 0, NULL, "NOT_PERMITTED");
		return;
	}
	if (d->state == NU54_STATE_PIN_LOCKED) {
		wallet_result(out, 0, NULL, "PIN_LOCKED"); /* every signature is refused */
		return;
	}
	memcpy(d->pending_challenge, ss_field(m, 0, "challenge")->ptr, 32);
	d->pending = NU54_PENDING_WALLET_CHECK;
}

void nu54_session_wallet_check_end(nu54_device_t *d, nu54_out_t *out)
{
	if (d->pending == NU54_PENDING_WALLET_CHECK) {
		d->pending = NU54_PENDING_NONE;
		wallet_result(out, 1, NULL, "TIMEOUT");
	}
}

void ss_wallet_check_button(nu54_device_t *d, int approve, nu54_out_t *out)
{
	d->pending = NU54_PENDING_NONE;
	if (!approve) {
		wallet_result(out, 1, NULL, "USER_REJECTED");
		return;
	}
	nu54_wallet_check_t w;
	uint8_t domain[32], h[32], digest[32], rs[64], sig[65];
	memcpy(w.device, d->address, 20);
	memcpy(w.challenge, d->pending_challenge, 32);
	nu54_eip712_domain_separator(d->chain_id, d->contract, domain);
	nu54_hash_wallet_check(&w, h);
	nu54_eip712_digest(domain, h, digest);
	if (d->cfg.platform.sign(d->cfg.platform.ctx, digest, rs) != 0 || nu54_sig_finish(digest, rs, d->address, sig) != NU54_SIG_OK) {
		wallet_result(out, 1, NULL, "NOT_PERMITTED");
		return;
	}
	wallet_result(out, 1, sig, NULL);
}
